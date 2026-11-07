//go:build windows

package trash

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/jaraslaukunin/vynsh/internal/fsutil"
)

var (
	shell32               = windows.NewLazySystemDLL("shell32.dll")
	procSHQueryRecycleBin = shell32.NewProc("SHQueryRecycleBinW")
	procSHEmptyRecycleBin = shell32.NewProc("SHEmptyRecycleBinW")
	procSHFileOperation   = shell32.NewProc("SHFileOperationW")
)

const (
	sherbNoConfirmation = 0x00000001
	sherbNoProgressUI   = 0x00000002
	sherbNoSound        = 0x00000004

	foDelete           = 0x0003
	fofSilent          = 0x0004
	fofNoConfirmation  = 0x0010
	fofAllowUndo       = 0x0040
	fofNoConfirmDir    = 0x0200
	fofNoErrorUI       = 0x0400
	fofWantNukeWarning = 0x4000
)

// shQueryRBInfo mirrors SHQUERYRBINFO.
type shQueryRBInfo struct {
	cbSize      uint32
	_           uint32
	i64Size     int64
	i64NumItems int64
}

// shFileOpStructW mirrors SHFILEOPSTRUCTW.
type shFileOpStructW struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

func coInit() func() {
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		return func() {}
	}
	return windows.CoUninitialize
}

// Locations returns the logical recycle bin locations Windows reports.
func locations() []string {
	return []string{"Recycle Bin"}
}

// Inspect queries the size and item count of the recycle bin across all drives.
func Inspect() (*Report, error) {
	rep := &Report{}
	info := Info{Path: "Recycle Bin"}
	sz, items, err := queryRecycleBin()
	if err != nil {
		info.Err = err.Error()
		rep.Locations = append(rep.Locations, info)
		return rep, err
	}
	info.Bytes = sz
	info.Items = items
	rep.Locations = append(rep.Locations, info)
	rep.Bytes = sz
	rep.Items = items
	return rep, nil
}

func queryRecycleBin() (int64, int64, error) {
	bi := shQueryRBInfo{cbSize: uint32(unsafe.Sizeof(shQueryRBInfo{}))}
	r1, _, _ := procSHQueryRecycleBin.Call(0, uintptr(unsafe.Pointer(&bi)))
	if int32(r1) < 0 {
		return 0, 0, fmt.Errorf("SHQueryRecycleBin failed (0x%x)", uint32(r1))
	}
	return bi.i64Size, bi.i64NumItems, nil
}

// Empty empties the recycle bin for all drives.
func Empty(dryRun bool) (*Report, error) {
	if dryRun {
		return Inspect()
	}
	defer coInit()()
	r1, _, _ := procSHEmptyRecycleBin.Call(0, 0, uintptr(sherbNoConfirmation|sherbNoProgressUI|sherbNoSound))
	// S_OK and S_FALSE (already empty) are both fine.
	if int32(r1) < 0 {
		return nil, fmt.Errorf("SHEmptyRecycleBin failed (0x%x)", uint32(r1))
	}
	return Inspect()
}

// Move sends paths to the recycle bin using the shell, so undo keeps working.
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
		if dryRun {
			res.Dest = "Recycle Bin"
			rep.Moved = append(rep.Moved, res)
			rep.Bytes += res.Bytes
			continue
		}
		if err := shellDelete(abs); err != nil {
			res.Err = err.Error()
			rep.Moved = append(rep.Moved, res)
			continue
		}
		res.Dest = "Recycle Bin"
		rep.Moved = append(rep.Moved, res)
		rep.Bytes += res.Bytes
	}
	return rep, nil
}

func shellDelete(p string) error {
	defer coInit()()
	from, err := windows.UTF16FromString(p)
	if err != nil {
		return err
	}
	// SHFileOperation expects a double NUL terminated list.
	from = append(from, 0)
	op := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI | fofNoConfirmDir | fofWantNukeWarning,
	}
	r1, _, _ := procSHFileOperation.Call(uintptr(unsafe.Pointer(&op)))
	if r1 != 0 {
		return fmt.Errorf("SHFileOperation failed (0x%x)", uint32(r1))
	}
	if op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("operation aborted by the shell")
	}
	return nil
}

// Supported reports whether a trash facility exists.
func Supported() bool { return true }
