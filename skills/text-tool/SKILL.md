---
name: text-tool
description: Write and edit files from a shell without the shell ever touching the payload. Use when a harness has no file-write tool, when a heredoc or quoting has mangled content, when a substitution must assert how many places it changed, or when converting line endings and byte order marks in place of dos2unix and unix2dos.
---

# text-tool

One program. It writes a file, adds to a file, changes part of a file, and
converts line endings. It runs on Windows and on Linux with nothing installed
beside it.

⛔ **USE YOUR HARNESS'S OWN WRITE AND EDIT TOOLS FIRST.** They put bytes on disk
with no shell in the path, which is better than anything here. This is for a
harness that has none, and for the four things those tools usually cannot do:
assert how many places a substitution changed, edit many files as one unit,
convert line endings, and carry bytes that are not valid text.

## 1. Get it

Download the asset for your host from the latest release of
`Azathothas/ToolKit`, put it on `PATH`, and check it answers.

```bash
text-tool --help
```

The published names are `text-tool-windows-amd64.exe`,
`text-tool-windows-arm64.exe`, `text-tool-linux-amd64` and
`text-tool-linux-arm64`. Rename the one you take to `text-tool` (or
`text-tool.exe`) so every example here works as written. On Linux make it
executable with `chmod +x`.

⛔ **READ THE EXIT CODE FROM THE PROCESS, NOT THROUGH A PIPE.** `text-tool ... |
head` gives you `head`'s exit code, and the refusal you needed to see is the one
you will miss.

| code | what it means |
| --- | --- |
| 0 | it did what you asked |
| 1 | it REFUSED, and nothing was written |
| 2 | it could not run: a bad flag, a missing file |

## 2. The four modes

```bash
text-tool write PATH --text 'hello'
text-tool append PATH --text 'one more line'
text-tool edit PATH --replace 'old' --text 'new' --expect 1
text-tool eol PATH --lf
```

`write` replaces the whole file and creates parent directories. `append` adds to
the end. `edit` changes one part. `eol` converts line endings and the byte order
mark, and is the whole of what `dos2unix` and `unix2dos` do.

## 3. Get the payload past your shell

This is the reason the tool exists. A payload that crosses a shell boundary
loses its quoting SILENTLY: the file is written, nothing returns non-zero, and
the damage is a substituted fragment in the middle of a long document.

Give the payload through exactly one of these. Pick by what your shell makes
easy, and when in doubt pick the first.

| channel | when |
| --- | --- |
| `--b64 BASE64` | ⭐ **the one that cannot be mangled.** `[A-Za-z0-9+/=]` needs no quoting in ANY shell. Use it for anything holding a quote, a backtick, a dollar sign, a backslash or a newline |
| `--from FILE` | copy another file's bytes, with no re-encoding |
| `--text S` | ⚠ your shell sees this one. Short, plain content only |
| stdin | when you give none of the others |

```bash
text-tool write notes.md --b64 SGVsbG8sICJ3b3JsZCIK
```

⛔ **FROM POWERSHELL, USE `--b64` OR `--from`.** PowerShell's native-command
pipe APPENDS a trailing CRLF to whatever it sends, so a payload piped to stdin
arrives with bytes you did not put there.

⛔ **FROM POWERSHELL, INVOKE A QUOTED PATH WITH `&`.** A quoted string at the
start of a PowerShell command is an EXPRESSION, not an invocation, and answers
`Unexpected token`. This is measured: the same line passed in `cmd` and failed in
both PowerShell 7 and Windows PowerShell 5.1.

```powershell
& "D:\tools\text-tool.exe" write notes.md --b64 SGVsbG8K
```

⭐ **A bare `text-tool` on PATH needs no `&`.** The call operator is only for a
path you quoted.

⛔ **FROM GIT BASH ON WINDOWS, A `--text` STARTING WITH `//` LOSES A SLASH.** Git
Bash rewrites an argument that looks like a path before the program is started,
so `--text '// a Go comment'` arrives as `/ a Go comment`. Nothing returns
non-zero and the file is written with the wrong bytes.

```bash
text-tool write x.go --text '//double-slash'   # writes /double-slash
```

