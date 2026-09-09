package toolkit

import "os/exec"

// setRawCommandLine hands Windows the exact command line, bypassing Go's own
// escaping. It is used only where the callee is cmd.exe.
func setRawCommandLine(c *exec.Cmd, line string) {
	if c.SysProcAttr == nil {
		configureProcess(c)
	}
	c.SysProcAttr.CmdLine = line
}
