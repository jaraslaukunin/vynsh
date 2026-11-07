// Package fsutil holds small, cross-platform file system helpers shared by the
// other vynsh packages.
package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// HomeDir returns the current user's home directory.
func HomeDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return ""
}

// ExpandPath expands a leading "~", environment variables and makes the path
// absolute. Relative paths are resolved against the working directory. The
// result uses the OS native separator and never ends with a trailing separator
// (except for a filesystem root).
func ExpandPath(p string) string {
	if p == "" {
		return ""
	}
	p = os.ExpandEnv(p)
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home := HomeDir(); home != "" {
			p = filepath.Join(home, p[1:])
		}
	}
	if !filepath.IsAbs(p) {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
	}
	return filepath.Clean(p)
}

// Exists reports whether the path exists.
func Exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// IsDir reports whether the path exists and is a directory (symlinks followed).
func IsDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// Size holds the accumulated size of a directory tree.
type Size struct {
	Bytes int64
	Files int64
	Dirs  int64
}

// DirSize walks root and sums regular file sizes without following symlinks.
// Unreadable entries are skipped rather than aborting the walk.
func DirSize(root string) Size {
	var out Size
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root && d == nil {
				return nil
			}
			if d != nil && d.IsDir() && p != root {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != root {
				out.Dirs++
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Mode().IsRegular() {
			out.Bytes += info.Size()
			out.Files++
		}
		return nil
	})
	return out
}

// UniquePath returns a path inside dir that does not exist yet, appending a
// numeric suffix when needed (like macOS does for duplicate trash items).
func UniquePath(dir, base string) string {
	candidate := filepath.Join(dir, base)
	if !Exists(candidate) {
		return candidate
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = base
		ext = ""
	}
	for i := 1; i < 10000; i++ {
		candidate = filepath.Join(dir, stem+" "+itoa(i)+ext)
		if !Exists(candidate) {
			return candidate
		}
	}
	return filepath.Join(dir, stem+" "+itoa(int(time.Now().UnixNano()))+ext)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

// protectedAlways lists paths that must never be deleted by vynsh, regardless
// of user flags. Comparisons are case-insensitive on Windows.
var protectedAlways = []string{"/", "/System", "/Library", "/usr", "/bin", "/sbin", "/etc", "/var", "/private", "/Applications"}

var protectedWindows = []string{
	`C:\`, `C:\Windows`, `C:\Program Files`, `C:\Program Files (x86)`, `C:\ProgramData`, `C:\Users`, `C:\Users\Public`,
}

// ProtectedPaths returns the absolute paths that vynsh refuses to delete.
func ProtectedPaths() []string {
	home := HomeDir()
	out := []string{}
	if runtime.GOOS == "windows" {
		out = append(out, protectedWindows...)
	} else {
		out = append(out, protectedAlways...)
	}
	if home != "" {
		out = append(out, home, filepath.Join(home, "Desktop"), filepath.Join(home, "Documents"), filepath.Join(home, "Downloads"))
	}
	out = append(out, extraProtected...)
	return out
}

// IsProtected reports whether path is one of the paths vynsh must never remove.
func IsProtected(path string) bool {
	p := filepath.Clean(path)
	norm := func(s string) string {
		s = filepath.Clean(s)
		if runtime.GOOS == "windows" {
			s = strings.ToLower(s)
		}
		return s
	}
	np := norm(p)
	for _, pp := range ProtectedPaths() {
		if np == norm(pp) {
			return true
		}
	}
	return false
}

// IsUnder reports whether path is inside (or equal to) base. Comparison is
// case-insensitive on Windows.
func IsUnder(path, base string) bool {
	p := filepath.Clean(path)
	b := filepath.Clean(base)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
		b = strings.ToLower(b)
	}
	rel, err := filepath.Rel(b, p)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// ErrUnsupported marks an operation that this platform does not implement.
var ErrUnsupported = errors.New("operation not supported on this platform")

// extraProtected holds user-configured paths that must never be deleted.
var extraProtected []string

// AddProtected registers additional protected paths (from the config file).
func AddProtected(paths ...string) {
	for _, p := range paths {
		if p != "" {
			extraProtected = append(extraProtected, ExpandPath(p))
		}
	}
}
