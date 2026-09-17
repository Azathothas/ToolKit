// text-tool - write and edit a file without the shell ever touching the payload.
//
// ⛔ THE DEFECT THIS EXISTS TO REMOVE IS NOT "QUOTING IS HARD". It is that a
// payload crossing a shell boundary loses its quoting SILENTLY, so the file is
// written, nothing returns non-zero, and the damage is a substituted fragment in
// the middle of a long document. docs/conventions/shell.md section 1 carries the
// measurement: one line of prose containing backticks, passed to `bash -c` inside
// a QUOTED heredoc, had the backticks executed.
//
// ⭐ USE YOUR HARNESS'S OWN WRITE AND EDIT TOOLS FIRST. They put the bytes on
// disk with no shell in the path at all, which is strictly better than anything
// here. This is for a harness that has none, and for the cases those tools cannot
// express: a substitution whose match count you want asserted, a line range, a
// file that is not valid UTF-8.
//
// ⭐ WHY GO AND NOT A SCRIPT. tools/check states the reason and it holds here: a
// rule written twice, in sh and in PowerShell, needs a third check comparing the
// two. One program runs natively on either host and has no halves to disagree.
// The predecessor, scripts/common/write-file.mjs, needed node, which
// scripts/README.md calls the one thing under scripts/ that does.
//
// SPDX-License-Identifier: 0BSD
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Azathothas/ToolKit/tools/text-tool/internal/edit"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(os.Stderr, usage)
		return 0
	}
	code, err := edit.Run(args, os.Stdout, os.Stderr, os.Stdin)
	if err != nil {
		if errors.Is(err, edit.ErrUsage) {
			fmt.Fprint(os.Stderr, usage)
		}
		fmt.Fprintln(os.Stderr, edit.Name+": "+strings.TrimSpace(err.Error()))
	}
	return code
}

const usage = `text-tool <write|append|edit|eol> PATH... [payload] [operation]

  write    replace the whole file, creating parents
  append   add to the end
  edit     change one part of it
  eol      convert line endings and the byte order mark. This is dos2unix and
           unix2dos, and it takes no payload

⭐ NAME AS MANY FILES AS YOU LIKE. Every argument after the mode is a path, up
to the first one starting with a dash. ⛔ ALL OF THEM CHANGE OR NONE DO: every
file is read and checked before any is written, so a refusal on the last one
leaves the first untouched.

THE PAYLOAD, and exactly one of these. Pick by what your shell makes easy:
  --b64 B64     base64. ⭐ [A-Za-z0-9+/=] needs no quoting in ANY shell, so this
                is the one that cannot be mangled. Use it for anything with a
                quote, a backtick, a dollar sign, a backslash or a newline
  --from FILE   copy another file's bytes, with no re-encoding
  --text S      a literal string. ⚠ Your shell sees this one, so it is for short,
                plain content only
  (stdin)       when none is given. ⛔ From PowerShell use --b64 or --from:
                PowerShell's native-command pipe APPENDS a trailing CRLF

EDIT OPERATIONS, exactly one:
  --replace FIND       substitute FIND with the payload. --expect is REQUIRED
  --replace-b64 B64    the same search, given as base64
  --replace-from FILE  the same search, read from a file
  --regex              read FIND as a regular expression, and $1 in the payload
  --after FIND         put the payload after each line matching FIND and KEEP
                       that line. --expect is REQUIRED
  --before FIND        the same, above the line. ⭐ USE THESE RATHER THAN A
                       SUBSTITUTION that has to retype what it matched: a
                       --replace whose replacement forgets to put the anchor
                       back DELETES it, which is the commonest way to damage a
                       file with this tool
  --line N             replace line N with the payload
  --insert-after N     put the payload after line N. 0 means before the first
  --insert-before N    put the payload before line N
  --delete N[,M]       delete line N, or lines N to M inclusive
  --between A B        replace from the line matching A to the line matching B.
                       --expect is REQUIRED. ⚠ TWO arguments, because an anchor
                       may hold a comma. ⛔ IT REPORTS THE LINES IT TOOK: an
                       anchor that also appears earlier in the file pairs with
                       the FIRST copy, which is still exactly one match

⚠ --line, --insert-after, --insert-before and --delete name a place in ONE file
and are refused when several are given. --replace and eol name the same thing in
each, so they take as many as you like.

EOL OPTIONS, for the eol mode:
  --lf | --crlf | --to lf | --to crlf    the ending to convert to
  --bom keep|strip|add                   the UTF-8 byte order mark

ALWAYS:
  --expect N    how many matches you believe are there, across ALL the files
                named. ⛔ A different number is REFUSED and nothing is written. A
                silent no-op that reports success is the failure this whole tool
                exists to remove
  --allow-unmatched  permit a named file that matched nothing. Without it, a
                file that matched nothing refuses the whole call, because a
                typo in a path and a file that has drifted look identical
  --files-from FILE  read paths from FILE, one per line; # and blanks skipped
  --help        print this and exit 0. -h and the word help do the same
  --count       report the matches and change nothing
  --dry-run     report what would change and write nothing
  --json        a structured answer on stdout, schema text-edit/2
  --eol MODE    keep (default), lf or crlf: the ending a PAYLOAD is written with

Exit codes: 0 it did it, 1 it refused, 2 it could not run.
⛔ Read the exit code from the process, unpiped.
`
