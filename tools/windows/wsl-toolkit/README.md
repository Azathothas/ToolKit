# wsl-toolkit, the executable

This executable is the primary interface. Run `wsl-toolkit man` for a paged
manual, `wsl-toolkit man --no-pager` for readable text, or `wsl-toolkit
examples` for canonical commands. [`wsl-toolkit.md`](wsl-toolkit.md) is the
short operator reference.

This page describes the source and build process for a maintainer.

---

## The shape

```text
tools/windows/wsl-toolkit/
  go.mod                    the module. ⛔ No dependencies, and that is a decision.
  main.go                   the dispatch, the global flags, and the two output rules
  cmd_*.go                  one file per command surface, all thin
  helper_route.go           the one place a command decides between the two paths
  internal/script/          the embedded PowerShell product, and its reconstruction
  internal/toolkit/         everything else: WSL, the engine, jobs, the fleet, the helper
```

⭐ **It mirrors `scripts/windows/wsl-toolkit/` on purpose.** The PowerShell
bundle is the low-level compatibility interface for existing consumers. The
executable embeds that interface and adds the persistent base, container jobs,
the host survey, and the generated manual.

⛔ **No dependencies.** Every line is the standard library, so there is no
`go.sum`, nothing to audit on a bump and nothing to fetch in CI. Keep it that
way: the two places that would want one, the helper's transport and the Windows
path resolver, are a few lines of `syscall` each.

---

## ⛔ The embedded script is GENERATED, and this directory holds a copy

`internal/script/wsl-toolkit.ps1` is written by
[`../../../scripts/windows/wsl-toolkit/build.ps1`](../../../scripts/windows/wsl-toolkit/README.md)
from the same parts that produce the tracked bundle. Go's `embed` directive
cannot reach outside its own package directory, so the file is here rather than
referenced.

⭐ **`build.ps1 -Check` compares BOTH copies against what the parts build**, and
that is what stops the two from ever being different scripts. A rebuild that
refreshed only the tracked bundle would ship a binary running the previous script
with nothing saying so.

⚠ **It is the one `.ps1` in this tree stored with LF.** git rewrites a
`text eol=crlf` file on checkout, so an ubuntu build and a windows build of one
commit would otherwise embed different bytes. The build leaves no lone carriage
return, so turning each LF back into CRLF reconstructs the released artefact
exactly. `TestStoredCopyReconstructsTheProduct` asserts that against the tracked
file, and CI and the release workflow both run it by name.

---

## Changing it

```bash
sh scripts/common/check-go.sh
```

That is gofmt, `go vet`, `go build` and `go test`, and it is what the local gate
and both CI jobs run. ⛔ When it and `.github/workflows/ci.yml` disagree about
what is checked, the check is what a session actually experiences and the
workflow is the defect.

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/build.ps1
```

Run that after editing any part of the PowerShell product, because it writes the
copy this module embeds.

### Building one to try

```bash
cd tools/windows/wsl-toolkit && go build -trimpath -o ../../../.tmp/wsl-toolkit.exe .
```

⚠ `.tmp/` is gitignored, and so is a binary built in this directory. ⛔ Kill a
stray copy before rebuilding: the Windows trap behind that is in
[`../../../docs/conventions/shell.md`](../../../docs/conventions/shell.md)
section 7, which owns it.

---

## What the suite covers, and what it cannot

```bash
cd tools/windows/wsl-toolkit && go test ./...
```

⭐ **Cases are named for the behaviour they assert**, not for the function they
call. The ones that matter are the guards that keep a container away from this
machine: the archive-entry rules, the containment test, the argument alphabet,
the exact-name distribution rule, the image-reference rule and the three-count
fleet verdict.

⛔ **Add a guard, mutation-prove it, and add its row to
[`../../repo/mutations.json`](../../repo/mutations.json).** `repo mutate` removes
what the guard protects, runs the case named for it, and reads the exit code from
the process that produced it. A guard with no row is a guard nobody has seen
refuse.
[`../../../docs/methodology/reviews.md`](../../../docs/methodology/reviews.md)
lens 2 is the rule and what it cost.

⚠ **A composite literal whose element opens immediately after the slice's own
brace trips `check-placeholders`.** Two braces with an uppercase letter after
them is the shape of an unfilled template, to a check that scans every file in
the tree. Bind the element in a variable first; that is a two-line change,
against widening a guard that protects every document here.

⛔ **What it does not cover:** anything that talks to `wsl.exe`, to a container
engine or to a real distribution. Those are part (b) of
[`../../../docs/methodology/gate.md`](../../../docs/methodology/gate.md), and
`acceptance.ps1` is how this tool answers it.

---

## The acceptance runner

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1 -Binary .tmp/wsl-toolkit.exe
```

