//go:build linux

package trash

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/jaraslaukunin/vynsh/internal/fsutil"
)

// dataHome returns $XDG_DATA_HOME (or ~/.local/share).
func dataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	return filepath.Join(fsutil.HomeDir(), ".local", "share")
}

// homeTrash is the freedesktop.org trash directory of the current user.
func homeTrash() string { return filepath.Join(dataHome(), "Trash") }

// locations lists the trash directories visible to the user.
func locations() []string {
	out := []string{filepath.Join(homeTrash(), "files")}
	// Per-volume trash directories live at the mount point root.
	if mounts, err := os.ReadDir("/media"); err == nil {
		for _, m := range mounts {
			out = append(out, filepath.Join("/media", m.Name(), fmt.Sprintf(".Trash-%d", os.Getuid()), "files"))
		}
	}
	return out
}

// Inspect reports the contents of every trash location.
func Inspect() (*Report, error) {
	rep := &Report{}
	for _, dir := range locations() {
		info := inspectDir(dir)
		rep.Locations = append(rep.Locations, info)
		rep.Items += info.Items
		rep.Bytes += info.Bytes
	}
	return rep, nil
}

// Empty removes the contents of the trash (both files and their .trashinfo
// metadata, so nothing is left half-referenced).
func Empty(dryRun bool) (*Report, error) {
	if dryRun {
		return Inspect()
	}
	rep := &Report{}
	var firstErr error
	for _, dir := range locations() {
		info, err := emptyDir(dir)
		rep.Locations = append(rep.Locations, info)
		rep.Items += info.Items
		rep.Bytes += info.Bytes
		if err != nil && firstErr == nil {
			firstErr = err
		}
		// Drop the matching metadata directory as well.
		if filepath.Base(dir) == "files" {
			if ierr := os.RemoveAll(filepath.Join(filepath.Dir(dir), "info")); ierr != nil && firstErr == nil {
				firstErr = ierr
			}
		}
	}
	return rep, firstErr
}

// Move moves paths into the freedesktop.org trash, writing the .trashinfo file
// that makes them restorable by any file manager.
func Move(paths []string, dryRun bool) (*MoveReport, error) {
	rep := &MoveReport{Dry: dryRun}
	filesDir := filepath.Join(homeTrash(), "files")
	infoDir := filepath.Join(homeTrash(), "info")
	for _, p := range paths {
		abs := fsutil.ExpandPath(p)
		res := MoveResult{Path: abs, DryRun: dryRun, Bytes: sizeOf(abs)}
		if !fsutil.Exists(abs) {
			res.Err = "does not exist"
			rep.Moved = append(rep.Moved, res)
			continue
		}
		if fsutil.IsProtected(abs) {
			res.Err = "refusing to move a protected system path"
			rep.Moved = append(rep.Moved, res)
			continue
		}
		if dryRun {
			res.Dest = filepath.Join(filesDir, filepath.Base(abs))
			rep.Moved = append(rep.Moved, res)
			rep.Bytes += res.Bytes
			continue
		}
		if err := os.MkdirAll(filesDir, 0o700); err != nil {
			res.Err = err.Error()
			rep.Moved = append(rep.Moved, res)
			continue
		}
		if err := os.MkdirAll(infoDir, 0o700); err != nil {
			res.Err = err.Error()
			rep.Moved = append(rep.Moved, res)
			continue
		}
		wasDir := fsutil.IsDir(abs)
		base := filepath.Base(abs)
		dest := filepath.Join(filesDir, fmt.Sprintf("%s.%d", base, time.Now().UnixNano()))
		if err := os.Rename(abs, dest); err != nil {
			if cerr := copyTree(abs, dest); cerr != nil {
				res.Err = cerr.Error()
				rep.Moved = append(rep.Moved, res)
				continue
			}
			_ = fsutil.ForceRemoveAll(abs)
		}
		info := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
			(&url.URL{Path: abs}).String(), time.Now().Format("2006-01-02T15:04:05"))
		if err := os.WriteFile(filepath.Join(infoDir, filepath.Base(dest)+".trashinfo"), []byte(info), 0o600); err != nil {
			res.Err = err.Error()
		}
		appendRecord(Record{Original: abs, Trashed: dest, Bytes: res.Bytes, IsDir: wasDir})
		res.Dest = dest
		rep.Moved = append(rep.Moved, res)
		rep.Bytes += res.Bytes
	}
	return rep, nil
}

// Supported reports whether a trash facility exists.
func Supported() bool { return true }
