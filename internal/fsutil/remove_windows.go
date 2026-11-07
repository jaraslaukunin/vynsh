//go:build windows

package fsutil

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// ForceRemove deletes a single file, clearing the read-only attribute first when
// Windows rejects the operation.
func ForceRemove(p string) error {
	if err := os.Remove(p); err == nil {
		return nil
	}
	_ = clearReadOnly(p)
	return os.Remove(p)
}

// ForceRemoveAll deletes a tree, retrying after clearing read-only attributes.
func ForceRemoveAll(p string) error {
	err := os.RemoveAll(p)
	if err == nil {
		return nil
	}
	clearReadOnlyDeep(p)
	return os.RemoveAll(p)
}

func clearReadOnly(p string) error {
	pw, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(pw)
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_READONLY == 0 {
		return nil
	}
	return windows.SetFileAttributes(pw, attrs&^windows.FILE_ATTRIBUTE_READONLY)
}

func clearReadOnlyDeep(root string) {
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		_ = clearReadOnly(p)
		return nil
	})
}