⭐ **Measured, and it is the whole reason this tool has channels.** The same
payload through `--b64` arrives intact, because base64 holds no character a shell
or its path translator will touch:

```bash
text-tool write x.go --b64 Ly9kb3VibGUtc2xhc2g=
```

⚠ **This is not something the tool can fix.** The mangling happens before the
program runs, so the only defence is a channel the shell has no opinion about.
Reach for `--b64` or `--from` for any payload that starts with a slash, and for
Go, C or JavaScript comments in particular.

⚠ **`--text` is read literally.** `--text 'a\nb'` writes a backslash and an `n`,
not a newline. For a real newline use `--b64` or `--from`. The tool prints a
note when it sees a literal `\n` or `\t` in `--text`, because that is almost
always a mistake.

## 4. --expect, which is the point

A substitution names no place of its own. One that silently matched nothing, or
matched four times when you meant one, is a different edit from the one you
asked for, and it is the kind nobody notices.

```bash
text-tool edit config.json --replace '"debug": true' --text '"debug": false' --expect 1
```

⛔ **A SEARCH WITHOUT `--expect` IS REFUSED OUTRIGHT**, and that is `--replace`,
`--after`, `--before` and `--between` alike. If you do not know the
number, ask first:

```bash
text-tool edit config.json --replace '"debug": true' --count
```

`--count` changes nothing and reports the matches. `--dry-run` reports what
would change and writes nothing.

When the number does not match, the file is untouched and you get exit 1:

```text
text-tool: --expect 1 and this matches 3 times across 1 file(s). Nothing was written
```

That is the tool working. Read the count, decide whether you meant it, and say
the real number.

## 5. The operations

```bash
text-tool edit PATH --replace 'FIND' --text 'NEW' --expect 1
text-tool edit PATH --after '## Heading' --text 'a new line' --expect 1
text-tool edit PATH --before '## Heading' --text 'a new line' --expect 1
text-tool edit PATH --line 12 --text 'the whole of line 12'
text-tool edit PATH --insert-after 12 --text 'goes after line 12'
text-tool edit PATH --insert-before 1 --text 'a new first line'
text-tool edit PATH --delete 5,9
text-tool edit PATH --between 'START' 'END' --text 'the replacement' --expect 1
```

⭐ **USE `--after` AND `--before` TO ADD SOMETHING BESIDE AN ANCHOR.** They keep
the anchor line. The obvious alternative is a substitution that matches the
anchor and replaces it with your text PLUS the anchor, and forgetting the second
half DELETES the anchor. That is the commonest way to damage a file with this
tool.

⛔ **`--between` NEEDS `--expect` FROM `wsl-toolkit-v4.0.0`, AND IT REPORTS THE
LINES IT TOOK.** ⚠ On a 3.1.0 binary the flag is optional there, so a reader
holding an older build will not see the refusal this section describes;
`text-tool --help` names what your copy really requires. It is the
widest operation here - it deletes a whole region rather than one line - and it
was the only search without a required count until 2026-09-17: with no
`--expect` it wrote nothing and exited **0**, both when two ranges matched and
when none did. ⚠ **A count is still not enough on its own.** An anchor that also
appears earlier in the file pairs the FIRST copy with the closing anchor, which
is exactly one match, so `--expect 1` is satisfied and a far bigger region goes.
That happened here, to a 43 KB script: 745 lines, reported as `1 match(es)`. The
report now names the span - `lines 2-9 (8 line(s))` - so read it.

⚠ **`--between` takes TWO arguments**, not one with a comma in it. An anchor
holding a comma is ordinary, and a separator that appears in the data is not a
separator.

⚠ `--replace-b64` and `--replace-from` give the SEARCH the same safe channels as
the payload. Reach for them whenever the text you are searching for holds
anything your shell would touch.

⚠ `--regex` reads the search as a regular expression and lets `$1` in the
payload refer to a group. It applies to `--replace`, `--between`, `--after` and
`--before`.

## 6. Many files at once

Every argument after the mode is a path, up to the first one starting with a
dash.

```bash
text-tool edit src/a.go src/b.go src/c.go --replace 'oldName' --text 'newName' --expect 7
```

