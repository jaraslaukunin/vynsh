package snapshot

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jaraslaukunin/vynsh/internal/scan"
)

// isolate points the snapshot store at a temporary directory.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", dir)
	} else {
		t.Setenv("HOME", dir)
	}
	return dir
}

func tree(root string, kids map[string]int64) *scan.Result {
	res := &scan.Result{Root: root, Files: 1}
	rootNode := &scan.Node{Name: filepath.Base(root), Path: root, IsDir: true, Files: 1}
	for name, size := range kids {
		res.Total += size
		res.Files++
		rootNode.Size += size
		rootNode.Files++
		rootNode.Children = append(rootNode.Children, &scan.Node{
			Name: name, Path: filepath.Join(root, name), IsDir: true, Size: size, Files: 1,
		})
	}
	res.Tree = rootNode
	return res
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"before update":  "before-update",
		"/home/u/../x":   "home-u-..-x",
		"  nice_name.1 ": "nice_name.1",
		"":               "",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	name := DefaultName("/home/u/projects")
	if !strings.HasPrefix(name, "projects-") {
		t.Errorf("DefaultName = %q, want prefix projects-", name)
	}
}

func TestSaveLoadListDelete(t *testing.T) {
	isolate(t)
	res := tree("/data/home", map[string]int64{"Library": 5000, "Movies": 1000})

	saved, err := Save("first", res, 2)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Name != "first" || saved.Total != 6000 || saved.Depth != 2 {
		t.Errorf("saved = %+v", saved.Meta)
	}

	loaded, err := Load("first")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Result == nil || loaded.Result.Total != 6000 {
		t.Fatalf("loaded result = %+v", loaded.Result)
	}
	if len(loaded.Result.Tree.Children) != 2 {
		t.Errorf("children = %d, want 2", len(loaded.Result.Tree.Children))
	}

	metas, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 1 || metas[0].Name != "first" {
		t.Fatalf("metas = %+v", metas)
	}

	if err := Delete("first"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	metas, _ = List()
	if len(metas) != 0 {
		t.Errorf("snapshot was not deleted: %+v", metas)
	}
	if _, err := Load("first"); err == nil {
		t.Error("loading a deleted snapshot should fail")
	}
}

func TestSaveDefaultNameAndPrune(t *testing.T) {
	isolate(t)
	res := tree("/data/home", map[string]int64{"x": 10})
	for i := 0; i < 3; i++ {
		if _, err := Save("", res, 1); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	metas, _ := List()
	if len(metas) != 3 {
		t.Fatalf("metas = %d, want 3", len(metas))
	}
	removed, err := Prune(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Errorf("removed = %v, want 2", removed)
	}
	metas, _ = List()
	if len(metas) != 1 {
		t.Errorf("metas after prune = %d, want 1", len(metas))
	}
}

func TestCompare(t *testing.T) {
	a := &Snapshot{
		Meta:   Meta{Name: "a", Created: time.Now(), Root: "/data", Total: 1000, Files: 2},
		Result: tree("/data", map[string]int64{"grow": 100, "shrink": 400, "gone": 300}),
	}
	b := &Snapshot{
		Meta:   Meta{Name: "b", Created: time.Now(), Root: "/data", Total: 1600, Files: 3},
		Result: tree("/data", map[string]int64{"grow": 900, "shrink": 100, "fresh": 500}),
	}
	d := Compare(a, b)
	if d.TotalDelta != 600 {
		t.Errorf("total delta = %d, want 600", d.TotalDelta)
	}
	byPath := map[string]Change{}
	for _, c := range d.Changes {
		byPath[c.Path] = c
		if c.Path == "" {
			t.Error("the tree root should not be reported as a change")
		}
	}
	if c := byPath["grow"]; c.Delta != 800 || c.Kind != KindGrown {
		t.Errorf("grow = %+v", c)
	}
	if c := byPath["shrink"]; c.Delta != -300 || c.Kind != KindShrunk {
		t.Errorf("shrink = %+v", c)
	}
	if c := byPath["gone"]; c.Kind != KindRemoved || c.Delta != -300 {
		t.Errorf("gone = %+v", c)
	}
	if c := byPath["fresh"]; c.Kind != KindAdded || c.Delta != 500 {
		t.Errorf("fresh = %+v", c)
	}
	// Sorted by absolute delta: 800, 500, -300, -300.
	if d.Changes[0].Path != "grow" || d.Changes[1].Path != "fresh" {
		t.Errorf("changes are not ordered by size: %+v", d.Changes)
	}
}

func TestLatestBefore(t *testing.T) {
	dir := isolate(t)
	res := tree(filepath.Join(dir, "data"), map[string]int64{"x": 1})
	if _, err := Save("older", res, 1); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := Save("newer", res, 1); err != nil {
		t.Fatal(err)
	}
	meta, err := LatestBefore("unsaved", res.Root)
	if err != nil {
		t.Fatalf("LatestBefore: %v", err)
	}
	if meta == nil || meta.Name != "newer" {
		t.Errorf("meta = %+v, want newer", meta)
	}
}

func TestPathStaysInsideTheStore(t *testing.T) {
	isolate(t)
	store, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../etc/passwd", "a/b/c", "..", "  spaced  "} {
		p, err := Path(name)
		if err != nil {
			continue
		}
		if filepath.Dir(p) != store {
			t.Errorf("Path(%q) = %s, escapes the store %s", name, p, store)
		}
	}
	if _, err := Path("   "); err == nil {
		t.Error("a blank name should be rejected")
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}
