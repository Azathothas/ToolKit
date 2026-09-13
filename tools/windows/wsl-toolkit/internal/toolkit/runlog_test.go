// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClock is the relay's clock in these cases, so a rendering is asserted
// exactly rather than as a plausible-looking one.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }
func (c *fakeClock) at(d time.Duration) {
	c.t = time.Date(2026, 8, 30, 4, 56, 24, 0, time.UTC).Add(d)
}

func settingsFor(t *testing.T, r LogRequest) LogSettings {
	t.Helper()
	s, err := ResolveLogSettings(r)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type relayHarness struct {
	log      *RunLog
	clock    *fakeClock
	out, err bytes.Buffer
}

func newRelay(t *testing.T, s LogSettings) *relayHarness {
	t.Helper()
	h := &relayHarness{clock: &fakeClock{}}
	h.clock.at(0)
	log, err := OpenRunLog(s, &h.out, &h.err)
	if err != nil {
		t.Fatal(err)
	}
	log.now, log.manual = h.clock.now, true
	log.Begin("eph-test", nil)
	h.log = log
	return h
}

func (h *relayHarness) lines(buf *bytes.Buffer) []string {
	var out []string
	for _, l := range strings.Split(buf.String(), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestACompleteLinePadsTheTagToFourAndAnUnterminatedOneMarksIt(t *testing.T) {
	h := newRelay(t, settingsFor(t, LogRequest{Mode: "epoch"}))
	h.log.emit("out", "obs", "a", false, 0)
	h.log.emit("out", "obs", "b", true, 0)
	var tags []string
	for _, l := range h.lines(&h.out) {
		tags = append(tags, l[strings.Index(l, " ")+1:strings.Index(l, " ")+5])
	}
	if strings.Join(tags, "|") != "out |out~" {
		t.Fatalf("tags = %q", tags)
	}
}

func TestGuestStdoutGoesToStdoutAndStderrTickAndNoteGoToStderr(t *testing.T) {
	h := newRelay(t, settingsFor(t, LogRequest{Mode: "epoch"}))
	h.log.emit("out", "obs", "a", false, 0)
	h.log.emit("err", "obs", "b", false, 0)
	h.log.emit("tick", "obs", "c", false, 0)
	h.log.emit("note", "obs", "d", false, 0)
	if len(h.lines(&h.out)) != 1 || len(h.lines(&h.err)) != 3 {
		t.Fatalf("stdout %q stderr %q", h.out.String(), h.err.String())
	}
}

// TestATickDoesNotAdvanceTheDeltaClock is the regression the stream log shipped
// once: a tick advancing the clock made a five-second gap render as a fraction of
// a second against a tick written to the other stream.
func TestATickDoesNotAdvanceTheDeltaClock(t *testing.T) {
	h := newRelay(t, settingsFor(t, LogRequest{Mode: "delta"}))
	h.log.emit("out", "obs", "one", false, time.Second)
	h.log.emit("tick", "obs", "quiet", false, 3*time.Second)
	h.log.emit("note", "obs", "watcher", false, 4*time.Second)
	h.log.emit("out", "obs", "two", false, 6*time.Second)
	var stamps []string
	for _, l := range h.lines(&h.out) {
		stamps = append(stamps, strings.Fields(l)[0])
	}
	if strings.Join(stamps, "|") != "+1.000|+5.000" {
		t.Fatalf("stamps = %q", stamps)
	}
	if got := strings.Fields(h.lines(&h.err)[0])[0]; got != "+2.000" {
		t.Fatalf("the tick reads %s, want the +2.000 gap the delta column would", got)
	}
}

func TestRelativeStampsComposeWithDeltaAndTheSeparatorSitsBeforeTheTag(t *testing.T) {
	h := newRelay(t, settingsFor(t, LogRequest{Mode: "rel"}))
	h.log.emit("out", "obs", "x", false, 65500*time.Millisecond)
	if got := h.lines(&h.out)[0]; got != "00:01:05.500 out  x" {
		t.Fatalf("relative line = %q", got)
	}
	h = newRelay(t, settingsFor(t, LogRequest{Columns: []string{"rel,delta"}}))
	h.log.emit("out", "obs", "one", false, time.Second)
	h.log.emit("out", "obs", "two", false, 6*time.Second)
	if got := h.lines(&h.out)[1]; got != "00:00:06.000 +5.000 out  two" {
		t.Fatalf("composed line = %q", got)
	}
	h = newRelay(t, settingsFor(t, LogRequest{Mode: "rel", Separator: "|", SeparatorSet: true}))
	h.log.emit("out", "obs", "x", false, time.Second)
	if got := h.lines(&h.out)[0]; got != "00:00:01.000|out  x" {
		t.Fatalf("separated line = %q", got)
	}
	h = newRelay(t, settingsFor(t, LogRequest{Mode: "rel", PrefixOnly: true}))
	h.log.emit("out", "obs", "a secret nobody should see", false, time.Second)
	if got := h.lines(&h.out)[0]; got != "00:00:01.000 out " {
		t.Fatalf("prefix-only line = %q", got)
	}
}

func TestTheStrftimeRendererAndItsRefusals(t *testing.T) {
	wall := time.Date(2026, 8, 30, 4, 56, 24, 123456789, time.FixedZone("TST", 5*3600+1800))
	for format, want := range map[string]string{
		"%Y-%m-%d %H:%M:%S": "2026-08-30 04:56:24",
		"%m/%M":             "08/56",
		"100%%":             "100%",
		"%6f":               "123456",
		"%9f":               "123456789",
		"%z":                "+05:30",
		"%Z":                "TST",
	} {
		if got, err := Strftime(format, wall, 0, false); err != nil || got != want {
			t.Errorf("wall %q = %q, %v; want %q", format, got, err, want)
		}
	}
	for elapsed, want := range map[time.Duration]string{
		time.Hour + 2*time.Minute + 3456*time.Millisecond: "01:02:03.456",
		25*time.Hour + time.Second:                        "25:00:01.000",
	} {
		if got, err := Strftime("%H:%M:%S.%3f", time.Time{}, elapsed, true); err != nil || got != want {
			t.Errorf("relative %s = %q, %v; want %q", elapsed, got, err, want)
		}
	}
	for _, c := range []struct {
		format   string
		relative bool
		names    string
	}{{"%q", false, "%q"}, {"%Y", true, "%Y"}, {"trailing %", false, "bare %"}} {
		if _, err := Strftime(c.format, wall, 0, c.relative); err == nil || !strings.Contains(err.Error(), c.names) {
			t.Errorf("format %q (relative %v) was not refused naming %s: %v", c.format, c.relative, c.names, err)
		}
	}
}

func TestProfilesAreStartingPointsAndContradictionsAreRefused(t *testing.T) {
	if s := settingsFor(t, LogRequest{Profile: "ci"}); strings.Join(s.Columns, "|") != "rel|delta" || s.Color || s.Tick != ProfileTick {
		t.Errorf("ci = %v colour %v tick %s", s.Columns, s.Color, s.Tick)
	}
	if s := settingsFor(t, LogRequest{Profile: "wall"}); strings.Join(s.Columns, "|") != "wall" || s.formatFor("wall") != "%Y-%m-%d %H:%M:%S" {
		t.Errorf("wall = %v %q", s.Columns, s.formatFor("wall"))
	}
	if s := settingsFor(t, LogRequest{Profile: "ci", Columns: []string{"epoch"}}); strings.Join(s.Columns, "|") != "epoch" {
		t.Errorf("an explicit column did not beat the profile: %v", s.Columns)
	}
	s, err := ResolveLogSettings(LogRequest{Profile: "ci", Color: "always", ColorSet: true})
	if err != nil || !s.Color {
		t.Errorf("an explicit --color always was overruled by the ci profile: %v %v", s.Color, err)
	}
	// ⚠ THE forensic PROFILE RENDERS. Its predecessor applied a dated format to
	// the elapsed column and refused itself before every run.
	if s, err := ResolveLogSettings(LogRequest{Profile: "forensic"}); err != nil || strings.Join(s.Columns, "|") != "wall|rel|delta" {
		t.Errorf("forensic = %v, %v", s.Columns, err)
	}
	if s := settingsFor(t, LogRequest{}); s.Active() || len(s.Columns) != 0 {
		t.Errorf("no options at all is not the unchanged stream: %+v", s)
	}
	for label, r := range map[string]LogRequest{
		"a mode beside a column":           {Mode: "delta", Columns: []string{"rel"}},
		"an unknown column":                {Columns: []string{"rel", "moon"}},
		"a column named twice":             {Columns: []string{"rel", "rel"}},
		"a format no column takes":         {Columns: []string{"epoch"}, Format: "%H"},
		"an unknown format specifier":      {Mode: "rel", Format: "%H:%q"},
		"a date specifier on elapsed time": {Mode: "rel", Format: "%Y"},
		"raw beside a renderer flag":       {Profile: "raw", Columns: []string{"rel"}},
		"prefix-only with no column":       {PrefixOnly: true},
		"colour with no column":            {Color: "always", ColorSet: true},
		"a separator with no column":       {Separator: "|", SeparatorSet: true},
		"an unknown profile":               {Profile: "loud"},
		"a progress token with whitespace": {ProgressPrefix: "PROGRESS "},
		"a line bound past the limit":      {MaxLineBytes: MaxLineBytesLimit + 1},
		"escalation with no heartbeat":     {Escalate: "2m", EscalateSet: true},
		"overwrite with no stream log":     {TextOverwrite: true},
		"one file for both sinks":          {TextPath: "run.log", EventPath: "RUN.LOG"},
		"a redaction that cannot compile":  {Redact: []string{"(unclosed"}},
		"an empty redaction":               {Redact: []string{" , "}},
	} {
		if _, err := ResolveLogSettings(r); err == nil {
			t.Errorf("%s was accepted", label)
		}
	}
	if _, err := ResolveLogSettings(LogRequest{Profile: "raw", EventPath: "events.jsonl", Tick: time.Second, TickSet: true}); err != nil {
		t.Errorf("raw with a sink and a heartbeat was refused, and neither renders: %v", err)
	}
}

func TestEscalationThresholdsAreDurationsSortedAndDeduplicated(t *testing.T) {
	got, err := ParseEscalation("12m, 5m,9m,5m")
	if err != nil || len(got) != 3 || got[0] != 5*time.Minute || got[2] != 12*time.Minute {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := ParseEscalation("5m,soon"); err == nil {
		t.Error("a threshold that is not a duration was accepted")
	}
	if got, err := ParseEscalation("none"); err != nil || len(got) != 0 {
		t.Errorf("none = %v, %v", got, err)
	}
}

func TestRedactionReplacesTheMatchAloneBeforeEverySink(t *testing.T) {
	s := settingsFor(t, LogRequest{Mode: "epoch", Redact: []string{"ghp_[A-Za-z0-9]+,SECRET"}})
	if got := s.Redact("token=ghp_abc123 aSECRETb"); got != "token=*** a***b" {
		t.Fatalf("redacted = %q", got)
	}
	if got := settingsFor(t, LogRequest{}).Redact("token=ghp_abc123"); got != "token=ghp_abc123" {
		t.Fatalf("no pattern changed the text: %q", got)
	}
	dir := t.TempDir()
	s.TextPath, s.EventPath = filepath.Join(dir, "run.log"), filepath.Join(dir, "events.jsonl")
	h := newRelay(t, s)
	_, _ = h.log.Stdout().Write([]byte("password SECRET here\n"))
	if err := h.log.Finish(RunOutcome{}, nil); err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]string{"live": h.out.String(), "text": mustRead(t, s.TextPath), "events": mustRead(t, s.EventPath)} {
		if strings.Contains(got, "SECRET") || !strings.Contains(got, "***") {
			t.Errorf("the %s sink was not redacted: %q", name, got)
		}
	}
}

func TestARedactionCommaListKeepsRegexCommasInsideSyntax(t *testing.T) {
	s := settingsFor(t, LogRequest{Redact: []string{`[,],z{1,3},SECRET`}})
	if s.RedactCount != 3 {
		t.Fatalf("compiled %d pattern(s), want the three expressions in the list", s.RedactCount)
	}
	if got := s.Redact("comma, zzz SECRET"); got != "comma*** *** ***" {
		t.Fatalf("redacted = %q", got)
	}
	for raw, want := range map[string]string{
		`[]a,b],x`:        `[]a,b]|x`,
		`[^],]q,y`:        `[^],]q|y`,
		`[[:alpha:],]+,z`: `[[:alpha:],]+|z`,
		`\,x,y`:           `\,x|y`,
		`a{2,}b,c`:        `a{2,}b|c`,
		`(unclosed[,x`:    `(unclosed[,x`,
	} {
		if got := strings.Join(splitRegexList(raw), "|"); got != want {
			t.Errorf("splitRegexList(%q) = %q, want %q", raw, got, want)
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("the pipe is being closed") }

// TestAClosedLiveStreamDoesNotStopTheRecord is the caller who stopped reading:
// the event log and the stream log are still the caller's to keep, and a record
// that ended at the closed pipe would read as a run with no exit.
func TestAClosedLiveStreamDoesNotStopTheRecord(t *testing.T) {
	dir := t.TempDir()
	s := settingsFor(t, LogRequest{Mode: "rel", TextPath: filepath.Join(dir, "run.log"), EventPath: filepath.Join(dir, "events.jsonl")})
	var stderr bytes.Buffer
	log, err := OpenRunLog(s, failingWriter{}, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{}
	clock.at(0)
	log.now, log.manual = clock.now, true
	log.Begin("eph-test", nil)
	_, _ = log.Stdout().Write([]byte("one\ntwo\n"))
	_, _ = log.Stderr().Write([]byte("to-stderr\n"))
	ferr := log.Finish(RunOutcome{Exit: 0}, nil)
	if ferr == nil || !strings.Contains(ferr.Error(), "stdout") {
		t.Fatalf("a closed stdout was not reported: %v", ferr)
	}
	if strings.Contains(ferr.Error(), "--event-log") || strings.Contains(ferr.Error(), "--stream-log") {
		t.Fatalf("a closed live stream was reported as an unwritable log: %v", ferr)
	}
	runs, err := ReadEventRuns(s.EventPath)
	if err != nil {
		t.Fatal(err)
	}
	events := runs[0].Events
	if last := events[len(events)-1]; last.Kind != "EXIT" {
		t.Fatalf("the record stopped at the closed stream: its last record is %+v", last)
	}
	if sum := SummarizeRun(s.EventPath, runs[0], 1); sum.StdoutLines != 2 || sum.StderrLines != 1 {
		t.Fatalf("the record lost lines written after the stream closed: %+v", sum)
	}
	if got := mustRead(t, s.TextPath); !strings.Contains(got, "out  two") || !strings.Contains(stderr.String(), "err  to-stderr") {
		t.Fatalf("the stream log or the open stderr lost lines: %q / %q", got, stderr.String())
	}
}

func TestAHeldCarriageReturnEndsTheLineAndIsNotPartOfItsText(t *testing.T) {
	dir := t.TempDir()
	s := settingsFor(t, LogRequest{Mode: "epoch", EventPath: filepath.Join(dir, "events.jsonl")})
	h := newRelay(t, s)
	_, _ = h.log.Stdout().Write([]byte("redrawn\r"))
	h.clock.at(StreamFlushAfter)
	h.log.check()
	_, _ = h.log.Stdout().Write([]byte("last\r"))
	if err := h.log.Finish(RunOutcome{}, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(h.out.String(), "\r") || strings.Contains(mustRead(t, s.EventPath), `\r`) {
		t.Fatalf("a held carriage return was written as part of a line: %q", h.out.String())
	}
	var bodies []string
	for _, l := range h.lines(&h.out) {
		bodies = append(bodies, l[strings.Index(l, " ")+1:])
	}
	if strings.Join(bodies, "|") != "out~ redrawn|out~ last" {
		t.Fatalf("lines = %q", bodies)
	}
}

func TestALineIsCutAtAWholeCharacterAndSaysHowMuchWent(t *testing.T) {
	for _, c := range []struct {
		text string
		max  int
		want string
	}{
		{"abcde", 10, "abcde"}, {"abcde", 0, "abcde"}, {"abcde", 3, "abc...(+2 bytes cut)"},
		{"ab\u00e9", 3, "ab...(+2 bytes cut)"},
	} {
		if got := LimitLineBytes(c.text, c.max); got != c.want {
			t.Errorf("LimitLineBytes(%q, %d) = %q, want %q", c.text, c.max, got, c.want)
		}
	}
	// ⛔ THE SAME LINE CUTS THE SAME WAY HOWEVER THE PIPE DELIVERED IT.
	s := settingsFor(t, LogRequest{MaxLineBytes: 5})
	one, many := newRelay(t, s), newRelay(t, s)
	_, _ = one.log.Stdout().Write([]byte("0123456789\n"))
	for _, b := range []byte("0123456789\n") {
		_, _ = many.log.Stdout().Write([]byte{b})
	}
	if one.out.String() != many.out.String() || one.out.String() != "01234...(+5 bytes cut)\n" {
		t.Fatalf("one write %q, byte by byte %q", one.out.String(), many.out.String())
	}
}

func TestTheSplitterEndsLinesOnNewlinesAndCarriageReturns(t *testing.T) {
	cases := []struct {
		label  string
		writes []string
		want   string
	}{
		{"two newline-terminated lines", []string{"a\nb\n"}, "=a|=b"},
		{"an unterminated tail is held", []string{"a\nb"}, "=a"},
		{"CRLF terminates once", []string{"a\r\nb\r\n"}, "=a|=b"},
		{"a carriage return marks a redrawn line", []string{"50%\r75%\r"}, "~50%"},
		{"a CRLF split across two reads is one line", []string{"abc\r", "\ndef\n"}, "=abc|=def"},
		{"an empty line is a line", []string{"\na\n"}, "=|=a"},
		{"an empty write is nothing", []string{""}, ""},
	}
	for _, c := range cases {
		h := newRelay(t, settingsFor(t, LogRequest{Mode: "epoch"}))
		for _, w := range c.writes {
			_, _ = h.log.Stdout().Write([]byte(w))
		}
		var got []string
		for _, l := range h.lines(&h.out) {
			tag := l[strings.Index(l, " ")+1 : strings.Index(l, " ")+5]
			body := l[strings.Index(l, " ")+6:]
			mark := "="
			if strings.HasSuffix(tag, "~") {
				mark = "~"
			}
			got = append(got, mark+body)
		}
		if strings.Join(got, "|") != c.want {
			t.Errorf("%s: got %q, want %q", c.label, strings.Join(got, "|"), c.want)
		}
	}
}

func TestAnUnterminatedLineIsShownEarlyAfterTheFlushBound(t *testing.T) {
	h := newRelay(t, settingsFor(t, LogRequest{Mode: "epoch"}))
	_, _ = h.log.Stdout().Write([]byte("Password: "))
	h.clock.at(StreamFlushAfter - time.Millisecond)
	h.log.check()
	if h.out.Len() != 0 {
		t.Fatalf("the line was flushed before its bound: %q", h.out.String())
	}
	h.clock.at(StreamFlushAfter)
	h.log.check()
	if got := h.lines(&h.out); len(got) != 1 || !strings.Contains(got[0], "out~ Password: ") {
		t.Fatalf("after the bound the relay shows %q", h.out.String())
	}
}

func TestWithNothingRenderingTheLiveBytesPassThroughExactly(t *testing.T) {
	dir := t.TempDir()
	s := settingsFor(t, LogRequest{EventPath: filepath.Join(dir, "events.jsonl")})
	h := newRelay(t, s)
	raw := "10%\r50%\r100%\r\ndone without a newline"
	_, _ = h.log.Stdout().Write([]byte(raw))
	if err := h.log.Finish(RunOutcome{}, nil); err != nil {
		t.Fatal(err)
	}
	if h.out.String() != raw {
		t.Fatalf("the live stream was rewritten: %q", h.out.String())
	}
	runs, err := ReadEventRuns(s.EventPath)
	if err != nil {
		t.Fatal(err)
	}
	if sum := SummarizeRun(s.EventPath, runs[0], 1); sum.StdoutLines != 4 {
		t.Fatalf("the sink recorded %d lines, want the four the splitter found", sum.StdoutLines)
	}
}

func TestAProgressLineIsParsedOnlyWhenItIsOne(t *testing.T) {
	for line, want := range map[string]string{
		"WTK 42 unpacking":         "42|unpacking",
		"WTK 42% unpacking":        "42|unpacking",
		"WTK 100":                  "100|",
		"WTK 42.5 linking":         "42.5|linking",
		"WTK\t7\tlabel with\ttabs": "7|label with\ttabs",
		"WTK nope":                 "",
		"WTK 101":                  "",
		"WTK NaN":                  "",
		"WTK +42":                  "",
		"WTK 4.25":                 "",
		"WTKINSTALL 42":            "",
		"WTK42":                    "",
		"ordinary output 42":       "",
	} {
		pct, label, ok := ParseProgress("WTK", line)
		got := ""
		if ok {
			got = strings.TrimSuffix(strings.TrimSuffix(FormatPercent(pct), "%"), ".0") + "|" + label
		}
		if got != want {
			t.Errorf("ParseProgress(%q) = %q, want %q", line, got, want)
		}
	}
	if _, _, ok := ParseProgress("", "WTK 42"); ok {
		t.Error("no token consumed a line")
	}
	if FormatPercent(42) != "42%" || FormatPercent(42.5) != "42.5%" {
		t.Errorf("FormatPercent = %s %s", FormatPercent(42), FormatPercent(42.5))
	}
}

func TestAConsumedProgressLineIsNotRenderedAndTheTickCarriesItsAge(t *testing.T) {
	dir := t.TempDir()
	s := settingsFor(t, LogRequest{Mode: "rel", ProgressPrefix: "WTK", Tick: time.Second, TickSet: true,
		EventPath: filepath.Join(dir, "events.jsonl")})
	h := newRelay(t, s)
	h.log.facts = func() TickFacts { return TickFacts{State: "running"} }
	_, _ = h.log.Stdout().Write([]byte("WTK 40 copying\n"))
	_, _ = h.log.Stdout().Write([]byte("WTK 60"))
	h.clock.at(StreamFlushAfter)
	h.log.check()
	h.clock.at(StreamFlushAfter + 5*time.Second)
	h.log.check()
	if strings.Contains(h.out.String(), "WTK 40") {
		t.Fatalf("a consumed progress line was rendered: %q", h.out.String())
	}
	// ⛔ A PARTIAL LINE IS NEVER PROGRESS: its label may still be arriving.
	if !strings.Contains(h.out.String(), "out~ WTK 60") {
		t.Fatalf("an unterminated progress-shaped line was not relayed: %q", h.out.String())
	}
	if !strings.Contains(h.err.String(), "progress 40% copying (7s ago)") {
		t.Fatalf("the tick does not carry the last progress and its age: %q", h.err.String())
	}
	_ = h.log.Finish(RunOutcome{}, nil)
	if events := mustRead(t, s.EventPath); !strings.Contains(events, `"kind":"PROGRESS"`) || !strings.Contains(events, `"progress_percent":40`) {
		t.Fatalf("the PROGRESS record is missing: %s", events)
	}
}

func TestTheHeartbeatReadsTheDiskAndEscalatesOncePerSilence(t *testing.T) {
	dir := t.TempDir()
	s := settingsFor(t, LogRequest{Mode: "rel", Tick: time.Second, TickSet: true, Escalate: "3s,6s", EscalateSet: true,
		EventPath: filepath.Join(dir, "events.jsonl")})
	h := newRelay(t, s)
	disk := int64(100 << 20)
	h.log.facts = func() TickFacts { d := disk; return TickFacts{State: "running", DiskBytes: &d} }
	_, _ = h.log.Stdout().Write([]byte("started\n"))
	for i := 1; i <= 7; i++ {
		if i == 4 {
			disk += 8 << 20
		}
		h.clock.at(time.Duration(i) * time.Second)
		h.log.check()
	}
	stderr := h.err.String()
	if !strings.Contains(stderr, "disk 100.0 MiB (unchanged)") || !strings.Contains(stderr, "(+8.0 MiB since the last tick)") {
		t.Fatalf("the heartbeat does not report the disk reading and its growth: %s", stderr)
	}
	if strings.Count(stderr, "block-buffers") != 1 {
		t.Fatalf("the buffering note should appear once, at the first threshold: %s", stderr)
	}
	if strings.Count(stderr, "of silence:") != 2 || !strings.Contains(stderr, "pass --timeout") {
		t.Fatalf("each threshold should fire once, and the second add the advice: %s", stderr)
	}
	_, _ = h.log.Stdout().Write([]byte("back\n"))
	if !strings.Contains(h.err.String(), "output resumed after 7s of silence") {
		t.Fatalf("output coming back is not said: %s", h.err.String())
	}
	_ = h.log.Finish(RunOutcome{}, nil)
	events := mustRead(t, s.EventPath)
	for _, want := range []string{`"kind":"TICK_FACTS"`, `"distro_state":"running"`, `"disk_grew_bytes":8388608`, `"prov":"inf"`} {
		if !strings.Contains(events, want) {
			t.Errorf("the event log carries no %s", want)
		}
	}
}

func TestFinishRecordsTheEndAndSaysWhatANonzeroExitCanMean(t *testing.T) {
	dir := t.TempDir()
	s := settingsFor(t, LogRequest{Mode: "rel", EventPath: filepath.Join(dir, "events.jsonl")})
	h := newRelay(t, s)
	_, _ = h.log.Stdout().Write([]byte("no newline at the end"))
	state := func() TickFacts { return TickFacts{State: "running"} }
	if err := h.log.Finish(RunOutcome{Exit: 137}, state); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.out.String(), "out~ no newline at the end") {
		t.Fatalf("the pending line was not flushed as unterminated: %q", h.out.String())
	}
	if !strings.Contains(h.err.String(), "128+9, which is SIGKILL") || !strings.Contains(h.err.String(), "did not send it") {
		t.Fatalf("137 was not read out: %q", h.err.String())
	}
	runs, err := ReadEventRuns(s.EventPath)
	if err != nil {
		t.Fatal(err)
	}
	last := runs[0].Events[len(runs[0].Events)-1]
	if last.Kind != "EXIT" || last.ExitCode == nil || *last.ExitCode != 137 || last.TimedOut == nil || *last.TimedOut {
		t.Fatalf("the last record is %+v", last)
	}
	if DiagnoseExit(0, "running") != "" || !strings.Contains(DiagnoseExit(3, "running"), "passed through unchanged") ||
		!strings.Contains(DiagnoseExit(124, "running"), "did not fire") {
		t.Error("a diagnosis invented something about 0, 3 or 124")
	}
	h = newRelay(t, settingsFor(t, LogRequest{Mode: "rel"}))
	_ = h.log.Finish(RunOutcome{Exit: 124, TimedOut: true, Timeout: time.Minute}, nil)
	if !strings.Contains(h.err.String(), "TIMED OUT") || strings.Contains(h.err.String(), "did not fire") {
		t.Fatalf("a deadline is said as the deadline: %q", h.err.String())
	}
}

func TestASinkForACommandThatNeverStartsRemovesTheFileItCreatedAndTruncatesNothing(t *testing.T) {
	dir := t.TempDir()
	kept := filepath.Join(dir, "kept.log")
	if err := os.WriteFile(kept, []byte("the previous run\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	created := filepath.Join(dir, "new", "events.jsonl")
	log, err := OpenRunLog(LogSettings{TextPath: kept, TextOverwrite: true, EventPath: created, Separator: " "}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Finish(RunOutcome{Exit: 2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Errorf("a sink created for a command that never started was left behind: %v", err)
	}
	if got := mustRead(t, kept); got != "the previous run\n" {
		t.Errorf("a replaced log was truncated although its command never started: %q", got)
	}
}

func TestASinkOpenFailureRemovesOnlyTheDirectoriesItCreated(t *testing.T) {
	root := t.TempDir()
	block := filepath.Join(root, "block")
	if err := os.WriteFile(block, []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	text := filepath.Join(root, "new", "deep", "run.log")
	events := filepath.Join(block, "events.jsonl")
	if _, err := OpenRunLog(LogSettings{TextPath: text, EventPath: events}, nil, nil); err == nil {
		t.Fatal("two sinks were opened although the second sink's parent is a file")
	}
	if _, err := os.Stat(text); !os.IsNotExist(err) {
		t.Errorf("the first sink was left behind: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "new")); !os.IsNotExist(err) {
		t.Errorf("the empty directory made for the first sink was left behind: %v", err)
	}
	if got := mustRead(t, block); got != "kept" {
		t.Errorf("the pre-existing blocker changed: %q", got)
	}
}

func TestASinkPathNamingAWindowsDeviceIsRefused(t *testing.T) {
	for path, device := range map[string]string{"nul": "NUL", `logs\CON.jsonl`: "CON", "logs/PRN.txt": "PRN", "AUX.log.1": "AUX", "Lpt3.log": "LPT3"} {
		if err := AssertSinkPath("--event-log", path); err == nil || !strings.Contains(err.Error(), "device "+device) {
			t.Errorf("%q was not refused naming %s: %v", path, device, err)
		}
	}
	for _, ok := range []string{`logs\run.jsonl`, "console.log", "", "com10.log"} {
		if err := AssertSinkPath("--event-log", ok); err != nil {
			t.Errorf("%q was refused: %v", ok, err)
		}
	}
}

func writeEvents(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func rec(seq int, kind string, rel float64, extra string) string {
	b, _ := json.Marshal(map[string]any{"schema": EventLogSchema, "seq": seq, "t_rel": rel, "t_wall": "2026-01-01T00:00:00Z", "kind": kind, "prov": "obs", "distro": "eph-x"})
	s := string(b)
	if extra != "" {
		s = s[:len(s)-1] + "," + extra + "}"
	}
	return s
}

func TestAnEventLogSplitsIntoRunsAndRefusesAGap(t *testing.T) {
	path := writeEvents(t,
		rec(1, "LOG", 0.5, `"stream":"stdout","text":"a","partial":false`),
		rec(2, "EXIT", 1, `"exit_code":0,"timed_out":false`),
		"",
		rec(1, "LOG", 0.2, `"stream":"stderr","text":"b","partial":false`),
		rec(2, "EXIT", 3, `"exit_code":7,"timed_out":false`),
	)
	runs, err := ReadEventRuns(path)
	if err != nil || len(runs) != 2 || runs[1].Index != 2 || len(runs[1].Events) != 2 {
		t.Fatalf("runs = %+v, %v", runs, err)
	}
	if last, _ := SelectRun(runs, 0); last.Index != 2 {
		t.Errorf("run 0 selected %d, want the last", last.Index)
	}
	if _, err := SelectRun(runs, 3); err == nil {
		t.Error("a run that does not exist was selected")
	}
	for label, lines := range map[string][]string{
		"a gap":           {rec(1, "LOG", 0, ""), rec(3, "EXIT", 1, "")},
		"no first record": {rec(2, "LOG", 0, "")},
		"another schema":  {strings.Replace(rec(1, "LOG", 0, ""), EventLogSchema, "wsl-toolkit-event/2", 1)},
		"not JSON":        {"{"},
		"nothing in it":   {""},
	} {
		if _, err := ReadEventRuns(writeEvents(t, lines...)); err == nil {
			t.Errorf("%s was read", label)
		}
	}
}

func TestARunSummaryMeasuresSilenceIncludingTheTailAndSaysWhatIsUnknown(t *testing.T) {
	path := writeEvents(t,
		rec(1, "TICK", 0.5, `"stream":"watcher","text":"quiet","partial":false`),
		rec(2, "LOG", 1, `"stream":"stdout","text":"a","partial":false`),
		rec(3, "LOG", 6, `"stream":"stderr","text":"oops!","partial":false`),
		rec(4, "LOG", 7, `"stream":"stdout","text":"b","partial":false`),
		rec(5, "LINE", 8, `"stream":"out","text":"spelled the wrong way"`),
		rec(6, "EXIT", 27, `"exit_code":37,"timed_out":true`),
	)
	runs, err := ReadEventRuns(path)
	if err != nil {
		t.Fatal(err)
	}
	s := SummarizeRun(path, runs[0], 1)
	if s.FirstOutput == nil || *s.FirstOutput != 1 {
		t.Errorf("first output = %v, want the first LOG and not the first record", s.FirstOutput)
	}
	if s.LongestSilence != 20 || s.LongestSilenceEnds != 27 {
		t.Errorf("longest silence %v ending %v, want the 20s tail", s.LongestSilence, s.LongestSilenceEnds)
	}
	if s.StdoutLines != 2 || s.StdoutBytes != 2 || s.StderrLines != 1 || s.StderrBytes != 5 {
		t.Errorf("counts %+v", s)
	}
	if s.ExitCode == nil || *s.ExitCode != 37 || s.TimedOut == nil || !*s.TimedOut {
		t.Errorf("exit %v timed out %v", s.ExitCode, s.TimedOut)
	}
	tick := Event{Kind: "TICK", TRel: 4}
	empty := SummarizeRun(path, EventRun{Index: 1, Events: []Event{tick}}, 1)
	if empty.FirstOutput != nil || empty.ExitCode != nil {
		t.Errorf("a run with no output and no exit reported values nobody measured: %+v", empty)
	}
}

func TestReplayUsesTheRecordedWallClockAndRendersEveryRun(t *testing.T) {
	path := writeEvents(t,
		strings.Replace(rec(1, "LOG", 1, `"stream":"stdout","text":"token=hunter2","partial":false`), "2026-01-01T00:00:00Z", "2020-02-03T04:05:06Z", 1),
		rec(2, "TICK", 3, `"stream":"watcher","text":"quiet","partial":false`),
		rec(3, "LOG", 6, `"stream":"stdout","text":"two","partial":true`),
		rec(1, "LOG", 0, `"stream":"stderr","text":"second run","partial":false`),
	)
	runs, err := ReadEventRuns(path)
	if err != nil {
		t.Fatal(err)
	}
	s := settingsFor(t, LogRequest{Columns: []string{"wall,delta"}, Redact: []string{"hunter2"}})
	var out, errw bytes.Buffer
	n, err := Replay(runs, s, &out, &errw)
	if err != nil || n != 4 {
		t.Fatalf("rendered %d, %v", n, err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if lines[0] != "2020-02-03 04:05:06 +1.000 out  token=***" {
		t.Errorf("first line = %q", lines[0])
	}
	if lines[1] != "2026-01-01 00:00:00 +5.000 out~ two" {
		t.Errorf("a replayed tick advanced the delta, or the partial mark was lost: %q", lines[1])
	}
	if !strings.Contains(errw.String(), "==> run 2 of 2") || !strings.Contains(errw.String(), "err  second run") {
		t.Errorf("the second run is not rendered as one: %q", errw.String())
	}
}

// TestALogTheRetiredScriptRecordedStillReadsReplaysAndSummarises reads a log the
// PowerShell product wrote on 2026-09-13 against a real Alpine distribution, with
// rel,delta columns, a one-second tick, redaction, an unterminated line and exit
// 3. Only its wall times were rewritten to UTC.
//
// ⛔ THE SCHEMA ID IS A PROMISE. A reader that answered a different field name
// under the same id would summarise a real run as having no exit at all.
func TestALogTheRetiredScriptRecordedStillReadsReplaysAndSummarises(t *testing.T) {
	path := filepath.Join("testdata", "retired-script-events.jsonl")
	runs, err := ReadEventRuns(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || len(runs[0].Events) != 10 {
		t.Fatalf("runs = %d, records = %d", len(runs), len(runs[0].Events))
	}
	s := SummarizeRun(path, runs[0], 1)
	if s.ExitCode == nil || *s.ExitCode != 3 || s.TimedOut == nil || *s.TimedOut || s.StdoutLines != 4 || s.StderrLines != 1 {
		t.Fatalf("summary = %+v", s)
	}
	var out, errw bytes.Buffer
	if _, err := Replay(runs, settingsFor(t, LogRequest{Columns: []string{"rel,delta"}}), &out, &errw); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"00:00:00.481 +0.481 out  first-line", "00:00:02.811 +2.330 out~ no newline then ", "00:00:03.459 +0.003 out  token=***"} {
		if !strings.Contains(out.String(), want+"\n") {
			t.Errorf("the replay does not render %q:\n%s", want, out.String())
		}
	}
	if !strings.Contains(errw.String(), "err  to-stderr") || !strings.Contains(errw.String(), "tick 1s silent") {
		t.Errorf("stderr, the tick or the note is missing:\n%s", errw.String())
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
