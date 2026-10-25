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
	"strconv"
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
	return fixtureWith(t, nil)
}

// fixtureWith is fixture with more files in the module, by path under it.
func fixtureWith(t *testing.T, extra map[string]string) string {
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
	for name, body := range extra {
		files[name] = body
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// runFixture runs tb over root with two workers and fails on a run error.
//
// ⭐ EVERY CASE THAT RUNS THE HARNESS OVER A FIXTURE IS PARALLEL. Each is a
// whole run with its own fixture and its own locked slots, and one after
// another they cost the gate minutes. ⚠ A case that calls t.Setenv cannot be,
// and Go runs those first, alone.
func runFixture(t *testing.T, root string, tb *Table, sel Selection) ([]Verdict, string) {
	t.Helper()
	if sel.Jobs == 0 {
		sel.Jobs = 2
	}
	var log bytes.Buffer
	got, err := Run(root, tb, sel, &log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if len(got) != len(tb.Mutations) && len(sel.Labels) == 0 && len(sel.Modules) == 0 && sel.Only == "" && sel.Shard.N <= 1 {
		t.Fatalf("got %d verdicts for %d rows:\n%s", len(got), len(tb.Mutations), log.String())
	}
	return got, log.String()
}

// TestRunTellsTheFourOutcomesApart is the case for the whole design.
//
// ⚠ IT WAS THREE UNTIL 2026-09-10. The skipped row is the fourth, and it was
// added after a real row was reported as THEATRE on a host that could not run
// its case at all.
func TestRunTellsTheFourOutcomesApart(t *testing.T) {
	t.Parallel()
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
	got, err := Run(root, tb, Selection{Jobs: 2}, &log)
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

// realTable is the tracked table, which is what CI shards.
func realTable(t *testing.T) *Table {
	t.Helper()
	tb, err := Load(filepath.Join("..", "..", "mutations.json"))
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

// TestEveryRowBelongsToExactlyOneShard: N runners sharing one table prove the
// whole of it, and none proves a row another already did.
//
// ⛔ AND A PATTERN GROUP IS NEVER SPLIT. Rows that share a baseline share a
// shard, or that baseline runs once per shard holding one of them.
func TestEveryRowBelongsToExactlyOneShard(t *testing.T) {
	tb := realTable(t)
	all := make([]int, len(tb.Mutations))
	for i := range all {
		all[i] = i
	}
	for _, n := range []int{1, 2, 3, 6, 7} {
		owner := map[int]int{}
		for k := 1; k <= n; k++ {
			here, _ := splitShard(groupRows(tb, all), Shard{K: k, N: n})
			for _, g := range here {
				for _, r := range g.rows {
					if was, dup := owner[r]; dup {
						t.Fatalf("N=%d: row %d is in shard %d and shard %d", n, r, was, k)
					}
					owner[r] = k
				}
			}
		}
		if len(owner) != len(tb.Mutations) {
			t.Fatalf("N=%d: %d of %d rows are in a shard", n, len(owner), len(tb.Mutations))
		}
		byPattern := map[string]int{}
		for i, m := range tb.Mutations {
			k := m.Module + "\x00" + m.Run + "\x00" + strings.Join(m.Args, "\x1f")
			if s, ok := byPattern[k]; ok && s != owner[i] {
				t.Fatalf("N=%d: the rows of pattern %q are in shards %d and %d", n, m.Run, s, owner[i])
			}
			byPattern[k] = owner[i]
		}
	}
	whole, rest := splitShard(groupRows(tb, all), Shard{})
	if len(rest) != 0 || len(groupRows(tb, all)) != len(whole) {
		t.Fatal("the zero shard leaves out a group, and it is the whole table")
	}
}

func TestAShardIsReadAsKOverN(t *testing.T) {
	for in, want := range map[string]Shard{"": {}, "1/1": {K: 1, N: 1}, "2/3": {K: 2, N: 3}, "3/3": {K: 3, N: 3}} {
		if got, err := ParseShard(in); err != nil || got != want {
			t.Errorf("%q read as %+v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"0/3", "4/3", "1/0", "a/b", "1", "1/2/3", "-1/2", " 1/2", "1/ 2", "/"} {
		if _, err := ParseShard(bad); err == nil {
			t.Errorf("%q is accepted", bad)
		}
	}
}

// TestAShardThatSelectsNothingIsRefused: a shard past the end of a small table
// runs nothing, and nothing is not a clean run.
func TestAShardThatSelectsNothingIsRefused(t *testing.T) {
	tb := &Table{Schema: TableSchema, Mutations: []Mutation{
		{Label: "a guard"},
	}}
	var out bytes.Buffer
	if _, err := Run(t.TempDir(), tb, Selection{Shard: Shard{K: 2, N: 2}}, &out); err == nil || !strings.Contains(err.Error(), "shard 2/2") {
		t.Fatalf("an empty shard answered %v", err)
	}
}

// TestRunRefusesAFilterThatSelectsNothing. ⛔ Zero rows run and exit 0 is the
// answer this whole file exists to prevent.
func TestRunRefusesAFilterThatSelectsNothing(t *testing.T) {
	only := Mutation{Label: "a guard"}
	tb := &Table{Schema: TableSchema, Mutations: []Mutation{only}}
	var out bytes.Buffer
	if _, err := Run(t.TempDir(), tb, Selection{Only: "no such label"}, &out); err == nil {
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
	t.Parallel()
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
	got, err := Run(root, tb, Selection{Jobs: 2}, &log)
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
	t.Parallel()
	root := fixture(t)
	m := Mutation{
		Label: "a mutation that does not compile", Module: "mod", File: "guard.go",
		Run: "TestEmptyIsRefused", Find: "return true\n", Replace: "return nope\n",
	}
	tb := &Table{Schema: TableSchema, Mutations: []Mutation{m}}
	var log bytes.Buffer
	got, err := Run(root, tb, Selection{Jobs: 2}, &log)
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

// TestASiblingPackageThatStopsCompilingIsNotARedCase is the false "ok" this
// harness gave before it read events.
//
// ⛔ WHAT WAS WRONG. The mutated run tested `./...` and read any non-zero exit
// as "went red". A mutation that broke a package the case never imports exited
// 1 while the case passed, and the row was certified. The case here is green
// with the mutation in, so the row is THEATRE.
func TestASiblingPackageThatStopsCompilingIsNotARedCase(t *testing.T) {
	t.Parallel()
	root := fixtureWith(t, map[string]string{
		"other/other.go":      "package other\n\nfunc One() int { return 1 }\n",
		"other/other_test.go": "package other\n\nimport \"testing\"\n\nfunc TestOne(t *testing.T) {\n\tif One() != 1 {\n\t\tt.Fatal(\"one\")\n\t}\n}\n",
	})
	m := Mutation{
		Label: "a mutation that breaks a sibling package", Module: "mod", File: "other/other.go",
		Run: "TestEmptyIsRefused", Find: "return 1 }", Replace: "return nope }",
	}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{})
	if got[0].State == "ok" {
		t.Fatalf("a sibling package's build failure was read as the case going red:\n%s", log)
	}
	if got[0].State != "THEATRE" || got[0].Cases != 1 {
		t.Fatalf("state %q with %d case(s), want THEATRE with 1 (%s)", got[0].State, got[0].Cases, got[0].Reason)
	}
}

// TestAPackageThatFailsOutsideItsCasesIsNotOk: a TestMain that fails after
// every named case passed is a failure, but not of a case this row names.
//
// ⛔ "ok" MEANS A NAMED CASE FAILED. Anything else that fails is a row naming
// the wrong case, and it is BROKEN with the line that failed.
func TestAPackageThatFailsOutsideItsCasesIsNotOk(t *testing.T) {
	t.Parallel()
	root := fixtureWith(t, map[string]string{
		"tm/tm.go":      "package tm\n\nfunc Healthy() bool { return true }\n",
		"tm/tm_test.go": "package tm\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) {\n\tcode := m.Run()\n\tif !Healthy() {\n\t\tos.Exit(1)\n\t}\n\tos.Exit(code)\n}\n\nfunc TestFineUnderTestMain(t *testing.T) {}\n",
	})
	m := Mutation{
		Label: "a guard only TestMain notices", Module: "mod", File: "tm/tm.go",
		Run: "TestFineUnderTestMain", Find: "return true }", Replace: "return false }",
	}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{})
	if got[0].State != "BROKEN" {
		t.Fatalf("state %q, want BROKEN: the package failed and no named case did:\n%s", got[0].State, log)
	}
	if !strings.Contains(got[0].Reason, "no case it names failed") {
		t.Fatalf("the reason does not say the failure was outside the cases: %q", got[0].Reason)
	}
}

// TestAGroupsBaselineRunsOnce: three rows naming one case pay for one unmutated
// run, not three.
//
// ⚠ COUNTED FROM INSIDE THE CASE. It appends a line to a file each time it
// runs, so the count is what happened rather than what the harness says.
func TestAGroupsBaselineRunsOnce(t *testing.T) {
	t.Parallel()
	ran := filepath.Join(t.TempDir(), "ran.log")
	counted := "package guard\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestCounted(t *testing.T) {\n" +
		"\tf, err := os.OpenFile(" + strconv.Quote(ran) + ", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)\n" +
		"\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n\t_, _ = f.WriteString(\"ran\\n\")\n\t_ = f.Close()\n" +
		"\tif Allowed(\"\") {\n\t\tt.Fatal(\"the empty string was allowed\")\n\t}\n}\n"
	root := fixtureWith(t, map[string]string{"counted_test.go": counted})
	base := Mutation{Module: "mod", File: "guard.go", Run: "TestCounted"}
	a, b, c := base, base, base
	a.Label, a.Find, a.Replace = "the guard", "if s == \"\" {", "if false {"
	b.Label, b.Find, b.Replace = "the refusal", "return false\n", "return true\n"
	c.Label, c.Find, c.Replace = "the default", "return true\n", "return true // unread\n"
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{a, b, c}}, Selection{Jobs: 3})
	for i, want := range []string{"ok", "ok", "THEATRE"} {
		if got[i].State != want {
			t.Fatalf("%q: state %q, want %q (%s)\n%s", got[i].Label, got[i].State, want, got[i].Reason, log)
		}
	}
	body, err := os.ReadFile(ran)
	if err != nil {
		t.Fatal(err)
	}
	// ⚠ FIVE, NOT FOUR. The THEATRE row fails the run, so its mutation runs a
	// second time before the verdict stands.
	if n := strings.Count(string(body), "ran\n"); n != 5 {
		t.Fatalf("the case ran %d times for three rows of one pattern, want 5: one baseline, one per row, and a second for the THEATRE row", n)
	}
}

// counted is a case body that appends a line to path each time it runs and
// reads the count as n. ⚠ The file sits outside the worker's own state, which
// is emptied before every test process.
func counted(path string) string { return countedOr(path, "t.Fatal(err)") }

// countedOr is counted with its own answer to an error, for a TestMain.
func countedOr(path, fail string) string {
	return "\tf, err := os.OpenFile(" + strconv.Quote(path) + ", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)\n" +
		"\tif err != nil {\n\t\t" + fail + "\n\t}\n\t_, _ = f.WriteString(\"ran\\n\")\n\t_ = f.Close()\n" +
		"\tbody, err := os.ReadFile(" + strconv.Quote(path) + ")\n\tif err != nil {\n\t\t" + fail + "\n\t}\n" +
		"\tn := strings.Count(string(body), \"ran\\n\")\n"
}

// countedImports opens a test file whose case uses counted.
const countedImports = "import (\n\t\"os\"\n\t\"strings\"\n\t\"testing\"\n)\n\n"

// TestAMutationThatMakesItsCasesSkipIsTheatre: the case ran unmutated, and the
// mutation turned its own skip gate on.
//
// ⛔ WHAT WAS WRONG. The verdict was read from the mutated run alone. A case
// that skipped itself there was SKIPPED, "this host cannot run it", and the run
// exited 0. The baseline had just run it here. A guard whose absence makes its
// case skip is not caught by it, and CI stays green.
func TestAMutationThatMakesItsCasesSkipIsTheatre(t *testing.T) {
	t.Parallel()
	root := fixtureWith(t, map[string]string{
		"tool.go":      "package guard\n\n// ToolPath is where the tool this package drives lives, or empty.\nfunc ToolPath() string { return \"tool\" }\n",
		"tool_test.go": "package guard\n\nimport \"testing\"\n\nfunc TestDrivesTheTool(t *testing.T) {\n\tif ToolPath() == \"\" {\n\t\tt.Skip(\"no tool on this host\")\n\t}\n}\n",
	})
	m := Mutation{
		Label: "a mutation that turns the case's skip on", Module: "mod", File: "tool.go",
		Run: "TestDrivesTheTool", Find: "return \"tool\" }", Replace: "return \"\" }",
	}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{})
	if got[0].State != "THEATRE" || got[0].Skipped != 1 {
		t.Fatalf("state %q with %d skip(s), want THEATRE with 1 (%s)\n%s", got[0].State, got[0].Skipped, got[0].Reason, log)
	}
	var out bytes.Buffer
	if code := Report(&out, got); code != 1 {
		t.Fatalf("Report returned %d over a row the mutation made skip, want 1:\n%s", code, out.String())
	}
}

// TestAMutationThatLeavesNoCaseRunningIsBroken: a TestMain that ends before
// m.Run when the code under test says the host is not ready.
//
// ⛔ ZERO CASES WITH EXIT 0 USED TO BE SKIPPED, because 0 skips is at least 0
// cases. The baseline ran the case here, so nothing about this host stopped it.
func TestAMutationThatLeavesNoCaseRunningIsBroken(t *testing.T) {
	t.Parallel()
	root := fixtureWith(t, map[string]string{
		"gate/gate.go":      "package gate\n\n// Ready is whether this host can run the package's cases.\nfunc Ready() bool { return true }\n",
		"gate/gate_test.go": "package gate\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) {\n\tif !Ready() {\n\t\tos.Exit(0)\n\t}\n\tos.Exit(m.Run())\n}\n\nfunc TestGated(t *testing.T) {}\n",
	})
	m := Mutation{
		Label: "a mutation that leaves no case running", Module: "mod", File: "gate/gate.go",
		Run: "TestGated", Find: "return true }", Replace: "return false }",
	}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{})
	if got[0].State != "BROKEN" || !strings.Contains(got[0].Reason, "no case it names ran") {
		t.Fatalf("state %q (%s), want BROKEN saying no case ran\n%s", got[0].State, got[0].Reason, log)
	}
	var out bytes.Buffer
	if code := Report(&out, got); code != 1 {
		t.Fatalf("Report returned %d over a row that ran no case, want 1:\n%s", code, out.String())
	}
}

// TestAFlakyBaselineRefusesItsGroup: a case that fails on every odd run.
//
// ⛔ WHAT WAS WRONG. The baseline failed, was retried, passed, and was filed as
// green. The row's one mutated run was the third, failed the same way, and
// certified a mutation that changes only a comment.
func TestAFlakyBaselineRefusesItsGroup(t *testing.T) {
	t.Parallel()
	runs := filepath.Join(t.TempDir(), "runs.log")
	flaky := "package guard\n\n" + countedImports + "func TestFlaky(t *testing.T) {\n" + counted(runs) +
		"\tif n%2 == 1 {\n\t\tt.Fatal(\"an odd run\")\n\t}\n}\n"
	root := fixtureWith(t, map[string]string{"flaky_test.go": flaky})
	m := Mutation{
		Label: "a comment, over a flaky case", Module: "mod", File: "guard.go",
		Run: "TestFlaky", Find: "return true\n", Replace: "return true // unread\n",
	}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{})
	if got[0].State != "BROKEN" || !strings.Contains(got[0].Reason, "passed when run again") {
		t.Fatalf("state %q (%s), want BROKEN naming the flake\n%s", got[0].State, got[0].Reason, log)
	}
	body, err := os.ReadFile(runs)
	if err != nil {
		t.Fatal(err)
	}
	// ⭐ AND THE MUTATION NEVER RAN. A refused group proves nothing, so it
	// spends nothing.
	if n := strings.Count(string(body), "ran\n"); n != 2 {
		t.Fatalf("the case ran %d times, want 2: the baseline and its retry", n)
	}
}

// TestARetriedBaselineIsSaidSo: a TestMain that fails its first run before any
// case starts.
//
// ⚠ GREEN, BECAUSE NO CASE IT NAMES FAILED, and a flake there cannot read as a
// red one. ⛔ BUT THE LOG SAYS SO. A retry nobody sees is a flake nobody looks
// at.
func TestARetriedBaselineIsSaidSo(t *testing.T) {
	t.Parallel()
	runs := filepath.Join(t.TempDir(), "runs.log")
	once := "package once\n\n" + countedImports + "func TestMain(m *testing.M) {\n" +
		countedOr(runs, "os.Exit(2)") + "\tif n == 1 {\n\t\tos.Exit(1)\n\t}\n\tos.Exit(m.Run())\n}\n\n" +
		"func TestOnceGuard(t *testing.T) {\n\tif Allowed(\"\") {\n\t\tt.Fatal(\"the empty string was allowed\")\n\t}\n}\n"
	root := fixtureWith(t, map[string]string{
		"once/once.go":      "package once\n\nfunc Allowed(s string) bool {\n\tif s == \"\" {\n\t\treturn false\n\t}\n\treturn true\n}\n",
		"once/once_test.go": once,
	})
	m := Mutation{
		Label: "a guard whose first baseline failed outside it", Module: "mod", File: "once/once.go",
		Run: "TestOnceGuard", Find: "if s == \"\" {", Replace: "if false {",
	}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{})
	if got[0].State != "ok" {
		t.Fatalf("state %q (%s), want ok\n%s", got[0].State, got[0].Reason, log)
	}
	if !strings.Contains(log, "retried: the baseline for "+strconv.Quote(m.Label)) {
		t.Fatalf("the log does not say the baseline was retried:\n%s", log)
	}
}

// TestTwoRunsThatDisagreeAreBroken: a case green on its first two runs and red
// on its third, under a mutation that changes only a comment.
//
// ⛔ ONE RUN WAS THE VERDICT. A case that decides by a timing window stays
// green when a loaded machine runs the unguarded write late, and the row read
// THEATRE. A verdict that fails the run is now seen twice, and two that differ
// are neither.
func TestTwoRunsThatDisagreeAreBroken(t *testing.T) {
	t.Parallel()
	runs := filepath.Join(t.TempDir(), "runs.log")
	third := "package guard\n\n" + countedImports + "func TestRedOnTheThird(t *testing.T) {\n" + counted(runs) +
		"\tif n == 3 {\n\t\tt.Fatal(\"the third run\")\n\t}\n}\n"
	root := fixtureWith(t, map[string]string{"third_test.go": third})
	m := Mutation{
		Label: "a comment, over a case that changes its mind", Module: "mod", File: "guard.go",
		Run: "TestRedOnTheThird", Find: "return true\n", Replace: "return true // unread\n",
	}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{})
	if got[0].State != "BROKEN" || !strings.Contains(got[0].Reason, "disagree, THEATRE and then ok") {
		t.Fatalf("state %q (%s), want BROKEN naming both runs\n%s", got[0].State, got[0].Reason, log)
	}
}

