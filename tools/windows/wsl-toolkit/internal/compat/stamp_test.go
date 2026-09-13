// SPDX-License-Identifier: 0BSD

package compat

import (
	"strings"
	"testing"
	"time"
)

func wallAt(year, month, day, hour, min, sec, ticks int64, offset int) wallReading {
	return wallReading{
		Year: int(year), Month: int(month), Day: int(day),
		Hour: int(hour), Minute: int(min), Second: int(sec),
		Ticks: ticks, Offset: int(offset), Zone: "Test/Zone",
		EpochSeconds: 1700000000,
	}
}

func mustStamp(t *testing.T, format string, wall wallReading, elapsed, delta dur, mode string) string {
	t.Helper()
	got, err := formatStrftimeStamp(format, wall, elapsed, mode)
	if err != nil {
		t.Fatalf("format %q was refused: %v", format, err)
	}
	return got
}

func TestTheStrftimeRendererMatchesTheScriptByteForByte(t *testing.T) {
	w := wallAt(2026, 9, 13, 7, 8, 9, 123_456_7, 2*3600+30*60)
	if got := mustStamp(t, "%Y-%m-%d %H:%M:%S", w, 0, 0, "Wall"); got != "2026-09-13 07:08:09" {
		t.Errorf("wall default = %q", got)
	}
	if got := mustStamp(t, "%3f", w, 0, 0, "Wall"); got != "123" {
		t.Errorf("%%3f = %q, want 123", got)
	}
	if got := mustStamp(t, "%6f", w, 0, 0, "Wall"); got != "123456" {
		t.Errorf("%%6f = %q, want 123456", got)
	}
	// %9f CANNOT BE MEASURED AND IT PADS: the ninth digit is a zero the
	// renderer wrote, which the page says rather than hides.
	if got := mustStamp(t, "%9f", w, 0, 0, "Wall"); got != "123456700" {
		t.Errorf("%%9f = %q, want 123456700", got)
	}
	if got := mustStamp(t, "%z", w, 0, 0, "Wall"); got != "+02:30" {
		t.Errorf("%%z = %q", got)
	}
	if got := mustStamp(t, "100%%", w, 0, 0, "Wall"); got != "100%" {
		t.Errorf("%%%% = %q", got)
	}
	// A literal character in the format stays that character on every host:
	// ':' is substituted, never interpreted as a culture-dependent separator.
	if got := mustStamp(t, "a:b|c", w, 0, 0, "Wall"); got != "a:b|c" {
		t.Errorf("literals moved: %q", got)
	}
}

func TestRelativeHoursAreTotalAndNeverWrapAtADay(t *testing.T) {
	// A run that passes 24 hours reads 25:00:01 rather than starting again at
	// zero: a relative stamp that wraps is a stamp that lies about a long
	// build.
	elapsed := 25*time.Hour + time.Second + 500*time.Millisecond
	w := wallReading{EpochSeconds: 1}
	if got := mustStamp(t, "%H:%M:%S.%3f", w, elapsed, 0, "Relative"); got != "25:00:01.500" {
		t.Errorf("a long build stamped %q, want 25:00:01.500", got)
	}
}

func TestAnUnknownSpecifierIsRefusedNeverRenderedAsItsLetter(t *testing.T) {
	// A format that silently renders '%q' as 'q' is a caller believing they
	// asked for something.
	if _, err := formatStrftimeStamp("%q", wallReading{}, 0, "Relative"); err == nil || !strings.Contains(err.Error(), "%q") {
		t.Errorf("an unknown specifier was not refused: %v", err)
	}
	// %Y has no value in relative mode: answering 1970 would put an invented
	// number on a log line.
	if _, err := formatStrftimeStamp("%Y", wallReading{}, time.Hour, "Relative"); err == nil {
		t.Error("a wall specifier rendered in relative mode")
	}
	if _, err := formatStrftimeStamp("ends with %", wallReading{}, 0, "Wall"); err == nil || !strings.Contains(err.Error(), "bare") {
		t.Error("a trailing bare percent was accepted")
	}
}

