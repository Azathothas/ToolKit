package toolkit

import (
	"os/exec"
	"syscall"
)

// DetachProcess makes a child survive its parent.
//
// CREATE_NEW_PROCESS_GROUP stops a Ctrl-C in the parent's console from reaching
// it, and DETACHED_PROCESS gives it no console at all. Without both, the helper
// dies with the shell that started it, which is the opposite of what starting it
// once through an approval path is for.
func DetachProcess(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x00000200 | 0x00000008, // CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS
	}
}
