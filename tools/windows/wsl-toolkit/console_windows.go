// SPDX-License-Identifier: 0BSD

//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

var procGetConsoleMode = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleMode")

// isConsole reports whether a file is a console a person can type into or read.
//
// ⛔ A CHARACTER DEVICE IS NOT A CONSOLE ON WINDOWS. NUL is one, and it is what an
// agent's stdin is usually attached to, so a check on the file mode alone called
// that session interactive: `distro remove` then printed a prompt nobody could
// answer, read end of file, and said "not confirmed" instead of naming --yes.
// GetConsoleMode answers only for a real console handle.
func isConsole(f *os.File) bool {
	if f == nil {
		return false
	}
	var mode uint32
	ok, _, _ := procGetConsoleMode.Call(f.Fd(), uintptr(unsafe.Pointer(&mode)))
	return ok != 0
}
