// mutate.go - delete each guard and confirm the test named for it goes red.
//
// ⛔ A TEST THAT STAYS GREEN WITH ITS SUBJECT REMOVED IS THEATRE, and it is
// worse than no test because it is read as coverage. This repository has found
// two: a guard whose case never exercised it, and a ledger stress case that went
// red in 0 of 10 runs against the defect it was written for.
//
// ⭐ FOUR OUTCOMES, NOT TWO, and that is the whole design. A previous harness
// reported "green with the guard gone" for three different things: a test that
// really did not cover its subject, a `-run` pattern that matched NO test, and a
// mutation that did not COMPILE. The last two prove nothing and must not read as
// the first. So every row reports the case count and the build status separately:
//
//	ok       the guard is real: the mutation built, cases ran, and one it names failed
//	THEATRE  it built, cases ran, and they stayed green
//	SKIPPED  it built, and every case it names skipped itself on this host
//	BROKEN   it did not build, or matched no case, or the find was not unique,
//	         or something failed that is not a case it names
//
// ⚠ SKIPPED IS THE FOURTH, AND IT WAS ADDED BECAUSE THIS HARNESS MADE EXACTLY
// THE MISTAKE IT EXISTS TO CATCH. A case that calls t.Skip prints `=== RUN` and
// leaves `go test` exiting 0, which is byte for byte what a case that stayed
// green looks like from out here. So the row for the workspace symlink guard,
// whose case needs a privilege Windows does not hand an ordinary process, was
// reported as THEATRE on the operator's own host: a true statement about the
// host printed as a false accusation against the test.
//
// ⛔ A SKIPPED ROW IS NOT PROVED AND IS NOT COUNTED AS PROVED. It does not fail
// the run either, and that is a deliberate amendment to the older rule that
// anything short of every row proved is a failure. A row that no host in the
// world can prove is a defect; a row THIS host cannot prove is the doctor's
// shape, which is to report absent rather than zero. ⭐ What makes that safe is
// that the ubuntu CI job runs this table too, and nothing skips there.
//
// ⛔ "ok" IS READ FROM THE EVENTS OF `go test -json`, NEVER FROM ITS EXIT CODE.
// The harness used to test `./...` and call any non-zero exit red once a case
// had run. A mutation that broke a SIBLING package exited 1 while every named
// case passed, and that row was certified. Now the baseline records which
// packages hold the row's cases, and the mutated run tests only those. A row is
// "ok" only when a case it names fails. gotest.go reads the events.
//
// ⚠ THE TABLE IS DATA, in mutations.json beside this module. A mutation is a
// claim about one guard, and claims belong where they can be read without
// recompiling the thing that checks them.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Mutation is one claim: remove this, and that test fails.
type Mutation struct {
	Label string `json:"label"`
	// Module is the Go module to copy, relative to the repository root.
	Module string `json:"module"`
	// File is the file to change, relative to Module.
	File    string `json:"file"`
	Find    string `json:"find"`
	Replace string `json:"replace"`
	// Run is the -run pattern naming the case or cases this guard belongs to.
	Run string `json:"run"`
	// Args are extra `go test` arguments. ⚠ Not decoration: the prefixWriter
	// lock is only RELIABLY caught under -race, so that row asks for it.
	Args []string `json:"args"`
}

// Table is the tracked file's shape.
type Table struct {
	Schema    string     `json:"schema"`
	Mutations []Mutation `json:"mutations"`
}

// TableSchema is the version this code reads.
const TableSchema = "repo-mutations/1"

// Verdict is what one row produced.
type Verdict struct {
	Label string
	// State is "ok", "THEATRE", "SKIPPED" or "BROKEN". ⛔ Never a boolean:
	// collapsing these is the defect this file was written to remove.
	State string
	// Cases is how many cases ran, and Skipped how many of those skipped
	// themselves. ⚠ Both, because "1 case ran" and "1 case ran and skipped"
	// are the same string to `go test`'s exit code and mean opposite things.
	Cases   int
	Skipped int
	Reason  string
}

// Load reads the table from a path.
func Load(path string) (*Table, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Table
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("%s is not readable as a mutation table: %w", path, err)
	}
	if t.Schema != TableSchema {
		return nil, fmt.Errorf("%s declares schema %q and this build reads %q", path, t.Schema, TableSchema)
	}
	if len(t.Mutations) == 0 {
		// ⛔ An empty table is not a clean run. A harness that checks nothing and
		// reports success is the shape of every dead check this tree has found.
		return nil, fmt.Errorf("%s holds no mutations, so nothing would be proved", path)
	}
	return &t, nil
}

