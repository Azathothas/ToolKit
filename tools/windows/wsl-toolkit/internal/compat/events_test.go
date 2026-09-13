// SPDX-License-Identifier: 0BSD

package compat

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeLog writes an event log the way the relay writes it, so the reader is
// tested against the writer's own schema.
func writeLog(t *testing.T, records ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "run.jsonl")
	body := strings.Join(records, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func logLine(seq int, kind string, pairs ...string) string {
	line := fmt.Sprintf(`{"schema":"wsl-toolkit-event/1","seq":%d,"t_rel":%s,"t_wall":"2026-09-01T10:00:00Z","kind":%q,"prov":"obs","distro":"eph-x-1a2b"`, seq, fmt.Sprintf("%g", float64(seq)), kind)
	for i := 0; i+1 < len(pairs); i += 2 {
		// Both halves arrive with their own JSON quoting, which is what makes
		// a bool's `false` and a string's quotes survive this helper.
		line += "," + pairs[i] + ":" + pairs[i+1]
	}
	return line + "}"
}

func TestTheReaderAndTheWriterAgreeOnOneSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.jsonl")
	sink, err := newEventSink(path, "eph-x-1a2b")
	if err != nil {
		t.Fatal(err)
	}
	nowD := dur(0)
	sink.writeRecord("LOG", nowD, "obs", "stdout", "hello", false, nil)
	sink.writeRecord("TICK", nowD+time.Second, "obs", "watcher", "1s silent", false, map[string]any{"silence_s": 1.0})
	sink.writeRecord("EXIT", nowD+2*time.Second, "obs", "", "", false, map[string]any{"exit_code": 3, "timed_out": false})
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	records, err := readEventLogFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("read %d record(s), want 3", len(records))
	}
	if records[0].Kind != "LOG" || records[0].Stream != "stdout" || records[0].Text != "hello" || records[0].Partial {
		t.Errorf("the LOG record mis-read: %+v", records[0])
	}
	if records[1].Seq != 2 || records[1].Kind != "TICK" {
		t.Errorf("the TICK record mis-read: %+v", records[1])
	}
	summary := eventLogSummary(records)
	if summary.ExitCode == nil || *summary.ExitCode != 3 {
		t.Errorf("the EXIT record mis-read: %+v", summary)
	}
}

func TestAGapInSeqIsReportedNeverSmoothedOver(t *testing.T) {
	// A gap means records were dropped: a finding about the RECORDING, and a
	// reader that renumbered would draw conclusions about a run it was not
	// shown.
	path := writeLog(t,
		logLine(1, "LOG", `"stream"`, `"stdout"`, `"text"`, `"a"`, `"partial"`, `false`),
		logLine(3, "LOG", `"stream"`, `"stdout"`, `"text"`, `"b"`, `"partial"`, `false`),
	)
	_, err := readEventLogFile(path)
	if err == nil || !strings.Contains(err.Error(), "seq jumps from 1 to 3") {
		t.Fatalf("a gap was accepted: %v", err)
	}
}

func TestAWrongSchemaIsRefusedByName(t *testing.T) {
	path := writeLog(t, `{"schema":"wsl-toolkit-event/9","seq":1,"t_rel":0,"kind":"LOG"}`)
	if _, err := readEventLogFile(path); err == nil || !strings.Contains(err.Error(), "wsl-toolkit-event/1") {
		t.Fatalf("a wrong schema was accepted: %v", err)
	}
	empty := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readEventLogFile(empty); err == nil || !strings.Contains(err.Error(), "holds no records") {
		t.Fatalf("an empty log was accepted: %v", err)
	}
}

func TestTheSummaryCountsTheTailAsSilence(t *testing.T) {
	// A run whose last line arrived at one second and which ended at four was
	// silent for the rest, and a summary that measured only the gaps BETWEEN
	// lines would report the quietest part as not having happened.
	records := []eventRecord{
		{Kind: "LOG", Stream: "stdout", Text: "one", TRel: 0.5},
		{Kind: "LOG", Stream: "stderr", Text: "bang", TRel: 1.0},
		{Kind: "EXIT", Data: map[string]any{"exit_code": float64(0)}, TRel: 4.0},
	}
	s := eventLogSummary(records)
	if s.OutLines != 1 || s.ErrLines != 1 {
		t.Errorf("lines mis-counted: %+v", s)
	}
	if s.FirstOutput == nil || *s.FirstOutput != 0.5 {
		t.Errorf("first output = %v", s.FirstOutput)
	}
	if s.LongestGap != 3.0 || s.LongestGapAt != 4.0 {
		t.Errorf("the tail was not counted as silence: %+v", s)
	}
	if s.ExitCode == nil || *s.ExitCode != 0 {
		t.Errorf("exit = %v", s.ExitCode)
	}
}

// -- the Replay action -----------------------------------------------------------

func replaySession(t *testing.T, from string, args ...string) (int, string, string, string) {
	t.Helper()
	o, err := Parse(append([]string{"-Action", "Replay", "-From", from}, args...))
	if err != nil {
		t.Fatal(err)
	}
	var report, notes bytes.Buffer
	s := &session{
		opts: o, baseDir: t.TempDir(),
		log: &console{report: &report, note: &notes}, out: &report, errw: &notes,
		stop: bgContext(),
	}
	s.relayOff = o.NoTimestamps || o.TimestampProfile == "raw"
	if !s.relayOff {
		settings, err := resolveStreamLogSettings(o, false)
		if err != nil {
			t.Fatal(err)
		}
		s.settings = settings
	}
	code := s.actionReplay()
	return code, report.String(), notes.String(), ""
}

