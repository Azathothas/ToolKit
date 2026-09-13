// SPDX-License-Identifier: 0BSD

// Package compat is the PowerShell compatibility interface, implemented
// natively in this executable.
//
// The PowerShell product at scripts/windows/wsl-toolkit remains a separate
// artefact for the callers who run it directly; this executable no longer
// carries or launches a copy of it. Everything the script's command line
// accepted is accepted here, with the same refusals, the same streams and the
// same exit codes, so a caller who drove one drives the other:
//
//	New, Run, Enter, List, Remove, Purge, Resources, HostAddress,
//	Doctor, Snapshot, Replay, Compare
//
// EXIT CODES -- New and Run behave the SAME way, because they drifted apart
// once and nothing noticed:
//
//	-Action Run -Command ...             exits with the inner command's code
//	-Action New -Command ...             exits with the inner command's code
//	-Action New -Command ... -Ephemeral  tears the distro down FIRST, then
//	                                     exits with the inner command's code
//	New with no -Command                 exits 0 when the distro came up
//	-CommandTimeoutSeconds N reached     exits 124, as coreutils' timeout does
//	any action, tool failure             exits 1, with a message naming what
//	-Action List, WSL refused to answer  exits 2, never an empty inventory
//
// THE STREAM LOG -- on by default, and -NoTimestamps turns all of it off. The
// failure it exists for: a command prints nothing for twenty minutes and a
// caller reading a pipe cannot tell that from a command that has died. Silence
// has to be a line, or it says nothing at all.
package compat

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// Run executes one invocation of the interface and returns the exit code the
// process should exit with. The report stream is where action reports and
// captured values go; notes go to the note stream, which for a process is
// stderr.
//
// ⛔ ONE ERROR LINE, on a refusal: "ERROR: what happened", on the note stream,
// and exit 1. Every refusal reads that way, so a caller scripts against one
// shape.
func Run(ctx context.Context, args []string, report, notes io.Writer, stdin io.Reader) int {
	return run(ctx, args, report, notes, stdin)
}

