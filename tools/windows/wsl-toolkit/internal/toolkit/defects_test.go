// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// ⛔ EVERY TEST HERE ANSWERS A DEFECT A CONSUMER FOUND IN A PUBLISHED BINARY,
// against a tree whose own gate was green and whose acceptance runner passed 23
// of 23. Each one is written to fail if the fix is removed, and several cover the
// adjacent case nobody had exercised rather than only the reproduction that was
// filed.

// -- WSL-34, issue 9: names that collide on the destination ------------------

func TestSafeArchiveNameRefusesWindowsGrammar(t *testing.T) {
	refused := []struct {
		name string
		why  string
	}{
		{"normal.txt:stream", "an NTFS alternate data stream: the file is created empty and the payload goes into a stream nothing reads"},
		{"dir/inner.txt:s", "a stream named below the first component"},
		{"a:b:c", "more than one colon"},
		{"report.", "a trailing dot, which the destination strips"},
		{"report ", "a trailing space, which the destination strips"},
		{"dir./file", "a trailing dot on a directory component"},
		{"a<b", "a character no name on the destination may carry"},
		{"a>b", "a character no name on the destination may carry"},
		{`a"b`, "a character no name on the destination may carry"},
		{"a|b", "a character no name on the destination may carry"},
		{"a?b", "a character no name on the destination may carry"},
		{"a*b", "a character no name on the destination may carry"},
		{"a\x01b", "a control character"},
		{"CON", "a device name"},
		{"logs/CON.jsonl", "a device name below a directory"},
		{`logs\CON.jsonl`, "a device name behind the other separator"},
		{"../escape", "a climb out of the destination"},
		{"/etc/passwd", "an absolute path"},
		{`C:\Windows`, "a drive"},
		{"NUL.d/keep.txt", "a device name is reserved WITH ANY EXTENSION, so a directory called NUL.d is one too"},
	}
	for _, c := range refused {
		got, err := SafeArchiveName(c.name)
		if err == nil {
			t.Errorf("SafeArchiveName(%q) = %q, want a refusal: %s", c.name, got, c.why)
			continue
		}
		if !errors.Is(err, ErrWorkspaceRefused) {
			t.Errorf("SafeArchiveName(%q) refused with %v, which does not wrap ErrWorkspaceRefused", c.name, err)
		}
	}

	// ⚠ The rule must not widen past what it is for. These are ordinary names a
	// build produces, and a guard that refused them would be a guard nobody can
	// ship.
	for _, ok := range []string{
		"result.txt", "dir/result.txt", "a.b.c", "CONTROL.md", "conf.d/x",
		"spaces in the middle.txt", "-leading-dash", "nullable.txt", "console.log", "aux-data/report.json",
		"UPPER.TXT", "x86_64-unknown-linux-gnu.tar.gz",
	} {
		if _, err := SafeArchiveName(ok); err != nil {
			t.Errorf("SafeArchiveName(%q) refused a legitimate name: %v", ok, err)
		}
	}
}

func TestExtractRefusesCaseCollision(t *testing.T) {
	dest := t.TempDir()
	archive := tarOf(t, map[string]string{"Result": "upper", "result": "lower"})
	_, _, err := extractInto(bytes.NewReader(archive), dest, DefaultWorkspaceLimits())
	if err == nil {
		t.Fatal("two names that differ only in case were accepted; on the destination they are one file and the second silently replaces the first")
	}
	if !errors.Is(err, ErrWorkspaceRefused) {
		t.Fatalf("refused with %v, which does not wrap ErrWorkspaceRefused", err)
	}
	for _, want := range []string{"Result", "result"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q, so a caller cannot tell which two entries collided: %v", want, err)
		}
	}
}

func TestExtractAcceptsDistinctNames(t *testing.T) {
	dest := t.TempDir()
	archive := tarOf(t, map[string]string{"one.txt": "a", "two.txt": "bb", "d/three.txt": "ccc"})
	n, _, err := extractInto(bytes.NewReader(archive), dest, DefaultWorkspaceLimits())
	if err != nil {
		t.Fatalf("three distinct names were refused: %v", err)
	}
	if n != 3 {
		t.Fatalf("extracted %d entries, want 3", n)
	}
	for name, want := range map[string]string{"one.txt": "a", "two.txt": "bb", filepath.Join("d", "three.txt"): "ccc"} {
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s holds %q, want %q", name, got, want)
		}
	}
}

func TestDestinationKeyFoldsCase(t *testing.T) {
	if destinationKey("A/B.txt") != destinationKey("a/b.txt") {
		t.Fatal("two paths differing only in case produce different keys, so a collision would not be seen")
	}
	if destinationKey("a/b.txt") == destinationKey("a/c.txt") {
		t.Fatal("two different paths produce one key, so a legitimate pair would be refused")
	}
}

// -- WSL-33, issue 8: a transfer that failed is not a job that passed ---------

