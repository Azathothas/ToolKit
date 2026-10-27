// SPDX-License-Identifier: 0BSD

package toolkit

import "syscall"

// shortPath is path's 8.3 name, which holds no space, and false where the volume
// keeps none. WSL-198.
func shortPath(path string) (string, bool) {
	long, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", false
	}
	buf := make([]uint16, 1024)
	n, err := syscall.GetShortPathName(long, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) >= len(buf) {
		return "", false
	}
	return syscall.UTF16ToString(buf[:n]), true
}