// TestAVerdictPrintsAsItsRowFinishes: the table's first row finishes last, and
// the second row's line is in the log before it.
//
// ⛔ WHAT WAS WRONG. A line waited for every row above it in the table. Groups
// go out heaviest first and the table opens with light rows, so a CI shard
// printed its first verdict at 97% of the run.
//
// ⚠ ORDERED BY A FILE, NOT A CLOCK. The slow case waits until the fast row has
// run twice, its baseline and its mutation.
func TestAVerdictPrintsAsItsRowFinishes(t *testing.T) {
	t.Parallel()
	marker := filepath.Join(t.TempDir(), "fast.log")
	fast := "package guard\n\n" + countedImports + "func TestFast(t *testing.T) {\n" + counted(marker) +
		"\t_ = n\n\tif Allowed(\"\") {\n\t\tt.Fatal(\"the empty string was allowed\")\n\t}\n}\n"
	slow := "package guard\n\nimport (\n\t\"os\"\n\t\"strings\"\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestWaitsForTheFastRow(t *testing.T) {\n" +
		"\tfor end := time.Now().Add(2 * time.Minute); ; time.Sleep(20 * time.Millisecond) {\n" +
		"\t\tbody, _ := os.ReadFile(" + strconv.Quote(marker) + ")\n" +
		"\t\tif strings.Count(string(body), \"ran\\n\") >= 2 {\n\t\t\tbreak\n\t\t}\n" +
		"\t\tif time.Now().After(end) {\n\t\t\tt.Fatal(\"the fast row never ran twice\")\n\t\t}\n\t}\n" +
		"\tif Allowed(\"\") {\n\t\tt.Fatal(\"the empty string was allowed\")\n\t}\n}\n"
	root := fixtureWith(t, map[string]string{"fast_test.go": fast, "slow_test.go": slow})
	first := Mutation{Label: "row one, slow", Module: "mod", File: "guard.go", Run: "TestWaitsForTheFastRow", Find: "if s == \"\" {", Replace: "if false {"}
	second := Mutation{Label: "row two, fast", Module: "mod", File: "guard.go", Run: "TestFast", Find: "if s == \"\" {", Replace: "if false {"}
	tb := &Table{Schema: TableSchema, Mutations: []Mutation{first, second}}
	got, log := runFixture(t, root, tb, Selection{Jobs: 2})
	// ⭐ WHAT Run RETURNS IS STILL IN TABLE ORDER, and Report reads that.
	for i, m := range tb.Mutations {
		if got[i].Label != m.Label || got[i].State != "ok" {
			t.Fatalf("verdict %d is %q %s, want %q ok (%s)\n%s", i, got[i].Label, got[i].State, m.Label, got[i].Reason, log)
		}
	}
	slowAt, fastAt := strings.Index(log, "row one, slow"), strings.Index(log, "row two, fast")
	if fastAt < 0 || slowAt < 0 || fastAt > slowAt {
		t.Fatalf("the fast row's line is not before the slow row's:\n%s", log)
	}
	if !strings.Contains(log, "[2/2] ok       row two, fast") {
		t.Fatalf("the fast row's line does not carry its place in the table:\n%s", log)
	}
}

