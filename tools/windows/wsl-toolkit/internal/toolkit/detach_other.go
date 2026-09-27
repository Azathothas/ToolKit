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

// StartDetached starts a child in a session of its own. There is no job object
// here to leave, so the bool is always true.
func StartDetached(mk func() *exec.Cmd) (*exec.Cmd, bool, error) {
	c := mk()
	DetachProcess(c)
	if err := c.Start(); err != nil {
		return nil, false, err
	}
	return c, true, nil
}
