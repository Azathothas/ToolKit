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
| what it publishes | nothing. No image, no package, no release. | [`../README.md`](../README.md) |
| work model | todo | [`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md) |
| push policy | commit and push, to this remote only, on `main` | [`../docs/conventions/git.md`](../docs/conventions/git.md) section 2 |
| `main` | protected. One approving review, three required status checks, linear history. Force push and deletion refused. Admin bypass is on. | `gh api repos/Azathothas/ToolKit/branches/main/protection` |
| CI | three jobs, ubuntu and windows | [`../.github/workflows/ci.yml`](../.github/workflows/ci.yml) |
| the local gate | `sh scripts/common/check-gate.sh --fast`, or its `.ps1` twin | [`../scripts/README.md`](../scripts/README.md) |
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

**What it cost.** A copy of `wsl-ephemeral.ps1` was found in
`pkgforge-dev/cross-libc-dlopen` on 2026-08-27, during a reading of that
repository for an unrelated reason. It was not in the register, it fetches
nothing, and it carries both P0 defects this repository has since closed. The
register cannot reach it and neither can a fix.

⚠ **So the register is a lower bound on who is affected, never the complete
set**, and an entry that changes a fetched file says which consumers were
checked rather than assuming the answer.

## 2. Every check has two halves, and one machine runs both

⛔ **A POSIX `sh` check cannot be assumed to run on Windows**, which is the
default host here. [`../scripts/README.md`](../scripts/README.md) carries the
measurement and the exceptions.

**What it cost.** A native PowerShell session resolves `sort` to `Sort-Object`,
which accepts `-u`, compares case-insensitively, and returned two of four
distinct values without erroring. A missing tool announces itself; an aliased
one answers differently and reports success.

⚠ **A twin that is written and not compared is two behaviours.**
`check-twins.sh` runs both halves of every pair on one tree. Adding a twin
without adding its row there is how drift starts.

## 3. A destructive tool has one deletion, and it reads the state back

⛔ Applies to anything here that removes something on a machine, which today is
[`../scripts/powershell-windows/wsl-ephemeral.ps1`](../scripts/powershell-windows/wsl-ephemeral.ps1).
Its own page carries the four-part safety model.

**What it cost.** `WSL-04`. The predecessor printed that it had deleted a disk
beside a `Remove-Item -ErrorAction SilentlyContinue`, so multi-gigabyte VHDX
files left behind read as disks that had gone.

⚠ **A guard applied at four call sites is a guard that will one day be applied
at three.** The containment check runs inside the deletion helper rather than
beside each caller, and every path reaches that helper.

## 4. The record moves in the same change as the work

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

## 5. An entry closes on a command, not on a paragraph

An entry is authored from [`ENTRY.md`](ENTRY.md) and closes in place, with the
acceptance command actually run and its real output pasted underneath.

⛔ **Nothing closes as "won't fix" or "somebody else's repository".** A blocked
entry stays open with the blocker named. `BSD-01` is the worked example: the
work left for `pkgforge-dev/docker-bsd` and [`bsd.md`](bsd.md) says where each
part went rather than pretending it finished here.

## 6. What a session owes at its end

Specified in
[`../docs/methodology/sessions.md`](../docs/methodology/sessions.md). What is
this repository's own:

- [`PROGRESS.md`](PROGRESS.md) rewritten, carrying the state line the record
  check parses;
- [`SUMMARY.md`](SUMMARY.md) overwritten with this session's table;
- the gate run, all three parts, and the entry closed with its evidence.

⚠ **`SUMMARY.md` is a snapshot and `PROGRESS.md` is the authority.** A session
that reads the first and acts on it is reading what was true last time.
