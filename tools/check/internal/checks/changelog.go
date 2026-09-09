// SPDX-License-Identifier: 0BSD

package checks

import (
	"regexp"
	"strings"
)

// ChangelogFile is the log this rule reads.
const ChangelogFile = "CHANGELOG.md"

// Changelog holds the four rules CHANGELOG.md states about itself.
//
//  1. newest first, so the top of the file is the latest thing that shipped;
//  2. every heading carries a date, because nothing can order it otherwise;
//  3. every entry names its record, because an entry with no record is a claim;
//  4. every entry says whether it deployed. "No deploy" is a complete and
//     common answer; silence is not one.
//
// ⚠ A REPOSITORY WITH NO CHANGELOG COULD NOT RUN THIS, which is exit 2 rather
// than a pass: it has neither broken these rules nor satisfied them. This
// returns that through Missing so the caller can tell the two apart.
func Changelog(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	body := t.Read(ChangelogFile)
	if len(body) == 0 {
		r.Extra["missing"] = true
		r.Extra["entries"] = 0
		return r
	}

	entries := 0
	prev := ""
	// The entry currently open, and what it has said so far.
	open, openLine := false, 0
	hasRecord, hasDeploy := false, false

	flush := func() {
		if !open {
			return
		}
		if !hasRecord {
			r.bad("%s: the entry at line %d names no record. An entry with no record is a claim", ChangelogFile, openLine)
		}
		if !hasDeploy {
			r.bad("%s: the entry at line %d does not say whether it deployed. Silence is not an answer", ChangelogFile, openLine)
		}
		open = false
	}

	for _, ln := range Lines(body) {
		switch {
		case strings.HasPrefix(ln.Text, "## "):
			flush()
			prev = ""
		case strings.HasPrefix(ln.Text, "### "):
			flush()
			entries++
			openLine = ln.N
			d := changelogDate.FindString(ln.Text)
			if d == "" {
				r.bad("%s:%d: no date in the heading. Nothing can order it", ChangelogFile, ln.N)
			} else {
				// ⚠ String comparison, which is why the date is ISO 8601: the
				// ordering a reader wants and the ordering the bytes give are
				// the same one only in that format.
				if prev != "" && d > prev {
					r.bad("%s:%d: out of order: %s comes after %s. Newest first", ChangelogFile, ln.N, d, prev)
				}
				prev = d
			}
			open, hasRecord, hasDeploy = true, false, false
		case open:
			low := strings.ToLower(ln.Text)
			if strings.Contains(low, "record:") {
				hasRecord = true
			}
			if strings.Contains(low, "deploy") {
				hasDeploy = true
			}
		}
	}
	flush()
	r.Extra["entries"] = entries
	return r
}

// changelogDate is an ISO 8601 date, with an optional time.
var changelogDate = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}(T[0-9]{2}:[0-9]{2}:[0-9]{2}Z)?`)