// TestTheLogSaysWhatIsStillRunning: a row slower than the heartbeat is named
// while it runs.
//
// ⛔ A SHARD THE TIME LIMIT KILLED could not say which row hung, because no
// line said what was running.
func TestTheLogSaysWhatIsStillRunning(t *testing.T) {
	t.Parallel()
	slow := "package guard\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestSlowly(t *testing.T) {\n\ttime.Sleep(1500 * time.Millisecond)\n" +
		"\tif Allowed(\"\") {\n\t\tt.Fatal(\"the empty string was allowed\")\n\t}\n}\n"
	root := fixtureWith(t, map[string]string{"slow_test.go": slow})
	m := Mutation{Label: "a row slower than the heartbeat", Module: "mod", File: "guard.go", Run: "TestSlowly", Find: "if s == \"\" {", Replace: "if false {"}
	_, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{Jobs: 1, heartbeat: 200 * time.Millisecond})
	for _, ln := range strings.Split(log, "\n") {
		if strings.Contains(ln, "still running") && strings.Contains(ln, m.Label) {
			return
		}
	}
	t.Fatalf("no line named the row while it ran:\n%s", log)
}

// TestAFileOneRowLeavesDoesNotReachTheNext: row A deletes a cleanup and leaves a
// file in the temporary directory. Row B, in another group, changes a comment,
// and its case asserts no such file is there.
//
// ⛔ WHAT WAS WRONG. A worker's state was made once per run. At --jobs 1, B's
// baseline ran after A's mutation on the same worker, met A's file, and B was
// BROKEN. At --jobs 2 it was THEATRE. One table, two verdicts, decided by
// scheduling.
func TestAFileOneRowLeavesDoesNotReachTheNext(t *testing.T) {
	t.Parallel()
	root := fixtureWith(t, map[string]string{
		"leak.go":      "package guard\n\nimport (\n\t\"os\"\n\t\"path/filepath\"\n)\n\n// Scratch writes a file in the temporary directory, and removes it.\nfunc Scratch() string {\n\tp := filepath.Join(os.TempDir(), \"repo-mutate-leak-probe\")\n\t_ = os.WriteFile(p, []byte(\"x\"), 0o600)\n\t_ = os.Remove(p)\n\treturn p\n}\n",
		"leak_test.go": "package guard\n\nimport (\n\t\"os\"\n\t\"path/filepath\"\n\t\"testing\"\n)\n\nfunc TestScratchCleansUp(t *testing.T) {\n\tif _, err := os.Stat(Scratch()); err == nil {\n\t\tt.Fatal(\"the scratch file was left behind\")\n\t}\n}\n\nfunc TestNothingLeftOver(t *testing.T) {\n\tif _, err := os.Stat(filepath.Join(os.TempDir(), \"repo-mutate-leak-probe\")); err == nil {\n\t\tt.Fatal(\"a file another process left is here\")\n\t}\n}\n",
	})
	a := Mutation{Label: "the scratch file's cleanup", Module: "mod", File: "leak.go", Run: "TestScratchCleansUp", Find: "\t_ = os.Remove(p)\n", Replace: ""}
	b := Mutation{Label: "a comment nothing reads", Module: "mod", File: "leak.go", Run: "TestNothingLeftOver", Find: "// Scratch writes", Replace: "// Scratch, unread, writes"}
	tb := &Table{Schema: TableSchema, Mutations: []Mutation{a, b}}
	for _, jobs := range []int{1, 2} {
		got, log := runFixture(t, root, tb, Selection{Jobs: jobs})
		if got[0].State != "ok" || got[1].State != "THEATRE" {
			t.Fatalf("--jobs %d: A is %s and B is %s (%s), want ok and THEATRE\n%s", jobs, got[0].State, got[1].State, got[1].Reason, log)
		}
	}
}

