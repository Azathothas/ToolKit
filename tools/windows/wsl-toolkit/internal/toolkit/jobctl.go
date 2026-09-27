// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Job control from a second process: stop a job, follow its output, wait for
// its end. The owner's lock and result say whether the job is running and how
// it ended; the engine is asked only where no owner is left to say. WSL-94.

// StopSchema versions `stop --json`.
const StopSchema = "wsl-toolkit-stop/1"

// StopReport is what one stop did.
type StopReport struct {
	Schema string `json:"schema"`
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	// Before is what the stop found: running, exited, created, gone, or not
	// started for a job still being prepared.
	Before string `json:"before"`
	// After is the state once the stop returned.
	After string `json:"after,omitempty"`
	Exit  *int   `json:"exit,omitempty"`
	// Stopped is true where this call ended something that was running.
	Stopped bool   `json:"stopped"`
	Note    string `json:"note,omitempty"`
}

// EndedByStop says whether a job's exit is a stop's doing: the engine's TERM or
// KILL, with a stop in the ledger.
//
// ⛔ ONE RULE FOR THE OWNER AND FOR A SECOND PROCESS. The owner reads it when
// its container exits, and `wait` reads it for a job whose owner is gone, so
// the two answer one verdict for one ending.
func (r *Runner) EndedByStop(id string, exit int) bool {
	return stoppedBySignal(exit) && r.stopRequested(id)
}

// stopRequested says whether the ledger holds a stop for a job.
func (r *Runner) stopRequested(id string) bool {
	entries, err := r.ledger.All()
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Event == "stop" && e.Kind == "job" && e.ID == id {
			return true
		}
	}
	return false
}

// StopJob stops one container job.
//
// ⛔ THE REQUEST IS RECORDED BEFORE THE ENGINE IS ASKED. The owner reads the
// ledger at three points: before its container starts, when the container
// announces itself, and after the container exits. A job still being prepared
// then never starts its container, and a job whose container is being created
// is stopped as the container starts.
//
// ⛔ ONLY A CONTAINER CARRYING THIS TOOL'S LABEL IS STOPPED. The name is built
// from a validated id, and the label is checked as well, so no stop can reach a
// container this tool did not start.
func (r *Runner) StopJob(ctx context.Context, id string, grace time.Duration) (StopReport, error) {
	rep := StopReport{Schema: StopSchema, ID: id, Kind: "job"}
	if !ValidJobID(id) {
		return rep, fmt.Errorf("%q is not a job id: an id is 16 lowercase hex characters", id)
	}
	known, err := r.jobKnown(id)
	if err != nil {
		return rep, err
	}
	if !known {
		return rep, fmt.Errorf("%w: %s. wsl-toolkit logs lists the jobs this machine still has", ErrUnknownJob, id)
	}
	if err := r.ledger.Append(LedgerEntry{Event: "stop", Kind: "job", ID: id, Note: "requested by wsl-toolkit stop"}); err != nil {
		return rep, fmt.Errorf("could not record the stop before acting on it: %w", err)
	}
	g := int64(grace.Round(time.Second) / time.Second)
	if g < 0 {
		g = 0
	}
	n := shellQuote("wtk-" + id)
	script := "n=" + n + "\n" + `
if ! podman container exists "$n" >/dev/null 2>&1; then echo 'BEFORE|gone'; exit 0; fi
owner=$(podman inspect --type container --format '{{index .Config.Labels "wsl-toolkit.owner"}}' "$n" 2>/dev/null)
if [ "$owner" != wsl-toolkit ]; then echo "wsl-toolkit: $n does not carry this tool's label, so it is not stopped" >&2; exit 2; fi
st=$(podman inspect --type container --format '{{.State.Status}}' "$n" 2>/dev/null)
printf 'BEFORE|%s\n' "$st"
case "$st" in running|paused|created|stopping)
  podman stop --time ` + strconv.FormatInt(g, 10) + ` "$n" >/dev/null 2>&1 || : ;;
esac
after=$(podman inspect --type container --format '{{.State.Status}}|{{.State.ExitCode}}' "$n" 2>/dev/null) || after='gone|'
printf 'AFTER|%s\n' "$after"
`
	budget := grace + 2*time.Minute
	out, stderr, code, err := r.baseCapture(ctx, []byte(script), budget)
	if err != nil || code != 0 {
		return rep, fmt.Errorf("could not stop job %s (exit %d): %s", id, code, guestFailure(stderr, out))
	}
	readStopOutput(&rep, out)
	if rep.Before == "gone" {
		st, _ := ReadJobState(r.home, id)
		switch {
		case st.Result != nil:
			rep.Before, rep.Note = "ended", "the job had already ended, so nothing was stopped"
			v := st.Result.Verdict()
			rep.Exit = &v
		case st.Owned:
			rep.Before, rep.Note = "not started", "the job's owner is still preparing it or creating its container. The stop is recorded, and the owner ends the job before its container starts or as it starts"
			rep.Stopped = true
		default:
			rep.Note = "no container holds this job and no process owns it, so nothing was stopped"
		}
	}
	return rep, nil
}

