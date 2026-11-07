// Package trash inspects and empties the operating system's recycle bin and
// moves files into it. The implementation is platform specific.
package trash

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jaraslaukunin/vynsh/internal/fsutil"
)

// Info describes the contents of one trash location.
type Info struct {
	Path  string `json:"path"`
	Items int64  `json:"items"`
	Bytes int64  `json:"bytes"`
	Err   string `json:"error,omitempty"`
}

// Report is the aggregated view of all trash locations.
type Report struct {
	Locations []Info `json:"locations"`
	Items     int64  `json:"items"`
	Bytes     int64  `json:"bytes"`
}

// MoveResult records one move-to-trash operation.
type MoveResult struct {
	Path   string `json:"path"`
	Dest   string `json:"dest,omitempty"`
	Bytes  int64  `json:"bytes"`
	DryRun bool   `json:"dry_run"`
	Err    string `json:"error,omitempty"`
}

// MoveReport aggregates move results.
type MoveReport struct {
	Moved []MoveResult `json:"moved"`
	Bytes int64        `json:"bytes"`
	Dry   bool         `json:"dry_run"`
}

// ErrNotSupported is returned by platforms without a recycle bin.
var ErrNotSupported = errors.New("trash is not supported on this platform")

// inspectDir walks a single trash directory.
func inspectDir(dir string) Info {
	info := Info{Path: dir}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		info.Err = "not present"
		return info
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		info.Err = err.Error()
		return info
	}
	for _, e := range ents {
		if e.Name() == ".DS_Store" || e.Name() == "desktop.ini" {
			continue
		}
		info.Items++
		full := filepath.Join(dir, e.Name())
		if e.IsDir() {
			info.Bytes += fsutil.DirSize(full).Bytes
		} else if fi, err := e.Info(); err == nil {
			info.Bytes += fi.Size()
		}
	}
	return info
}

// emptyDir removes everything inside a trash directory, keeping the directory.
func emptyDir(dir string) (Info, error) {
	info := inspectDir(dir)
	if info.Err == "not present" {
		return info, nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return info, err
	}
	var firstErr error
	for _, e := range ents {
		if e.Name() == ".DS_Store" || e.Name() == "desktop.ini" {
			continue
		}
		if err := fsutil.ForceRemoveAll(filepath.Join(dir, e.Name())); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return info, firstErr
}

// moveInto renames src into dir, giving it a unique name on collision. If the
// rename fails because the destination is on another device, the tree is copied
// and the source removed.
func moveInto(src, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dest := fsutil.UniquePath(dir, filepath.Base(src))
	if err := os.Rename(src, dest); err == nil {
		return dest, nil
	}
	if err := copyTree(src, dest); err != nil {
		return "", err
	}
	if err := fsutil.ForceRemoveAll(src); err != nil {
		return dest, err
	}
	return dest, nil
}

func copyTree(src, dest string) error {
	st, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(link, dest)
	}
	if !st.IsDir() {
		return copyFile(src, dest, st.Mode())
	}
	if err := os.MkdirAll(dest, st.Mode().Perm()); err != nil {
		return err
	}
	ents, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dest, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// sizeOf returns the size of a path, following nothing.
func sizeOf(p string) int64 {
	st, err := os.Lstat(p)
	if err != nil {
		return 0
	}
	if st.IsDir() {
		return fsutil.DirSize(p).Bytes
	}
	return st.Size()
}

// withHint adds the usual macOS cause to permission errors, which are almost
// always TCC (Privacy & Security) rather than a broken path.
func withHint(err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("%w — grant your terminal “Full Disk Access” in System Settings → Privacy & Security", err)
	}
	return err
}
