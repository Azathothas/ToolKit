// SPDX-License-Identifier: 0BSD

//go:build !windows

package toolkit

import (
	"errors"
	"os"
	"syscall"
)

// lockExclusive holds an exclusive flock on path until the returned function
// runs or the process ends. The executable is Windows-only; this half exists so
// the suite proves the owner protocol on the second host it runs on.
func lockExclusive(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errLockHeld
		}
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}

// lockProbe answers whether another process holds path, and whether it exists.
func lockProbe(path string) (held, exists bool, err error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, true, err
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return true, true, nil
		}
		return false, true, err
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false, true, nil
}