⭐ **It drives a real machine**, which is the whole reason it exists: it registers
nothing the suite above can reach. Twenty-three cases, covering both accounts, all
twelve catalog images, direct and helper execution, hostile archive names, a
workspace a container tried to destroy, a failing command, a deadline, and the
counts returning to zero afterwards.

⭐ **One case asks both paths the same question and compares the answers.** That
is what a difference between them looks like from outside, and it is how the
helper was found to be dropping `--user`: it accepted the flag and ran the job
as root.

| flag | what it changes |
| --- | --- |
| `-Binary <path>` | which executable is driven. Required |
| `-Quick` | skips the twelve-image fleet row, which is the only slow one |
| `-Json` | one object on stdout, for a caller that is not a person |

⛔ **It asserts the number of cases it ran**, in both modes. A table that stopped
early exits 0 over a smaller suite.

⛔ **It refuses to touch a distribution it did not make.** It reads the
distribution list before it starts and asserts the same names are registered at
the end, so a run that damaged `podman-machine-default` fails rather than being
noticed later.

⚠ **PowerShell 7 or newer.** It passes each argument through
`ProcessStartInfo.ArgumentList`, which is .NET Core only.
[`../../../docs/conventions/shell.md`](../../../docs/conventions/shell.md)
section 8 owns why `Start-Process -ArgumentList` cannot be used here.

⚠ **It leaves the machine where it found it, and that is asserted rather than
assumed.** Its scratch is under `.tmp/`, removed in a `finally`; the base
distribution stays, because building it is the expensive part and the next run
reuses it. `gc --apply` removes what a run made.

---

## Releasing it

⭐ **Two halves, on purpose**, and the same two the script has.
[`../../../scripts/windows/wsl-toolkit/release.ps1`](../../../scripts/windows/wsl-toolkit/README.md)
verifies and pushes a tag;
[`../../../.github/workflows/release.yml`](../../../.github/workflows/release.yml)
checks that tag out clean, re-runs the verification, cross compiles both targets
and publishes.

| asset | what it is |
| --- | --- |
| `wsl-toolkit.ps1` | the PowerShell product, byte for byte as the tree holds it |
| `launcher.ps1` | the wrapper that fetches and verifies either one |
| `wsl-toolkit-windows-amd64.exe` | this executable |
| `wsl-toolkit-windows-arm64.exe` | the same, for an arm64 Windows host |
| `SHA256SUMS` | ⭐ computed in CI over the bytes that are uploaded |

⛔ **The workflow runs the staged binary before it publishes it**, and asks it
the one question whose answer proves it carries the right script: its version,
which it reads out of the embedded copy rather than declaring. A binary that
builds and cannot start is a release nobody can use.

⛔ **`-trimpath` and no build stamp.** Two builds of one commit produce identical
bytes, so a consumer can rebuild and compare rather than trusting the publisher.
A path or a timestamp compiled in would make every build unique and that property
unavailable.

⚠ **Only Windows targets are published**, because every command that is not a
survey drives `wsl.exe`. The module builds and its suite passes on Linux, which
is how CI catches a guard whose answer depends on the host it runs on.

---

## The version has one home

`$script:ToolkitVersion` in
[`../../../scripts/windows/wsl-toolkit/src/20-prelude.ps1`](../../../scripts/windows/wsl-toolkit/src/20-prelude.ps1).
This module READS it out of the embedded script and declares none of its own, so
the executable and the script it carries cannot disagree about what they are.
⛔ Nothing here may hold a copy of it.

---

## Related

- [`wsl-toolkit.md`](wsl-toolkit.md), what the executable does
- [`../../../scripts/windows/wsl-toolkit/README.md`](../../../scripts/windows/wsl-toolkit/README.md),
  the same page for the PowerShell product
- [`../../../scripts/README.md`](../../../scripts/README.md), the contract every
  check in this repository is held to
