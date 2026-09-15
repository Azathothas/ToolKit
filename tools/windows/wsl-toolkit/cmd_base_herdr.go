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

const baseHerdrUsage = `wsl-toolkit base herdr [--] ARGS...

  Run one herdr command inside the base and return its answer. Every argument is
  herdr's and reaches it unchanged; its exit code is the answer.

  This is the route for an agent on Windows that cannot run herdr itself: a
  sandboxed one, or one that would otherwise have to quote a herdr command inside
  a shell string. An agent that CAN run herdr on Windows should use herdr's own
  --machine prefix instead, which talks to the base over SSH.

  Examples:
    wsl-toolkit base herdr -- agent list
    wsl-toolkit base herdr -- pane read w1:p1 --source recent --lines 40
    wsl-toolkit base herdr -- agent prompt w1:p1 "review the current diff" --wait --timeout 120000
`

// cmdBaseHerdr runs one herdr command in the base and forwards its answer.
//
// ⛔ IT EXISTS BECAUSE THE ALTERNATIVE IS A QUOTED SHELL STRING. Reaching herdr
// through `base exec -c 'herdr agent prompt w1:p1 "..."'` means a caller composes a
// command line that this tool then hands to a shell, and a prompt is prose: it
// carries quotes, dollar signs and backticks. Measured on 2026-09-09 against a real
// distribution, a payload's backtick was EXECUTED and the command still reported
// exit 0. Here every argument arrives as an argument and is quoted once, by
// AgentScript, which is the same path `base agent` uses.
//
// ⛔ NO FLAG OF THIS TOOL IS READ OUT OF THEM. `--json`, `--timeout` and `--source`
// are all herdr's own flags and all of them would otherwise be ambiguous.
//
// ⚠ IT IS A CHANNEL, NOT A WRAPPER. It knows no herdr subcommand, adds no default
// and parses no answer, so a herdr release that adds, renames or retires a command
// reaches the caller unchanged and nothing here has to be taught about it.
func cmdBaseHerdr(ctx context.Context, args []string) (int, error) {
	// ⚠ REGISTERED AND NEVER PARSED, as `base agent` is: the manual generates its
	// entry from the registered flag set, and this form has no flag of its own.
	_ = newFlagSet("base herdr")
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(os.Stderr, baseHerdrUsage)
		return exitOK, flag.ErrHelp
	}
	passed := args
	if len(passed) > 0 && passed[0] == "--" {
		passed = passed[1:]
	}
	if len(passed) == 0 {
		fmt.Fprint(os.Stderr, baseHerdrUsage)
		return exitCannot, errors.New("base herdr takes herdr's own arguments: base herdr -- agent list")
	}
	// ⛔ A BARE `herdr` LAUNCHES OR ATTACHES ITS TERMINAL UI, which herdr's own agent
	// skill says in those words, and there is no terminal on this path. Naming the
	// route beats a command that hangs with nothing to draw on.
	if herdrWantsATerminal(passed) {
		return exitCannot, fmt.Errorf("herdr %s wants a terminal, and this channel has none. Attach instead:\n"+
			"  wsl-toolkit base attach\n"+
			"Then run it in the pane. Every other herdr command works here, for example: base herdr -- agent list",
			strings.Join(passed, " "))
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if !toolkit.ConfigNamesAdapter(cfg, "herdr") {
		return exitCannot, fmt.Errorf("%s does not name the herdr adapter, so it has no herdr to run. Add {\"name\": \"herdr\"} to base.adapters and run: wsl-toolkit base ensure", cfg.Base.Name)
	}
	if c, err := useHelper(ctx, false); err != nil {
		return exitCannot, err
	} else if c != nil {
		return exitCannot, errors.New("base herdr runs a command in the base, which the restricted helper protocol does not accept. Make this call through the session's WSL approval path")
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
		Script:  toolkit.AgentScript("herdr", passed),
		Payload: true,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}))
}

// herdrWantsATerminal says whether a herdr invocation would draw a full-screen UI.
//
// ⛔ THE EMPTY CASE IS THE ONE THAT MATTERS, and it is not hypothetical: herdr's own
// agent skill instructs a reader not to run bare `herdr` for discovery because it
// launches or attaches the terminal UI. `attach` is the other, for both agents and
// terminals.
func herdrWantsATerminal(args []string) bool {
	if len(args) == 0 {
		return true
	}
	switch args[0] {
	case "attach":
		return true
	case "agent", "terminal":
		return len(args) > 1 && args[1] == "attach"
	}
	return false
}
