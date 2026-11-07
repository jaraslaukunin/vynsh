package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The mascot is a pixel-art beetle. Every sprite is a grid of "pixels" that is
// drawn with half-block characters (▀ ▄ █), so two pixel rows fit into one
// terminal row and the figure looks like a real 8-bit sprite.
//
// Pixel palette:
//
//	S shell   D shell shade   E eye white   P pupil   L legs/antennae   . empty

const spriteW = 13

// sprite rows are pixel rows; a sprite always has an even number of rows.
type sprite []string

var palette = map[byte]lipgloss.Color{
	'S': lipgloss.Color("141"), // violet shell
	'D': lipgloss.Color("54"),  // seam between the wings
	'H': lipgloss.Color("60"),  // head, dark
	'E': lipgloss.Color("255"), // eye white
	'P': lipgloss.Color("16"),  // pupil
	'L': lipgloss.Color("248"), // legs and antennae
}

// base is the beetle sitting still with its eyes open.
var base = sprite{
	".L.........L.",
	"..L.......L..",
	"...HHHHHHH...",
	"..HEEHHHEEH..",
	"..HPEHHHPEH..",
	".SSSSSSSSSSS.",
	"SSSSSSDSSSSSS",
	"SSSSSSDSSSSSS",
	".SSSSSDSSSSS.",
	"..SSSSDSSSS..",
	"..L.L...L.L..",
	".L.L.....L.L.",
}

// poses are the animation frames. Each one is a full sprite.
var poses = map[string]sprite{
	"sit": base,
	"blink": replaceRows(base, map[int]string{
		3: "..HHHHHHHHH..",
		4: "..HHHHHHHHH..",
	}),
	"antenna": replaceRows(base, map[int]string{
		0: "L..........L.",
		1: "..L.......L..",
	}),
	"type1": replaceRows(base, map[int]string{
		10: "..L.L...L.L..",
		11: ".L...L...L.L.",
	}),
	"type2": replaceRows(base, map[int]string{
		10: "..L..L.L...L.",
		11: ".L.L...L...L.",
	}),
	"walk1": replaceRows(base, map[int]string{
		10: ".L.L.L.L.L...",
		11: "..L.L.L.L.L..",
	}),
	"walk2": replaceRows(base, map[int]string{
		10: "..L.L.L.L.L..",
		11: ".L.L.L.L.L...",
	}),
	"happy": replaceRows(base, map[int]string{
		3: "..HHHHHHHHH..",
		4: "..HPHHHPHHH..",
	}),
}

// replaceRows returns a copy of s with the given rows swapped in.
func replaceRows(s sprite, rows map[int]string) sprite {
	out := make(sprite, len(s))
	copy(out, s)
	for i, r := range rows {
		out[i] = r
	}
	return out
}

// renderSprite turns pixel rows into terminal rows using half blocks.
func renderSprite(s sprite) []string {
	lines := make([]string, 0, len(s)/2)
	for y := 0; y+1 < len(s); y += 2 {
		var b strings.Builder
		for x := 0; x < spriteW; x++ {
			top := pixel(s, y, x)
			bottom := pixel(s, y+1, x)
			b.WriteString(cell(top, bottom))
		}
		lines = append(lines, b.String())
	}
	return lines
}

// pixel returns the palette character at (x, y), or '.' outside the sprite.
func pixel(s sprite, y, x int) byte {
	if y < 0 || y >= len(s) || x < 0 || x >= len(s[y]) {
		return '.'
	}
	return s[y][x]
}

// cell draws one terminal cell from a top and a bottom pixel.
func cell(top, bottom byte) string {
	tc, tok := palette[top]
	bc, bok := palette[bottom]
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
	frame   int  // animation tick
	busy    bool // vynsh is working: the beetle types
	happy   bool // something was just cleaned successfully
	walking bool // the beetle is strolling across the floor
	x       int  // column where the beetle stands
	dir     int  // +1 or -1 while walking
}

// pose picks the sprite for the current state.
func (b beetleState) pose() string {
	switch {
	case b.happy:
		return "happy"
	case b.walking:
		if (b.frame/2)%2 == 0 {
			return "walk1"
		}
		return "walk2"
	case b.busy:
		if (b.frame/2)%2 == 0 {
			return "type1"
		}
		return "type2"
	}
	switch b.frame % 40 {
	case 0, 1:
		return "blink"
	case 20, 21, 22:
		return "antenna"
	}
	return "sit"
}

// render draws the beetle on a floor line of the given width.
func (b beetleState) render(floorWidth int) string {
	lines := renderSprite(poses[b.pose()])
	pad := strings.Repeat(" ", max0(b.x))
	var out []string
	for _, l := range lines {
		out = append(out, pad+l)
	}
	floor := strings.Repeat("─", max0(floorWidth))
	out = append(out, dimStyle.Render(floor))
	return strings.Join(out, "\n")
}

// mascotPanel renders the beetle, its floor and the line of dialogue below.
func mascotPanel(state beetleState, speech string, width int) string {
	if speech == "" {
		speech = "ready"
	}
	art := state.render(width)
	text := wrapText(speech, max0(width-2))
	suffix := " ›"
	if state.busy {
		suffix = " ›"
	}
	return art + "\n\n" + textStyle.Render(text) + labelStyle.Render(suffix)
}

// walkStep advances the beetle by one animation tick. It starts a short walk
// now and then, bounces off the panel edges and stops when vynsh is busy.
func (b *beetleState) walkStep(maxX int) {
	if b.busy {
		b.walking = false
		return
	}
	if b.walking {
		if b.frame%2 == 0 {
			b.x += b.dir
		}
		if b.x <= 0 {
			b.x = 0
			b.dir = 1
		}
		if b.x >= maxX {
			b.x = maxX
			b.dir = -1
		}
		// Walk for a bit, then sit down again.
		if b.frame%60 >= 40 {
			b.walking = false
		}
		return
	}
	if b.frame%60 == 0 && b.frame > 0 {
		b.walking = true
		if b.dir == 0 {
			b.dir = 1
		}
	}
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
	return spinFrames[(frame/2)%len(spinFrames)]
}

// checkSprites is used by tests: every sprite must be a rectangle of the same
// width with an even number of rows.
func checkSprites() error {
	for name, s := range poses {
		if len(s)%2 != 0 {
			return fmt.Errorf("%s has an odd number of rows (%d)", name, len(s))
		}
		for y, row := range s {
			if len(row) != spriteW {
				return fmt.Errorf("%s row %d has width %d, want %d", name, y, len(row), spriteW)
			}
		}
	}
	return nil
}
