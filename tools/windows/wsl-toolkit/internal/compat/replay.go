// SPDX-License-Identifier: 0BSD

package compat

import (
	"fmt"
	"strconv"
	"strings"
)

// Replay and Compare, ported from the script's action-replay.ps1: render one
// recorded run again, in whatever timestamp shape is asked for, and put two
// runs side by side.

// actionReplay renders a recorded run again.
//
// ⭐ IT IS POSSIBLE BECAUSE THE RENDERER IS ALREADY A PURE FUNCTION OF THE
// RECORD. The prefix builder needs a clock reading and a tag, and both are in
// every record, so this reuses the renderer rather than growing a second one.
// A second renderer is how a log file and a terminal come to show different
// runs.
//
// ⛔ IT RUNS NOTHING AND CREATES NOTHING. It reads a file and writes to the
// two streams.
func (s *session) actionReplay() int {
	if s.opts.From == "" {
		s.fail("Action Replay requires -From <event log>.")
		return 1
	}
	records, err := readEventLogFile(s.opts.From)
	if err != nil {
		s.fail(err.Error())
		return 1
	}

	// ⭐ THE SETTINGS Run ALREADY RESOLVED, not a second resolution. Resolving
	// them again would be a second place where a profile is expanded and the
	// two would drift: a Replay would render a line one way and a live run the
	// other, from one set of flags.
	// ⛔ -NoTimestamps and -TimestampProfile raw turn the relay off entirely,
	// so Run builds no settings for them; on a Replay that means the plain
	// text.
	if s.relayOff || s.settings == nil {
		for _, r := range records {
			if r.Kind != "LOG" && r.Kind != "TICK" && r.Kind != "NOTE" {
				continue
			}
			isErr := r.Kind != "LOG" || r.Stream == "stderr"
			if isErr {
				s.log.noteLine(r.Text)
			} else {
				fmt.Fprintln(s.out, r.Text)
			}
		}
		plain := eventLogSummary(records)
		s.log.step(fmt.Sprintf("replayed %d record(s) from %s, with no prefix", plain.Records, s.opts.From))
		s.log.ok(formatRunSummary(plain))
		return 0
	}

	distro := "(unknown)"
	if records[0].Distro != "" {
		distro = records[0].Distro
	}
	// The state's own clock is never read here: every prefix is built from the
	// record's own t_rel, with the record's own wall reading beside it.
	state := newStreamState(distro, s.settings, func() dur { return 0 }, s.out, s.errw)
	last := dur(0)
	for _, r := range records {
		if r.Kind != "LOG" && r.Kind != "TICK" && r.Kind != "NOTE" {
			continue
		}
		nowD := durFromSeconds(r.TRel - records[0].TRel)
		tag := "note"
		switch r.Kind {
		case "LOG":
			tag = "out"
			if r.Stream == "stderr" {
				tag = "err"
			}
		case "TICK":
			tag = "tick"
		}
		partial := r.Partial
		// ⚠ THE RECORD'S OWN WALL READING, not this machine's clock. A replay
		// of last week's run stamped with today's date is a document that says
		// something false about when the work happened.
		var wall *wallReading
		if parsed := parseWallReading(r.TWall); parsed != nil {
			wall = parsed
		}
		prefix, err := formatStreamLogPrefix(state, tag, nowD, nowD-last, partial, wall)
		if err != nil {
			prefix = "??"
		}
		body := ""
		if !s.settings.PrefixOnly {
			body = " " + r.Text
		}
		if tag == "out" {
			fmt.Fprintln(s.out, prefix+body)
		} else {
			fmt.Fprintln(s.errw, prefix+body)
		}
		last = nowD
	}

	sum := eventLogSummary(records)
	s.log.step(fmt.Sprintf("replayed %d record(s) from %s", sum.Records, s.opts.From))
	s.log.ok(formatRunSummary(sum))
	return 0
}

