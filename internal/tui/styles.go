package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/MuZiJin701/wx-mini-video/internal/tui/ui"
)

// styles 是按当前色板构建的一组 lipgloss 样式；外观切换时整体重建。
//
// 约束：用于单行区域（页签、列表行、扁平按钮、状态提示、目标条切换按钮）的悬停样式
// 不得设置 Border 或非零 Padding 变化——那会改变物理行数与显示宽度，导致鼠标移动时
// 界面错乱。单行悬停只能改颜色/加粗。
type styles struct {
	palette ui.Palette

	screen   lipgloss.Style
	text     lipgloss.Style
	muted    lipgloss.Style
	title    lipgloss.Style
	kicker   lipgloss.Style
	note     lipgloss.Style
	success  lipgloss.Style
	warning  lipgloss.Style
	errStyle lipgloss.Style
	accent   lipgloss.Style
	emphasis lipgloss.Style

	// hover 是单行区域的通用悬停强调（无边框、无内边距）。
	hover lipgloss.Style
	// rowHover 是候选列表行的悬停底色（行本身已补齐到表格宽度）。
	rowHover lipgloss.Style

	selected    lipgloss.Style
	selectedDim lipgloss.Style

	rowEven lipgloss.Style
	rowOdd  lipgloss.Style

	tabActive   lipgloss.Style
	tabInactive lipgloss.Style
	tabHover    lipgloss.Style

	// chip 是单行扁平控件（外观下拉、更换目标、操作按钮的紧凑形态）。
	chip         lipgloss.Style
	chipFocused  lipgloss.Style
	chipHover    lipgloss.Style
	chipActive   lipgloss.Style
	chipDisabled lipgloss.Style

	// button* 为全界面统一的 1 行无边框扁平色块按钮。
	button         lipgloss.Style
	buttonFocused  lipgloss.Style
	buttonHover    lipgloss.Style
	buttonDisabled lipgloss.Style
	primaryButton  lipgloss.Style
	primaryFocus   lipgloss.Style
	primaryHover   lipgloss.Style

	frame        lipgloss.Style
	frameFocused lipgloss.Style
	frameLine    lipgloss.Style

	input        lipgloss.Style
	inputFocused lipgloss.Style

	entryLabel lipgloss.Style

	panelTitle lipgloss.Style

	link        lipgloss.Style
	linkFocused lipgloss.Style
	footerHint  lipgloss.Style

	progressBar  lipgloss.Style
	progressRest lipgloss.Style

	dropdownItem   lipgloss.Style
	dropdownActive lipgloss.Style
	dropdownHover  lipgloss.Style

	closeHint lipgloss.Style
}

