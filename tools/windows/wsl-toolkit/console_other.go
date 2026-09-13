// SPDX-License-Identifier: 0BSD

//go:build !windows

package main

import "os"

// isConsole reads the file mode. This tool acts only on Windows; off it, the
// suite needs an answer, and a character device is the nearest one.
func isConsole(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