// TestACaseRunsInItsWorkersOwnState: the environment a worker is given is the
// one its `go test` process gets.
//
// ⛔ CHECKED FROM INSIDE THE CASE. A worker environment that is built and then
// never handed to the process isolates nothing, and every row still passes.
//
// ⚠ PINNED TO A DIRECTORY ONLY THIS RUN COULD HAVE MADE. This case also runs
// inside a worker of an outer harness, whose own HOME and TMP sit side by side
// just as a worker's do. A check of the shape alone passed on what it inherited.
func TestACaseRunsInItsWorkersOwnState(t *testing.T) {
	tmp := privateTemp(t)
	own := "package guard\n\nimport (\n\t\"os\"\n\t\"path/filepath\"\n\t\"strings\"\n\t\"testing\"\n)\n\nfunc TestOwnState(t *testing.T) {\n" +
		"\tslot := filepath.Dir(os.TempDir())\n" +
		"\tfor _, k := range []string{\"HOME\", \"USERPROFILE\", \"LOCALAPPDATA\", \"WSL_TOOLKIT_HOME\"} {\n" +
		"\t\tv := os.Getenv(k)\n" +
		"\t\tif filepath.Dir(v) != slot || !strings.HasPrefix(v, " + strconv.Quote(tmp) + ") {\n\t\t\tt.Fatalf(\"%s is %q, outside %s\", k, v, slot)\n\t\t}\n\t}\n" +
		"\tif Allowed(\"\") {\n\t\tt.Fatal(\"the empty string was allowed\")\n\t}\n}\n"
	root := fixtureWith(t, map[string]string{"own_test.go": own})
	m := Mutation{Label: "the guard, in its own state", Module: "mod", File: "guard.go", Run: "TestOwnState", Find: "if s == \"\" {", Replace: "if false {"}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{Jobs: 1})
	if got[0].State != "ok" {
		t.Fatalf("state %q, want ok (%s)\n%s", got[0].State, got[0].Reason, log)
	}
}

// TestRunSaysWhatItLeftOut: a row a filter or a shard does not select is
// counted by module and printed with the reason.
//
// ⛔ NOTHING IS LEFT OUT IN SILENCE. A log that lists only what ran reads as
// the whole table.
func TestRunSaysWhatItLeftOut(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	real := Mutation{Label: "the empty-string guard", Module: "mod", File: "guard.go", Run: "TestEmptyIsRefused", Find: "if s == \"\" {", Replace: "if false {"}
	away := Mutation{Label: "a row of another module", Module: "elsewhere", File: "x.go", Run: "TestX", Find: "x", Replace: "y"}
	tb := &Table{Schema: TableSchema, Mutations: []Mutation{real, away}}
	_, log := runFixture(t, root, tb, Selection{Modules: []string{"mod"}, Jobs: 1})
	if !strings.Contains(log, "not run: 1 row(s) of elsewhere, because its module is not one --module names") {
		t.Fatalf("the log does not say what --module left out:\n%s", log)
	}
}
