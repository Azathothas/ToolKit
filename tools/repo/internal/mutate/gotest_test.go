// gotest_test.go - which case a dead binary died in, read from its own trace.
//
// ⛔ THE DEATH THIS FILE EXISTS FOR IS ONE THE CASE DID NOT CAUSE. A goroutine
// that init started panicked while a case waited, and the harness counted the
// open case as failed. The row was certified for a guard the case never
// reached.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"strings"
	"testing"
)

// The traces below are the shapes Go prints, cut to the lines that decide.

// traceFromInit is a goroutine init started, dying while a case runs, with
// every goroutine printed after it. ⚠ The main goroutine and the case's own
// both sit in testing.tRunner.
const traceFromInit = "=== RUN   TestWaits\n" +
	"panic: the watchdog fired\n\n" +
	"goroutine 7 [running]:\nexample.test/watch.watchdog()\n\t/m/watch/watch.go:15 +0x4a\n" +
	"created by example.test/watch.init.0 in goroutine 1\n\t/m/watch/watch.go:8 +0x1a\n" +
	"[originating from goroutine 1]:\nexample.test/watch.init.0(...)\n\t/m/watch/watch.go:8 +0x1a\n" +
	"runtime.doInit1(...)\n\t/go/src/runtime/proc.go:7350 +0xd8\nruntime.main(...)\n\t/go/src/runtime/proc.go:254 +0x34\n\n" +
	"goroutine 1 [chan receive]:\ntesting.(*T).Run(0xc000, {0x1, 0x2}, 0x3)\n\t/go/src/testing/testing.go:2266 +0x4d2\n" +
	"testing.runTests.func1(0xc000)\n\t/go/src/testing/testing.go:2742 +0x37\n" +
	"testing.tRunner(0xc000, 0xc001)\n\t/go/src/testing/testing.go:2193 +0xc3\n\n" +
	"goroutine 8 [select]:\nexample.test/watch.TestWaits(0xc002)\n\t/m/watch/watch_test.go:9 +0x1a\n" +
	"testing.tRunner(0xc002, 0x4)\n\t/go/src/testing/testing.go:2193 +0xc3\n" +
	"FAIL\texample.test/watch\t0.373s\n"

// traceFromTheCase is a goroutine the case started through the code under
// test. ⚠ Its own frames name only Tally. The ancestry reaches the case.
const traceFromTheCase = "=== RUN   TestTallyCounts\n" +
	"panic: assignment to entry in nil map\n\n" +
	"goroutine 9 [running]:\nexample%2etest.Tally.func1()\n\t/m/tally.go:9 +0x47\n" +
	"created by example%2etest.Tally in goroutine 8\n\t/m/tally.go:6 +0x1a\n" +
	"[originating from goroutine 8]:\nexample%2etest.Tally(...)\n\t/m/tally.go:6 +0x1a\n" +
	"example%2etest.TestTallyCounts(...)\n\t/m/tally_test.go:6 +0x13\n" +
	"testing.tRunner(...)\n\t/go/src/testing/testing.go:2196 +0xc3\ncreated by testing.(*T).Run\n\t/go/src/testing/testing.go:2258 +0x4b4\n" +
	"FAIL\texample.test\t0.040s\n"

// traceOfATimeout is the -timeout panic. ⚠ The goroutine it prints first is
// the alarm's, which runs in no case at all.
const traceOfATimeout = "=== RUN   TestRunReturns\n" +
	"panic: test timed out after 4s\n\trunning tests:\n\t\tTestRunReturns (4s)\n\n" +
	"goroutine 6 [running]:\ntesting.(*M).startAlarm.func1()\n\t/go/src/testing/testing.go:2959 +0x34a\n" +
	"created by time.goFunc\n\t/go/src/time/sleep.go:182 +0x2d\n" +
	"FAIL\texample.test\t4.012s\n"

// traceOfAnExit is what os.Exit prints, which is nothing.
const traceOfAnExit = "=== RUN   TestStayingDoesNotExit\nFAIL\texample.test\t0.040s\n"

// traceOfAStackOverflow is a recursion in the case's own goroutine. ⚠ The
// runtime's own stack comes first, and every goroutine header carries gp=, m=
// and mp=.
const traceOfAStackOverflow = "=== RUN   TestDepth\n" +
	"runtime: goroutine stack exceeds 1048576-byte limit\nfatal error: stack overflow\n\n" +
	"runtime stack:\nruntime.throw({0x1?, 0x2?})\n\t/go/src/runtime/panic.go:1243 +0x4d fp=0x1 sp=0x2 pc=0x3\n\n" +
	"goroutine 7 gp=0xc000102380 m=0 mp=0x5a8f40 [running]:\nexample%2etest.Depth(0x153a3?)\n\t/m/so.go:7 +0x49 fp=0x1 sp=0x2 pc=0x3\n" +
	"...86852 frames elided...\nexample%2etest.TestDepth(0xc000)\n\t/m/so_test.go:6 +0x1d fp=0x1 sp=0x2 pc=0x3\n" +
	"testing.tRunner(0xc000, 0x4)\n\t/go/src/testing/testing.go:2193 +0xc3 fp=0x1 sp=0x2 pc=0x3\n\n" +
	"goroutine 1 gp=0xc000002380 m=nil [chan receive]:\ntesting.(*T).Run(0xc000, {0x1, 0x2}, 0x3)\n\t/go/src/testing/testing.go:2266 +0x4d2\n" +
	"FAIL\texample.test\t0.120s\n"

