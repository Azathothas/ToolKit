// SPDX-License-Identifier: 0BSD

package checks

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// RecordDir is where the work record lives.
const RecordDir = "TODO/"

// Record asserts the work record agrees with itself.
//
// ⛔ THE WRITER DOES NOT GRADE ITSELF. scripts/common/set-record.mjs moves the
// numbers and prints this check's command; this reads the rows independently
// and says whether they add up. The incident behind that split is in
// docs/methodology/work-todo.md: a session closed two entries, wrote it into
// the entries, the index and the record, pushed, then rewrote a fourth file and
// never pushed again, so the published state said those entries were open
// beside entries saying done for the whole of the next session.
//
// Seven rules, and the arithmetic ones exist because a count is the part a
// careful person gets wrong:
//
//  1. every row in the index has an entry heading somewhere under TODO/;
//  2. the entry is in the file the row names;
//  3. the entry's own Status matches the row's;
//  4. every entry has a row, so nothing is worked on off the books;
//  5. the index's own total line agrees with its rows;
//  6. the per-priority table agrees, and so does PROGRESS.md's state line;
//  7. PROGRESS.md's work order agrees with the index about what is finished.
func Record(t *Tree) Result {
	r := Result{Extra: map[string]any{}}

	index := RecordDir + "INDEX.md"
	if len(t.Read(index)) == 0 {
		r.bad("%s", index+" is missing, and it is the list every other rule reads")
		return r
	}

	type entryRow struct {
		id, pri, eff, status, file string
		line                       int
	}
	var rows []entryRow
	for _, ln := range Lines(t.Read(index)) {
		cells := tableCells(ln.Text)
		if len(cells) < 6 || !entryIDOnly.MatchString(cells[0]) {
			continue
		}
		file := cells[5]
		if m := linkTargetOnly.FindStringSubmatch(file); m != nil {
			file = m[1]
		}
		rows = append(rows, entryRow{cells[0], cells[1], cells[2], cells[3], strings.TrimSpace(file), ln.N})
	}
	if len(rows) == 0 {
		r.bad("%s", index+" has no entry rows, so nothing can be checked against them")
		return r
	}

	// Where each entry heading actually is, and what Status it declares.
	headingIn := map[string]string{}
	statusIn := map[string]string{}
	for _, f := range t.Files {
		if !strings.HasPrefix(f, RecordDir) || !strings.HasSuffix(f, ".md") {
			continue
		}
		switch f {
		case index, RecordDir + "PROGRESS.md", RecordDir + "RULES.md":
			continue
		}
		current := ""
		for _, ln := range Lines(t.Read(f)) {
			if m := entryHeading.FindStringSubmatch(ln.Text); m != nil {
				current = m[1]
				if prev, ok := headingIn[current]; ok {
					r.bad("%s", sprintf("%s: entry %q also has a heading in %s", f, current, prev))
					continue
				}
				headingIn[current] = f
				continue
			}
			if current == "" {
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(ln.Text), "## ") {
				current = ""
				continue
			}
			if m := statusField.FindStringSubmatch(ln.Text); m != nil {
				if _, seen := statusIn[current]; !seen {
					statusIn[current] = strings.ToLower(m[1])
				}
			}
		}
	}

	counts := map[string]int{}
	perPri := map[string]map[string]int{}
	for _, row := range rows {
		counts[row.status]++
		if perPri[row.pri] == nil {
			perPri[row.pri] = map[string]int{}
		}
		perPri[row.pri][row.status]++
		perPri[row.pri]["total"]++

		where, ok := headingIn[row.id]
		if !ok {
			r.bad("%s", sprintf("%s:%d: row %q has no entry heading `## %s.` in any file under %s",
				index, row.line, row.id, row.id, RecordDir))
			continue
		}
		if where != row.file && where != RecordDir+row.file {
			r.bad("%s", sprintf("%s:%d: row %q names %s and the entry is in %s", index, row.line, row.id, row.file, where))
		}
		if st, ok := statusIn[row.id]; ok && st != strings.ToLower(row.status) {
			r.bad("%s", sprintf("%s: the index says %q and %s says %q", row.id, row.status, where, st))
		}
	}

	known := map[string]bool{}
	for _, row := range rows {
		known[row.id] = true
	}
	var orphans []string
	for id, f := range headingIn {
		if !known[id] {
			orphans = append(orphans, sprintf("%s: entry %q has no row in %s", f, id, index))
		}
	}
	sort.Strings(orphans)
	for _, o := range orphans {
		r.bad("%s", o)
	}

	total := len(rows)
	r.Extra["entries"] = total
	r.Extra["open"] = counts["open"]
	r.Extra["blocked"] = counts["blocked"]
	r.Extra["done"] = counts["done"]

	// The index's own count line.
	checkCounts(&r, t, index, index, total, counts)
	// ⛔ AND THE RECORD'S. Reading two of the three files is the shape whose
	// absence work-todo.md blames for the incident above: PROGRESS.md sat
	// declaring one set of numbers beside an index declaring another, and the
	// first version of this check reported clean.
	progress := RecordDir + "PROGRESS.md"
	if len(t.Read(progress)) > 0 {
		checkCounts(&r, t, progress, index, total, counts)
	}

	// ⛔ AND THE WORK ORDER, which nothing checked for as long as this check has
	// existed. It is rule 7 and it is the newest, because the defect it catches
	// has now misrouted two sessions.
	status := make(map[string]string, len(rows))
	for _, row := range rows {
		status[row.id] = strings.ToLower(row.status)
	}
	checkWorkOrder(&r, t, status)

	// The per-priority table.
	//
	// ⛔ INCLUDING ITS `all` ROW, which was not checked for as long as this
	// check existed. Measured on 2026-09-10: the row read `9 0 54 63` while the
	// four rows above it summed to `16 0 62 78` and the state line agreed with
	// the rows, and `check-record` reported ok. The index's own header says
	// "the counts below are checked, not typed", and one row of them was typed.
	// It is the exact defect class this check was built for, in this check.
	// `TOOL-18`.
	perPri["all"] = map[string]int{
		"open": counts["open"], "blocked": counts["blocked"],
		"done": counts["done"], "total": total,
	}
	for _, p := range []string{"P0", "P1", "P2", "P3", "all"} {
		want := perPri[p]
		if want == nil {
			want = map[string]int{}
		}
		got, ok := priorityRow(t, index, p)
		if !ok {
			r.bad("%s", sprintf("%s: the priority table has no row for %s", index, p))
			continue
		}
		for _, k := range []string{"open", "blocked", "done", "total"} {
			if got[k] != want[k] {
				r.bad("%s", sprintf("%s: %s declares %s %d, the rows say %d", index, p, k, got[k], want[k]))
			}
		}
	}
	return r
}

