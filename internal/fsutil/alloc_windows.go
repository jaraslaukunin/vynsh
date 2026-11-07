//go:build windows

package fsutil

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                   = windows.NewLazySystemDLL("kernel32.dll")
	procGetCompressedFileSizeW = kernel32.NewProc("GetCompressedFileSizeW")
)

// Allocated returns the bytes a file occupies on disk, using the sparse and
// compression aware Win32 API.
func Allocated(path string) int64 {
	if st, err := os.Lstat(path); err == nil {
		if st.IsDir() {
			return DirSize(path).Bytes
		}
		p, perr := windows.UTF16PtrFromString(path)
		if perr == nil {
			var high uint32
			low, _, callErr := procGetCompressedFileSizeW.Call(
				uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&high)))
			if callErr == nil && uint32(low) != 0xFFFFFFFF {
				alloc := int64(uint64(high)<<32 | uint64(uint32(low)))
				if alloc > 0 && alloc <= st.Size() {
					return alloc
				}
			}
		}
		return st.Size()
	}
	return 0
}