// Selection is which rows one run proves, and how many run at once. Every
// filter that is set must agree, and of those rows, the ones in Shard run.
type Selection struct {
	// Only keeps the rows whose label CONTAINS it. ⚠ For a person at a
	// prompt: one label can be part of another, and then both run.
	Only string
	// Labels keeps the rows whose label IS one of these. ⭐ For a caller that
	// names rows it computed, such as the pre-push hook.
	Labels []string
	// Modules keeps the rows of these modules, as tools/check.
	Modules []string
	Shard   Shard
	// Jobs is how many rows run at once. Zero is DefaultJobs.
	Jobs int
}

// Shard is the K-th of N runners sharing one table. The zero value is the
// whole table. plan.go deals whole pattern groups to the shards, heaviest
// first, so each group's baseline runs once and the shares weigh about the
// same.
//
// ⛔ EVERY ROW BELONGS TO EXACTLY ONE SHARD. The union of N shards is the table
// and no row runs twice, which a case asserts over the real table for several N.
type Shard struct {
	K, N int
}

// ParseShard reads K/N, as 2/3.
func ParseShard(s string) (Shard, error) {
	if s == "" {
		return Shard{}, nil
	}
	refused := fmt.Errorf("--shard %q is not K/N with 1 <= K <= N, as 2/3", s)
	k, n, ok := strings.Cut(s, "/")
	if !ok {
		return Shard{}, refused
	}
	kk, errK := strconv.Atoi(k)
	nn, errN := strconv.Atoi(n)
	if errK != nil || errN != nil || nn < 1 || kk < 1 || kk > nn {
		return Shard{}, refused
	}
	return Shard{K: kk, N: nn}, nil
}

// Run applies every selected mutation and reports one verdict each, in table
// order. Progress is written to w as it goes, because a full pass takes minutes
// and a silent one reads as a hang.
func Run(root string, t *Table, sel Selection, w io.Writer) ([]Verdict, error) {
	rows, left, err := choose(t, sel)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		if sel.Only != "" {
			return nil, fmt.Errorf("no mutation's label contains %q", sel.Only)
		}
		return nil, fmt.Errorf("no row is selected, so nothing would be proved")
	}
	here, others := splitShard(groupRows(t, rows), sel.Shard)
	if len(here) == 0 {
		return nil, fmt.Errorf("shard %d/%d holds none of the %d selected row(s), so it would prove nothing", sel.Shard.K, sel.Shard.N, len(rows))
	}
	for _, g := range others {
		for _, r := range g.rows {
			left = append(left, leftOut{r, fmt.Sprintf("the plan gives it to another of the %d shards", sel.Shard.N)})
		}
	}
	sort.SliceStable(left, func(a, b int) bool { return left[a].row < left[b].row })
	reportLeftOut(w, t, left)

	// ⭐ THE ROWS THIS RUN PROVES, IN TABLE ORDER, and where each one prints.
	var mine []int
	for _, g := range here {
		mine = append(mine, g.rows...)
	}
	sort.Ints(mine)
	at := map[int]int{}
	for p, r := range mine {
		at[r] = p
	}

	jobs := sel.Jobs
	if jobs <= 0 {
		jobs = DefaultJobs()
	}
	jobs = min(jobs, len(here))
	fmt.Fprintf(w, "  proving %d row(s) in %d pattern group(s), %d at a time\n", len(mine), len(here), jobs)

	// ⛔ SWEPT BEFORE ANYTHING IS STAGED, because a killed run leaves its copy
	// behind and nothing else collects it. Finding 32.
	sweepStaleCopies(w)
	sweepSlots(w)
	goEnv, err := readGoEnv(root)
	if err != nil {
		return nil, err
	}
	var workers []*worker
	defer func() {
		for _, wk := range workers {
			wk.slot.release()
		}
	}()
	for k := 0; k < jobs; k++ {
		s, err := takeSlot()
		if err != nil {
			return nil, fmt.Errorf("no staging directory for worker %d: %w", k+1, err)
		}
		wk, err := newWorker(root, s, goEnv)
		if err != nil {
			s.release()
			return nil, fmt.Errorf("no staging directory for worker %d: %w", k+1, err)
		}
		workers = append(workers, wk)
	}

	width := 0
	for _, m := range t.Mutations {
		width = max(width, len(m.Label))
	}
	p := &printer{w: w, width: width, done: make([]bool, len(mine)), got: make([]Verdict, len(mine))}

	queue := make(chan group, len(here))
	for _, g := range dispatchOrder(here) {
		queue <- g
	}
	close(queue)

	var wg sync.WaitGroup
	for _, wk := range workers {
		wg.Add(1)
		go func(wk *worker) {
			defer wg.Done()
			for g := range queue {
				wk.group(t, g, func(row int, v Verdict) { p.put(at[row], v) })
			}
		}(wk)
	}
	wg.Wait()
	return p.got, nil
}

