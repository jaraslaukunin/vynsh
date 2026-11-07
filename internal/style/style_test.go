package style

import (
	"bytes"
	"strings"
	"testing"
)

func TestWidthIgnoresANSIColors(t *testing.T) {
	th := &Theme{Enabled: true}
	if got := Width(th.Red("abc")); got != 3 {
		t.Errorf("Width = %d, want 3", got)
	}
	if got := Width("простой"); got != 7 {
		t.Errorf("Width of cyrillic = %d, want 7", got)
	}
}

func TestPadAndTruncate(t *testing.T) {
	th := &Theme{Enabled: true}
	if got := Pad(th.Red("ab"), 5); Width(got) != 5 {
		t.Errorf("Pad width = %d, want 5", Width(got))
	}
	if got := PadLeft("ab", 4); got != "  ab" {
		t.Errorf("PadLeft = %q", got)
	}
	if got := Truncate("abcdefgh", 5); Width(got) > 5 {
		t.Errorf("Truncate = %q (width %d)", got, Width(got))
	}
	if got := Truncate("abc", 5); got != "abc" {
		t.Errorf("Truncate short = %q", got)
	}
}

func TestTableAlignment(t *testing.T) {
	th := &Theme{}
	tbl := &Table{
		Headers: []string{"SIZE", "PATH"},
		Rows: [][]string{
			{"1 KB", "a"},
			{"10 MB", "bb"},
		},
		Align: []Align{Right, Left},
	}
	out := tbl.Render(th)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want 4:\n%s", len(lines), out)
	}
	if !strings.HasSuffix(lines[0], "PATH") {
		t.Errorf("header missing PATH: %q", lines[0])
	}
	if !strings.HasPrefix(lines[2], " 1 KB") {
		t.Errorf("right alignment failed: %q", lines[2])
	}
	if !strings.HasPrefix(lines[3], "10 MB") {
		t.Errorf("wider value should fill the column: %q", lines[3])
	}
}

func TestBar(t *testing.T) {
	th := &Theme{Enabled: false}
	if got := Bar(0.5, 10, th); len([]rune(got)) != 10 {
		t.Errorf("bar = %q", got)
	}
	if got := Bar(-1, 4, th); got != "░░░░" {
		t.Errorf("bar(-1) = %q", got)
	}
	if got := Bar(2, 4, th); got != "████" {
		t.Errorf("bar(2) = %q", got)
	}
}

func TestNoColorThemeIsPlain(t *testing.T) {
	th := &Theme{}
	if th.Red("x") != "x" || th.Bold("y") != "y" {
		t.Error("disabled theme must not add escapes")
	}
}

func TestSpinnerDisabledWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	sp := NewSpinner(&buf, &Theme{}, "working")
	sp.Start()
	sp.Update("still working")
	sp.Stop()
	if buf.Len() != 0 {
		t.Errorf("spinner wrote to a non-terminal: %q", buf.String())
	}
}
