// record_test.go - the work order and the index have to agree about what is
// finished.
//
// ⛔ THE CASES BELOW ARE THE TWO REAL INCIDENTS, not invented shapes.
// `TODO/PROGRESS.md`'s finding 29 is the first: an item read "`WSL-67` has
// left: driving `pkgin` and `pkg_add`" for a whole session after the entry had
// closed, and a resuming session was sent to re-ask for an approval already
// given. Finding 29 names this check as the fix and says it is not written.
// The second is 2026-09-17, when a prompt sent a session to `WSL-68`'s "four
// remaining items, which need nobody" after three had closed and the operator
// had deferred the fourth.
//
// ⚠ IN BOTH, `INDEX.md` AND THE ENTRY READ `done` THROUGHOUT. Only the work
// order was wrong, which is why none of the six rules before this one fired.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// treeWithProgress writes one PROGRESS.md and returns a Tree over it.
func treeWithProgress(t *testing.T, body string) *Tree {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "TODO")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("could not make %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PROGRESS.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("could not write PROGRESS.md: %v", err)
	}
	return &Tree{Root: root, Files: []string{"TODO/PROGRESS.md"}, cache: map[string][]byte{}}
}

func TestTheWorkOrderMustAgreeWithTheIndexAboutWhatIsFinished(t *testing.T) {
	const header = "# PROGRESS.md\n\n## The work order, set by the operator on 2026-09-14\n\n"
	const after = "\n## Rulings in force\n\n1. something else entirely, `WSL-99`.\n"

	cases := []struct {
		name string
		// order is the body of the work order section. whole, when set,
		// replaces the composed file instead: it is for the one case whose
		// subject is the section NOT being there.
		order  string
		whole  string
		status map[string]string
		want   int
		// wantDetail is asserted where two different guards would both report
		// one problem, so a count alone could not tell them apart.
		wantDetail string
	}{
		{
			// The shape of the record as it stands: a Closed item over done
			// entries, and a live item over an open one.
			name: "an order that agrees",
			order: "1. ⭐ **Closed:** `WSL-74` and `WSL-75`.\n" +
				"2. **`WSL-68`, the sealed base.** It uses the verifier `WSL-84` fixed.\n",
			status: map[string]string{"WSL-74": "done", "WSL-75": "done", "WSL-68": "open", "WSL-84": "done"},
			want:   0,
		},
		{
			// ⛔ FINDING 29, EXACTLY. The item is not marked Closed, it names
			// one entry, and that entry is done. Nothing else in the record
			// disagrees with anything.
			name:   "an item still describing work the index says is finished",
			order:  "1. **`WSL-67`** has left: driving `pkgin` and `pkg_add`, which waits for two downloads.\n",
			status: map[string]string{"WSL-67": "done"},
			want:   1,
		},
		{
			// The mirror image: a victory lap over work still open.
			name:   "a Closed item naming an entry that is open",
			order:  "1. ⭐ **Closed:** `WSL-74`, `WSL-76` and `WSL-75`.\n",
			status: map[string]string{"WSL-74": "done", "WSL-75": "done", "WSL-76": "open"},
			want:   1,
		},
		{
			// ⚠ AN ITEM MAY BE ABOUT SOMETHING THAT IS NOT AN ENTRY. Item 5 of
			// the real order is the release, and it names none. Refusing it
			// would make the rule one people work around.
			name:   "an item that names no entry at all",
			order:  "1. **`wsl-toolkit-v3.0.0`** is cut after the issues are closed.\n",
			status: map[string]string{"WSL-74": "done"},
			want:   0,
		},
		{
			// ⚠ AN ID THE INDEX HAS NEVER HEARD OF IS NOT THIS RULE'S BUSINESS.
			// Rule 1 owns that, and reporting it twice would make one defect
			// read as two.
			name:   "an item naming an id with no row",
			order:  "1. **`WSL-91`, which does not exist yet.**\n",
			status: map[string]string{"WSL-74": "done"},
			want:   0,
		},
		{
			// ⛔ A CHECK THAT QUIETLY STOPS CHECKING is the shape this file is
			// about. A reworded heading must be loud.
			// ⚠ THE COUNT ALONE CANNOT PROVE THIS ONE. With the heading guard
			// off, the "no numbered items" guard below reports one problem too,
			// so the case reads the message rather than the number.
			name:       "no work order section at all",
			whole:      "# PROGRESS.md\n\n## State\n\nnothing here.\n",
			status:     map[string]string{"WSL-74": "done"},
			want:       1,
			wantDetail: "carries no `## The work order` section",
		},
		{
			name:   "a work order section with no numbered items",
			order:  "It is all prose now, and it mentions `WSL-74`.\n",
			status: map[string]string{"WSL-74": "done"},
			want:   1,
		},
		{
			// ⚠ AN ITEM MAY NAME ONE ENTRY TWICE, once as its subject and once in
			// the sentence about what it uses, and the real item 3 does. Listing it
			// twice made one defect read as two. Found by planting the defect in the
			// real record, which is the pass a fixture could not have made.
			name:       "an item that names the same done entry twice",
			order:      "1. **`WSL-84`**, and it uses what `WSL-84` fixed.\n",
			status:     map[string]string{"WSL-84": "done"},
			want:       1,
			wantDetail: "(WSL-84)",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := header + c.order + after
			if c.whole != "" {
				body = c.whole
			}
			tree := treeWithProgress(t, body)
			r := Result{Extra: map[string]any{}}
			checkWorkOrder(&r, tree, c.status)
			if r.Problems != c.want {
				t.Fatalf("problems = %d, want %d: %s", r.Problems, c.want, strings.Join(r.Detail, " | "))
			}
			if c.wantDetail != "" && !strings.Contains(strings.Join(r.Detail, " | "), c.wantDetail) {
				t.Fatalf("no finding said %q: %s", c.wantDetail, strings.Join(r.Detail, " | "))
			}
		})
	}
}

