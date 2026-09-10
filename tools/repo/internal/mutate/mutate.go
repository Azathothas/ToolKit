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
//	ok       the guard is real: the mutation built, cases ran, and they went red
//	THEATRE  it built, cases ran, and they stayed green
//	SKIPPED  it built, and every case it names skipped itself on this host
//	BROKEN   it did not build, or matched no case, or the find was not unique
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
// ⚠ THE TABLE IS DATA, in mutations.json beside this module. A mutation is a
// claim about one guard, and claims belong where they can be read without
// recompiling the thing that checks them.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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

var runLine = regexp.MustCompile(`(?m)^=== RUN\s+Test`)

// skipLine counts the cases that excused themselves. ⚠ `--- SKIP:` is indented
// for a subtest, so the anchor is the marker rather than the start of the line.
var skipLine = regexp.MustCompile(`(?m)^\s*--- SKIP:\s+Test`)

// Run applies every mutation and reports one verdict each. Progress is written
// to w as it goes, because a full pass takes minutes and a silent one reads as
// a hang.
func Run(root string, t *Table, only string, w io.Writer) ([]Verdict, error) {
	width := 0
	for _, m := range t.Mutations {
		if len(m.Label) > width {
			width = len(m.Label)
		}
	}
	var out []Verdict
	for _, m := range t.Mutations {
		if only != "" && !strings.Contains(m.Label, only) {
			continue
		}
		v := one(root, m)
		out = append(out, v)
		switch v.State {
		case "ok":
			fmt.Fprintf(w, "  ok       %-*s  %d case(s), went red\n", width, v.Label, v.Cases)
		case "THEATRE":
			fmt.Fprintf(w, "  THEATRE  %-*s  %d case(s), still green\n", width, v.Label, v.Cases)
		case "SKIPPED":
			fmt.Fprintf(w, "  SKIPPED  %-*s  %d case(s), all skipped here\n", width, v.Label, v.Cases)
		default:
			fmt.Fprintf(w, "  BROKEN   %-*s  (%s)\n", width, v.Label, v.Reason)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no mutation's label contains %q", only)
	}
	return out, nil
}

// one copies the module, applies the change, and asks the named cases.
func one(root string, m Mutation) Verdict {
	v := Verdict{Label: m.Label, State: "BROKEN"}

	tmp, err := os.MkdirTemp("", "mutate-")
	if err != nil {
		v.Reason = "no temporary directory: " + err.Error()
		return v
	}
	defer os.RemoveAll(tmp)
	dest := filepath.Join(tmp, "t")
	if err := copyTree(filepath.Join(root, filepath.FromSlash(m.Module)), dest); err != nil {
		v.Reason = "the module could not be copied: " + err.Error()
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

	if out, err := run(dest, "go", "build", "./..."); err != nil {
		v.Reason = "does not compile"
		_ = out
		return v
	}

	args := append([]string{"test", "./...", "-run", m.Run, "-count=1", "-v"}, m.Args...)
	out, testErr := run(dest, "go", args...)
	v.Cases = len(runLine.FindAllString(out, -1))
	v.Skipped = len(skipLine.FindAllString(out, -1))
	switch {
	case v.Cases == 0:
		v.Reason = fmt.Sprintf("0 cases matched %q", m.Run)
	case testErr != nil:
		v.State = "ok"
	case v.Skipped >= v.Cases:
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

func run(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// copyTree copies a module. ⚠ Regular files and directories only: a module
// holds source, and following anything else would be copying whatever a symlink
// happened to point at on this host.
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
