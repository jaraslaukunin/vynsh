package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jaraslaukunin/vynsh/internal/dupes"
)

func dupeModel() *model {
	m := sampleModel()
	m.tab = tabDupes
	m.dupes.scanned = true
	m.dupes.build([]dupes.Group{
		{Size: 2 << 20, Hash: "a", Paths: []string{"/h/a/one.bin", "/h/b/one-copy.bin", "/h/c/one-again.bin"}},
		{Size: 5 << 20, Hash: "b", Paths: []string{"/h/x.iso", "/h/y.iso"}},
	})
	return m
}

func TestDupeBuildMarksAllButFirst(t *testing.T) {
	m := dupeModel()
	if m.dupes.marked["/h/a/one.bin"] {
		t.Error("the first copy of a group must be kept")
	}
	if !m.dupes.marked["/h/b/one-copy.bin"] || !m.dupes.marked["/h/c/one-again.bin"] {
		t.Error("extra copies should be marked by default")
	}
	bytes, n := m.dupes.markedBytes()
	if n != 3 || bytes != 2*(2<<20)+5<<20 {
		t.Errorf("marked = %d copies / %d bytes", n, bytes)
	}
}

func TestDupeCursorSkipsHeaders(t *testing.T) {
	m := dupeModel()
	if m.dupes.rows[m.dupes.cursor].header {
		t.Fatal("cursor must start on a file row")
	}
	for i := 0; i < 10; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		if m.dupes.rows[m.dupes.cursor].header {
			t.Fatalf("cursor landed on a header at %d", m.dupes.cursor)
		}
	}
}

func TestDupeSafeMarksNeverRemovesEveryCopy(t *testing.T) {
	m := dupeModel()
	// Mark every copy of the second group, including the first one.
	m.dupes.marked["/h/x.iso"] = true
	m.dupes.marked["/h/y.iso"] = true
	remove, blocked := m.dupes.safeMarks()
	for _, p := range remove {
		if p == "/h/x.iso" || p == "/h/y.iso" {
			t.Fatalf("both copies of a group were marked: %v", remove)
		}
	}
	if len(blocked) != 1 {
		t.Errorf("blocked groups = %v, want 1", blocked)
	}
	if len(remove) != 2 {
		t.Errorf("remove = %v, want the two extra copies of the first group", remove)
	}
}

func TestDupeEnterAsksBeforeTrashing(t *testing.T) {
	m := dupeModel()
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.prompt == nil {
		t.Fatal("enter must ask for confirmation")
	}
	if !strings.Contains(m.prompt.question, "trash") {
		t.Errorf("question = %q", m.prompt.question)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m.prompt != nil {
		t.Error("'n' must cancel")
	}
}

func TestDupeViewRenders(t *testing.T) {
	m := dupeModel()
	out := m.dupesView()
	for _, want := range []string{"Duplicates", "× 3", "one-copy.bin", "kept"} {
		if !strings.Contains(out, want) {
			t.Errorf("duplicates view misses %q:\n%s", want, out)
		}
	}
}
