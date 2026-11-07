// Package logo holds the vynsh mascot as pixel data. The same sprite is used by
// the terminal dashboard (rendered with half blocks) and by `vynsh logo`, which
// exports it as a square PNG for branding.
package logo

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
)

// Sprite is a grid of pixels. Each byte is a palette key; '.' is transparent.
type Sprite []string

// Width is the sprite width in pixels.
func (s Sprite) Width() int {
	w := 0
	for _, row := range s {
		if len(row) > w {
			w = len(row)
		}
	}
	return w
}

// Height is the sprite height in pixels.
func (s Sprite) Height() int { return len(s) }

// At returns the palette key at (x, y), or '.' outside the sprite.
func (s Sprite) At(x, y int) byte {
	if y < 0 || y >= len(s) || x < 0 || x >= len(s[y]) {
		return '.'
	}
	return s[y][x]
}

// RGBA is a straight (non-premultiplied) colour.
type RGBA struct{ R, G, B, A uint8 }

// Palette maps sprite keys to colours.
var Palette = map[byte]RGBA{
	'S': {0xA7, 0x8B, 0xFA, 0xFF}, // shell: violet-400
	'H': {0x8B, 0x5C, 0xF6, 0xFF}, // shell shade: violet-500
	'E': {0xFF, 0xFF, 0xFF, 0xFF}, // eyes
	'L': {0x4C, 0x1D, 0x95, 0xFF}, // antennae and legs: violet-900
}

// The beetle. The art is the reference the mascot was drawn from: two antennae,
// a wide body, two eyes, a darker band along the bottom of the shell and three
// legs. Keep the rows as they are: the terminal renderer draws them one to one.
//
//	L.......L     antenna tips
//	.L.....L.     antennae
//	.SSSSSSS.     body
//	.SEESEES.     eyes
//	.SHHHHHH.     shell shade
//	..L.L.L..     legs
var Base = Sprite{
	"L.......L",
	".L.....L.",
	".SSSSSSS.",
	".SEESEES.",
	".SHHHHHH.",
	"..L.L.L..",
}

// Blink closes both eyes.
var Blink = Sprite{
	"L.......L",
	".L.....L.",
	".SSSSSSS.",
	".SSSSSSS.",
	".SHHHHHH.",
	"..L.L.L..",
}

// Wink closes the right eye only.
var Wink = Sprite{
	"L.......L",
	".L.....L.",
	".SSSSSSS.",
	".SEESSSS.",
	".SHHHHHH.",
	"..L.L.L..",
}

// AntennaeUp and AntennaeDown wiggle the antennae.
var AntennaeUp = Sprite{
	"L.......L",
	"..L...L..",
	".SSSSSSS.",
	".SEESEES.",
	".SHHHHHH.",
	"..L.L.L..",
}

var AntennaeDown = Sprite{
	".L.....L.",
	"L.......L",
	".SSSSSSS.",
	".SEESEES.",
	".SHHHHHH.",
	"..L.L.L..",
}

// Hop tucks the legs in: the beetle is in the air.
var Hop = Sprite{
	"L.......L",
	".L.....L.",
	".SSSSSSS.",
	".SEESEES.",
	".SHHHHHH.",
	"..L...L..",
}

// Type1 and Type2 alternate the feet while vynsh is working.
var Type1 = Sprite{
	"L.......L",
	".L.....L.",
	".SSSSSSS.",
	".SEESEES.",
	".SHHHHHH.",
	".L.L.L.L.",
}

var Type2 = Sprite{
	"L.......L",
	".L.....L.",
	".SSSSSSS.",
	".SEESEES.",
	".SHHHHHH.",
	"..L...L..",
}

// All returns every sprite variant by name.
func All() map[string]Sprite {
	return map[string]Sprite{
		"base":      Base,
		"blink":     Blink,
		"wink":      Wink,
		"antennae":  AntennaeUp,
		"hop":       Hop,
		"type1":     Type1,
		"type2":     Type2,
		"antennae2": AntennaeDown,
	}
}

// Check validates that every sprite is a rectangle of the same size, so the
// terminal renderer and the PNG exporter always agree.
func Check() error {
	want := 0
	for name, s := range All() {
		if len(s) == 0 {
			return fmt.Errorf("%s is empty", name)
		}
		w := len(s[0])
		for y, row := range s {
			if len(row) != w {
				return fmt.Errorf("%s row %d has width %d, want %d", name, y, len(row), w)
			}
		}
		if want == 0 {
			want = w
		} else if w != want {
			return fmt.Errorf("%s has width %d, want %d", name, w, want)
		}
	}
	return nil
}

