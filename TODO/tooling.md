# TODO: tooling

Entries for this repository's own checks and helpers.

[`INDEX.md`](INDEX.md) is the list; [`PROGRESS.md`](PROGRESS.md) is the order.

---

## DOC-01. A `binfmt_misc` check for the podman machine on WSL2

**Source** `Azathothas/TEMPLATE` issue 2, suggestion 1. ⚠ The issue proposed it
for `scripts/doctor/`; that placement is refused below and this is where it
landed instead.
**Category** tooling, **Priority** P2, **Effort** S, **Status** done

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
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

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
**Category** tooling, **Priority** P1, **Effort** M, **Status** done

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

---

## TOOL-03. `git-sync.ps1` bound a gate string to the author identity

**Source** ⭐ **Found by using it**, while bumping the `Azathothas/TEMPLATE` pin
at the end of the `WSL-06` to `WSL-11` batch. Not reported by anything.
**Category** tooling, **Priority** P0, **Effort** S, **Status** done

**Problem.** `git-sync.ps1` made a commit whose author and committer were
`sh scripts/common/check-control-bytes.sh <sh scripts/common/check-no-secrets.sh --public>`,
and then printed `identity verified: ... author and committer`. ⛔ **The one
script whose stated job is enforcing the identity rule invented an identity and
called it verified.**

**Premise.** ⭐ **Measured, by reproducing it.** The call was:

```powershell
pwsh -NoProfile -File scripts/common/git-sync.ps1 -Message "..." -BodyFile msg.txt -Gate "a","b","c","d"
```

⛔ **`-File` does not take PowerShell expressions.** The calling shell evaluates
`"a","b","c","d"` and hands the child **four separate command-line arguments**.
The child binds the first to `-Gate` and the remaining three **positionally**,
onto whichever parameters are still free **in declaration order**. `-Message`
and `-BodyFile` were already given by name in that call, so the three landed on
`-Name`, `-Email` and `-Branch`. One gate ran, two gates became a person, and
one became a branch.

⚠ **Which parameters absorb the overflow depends on which were named**, so the
damage moves with the call. Given `-Message` alone, the same four strings put
one in `-BodyFile` and the other two in `-Name` and `-Email`. That is the
argument against fixing this by reordering the parameter block.

The push is what failed, with `fatal: invalid refspec 'sh scripts/common/check-changelog.sh'`.
⚠ **The push failing is luck, not a guard.** Had the fourth string been absent,
the commit would have gone to the remote under a fabricated author.

⚠ **The identity check could not catch it.** It asserts that author equals
committer, which was true: both were the same wrong string. Nothing asserts
that an identity is a person, and nothing can.

**Approach.** `[CmdletBinding(PositionalBinding = $false)]` on the parameter
block. A stray positional argument then fails to bind and the script refuses to
run at all, which is the loud version of what happened silently.

⛔ Not a validation pattern on `-Name`. An identity is not a shape a regex
knows, and a rule that rejects unusual names would be wrong in the other
direction.

**Decision.** Turn positional binding off rather than reordering the parameters
so the harmless ones absorb the overflow. Reordering makes the misbinding land
somewhere less damaging, which is a way of surviving the bug rather than
removing it. ⚠ **`-Message` positionally is a loss**, and it is a small one: no
caller in this tree uses it, and every documented example names the parameter.

**Prove.** ⛔ Read the exit code from the process that produced it, unpiped.

```powershell
pwsh -NoProfile -File scripts/common/git-sync.ps1 -Message x -NoPush -Gate "a","b","c","d"
```

It refuses to bind, exits non-zero, and no commit is made.

⚠ **Not `-Check`.** `-Check` returns before the gates run and never reads
`-Name` or `-Email`, so it exits 0 whether the binding is right or wrong. See
the mis-verification below.

### Closed 2026-08-27

**What changed.** One attribute, `[CmdletBinding(PositionalBinding = $false)]`,
and a comment above it naming the incident.

⚠ **`git-sync.sh` does not share the defect and was checked rather than
assumed.** It reads `--gate` with a `case` in a `while` loop, so a bare argument
is an explicit error branch and there is no positional binding to abuse.

**Mutation proof.** ⛔ The defect was measured on the shipped code, which is the
strongest form: not a simulated revert, the real thing, in a real repository.