// TestADeathIsTiedToACaseOnlyByItsTrace: the dying goroutine, or one that
// started it, ran in testing.tRunner, or the timeout lists the case.
func TestADeathIsTiedToACaseOnlyByItsTrace(t *testing.T) {
	cases := []struct {
		name  string
		out   string
		open  []string
		tied  string
		death string
	}{
		// ⛔ THE FINDING. Every goroutine is printed, the case's among them,
		// and only the first is the one that died.
		{"a goroutine init started", traceFromInit, []string{"TestWaits"}, "", "panic: the watchdog fired"},
		{"a goroutine the case started", traceFromTheCase, []string{"TestTallyCounts"}, "TestTallyCounts", "panic: assignment to entry in nil map"},
		{"the timeout, listing one of two", traceOfATimeout, []string{"TestOther", "TestRunReturns"}, "TestRunReturns", "panic: test timed out after 4s"},
		{"an os.Exit, with no trace", traceOfAnExit, []string{"TestStayingDoesNotExit"}, "", ""},
		{"a stack overflow, with the runtime's header", traceOfAStackOverflow, []string{"TestDepth"}, "TestDepth", "fatal error: stack overflow"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tied, death := diedIn(c.out, c.open)
			if strings.Join(tied, ",") != c.tied {
				t.Errorf("tied %q, want %q", tied, c.tied)
			}
			if death != c.death {
				t.Errorf("the death read as %q, want %q", death, c.death)
			}
		})
	}
}

// TestACaseThatKillsItsBinaryIsRed: a goroutine the case starts through the
// code under test panics with the guard gone, and the case is red.
//
// ⚠ NO EVENT SAYS SO. test2json reports `run` for the case and `fail` for the
// package, and nothing for the case itself. ⛔ AND THE GOROUTINE'S OWN TRACE
// NAMES ONLY Tally. The ancestry the worker asks for is what reaches the case.
//
// ⛔ THE CLOSE IS NOT DEFERRED. A deferred close runs while the goroutine
// panics. The case then woke and failed on its own before the binary died, in
// 4 runs of 12, and the row read THEATRE.
func TestACaseThatKillsItsBinaryIsRed(t *testing.T) {
	t.Parallel()
	root := fixtureWith(t, map[string]string{
		"tally.go":      "package guard\n\nfunc Tally(words []string) map[string]int {\n\tcounts := map[string]int{}\n\tdone := make(chan struct{})\n\tgo func() {\n\t\tfor _, w := range words {\n\t\t\tcounts[w]++\n\t\t}\n\t\tclose(done)\n\t}()\n\t<-done\n\treturn counts\n}\n",
		"tally_test.go": "package guard\n\nimport \"testing\"\n\nfunc TestTallyCounts(t *testing.T) {\n\tif Tally([]string{\"a\", \"a\"})[\"a\"] != 2 {\n\t\tt.Fatal(\"two of a counted wrong\")\n\t}\n}\n",
	})
	m := Mutation{
		Label: "the map the counting goroutine writes", Module: "mod", File: "tally.go",
		Run: "TestTallyCounts", Find: "counts := map[string]int{}", Replace: "var counts map[string]int",
	}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{})
	if got[0].State != "ok" {
		t.Fatalf("state %q, want ok: the case's own goroutine died with the guard gone (%s)\n%s", got[0].State, got[0].Reason, log)
	}
}

// TestACaseThatOverflowsItsStackIsRed: the mutation deletes a recursion's base
// case, and the case's own goroutine overflows its stack.
//
// ⚠ A FATAL ERROR, NOT A PANIC. Nothing recovers it, so no event says the case
// failed. The runtime's header for it carries gp=, m= and mp=, and the case's
// goroutine has to be read through that header.
func TestACaseThatOverflowsItsStackIsRed(t *testing.T) {
	t.Parallel()
	root := fixtureWith(t, map[string]string{
		"depth.go":      "package guard\n\nimport \"runtime/debug\"\n\n// A small ceiling, so the overflow comes quickly.\nfunc init() { debug.SetMaxStack(1 << 20) }\n\n// Depth counts down to zero.\nfunc Depth(n int) int {\n\tif n <= 0 {\n\t\treturn 0\n\t}\n\treturn 1 + Depth(n-1)\n}\n",
		"depth_test.go": "package guard\n\nimport \"testing\"\n\nfunc TestDepthCounts(t *testing.T) {\n\tif Depth(3) != 3 {\n\t\tt.Fatal(\"three counted wrong\")\n\t}\n}\n",
	})
	m := Mutation{
		Label: "a recursion's base case", Module: "mod", File: "depth.go",
		Run: "TestDepthCounts", Find: "if n <= 0 {", Replace: "if n < -1<<40 {",
	}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{})
	if got[0].State != "ok" {
		t.Fatalf("state %q, want ok: the case's own goroutine overflowed with the guard gone (%s)\n%s", got[0].State, got[0].Reason, log)
	}
}

