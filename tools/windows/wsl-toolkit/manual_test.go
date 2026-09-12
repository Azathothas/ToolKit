// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// ⛔ THE CLAIM AUDIT, MADE MECHANICAL. Reading the manual against the binary is a
// review lens, and one of the three this repository runs; a lens is performed by
// a person and skipped by a session in a hurry. `--user` was in the code and in
// neither manual, and only a hand pass found it. This asserts the same thing on
// every gate run, on both hosts, without anybody remembering to look.
//
// ⭐ IT READS THE REAL FLAG SETS. Every flag registers itself simply by being
// created, so a flag added tomorrow is covered without this file being touched.
//
// ⛔ THAT SENTENCE WAS ONCE TRUE OF FLAGS AND FALSE OF COMMANDS, and it took a
// new command to notice. The list below used to name every command by hand, so
// `inspect` arrived with two flags and this case stayed green over both while
// claiming to cover them. The top level is walked from main.go's own dispatch
// table now, and a command missing from it cannot reach a caller either.
//
// ⚠ SUBCOMMANDS ARE STILL BY HAND, because their flag sets are built inside
// their parent's dispatch and there is no table to walk. That is the residue
// of the same hole, it is smaller, and it is named rather than left implied.

func TestGeneratedManPageIsCurrent(t *testing.T) {
	want, err := os.ReadFile("wsl-toolkit.1")
	if err != nil {
		t.Fatalf("the generated man page could not be read: %v", err)
	}
	got, err := renderManPage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatal("wsl-toolkit.1 is not current. Run: go run . man --output wsl-toolkit.1")
	}
}

func TestReadableManualComesFromCLI(t *testing.T) {
	body, err := renderManualText(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--container-lifecycle", "--platform", "--no-pager", "wsl-toolkit examples"} {
		if !strings.Contains(body, want) {
			t.Errorf("the readable manual does not contain %q", want)
		}
	}
	if strings.Contains(body, ".TH WSL-TOOLKIT") {
		t.Error("the readable manual contains roff control text")
	}
}

// TestGeneratedManPageIsText asserts the one thing a drift check structurally
// cannot.
//
// ⛔ TestGeneratedManPageIsCurrent COMPARES GENERATED OUTPUT WITH GENERATED
// OUTPUT, so a generator that emits the wrong bytes agrees with itself and the
// gate stays green. It did: roff's font escape is `\fB` and Go's form feed is
// `\f`, spelled identically inside an interpreted string literal, so the SEE
// ALSO line shipped 0x0C where it meant to change font.
//
// ⚠ The tree's own control-byte rule did not catch it either, because the
// generated page was not yet tracked when it was written. A rule that runs over
// the tracked set is blind for exactly as long as a new file stays untracked,
// which is the window a generator lands in.
func TestGeneratedManPageIsText(t *testing.T) {
	page, err := renderManPage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range page {
		if r == '\n' {
			continue
		}
		if r < 0x20 || r == 0x7f {
			t.Fatalf("the generated man page carries control byte %#x at offset %d", r, i)
		}
	}
}

// TestANoteDoesNotChangeTheVerdict is the case for a regression this session
// shipped and caught by running the command rather than by reading it.
//
// ⛔ A capability finding written into Problems turns a machine that runs
// containers perfectly well into `not-ready`. `ready` is the FIRST command an
// agent runs, so that answer stops it from doing anything at all, over a
// limitation most jobs never reach. WSL-60.
func TestANoteDoesNotChangeTheVerdict(t *testing.T) {
	base := func() ReadyReport {
		r := ReadyReport{}
		r.Base.Healthy = true
		r.Route.Selected = "direct"
		return r
	}

	r := base()
	r.Notes = append(r.Notes, "this base has no cgroup delegation, so a memory limit is not enforced")
	r.settleVerdict()
	if !r.Ready || r.Verdict != "ready" {
		t.Fatalf("a note made the verdict %q (ready=%v); a note is true and stops nothing", r.Verdict, r.Ready)
	}

	// ⛔ AND THE OTHER HALF: a real problem MUST still refuse. A test that only
	// asserted the note is harmless would pass over a verdict that can no longer
	// say no at all.
	p := base()
	p.Problems = append(p.Problems, "the engine cannot run a container")
	p.settleVerdict()
	if p.Ready || p.Verdict == "ready" {
		t.Fatalf("a problem left the verdict %q (ready=%v); Problems is what refuses", p.Verdict, p.Ready)
	}
}