```text
2026-08-27T11:13:53Z git-sync: committed f5e8afa wsl-ephemeral: bump the ToolKit pin to ea5d483
2026-08-27T11:13:53Z git-sync: identity verified: sh scripts/common/check-control-bytes.sh
                               <sh scripts/common/check-no-secrets.sh --public>, author and committer
2026-08-27T11:13:53Z git-sync: pushing sh scripts/common/check-changelog.sh to origin
fatal: invalid refspec 'sh scripts/common/check-changelog.sh'
git-sync: git push failed
```

```text
author:    sh scripts/common/check-control-bytes.sh <sh scripts/common/check-no-secrets.sh --public>
committer: sh scripts/common/check-control-bytes.sh <sh scripts/common/check-no-secrets.sh --public>
```

⭐ **`identity verified` is printed one line under the fabricated identity.** That
is the sentence this entry exists to stop being printable.

**Acceptance, after the fix.** Every code read from the process that produced it.

A stand-in carrying `git-sync`'s exact parameter block, handed the four gate
strings that caused the incident:

| positional binding | what bound |
| --- | --- |
| ⛔ **on**, the shipped state | `Gate=[check-docs.sh]`, `Message=[subject]`, **`Name=[sh scripts/common/check-no-secrets.sh --public]`**, **`Email=[sh scripts/common/check-changelog.sh]`**, exit 0 |
| ⭐ **off**, the fix | `A positional parameter cannot be found that accepts argument 'sh scripts/common/check-control-bytes.sh'`, **exit 1** |

And the real script, with the same argv:

| call | exit |
| --- | --- |
| `-Message x -NoPush -Gate "a","b","c","d"` | ⭐ 1, refused at binding, nothing ran |
| `-Message "..." -BodyFile msg.txt -Gate "sh scripts/common/check-gate.sh --fast"` | 0, unchanged, and it is what every commit in this batch used |

### ⚠ The first verification of this fix was wrong, and how it was wrong matters

⛔ **`-Check -Gate "a","b","c","d"` exited 0 with the fix in place**, and was
briefly read as the fix not working. Two mistakes, both in this repository's own
rules:

1. ⛔ **The exit code was read through a pipe.** The call ended in
   `| Select-Object -First 4`, which can stop the upstream early and leave
   `$LASTEXITCODE` holding the previous command's value.
   [`../docs/conventions/shell.md`](../docs/conventions/shell.md) section 2, and
   the fifth absolute in [`../docs/AGENTS.md`](../docs/AGENTS.md).
