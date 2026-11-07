package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The mascot is a small robot that types at a keyboard. It blinks, and its keys
// light up while vynsh is working — a friendly, low-key sign that something is
// running.
//
//	  ╭─────╮
//	  │ ◕ ◕ │
//	  │  ─  │
//	  ╰──┬──╯
//	╭────┴────╮
//	│ ░▓░▓░▓░ │
//	╰─────────╯
type mascotFrame struct {
	eyes string
	keys string
}

var mascotFrames = []mascotFrame{
	{eyes: "◕ ◕", keys: "░▓░▓░▓░"},
	{eyes: "◕ ◕", keys: "▓░▓░▓░▓"},
	{eyes: "─ ─", keys: "░▓░▓░▓░"}, // blink
	{eyes: "◕ ◕", keys: "▓░▓░▓░▓"},
	{eyes: "^ ^", keys: "▀▀▀▀▀▀▀"}, // happy
}

// mascotArt renders the robot itself, without the surrounding panel.
func mascotArt(frame int, busy bool) string {
	f := mascotFrames[frame%len(mascotFrames)]
	if !busy && f.eyes == "^ ^" {
		f = mascotFrames[0]
	}
	head := lipgloss.NewStyle().Foreground(colAccent2)
	body := lipgloss.NewStyle().Foreground(colAccent)
	keys := lipgloss.NewStyle().Foreground(colBad)
	lines := []string{
		head.Render("  ╭─────╮  "),
		head.Render("  │ ") + lipgloss.NewStyle().Bold(true).Foreground(colWarn).Render(f.eyes) + head.Render(" │  "),
		head.Render("  │  ─  │  "),
		head.Render("  ╰──┬──╯  "),
		body.Render("╭────┴────╮"),
		body.Render("│ ") + keys.Render(f.keys) + body.Render(" │"),
		body.Render("╰─────────╯"),
	}
	return strings.Join(lines, "\n")
}

// mascotPanel renders the art plus the mascot's line of dialogue, wrapped to the
// available width.
func mascotPanel(frame int, speech string, busy bool, width int) string {
	if speech == "" {
		speech = "ready"
	}
	art := mascotArt(frame, busy)
	text := wrapText(speech, max0(width-2))
	suffix := " ›"
	if busy {
		suffix = " working… ›"
	}
	return art + "\n\n" + textStyle.Render(text) + labelStyle.Render(suffix)
}

// wrapText greedily wraps s into lines of at most w cells.
func wrapText(s string, w int) string {
	if w <= 0 {
		return s
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	var out []string
	line := ""
	for _, word := range words {
		if line == "" {
			line = word
			continue
		}
		if lipgloss.Width(line)+1+lipgloss.Width(word) > w {
			out = append(out, line)
			line = word
			continue
		}
		line += " " + word
	}
	out = append(out, line)
	return strings.Join(out, "\n")
}

// spinner returns the rotating glyph for the given frame.
func spinner(frame int) string {
	return spinFrames[frame%len(spinFrames)]
}
