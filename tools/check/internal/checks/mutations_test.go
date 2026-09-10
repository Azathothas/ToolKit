// mutations_test.go - the check that keeps the mutation table honest.
//
// ⛔ THE ROW THAT MATTERS IS THE STALE ONE. Every other rule here refuses
// something obviously malformed; the one that was actually paid for is a row
// that is well-formed, readable, committed, and points at a line the code no
// longer has. It costs nothing at read time and silently removes a guard from
// the set anybody believes is proved.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"strings"
	"testing"
)

// table builds a tree holding one module, one source file, one test file and a
// mutation table, so a case can make exactly one thing wrong.
func table(t *testing.T, rows string) *Tree {
	t.Helper()
	files := map[string][]byte{
		"tools/x/go.mod":            []byte("module example.test\n\ngo 1.25.0\n"),
		"tools/x/guard.go":          []byte("package x\n\nfunc F(s string) bool {\n\tif s == \"\" {\n\t\treturn false\n\t}\n\treturn true\n}\n"),
		"tools/x/x_test.go":         []byte("package x\n\nimport \"testing\"\n\nfunc TestEmptyIsRefused(t *testing.T) {}\n"),
		"tools/repo/mutations.json": []byte("{\"schema\":\"repo-mutations/1\",\"mutations\":[" + rows + "]}"),
	}
	tree := &Tree{Root: t.TempDir(), cache: map[string][]byte{}}
	for name, body := range files {
		tree.Files = append(tree.Files, name)
		tree.cache[name] = body
	}
	return tree
}

// good is a row that reaches everything it names.
const good = `{"label":"the empty-string guard","module":"tools/x","file":"guard.go",` +
	`"find":"\tif s == \"\" {","replace":"\tif false {","run":"TestEmptyIsRefused"}`

func TestMutationsAcceptsARowThatReachesItsSubject(t *testing.T) {
	if got := Mutations(table(t, good)); got.Problems != 0 {
		t.Fatalf("a row naming code that exists was refused: %v", got.Detail)
	}
}

// TestMutationsRefusesARowThatPointsAtNothing is the defect this check exists
// for. ⛔ The find string is the ONE field that rots silently: the code moves,
// the row keeps parsing, and `repo mutate` cannot apply it.
func TestMutationsRefusesARowThatPointsAtNothing(t *testing.T) {
	cases := []struct {
		name string
		row  string
		want string
	}{
		{
			"the code moved out from under the find string",
			`{"label":"a","module":"tools/x","file":"guard.go","find":"\tif s == nil {","replace":"x","run":"TestEmptyIsRefused"}`,
			"0 times",
		},
		{
			"the find string is not unique",
			`{"label":"a","module":"tools/x","file":"guard.go","find":"return","replace":"x","run":"TestEmptyIsRefused"}`,
			"2 times",
		},
		{
			"the file is not tracked",
			`{"label":"a","module":"tools/x","file":"gone.go","find":"x","replace":"y","run":"TestEmptyIsRefused"}`,
			"not tracked",
		},
		{
			"the module is not a module",
			`{"label":"a","module":"tools/nope","file":"guard.go","find":"x","replace":"y","run":"TestEmptyIsRefused"}`,
			"go.mod",
		},
		{
			"the case it names does not exist",
			`{"label":"a","module":"tools/x","file":"guard.go","find":"\tif s == \"\" {","replace":"\tif false {","run":"TestRenamedLastWeek"}`,
			"no test file",
		},
		{
			"one alternative of the pattern does not exist",
			`{"label":"a","module":"tools/x","file":"guard.go","find":"\tif s == \"\" {","replace":"\tif false {","run":"TestEmptyIsRefused|TestGone"}`,
			"TestGone",
		},
		{
			"the replacement is the find string",
			`{"label":"a","module":"tools/x","file":"guard.go","find":"\tif s == \"\" {","replace":"\tif s == \"\" {","run":"TestEmptyIsRefused"}`,
			"with itself",
		},
		{
			"two rows carry one label",
			good + "," + good,
			"labelled",
		},
		{
			// ⛔ THE ONE THAT LOOKS LIKE A PASS. Mutating the assertion makes
			// the named case go red and the harness prints `ok`, so the row
			// reads as a proved guard over a guard nothing touched.
			"the row mutates the test rather than the code",
			`{"label":"a","module":"tools/x","file":"x_test.go","find":"func TestEmptyIsRefused","replace":"func TestEmptyWasRefused","run":"TestEmptyIsRefused"}`,
			"which is a test",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Mutations(table(t, c.row))
			if got.Problems == 0 {
				t.Fatal("the row was accepted, so the guard it names is unproved and nothing says so")
			}
			if !strings.Contains(strings.Join(got.Detail, "\n"), c.want) {
				t.Fatalf("the finding does not say why: %v", got.Detail)
			}
		})
	}
}

// TestMutationsRefusesATableThatProvesNothing. ⛔ Missing and empty are the two
// ways this check could report success over a repository with no guards proved
// at all, which is the shape of every dead check this tree has found.
func TestMutationsRefusesATableThatProvesNothing(t *testing.T) {
	empty := Mutations(table(t, ""))
	if empty.Problems == 0 {
		t.Error("an empty table was reported as fine")
	}
	tree := table(t, good)
	tree.cache[MutationTable] = nil
	if gone := Mutations(tree); gone.Problems == 0 {
		t.Error("a missing table was reported as fine")
	}
}
