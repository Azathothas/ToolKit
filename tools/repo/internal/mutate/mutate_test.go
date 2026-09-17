// mutate_test.go - the harness itself, which is the thing nobody else checks.
//
// ⛔ THE FAILURE MODE THIS COVERS IS THE ONE THAT ALREADY HAPPENED. An earlier
// version of this harness reported "green with the guard gone" for three
// different things: a test that genuinely did not cover its subject, a `-run`
// pattern that matched NO test, and a mutation that did not COMPILE. Only the
// first is a finding. The other two prove nothing, and reading them as a pass is
// how a session concludes its guards are real when three of them are not.
//
// ⚠ AND A FOURTH, ADDED ON 2026-09-10 AFTER IT MISFIRED THE OTHER WAY: a case
// that skips itself was reported as THEATRE, which accuses a good test of being
// empty because the host could not run it.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTable(t *testing.T, tb Table) string {
	t.Helper()
	raw, err := json.Marshal(tb)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "mutations.json")
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestLoadRefusesATableThatWouldProveNothing.
//
// ⛔ AN EMPTY TABLE IS NOT A CLEAN RUN. A harness that checks nothing and
// reports success is the shape of every dead check this tree has found.
func TestLoadRefusesATableThatWouldProveNothing(t *testing.T) {
	// ⚠ BOUND TO VARIABLES rather than written inline. A Go composite
	// literal of slice-of-struct opens with `{{`, which this tree's placeholder
	// check reads as an unfilled template. Binding is the fix; widening that
	// guard to allow `{{` would be the wrong half to change.
	one := Mutation{Label: "x"}
	cases := []struct {
		name  string
		table Table
		want  string
	}{
		{"no mutations at all", Table{Schema: TableSchema}, "no mutations"},
		{"a schema this build does not read", Table{Schema: "repo-mutations/99", Mutations: []Mutation{one}}, "schema"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Load(writeTable(t, c.table))
			if err == nil {
				t.Fatal("the table was accepted and nothing would have been proved")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("the refusal does not say why: %v", err)
			}
		})
	}
}

func TestLoadRefusesAMissingTable(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("a table that is not there was read as an empty success")
	}
}

// fixture writes a tiny module with one guard and one case for it.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "mod")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod":        "module example.test\n\ngo 1.25.0\n",
		"guard.go":      "package guard\n\nfunc Allowed(s string) bool {\n\tif s == \"\" {\n\t\treturn false\n\t}\n\treturn true\n}\n",
		"guard_test.go": "package guard\n\nimport \"testing\"\n\nfunc TestEmptyIsRefused(t *testing.T) {\n\tif Allowed(\"\") {\n\t\tt.Fatal(\"the empty string was allowed\")\n\t}\n}\n",
		// A case that excuses itself, which is what a platform-bound guard
		// does on the host that cannot run it. ⛔ From outside the process it
		// is byte for byte a case that ran and passed.
		"skip_test.go": "package guard\n\nimport \"testing\"\n\nfunc TestSkipsHere(t *testing.T) {\n\tt.Skip(\"this host cannot run it\")\n}\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestRunTellsTheFourOutcomesApart is the case for the whole design.
