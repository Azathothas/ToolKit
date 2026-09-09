// Package deslop answers which files in a tree address a reader as an agent,
// and removes them when asked.
//
// ⭐ AN INVENTORY, NOT A GATE. It exits 0 whether it finds twenty agent-facing
// files or none, because in the repository that SHIPS them their presence is
// correct. Only --apply changes anything, and only then can it fail.
//
// ⛔ IT IS AIMED AT ANOTHER TREE. Every pattern below names a path
// `Azathothas/TEMPLATE` ships. Run with --apply here and it removes THIS
// repository's own router and methodology, which are content it wants rather
// than content it regrets.
//
// SPDX-License-Identifier: 0BSD
package deslop

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/Azathothas/ToolKit/tools/repo/internal/gitrepo"
)

// Schema versions the structured answer, because its reader is a program.
//
// ⚠ VERSION 2 ADDS `files`, which the shell implementation did not carry.
// The human output always listed them and the structured one made a caller run
// it twice to find out which files the count was about. A field added to a
// versioned schema is still a change to it, so the version moved rather than
// the field appearing under the old number.
const Schema = "deslop/2"

// Options are the flags this tool takes.
type Options struct {
	JSON  bool
	Apply bool
}

// Report is what one run found.
type Report struct {
	Schema           string   `json:"schema"`
	AgentFacing      int      `json:"agent_facing"`
	ReferencingFiles int      `json:"referencing_files"`
	Applied          bool     `json:"applied"`
	Files            []string `json:"files,omitempty"`
	Removed          int      `json:"removed,omitempty"`
	Survived         []string `json:"survived,omitempty"`
}

