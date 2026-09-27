package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// ⭐ WHY THIS COMMAND EXISTS. A job's structured answer holds a BOUNDED copy of
// its output and says so; the complete text is spooled to a file whose path the
// result names. Without a command to read it, that path is a string a caller has
// to resolve by hand, and the recovery this tool advertises is one nobody
// performs. `logs` is the other half of the truncation fix.

const logsUsage = `wsl-toolkit logs [JOB-ID]

  With no id, list the jobs and detached sessions this machine still has,
  newest first, each with its state. With one, write that job's streams.

  --stderr    write the error stream instead of the output stream
  --both      write both, the output first
  --tail N    only the last N lines
  --follow    write both streams, each to its own, as the job writes them,
              and answer with the job's own exit code once it ends
  --json      list as structured data

  A transcript lives beside the job's own state and is removed by
  wsl-toolkit gc under the same age policy as everything else, so an id
  that ran long enough ago will not be here. A detached base session keeps
  its streams in the base, and logs reads them there.
`

func cmdLogs(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("logs")
	wantErr := fs.Bool("stderr", false, "write the error stream instead of the output stream")
	both := fs.Bool("both", false, "write both streams, the output first")
	tail := fs.Int("tail", 0, "only the last N lines. 0 means all of it")
	follow := fs.Bool("follow", false, "write both streams, each to its own, as the job writes them, and answer with the job's exit code once it ends")
	asJSON := fs.Bool("json", false, "write a structured answer")
	id, rest := splitLogsArgs(args)
	if err := parseArgs(fs, rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, logsUsage)
			return exitOK, nil
		}
		return exitCannot, err
	}
	if *tail < 0 {
		return exitCannot, fmt.Errorf("--tail %d is negative. Pass 0 for all of it", *tail)
	}
	if *follow {
		// ⭐ ONE STATE MACHINE FOR --follow AND wait: the host's record first, a
		// running owner's transcript next, and the engine only for a job whose
		// owner is gone without saying. WSL-94.
		switch {
		case id == "":
			return exitCannot, errors.New("--follow reads one job: wsl-toolkit logs JOB-ID --follow")
		case *wantErr || *both:
			return exitCannot, errors.New("--follow writes both streams, each to its own, so --stderr and --both have nothing to choose")
		case *asJSON:
			return exitCannot, errors.New("--follow writes the job's streams as they arrive, which is not a document. wsl-toolkit wait JOB-ID --json answers how it ended")
		}
		end, err := jobEnd(ctx, id, *tail, os.Stdout, os.Stderr)
		if err != nil {
			return jobEndCode(err), err
		}
		if end.Note != "" {
			logf("  %s", end.Note)
		}
		return end.verdict, nil
	}
	// ⛔ Home, not EnsureHome. `logs` READS transcripts, and a reader that
	// created the directory it was about to say was empty has changed the thing
	// it was asked to measure. WSL-55.
	home, err := toolkit.Home()
	if err != nil {
		return exitCannot, err
	}
	if id == "" {
		return listTranscripts(home, *asJSON)
	}
	if err := toolkit.AssertArgvSafe([]string{id}); err != nil {
		return exitCannot, err
	}
	// ⭐ A SESSION'S STREAMS ARE IN THE GUEST, so its host directory holds a
	// record and no transcript, and `logs ID` reads them where they are. WSL-95.
	if toolkit.ValidJobID(id) {
		if rec, isSession, err := toolkit.ReadSessionRecord(home, id); err != nil {
			return exitCannot, err
		} else if isSession {
			return sessionLogs(ctx, rec, *wantErr, *both, *tail)
		}
	}
	dir := filepath.Join(home, "jobs", id)
	if _, err := toolkit.ResolveInside(home, dir); err != nil {
		// ⛔ A caller-supplied path component is how a log reader becomes a file
		// reader. It is resolved and contained before anything is opened.
		return exitCannot, err
	}
	if _, err := os.Stat(dir); err != nil {
		return exitCannot, fmt.Errorf("no transcript for job %q. wsl-toolkit logs lists what is still here", id)
	}
	names := []string{"stdout.log"}
	switch {
	case *both:
		names = []string{"stdout.log", "stderr.log"}
	case *wantErr:
		names = []string{"stderr.log"}
	}
	wrote := false
	for _, name := range names {
		n, err := copyTranscript(filepath.Join(dir, name), *tail, os.Stdout)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return exitCannot, err
		}
		wrote = wrote || n
	}
	if !wrote {
		logf("  job %s has a directory and no transcript in it", id)
		return exitFailed, nil
	}
	return exitOK, nil
}

// sessionLogs writes a detached session's recorded streams, as `logs ID` does
// for a job.
func sessionLogs(ctx context.Context, rec toolkit.SessionRecord, wantErr, both bool, tail int) (int, error) {
	names := []string{"stdout"}
	switch {
	case both:
		names = []string{"stdout", "stderr"}
	case wantErr:
		names = []string{"stderr"}
	}
	w, err := toolkit.FindWsl()
	if err != nil {
		return exitCannot, err
	}
	st, err := toolkit.SessionNow(ctx, w, rec)
	if err != nil {
		return exitCannot, err
	}
	if st.Gone {
		return exitFailed, fmt.Errorf("session %s: the guest holds nothing for it, so there is nothing to write", rec.ID)
	}
	if err := toolkit.ReadSessionStreams(ctx, w, rec, names, tail, os.Stdout); err != nil {
		return exitCannot, err
	}
	return exitOK, nil
}