// TestTheWorkOrderRuleReadsOnlyBacktickedIds is its own case because the
// distinction is what keeps the rule usable.
//
// ⛔ THE CORRECTION WRITTEN UNDER AN ITEM NAMES THE ENTRY IT CORRECTS. The real
// item 2 carries "⚠ This item said until 2026-09-16 that `WSL-67` still had
// `pkgin` to drive", and that mention is the record doing its job. Backticks
// are what separate the entries an item is ABOUT from the ones it discusses,
// and every id in the real order is in a code span.
func TestTheWorkOrderRuleReadsOnlyBacktickedIds(t *testing.T) {
	const head = "# PROGRESS.md\n\n## The work order, set by the operator on 2026-09-14\n\n"
	status := map[string]string{"WSL-74": "done", "WSL-76": "open"}

	// ⛔ THE DISCRIMINATING PAIR. Both bodies say the same thing; only the
	// backticks differ, and WSL-76 is open. A reader that took bare text would
	// call the Closed item a victory lap over open work.
	bare := head + "1. ⭐ **Closed:** `WSL-74`. This corrects what WSL-76 said until 2026-09-16.\n"
	r := Result{Extra: map[string]any{}}
	checkWorkOrder(&r, treeWithProgress(t, bare), status)
	if r.Problems != 0 {
		t.Fatalf("a bare mention was read as one of the item's entries: %s", strings.Join(r.Detail, " | "))
	}

	spanned := head + "1. ⭐ **Closed:** `WSL-74`. This corrects what `WSL-76` said until 2026-09-16.\n"
	r = Result{Extra: map[string]any{}}
	checkWorkOrder(&r, treeWithProgress(t, spanned), status)
	if r.Problems != 1 {
		t.Fatalf("a backticked open id inside a Closed item was not reported: %s", strings.Join(r.Detail, " | "))
	}
}
