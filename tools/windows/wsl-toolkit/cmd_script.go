package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/script"
	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// cmdScript runs the embedded wsl-toolkit.ps1 with every argument forwarded
// unchanged.
//
// ⭐ The script is embedded rather than rewritten, so there is one
// implementation of the throwaway-distro behaviour rather than two that drift.
func cmdScript(ctx context.Context, args []string) (int, error) {
	if len(args) == 1 && (args[0] == "--extract-only" || args[0] == "--where") {
		path, err := extractScript()
		if err != nil {
			return exitCannot, err
		}
		fmt.Println(path)
		return exitOK, nil
	}
	path, err := extractScript()
	if err != nil {
		return exitCannot, err
	}
	host, err := findPowerShell()
	if err != nil {
		return exitCannot, err
	}
	full := append([]string{"-NoProfile", "-File", path}, args...)
	note("running the embedded script " + versionString() + " through " + filepath.Base(host))

	// ⛔ The child's streams are this process's own, not captured and replayed.
	// The script puts its heartbeat on stderr on purpose.
	code, err := toolkit.RunForeground(ctx, host, full)
	if err != nil && code == exitOK {
		return exitCannot, err
	}
	return code, nil
}

// extractScript writes the embedded product to the state directory and returns
// its path. ⭐ The file name carries the digest, so a different build cannot
// serve a stale one, and the common case is a stat rather than a write.
func extractScript() (string, error) {
	home, err := toolkit.EnsureHome()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "script")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	want := script.Bytes()
	path := filepath.Join(dir, "wsl-toolkit-"+script.Digest()[:12]+".ps1")
	if have, err := os.ReadFile(path); err == nil && bytes.Equal(have, want) {
		return path, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, want, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	// ⛔ Read it back. A write that reported success and produced different
	// bytes would run a script nobody wrote.
	back, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(back, want) {
		return "", fmt.Errorf("%s does not hold the embedded script after writing it", path)
	}
	return path, nil
}

// findPowerShell resolves a host for the script. ⚠ PowerShell 7 first: 5.1 runs
// it too, and loses a double quote when it builds a child's argument list, which
// nothing here can repair. The script's page names -CommandB64 as the answer.
func findPowerShell() (string, error) {
	var problems []string
	for _, name := range []string{"pwsh", "powershell"} {
		exe, err := toolkit.ResolveExecutable(name)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		return exe.Resolved, nil
	}
	return "", fmt.Errorf("no PowerShell host found: %s", strings.Join(problems, "; "))
}
