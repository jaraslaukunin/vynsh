package scan

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunTotalsAndTree(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	writeFile(t, filepath.Join(root, "sub", "b.bin"), 2000)
	writeFile(t, filepath.Join(root, "sub", "deep", "c.bin"), 3000)
	writeFile(t, filepath.Join(root, "other", "d.log"), 50)

	res, err := Run(Options{Root: root, Depth: 2, TopFiles: 3, TopDirs: 5, NoDefault: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 5150 {
		t.Errorf("Total = %d, want 5150", res.Total)
	}
	if res.Files != 4 {
		t.Errorf("Files = %d, want 4", res.Files)
	}
	if res.Dirs != 3 {
		t.Errorf("Dirs = %d, want 3", res.Dirs)
	}
	if len(res.TopFiles) != 3 {
		t.Fatalf("TopFiles = %d, want 3", len(res.TopFiles))
	}
	if res.TopFiles[0].Size != 3000 {
		t.Errorf("biggest file = %d, want 3000", res.TopFiles[0].Size)
	}
	if res.Tree.Size != 5150 {
		t.Errorf("tree size = %d, want 5150", res.Tree.Size)
	}
	sort.Slice(res.Tree.Children, func(i, j int) bool { return res.Tree.Children[i].Size > res.Tree.Children[j].Size })
	if len(res.Tree.Children) != 2 {
		t.Fatalf("root children = %d, want 2", len(res.Tree.Children))
	}
	if res.Tree.Children[0].Name != "sub" || res.Tree.Children[0].Size != 5000 {
		t.Errorf("first child = %+v, want sub/5000", res.Tree.Children[0])
	}
	// .bin should dominate the extension stats.
	if len(res.ByExt) == 0 || res.ByExt[0].Ext != ".bin" || res.ByExt[0].Size != 5000 {
		t.Errorf("ByExt[0] = %+v, want .bin/5000", res.ByExt)
	}
}

func TestDepthLimitsTree(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a", "b", "c", "big.bin"), 4096)
	res, err := Run(Options{Root: root, Depth: 1, NoDefault: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 4096 {
		t.Errorf("Total = %d, want 4096", res.Total)
	}
	if len(res.Tree.Children) != 1 {
		t.Fatalf("children = %d, want 1", len(res.Tree.Children))
	}
	child := res.Tree.Children[0]
	if child.Size != 4096 || child.Files != 1 {
		t.Errorf("child = %+v, want size 4096 files 1", child)
	}
	if len(child.Children) != 0 {
		t.Errorf("depth 1 must not materialise grandchildren, got %d", len(child.Children))
	}
}

func TestSingleFileRoot(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "one.bin")
	writeFile(t, f, 777)
	res, err := Run(Options{Root: f})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 777 || res.Files != 1 {
		t.Errorf("got total=%d files=%d, want 777/1", res.Total, res.Files)
	}
}

func TestExcludes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep", "a.bin"), 10)
	writeFile(t, filepath.Join(root, "skipme", "b.bin"), 20)
	res, err := Run(Options{Root: root, Depth: 2, Exclude: []string{filepath.Join(root, "skipme")}, NoDefault: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 10 {
		t.Errorf("Total = %d, want 10 (skipme excluded)", res.Total)
	}
}

func TestSymlinksNotFollowed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "huge.bin"), 1<<20)
	writeFile(t, filepath.Join(root, "small.bin"), 10)
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	res, err := Run(Options{Root: root, Depth: 2, NoDefault: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Total != 10 {
		t.Errorf("Total = %d, want 10 (symlink must not be followed)", res.Total)
	}
}
