package toolkit

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

// MatrixSpec is one command run across a set of images.
type MatrixSpec struct {
	Images      []Image
	Script      []byte
	Workspace   string
	Excludes    []string
	ArtifactDir string
	Env         map[string]string
	Timeout     time.Duration // per row
	Network     bool
	Parallel    int
	Limits      WorkspaceLimits
	Transcripts string
	User        string
	MaxOutput   int64
	// OnRow is called once per row, THE MOMENT IT FINISHES rather than when the
	// fleet does.
	//
	// ⛔ A twelve-image fleet printed nothing for its whole duration and then
	// printed a table. A caller could not tell a slow pull from a hung one, and
	// over the helper it could not even see the row labels. It is called from
	// several goroutines, so an implementation locks.
	OnRow func(JobResult)
}

// MatrixReport is what a fleet run produced.
//
// ⛔ Three counts, because "the image could not be pulled" and "the subject is
// broken" need different next moves.
type MatrixReport struct {
	Schema    string        `json:"schema"`
	Started   time.Time     `json:"started"`
	Duration  time.Duration `json:"duration_ns"`
	Rows      []JobResult   `json:"rows"`
	Ran       int           `json:"ran"`
	Failed    int           `json:"failed"`
	Unreached int           `json:"unreached"`
	TimedOut  int           `json:"timed_out"`
}

// MatrixSchema versions the report, because its reader is a program.
const MatrixSchema = "wsl-toolkit-matrix/1"

// DefaultParallel is how many rows run at once. ⚠ Four rather than one per
// image: every row is a container inside one WSL distribution sharing one
// utility VM's memory, and the out-of-memory killer reports exit 137 for a
// reason that has nothing to do with the subject.
const DefaultParallel = 4

// Verdict is the exit code a fleet run reports.
//
//	0  every row ran and every row passed
//	1  every row ran and at least one disagreed
//	2  NOTHING ran, which reads exactly like everything agreeing and is not
func (m MatrixReport) Verdict() int {
	if m.Ran == 0 {
		return 2
	}
	if m.Failed > 0 || m.Unreached > 0 {
		return 1
	}
	return 0
}

// Recount derives the four counts from the rows.
//
// ⛔ THE ONLY PLACE THEY ARE COMPUTED. A caller that changes a row after the
// fleet returned - the helper route marking every row when one download failed -
// calls this rather than adjusting a number, because a count edited beside the
// row it describes is a count that stops matching it.
func (m *MatrixReport) Recount() {
	m.Ran, m.Failed, m.Unreached, m.TimedOut = 0, 0, 0, 0
	for _, row := range m.Rows {
		switch {
		case row.Unreached:
			m.Unreached++
		case row.TimedOut:
			m.Ran++
			m.TimedOut++
			m.Failed++
		case row.Failed():
			// Failed(), not Exit. A row whose container succeeded and whose
			// artifacts could not be fetched delivered nothing, and counting it
			// under a green fleet is what let a build lose its output silently.
			m.Ran++
			m.Failed++
		default:
			m.Ran++
		}
	}
}