//
// ⚠ IT WAS THREE UNTIL 2026-09-10. The skipped row is the fourth, and it was
// added after a real row was reported as THEATRE on a host that could not run
// its case at all.
func TestRunTellsTheFourOutcomesApart(t *testing.T) {
	root := fixture(t)
	base := Mutation{Module: "mod", File: "guard.go", Run: "TestEmptyIsRefused"}

	real := base
	real.Label = "the empty-string guard"
	real.Find = "if s == \"\" {"
	real.Replace = "if false {"

	theatre := base
	theatre.Label = "a change no case looks at"
	theatre.Find = "return true\n"
	theatre.Replace = "return true // nothing asserts on this\n"

	broken := base
	broken.Label = "a mutation that does not compile"
	broken.Find = "return true\n"
	broken.Replace = "return nope\n"

	missing := base
	missing.Label = "a find that is not there any more"
	missing.Find = "this text does not appear"
	missing.Replace = "x"

	noCase := base
	noCase.Label = "a run pattern matching no test"
	noCase.Find = "if s == \"\" {"
	noCase.Replace = "if false {"
	noCase.Run = "TestNoSuchCaseAnywhere"

	// ⛔ THE ROW THAT LOOKS EXACTLY LIKE THEATRE FROM OUTSIDE. The mutation
	// compiles, a case runs, and the process exits 0. The only thing telling
	// the two apart is the SKIP line, and this is the case that proves the
	// harness reads it.
	skipped := base
	skipped.Label = "a guard whose case this host cannot run"
	skipped.Find = "return true\n"
	skipped.Replace = "return true // nothing asserts on this\n"
	skipped.Run = "TestSkipsHere"

	tb := &Table{Schema: TableSchema, Mutations: []Mutation{real, theatre, broken, missing, noCase, skipped}}
	var log bytes.Buffer
	got, err := Run(root, tb, "", &log)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ok", "THEATRE", "BROKEN", "BROKEN", "BROKEN", "SKIPPED"}
	if len(got) != len(want) {
		t.Fatalf("got %d verdicts, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].State != want[i] {
			t.Errorf("%q: state %q, want %q (%s)", got[i].Label, got[i].State, want[i], got[i].Reason)
		}
	}
	if got[0].Cases != 1 {
		t.Errorf("the proved row counted %d case(s), want 1", got[0].Cases)
	}
	// ⛔ The three BROKEN rows must not be indistinguishable from each other:
	// "did not compile", "matched 0 times" and "0 cases matched" are different
	// mistakes with different fixes.
	reasons := got[2].Reason + "|" + got[3].Reason + "|" + got[4].Reason
	for _, want := range []string{"compile", "matched 0 times", "0 cases"} {
		if !strings.Contains(reasons, want) {
			t.Errorf("no BROKEN row says %q: %s", want, reasons)
		}
	}
	// ⛔ AND THE SKIPPED ROW COUNTED ITS SKIP. A state of "SKIPPED" reached by
	// any other route would satisfy the table above and prove nothing.
	if got[5].Skipped != 1 || got[5].Cases != 1 {
		t.Errorf("the skipped row counted %d case(s) and %d skip(s), want 1 and 1", got[5].Cases, got[5].Skipped)
	}
}

// TestReportFailsOnEveryRowThatDisagrees. ⚠ A THEATRE row is a test to fix and
// a BROKEN row is a claim nobody is checking. Neither is a pass.
//
// ⛔ AND A SKIPPED ROW IS NEITHER, which is the case worth having. It is not
// proved, so it must not be counted as proved; it is not wrong, so it must not
// fail the run. A harness that failed on it would be red on the operator's host
// forever, and a harness that is always red is one nobody reads.
func TestReportFailsOnEveryRowThatDisagrees(t *testing.T) {
	// ⚠ BOUND TO VARIABLES rather than written inline. A Go composite
	// literal of slice-of-struct opens with `{{`, which this tree's placeholder
	// check reads as an unfilled template. Binding is the fix; widening that
	// guard to allow `{{` would be the wrong half to change.
	proved := Verdict{Label: "a", State: "ok"}
	cases := []struct {
		name     string
		verdicts []Verdict
		want     int
	}{
		{"everything proved", []Verdict{proved}, 0},
		{"one theatre", []Verdict{proved, {Label: "b", State: "THEATRE"}}, 1},
		{"one broken", []Verdict{proved, {Label: "b", State: "BROKEN", Reason: "why"}}, 1},
		{"one skipped", []Verdict{proved, {Label: "b", State: "SKIPPED", Reason: "not on this host"}}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			got := Report(&out, c.verdicts)
			if got != c.want {
				t.Fatalf("Report returned %d, want %d:\n%s", got, c.want, out.String())
			}
			// ⛔ EXIT 0 IS NOT ENOUGH FOR THE SKIPPED ROW. A run that passes it
			// over in silence has told the reader every guard was proved, which
			// is the false green this file exists to refuse.
			if c.name == "one skipped" {
				if !strings.Contains(out.String(), "SKIPPED: b") {
					t.Fatalf("the tally does not name the unproved row:\n%s", out.String())
				}
				if !strings.Contains(out.String(), "1 of 2 guards proved") {
					t.Fatalf("the skipped row was counted as proved:\n%s", out.String())
				}
			}
		})
	}
}

// TestRunRefusesAFilterThatSelectsNothing. ⛔ Zero rows run and exit 0 is the
// answer this whole file exists to prevent.
func TestRunRefusesAFilterThatSelectsNothing(t *testing.T) {
	only := Mutation{Label: "a guard"}
	tb := &Table{Schema: TableSchema, Mutations: []Mutation{only}}
	var out bytes.Buffer
	if _, err := Run(t.TempDir(), tb, "no such label", &out); err == nil {
		t.Fatal("a filter that selected nothing was reported as a clean run")
	}
}