// checkWorkOrder asserts that PROGRESS.md's work order and the index agree
// about what is finished.
//
// ⛔ THE WORK ORDER OUTLIVED THE WORK BY A SESSION, TWICE, AND NO CHECK FIRED.
// On 2026-09-16 an item still read "`WSL-67` has left: driving `pkgin` and
// `pkg_add`" after the entry had closed on a driven NetBSD guest, and a session
// resumed on that sentence went to re-ask for an approval already given. The
// index and the entry both read `done` throughout, so the record disagreed with
// itself and only the work order was wrong. On 2026-09-17 a prompt sent a
// session to `WSL-68`'s "four remaining items, which need nobody" after three
// had closed and the operator had deferred the fourth. `TODO/PROGRESS.md`'s own
// finding 29 names this check as the fix.
//
// The rule, in two halves, because each catches the other's blind spot:
//
//   - an item marked Closed must name only entries the index calls done. A
//     victory lap over open work.
//   - an item NOT marked Closed that names entries must name at least one the
//     index does not call done. That is the half that fires on the defect
//     above, and an item naming no entry at all is exempt because an item may
//     be about a release or a decision rather than an entry.
//
// ⛔ THE SECTION MUST EXIST. A check that quietly stops checking when a heading
// is reworded is the shape this whole file is about.
func checkWorkOrder(r *Result, t *Tree, status map[string]string) {
	progress := RecordDir + "PROGRESS.md"
	body := t.Read(progress)
	if len(body) == 0 {
		return
	}
	lines := Lines(body)
	start := -1
	for _, ln := range lines {
		if workOrderHeading.MatchString(ln.Text) {
			start = ln.N
			break
		}
	}
	if start < 0 {
		r.bad("%s", sprintf("%s carries no `## The work order` section, so nothing says which entries the order believes are finished", progress))
		return
	}

	// One item is its numbered line plus everything indented under it, and it
	// ends at the next numbered line or the next heading.
	type item struct {
		num    string
		line   int
		closed bool
		ids    []string
	}
	var items []item
	var cur *item
	for _, ln := range lines {
		if ln.N <= start {
			continue
		}
		if strings.HasPrefix(ln.Text, "## ") {
			break
		}
		if m := orderedItem.FindStringSubmatch(ln.Text); m != nil {
			items = append(items, item{num: m[1], line: ln.N})
			cur = &items[len(items)-1]
		}
		if cur == nil {
			continue
		}
		if workOrderClosed.MatchString(ln.Text) {
			cur.closed = true
		}
		for _, id := range quotedEntryID.FindAllStringSubmatch(ln.Text, -1) {
			cur.ids = append(cur.ids, id[1])
		}
	}
	if len(items) == 0 {
		r.bad("%s", sprintf("%s:%d: the work order section has no numbered items", progress, start))
		return
	}

	for _, it := range items {
		// ⚠ EACH ID ONCE. An item may name one entry twice - the real item 3
		// does, once as its subject and once in the sentence about what it uses -
		// and a finding that listed it twice read as two defects. Found by planting
		// the defect in the real record rather than in a fixture.
		var known []string
		seen := map[string]bool{}
		for _, id := range it.ids {
			if _, ok := status[id]; !ok || seen[id] {
				continue
			}
			seen[id] = true
			known = append(known, id)
		}
		if len(known) == 0 {
			continue
		}
		if it.closed {
			for _, id := range known {
				if status[id] != "done" {
					r.bad("%s", sprintf("%s:%d: work order item %s says Closed and names %s, which %sINDEX.md calls %q",
						progress, it.line, it.num, id, RecordDir, status[id]))
				}
			}
			continue
		}
		unfinished := false
		for _, id := range known {
			if status[id] != "done" {
				unfinished = true
				break
			}
		}
		if !unfinished {
			r.bad("%s", sprintf("%s:%d: work order item %s is not marked Closed and every entry it names (%s) is done in %sINDEX.md, so the order is behind the work",
				progress, it.line, it.num, strings.Join(known, ", "), RecordDir))
		}
	}
}

