package main

import (
	"context"
	"os"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// ⭐ One routing decision at the top of each command, and the two paths share
// everything below it: the same job data goes over the protocol and the helper
// runs the same functions.
//
// ⛔ What is not shared is anything touching this machine's filesystem. The
// client builds the workspace archive and extracts the artifacts itself, with
// the same validation, so the helper never opens a path a caller named.

func helperRunJob(ctx context.Context, c *toolkit.HelperClient, j jobFlags, ref, label string, payload []byte, env map[string]string) (toolkit.JobResult, error) {
	req := toolkit.HelperRunRequest{
		Image: ref, ScriptB64: toolkit.EncodeScript(payload), Env: env,
		TimeoutMS: j.timeout.Milliseconds(), Network: !j.noNetwork,
		Platform: j.platform, ContainerLifecycle: j.lifecycle,
		Artifacts: j.artifactDir != "", MaxBytes: j.maxBytes, MaxEntries: j.maxEntries,
		User: j.user, MaxOutput: j.maxOutput, TickMS: j.tick.Milliseconds(),
	}
	if j.workspace != "" {
		note("uploading the workspace to the helper")
		id, err := c.UploadWorkspace(ctx, j.workspace, j.limits(), toolkit.SortedExcludes(j.excludes))
		if err != nil {
			return toolkit.JobResult{}, err
		}
		req.StagingID = id
	}
	liveOut, liveErr := j.sinks()
	// ⭐ The bytes are arriving anyway, so they are written to this machine
	// as well. Without it `wsl-toolkit logs` on the helper route named a
	// directory that exists only under the helper's own state, which is a
	// different path whenever the two do not share WSL_TOOLKIT_HOME.
	spool := newClientSpool(note)
	res, artifactsID, err := c.RunStream(ctx, req, toolkit.HelperSinks{
		Stdout: spool.Tee(liveOut, false), Stderr: spool.Tee(liveErr, true), Log: note,
		Tick: tickPrinter(j),
	})
	if err != nil {
		spool.Discard()
		return res, err
	}
	if local := spool.Finish(res.ID); local != "" {
		res.Transcript = local
	}
	res.Label = label
	if j.artifactDir != "" && artifactsID != "" && res.ArtifactError == "" {
		got, err := c.DownloadArtifacts(ctx, artifactsID, j.artifactDir, j.limits())
		res.Artifacts, res.ArtifactsAttempted = got.Delivered, got.Attempted
		if err != nil {
			// ⛔ A FIELD ON THE RESULT, NOT AN ERROR RETURNED PAST IT. Returning
			// an error here made the caller report exit 2 for a job that ran and
			// discarded the container's own exit code. The verdict reads this
			// field, so both routes reach the same number the same way.
			res.ArtifactError = err.Error()
			// ⭐ AND THE HELPER STILL HAS THE SET. It is not acknowledged when
			// the download fails, so naming it here is what makes the copy
			// recoverable rather than merely existing. WSL-46, issue 24.
			res.RetainedKind, res.Retained = "helper", artifactsID
			return res, nil
		}
	}
	return res, nil
}

func helperRunMatrix(ctx context.Context, c *toolkit.HelperClient, j jobFlags, images []string, parallel int, payload []byte, env map[string]string, transcripts string) (toolkit.MatrixReport, error) {
	req := toolkit.HelperMatrixRequest{
		HelperRunRequest: toolkit.HelperRunRequest{
			ScriptB64: toolkit.EncodeScript(payload), Env: env,
			TimeoutMS: j.timeout.Milliseconds(), Network: !j.noNetwork,
			Platform: j.platform, ContainerLifecycle: j.lifecycle,
			Artifacts: j.artifactDir != "", MaxBytes: j.maxBytes, MaxEntries: j.maxEntries,
			User: j.user, MaxOutput: j.maxOutput, TickMS: j.tick.Milliseconds(),
		},
		Images: images, Parallel: parallel,
	}
	if j.workspace != "" {
		note("uploading the workspace to the helper")
		id, err := c.UploadWorkspace(ctx, j.workspace, j.limits(), toolkit.SortedExcludes(j.excludes))
		if err != nil {
			return toolkit.MatrixReport{}, err
		}
		req.StagingID = id
	}
	report, artifactsID, err := c.MatrixStream(ctx, req, toolkit.HelperSinks{
		Log: note, Row: rowPrinter(), Tick: tickPrinter(j),
	})
	if err != nil {
		return report, err
	}
	if j.artifactDir != "" && artifactsID != "" {
		if _, err := c.DownloadArtifacts(ctx, artifactsID, j.artifactDir, j.limits()); err != nil {
			// Every row asked for output and none of it arrived, so every row
			// carries the failure and the fleet's counts move with them, and
			// every row names the set the helper still holds.
			for i := range report.Rows {
				if report.Rows[i].ArtifactError == "" {
					report.Rows[i].ArtifactError = err.Error()
					report.Rows[i].RetainedKind, report.Rows[i].Retained = "helper", artifactsID
				}
			}
			report.Recount()
		}
	}
	if transcripts != "" {
		if err := os.MkdirAll(transcripts, 0o700); err != nil {
			return report, err
		}
		runner := &transcriptWriter{dir: transcripts}
		for i := range report.Rows {
			if err := runner.write(&report.Rows[i]); err != nil {
				logf("  could not write the transcript for %s: %s", report.Rows[i].Label, err.Error())
			}
		}
	}
	return report, nil
}

// newClientSpool opens one, tolerating a machine with no writable state
// directory: a nil spool writes nothing and every method accepts it.
//
// ⚠ THE LOG IS PASSED IN, NOT OPTIONAL. Tolerating a failure and hiding it
// are different things, and this function used to do both.
func newClientSpool(log func(string)) *toolkit.ClientSpool {
	home, err := toolkit.EnsureHome()
	if err != nil {
		if log != nil {
			log("no local transcript for this job: " + err.Error())
		}
		return nil
	}
	led, err := toolkit.OpenLedger()
	if err != nil {
		led = nil
	}
	return toolkit.NewClientSpool(home, led, log)
}

// transcriptWriter writes a helper-run row the same way the direct path does, so
// a transcript directory does not say which path produced it.
type transcriptWriter struct{ dir string }

func (t *transcriptWriter) write(res *toolkit.JobResult) error {
	return toolkit.WriteJobTranscript(t.dir, res)
}
