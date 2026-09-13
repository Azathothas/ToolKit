// SPDX-License-Identifier: 0BSD

package compat

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// executablePath resolves a program name through PATH.
func executablePath(name string) (string, error) {
	return exec.LookPath(name)
}

// engineCommand starts an engine command with the environment this process
// already has. The engine is not WSL, so it does not get wslEnv.
func engineCommand(path string, args []string) *exec.Cmd {
	return exec.Command(path, args...)
}

// captureEngine runs an engine command and captures both streams merged,
// refusing a non-zero exit with what it said.
func captureEngine(path string, args []string) (string, error) {
	var buf strings.Builder
	cmd := engineCommand(path, args)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	text := buf.String()
	if err != nil {
		var ee *exitStatus
		if asExitStatus(err, &ee) {
			return text, fmt.Errorf("the engine answered exit %d: %s", ee.code, strings.TrimSpace(text))
		}
		return text, err
	}
	return text, nil
}

// exitStatus is a child's own non-zero exit, as distinct from a child that
// could not be started at all.
type exitStatus struct{ code int }

func (e *exitStatus) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// asExitStatus unwraps a child's own non-zero exit from a Run error.
func asExitStatus(err error, target **exitStatus) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		*target = &exitStatus{code: ee.ExitCode()}
		return true
	}
	return false
}

// nativeArgumentString joins arguments into the one command-line string a
// dry-run plan prints. ⛔ AN ARGUMENT CARRYING A DOUBLE QUOTE OR A BACKSLASH
// IS REFUSED RATHER THAN ESCAPED. Every argument the plan prints is one this
// tool built, so a hand-rolled escape for a case that cannot occur is how a
// quoting bug gets written and never exercised; the refusal is the guard the
// script's host provided, ported.
func nativeArgumentString(args []string) (string, error) {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		if strings.ContainsAny(a, `"\\`) {
			return "", fmt.Errorf("refusing to build a command line containing a quote or a backslash: '%s'. "+
				"This tool passes only arguments it built itself.", a)
		}
		if strings.ContainsAny(a, " \t") {
			parts = append(parts, `"`+a+`"`)
			continue
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " "), nil
}