// checkCounts compares one file's declared state line against the rows.
func checkCounts(r *Result, t *Tree, file, index string, total int, counts map[string]int) {
	for _, ln := range Lines(t.Read(file)) {
		m := stateLine.FindStringSubmatch(ln.Text)
		if m == nil {
			continue
		}
		want := map[string]int{"total": total, "open": counts["open"], "blocked": counts["blocked"], "done": counts["done"]}
		for i, k := range []string{"total", "open", "blocked", "done"} {
			n, err := strconv.Atoi(m[i+1])
			if err != nil || n != want[k] {
				r.bad("%s", sprintf("%s:%d: declares %s %s, the rows in %s say %d", file, ln.N, k, m[i+1], index, want[k]))
			}
		}
		return
	}
	r.bad("%s", sprintf("%s carries no `total N open N blocked N done N` line, so nothing in it can be checked against the rows", file))
}

// priorityRow reads one row of the priority table.
func priorityRow(t *Tree, index, pri string) (map[string]int, bool) {
	for _, ln := range Lines(t.Read(index)) {
		cells := tableCells(ln.Text)
		if len(cells) < 5 {
			continue
		}
		if strings.Trim(cells[0], "* ") != pri {
			continue
		}
		out := map[string]int{}
		for i, k := range []string{"open", "blocked", "done", "total"} {
			n, err := strconv.Atoi(strings.Trim(cells[i+1], "* "))
			if err != nil {
				return nil, false
			}
			out[k] = n
		}
		return out, true
	}
	return nil, false
}

// tableCells splits a markdown table row into trimmed cells.
func tableCells(line string) []string {
	s := strings.TrimSpace(line)
	if !strings.HasPrefix(s, "|") {
		return nil
	}
	parts := strings.Split(strings.Trim(s, "|"), "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

var (
	entryIDOnly    = regexp.MustCompile(`^[A-Z]{2,6}-[0-9]{2,}$`)
	entryHeading   = regexp.MustCompile(`^##\s+([A-Z]{2,6}-[0-9]{2,})\.`)
	statusField    = regexp.MustCompile(`\*\*Status\*\*[^A-Za-z]*([A-Za-z]+)`)
	linkTargetOnly = regexp.MustCompile(`\(([^)]+)\)`)
	stateLine      = regexp.MustCompile(`total\s+([0-9]+)\s+open\s+([0-9]+)\s+blocked\s+([0-9]+)\s+done\s+([0-9]+)`)

	// ⚠ THE HEADING IS MATCHED ON ITS STEM, not its whole text: it carries the
	// date the operator set the order, and that date moves.
	workOrderHeading = regexp.MustCompile(`^##\s+The work order\b`)
	orderedItem      = regexp.MustCompile(`^([0-9]+)\.\s`)
	workOrderClosed  = regexp.MustCompile(`\*\*Closed:\*\*`)
	// ⛔ BACKTICKED ONLY. An id in prose without them is a mention; the order
	// names the entries it is about in code spans, and reading bare text would
	// pull ids out of the corrections written underneath an item.
	quotedEntryID = regexp.MustCompile("`([A-Z]{2,6}-[0-9]{2,})`")
)
