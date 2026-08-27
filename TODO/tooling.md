# TODO: tooling

Entries for this repository's own checks and helpers.

[`INDEX.md`](INDEX.md) is the list; [`PROGRESS.md`](PROGRESS.md) is the order.

---

## DOC-01. A `binfmt_misc` check for the podman machine on WSL2

**Source** `Azathothas/TEMPLATE` issue 2, suggestion 1. ⚠ The issue proposed it
for `scripts/doctor/`; that placement is refused below and this is where it
landed instead.
**Category** tooling · **Priority** P2 · **Effort** S · **Status** done

**Problem.** On Windows with a podman machine, cross-architecture containers
fail with `Exec format error` while every visible signal says the machine is
healthy.

**Premise.** ⭐ **Measured on 2026-08-27**, on the reporting machine, and the
diagnosis held. `/proc/sys/fs/binfmt_misc` carries a `binfmt_misc` instance
with a systemd autofs stacked on the same path; reading it returns `ELOOP`.
⛔ `systemd-binfmt.service` reports `status=0/SUCCESS` having registered
nothing. The machine measured here already has the reporter's fix installed:
`podman-binfmt-fix.service` is `enabled` and the qemu handler count is **31**,
the number the issue predicted.

⚠ One claim in the source issue was **not** reproduced and is not treated as
verified: the ten-hour kernel against seconds-old userspace. This machine had
been cold-started, so the two timestamps were 7 seconds apart. That
`podman machine stop` does not restart the WSL2 kernel is architectural and is
documented in `scripts/powershell-windows/wsl-ephemeral.md`; the specific skew
is unmeasured here.

**Approach.** A standalone script in `scripts/`, run on request, following the
contract in [`../scripts/README.md`](../scripts/README.md): a header naming the
defect, exit 0 pass, 1 fail, 2 could not run, a `--json` switch, and no
dependence on the directory it runs from.

It reports the handler count, names the `ELOOP` case specifically rather than
reporting a generic failure, and distinguishes "no podman machine" (exit 2, it
could not run) from "the machine is broken" (exit 1).

**Decision.** ⛔ **Refused for `scripts/doctor/`, which is where the issue asked
for it.**

The probe is read-only, spawns one process per tool, makes no network call
without `--net`, and is inherited by every project started from the template.
Reaching into the machine costs a `podman machine ssh`, which is slow, can hang,
and, ⭐ **measured on 2026-08-27, writes a 99-byte `NUL` file into the working
directory under Git Bash** because it passes `-o UserKnownHostsFile=NUL` to its
own ssh. A probe that litters the repository it is probing is a worse defect
than the one it detects. Most projects starting from that template will never
run a container.

⚠ The alternative considered and rejected: reading `/proc/sys/fs/binfmt_misc`
directly in `doctor.sh` when it runs on Linux. Cheap and honest, but it adds a
field to a schema whose two twins must agree, for an answer only one host can
give, to benefit a case this repository is the only known instance of.

**Prove.** Against a machine in the broken state, exit 1 and the message names
`ELOOP` and the stacked mount. Against this machine, exit 0 and the reported
count is 31.

```bash
podman machine ssh 'ls -1 /proc/sys/fs/binfmt_misc/ | grep -c "^qemu-"'
```

⚠ Run the check from a scratch directory, not from a repository, until it is
confirmed not to leave a `NUL` behind. The defect it exists to document is one
it can commit itself.

### Closed 2026-08-27

**What changed.** `scripts/common/check-binfmt.sh` and its PowerShell twin. It
lives in `common/` rather than `powershell-windows/` because the job exists on
both platforms: on Linux it reads `/proc/sys/fs/binfmt_misc` directly, on
Windows it reaches a WSL2 kernel, and it exits 2 where neither is available.

### ⭐ The `NUL` problem was designed out rather than tested for

The entry's approach assumed `podman machine ssh` and warned about running the
result from a scratch directory. ⭐ **It does not use `podman machine ssh` at
all**, and the reason came out of `WSL-03` in this same session: every WSL2
distro on a machine shares **one kernel**, so the handlers the podman machine
registered are readable from any distro with a plain `wsl -d DISTRO`. No ssh,
no key file, nothing written anywhere.

⚠ **It was still run from a scratch directory, because the entry said to.**
Nothing was written:

```bash
sh scripts/common/check-binfmt.sh
```

```text
check-binfmt
  read from      wsl:podman-machine-default
  kernel         7.2.0-WSL2-STABLE
  qemu handlers  31
  status file    present
```

⭐ **Exit 0 and the count is 31**, which is what the Prove asked for, and it
agrees with the number the source issue predicted.

### ⚠ It exited 2 over a healthy machine on the first run

⛔ Worth recording because the cause is already a rule in this tree and the
script still hit it. Git Bash rewrites any argument that looks like a POSIX path
into a Windows path before the target sees it, so `/bin/sh` reached the Linux
side as `C:/Program Files/Git/bin/sh` and the distro reported as unstartable.
[`../docs/conventions/shell.md`](../docs/conventions/shell.md) section 7 names
this and says to carry **both** `MSYS_NO_PATHCONV` and `MSYS2_ARG_CONV_EXCL`.
The sh half now does.