// group proves one pattern group: its baseline once, then each row.
//
// ⛔ THE BASELINE RUNS BEFORE THE GROUP'S ROWS, IN THE SAME COPY, AND NEVER
// BESIDE ONE OF THEM. One worker owns the whole group, so nothing of the same
// pattern runs in that copy while the baseline does.
func (wk *worker) group(t *Table, g group, put func(row int, v Verdict)) {
	first := t.Mutations[g.rows[0]]
	dest, err := wk.dir(first.Module)
	if err != nil {
		for _, r := range g.rows {
			put(r, Verdict{Label: t.Mutations[r].Label, State: "BROKEN", Reason: "the module could not be staged: " + err.Error()})
		}
		return
	}
	b := wk.baseline(dest, first)
	for _, r := range g.rows {
		put(r, wk.one(t.Mutations[r], b))
	}
}

type baseline struct {
	green   bool
	cases   int
	skipped int
	// pkgs are the packages the row's cases ran in, unmutated.
	pkgs []string
	why  string
}

// one runs the named cases against the mutated copy. The baseline has already
// run them UNMUTATED.
//
// ⛔ THE UNMUTATED RUN IS FINDING 1, AND IT IS THE OLDEST IN THE RECORD. Without
// it a case that was ALREADY FAILING reported "went red" when the guard was
// removed, because the harness only ever saw red and never asked what red meant.
// That is a harness certifying a guard it never tested, which is the exact class
// this harness exists to catch, in the instrument that catches it.
func (wk *worker) one(m Mutation, b baseline) Verdict {
	v := Verdict{Label: m.Label, State: "BROKEN"}

	// ⛔ THE ROW IS REFUSED IF ITS CASES WERE NOT GREEN UNMUTATED. Finding 1.
	// A case already red reports "went red" the moment a guard is deleted, and
	// there is no way to tell that from a guard that works.
	if !b.green {
		v.Cases, v.Skipped = b.cases, b.skipped
		v.Reason = "the case was not green BEFORE the mutation: " + b.why
		return v
	}

	dest, err := wk.dir(m.Module)
	if err != nil {
		v.Reason = "the module could not be staged: " + err.Error()
		return v
	}
	path := filepath.Join(dest, filepath.FromSlash(m.File))
	body, err := os.ReadFile(path)
	if err != nil {
		v.Reason = "the file could not be read: " + err.Error()
		return v
	}
	// ⛔ EXACTLY ONE MATCH. Two means the mutation hits somewhere it was not
	// aimed, and zero means the code moved and this row has been proving
	// nothing since. Both are BROKEN rather than a pass.
	if n := strings.Count(string(body), m.Find); n != 1 {
		v.Reason = fmt.Sprintf("matched %d times, not once", n)
		return v
	}
	changed := strings.Replace(string(body), m.Find, m.Replace, 1)
	if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
		v.Reason = "the change could not be written: " + err.Error()
		return v
	}
	// ⛔ RESTORED BEFORE THIS FUNCTION RETURNS, BY ANY ROUTE, and the restore is
	// VERIFIED. Every later row this worker runs in the module shares the copy,
	// so a mutation left behind would be reported as the next row's guard
	// failing. Where the restore does not take, the copy is dropped, and the
	// next row makes a fresh one rather than run on a dirty tree.
	defer func() {
		if err := os.WriteFile(path, body, 0o600); err != nil {
			wk.drop(m.Module)
			return
		}
		back, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(back, body) {
			wk.drop(m.Module)
		}
	}()

	// ⭐ ONLY THE PACKAGES THAT HOLD THE CASES, AND NO VET. Under `./...` a
	// toolkit row compiled and vetted the plain package and its test variant,
	// and linked two test binaries. Only one of them held a case. Measured: 7.3
	// s of action graph down to 5.0 s. ⚠ Vet stays in the gate, which runs it
	// over the tree as committed.
	args := []string{"-json", "-vet=off", "-count=1", "-run", m.Run}
	args = append(args, b.pkgs...)
	args = append(args, m.Args...)
	r, err := wk.goTest(dest, args...)
	return judge(v, r, err)
}

