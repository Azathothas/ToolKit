// SPDX-License-Identifier: 0BSD

//go:build !windows

package compat

import (
	"path/filepath"
	"syscall"
)

// freeBytes answers on the volume holding a path where the host can answer at
// all. The executable manages Windows hosts, so this exists so the suite can
// exercise the callers of the measurement on Linux; a host where statfs
// refuses reports "could not measure", which is the third answer and never a
// fabricated zero.
func freeBytes(path string) (int64, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, false
	}
	for {
		var st syscall.Statfs_t
		if err := syscall.Statfs(abs, &st); err == nil {
			return int64(st.Bavail) * int64(st.Bsize), true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return 0, false
		}
		abs = parent
	}
}
