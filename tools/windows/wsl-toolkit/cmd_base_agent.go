// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

const baseAgentUsage = `wsl-toolkit base agent NAME [--] ARGS...

  Run an agent adapter's command in the base, in the guest directory this Windows
  directory is granted at. Every argument after NAME is the agent's, and reaches it
  unchanged; its exit code is the answer. A directory no grant covers is refused
  with the command that grants it. The agent's own screen runs in a herdr pane.

  The adapter's launcher, NAME.exe in the account's bin directory, is this command
  for the base instance.
`

// cmdBaseAgent runs an agent adapter's command in the base, in the guest directory the
// caller's Windows directory is granted at, with the caller's arguments unchanged.
//
// ⛔ EVERYTHING AFTER THE AGENT'S NAME IS THE AGENT'S. No flag of this tool is read
// from it, so an argument that happens to spell one reaches the agent as written.
//
// ⛔ ONE PATH INTO THE GUEST. The command travels framed on stdin, as `base exec`
// sends it, with /dev/null as its stdin, and its exit code comes back unchanged. The
// agent's own screen needs a terminal, so it runs in a herdr pane, and this names the
// way there rather than starting it where it cannot draw. WSL-78.
func cmdBaseAgent(ctx context.Context, args []string) (int, error) {
	// ⚠ REGISTERED AND NEVER PARSED. The manual documents every form from the flag
	// set it registers, and this form has no flag of its own to parse out of an
	// agent's arguments.
	_ = newFlagSet("base agent")
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(os.Stderr, baseAgentUsage)
		return exitOK, flag.ErrHelp
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprint(os.Stderr, baseAgentUsage)
		return exitCannot, errors.New("base agent takes an agent's name, then the agent's own arguments: base agent muse -- exec --json PROMPT")
	}
	name, passed := args[0], args[1:]
	if len(passed) > 0 && passed[0] == "--" {
		passed = passed[1:]
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	command, err := toolkit.AgentCommand(cfg, name)
	if err != nil {
		return exitCannot, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return exitCannot, err
	}
	dir, err := toolkit.GrantedGuestDir(cfg, cwd)
	if err != nil {
		return exitCannot, err
	}
	if toolkit.AgentInteractive(passed) {
		instance := ""
		if toolkit.SelectedInstance.Name != toolkit.DefaultInstance {
			instance = "--instance " + toolkit.SelectedInstance.Name + " "
		}
		return exitCannot, fmt.Errorf("%s's own screen needs a terminal, so it runs in a herdr pane in %s and not here. This prints the lines that reach the pane:\n"+
			"  wsl-toolkit %sbase attach\n"+
			"Start it there at %s. A command with no screen runs from here, for example: %s exec --json PROMPT",
			name, cfg.Base.Name, instance, dir, name)
	}
	if c, err := useHelper(ctx, false); err != nil {
		return exitCannot, err
	} else if c != nil {
		return exitCannot, errors.New("base agent runs a command in the base, which the restricted helper protocol does not accept. Make this call through the session's WSL approval path")
	}
	w, err := toolkit.FindWsl()
	if err != nil {
		return exitCannot, err
	}
	registered, err := w.Exists(ctx, cfg.Base.Name)
	if err != nil {
		return exitCannot, err
	}
	if !registered {
		return exitCannot, fmt.Errorf("%s is not registered. Build it with: wsl-toolkit base ensure", cfg.Base.Name)
	}
	return baseExecResult(w.Exec(ctx, toolkit.ExecRequest{
		Distro:  cfg.Base.Name,
		User:    cfg.Base.User,
		Script:  toolkit.AgentScript(command, passed),
		Dir:     dir,
		Payload: true,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}))
}

// runLauncher is this executable started under an agent's launcher name: the agent's
// command, for the instance the name carries, with every argument the agent's.
func runLauncher(ctx context.Context, agent, instance string, args []string) int {
	quiet = true
	if code, err := applyInstance(ctx, instance); err != nil {
		fmt.Fprintln(os.Stderr, agent+": "+err.Error())
		return code
	}
	code, err := cmdBaseAgent(ctx, append([]string{agent, "--"}, args...))
	if err != nil {
		fmt.Fprintln(os.Stderr, agent+": "+err.Error())
		if code == exitOK {
			code = exitCannot
		}
	}
	return code
}
