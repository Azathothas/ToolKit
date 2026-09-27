package toolkit

import (
	"errors"
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup  = 0x00000200
	detachedProcess        = 0x00000008
	createBreakawayFromJob = 0x01000000
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
		CreationFlags: createNewProcessGroup | detachedProcess,
	}
}

// StartDetached starts a child that outlives this process, and leaves the job
// object this process is in where that object permits it, so a harness that
// closes its job object does not end the child with it. The bool is false where
// the child stays in that job object.
//
// ⚠ A JOB OBJECT MAY REFUSE A BREAKAWAY, and CreateProcess then answers access
// denied. The start is made again without it, from a fresh command, because a
// command cannot be started twice.
func StartDetached(mk func() *exec.Cmd) (*exec.Cmd, bool, error) {
	c := mk()
	c.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNewProcessGroup | detachedProcess | createBreakawayFromJob,
	}
	err := c.Start()
	if err == nil {
		return c, true, nil
	}
	if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		return nil, false, err
	}
	c = mk()
	DetachProcess(c)
	if err := c.Start(); err != nil {
		return nil, false, err
	}
	return c, false, nil
}
