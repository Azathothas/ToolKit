# selftest.ps1

Run [`wsl-toolkit.ps1`](../../../tools/windows/wsl-toolkit/wsl-toolkit.md)'s pure functions against a table of
cases. No WSL, no container engine, nothing created, nothing written.

```powershell
pwsh -NoProfile -File scripts/windows/wsl-toolkit/selftest.ps1
```

```text
selftest: 131 case(s) passed over 36 function(s) loaded from wsl-toolkit.ps1.
```

---

## What it is for

⭐ **Most of `wsl-toolkit.ps1` can only be proved by driving a real distro**,
which needs WSL, an engine, several hundred megabytes and a minute. The parts
that decide what a caller **sees** do not:

| what it holds | the wrong answer it would otherwise give |
| --- | --- |
| the timestamp renderer | a month where a minute belongs, on every line of every log |
| the line splitter | a progress bar shown as nothing at all, or one line printed as two |
| the file channel | a CRLF payload sent to die on `/bin/sh`, or a UTF-16 one sent to die on a NUL |
| the argument prologue | a value with a quote in it breaking the script it was passed to |
| the transport alphabet | a payload that arrives mangled instead of being refused |

⛔ **Before this file, none of them had a test.** Two defects were found by it on
its first run, and both are recorded in
[`../../../docs/HISTORY/wsl-toolkit.md`](../../../docs/HISTORY/wsl-toolkit.md).

## How it loads the functions

⛔ **`wsl-toolkit.ps1` cannot be dot-sourced.** Its top level dispatches an
action and calls `exit`, which would end the session running the test. So this
parses the file, takes the function definitions it names, and defines those
alone.

⭐ **The number it found is asserted against the number it asked for.** A renamed
function would otherwise drop its cases silently and leave a smaller suite
reporting green. The same rule holds the case count: under forty cases is a
refusal, not a pass.

⚠ **Four output helpers and the host-address lookup are replaced by doubles**,
so a case can read what was reported and so nothing here touches a network
interface.

## What it does not cover

⛔ **Anything that talks to `wsl.exe`, to a container engine, or to the
filesystem.** The relay loop, the disk preflight and every destructive path are
proved by running them, which is part (b) of
[`../../../docs/methodology/gate.md`](../../../docs/methodology/gate.md).

## The contract it satisfies

[`../README.md`](../../README.md)'s five-point check contract, though it is a test
rather than a check:

| | |
| --- | --- |
| exit codes | 0 pass, 1 fail, ⚠ 2 only when `wsl-toolkit.ps1` is not beside it |
| `-Json` | `{"schema":"wsl-toolkit-selftest/1","cases":131,"failed":0,"functions":36}` |
| working directory | resolved from the script's own location |
| writes | none |

## Where it runs

| | |
| --- | --- |
| the local gate | inside the `bundle` check, which runs `build.ps1 -Test` |
| CI, ubuntu | the gate, plus a second run of its own |
| ⭐ CI, windows | the gate, plus a run under **each** PowerShell host with the two case counts compared. Every P0 this tool has had lived in 5.1 and was invisible on 7. `TOOL-11`. |

⚠ **Three hosts, and the third is not decoration.** Measured on 2026-09-10, the
same file reports `131 case(s) over 36 function(s)` under PowerShell 7 on
Windows, under Windows PowerShell 5.1, and under PowerShell 7 on Linux.

⛔ **TWO CASES USED TO FAIL ON THE THIRD**, because they reached for `$env:TEMP`
and `$env:WINDIR`, which are null there, and `Join-Path` refuses a null path.
`WSL-57`. The rule the fix follows is this tree's own: resolve a program rather
than spell its path.

⭐ **The Linux half is reachable from the Windows host in seconds**, so a
failure there does not have to be found by CI and fixed by pushing again:

```powershell
podman run --rm -v "${PWD}:/repo:ro" mcr.microsoft.com/powershell:latest pwsh -NoProfile -File /repo/scripts/windows/wsl-toolkit/selftest.ps1
```

⚠ **Windows PowerShell 5.1 and PowerShell 7 both run it.** It is ASCII-only, so
it needs no byte order mark.

## Related

- [`tools/windows/wsl-toolkit/wsl-toolkit.md`](../../../tools/windows/wsl-toolkit/wsl-toolkit.md), the tool under test.
- [`../../../docs/methodology/reviews.md`](../../../docs/methodology/reviews.md), lens
  2: a guard that has never been seen to refuse is a guard nobody knows works.
  Ten of the cases here plant exactly the defect their guard exists to catch.
