// SPDX-License-Identifier: 0BSD

package checks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestEveryCaseIsDealtToOneProcess holds casePatterns to the rule a split
// package rests on: every listed case is matched by exactly one pattern.
//
// ⛔ CHECKED BY go's OWN MATCHING HERE AND BY unevenlyDealt, and a name that
// begins another's is in every set. "TestA" sorted just before "TestAB" is the
// shape a prefix pattern gets wrong.
func TestEveryCaseIsDealtToOneProcess(t *testing.T) {
	long := make([]string, 0, 2000)
	for i := 0; i < 2000; i++ {
		long = append(long, fmt.Sprintf("TestAGrantThatCameUpWritableIsRemountedReadOnlyInCase%04d", i))
	}
	sets := [][]string{
		{"TestOne"},
		{"TestA", "TestAB", "TestABC", "TestB"},
		{"TestB", "TestA", "TestAB", "TestA"},
		{"TestX", "TestXY", "TestXZ", "TestY", "TestYX", "ExampleX", "FuzzX", "BenchmarkX"},
		long,
	}
	for si, names := range sets {
		for k := 1; k <= 5; k++ {
			patterns := casePatterns(names, k)
			if len(patterns) > k {
				t.Fatalf("set %d: %d patterns for k = %d", si, len(patterns), k)
			}
			if bad := unevenlyDealt(names, patterns); bad != "" {
				t.Fatalf("set %d, k = %d: %s\npatterns: %q", si, k, bad, patterns)
			}
			// ⭐ EACH PATTERN STAYS SHORT. 2,000 long names spelled out would be
			// over 100,000 characters, past what one argument may carry.
			for _, p := range patterns {
				if len(p) > 4096 {
					t.Fatalf("set %d, k = %d: a pattern of %d characters", si, k, len(p))
				}
			}
		}
	}
	// ⛔ AND unevenlyDealt REFUSES what it should. A check of the deal that
	// passes everything proves nothing about it.
	if unevenlyDealt([]string{"TestA", "TestB"}, []string{"^(?:TestA$)"}) == "" {
		t.Error("a case no pattern matches was not named")
	}
	if unevenlyDealt([]string{"TestA", "TestAB"}, []string{"^(?:TestA)", "^(?:TestAB$)"}) == "" {
		t.Error("a case two patterns match was not named")
	}
}

// TestASplitPackageThatMovedIsAFinding: a package named in splitTests that
// moved would run in one process again, and its cases would still pass.
//
// ⚠ THE GATE READS THE REAL TREE. This case reads a tree of its own, because a
// mutation row runs the module alone, away from the repository.
func TestASplitPackageThatMovedIsAFinding(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "mod", "p"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "mod", "p", "p_test.go"), []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	found := missingSplits(root, map[string]bool{"mod/p": true, "mod/gone": true})
	if len(found) != 1 || !strings.Contains(found[0], "splitTests names mod/gone, which holds no test file") {
		t.Fatalf("one split package of two moved, and the check found %q", found)
	}
	if len(splitTests) == 0 {
		t.Fatal("no package is split, so the go check runs every case in one process")
	}
}

// TestASplitPackageRunsInTwoToFourProcesses holds the bound the go check runs
// a split package in, on any host.
func TestASplitPackageRunsInTwoToFourProcesses(t *testing.T) {
	if n := testProcesses(); n < 2 || n > 4 {
		t.Fatalf("a split package runs in %d processes on %d CPUs, want 2 to 4", n, runtime.NumCPU())
	}
}

