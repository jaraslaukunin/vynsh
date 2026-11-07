package trash

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jaraslaukunin/vynsh/internal/fsutil"
)

// isolate points the trash and the manifest at a temporary home directory.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Skip("the Windows trash goes through the shell and cannot be faked here")
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	return home
}

func TestMoveRecordsManifestAndRestore(t *testing.T) {
	home := isolate(t)
	work := filepath.Join(home, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(work, "notes.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := Move([]string{file}, false)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(rep.Moved) != 1 || rep.Moved[0].Err != "" {
		t.Fatalf("move report = %+v", rep)
	}
	if fsutil.Exists(file) {
		t.Fatal("file should have left its original location")
	}
	if !fsutil.Exists(rep.Moved[0].Dest) {
		t.Fatalf("file is not in the trash: %s", rep.Moved[0].Dest)
	}

	records, err := Restorable()
	if err != nil {
		t.Fatalf("Restorable: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Original != file {
		t.Errorf("original = %s, want %s", records[0].Original, file)
	}
	if records[0].Bytes != 5 {
		t.Errorf("bytes = %d, want 5", records[0].Bytes)
	}

	// A dry run must not touch anything.
	dry, err := Restore(records, true)
	if err != nil {
		t.Fatalf("dry Restore: %v", err)
	}
	if len(dry.Results) != 1 || dry.Results[0].Restored != file {
		t.Errorf("dry run results = %+v", dry.Results)
	}
	if fsutil.Exists(file) {
		t.Fatal("dry run restored the file for real")
	}

	res, err := Restore(records, false)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(res.Results) != 1 || res.Results[0].Err != "" {
		t.Fatalf("restore results = %+v", res.Results)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("file was not restored: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q", data)
	}
	// It is no longer restorable.
	again, _ := Restorable()
	if len(again) != 0 {
		t.Errorf("records after restore = %+v, want none", again)
	}
}

func TestRestoreKeepsBothWhenOriginalExists(t *testing.T) {
	home := isolate(t)
	file := filepath.Join(home, "report.pdf")
	if err := os.WriteFile(file, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Move([]string{file}, false); err != nil {
		t.Fatalf("Move: %v", err)
	}
	// The user recreated the file in the meantime.
	if err := os.WriteFile(file, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	records, err := Restorable()
	if err != nil || len(records) != 1 {
		t.Fatalf("records = %+v, err = %v", records, err)
	}
	res, err := Restore(records, false)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got := res.Results[0].Restored
	if got == file {
		t.Fatalf("restore must not overwrite the existing file")
	}
	if !strings.Contains(filepath.Base(got), "report") {
		t.Errorf("restored name = %s, want a report… variant", got)
	}
	if data, _ := os.ReadFile(file); string(data) != "v2" {
		t.Errorf("existing file was modified: %q", data)
	}
	if data, _ := os.ReadFile(got); string(data) != "v1" {
		t.Errorf("restored file content = %q, want v1", data)
	}
}

func TestRestoreMissingItem(t *testing.T) {
	isolate(t)
	recs := []Record{{Original: "/nowhere/x", Trashed: "/nowhere/trash/x", Bytes: 1}}
	rep, err := Restore(recs, false)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(rep.Results) != 1 || rep.Results[0].Err == "" {
		t.Fatalf("results = %+v, want an error", rep.Results)
	}
}

func TestMoveRefusesProtectedPaths(t *testing.T) {
	home := isolate(t)
	rep, err := Move([]string{home}, false)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(rep.Moved) != 1 || rep.Moved[0].Err == "" {
		t.Fatalf("moving the home directory must fail: %+v", rep.Moved)
	}
	if !fsutil.Exists(home) {
		t.Fatal("home directory disappeared!")
	}
}