⚠ ⭐ **The PowerShell twin never had the defect**, because a native PowerShell
session does no path conversion. That is the argument for twins arriving in a
new place: not a missing tool this time, but a rewriting one.

**Both halves answer identically:**

```text
{"schema":"check-binfmt/1","source":"wsl:podman-machine-default","kernel":"7.2.0-WSL2-STABLE","handlers":31,"status_file":"present","stacked":0,"problem":""}
```

**Mutation proof.** `--require 99` against a machine with 31 exits **1** and
names both numbers; `--require 1` exits 0. ⚠ **The `ELOOP` branch is NOT
proven**: this machine already carries the reporter's fix, so the broken state
cannot be reached here without breaking the operator's podman machine, which is
not a thing to do for a test. ⛔ That branch is read, not measured, and this
sentence is the record of it.

⚠ **The default is a report, not an assertion.** Zero handlers exits 0 with a
loud note, because a machine that never wanted cross-architecture execution is
not broken. `--require N` is what turns it into a gate, per the contract note in
[`../scripts/README.md`](../scripts/README.md) about checks that measure an
open defect.

---

## TOOL-02. One command that runs the whole local gate

**Source** ⭐ **The operator, mid-session on 2026-08-27**, having watched this
session re-type the same nine checks before each of five commits.
⚠ **Authored and implemented in the same session**, which
[`../docs/methodology/authoring.md`](../docs/methodology/authoring.md) warns
against. The warning is about barrelling into code from an intake without
checking the premise; the premise here was measured in the session that asked
for it, and the instruction was direct. Recorded rather than glossed.
**Category** tooling · **Priority** P1 · **Effort** S · **Status** done

**Problem.** Part (a) of [`../docs/methodology/gate.md`](../docs/methodology/gate.md)
is a list of nine things. A list run by hand is run in the order somebody
recalls it, and the entry that gets missed is whichever was added last. This
session ran it five times and typed a different subset each time.

**Premise.** ⭐ Measured, in this session's own transcript: PSScriptAnalyzer was
invoked as a separate ad-hoc command before every commit, and `check-twins` was
run only twice in five.

**Approach.** `check-gate.sh` and `check-gate.ps1`, delegating to the checks
that already exist. `check-powershell.ps1` for the two PowerShell assertions CI
had inline. A twin is earned here by the rule in `check-twins.sh`: this is what
a session runs FIRST, before anything has established a POSIX shell is
reachable.

**Decision.** ⛔ **Not a second set of rules.** Every line delegates and reads
the delegate's own exit code. The alternative, a runner that re-implements the
checks, loses because it would be a second place for each rule to be wrong and
CI would still be the one that gates a push.

**Prove.**

```bash
sh scripts/common/check-gate.sh --fast
```

Exit 0 on a green tree, and exit 1 with the failing check named when something
is planted.

### Closed 2026-08-27

**What changed.** Three files: `check-gate.sh`, `check-gate.ps1` and
`check-powershell.ps1`. `check-twins.sh` gained the `check-gate` and
`check-binfmt` pairs, and CI's inline PSScriptAnalyzer step now calls
`check-powershell.ps1` and asserts the analyzer was not skipped.

⭐ **Both halves answer identically on this machine**, which is what
`check-twins` now holds:

```text
{"schema":"check-gate/1","total":13,"passed":12,"failed":0,"skipped":1}
```

**Measured, on this Windows 11 Pro 26200 machine, 2026-08-27:**

| run | elapsed |
| --- | --- |
| `check-gate.sh` full | 208s |
| `check-twins.sh` alone, inside it | 171s |
| `check-gate.sh --fast` | ⭐ 41s |

⚠ **`--fast` exists because of the first two rows.** A gate too slow to run
before each commit is a gate run once at the end, which is where the list got
retyped from memory in the first place. It skips `check-twins` and nothing
else, and reports it as a skip.

### ⛔ Three defects this found, two of them its own

**1. It hung for ten minutes.** `check-gate` runs `check-twins`, and adding
`check-gate` to `check-twins`' pair list made them call each other without
bound. It left twenty stray `sh` processes, and those held their own script
files open, so the next write to `check-twins.sh` failed with `EPERM` on the
atomic rename. ⭐ **Two rules in this tree predicted both halves**:
[`../docs/conventions/shell.md`](../docs/conventions/shell.md) section 9 on
unbounded commands, and section 7 on a running binary holding its own file.
A recursion guard now breaks the cycle at both ends.