2. ⛔ **`-Check` cannot show this defect at all.** It returns before the gates
   run and never reads `-Name` or `-Email`, so it exits 0 whether they were
   bound from a gate string or not. A test whose name claims more than it
   checks is its own row in
   [`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md).

⭐ Re-run unpiped, against a call that actually reaches the binding, the fix
refuses on both hosts. Written down because a fix verified by the wrong command
is indistinguishable from a fix that works, until it is not.

**What was done about the commit it made.** ⚠ It never reached a remote: the
push is what failed. It was reset with `git reset --soft HEAD~1` and remade,
and the pin bump that eventually landed in `Azathothas/TEMPLATE` as `83f573c`
carries the correct author. ⛔ **No history was rewritten anywhere published.**

**Consumers.** ⚠ `Azathothas/TEMPLATE` carries its own copy of this script and
therefore its own copy of this defect, because that is where this one came from.
⛔ **Not fixed there in this session**: that repository is read-only to this one
except for the pin, and a fix there is its own change with its own gate. It is
named here so the next session working in that tree has it written down.

---

## TOOL-04. Two rules the conventions state and nothing checked

**Source** Issue 4, which asks for the scripts copied from
`Azathothas/TEMPLATE` to be iterated on, and for that template's non-essential
scripts to move here. `check-markers` and `check-one-home` exist there and did
not exist here.
**Category** tooling, **Priority** P1, **Effort** M, **Status** done

**Problem.** [`../docs/conventions/prose.md`](../docs/conventions/prose.md)
states two rules a machine can hold and nothing held either: that the only
characters outside ASCII are the five it defines, used sparingly, and that every
fact lives in exactly one document. ⛔ `check-docs.sh` enforced the character
half over markdown alone, which is how every script in the tree went unchecked
for it.

**Premise.** ⭐ **Measured by running the two checks from a clone of
`Azathothas/TEMPLATE` against this tree, before importing either.** 167 marker
problems and 17 two-home sentences. Both numbers are in `DOC-02` and `DOC-03`,
which are the entries that cleared them.

**Approach.** Copy both pairs, adapt what names a file only that template has,
and wire them into the gate, into `check-twins.sh` and into CI on both hosts.

⛔ **The adaptation is the work, not the copy.** `check-one-home` exempts the
entry-point routers from each other and named three files, two of which do not
exist here; it now names `AGENTS.md` and `docs/AGENTS.md` and nothing else. Both
headers carried measurements taken in that template's tree and now carry this
one's beside them.

⚠ **`check-docs` loses the character rule in the same change.** Two checks
enforcing one rule is two places for it to be wrong, and they would have been
wrong differently: `check-docs` strips fenced blocks before it looks and a
whole-tree scan that did not would refuse the page naming the character it bans.

**Consumers.** None. New checks, and no existing interface moved.

**Prove.**

```bash
sh scripts/common/check-gate.sh --fast
```

Both names appear in the run and both pass.

---

### Closing

**Closed 2026-08-29T15:18:27Z.** Four files imported, adapted, and wired into
`check-gate.sh`, `check-gate.ps1`, `check-twins.sh` and both CI jobs. The gate
went from thirteen entries to fifteen.

⚠ **The two new pairs take `check-twins` from ten comparisons to twelve**, and
the `fill-license` row restored by `TOOL-07` makes thirteen. The 208-second
full-run figure both gate headers carried stopped describing this tree, so it
was re-taken rather than adjusted: 379s full, 270s for `check-twins` alone,
measured 2026-08-29.

---

## TOOL-05. `check-remote-items` reported red for an item that only needed reading

**Source** Found while running the gate against this tree on 2026-08-29, with
four issues open.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

**Problem.** Two defects in one file, and `Azathothas/TEMPLATE` had already
fixed both in its copy.

1. ⛔ **An unread item was treated as a failed check.** Any repository with an
   open issue was permanently red, which is how a check stops being read: the
   one state it cannot report is the state it exists for.
2. ⛔ **The two modes disagreed about the same tree.** Text exited 1 and
   `--json` exited 0, so a gate runner saw green where a person saw red. And
   `--json` printed the whole human report on stdout first, so piping it into a
   parser failed while every other check here was machine-readable.

**Premise.** ⭐ **Both reproduced here before the fix**, on this tree with its
four open issues:

```text
text mode  rc=1
json mode  rc=0
json stdout begins with the human report, so it does not parse
```

**Approach.** Take that template's version of both halves. ⛔ Not a rewrite: the
fix is one exit expression computed once and shared by both modes, plus a
file-descriptor swap at the top so the human report goes to stderr under
`--json`. Re-deriving it here would be a second implementation of a fix that
already exists.

**Consumers.** None. The script is not fetched by anything; it is run by
`.github/workflows/remote-items.yml` in this repository.

**Prove.**

```bash
sh scripts/common/check-remote-items.sh --json
```

Exit 0 with four issues open, and stdout parses as one JSON document.

---

### Closing

**Closed 2026-08-29T15:18:27Z.**

```text
{"schema":"check-remote-items/1","problems":0,"needs_human":1,"open_prs":0}
```

⭐ **All four combinations agree**: both halves, both modes, exit 0, and the
`.ps1` twin printed the same document.

⚠ **The weekly workflow was red for as long as an issue was open**, which is
every week since the first one was filed. Nothing was wrong with the tree it was
reporting on.

---

## TOOL-06. `check-gate.ps1` skipped six checks on the host it exists for

**Source** Found while wiring `TOOL-04`'s two new checks into both halves of the
gate runner.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

**Problem.** ⛔ **The PowerShell gate runner shelled out to the `.sh` half of
every twinned check, and skipped all six when no POSIX shell was found.** Its own
header says it earns a twin because a native PowerShell session may have no `sh`
at all, and [`../scripts/README.md`](../scripts/README.md) says to run the `.ps1`
half on Windows. It was the one place not doing either.

⚠ On a machine with Git Bash the defect is invisible: everything runs and
everything passes. It fires only on the machine the twins were written for, and
there it reports six skips and a green exit.

**Premise.** Read at `7127ff7`, in the branch taken when no shell is found: six
skip calls naming `check-docs`, `check-placeholders`, `check-control-bytes`,
`check-record`, `check-changelog` and `check-no-secrets`, each with the reason
`no POSIX shell on this host`, beside an else branch running the `.sh` half of
each.

⛔ **And the line-endings check was worse than skipped.** It sat inside the shell
branch and needs no shell of either kind, so on a host without one it was neither
run nor reported. The counts still added up and the name was simply absent.

**Approach.** One helper, `Invoke-PsCheck`, that runs a check's PowerShell twin
through this same host, found by path rather than by the name `pwsh` because a
5.1 caller must get 5.1 back. Every twinned check goes through it. The
line-endings check and the probe move out of the shell branch. What still needs
`sh` is what has no twin: `sh -n`, `shellcheck`, and `check-twins.sh` itself.

⭐ **This also makes the `check-gate` row in `check-twins.sh` worth more than it
was.** The two halves now run different implementations of every twinned check
to reach the same counts, so that one comparison exercises both.

**Consumers.** None. A gate runner is not fetched by anything.

**Prove.**

```bash
sh scripts/common/check-twins.sh
```

The `check-gate` pair agrees, which it can only do if both halves ran the same
number of checks and got the same answers.

---

### Closing

**Closed 2026-08-29T15:18:27Z.**

⚠ **The no-shell path is reasoned, not measured.** This machine has Git Bash and
removing it to prove the branch was not worth the disruption, so what was
verified is that every check now runs through its twin here and that the two gate
halves agree. ⭐ What would have had to be true for a measurement: a Windows
session with no `sh`, no `bash` and no Git for Windows on `PATH`, and nothing at
the two fallback paths `Get-PosixShell` probes.

---

## TOOL-07. The helpers `Azathothas/TEMPLATE` is dropping move here

**Source** Issue 4, and `Azathothas/TEMPLATE` issue 9, which proposes removing
its non-essential scripts and pointing at this repository instead. Ruled by the
operator on 2026-08-29.
**Category** tooling, **Priority** P2, **Effort** M, **Status** done

**Problem.** Two general-purpose helpers live only in a template repository,
where every project that starts from it gets a copy and none of the copies gets
a fix. This repository exists so a tool has one home.

**Premise.** Read in a clone of `Azathothas/TEMPLATE` at `6eaf4b5`. `deslop` is
an inventory of the files in a tree that address a reader as an agent, with an
apply mode that removes them; `fill-license` writes a `LICENSE` from one of
twelve SPDX texts and refuses four of them, because rewriting the notice in the
GPL family or in SPDX's ISC instance attributes the software to somebody else.

⛔ **`mine-repo` stays there**, on the operator's ruling in that repository's
issue 6: it encodes a methodology rather than a general job.

**Approach.** Copy both pairs and the `LICENSES/` texts `fill-license` reads,
adapt what names a file only that template has, and restore the `fill-license`
comparison in `check-twins.sh`, which had been removed along with the licences.

⛔ **Fix the defect both `deslop` halves carried while copying them.** They
removed files in a loop and then printed the count they had planned to remove:
an unconditional fallback in the sh half, and an unconditional count beside a
suppressed `Remove-Item` in the PowerShell one. Both now read the state back and
report what actually went, and exit 1 naming whatever survived. That is a row in
[`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md),
and it is the same shape `WSL-04` took out of this repository's other deleter.

