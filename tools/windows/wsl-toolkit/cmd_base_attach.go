// SPDX-License-Identifier: 0BSD

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// baseAttach is what `base attach` answers: the commands that reach the base's
// herdr server, with this caller's values filled in.
type baseAttach struct {
	Schema       string   `json:"schema"`
	Distribution string   `json:"distribution"`
	Account      string   `json:"account"`
	SSHAlias     string   `json:"ssh_alias"`
	Windows      string   `json:"windows"`
	Linux        []string `json:"linux"`
	Agents       string   `json:"agents"`
	Problems     []string `json:"problems,omitempty"`
}

// cmdBaseAttach prints how to attach, and attaches nothing.
//
// ⭐ PRINTED RATHER THAN RUN, because the two callers want different things. The
// operator pastes the Windows line into a terminal of their own, and an agent
// needs the lines as data. Running herdr's client from here would take the
// terminal of whichever process asked.
//
// ⛔ `--remote-keybindings server` IS PART OF THE LINE, AND IT IS NOT DECORATION.
// herdr's documentation says a remote attach uses the client's own keys unless it
// is passed, and a Windows client with no configuration has herdr's defaults, under
// which prefix then x closed a pane in the base at once, measured on 2026-09-14. The
// base's tracked configuration protects only a client that uses it.
func cmdBaseAttach(cfg toolkit.Config, asJSON bool) (int, error) {
	ans := attachAnswer(cfg)
	code := exitOK
	if len(ans.Problems) > 0 {
		code = exitFailed
	}
	if asJSON {
		return code, writeJSON(ans)
	}
	fmt.Println(ans.Windows)
	fmt.Fprintf(os.Stderr, "  from Windows   %s\n", ans.Windows)
	fmt.Fprintf(os.Stderr, "  in the base    %s, then: %s\n", ans.Linux[0], ans.Linux[1])
	fmt.Fprintf(os.Stderr, "  for an agent   %s\n", ans.Agents)
	fmt.Fprintf(os.Stderr, "  detach with prefix then q, where the prefix is ctrl+b. Panes and agents keep running\n")
	for _, p := range ans.Problems {
		fmt.Fprintf(os.Stderr, "  ! %s\n", p)
	}
	return code, nil
}

// attachAnswer is the document `base attach` prints.
func attachAnswer(cfg toolkit.Config) baseAttach {
	alias, problems := toolkit.HerdrDoor(cfg)
	invocation := toolInvocation()
	// ⭐ THE CLIENT OF THE BUILD THE BASE RUNS, when the herdr adapter follows the nightly
	// channel: a 0.9.0 client has no --machine and cannot type into a development server
	// from Windows. Quoted for PowerShell, and run with the call operator. WSL-90.
	client := "herdr"
	if path := toolkit.HerdrWindowsClient(cfg); path != "" {
		client = "& '" + strings.ReplaceAll(path, "'", "''") + "'"
	}
	return baseAttach{
		Schema:       "wsl-toolkit-base-attach/1",
		Distribution: cfg.Base.Name,
		Account:      cfg.Base.User,
		SSHAlias:     alias,
		Windows:      client + " --remote " + alias + " --remote-keybindings server",
		Linux:        []string{invocation + " base shell", "herdr"},
		Agents:       invocation + " base herdr -- agent list",
		Problems:     problems,
	}
}

// toolInvocation is how this process was pointed at its base, as a command a
// reader can paste into PowerShell: the instance and the configuration file, when
// either was chosen.
func toolInvocation() string {
	parts := []string{"wsl-toolkit"}
	if name := toolkit.SelectedInstance.Name; name != "" && name != toolkit.DefaultInstance {
		parts = append(parts, "--instance", name)
	}
	if path := toolkit.ExplicitConfigPath; path != "" {
		// ⚠ ABSOLUTE, because the line is pasted into another terminal, which may
		// stand in another directory. Measured: `--config .tmp\wsl76\wsl-toolkit.json`
		// came back as typed.
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		if strings.ContainsAny(path, " '\t") {
			path = "'" + strings.ReplaceAll(path, "'", "''") + "'"
		}
		parts = append(parts, "--config", path)
	}
	return strings.Join(parts, " ")
}
