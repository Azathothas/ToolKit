// Package forksafe writes a file that no process forked meanwhile holds open.
//
// ⛔ A CHILD FORKED WHILE A FILE IS OPEN FOR WRITING KEEPS THAT DESCRIPTOR
// until it execs. Running the file in that window fails with ETXTBSY, "text
// file busy". Go issue 22315. The Linux proof's go job failed on it: a parallel
// case wrote a stand-in, another case started a process, and the helper that
// ran the stand-in exited 126. WSL-205.
//
// ⭐ syscall.ForkLock IS HELD FOR READING across the open, the write and the
// close. Every fork takes it for writing, so no fork overlaps the descriptor.
//
// ⚠ ONLY TEST FILES CALL THIS. Every test file of this module writes through
// it, and a case in this package reads them for a call that does not.
//
// SPDX-License-Identifier: 0BSD
package forksafe

import (
	"os"
	"syscall"
)

// WriteFile is os.WriteFile, with no fork while the file is open.
func WriteFile(name string, data []byte, perm os.FileMode) error {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	return os.WriteFile(name, data, perm)
}