**2. It reported a skipped analyzer as a passed check.** The first version
scored `check-powershell` as one entry. That check exits 0 whether the analyzer
ran or was absent, so with the analyzer mutated to look uninstalled the gate
printed `ok powershell (parse + analyzer)`. ⛔ **That is the exact row in
[`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md)
this file's own header cites**, committed by the file citing it. The parse and
the analyzer are now scored separately, off a fixed `analyzer=` status line
rather than off prose.

**3. `check-gate.ps1` shipped without a UTF-8 BOM** and PSScriptAnalyzer caught
it on the first run, as `PSUseBOMForUnicodeEncodedFile`. It holds non-ASCII, so
Windows PowerShell 5.1 would have decoded every marker as the system code page.
[`../docs/conventions/shell.md`](../docs/conventions/shell.md) section 8 names
this and the analyzer is the check that holds it.

**Mutation proof.** ⛔ Each planted, run, and read unpiped.

| planted | result |
| --- | --- |
| an unused parameter and a plural-noun function in a tracked `.ps1` | `check-powershell` exit 1, three findings named with rule, file and line |
| the analyzer module made to look absent | `SKIP  PSScriptAnalyzer -- not installed on this host`, gate exit 0, ⭐ **and the skip named in the summary line** |
| a status changed in `INDEX.md` alone | `check-record` exit 1, seven problems, both files named |

⚠ **What is NOT proven.** The gate has not been seen to fail on a real
`check-docs`, `check-no-secrets` or `line-endings` defect in this session,
because none was planted for those three. They are pass-through delegations to
checks with their own mutation proofs, and the delegation is what was tested.

---

## TOOL-01. A record checker, so the counts cannot disagree with the rows

**Source** [`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md),
which calls this the model's one mechanical hazard and says to automate it.
**Category** tooling · **Priority** P1 · **Effort** M · **Status** done

⚠ **Half done.** The reader exists as `scripts/common/check-record.sh`; the
writer does not, so the arithmetic is still manual and the reader is what
catches it. The entry stays open until both exist.

⭐ **That paragraph was true when it was written and is not now.** Both halves
exist. It is kept rather than edited because it is the reason the entry was
still open, and the closure below is what changed it.

**Problem.** Closing one entry moves several numbers: the index totals, the
priority table rows, and the record's own counts. Doing that arithmetic by hand
is how a published record says an entry is open beside an entry saying done.

**Premise.** ⭐ Measured: the reader was written this session and caught a real
disagreement on its first run, before any of it was committed.

**Approach.** Two scripts. ⚠ Only the reader is written.

- **The reader**, and ⭐ it runs as a gate, so a count that disagrees with the
  rows cannot reach a commit. It asserts that every entry has an index row and
  every row an entry, that no status disagrees between the two, and that the
  declared counts match the rows.
- **The writer**, which moves a status and re-derives every count. Not written.
  Until it exists the arithmetic is manual and the reader is what catches it.

**Decision.** Reader first, on purpose. The reader alone turns a silent
inconsistency into a failed gate, which is the whole of the documented damage.
The writer only saves typing, and a writer without a reader would be a second
thing to trust.

**Prove.**

```bash
sh scripts/common/check-record.sh
```

Exit 0 on a consistent tree. ⛔ Mutation-prove it: change one status in
`INDEX.md` without changing the entry, confirm exit 1 naming both files, then
put it back.

### Closed 2026-08-27

**What changed.** `scripts/common/set-record.mjs`, the writer this entry has
been open for. It moves a status in both places and re-derives every count from
the rows.

⭐ **It does not run the reader and report green.** A writer that grades its own
work is one bug away from hiding the bug, and this entry's own Decision says the
reader has to assert independently. It prints the command; `check-gate` runs it.

⚠ **Node, and no PowerShell twin**, for the reason `write-file.mjs` has none and
[`../scripts/README.md`](../scripts/README.md) now records: a twin here is a
second implementation of table arithmetic, in the one file whose entire job is
that the arithmetic is right.

**Acceptance, and it moved all seven numbers.** `WSL-06` was taken to `done`
and back purely as the test:

```text
  TODO/wsl-ephemeral.md: WSL-06 status -> done
  TODO/INDEX.md: WSL-06 status -> done
  TODO/INDEX.md: counts -> total 15  open 9  blocked 0  done 6
  TODO/INDEX.md: priority table, 2 row(s)
  TODO/PROGRESS.md: counts -> total 15  open 9  blocked 0  done 6
```

⭐ Then the **independent** reader, unpiped:

```bash
sh scripts/common/check-record.sh
```

```text
record ok: 15 entries (9 open, 0 blocked, 6 done), counts agree with rows
```

**Mutation proof**, exactly as the Prove above specifies. `WSL-07` set to
`done` in `INDEX.md` alone:

```text
record check failed, 7 problem(s):

  WSL-07: index says 'done', TODO/wsl-ephemeral.md says 'open'
  TODO/INDEX.md: declares open 10, rows say 9
  TODO/INDEX.md: declares done 5, rows say 6
  TODO/INDEX.md: P2 declares open 4, rows say 3
  TODO/INDEX.md: P2 declares done 1, rows say 2
  TODO/PROGRESS.md: declares open 10, rows say 9
  TODO/PROGRESS.md: declares done 5, rows say 6
```

⛔ Exit 1, both files named, and the mutation restored afterwards.

⚠ **The writer's refusals were tested and are not decoration:** an id with no
row exits 1 naming it, and a status outside `open|blocked|done` exits 1 listing
the three. Neither wrote anything.
