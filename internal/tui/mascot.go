package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/jaraslaukunin/vynsh/internal/logo"
)

// The mascot is the vynsh beetle, drawn from internal/logo so that the terminal
// dashboard and the exported logo always show the same creature. It sits on the
// top edge of the input box and hops up and down now and then.

// beetleColors maps the logo palette to terminal colours.
var beetleColors = func() map[byte]lipgloss.Color {
	out := map[byte]lipgloss.Color{}
	for key, c := range logo.Palette {
		out[key] = lipgloss.Color(hex(c))
	}
	return out
}()

func hex(c logo.RGBA) string {
	const digits = "0123456789abcdef"
	b := []byte{
		digits[c.R>>4], digits[c.R&0xf],
		digits[c.G>>4], digits[c.G&0xf],
		digits[c.B>>4], digits[c.B&0xf],
	}
	return "#" + string(b)
}

// beetleArt renders the sprite one terminal row per pixel row, drawing every
// pixel as two solid blocks so it keeps the square shape of the artwork. Half
// blocks are deliberately not used: they cut one-pixel details (the eyes, the
// shade band) in half and the beetle stops looking like the logo.
func beetleArt(s logo.Sprite) []string {
	lines := make([]string, 0, s.Height())
	for y := 0; y < s.Height(); y++ {
		var b strings.Builder
		for x := 0; x < s.Width(); x++ {
			b.WriteString(beetlePixel(s.At(x, y)))
		}
		lines = append(lines, b.String())
	}
	return lines
}

// beetlePixel draws one sprite pixel as two blocks of its colour. The cell
// background is painted as well: terminals add a little space between lines, and
// a background fill keeps the beetle looking like one solid shape instead of a
// stack of stripes.
func beetlePixel(key byte) string {
	c, ok := beetleColors[key]
	if !ok {
		return "  "
	}
	return lipgloss.NewStyle().Foreground(c).Background(c).Render("██")
}

// beetleState is everything needed to draw one frame of the beetle.
type beetleState struct {
	frame   int
	busy    bool
	happy   bool
	walking bool
}

// The animation runs off a single frame counter (one tick is 150 ms):
//
//	every  ~5 s  a short hop (legs tucked, one row up)
//	every  ~7 s  a blink
//	every ~14 s  a wink
//	every  ~4 s  the antennae wiggle for a moment
//	while busy   the feet tap alternately and the antennae wiggle
//	after work   a double hop of joy with squinting eyes
func (b beetleState) hopping() bool {
	if b.happy {
		switch b.frame % 24 {
		case 0, 1, 2, 12, 13, 14:
			return true
		}
		return false
	}
	switch b.frame % 36 {
	case 6, 7:
		return true
	}
	return false
}

// blinking reports whether both eyes are closed right now.
func (b beetleState) blinking() bool {
	return b.frame%48 == 0
}

// winking reports whether one eye is closed.
func (b beetleState) winking() bool {
	// Offset from the blink frames, otherwise the blink would always win.
	return !b.busy && b.frame%96 == 49
}

// wiggling reports whether the antennae are moving.
func (b beetleState) wiggling() bool {
	if b.busy {
		return (b.frame/2)%2 == 0
	}
	return b.frame%28 >= 14 && b.frame%28 < 20
}

// sprite picks the sprite for the current state.
func (b beetleState) sprite() logo.Sprite {
	switch {
	case b.hopping():
		return logo.Hop
	case b.walking:
		if (b.frame/3)%2 == 0 {
			return logo.Type1
		}
		return logo.Type2
	case b.blinking():
		return logo.Blink
	case b.winking():
		return logo.Wink
	case b.busy:
		if (b.frame/2)%2 == 0 {
			return logo.Type1
		}
		return logo.Type2
	case b.wiggling():
		if (b.frame/4)%2 == 0 {
			return logo.AntennaeUp
		}
		return logo.AntennaeDown
	}
	return logo.Base
}

// withBeetle draws the beetle above the input box: its feet touch the top
// border, and while hopping it floats one row higher with tucked legs.
func (m *model) withBeetle(input string, width int) string {
	state := beetleState{
		frame:   m.frame,
		busy:    m.isBusy(),
		happy:   m.frame < m.happyUntil,
		walking: m.beetleWalk > 0,
	}
	art := beetleArt(state.sprite())

	// Keep the beetle off short terminals so the panels stay readable.
	if m.height > 0 && m.height < 28 {
		return input
	}

	if state.hopping() {
		// Float one row up: the legs leave the border.
		art = append([]string{""}, art[:len(art)-1]...)
	}

	indent := beetleAnchor(width) + m.beetleShift
	if indent < 2 {
		indent = 2
	}
	rows := make([]string, 0, len(art)+4)
	for _, line := range art {
		if strings.TrimSpace(line) == "" {
			rows = append(rows, "")
			continue
		}
		rows = append(rows, strings.Repeat(" ", indent)+line)
	}
	return strings.Join(append(rows, strings.Split(input, "\n")...), "\n")
}

// beetleAnchor is where the beetle sits by default: the right end of the input
// box, so it never covers the hints and takes no room from the content.
func beetleAnchor(width int) int {
	// Each sprite pixel is two columns wide; the box occupies width-2 columns
	// (see footerView), so park the beetle just inside its right border.
	anchor := width - 3 - logo.Base.Width()*2
	if anchor < 2 {
		anchor = 2
	}
	return anchor
}

// spinner returns the rotating glyph for the given frame.
func spinner(frame int) string {
	return spinFrames[(frame/2)%len(spinFrames)]
}
