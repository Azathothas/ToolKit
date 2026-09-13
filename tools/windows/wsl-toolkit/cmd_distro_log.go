// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// logFlags are the stream log's flags, bound once so new, run and replay spell
// every one the same way.
type logFlags struct {
	fs            *flag.FlagSet
	profile       string
	mode          string
	columns       stringList
	format        string
	separator     string
	prefixOnly    bool
	color         string
	redact        stringList
	maxLineBytes  int
	textPath      string
	textOverwrite bool
	eventPath     string
	progress      string
	tick          time.Duration
	escalate      string
}

func (l *logFlags) bind(fs *flag.FlagSet) {
	l.bindRenderer(fs)
	fs.StringVar(&l.textPath, "stream-log", "", "append the rendered, uncoloured lines to this file")
	fs.BoolVar(&l.textOverwrite, "stream-log-overwrite", false, "replace the --stream-log file when the command starts, rather than appending")
	fs.StringVar(&l.eventPath, "event-log", "", "append one "+toolkit.EventLogSchema+" JSON record per line, per event, to this file")
	fs.StringVar(&l.progress, "progress-prefix", "", "consume a line that is this token, whitespace and a percentage with an optional label, and report the last one on the heartbeat")
	fs.DurationVar(&l.tick, "tick", 0, "after this much silence, write a heartbeat reading the distribution's state and disk. 0 is off; a rendering --log-profile turns it on at 30s")
	fs.StringVar(&l.escalate, "tick-escalate", "", "silence thresholds at which the heartbeat says more, comma separated, or none. Defaults to 2m,5m,15m whenever a heartbeat is on")
}

func (l *logFlags) bindRenderer(fs *flag.FlagSet) {
	l.fs = fs
	fs.StringVar(&l.profile, "log-profile", "", "raw, human, ci, forensic or wall. Empty forwards the command's streams unchanged. Flags passed beside a profile win over it")
	fs.StringVar(&l.mode, "timestamp-mode", "", "one timestamp column: rel, delta, wall, iso or epoch")
	fs.Var(&l.columns, "timestamp-column", "a timestamp column: rel, delta, wall, iso or epoch. Repeatable; a comma list is accepted")
	fs.StringVar(&l.format, "timestamp-format", "", "a strftime format for the rel and wall columns: %Y %m %d %H %M %S %z %Z %3f %6f %9f %%")
	fs.StringVar(&l.separator, "timestamp-separator", " ", "the text between the timestamp columns and the tag")
	fs.BoolVar(&l.prefixOnly, "prefix-only", false, "write the timestamp prefix and not the line itself")
	fs.StringVar(&l.color, "color", "auto", "colour the live prefix: auto, always or never. A file is never coloured")
	fs.Var(&l.redact, "redact", "a regular expression whose matches become *** before any sink sees a line. Repeatable; a comma list is accepted, and [,] is a literal comma")
	fs.IntVar(&l.maxLineBytes, "max-line-bytes", 0, "cut a line after this many bytes, at a character boundary, and say how many went. 0 never cuts")
}

func (l *logFlags) passed(name string) bool {
	found := false
	if l.fs != nil {
		l.fs.Visit(func(f *flag.Flag) {
			if f.Name == name {
				found = true
			}
		})
	}
	return found
}

// settings resolves the flags into validated settings, resolving the two sink
// paths the way every other path flag resolves. It opens nothing.
func (l *logFlags) settings(cfg toolkit.Config) (toolkit.LogSettings, error) {
	req := toolkit.LogRequest{
		Profile: l.profile, Mode: l.mode, Columns: l.columns, Format: l.format,
		Separator: l.separator, SeparatorSet: l.passed("timestamp-separator"), PrefixOnly: l.prefixOnly,
		Color: l.color, ColorSet: l.passed("color"), TextOverwrite: l.textOverwrite,
		Redact: l.redact, MaxLineBytes: l.maxLineBytes, ProgressPrefix: l.progress,
		Tick: l.tick, TickSet: l.passed("tick"), Escalate: l.escalate, EscalateSet: l.passed("tick-escalate"),
		Terminal: isTerminal(os.Stdout) && isTerminal(os.Stderr),
	}
	var err error
	// ⛔ THE RESERVED-NAME REFUSAL READS WHAT THE CALLER TYPED, before resolution
	// joins it to a directory, and again inside the resolver on the resolved path.
	for label, v := range map[string]string{"--stream-log": l.textPath, "--event-log": l.eventPath} {
		if err := toolkit.AssertSinkPath(label, v); err != nil {
			return toolkit.LogSettings{}, err
		}
	}
	if l.textPath != "" {
		if req.TextPath, err = pathFromProject(cfg, l.textPath); err != nil {
			return toolkit.LogSettings{}, fmt.Errorf("--stream-log: %w", err)
		}
	}
	if l.eventPath != "" {
		if req.EventPath, err = pathFromProject(cfg, l.eventPath); err != nil {
			return toolkit.LogSettings{}, fmt.Errorf("--event-log: %w", err)
		}
	}
	return toolkit.ResolveLogSettings(req)
}

