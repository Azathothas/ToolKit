// SPDX-License-Identifier: 0BSD

//go:build windows

package compat

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// freeBytes answers on the volume holding a path, walking up to the nearest
// existing ancestor, because the directory an import is about to create does
// not exist yet and the volume is the same either way.
func freeBytes(path string) (int64, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, false
	}
	for {
		p, err := syscall.UTF16PtrFromString(abs)
		if err != nil {
			return 0, false
		}
		var free, total, totalFree uint64
		r, _, _ := procGetDiskFreeSpaceEx.Call(
			uintptr(unsafe.Pointer(p)),
			uintptr(unsafe.Pointer(&free)),
			uintptr(unsafe.Pointer(&total)),
			uintptr(unsafe.Pointer(&totalFree)),
		)
		if r != 0 {
			return int64(free), true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return 0, false
		}
		abs = parent
	}
}