// readStopOutput reads the stop script's two lines. ⭐ A function with no
// process in it, so its case runs on any host.
func readStopOutput(rep *StopReport, out string) {
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(strings.TrimRight(line, "\r")), "|")
		if !ok {
			continue
		}
		switch key {
		case "BEFORE":
			rep.Before = val
		case "AFTER":
			state, ec, _ := strings.Cut(val, "|")
			rep.After = state
			if n, err := strconv.Atoi(strings.TrimSpace(ec)); err == nil && state != "gone" {
				rep.Exit = &n
			}
		}
	}
	switch rep.Before {
	case "running", "paused", "created", "stopping":
		rep.Stopped = true
	}
}

// jobKnown says whether this state directory holds anything about a job: its
// host directory or a ledger record.
func (r *Runner) jobKnown(id string) (bool, error) {
	if info, err := os.Stat(filepath.Join(r.home, "jobs", id)); err == nil && info.IsDir() {
		return true, nil
	}
	entries, err := r.ledger.All()
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.ID == id && e.Kind == "job" {
			return true, nil
		}
	}
	return false, nil
}

// FollowPoll is how often a follower reads a transcript that is still growing.
const FollowPoll = 250 * time.Millisecond

// FollowTranscript writes a job's transcript as its owner writes it, from the
// start or from the last tail lines, until the owner lets its lock go. It then
// reads what is left and returns.
func FollowTranscript(ctx context.Context, dir string, tail int, stdout, stderr io.Writer) error {
	lock := filepath.Join(dir, ownerLockName)
	streams := []*followedFile{
		{path: filepath.Join(dir, "stdout.log"), to: stdout},
		{path: filepath.Join(dir, "stderr.log"), to: stderr},
	}
	for _, f := range streams {
		if tail > 0 {
			if err := f.startAtTail(tail); err != nil {
				return err
			}
		}
	}
	for {
		held, _, err := lockProbe(lock)
		if err != nil {
			return err
		}
		for _, f := range streams {
			if err := f.copyNew(); err != nil {
				return err
			}
		}
		if !held {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(FollowPoll):
		}
	}
}

// followedFile is one transcript being read as it grows.
type followedFile struct {
	path string
	to   io.Writer
	off  int64
}

func (f *followedFile) startAtTail(n int) error {
	h, err := os.Open(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = h.Close() }()
	info, err := h.Stat()
	if err != nil {
		return err
	}
	lines, err := LastLines(h, n)
	if err != nil {
		return err
	}
	kept := int64(len(strings.Join(lines, "\n")))
	if len(lines) > 0 {
		kept++
	}
	if kept > info.Size() {
		kept = info.Size()
	}
	f.off = info.Size() - kept
	return nil
}

func (f *followedFile) copyNew() error {
	h, err := os.Open(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = h.Close() }()
	info, err := h.Stat()
	if err != nil {
		return err
	}
	if info.Size() <= f.off {
		return nil
	}
	n, err := io.Copy(f.to, io.NewSectionReader(h, f.off, info.Size()-f.off))
	f.off += n
	return err
}

