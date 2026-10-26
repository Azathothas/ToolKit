// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-170: a container engine inside a job ---------------------------------

// TestAJobWhoseEngineFoundNoFuseIsToldTheNestedShape: a job that failed with the
// words a nested podman measured on a scratch base gave, with no /dev/fuse, is
// told the shape that runs one. Nothing else is.
func TestAJobWhoseEngineFoundNoFuseIsToldTheNestedShape(t *testing.T) {
	failed := toolkit.JobResult{Label: "quay.io/podman/stable", Exit: 127,
		Stderr: "Error: mounting storage: fuse-overlayfs failed\n" + nestedEngineFuse + "\n"}
	hint := nestedEngineHint(failed)
	for _, want := range []string{"no /dev/fuse", "--user", "--device /dev/fuse", "wsl-toolkit examples"} {
		if !strings.Contains(hint, want) {
			t.Errorf("the note for a nested engine with no /dev/fuse does not carry %q: %q", want, hint)
		}
	}
	if out := captureStderr(t, func() { _, _ = reportJob(failed, false) }); !strings.Contains(out, "  ! "+hint+"\n") {
		t.Errorf("run does not print the note:\n%s", out)
	}
	for _, quiet := range []toolkit.JobResult{{Exit: 0, Stderr: nestedEngineFuse}, {Exit: 1, Stderr: "Error: something else\n"}} {
		if got := nestedEngineHint(quiet); got != "" {
			t.Errorf("a job with exit %d and stderr %q is told %q", quiet.Exit, quiet.Stderr, got)
		}
	}
}

// TestTheNestedEngineExampleIsTheMeasuredShape: `examples` prints the shape a
// scratch base ran a nested podman in, rootless inside the base's own engine
// with /dev/fuse and no added capability, and each of its flags is one `run`
// takes.
func TestTheNestedEngineExampleIsTheMeasuredShape(t *testing.T) {
	var command string
	for _, e := range examples {
		if strings.Contains(e.What, "container engine inside a job") {
			command = e.Command
		}
	}
	if command == "" {
		t.Fatal("examples carries no container engine inside a job")
	}
	flags, _, ok := strings.Cut(command, " -c ")
	if !ok || !strings.HasPrefix(flags, "wsl-toolkit run ") {
		t.Fatalf("the nested example is not one run with a command: %s", command)
	}
	words := strings.Fields(strings.TrimPrefix(flags, "wsl-toolkit run "))
	joined := " " + strings.Join(words, " ") + " "
	for _, want := range []string{" --user podman ", " --device /dev/fuse "} {
		if !strings.Contains(joined, want) {
			t.Errorf("the nested example does not carry %q: %s", strings.TrimSpace(want), command)
		}
	}
	if strings.Contains(joined, "--cap-add") {
		t.Errorf("the nested example adds a capability the measured shape did not need: %s", command)
	}
	sets, err := collectManualFlagSets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	run := sets["run"]
	if run == nil {
		t.Fatal("run registered no flag set")
	}
	for _, w := range words {
		if name, ok := strings.CutPrefix(w, "--"); ok && run.Lookup(name) == nil {
			t.Errorf("the nested example passes --%s, which run does not take", name)
		}
	}
}