// actionCompare puts two recorded runs side by side.
//
// ⭐ THE LONGEST SILENCE IS WHY THIS EXISTS. Two runs that both exited 0 are
// the same result and can be very different runs, and the figure that says so
// is not in the exit code.
//
// ⛔ IT REPORTS AND DOES NOT JUDGE. There is no threshold at which it calls a
// difference a regression: a ratio that is a regression for one workload is
// noise for another.
func (s *session) actionCompare() int {
	if s.opts.From == "" {
		s.fail("Action Compare requires -From <event log>.")
		return 1
	}
	if s.opts.Against == "" {
		s.fail("Action Compare requires -Against <event log>.")
		return 1
	}
	aRecords, err := readEventLogFile(s.opts.From)
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	bRecords, err := readEventLogFile(s.opts.Against)
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	a := eventLogSummary(aRecords)
	b := eventLogSummary(bRecords)

	spanOpt := func(v *float64) string {
		if v == nil {
			return "-"
		}
		return formatDuration(durFromSeconds(*v))
	}
	span := func(v float64) string { return formatDuration(durFromSeconds(v)) }
	numOpt := func(v *int) string {
		if v == nil {
			return "-"
		}
		return strconv.Itoa(*v)
	}
	num := func(v int64) string { return strconv.FormatInt(v, 10) }

	s.log.step("A: " + s.opts.From)
	s.log.step("B: " + s.opts.Against)
	// ⛔ A DASH WHERE THE VALUE IS UNKNOWN, never a zero. A fabricated number
	// on a report is worse than a blank, because a blank gets checked and a
	// number gets used.
	type row struct{ label, a, b, d string }
	rows := []row{
		{"elapsed", span(a.Duration), span(b.Duration), deltaF(a.Duration, b.Duration)},
		{"to first output", spanOpt(a.FirstOutput), spanOpt(b.FirstOutput), deltaFOpt(a.FirstOutput, b.FirstOutput)},
		{"longest silence", span(a.LongestGap), span(b.LongestGap), deltaF(a.LongestGap, b.LongestGap)},
		{"out lines", num(a.OutLines), num(b.OutLines), deltaI(a.OutLines, b.OutLines)},
		{"out bytes", num(a.OutBytes), num(b.OutBytes), deltaI(a.OutBytes, b.OutBytes)},
		{"err lines", num(a.ErrLines), num(b.ErrLines), deltaI(a.ErrLines, b.ErrLines)},
		{"err bytes", num(a.ErrBytes), num(b.ErrBytes), deltaI(a.ErrBytes, b.ErrBytes)},
		{"exit code", numOpt(a.ExitCode), numOpt(b.ExitCode), deltaIOpt(a.ExitCode, b.ExitCode)},
	}
	s.log.noteLine(fmt.Sprintf("  %-16s %-14s %-14s %s", "figure", "A", "B", "B - A"))
	for _, r := range rows {
		s.log.noteLine(fmt.Sprintf("  %-16s %-14s %-14s %s", r.label, r.a, r.b, r.d))
	}
	if a.LongestGap != b.LongestGap {
		longer := "B"
		at := b.LongestGapAt
		if b.LongestGap < a.LongestGap {
			longer = "A"
			at = a.LongestGapAt
		}
		longest := a.LongestGap
		if b.LongestGap > longest {
			longest = b.LongestGap
		}
		s.log.warn(fmt.Sprintf("%s has the longer silence, %s, ending at %s into the run.",
			longer, span(longest), span(at)))
	}
	if (a.ExitCode == nil) != (b.ExitCode == nil) || (a.ExitCode != nil && b.ExitCode != nil && *a.ExitCode != *b.ExitCode) {
		s.log.warn("the two runs did not end the same way: A " + numOpt(a.ExitCode) + ", B " + numOpt(b.ExitCode))
	}
	return 0
}

// deltaF formats B - A for two known floats, with an explicit + on a rise.
func deltaF(x, y float64) string {
	return signedFloat(y - x)
}

func deltaFOpt(x, y *float64) string {
	if x == nil || y == nil {
		return "-"
	}
	return deltaF(*x, *y)
}

func deltaI(x, y int64) string {
	return signedInt(y - x)
}

func deltaIOpt(x, y *int) string {
	if x == nil || y == nil {
		return "-"
	}
	return signedInt(int64(*y - *x))
}

func signedFloat(d float64) string {
	t := fmt.Sprintf("%.3f", d)
	t = strings.TrimRight(strings.TrimRight(t, "0"), ".")
	if t == "-0" || t == "" {
		t = "0"
	}
	if d > 0 {
		return "+" + t
	}
	return t
}

func signedInt(d int64) string {
	if d > 0 {
		return "+" + strconv.FormatInt(d, 10)
	}
	return strconv.FormatInt(d, 10)
}

// fail writes the one error line the interface reports through. It goes to
// the session's notes stream, never to the report: an error is not a result,
// and -Action HostAddress makes that concrete - a caller assigning the report
// would otherwise act on the word "ERROR:" where an address goes.
func (s *session) fail(message string) {
	fmt.Fprintf(s.errw, "ERROR: %s\n", message)
}