func TestAReplayRendersTheRecordsOwnWallReading(t *testing.T) {
	// ⚠ A replay of last week's run stamped with today's date is a document
	// that says something false about when the work happened.
	path := writeLog(t,
		logLine(1, "LOG", `"stream"`, `"stdout"`, `"text"`, `"built 31 modules"`, `"partial"`, `false`),
	)
	code, report, _, _ := replaySession(t, path, "-TimestampMode", "Wall", "-TimestampFormat", "%Y-%m-%d %H:%M:%S")
	if code != 0 {
		t.Fatalf("replay exited %d", code)
	}
	if !strings.Contains(report, "2026-09-01 10:00:00") {
		t.Errorf("the record's own wall reading was not rendered: %q", report)
	}
	if !strings.Contains(report, "built 31 modules") {
		t.Errorf("the body was lost: %q", report)
	}
	if strings.Contains(report, time.Now().Format("2006-01-02")) && !strings.Contains(report, "2026-09-01") {
		t.Errorf("the replay stamped last week's run with today: %q", report)
	}
}

func TestAReplayWithoutTheRelayPrintsThePlainLines(t *testing.T) {
	path := writeLog(t,
		logLine(1, "LOG", `"stream"`, `"stdout"`, `"text"`, `"out line"`, `"partial"`, `false`),
		logLine(2, "LOG", `"stream"`, `"stderr"`, `"text"`, `"err line"`, `"partial"`, `false`),
		logLine(3, "NOTE", `"text"`, `"a note"`, `"partial"`, `false`),
	)
	code, report, notes, _ := replaySession(t, path, "-NoTimestamps")
	if code != 0 {
		t.Fatalf("replay exited %d", code)
	}
	// The guest's own line reaches the report stream un-prefixed, ahead of the
	// summary the action appends: the report is where the script's host put
	// Write-Output AND Write-Host for a child process.
	if !strings.HasPrefix(report, "out line\n") {
		t.Errorf("the report stream = %q, want the guest's own line first", report)
	}
	if !strings.Contains(notes, "err line") || !strings.Contains(notes, "a note") {
		t.Errorf("the watcher's lines left stderr: %q", notes)
	}
}

func TestAReplaySummaryCarriesTheReading(t *testing.T) {
	path := writeLog(t,
		logLine(1, "LOG", `"stream"`, `"stdout"`, `"text"`, `"x"`, `"partial"`, `false`),
		logLine(2, "EXIT", `"exit_code"`, `7`, `"timed_out"`, `false`),
	)
	code, report, _, _ := replaySession(t, path)
	if code != 0 {
		t.Fatalf("replay exited %d", code)
	}
	if !strings.Contains(report, "exit 7") || !strings.Contains(report, "longest silence") {
		t.Errorf("the summary did not reach the report stream: %q", report)
	}
}

// -- Compare -----------------------------------------------------------------------

func TestComparePutsTwoRunsSideBySideAndJudgesNothing(t *testing.T) {
	a := writeLog(t,
		logLine(1, "LOG", `"stream"`, `"stdout"`, `"text"`, `"x"`, `"partial"`, `false`),
		logLine(2, "EXIT", `"exit_code"`, `0`, `"timed_out"`, `false`),
	)
	b := writeLog(t,
		logLine(1, "LOG", `"stream"`, `"stdout"`, `"text"`, `"x"`, `"partial"`, `false`),
		logLine(2, "EXIT", `"exit_code"`, `0`, `"timed_out"`, `false`),
	)
	o, err := Parse([]string{"-Action", "Compare", "-From", a, "-Against", b})
	if err != nil {
		t.Fatal(err)
	}
	var report, notes bytes.Buffer
	s := &session{opts: o, baseDir: t.TempDir(), log: &console{report: &report, note: &notes}, out: &report, errw: &notes, stop: bgContext()}
	if code := s.actionCompare(); code != 0 {
		t.Fatalf("compare exited %d", code)
	}
	body := notes.String()
	if !strings.Contains(body, "B - A") || !strings.Contains(body, "exit code") {
		t.Errorf("the table is missing its columns: %q", body)
	}
	// ⛔ IT REPORTS AND DOES NOT JUDGE: no threshold calls a difference a
	// regression.
	if strings.Contains(strings.ToLower(body), "regression") {
		t.Errorf("the compare invented a verdict: %q", body)
	}
}

func TestCompareRefusesAMissingHalfByName(t *testing.T) {
	a := writeLog(t, logLine(1, "EXIT", `"exit_code"`, `0`, `"timed_out"`, `false`))
	o, err := Parse([]string{"-Action", "Compare", "-From", a})
	if err != nil {
		t.Fatal(err)
	}
	var report, notes bytes.Buffer
	s := &session{opts: o, baseDir: t.TempDir(), log: &console{report: &report, note: &notes}, out: &report, errw: &notes, stop: bgContext()}
	if code := s.actionCompare(); code != 1 {
		t.Fatalf("a compare with no -Against exited %d", code)
	}
}
