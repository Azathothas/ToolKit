//go:build !windows

package toolkit

import (
	"os/exec"
	"syscall"
)

// DetachProcess puts the child in its own process group, so a signal to the
// parent's group does not reach it.
func DetachProcess(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
