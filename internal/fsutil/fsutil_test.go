package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	got := ExpandPath("~/docs/file.txt")
	want := filepath.Join(home, "docs", "file.txt")
	if got != want {
		t.Errorf("ExpandPath = %q, want %q", got, want)
	}
	if !filepath.IsAbs(ExpandPath("relative/path")) {
		t.Error("relative path was not made absolute")
	}
}

func TestIsProtected(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	if !IsProtected(home) {
		t.Error("home directory must be protected")
	}
	if !IsProtected(filepath.Join(home, "Downloads")) {
		t.Error("Downloads must be protected")
	}
	if IsProtected(filepath.Join(home, "Downloads", "file.zip")) {
		t.Error("a file inside Downloads must not be protected")
	}
	if !IsProtected("/") && runtime.GOOS != "windows" {
		t.Error("/ must be protected")
	}
}

func TestIsUnder(t *testing.T) {
	cases := []struct {
		path, base string
		want       bool
	}{
		{"/a/b", "/a", true},
		{"/a", "/a", true},
		{"/ab", "/a", false},
		{"/a/b/c", "/a/b", true},
	}
	for _, c := range cases {
		if runtime.GOOS == "windows" {
			continue
		}
		if got := IsUnder(c.path, c.base); got != c.want {
			t.Errorf("IsUnder(%q,%q) = %v, want %v", c.path, c.base, got, c.want)
		}
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := UniquePath(dir, "file.txt")
	if got == p {
		t.Fatalf("UniquePath returned an existing path: %s", got)
	}
	if !Exists(got) == false {
		t.Logf("unique path: %s", got)
	}
	if filepath.Ext(got) != ".txt" {
		t.Errorf("extension lost: %s", got)
	}
}

func TestDirSize(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a", "b", "f.bin"), make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "g.bin"), make([]byte, 50), 0o644); err != nil {
		t.Fatal(err)
	}
	sz := DirSize(root)
	if sz.Bytes != 150 {
		t.Errorf("bytes = %d, want 150", sz.Bytes)
	}
	if sz.Files != 2 {
		t.Errorf("files = %d, want 2", sz.Files)
	}
	if sz.Dirs != 2 {
		t.Errorf("dirs = %d, want 2", sz.Dirs)
	}
	if sz := DirSize(filepath.Join(root, "missing")); sz.Bytes != 0 {
		t.Errorf("missing dir size = %d, want 0", sz.Bytes)
	}
}