⛔ **ALL OF THEM CHANGE OR NONE DO.** Every file is read and checked before any
is written, so a refusal on the last file leaves the first untouched.

⚠ **`--expect` is the TOTAL across every file named.** The per-file counts are
in the report.

⛔ **A NAMED FILE THAT MATCHED NOTHING REFUSES THE WHOLE CALL.** A path with a
typo and a file that has drifted look identical from inside the tool, and both
answer zero. Pass `--allow-unmatched` when you mean it.

For more paths than a command line holds, put them in a file, one per line;
blank lines and lines starting with `#` are skipped.

```bash
text-tool edit --files-from paths.txt --replace 'old' --text 'new' --expect 12
```

⚠ `--line`, `--insert-after`, `--insert-before` and `--delete` name a place in
ONE file and are refused when several are given. A line number means a different
place in each file. `--replace`, `--after`, `--before` and `eol` name the same
thing in each, so they take as many files as you like.

## 7. Line endings and the byte order mark

```bash
text-tool eol PATH --lf
text-tool eol PATH --crlf
text-tool eol a.txt b.txt c.txt --lf --bom strip
```

`--lf` is `dos2unix` and `--crlf` is `unix2dos`. `--to lf` and `--to crlf` are
the long spellings. `--bom` takes `keep` (the default), `strip` or `add`.

⚠ **The count is endings CONVERTED, not lines in the file.** A file already in
the wanted ending answers 0, so you can tell "nothing to do" from "everything
moved" without comparing byte totals.

⚠ **A lone carriage return is left alone.** Only a CR that is followed by an LF
is a line ending here. Rewriting a bare CR would edit data inside a quoted
string rather than the file's line structure.

⚠ **`--eol` is a different flag from the `eol` mode.** The `eol` MODE converts
the whole file. The `--eol` FLAG says which ending a PAYLOAD is written with in
the other modes, and its default, `keep`, takes the ending the file already
uses. You rarely need the flag.

## 8. What it will not do to your file

- It reads and writes BYTES. A file that is not valid UTF-8 round-trips
  unchanged, because nothing here decodes.
- A file with no trailing newline does not gain one.
- A file's existing line endings are kept, and a line inserted into a CRLF file
  takes CRLF.
- The write is atomic: a full file is put in place, so an interrupted run cannot
  leave a half-written file.
- A refused call writes nothing at all.

## 9. Reading the answer

```bash
text-tool edit PATH --replace 'old' --text 'new' --expect 2 --json
```

`--json` gives `text-edit/2`: a `mode`, an `op` naming the operation that ran, a
total `matches`, a `changed`, and a `files` array with one entry per path
holding `path`, `matches`, `bytes_before`, `bytes_after`, `eol`, `changed` and
the line numbers touched.

⭐ **`op` is what tells a `--between` apart from a `--replace` afterwards**, and
for a `--between` the human line also names the SPAN it took, as
`lines 2-9 (8 line(s))`. A wrong range is still exactly one match, so the span
is the only thing that shows it.

⚠ **`lines` is capped at twenty.** When it is shorter than `matches` the report
sets `lines_truncated`, so the two numbers disagreeing is never a mystery.

⚠ **`changed` is what HAPPENED, not what would have.** A `--dry-run` reports
`changed: false`, because nothing moved.

## 10. When it refuses

Every refusal writes nothing and exits 1. Read the message; it names the number
it found and what to do.

| message | do this |
| --- | --- |
| `--expect N and this matches M times` | run `--count`, decide whether you meant M, then say M |
| `needs --expect N` | you used `--replace`, `--after`, `--before` or `--between` without it |
| `file(s) matched nothing` | check the path for a typo; the file may have drifted; `--allow-unmatched` if you meant it |
| `names a place in ONE file` | run it once per file, or use `--replace` |
| `takes one operation and N were given` | pick one |
| `eol needs a target` | add `--lf`, `--crlf` or `--bom` |
| `--between takes two patterns` | pass them as two arguments, not one with a comma |

⛔ **DO NOT WORK AROUND A REFUSAL BY REACHING FOR A HEREDOC.** The refusal is the
tool telling you the edit you described is not the edit you meant. Writing the
file another way applies the wrong edit silently.
