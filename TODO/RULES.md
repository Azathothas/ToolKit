# RULES.md

The part of the record that does not change between sessions.
[`PROGRESS.md`](PROGRESS.md) is what changed since last time and carries the
work order; [`INDEX.md`](INDEX.md) is the list of entries; this is the standing
state and the rules that are this repository's own.

⛔ **A rule is here only if it cost something here.** Everything general lives
in [`../docs/conventions/`](../docs/conventions/) and
[`../docs/methodology/`](../docs/methodology/), and this file links rather than
repeats, so the two cannot fork.

---

## The standing facts

⚠ **Read from the machine or the API, never typed from memory.** Each row names
where it is checked.

| fact | value | where it is read from |
| --- | --- | --- |
| repository | `Azathothas/ToolKit`, public, 0BSD | `gh api repos/Azathothas/ToolKit` |
| what it publishes | the `wsl-toolkit` tool, as a GitHub release on a `wsl-toolkit-v*` tag: from `wsl-toolkit-v3.0.0`, the executable for two Windows architectures, `SHA256SUMS`, and one `.cosign.bundle` per published file. Nothing else. ⚠ Every earlier release also carries `wsl-toolkit.ps1` and `launcher.ps1` | `gh release view TAG --repo Azathothas/ToolKit --json assets` |
| work model | todo | [`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md) |
| push policy | commit and push, to this remote only, on `main` | [`../docs/conventions/git.md`](../docs/conventions/git.md) section 2 |
| `main` | protected. One approving review, three required status checks, linear history. Force push and deletion refused. Admin bypass is on. | `gh api repos/Azathothas/ToolKit/branches/main/protection` |
| CI | `ci.yml` on every push: five jobs, one of which is a two-host matrix, so six runs. `release.yml` on a `wsl-toolkit-v*` tag, and it calls `release-smoke.yml`, which also runs weekly. `remote-items.yml` weekly. | [`../.github/workflows/`](../.github/workflows/) |
| ⚠ what `main` actually REQUIRES | three of those six: `checks (ubuntu)`, `powershell (windows)`, `the two probes agree`. The `go` matrix and the mutation job are not required, so a red one does not block a merge. | `gh api repos/Azathothas/ToolKit/branches/main/protection` |
| the local gate | `sh scripts/common/check-gate.sh`, or its `.ps1` twin | [`../scripts/README.md`](../scripts/README.md) |
| the identity a commit carries | the machine's `git config`, per invocation | [`../docs/conventions/git.md`](../docs/conventions/git.md) section 1 |

⛔ **The gate's measured cost belongs in [`PROGRESS.md`](PROGRESS.md), not
here.** It is re-timed on the machine that ran it and it moves, which is the
definition of something that does not belong in this file.

---

## 1. A tool here has callers this tree cannot see

⭐ **This is the rule that makes this repository different from an ordinary
project, and it is the one most likely to be got wrong.** A file here is fetched
by URL by other repositories and by the operator's own scripts. Nothing in this
tree fails when their contract is broken; only they do, later, on a machine
nobody is watching.

[`../docs/consumers.md`](../docs/consumers.md) is the register, the definition
of a break, and what a breaking change owes.

**What it cost.** A copy of this tool, under its old name `wsl-ephemeral.ps1`,
was found in `pkgforge-dev/cross-libc-dlopen` on 2026-08-27, during a reading of
that repository for an unrelated reason. It was not in the register, it fetches
nothing, and it carries both P0 defects this repository has since closed. The
register cannot reach it and neither can a fix.

⚠ **So the register is a lower bound on who is affected, never the complete
set**, and an entry that changes a fetched file says which consumers were
checked rather than assuming the answer.

## 2. The rules are ONE program, and it is not shell

⛔ **Every rule this repository enforces over its own tree lives in
[`../tools/check/`](../tools/check/).** One binary, one tree walk, and it runs
natively on either host. `scripts/common/check-*.sh` and their `.ps1` twins are
wrappers around one named check of it.

**What it cost to learn.** Each rule used to be written twice, in sh and in
PowerShell, because the default host here is Windows and a POSIX check cannot
be assumed to run on it. That is a real hazard: a native PowerShell session
resolves `sort` to `Sort-Object`, which accepts `-u`, compares
case-insensitively, and returned two of four distinct values without erroring.
A missing tool announces itself; an aliased one answers differently and reports
success.

⚠ **But keeping two implementations in step needed a third check that ran both
halves of every pair**, and that check was most of a gate taking about twelve
minutes on this machine. A gate that long is a gate a session skips, and
`--fast` existed to skip exactly it. One implementation has no halves to
compare and nothing to skip.

⛔ **ONE PAIR IS GENUINELY TWO IMPLEMENTATIONS, and it is the doctor probe.**
It RUNS BEFORE YOU KNOW WHAT IS INSTALLED, which is its whole job, so it cannot
require a POSIX layer and cannot require a Go toolchain either: "is there a Go
toolchain" is one of the questions it answers. Everything else that was a pair
is a wrapper now, over [`../tools/check/`](../tools/check/) for the rules and
[`../tools/repo/`](../tools/repo/) for the tools that are not rules.

⛔ **`check-twins.sh` still exists and still matters.** It covers the earned
pair, and it covers the wrapper pairs for a narrower reason: a wrapper that
stops forwarding is drift a schema comparison cannot see, and that has happened
once already. It is no longer in the gate;
[`../scripts/README.md`](../scripts/README.md) says why and where it runs
instead.

## 3. A destructive tool has one deletion, and it reads the state back

⛔ Applies to anything here that removes something on a machine. The compiled
`wsl-toolkit` has one deletion, `RemoveInside` in
[`../tools/windows/wsl-toolkit/internal/toolkit/paths.go`](../tools/windows/wsl-toolkit/internal/toolkit/paths.go).

**What it cost.** `WSL-04`. The predecessor printed that it had deleted a disk
beside a `Remove-Item -ErrorAction SilentlyContinue`, so multi-gigabyte VHDX
files left behind read as disks that had gone.

⚠ **A guard applied at four call sites is a guard that will one day be applied
at three.** The containment check runs inside the deletion helper rather than
beside each caller, and every removal of state reaches that helper.

⚠ **THE LINE IS AROUND STATE, NOT AROUND THE WORD "REMOVE", and it is drawn
where it is for a reason.** The rollback half of a write - `os.Rename` fails and
the same function removes the exact temporary it created two lines above - has
no caller-supplied path to contain, and the only root it could pass is the
file's own directory, which makes the containment check vacuous. A guard that
cannot refuse anything is theatre. `RemoveInside`'s own comment carries the
same sentence, which is where a reader of the code will look.

## 4. TWO files here are GENERATED, and the tree holds both halves

| generated file | source and regeneration | drift proof |
| --- | --- | --- |
| `tools/windows/wsl-toolkit/internal/toolkit/packages.sh` | the block between the two `shared package table` markers in [`../scripts/common/bootstrap.sh`](../scripts/common/bootstrap.sh); `sh scripts/common/check.sh package-table --fix` | the gate's `package-table` rule regenerates and compares it byte for byte |
| `tools/windows/wsl-toolkit/wsl-toolkit.1` | the native command registry and flag bindings; from `tools/windows/wsl-toolkit`, `go run . man --output wsl-toolkit.1` | `TestGeneratedManPageIsCurrent` regenerates and compares it byte for byte |

⚠ Nothing reads `packages.sh` yet: wiring the base provisioner to it is the rest
of `WSL-70`.

## 5. The record moves in the same change as the work

Specified in
[`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md), which
also carries the incident that produced it and the arithmetic hazard behind it.
The mechanics here:

