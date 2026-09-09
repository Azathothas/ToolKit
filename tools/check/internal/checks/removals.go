// SPDX-License-Identifier: 0BSD

package checks

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

// Removals refuses a tracked shell script that can delete the checkout.
//
// ⛔ THIS CHECK HAS A DATE AND A COST. A script running in the development guest
// ended with `rm -rf "$PGT_REPO"`; the variable still pointed at the Windows
// drive mount this repository is developed on; `rm` on a drive mount does not go
// through the recycle bin, and 29,339 files went in one call. What had been
// pushed came back from `origin` and what had not did not.
//
// The prevention in this repository is structural rather than a sourced shell
// library: tools/windows/wsl-toolkit copies a workspace into a container and
// mounts no host directory at all, and its one deletion resolves links on both
// sides before it removes anything. TODO/RULES.md section 3 owns that.
//
// This check covers what that cannot: a shell script in this tree, run by hand.
//
// WHAT IT REFUSES, and both are narrow on purpose:
//
//  1. A removal naming one of the variables that hold the checkout. There are
//     four spellings and all four are here. `rm -rf "$REPO/build"` is refused
//     too, because the guard's own rule 3 refuses any ANCESTOR of the checkout
//     and a script that composes a path under it is one edit away from naming
//     the root of it.
//  2. A recursive removal of anything under /mnt, which is where a host mount
//     is. That one has no legitimate use in this tree at all.
//
// ⚠ A BLANKET RULE WAS MEASURED AND REJECTED. `rm -r` with any variable in it
// occurs 433 times across 217 tracked scripts, nearly all of them a workdir the
// script itself made under /tmp. Refusing all of those would be a check nobody
// could make green, and a check nobody can make green gets deleted rather than
// satisfied. These two patterns had ZERO occurrences when this was written,
// which is what makes them enforceable rather than aspirational.
//
// ⭐ It also asserts the guard is STILL THERE and still exports what callers
// use. A rule that can be satisfied by deleting the thing it protects is not a
// rule, and `scripts/common/plant-guard.sh` plants exactly that.
//
// ⛔ ITS LIMIT, STATED. This is a LINE-LEVEL LEXICAL rule: it sees an `rm` and a
// checkout variable on one line. An author who assigns the path to another
// variable on one line and removes it on the next defeats it, and
// scripts/common/plant-removals.sh does exactly that to write its own plants
// without failing this check. That is not a hole to close by making the pattern
// cleverer - a shell script's meaning is not recoverable by regular expression -
// it is why prevention lives in `pgt_rm`, which refuses at RUN time on the
// resolved path and cannot be talked out of it by how the call was spelled.
// This check exists to stop the OBVIOUS spelling reaching a commit, and that is
// the spelling that cost this repository a working tree.
func Removals(t *Tree) Result {
	r := Result{Extra: map[string]any{}}

	var srcs []string
	for _, f := range t.Ours() {
		if path.Ext(f) == ".sh" {
			srcs = append(srcs, f)
		}
	}
	sort.Strings(srcs)

	sites := 0
	for _, f := range srcs {
		for _, ln := range Lines(t.Read(f)) {
			text := strings.TrimSpace(ln.Text)
			// ⚠ A whole-line comment only. guard.sh's own header quotes the
			// call that caused this, and a check that could not tell a quotation
			// from a call would make the file documenting the defect the file
			// that fails. Anything on a line WITH code is read as code, so a
			// trailing comment carrying the pattern is still a finding - which
			// is the safe direction to be wrong in.
			if text == "" || strings.HasPrefix(text, "#") {
				continue
			}
			if m := rmCheckoutRe.FindString(text); m != "" {
				sites++
				r.bad("%s:%d: removes a path built from the checkout: %s\n"+
					"    use pgt_rm from scripts/common/lib/guard.sh, which refuses it",
					f, ln.N, strings.TrimSpace(m))
				continue
			}
			if m := rmHostMountRe.FindString(text); m != "" {
				sites++
				r.bad("%s:%d: recursively removes something under /mnt: %s\n"+
					"    that is a host mount, and a delete there is not recoverable from this side",
					f, ln.N, strings.TrimSpace(m))
			}
		}
	}
	r.Extra["scripts"] = len(srcs)
	r.Extra["sites"] = sites

	return r
}

// The four spellings of "the checkout". ⚠ All four, because a script that used
// a different one would be refused by the guard at run time and accepted by
// this check at commit time, and the two disagreeing is worse than either.
var rmCheckoutRe = regexp.MustCompile(
	`\brm\b[^|;&]*\$\{?(PGT_REPO|REPO|PGT_MIRROR|PGT_GUARD_REPO)\b`)

// A recursive rm whose target is under /mnt. ⚠ The flag may be spelled -rf,
// -fr, -Rf, -r --force or --recursive, so the class is "any option cluster
// containing r or R, or the long form", not the two spellings somebody happened
// to think of. ⛔ The long form is a separate alternative because a single-dash
// cluster cannot match a second dash, so `--recursive` slipped through the
// first version of this pattern.
var rmHostMountRe = regexp.MustCompile(
	`\brm\b\s+((-[a-zA-Z]*[rR][a-zA-Z]*|--recursive|--force)\s+)*` +
		`(-[a-zA-Z]*[rR][a-zA-Z]*|--recursive)\s+[^|;&]*/mnt/`)
