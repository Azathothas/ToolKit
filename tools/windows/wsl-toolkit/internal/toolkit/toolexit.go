// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
)

// ExitTool is what a command that answers with somebody else's exit code
// answers where THIS TOOL could not run the work: `run`, `wait`, `base exec`
// and the rest that forwards a job's own code. WSL-127.
//
// ⛔ IT WAS 2, AND 2 IS ALSO A JOB'S CODE. A refusal, a missing base and an
// image that could not be pulled answered exactly what `make` and a shell's
// misuse answer, so a caller without --json could not tell "it did not run"
// from "it ran and said 2". agent-box, podbox and dash.vpn each wrote their own
// way around it.
//
// ⭐ 125, AS coreutils' timeout AND podman run ANSWER THEIR OWN FAILURE. This
// tool already answers 124 for a deadline as timeout does, so the pair keeps
// one convention. A payload may still exit 125 itself, which is why the code
// can be moved: --tool-exit, or ToolExitEnv.
//
// ⚠ A COMMAND THAT NEVER FORWARDS A CODE KEEPS 2. Its refusal cannot be
// mistaken for anybody else's, and moving it would break every caller that
// reads `config validate` or `gc --job` for a gain of nothing.
const ExitTool = 125

// ToolExitEnv chooses ExitTool's value for every command of a process tree, as
// the global --tool-exit does for one call. WSL-127.
const ToolExitEnv = "WSL_TOOLKIT_TOOL_EXIT"

// toolExit is the code in force. ⚠ Atomic, because a fleet's rows seal their
// results on goroutines of their own while the command layer may set it.
var toolExit atomic.Int64

func init() { toolExit.Store(ExitTool) }

// ToolExit is the code a forwarding command answers where this tool failed.
func ToolExit() int { return int(toolExit.Load()) }

// SetToolExit puts a code ParseToolExit accepted in force.
func SetToolExit(code int) { toolExit.Store(int64(code)) }

// ParseToolExit reads a code for this tool's own failure. WSL-127.
//
// ⛔ A CODE THIS TOOL ALREADY ANSWERS WITH ANOTHER MEANING IS REFUSED. 0 is
// success, 1 is a job that ran and disagreed, 124 is a deadline and 130 a stop,
// so each of them would put a second meaning back on one number, which is the
// defect this exists to remove.
//
// ⭐ 2 IS ACCEPTED, and it is the way back to what 7.x answered for a caller
// that is not ready to move.
func ParseToolExit(v string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("%q is not a number. Pass a code from 2 to 255", v)
	}
	switch {
	case n < 2 || n > 255:
		return 0, fmt.Errorf("%d is not a code a process can answer as a failure. Pass a code from 2 to 255", n)
	case n == ExitTimeout:
		return 0, fmt.Errorf("%d already answers a deadline. Pass a code from 2 to 255 other than 124 and 130", n)
	case n == ExitStopped:
		return 0, fmt.Errorf("%d already answers a stop. Pass a code from 2 to 255 other than 124 and 130", n)
	}
	return n, nil
}

// ExitStopped is what a job, a session or a command answers when it was stopped
// or cancelled, as a shell answers SIGINT.
const ExitStopped = 130
