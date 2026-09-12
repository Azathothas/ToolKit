package toolkit

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Executable struct {
	Path     string   `json:"path"`
	Resolved string   `json:"resolved_path"`
	Kind     string   `json:"kind"`
	Notes    []string `json:"notes,omitempty"`
}

// ResolveExecutable does not equate a PATH hit with an executable that runs.
// It resolves filesystem links and Scoop descriptors, and keeps looking after
// inaccessible app-execution aliases. It never installs a second tool.
func ResolveExecutable(name string) (Executable, error) {
	return resolveExecutable(name, nil)
}

// resolveExecutable can skip candidates that a caller already proved unusable.
func resolveExecutable(name string, skip map[string]bool) (Executable, error) {
	var candidates []string
	if strings.ContainsAny(name, `/\`) {
		candidates = append(candidates, name)
	} else {
		if p, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, p)
		}
		dirs := filepath.SplitList(os.Getenv("PATH"))
		if runtime.GOOS == "windows" {
			for _, root := range []string{os.Getenv("USERPROFILE"), os.Getenv("ProgramData")} {
				if root != "" {
					dirs = append(dirs, filepath.Join(root, "scoop", "shims"))
				}
			}
			for _, pair := range [][2]string{
				{"SystemRoot", "System32"}, {"ProgramFiles", "Git/cmd"},
				{"ProgramFiles", "PowerShell/7"}, {"ProgramFiles", "GitHub CLI"},
				{"ProgramFiles", "Go/bin"}, {"ProgramFiles", "nodejs"},
				{"USERPROFILE", ".cargo/bin"}, {"USERPROFILE", ".local/bin"},
				{"USERPROFILE", "go/bin"}, {"LOCALAPPDATA", "Microsoft/WinGet/Links"},
				{"ProgramFiles", "WinGet/Links"},
			} {
				if root := os.Getenv(pair[0]); root != "" {
					dirs = append(dirs, filepath.Join(root, filepath.FromSlash(pair[1])))
				}
			}
			if name == "powershell" {
				dirs = append(dirs, filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0"))
			}
		} else {
			dirs = append(dirs, "/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin")
		}
		exts := []string{""}
		if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
			exts = []string{".exe", ".com", ".cmd", ".bat", ".ps1"}
		}
		for _, dir := range dirs {
			if dir == "" || dir == "." {
				continue
			}
			for _, ext := range exts {
				candidates = append(candidates, filepath.Join(dir, name+ext))
			}
		}
	}
	seen := map[string]bool{}
	var notes []string
	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		key := abs
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		if skip[key] {
			continue
		}
		st, err := os.Stat(abs)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				notes = append(notes, "inaccessible candidate: "+abs)
			}
			continue
		}
		if !st.Mode().IsRegular() {
			// ⚠ A WINDOWS APP EXECUTION ALIAS IS A ZERO-BYTE REPARSE POINT, so
			// this is where python3 and winget land. It is not a regular file
			// and it does run, so it is reported as what it is rather than as
			// absent: the version probe below is what decides whether it works.
			if runtime.GOOS == "windows" && st.Size() == 0 && st.Mode()&os.ModeIrregular != 0 {
				return Executable{Path: abs, Resolved: abs, Kind: "app-execution-alias", Notes: notes}, nil
			}
			continue
		}
		if runtime.GOOS != "windows" && st.Mode()&0111 == 0 {
			continue
		}
		// ⛔ A PATH THAT WILL NOT CANONICALISE IS STILL AN EXECUTABLE. The first
		// version of this skipped one, and on this machine that reported node,
		// ruby and java as ABSENT on a host where all three run: scoop installs
		// through a directory JUNCTION at `current`, and a junction whose target
		// this process may not read fails EvalSymlinks while the file behind it
		// executes perfectly. Reporting a working tool as missing is the more
		// expensive error, so the unresolved path is kept and the note says the
		// canonical one is unknown.
		kind := "executable"
		resolved, err := RealPath(abs)
		if err != nil {
			resolved = abs
			kind = "unresolved-link"
			notes = append(notes, "the canonical path is unknown: "+abs+" is a link this process cannot follow")
		}
		switch strings.ToLower(filepath.Ext(resolved)) {
		case ".ps1":
			kind = "powershell-script"
		case ".cmd", ".bat":
			kind = "command-script"
		}
		if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(abs), ".exe") {
			if b, err := os.ReadFile(strings.TrimSuffix(abs, filepath.Ext(abs)) + ".shim"); err == nil {
				for _, line := range strings.Split(string(b), "\n") {
					k, value, ok := strings.Cut(line, "=")
					if !ok || strings.TrimSpace(k) != "path" {
						continue
					}
					target := strings.Trim(strings.TrimSpace(value), `"`)
					if t, err := RealPath(target); err == nil {
						if info, err := os.Stat(t); err == nil && info.Mode().IsRegular() {
							resolved = t
							kind = "scoop-target"
						}
					}
				}
			}
		}
		return Executable{Path: abs, Resolved: resolved, Kind: kind, Notes: notes}, nil
	}
	return Executable{Notes: notes}, fmt.Errorf("%s: no accessible executable found", name)
}
