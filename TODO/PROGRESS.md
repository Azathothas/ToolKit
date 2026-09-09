# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-09T21:00:00Z
baseline        450b380, clean main; gate 17 checks, all passing, 31s.
entries         total 62  open 8  blocked 0  done 54
gate            17 checks, one binary, 41s, all passing
```

## Active work

None. [WSL-32](wsl-toolkit-go.md) through `WSL-39` closed on 2026-09-10, one per
issue, each with its acceptance output pasted underneath it. That resolves
[issues 7 to 14](https://github.com/Azathothas/ToolKit/issues) in full.

## What this session shipped

Eight defects a consumer agent found by testing the published
`wsl-toolkit-v1.1.0` from outside this tree, five of them P1.
[`../CHANGELOG.md`](../CHANGELOG.md) is the shipped record and says what each
one was; [`wsl-toolkit-go.md`](wsl-toolkit-go.md) carries one entry per issue
with its evidence.

⭐ **The finding worth keeping is not any of the eight.** This tree's own
gate was green when they were filed, its acceptance runner passed 23 of 23, and
it had been through three review lenses. A short test from outside found what
none of that did, and the operator's framing was that the eight are therefore a
floor rather than a list.

## Measurements

Read from the machine, on Windows 11 Pro 26200, on 2026-09-10:

```text
acceptance  37 of 37 cases pass against the real base, both routes, both
            accounts, every catalog image. It was 23 before this session; the
            14 new cases each fail against wsl-toolkit-v1.1.0.
mutation    23 of 23 guards proved: each one deleted, the named case run, and
            the case count and the build status reported separately.
linux       both Go modules vet and test clean inside
            docker.io/library/golang:1.25, driven by this tool.
race        the whole Go suite passes under -race.
gate        17 checks, one binary, 41s on this host.
surface     71 flags across 12 commands, every one of them named in the manual,
            asserted by a test rather than by a reading.
```

## Decisions

The operator ruled on four forks on 2026-09-09, and each is recorded in the
entry it belongs to:

- both routes stream, which costs a helper protocol version ([WSL-35](wsl-toolkit-go.md));
- a failed transfer exits 1 and the guest copy is KEPT ([WSL-33](wsl-toolkit-go.md));
- `gc` spares live work and `--include-live` is the only way past it ([WSL-36](wsl-toolkit-go.md));
- the eight fixes ship as `wsl-toolkit-v1.2.0` before anything else is started.

## Work order

⭐ **The two bodies of work this session did not start**, in the operator's own
priority order:

1. **Port the rest of the slow shell to Go.** `scripts/doctor/doctor.sh` and its
   twin (646 lines), `git-sync` (302), `check-binfmt` (212),
   `check-remote-items` (254), `deslop` (220) and `fill-license` (236). ⚠ When
   the last pair goes, `check-twins.sh` goes with it, and
   [RULES.md](RULES.md) section 2 and `scripts/README.md` move in the same
   change.
2. **Iterate on the core WSL behaviour**: the transport, the base lifecycle, the
   job model and the fleet.

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
