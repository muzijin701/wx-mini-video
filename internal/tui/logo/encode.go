package logo

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"
)

const (
	// kittyImageID identifies the single logo placement owned by this package.
	// ClearBytes deletes exactly this image.
	kittyImageID  = 7320
	kittyChunkLen = 4096

	// Keep a malformed placement or a very large terminal from allocating an
	// unbounded bitmap. The real brand area is a handful of cells.
	maxCanvasSide   = 4096
	maxCanvasPixels = 4 * 1024 * 1024
)

var (
	// ErrUnavailable is returned when a bitmap protocol was not detected.
	ErrUnavailable = errors.New("logo: terminal graphics unavailable")
	// ErrInvalidSize is returned for a non-positive or unreasonable canvas.
	ErrInvalidSize = errors.New("logo: invalid logo placement")
)

// EncodeLogo 使用深色变体渲染内置品牌图标。
func (c Capability) EncodeLogo(cols, rows int, bgRGB [3]uint8) ([]byte, error) {
	return c.EncodeLogoForTheme(cols, rows, bgRGB, true)
}

// EncodeLogoForTheme 按指定明暗变体渲染内置个人图标为 cols x rows 单元格的终端图像序列。
func (c Capability) EncodeLogoForTheme(cols, rows int, bgRGB [3]uint8, dark bool) ([]byte, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	if cols <= 0 || rows <= 0 {
		return nil, ErrInvalidSize
	}

	width64 := int64(cols) * int64(c.CellWidth)
	height64 := int64(rows) * int64(c.CellHeight)
	if width64 <= 0 || height64 <= 0 ||
		width64 > maxCanvasSide || height64 > maxCanvasSide ||
		width64*height64 > maxCanvasPixels {
		return nil, ErrInvalidSize
	}
	width, height := int(width64), int(height64)

	source, err := decodeLogo(dark)
	if err != nil {
		return nil, err
	}
	canvas := renderLogoCanvas(source, width, height, bgRGB)

	switch c.Protocol {
	case ProtocolSixel:
		return encodeSixel(canvas)
	case ProtocolKitty:
		return encodeKitty(canvas, cols, rows)
	default:
		return nil, ErrUnavailable
	}
}

// DrawBytes wraps an encoded logo in DECSC/DECRC and an absolute cursor
// position. row and col are zero-based terminal cells.
func (c Capability) DrawBytes(row, col, cols, rows int, bgRGB [3]uint8) ([]byte, error) {
	if row < 0 || col < 0 {
		return nil, ErrInvalidSize
	}
	imageBytes, err := c.EncodeLogo(cols, rows, bgRGB)
	if err != nil {
		return nil, err
	}
	return drawSequence(row, col, imageBytes), nil
}

// ClearBytes returns the protocol-specific cleanup sequence. Kitty deletes
// both the image data and its placements; Sixel has no equivalent deletion
// command, so it returns an empty slice.
func (c Capability) ClearBytes() []byte {
	if c.Protocol != ProtocolKitty {
		return nil
	}
	return []byte("\x1b_Ga=d,d=I,i=" + strconv.Itoa(kittyImageID) + ",q=2\x1b\\")
}

func drawSequence(row, col int, encoded []byte) []byte {
	sequence := make([]byte, 0, len(encoded)+32)
	sequence = append(sequence, "\x1b7\x1b["...)
	sequence = strconv.AppendInt(sequence, int64(row+1), 10)
	sequence = append(sequence, ';')
	sequence = strconv.AppendInt(sequence, int64(col+1), 10)
	sequence = append(sequence, 'H')
	sequence = append(sequence, encoded...)
	sequence = append(sequence, "\x1b8"...)
	return sequence
}

// sixelEraseSequence 用背景色空格覆写先前 Sixel 图像占用的单元格，防止关闭或移动图标后在浮层边缘残留像素。
func sixelEraseSequence(row, col, cols, rows int, bgRGB [3]uint8) []byte {
	if row < 0 || col < 0 || cols <= 0 || rows <= 0 {
		return nil
	}
	spaces := strings.Repeat(" ", cols)
	bgPrefix := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", bgRGB[0], bgRGB[1], bgRGB[2])
	sequence := make([]byte, 0, rows*(len(bgPrefix)+cols+16)+16)
	sequence = append(sequence, "\x1b7"...)
	for r := 0; r < rows; r++ {
		sequence = append(sequence, "\x1b["...)
		sequence = strconv.AppendInt(sequence, int64(row+r+1), 10)
		sequence = append(sequence, ';')
		sequence = strconv.AppendInt(sequence, int64(col+1), 10)
		sequence = append(sequence, 'H')
		sequence = append(sequence, bgPrefix...)
		sequence = append(sequence, spaces...)
	}
	sequence = append(sequence, "\x1b[0m\x1b8"...)
	return sequence
}

// renderLogoCanvas builds an opaque canvas and composites the scaled icon
// onto it. Gaps left by the square icon keep the requested background.
func renderLogoCanvas(source *image.RGBA, width, height int, bgRGB [3]uint8) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	background := color.RGBA{R: bgRGB[0], G: bgRGB[1], B: bgRGB[2], A: 255}
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)

	side := width
	if height < side {
		side = height
	}
	if side <= 0 {
		return canvas
	}
	drawScaledLogo(canvas, source, (width-side)/2, (height-side)/2, side)
	return canvas
}

