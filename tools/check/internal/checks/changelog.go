// SPDX-License-Identifier: 0BSD

package checks

import (
	"regexp"
	"strings"
	"time"
)

// ChangelogFile is the log this rule reads.
const ChangelogFile = "CHANGELOG.md"

// Changelog holds the four rules CHANGELOG.md states about itself.
//
//  1. newest first, so the top of the file is the latest thing that shipped;
//  2. every heading carries a date, because nothing can order it otherwise;
//  3. every entry names its record, because an entry with no record is a claim;
//  4. every entry says whether it deployed. "No deploy" is a complete and
//     common answer; silence is not one;
//  5. no heading is dated later than the clock.
//
// ⛔ RULE 5 IS THERE BECAUSE A STAMP WAS TYPED, NOT READ. On 2026-09-17 five
// stamps were written from memory and two were in the future, and every rule
// above passed them: a future stamp is in order and well formed. TOOL-36.
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
				if future(d, changelogNow()) {
					r.bad("%s:%d: %s is later than the clock. A stamp is read with `date -u`, never typed", ChangelogFile, ln.N, d)
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

// changelogNow is the clock rule 5 reads, a variable so a case can set it.
var changelogNow = time.Now

// changelogSkew is how far ahead of this machine's clock a stamp may be, for a
// stamp read on another machine whose clock runs a little fast.
const changelogSkew = 10 * time.Minute

// future reports whether stamp is later than now allows. A date alone is late
// only when it is after the date that now plus the skew falls on.
func future(stamp string, now time.Time) bool {
	limit := now.UTC().Add(changelogSkew)
	if t, err := time.Parse("2006-01-02T15:04:05Z", stamp); err == nil {
		return t.After(limit)
	}
	if t, err := time.Parse("2006-01-02", stamp); err == nil {
		return t.After(limit.Truncate(24 * time.Hour))
	}
	return false
}

// changelogDate is an ISO 8601 date, with an optional time.
var changelogDate = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}(T[0-9]{2}:[0-9]{2}:[0-9]{2}Z)?`)
