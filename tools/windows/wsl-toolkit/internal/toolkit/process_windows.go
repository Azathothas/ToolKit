package toolkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

func configureProcess(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return os.ErrProcessDone
		}
		// A timed-out shim can leave its real executable behind. Restrict the
		// tree kill to the process this command created.
		kill := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "taskkill.exe"), "/PID", strconv.Itoa(c.Process.Pid), "/T", "/F")
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := kill.Run(); err != nil {
			return c.Process.Kill()
		}
		return nil
	}
}