// drawScaledLogo performs a separable Lanczos-2 resize directly into the
// destination. image.RGBA stores premultiplied alpha, so the resampled RGBA
// channels are clamped to valid premultiplied bounds before Source-Over
// compositing onto the opaque background.
func drawScaledLogo(dst *image.RGBA, source *image.RGBA, left, top, side int) {
	sourceBounds := source.Bounds()
	sourceWidth := sourceBounds.Dx()
	sourceHeight := sourceBounds.Dy()
	if sourceWidth <= 0 || sourceHeight <= 0 || side <= 0 {
		return
	}

	xTaps := buildLanczosTaps(sourceWidth, side)
	yTaps := buildLanczosTaps(sourceHeight, side)

	// Horizontal pass: side * sourceHeight premultiplied float RGBA pixels.
	horiz := make([]float64, side*sourceHeight*4)
	for sy := 0; sy < sourceHeight; sy++ {
		rowOffset := sy * side * 4
		for dx, tap := range xTaps {
			var r, g, b, a float64
			for i, w := range tap.weights {
				sample := source.RGBAAt(sourceBounds.Min.X+tap.start+i, sourceBounds.Min.Y+sy)
				r += float64(sample.R) * w
				g += float64(sample.G) * w
				b += float64(sample.B) * w
				a += float64(sample.A) * w
			}
			dstOff := rowOffset + dx*4
			horiz[dstOff] = r
			horiz[dstOff+1] = g
			horiz[dstOff+2] = b
			horiz[dstOff+3] = a
		}
	}

	// Vertical pass and Source-Over composition onto opaque background.
	for dy, tap := range yTaps {
		for dx := 0; dx < side; dx++ {
			var red, green, blue, alpha float64
			for i, w := range tap.weights {
				srcOff := ((tap.start+i)*side + dx) * 4
				red += horiz[srcOff] * w
				green += horiz[srcOff+1] * w
				blue += horiz[srcOff+2] * w
				alpha += horiz[srcOff+3] * w
			}

			alpha = clampFloat(alpha, 0, 255)
			red = clampFloat(red, 0, alpha)
			green = clampFloat(green, 0, alpha)
			blue = clampFloat(blue, 0, alpha)

			background := dst.RGBAAt(left+dx, top+dy)
			inverse := 1 - alpha/255
			dst.SetRGBA(left+dx, top+dy, color.RGBA{
				R: clampByte(red + float64(background.R)*inverse),
				G: clampByte(green + float64(background.G)*inverse),
				B: clampByte(blue + float64(background.B)*inverse),
				A: 255,
			})
		}
	}
}

type lanczosTap struct {
	start   int
	weights []float64
}

func buildLanczosTaps(srcSize, dstSize int) []lanczosTap {
	taps := make([]lanczosTap, dstSize)
	scale := float64(srcSize) / float64(dstSize)
	filterScale := 1.0
	if scale > 1.0 {
		filterScale = 1.0 / scale
	}
	radius := 2.0 / filterScale

	for dstIndex := 0; dstIndex < dstSize; dstIndex++ {
		center := (float64(dstIndex)+0.5)*scale - 0.5
		lo := int(math.Ceil(center - radius))
		hi := int(math.Floor(center + radius))
		if lo < 0 {
			lo = 0
		}
		if hi >= srcSize {
			hi = srcSize - 1
		}
		if hi < lo {
			clamped := int(math.Round(center))
			if clamped < 0 {
				clamped = 0
			} else if clamped >= srcSize {
				clamped = srcSize - 1
			}
			taps[dstIndex] = lanczosTap{start: clamped, weights: []float64{1}}
			continue
		}

		weights := make([]float64, hi-lo+1)
		var sum float64
		for i := range weights {
			w := lanczos2((float64(lo+i) - center) * filterScale)
			weights[i] = w
			sum += w
		}
		if sum == 0 {
			clamped := int(math.Round(center))
			if clamped < lo {
				clamped = lo
			} else if clamped > hi {
				clamped = hi
			}
			weights[clamped-lo] = 1
		} else {
			inv := 1.0 / sum
			for i := range weights {
				weights[i] *= inv
			}
		}
		taps[dstIndex] = lanczosTap{start: lo, weights: weights}
	}
	return taps
}

func lanczos2(x float64) float64 {
	if x < 0 {
		x = -x
	}
	if x == 0 {
		return 1
	}
	if x >= 2 {
		return 0
	}
	px := math.Pi * x
	return (math.Sin(px) / px) * (math.Sin(px/2) / (px / 2))
}

func clampFloat(value, minVal, maxVal float64) float64 {
	if value < minVal {
		return minVal
	}
	if value > maxVal {
		return maxVal
	}
	return value
}

func clampByte(value float64) uint8 {
	if value <= 0 {
		return 0
	}
	if value >= 255 {
		return 255
	}
	return uint8(value + 0.5)
}