// RunMatrix commissions a container per image, runs one command in each, and
// decommissions all of them.
//
// ⭐ The workspace is sent once and copied per row inside the distribution. Each
// row still gets its OWN copy: two rows sharing a directory is two rows able to
// change each other's result.
func (r *Runner) RunMatrix(ctx context.Context, spec MatrixSpec) (MatrixReport, error) {
	started := time.Now()
	report := MatrixReport{Schema: MatrixSchema, Started: started.UTC()}
	if len(spec.Images) == 0 {
		return report, fmt.Errorf("a fleet over no images cannot report a result")
	}
	parallel := spec.Parallel
	if parallel <= 0 {
		parallel = DefaultParallel
	}
	if parallel > len(spec.Images) {
		parallel = len(spec.Images)
	}
	limits := spec.Limits
	if limits.MaxBytes == 0 {
		limits = DefaultWorkspaceLimits()
	}

	staged := ""
	if spec.Workspace != "" {
		id, err := newJobID()
		if err != nil {
			return report, err
		}
		guestHome, err := r.guestHome(ctx)
		if err != nil {
			return report, err
		}
		staged = guestHome + "/" + GuestRoot + "/staging/" + id
		stagingRoot := guestHome + "/" + GuestRoot + "/staging"
		if err := r.ledger.Append(LedgerEntry{
			Event: "open", Kind: "staging", ID: id, Distro: r.cfg.Base.Name, GuestDir: staged,
		}); err != nil {
			return report, err
		}
		defer func() {
			tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
			defer cancel()
			if err := r.removeGuestDir(tctx, stagingRoot, staged); err != nil {
				r.log("could not remove the staged workspace: " + err.Error())
				return
			}
			if err := r.ledger.Append(LedgerEntry{Event: "close", Kind: "staging", ID: id}); err != nil {
				r.log("could not close the staging record: " + err.Error())
			}
		}()
		if _, err := r.wsl.SendWorkspace(ctx, r.cfg.Base.Name, r.cfg.Base.User, staged, spec.Workspace, limits, spec.Excludes, r.log); err != nil {
			return report, err
		}
	}

	rows := make([]JobResult, len(spec.Images))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, img := range spec.Images {
		wg.Add(1)
		go func(i int, img Image) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r.log(fmt.Sprintf("row %d/%d: %s", i+1, len(spec.Images), img.ID))
			artifacts := ""
			if spec.ArtifactDir != "" {
				artifacts = joinHostPath(spec.ArtifactDir, img.ID)
			}
			rows[i] = r.Run(ctx, JobSpec{
				Image: img.Ref, Script: spec.Script, StagedFrom: staged,
				Workspace: "", ArtifactDir: artifacts, Env: spec.Env,
				Timeout: spec.Timeout, Network: spec.Network, Limits: limits,
				Label: img.ID, User: spec.User, MaxOutput: spec.MaxOutput,
			})
			if spec.Transcripts != "" {
				if err := r.WriteTranscript(spec.Transcripts, &rows[i]); err != nil {
					r.log("could not write the transcript for " + img.ID + ": " + err.Error())
				}
			}
			if spec.OnRow != nil {
				spec.OnRow(rows[i])
			}
		}(i, img)
	}
	wg.Wait()

	report.Rows = rows
	report.Recount()
	report.Duration = time.Since(started)
	sort.SliceStable(report.Rows, func(i, j int) bool { return report.Rows[i].Label < report.Rows[j].Label })
	return report, nil
}

// removeGuestDir deletes one directory inside the guest, with the containment
// test made by the guest itself.
func (r *Runner) removeGuestDir(ctx context.Context, root, dir string) error {
	out, stderr, code, err := r.baseCapture(ctx, []byte(GuestRemoveScript(root, dir)), 5*time.Minute)
	if err != nil || code != 0 {
		return fmt.Errorf("exit %d: %s", code, firstLine(stderr+out))
	}
	return nil
}

func joinHostPath(parts ...string) string {
	out := parts[0]
	for _, p := range parts[1:] {
		out = out + string(pathSeparator) + p
	}
	return out
}

// RenderMatrix writes the human-readable table.
func RenderMatrix(w io.Writer, report MatrixReport) error {
	width := 6
	for _, row := range report.Rows {
		if len(row.Label) > width {
			width = len(row.Label)
		}
	}
	for _, row := range report.Rows {
		state := "ok"
		switch {
		case row.Unreached:
			state = "unreached"
		case row.TimedOut:
			state = "timeout"
		case row.Exit != 0:
			state = fmt.Sprintf("exit %d", row.Exit)
		}
		if _, err := fmt.Fprintf(w, "  %-*s  %-10s %8s  %s\n",
			width, row.Label, state, row.Duration.Round(time.Millisecond), row.Image); err != nil {
			return err
		}
		if row.Error != "" {
			if _, err := fmt.Fprintf(w, "  %-*s  %s\n", width, "", row.Error); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintf(w, "\n  %d ran, %d failed, %d unreached, %d timed out, in %s\n",
		report.Ran, report.Failed, report.Unreached, report.TimedOut, report.Duration.Round(time.Second)); err != nil {
		return err
	}
	// ⭐ ONE LINE FOR THE WHOLE FLEET, not one per row. A twelve-row table
	// followed by twelve paths is a table nobody reads, and the id of each row is
	// what `logs` with no arguments lists.
	if TranscriptsKept(report) {
		if _, err := fmt.Fprintf(w, "  each row's complete output is kept: wsl-toolkit logs\n"); err != nil {
			return err
		}
	}
	return nil
}

// TranscriptsKept reports whether any row in this report left output behind to
// read back, so a caller can say so once rather than per row.
func TranscriptsKept(report MatrixReport) bool {
	for _, row := range report.Rows {
		if row.Transcript != "" {
			return true
		}
	}
	return false
}