⚠ **`deslop` is aimed at another tree.** Run here with its apply mode it removes
this repository's own router and methodology, which are content it wants. Both
headers say so now.

**Consumers.** None yet. ⭐ If `Azathothas/TEMPLATE` acts on its issue 9, it
becomes one, and that is a change in that repository.

**Prove.**

```bash
sh scripts/common/check-twins.sh
```

The eight fillable licences are byte-identical between the two implementations
and all four refusals hold in both.

---

### Closing

**Closed 2026-08-29T15:18:27Z.**

⭐ **`fill-license` reproduces this repository's own `LICENSE` byte for byte**
from `--id 0BSD --holder Azathothas`, which is a stronger check than the twin
comparison: it says the tool agrees with a file nobody generated with it.

⚠ **`GPL-3.0-only` exits 1 and writes nothing**, verified unpiped. A version
that stopped refusing would corrupt an attribution and exit 0 doing it.

---

## TOOL-08. The CI step that parses the workflows had never parsed one

**Source** Found on 2026-08-29 while validating a change to
`.github/workflows/ci.yml`, by running that step's own payload on this machine
before pushing it.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

**Problem.** ⛔ **The `yaml parses` step iterated zero files, printed nothing and
exited 0**, for as long as it had existed. Every workflow edit in this
repository's history went through a CI job that reported success over a file it
had not opened.

⚠ **The failure is the quiet one.** Nothing errored, the step was green, and its
output was an empty block that nobody scrolls to. A broken workflow would have
been caught by the workflow failing to run, which is a worse and later signal
than the check that existed to prevent it.

