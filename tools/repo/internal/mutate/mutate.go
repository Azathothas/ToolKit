// mutate.go - delete each guard and confirm the test named for it goes red.
//
// ⛔ A TEST THAT STAYS GREEN WITH ITS SUBJECT REMOVED IS THEATRE, and it is
// worse than no test because it is read as coverage. This repository has found
// two: a guard whose case never exercised it, and a ledger stress case that went
// red in 0 of 10 runs against the defect it was written for.
//
// ⭐ THREE OUTCOMES, NOT TWO, and that is the whole design. A previous harness
// reported "green with the guard gone" for three different things: a test that
// really did not cover its subject, a `-run` pattern that matched NO test, and a
// mutation that did not COMPILE. The last two prove nothing and must not read as
// the first. So every row reports the case count and the build status separately:
//
//	ok       the guard is real: the mutation built, cases ran, and they went red
//	THEATRE  it built, cases ran, and they stayed green
//	BROKEN   it did not build, or matched no case, or the find was not unique
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
	Label  string
	State  string // "ok", "THEATRE" or "BROKEN"
	Cases  int
	Reason string
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
	switch {
	case v.Cases == 0:
		v.Reason = fmt.Sprintf("0 cases matched %q", m.Run)
	case testErr == nil:
		v.State = "THEATRE"
	default:
		v.State = "ok"
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

// Report writes the tally and returns the exit code. ⛔ Anything other than
// every row proved is a failure: a THEATRE row is a test to fix and a BROKEN row
// is a claim nobody is checking.
func Report(w io.Writer, verdicts []Verdict) int {
	proved := 0
	var theatre, broken []Verdict
	for _, v := range verdicts {
		switch v.State {
		case "ok":
			proved++
		case "THEATRE":
			theatre = append(theatre, v)
		default:
			broken = append(broken, v)
		}
	}
	fmt.Fprintf(w, "\n%d of %d guards proved.\n", proved, len(verdicts))
	for _, v := range theatre {
		fmt.Fprintf(w, "  THEATRE: %s: %d case(s) ran and stayed green with the guard removed\n", v.Label, v.Cases)
	}
	for _, v := range broken {
		fmt.Fprintf(w, "  BROKEN:  %s: %s\n", v.Label, v.Reason)
	}
	if proved != len(verdicts) {
		return 1
	}
	return 0
}
