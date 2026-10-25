// plan_test.go - which rows a run selects, and how the shards are cut.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"fmt"
	"strings"
	"testing"
)

// TestTheHeaviestGroupIsPlacedFirst: three groups of cost 1, 1 and 2 over two
// shards balance at 2 and 2 only when the 2 is placed first.
//
// ⚠ IN TABLE ORDER THEY LAND AT 3 AND 1, because the heaviest is placed last.
func TestTheHeaviestGroupIsPlacedFirst(t *testing.T) {
	light, heavy := group{cost: 1}, group{cost: 2}
	groups := []group{light, light, heavy}
	plan := planShards(groups, 2)
	load := make([]int, 2)
	for i, k := range plan {
		load[k] += groups[i].cost
	}
	if load[0] != 2 || load[1] != 2 {
		t.Fatalf("the shards weigh %v, want 2 and 2", load)
	}
}

// TestWorkersStartWithTheHeaviestGroup: a heavy group handed out last keeps
// one worker busy after the others are done.
func TestWorkersStartWithTheHeaviestGroup(t *testing.T) {
	var groups []group
	for _, c := range []int{1, 5, 3, 5} {
		groups = append(groups, group{cost: c, rows: []int{len(groups)}})
	}
	var got []int
	for _, g := range dispatchOrder(groups) {
		got = append(got, g.rows[0])
	}
	// Costs 5, 5, 3, 1, and the two of 5 in table order.
	if want := []int{1, 3, 2, 0}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("the groups are handed out as rows %v, want %v", got, want)
	}
}

// TestAToolkitRowWeighsMost: the plan weighs a row by what it rebuilds. A row
// in internal/toolkit rebuilds about 57,000 lines and main over them.
func TestAToolkitRowWeighsMost(t *testing.T) {
	cases := []struct {
		module, file string
		want         int
	}{
		{toolkitModule, "internal/toolkit/configlayer.go", weightToolkit},
		{toolkitModule, "internal/toolkit/doors.sh", weightToolkit},
		{toolkitModule, "cmd_run.go", weightMain},
		{toolkitModule, "internal/toolkit/adapters/muse/probe.sh", weightOther},
		{"tools/check", "internal/checks/docs.go", weightOther},
	}
	for _, c := range cases {
		if got := rowWeight(Mutation{Module: c.module, File: c.file}); got != c.want {
			t.Errorf("%s %s weighs %d, want %d", c.module, c.file, got, c.want)
		}
	}
	if !(weightToolkit > 3*weightMain && weightMain > weightOther) {
		t.Fatalf("the weights %d, %d and %d no longer order toolkit over main over the rest", weightToolkit, weightMain, weightOther)
	}
}

func selectionTable() *Table {
	a := Mutation{Label: "a guard", Module: "tools/repo"}
	b := Mutation{Label: "a guard, again", Module: "tools/repo"}
	c := Mutation{Label: "a check", Module: "tools/check"}
	return &Table{Schema: TableSchema, Mutations: []Mutation{a, b, c}}
}

// TestAnExactLabelSelectsThatRowAlone. ⛔ --only matches part of a label, so
// "a guard" also ran "a guard, again". A caller that names the rows it added
// must get those rows and no others.
func TestAnExactLabelSelectsThatRowAlone(t *testing.T) {
	rows, left, err := choose(selectionTable(), Selection{Labels: []string{"a guard"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0] != 0 {
		t.Fatalf("--label \"a guard\" selected rows %v, want only row 0", rows)
	}
	if len(left) != 2 {
		t.Fatalf("%d row(s) were reported as left out, want 2", len(left))
	}
	if rows, _, _ := choose(selectionTable(), Selection{Only: "a guard"}); len(rows) != 2 {
		t.Fatalf("--only \"a guard\" selected %v; it is the substring filter and should match two", rows)
	}
}

// TestAnUnknownExactLabelIsNamed. ⛔ A typo in a list of labels is an error that
// names it, not a smaller run.
func TestAnUnknownExactLabelIsNamed(t *testing.T) {
	_, _, err := choose(selectionTable(), Selection{Labels: []string{"a guard", "a gaurd"}})
	if err == nil || !strings.Contains(err.Error(), `"a gaurd"`) {
		t.Fatalf("an unknown label answered %v", err)
	}
}

// TestAModuleFilterKeepsThatModule, and a module no row names is an error.
func TestAModuleFilterKeepsThatModule(t *testing.T) {
	rows, left, err := choose(selectionTable(), Selection{Modules: []string{"tools/check/"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0] != 2 {
		t.Fatalf("--module tools/check selected rows %v, want only row 2", rows)
	}
	if len(left) != 2 || !strings.Contains(left[0].why, "--module") {
		t.Fatalf("the rows left out say %+v", left)
	}
	if _, _, err := choose(selectionTable(), Selection{Modules: []string{"tools/nowhere"}}); err == nil || !strings.Contains(err.Error(), "tools/nowhere") {
		t.Fatalf("a module no row names answered %v", err)
	}
}

// TestALabelsFileReadsEitherLineEnding: a file written on Windows ends its
// lines in CRLF, and a label read with the CR on it matches no row.
func TestALabelsFileReadsEitherLineEnding(t *testing.T) {
	got, err := ReadLabels(strings.NewReader("a guard\r\na check\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != "a guard|a check" {
		t.Fatalf("read %q", got)
	}
	// ⛔ AN EMPTY FILE IS NOT "NO FILTER". An empty list selects every row, so
	// a caller with nothing to prove would run the whole table and pass.
	if _, err := ReadLabels(strings.NewReader("\r\n\n")); err == nil {
		t.Fatal("a file with no label was read as an empty list")
	}
}
