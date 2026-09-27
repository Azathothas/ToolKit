// SPDX-License-Identifier: 0BSD

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-94 and WSL-95: the job-control surface --------------------------------

const cliJobID = "0123456789abcdef"

// TestJobControlRefusesWhatItCannotAnswer walks every refusal the three
// commands make before they reach WSL, and requires the refusal's own words.
func TestJobControlRefusesWhatItCannotAnswer(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	ctx := context.Background()
	for _, c := range []struct {
		name string
		run  func() (int, error)
		want string
	}{
		{"stop with no id", func() (int, error) { return cmdStop(ctx, nil) }, "takes a job id"},
		{"stop with a word", func() (int, error) { return cmdStop(ctx, []string{"latest"}) }, "is not a job id"},
		{"stop with a negative grace", func() (int, error) { return cmdStop(ctx, []string{cliJobID, "--grace", "-1s"}) }, "negative"},
		{"wait with no id", func() (int, error) { return cmdWait(ctx, nil) }, "takes a job id"},
		{"wait with a negative timeout", func() (int, error) { return cmdWait(ctx, []string{cliJobID, "--timeout", "-1s"}) }, "negative"},
		{"wait for an id nothing ran", func() (int, error) { return cmdWait(ctx, []string{cliJobID}) }, "no such job"},
		{"follow with no id", func() (int, error) { return cmdLogs(ctx, []string{"--follow"}) }, "reads one job"},
		{"follow one stream", func() (int, error) { return cmdLogs(ctx, []string{cliJobID, "--follow", "--stderr"}) }, "each to its own"},
		{"follow both, in order", func() (int, error) { return cmdLogs(ctx, []string{cliJobID, "--follow", "--both"}) }, "each to its own"},
		{"follow as a document", func() (int, error) { return cmdLogs(ctx, []string{cliJobID, "--follow", "--json"}) }, "not a document"},
		{"base exec --json attached", func() (int, error) { return cmdBaseExec(ctx, []string{"-c", "true", "--json"}) }, "--detach"},
		{"run --detach through the helper", func() (int, error) {
			return cmdRun(ctx, []string{"--image", "alpine", "-c", "true", "--detach", "--via-helper"})
		}, "--detach starts a copy of this program"},
	} {
		code, err := c.run()
		if code != exitCannot || err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s answered %d, %v; want %d and %q", c.name, code, err, exitCannot, c.want)
		}
	}
}

func TestAnUnknownIDIsItsOwnRefusal(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	_, err := jobEnd(context.Background(), cliJobID, 0, nil, nil)
	if !errors.Is(err, toolkit.ErrUnknownJob) {
		t.Fatalf("an id nothing ran answered %v", err)
	}
	if code := jobEndCode(err); code != exitCannot {
		t.Fatalf("an unknown id exits %d", code)
	}
	if code := jobEndCode(context.Canceled); code != 130 {
		t.Fatalf("an interrupted wait exits %d", code)
	}
}

// TestARecordedEndIsTheAnswer: a job whose owner recorded its end is answered
// from the host's record, with the verdict the owner sealed, and no WSL call.
func TestARecordedEndIsTheAnswer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	owner, err := toolkit.ClaimJob(home, cliJobID)
	if err != nil {
		t.Fatal(err)
	}
	res := toolkit.JobResult{ID: cliJobID, Exit: 0, ArtifactError: "a member was refused"}
	res.Seal()
	if err := owner.Finish(res); err != nil {
		t.Fatal(err)
	}
	end, err := jobEnd(context.Background(), cliJobID, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if end.Result == nil || end.verdict != res.EffectiveExit || end.verdict == 0 {
		t.Fatalf("a job that exited 0 and lost its artifacts is answered with %d, %+v", end.verdict, end.Result)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan struct{})
	go func() { _, _ = buf.ReadFrom(r); close(done) }()
	fn()
	_ = w.Close()
	os.Stdout = old
	<-done
	return buf.String()
}

func TestADetachedAnswerSaysWhenTheOwnerStaysInTheJobObject(t *testing.T) {
	home := t.TempDir()
	for _, left := range []bool{true, false} {
		out := captureStdout(t, func() {
			if code, err := reportDetached(home, cliJobID, left, true); code != exitOK || err != nil {
				t.Errorf("the answer exited %d, %v", code, err)
			}
		})
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("the answer is not one document: %q", out)
		}
		if doc["schema"] != DetachedSchema || doc["id"] != cliJobID || doc["kind"] != "job" {
			t.Errorf("the answer is %v", doc)
		}
		_, warned := doc["warning"]
		if warned == left {
			t.Errorf("left the job object %v, and the answer carries a warning: %v", left, warned)
		}
	}
	plain := captureStdout(t, func() { _, _ = reportDetached(home, cliJobID, true, false) })
	if plain != cliJobID+"\n" {
		t.Errorf("without --json the answer is %q, want the id alone on stdout", plain)
	}
}
