package logo

import (
	"bytes"
	"errors"
	"io"
	"sync"
)

// DefaultBackground is the dark canvas colour from the violet design tokens.
// Callers that follow the active theme should call SetBackground.
var DefaultBackground = [3]uint8{0x14, 0x12, 0x1E}

// Painter is an io.Writer for tea.WithOutput. It forwards every renderer
// write unchanged and then appends the logo image sequence, if a bitmap
// placement is enabled and configured. All state is mutex-protected because
// Bubble Tea's render goroutine and the model goroutine share the Painter.
type Painter struct {
	mu         sync.Mutex
	out        io.Writer
	capability Capability

	enabled      bool
	hasPlacement bool
	row          int
	col          int
	cols         int
	rows         int
	background   [3]uint8
	dark         bool

	lastDrawn    bool
	lastRow      int
	lastCol      int
	lastCols     int
	lastRows     int
	pendingErase bool
	eraseRow     int
	eraseCol     int
	eraseCols    int
	eraseRows    int

	pendingClear bool
	cacheReady   bool
	cacheKey     paintKey
	cached       []byte
}

type paintKey struct {
	cols       int
	rows       int
	background [3]uint8
	dark       bool
	protocol   Protocol
	cellWidth  int
	cellHeight int
}

// NewPainter wraps out. Drawing is disabled until SetEnabled(true); a caller
// should only enable it when capability.Available() is true.
func NewPainter(out io.Writer, capability Capability) *Painter {
	return &Painter{
		out:        out,
		capability: capability,
		background: DefaultBackground,
		dark:       true,
	}
}

// SetEnabled toggles bitmap output. Disabling a kitty placement deletes it
// immediately; disabling a previously drawn Sixel placement schedules a cell
// erase before the next frame so overlays never leave pixel residue.
func (p *Painter) SetEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enabled == enabled {
		return
	}
	if !enabled {
		if p.capability.Protocol == ProtocolKitty {
			p.writeClearLocked()
		} else if p.capability.Protocol == ProtocolSixel && p.lastDrawn {
			p.scheduleSixelEraseLocked()
		}
	}
	p.enabled = enabled
	p.pendingClear = false
}

// SetPlacement sets the zero-based top-left cell and the cell extent of the
// logo. Non-positive extents or negative coordinates hide it.
func (p *Painter) SetPlacement(row, col, cols, rows int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if row < 0 || col < 0 || cols <= 0 || rows <= 0 {
		if p.hasPlacement && p.enabled && p.capability.Protocol == ProtocolKitty {
			p.writeClearLocked()
		} else if p.capability.Protocol == ProtocolSixel && p.lastDrawn {
			p.scheduleSixelEraseLocked()
		}
		p.hasPlacement = false
		p.pendingClear = false
		return
	}

	hadPlacement := p.hasPlacement
	changed := !hadPlacement || p.row != row || p.col != col || p.cols != cols || p.rows != rows
	p.row, p.col, p.cols, p.rows = row, col, cols, rows
	p.hasPlacement = true
	if changed && hadPlacement && p.enabled {
		if p.capability.Protocol == ProtocolKitty {
			p.pendingClear = true
		} else if p.capability.Protocol == ProtocolSixel && p.lastDrawn {
			p.scheduleSixelEraseLocked()
		}
	}
}

// SetBackground updates the colour used for the area around the icon and
// invalidates the cached encoding.
func (p *Painter) SetBackground(background [3]uint8) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.background == background {
		return
	}
	p.background = background
	p.invalidateCacheLocked()
}

// SetDark switches between the dark and light logo assets and invalidates cache.
func (p *Painter) SetDark(dark bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dark == dark {
		return
	}
	p.dark = dark
	p.invalidateCacheLocked()
}

