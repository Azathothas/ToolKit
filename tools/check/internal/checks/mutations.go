// mutations.go - the mutation table still points at the code it claims to prove.
//
// ⛔ WHY A SECOND INSTRUMENT FOR AN INSTRUMENT. tools/repo/mutations.json is a
// table of claims: delete this line, and the test named beside it goes red.
// `repo mutate` proves every one of them by copying the module, applying the
// change and running the case, which takes about ten minutes. Nothing runs it
// on a schedule, so between two runs the table rots silently: a row whose code
// moved matches nothing, applies nothing, and the guard it names goes unproved
// with no one told.
//
// ⭐ IT HAS ALREADY HAPPENED, TWICE, AND IN ONE SESSION. `Extract`'s signature
// changed, so the row for the destination collision set stopped matching; the
// verdict rule moved out of cmd_run.go into internal/toolkit/job.go, so the row
// for the transfer branch stopped matching. Both were found by a full pass, and
// a full pass is exactly what nobody runs on the day the code moves.
//
// ⚠ THIS CHECK DOES NOT PROVE A GUARD, and must not be read as doing so. It
// asserts that each row still ADDRESSES something: the module is a module, the
// file is tracked, the find string is present exactly once, the replacement is
// a real change, and the case it names exists. Whether the case goes red with
// the guard gone is `repo mutate`'s answer and this one cannot substitute for
// it. What it buys is that the answer is never stale by more than one gate run.
//
// ⚠ THE SCHEMA IS NOT ASSERTED HERE. tools/repo owns it and reads it at run
// time, and these are two Go modules that cannot import each other, so a copy
// of the version string in this file would be a second home for a fact that
// only one of them enforces.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"encoding/json"
	"strings"
)

// MutationTable is the tracked table, relative to the repository root.
const MutationTable = "tools/repo/mutations.json"

// mutationRow is the subset of a row this check reads. ⚠ Deliberately not the
// harness's full struct: the fields it does not name are the harness's business
// and copying them here would make this file drift the moment one is added.
type mutationRow struct {
	Label   string `json:"label"`
	Module  string `json:"module"`
	File    string `json:"file"`
	Find    string `json:"find"`
	Replace string `json:"replace"`
	Run     string `json:"run"`
}

// Mutations asserts every row of the mutation table still reaches its subject.
func Mutations(t *Tree) Result {
	r := Result{Extra: map[string]any{}}

	raw := t.Read(MutationTable)
	if len(raw) == 0 {
		r.bad("%s is missing, so every guard it proves is unproved and nothing says so", MutationTable)
		return r
	}
	var table struct {
		Mutations []mutationRow `json:"mutations"`
	}
	if err := json.Unmarshal(raw, &table); err != nil {
		r.bad("%s does not parse as a mutation table: %v", MutationTable, err)
		return r
	}
	if len(table.Mutations) == 0 {
		// ⛔ The harness refuses an empty table for this reason and so does
		// this: a table holding nothing reports success over nothing.
		r.bad("%s holds no rows, so it proves nothing and reports no problem", MutationTable)
		return r
	}
	r.Extra["rows"] = len(table.Mutations)

	// The set of tracked files, and the test functions each module defines.
	// ⚠ Built once: the alternative is a tree walk per row, and the table is
	// long enough that it showed up in the gate's own timing.
	tracked := make(map[string]bool, len(t.Files))
	for _, f := range t.Files {
		tracked[f] = true
	}
	testFuncs := map[string]map[string]bool{}

	seen := map[string]bool{}
	for _, m := range table.Mutations {
		switch {
		case m.Label == "":
			r.bad("%s: a row has no label, and the report and the --only filter both key on it", MutationTable)
			continue
		case seen[m.Label]:
			r.bad("%s: two rows are labelled %q, so one of them cannot be named or reported separately", MutationTable, m.Label)
			continue
		}
		seen[m.Label] = true

		if m.Module == "" || m.File == "" || m.Find == "" || m.Run == "" {
			r.bad("%s: %q is missing one of module, file, find or run", MutationTable, m.Label)
			continue
		}
		if !tracked[m.Module+"/go.mod"] {
			r.bad("%s: %q names module %q, which has no tracked go.mod", MutationTable, m.Label, m.Module)
			continue
		}
		path := m.Module + "/" + m.File
		if !tracked[path] {
			r.bad("%s: %q names %s, which is not tracked", MutationTable, m.Label, path)
			continue
		}
		// ⛔ EXACTLY ONE, which is the harness's own rule. Zero means the code
		// moved and this row has been proving nothing since; two means the
		// mutation lands somewhere it was not aimed.
		if n := strings.Count(string(t.Read(path)), m.Find); n != 1 {
			r.bad("%s: %q matches %s %d times, not once, so `repo mutate` cannot apply it", MutationTable, m.Label, path, n)
			continue
		}
		if m.Replace == m.Find {
			r.bad("%s: %q replaces its find string with itself, so the case runs against unchanged code and can only stay green", MutationTable, m.Label)
		}
		// ⚠ A `-run` pattern is a regular expression and this reads it as a
		// list of literal names, which is what every row uses. A row that
		// needed a real pattern would be reported here, and the answer then is
		// to name the cases rather than to loosen this.
		funcs, ok := testFuncs[m.Module]
		if !ok {
			funcs = moduleTestFuncs(t, m.Module)
			testFuncs[m.Module] = funcs
		}
		for _, want := range strings.Split(m.Run, "|") {
			if want == "" {
				r.bad("%s: %q has an empty alternative in its run pattern", MutationTable, m.Label)
				continue
			}
			if !funcs[want] {
				r.bad("%s: %q names case %s, which no test file under %s defines", MutationTable, m.Label, want, m.Module)
			}
		}
	}
	return r
}

// moduleTestFuncs collects the test functions a module defines, by name.
func moduleTestFuncs(t *Tree, module string) map[string]bool {
	out := map[string]bool{}
	prefix := module + "/"
	for _, f := range t.Files {
		if !strings.HasPrefix(f, prefix) || !strings.HasSuffix(f, "_test.go") {
			continue
		}
		for _, ln := range Lines(t.Read(f)) {
			name, ok := testFuncName(ln.Text)
			if ok {
				out[name] = true
			}
		}
	}
	return out
}

// testFuncName reads `func TestX(` and returns TestX. ⚠ Text, not go/parser: a
// gate that parses every test file in the tree to answer one question pays for
// it on every run, and the declaration this reads is one line by gofmt's rule.
func testFuncName(line string) (string, bool) {
	const decl = "func Test"
	if !strings.HasPrefix(line, decl) {
		return "", false
	}
	i := strings.IndexByte(line, '(')
	if i < len(decl) {
		return "", false
	}
	return strings.TrimSpace(line[len("func "):i]), true
}