func TestFailedReadsTheTransfer(t *testing.T) {
	cases := []struct {
		name string
		res  JobResult
		want bool
	}{
		{"a clean run", JobResult{Exit: 0}, false},
		{"the command failed", JobResult{Exit: 7}, true},
		{"the deadline passed", JobResult{Exit: 124, TimedOut: true}, true},
		{"the command passed and its output did not arrive", JobResult{Exit: 0, ArtifactError: "refused"}, true},
		{"both", JobResult{Exit: 7, ArtifactError: "refused"}, true},
	}
	for _, c := range cases {
		if got := c.res.Failed(); got != c.want {
			t.Errorf("%s: Failed() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMatrixCountsAFailedTransfer(t *testing.T) {
	report := MatrixReport{Rows: []JobResult{
		{Label: "alpine", Exit: 0},
		{Label: "debian12", Exit: 0, ArtifactError: "artifacts: workspace refused"},
		{Label: "fedora", Unreached: true},
		{Label: "arch", Exit: 124, TimedOut: true},
	}}
	report.Recount()
	if report.Ran != 3 {
		t.Errorf("ran = %d, want 3", report.Ran)
	}
	if report.Failed != 2 {
		t.Errorf("failed = %d, want 2: the row whose artifacts did not arrive delivered nothing", report.Failed)
	}
	if report.Unreached != 1 {
		t.Errorf("unreached = %d, want 1", report.Unreached)
	}
	if report.TimedOut != 1 {
		t.Errorf("timed_out = %d, want 1", report.TimedOut)
	}
	if report.Verdict() == 0 {
		t.Error("a fleet with a failed row reported success")
	}
}

func TestRecountIsIdempotent(t *testing.T) {
	report := MatrixReport{Rows: []JobResult{{Exit: 0}, {Exit: 1}}}
	report.Recount()
	first := report
	report.Recount()
	if report.Ran != first.Ran || report.Failed != first.Failed {
		t.Fatal("counting twice gives a different answer, so a caller that adjusts a row cannot safely recount")
	}
}

// -- WSL-39, issue 14: an image that was never pulled did not run -------------

func TestMarkerStripperFindsASplitToken(t *testing.T) {
	token := "wtk-started-0123456789abcdef"
	full := "before\n\n" + token + "\nafter\n"
	// Every split point, because the token can arrive across any two writes and
	// a filter that only looked at one write would pass half of it through.
	for cut := 0; cut <= len(full); cut++ {
		var got bytes.Buffer
		m := newMarkerStripper(&got, token)
		if _, err := m.Write([]byte(full[:cut])); err != nil {
			t.Fatalf("cut %d: %v", cut, err)
		}
		if _, err := m.Write([]byte(full[cut:])); err != nil {
			t.Fatalf("cut %d: %v", cut, err)
		}
		if err := m.Flush(); err != nil {
			t.Fatalf("cut %d: %v", cut, err)
		}
		if !m.Seen() {
			t.Fatalf("cut %d: the marker was not seen, so a job that ran would be reported as unreached", cut)
		}
		if want := "before\n\nafter\n"; got.String() != want {
			t.Fatalf("cut %d: stream is %q, want %q", cut, got.String(), want)
		}
	}
}

func TestMarkerStripperPassesOutputThatHasNoToken(t *testing.T) {
	token := "wtk-started-deadbeefdeadbeef"
	payload := "line one\nline two\nno newline at the end"
	var got bytes.Buffer
	m := newMarkerStripper(&got, token)
	for _, b := range []byte(payload) {
		if _, err := m.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Flush(); err != nil {
		t.Fatal(err)
	}
	if m.Seen() {
		t.Fatal("a stream with no marker reported one, so a job that never started would read as one that ran")
	}
	if got.String() != payload {
		t.Fatalf("byte-at-a-time writes produced %q, want %q", got.String(), payload)
	}
}

func TestNewMarkerTokenIsPerJob(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		tok, err := newMarkerToken()
		if err != nil {
			t.Fatal(err)
		}
		if seen[tok] {
			t.Fatal("two jobs got the same marker, so one payload could forge another's")
		}
		if !strings.HasPrefix(tok, "wtk-started-") || len(tok) != len("wtk-started-")+32 {
			t.Fatalf("token %q is not the shape the wrapper writes", tok)
		}
		seen[tok] = true
	}
}

func TestPartialSuffix(t *testing.T) {
	tok := []byte("abcd")
	cases := map[string]int{"": 0, "x": 0, "a": 1, "xab": 2, "abc": 3, "abcd": 0, "xxabc": 3, "aab": 2}
	for in, want := range cases {
		if got := partialSuffix([]byte(in), tok); got != want {
			t.Errorf("partialSuffix(%q, %q) = %d, want %d", in, tok, got, want)
		}
	}
}

// -- WSL-36, issue 11: cleanup does not kill live work ----------------------

func TestCleanupPlanSparesLiveWork(t *testing.T) {
	now := time.Now()
	targets := []CleanupTarget{
		{Kind: "container", Name: "wtk-live (aaa)", JobID: "live", Live: true, ModTime: now.Add(-30 * time.Second)},
		{Kind: "container", Name: "wtk-old (bbb)", JobID: "old", ModTime: now.Add(-48 * time.Hour)},
		{Kind: "guest-dir", Name: "/home/toolkit/.wsl-toolkit/jobs/live", JobID: "live", Live: true, ModTime: now.Add(-30 * time.Second)},
		{Kind: "guest-dir", Name: "/home/toolkit/.wsl-toolkit/jobs/old", JobID: "old", ModTime: now.Add(-48 * time.Hour)},
		{Kind: "container", Name: "wtk-orphan (ccc)", JobID: "orphan"}, // no directory, so no age
	}

	policy := CleanupPolicy{OlderThan: 24 * time.Hour}
	remove, spared := policy.Select(targets, now)
	if names := strings.Join(Names(remove), " "); strings.Contains(names, "wtk-live") {
		t.Fatalf("a container running right now is in the removal set: %s", names)
	}
	if len(spared) != 2 {
		t.Fatalf("spared %d, want 2 (the live container and its directory): %v", len(spared), spared)
	}
	for _, want := range []string{"wtk-old (bbb)", "/home/toolkit/.wsl-toolkit/jobs/old", "wtk-orphan (ccc)"} {
		if !contains(Names(remove), want) {
			t.Errorf("%q is not in the removal set, so an abandoned resource would never be collected", want)
		}
	}

	// ⛔ The dry run and the apply consume this one slice, so what is listed is
	// what is removed. Assert the split adds up rather than trusting it.
	if len(remove)+len(spared) != len(targets) {
		t.Fatalf("%d targets became %d removed plus %d spared", len(targets), len(remove), len(spared))
	}
}

func TestCleanupPlanIncludeLive(t *testing.T) {
	now := time.Now()
	targets := []CleanupTarget{
		{Kind: "container", Name: "wtk-live (aaa)", Live: true, ModTime: now.Add(-30 * time.Second)},
	}
	remove, spared := CleanupPolicy{IncludeLive: true}.Select(targets, now)
	if len(remove) != 1 || len(spared) != 0 {
		t.Fatalf("--include-live removed %d and spared %d, want 1 and 0", len(remove), len(spared))
	}
	remove, spared = CleanupPolicy{}.Select(targets, now)
	if len(remove) != 0 || len(spared) != 1 {
		t.Fatalf("without --include-live it removed %d and spared %d, want 0 and 1", len(remove), len(spared))
	}
}

func TestCleanupPlanAgeBoundary(t *testing.T) {
	now := time.Now()
	// Exactly at the boundary is OLD ENOUGH. The rule is stated as "untouched for
	// at least this long", so the comparison is strict on the young side.
	at := CleanupTarget{Kind: "guest-dir", Name: "at", ModTime: now.Add(-24 * time.Hour)}
	just := CleanupTarget{Kind: "guest-dir", Name: "just-inside", ModTime: now.Add(-24*time.Hour + time.Second)}
	remove, _ := CleanupPolicy{OlderThan: 24 * time.Hour}.Select([]CleanupTarget{at, just}, now)
	if !contains(Names(remove), "at") {
		t.Error("something exactly at the age limit was spared; the flag reads as at-least")
	}
	if contains(Names(remove), "just-inside") {
		t.Error("something one second inside the window was removed")
	}
}

func TestCleanupOrderPutsContainersFirst(t *testing.T) {
	now := time.Now()
	targets := []CleanupTarget{
		{Kind: "host-dir", Name: "h"},
		{Kind: "guest-dir", Name: "g"},
		{Kind: "container", Name: "c"},
	}
	remove, _ := CleanupPolicy{}.Select(targets, now)
	got := Names(remove)
	if len(got) != 3 || got[0] != "c" {
		// A directory a running container has open cannot be removed, and the
		// error would name the directory rather than the container holding it.
		t.Fatalf("removal order is %v, want the container first", got)
	}
}

func TestJobIDRecovery(t *testing.T) {
	if got := jobIDFromContainer("wtk-8fa39b8bb07a0154"); got != "8fa39b8bb07a0154" {
		t.Errorf("jobIDFromContainer = %q", got)
	}
	if got := jobIDFromPath("/home/toolkit/.wsl-toolkit/jobs/8fa39b8bb07a0154"); got != "8fa39b8bb07a0154" {
		t.Errorf("jobIDFromPath = %q", got)
	}
	if got := jobIDFromContainer("not-ours"); got != "not-ours" {
		t.Errorf("a name without the prefix should come back unchanged, got %q", got)
	}
}

// -- WSL-35, issue 10: output is complete, and it says when a copy is not -----

func TestJobStreamsSpoolTheCompleteOutput(t *testing.T) {
	home := t.TempDir()
	var live bytes.Buffer
	s := newJobStreams(home, "abc123", &live, nil, 0, nil)
	// Past the bounded copy's ceiling, with a marker at the very end. The marker
	// is what the filed defect said was missing.
	big := strings.Repeat("x", (8<<20)+1024)
	if _, err := s.Out.Write([]byte(big)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Out.Write([]byte("END-MARKER")); err != nil {
		t.Fatal(err)
	}
	s.Close()

	var res JobResult
	s.Apply(&res)
	if !res.StdoutTruncated {
		t.Error("the in-memory copy was cut and stdout_truncated is false, which is the field a caller reads to find out")
	}
	if want := int64(len(big) + len("END-MARKER")); res.StdoutBytes != want {
		t.Errorf("stdout_bytes = %d, want %d: it counts what the command WROTE", res.StdoutBytes, want)
	}
	if int64(len(res.Stdout)) >= res.StdoutBytes {
		t.Error("the in-memory copy is not shorter than the output, so nothing was bounded")
	}
	body, err := os.ReadFile(filepath.Join(home, "jobs", "abc123", "stdout.log"))
	if err != nil {
		t.Fatalf("no transcript: %v", err)
	}
	if !strings.HasSuffix(string(body), "END-MARKER") {
		t.Error("the transcript does not end with the marker, so the complete output was not preserved")
	}
	if len(body) != len(big)+len("END-MARKER") {
		t.Errorf("the transcript is %d bytes and the command wrote %d", len(body), len(big)+len("END-MARKER"))
	}
	if !strings.HasSuffix(live.String(), "END-MARKER") {
		t.Error("the live stream did not receive the end of the output")
	}
}

func TestJobStreamsBelowTheLimitAreNotTruncated(t *testing.T) {
	home := t.TempDir()
	s := newJobStreams(home, "belowlimit", nil, nil, 0, nil)
	payload := strings.Repeat("y", (8<<20)-1) + "Z"
	if _, err := s.Out.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	var res JobResult
	s.Apply(&res)
	if res.StdoutTruncated {
		t.Error("output exactly at the limit was reported as truncated")
	}
	if res.Stdout != payload {
		t.Errorf("the in-memory copy is %d bytes and the payload is %d", len(res.Stdout), len(payload))
	}
	if !strings.HasSuffix(res.Stdout, "Z") {
		t.Error("the last byte at the limit was dropped")
	}
}

func TestJobStreamsWithoutAHomeStillWork(t *testing.T) {
	// ⚠ A missing state directory is not a reason to refuse a job. The bounded
	// copy and the live stream still work, and the result says there is no
	// transcript rather than naming one that is not there.
	var live bytes.Buffer
	s := newJobStreams("", "", &live, nil, 0, nil)
	if _, err := s.Out.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	s.Close()
	var res JobResult
	s.Apply(&res)
	if res.Stdout != "hello" || live.String() != "hello" {
		t.Fatalf("stdout = %q, live = %q", res.Stdout, live.String())
	}
	if res.Transcript != "" {
		t.Errorf("a transcript path was reported with no state directory: %q", res.Transcript)
	}
}

// -- WSL-35, the helper's own half ------------------------------------------

func TestReadHelperEventsRequiresAFinalResult(t *testing.T) {
	stream := eventLines(t,
		HelperEvent{Kind: "log", Text: "starting"},
		HelperEvent{Kind: "stdout", B64: base64.StdEncoding.EncodeToString([]byte("hi"))},
	)
	err := ReadHelperEvents(strings.NewReader(stream), func(HelperEvent) error { return nil })
	if err == nil {
		t.Fatal("a stream that stopped before its result was accepted, so a helper that died mid-job reads as a job that finished")
	}
	if !strings.Contains(err.Error(), "without a result") {
		t.Errorf("the error does not say what is wrong: %v", err)
	}
}

func TestReadHelperEventsDeliversInOrder(t *testing.T) {
	res := JobResult{Exit: 3, Label: "alpine"}
	stream := eventLines(t,
		HelperEvent{Kind: "stdout", B64: base64.StdEncoding.EncodeToString([]byte("one "))},
		HelperEvent{Kind: "stdout", B64: base64.StdEncoding.EncodeToString([]byte("two"))},
		HelperEvent{Kind: "stderr", B64: base64.StdEncoding.EncodeToString([]byte("err"))},
		HelperEvent{Kind: "result", Result: &res, ArtifactsID: "abc"},
	)
	var out, errBuf bytes.Buffer
	var got JobResult
	sinks := HelperSinks{Stdout: &out, Stderr: &errBuf}
	err := ReadHelperEvents(strings.NewReader(stream), func(ev HelperEvent) error {
		if ev.Kind == "result" && ev.Result != nil {
			got = *ev.Result
			return nil
		}
		return sinks.apply(ev)
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "one two" {
		t.Errorf("stdout = %q, want %q", out.String(), "one two")
	}
	if errBuf.String() != "err" {
		t.Errorf("stderr = %q, want %q", errBuf.String(), "err")
	}
	if got.Exit != 3 {
		t.Errorf("the result did not survive the stream: %+v", got)
	}
}

func TestReadHelperEventsSurfacesAnError(t *testing.T) {
	stream := eventLines(t, HelperEvent{Kind: "error", Text: "the fleet could not start"})
	err := ReadHelperEvents(strings.NewReader(stream), func(HelperEvent) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "could not start") {
		t.Fatalf("an error event did not become an error: %v", err)
	}
}

func TestHelperSinksToleratesNilWriters(t *testing.T) {
	// A caller under --json passes nil for stdout on purpose.
	err := HelperSinks{}.apply(HelperEvent{Kind: "stdout", B64: base64.StdEncoding.EncodeToString([]byte("x"))})
	if err != nil {
		t.Fatalf("a nil sink produced an error: %v", err)
	}
}

func TestChunkWriterEncodesArbitraryBytes(t *testing.T) {
	// ⛔ The container's bytes are not text. A chunk cut mid-rune must survive
	// the round trip unchanged.
	raw := []byte{0x00, 0x01, 0xff, 0xfe, 0x80, '\n', 'a'}
	var lines bytes.Buffer
	ew := &eventWriter{enc: json.NewEncoder(&lines)}
	cw := &chunkWriter{out: ew, kind: "stdout"}
	if _, err := cw.Write(raw); err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	sinks := HelperSinks{Stdout: &got}
	var ev HelperEvent
	if err := json.Unmarshal(lines.Bytes(), &ev); err != nil {
		t.Fatal(err)
	}
	if err := sinks.apply(ev); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), raw) {
		t.Fatalf("bytes came back as % x, want % x", got.Bytes(), raw)
	}
}

// -- WSL-38, issue 13: names the caller typed ------------------------------

func TestValidEnvName(t *testing.T) {
	for _, ok := range []string{"PATH", "_x", "A1", "a_b_C9"} {
		if !ValidEnvName(ok) {
			t.Errorf("ValidEnvName(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", "BAD-NAME", "1START", "has space", "a=b", "a.b", "a b"} {
		if ValidEnvName(bad) {
			t.Errorf("ValidEnvName(%q) = true, so the job would run without it and say nothing", bad)
		}
	}
}

// -- helpers ---------------------------------------------------------------

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

func eventLines(t *testing.T, events ...HelperEvent) string {
	t.Helper()
	var b strings.Builder
	for _, ev := range events {
		ev.Schema = HelperEventSchema
		raw, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.String()
}

func tarOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	// Deterministic order, so a failure names the same pair every run.
	sort.Strings(names)
	for _, name := range names {
		body := files[name]
		hdr := &tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// -- the caller-set capture ceiling ----------------------------------------

func TestMaxOutputRaisesWhatTheAnswerKeeps(t *testing.T) {
	home := t.TempDir()
	payload := strings.Repeat("z", 4096)

	// The default ceiling is far above this, so nothing is cut either way. The
	// point of the case is that a SMALL ceiling is honoured, which is the only
	// direction a test can prove without allocating gigabytes.
	s := newJobStreams(home, "small", nil, nil, 1024, nil)
	if _, err := s.Out.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	var res JobResult
	s.Apply(&res)
	if !res.StdoutTruncated {
		t.Error("a 4 KiB payload under a 1 KiB ceiling was not reported as truncated")
	}
	if len(res.Stdout) != 1024 {
		t.Errorf("the answer kept %d bytes, want 1024", len(res.Stdout))
	}
	if res.StdoutBytes != int64(len(payload)) {
		t.Errorf("stdout_bytes = %d, want %d", res.StdoutBytes, len(payload))
	}
	body, err := os.ReadFile(filepath.Join(home, "jobs", "small", "stdout.log"))
	if err != nil || len(body) != len(payload) {
		t.Fatalf("the transcript is %d bytes and the payload is %d: a ceiling on the answer must not bound the transcript", len(body), len(payload))
	}
}

func TestDefaultOutputLimits(t *testing.T) {
	out, errLimit := DefaultOutputLimits()
	if out != 8<<20 || errLimit != 2<<20 {
		t.Fatalf("limits are %d and %d; the documented pair is 8 MiB and 2 MiB", out, errLimit)
	}
}

// -- an open record is evidence, and evidence goes stale --------------------

func TestStillRunning(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		e    LedgerEntry
		want bool
	}{
		{"a job inside its own deadline", LedgerEntry{At: now.Add(-time.Minute), Deadline: now.Add(29 * time.Minute)}, true},
		{"a job past its deadline and the grace after it", LedgerEntry{At: now.Add(-time.Hour), Deadline: now.Add(-30 * time.Minute)}, false},
		{"a job just past its deadline", LedgerEntry{At: now.Add(-time.Hour), Deadline: now.Add(-time.Minute)}, true},
		{"a twelve-hour fleet, one hour in", LedgerEntry{At: now.Add(-time.Hour), Deadline: now.Add(11 * time.Hour)}, true},
		{"an upload with no deadline, minutes old", LedgerEntry{At: now.Add(-5 * time.Minute)}, true},
		{"an upload with no deadline, a day old", LedgerEntry{At: now.Add(-24 * time.Hour)}, false},
		{"a record with no timestamp at all", LedgerEntry{}, false},
	}
	for _, c := range cases {
		if got := stillRunning(c.e, now); got != c.want {
			t.Errorf("%s: stillRunning = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestAKeptJobIsNotLiveForever is the case the acceptance run found.
//
// ⛔ A job whose artifacts failed keeps its guest directory ON PURPOSE. If its
// ledger record also stayed open, cleanup would read the directory as live work
// and spare it at every age, turning a deliberate hold into a permanent leak.
// The record closes; the directory does not.
func TestAKeptJobIsNotLiveForever(t *testing.T) {
	now := time.Now()
	closed := LedgerEntry{At: now.Add(-time.Minute), Deadline: now.Add(-time.Hour)}
	if stillRunning(closed, now) {
		t.Fatal("a record whose deadline passed an hour ago still counts as live work")
	}
	target := CleanupTarget{Kind: "guest-dir", Name: "/home/toolkit/.wsl-toolkit/jobs/kept", ModTime: now.Add(-time.Minute)}
	remove, _ := CleanupPolicy{}.Select([]CleanupTarget{target}, now)
	if len(remove) != 1 {
		t.Fatal("a kept job directory with no live marker was spared by a cleanup with no age limit")
	}
}

// -- the second reading of the core -----------------------------------------

// TestPrefixWriterIsSafeFromTwoStreams is the case for a race a second reading
// found.
//
// ⚠ RUN IT UNDER -race. Measured against the broken version ten times each
// way, it went red in 6 of 10 plain runs and 10 of 10 with the detector. The
// assertions below can catch the corruption on their own, just not dependably,
// and a case that fails six times in ten is one somebody reruns until it is
// green.
//
// ⛔ provision passes ONE prefixWriter as both Stdout and Stderr, and os/exec
// runs a copier per stream when the writer is not an *os.File. `seen` locked
// itself; `partial` did not, so the two copiers appended to the same slice. The
// symptom would be an interleaved or dropped provisioning line, in the one place
// whose output decides whether the base is usable.
func TestPrefixWriterIsSafeFromTwoStreams(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	w := &prefixWriter{to: func(s string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, s)
	}}

	var wg sync.WaitGroup
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				fmt.Fprintf(w, "stream%d line%d\n", g, i)
			}
		}(g)
	}
	wg.Wait()
	w.Flush()

	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 400 {
		t.Fatalf("got %d line(s), want 400: two streams through one writer lost or split some", len(lines))
	}
	seen := map[string]bool{}
	for _, l := range lines {
		if seen[l] {
			t.Fatalf("line %q arrived twice", l)
		}
		seen[l] = true
	}
	for g := 0; g < 2; g++ {
		for i := 0; i < 200; i++ {
			want := fmt.Sprintf("stream%d line%d", g, i)
			if !seen[want] {
				t.Fatalf("%q never arrived", want)
			}
		}
	}
}

// TestPrefixWriterFlushesAnUnterminatedLine, which is exactly the line a failing
// step tends to end on.
func TestPrefixWriterFlushesAnUnterminatedLine(t *testing.T) {
	var lines []string
	w := &prefixWriter{prefix: "  ", to: func(s string) { lines = append(lines, s) }}
	if _, err := w.Write([]byte("first\r\nno newline at the end")); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != "  first" {
		t.Fatalf("before the flush: %v", lines)
	}
	w.Flush()
	if len(lines) != 2 || lines[1] != "  no newline at the end" {
		t.Fatalf("after the flush: %v", lines)
	}
	// ⚠ A second flush over nothing says nothing.
	w.Flush()
	if len(lines) != 2 {
		t.Fatalf("an empty flush emitted something: %v", lines)
	}
}

// TestPrefixWriterBoundsAnUnterminatedLine so a child that writes megabytes
// without a newline cannot grow the held buffer without limit. The text is
// flushed rather than dropped: a long line is still information.
func TestPrefixWriterBoundsAnUnterminatedLine(t *testing.T) {
	var lines []string
	w := &prefixWriter{to: func(s string) { lines = append(lines, s) }}
	if _, err := w.Write([]byte(strings.Repeat("x", maxPartialLine+10))); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("got %d line(s), want 1: the held text was neither flushed nor kept", len(lines))
	}
	if len(w.partial) != 0 {
		t.Errorf("%d byte(s) are still held after the ceiling was passed", len(w.partial))
	}
}

// TestANameOfDotsIsNotAName is the other second-reading finding.
//
// ⛔ `matrix --artifacts out` writes each row into `out/<id>`, so an id of `..`
// put a fleet's output in the parent of the directory the caller named. Both
// dot names passed the character rule, because a dot is legal in `debian12` and
// in `ubuntu-24.04`.
func TestANameOfDotsIsNotAName(t *testing.T) {
	for _, bad := range []string{".", "..", "...", ".hidden", ".."} {
		if isImageID(bad) {
			t.Errorf("isImageID(%q) = true, and it is used as a path component", bad)
		}
		if isDistroName(bad) {
			t.Errorf("isDistroName(%q) = true", bad)
		}
	}
	// ⚠ And the rule must not widen. A dot inside a name is ordinary.
	for _, ok := range []string{"alpine", "debian12", "ubuntu-24.04", "a..b", "void-musl", "rocky8", "x.y.z"} {
		if !isImageID(ok) {
			t.Errorf("isImageID(%q) = false, and it is an ordinary catalog id", ok)
		}
		if !isDistroName(ok) {
			t.Errorf("isDistroName(%q) = false", ok)
		}
	}
}

// TestAConfigWithADotIdIsRefusedWhenItIsREAD, which is where this tree validates.
func TestAConfigWithADotIdIsRefusedWhenItIsRead(t *testing.T) {
	// ⚠ The element is BOUND TO A VARIABLE rather than written inline. A Go
	// composite literal of slice-of-struct opens with `{{`, which the tree's
	// placeholder check reads as an unfilled template. Binding it is the fix;
	// widening that guard to allow `{{` would be the wrong half to change.
	bad := Image{ID: "..", Ref: "docker.io/library/alpine:latest", Libc: "musl", Kind: "musl"}
	good := Image{ID: "alpine", Ref: "docker.io/library/alpine:latest", Libc: "musl", Kind: "musl"}

	cfg := DefaultConfig()
	cfg.Images = []Image{bad}
	if err := cfg.Validate(); err == nil {
		t.Fatal("a catalog entry with the id \"..\" was accepted, and matrix --artifacts would write a row into the parent of the directory the caller named")
	}
	cfg.Images = []Image{good}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("an ordinary catalog entry was refused: %v", err)
	}
}

// TestLedgerSurvivesConcurrentUse runs appends against compactions and asserts
// the ledger stays coherent. Under -race it also proves there is no data race
// on the file or the mutex.
//
// ⚠ IT DOES NOT PROVE THE FIX IT WAS WRITTEN FOR, and saying so is the
// point. Compact used to read the file with no lock and then take the lock to
// write, so a record appended between those two steps was read by neither and
// discarded by the write: a lost OPEN record, which is what cleanup reads as
// work in flight, in a helper that serves requests concurrently.
//
// ⛔ THAT WINDOW IS TOO NARROW TO HIT. Measured: the broken version was run
// against this case ten times and went red in NONE of them, because Open took
// the lock too and an append almost never lands in the gap between its release
// and Compact's acquire. A test that passes ten times out of ten against the
// defect is worse than no test, so this one does not claim to cover it.
//
// ⭐ The invariant is held by STRUCTURE instead: Compact takes the lock once
// and calls openLocked, so its read and its write cannot be separated. That is
// checkable by reading nine lines, and it is what a reviewer should check.
func TestLedgerSurvivesConcurrentUse(t *testing.T) {
	dir := t.TempDir()
	l := &Ledger{path: filepath.Join(dir, "ledger.jsonl")}

	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("old%02d", i)
		if err := l.Append(LedgerEntry{Event: "open", Kind: "job", ID: id}); err != nil {
			t.Fatal(err)
		}
		if err := l.Append(LedgerEntry{Event: "close", Kind: "job", ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Append(LedgerEntry{Event: "open", Kind: "job", ID: "live"}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = l.Append(LedgerEntry{Event: "open", Kind: "job", ID: fmt.Sprintf("new%02d", i)})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			if _, err := l.Compact(); err != nil {
				t.Error(err)
			}
		}
	}()
	wg.Wait()

	open, err := l.Open()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range open {
		got[e.ID] = true
	}
	if !got["live"] {
		t.Error("the record open before any of this started was lost")
	}
	missing := 0
	for i := 0; i < 50; i++ {
		if !got[fmt.Sprintf("new%02d", i)] {
			missing++
		}
	}
	if missing > 0 {
		t.Fatalf("%d of 50 records appended during a compaction were lost. Each one is a job cleanup would not see as live", missing)
	}
	if len(got) < 51 {
		t.Fatalf("the ledger holds %d open record(s) and at least 51 were opened", len(got))
	}
	if got["old00"] {
		t.Error("a closed record survived compaction")
	}
}

// TestLedgerOpenIsOrdered, because Compact writes what Open returns and a map's
// order is not one.
func TestLedgerOpenIsOrdered(t *testing.T) {
	dir := t.TempDir()
	l := &Ledger{path: filepath.Join(dir, "ledger.jsonl")}
	base := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		if err := l.Append(LedgerEntry{
			Event: "open", Kind: "job", ID: fmt.Sprintf("j%02d", i),
			At: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	var first []string
	for run := 0; run < 5; run++ {
		open, err := l.Open()
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, e := range open {
			ids = append(ids, e.ID)
		}
		if run == 0 {
			first = ids
			continue
		}
		if strings.Join(ids, ",") != strings.Join(first, ",") {
			t.Fatalf("two reads of one ledger returned different orders:\n  %v\n  %v", first, ids)
		}
	}
	if len(first) != 12 || first[0] != "j00" || first[11] != "j11" {
		t.Fatalf("the order is not by time: %v", first)
	}
}

// -- the fifth review: what a failure is allowed to hide ----------------------

// ⛔ THE WHOLE TYPE SHIPPED IN v1.2.0 WITH NO UNIT CASE. ClientSpool is what
// makes `wsl-toolkit logs` work on the helper route, and the only thing covering
// it was one acceptance case that exercises the direct route. These five are the
// missing half, and three of them are for defects the fifth review found.

// TestClientSpoolSaysWhyItHasNoTranscript is the finding itself.
//
// ⛔ EVERY ONE OF THESE RETURNS WAS SILENT. A job ran, succeeded, and `logs`
// found nothing, with no line anywhere naming the step that gave up. The direct
// route's openSpool logged the same failure all along, so one route told the
// operator and the other did not. That is issue #10's defect class, silent
// degradation, in a place the issue did not name.
func TestClientSpoolSaysWhyItHasNoTranscript(t *testing.T) {
	t.Run("no state directory at all", func(t *testing.T) {
		var said []string
		if got := NewClientSpool("", nil, func(s string) { said = append(said, s) }); got != nil {
			t.Fatalf("a spool was returned for an empty home")
		}
		if len(said) != 1 {
			t.Fatalf("said %d line(s), want 1: %v", len(said), said)
		}
	})

	t.Run("the staging directory cannot be made", func(t *testing.T) {
		home := t.TempDir()
		// A FILE where the directory has to go. MkdirAll cannot win against it,
		// on either platform, which is what makes this deterministic.
		if err := os.WriteFile(filepath.Join(home, "incoming"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		var said []string
		if got := NewClientSpool(home, nil, func(s string) { said = append(said, s) }); got != nil {
			t.Fatalf("a spool was returned although its directory could not be made")
		}
		if len(said) != 1 {
			t.Fatalf("said %d line(s), want 1: %v", len(said), said)
		}
		if !strings.Contains(said[0], "staging directory") {
			t.Fatalf("the line does not name the step that failed: %q", said[0])
		}
	})
}

// TestClientSpoolKeepsTheLiveStreamWhenTheSpoolFails guards the sink that must
// not be io.MultiWriter.
//
// ⛔ MultiWriter STOPS AT THE FIRST SINK THAT ERRORS and returns that error.
// The live writer here is the caller's own stdout and the spool is a local
// convenience, so a full disk on this machine would have aborted a job running on
// the helper and discarded a result that had already been produced.
func TestClientSpoolKeepsTheLiveStreamWhenTheSpoolFails(t *testing.T) {
	home := t.TempDir()
	s := NewClientSpool(home, nil, nil)
	if s == nil {
		t.Fatal("no spool")
	}
	// The spool file is closed underneath it, which is the cheapest stand-in for
	// a sink that has started refusing writes.
	if err := s.out.Close(); err != nil {
		t.Fatal(err)
	}

	var live bytes.Buffer
	w := s.Tee(&live, false)
	n, err := w.Write([]byte("the answer\n"))
	if err != nil {
		t.Fatalf("a failing spool failed the write: %v", err)
	}
	if n != len("the answer\n") {
		t.Fatalf("wrote %d bytes, want %d", n, len("the answer\n"))
	}
	if live.String() != "the answer\n" {
		t.Fatalf("the live stream got %q", live.String())
	}
	s.Discard()
}

// TestClientSpoolFinishWithoutAnIdLeavesNothingBehind covers a result that
// carried no job id: there is nothing to file the transcript under, and waiting
// for cleanup to notice would leak one directory per such job.
func TestClientSpoolFinishWithoutAnIdLeavesNothingBehind(t *testing.T) {
	home := t.TempDir()
	s := NewClientSpool(home, nil, nil)
	if s == nil {
		t.Fatal("no spool")
	}
	dir := s.dir
	if got := s.Finish(""); got != "" {
		t.Fatalf("Finish returned %q for a job with no id", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the staging directory survived a job with no id: %v", err)
	}
}

// TestClientSpoolFinishYieldsToAnExistingTranscript covers a client and a helper
// that share one state directory. ⚠ The helper's copy is COMPLETE; this
// client's starts wherever it began reading, so overwriting would trade a whole
// transcript for part of one.
func TestClientSpoolFinishYieldsToAnExistingTranscript(t *testing.T) {
	home := t.TempDir()
	s := NewClientSpool(home, nil, nil)
	if s == nil {
		t.Fatal("no spool")
	}
	staging := s.dir
	dest := filepath.Join(home, "jobs", "JOB-1")
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "stdout.log"), []byte("the helper's own"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := s.Finish("JOB-1"); got != dest {
		t.Fatalf("Finish returned %q, want the existing %q", got, dest)
	}
	kept, err := os.ReadFile(filepath.Join(dest, "stdout.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != "the helper's own" {
		t.Fatalf("the complete transcript was overwritten with %q", string(kept))
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("the staging directory survived: %v", err)
	}
}

// TestClientSpoolFinishReturnsTheTranscriptItCouldNotFile is the third finding.
//
// ⛔ IT USED TO RETURN THE EMPTY STRING while a complete transcript sat on
// this disk, so the caller reported no transcript for a job whose output it was
// holding. Returning the staging path is strictly better: `logs` reads it, gc
// collects it by age, and the operator is told why it is not under `jobs/`.
func TestClientSpoolFinishReturnsTheTranscriptItCouldNotFile(t *testing.T) {
	home := t.TempDir()
	var said []string
	s := NewClientSpool(home, nil, func(line string) { said = append(said, line) })
	if s == nil {
		t.Fatal("no spool")
	}
	if _, err := s.out.Write([]byte("output worth keeping\n")); err != nil {
		t.Fatal(err)
	}
	// A FILE where `jobs` has to be a directory, so filing cannot succeed.
	if err := os.WriteFile(filepath.Join(home, "jobs"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := s.Finish("JOB-2")
	if got == "" {
		t.Fatal("Finish denied having a transcript it was holding")
	}
	kept, err := os.ReadFile(filepath.Join(got, "stdout.log"))
	if err != nil {
		t.Fatalf("the path Finish returned does not hold the output: %v", err)
	}
	if string(kept) != "output worth keeping\n" {
		t.Fatalf("the transcript holds %q", string(kept))
	}
	if len(said) != 1 || !strings.Contains(said[0], got) {
		t.Fatalf("the operator was not told where it stayed: %v", said)
	}
}

// TestMatrixTablePointsAtLogsOnce covers the fleet's half of the same gap.
//
// ⭐ ONCE, not once per row. A twelve-row table followed by twelve paths is a
// table nobody reads, and `logs` with no arguments is what lists the ids.
func TestMatrixTablePointsAtLogsOnce(t *testing.T) {
	t.Run("rows that kept their output", func(t *testing.T) {
		var out bytes.Buffer
		report := MatrixReport{Rows: []JobResult{
			{Label: "alpine", Transcript: "one"},
			{Label: "debian12", Transcript: "two"},
		}}
		if err := RenderMatrix(&out, report); err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(out.String(), "wsl-toolkit logs"); n != 1 {
			t.Fatalf("the table points at logs %d times, want exactly 1:\n%s", n, out.String())
		}
	})

	t.Run("rows that kept nothing", func(t *testing.T) {
		var out bytes.Buffer
		// ⚠ BOUND TO A VARIABLE rather than written inline. A Go composite
		// literal of slice-of-struct opens with `{{`, which this tree's
		// placeholder check reads as an unfilled template. Binding it is the fix;
		// widening that guard to allow `{{` would be the wrong half to change.
		row := JobResult{Label: "alpine"}
		report := MatrixReport{Rows: []JobResult{row}}
		if err := RenderMatrix(&out, report); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "wsl-toolkit logs") {
			t.Fatalf("the table offered a transcript nothing wrote:\n%s", out.String())
		}
	})
}
