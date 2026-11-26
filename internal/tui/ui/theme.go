package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette 是紫色现代扁平化外观模式下的语义色 token。
type Palette struct {
	Primary       lipgloss.Color
	PrimaryHover  lipgloss.Color
	Accent        lipgloss.Color
	Foreground    lipgloss.Color
	Background    lipgloss.Color
	Surface       lipgloss.Color
	Panel         lipgloss.Color
	Success       lipgloss.Color
	Warning       lipgloss.Color
	Error         lipgloss.Color
	Muted         lipgloss.Color
	Line          lipgloss.Color
	EntryBG       lipgloss.Color
	ButtonFG      lipgloss.Color
	DisabledBG    lipgloss.Color
	DisabledFG    lipgloss.Color
	GradientStart lipgloss.Color
	GradientEnd   lipgloss.Color
}

// RGB 把 lipgloss 颜色解析为 RGB 三元组，供终端图片背景与原生 ANSI 转义使用。
func RGB(color lipgloss.Color) [3]uint8 {
	hex := strings.TrimPrefix(string(color), "#")
	var out [3]uint8
	if len(hex) != 6 {
		return out
	}
	var r, g, b int
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return out
	}
	return [3]uint8{uint8(r), uint8(g), uint8(b)}
}

// PaletteFor 返回对应明暗模式的紫色扁平色板。
func PaletteFor(dark bool) Palette {
	if dark {
		return Palette{
			Primary:       lipgloss.Color("#8B5CF6"),
			PrimaryHover:  lipgloss.Color("#7C3AED"),
			Accent:        lipgloss.Color("#C4B5FD"),
			Foreground:    lipgloss.Color("#F3F1FA"),
			Background:    lipgloss.Color("#14121E"),
			Surface:       lipgloss.Color("#1E1A2E"),
			Panel:         lipgloss.Color("#2A2440"),
			Success:       lipgloss.Color("#34D399"),
			Warning:       lipgloss.Color("#FBBF24"),
			Error:         lipgloss.Color("#F87171"),
			Muted:         lipgloss.Color("#9E95BA"),
			Line:          lipgloss.Color("#3E355C"),
			EntryBG:       lipgloss.Color("#1B1729"),
			ButtonFG:      lipgloss.Color("#FFFFFF"),
			DisabledBG:    lipgloss.Color("#1B1829"),
			DisabledFG:    lipgloss.Color("#635B7D"),
			GradientStart: lipgloss.Color("#6D28D9"),
			GradientEnd:   lipgloss.Color("#C4B5FD"),
		}
	}
	return Palette{
		Primary:       lipgloss.Color("#6D28D9"),
		PrimaryHover:  lipgloss.Color("#5B21B6"),
		Accent:        lipgloss.Color("#5B21B6"),
		Foreground:    lipgloss.Color("#1E1833"),
		Background:    lipgloss.Color("#F7F6FC"),
		Surface:       lipgloss.Color("#ECE8F8"),
		Panel:         lipgloss.Color("#E3DCF5"),
		Success:       lipgloss.Color("#059669"),
		Warning:       lipgloss.Color("#B45309"),
		Error:         lipgloss.Color("#DC2626"),
		Muted:         lipgloss.Color("#6B6189"),
		Line:          lipgloss.Color("#CFC6EA"),
		EntryBG:       lipgloss.Color("#FFFFFF"),
		ButtonFG:      lipgloss.Color("#FFFFFF"),
		DisabledBG:    lipgloss.Color("#EFEBF7"),
		DisabledFG:    lipgloss.Color("#A49BC0"),
		GradientStart: lipgloss.Color("#6D28D9"),
		GradientEnd:   lipgloss.Color("#DDD6FE"),
	}
}
