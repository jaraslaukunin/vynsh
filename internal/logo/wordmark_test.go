package logo

import (
	"testing"
)

func TestWordmarkShape(t *testing.T) {
	w := Wordmark("vynsh")
	if w.Height() != 7 {
		t.Fatalf("height = %d, want 7", w.Height())
	}
	// 5 letters, 5px each, 1px spacing.
	if w.Width() != 5*5+4 {
		t.Errorf("width = %d, want 29", w.Width())
	}
	if err := w.check(); err != nil {
		t.Fatal(err)
	}
}

func TestHeroLockup(t *testing.T) {
	h := Hero(Base, "vynsh", 3)
	if h.Height() != 7 {
		t.Errorf("height = %d, want 7", h.Height())
	}
	want := Base.Width() + 3 + 29
	if h.Width() != want {
		t.Errorf("width = %d, want %d", h.Width(), want)
	}
	// The mascot must be on the left, the text on the right.
	if h.At(0, 0) != 'L' {
		t.Errorf("top-left pixel = %q, want the antenna", string(h.At(0, 0)))
	}
	// The last letter "h" reaches the right edge on its middle rows.
	if h.At(h.Width()-1, 4) != 'S' {
		t.Errorf("right edge at the baseline = %q, want a stroke of h", string(h.At(h.Width()-1, 4)))
	}
	// And the text must not overlap the mascot.
	if h.At(Base.Width(), 4) != '.' && h.At(Base.Width()+1, 4) != '.' {
		t.Error("mascot and wordmark overlap")
	}
}

func TestHeroUnknownCharacters(t *testing.T) {
	h := Hero(Base, "v!?", 2)
	if err := h.check(); err != nil {
		t.Fatalf("unknown characters broke the sprite: %v", err)
	}
}

// check validates that the sprite is rectangular.
func (s Sprite) check() error {
	w := s.Width()
	for y := 0; y < s.Height(); y++ {
		if len(s[y]) != w {
			return &shapeError{y, len(s[y]), w}
		}
	}
	return nil
}

type shapeError struct{ row, got, want int }

func (e *shapeError) Error() string {
	return "row " + itoa(e.row) + " has width " + itoa(e.got) + ", want " + itoa(e.want)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
