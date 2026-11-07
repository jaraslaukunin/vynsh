package trash

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectAndEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "folder", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "folder", "nested", "a.bin"), make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.bin"), make([]byte, 50), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	info := inspectDir(dir)
	if info.Items != 2 {
		t.Errorf("items = %d, want 2 (dot files ignored)", info.Items)
	}
	if info.Bytes != 150 {
		t.Errorf("bytes = %d, want 150", info.Bytes)
	}

	info, err := emptyDir(dir)
	if err != nil {
		t.Fatalf("emptyDir: %v", err)
	}
	if info.Items != 2 {
		t.Errorf("report items = %d, want 2", info.Items)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].Name() != ".DS_Store" {
		t.Errorf("directory should only keep .DS_Store, found %d entries", len(ents))
	}
}

func TestInspectMissingDir(t *testing.T) {
	info := inspectDir(filepath.Join(t.TempDir(), "nope"))
	if info.Err == "" {
		t.Error("expected an error for a missing directory")
	}
}

func TestMoveIntoUniqueNames(t *testing.T) {
	src := t.TempDir()
	a := filepath.Join(src, "report.pdf")
	if err := os.WriteFile(a, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()

	first, err := moveInto(a, dest)
	if err != nil {
		t.Fatalf("moveInto: %v", err)
	}
	if filepath.Base(first) != "report.pdf" {
		t.Errorf("first destination = %s", first)
	}
	if err := os.WriteFile(a, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := moveInto(a, dest)
	if err != nil {
		t.Fatalf("second moveInto: %v", err)
	}
	if second == first {
		t.Fatalf("second move reused the same name: %s", second)
	}
	data, err := os.ReadFile(second)
	if err != nil || string(data) != "two" {
		t.Fatalf("moved content = %q, err = %v", data, err)
	}
}

func TestMoveIntoDirectory(t *testing.T) {
	src := t.TempDir()
	sub := filepath.Join(src, "project")
	if err := os.MkdirAll(filepath.Join(sub, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "inner", "x.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	moved, err := moveInto(sub, dest)
	if err != nil {
		t.Fatalf("moveInto: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(moved, "inner", "x.txt")); err != nil || string(data) != "hello" {
		t.Fatalf("content lost: %q %v", data, err)
	}
}
