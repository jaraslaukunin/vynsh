package del

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mk(t *testing.T, root, name string, size int, mod time.Time) string {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	if !mod.IsZero() {
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestBuildFiltersByAge(t *testing.T) {
	root := t.TempDir()
	old := time.Now().Add(-100 * 24 * time.Hour)
	oldFile := mk(t, root, "old.log", 10, old)
	newFile := mk(t, root, "new.log", 10, time.Now())

	plan, err := Build([]string{root}, Filter{Files: true, OlderThan: 30 * 24 * time.Hour}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 || plan.Items[0].Path != oldFile {
		t.Fatalf("items = %+v, want only %s", plan.Items, oldFile)
	}
	_ = newFile
	if plan.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", plan.Skipped)
	}
}

func TestBuildFiltersBySizeAndExt(t *testing.T) {
	root := t.TempDir()
	big := mk(t, root, "big.zip", 5000, time.Now())
	mk(t, root, "small.zip", 10, time.Now())
	mk(t, root, "big.txt", 5000, time.Now())

	plan, err := Build([]string{root}, Filter{Files: true, MinSize: 1000, Exts: []string{"zip"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 || plan.Items[0].Path != big {
		t.Fatalf("items = %+v, want only big.zip", plan.Items)
	}
}

func TestBuildNameGlobAndExclude(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "draft-1.csv", 10, time.Now())
	mk(t, root, "draft-2.csv", 10, time.Now())
	keep := mk(t, root, "draft-keep.csv", 10, time.Now())

	plan, err := Build([]string{root}, Filter{Files: true, Names: []string{"draft-*.csv"}, ExcludeNames: []string{"*keep*"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("items = %d, want 2 (%+v)", len(plan.Items), plan.Items)
	}
	for _, it := range plan.Items {
		if it.Path == keep {
			t.Fatalf("excluded file %s was selected", keep)
		}
	}
}

func TestBuildNestedDepthLimit(t *testing.T) {
	root := t.TempDir()
	mk(t, root, "top.txt", 1, time.Now())
	mk(t, root, filepath.Join("a", "b", "deep.txt"), 1, time.Now())

	plan, err := Build([]string{root}, Filter{Files: true, MaxDepth: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 || filepath.Base(plan.Items[0].Path) != "top.txt" {
		t.Fatalf("items = %+v, want only top.txt", plan.Items)
	}
}

func TestBuildSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := mk(t, t.TempDir(), "target.txt", 10, time.Now())
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	plan, err := Build([]string{root}, Filter{Files: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 0 {
		t.Fatalf("items = %+v, want none", plan.Items)
	}
}

func TestApplyDryRunDoesNotDelete(t *testing.T) {
	root := t.TempDir()
	f := mk(t, root, "a.txt", 10, time.Now())
	plan, err := Build([]string{root}, Filter{Files: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := Apply(plan, ApplyOptions{DryRun: true})
	if len(res.Deleted) != 1 || res.Bytes != 10 {
		t.Fatalf("res = %+v, want 1 item/10 bytes", res)
	}
	if _, err := os.Stat(f); err != nil {
		t.Fatalf("dry run deleted the file: %v", err)
	}
}

func TestApplyDeletesFilesAndEmptyDirs(t *testing.T) {
	root := t.TempDir()
	mk(t, root, filepath.Join("sub", "a.txt"), 10, time.Now())
	mk(t, root, "keep.txt", 10, time.Now())

	plan, err := Build([]string{filepath.Join(root, "sub")}, Filter{Files: true, Dirs: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := Apply(plan, ApplyOptions{PruneDirs: true})
	if len(res.Failed) != 0 {
		t.Fatalf("failed = %+v", res.Failed)
	}
	if _, err := os.Stat(filepath.Join(root, "sub")); !os.IsNotExist(err) {
		t.Fatalf("sub directory should be gone, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "keep.txt")); err != nil {
		t.Fatalf("keep.txt must stay: %v", err)
	}
}

func TestApplyKeepsDirWithContent(t *testing.T) {
	root := t.TempDir()
	mk(t, root, filepath.Join("sub", "a.txt"), 10, time.Now())
	mk(t, root, filepath.Join("sub", "b.txt"), 10, time.Now())

	// Only a.txt matches: sub must survive because b.txt is still there.
	plan, err := Build([]string{root}, Filter{Files: true, Dirs: true, Names: []string{"a.txt"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	Apply(plan, ApplyOptions{PruneDirs: true})
	if _, err := os.Stat(filepath.Join(root, "sub", "b.txt")); err != nil {
		t.Fatalf("b.txt must stay: %v", err)
	}
}

func TestApplyLimit(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		mk(t, root, filepath.Join("f", string(rune('a'+i))+".txt"), 1, time.Now())
	}
	plan, err := Build([]string{root}, Filter{Files: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := Apply(plan, ApplyOptions{Limit: 2})
	if !res.Limited || len(res.Deleted) != 2 {
		t.Fatalf("res = %+v, want limited to 2", res)
	}
}

func TestBuildGlobRoots(t *testing.T) {
	root := t.TempDir()
	a := mk(t, root, "a.bin", 10, time.Now())
	b := mk(t, root, "b.bin", 10, time.Now())
	mk(t, root, "c.txt", 10, time.Now())

	plan, err := Build([]string{filepath.Join(root, "*.bin")}, Filter{Files: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(plan.Items))
	}
	seen := map[string]bool{}
	for _, it := range plan.Items {
		seen[it.Path] = true
	}
	if !seen[a] || !seen[b] {
		t.Errorf("expected %s and %s, got %+v", a, b, plan.Items)
	}
}
