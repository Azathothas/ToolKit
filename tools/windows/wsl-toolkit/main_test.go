package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// The cases here cover the dispatch layer's own verdicts: the places where a
// structured answer becomes an exit code. Each is named for the behaviour it
// asserts rather than the function it calls.

func TestACleanupThatCouldNotRemoveSomethingDoesNotExitZero(t *testing.T) {
	plan := toolkit.CleanupPlan{
		Schema:  toolkit.CleanupSchema,
		Removed: []string{"container abc123"},
		Failed:  []string{"directory /home/toolkit/.wsl-toolkit/jobs/j-7"},
	}
	code, err := gcVerdict(plan, nil)
	if code != exitFailed {
		t.Fatalf("a cleanup with one entry it could not remove exited %d, so nothing downstream can tell it apart from a clean run", code)
	}
	if err == nil || !strings.Contains(err.Error(), "j-7") {
		t.Fatalf("the error must name what stayed behind, got %v", err)
	}
}

func TestACleanupThatRemovedEverythingExitsZero(t *testing.T) {
	plan := toolkit.CleanupPlan{Schema: toolkit.CleanupSchema, Removed: []string{"directory /x"}}
	code, err := gcVerdict(plan, nil)
	if code != exitOK || err != nil {
		t.Fatalf("a clean cleanup exited %d with %v", code, err)
	}
}

func TestACleanupThatCouldNotRunReportsItsOwnError(t *testing.T) {
	want := errors.New("the base is not registered")
	code, err := gcVerdict(toolkit.CleanupPlan{Schema: toolkit.CleanupSchema}, want)
	if code != exitFailed || !errors.Is(err, want) {
		t.Fatalf("a cleanup that could not run exited %d with %v", code, err)
	}
}