// Write forwards a renderer frame and then appends the image sequence.
func (p *Painter) Write(frame []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.out == nil {
		return len(frame), nil
	}
	if p.pendingClear {
		p.writeClearLocked()
		p.pendingClear = false
	}

	altExit := isAltScreenExit(frame)
	if altExit {
		p.writeClearLocked()
		p.enabled = false
		p.hasPlacement = false
		p.pendingClear = false
		p.pendingErase = false
		p.lastDrawn = false
	} else if p.pendingErase {
		p.pendingErase = false
		if p.capability.Protocol == ProtocolSixel {
			if seq := sixelEraseSequence(p.eraseRow, p.eraseCol, p.eraseCols, p.eraseRows, p.background); len(seq) > 0 {
				_, _ = p.out.Write(seq)
			}
		}
	}

	written, err := p.out.Write(frame)
	if err != nil {
		return written, err
	}
	if written != len(frame) {
		return written, io.ErrShortWrite
	}
	if !p.canDrawLocked() {
		return written, nil
	}

	logoBytes := p.encodedLocked()
	if len(logoBytes) == 0 {
		return written, nil
	}
	_, _ = p.out.Write(drawSequence(p.row, p.col, logoBytes))
	if p.capability.Protocol == ProtocolSixel {
		p.lastDrawn = true
		p.lastRow, p.lastCol, p.lastCols, p.lastRows = p.row, p.col, p.cols, p.rows
	}
	return written, nil
}

// Close removes a kitty placement and disables drawing.
func (p *Painter) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enabled && p.capability.Protocol == ProtocolKitty {
		p.writeClearLocked()
	}
	p.enabled = false
	p.hasPlacement = false
	p.pendingClear = false
	p.pendingErase = false
	p.lastDrawn = false
	p.invalidateCacheLocked()
	return nil
}

// Fd lets Bubble Tea treat the painter's underlying output as a terminal.
func (p *Painter) Fd() uintptr {
	p.mu.Lock()
	defer p.mu.Unlock()
	if file, ok := p.out.(interface{ Fd() uintptr }); ok {
		return file.Fd()
	}
	return ^uintptr(0)
}

// Read exists only to satisfy the term.File interface.
func (p *Painter) Read(buffer []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if reader, ok := p.out.(io.Reader); ok {
		return reader.Read(buffer)
	}
	return 0, errors.New("logo: painter output is not readable")
}

func (p *Painter) scheduleSixelEraseLocked() {
	p.pendingErase = true
	p.eraseRow = p.lastRow
	p.eraseCol = p.lastCol
	p.eraseCols = p.lastCols
	p.eraseRows = p.lastRows
	p.lastDrawn = false
}

func (p *Painter) canDrawLocked() bool {
	return p.enabled && p.hasPlacement && p.capability.Available()
}

func (p *Painter) encodedLocked() []byte {
	key := paintKey{
		cols:       p.cols,
		rows:       p.rows,
		background: p.background,
		dark:       p.dark,
		protocol:   p.capability.Protocol,
		cellWidth:  p.capability.CellWidth,
		cellHeight: p.capability.CellHeight,
	}
	if p.cacheReady && p.cacheKey == key {
		return p.cached
	}

	p.cacheReady = true
	p.cacheKey = key
	p.cached = nil
	encoded, err := p.capability.EncodeLogoForTheme(p.cols, p.rows, p.background, p.dark)
	if err != nil {
		return nil
	}
	p.cached = encoded
	return p.cached
}

func (p *Painter) invalidateCacheLocked() {
	p.cacheReady = false
	p.cacheKey = paintKey{}
	p.cached = nil
}

func (p *Painter) writeClearLocked() {
	if p.out == nil {
		return
	}
	clear := p.capability.ClearBytes()
	if len(clear) == 0 {
		return
	}
	_, _ = p.out.Write(clear)
}

func isAltScreenExit(frame []byte) bool {
	return bytes.Contains(frame, []byte("\x1b[?1049l")) || bytes.Contains(frame, []byte("\x1b[?1047l"))
}

var (
	_ io.Writer          = (*Painter)(nil)
	_ io.ReadWriteCloser = (*Painter)(nil)
	_ interface {
		io.ReadWriteCloser
		Fd() uintptr
	} = (*Painter)(nil)
)
