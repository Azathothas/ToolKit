// SPDX-License-Identifier: 0BSD

package compat

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The WSL helpers, ported from the script's wsl-host.ps1.

// resolveWsl is the one hook every wsl.exe path goes through. It is a
// package variable so the suite can hold a fake WSL in front of the decisions
// that read it: a guard whose answer depends on which host the suite runs on
// is a guard CI cannot hold.
var resolveWsl = findWsl

// findWsl is Get-WslExe: PATH first, then the System32 fallback, and a
// refusal that says WSL2 is required rather than a confusion deeper in.
func findWsl() (string, error) {
	if p, err := exec.LookPath("wsl.exe"); err == nil {
		return p, nil
	}
	if windir := envValue("WINDIR"); windir != "" {
		fallback := filepath.Join(windir, "System32", "wsl.exe")
		if st, err := os.Stat(fallback); err == nil && !st.IsDir() {
			return fallback, nil
		}
	}
	return "", fmt.Errorf("wsl.exe not found. WSL2 is required.")
}

// distroNames is Get-WslDistroNames: every distribution registered on this
// machine.
//
// ⛔ AN ENUMERATION THAT WAS REFUSED THROWS, it does not return an empty
// list. Discarding the reason folds "WSL said no distributions" together with
// "WSL would not answer me", so a process that cannot reach WSL is told the
// machine is empty and the command exits 0. An agent reading that answer
// concludes there is nothing here.
//
// ⚠ A machine with genuinely no distributions is a real answer and stays an
// empty list. The two are separated by the EXIT CODE and what came back on
// stderr, not by the emptiness of the output.
func (s *session) distroNames() ([]string, error) {
	wsl, err := resolveWsl()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(wsl, "--list", "--quiet")
	cmd.Env = wslEnv()
	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	text := out.String()
	errText := errBuf.String()
	if runErr != nil {
		code := 1
		if ee, ok := runErr.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			return nil, fmt.Errorf("could not list the distributions on this machine: %v", runErr)
		}
		_, err := resolveDistroListing(code, splitLines(text), errText)
		return nil, err
	}
	names, err := resolveDistroListing(0, splitLines(text), errText)
	if err != nil {
		return nil, err
	}
	return names, nil
}

// resolveDistroListing is the DECISION half of distroNames, separated so it
// can be proved without WSL: a pure function of the child's exit code, what it
// wrote, and what it wrote to stderr.
func resolveDistroListing(exitCode int, lines []string, errorText string) ([]string, error) {
	if exitCode != 0 {
		why := strings.Trim(strings.ReplaceAll(errorText, "\x00", ""), " \t\r\n")
		if why == "" {
			why = fmt.Sprintf("it exited %d and said nothing", exitCode)
		}
		return nil, fmt.Errorf("could not list the distributions on this machine: %s", why)
	}
	names := []string{}
	for _, line := range lines {
		// Belt and braces: strip NULs in case WSL_UTF8 is unsupported here.
		clean := strings.TrimSpace(strings.ReplaceAll(line, "\x00", ""))
		if clean != "" {
			names = append(names, clean)
		}
	}
	// ⚠ A MACHINE WITH NO DISTRIBUTIONS IS A REAL ANSWER and stays an empty
	// list. Only the exit code separates it from a refusal.
	return names, nil
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(text, "\n")
}

// networkingMode is which networking mode WSL is configured for, read from
// %USERPROFILE%\.wslconfig without starting anything.
//
// ⛔ A COMMENTED SETTING IS NOT A SETTING. Real .wslconfig files carry the
// alternatives commented out above the live one, which is how Microsoft's own
// example is written. A parser that grepped for the key would answer mirrored
// on a host running NAT, which is the wrong answer in the direction that costs
// an hour: 127.0.0.1 is a plausible address that never connects.
//
// ⚠ THE SECTION MATTERS. `[experimental]` carries keys with related names, and
// only `[wsl2]` sets this one. ⚠ LAST ONE WINS, because that is what an ini
// parser does and what WSL does.
func networkingMode() networkingAnswer {
	profile := envValue("USERPROFILE")
	if strings.TrimSpace(profile) != "" {
		candidate := filepath.Join(profile, ".wslconfig")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			body, err := os.ReadFile(candidate)
			if err == nil {
				mode := parseWslConfigMode(string(body))
				if mode != "" {
					return networkingAnswer{Mode: mode, Source: ".wslconfig", Path: candidate}
				}
				return networkingAnswer{Mode: "nat", Source: "the WSL default, no key in .wslconfig", Path: candidate}
			}
		}
	}
	return networkingAnswer{Mode: "nat", Source: "the WSL default, no .wslconfig", Path: ""}
}

type networkingAnswer struct {
	Mode   string
	Source string
	Path   string
}

// parseWslConfigMode is the pure half of networkingMode, so the parser is
// proved without a file. Empty means the key was not set.
func parseWslConfigMode(body string) string {
	section := ""
	mode := ""
	for _, raw := range splitLines(body) {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		if section != "wsl2" {
			continue
		}
		if strings.HasPrefix(line, "networkingMode") {
			rest := strings.TrimPrefix(line, "networkingMode")
			rest = strings.TrimLeft(rest, " \t")
			if strings.HasPrefix(rest, "=") {
				// ⛔ THE VALUE ENDS AT THE FIRST WHITESPACE OR COMMENT, which is
				// what the script's regex captured: `networkingMode=nat nat`
				// has one live value and a parser that took both words would
				// answer with a mode WSL never configured.
				value := strings.TrimSpace(strings.TrimPrefix(rest, "="))
				if i := strings.IndexAny(value, " \t#;"); i >= 0 {
					value = value[:i]
				}
				value = strings.ToLower(value)
				if value != "" {
					mode = value
				}
			}
		}
	}
	return mode
}
