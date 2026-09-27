// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"syscall"
)

// errSharingViolation is ERROR_SHARING_VIOLATION: another handle is open on the
// file in a way this open cannot share.
const errSharingViolation = syscall.Errno(32)

// lockExclusive opens path with no sharing at all, so every other open fails
// while this handle is open, and Windows closes the handle when the process
// ends however it ends. That is the whole of the liveness signal.
func lockExclusive(path string) (func(), error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil,
		syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if errors.Is(err, errSharingViolation) {
			return nil, errLockHeld
		}
		return nil, err
	}
	return func() { _ = syscall.CloseHandle(h) }, nil
}

// lockProbe answers whether another process holds path, and whether the file
// exists at all. ⚠ A probe that succeeds holds the file for as long as two
// system calls take, which is why a claim retries a sharing violation.
func lockProbe(path string) (held, exists bool, err error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, false, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, 0, nil,
		syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	switch {
	case err == nil:
		_ = syscall.CloseHandle(h)
		return false, true, nil
	case errors.Is(err, errSharingViolation):
		return true, true, nil
	case errors.Is(err, syscall.ERROR_FILE_NOT_FOUND), errors.Is(err, syscall.ERROR_PATH_NOT_FOUND):
		return false, false, nil
	}
	return false, true, err
}