// TestADeathTheCaseDidNotCauseIsNotOk is the finding: init starts a watchdog,
// the mutation makes it panic, and the case only waits.
//
// ⚠ THE WATCHDOG PRINTS EVERY GOROUTINE, so the case's own goroutine and the
// main one are in the output, both in testing.tRunner. Only the first block
// is the goroutine that died.
func TestADeathTheCaseDidNotCauseIsNotOk(t *testing.T) {
	t.Parallel()
	root := fixtureWith(t, map[string]string{
		"watch/watch.go": "package watch\n\nimport \"runtime/debug\"\n\n// Started closes when the case is running, and Checked when the watchdog\n// has looked.\nvar Started, Checked = make(chan struct{}), make(chan struct{})\n\n" +
			"func init() { go watchdog() }\n\nfunc armed() bool { return false }\n\n" +
			"func watchdog() {\n\t<-Started\n\tdebug.SetTraceback(\"all\")\n\tif armed() {\n\t\tpanic(\"the watchdog fired\")\n\t}\n\tclose(Checked)\n}\n",
		"watch/watch_test.go": "package watch\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nfunc TestWaits(t *testing.T) {\n\tclose(Started)\n\tselect {\n\tcase <-Checked:\n\tcase <-time.After(time.Minute):\n\t\tt.Fatal(\"the watchdog never looked\")\n\t}\n}\n",
	})
	m := Mutation{
		Label: "a watchdog the case does not run", Module: "mod", File: "watch/watch.go",
		Run: "TestWaits", Find: "func armed() bool { return false }", Replace: "func armed() bool { return true }",
	}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{})
	if got[0].State != "BROKEN" {
		t.Fatalf("state %q, want BROKEN: the binary died in a goroutine init started (%s)\n%s", got[0].State, got[0].Reason, log)
	}
	for _, want := range []string{"nothing ties the death", "the watchdog fired"} {
		if !strings.Contains(got[0].Reason, want) {
			t.Fatalf("the reason does not say %q: %q", want, got[0].Reason)
		}
	}
}

// TestACaseThatEndsTheProcessIsNotOk: the code under test calls os.Exit with
// the guard gone. ⛔ Nothing in the output says which goroutine called it, so
// it is not read as the case going red.
func TestACaseThatEndsTheProcessIsNotOk(t *testing.T) {
	t.Parallel()
	root := fixtureWith(t, map[string]string{
		"exit.go":      "package guard\n\nimport \"os\"\n\nfunc Leave(s string) {\n\tif s == \"stay\" {\n\t\treturn\n\t}\n\tos.Exit(3)\n}\n",
		"exit_test.go": "package guard\n\nimport \"testing\"\n\nfunc TestStayingDoesNotExit(t *testing.T) {\n\tLeave(\"stay\")\n}\n",
	})
	m := Mutation{
		Label: "a guard whose absence ends the process", Module: "mod", File: "exit.go",
		Run: "TestStayingDoesNotExit", Find: "if s == \"stay\" {", Replace: "if false {",
	}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{})
	if got[0].State != "BROKEN" || !strings.Contains(got[0].Reason, "no trace") {
		t.Fatalf("state %q (%s), want BROKEN saying the binary ended with no trace\n%s", got[0].State, got[0].Reason, log)
	}
}

// TestACaseTheTimeoutKillsIsRed: the mutation makes the case hang, and the
// -timeout panic lists it among the running tests.
func TestACaseTheTimeoutKillsIsRed(t *testing.T) {
	t.Parallel()
	root := fixtureWith(t, map[string]string{
		"wait.go":      "package guard\n\nfunc Run(work func()) {\n\tdone := make(chan struct{})\n\tgo func() {\n\t\twork()\n\t\tclose(done)\n\t}()\n\t<-done\n}\n",
		"wait_test.go": "package guard\n\nimport \"testing\"\n\nfunc TestRunReturns(t *testing.T) {\n\tRun(func() {})\n}\n",
	})
	m := Mutation{
		Label: "the close a waiting case needs", Module: "mod", File: "wait.go",
		Run: "TestRunReturns", Find: "\t\tclose(done)\n", Replace: "", Args: []string{"-timeout", "4s"},
	}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{m}}, Selection{})
	if got[0].State != "ok" {
		t.Fatalf("state %q, want ok: the timeout named the case (%s)\n%s", got[0].State, got[0].Reason, log)
	}
}
