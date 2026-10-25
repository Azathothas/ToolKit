//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

// lock_flock.go - a slot's lock is flock(2) on its lock file.
//
// ⭐ THE KERNEL DROPS THE LOCK WITH THE LAST DESCRIPTOR, so a run that is killed
// gives its slots back the moment it dies.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"os"
	"syscall"
)

// lockSlot takes the lock at path, or says another process holds it.
func lockSlot(path string) (unlock func(), ok bool) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, false
	}
	return func() { _ = f.Close() }, true
}
