package toolkit

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sort"
	"strings"
	"time"
)

// The native survey answers the question the two script probes cannot agree on.
//
// The reported complaint: running scripts/doctor/doctor.sh and doctor.ps1 on one
// machine produces different answers about paths, and a restricted caller sees
// shims and symlinks where an unrestricted one sees the real file. Both are
// true, and neither is a defect in those scripts: a POSIX shell resolves a name
// through its own PATH and a PowerShell host resolves it through Windows'.
//
// ⭐ THIS ONE RESOLVES THE FILE AND SAYS WHAT KIND IT IS. A scoop shim, a node
// `.ps1` wrapper and a `.cmd` launcher are three different things that all
// answer to `--version`, and only one of them can be handed to CreateProcess.
// Reporting the resolved target and the kind is what turns "it is on PATH" into
// "here is what would actually run".

// WslFacts is what this process can find out about WSL right now.
//
// ⛔ "CAN THIS PROCESS CALL wsl.exe" IS A SEPARATE FACT FROM "IS WSL
// INSTALLED", and conflating them is the reported failure. A sandboxed caller
// gets E_ACCESSDENIED from a machine where WSL works perfectly, and the message
// Windows returns says neither which it is nor what to do.
type WslFacts struct {
	Present    bool     `json:"present"`
	Path       string   `json:"path,omitempty"`
	Callable   bool     `json:"callable"`
	Denied     bool     `json:"denied"`
	Reason     string   `json:"reason,omitempty"`
	Distros    []Distro `json:"distros,omitempty"`
	BaseName   string   `json:"base_name"`
	BaseExists bool     `json:"base_exists"`
	Helper     string   `json:"helper"`
	Advice     []string `json:"advice,omitempty"`
}

// ReadWslFacts asks, without changing anything.
func ReadWslFacts(ctx context.Context, cfg Config) WslFacts {
	f := WslFacts{BaseName: cfg.Base.Name, Helper: "not started"}
	w, err := FindWsl()
	if err != nil {
		f.Reason = err.Error()
		f.Advice = append(f.Advice, "wsl.exe was not found, so every distro action refuses by name rather than failing halfway")
		return f
	}
	f.Present, f.Path = true, w.Path
	bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	distros, err := w.List(bounded, cfg.Base.Name)
	if err != nil {
		f.Reason = err.Error()
		f.Denied = strings.Contains(err.Error(), ErrWslDenied.Error())
		if f.Denied {
			f.Advice = append(f.Advice,
				"wsl.exe is installed and THIS process is not allowed to call it. That is a property of how this process was started, not of WSL",
				"either make the call through this session's own approval path, or start the helper once through it: wsl-toolkit helper serve --detach")
		}
	} else {
		f.Callable = true
		f.Distros = distros
		for _, d := range distros {
			if d.Owned {
				f.BaseExists = true
			}
		}
		if !f.BaseExists {
			f.Advice = append(f.Advice, "the base distribution is not registered. Build it with: wsl-toolkit base ensure")
		}
	}
	if ep, err := ReadHelperEndpoint(); err == nil {
		f.Helper = ep.Address
	}
	return f
}