// splitLogsArgs takes the job id off the front, if one is there.
//
// ⛔ THE ID IS THE FIRST ARGUMENT OR THERE IS NONE. Scanning the whole list
// for the first word without a leading dash reads `logs --tail 5` as a request
// for job "5", because a flag's VALUE has no dash either. One fixed position
// cannot be confused with an option's argument.
func splitLogsArgs(args []string) (id string, rest []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

// copyTranscript writes one recorded stream to w, or its last N lines.
func copyTranscript(path string, tail int, w io.Writer) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if tail <= 0 {
		// ⭐ Copied rather than read into memory. The whole reason a transcript
		// exists is that the output did not fit in a buffer, so a reader that
		// buffered it would reintroduce the limit it is here to escape.
		_, err := io.Copy(w, f)
		return true, err
	}
	lines, err := toolkit.LastLines(f, tail)
	if err != nil {
		return false, err
	}
	for _, ln := range lines {
		if _, err := fmt.Fprintln(w, ln); err != nil {
			return true, err
		}
	}
	return true, nil
}

type transcriptRow struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Modified time.Time `json:"modified"`
	Stdout   int64     `json:"stdout_bytes"`
	Stderr   int64     `json:"stderr_bytes"`
	// Kind is job or session, State is running, ended, detached or no owner,
	// and Exit is the recorded verdict of an ended job. WSL-94.
	Kind  string `json:"kind"`
	State string `json:"state"`
	Exit  *int   `json:"exit,omitempty"`
}

func listTranscripts(home string, asJSON bool) (int, error) {
	root := filepath.Join(home, "jobs")
	entries, err := os.ReadDir(root)
	// ⛔ ONLY A MISSING DIRECTORY MEANS "NOT YET". Every error used to land
	// here, so an unreadable `jobs` produced the sentence "no transcripts on this
	// machine yet" and exit 0: a true-sounding answer to a question this process
	// could not answer, which is issue #10's defect class in a place the issue did
	// not name.
	//
	// ⚠ ON WINDOWS THIS DOES NOT SEPARATE A FILE FROM AN ABSENCE, and the
	// narrowing is still worth having. Measured: `os.ReadDir` on a path that is a
	// FILE returns ERROR_PATH_NOT_FOUND, for which `os.IsNotExist` reports true, so
	// a file named `jobs` still reads as "not yet". That is a tolerable answer for
	// a situation this tool did not create. What the narrowing does catch is every
	// OTHER failure: a directory that cannot be opened, a path the OS refuses, a
	// volume that went away.
	if err != nil && os.IsNotExist(err) {
		if asJSON {
			return exitOK, writeJSON(map[string]any{"schema": "wsl-toolkit-logs/1", "transcripts": []transcriptRow{}})
		}
		logf("  no transcripts on this machine yet")
		return exitOK, nil
	}
	if err != nil {
		return exitCannot, fmt.Errorf("the transcripts in %s could not be listed: %w", root, err)
	}
	var rows []transcriptRow
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		row := transcriptRow{ID: e.Name(), Path: dir}
		if info, err := e.Info(); err == nil {
			row.Modified = info.ModTime()
		}
		row.Stdout, _ = toolkit.FileSize(filepath.Join(dir, "stdout.log"))
		row.Stderr, _ = toolkit.FileSize(filepath.Join(dir, "stderr.log"))
		row.Kind, row.State = "job", "unknown"
		if st, err := toolkit.ReadJobState(home, e.Name()); err == nil && st.Known {
			row.State = st.Word()
			if st.Session != nil {
				row.Kind = "session"
			}
			if st.Result != nil {
				v := st.Result.Verdict()
				row.Exit = &v
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Modified.After(rows[j].Modified) })
	if asJSON {
		return exitOK, writeJSON(map[string]any{"schema": "wsl-toolkit-logs/1", "transcripts": rows})
	}
	if len(rows) == 0 {
		logf("  no transcripts on this machine yet")
		return exitOK, nil
	}
	fmt.Fprintf(os.Stderr, "  %-18s %-8s %-9s %5s %-20s %10s %10s\n", "id", "kind", "state", "exit", "when", "stdout", "stderr")
	for _, r := range rows {
		exit := "-"
		if r.Exit != nil {
			exit = strconv.Itoa(*r.Exit)
		}
		fmt.Printf("  %-18s %-8s %-9s %5s %-20s %10s %10s\n", r.ID, r.Kind, r.State, exit, r.Modified.Format("2006-01-02 15:04:05"),
			toolkit.HumanBytes(r.Stdout), toolkit.HumanBytes(r.Stderr))
	}
	fmt.Fprintf(os.Stderr, "\n  %d job(s). wsl-toolkit logs ID writes one; --follow follows one that is running.\n", len(rows))
	return exitOK, nil
}
