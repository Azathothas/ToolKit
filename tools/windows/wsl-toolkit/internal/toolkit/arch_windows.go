// SPDX-License-Identifier: 0BSD

//go:build windows

package toolkit

import (
	"syscall"
	"unsafe"
)

var procIsWow64Process2 = kernel32.NewProc("IsWow64Process2")

// readNativeMachine asks IsWow64Process2 for the machine's own architecture.
//
// ⚠ IsWow64Process2 is in Windows 10 1511 and later. An older Windows has no
// such call, and the answer is false, so the executable's own architecture
// stands in, as it did before. WSL-188.
func readNativeMachine() (uint16, bool) {
	if procIsWow64Process2.Find() != nil {
		return 0, false
	}
	self, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, false
	}
	var process, native uint16
	r, _, _ := procIsWow64Process2.Call(uintptr(self), uintptr(unsafe.Pointer(&process)), uintptr(unsafe.Pointer(&native)))
	if r == 0 || native == 0 {
		return 0, false
	}
	return native, true
}
