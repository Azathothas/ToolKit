// text - write and edit a file without the shell ever touching the payload.
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

	"github.com/Azathothas/ToolKit/tools/text/internal/edit"
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
		fmt.Fprintln(os.Stderr, "text: "+strings.TrimSpace(err.Error()))
	}
	return code
}

const usage = `text <write|append|edit> PATH [payload] [operation]

  write    replace the whole file, creating parents
  append   add to the end
  edit     change one part of it

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
  --regex              read FIND as a regular expression, and $1 in the payload
  --line N             replace line N with the payload
  --insert-after N     put the payload after line N. 0 means before the first
  --insert-before N    put the payload before line N
  --delete N[,M]       delete line N, or lines N to M inclusive
  --between A,B        replace from the line matching A to the line matching B

ALWAYS:
  --expect N    how many matches you believe are there. ⛔ A different number is
                REFUSED and the file is untouched. A silent no-op that reports
                success is the failure this whole tool exists to remove
  --count       report the matches and change nothing
  --dry-run     report what would change and write nothing
  --json        a structured answer on stdout
  --eol MODE    keep (default), lf or crlf

Exit codes: 0 it did it, 1 it refused, 2 it could not run.
⛔ Read the exit code from the process, unpiped.
`
