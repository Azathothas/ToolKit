// SPDX-License-Identifier: 0BSD

package checks

import (
	"os/exec"
	"strings"
)

// LineEndings compares what git holds against what .gitattributes resolves,
// using git's own answer rather than a second table. The index column is the
// half that decides what a commit contains.
//
// The defect it exists to catch is invisible to review. A carriage return in a
// file .gitattributes says is LF shows nothing in `git diff`, because the index
// is normalised either way, and it is very visible to everything else: a
// regex, a shell reading a script, a compiler reading an embedded source.
//
// The drift arrives from your own tooling. Python's write_text translates \n
// to \r\n on Windows unless it is told not to, and a single pass over this
// tree using it rewrote 182 files and reached four commits before anything
// noticed. Nothing was watching, because this check did not exist.
func LineEndings(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	// Scoped to the files this project writes. `--eol` stats every path it is
	// given, and asking it about 28,000 vendored files costs eight seconds to
	// produce rows this check then discards. A gate people stop running
	// because it is slow is a gate that never runs.
	out, err := exec.Command("git", "-C", t.Root, "ls-files", "--eol", "--",
		".", ":!references", ":!docs/history/oracle", ":!evidence").Output()
	if err != nil {
		r.bad("cannot read git ls-files --eol: %v", err)
		return r
	}
	checked := 0
	for _, ln := range strings.Split(string(out), "\n") {
		// The filename is everything after the tab. Splitting the whole line
		// on whitespace loses every path containing a space, and then reports
		// the last word of it as a file: four vendored names did exactly that.
		head, file, ok := strings.Cut(ln, "\t")
		if !ok {
			continue
		}
		if Vendored(file) || strings.HasPrefix(file, "evidence/") {
			continue
		}
		fields := strings.Fields(head)
		if len(fields) < 2 {
			continue
		}
		index, attr := fields[0], ""
		for _, f := range fields {
			if strings.HasPrefix(f, "attr/") {
				attr = strings.TrimPrefix(f, "attr/")
			}
		}
		if strings.Contains(attr, "-text") {
			continue // declared binary; line endings are not this check's business
		}
		if index == "i/none" || index == "i/-text" {
			continue // binary, or a file with no line endings at all
		}
		checked++
		// The rule this repository states is LF everywhere except PowerShell,
		// which keeps CRLF because 5.1 mis-parses a here-string terminated by
		// a bare LF.
		want := "i/lf"
		if strings.Contains(attr, "eol=crlf") {
			want = "i/crlf"
		}
		if index != want {
			r.bad("%s: git holds %s in the index where .gitattributes resolves %s; a carriage return here is invisible to git diff and visible to everything else",
				file, index, want)
		}
	}
	r.Extra["files"] = checked
	return r
}
