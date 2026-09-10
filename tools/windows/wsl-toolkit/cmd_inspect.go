package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// ⭐ WHY THIS COMMAND EXISTS. When a job fails, what a caller has is an exit
// code and a transcript. Everything else that would explain it - which engine,
// on what storage, under which cgroup version, whether the guest had any disk
// left, and what the container's own last state was - is a sequence somebody
// has to type into the guest by hand. WSL-56.
//
// ⛔ IT IS NOT `resources`. That one enumerates what this tool is holding; this
// one describes one job and the machine it ran on. A version of this that
// starts listing has become a copy of the other.

const inspectUsage = `wsl-toolkit inspect [JOB-ID]

  What one job was, and what the machine was doing when it ran.

  With no id, the machine half alone: the engine, its storage and the
  filesystem under it.

  --since D   how far back to read the engine's event journal (default 24h)
  --json      write a structured answer

  An id nothing on this machine has heard of is a refusal, not an empty
  answer. wsl-toolkit logs lists the jobs this machine still has.
`

func cmdInspect(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("inspect")
	since := fs.Duration("since", 24*time.Hour, "how far back to read the engine's event journal")
	asJSON := fs.Bool("json", false, "write a structured answer")
	id, rest := splitLogsArgs(args)
	if err := parseArgs(fs, rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, inspectUsage)
			return exitOK, nil
		}
		return exitCannot, err
	}
	if *since <= 0 {
		return exitCannot, fmt.Errorf("--since %s would read no journal at all. Pass a positive duration", *since)
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	// ⛔ A RUNNER IS NOT REQUIRED TO ANSWER THE HOST HALF. Building one calls
	// FindWsl, so on a machine with no wsl.exe this command used to exit 2 with
	// no answer at all - including for the transcript and the ledger record,
	// which are on this machine's own disk and which `logs` reads with no
	// runner whatever. Found by the door sweep. What cannot be reached is named
	// as unreachable and the rest is still reported.
	var rep toolkit.InspectReport
	runner, rErr := toolkit.NewRunner(cfg, note)
	if rErr != nil {
		rep, err = toolkit.InspectHostOnly(id, rErr.Error())
	} else {
		rep, err = runner.Inspect(ctx, id, *since)
	}
	if err != nil {
		// ⛔ AN UNKNOWN ID IS A REFUSAL WITH ITS OWN CODE. Reporting it as
		// exitCannot beside a printed empty document is the shape this command
		// was written against.
		if errors.Is(err, toolkit.ErrUnknownJob) {
			return exitFailed, err
		}
		return exitCannot, err
	}
	if *asJSON {
		return exitOK, writeJSON(rep)
	}
	return exitOK, renderInspect(os.Stdout, rep)
}

// renderInspect writes the human-readable report.
//
// ⚠ IT SAYS WHAT IT COULD NOT READ. A row that is simply absent from a report
// reads as a fact about the machine rather than as a gap in the reading, and
// this whole command exists to be believed when a job has already failed.
func renderInspect(w *os.File, rep toolkit.InspectReport) error {
	p := func(format string, args ...any) error {
		_, err := fmt.Fprintf(w, format, args...)
		return err
	}
	if j := rep.Job; j != nil {
		if err := p("==> Job %s\n", j.ID); err != nil {
			return err
		}
		if err := p("    container   %s\n", j.Container); err != nil {
			return err
		}
		if j.Image != "" {
			if err := p("    image       %s\n", j.Image); err != nil {
				return err
			}
		}
		switch {
		case j.ExitCode != nil:
			if err := p("    last exit   %d, from the engine's own journal\n", *j.ExitCode); err != nil {
				return err
			}
		default:
			if err := p("    last exit   the journal does not reach back to this container\n"); err != nil {
				return err
			}
		}
		if j.StillHere != "" {
			if err := p("    ⚠ still here  %s\n", j.StillHere); err != nil {
				return err
			}
		}
		if !j.Deadline.IsZero() {
			if err := p("    deadline    %s\n", j.Deadline.UTC().Format(time.RFC3339)); err != nil {
				return err
			}
		}
		if j.Record != nil && !j.Closed {
			if err := p("    ⚠ open record, so this job was never closed out\n"); err != nil {
				return err
			}
		}
		if j.Transcript != "" {
			if err := p("    transcript  %s (%s)\n", j.Transcript, toolkit.HumanBytes(j.TranscriptBytes)); err != nil {
				return err
			}
		}
		for _, d := range j.GuestDirs {
			size := toolkit.HumanBytes(d.Bytes)
			if d.Bytes < 0 {
				size = "size unreadable"
			}
			if err := p("    in the guest %s (%s)\n", d.Path, size); err != nil {
				return err
			}
		}
		if len(j.Events) == 0 {
			if err := p("    events      none in the window read\n"); err != nil {
				return err
			}
		}
		for _, e := range j.Events {
			line := fmt.Sprintf("    %-11s %s", e.Status, e.At.Format(time.RFC3339Nano))
			if e.ExitCode != nil {
				line += fmt.Sprintf("  exit %d", *e.ExitCode)
			}
			if err := p("%s\n", line); err != nil {
				return err
			}
		}
		if err := p("    known from  %s\n", strings.Join(j.Known, ", ")); err != nil {
			return err
		}
	}

	if err := p("==> The engine jobs run in\n"); err != nil {
		return err
	}
	if !rep.Engine.Reached {
		if err := p("    not reached: %s\n", rep.Engine.Reason); err != nil {
			return err
		}
	} else {
		rows := [][2]string{
			{"podman", rep.Engine.Version},
			{"runtime", rep.Engine.Runtime},
			{"storage", rep.Engine.StorageDriver + " on " + rep.Engine.StorageBacked},
			{"storage at", rep.Engine.StorageRoot},
			{"cgroups", rep.Engine.CgroupVersion + ", " + rep.Engine.CgroupManager},
			{"rootless", rep.Engine.Rootless},
			{"events", rep.Engine.EventLogger},
			{"logs", rep.Engine.LogDriver},
		}
		for _, r := range rows {
			if err := p("    %-11s %s\n", r[0], r[1]); err != nil {
				return err
			}
		}
	}
	if err := p("==> The distribution it runs in\n"); err != nil {
		return err
	}
	if err := p("    distro      %s\n", rep.Guest.Distro); err != nil {
		return err
	}
	if d := rep.Guest.Disk; d != nil {
		if err := p("    disk        %s of %s used (%d%%) on %s, %s free\n",
			toolkit.HumanBytes(d.UsedByte), toolkit.HumanBytes(d.TotalByte), d.UsedPct,
			d.Mount, toolkit.HumanBytes(d.FreeByte)); err != nil {
			return err
		}
	} else {
		if err := p("    disk        could not be read\n"); err != nil {
			return err
		}
	}
	for _, warn := range rep.Warnings {
		if err := p("    ! %s\n", warn); err != nil {
			return err
		}
	}
	return nil
}
