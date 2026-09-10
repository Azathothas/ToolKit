# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-09T21:00:00Z
baseline        450b380, clean main; gate 17 checks, all passing, 31s.
entries         total 66  open 8  blocked 0  done 58
gate            18 checks, one binary, 43s, all passing
```

## Active work

None. All three bodies of work the operator named are closed.
[WSL-32](wsl-toolkit-go.md) through `WSL-39` resolve
[issues 7 to 14](https://github.com/Azathothas/ToolKit/issues) in full;
[TOOL-14](tooling.md) ports five of the last six shell pairs;
[WSL-40](wsl-toolkit-go.md) and [WSL-41](wsl-toolkit-go.md) are the core pass and
the review that followed it. Each entry carries the command that closed it and
its real output.

## What this session shipped

Two releases. `wsl-toolkit-v1.2.0` carries the eight defects a consumer agent
found by testing the published `v1.1.0` from outside this tree, five of them P1.
`wsl-toolkit-v1.3.0` carries the core pass and the review after it: three defects
nobody reported, five more in the class of "a failure reported as a benign
outcome", and the two lines that make `logs` reachable at all.
[`../CHANGELOG.md`](../CHANGELOG.md) is the shipped record;
[`wsl-toolkit-go.md`](wsl-toolkit-go.md) carries one entry per unit of work with
its evidence. Between them, `tools/repo/` became this repository's tool box.

⭐ **The finding worth keeping is not any of the eight.** This tree's own
gate was green when they were filed, its acceptance runner passed 23 of 23, and
it had been through three review lenses. A short test from outside found what
none of that did, and the operator's framing was that the eight are therefore a
floor rather than a list.

⭐ **The second finding worth keeping is what a NAMED LENS gets that a sweep
does not.** The fifth review took one defect class, a failure reported as a
benign outcome, and read every discarded error and every `return exitOK` with a
failure above it. That produced five findings in a tree that had just passed 37
acceptance cases and 29 proved mutations. A lens with a name finds things; another
pass of general care does not.

## Measurements

Read from the machine, on Windows 11 Pro 26200, on 2026-09-10:

```text
acceptance  39 of 39 cases pass against the real base, both routes, both
            accounts, every catalog image. It was 23 before this session; the
            14 issue cases each fail against wsl-toolkit-v1.1.0, and the two
            newest cover the helper route's own transcript and the line that
            names the job id.
mutation    48 of 48 guards proved: each one deleted, the named case run, and
            the case count and the build status reported separately. One row
            asks for -race, because without it that guard goes red in only 6
            runs of 10. It is `repo mutate` now, in the tree, so the number is
            one anybody can reproduce.
linux       all three Go modules vet and test clean inside
            docker.io/library/golang:1.25, driven by this tool.
race        the whole Go suite passes under -race.
gate        18 checks, one binary, 43s on this host. The eighteenth is
            `hooks`, and it is the one that stops `commits` being a rule
            with no instrument.
surface     71 flags across 12 commands, every one of them named in the manual,
            asserted by a test rather than by a reading.
```

⚠ **One guard is deliberately absent and the harness says so in place of it.**
`Ledger.Compact`'s lost-append window is too narrow to hit: the broken version
was run against a 50-append, 20-compaction stress case ten times and went red in
NONE of them. A row reporting theatre every run would be noise and a row
reporting ok would be a lie, so the invariant is held by structure and the test
written for it states what it does not prove.

## Decisions

The operator ruled on four forks on 2026-09-09, and each is recorded in the
entry it belongs to:

- both routes stream, which costs a helper protocol version ([WSL-35](wsl-toolkit-go.md));
- a failed transfer exits 1 and the guest copy is KEPT ([WSL-33](wsl-toolkit-go.md));
- `gc` spares live work and `--include-live` is the only way past it ([WSL-36](wsl-toolkit-go.md));
- the eight fixes ship as `wsl-toolkit-v1.2.0` before anything else is started.

## Work order

Nothing is queued. What a next session would pick up, in the order it is worth
picking up:

1. **The doctor pair is the last shell twin**, `scripts/doctor/doctor.sh` and its
   PowerShell twin, 646 lines between them. [TOOL-14](tooling.md) says why it was
   left: it is the only pair whose two halves ask DIFFERENT questions of
   different hosts, so porting it is a behaviour decision and not a translation.
   ⚠ When it goes, `check-twins.sh` goes with it, and
   [RULES.md](RULES.md) section 2 and `scripts/README.md` move in the same change.
2. **A sixth lens.** The fifth was "what a failure is allowed to hide" and it
   found five things. An obvious next one is concurrency: what two of this tool
   running at once do to one state directory, which the ledger work touched and
   did not finish.

⚠ **What a later session should know before touching this tool.** The two
generated products remain the trap: `scripts/windows/wsl-toolkit/wsl-toolkit.ps1`
and `tools/windows/wsl-toolkit/internal/script/wsl-toolkit.ps1` are BOTH built
from the parts, and editing either by hand is lost at the next build.
[RULES.md](RULES.md) section 4 owns that.

⭐ **Three guards added this session exist to stop a CLASS coming back**, and a
session that finds one inconvenient should read why before changing it:

| guard | what it refuses |
| --- | --- |
| `TestEveryJobFlagCrossesTheWire` | a job flag the direct path honours and the helper drops. That has now happened twice. |
| `TestManualNamesEveryFlag` | a flag the binary has and the manual does not. It reads the real flag sets, so a flag added tomorrow is covered without the test being touched. |
| `TestEveryFlagSetRefusesPositionals` | a subcommand that tolerates a stray word, and therefore ignores every option after it. |
| `TestPrefixWriterIsSafeFromTwoStreams` | a shared writer losing a provisioning line. ⚠ Run it under `-race`: without the detector it catches the corruption in 6 runs of 10. |
| `TestClientSpoolSaysWhyItHasNoTranscript` | a helper-route job that quietly keeps no local transcript. The direct route has always said so; this is the half that did not. |
| `TestListTranscriptsSeparatesAnEmptyMachineFromAnUnreadableOne` | a read failure reported as a machine that has simply not run a job yet. |