func cmdDistroReplay(_ context.Context, args []string) (int, error) {
	fs := newFlagSet("distro replay")
	from := fs.String("from", "", "the "+toolkit.EventLogSchema+" file to render again")
	runIndex := fs.Int("run", 0, "render only this run of an appended log, counting from 1. 0 renders every run")
	var l logFlags
	l.bindRenderer(fs)
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	if *from == "" {
		return exitCannot, errors.New("--from is required: it names the event log a run was recorded into")
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	path, err := pathFromProject(cfg, *from)
	if err != nil {
		return exitCannot, fmt.Errorf("--from: %w", err)
	}
	// ⭐ A REPLAY WITH NO RENDERER FLAG RENDERS THE human PROFILE, because a
	// replay of a raw log with no prefix would lose the timing the record carries.
	if !l.passed("log-profile") && !l.passed("timestamp-mode") && !l.passed("timestamp-column") {
		l.profile = "human"
	}
	s, err := l.settings(cfg)
	if err != nil {
		return exitCannot, err
	}
	runs, err := toolkit.ReadEventRuns(path)
	if err != nil {
		return exitFailed, err
	}
	selected := runs
	if *runIndex != 0 {
		one, err := toolkit.SelectRun(runs, *runIndex)
		if err != nil {
			return exitCannot, err
		}
		selected = []toolkit.EventRun{one}
	}
	n, err := toolkit.Replay(selected, s, os.Stdout, os.Stderr)
	if err != nil {
		return exitFailed, err
	}
	for _, run := range selected {
		sum := toolkit.SummarizeRun(path, run, len(runs))
		logf("  replayed run %d of %d from %s: %s", run.Index, len(runs), path, summaryLine(sum))
	}
	logf("  %d line(s) rendered. Nothing was run", n)
	return exitOK, nil
}

func summaryLine(s toolkit.EventSummary) string {
	first := "no output"
	if s.FirstOutput != nil {
		first = spanOf(*s.FirstOutput) + " to first output"
	}
	code := "no exit recorded"
	if s.ExitCode != nil {
		code = "exit " + strconv.Itoa(*s.ExitCode)
		if s.TimedOut != nil && *s.TimedOut {
			code += ", timed out"
		}
	}
	return fmt.Sprintf("elapsed %s | %s | longest silence %s | out %d lines %s | err %d lines %s | %s",
		spanOf(s.Duration), first, spanOf(s.LongestSilence), s.StdoutLines, toolkit.HumanBytes(s.StdoutBytes),
		s.StderrLines, toolkit.HumanBytes(s.StderrBytes), code)
}

func spanOf(seconds float64) string {
	return toolkit.FormatSpan(time.Duration(seconds * float64(time.Second)))
}

// eventComparison is what `distro compare` answers.
type eventComparison struct {
	Schema string               `json:"schema"`
	Before toolkit.EventSummary `json:"before"`
	After  toolkit.EventSummary `json:"after"`
	// Delta is after minus before. ⛔ A figure either side could not measure is
	// null rather than zero: a run with no output did not answer instantly.
	Delta map[string]*float64 `json:"delta"`
}

func cmdDistroCompare(_ context.Context, args []string) (int, error) {
	fs := newFlagSet("distro compare")
	before := fs.String("before", "", "the earlier "+toolkit.EventLogSchema+" file")
	after := fs.String("after", "", "the later "+toolkit.EventLogSchema+" file")
	beforeRun := fs.Int("before-run", 0, "the run of --before to compare, counting from 1. 0 is its last run")
	afterRun := fs.Int("after-run", 0, "the run of --after to compare, counting from 1. 0 is its last run")
	asJSON := fs.Bool("json", false, "write the structured comparison")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	if *before == "" || *after == "" {
		return exitCannot, errors.New("--before and --after are both required")
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	sums := make([]toolkit.EventSummary, 2)
	for i, side := range []struct {
		flag, value string
		run         int
	}{{"--before", *before, *beforeRun}, {"--after", *after, *afterRun}} {
		path, err := pathFromProject(cfg, side.value)
		if err != nil {
			return exitCannot, fmt.Errorf("%s: %w", side.flag, err)
		}
		runs, err := toolkit.ReadEventRuns(path)
		if err != nil {
			return exitFailed, fmt.Errorf("%s: %w", side.flag, err)
		}
		run, err := toolkit.SelectRun(runs, side.run)
		if err != nil {
			return exitCannot, fmt.Errorf("%s: %w", side.flag, err)
		}
		sums[i] = toolkit.SummarizeRun(path, run, len(runs))
	}
	a, b := sums[0], sums[1]
	delta := func(x, y *float64) *float64 {
		if x == nil || y == nil {
			return nil
		}
		d := *y - *x
		return &d
	}
	f := func(v float64) *float64 { return &v }
	intPtr := func(v *int) *float64 {
		if v == nil {
			return nil
		}
		x := float64(*v)
		return &x
	}
	report := eventComparison{Schema: "wsl-toolkit-event-comparison/1", Before: a, After: b, Delta: map[string]*float64{
		"duration_seconds":        delta(f(a.Duration), f(b.Duration)),
		"first_output_seconds":    delta(a.FirstOutput, b.FirstOutput),
		"longest_silence_seconds": delta(f(a.LongestSilence), f(b.LongestSilence)),
		"stdout_lines":            delta(f(float64(a.StdoutLines)), f(float64(b.StdoutLines))),
		"stdout_bytes":            delta(f(float64(a.StdoutBytes)), f(float64(b.StdoutBytes))),
		"stderr_lines":            delta(f(float64(a.StderrLines)), f(float64(b.StderrLines))),
		"stderr_bytes":            delta(f(float64(a.StderrBytes)), f(float64(b.StderrBytes))),
		"exit_code":               delta(intPtr(a.ExitCode), intPtr(b.ExitCode)),
	}}
	for _, s := range sums {
		if s.Runs > 1 {
			logf("  %s holds %d runs, and run %d is compared", s.Path, s.Runs, s.Run)
		}
	}
	if *asJSON {
		return exitOK, writeJSON(report)
	}
	num := func(v *float64, span bool) string {
		if v == nil {
			return "-"
		}
		if span {
			return spanOf(*v)
		}
		return strconv.FormatFloat(*v, 'f', -1, 64)
	}
	signed := func(v *float64) string {
		if v == nil {
			return "-"
		}
		return strconv.FormatFloat(*v, 'f', 3, 64)
	}
	rows := []struct {
		label  string
		a, b   *float64
		key    string
		isSpan bool
	}{
		{"elapsed", f(a.Duration), f(b.Duration), "duration_seconds", true},
		{"to first output", a.FirstOutput, b.FirstOutput, "first_output_seconds", true},
		{"longest silence", f(a.LongestSilence), f(b.LongestSilence), "longest_silence_seconds", true},
		{"out lines", f(float64(a.StdoutLines)), f(float64(b.StdoutLines)), "stdout_lines", false},
		{"out bytes", f(float64(a.StdoutBytes)), f(float64(b.StdoutBytes)), "stdout_bytes", false},
		{"err lines", f(float64(a.StderrLines)), f(float64(b.StderrLines)), "stderr_lines", false},
		{"err bytes", f(float64(a.StderrBytes)), f(float64(b.StderrBytes)), "stderr_bytes", false},
		{"exit code", intPtr(a.ExitCode), intPtr(b.ExitCode), "exit_code", false},
	}
	// ⭐ THE TABLE IS THE ANSWER, so it is the one thing on stdout. It reports and
	// does not judge: a ratio that is a regression for one workload is noise for
	// another, and a threshold here would be a standard nobody set.
	fmt.Printf("%-16s %-14s %-14s %s\n", "figure", "before", "after", "after - before")
	for _, r := range rows {
		fmt.Printf("%-16s %-14s %-14s %s\n", r.label, num(r.a, r.isSpan), num(r.b, r.isSpan), signed(report.Delta[r.key]))
	}
	if a.LongestSilence != b.LongestSilence {
		longer, at := "after", b.LongestSilenceEnds
		if a.LongestSilence > b.LongestSilence {
			longer, at = "before", a.LongestSilenceEnds
		}
		logf("  ! %s has the longer silence, ending %s into its run", longer, spanOf(at))
	}
	if intPtrString(a.ExitCode) != intPtrString(b.ExitCode) {
		logf("  ! the two runs did not end the same way: before %s, after %s", intPtrString(a.ExitCode), intPtrString(b.ExitCode))
	}
	return exitOK, nil
}

func intPtrString(v *int) string {
	if v == nil {
		return "no exit recorded"
	}
	return strconv.Itoa(*v)
}
