//go:build darwin

package trash

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"

	"github.com/jaraslaukunin/vynsh/internal/fsutil"
)

// homeTrash returns the user's ~/.Trash directory.
func homeTrash() string { return filepath.Join(fsutil.HomeDir(), ".Trash") }

// Locations returns every trash directory reachable for the current user:
// the home trash plus per-volume .Trashes directories for mounted volumes.
func locations() []string {
	out := []string{homeTrash()}
	parts, err := disk.Partitions(false)
	if err != nil {
		return out
	}
	uid := fmt.Sprintf("%d", os.Getuid())
	for _, p := range parts {
		mp := p.Mountpoint
		if mp == "/" || strings.HasPrefix(mp, "/System/Volumes/") || strings.Contains(mp, "/Library/Developer/") {
			continue
		}
		if _, err := os.Stat(filepath.Join(mp, ".Trashes")); err == nil {
			out = append(out, filepath.Join(mp, ".Trashes", uid))
		}
	}
	return out
}

// Inspect reports the contents of all trash locations.
func Inspect() (*Report, error) {
	rep := &Report{}
	for _, dir := range locations() {
		info := inspectDir(dir)
		info.Path = dir
		rep.Locations = append(rep.Locations, info)
		rep.Items += info.Items
		rep.Bytes += info.Bytes
	}
	return rep, nil
}

// Empty removes the contents of every trash location. With dryRun nothing is
// deleted and only the report is returned.
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
	}
	return rep, firstErr
}

// Move sends paths to the trash directory that lives on the same volume.
func Move(paths []string, dryRun bool) (*MoveReport, error) {
	rep := &MoveReport{Dry: dryRun}
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
		dir := trashDirFor(abs)
		if dir == "" {
			res.Err = "no writable trash directory on this volume"
			rep.Moved = append(rep.Moved, res)
			continue
		}
		if dryRun {
			res.Dest = filepath.Join(dir, filepath.Base(abs))
			rep.Moved = append(rep.Moved, res)
			rep.Bytes += res.Bytes
			continue
		}
		wasDir := fsutil.IsDir(abs)
		dest, err := moveInto(abs, dir)
		if err != nil {
			res.Err = withHint(err).Error()
			rep.Moved = append(rep.Moved, res)
			continue
		}
		appendRecord(Record{Original: abs, Trashed: dest, Bytes: res.Bytes, IsDir: wasDir})
		res.Dest = dest
		rep.Moved = append(rep.Moved, res)
		rep.Bytes += res.Bytes
	}
	return rep, nil
}

// trashDirFor picks the trash directory on the same volume as p.
func trashDirFor(p string) string {
	if fsutil.IsUnder(p, fsutil.HomeDir()) {
		return homeTrash()
	}
	parts, err := disk.Partitions(false)
	if err != nil {
		return ""
	}
	uid := fmt.Sprintf("%d", os.Getuid())
	best := ""
	for _, part := range parts {
		if part.Mountpoint == "/" || strings.HasPrefix(part.Mountpoint, "/System/Volumes/") || strings.Contains(part.Mountpoint, "/Library/Developer/") {
			continue
		}
		if fsutil.IsUnder(p, part.Mountpoint) && len(part.Mountpoint) > len(best) {
			best = part.Mountpoint
		}
	}
	if best == "" {
		return homeTrash()
	}
	dir := filepath.Join(best, ".Trashes", uid)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	return dir
}

// Supported reports whether a trash facility exists.
func Supported() bool { return true }
