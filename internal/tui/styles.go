// Package tui renders the interactive dashboard. The look follows the muted,
// boxed style of modern terminal agents: a header with the mascot, numbered
// views, and a footer that always shows the keys that are available.
package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	colAccent  = lipgloss.Color("99") // muted violet
	colAccent2 = lipgloss.Color("81") // cyan
	colGood    = lipgloss.Color("42")
	colWarn    = lipgloss.Color("214")
	colBad     = lipgloss.Color("203")
	colDim     = lipgloss.Color("242")
	colText    = lipgloss.Color("252")

	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	accentStyle = lipgloss.NewStyle().Bold(true).Foreground(colAccent2)
	labelStyle  = lipgloss.NewStyle().Foreground(colDim)
	valueStyle  = lipgloss.NewStyle().Bold(true).Foreground(colText)
	goodStyle   = lipgloss.NewStyle().Foreground(colGood)
	warnStyle   = lipgloss.NewStyle().Foreground(colWarn)
	badStyle    = lipgloss.NewStyle().Foreground(colBad)
	dimStyle    = lipgloss.NewStyle().Foreground(colDim)
	textStyle   = lipgloss.NewStyle().Foreground(colText)

	selectedRow = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("57"))
	boxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(colDim).Padding(0, 1)
	panelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(colAccent).Padding(0, 1)
	activeTab   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(colAccent).Padding(0, 1)
	inactiveTab = lipgloss.NewStyle().Foreground(colDim).Padding(0, 1)
	checkboxOn  = lipgloss.NewStyle().Foreground(colGood).Bold(true)
	checkboxOff = lipgloss.NewStyle().Foreground(colDim)
	cursorMark  = lipgloss.NewStyle().Foreground(colAccent2).Bold(true)

	// spinFrames rotate while something is running, like an agent "thinking".
	spinFrames = []string{"✻", "✳", "✶", "✽", "✶", "✳"}
)

// bar renders a proportion bar coloured by pressure.
func bar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct/100*float64(width) + 0.5)
	body := repeat("█", filled) + repeat("░", width-filled)
	st := goodStyle
	switch {
	case pct >= 90:
		st = badStyle
	case pct >= 75:
		st = warnStyle
	}
	return st.Render(body)
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

// padRight pads s to w visible columns.
func padRight(s string, w int) string {
	d := w - lipgloss.Width(s)
	if d <= 0 {
		return s
	}
	return s + repeat(" ", d)
}

// padLeft pads s on the left to w visible columns.
func padLeft(s string, w int) string {
	d := w - lipgloss.Width(s)
	if d <= 0 {
		return s
	}
	return repeat(" ", d) + s
}

// clamp shortens s so that it fits into w terminal cells.
func clamp(s string, w int) string {
	if w <= 0 || lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > w-1 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
