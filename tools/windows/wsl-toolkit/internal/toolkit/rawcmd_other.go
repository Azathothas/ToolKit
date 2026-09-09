//go:build !windows

package toolkit

import "os/exec"

// setRawCommandLine has nothing to do off Windows: there is no command script
// to run and no CreateProcess to route around.
func setRawCommandLine(*exec.Cmd, string) {}
