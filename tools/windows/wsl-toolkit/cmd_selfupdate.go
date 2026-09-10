package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

const selfupdateUsage = `wsl-toolkit selfupdate [--check] [--tag TAG] [--json]

  Move this executable to a published release, verifying it first.

  --check   report whether a newer release exists and change nothing
  --tag     a specific wsl-toolkit-vX.Y.Z, so a caller can move DOWN as well
  --json    a structured answer

  The release assets are public, so nothing here reads a credential. A digest
  that does not match SHA256SUMS is a refusal and the running executable is
  left untouched.
`

func cmdSelfUpdate(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("selfupdate")
	asJSON := fs.Bool("json", false, "write a structured answer")
	check := fs.Bool("check", false, "report whether a newer release exists and change nothing")
	tag := fs.String("tag", "", "a specific release tag, so a caller can move down as well as up")
	if err := parseArgs(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, selfupdateUsage)
			return exitOK, nil
		}
		return exitCannot, err
	}
	running := versionString()

	// ⛔ SWEPT ON EVERY CALL, including --check. A Windows executable cannot
	// delete itself while it is running, so an update leaves the old copy for
	// the NEXT run to collect - and a caller that only ever asks whether an
	// update exists is a next run too.
	toolkit.SweepPreviousExecutables(note)

	if *check {
		// ⛔ --check CHANGES NOTHING AND SAYS SO IN THE ANSWER. A caller that
		// wired this into a pipeline must be able to tell a check from an
		// update by reading the object rather than by remembering the flag.
		st := toolkit.CheckUpdate(ctx, running)
		if *asJSON {
			return checkVerdict(st), writeJSON(toolkit.UpdateResult{
				Schema: "wsl-toolkit-selfupdate/1", Running: st.Running, Latest: st.Latest,
				CheckedOnly: true, Reason: st.Reason,
			})
		}
		switch {
		case !st.Checked:
			logf("  could not ask whether a newer release exists: %s", st.Reason)
		case st.Available:
			logf("  %s is published and this is %s. Run: %s", st.Latest, st.Running, st.Command)
		default:
			logf("  %s is the newest published release", st.Running)
		}
		return checkVerdict(st), nil
	}

	if *tag != "" {
		// ⚠ SAID BEFORE IT HAPPENS, because it cannot be undone by this tool.
		// A release older than the one that introduced `selfupdate` does not
		// carry the command, so moving back to one is a one-way trip: measured
		// on 2026-09-10 by moving a copy to v1.2.0, which answers
		// `"selfupdate" is not a command`. Naming the tag is the caller's
		// choice and this is the fact they need before making it.
		note("a release older than this one may not carry selfupdate; moving back to one means downloading the next by hand")
	}
	res, err := toolkit.SelfUpdate(ctx, running, *tag, note)
	if err != nil {
		return exitCannot, err
	}
	if *asJSON {
		return exitOK, writeJSON(res)
	}
	if res.Replaced {
		logf("  updated to %s. Verify with: wsl-toolkit version", res.Latest)
	} else {
		logf("  nothing to do: %s", res.Reason)
	}
	return exitOK, nil
}

// checkVerdict is 0 when this executable is current, 1 when a newer release
// exists, and 2 when the question could not be asked.
//
// ⛔ A NETWORK THAT COULD NOT BE REACHED IS NOT "UP TO DATE". Answering 0 there
// would make a pipeline that gates on this silently stop updating the day
// GitHub was unreachable, which is a check reporting success over a failure.
func checkVerdict(st toolkit.UpdateStatus) int {
	switch {
	case !st.Checked:
		return exitCannot
	case st.Available:
		return exitFailed
	default:
		return exitOK
	}
}
