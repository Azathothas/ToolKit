// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-116: a printed command names the instance it was printed for --------

// ⛔ Run as printed, a hint without the instance reached the default one, which
// has never heard of the job, and answered "no such job".
func TestAJobHintNamesTheInstanceItWasPrintedFor(t *testing.T) {
	previous := toolkit.SelectedInstance
	t.Cleanup(func() { toolkit.SelectedInstance = previous })
	toolkit.SelectedInstance = toolkit.Instance{Name: "podbox", Distro: "wsl-toolkit-podbox"}
	const want = "wsl-toolkit --instance podbox "

	follow, wait, stop := jobHints(cliJobID)
	hints := map[string]string{
		"follow":         follow,
		"wait":           wait,
		"stop":           stop,
		"not registered": errNotRegistered("wsl-toolkit-podbox").Error(),
		"transcript":     transcriptHint(toolkit.JobResult{ID: cliJobID, Transcript: "x"}),
		// ⛔ A helper lives in its instance's state, and the advice for a refused
		// wsl.exe started the default one's. A contract review found it.
		"denied advice": deniedAdvice(),
	}
	for name, h := range hints {
		if !strings.Contains(h, want) {
			t.Errorf("the %s hint is %q, which another instance's caller would run against the default one", name, h)
		}
	}
	if follow != want+"logs "+cliJobID+" --follow" {
		t.Errorf("the follow hint is %q", follow)
	}

	// ⭐ The detached answer is what a program reads and runs.
	var code int
	out := captureStdout(t, func() { code, _ = reportDetached(t.TempDir(), cliJobID, true, true) })
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil || code != exitOK {
		t.Fatalf("the detached answer did not parse (%d): %v\n%s", code, err, out)
	}
	for _, k := range []string{"follow", "wait", "stop"} {
		if s, _ := doc[k].(string); !strings.HasPrefix(s, want) {
			t.Errorf("the detached answer's %s is %q", k, s)
		}
	}

	// ⚠ The default instance's spelling is unchanged, so no caller of it breaks.
	toolkit.SelectedInstance = toolkit.Instance{Name: toolkit.DefaultInstance}
	if follow, _, _ := jobHints(cliJobID); follow != "wsl-toolkit logs "+cliJobID+" --follow" {
		t.Errorf("the default instance's follow hint became %q", follow)
	}
}

// ⛔ `shipped cat --help` printed no NAME, so the help for a form that takes a
// name never said it took one. WSL-108, and a contract review.
func TestShippedCatHelpSaysItTakesAName(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	code, runErr := cmdShipped(context.Background(), []string{"cat", "--help"})
	_ = w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	if runErr != nil && !errors.Is(runErr, flag.ErrHelp) {
		t.Fatalf("shipped cat --help answered %d, %v", code, runErr)
	}
	if !strings.Contains(string(out), "cat NAME") {
		t.Fatalf("shipped cat --help printed %q, which names no NAME", out)
	}
}

// ⭐ The refusal for an id nothing here belongs to names where this instance's
// jobs are listed.
func TestAnUnknownJobNamesWhereThisInstancesJobsAreListed(t *testing.T) {
	previous := toolkit.SelectedInstance
	t.Cleanup(func() { toolkit.SelectedInstance = previous })
	toolkit.SelectedInstance = toolkit.Instance{Name: "podbox", Distro: "wsl-toolkit-podbox"}
	_, err := toolkit.CleanupPolicy{Job: cliJobID}.Unmatched(nil)
	if !errors.Is(err, toolkit.ErrUnknownJob) || !strings.Contains(err.Error(), "wsl-toolkit --instance podbox logs") {
		t.Fatalf("an unknown job answered %v", err)
	}
}
