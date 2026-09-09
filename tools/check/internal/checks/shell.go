// SPDX-License-Identifier: 0BSD

package checks

import (
	"os/exec"
	"path"
	"sort"
)

// Shell refuses a tracked shell script that does not parse.
//
// The gate had no such check, and the hole is a specific one rather than a
// theoretical gap: this tree's largest single body of work is editing shell
// comments, and a comment in a shell script is not always inert. A `#` line
// inside a single-quoted awk or sed program is part of that string, so an
// apostrophe written into one ends the quote, and everything after it is
// reparsed as shell. The failure is silent until the script runs: nothing
// compiles a shell script, `git diff` shows an ordinary comment edit, and the
// only symptom is a syntax error on a line the edit never touched.
//
// It is the same job check c-runtime does for the carried C, for the same
// reason. The experiments and the proof-of-concept projects are the
// independent acceptance apparatus and stay shell, so nothing else in the
// toolchain will ever parse them.
//
// The oracle tree and references/ are excluded with everything else vendored:
// Tree.Ours() already drops them, and an oracle kept for byte-identical
// comparison must not be edited to satisfy a check.
func Shell(t *Tree) Result {
	r := Result{Extra: map[string]any{}}

	sh := posixShell()
	r.Extra["shell"] = sh
	if sh == "" {
		// No shell to ask. Report the absence rather than the silence: a
		// skip is neither a pass nor a failure, and "scripts: 0" beside an
		// empty shell name is what tells a reader which of the two this is.
		r.Extra["scripts"] = 0
		return r
	}

	var srcs []string
	for _, f := range t.Ours() {
		if path.Ext(f) == ".sh" {
			srcs = append(srcs, f)
		}
	}
	sort.Strings(srcs)
	for _, f := range srcs {
		// The slash path, not the native one: dash echoes the argument it was
		// given, and a finding that spells the path differently from every other
		// check is one a reader has to translate.
		cmd := exec.Command(sh, "-n", f)
		cmd.Dir = t.Root
		if out, err := cmd.CombinedOutput(); err != nil {
			r.bad("%s does not parse: %s", f, firstError(string(out)))
		}
	}
	r.Extra["scripts"] = len(srcs)
	return r
}

// all on a development machine that is not Linux.
func posixShell() string {
	for _, name := range []string{"dash", "sh", "bash"} {
		p, err := exec.LookPath(name)
		if err != nil || windowsOnly(p) {
			continue
		}
		return p
	}
	return ""
}
