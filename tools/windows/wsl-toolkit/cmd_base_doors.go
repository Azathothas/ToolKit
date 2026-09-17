// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

const baseDoorsUsage = `wsl-toolkit base doors [--json]

  Attack this base's doors from the inside, as its unprivileged account, and
  report what got through. Every row is an attempt that was made, never a
  setting read back.

  Exit 0 means every door this base's CONFIGURATION claims to close was in fact
  closed. ⛔ It does not mean the base reaches nothing: read the open doors the
  report lists beneath, which no setting here claims to shut.
`

// cmdBaseDoors is the command WSL-68 approach step 3 asks for: the attack that
// lived in a scratch script, as something with a name, a schema and tests.
//
// ⛔ IT IS A REPORT AND NOT A BOUNDARY. Nothing in this tree has measured a WSL
// distribution to be a security boundary, and this command exists so that the
// claim is made of measurements rather than of settings. The manual's safety
// model says what is NOT sealed, with these same doors beside it.
func cmdBaseDoors(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("base doors")
	asJSON := fs.Bool("json", false, "write a structured answer")
	viaHelper := fs.Bool("via-helper", false, "prove the helper refusal for the doors probe")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if c, err := useHelper(ctx, *viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		return exitCannot, fmt.Errorf("base doors runs a probe that mounts, writes and opens sockets inside the base, which the restricted helper protocol does not accept. Make this call through the session's WSL approval path")
	}
	w, err := toolkit.FindWsl()
	if err != nil {
		return exitCannot, err
	}
	exists, err := w.Exists(ctx, cfg.Base.Name)
	if err != nil {
		return exitCannot, err
	}
	if !exists {
		return exitCannot, fmt.Errorf("%s is not registered. Build it with: wsl-toolkit base ensure", cfg.Base.Name)
	}
	base, err := toolkit.NewBase(cfg, note)
	if err != nil {
		return exitCannot, err
	}
	rep, err := base.Doors(ctx)
	if err != nil {
		return exitCannot, err
	}
	if *asJSON {
		return doorsVerdict(rep), writeJSON(rep)
	}
	renderDoors(rep)
	return doorsVerdict(rep), nil
}

// doorsVerdict is 1 when a door the configuration claims to close was not
// closed, and 0 otherwise. ⛔ An open door nothing claims to close does NOT make
// it 1: the tool would then be refusing a base it built correctly, and a caller
// would learn to ignore the exit code.
func doorsVerdict(rep toolkit.DoorsReport) int {
	if rep.Sealed() {
		return exitOK
	}
	return exitFailed
}

func renderDoors(rep toolkit.DoorsReport) {
	out := os.Stderr
	fmt.Fprintf(out, "  distro      %s\n", rep.Distro)
	fmt.Fprintf(out, "  account     %s\n", rep.Account)
	if rep.HostAddress != "" {
		fmt.Fprintf(out, "  host        %s\n", rep.HostAddress)
	} else {
		fmt.Fprintf(out, "  host        not resolved, so the Windows host doors were not tried\n")
	}
	width := 0
	for _, d := range rep.Doors {
		if len(d.ID) > width {
			width = len(d.ID)
		}
	}
	for _, d := range rep.Doors {
		claim := ""
		if d.Claimed {
			claim = "  <- " + d.ClaimedBy
		}
		fmt.Fprintf(out, "  %-*s  %-8s  %s%s\n", width, d.ID, d.Verdict, d.Detail, claim)
	}
	for _, p := range rep.Problems {
		fmt.Fprintf(out, "  ! %s\n", p)
	}
	// ⭐ LAST, AND IT IS THE SENTENCE THE READER NEEDS. A base whose every claim
	// holds still reaches whatever this line lists, and a report that ended at
	// "no problems" is how a containment claim gets made by accident.
	//
	// ⚠ A CLAIMED DOOR THAT IS OPEN IS NOT REPEATED HERE. It is already a problem
	// above, and listing it under "no setting closes them" would be false.
	if unclaimed := unclaimedOpen(rep); len(unclaimed) > 0 {
		fmt.Fprintf(out, "\n  ⚠ OPEN, AND NO SETTING HERE CLOSES THEM: %s\n", strings.Join(unclaimed, ", "))
		fmt.Fprintf(out, "    A base that keeps its own promises is not a base that reaches nothing.\n")
		fmt.Fprintf(out, "    read   wsl-toolkit man, the safety model\n")
	}
}

// unclaimedOpen is every door that got through and that no setting in this
// tool's configuration promises to shut.
func unclaimedOpen(rep toolkit.DoorsReport) []string {
	claimed := map[string]bool{}
	for _, d := range rep.Doors {
		if d.Claimed {
			claimed[d.ID] = true
		}
	}
	var out []string
	for _, id := range rep.Open {
		if !claimed[id] {
			out = append(out, id)
		}
	}
	return out
}
