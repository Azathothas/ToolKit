package main

import (
	"context"
	"fmt"
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
		Artifacts: j.artifactDir != "", MaxBytes: j.maxBytes, MaxEntries: j.maxEntries,
		User: j.user,
	}
	if j.workspace != "" {
		note("uploading the workspace to the helper")
		id, err := c.UploadWorkspace(ctx, j.workspace, j.limits(), toolkit.SortedExcludes(j.excludes))
		if err != nil {
			return toolkit.JobResult{}, err
		}
		req.StagingID = id
	}
	res, artifactsID, err := c.Run(ctx, req)
	if err != nil {
		return res, err
	}
	res.Label = label
	if j.artifactDir != "" && artifactsID != "" {
		n, err := c.DownloadArtifacts(ctx, artifactsID, j.artifactDir, j.limits())
		if err != nil {
			return res, fmt.Errorf("the job ran and its artifacts could not be fetched: %w", err)
		}
		res.Artifacts = n
	}
	return res, nil
}

func helperRunMatrix(ctx context.Context, c *toolkit.HelperClient, j jobFlags, images []string, parallel int, payload []byte, env map[string]string, transcripts string) (toolkit.MatrixReport, error) {
	req := toolkit.HelperMatrixRequest{
		HelperRunRequest: toolkit.HelperRunRequest{
			ScriptB64: toolkit.EncodeScript(payload), Env: env,
			TimeoutMS: j.timeout.Milliseconds(), Network: !j.noNetwork,
			Artifacts: j.artifactDir != "", MaxBytes: j.maxBytes, MaxEntries: j.maxEntries,
			User: j.user,
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
	report, artifactsID, err := c.Matrix(ctx, req)
	if err != nil {
		return report, err
	}
	if j.artifactDir != "" && artifactsID != "" {
		if _, err := c.DownloadArtifacts(ctx, artifactsID, j.artifactDir, j.limits()); err != nil {
			return report, fmt.Errorf("the fleet ran and its artifacts could not be fetched: %w", err)
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

// transcriptWriter writes a helper-run row the same way the direct path does, so
// a transcript directory does not say which path produced it.
type transcriptWriter struct{ dir string }

func (t *transcriptWriter) write(res *toolkit.JobResult) error {
	return toolkit.WriteJobTranscript(t.dir, res)
}