// LastLines reads the end of a file without holding all of it, growing a window
// from the end until it holds enough newlines.
func LastLines(f *os.File, n int) ([]string, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	window := int64(64 << 10)
	for {
		if window > size {
			window = size
		}
		buf := make([]byte, window)
		if _, err := f.ReadAt(buf, size-window); err != nil && err != io.EOF {
			return nil, err
		}
		text := strings.TrimSuffix(string(buf), "\n")
		lines := strings.Split(text, "\n")
		if len(lines) > n {
			return lines[len(lines)-n:], nil
		}
		if window == size {
			return lines, nil
		}
		window *= 4
	}
}

// markerLine is the line a job's wrapper writes to stderr before the payload
// starts. The engine's own log of a container holds it, and a follower that
// reads that log strips it.
var markerLine = regexp.MustCompile(`^wtk-started-[0-9a-f]{32}$`)

// markerFilter drops the wrapper's first two stderr lines, an empty line and
// the marker, when they are exactly that, and passes every other byte through.
type markerFilter struct {
	to      io.Writer
	held    []byte
	decided bool
}

func (m *markerFilter) Write(p []byte) (int, error) {
	if m.decided {
		return m.to.Write(p)
	}
	m.held = append(m.held, p...)
	if !bytes.HasPrefix(m.held, []byte("\n")) {
		return len(p), m.release(m.held)
	}
	end := bytes.IndexByte(m.held[1:], '\n')
	if end < 0 {
		if len(m.held) > 64 {
			return len(p), m.release(m.held)
		}
		return len(p), nil
	}
	line := string(m.held[1 : end+1])
	rest := m.held[end+2:]
	if markerLine.MatchString(line) {
		return len(p), m.release(rest)
	}
	return len(p), m.release(m.held)
}

func (m *markerFilter) release(b []byte) error {
	m.decided, m.held = true, nil
	if len(b) == 0 {
		return nil
	}
	_, err := m.to.Write(b)
	return err
}

// Flush writes what the filter still holds, for a stream that ended short.
func (m *markerFilter) Flush() error {
	if m.decided {
		return nil
	}
	return m.release(m.held)
}

// FollowContainer writes a job's container log from the engine, for a job whose
// owner is gone, and returns the container's own exit, or nil where the
// container is gone too.
func (r *Runner) FollowContainer(ctx context.Context, id string, tail int, stdout, stderr io.Writer) (exit *int, present bool, err error) {
	n := shellQuote("wtk-" + id)
	tailArg := ""
	if tail > 0 {
		tailArg = " --tail " + strconv.Itoa(tail)
	}
	exists := "podman container exists " + n + " >/dev/null 2>&1"
	out, _, code, err := r.baseCapture(ctx, []byte(exists+" && echo yes || echo no\n"), 2*time.Minute)
	if err != nil || code != 0 {
		return nil, false, fmt.Errorf("could not ask the engine about job %s (exit %d)", id, code)
	}
	if !strings.Contains(out, "yes") {
		return nil, false, nil
	}
	filter := &markerFilter{to: stderr}
	if _, err := r.wsl.Exec(ctx, ExecRequest{
		Distro: r.cfg.Base.Name, User: r.cfg.Base.User,
		Script: append(guestRuntimePrologue(), []byte("exec podman logs --follow"+tailArg+" "+n+"\n")...),
		Stdout: stdout, Stderr: filter,
	}); err != nil && ctx.Err() != nil {
		return nil, true, ctx.Err()
	}
	_ = filter.Flush()
	out, _, code, err = r.baseCapture(ctx, []byte("podman inspect --type container --format '{{.State.ExitCode}}' "+n+" 2>/dev/null || :\n"), 2*time.Minute)
	if err != nil || code != 0 {
		return nil, true, nil
	}
	if v, perr := strconv.Atoi(strings.TrimSpace(out)); perr == nil {
		return &v, true, nil
	}
	return nil, true, nil
}
