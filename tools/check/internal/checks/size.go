// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The destination's per-file limits. ⛔ The hard one is a REFUSAL, not a
// warning: a push carrying a file at or over it fails, and it fails after the
// commit exists, which is the worst moment to find out.
const (
	HardFileLimit = 100 << 20 // 100 MiB, refused outright
	WarnFileLimit = 50 << 20  // 50 MiB, warned about
)

// largeFilesFile accounts for every tracked file over the warn threshold, with
// the size it may not exceed and the reason it is here at all.
const largeFilesFile = "references/LARGE-FILES.txt"

// Size refuses a tracked file the destination will not take.
//
// ⛔ The hazard is not size, it is GROWTH. Four of these are mined build logs
// that grow with every upstream run, the largest is 1.9 MiB short of the hard
// limit, and the fetcher takes whatever is there. So a re-mine of that tree is
// a push that stops working, and before this nothing said so: the first push
// WARNED on three of them, and the limit above them refuses.
//
// ⛔ Three rules, and the middle one is the early warning the other two are
// not. A file at the hard limit is already a broken push; a file over the warn
// threshold that nothing accounts for is a new one nobody decided on; and a
// listed file bigger than the size recorded beside it has GROWN, which is the
// case that fires before the push does.
//
// ⚠ Clearing the entry that removes these trees clears this too, and then every
// row here is stale and rule 3 says so. That is the intended end state rather
// than a permanent fixture.
func Size(t *Tree) Result {
	r := Result{Extra: map[string]any{}}

	type row struct {
		max    int64
		reason string
	}
	listed := map[string]row{}
	body := t.Read(largeFilesFile)
	// ⚠ THE ACCOUNTING FILE IS REQUIRED ONLY ONCE SOMETHING NEEDS ACCOUNTING
	// FOR. Nothing in this tree is within an order of magnitude of the warn
	// threshold, and demanding a listing of an empty set is a check that fails
	// on a correct tree. The rule below asks for it by name the moment a file
	// crosses, which is when the answer is worth writing down.
	for _, ln := range Lines(body) {
		if strings.HasPrefix(ln.Text, "#") || strings.TrimSpace(ln.Text) == "" {
			continue
		}
		f := strings.SplitN(ln.Text, "\t", 3)
		if len(f) < 3 {
			r.bad("%s:%d: want a path, a maximum in bytes and a reason, tab separated", largeFilesFile, ln.N)
			continue
		}
		max, err := strconv.ParseInt(strings.TrimSpace(f[1]), 10, 64)
		if err != nil {
			r.bad("%s:%d: %q is not a size in bytes", largeFilesFile, ln.N, f[1])
			continue
		}
		listed[f[0]] = row{max: max, reason: f[2]}
	}

	// ⚠ The size read is the WORKING TREE's, not the blob's, and the two can
	// differ where .gitattributes normalises line endings. For the files this
	// is about - mined build logs already in LF - the difference is zero, and
	// stating the boundary is cheaper than a second read that would have to
	// ask git for every object.
	var overWarn, checked int
	var largest int64
	largestPath := ""
	for _, p := range t.Files {
		fi, err := os.Stat(filepath.Join(t.Root, filepath.FromSlash(p)))
		if err != nil || fi.IsDir() {
			continue
		}
		checked++
		n := fi.Size()
		if n > largest {
			largest, largestPath = n, p
		}
		if n >= HardFileLimit {
			r.bad("%s is %s and the destination refuses a file at %s; this is a push that FAILS rather than warns",
				p, mib(n), mib(HardFileLimit))
		}
		l, ok := listed[p]
		if ok {
			// ⛔ The ceiling binds whatever the thresholds say. A row may sit
			// UNDER the warn threshold deliberately - a file a couple of MiB
			// below it is exactly the one worth watching - and a rule that only
			// looked at files already over the threshold would notice that one
			// after it crossed rather than when it grew.
			if n > l.max {
				r.bad("%s has GROWN to %s, past the %s recorded for it; it is %s from the hard limit, and a re-mine is what grows it",
					p, mib(n), mib(l.max), mib(HardFileLimit-n))
			}
		}
		if n < WarnFileLimit {
			continue
		}
		overWarn++
		if !ok {
			r.bad("%s is %s, over the %s the destination warns at. Record it in %s with the size it may not exceed and the reason it is here",
				p, mib(n), mib(WarnFileLimit), largeFilesFile)
		}
	}
	tracked := map[string]bool{}
	for _, p := range t.Files {
		tracked[p] = true
	}
	var stale []string
	for p := range listed {
		if !tracked[p] {
			stale = append(stale, p)
		}
	}
	sort.Strings(stale)
	for _, p := range stale {
		r.bad("%s lists %s and no such file is tracked; re-derive the file", largeFilesFile, p)
	}

	r.Extra["files"] = checked
	r.Extra["over_warn"] = overWarn
	r.Extra["largest"] = largestPath
	r.Extra["largest_bytes"] = largest
	r.Extra["headroom_bytes"] = int64(HardFileLimit) - largest
	return r
}

// mib renders a byte count the way the limits are stated.
func mib(n int64) string {
	return sprintf("%.1f MiB", float64(n)/(1<<20))
}