// ASCII renders a sprite as text, one character per pixel, so it can be checked
// without an image viewer.
func ASCII(s Sprite, fill, empty rune) string {
	var b strings.Builder
	for _, row := range s {
		for _, ch := range row {
			if ch == '.' {
				b.WriteRune(empty)
			} else {
				b.WriteRune(fill)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Options controls PNG export.
type Options struct {
	// Size is the square side length in pixels.
	Size int
	// Background is the backdrop colour; ignored when Transparent is set.
	Background RGBA
	// Transparent leaves the area around the beetle empty.
	Transparent bool
	// Padding is extra breathing room around the sprite, in final pixels.
	Padding int
	// Tight crops the canvas to the art instead of centring it in a square.
	Tight bool
}

// PNG renders the sprite into a square image. Scaling uses integer factors and
// nearest-neighbour sampling, so the result stays crisp pixel art.
func PNG(s Sprite, opts Options) *image.NRGBA {
	size := opts.Size
	if size <= 0 {
		size = 1024
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	if !opts.Transparent {
		fill(img, opts.Background)
	}

	w, h := s.Width(), s.Height()
	avail := size - 2*opts.Padding
	if opts.Tight || avail <= 0 {
		avail = size
	}
	scale := avail / maxInt(w, h)
	if scale < 1 {
		scale = 1
	}
	artW, artH := w*scale, h*scale
	offX, offY := (size-artW)/2, (size-artH)/2
	if opts.Tight {
		img = image.NewNRGBA(image.Rect(0, 0, artW, artH))
		if !opts.Transparent {
			fill(img, opts.Background)
		}
		offX, offY = 0, 0
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c, ok := Palette[s.At(x, y)]
			if !ok {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				row := (offY + y*scale + dy) * img.Stride
				for dx := 0; dx < scale; dx++ {
					i := row + (offX+x*scale+dx)*4
					img.Pix[i] = c.R
					img.Pix[i+1] = c.G
					img.Pix[i+2] = c.B
					img.Pix[i+3] = c.A
				}
			}
		}
	}
	return img
}

// fill paints the whole image with c.
func fill(img *image.NRGBA, c RGBA) {
	for y := 0; y < img.Rect.Dy(); y++ {
		row := y * img.Stride
		for x := 0; x < img.Rect.Dx(); x++ {
			i := row + x*4
			img.Pix[i] = c.R
			img.Pix[i+1] = c.G
			img.Pix[i+2] = c.B
			img.Pix[i+3] = c.A
		}
	}
}

// WritePNG encodes the sprite and writes it to w.
func WritePNG(w io.Writer, s Sprite, opts Options) error {
	return png.Encode(w, PNG(s, opts))
}

// SVG renders the sprite as a vector image: one rectangle per pixel with
// crisp edges. Unlike a PNG it stays sharp at any size, which matters for the
// README where the logo is shown small.
func SVG(s Sprite, bg *RGBA, displayWidth int) string {
	if displayWidth <= 0 {
		displayWidth = s.Width() * 32
	}
	displayHeight := displayWidth * s.Height() / s.Width()
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" shape-rendering="crispEdges">`+"\n",
		s.Width(), s.Height(), displayWidth, displayHeight)
	if bg != nil {
		fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/>`+"\n", s.Width(), s.Height(), cssColor(*bg))
	}
	for y := 0; y < s.Height(); y++ {
		for x := 0; x < s.Width(); x++ {
			c, ok := Palette[s.At(x, y)]
			if !ok {
				continue
			}
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="1" height="1" fill="%s"/>`+"\n", x, y, cssColor(c))
		}
	}
	b.WriteString("</svg>\n")
	return b.String()
}

// cssColor formats a colour for SVG.
func cssColor(c RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// ParseHex turns "#rrggbb" (or "#rrggbbaa") into a colour.
func ParseHex(s string) (color.NRGBA, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	parse := func(b string) (uint8, error) {
		var v uint8
		if _, err := fmt.Sscanf(b, "%02x", &v); err != nil {
			return 0, err
		}
		return v, nil
	}
	if len(s) != 6 && len(s) != 8 {
		return color.NRGBA{}, fmt.Errorf("colour %q must be #rrggbb", s)
	}
	r, err := parse(s[0:2])
	if err != nil {
		return color.NRGBA{}, err
	}
	g, err := parse(s[2:4])
	if err != nil {
		return color.NRGBA{}, err
	}
	b, err := parse(s[4:6])
	if err != nil {
		return color.NRGBA{}, err
	}
	a := uint8(255)
	if len(s) == 8 {
		if a, err = parse(s[6:8]); err != nil {
			return color.NRGBA{}, err
		}
	}
	return color.NRGBA{R: r, G: g, B: b, A: a}, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