func run(ctx context.Context, args []string, report, notes io.Writer, stdin io.Reader) int {
	o, err := Parse(args)
	if err != nil {
		fmt.Fprintf(notes, "ERROR: %s\n", err.Error())
		return 1
	}

	// ⛔ THE STATE DIRECTORY IS RESOLVED BEFORE ANYTHING ELSE, and a caller
	// with neither a -StateDir nor LOCALAPPDATA is refused here rather than
	// somewhere inside an action that already created state.
	baseDir := ""
	if strings.TrimSpace(o.StateDir) != "" {
		baseDir = fullPathOr(o.StateDir)
	} else if appData := envValue("LOCALAPPDATA"); appData != "" {
		baseDir = filepath.Join(appData, "wsl-ephemeral")
	}
	if strings.TrimSpace(baseDir) == "" {
		fmt.Fprintln(notes, "ERROR: Pass -StateDir or set LOCALAPPDATA to choose a state directory.")
		return 1
	}

	if err := assertParametersApplyToAction(&o); err != nil {
		fmt.Fprintf(notes, "ERROR: %s\n", err.Error())
		return 1
	}

	s := newSession(o, baseDir, report, notes, stdin, ctx)

	// ⭐ raw IS -NoTimestamps UNDER ANOTHER NAME, resolved here so that
	// exactly one variable decides whether the relay runs.
	s.relayOff = o.NoTimestamps || o.TimestampProfile == "raw"

	// ⛔ REFUSED, NEVER IGNORED. Every combination below is one where a
	// parameter the caller typed would have no effect at all.
	if s.relayOff {
		dead := explicitDeadParams(&o)
		if len(dead) > 0 {
			off := "-NoTimestamps"
			if !o.NoTimestamps {
				off = "-TimestampProfile raw"
			}
			fmt.Fprintf(notes, "ERROR: %s turns the stream log off, so -%s would do nothing. Drop %s to use them, or drop them.\n",
				off, strings.Join(dead, " and -"), off)
			return 1
		}
	} else {
		// ⛔ RESOLVED ONCE, HERE, AND EVERY SINK READS THE RESULT. A typo in a
		// format or an unusable log path is otherwise found by the first line
		// of output, which on -Action New is after a pull, an export and an
		// import.
		settings, err := resolveStreamLogSettings(o, writerIsTerminal(report))
		if err != nil {
			fmt.Fprintf(notes, "ERROR: %s\n", err.Error())
			return 1
		}
		s.settings = settings

		// ⛔ THE SINK PATHS ARE REFUSED HERE, not where they are opened. The
		// openers are reached only when a command actually runs, so -DryRun
		// returned before them and reported a plan the real run would have
		// refused.
		if err := assertSinkPathIsUsable(o.StreamLogPath, "-StreamLogPath"); err != nil {
			fmt.Fprintf(notes, "ERROR: %s\n", err.Error())
			return 1
		}
		if err := assertSinkPathIsUsable(o.EventLog, "-EventLog"); err != nil {
			fmt.Fprintf(notes, "ERROR: %s\n", err.Error())
			return 1
		}

		// Rendered once and thrown away, so a bad specifier is refused before
		// a distro exists to refuse it against.
		reading := wallNow()
		for _, c := range settings.Columns {
			if _, err := formatStampColumn(c, settings.Format, reading, 0, 0); err != nil {
				fmt.Fprintf(notes, "ERROR: %s\n", err.Error())
				return 1
			}
		}
	}

	// Resolved ONCE, here, so New and Run cannot disagree about which switch
	// won and a bad -CommandFile is refused before a distro is built for it.
	// The ScriptArg sources are read inside, in the one place, so a value
	// reaches a command written as text, as a file and as base64 identically.
	commandBytes, err := s.resolveCommandBytes()
	if err != nil {
		fmt.Fprintf(notes, "ERROR: %s\n", err.Error())
		return 1
	}
	if o.UserEnv && commandBytes != nil {
		commandBytes = addGuestUserEnvironment(commandBytes)
	}
	s.commandBytes = commandBytes

	return s.dispatch(ctx)
}

// explicitDeadParams names the relay-rendering parameters the caller passed
// that would do nothing under -NoTimestamps, in the order the script listed
// them.
func explicitDeadParams(o *Options) []string {
	names := []string{
		"TimestampMode", "TimestampFormat", "TimestampColumns", "TimestampSeparator",
		"PrefixOnly", "Color", "StreamLogPath", "StreamLogOverwrite", "EventLog",
		"Redact", "MaxLineBytes", "TickSeconds", "TickEscalateSeconds",
		"CommandTimeoutSeconds",
	}
	var dead []string
	for _, n := range names {
		if o.wasPassed(n) {
			dead = append(dead, n)
		}
	}
	return dead
}

// dispatch is the action switch of the script's main.
func (s *session) dispatch(ctx context.Context) int {
	switch s.opts.Action {
	case ActionNew:
		return s.actionNew(ctx)
	case ActionRun:
		return s.actionRun(ctx)
	case ActionEnter:
		return s.actionEnter(ctx)
	case ActionList:
		return s.actionList()
	// These three are read-only and none of them creates a distro.
	case ActionSnapshot:
		return s.actionSnapshot()
	case ActionReplay:
		return s.actionReplay()
	case ActionCompare:
		return s.actionCompare()
	case ActionResources:
		return s.actionResources()
	case ActionHostAddress:
		code, err := s.actionHostAddress()
		if err != nil {
			s.fail(err.Error())
			return 1
		}
		return code
	case ActionDoctor:
		return s.actionDoctor()
	case ActionRemove:
		return s.actionRemove()
	case ActionPurge:
		return s.actionPurge()
	default:
		s.fail(fmt.Sprintf("Action '%s' is not one of the twelve this interface answers.", s.opts.Action))
		return 1
	}
}
