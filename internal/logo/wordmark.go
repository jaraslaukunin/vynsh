package logo

import "strings"

// A tiny pixel font for the wordmark. Every glyph is five pixels wide and
// seven tall, which matches the beetle: the x-height is five pixels and only
// the descender of "y" goes below the baseline.
//
// The glyphs are drawn by hand so the lockup stays sharp at any size and looks
// the same in the PNG and the SVG export.
var glyphs = map[rune][7]string{
	'v': {
		"#...#",
		"#...#",
		".#.#.",
		".#.#.",
		"..#..",
		".....",
		".....",
	},
	'y': {
		"#...#",
		"#...#",
		".#.#.",
		".#.#.",
		"..#..",
		"..#..",
		".##..",
	},
	'n': {
		"####.",
		"#...#",
		"#...#",
		"#...#",
		"#...#",
		".....",
		".....",
	},
	's': {
		".####",
		"#....",
		".###.",
		"....#",
		"####.",
		".....",
		".....",
	},
	'h': {
		"#....",
		"#....",
		"####.",
		"#...#",
		"#...#",
		".....",
		".....",
	},
	'i': {
		"#",
		"#",
		"#",
		"#",
		"#",
		".",
		".",
	},
	' ': {
		"..",
		"..",
		"..",
		"..",
		"..",
		"..",
		"..",
	},
}

// glyphWidth is the advance of a normal glyph; the letter spacing is one pixel.
const glyphWidth = 5

// Wordmark renders text as pixel letters in the shell colour.
func Wordmark(text string) Sprite {
	text = strings.ToLower(text)
	width := 0
	glyphWidths := make([]int, 0, len(text))
	for _, r := range text {
		w := glyphWidth
		if g, ok := glyphs[r]; ok {
			w = len(g[0])
		}
		glyphWidths = append(glyphWidths, w)
		width += w + 1
	}
	if width > 0 {
		width--
	}
	grid := make([][]byte, 7)
	for y := range grid {
		grid[y] = []byte(strings.Repeat(".", width))
	}
	x := 0
	for i, r := range text {
		g, ok := glyphs[r]
		if !ok {
			g = glyphs[' ']
		}
		for y := 0; y < 7; y++ {
			for gx := 0; gx < len(g[y]); gx++ {
				if g[y][gx] == '#' && x+gx < width {
					grid[y][x+gx] = 'S'
				}
			}
		}
		x += glyphWidths[i] + 1
	}
	out := make(Sprite, 7)
	for y := range grid {
		out[y] = string(grid[y])
	}
	return out
}

// Hero composes the mascot and the wordmark into one lockup, with the beetle on
// the left and the text to the right, both sitting on the same baseline.
func Hero(mascot Sprite, text string, gap int) Sprite {
	mark := Wordmark(text)
	if gap < 0 {
		gap = 0
	}
	width := mascot.Width() + gap + mark.Width()
	height := mascot.Height()
	if mark.Height() > height {
		height = mark.Height()
	}
	// Centre the mascot vertically against the taller text grid.
	mascotTop := (height - mascot.Height()) / 2

	grid := make([][]byte, height)
	for y := range grid {
		grid[y] = []byte(strings.Repeat(".", width))
	}
	put := func(s Sprite, ox, oy int) {
		for y := 0; y < s.Height(); y++ {
			for x := 0; x < s.Width(); x++ {
				c := s.At(x, y)
				if c == '.' {
					continue
				}
				if ty, tx := oy+y, ox+x; ty >= 0 && ty < height && tx >= 0 && tx < width {
					grid[ty][tx] = c
				}
			}
		}
	}
	put(mascot, 0, mascotTop)
	put(mark, mascot.Width()+gap, 0)

	out := make(Sprite, height)
	for y := range grid {
		out[y] = string(grid[y])
	}
	return out
}
