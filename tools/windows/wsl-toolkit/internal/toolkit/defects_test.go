// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
