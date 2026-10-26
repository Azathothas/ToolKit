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

const baseAgentUsage = `wsl-toolkit base agent [--env NAME=VALUE]... NAME [--] ARGS...

  Run an agent adapter's command in the base, in the guest directory this Windows
  directory is granted at. Every argument after NAME is the agent's, and reaches it
  unchanged; its exit code is the answer. A directory no grant covers is refused
  with the command that grants it. The agent's own screen runs in a herdr pane.

  --env NAME=VALUE, before NAME, is exported to the agent this command starts.
  Repeatable. An agent herdr starts in a pane inherits herdr's environment, which
  this does not reach.

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
	// ⚠ ITS FLAGS COME BEFORE THE AGENT'S NAME, AND NONE AFTER IT. The flag
	// package stops at the first word that is not a flag, which is the name, so
	// an argument after it that spells --env reaches the agent as written. WSL-146.
	name, passed, pairs, err := parseAgentArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(os.Stderr, baseAgentUsage)
		return exitOK, flag.ErrHelp
	}
	if err != nil {
		return exitCannot, err
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
			instance = toolkit.InstanceArgs(toolkit.SelectedInstance.Name) + " "
		}
		// ⛔ AND --env DOES NOT GO WITH IT. A pane inherits the herdr server's
		// environment, so the refusal says the variables stay behind. WSL-146.
		envNote := ""
		if len(pairs) > 0 {
			envNote = ". --env reaches only a command this starts, and an agent in a herdr pane inherits herdr's environment"
		}
		return exitCannot, fmt.Errorf("%s's own screen needs a terminal, so it runs in a herdr pane in %s and not here. This prints the lines that reach the pane:\n"+
			"  wsl-toolkit %sbase attach\n"+
			"Start it there at %s. A command with no screen runs from here, for example: %s exec --json PROMPT%s",
			name, cfg.Base.Name, instance, dir, name, envNote)
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
		return exitCannot, errNotRegistered(cfg.Base.Name)
	}
	return baseExecResult(w.Exec(ctx, toolkit.ExecRequest{
		Distro:  cfg.Base.Name,
		User:    cfg.Base.User,
		Script:  agentPayload(command, passed, pairs),
		Dir:     dir,
		Payload: true,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}))
}

// parseAgentArgs reads `base agent`'s own flags, the agent's name and the agent's
// arguments, in that order. WSL-146.
//
// ⚠ ITS FLAGS COME BEFORE THE AGENT'S NAME, AND NONE AFTER IT. The flag package
// stops at the first word that is not a flag, which is the name, so an argument
// after it that spells --env reaches the agent as written.
func parseAgentArgs(args []string) (name string, passed []string, pairs []toolkit.EnvPair, err error) {
	fs := newFlagSet("base agent")
	var env stringList
	fs.Var(&env, "env", "NAME=VALUE exported to the agent this command starts, written before the agent's name. Repeatable. An agent herdr starts in a pane does not get it")
	if err := fs.Parse(args); err != nil {
		return "", nil, nil, err
	}
	rest := fs.Args()
	if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
		fmt.Fprint(os.Stderr, baseAgentUsage)
		return "", nil, nil, errors.New("base agent takes an agent's name, then the agent's own arguments: base agent muse -- exec --json PROMPT")
	}
	name, passed = rest[0], rest[1:]
	if len(passed) > 0 && passed[0] == "--" {
		passed = passed[1:]
	}
	if err := gitBashRewriteAny("an argument to "+name, passed, guestText); err != nil {
		return "", nil, nil, err
	}
	pairs, err = agentEnv(env)
	return name, passed, pairs, err
}

// agentPayload is the command `base agent` sends: the agent's exec, with every
// --env assigned in front of it. WSL-146.
//
// ⭐ THE ONE WRITER `base exec --env` USES, so the agent inherits each variable
// and no value is read as shell.
func agentPayload(command string, passed []string, pairs []toolkit.EnvPair) []byte {
	return toolkit.ComposePayload(toolkit.AgentScript(command, passed), pairs, false)
}

// agentEnv reads every --env of `base agent`, in the order given, so a later
// value for a name wins. WSL-146.
//
// ⛔ AN AGENT herdr STARTS IS NOT REACHED BY THIS. Its pane inherits the herdr
// server's environment, which reads no profile, so a variable for such an agent
// belongs in the agent's own configuration. Finding 51 measured it.
func agentEnv(raw []string) ([]toolkit.EnvPair, error) {
	var pairs []toolkit.EnvPair
	for _, v := range raw {
		if err := gitBashRewrite("--env", v, guestText); err != nil {
			return nil, err
		}
		p, err := toolkit.ParseEnvPair(v)
		if err != nil {
			return nil, fmt.Errorf("--env %w", err)
		}
		pairs = append(pairs, p)
	}
	return pairs, nil
}

// runLauncher is this executable started under an agent's launcher name: the agent's
// command, for the instance the name carries, with every argument the agent's.
//
// ⭐ IT ANSWERS THE AGENT'S CODE, so its own failure answers toolkit.ToolExit,
// as `base agent` does. WSL-127.
func runLauncher(ctx context.Context, agent, instance string, args []string) int {
	quiet = true
	if err := readToolExit(false); err != nil {
		fmt.Fprintln(os.Stderr, agent+": "+err.Error())
		return ownExit(true, exitCannot, err)
	}
	// ⛔ THE LAUNCHER SETTLES THE STATE DIRECTORY TOO. main sends a launcher
	// name here before run, so `muse.exe` from Git Bash with
	// WSL_TOOLKIT_HOME=/d/x read `C:\d\x`, the second root WSL-187 removes, and
	// handed it on to every child. WSL-187.
	if err := settleHome(); err != nil {
		fmt.Fprintln(os.Stderr, agent+": "+err.Error())
		return ownExit(true, exitCannot, err)
	}
	// ⛔ A NAME WITH NO SUFFIX SELECTS THE DEFAULT BASE, so neither the environment
	// nor a working tree's pointer moves it to another instance. WSL-167.
	if instance == toolkit.DefaultInstance {
		os.Unsetenv(toolkit.InstanceEnv)
	} else if code, err := applyInstance(ctx, instance); err != nil {
		fmt.Fprintln(os.Stderr, agent+": "+err.Error())
		return ownExit(true, code, err)
	}
	code, err := cmdBaseAgent(ctx, append([]string{agent, "--"}, args...))
	if err != nil {
		fmt.Fprintln(os.Stderr, agent+": "+err.Error())
		if code == exitOK {
			code = exitCannot
		}
	}
	return ownExit(true, code, err)
}
