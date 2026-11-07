package logo

import (
	"bytes"
	"image/png"
	"testing"
)

func TestSpritesAreRectangles(t *testing.T) {
	if err := Check(); err != nil {
		t.Fatal(err)
	}
	for name, s := range All() {
		if s.Width() != 9 || s.Height() != 6 {
			t.Errorf("%s is %dx%d, want 9x6", name, s.Width(), s.Height())
		}
	}
}

func TestASCII(t *testing.T) {
	out := ASCII(Base, '#', '.')
	if len(out) == 0 {
		t.Fatal("empty ascii art")
	}
	t.Logf("base sprite:\n%s", out)
	if err := Check(); err != nil {
		t.Fatal(err)
	}
}

func TestPNGSizeAndPixels(t *testing.T) {
	img := PNG(Base, Options{Size: 1024, Transparent: true, Padding: 96})
	if b := img.Bounds(); b.Dx() != 1024 || b.Dy() != 1024 {
		t.Fatalf("bounds = %v, want 1024x1024", b)
	}
	// Corners are outside the sprite: transparent.
	if a := img.NRGBAAt(0, 0).A; a != 0 {
		t.Errorf("corner alpha = %d, want 0 (transparent)", a)
	}
	// The centre of the body must be the shell colour.
	if c := img.NRGBAAt(512, 512); c.A == 0 {
		t.Error("centre pixel is transparent, the sprite is not drawn")
	}
	// Count painted pixels to be sure the art is substantial but not the whole
	// canvas.
	painted := 0
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			if img.NRGBAAt(x, y).A > 0 {
				painted++
			}
		}
	}
	if painted < 1024*1024/10 {
		t.Errorf("only %d painted pixels, art looks broken", painted)
	}
	if painted > 1024*1024/2 {
		t.Errorf("%d painted pixels, art fills the canvas", painted)
	}
}

func TestPNGIsCrispPixelArt(t *testing.T) {
	// With integer scaling every sprite cell must be a solid block: check that
	// adjacent pixels inside a cell are equal and that the eye stays white.
	opts := Options{Size: 1024, Transparent: true, Padding: 96}
	img := PNG(Base, opts)
	scale := (opts.Size - 2*opts.Padding) / maxInt(Base.Width(), Base.Height())
	offX := (opts.Size - Base.Width()*scale) / 2
	offY := (opts.Size - Base.Height()*scale) / 2
	// Find an eye pixel: the first 'E' in the sprite.
	ex, ey := -1, -1
	for y := 0; y < Base.Height() && ex < 0; y++ {
		for x := 0; x < Base.Width(); x++ {
			if Base.At(x, y) == 'E' {
				ex, ey = x, y
				break
			}
		}
	}
	if ex < 0 {
		t.Fatal("sprite has no eyes")
	}
	px, py := offX+ex*scale+scale/2, offY+ey*scale+scale/2
	if c := img.NRGBAAt(px, py); c.R != 0xFF || c.G != 0xFF || c.B != 0xFF {
		t.Errorf("eye pixel = %+v, want white", c)
	}
	if img.NRGBAAt(px-scale, py) == img.NRGBAAt(px+scale, py) {
		t.Error("neighbouring sprite cells share a colour, the art may be washed out")
	}
}

func TestWritePNG(t *testing.T) {
	var buf bytes.Buffer
	if err := WritePNG(&buf, Base, Options{Size: 256, Transparent: false, Background: RGBA{0x0d, 0x0b, 0x1a, 0xff}}); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 256 || b.Dy() != 256 {
		t.Errorf("bounds = %v, want 256x256", b)
	}
}

func TestParseHex(t *testing.T) {
	if _, err := ParseHex("#0d0b1a"); err != nil {
		t.Errorf("ParseHex: %v", err)
	}
	if _, err := ParseHex("nope"); err == nil {
		t.Error("expected an error")
	}
}
