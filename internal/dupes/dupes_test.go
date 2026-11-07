package dupes

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindGroupsIdenticalFiles(t *testing.T) {
	root := t.TempDir()
	payload := make([]byte, 4096)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	a := filepath.Join(root, "a", "one.bin")
	b := filepath.Join(root, "b", "two.bin")
	c := filepath.Join(root, "c", "three.bin")
	write(t, a, payload)
	write(t, b, payload)
	write(t, c, payload)
	write(t, filepath.Join(root, "unique.bin"), []byte("something else entirely"))

	res, err := Find([]string{root}, Options{MinSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Groups) != 1 {
		t.Fatalf("groups = %d, want 1 (%+v)", len(res.Groups), res.Groups)
	}
	g := res.Groups[0]
	if len(g.Paths) != 3 {
		t.Errorf("paths = %v, want 3", g.Paths)
	}
	if g.Size != 4096 {
		t.Errorf("size = %d, want 4096", g.Size)
	}
	if res.Wasted != 2*4096 {
		t.Errorf("wasted = %d, want %d", res.Wasted, 2*4096)
	}
}

func TestFindIgnoresSameSizeDifferentContent(t *testing.T) {
	root := t.TempDir()
	shared := make([]byte, 8192)
	a := append([]byte{}, shared...)
	b := append([]byte{}, shared...)
	b[0] = 1 // same length, same prefix region differs at byte 0
	write(t, filepath.Join(root, "a.bin"), a)
	write(t, filepath.Join(root, "b.bin"), b)

	res, err := Find([]string{root}, Options{MinSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Groups) != 0 {
		t.Fatalf("groups = %+v, want none", res.Groups)
	}
}

func TestFindRespectsMinSize(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a.txt"), []byte("tiny"))
	write(t, filepath.Join(root, "b.txt"), []byte("tiny"))
	res, err := Find([]string{root}, Options{MinSize: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesScanned != 0 || len(res.Groups) != 0 {
		t.Fatalf("res = %+v, want nothing scanned", res)
	}
}

func TestFindLargeFilePrefixCollision(t *testing.T) {
	// Files sharing the first 64 KiB but differing later must not be reported
	// as duplicates (partial hashing alone would be a false positive).
	root := t.TempDir()
	head := make([]byte, 70*1024)
	a := append(append([]byte{}, head...), []byte("AAA")...)
	b := append(append([]byte{}, head...), []byte("BBB")...)
	write(t, filepath.Join(root, "a.bin"), a)
	write(t, filepath.Join(root, "b.bin"), b)

	res, err := Find([]string{root}, Options{MinSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Groups) != 0 {
		t.Fatalf("groups = %+v, want none", res.Groups)
	}
}
