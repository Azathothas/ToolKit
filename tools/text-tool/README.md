# text-tool

One program that writes a file, adds to a file, changes part of a file, and
converts line endings, without the shell ever touching the payload. It runs on
Windows and on Linux and needs nothing installed beside it.

⭐ **The page to hand an agent is
[`../../skills/text-tool/SKILL.md`](../../skills/text-tool/SKILL.md).** It stands
alone and is checked to name only flags this program has. This file is the
reference for the program itself.

## Why it exists

⛔ **THE DEFECT IS NOT "QUOTING IS HARD". It is that a payload crossing a shell
boundary loses its quoting SILENTLY.** The file is written, nothing returns
non-zero, and the damage is a substituted fragment in the middle of a long
document. [`../../docs/conventions/shell.md`](../../docs/conventions/shell.md)
section 1 carries the measurement: one line of prose holding backticks, passed to
`bash -c` inside a QUOTED heredoc, had its backticks executed.

⭐ **Use your harness's own write and edit tools first.** They put bytes on disk
with no shell in the path, which is strictly better than anything here. This is
for a harness that has none, and for four things those tools usually cannot
express: asserting how many places a substitution changed, editing many files as
one unit, converting line endings, and carrying bytes that are not valid text.

⭐ **Why Go and not a script.** [`../check`](../check) states the reason and it
holds here: a rule written twice, in sh and in PowerShell, needs a third check
comparing the two. One program runs natively on either host and has no halves to
disagree. Its predecessor, `scripts/common/write-file.mjs`, needed node, which
[`../../scripts/README.md`](../../scripts/README.md) calls the one thing under
`scripts/` that does.

## Build and drive

```bash
cd tools/text-tool && go build -o ../../.tmp/text-tool .
go test ./... -count=1
```

The wrappers build it on demand and are the way to reach it from inside this
checkout:

```bash
sh scripts/common/text-tool.sh --help
```

```powershell
pwsh -NoProfile -File scripts/common/text-tool.ps1 --help
```

⚠ **Both wrappers keep the CALLER's working directory**, unlike the others under
`scripts/common/`, because the paths given to this tool are the caller's paths.

## What it guarantees

| property | and what it means |
| --- | --- |
| bytes, never strings | a file that is not valid UTF-8 round-trips unchanged, because nothing here decodes |
| the file's own endings | CRLF stays CRLF, and a line inserted into such a file takes CRLF |
| no newline invented | a file whose last line has none does not gain one |
| atomic | a whole file is put in place, so an interrupted run cannot leave half a file |
| all or nothing | with several files named, every one is read and checked before any is written |
| nothing on a refusal | a refused call writes no byte to any named file |

## The count guard

⛔ **`--replace`, `--after` and `--before` require `--expect N`, and a different
number is refused with the file untouched.** A substitution names no place of its
own; one that silently matched nothing is what a caller never notices. The guard
refused its own author three times on the day it was written, which is three
silent no-ops that did not happen.

⚠ **`--expect` is the TOTAL across every file named.** The per-file counts are in
the report, so a refusal names which file disagreed.

## Shells

⛔ **THE CLAIM THIS TOOL MAKES IS ABOUT SHELLS, so it is the one claim the Go
suite cannot prove.** Those cases call `Run()` directly and never cross a shell
boundary, which is exactly where a payload loses its quoting.
[`shell-matrix.sh`](shell-matrix.sh) runs one base64 payload through every shell
on the host and compares the digests.

```bash
sh tools/text-tool/shell-matrix.sh .tmp/text-tool
```

⚠ **A shell that is not installed is SKIPPED and named**, never silently passed,
and a run that found fewer than two shells exits 1 rather than reporting that
they agreed.

## Reading the answer

`--json` gives `text-edit/2`: a `mode`, a total `matches`, a `changed`, and a
`files` array with one entry per path.

⚠ **One shape whether one file was named or twenty.** The first version emitted a
flat object for a single file, which would have given a caller two shapes to
parse from one command.

⚠ **`lines` is capped at twenty**, and when it is shorter than `matches` the
report sets `lines_truncated`. A report whose two halves disagree with nothing
saying why is a defect even when every number in it is correct.

## Published

From `wsl-toolkit-v3.1.0` each release publishes `text-tool-windows-amd64.exe`,
`text-tool-windows-arm64.exe`, `text-tool-linux-amd64` and
`text-tool-linux-arm64`, each with a cosign bundle, and
[`../../docs/consumers.md`](../../docs/consumers.md) says how to verify one.
`release.yml` runs the staged Windows binary before publishing it: it writes a
file, edits it, and proves it REFUSES a wrong `--expect`, because a build with
the guard compiled out would pass every other line.

## Exit codes

⛔ **Read them from the process, never through a pipe.**

| code | meaning |
| --- | --- |
| 0 | it did what it was asked |
| 1 | it refused, and nothing was written |
| 2 | it could not run |
