//go:build !windows

package fsutil

import "os"

// ForceRemove deletes a single file or empty directory.
func ForceRemove(p string) error { return os.Remove(p) }

// ForceRemoveAll deletes a path recursively; on Unix this is os.RemoveAll.
func ForceRemoveAll(p string) error { return os.RemoveAll(p) }
