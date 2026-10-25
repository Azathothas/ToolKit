//go:build windows

// lock_windows.go - a slot's lock is its lock file held open with no sharing.
//
// ⭐ A SECOND OPEN FAILS UNTIL THE FIRST HANDLE CLOSES, and Windows closes every
// handle of a process that exits, killed or not. So the lock cannot outlive the
// run that took it, which is the property an age threshold only guessed at.
//
// SPDX-License-Identifier: 0BSD

package mutate

import "syscall"

// lockSlot takes the lock at path, or says another process holds it.
func lockSlot(path string) (unlock func(), ok bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, false
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, false
	}
	return func() { _ = syscall.CloseHandle(h) }, true
}