// TestARowWhoseCaseWasAlreadyRedIsRefused is finding 1, the oldest in the
// record, and it is a defect in the instrument that proves every other guard.
//
// ⛔ WHAT WAS WRONG. The harness deleted a guard, ran the case, saw red and
// reported "went red". It never asked what the case did BEFORE the mutation, so
// a case that was already failing certified every guard it was named by. That
// is a harness reporting a result it did not measure, which is the exact class
// it exists to catch.
func TestARowWhoseCaseWasAlreadyRedIsRefused(t *testing.T) {
	root := fixture(t)
	// A second case in the same package that fails on its own, and a row whose
	// -run pattern reaches it. The guard it names is real and its removal would
	// genuinely be caught; the point is that this run cannot tell.
	dir := filepath.Join(root, "mod")
	broken := "package guard\n\nimport \"testing\"\n\nfunc TestAlreadyRed(t *testing.T) {\n\tt.Fatal(\"this case was red before anybody mutated anything\")\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "red_test.go"), []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}

	m := Mutation{
		Label: "a guard whose case was already failing", Module: "mod", File: "guard.go",
		Run: "TestAlreadyRed", Find: "if s == \"\" {", Replace: "if false {",
	}
	tb := &Table{Schema: TableSchema, Mutations: []Mutation{m}}
	var log bytes.Buffer
	got, err := Run(root, tb, "", &log)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d verdicts", len(got))
	}
	// ⛔ THE VERDICT THIS USED TO GIVE WAS "ok". Anything but BROKEN here means
	// the harness is certifying a guard over a case that proves nothing.
	if got[0].State != "BROKEN" {
		t.Fatalf("a row over an already-red case reported %q, not BROKEN", got[0].State)
	}
	if !strings.Contains(got[0].Reason, "not green BEFORE") {
		t.Fatalf("the refusal does not say the case was already red: %q", got[0].Reason)
	}
	// ⭐ AND THE REASON CARRIES THE CASE'S OWN FIRST FAILING LINE, file and
	// number, or a reader cannot tell which of several cases the pattern
	// reached was the failing one.
	if !strings.Contains(got[0].Reason, "red_test.go") {
		t.Fatalf("the refusal does not name the failing case: %q", got[0].Reason)
	}
}

// TestABrokenRowSaysWhatTheCompilerSaid is finding 41.
//
// ⛔ A row whose mutation stops the module compiling is neither red nor green,
// and it reported the bare words "does not compile" while DISCARDING the
// compiler's output. Met three times in one day, every time because deleting a
// guard left the variable it read unused, and every time the session had to
// reproduce the build by hand to find that out.
func TestABrokenRowSaysWhatTheCompilerSaid(t *testing.T) {
	root := fixture(t)
	m := Mutation{
		Label: "a mutation that does not compile", Module: "mod", File: "guard.go",
		Run: "TestEmptyIsRefused", Find: "return true\n", Replace: "return nope\n",
	}
	tb := &Table{Schema: TableSchema, Mutations: []Mutation{m}}
	var log bytes.Buffer
	got, err := Run(root, tb, "", &log)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].State != "BROKEN" {
		t.Fatalf("state %q, want BROKEN", got[0].State)
	}
	// ⭐ THE ROW IS NAMED AS THE THING TO REWRITE, not the tree.
	if !strings.Contains(got[0].Reason, "this row proves nothing") {
		t.Fatalf("the reason does not say the row is at fault: %q", got[0].Reason)
	}
	// ⛔ AND THE COMPILER'S OWN LINE IS THERE. "does not compile" alone is what
	// this case exists to refuse.
	if !strings.Contains(got[0].Reason, "nope") && !strings.Contains(got[0].Reason, "guard.go") {
		t.Fatalf("the reason does not carry what the compiler said: %q", got[0].Reason)
	}
}

// TestAKilledRunsStagedCopyIsSweptOnTheNextRun is finding 32.
//
// ⛔ `one` stages a copy of the module and removes it with a defer, which a
// killed process never runs. One was measured at 127 MiB, left since
// 2026-09-14, and `wsl-toolkit gc` does not cover it because the directory is
// not that tool's job state.
func TestAKilledRunsStagedCopyIsSweptOnTheNextRun(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	if os.TempDir() != tmp {
		t.Skipf("this host resolves TempDir to %s rather than the fixture", os.TempDir())
	}
	stale := filepath.Join(tmp, "mutate-999999")
	if err := os.MkdirAll(filepath.Join(stale, "t"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "t", "big.go"), make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}
	// ⚠ AN AGE THRESHOLD, because a CONCURRENT run's copy is live. Backdating
	// is what makes this one abandoned rather than in use.
	old := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(tmp, "mutate-000001")
	if err := os.MkdirAll(fresh, 0o700); err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer
	sweepStaleCopies(&log)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("the abandoned copy survived: %v", err)
	}
	// ⛔ AND THE LIVE ONE IS UNTOUCHED. A sweep that removed a running pass's
	// staging would be worse than the leak it fixes.
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("a copy younger than the threshold was removed: %v", err)
	}
	if !strings.Contains(log.String(), "swept 1") {
		t.Fatalf("the sweep did not say what it did: %q", log.String())
	}
}
