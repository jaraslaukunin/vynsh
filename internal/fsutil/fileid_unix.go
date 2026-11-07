//go:build !windows

package fsutil

import (
	"io/fs"
	"syscall"
)

// FileID returns the device, inode and hard link count of a file when the
// platform exposes them.
func FileID(info fs.FileInfo) (dev, ino, nlink uint64, ok bool) {
	st, isStat := info.Sys().(*syscall.Stat_t)
	if !isStat || st == nil {
		return 0, 0, 0, false
	}
	return uint64(st.Dev), uint64(st.Ino), uint64(st.Nlink), true
}
