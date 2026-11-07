//go:build windows

package fsutil

import "io/fs"

// FileID reports that inode information is not available through os.FileInfo on
// Windows; callers fall back to counting every file once.
func FileID(info fs.FileInfo) (dev, ino, nlink uint64, ok bool) {
	return 0, 0, 1, false
}
