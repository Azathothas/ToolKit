package toolkit

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var procGetFinalPathNameByHandleW = kernel32.NewProc("GetFinalPathNameByHandleW")

// RealPath answers what a path actually is on disk, following the link kinds
// Windows has.
//
// ⛔ filepath.EvalSymlinks IS NOT ENOUGH HERE, and that is the whole reason this
// exists. Measured on this host on 2026-09-09: every tool scoop installs sits
// behind a directory JUNCTION named `current`, and EvalSymlinks failed on all of
// them, so the first survey reported node, ruby and java as ABSENT on a machine
// where all three run. GetFinalPathNameByHandle asks the filesystem what an open
// handle refers to, which resolves a junction, a symlink and a mount point
// alike. It is the answer the reported complaint asks for: the true path rather
// than the shim.
//
// ⚠ The API returns the extended-length form, so the prefix is trimmed. Leaving
// it on produces a path that is correct and that no reader recognises.
func RealPath(path string) (string, error) {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		return p, nil
	}
	p16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	// FILE_FLAG_BACKUP_SEMANTICS is what lets a DIRECTORY be opened as a
	// handle, and the junction case is a directory.
	h, err := syscall.CreateFile(p16, 0,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer func() {
		// Nothing reads this handle after here and a close failure cannot
		// change the answer, so it is discarded explicitly rather than by
		// omission.
		_ = syscall.CloseHandle(h)
	}()
	buf := make([]uint16, 1024)
	for attempt := 0; attempt < 2; attempt++ {
		n, _, callErr := procGetFinalPathNameByHandleW.Call(
			uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
		switch {
		case n == 0:
			return "", callErr
		case int(n) >= len(buf):
			// The return is the required length when the buffer was too small.
			buf = make([]uint16, n+1)
		default:
			return trimExtendedPrefix(syscall.UTF16ToString(buf[:n])), nil
		}
	}
	return "", syscall.ERROR_INSUFFICIENT_BUFFER
}

func trimExtendedPrefix(p string) string {
	if rest, ok := strings.CutPrefix(p, `\\?\UNC\`); ok {
		return `\\` + rest
	}
	if rest, ok := strings.CutPrefix(p, `\\?\`); ok {
		return rest
	}
	return p
}