// isAgentFacing matches the WHOLE PATH, anchored.
//
// ⛔ An unanchored match on "agent" would take `src/agents/` in a project that
// happens to build one, which is a deletion of somebody's source code.
//
// ⚠ THE FAMILY OF ROUTER FILENAMES IS DELIBERATELY WIDE. Several tools read a
// file of this shape under their own name from anywhere in a tree, so a project
// that wants none of them wants all of these gone.
func isAgentFacing(p string) bool {
	base := path.Base(p)
	switch base {
	case "AGENTS.md", "CLAUDE.md", "GEMINI.md", ".cursorrules", ".windsurfrules":
		return true
	}
	switch p {
	case "ROUTE.md", "ADOPT.md", "MAINTAIN.md", ".github/copilot-instructions.md":
		return true
	}
	for _, prefix := range []string{"bootstrap/", "docs/methodology/", "docs/templates/"} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// Run is the whole tool. It writes its report to out and its refusals to errOut,
// and returns the exit code.
//
// Exit codes: 0 the inventory ran, or the removal succeeded; 1 --apply was asked
// for and could not be done safely; 2 could not run.
func Run(opts Options, out, errOut io.Writer) int {
	repo, err := gitrepo.Open()
	if err != nil {
		fmt.Fprintf(errOut, "deslop: %s\n", err)
		return 2
	}
	files, err := repo.Files()
	if err != nil {
		fmt.Fprintf(errOut, "deslop: %s\n", err)
		return 2
	}

	var hits, others []string
	for _, f := range files {
		if isAgentFacing(f) {
			hits = append(hits, f)
			continue
		}
		others = append(others, f)
	}

	// ⭐ THE REFERENCES MATTER MORE THAN THE FILES. Deleting a document is easy;
	// the expensive part is the twenty links elsewhere that now resolve to
	// nothing. Counting them here means the size of the real job is visible
	// BEFORE anything is removed.
	refs := countReferences(repo, hits, others)

	rep := Report{Schema: Schema, AgentFacing: len(hits), ReferencingFiles: refs, Files: hits}

	if !opts.Apply {
		if opts.JSON {
			return writeJSON(out, errOut, rep)
		}
		if len(hits) == 0 {
			fmt.Fprintln(out, "no agent-facing files in this tree.")
			return 0
		}
		fmt.Fprintf(out, "agent-facing files, %d:\n\n", len(hits))
		for _, f := range hits {
			fmt.Fprintln(out, f)
		}
		fmt.Fprintf(out, "\n%d other file(s) reference one of them, and every such link breaks on removal.\n\n", refs)
		fmt.Fprint(out, adviceBeforeRemoving)
		fmt.Fprintln(out, "\nNothing was changed. Pass --apply to remove the list above.")
		return 0
	}

	// ⛔ REFUSES ON A DIRTY TREE. A deletion of this size mixed into uncommitted
	// work cannot be reviewed, and cannot be undone with one command.
	dirty, err := repo.Dirty()
	if err != nil {
		fmt.Fprintf(errOut, "deslop: %s\n", err)
		return 2
	}
	if dirty {
		fmt.Fprintln(errOut, "deslop: the tree is dirty. Commit or stash first.")
		fmt.Fprintln(errOut, "deslop: a removal this size has to be reviewable on its own.")
		return 1
	}
	if len(hits) == 0 {
		if opts.JSON {
			rep.Applied = true
			return writeJSON(out, errOut, rep)
		}
		fmt.Fprintln(out, "nothing to remove.")
		return 0
	}

	// ⛔ THE STATE IS READ BACK, and the report is what is TRUE rather than what
	// was attempted. The shell predecessor to this ran `git rm || rm -f || true`
	// and then printed the number it had PLANNED to remove, so a file something
	// held open read as a file that had gone.
	var survived []string
	removed := 0
	for _, f := range hits {
		if _, err := repo.Git("rm", "-q", "--", f); err != nil {
			_ = os.Remove(pathIn(repo.Root, f))
		}
		if exists(pathIn(repo.Root, f)) {
			survived = append(survived, f)
			continue
		}
		removed++
	}
	rep.Applied, rep.Removed, rep.Survived = true, removed, survived
	if opts.JSON {
		code := writeJSON(out, errOut, rep)
		if len(survived) > 0 {
			return 1
		}
		return code
	}
	fmt.Fprintf(out, "removed %d of %d agent-facing file(s).\n\n", removed, len(hits))
	if len(survived) > 0 {
		fmt.Fprintln(out, "⛔ STILL PRESENT after the removal:")
		for _, f := range survived {
			fmt.Fprintf(out, "  %s\n", f)
		}
		fmt.Fprintln(out, "\nSomething is holding them open, or the path is not writable. Nothing was")
		fmt.Fprintln(out, "reported as removed that is still there.")
		return 1
	}
	fmt.Fprintf(out, "⛔ NOT DONE YET. %d file(s) referenced them and those links now resolve\n", refs)
	fmt.Fprint(out, "to nothing. Run the documentation check and fix every one:\n\n")
	fmt.Fprint(out, "    sh scripts/common/check-docs.sh\n\n")
	fmt.Fprintln(out, "⛔ History was NOT touched, and rewriting it would not un-publish anything.")
	fmt.Fprintln(out, "Every fork, mirror and archive keeps its copy. docs/security/remote-ops.md.")
	return 0
}

const adviceBeforeRemoving = `⛔ Read the lean-adoption page in Azathothas/TEMPLATE before removing any
of this. Four practices under docs/methodology/ are engineering rather than
agent instruction. Lift them into the project's own contributing guide first.
`

// countReferences counts the files that mention one of the hits by its FULL
// PATH.
//
// ⛔ NOT BY BASE NAME, and the difference was measured while porting this:
// matching `gate.md` as well as `docs/methodology/gate.md` took the count on
// this tree from 23 to 30. The wider match is arguably the more useful answer,
// since a link from a sibling directory carries the base name alone. It is not
// what the shell implementation did, and a port that quietly changes a number
// its caller reads is a port nobody can check. Widening it is a decision to
// make on its own, with the count it changes stated.
//
// ⚠ Either way this is a starting point for a person and never an answer:
// whether a file addresses an agent is a reading.
func countReferences(repo *gitrepo.Repo, hits, others []string) int {
	if len(hits) == 0 {
		return 0
	}
	needles := make([]string, 0, len(hits))
	seen := map[string]bool{}
	for _, h := range hits {
		if h != "" && !seen[h] {
			seen[h] = true
			needles = append(needles, h)
		}
	}
	count := 0
	for _, f := range others {
		body, err := repo.Read(f)
		if err != nil {
			continue
		}
		text := string(body)
		for _, n := range needles {
			if strings.Contains(text, n) {
				count++
				break
			}
		}
	}
	return count
}

func writeJSON(out, errOut io.Writer, rep Report) int {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		fmt.Fprintf(errOut, "deslop: %s\n", err)
		return 2
	}
	return 0
}

func pathIn(root, rel string) string {
	return root + string(os.PathSeparator) + strings.ReplaceAll(rel, "/", string(os.PathSeparator))
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
