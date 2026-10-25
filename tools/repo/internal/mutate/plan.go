// plan.go - which rows one run proves, and in what share.
//
// ⛔ NOTHING IS LEFT OUT IN SILENCE. A row a filter or a shard leaves out is
// counted per module and printed with the reason. A run that names only what it
// did reads as the whole table.
//
// ⭐ A SHARD HOLDS WHOLE PATTERN GROUPS. Rows that share a module, a -run
// pattern and extra arguments share one baseline, the unmutated run that proves
// their cases are green. The old plan dealt row i to shard i mod N, which
// scattered every group and paid 1,156 baselines for 772 patterns over six
// shards. A group now lives in one shard and its baseline runs once.
//
// ⚠ THE PLAN IS A PURE FUNCTION OF THE TABLE AND THE FILTERS. Every runner
// computes it alone, so it reads no clock, no host and no map order.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// group is the rows that share one baseline.
type group struct {
	key  string
	rows []int // indexes into the table, ascending
	cost int
}

// groupKey is what makes two rows share a baseline.
func groupKey(m Mutation) string {
	return m.Module + "\x00" + m.Run + "\x00" + strings.Join(m.Args, "\x1f")
}

// The weights are in halves, so a toolkit row is 10 and a plain one 2.
//
// ⭐ MEASURED, NOT GUESSED. A row that changes internal/toolkit recompiles a
// package of about 57,000 lines and the main package over it. It took a median
// 8.17 s in CI. A wsl-toolkit main-package row rebuilds main alone, and a row
// of another module builds a small one.
const (
	weightToolkit = 10
	weightMain    = 3
	weightOther   = 2
)

const toolkitModule = "tools/windows/wsl-toolkit"

// rowWeight is what one row costs, read from its module and file alone.
func rowWeight(m Mutation) int {
	if m.Module != toolkitModule {
		return weightOther
	}
	switch path.Dir(m.File) {
	case "internal/toolkit":
		return weightToolkit
	case ".":
		return weightMain
	}
	return weightOther
}

// baselineWeight is one baseline's cost. ⚠ A wsl-toolkit baseline tests every
// package of the module, so it costs what its heaviest row does.
func baselineWeight(module string) int {
	if module == toolkitModule {
		return weightToolkit
	}
	return weightOther
}

// groupRows collects the selected rows into groups, in the order each group's
// first row appears in the table.
func groupRows(t *Table, rows []int) []group {
	var out []group
	at := map[string]int{}
	for _, i := range rows {
		m := t.Mutations[i]
		k := groupKey(m)
		g, ok := at[k]
		if !ok {
			g = len(out)
			at[k] = g
			out = append(out, group{key: k, cost: baselineWeight(m.Module)})
		}
		out[g].rows = append(out[g].rows, i)
		out[g].cost += rowWeight(m)
	}
	return out
}

// planShards gives each group a shard, 0 to n-1, longest first: the heaviest
// group goes to the lightest shard. Ties break on table order and on the lower
// shard, so every runner computes the same plan.
//
// ⛔ EVERY GROUP GETS EXACTLY ONE SHARD, and so does every row in it. A case
// asserts that over the real table for several N.
func planShards(groups []group, n int) []int {
	plan := make([]int, len(groups))
	if n <= 1 {
		return plan
	}
	order := make([]int, len(groups))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return groups[order[a]].cost > groups[order[b]].cost
	})
	load := make([]int, n)
	for _, g := range order {
		best := 0
		for k := 1; k < n; k++ {
			if load[k] < load[best] {
				best = k
			}
		}
		plan[g] = best
		load[best] += groups[g].cost
	}
	return plan
}

// dispatchOrder is the order a run hands its groups to its workers: heaviest
// first, so the last group to start is a small one and no worker finishes long
// after the others. Ties keep table order.
func dispatchOrder(groups []group) []group {
	order := append([]group(nil), groups...)
	sort.SliceStable(order, func(a, b int) bool { return order[a].cost > order[b].cost })
	return order
}

// splitShard keeps the groups the plan gives to shard s and returns the rest.
// The zero shard keeps everything.
func splitShard(groups []group, s Shard) (here, others []group) {
	if s.N <= 1 {
		return groups, nil
	}
	plan := planShards(groups, s.N)
	for i, g := range groups {
		if plan[i] == s.K-1 {
			here = append(here, g)
		} else {
			others = append(others, g)
		}
	}
	return here, others
}

// leftOut is one row a run does not prove, and why.
type leftOut struct {
	row int
	why string
}

// choose applies the filters in sel to the table.
//
// ⛔ AN EXACT LABEL THAT NO ROW CARRIES IS AN ERROR NAMING IT. A caller who
// lists the rows it added and gets a typo back as "nothing to run" has been told
// the rows passed.
func choose(t *Table, sel Selection) (rows []int, left []leftOut, err error) {
	labels := map[string]bool{}
	for _, l := range sel.Labels {
		labels[l] = true
	}
	modules := map[string]bool{}
	for _, m := range sel.Modules {
		modules[strings.TrimSuffix(strings.ReplaceAll(m, `\`, "/"), "/")] = true
	}
	known := map[string]bool{}
	knownModules := map[string]bool{}
	for _, m := range t.Mutations {
		known[m.Label] = true
		knownModules[m.Module] = true
	}
	for _, l := range sel.Labels {
		if !known[l] {
			return nil, nil, fmt.Errorf("no row is labelled exactly %q", l)
		}
	}
	for m := range modules {
		if !knownModules[m] {
			return nil, nil, fmt.Errorf("--module %q names no row's module", m)
		}
	}
	for i, m := range t.Mutations {
		switch {
		case len(modules) > 0 && !modules[m.Module]:
			left = append(left, leftOut{i, "its module is not one --module names"})
		case len(labels) > 0 && !labels[m.Label]:
			left = append(left, leftOut{i, "its label is not one --label or --labels-file names"})
		case sel.Only != "" && !strings.Contains(m.Label, sel.Only):
			left = append(left, leftOut{i, fmt.Sprintf("its label does not contain %q", sel.Only)})
		default:
			rows = append(rows, i)
		}
	}
	return rows, left, nil
}

// reportLeftOut prints one line per module and reason, in table order.
func reportLeftOut(w io.Writer, t *Table, left []leftOut) {
	type key struct{ module, why string }
	count := map[key]int{}
	var order []key
	for _, l := range left {
		k := key{t.Mutations[l.row].Module, l.why}
		if count[k] == 0 {
			order = append(order, k)
		}
		count[k]++
	}
	for _, k := range order {
		fmt.Fprintf(w, "  not run: %d row(s) of %s, because %s\n", count[k], k.module, k.why)
	}
}

// ReadLabels reads one exact label per line, LF or CRLF. Blank lines are
// skipped.
//
// ⛔ A FILE WITH NO LABEL IS AN ERROR. An empty list means "no filter" to the
// selection, so an empty file would quietly run the whole table.
//
// ⚠ NO CR IS TRIMMED HERE, because bufio.ScanLines drops it itself. A trim
// that sat here was reported THEATRE by this harness, and it was right.
func ReadLabels(r io.Reader) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		ln := sc.Text()
		if ln == "" {
			continue
		}
		out = append(out, ln)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("it names no label, and an empty list would select every row")
	}
	return out, nil
}
