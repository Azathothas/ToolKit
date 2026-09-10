// SPDX-License-Identifier: 0BSD

package checks

import (
	"os/exec"
	"strings"
)

// LineEndings compares what is on disk against what .gitattributes resolves,
// using git's own answer rather than a second table.
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
//
// ⛔ AND FOR ITS FIRST LIFE IT STILL WAS NOT WATCHING, WHICH IS WORSE. Two
// defects, and the first hid the second. `git ls-files --eol` writes the
// attribute column as `attr/text eol=crlf`, with a SPACE in it, so splitting
// the row on whitespace put `eol=crlf` in a field of its own and the parse kept
// only `text`. Every file therefore resolved as "expected LF". Then the
// comparison read the INDEX column, and git normalises a `text` file to LF in
// the index BY DEFINITION, so "expected LF" was a tautology: the check could
// not fail on any text file in the tree, and 23 of 54 tracked .ps1 files were
// sitting in the working tree with LF under an `eol=crlf` attribute while it
// reported green. Measured on 2026-09-10 by planting five CRLF into a file
// declared `eol=lf` and reading the exit code, which was 0. TOOL-20.
//
// ⭐ THE WORKING TREE IS THE COLUMN THAT CAN DISAGREE, and it is the one every
// other tool reads. The index is asserted too, because a file committed with
// `-text` or with normalisation off is a different defect and the two are worth
// telling apart.
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
		index, worktree, attr, ok := parseEOLRow(head)
		if !ok {
			continue
		}
		if strings.Contains(attr, "-text") {
			continue // declared binary; line endings are not this check's business
		}
		if index == "i/none" || index == "i/-text" {
			continue // binary, or a file with no line endings at all
		}
		checked++
		// ⛔ THE INDEX IS ALWAYS LF FOR A `text` FILE, whatever eol= says. That
		// attribute governs CHECKOUT, not storage, and a row that is not i/lf
		// here means normalisation is off rather than that the endings are
		// wrong on disk.
		if index != "i/lf" {
			r.bad("%s: git holds %s in the index where a text file normalises to i/lf; `git add --renormalize` is what fixes it",
				file, index)
		}
		want := wantedWorktreeEnding(attr)
		// ⚠ A FILE WITH NO LINE ENDING AT ALL IS NOT A VIOLATION. One line and
		// no trailing newline reports w/none, and there is nothing in it for an
		// attribute to resolve.
		if worktree == "w/none" {
			continue
		}
		if worktree != want {
			r.bad("%s: the working tree holds %s where .gitattributes resolves %s; this is invisible to git diff and visible to everything that reads the file",
				file, worktree, want)
		}
	}
	r.Extra["files"] = checked
	return r
}

// wantedWorktreeEnding is the rule this repository states, in one place so the
// check and its test cannot disagree about it: LF everywhere except PowerShell,
// which keeps CRLF because 5.1 mis-parses a here-string terminated by a bare LF.
func wantedWorktreeEnding(attr string) string {
	if strings.Contains(attr, "eol=crlf") {
		return "w/crlf"
	}
	return "w/lf"
}

// parseEOLRow splits one `git ls-files --eol` row into its three columns.
//
// ⛔ THE ATTRIBUTE COLUMN CONTAINS SPACES and is therefore the last field
// rather than a field. `i/lf w/crlf attr/text eol=crlf` splits on whitespace
// into FOUR tokens, and a parse that took the one beginning `attr/` kept
// `text` and silently dropped the half that says which ending is wanted.
func parseEOLRow(head string) (index, worktree, attr string, ok bool) {
	at := strings.Index(head, "attr/")
	if at < 0 {
		return "", "", "", false
	}
	attr = strings.TrimSpace(head[at+len("attr/"):])
	fields := strings.Fields(head[:at])
	if len(fields) < 2 {
		return "", "", "", false
	}
	return fields[0], fields[1], attr, true
}
