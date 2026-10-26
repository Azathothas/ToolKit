// SPDX-License-Identifier: 0BSD

package main

import (
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

// RefusalSchema versions the answer a refused `run --json` or `base exec
// --json` writes, where no answer of the command's own was written. WSL-127.
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
		return sub == "exec" || sub == "shell" || sub == "agent"
	case "distro":
		switch sub {
		case "run", "enter":
			return true
		case "new":
			return hasFlag(args[1:], "c") || hasFlag(args[1:], "command-base64") || hasFlag(args[1:], "script")
		}
	case "bsd":
		return sub == "run"
	}
	return false
}

// refusesWithDocument says whether a refusal of this invocation is written as
// a RefusalSchema answer: `run` and `base exec`, under --json. WSL-127.
//
// ⚠ THE TWO THAT START THE WORK A CALLER BRANCHES ON. `wait` starts nothing,
// so a document saying no job started would describe the wait rather than
// the job; its 125 says this tool failed.
func refusesWithDocument(cmd string, args []string) bool {
	switch {
	case cmd == "run":
	case cmd == "base" && len(args) > 0 && args[0] == "exec":
		args = args[1:]
	default:
		return false
	}
	return hasFlag(args, "json")
}

// hasFlag says whether args set the named flag, in any spelling Go's flag
// package reads: -name, --name, -name=V and --name=V. A boolean written
// =false is not set. The scan stops at --.
func hasFlag(args []string, name string) bool {
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
		if !hasValue {
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

// writeRefusal answers a refused `run --json` or `base exec --json` with a
// document, where the command wrote none. WSL-127.
func writeRefusal(cmd string, code int, err error) {
	if answered {
		return
	}
	if werr := writeJSON(refusal{Schema: RefusalSchema, Command: cmd, EffectiveExit: code, Error: err.Error()}); werr != nil {
		fmt.Fprintln(os.Stderr, "wsl-toolkit: ! could not write the refusal as a document: "+werr.Error())
	}
}

// readToolExit puts the code WSL_TOOLKIT_TOOL_EXIT names in force, or the
// default where it names none.
//
// ⛔ A VALUE THAT IS NOT A CODE IS A REFUSAL, unlike WSL_TOOLKIT_JSON's. A
// caller who set it branches on the number, and a number quietly replaced by
// the default would send that branch the wrong way.
func readToolExit() error {
	toolkit.SetToolExit(toolkit.ExitTool)
	v := os.Getenv(toolkit.ToolExitEnv)
	if strings.TrimSpace(v) == "" {
		return nil
	}
	code, err := toolkit.ParseToolExit(v)
	if err != nil {
		return fmt.Errorf("%s=%s: %w", toolkit.ToolExitEnv, v, err)
	}
	toolkit.SetToolExit(code)
	return nil
}