func newStyles(p ui.Palette) styles {
	border := lipgloss.NormalBorder()
	base := lipgloss.NewStyle().Foreground(p.Foreground).Background(p.Background)
	return styles{
		palette:  p,
		screen:   base,
		text:     base,
		muted:    base.Foreground(p.Muted),
		title:    base.Foreground(p.Foreground).Bold(true),
		kicker:   base.Foreground(p.Primary).Bold(true),
		note:     base.Foreground(p.Muted),
		success:  base.Foreground(p.Success),
		warning:  base.Foreground(p.Warning),
		errStyle: base.Foreground(p.Error),
		accent:   base.Foreground(p.Accent).Bold(true),
		emphasis: base.Bold(true),

		hover:    base.Foreground(p.Accent).Bold(true),
		rowHover: base.Foreground(p.Foreground).Background(p.Surface),

		selected:    lipgloss.NewStyle().Foreground(p.ButtonFG).Background(p.Primary).Bold(true),
		selectedDim: lipgloss.NewStyle().Foreground(p.Muted).Background(p.Panel),

		rowEven: lipgloss.NewStyle().Foreground(p.Foreground).Background(p.Background),
		rowOdd:  lipgloss.NewStyle().Foreground(p.Foreground).Background(p.Surface),

		tabActive:   lipgloss.NewStyle().Foreground(p.ButtonFG).Background(p.Primary).Bold(true),
		tabInactive: lipgloss.NewStyle().Foreground(p.Muted).Background(p.Background),
		tabHover:    lipgloss.NewStyle().Foreground(p.Foreground).Background(p.Surface),

		chip: lipgloss.NewStyle().
			Foreground(p.Foreground).
			Background(p.Panel).
			Padding(0, 1),
		chipFocused: lipgloss.NewStyle().
			Foreground(p.Background).
			Background(p.Accent).
			Bold(true).
			Padding(0, 1),
		chipHover: lipgloss.NewStyle().
			Foreground(p.Background).
			Background(p.Accent).
			Bold(true).
			Padding(0, 1),
		chipActive: lipgloss.NewStyle().
			Foreground(p.ButtonFG).
			Background(p.Primary).
			Bold(true).
			Padding(0, 1),
		chipDisabled: lipgloss.NewStyle().
			Foreground(p.DisabledFG).
			Background(p.DisabledBG).
			Padding(0, 1),

		button: lipgloss.NewStyle().
			Foreground(p.Foreground).
			Background(p.Panel).
			Padding(0, 1),
		buttonFocused: lipgloss.NewStyle().
			Foreground(p.Background).
			Background(p.Accent).
			Bold(true).
			Padding(0, 1),
		buttonHover: lipgloss.NewStyle().
			Foreground(p.Foreground).
			Background(p.Surface).
			Bold(true).
			Padding(0, 1),
		buttonDisabled: lipgloss.NewStyle().
			Foreground(p.DisabledFG).
			Background(p.DisabledBG).
			Padding(0, 1),
		primaryButton: lipgloss.NewStyle().
			Foreground(p.ButtonFG).
			Background(p.Primary).
			Bold(true).
			Padding(0, 1),
		primaryFocus: lipgloss.NewStyle().
			Foreground(p.Background).
			Background(p.Accent).
			Bold(true).
			Padding(0, 1),
		primaryHover: lipgloss.NewStyle().
			Foreground(p.ButtonFG).
			Background(p.PrimaryHover).
			Bold(true).
			Padding(0, 1),

		frame: base.
			Border(border, true, true, true, true).
			BorderForeground(p.Line).
			BorderBackground(p.Background).
			Background(p.Background),
		frameLine: lipgloss.NewStyle().Foreground(p.Line).Background(p.Surface),
		frameFocused: base.
			Border(border, true, true, true, true).
			BorderForeground(p.Primary).
			BorderBackground(p.Background),

		input: base.
			Foreground(p.Foreground).
			Background(p.EntryBG).
			Border(border, true, true, true, true).
			BorderForeground(p.Line).
			BorderBackground(p.Background).
			Padding(0, 1),
		inputFocused: base.
			Foreground(p.Foreground).
			Background(p.EntryBG).
			Border(border, true, true, true, true).
			BorderForeground(p.Primary).
			BorderBackground(p.Background).
			Padding(0, 1),

		entryLabel: base.Foreground(p.Muted),

		panelTitle: lipgloss.NewStyle().Foreground(p.Foreground).Background(p.Surface).Bold(true),

		link:        base.Foreground(p.Muted).Underline(true),
		linkFocused: lipgloss.NewStyle().Foreground(p.ButtonFG).Background(p.Primary).Bold(true).Underline(true),
		footerHint:  base.Foreground(p.Muted),

		progressBar:  base.Foreground(p.Primary),
		progressRest: base.Foreground(p.Line),

		// 下拉项由 interiorLine 按精确内宽渲染，不得再加水平 Padding 以免顶出右边框。
		dropdownItem:   lipgloss.NewStyle().Foreground(p.Foreground).Background(p.Surface),
		dropdownActive: lipgloss.NewStyle().Foreground(p.ButtonFG).Background(p.Primary).Bold(true),
		dropdownHover:  lipgloss.NewStyle().Foreground(p.Background).Background(p.Accent).Bold(true),

		closeHint: base.Foreground(p.Accent).Underline(true),
	}
}
