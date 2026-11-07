//go:build !darwin && !windows && !linux

package trash

// Inspect returns ErrNotSupported.
func Inspect() (*Report, error) { return nil, ErrNotSupported }

// Empty returns ErrNotSupported.
func Empty(dryRun bool) (*Report, error) { return nil, ErrNotSupported }

// Move returns ErrNotSupported.
func Move(paths []string, dryRun bool) (*MoveReport, error) { return nil, ErrNotSupported }

// Supported reports that this platform has no trash integration.
func Supported() bool { return false }
