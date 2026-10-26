// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// The tool's own failure, told apart from a job's exit 2. WSL-127.
//
// ⛔ A COMMAND THAT ANSWERS WITH SOMEBODY ELSE'S CODE answered 2 for its own
// refusal too, and 2 is what `make`, `grep` and a shell's misuse answer. Without
// --json a caller could not tell a job that ran and said 2 from one this tool
// never started. Those commands now answer toolkit.ToolExit, 125 by default,
// and every other command keeps 2.

// toolExitFlag is the global flag that moves the code for one call.
const toolExitFlag = "--tool-exit"

// RefusalSchema versions the answer a refused `run --json`, `base exec --json`,
// `distro run --json`, `distro new -c --json` or `bsd run --json` writes, where
// no answer of the command's own was written. WSL-127.
const RefusalSchema = "wsl-toolkit-refusal/1"

// refusal is that answer. ⭐ Its two fields a caller branches on carry the
// names they carry on a job's own result, so one reader takes either.
type refusal struct {
	Schema        string `json:"schema"`
	Command       string `json:"command"`
	JobStarted    bool   `json:"job_started"`
	EffectiveExit int    `json:"effective_exit"`
	Error         string `json:"error"`
}

// answered is true once this process wrote an answer to stdout, so a refusal
// is never a second document after the command's own.
var answered bool

// forwardsExit says whether an invocation answers with somebody else's exit
// code: a job's, a session's, a guest command's, a shell's or an agent's.
//
// ⚠ READ FROM THE WORDS, BEFORE THE COMMAND PARSES THEM, because a flag the
// command refuses is this tool's failure too and must answer the same code.
// `logs` forwards only with --follow, and `distro new` only with a command.
//
// ⛔ `base herdr` AND `base bootstrap` FORWARD TOO. Each answers herdr's or the
// bootstrap's own code, and herdr answers 2 itself, so a missing base that
// answered 2 there could not be told from herdr's own 2. WSL-127.
func forwardsExit(cmd string, args []string) bool {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch cmd {
	case "run", "wait":
		return true
	case "logs":
		return hasFlag(args, "follow")
	case "base":
		switch sub {
		case "exec", "shell", "agent", "herdr", "bootstrap":
			return true
		}
	case "distro":
		switch sub {
		case "run", "enter":
			return true
		case "new":
			return hasValueFlag(args[1:], "c") || hasValueFlag(args[1:], "command-base64") || hasValueFlag(args[1:], "script")
		}
	case "bsd":
		return sub == "run"
	}
	return false
}

// refusalDocument names the command a refusal of this invocation is written
// for as a RefusalSchema answer, under --json: `run`, `base exec`, `distro
// run`, `distro new` given a command, and `bsd run`. WSL-127.
//
// ⚠ THE ONES THAT START THE WORK A CALLER BRANCHES ON. `wait` and `logs
// --follow` start nothing, so a document saying no job started would describe
// the wait rather than the job; their 125 says this tool failed. `distro enter`
// structures only its --dry-run plan.
//
// ⭐ THE NAME CARRIES THE SUBCOMMAND, so a refused `base exec` is not read as
// some other `base` command.
func refusalDocument(cmd string, args []string) (string, bool) {
	name := cmd
	switch {
	case cmd == "run":
	case cmd == "base" && len(args) > 0 && args[0] == "exec",
		cmd == "distro" && len(args) > 0 && args[0] == "run",
		cmd == "distro" && len(args) > 0 && args[0] == "new" && forwardsExit(cmd, args),
		cmd == "bsd" && len(args) > 0 && args[0] == "run":
		name, args = cmd+" "+args[0], args[1:]
	default:
		return "", false
	}
	return name, hasFlag(args, "json")
}

// hasFlag says whether args set the named BOOLEAN flag, in any spelling Go's
// flag package reads: -name, --name, -name=V and --name=V. A boolean written
// =false is not set. The scan stops at --.
func hasFlag(args []string, name string) bool { return flagGiven(args, name, true) }

// hasValueFlag says whether args give the named flag that takes a value.
//
// ⛔ ANY =V SETS IT. `-c=false` runs the command `false`, and reading that value
// as a boolean called the flag unset, so a refusal of the call answered 2.
func hasValueFlag(args []string, name string) bool { return flagGiven(args, name, false) }

func flagGiven(args []string, name string, boolean bool) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if !strings.HasPrefix(a, "-") {
			continue
		}
		spelled := strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-")
		given, value, hasValue := strings.Cut(spelled, "=")
		if given != name {
			continue
		}
		if !hasValue || !boolean {
			return true
		}
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
		return true
	}
	return false
}

// ownExit is the code a refusal answers: ToolExit where the invocation
// forwards somebody else's code, and the command's own everywhere else.
//
// ⛔ ONLY A 2 THAT CAME WITH AN ERROR MOVES. A forwarded code always comes back
// with none, so a job that exited 2 keeps its 2, and a cancellation keeps 130.
func ownExit(forwards bool, code int, err error) int {
	if forwards && err != nil && code == exitCannot {
		return toolkit.ToolExit()
	}
	return code
}

// cannotOrStopped is the code a forwarding command answers for an error of its
// own: 130 where this call was cancelled, as the direct route answers one, and
// 2 otherwise, which ownExit moves. WSL-127.
func cannotOrStopped(ctx context.Context) int {
	if errors.Is(ctx.Err(), context.Canceled) {
		return toolkit.ExitStopped
	}
	return exitCannot
}

// writeRefusal answers a refused command under --json with a document, where
// the command wrote none. cmd is the name refusalDocument gave. WSL-127.
func writeRefusal(cmd string, code int, err error) {
	if answered {
		return
	}
	if werr := writeJSON(refusal{Schema: RefusalSchema, Command: cmd, EffectiveExit: code, Error: err.Error()}); werr != nil {
		fmt.Fprintln(os.Stderr, "wsl-toolkit: ! could not write the refusal as a document: "+werr.Error())
	}
}

// readToolExit puts the code WSL_TOOLKIT_TOOL_EXIT names in force, or the
// default where it names none. asked says --tool-exit was given on this call.
//
// ⛔ A VALUE THAT IS NOT A CODE IS A REFUSAL, unlike WSL_TOOLKIT_JSON's. A
// caller who set it branches on the number, and a number quietly replaced by
// the default would send that branch the wrong way.
//
// ⚠ AN EMPTY --tool-exit IS NOT A CODE EITHER. Only the variable reads empty as
// unset, because a shell leaves a variable empty and a caller who wrote the
// flag meant to pick a code.
func readToolExit(asked bool) error {
	toolkit.SetToolExit(toolkit.ExitTool)
	v := os.Getenv(toolkit.ToolExitEnv)
	if strings.TrimSpace(v) == "" {
		if asked {
			return errors.New(toolExitFlag + " was given no code. Pass a code from 2 to 255")
		}
		return nil
	}
	code, err := toolkit.ParseToolExit(v)
	if err != nil {
		return fmt.Errorf("%s=%s: %w", toolkit.ToolExitEnv, v, err)
	}
	toolkit.SetToolExit(code)
	return nil
}
