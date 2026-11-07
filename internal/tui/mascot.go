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

// beetleArt renders a sprite with half-block characters: two pixel rows per
// terminal row, so the 9x6 sprite becomes three lines of text.
func beetleArt(s logo.Sprite) []string {
	lines := make([]string, 0, s.Height()/2)
	for y := 0; y+1 < s.Height(); y += 2 {
		var b strings.Builder
		for x := 0; x < s.Width(); x++ {
			b.WriteString(beetleCell(s.At(x, y), s.At(x, y+1)))
		}
		lines = append(lines, b.String())
	}
	return lines
}

// beetleCell draws one terminal cell from a top and a bottom pixel.
func beetleCell(top, bottom byte) string {
	tc, tok := beetleColors[top]
	bc, bok := beetleColors[bottom]
	switch {
	case !tok && !bok:
		return " "
	case tok && !bok:
		return lipgloss.NewStyle().Foreground(tc).Render("▀")
	case !tok && bok:
		return lipgloss.NewStyle().Foreground(bc).Render("▄")
	default:
		return lipgloss.NewStyle().Foreground(tc).Background(bc).Render("▀")
	}
}

// beetleState is everything needed to draw one frame of the beetle.
type beetleState struct {
	frame int
	busy  bool
	happy bool
}

// hopping reports whether the beetle is in the air right now. It hops briefly
// every couple of seconds, and twice in a row after a successful cleanup.
func (b beetleState) hopping() bool {
	if !b.busy && b.happy {
		switch b.frame % 20 {
		case 0, 1, 2, 10, 11, 12:
			return true
		}
		return false
	}
	switch b.frame % 30 {
	case 8, 9:
		return true
	}
	return false
}

// blink reports whether the beetle has its eyes closed.
func (b beetleState) blink() bool {
	return b.frame%45 == 0
}

// sprite picks the sprite for the current state.
func (b beetleState) sprite() logo.Sprite {
	switch {
	case b.hopping():
		return logo.Hop
	case b.blink():
		return logo.Blink
	}
	return logo.Base
}

// withBeetle draws the beetle above the input box: its feet touch the top
// border, and while hopping it floats one row higher with tucked legs.
func (m *model) withBeetle(input string, width int) string {
	state := beetleState{frame: m.frame, busy: m.isBusy(), happy: m.frame < m.happyUntil}
	art := beetleArt(state.sprite())

	// Keep the beetle off very short terminals.
	switch {
	case m.height > 0 && m.height < 20:
		return input
	case m.height > 0 && m.height < 26:
		if len(art) > 2 {
			art = art[len(art)-2:]
		}
	}

	if state.hopping() {
		// Float one row up: the legs leave the border.
		art = append([]string{""}, art[:len(art)-1]...)
	}

	indent := beetleIndent(width)
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

// beetleIndent places the beetle a little in from the left edge of the box.
func beetleIndent(width int) int {
	indent := width/4 - 3
	if indent < 2 {
		indent = 2
	}
	max := width - logo.Base.Width() - 4
	if max < 2 {
		max = 2
	}
	if indent > max {
		indent = max
	}
	return indent
}

// spinner returns the rotating glyph for the given frame.
func spinner(frame int) string {
	return spinFrames[(frame/2)%len(spinFrames)]
}