**Premise.** ⭐ **Reproduced on this machine on 2026-08-29** before anything was
changed:

```text
glob.glob('**/*.yml', recursive=True)  ->  []
pathlib.Path('.').rglob('*.yml')       ->  3 files, all under .github/
```

Python's `glob` does not descend into a directory whose name begins with a dot,
and every yaml file in this repository is under `.github/`. ⚠ The expression is
correct in a tree whose yaml sits anywhere else, which is why it survived review:
it is wrong about this tree specifically.

**Approach.** Enumerate with `git ls-files -z` rather than with `glob`, which is
what every check under `scripts/common/` already does and which cannot have this
class of blind spot. ⭐ **And assert the count before the verdict**, so the step
refuses an empty scope instead of reporting green over it. `check-one-home.sh`
carries the same rule for the same reason: a guard that cannot tell "nothing
wrong" from "nothing examined" is not a guard.

⚠ **`chr(0)` rather than a backslash escape** for the separator. The payload
crosses a shell on its way into `python3 -c`, and
[`../docs/conventions/shell.md`](../docs/conventions/shell.md) section 1 is the
measurement behind not writing one there.

**Consumers.** None. A workflow is not fetched by anything.

**Prove.**

```bash
uv run --with pyyaml python -c "import subprocess, sys, yaml"
```

⚠ That line only establishes the interpreter. The acceptance is the step's own
payload, run on this machine, reporting a non-zero file count and exit 0, and
the same payload with a pattern that matches nothing exiting non-zero.

---

### Closing

**Closed 2026-08-29T15:18:27Z.**

```text
ok   .github/dependabot.yml
ok   .github/workflows/ci.yml
ok   .github/workflows/remote-items.yml
3 file(s) parsed
rc=0
```

⭐ **The guard was mutation-proven**, which is what separates this from the
version it replaces:

```text
no yaml file in scope, so this step cannot report a pass
rc=1
```

⚠ **Three files where the step had been reporting on none.** The CI logs that
would show the empty output are past their retention, so what is recorded here
is the local reproduction of the expression rather than a log line.

---

## TOOL-09. `check-docs.ps1` collapsed `..` with a regex that matches `..`

**Source** found on 2026-08-30, when the two halves of the twin disagreed about a tree neither had seen before.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

---

## Problem

`check-docs.ps1` reported seven correct links as broken, and its `sh` twin
reported the same tree clean. Every one of the seven was in a file three
directories deep, which nothing in this repository was until this session put
the WSL tool under `scripts/windows/wsl-toolkit/`.

```text
scripts/windows/wsl-toolkit/wsl-toolkit.md:172 broken link -> ../../../docs/public/README.md
```

That file exists. A gate that reports a defect which is not there is worse than
one that misses a defect: it sends a reader after nothing, and the reader's only
way out is to distrust the check.

## Premise

⭐ **Measured by reading both halves, after the disagreement made it obvious
which one to doubt.** The `sh` half asks the filesystem, `[ -e "$dir/$target" ]`,
which is correct by construction. The PowerShell half hand-rolled a normaliser:

```text
while ($norm -match '[^/]+/\.\./') { $norm = $norm -replace '[^/]+/\.\./', '' }
```

⛔ **`[^/]+` matches `..` itself.** On
`scripts/windows/wsl-toolkit/../../../docs/x` the global replace consumes
`wsl-toolkit/../` and then, continuing from where it stopped, consumes `../../`
as one more segment-and-parent pair. The result is
`scripts/windows/docs/x`, which does not exist.

⚠ **It was invisible for as long as nothing in the tree was three deep**, which
is exactly the shape `scripts/README.md` already warns about for the twins: a
scope difference with nothing to exercise it is a difference nothing reports.

## Approach

Let the framework resolve the path. `[IO.Path]::GetFullPath` on the joined path,
tested with `Test-Path`, and the repo-relative form derived by trimming the root
prefix off the absolute answer. That is the same thing the `sh` half does, by the
same authority.

⛔ **Do not fix the regex.** A correct one is writable, with a negative
lookahead, and it would be a second implementation of path resolution living
beside a correct one that is already free.

## Consumers

None. This is a check, and no other repository fetches it.

## Prove

```bash
sh scripts/common/check-docs.sh --json
```

```bash
pwsh -NoProfile -File scripts/common/check-docs.ps1 -Json
```

Byte-identical JSON from both, which is what `check-twins.sh` compares, and a
planted three-deep broken link refused by both.

---

## Closing