// TestReadyAnswersWithTheCommandThatHelps holds the rule that `ready` must not
// answer a failed `base ensure` with `base ensure`.
//
// ⛔ A base whose engine has stale run state REFUSES an unflagged ensure on
// purpose and names --repair. Sending the reader back to the command that just
// refused them is the loop this whole shape exists to remove. WSL-61.
func TestReadyAnswersWithTheCommandThatHelps(t *testing.T) {
	// ⚠ THE REAL SHAPE, which is the refusal WRAPPING what podman said. The
	// first version of this case used the refusal alone, and it failed: the
	// classifier reads the engine's words, and a refusal that drops them cannot
	// be recognised as the condition it refuses over.
	stale := "the engine's run state is stale and --repair was not given. " +
		"Run: wsl-toolkit base ensure --repair (a container did not run as toolkit (exit 125): " +
		"Error: current system boot ID differs from cached boot ID)"

	// From the error alone, which is what the ensure path holds.
	if got := baseRemediationFor(toolkit.BaseState{}, errors.New(stale)); got != "wsl-toolkit base ensure --repair" {
		t.Errorf("from the error: got %q, want the repair command", got)
	}

	// From the state, which is what the unhealthy path holds.
	st := toolkit.BaseState{Remediations: []toolkit.Remediation{{
		ID: "stale-run-state", Command: "wsl-toolkit base ensure --repair", Repairable: true,
	}}}
	if got := baseRemediationFor(st, nil); got != "wsl-toolkit base ensure --repair" {
		t.Errorf("from the state: got %q, want the repair command", got)
	}

	// ⛔ A finding this tool CANNOT repair must not become the command to run.
	// Offering one would send a caller to run something that fixes nothing.
	unfixable := toolkit.BaseState{Remediations: []toolkit.Remediation{{
		ID: "cgroup-delegation", Command: "wsl-toolkit base status --probe --json", Repairable: false,
	}}}
	if got := baseRemediationFor(unfixable, nil); got != "wsl-toolkit base ensure" {
		t.Errorf("an unrepairable finding became the next command: %q", got)
	}

	// And the ordinary case is unchanged.
	if got := baseRemediationFor(toolkit.BaseState{}, nil); got != "wsl-toolkit base ensure" {
		t.Errorf("the plain case moved: %q", got)
	}
}

// TestAFindingGoesInTheBucketThatDoesNotRefuse covers the CALL SITE, which the
// verdict case above cannot see.
//
// ⚠ THE FIRST VERSION OF THAT CASE WAS THEATRE and the mutation harness said so:
// it constructed a report and called settleVerdict directly, so removing the
// guard at the call site left it green. A case that asserts a rule and never
// reaches the code that applies it is a case whose name claims more than it
// checks.
func TestAFindingGoesInTheBucketThatDoesNotRefuse(t *testing.T) {
	rems := []toolkit.Remediation{
		{ID: "cgroup-delegation", What: "no delegation", Costs: "limits are not enforced", Repairable: false},
		{ID: "stale-run-state", What: "stale run state", Costs: "nothing runs", Repairable: true},
	}
	notes := readyNotes(rems)
	if len(notes) != 1 {
		t.Fatalf("readyNotes gave %d note(s), want 1: %q", len(notes), notes)
	}
	if !strings.Contains(notes[0], "no delegation") {
		t.Errorf("the note is not the unrepairable finding: %q", notes[0])
	}
	if strings.Contains(notes[0], "stale run state") {
		t.Error("a repairable finding became a note; it is already the command to run")
	}
	// ⛔ AND THE NOTES MUST REACH A REPORT THAT IS STILL READY. This is the half
	// the verdict case proves, asserted here against the value this function
	// actually produced rather than against one written by hand.
	r := ReadyReport{}
	r.Base.Healthy = true
	r.Route.Selected = "direct"
	r.Notes = append(r.Notes, notes...)
	r.settleVerdict()
	if !r.Ready {
		t.Fatalf("a machine that runs containers reported %q", r.Verdict)
	}
}
