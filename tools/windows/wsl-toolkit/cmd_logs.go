package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

  With no id, list the transcripts this machine still has, newest first.
  With one, write that job's streams.

  --stderr    write the error stream instead of the output stream
  --both      write both, the output first
  --tail N    only the last N lines
  --json      list as structured data

  A transcript lives beside the job's own state and is removed by
  wsl-toolkit gc under the same age policy as everything else, so an id
  that ran long enough ago will not be here.
`

func cmdLogs(args []string) (int, error) {
	fs := newFlagSet("logs")
	wantErr := fs.Bool("stderr", false, "write the error stream instead of the output stream")
	both := fs.Bool("both", false, "write both streams, the output first")
	tail := fs.Int("tail", 0, "only the last N lines. 0 means all of it")
	asJSON := fs.Bool("json", false, "write a structured answer")
	id, rest := splitLogsArgs(args)
	if err := parseArgs(fs, rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, logsUsage)
			return exitOK, nil
		}
		return exitCannot, err
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
		n, err := writeTranscript(filepath.Join(dir, name), *tail)
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

// writeTranscript puts one stream on stdout, optionally its last N lines.
func writeTranscript(path string, tail int) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if tail <= 0 {
		// ⭐ Copied rather than read into memory. The whole reason a transcript
		// exists is that the output did not fit in a buffer, so a reader that
		// buffered it would reintroduce the limit it is here to escape.
		_, err := io.Copy(os.Stdout, f)
		return true, err
	}
	lines, err := lastLines(f, tail)
	if err != nil {
		return false, err
	}
	for _, ln := range lines {
		fmt.Println(ln)
	}
	return true, nil
}

// lastLines reads the end of a file without holding all of it.
//
// ⚠ It reads a window from the end and grows it until it has enough newlines, so
// a 400 MiB transcript costs one small read rather than a full scan.
func lastLines(f *os.File, n int) ([]string, error) {
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

type transcriptRow struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Modified time.Time `json:"modified"`
	Stdout   int64     `json:"stdout_bytes"`
	Stderr   int64     `json:"stderr_bytes"`
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
	fmt.Fprintf(os.Stderr, "  %-18s %-20s %10s %10s\n", "job", "when", "stdout", "stderr")
	for _, r := range rows {
		fmt.Printf("  %-18s %-20s %10s %10s\n", r.ID, r.Modified.Format("2006-01-02 15:04:05"),
			toolkit.HumanBytes(r.Stdout), toolkit.HumanBytes(r.Stderr))
	}
	fmt.Fprintf(os.Stderr, "\n  %d transcript(s). wsl-toolkit logs ID writes one.\n", len(rows))
	return exitOK, nil
}
