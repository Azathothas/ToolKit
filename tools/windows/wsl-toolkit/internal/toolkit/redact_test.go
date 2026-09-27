// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// driveRelaySinks writes one job's output through the wiring Runner.Run uses,
// then reads back every copy a caller can see: the answer's two fields and the
// two transcript files.
func driveRelaySinks(t *testing.T, s LogSettings, out, errText []string, pollAfterFlush bool) (JobResult, string, string) {
	t.Helper()
	home := t.TempDir()
	const id = "0123456789abcdef"
	streams := newJobStreams(home, id, nil, nil, 0, nil)
	log, err := OpenRunLog(s, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{}
	clock.at(0)
	log.now, log.manual = clock.now, true
	log.Begin("wtk-"+id, nil)
	outSink, errSink := relaySinks(log, streams)
	for _, p := range out {
		if _, err := io.WriteString(outSink, p); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range errText {
		if _, err := io.WriteString(errSink, p); err != nil {
			t.Fatal(err)
		}
	}
	log.FlushCopies()
	if pollAfterFlush {
		// ⚠ The poll can flush the same tail early before the copies close. It
		// is driven here so a second copy of the tail shows in the transcript.
		clock.at(StreamFlushAfter + 1)
		log.check()
	}
	streams.Close()
	var res JobResult
	streams.Apply(&res)
	if err := log.Finish(RunOutcome{}); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(home, "jobs", id, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	return res, read("stdout.log"), read("stderr.log")
}

// TestARedactionReachesEveryCopyOfAJobsOutput is WSL-101: the live stream was
// redacted while the answer and the transcript kept the secret.
func TestARedactionReachesEveryCopyOfAJobsOutput(t *testing.T) {
	s := settingsFor(t, LogRequest{Redact: []string{"SECRET[0-9]+"}})
	out := []string{"token=SECRET123\n", "crlf SECRET1\r\n", "redraw SECRET2\rdone\n", "tail SECRET3"}
	errText := []string{"err-SECRET456\n"}
	res, stdoutLog, stderrLog := driveRelaySinks(t, s, out, errText, false)

	for where, got := range map[string]string{
		"the answer's stdout": res.Stdout, "the answer's stderr": res.Stderr,
		"stdout.log": stdoutLog, "stderr.log": stderrLog,
	} {
		if strings.Contains(got, "SECRET") {
			t.Errorf("%s holds a redacted secret: %q", where, got)
		}
	}
	// ⭐ Every line keeps its own ending, and the tail with no newline stays
	// without one: the copy differs from the command's bytes only where a
	// redaction matched.
	const wantOut = "token=***\ncrlf ***\r\nredraw ***\rdone\ntail ***"
	if res.Stdout != wantOut || stdoutLog != wantOut {
		t.Errorf("stdout copies = %q and %q, want %q", res.Stdout, stdoutLog, wantOut)
	}
	if res.Stderr != "err-***\n" || stderrLog != "err-***\n" {
		t.Errorf("stderr copies = %q and %q", res.Stderr, stderrLog)
	}
	// The counts are of what the command WROTE, which is the field's meaning.
	if want := int64(len(strings.Join(out, ""))); res.StdoutBytes != want {
		t.Errorf("stdout_bytes = %d, want %d, the command's own count", res.StdoutBytes, want)
	}
	if want := int64(len(strings.Join(errText, ""))); res.StderrBytes != want {
		t.Errorf("stderr_bytes = %d, want %d", res.StderrBytes, want)
	}
}

// TestATailIsCopiedOnceWhenThePollFlushesItAgain holds the guard that keeps a
// tail already in the copy from being copied a second time.
func TestATailIsCopiedOnceWhenThePollFlushesItAgain(t *testing.T) {
	s := settingsFor(t, LogRequest{Redact: []string{"SECRET[0-9]+"}})
	res, stdoutLog, _ := driveRelaySinks(t, s, []string{"line\n", "tail SECRET3"}, nil, true)
	const want = "line\ntail ***"
	if res.Stdout != want || stdoutLog != want {
		t.Errorf("stdout copies = %q and %q, want the tail once: %q", res.Stdout, stdoutLog, want)
	}
}

// TestWithoutARedactionTheCopiesKeepTheBytesExactly holds the other direction:
// a relay that renders a prefix and redacts nothing leaves the copies raw.
func TestWithoutARedactionTheCopiesKeepTheBytesExactly(t *testing.T) {
	s := settingsFor(t, LogRequest{Mode: "rel"})
	out := []string{"token=SECRET123\n", "crlf\r\n", "redraw\rdone\n", "tail"}
	res, stdoutLog, _ := driveRelaySinks(t, s, out, nil, false)
	want := strings.Join(out, "")
	if res.Stdout != want || stdoutLog != want {
		t.Errorf("stdout copies = %q and %q, want the bytes exactly: %q", res.Stdout, stdoutLog, want)
	}
}