// judge turns the mutated run into a verdict.
//
// ⭐ A BUILD FAILURE IS STILL TOLD APART FROM A GUARD GOING RED, and the
// baseline is why it can be. It proved the module compiles and the pattern
// reaches a case UNMUTATED. So a case package that does not build now is the
// mutation's doing.
func judge(v Verdict, r testRun, err error) Verdict {
	v.State = "BROKEN"
	v.Cases, v.Skipped = r.cases, r.skipped
	switch {
	case len(r.buildFailed) > 0:
		// Finding 41: the compiler's own line, because the ROW is what has to
		// be rewritten. Deleting a guard regularly leaves the variable it read
		// unused, and a row can avoid that by comparing against a value the
		// setting never takes.
		v.Reason = "the MUTATED module does not compile, so this row proves nothing: " + buildFailure(r.text)
	case r.failed > 0:
		v.State = "ok"
	case err != nil:
		// ⛔ NOT "ok". Something failed, and it was not a case this row names:
		// a TestMain, or a package init. The row should name what actually
		// catches the mutation.
		v.Reason = fmt.Sprintf("no case it names failed, yet the run did, after %d case(s): %s", r.cases, testFailure(r.text))
	case r.skipped >= r.cases:
		// ⛔ BEFORE THE THEATRE BRANCH, because from here the two are the same
		// output: cases ran and the process exited 0. A skip proves nothing
		// about the guard and says nothing against the test.
		v.State = "SKIPPED"
		v.Reason = "every case it names skipped itself on this host"
	default:
		v.State = "THEATRE"
	}
	return v
}

// baseline runs the row's cases in an UNMUTATED copy, once per pattern group,
// across the whole module, and records the packages they ran in.
//
// ⚠ IT COSTS ONE EXTRA TEST RUN PER DISTINCT PATTERN, not per row, and that is
// not small. The table of 1,202 rows held 772 patterns, so two runs in five
// were baselines. That is why a shard never splits a group.
func (wk *worker) baseline(dest string, m Mutation) baseline {
	args := append([]string{"-json", "-vet=off", "-count=1", "-run", m.Run, "./..."}, m.Args...)
	r, err := wk.goTest(dest, args...)
	if err != nil {
		// ⛔ ONE RETRY, AND ONLY ON A FAILURE. Measured on 2026-09-17: a
		// baseline failed once inside a 21-row pass and passed alone moments
		// later, with no code between the two runs. The staging churns a
		// temporary directory hard, and a Windows delete is asynchronous, so a
		// transient failure there turns a GOOD row into BROKEN.
		//
		// ⭐ IT CANNOT MASK A RED CASE. A case that genuinely fails, fails both
		// times; retrying a negative only discards a flake. Retrying a PASS
		// would be the dangerous direction and is not done.
		r, err = wk.goTest(dest, args...)
	}
	got := baseline{cases: r.cases, skipped: r.skipped, pkgs: r.pkgs}
	switch {
	case got.cases == 0:
		// ⛔ NAMED HERE RATHER THAN AFTER THE MUTATION. A pattern that matches
		// nothing is a row pointing at a case that has been renamed or deleted,
		// and finding that out before the mutation says so plainly.
		got.why = fmt.Sprintf("0 cases matched %q, so this row names no test", m.Run)
	case err != nil:
		got.why = "it fails unmutated, TWICE: " + testFailure(r.text)
	default:
		got.green = true
	}
	return got
}

// buildFailure and testFailure pick the line worth reporting out of a
// toolchain's output.
//
// ⛔ THE FIRST REAL LINE, NOT THE LAST. `go build` prints the package header
// first and the error under it, and `go test` ends with a summary that says
// only FAIL. A reader needs the line naming the file.
func buildFailure(out string) string { return firstUseful(out, "#") }

func testFailure(out string) string { return firstUseful(out, "") }

// noise are the lines a toolchain prints that say nothing about a failure.
//
// ⛔ `?   pkg [no test files]` IS THE ONE THAT CAUGHT THIS OUT. It is the first
// line `go test ./...` prints for a module whose root package holds no tests,
// so a reporter taking the first line reported it as the reason a case failed,
// over a real failure forty lines below. A reason that names the wrong thing
// sends a reader to the wrong file, which is worse than no reason.
var noise = []string{"=== RUN", "--- PASS", "=== CONT", "=== PAUSE", "?   ", "ok  ", "--- SKIP"}