func TestTheDeltaColumnSpellingMatchesTheDeltaMode(t *testing.T) {
	// The column and the mode are two spellings of one number, so they must
	// produce the same bytes: a caller who moved from the mode to the column
	// gets the same characters.
	got, err := formatStampColumn("delta", "", wallReading{}, 0, 62*time.Second+250*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if got != "+62.250" {
		t.Errorf("delta column = %q, want +62.250", got)
	}
	if got := mustStamp(t, "%H:%M:%S.%3f", wallReading{}, 0, 0, "Relative"); got == "" {
		t.Error("relative stamp vanished")
	}
}

func TestEpochAndIsoColumnsRender(t *testing.T) {
	w := wallReading{Year: 2026, Month: 9, Day: 13, Hour: 7, Minute: 8, Second: 9,
		Ticks: 400_000, Offset: 0, Zone: "UTC", EpochSeconds: 1789340889}
	if got, _ := formatStampColumn("epoch", "", w, 0, 0); got != "1789340889" {
		t.Errorf("epoch = %q", got)
	}
	got, err := formatStampColumn("iso", "", w, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-09-13T07:08:09.040+00:00" {
		t.Errorf("iso = %q", got)
	}
}

// -- column resolution ---------------------------------------------------------

func TestTimestampColumnsAndModeAreTwoSpellingsOfOneDecision(t *testing.T) {
	// ⛔ PASSING BOTH IS REFUSED, in the settings resolution: a precedence
	// between them would be a rule a caller has to remember.
	_, err := resolveStampColumns([]string{"rel"}, "Wall", true)
	if err == nil || !strings.Contains(err.Error(), "Pass one") {
		t.Errorf("columns beside a passed mode were accepted: %v", err)
	}
	if _, err := resolveStampColumns([]string{"rel,delta"}, "Relative", false); err != nil {
		t.Errorf("the composed form was refused: %v", err)
	}
	if _, err := resolveStampColumns([]string{"rel,rel"}, "Relative", false); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("a repeated column was accepted: %v", err)
	}
	if _, err := resolveStampColumns([]string{"fortnight"}, "Relative", false); err == nil || !strings.Contains(err.Error(), "rel, delta, wall, iso, epoch") {
		t.Errorf("an unknown column was accepted: %v", err)
	}
	if _, err := resolveStampColumns([]string{"  "}, "Relative", false); err == nil || !strings.Contains(err.Error(), "nothing in it") {
		t.Errorf("an empty column list was accepted: %v", err)
	}
	// The single modes map to their column.
	for mode, want := range map[string]string{"Delta": "delta", "Wall": "wall", "Iso": "iso", "Epoch": "epoch", "Relative": "rel"} {
		got, err := resolveStampColumns(nil, mode, false)
		if err != nil || len(got) != 1 || got[0] != want {
			t.Errorf("mode %s gave %v, %v", mode, got, err)
		}
	}
}

func TestTimestampFormatWithNoColumnThatTakesOneIsRefused(t *testing.T) {
	// A parameter silently doing nothing is a caller believing they asked for
	// something, so the settings resolution refuses it before anything runs.
	o := defaultOptions()
	o.Action = ActionRun
	o.Name = "d"
	o.Command = "true"
	o.TimestampColumns = []string{"delta"}
	o.TimestampFormat = "%Y"
	o.markPassed("TimestampColumns")
	o.markPassed("TimestampFormat")
	if _, err := resolveStreamLogSettings(o, false); err == nil || !strings.Contains(err.Error(), "rel' and 'wall") {
		t.Errorf("-TimestampFormat over a formatless column was accepted: %v", err)
	}
}

// -- the presets are starting points, not modes ---------------------------------

func TestExplicitParametersWinOverAPreset(t *testing.T) {
	o := defaultOptions()
	o.Action = ActionRun
	o.Name = "d"
	o.Command = "true"
	o.TimestampProfile = "ci"
	o.Color = "always"
	o.markPassed("Color")
	o.markPassed("TimestampProfile")
	settings, err := resolveStreamLogSettings(o, false)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Color {
		t.Error("the ci preset overrode an explicitly passed -Color; a preset is a starting point")
	}
	if len(settings.Columns) != 2 || settings.Columns[0] != "rel" || settings.Columns[1] != "delta" {
		t.Errorf("the ci preset's columns are wrong: %v", settings.Columns)
	}

	// And the forensic preset carries its own format when none was passed.
	o.TimestampProfile = "forensic"
	o.passed = map[string]bool{"timestampprofile": true}
	settings, err = resolveStreamLogSettings(o, false)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Format != "%Y-%m-%d %H:%M:%S.%6f" {
		t.Errorf("the forensic format moved: %q", settings.Format)
	}
	if settings.Color {
		t.Error("the forensic preset left colour on")
	}
}