// WriteDoctorText renders the survey for a person.
//
// ⛔ IT GOES TO THE WRITER IT IS GIVEN AND NOWHERE ELSE. The caller decides
// whether that is stdout, and nothing here writes progress beside it.
func WriteDoctorText(w io.Writer, r DoctorReport) error {
	p := func(format string, args ...any) error {
		_, err := fmt.Fprintf(w, format, args...)
		return err
	}
	if err := p("wsl-toolkit doctor  %s  (%s)\n\n", r.Schema, r.Generated); err != nil {
		return err
	}

	if err := p("HOST\n"); err != nil {
		return err
	}
	hostKeys := []string{"os", "arch", "flavor", "kernel", "distro", "distro_version", "shell", "network"}
	for _, k := range hostKeys {
		if v, ok := r.Host[k]; ok && fmt.Sprint(v) != "" {
			if err := p("  %-14s %v\n", k, v); err != nil {
				return err
			}
		}
	}

	if r.Wsl != nil {
		if err := p("\nWSL\n"); err != nil {
			return err
		}
		if err := p("  %-14s %v\n", "installed", r.Wsl.Present); err != nil {
			return err
		}
		if r.Wsl.Path != "" {
			if err := p("  %-14s %s\n", "path", r.Wsl.Path); err != nil {
				return err
			}
		}
		// ⛔ THE TWO ROWS ARE SEPARATE ON PURPOSE. "installed" and "this process
		// may call it" are different facts and a reader acting on the first when
		// the second is false goes looking for a broken WSL.
		state := "yes"
		switch {
		case r.Wsl.Denied:
			state = "NO, this process is refused"
		case !r.Wsl.Callable:
			state = "no"
		}
		if err := p("  %-14s %s\n", "callable here", state); err != nil {
			return err
		}
		if r.Wsl.Reason != "" && !r.Wsl.Callable {
			if err := p("  %-14s %s\n", "reason", firstLine(r.Wsl.Reason)); err != nil {
				return err
			}
		}
		if err := p("  %-14s %s (%s)\n", "base distro", r.Wsl.BaseName, registeredWord(r.Wsl.BaseExists)); err != nil {
			return err
		}
		if err := p("  %-14s %s\n", "local helper", r.Wsl.Helper); err != nil {
			return err
		}
		for _, d := range r.Wsl.Distros {
			mark := " "
			if d.Owned {
				mark = "*"
			}
			runningWord := "stopped"
			if d.Running {
				runningWord = "running"
			}
			if err := p("  %s %-28s %s\n", mark, d.Name, runningWord); err != nil {
				return err
			}
		}
	}

	if err := p("\nTOOLS  (%d found, %d missing)\n", r.Summary["tools_found"], r.Summary["tools_missing"]); err != nil {
		return err
	}
	// ⭐ GROUPED, AND THE RESOLVED TARGET IS THE COLUMN THAT MATTERS. A name on
	// PATH is not an executable: a scoop shim, a node .ps1 wrapper and a .cmd
	// launcher all answer to --version and only one can be handed to
	// CreateProcess. This is the column the reported complaint is about.
	byGroup := map[string][]ToolProbe{}
	var groups []string
	for _, t := range r.Tools {
		if _, ok := byGroup[t.Group]; !ok {
			groups = append(groups, t.Group)
		}
		byGroup[t.Group] = append(byGroup[t.Group], t)
	}
	sort.Strings(groups)
	for _, g := range groups {
		if err := p("\n  -- %s\n", g); err != nil {
			return err
		}
		for _, t := range byGroup[g] {
			mark := "no "
			if t.Found {
				mark = "yes"
			}
			kind := t.Kind
			if kind == "executable" {
				kind = ""
			}
			line := fmt.Sprintf("  %s %-16s %-14s %-16s %s", mark, t.ID, t.Version, kind, t.Resolved)
			if t.Path != "" && t.Path != t.Resolved {
				line += "\n      via " + t.Path
			}
			if err := p("%s\n", strings.TrimRight(line, " ")); err != nil {
				return err
			}
			for _, n := range t.Notes {
				if err := p("      ! %s\n", n); err != nil {
					return err
				}
			}
		}
	}

	if len(r.Notes) > 0 {
		if err := p("\nNOTES\n"); err != nil {
			return err
		}
		for _, n := range r.Notes {
			if err := p("  %s\n", n); err != nil {
				return err
			}
		}
	}
	if r.Wsl != nil && len(r.Wsl.Advice) > 0 {
		if err := p("\nWHAT TO DO\n"); err != nil {
			return err
		}
		for _, a := range r.Wsl.Advice {
			if err := p("  %s\n", a); err != nil {
				return err
			}
		}
	}
	// ⭐ A PROBE IS NOT A GATE. A missing tool is data, so this exits 0 whether
	// or not anything is missing, exactly like scripts/doctor/ does.
	return p("\nThis is a probe, not a gate. A missing tool is data.\nMachine-readable: wsl-toolkit doctor --json\n")
}

func registeredWord(b bool) string {
	if b {
		return "registered"
	}
	return "not registered"
}

// hostShell names the shell this survey is running under, which is the point:
// it is Go, so it is neither of the two the script probes report.
func hostShell() string {
	return "native Go " + runtime.Version()
}