func firstUseful(out, skipPrefix string) string {
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(strings.TrimRight(ln, "\r"))
		if ln == "" || ln == "FAIL" || ln == "PASS" {
			continue
		}
		if skipPrefix != "" && strings.HasPrefix(ln, skipPrefix) {
			continue
		}
		skip := false
		for _, n := range noise {
			if strings.HasPrefix(ln, strings.TrimSpace(n)) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if len(ln) > 200 {
			ln = ln[:200] + "..."
		}
		return ln
	}
	return "the toolchain said nothing"
}

// sweepStaleCopies removes staged module copies that an earlier run was KILLED
// before it could clean up.
//
// ⛔ FINDING 32. A copy made with os.MkdirTemp is removed by a defer, which a
// killed process never runs. So the copy survives in whatever TEMP named. One
// was measured at 127 MiB, sitting since 2026-09-14. `wsl-toolkit gc` does not
// cover it, because the directory is not that tool's job state. The fixed slots
// have their own sweep in pool.go. This one collects a worker's private
// fallback copies, and those an older build left.
//
// ⚠ AN AGE THRESHOLD, because a concurrent run's copy is live. Anything this
// process did not make and has not been touched for an hour was left behind.
func sweepStaleCopies(w io.Writer) {
	dir := os.TempDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	removed, bytes := 0, int64(0)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "mutate-") {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < time.Hour {
			continue
		}
		p := filepath.Join(dir, e.Name())
		size := treeSize(p)
		if err := os.RemoveAll(p); err != nil {
			continue
		}
		removed++
		bytes += size
	}
	if removed > 0 && w != nil {
		fmt.Fprintf(w, "  swept %d staged cop(y/ies) a killed run left behind, %d MiB\n", removed, bytes>>20)
	}
}

func treeSize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// copyTree copies a module. ⚠ Regular files and directories only: a module
// holds source, and following anything else would be copying whatever a symlink
// happened to point at on this host.
//
// ⛔ THE COPY WAS THE COST, AND IT WAS PAID 419 TIMES. Measured on 2026-09-17:
// the copy was 3.13 s of a 6.4 s wsl-toolkit row. So each worker copies a module
// once and every row it runs there reuses it, restoring the mutated file byte
// for byte. The row still runs against a copy and never against the tree.
func copyTree(src, dest string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, 0o700)
		case info.Mode().IsRegular():
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(target, b, 0o600)
		default:
			return nil
		}
	})
}

// verdictLine is one row's line in the progress log.
func verdictLine(v Verdict, width int) string {
	switch v.State {
	case "ok":
		return fmt.Sprintf("  ok       %-*s  %d case(s), went red\n", width, v.Label, v.Cases)
	case "THEATRE":
		return fmt.Sprintf("  THEATRE  %-*s  %d case(s), still green\n", width, v.Label, v.Cases)
	case "SKIPPED":
		return fmt.Sprintf("  SKIPPED  %-*s  %d case(s), all skipped here\n", width, v.Label, v.Cases)
	}
	return fmt.Sprintf("  BROKEN   %-*s  (%s)\n", width, v.Label, v.Reason)
}

// Report writes the tally and returns the exit code.
//
// ⛔ A THEATRE ROW IS A TEST TO FIX AND A BROKEN ROW IS A CLAIM NOBODY IS
// CHECKING, and either one fails the run. ⚠ A SKIPPED row does not: it is a
// claim THIS host could not check, it is listed by name so it cannot be
// mistaken for a proved one, and the ubuntu CI job is where it gets answered.
func Report(w io.Writer, verdicts []Verdict) int {
	proved := 0
	var theatre, skipped, broken []Verdict
	for _, v := range verdicts {
		switch v.State {
		case "ok":
			proved++
		case "THEATRE":
			theatre = append(theatre, v)
		case "SKIPPED":
			skipped = append(skipped, v)
		default:
			broken = append(broken, v)
		}
	}
	fmt.Fprintf(w, "\n%d of %d guards proved.\n", proved, len(verdicts))
	for _, v := range theatre {
		fmt.Fprintf(w, "  THEATRE: %s: %d case(s) ran and stayed green with the guard removed\n", v.Label, v.Cases)
	}
	for _, v := range skipped {
		fmt.Fprintf(w, "  SKIPPED: %s: %s, so it is unproved rather than proved or theatre\n", v.Label, v.Reason)
	}
	for _, v := range broken {
		fmt.Fprintf(w, "  BROKEN:  %s: %s\n", v.Label, v.Reason)
	}
	if len(theatre) > 0 || len(broken) > 0 {
		return 1
	}
	return 0
}