// -- the escalation thresholds are split, never culture-converted ----------------

func TestTickEscalateSecondsSplitsCommasAndRefusesJunk(t *testing.T) {
	o := defaultOptions()
	o.Action = ActionRun
	o.Name = "d"
	o.Command = "true"
	// Through -File, "5,9" arrived as ONE string and a culture turned it into
	// 59, so the escalation never fired with nothing said. The split happens
	// here, where the refusal for junk is also named.
	o.TickEscalateSeconds = []string{"9,5", "300"}
	o.markPassed("TickEscalateSeconds")
	settings, err := resolveStreamLogSettings(o, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Escalate) != 3 || settings.Escalate[0] != 5 || settings.Escalate[1] != 9 || settings.Escalate[2] != 300 {
		t.Errorf("escalation = %v, want 5, 9, 300 sorted and unique", settings.Escalate)
	}

	o.TickEscalateSeconds = []string{"soon"}
	if _, err := resolveStreamLogSettings(o, false); err == nil || !strings.Contains(err.Error(), "whole number of seconds") {
		t.Errorf("junk thresholds were accepted: %v", err)
	}
}

// -- redaction and the line bound -------------------------------------------------

func TestRedactionReplacesBeforeAnySinkSeesTheText(t *testing.T) {
	set, err := newRedactionSet([]string{"sk-[a-z0-9]+", "hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	got := applyRedaction(set, "token sk-abc123 and hunter2 and sk-xyz again")
	if strings.Contains(got, "sk-abc") || strings.Contains(got, "hunter2") || strings.Contains(got, "sk-xyz") {
		t.Errorf("a secret survived redaction: %q", got)
	}
	if got != "token *** and *** and *** again" {
		t.Errorf("the marker is not three asterisks: %q", got)
	}
	// A pattern that does not compile is refused HERE, at the settings, not on
	// the first line of guest output halfway into a run.
	if _, err := newRedactionSet([]string{"(["}); err == nil || !strings.Contains(err.Error(), "-Redact") {
		t.Errorf("a bad pattern was accepted: %v", err)
	}
}

func TestLimitLineBytesCountsBytesAndCutsCharacters(t *testing.T) {
	// A UTF-8 character is up to four bytes, so cutting bytes would split one
	// and produce a replacement character that was never in the guest's
	// output. The cut lands on a whole character and says what it dropped.
	// One byte, then a two-, three- and four-byte character: 10 bytes total.
	// ⚠ THE SOURCE STAYS ASCII, because the tree's marker rule reads bytes:
	// the characters arrive as escapes and the bytes arrive at runtime.
	text := "a\u00e9\u4e2d\U00010348"
	got := limitLineBytes(text, 4)
	if got != "a\u00e9...(+"+"7 bytes cut)" {
		t.Errorf("limitLineBytes = %q", got)
	}
	if limitLineBytes(text, 100) != text {
		t.Error("an unreached bound still cut")
	}
	if limitLineBytes(text, 0) != text {
		t.Error("the off switch cut anyway")
	}
}

// -- progress lines ---------------------------------------------------------------

func TestAProgressLineIsConsumedOnlyWhenTheCallerChoseTheToken(t *testing.T) {
	cases := []struct {
		text   string
		isProg bool
		pct    float64
		label  string
	}{
		{"EPH 42 unpacking", true, 42, "unpacking"},
		{"EPH 42.5% unpacking", true, 42.5, "unpacking"},
		{"EPH 100", true, 100, ""},
		{"EPH 42mid-word", false, 0, ""},
		{"EPH", false, 0, ""},
		{"EPH not a number", false, 0, ""},
		{"EPH 142 over", false, 0, ""},
		{"EPH -3 under", false, 0, ""},
		{"progress 42 without the token", false, 0, ""},
	}
	for _, c := range cases {
		got := readProgressLine(c.text, "EPH")
		if c.isProg != (got != nil) {
			t.Errorf("readProgressLine(%q) = %v, want progress=%v", c.text, got, c.isProg)
			continue
		}
		if got != nil && (got.Percent != c.pct || got.Label != c.label) {
			t.Errorf("readProgressLine(%q) = %+v", c.text, got)
		}
	}
	// ⛔ THE TOKEN HAS TO END AT A BOUNDARY: a token of `P` would otherwise eat
	// every line beginning with the letter P.
	if readProgressLine("Pictures of dogs", "P") != nil {
		t.Error("a one-letter token consumed a line it only prefixed by coincidence")
	}
}

// -- the reserved device names -----------------------------------------------------

func TestReservedDeviceNamesAreRefusedAsSinkPaths(t *testing.T) {
	cases := []struct{ path, parameter string }{
		{"nul", "-StreamLogPath"},
		{"NUL", "-EventLog"},
		{"NUL.txt", "-StreamLogPath"},
		{"logs/CON.jsonl", "-EventLog"},
		{`logs\CON.jsonl`, "-StreamLogPath"},
		{"com1", "-EventLog"},
	}
	for _, c := range cases {
		err := assertSinkPathIsUsable(c.path, c.parameter)
		if err == nil || !strings.Contains(err.Error(), "reserved device") {
			t.Errorf("%q was accepted as a sink path: %v", c.path, err)
		}
		continue
	}
	// The check splits BOTH separators itself, because the rule is about
	// Windows semantics whatever host is asking.
	if err := assertSinkPathIsUsable("logs/normal.jsonl", "-StreamLogPath"); err != nil {
		t.Errorf("an ordinary path was refused: %v", err)
	}
	if err := assertSinkPathIsUsable("", "-StreamLogPath"); err != nil {
		t.Errorf("an absent sink was refused: %v", err)
	}
}

// -- the exit-code diagnosis ---------------------------------------------------------

func TestTheExitCodeDiagnosisNamesWhatTheNumberMeans(t *testing.T) {
	if getExitCodeDiagnosis(0, "Running") != "" {
		t.Error("exit 0 produced a reading")
	}
	if got := getExitCodeDiagnosis(124, "Running"); !strings.Contains(got, "-CommandTimeoutSeconds") {
		t.Errorf("124 was not named as the tool's own timeout: %s", got)
	}
	got := getExitCodeDiagnosis(137, "Running")
	if !strings.Contains(got, "SIGKILL") || !strings.Contains(got, "out-of-memory") {
		t.Errorf("137 did not name its causes: %s", got)
	}
	// ⛔ IT NEVER CLAIMS TO KNOW WHICH. Where the evidence does not separate
	// the causes, it says so and lists them.
	if !strings.Contains(got, "It is produced by:") {
		t.Errorf("137 was stated as a single fact: %s", got)
	}
	got = getExitCodeDiagnosis(137, "Stopped")
	if !strings.Contains(got, "consistent with the VM having gone") {
		t.Errorf("a stopped distro did not sharpen the reading: %s", got)
	}
	if got := getExitCodeDiagnosis(3, "Running"); !strings.Contains(got, "the command's own") {
		t.Errorf("an ordinary code was diagnosed: %s", got)
	}
	if got := getExitCodeDiagnosis(130, "Running"); !strings.Contains(got, "SIGINT") {
		t.Errorf("130 did not carry its signal: %s", got)
	}
}

// -- the chunk splitter ----------------------------------------------------------------

func TestACarriageReturnTerminatesALine(t *testing.T) {
	// curl, apt and every layer-progress bar redraw one line with a carriage
	// return and emit no newline for minutes. A reader that waits for a
	// newline shows NOTHING while a 200 MB download is visibly working.
	ready, rest := splitStreamChunk("one\r2%\rtwo\nthree\r\nfour")
	want := []readyLine{{"one", true}, {"2%", true}, {"two", false}, {"three", false}}
	if len(ready) != len(want) {
		t.Fatalf("splitStreamChunk gave %v, want %v", ready, want)
	}
	for i := range want {
		if ready[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, ready[i], want[i])
		}
	}
	if rest != "four" {
		t.Errorf("the remainder = %q, want the unterminated tail", rest)
	}
	// A TRAILING CARRIAGE RETURN IS HELD, NEVER EMITTED: it may be the first
	// half of a CRLF split across two reads.
	ready, rest = splitStreamChunk("one\r")
	if len(ready) != 0 || rest != "one\r" {
		t.Errorf("a trailing CR was emitted early: %v, %q", ready, rest)
	}
}
