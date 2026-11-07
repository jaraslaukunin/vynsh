//go:build !windows

package fsutil

import (
	"os"
	"syscall"
)

// Allocated returns the number of bytes a file really occupies on disk. It is
// smaller than the logical size for sparse files such as VM disk images; the
// logical size is returned when the platform does not expose allocations.
func Allocated(path string) int64 {
	st, err := os.Lstat(path)
	if err != nil {
		return 0
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok || sys == nil {
		return st.Size()
	}
	alloc := int64(sys.Blocks) * 512
	if alloc <= 0 || alloc > st.Size() {
		return st.Size()
	}
	return alloc
}