```bash
node scripts/common/set-record.mjs status WSL-06 done
```

```bash
sh scripts/common/check-record.sh
```

⛔ **The writer does not grade itself.** `set-record.mjs` moves the numbers and
prints the reader's command; `check-record.sh` asserts independently and runs as
a gate.

## 6. An entry closes on a command, not on a paragraph

An entry is authored from [`ENTRY.md`](ENTRY.md) and closes in place, with the
acceptance command actually run and its real output pasted underneath.

⛔ **Nothing closes as "won't fix" or "somebody else's repository".** A blocked
entry stays open with the blocker named. `BSD-01` is the worked example: the
work left for `pkgforge-dev/docker-bsd` and [`bsd.md`](bsd.md) says where each
part went rather than pretending it finished here.

## 7. What a session owes at its end

Specified in
[`../docs/methodology/sessions.md`](../docs/methodology/sessions.md). What is
this repository's own:

- [`PROGRESS.md`](PROGRESS.md) rewritten, carrying the state line the record
  check parses;
- [`SUMMARY.md`](SUMMARY.md) overwritten with this session's table;
- the gate run, all three parts, and the entry closed with its evidence.

⚠ **`SUMMARY.md` is a snapshot and `PROGRESS.md` is the authority.** A session
that reads the first and acts on it is reading what was true last time.