// splitModule writes a module under root/mod. Each case of package p logs its
// process to ran, and so does the one case of package q. failing names a case of
// p that fails.
func splitModule(t *testing.T, root, ran string, cases []string, failing string) {
	t.Helper()
	logCase := func(pkg, name string) string {
		fail := ""
		if name == failing {
			fail = "\tt.Fatal(\"the failing case failed\")\n"
		}
		return "func " + name + "(t *testing.T) {\n" +
			"\tp := filepath.Join(" + strconv.Quote(ran) + ", " + strconv.Quote(pkg+"-"+name) + ")\n" +
			"\tf, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)\n" +
			"\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n" +
			"\t_, _ = f.WriteString(process + \"\\n\")\n\t_ = f.Close()\n" + fail + "}\n\n"
	}
	// ⚠ THE PROCESS IS ITS ID AND THE TIME IT STARTED. An id alone can be given
	// again to a process that starts after another ends.
	head := "import (\n\t\"os\"\n\t\"path/filepath\"\n\t\"strconv\"\n\t\"testing\"\n\t\"time\"\n)\n\n" +
		"var process = strconv.Itoa(os.Getpid()) + \"-\" + strconv.FormatInt(time.Now().UnixNano(), 10)\n\n"
	p := "package p\n\n" + head
	for _, c := range cases {
		p += logCase("p", c)
	}
	files := map[string]string{
		"go.mod":      "module example.test\n\ngo 1.25.0\n",
		"p/p.go":      "package p\n",
		"p/p_test.go": p,
		"q/q.go":      "package q\n",
		"q/q_test.go": "package q\n\n" + head + logCase("q", "TestQ"),
	}
	for name, body := range files {
		at := filepath.Join(root, "mod", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(at), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(at, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// ranIn reads which processes each case ran in.
func ranIn(t *testing.T, ran string) map[string][]string {
	t.Helper()
	entries, err := os.ReadDir(ran)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(ran, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		got[e.Name()] = strings.Fields(string(b))
	}
	return got
}

// TestASplitPackagesCasesRunOnceEachInSeveralProcesses runs the go check's test
// step over a module of its own.
//
// ⛔ READ FROM INSIDE THE CASES. Each logs the process it ran in, so a split that
// ran everything in one process, or a case twice, shows here.
func TestASplitPackagesCasesRunOnceEachInSeveralProcesses(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no Go toolchain on PATH")
	}
	root, ran := t.TempDir(), t.TempDir()
	cases := []string{"TestA", "TestAB", "TestB", "TestC", "TestCD", "TestD", "TestE", "TestF"}
	splitModule(t, root, ran, cases, "")
	found := goTestModule(root, goBin, "mod", filepath.Join(root, "mod"), map[string]bool{"mod/p": true}, 3)
	if len(found) != 0 {
		t.Fatalf("the module's cases all pass, and the step found %q", found)
	}
	got := ranIn(t, ran)
	pids := map[string]bool{}
	for _, c := range cases {
		in := got["p-"+c]
		if len(in) != 1 {
			t.Errorf("%s ran %d times, want once", c, len(in))
			continue
		}
		pids[in[0]] = true
	}
	if len(pids) != 3 {
		t.Errorf("p's cases ran in %d process(es), want 3", len(pids))
	}
	if len(got["q-TestQ"]) != 1 {
		t.Errorf("the package that is not split ran its case %d times, want once", len(got["q-TestQ"]))
	}
	if len(got) != len(cases)+1 {
		keys := make([]string, 0, len(got))
		for k := range got {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Errorf("cases logged: %v", keys)
	}
}

// TestAFailureInOneProcessIsAFinding: a case that fails in one of the processes
// fails the step and is named.
func TestAFailureInOneProcessIsAFinding(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no Go toolchain on PATH")
	}
	root, ran := t.TempDir(), t.TempDir()
	splitModule(t, root, ran, []string{"TestA", "TestB", "TestC", "TestD"}, "TestC")
	found := goTestModule(root, goBin, "mod", filepath.Join(root, "mod"), map[string]bool{"mod/p": true}, 2)
	if len(found) != 1 || !strings.Contains(found[0], "TestC") || !strings.Contains(found[0], "the failing case failed") {
		t.Fatalf("one case failed, and the step found %q", found)
	}
}

// TestASplitPackagesCasesRunAgainOnEveryCheck: a second go check over a module
// that did not change runs the split package's cases again.
//
// ⚠ GOFLAGS IS EMPTIED. CI's carries -count=1, which would run them again with
// or without the check's own.
func TestASplitPackagesCasesRunAgainOnEveryCheck(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no Go toolchain on PATH")
	}
	t.Setenv("GOFLAGS", "")
	root, ran := t.TempDir(), t.TempDir()
	cases := []string{"TestA", "TestB", "TestC"}
	splitModule(t, root, ran, cases, "")
	for run := 1; run <= 2; run++ {
		if found := goTestModule(root, goBin, "mod", filepath.Join(root, "mod"), map[string]bool{"mod/p": true}, 2); len(found) != 0 {
			t.Fatalf("check %d: the module's cases all pass, and the step found %q", run, found)
		}
	}
	got := ranIn(t, ran)
	// ⛔ THE CACHE ANSWERS HERE, or the count below proves nothing. The package
	// that is not split ran its case in the first check alone.
	if n := len(got["q-TestQ"]); n != 1 {
		t.Fatalf("the package that is not split ran its case %d times in two checks, so the test cache answered nothing", n)
	}
	for _, c := range cases {
		if n := len(got["p-"+c]); n != 2 {
			t.Errorf("%s ran %d time(s) in two checks, want 2: a cached pass stood in for a run", c, n)
		}
	}
}