**Closed 2026-08-30T09:10:00Z.** Both halves now agree exactly:

```text
{"schema":"check-docs/1","problems":0,"files":42,"links":460,"shell_blocks":120}
{"schema":"check-docs/1","problems":0,"files":42,"links":460,"shell_blocks":120}
```

⭐ **Mutation-proved, because a fix to a guard is a guard nobody has watched
refuse.** A broken link three levels up was planted and both halves named it:

```text
scripts/windows/wsl-toolkit/selftest.md:86 broken link -> ../../../docs/nope/nothing.md
ps exit=1
```

⚠ **What found this was the twin comparison, not a reading**, and neither half
was run against a three-deep file before this session created one. ⭐ The lesson
already in `scripts/README.md` is the right one and it now has a second incident:
prove a scope rule with a fixture rather than trusting the comparison to notice.
`docs/conventions/forbidden-patterns.md` carries the regex as a row.

---

## TOOL-10. `check-no-secrets.ps1` could not match a Windows home path at all

**Source** found on 2026-08-30 by the full gate, when the two halves disagreed about a build transcript pasted into an entry.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

---

## Problem

A build transcript pasted into `TODO/wsl-ephemeral.md` carried an absolute path
with the operator's username in it. This repository is public, and
`docs/public/README.md` names exactly that as a thing that must not be
published: it is not a credential, it is a map.

⛔ **The `sh` half caught it. The PowerShell twin reported the tree clean.** The
twin is the half that runs on the host that produces such paths, so the check
that keeps a username out of a public repository was blind on the only machine
that could put one there.

## Premise

⭐ **Measured by reading the two expressions side by side, after the gate's two
halves gave different answers on one tree.**

```text
sh   ([A-Za-z]:[\\/]Users[\\/]|/home/|/Users/)[A-Za-z0-9._-]+
ps   ([A-Za-z]:[\/]Users[\/]|/home/|/Users/)[A-Za-z0-9._-]+
```

⛔ **Inside a .NET character class `\/` is just `/`.** The backslash escapes a
character that was never special, so the class matched a forward slash alone and
a drive-letter path with backslash separators could not match. The `sh` half's
`[\\/]` is a two-character class and matches both.

⚠ **Nothing about the two lines looks different at a glance**, which is why this
survived: the twin was written from the sh half and the escape was dropped in
transcription.

## Approach

One character class, in the twin: `[\\/]` in both positions.

⛔ **Do not narrow the sh half to agree.** The sh half is right, and making two
implementations agree by breaking the correct one is the failure mode a twin
exists to prevent.

## Consumers

None. This is a check, and no other repository fetches it.

## Prove

```bash
sh scripts/common/check-no-secrets.sh --public --json
```

```bash
pwsh -NoProfile -File scripts/common/check-no-secrets.ps1 -Public -Json
```

The same `findings` count from both, and a planted Windows home path named by
both.

---

## Closing

**Closed 2026-08-30T09:35:00Z.** The twin's class is `[\\/]`, and the pasted
path was elided from the entry rather than left with the check narrowed around
it.

```text
{"schema":"check-no-secrets/1","findings":0,"public_rules":true,"history_scanned":false}
{"schema":"check-no-secrets/1","findings":0,"public_rules":true,"history_scanned":false}
```

⭐ **Mutation-proved on both halves**, because agreement on a clean tree is also
what two broken checks produce:

A drive-letter home path with backslash separators was appended to
`TODO/SUMMARY.md`, and **both halves named the same file and the same line
number**, which neither had done before the fix:

```text
=== sh half ===
TODO/SUMMARY.md:37:A planted path: (the path, not reproduced here)
=== ps twin ===
TODO/SUMMARY.md:37:A planted path: (the path, not reproduced here)
```

⚠ **The planted path is not reproduced above, and that is the check working on
its own record.** It was pasted verbatim first, and the next run of the fixed
check reported this entry as the finding. A mutation's evidence is the file and
the line it was found at; the string itself is the thing the rule exists to keep
out of a public tree.

⚠ **What found this was `--fast`, by not running.** The local gate skips
`check-twins`, and `check-no-secrets` passed its PowerShell twin all session. The
full run is what surfaced it, which is the argument for running the full gate
before a push rather than only `--fast`, exactly as `check-gate.sh`'s own header
says.

⛔ **Second twin divergence this session**, after `TOOL-09`. Both were in a check
rather than in the code being checked, both were invisible until a tree existed
that exercised them, and both were found by comparison rather than by reading.
