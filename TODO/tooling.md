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

---

## TOOL-11. CI does not run Windows PowerShell 5.1, which is where every P0 has been

**Source** the operator, 2026-08-30. It was on the list this session put to them and they took it; the gap it names had been an open question for a week.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

---

## Problem

CI's Windows job runs PowerShell 7 only. Both P0 defects this tool has ever had
lived in Windows PowerShell 5.1, and neither was visible on 7:

- `WSL-12`, `-Action New` failing outright on 5.1;
- the double quote 5.1 drops when it builds a child process's argument list.

⚠ **This session had to check 5.1 by hand**, twice, and the last session carried
"the stream log has not been run under 5.1" as an open question for a week
because nothing automatic could answer it.

## Premise

⭐ **Measured on 2026-08-30**: the selftest passes under both hosts on this
machine, 117 cases each, and `windows-latest` ships `powershell.exe` as standard,
so the job needs nothing installed.

⚠ **The two hosts genuinely differ where this tool is sensitive.**
`-Action Doctor` measures the clock granularity and answers 100 ns on 7.6.5
against 513,600 ns on 5.1, on one machine, the same minute. A suite that runs on
one of them is a suite that has not been run.

## Approach

One step in the existing Windows job, beside the pwsh 7 one: the selftest under
`powershell.exe`, with the exit code read from the process.

⛔ **Beside, not instead.** Two hosts is the point; replacing one with the other
trades a blind spot for a different blind spot.

⚠ **Assert the case count there too**, as the pwsh step already does. A suite
that stopped early exits 0 over a smaller suite on either host.

## Consumers

None. This is CI.

## Prove

```bash
gh run list --repo Azathothas/ToolKit --workflow ci.yml --limit 1
```

A green run whose Windows job shows both selftest steps, and a red one when a
5.1-only defect is planted.

## Closing

**Closed 2026-09-10.** One step in the Windows job, running the selftest under
`pwsh` and under `powershell.exe` and comparing what the two report. From the
run on `0f711db`:

```text
pwsh 7: 131 case(s) over 36 function(s)
5.1   : 131 case(s) over 36 function(s)
```

⭐ **THE COUNTS ARE COMPARED, NOT TYPED.** A number written into the workflow
would be a number to update on every case added, and a suite that stopped early
exits 0 over a smaller suite on either host. Two hosts running one file must
reach one count.

And the planted half, in a copy of the tree with a PowerShell 7 ternary appended
to the bundle:

```text
--- pwsh 7 ---
{"schema":"wsl-toolkit-selftest/1","cases":131,"failed":0,"functions":36}
pwsh7-exit=0
--- Windows PowerShell 5.1 ---
selftest: ...\wsl-toolkit.ps1 does not parse; check-powershell.ps1 owns that verdict.
ps51-exit=1
```

⛔ **The old job was green over exactly that.** It ran `pwsh` alone, so a
construct 5.1 cannot parse reached a release without one red check.

⚠ **What this still does not cover** is a defect that needs a real distribution
to show itself. `windows-latest` has no WSL2, so the 5.1 evidence is over the
pure functions; `WSL-12`, the P0 this entry cites, was in a path only a real
`-Action New` reaches. That gap is the operator's host and it is named rather
than papered over.

---

## TOOL-12. nothing checks that a published release can be consumed

**Source** the operator, 2026-08-30, accepting it and asking for the weekly re-check as well.
**Category** tooling, **Priority** P2, **Effort** S, **Status** done

---

## Problem

`release.yml` verifies the tree, builds, tests and publishes. Nothing then checks
that the thing it published can be fetched and run. A release with a missing
asset, a wrong name in `SHA256SUMS` or an unrunnable file would be reported as a
successful publish.

⚠ **This session verified the first release by hand, once**, from an empty
directory holding only the launcher. That proves the path worked on 2026-08-30
and says nothing about the next one.

## Premise

⭐ **Measured, by doing it**: a bare `launcher.ps1` with
`-LauncherRelease wsl-toolkit-v1.0.0` resolved the release, downloaded both
assets, matched the published digest and ran `-Action Doctor`, exit 0.

⭐ **`-Action Doctor` needs no WSL for most of its rows**, which is what makes
this cheap on a runner: it reports what is absent rather than failing.

## Approach

Two things, and the second is the one the operator added.

1. A job in `release.yml` after publish: fetch with the launcher into a scratch
   directory, verify, run `-Action Doctor`, read the exit code.
2. A scheduled workflow that does the same against the latest release weekly, so
   a release that STOPS being fetchable is reported rather than discovered by a
   consumer. ⚠ The realistic causes are not a bad publish: a moved asset, a
   changed proxy allowlist, an API shape change. None of those is visible at
   publish time.

⛔ **The smoke must fetch over the network, not use the tree it just built.**
Using the local file would test everything except the thing that can break.

## Consumers

None directly. ⭐ It is the only automatic check that any consumer path works at
all, which is why it is worth more than its size.

## Prove

```bash
gh run list --repo Azathothas/ToolKit --workflow release.yml --limit 1
```

A publish whose smoke job is green, and a red one when the run is pointed at a
tag whose assets were removed.

## Closing

**Closed 2026-09-10.** Both halves, and the weekly one is the same definition
called on a schedule rather than a second copy.

`.github/workflows/release-smoke.yml` fetches a published release, verifies
every digest and drives the binary from a temp directory with no repository
present. `release.yml` calls it after `publish`; it also runs weekly against
whatever the latest release is then, because a release that STOPS being
fetchable is invisible at publish time.

Against `wsl-toolkit-v2.0.0`, from this host:

```text
consumer: 12 case(s) passed against wsl-toolkit-v2.0.0, 0 skipped.
CONSUMER_EXIT=0
```

and on a GitHub windows runner, where `wsl.exe` answers and no distribution can
be built:

```text
tag wsl-toolkit-v2.0.0: 12 case(s), 0 failed, 6 skipped
```

⭐ **PUTTING IT IN CI FOUND TWO DEFECTS IN IT, WHICH IS THE ARGUMENT FOR THE
ENTRY.** The suite had only ever run on a machine that can do everything, so
both were invisible.

1. **A property read on a missing field.** `Set-StrictMode -Version Latest`
   makes `$obj.absent` throw, including inside the `$null -ne $obj.absent` test
   written to tolerate it. A host with no WSL answers `doctor --json` without
   the field a host with WSL carries, so the case that allowed for that was the
   line that died on it.
2. ⛔ **A hand-rolled readiness probe, twice, and both were wrong.** It read
   `base status --json` and took anything but exit 2 as "jobs can run here".
   That is true on a machine with WSL and false on a runner with docker and
   none, so five job cases ran and every one FAILED over a host that was never
   going to work. ⚠ The second attempt read `ready --json` and gated on
   `route.wsl_callable`, which is a better question and still the wrong one:
   **wsl.exe IS callable on a GitHub windows runner** and a distribution still
   cannot be built there.

⭐ **The probe is `base ensure` now, which is the thing itself rather than a
signal that correlates with it.** It costs nothing extra because the first job
case had to run it anyway, and a host that cannot build a base skips the job
cases with the engine's own words attached. ⛔ A suite that fails where it
should skip is a suite whose red means nothing.

⚠ **What this still does not prove** is that a job runs on a runner, because a
GitHub windows runner cannot build a WSL2 distribution. Six of the twelve cases skip there and are counted as
skipped; the six that run are the ones a consumer meets first: the digests, that
the executable and the published script are one product, and that the tool
surveys a host from an empty state directory without building anything. The
other six are proved on the operator's machine, which is named rather than
implied.

---

## TOOL-13. The gate took thirteen minutes, and half of it was comparing two copies of every rule

**Source** the operator, on 2026-09-09, pointing at `Azathothas/pg-toolkit`,
which hit the same wall and answered it the same way.
**Category** tooling, **Priority** P1, **Effort** L, **Status** done

## Problem

`sh scripts/common/check-gate.sh` took about twelve minutes on this machine. A
session ran
`--fast` instead, which skipped `check-twins` and therefore skipped the one
check that compares the two implementations of every rule. So the expensive
half was never run by the people it was written for, and the cheap half was
running eighteen separate processes over the same tree.

## Premise

Measured on this host on 2026-09-09: the `--fast` gate, 6m20s. `check-twins`
alone, 5m35s, because it runs the whole gate twice and then every other pair.
The remaining time is per-check process startup and re-reading the same files:
`check-markers` spawns one `awk` per file, and every check calls `git ls-files`
again.

⚠ The twin requirement is not the defect and was not wrong. A POSIX check
cannot be assumed to run on Windows, which is the default host here, and
[`../scripts/README.md`](../scripts/README.md) carries the measurement that
proved it: a native PowerShell session resolves `sort` to `Sort-Object` and
returns two of four distinct values without erroring. The defect is that
answering it with a SECOND implementation makes a third check necessary.

## Approach

Copy `pg-toolkit`'s `cmd/check` and `internal/checks` verbatim, then adapt each
rule to the semantics this repository already enforces. One binary, one tree
walk shared by every check, running natively on either host.

⛔ **A port is not the place to tighten a rule.** Every ported check keeps this
repository's own semantics, not the ones it was copied from: integer marker
density, the placeholder rule's Go-template and Actions exclusions, one-home's
table-row and heading skips and its router exemption, and this repository's own
index, priority table and state line for the record.

⛔ **Every documented command keeps working.** `scripts/common/check-*.sh` and
their `.ps1` twins become wrappers around one named check, so nothing a page or
a session already says has to be rewritten to keep running.

## Consumers

None. Nothing under `scripts/common/` is fetched by URL;
[`../docs/consumers.md`](../docs/consumers.md) has no row for a check.

## Prove

The whole gate green in under a minute, the same rules, with each ported check
shown to still refuse what it existed to refuse. `check-twins` green over the
pairs that remain.

## Closing

**Closed 2026-09-09T16:05:00Z.** `tools/check` is 17 checks in one binary. The
gate is about 30 seconds, including shellcheck, PSScriptAnalyzer, a rebuild of
both generated products and the full Go suite.

```text
$ time sh scripts/common/check-gate.sh
  ok     docs
  ok     markers
  ok     record
  ok     one-home
  ok     control-bytes
  ok     placeholders
  ok     shell
  ok     removals
  ok     line-endings
  ok     size
  ok     changelog
  ok     secrets
  ok     shellcheck
  ok     powershell
  ok     bundle
  ok     go
  ok     commits

VERDICT: the tree agrees with itself.

real    0m31.292s
```

⭐ **Two rules were written down here and enforced nowhere.**
[`../docs/conventions/prose.md`](../docs/conventions/prose.md) bans a list of
adjectives that assert quality instead of demonstrating it, and no check had
ever read a file for them. There were two live uses, `bulletproof` in
`shell.md` describing a transport and `elegant` in a sweep describing a closed
route. Both are gone, and the rule now has something enforcing it. ⚠ A word
inside a code span is a specimen rather than a use, which is the same exemption
the character rule makes and the reason this paragraph can name them.

⛔ **Copying the tree loader verbatim dropped a scope this repository had
chosen.** `pg-toolkit`'s `Load` reads tracked files only; every check here scans
tracked PLUS untracked-but-not-ignored, because `git ls-files` alone cannot see
a file that has never been staged, which is exactly when a new file is most
likely to carry a credential. A planted AWS key went unreported until the second
list was put back, which is what a verbatim copy costs and why each rule was
then read against its own predecessor rather than trusted.

⛔ **A correction, and the title above keeps the number it was written with.**
"Thirteen minutes" was never measured. What was measured on this host on
2026-09-09 is the `--fast` gate at 6m20s and `check-twins` at 5m35s on its own,
which `--fast` skips; the full run is their sum, about twelve minutes, and no
complete run was ever timed end to end. The premise holds and the figure was
loose, which is the difference this note exists to record. Found by auditing the
re-orientation prompt written at the end of the same session.

⚠ **`check-twins` survives and left the gate.** The pairs that are genuinely two
implementations are the doctor probe, `git-sync`, `check-binfmt`,
`check-remote-items`, `deslop` and `fill-license`. Comparing them costs minutes
and catches drift that only arrives when somebody edits one half, so CI runs it
on every push and the gate does not.

---

## TOOL-14. The last six shell pairs become one program, and `check-twins` goes

**Source** The operator's priority order on 2026-09-09, and
[`TOOL-13`](#tool-13-the-gate-took-thirteen-minutes-and-half-of-it-was-comparing-two-copies-of-every-rule),
which removed eighteen of the pairs and named the six it left.
**Category** tooling, **Priority** P2, **Effort** L, **Status** done

## Problem

Six tools in this tree are still written twice, in sh and in PowerShell:
`scripts/doctor/doctor`, `git-sync`, `check-binfmt`, `check-remote-items`,
`deslop` and `fill-license`. That is about 1,870 lines of shell and about the
same again of PowerShell saying the same things, and `check-twins.sh` exists
only to run both halves of each pair and compare their answers. It is measured
at 5m35s and it is no longer in the gate, so nothing runs it on a schedule.

⚠ **The twin requirement was never wrong.** A native PowerShell session resolves
`sort` to `Sort-Object`, which accepts `-u`, compares case-insensitively, and
returned two of four distinct values without erroring. A POSIX check cannot be
assumed to run on the default host here. Answering that with a second
implementation is what makes a third check necessary.

## Premise

Measured: `wc -l` over the six pairs gives 646, 302, 212, 254, 220 and 236 lines
on the sh side. `check-twins.sh` is 398 lines and its own header names exactly
these six as what is left for it to compare.

⭐ **The doctor's inventory already has one home in Go.**
`internal/toolkit/ToolCatalog()` carries it, and
`TestNativeInventoryCoversStandaloneProbe` parses `scripts/doctor/doctor.ps1`
and asserts the two agree row for row. So the largest of the six is already
half ported, and the test proving it is the thing that would be deleted.

## Approach

One Go module at `tools/repo`, subcommand per tool, the same shape
[`../tools/check/`](../tools/check/) already has. Each `scripts/**` script
becomes a wrapper that builds the binary into `.tmp/` and execs it, exactly as
[`../scripts/common/check.sh`](../scripts/common/check.sh) does.

⛔ **Not a second gate.** `tools/check` holds the rules this repository enforces
over its own tree. These six are host probes, a commit path, a licence writer
and two remote readers; folding them into the gate would make `check-gate` do
things that are not checks.

⛔ **The behaviour is ported, not redesigned.** Each script's header records what
it cost to learn, and every one of those refusals moves across: `fill-license`'s
table rather than a regex, `check-binfmt` reading the kernel rather than a unit's
exit code, `git-sync` refusing an attribution line rather than stripping it,
`doctor` exiting 0 over a missing tool because it is a probe.

⚠ **`check-twins.sh` disappears when the last pair does**, and
[`RULES.md`](RULES.md) section 2 and
[`../scripts/README.md`](../scripts/README.md) move in the same change.

## Consumers

⭐ **`Azathothas/TEMPLATE` ships these scripts**, so a consumer fetching one raw
URL gets a shell script today. [`../docs/consumers.md`](../docs/consumers.md)
says which rows are affected and whether a wrapper keeps each contract; a
consumer that fetches `deslop.sh` and runs it with no Go toolchain is a break,
and the wrapper says so by name rather than failing at a missing binary.

## Prove

```bash
sh scripts/common/check-gate.sh
```

Green, plus each ported tool's own acceptance: the same command run through the
old script and the new wrapper answering identically on this machine, recorded
per tool as it lands.

## Closing

**Closed 2026-09-10.** Five of the six moved into
[`../tools/repo/`](../tools/repo/), one Go module with a subcommand each:
`deslop`, `license`, `binfmt`, `remote-items` and `git-sync`. Each script and
its PowerShell twin is a wrapper now, so the documented commands and the raw-URL
fetch both keep working.

Each port was checked against the implementation it replaces rather than against
a reading of it:

```text
deslop              8 agent-facing files and the same reference count from all
                    three implementations on this tree.
fill-license        byte-identical output for the nine licences it fills, the
                    same three refusals, the same exit codes, and ISC under
                    --force matching too.
binfmt              31 handlers, kernel 7.2.0-WSL2-STABLE, read through
                    wsl:podman-machine-default. ⚠ The shell half did not
                    finish inside a ten-minute budget on the same host; the Go
                    one answered in seconds.
remote-items        both report problems 0, needs_human 0, open_prs 0.
git-sync            --check produces the same three lines and exit 0.
```

`check-twins.sh` compares four of the five as wrapper pairs, which proves the
FORWARDING rather than the rule, and that is a class this repository has already
been bitten by: after the gate was ported, a `.ps1` wrapper passed `-Json` to a
binary that takes `--json`.

### ⛔ The premise was wrong about the sixth, and the correction is the entry

**What was believed:** six pairs, `check-twins.sh` disappearing when the last one
does, and `RULES.md` section 2 with `scripts/README.md` moving in that change.

**What was measured:** `scripts/doctor/` cannot be ported, and
`check-twins.sh`'s own header already said so before this entry was written.
Its rule is that a twin exists only where a single implementation cannot run,
and the probe earns one because it RUNS BEFORE YOU KNOW WHAT IS INSTALLED. A
wrapper that builds a Go binary cannot be the first thing a session runs to find
out whether a Go toolchain is there: the probe would have to build the answer to
one of its own questions.

**What that changes:** the doctor pair stays, `check-twins.sh` stays for it, and
`RULES.md` section 2 narrows rather than going. Five of six is the end state,
not five of six so far.

⭐ **The entry was authored from a count rather than from the rule**, and the
rule was already written down in the file the entry proposed deleting. Reading
`check-twins.sh` before writing the approach would have caught it; reading it
before writing the CODE is what did.

### What the ports changed on purpose, and where each is stated

- `deslop`'s structured answer gains the file list and moves to `deslop/2`. The
  human output always listed them and the structured one made a caller run it
  twice.
- `fill-license` runs its over-replacement guard BEFORE it writes. The shell
  version wrote the file and then checked it, so a corrupted licence existed on
  disk for the length of the check.
- `git-sync --check --json` puts the document on stdout and its progress on
  stderr. Both went to stdout, so piping it into a parser failed;
  `check-remote-items` had the same defect and its header records it, and fixing
  one and not the other would leave two answers to one question in one directory.
- `binfmt` drops the `MSYS_NO_PATHCONV` and `MSYS2_ARG_CONV_EXCL` workaround,
  because `exec.Command` passes its argument list to `CreateProcess` untouched.
  `WSL_UTF8` stays: without it `wsl.exe` emits UTF-16LE.
- `remote-items` drops `jq` and `curl`. The second matters beyond tidiness: the
  shell version fetched `action.yml` from `raw.githubusercontent.com` with no
  credential, so a private action or a rate-limited runner read as "runtime
  unverified" rather than as what it declares. `gh` already holds the token.
- `remote-items` resolves an annotated tag in two hops. A tag object is not a
  commit, and most released actions use annotated tags.

⚠ **`deslop`'s reference count was left as it was.** Matching base names as well
as full paths took it from 23 to 30 on this tree, which is arguably the better
answer and is not what the shell did. A port that quietly changes a number its
caller reads is a port nobody can check, so it is a decision to make on its own.

---

## TOOL-15. The rule that could only ever speak after the fact

**Source** The operator, on 2026-09-10, after a commit crediting a tool reached
a protected `main`: "despite all the rules and conventions and checks, why does
this keep happening".
**Category** tooling, **Priority** P1, **Effort** M, **Status** done

## Problem

It keeps happening because the instrument cannot fire in time.

`check commits` is the only rule in the gate whose subject is `git log` rather
than the tracked tree. Every session's procedure, written down in
[`RULES.md`](RULES.md) and followed here, is **run the gate, then commit**. At
the moment the gate runs, the commit being made DOES NOT EXIST. The rule reads
old commits, agrees with them, and reports green.

⛔ **It has never once prevented the defect it names.** It has reported it
afterwards, twice:

1. A session added a co-author trailer to eighteen commits. Recorded in the
   header of `tools/check/internal/checks/commits.go`, and the reason that file
   exists at all.
2. 2026-09-10, one commit, this session. The gate was run and was green, because
   the commit did not exist yet. CI caught it eleven minutes later, by which
   time it was pushed to a branch whose protection forbids a force push, and
   undoing it cost the operator's own intervention.

Both times the remedy was rewriting published history. That is the most
expensive remedy this repository has, and it was reached twice by a rule that
was never able to say no.

## Premise

Measured, not read:

- ⭐ **The gate is green over the tree at the moment the bad commit is
  made.** Reproduced deliberately: 17 checks, all ok, verdict "the tree agrees
  with itself", then a commit whose message carried the trailer. Nothing in the
  procedure was skipped.
- ⭐ **CI's checkout is shallow, so it reads exactly one commit.** `git log`
  in that checkout printed the tip alone, which is why CI reported four problems
  attributed to one hash rather than to the history.
- ⚠ **The counter-instruction is live and the rule is not.** A harness
  reminder asking for the trailer is re-asserted in context continuously; the
  repository's rule is read once, at orientation. That asymmetry is the mechanism
  and no amount of care changes it, which is why the fix has to be mechanical.

## Approach

Move the instrument to the moment the message is written.

`check commit-msg FILE` applies the same four rules to a message that is not a
commit yet. `.githooks/commit-msg` calls it with the file git hands it, and a
refusal writes nothing: git discards the message and the commit does not happen.
⛔ The hook CALLS the check rather than restating it, so there is one copy
of the rule and no drift.

A hook is not cloned, so a repository that merely ships one has a preference
again. The gate gains an eighteenth check, `hooks`, which refuses a tree with no
tracked hook and a checkout whose `core.hooksPath` does not resolve to it, and
quotes the one command that fixes it. CI installs the hooks the same way a laptop
does, in one line per gate job, so that rule has no "unless it is CI" branch.

⚠ **The executable bit is deliberately not checked.** git on Windows runs a
hook through its bundled sh whatever the mode says, so reading the working copy's
mode would report a problem that does not exist for half the people who run it.

## Consumers

⚠ **Every existing checkout of this repository will fail the gate until it
runs one command**, and the finding names it. That is the intended cost: a
checkout not running the hook is one where this rule is a preference.

```bash
git config core.hooksPath .githooks
```

## Prove

```bash
git config core.hooksPath .githooks
git commit -F a-message-that-credits-a-tool.txt
```

## Closing

**Closed 2026-09-10.** Driven, not read: the exact message that reached `main`
was written to a file and committed, and git refused it.

```text
$ git commit -F .tmp/try-msg.txt
  FAIL   pending (a message the hook must refuse) carries "co-authored-by: claude opus 5 <noreply@anthropic"; TODO/RULES.md: no tool is credited in a commit
  FAIL   pending (a message the hook must refuse) names "anthropic" in its message; TODO/RULES.md: no tool name in the body
  FAIL   pending (a message the hook must refuse) names "claude" in its message; TODO/RULES.md: no tool name in the body
  FAIL   pending (a message the hook must refuse) names "opus" in its message; TODO/RULES.md: no tool name in the body

commit-msg: 4 problems

The commit was refused and nothing was written.
```

`git log -1` was unchanged afterwards, which is the half that matters: the
refusal is not a warning printed beside a commit that happened anyway.

```text
$ sh scripts/common/repo.sh mutate
  ok       the commit-msg rule being applied at all                    6 case(s), went red
  ok       git's own comment lines being stripped first                6 case(s), went red
  ok       a message that could not be read being a refusal            1 case(s), went red
  ok       the hook path comparison                                    1 case(s), went red
  ok       a tree that ships no commit-msg hook being a finding        1 case(s), went red

44 of 44 guards proved.
```

⭐ **`tools/check` had ZERO tests before this.** `go test` over the gate ran
no cases at all, so the gate's own `go` check was vacuously green about it. Five
cases is not coverage of eighteen checks; it is the first five, and the hole is
named here rather than quietly filled.

⛔ **The mutation harness moved into the tree in the same session**, for a
related reason of its own. `TOOL-16` owns it.

⚠ **What this does not fix.** `git commit --no-verify` bypasses the hook,
and nothing in a local repository can stop that. The remaining defence is CI,
which is where both incidents were actually caught. The difference is that the
hook makes the bypass a DECISION rather than an accident, and the accident is
what happened twice.

---

## TOOL-16. Evidence that evaporates with the session

**Source** Found while closing `TOOL-15`: three records had just been written
citing a command that cannot be run.
**Category** tooling, **Priority** P2, **Effort** M, **Status** done

## Problem

The mutation harness was a Python script under `.tmp/`, which is gitignored.

`WSL-40`, `WSL-41` and `TOOL-15` each close with a pasted run of it, under the
heading that says what proved them. ⛔ **None of those commands could be run
by anyone reading the record afterwards**, including the next session in this
same repository. The output was real when it was pasted and unreproducible by
the time anybody read it.

⚠ **That is the same defect this repository keeps finding in its own
checks, one level up.** A rule with no instrument is a preference; an instrument
nobody else can run is a claim. The harness was proving 44 guards and was itself
the least durable thing in the tree.

## Premise

- ⭐ **The discipline is not one session's.** The harness was rebuilt from
  scratch in at least two sessions, each time to answer the same question, and
  each time the rows were retyped.
- ⭐ **It has caught real theatre twice.** A guard whose case never
  exercised it, and a ledger stress case that went red in 0 of 10 runs against
  the defect it was written for.
- ⛔ **Its own failure mode has bitten.** An earlier version reported "green
  with the guard gone" for three different things: a case that really did not
  cover its subject, a `-run` pattern matching NO test, and a mutation that did
  not COMPILE. Only the first is a finding.

## Approach

`repo mutate`, in the tool box that already holds what is not a gate check. The
table is `tools/repo/mutations.json`, generated from the script rather than
retyped, because 44 rows of Go fragments carrying tabs, newlines and quotes is
exactly the transcription nobody should do by hand.

⚠ **It is NOT a gate check, and that is the closest call in this entry.**
It proves the tests guarding this tree are real, which is a thing to run
deliberately when guards change. One pass copies every module and runs a suite
per row, and a gate somebody waits minutes for is a gate they skip.

The three outcomes stay three: `ok`, `THEATRE`, and `BROKEN` with the reason
named, because "did not compile", "matched 0 times" and "0 cases matched" are
different mistakes with different fixes.

## Consumers

Nothing published changes. The three records that cited `python .tmp/mutate.py`
now cite `sh scripts/common/repo.sh mutate`, which produces the same output
because it is the same table.

## Prove

```bash
sh scripts/common/repo.sh mutate
```

## Closing

**Closed 2026-09-10.** The port reproduces the script's answer exactly, over
both modules, with no row changed:

```text
$ sh scripts/common/repo.sh mutate
  ...
  ok       the harness refusing a table that would prove nothing       3 case(s), went red
  ok       the harness failing when a row is theatre                   4 case(s), went red

48 of 48 guards proved.
```

⭐ **44 rows came over unchanged and four are new**, and the four are the
harness's own guards, proved by the harness. That is not circular: a mutation is
applied to a COPY of the module and the case that fails is the one built from the
mutated source, so a harness that had stopped telling its three outcomes apart
would report those four rows green and be caught by them.

```text
$ sh scripts/common/repo.sh mutate --only leading-dot
  ok       the leading-dot refusal for an image id                     2 case(s), went red
  ok       the leading-dot refusal for a distribution name             1 case(s), went red

2 of 2 guards proved.
```

⭐ **The harness has cases of its own now**, and the one that matters builds
a throwaway module with one guard and asks for all five answers at once: a real
guard, a change no case looks at, a mutation that does not compile, a find that
is not there, and a `-run` pattern matching no test. Three of those are BROKEN
for three different reasons, and the case asserts the reasons are distinguishable
rather than merely that all three failed.

---

## TOOL-17. The suite that could not have caught any of them

**Source** Found by counting, on 2026-09-10: a consumer agent filed thirteen
defects against `wsl-toolkit-v1.3.0` on a day when this tree's own suite passed
39 acceptance cases, 129 Go cases and 48 proved mutations.
**Category** tooling, **Priority** P1, **Effort** L, **Status** done

## Problem

Thirteen defects, zero of them reachable by the suite that was green when they
were filed. That is the second time in two sessions, and the ratio is the finding
rather than any individual case.

The suite has four structural blind spots, and every one of the thirteen lands in
at least one of them.

1. **It never changes anything mid-flight.** Each case builds state, acts, and
   asserts. One case writes a `config.json`, in a fresh state home, BEFORE its
   first invocation; none mutates a config that a running process has already
   read. So the frozen config in
   [issue 17](https://github.com/Azathothas/ToolKit/issues/17) could not appear,
   and nothing removes the base under a live helper, so the cached failure in
   [issue 19](https://github.com/Azathothas/ToolKit/issues/19) could not either.
2. **It measures elapsed time for every case and asserts on it in none.**
   `Test-Case` records `seconds` at `acceptance.ps1:67` and puts it in the report.
   Nothing compares it to anything, so
   [issue 20](https://github.com/Azathothas/ToolKit/issues/20)'s twelve second
   wait for a two second deadline reads as a pass. ⭐ **The number is
   already there**, which makes this the cheapest of the four to fix: a case needs
   a way to declare an expected ceiling, not a way to measure.
3. **It matches substrings, never exact bytes.** Cases ask whether output
   CONTAINS a marker. The invented newline in
   [issue 23](https://github.com/Azathothas/ToolKit/issues/23) survives every one
   of them, and this repository INTRODUCED that byte in v1.2.0.
4. **It reads the process exit and rarely the object.** `--json` is exercised
   where the assertion needs a field, so a command that accepts `--json` and
   prints nothing, [issue 22](https://github.com/Azathothas/ToolKit/issues/22),
   passes by never being asked.

⛔ **A fifth blind spot is not the suite's shape but its subject.** Every
case runs a binary built from the working tree, against state the case just
created. The reporter ran the PUBLISHED artifact, from a fresh state directory,
as an outside process with no knowledge of the tree. Four defects last session
and thirteen this session came from that vantage point, and nothing in this
repository occupies it.

## Premise

Counted, not estimated. Thirteen issues, mapped one by one to the blind spot that
hid them, with two landing in more than one.

Two of the four claims were checked against the file on 2026-09-10 and both
needed narrowing, which is recorded above rather than quietly corrected: the
suite does time every case, and it does write one config. Neither ASSERTS the
thing the defect needed asserted, so both blind spots stand, but the first
drafting of this entry overstated them.

⚠ The other two claims, substring matching and unparsed JSON, are still a
reading. The honest version of this entry begins by taking three of the thirteen
and confirming a case really cannot be written for them in the suite's current
shape.

## Approach

Four capabilities the suite does not have, added as capabilities rather than as
thirteen cases:

- a case may mutate config, state or the base BETWEEN steps, and assert on what a
  long-lived helper does afterwards;
- a case may assert wall time, with the clock around the process rather than
  inside it;
- a case may assert an exact byte sequence and an exact count, not a substring;
- a case may assert on the parsed JSON object, and there is one shared assertion
  that every `--json` surface emits exactly one parsable object on stdout.

Then a fifth thing, which is a different program: a CONSUMER harness that fetches
a published release by tag, verifies its digests, runs it from an empty state
directory with no repository present, and asserts the manual's own claims. It is
the vantage point that found seventeen defects in two sessions.

⛔ **It must not become a second acceptance suite that drifts from the
first.** The consumer harness asserts the MANUAL's claims, which is a different
subject from the acceptance suite's, and if it starts asserting internals it has
become a copy.

## Order

⛔ **FIVE ENTRIES CANNOT BE PROVED UNTIL THIS ONE IS BUILT.** The `Prove`
sections of [WSL-44](wsl-toolkit-go.md), [WSL-45](wsl-toolkit-go.md),
[WSL-46](wsl-toolkit-go.md), [WSL-49](wsl-toolkit-go.md) and
[WSL-50](wsl-toolkit-go.md) each require one of the four capabilities named here:
mutation between steps, a wall-time ceiling, byte-exact comparison, or an
assertion on a parsed object. Writing those fixes first means closing them on
cases that do not exist, which is how the thirteen got shipped in the first
place.

That makes this entry a prerequisite rather than a follow-up, despite being the
one nobody asked for.

## Decision

**SETTLED 2026-09-10: not in CI. It runs after each release, from the release
checklist.**

Running it in CI means every commit depends on a published release and on a
network, and a red CI caused by a registry outage teaches people to ignore CI,
which costs more than the harness is worth. The release is the event this harness
is about, so the release is where it runs.

⚠ **The checklist is the instrument, and a checklist is a preference.**
This repository has now twice learned what an uninstrumented rule is worth, so
the honest version is that `release.ps1` runs the harness against the tag it just
pushed, once CI has published the assets, and refuses to report success until it
has. Whoever builds this decides whether that is one command or two.

## Consumers

None: this is a test surface and nothing fetches it.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

Passing means the four capabilities exist and are used by at least one case each,
demonstrated by taking three of the thirteen reported defects, writing a case for
each against the CURRENT binary, and watching all three fail. ⭐ A capability
that cannot reproduce a known defect is not a capability, and a case written
against an already fixed binary proves only that it passes.

## Closed 2026-09-10

FOUR capabilities, FOUR reproductions, and each was watched to fail against the
binary that had the defect BEFORE any fix was written. That run is the evidence,
because a case written after the fix proves only that it passes:

```text
  FAIL  every surface that advertises --json puts exactly one object on stdout
        expected: True
        actual  : base ensure: base ensure advertises --json and put nothing on stdout
  FAIL  a payload that writes no error output is reported as writing none
        expected: stderr=len=0 [] bytes=0
        actual  : stderr=len=1 [\n] bytes=1
  FAIL  a deadline bounds the caller and the reported duration is the one it waited
        wall time: 15.57s, and this case allows 10s
        expected: True
        actual  : it waited 15.56s and reported 4.57s
  FAIL  a helper resolves a catalog id against the config as it is now
        expected: True
        actual  : the fleet exited 2: wsl-toolkit: the helper refused: "midflight" is
                  not a catalog image. Available: alpine arch chimera debian debian12
                  fedora gentoo photon rocky8 ubuntu2204 void-musl wolfi

acceptance FAILED: 4 of 42 case(s).
```

The other 38 cases passed in that same run, which is the second half of the
demonstration: the capabilities did not break anything that already worked, and
none of those 38 could see any of the four defects.

| capability | where it lives | the defect it reaches |
| --- | --- | --- |
| a wall-time ceiling | `Test-Case -MaxSeconds`, and `Measure-Tool` puts the clock AROUND the process | [WSL-45](wsl-toolkit-go.md), issue 20 |
| a byte-exact expectation | `Show-Bytes` renders a string as `len=N [...]` with escapes, so an expectation is safe to write down | [WSL-46](wsl-toolkit-go.md), issue 23 |
| an assertion on the parsed object | `Read-ToolJson` THROWS on empty, unparsable or two documents | [WSL-46](wsl-toolkit-go.md), issue 22 |
| mutation between steps | `New-StateHome` and `Set-StateConfig` change state a running helper has already read | [WSL-44](wsl-toolkit-go.md), issue 17 |

⚠ **The wall-time capability had to be `Measure-Tool` and not the case
timer alone.** A duration the tool reports about itself cannot catch a tool that
returns late, because both numbers come from the same run and the one that lies
is the one being read. The case asserts the two clocks agree to within a second
AND that the wall time is under the ceiling, so neither can be satisfied alone.

**And the fifth thing, which is a different program.**
`tools/windows/wsl-toolkit/consumer.ps1` occupies the vantage point that found
seventeen defects in two sessions: it downloads a published release by tag,
verifies every digest in `SHA256SUMS` against the bytes it received, and runs the
binary from a temp state directory with a working directory that is not this
repository. Driven against the real `wsl-toolkit-v1.3.0` on 2026-09-10:

```text
consumer: Azathothas/ToolKit wsl-toolkit-v1.3.0
  ok    every digest in SHA256SUMS matches the file it names
  ok    the executable and the published script are the same product
  ok    the survey runs from an empty state directory and creates no distribution
  ok    the catalog is fully qualified, which is what the manual says it is
  ok    the state directory it names is the one it was told to use
  ok    the usage text names the commands the manual documents
  ok    a released binary runs a container job from an empty state directory
  ok    a container gets a copy of a workspace and never the host directory
  ok    a failing payload returns its own exit code
  ok    what a job writes to /out comes back to the directory named
  FAIL  a job past its deadline returns 124 and the caller is not held past it
        wall time: 15.28s, and this case allows 12s
  ok    gc --apply removes what this run made
```

⭐ **It found WSL-45 on its own, from outside**, which is the whole argument
for it existing: the same defect, measured independently of the acceptance
suite's version of the same case.

⛔ **IT NEARLY REMOVED THE OPERATOR'S OWN BASE, and that is worth recording
rather than quietly fixing.** A separate state directory is NOT isolation: the
distribution name comes from the configuration and defaults to `wsl-toolkit`
whatever `WSL_TOOLKIT_HOME` says, so the first draft would have adopted the real
base and unregistered it in its own teardown. The file now writes
`base.name = wsl-toolkit-consumer` before its first invocation.
[WSL-43](wsl-toolkit-go.md) is the entry that makes an instance a first-class
thing instead of a convention this file has to remember.

⚠ **It is red against `wsl-toolkit-v1.3.0` and that is correct.** It tests a
PUBLISHED artifact, and the published artifact has the defect [WSL-45](wsl-toolkit-go.md)
is open for. It goes green when a release carrying the fix exists, which is what
a post-release check is supposed to do.

---

## TOOL-18. The row of the counts that was typed

**Source** Found on 2026-09-10 while closing four entries: the index's `all` row
read `9 0 54 63` while the four rows above it summed to `20 0 58 78`, and the
gate had been green over that disagreement for at least a session.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

## Problem

[`INDEX.md`](INDEX.md)'s own header says the counts below it are checked and not
typed. One row of them was typed.

`check-record` reads the state line, both files' state lines, and the `P0` to
`P3` rows of the priority table. It never read the `**all**` row, so that row
could say anything at all and the gate stayed green. It did say something else:
every number in it was wrong, and it disagreed with the state line two lines
above it as well as with the rows below.

⛔ **It is this check's own defect class, in this check.** The incident
[`work-todo.md`](../docs/methodology/work-todo.md) records is a file declaring a
count nothing compared against another file, and this check exists because of it.

## Premise

Measured, not read. The wrong row was planted into a corrected index and the
check was run unpiped:

```text
  ok     record
check-record exit=0
```

## Approach

`tools/check/internal/checks/record.go` already has `priorityRow`, which trims
the bold markers, so it can read the `all` row as it stands. The loop over
`P0 P1 P2 P3` gains `all`, whose wanted values are the totals the check has
already derived from the rows.

⛔ **Not a second derivation.** The numbers compared against are the ones the
state-line check already computed, so the two cannot disagree about what the
rows say.

## Consumers

None: this is a check over this repository's own record and nothing fetches it.

## Prove

```bash
sh scripts/common/check-record.sh
```

Passing means the check goes RED with a wrong `all` row and green with a correct
one, demonstrated by planting one rather than by reading the code.

## Closing

**Closed 2026-09-10.** One line in the loop, and the guard was proved by
planting the exact row that was there:

```text
  FAIL   TODO/INDEX.md: all declares done 54, the rows say 62
  FAIL   TODO/INDEX.md: all declares open 9, the rows say 16
  FAIL   TODO/INDEX.md: all declares total 63, the rows say 78

record: 3 problems
exit-with-defect=1
```

and green once the row is right:

```text
  ok     record
exit-restored=0
```

⚠ **The blocked column was right by accident.** Both the typed row and the
rows said `0`, so one of the four numbers agreed and the other three did not.
That is worth noting because a check written to compare one column would have
passed too.

---

## TOOL-19. The instrument that proves the guards had stopped proving three of them

**Source** Found on 2026-09-10, running the full mutation table for the first time since the code under it moved. Two rows matched nothing and a third was reported as theatre against a case that had never run.
**Category** tooling, **Priority** P1, **Effort** M, **Status** done

---

## Problem

[`../tools/repo/mutations.json`](../tools/repo/mutations.json) is a table of
claims: delete this line, and the case named beside it goes red. `repo mutate`
proves them by copying the module, applying the change and running the case.
⛔ **Nothing runs it.** It is not in the gate, it is not in CI, and it takes
about ten minutes, so between two runs the table rots with nobody told.

Three of its rows were wrong, and each was wrong in a different way:

- **Two matched nothing.** `Extract`'s signature changed, so the row for the
  destination collision set no longer found its `find` string. The verdict rule
  moved out of `cmd_run.go` into `internal/toolkit/job.go`, so the row for the
  transfer branch no longer found its. Both rows still parsed, still read
  correctly, and had been proving nothing since the day the code moved.
- **One was reported as THEATRE and is not.**
  `TestAWorkspaceSymlinkPointingOutOfTheTreeIsLeftOutAndNamed` calls `t.Skip` on
  Windows, because creating a symbolic link there needs developer mode or an
  elevated process. ⚠ A skipped case prints `=== RUN` and leaves `go test`
  exiting 0, which is byte for byte what a case that stayed green looks like from
  outside the process. So the harness accused a good test of being empty.

⭐ **The second is this harness's own defect class, in this harness.** Its file
header says three outcomes and not two, because collapsing "the test is empty",
"the pattern matched nothing" and "it did not compile" into one answer is what
the previous harness did. It then collapsed a fourth.

## Premise

Measured, by running it. `repo mutate` over all 61 rows on this host:

```text
58 of 61 guards proved.
  THEATRE: a workspace entry that cannot travel is named rather than dropped: 1 case(s) ran and stayed green with the guard removed
  BROKEN:  the destination collision set: matched 0 times, not once
  BROKEN:  the transfer branch of the verdict: matched 0 times, not once
```

⚠ **The run cost about ten minutes**, which is the number that decides the shape
of the fix: whatever goes into the gate cannot be this.

## Approach

Three parts, and the split between the first two is the whole design.

1. ⭐ **A gate check, `check mutations`**, over the table as data: the module has
   a tracked `go.mod`, the file is tracked, the `find` string appears **exactly
   once**, the replacement is a real change, every alternative of the `run`
   pattern names a test function some `_test.go` in that module defines, and no
   two rows share a label. It reads files the tree walk has already read and adds
   no measurable time. ⛔ **It does not prove a guard and must not be read as
   doing so**: whether a case goes red is `repo mutate`'s answer.
2. **A fourth outcome in the harness**, `SKIPPED`, counted from `--- SKIP:`
   lines against `=== RUN` ones. ⛔ Not proved, so it is not counted as proved;
   not wrong, so it does not fail the run. ⚠ That is a deliberate amendment to
   the older rule that anything short of every row proved is a failure: a
   harness that went red on the operator's own host every time would be one
   nobody reads.
3. **A CI job on ubuntu** running the whole table, which is what makes part 2
   safe: nothing skips there, so a row this host cannot answer is answered.

⛔ **The two stale rows are repointed, not deleted.** A row removed because it
stopped matching is a guard quietly dropped from the set.

## Consumers

None. The mutation table, the gate and CI are this repository's own and nothing
fetches any of them.

## Prove

```bash
sh scripts/common/check-gate.sh
```

Passing means `check mutations` goes RED with a row whose code has moved and
green with it repointed, demonstrated by planting the exact row that was there
rather than by reading the code, and `repo mutate` reporting the skipped row as
SKIPPED rather than as theatre.

## Closing

**Closed 2026-09-10.** The check was proved by putting the real stale row back:

```text
  FAIL   tools/repo/mutations.json: "the destination collision set" matches tools/windows/wsl-toolkit/internal/toolkit/workspace.go 0 times, not once, so `repo mutate` cannot apply it

mutations: 1 problems
exit-with-the-stale-row=1
```

and green once it points at the line the code has now:

```text
  ok     mutations
exit-restored=0
```

The three rows, after the change:

```text
  ok       the destination collision set                                          1 case(s), went red
  ok       the transfer branch of the verdict                                     1 case(s), went red
  SKIPPED  a workspace entry that cannot travel is named rather than dropped      1 case(s), all skipped here
```

⭐ **The check found a fourth stale row while it was being written.** Renaming
`TestRunTellsTheThreeOutcomesApart` to `TestRunTellsTheFourOutcomesApart` and
`TestReportFailsUnlessEveryRowIsProved` to
`TestReportFailsOnEveryRowThatDisagrees` broke three rows that named them, and
the gate said so in under a second. That is the class this entry is about,
caught on the day it was created rather than a session later.

⚠ **What it costs, with its conditions.** On its own on this host: 556 ms,
560 ms and 390 ms over three runs, and most of that is loading the tracked file
list, which the gate has already loaded and shares. The whole gate measured 42s
over 19 checks here on 2026-09-10, against the 43s over 18 the previous session
recorded on the same machine.

⛔ **Those are two runs and not a controlled comparison**, so what they support
is that the difference is inside the noise. They do not support "the cost did
not move", which is what this paragraph said until the claim audit read it.

---

## TOOL-20. The check for the endings could never once have failed

**Source** Found on 2026-09-10 while writing a CRLF file for `WSL-25`, when the tree turned out to hold two answers about how a `.ps1` is stored and nothing had noticed.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

---

## Problem

`check line-endings` exists to catch one class:
[`../docs/conventions/shell.md`](../docs/conventions/shell.md) section 5 states
that a carriage return in a file `.gitattributes` says is LF is invisible to
`git diff` and visible to everything else. Its own header names the incident
that produced it, 182 files rewritten across four commits with nothing watching.

⛔ **It could not fail on any text file in the tree, and 23 of 54 tracked `.ps1`
files were sitting in the working tree with LF under an `eol=crlf` attribute
while it reported green.**

Two defects, and the first hid the second:

- ⛔ **The attribute column contains a space, and the parse split on
  whitespace.** `git ls-files --eol` writes a row as
  `i/lf  w/crlf  attr/text eol=crlf`, so `strings.Fields` produced four tokens
  and the loop that kept the one beginning `attr/` kept `text`. The half that
  says WHICH ending is wanted was dropped on every row, so every file resolved
  as "expected LF".
- ⛔ **It read the INDEX column, and git normalises a `text` file to LF in the
  index by definition.** `eol=` governs CHECKOUT, not storage. So even with the
  attribute parsed, `i/lf` against an expectation of `i/lf` is a tautology; the
  column that can disagree is the working tree, which is also the one every
  other tool reads.

## Premise

⭐ **Measured, by planting the defect the check is named for and reading the
exit code unpiped**, on 2026-09-10 on Windows 11 Pro 26200:

```text
$ python -c "...replace 5 LF with CRLF in docs/consumers.md..."
$ git ls-files --eol docs/consumers.md
i/lf    w/mixed attr/text eol=lf         docs/consumers.md
$ sh scripts/common/check.sh line-endings
  ok     line-endings
CHECK EXIT=0
```

⚠ **`w/mixed` is the loudest thing git can say about a file** and the check
reported `ok` over it.

## Approach

`parseEOLRow` takes the attribute as everything after `attr/` rather than as a
field, and `wantedWorktreeEnding` holds the LF-except-PowerShell rule in one
place so the check and its test cannot disagree about it. The comparison moves
to the `w/` column, and the `i/` column keeps an assertion of its own, because a
file committed with normalisation off is a different defect and the two are
worth telling apart.

⛔ **`w/none` is not a violation.** One line with no trailing newline reports it,
and there is nothing in such a file for an attribute to resolve.

⛔ **The 23 drifted files are renormalised from the index, not re-committed.**
The index was already right; only the working tree had drifted, which is exactly
why `git status` was clean and why nobody saw it. Deleting and checking them out
again is what makes this machine agree with a fresh clone.

## Consumers

None. No row of [`../docs/consumers.md`](../docs/consumers.md) fetches a file
this touches, and the tracked bytes of every published asset are unchanged:
`build.ps1 -Check` reproduces both products byte for byte after the
renormalisation.

## Prove

```bash
sh scripts/common/repo.sh mutate -only eol
```

Both new rows proved, and `sh scripts/common/check.sh line-endings` exiting 0
over a tree where every tracked file's working-tree ending agrees with its
attribute.

---

## Closing

**Closed 2026-09-10T11:42:00Z.** The parse keeps the whole attribute, the
comparison reads the working tree, 23 files were renormalised, and two mutation
rows now hold the two halves.

```text
$ sh scripts/common/repo.sh mutate -only "eol"
  ok       the eol attribute keeping the half that says which ending               6 case(s), went red

1 of 1 guards proved.
EXIT=0

$ sh scripts/common/repo.sh mutate -only "working-tree ending"
  ok       the working-tree ending following the attribute rather than a constant  1 case(s), went red

1 of 1 guards proved.
EXIT=0

$ sh scripts/common/check.sh line-endings
  ok     line-endings
line-endings EXIT=0

$ git ls-files --eol '*.ps1' | awk '{print $1, $2, $3, $4}' | sort | uniq -c
     53 i/lf w/crlf attr/text eol=crlf
      1 i/lf w/lf attr/text eol=lf
```

⚠ **What this says about the gate's other checks is the finding worth keeping.**
This one had a test file and no mutation row, and its two defects were the kind
a reading passes over: a `strings.Fields` on a column that happens to contain a
space, and a comparison against the wrong one of two columns whose names differ
by one letter. ⛔ **A check with no mutation row is a check nobody has seen
fail**, and 74 rows covered none of the eleven tree-walking checks' actual
comparisons before this one.

---

## TOOL-21. The sanctioned way to commit on Windows could not name two files

**Source** Found on 2026-09-10 while committing `TOOL-20`, by trying to pass six paths to the wrapper AGENTS.md section 5 names as the way to commit on this host.
**Category** tooling, **Priority** P2, **Effort** S, **Status** done

---

## Problem

[`../docs/AGENTS.md`](../docs/AGENTS.md) section 5 says: to commit and push, use
`git-sync.sh`, or `git-sync.ps1` on Windows. ⛔ **The `.ps1` half could not stage
a named set of files at all.**

- `-Path a -Path b` is refused: `parameter 'Path' is specified more than once`.
- `-Path a,b` binds the single string `a,b`, which is forwarded verbatim as one
  pathspec, and `git add` then reports a path that does not exist.

⚠ **The failure reads as the caller's mistake rather than the wrapper's.** The
message names a path, so a reader goes looking for a typo in a path that is
correct.

## Premise

⭐ **Measured on 2026-09-10 under PowerShell 7.6.5**, against a script declaring
`[string[]]$Path`, which is what this wrapper declares:

```text
through -File, -Path "a/one.md,b/two.md"   count=1   [0] = 'a/one.md,b/two.md'
through -File, -Path a -Path b             REFUSED: specified more than once
in-process, -Path @('a/one.md','b/two.md') count=2   [0] = 'a/one.md' [1] = 'b/two.md'
```

⚠ **This is a class this repository had already documented and paid for twice**,
in `WSL-22` and `WSL-24`, and
[`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md)
carries both rows. It was not applied to the tool the rule's own author commits
with, which is the shape worth naming: a rule written into a page about one file
does not travel to the next file by itself.

## Approach

`-Path` splits its own value on commas, and an empty element is refused by name
rather than forwarded. ⛔ **An empty pathspec means EVERYTHING to `git add --`**,
so a trailing comma would have staged the whole repository under a message
naming two files.

⛔ **`-Gate` is deliberately NOT split.** Its values are commands and a command
may contain a comma, which is the case `forbidden-patterns.md` calls "arbitrary
text has no safe delimiter either". Through `-File` it takes one gate, and
several go to `repo.ps1 git-sync --gate A --gate B`. Saying so is the fix for
that half; splitting it would have been a silent corruption.

## Consumers

None. No row of [`../docs/consumers.md`](../docs/consumers.md) fetches
`git-sync.ps1`; it is an internal helper.

## Prove

```bash
pwsh -NoProfile -File scripts/common/git-sync.ps1 -Message S -BodyFile B -Path "one,two"
```

Two files staged rather than one refusal, and `-Path "one,,two"` refused by name.

---

## Closing

**Closed 2026-09-10T11:58:00Z.** `-Path` splits on commas, an empty element is a
refusal, and `-Gate` says why it does not split.

```text
$ pwsh -NoProfile -File scripts/common/git-sync.ps1 -Path "TODO/INDEX.md,,TODO/tooling.md" -Message x -BodyFile /tmp/b.txt
git-sync.ps1: -Path has an empty element. It splits on commas, so 'TODO/INDEX.md,,TODO/tooling.md' is not a path.
EXIT=2

$ pwsh -NoProfile -File scripts/common/git-sync.ps1 -Message "..." -BodyFile ... -Path "tools/check/internal/checks/eol.go,tools/check/internal/checks/eol_test.go,tools/repo/mutations.json,TODO/tooling.md,TODO/INDEX.md,TODO/PROGRESS.md,scripts/common/git-sync.ps1"
2026-09-10T11:58:00Z git-sync: staged the named path(s)
2026-09-10T11:58:00Z git-sync: 7 file(s) staged
```

⚠ **The wrapper is what was wrong, and the rule it broke was already written
down.** Both `WSL-22` and `WSL-24` are the same defect in a different file, and
`forbidden-patterns.md` had the row before this wrapper was written. A rule
recorded against one file does not reach the next one on its own; the check that
would have caught this does not exist, and writing it means walking every
tracked `.ps1` for a list parameter, which is its own entry rather than a line
here.

---

## TOOL-22. the runtime column had never once produced an answer

**Source** found while re-deriving what pull request 15 asserts, 2026-09-10.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

---

## Problem

`check-remote-items` reports four things about an action pin. The third is the
one it exists for: what runtime the PINNED COMMIT declares, written because a
session pinned `actions/checkout` to a tag that targeted a deprecated Node and
resolving the tag had said nothing about it.

⛔ **That column had never produced an answer.** Every pin it has ever read
printed the same line:

```text
  ⚠     could not read action.yml at that commit; runtime unverified
```

⚠ **And it is a note, not a failure**, so the check stayed green while the one
question it was built to ask went unanswered.

## Premise

⭐ **Measured on 2026-09-10** against `actions/setup-go@b7ad1dad`, by running
the two commands rather than reading the code:

| command | answer |
| --- | --- |
| `gh api PATH -H 'Accept: ...raw' -f ref=SHA` | ⛔ HTTP 404 |
| `gh api PATH -H 'Accept: ...raw' -X GET -f ref=SHA` | the manifest |

⛔ **`gh api` picks its method from its arguments.** It is GET normally and POST
the moment any parameter is added, so `-f ref=SHA` turned reading a file at a
commit into a POST to the contents endpoint. `DeclaredRuntime` was correct and
tested; nothing ever handed it bytes.

⚠ **It is also the rule [`../docs/security/remote-ops.md`](../docs/security/remote-ops.md)
states as "treat an API call as read-only".** The helper could issue a write
because a caller added a parameter, which is not a thing a caller should have to
remember.

## Approach

`api` in `tools/repo/internal/remote/remote.go` pins `-X GET` before the path,
once, for every caller. ⛔ Not in `file` alone: the hazard is the helper, and a
fix at one call site is a fix that the next call site does not get.

The argv is split into `apiArgs`, so a case can assert the method without a
network. ⛔ A test needing `gh` and a token proves nothing on the machine where
this went wrong.

## Consumers

None. No row of [`../docs/consumers.md`](../docs/consumers.md) fetches this
tool; it reads this repository's own open items.

## Prove

```bash
sh scripts/common/check-remote-items.sh --repo pkgforge-dev/EasyEffects-AppImage
```

A `runtime:` line under each pin, and `go test ./internal/remote/` green with a
case that fails when `-X GET` is taken back out.

---

## Closing

**Closed 2026-09-10T12:35:00Z.** `-X GET` is pinned in `api`, the argv is
`apiArgs`, and two cases hold it.

⚠ **Driven against another repository's open items**, because merging pull
request 15 left this one with none, and a check with nothing to read cannot show
that it reads. Read-only, on a public repository:

```text
  #2 [app/renovate] Update actions/checkout action to v7
    actions/checkout@3d3c42e5aac5  (labelled v7.0.1)
      commit exists in actions/checkout
      label v7.0.1 matches the pin
      runtime: node24
      v7.0.1 is the latest release

  #1 [app/renovate] Update actions/checkout action to v6.1.0
    actions/checkout@d23441a48e51  (labelled v6.1.0)
      commit exists in actions/checkout
      label v6.1.0 matches the pin
      runtime: node24
  ⚠     v7.0.1 is already released; this proposes v6.1.0
```

⭐ **The guard was proved by planting the defect**, and both cases went red:

```text
--- FAIL: TestApiArgsAlwaysAsksForGET (0.00s)
    remote_test.go:160: apiArgs gave 6 arguments, want 8: ["api" "repos/o/r/contents/action.yml" ...]
FAIL
```

⚠ **The second case was weaker than its name**, and the mutation is what showed
it: it asserted the path at a fixed index, so it failed on the index shift
rather than on aliasing. It scans for the path now.

⛔ **What this did not fix.** The runtime line is still a note rather than a
failure when it genuinely cannot be read, which is right for a rate-limited or
private action and wrong for one that simply has no manifest. Telling those two
apart needs the HTTP status, which `gh api` does not hand back through this
helper, and that is its own entry rather than a line here.

---

## TOOL-23. The bundle rule rebuilt the products it was meant to compare

**Source** found on 2026-09-13 while checking the gate was safe to run over
uncommitted work, before running it. Fixed in the same session on the operator's
instruction to finish every task unattended.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

---

## Problem

[`RULES.md`](RULES.md) section 4 says the gate's `bundle` rule rebuilds from the
parts and compares both products byte for byte. ⛔ **It compared nothing.**
`tools/check/internal/checks/tools.go` ran `build.ps1 -Test`, and without `-Check`
that script writes both products first and then tests what it wrote. So the two
defects the rule exists for both passed, and took their evidence with them:

- a part edited and never rebuilt exited 0, with the tracked product rewritten;
- a product edited by hand exited 0, with the edit destroyed.

⚠ **Both CI push jobs run only the gate**, so neither could fail a push. Only
`release.yml` ran `-Check`, which is after the commit has been published.

## Premise

⭐ **Measured on 2026-09-13**, in copies of `scripts/windows/wsl-toolkit` and the
embedded product rather than in the tree:

| planted | `build.ps1 -Check` | `build.ps1 -Test`, what the gate ran |
| --- | --- | --- |
| a part edited, the product not rebuilt | exit 1 | exit 0, product rewritten |
| the product edited by hand | exit 1 | exit 0, edit destroyed |
| nothing | exit 0 | exit 0 |

## Approach

`Bundle` passes `-Check -Test`. With both, `build.ps1` compares in memory, writes
nothing, and runs its tests only when the comparison found no difference, so the
rule refuses a stale product and still proves the selftest, the surface lock, the
case-shadow scan and the analyzer. The flags are `bundleArgs`, so a case holds
`-Check` in them with no PowerShell. [`../scripts/README.md`](../scripts/README.md),
the former `scripts/windows/wsl-toolkit/README.md` and the former
`scripts/windows/wsl-toolkit/selftest.md` each named which flags run where, and
each was corrected.

⛔ **Not a second comparison written in Go.** `build.ps1 -Check` is the one
implementation of "the products match their parts", and the release workflow
already relies on it.

## Consumers

None. The rule reads this repository's own tree, and no row of
[`../docs/consumers.md`](../docs/consumers.md) runs it.

## Prove

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1
```

A green gate, `TestTheBundleRuleComparesRatherThanRebuilds` red when `-Check` is
taken out, and the rule refusing a planted stale part in a clone of the tree.

---

## Closing

**Closed 2026-09-13T03:33:20Z.** Driven in a local clone at `622f46a`, with one
part edited and never rebuilt, against the rule built from that commit and the
rule built from this change:

```text
unplanted          old -Test         ok bundle, exit 0
                   new -Check -Test  ok bundle, exit 0
planted            new -Check -Test  FAIL bundle: ! the tracked bundle disagrees
                                     with its parts at line 539, exit 1,
                                     product untouched
                   old -Test         ok bundle, exit 0, product rewritten
```

```text
  ok       the bundle rule comparing before it tests      1 case(s), went red
```

---

## TOOL-24. The gate printed a check that could not run as a pass

**Source** found on 2026-09-13 while reading `tools/check/main.go` for `TOOL-23`,
then measured. Fixed in the same session on the operator's instruction.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

---

## Problem

[`../scripts/README.md`](../scripts/README.md) promises that a check whose tool is
missing is reported with what was missing rather than counted as agreement.
⛔ **The gate counted it as agreement.** `gate()` printed `ok` for every check with
no problems and its JSON carried only problem counts, so a check's own
`skipped` reason never reached either output. `check NAME` printed the same `ok`.

## Premise

⭐ **Measured on 2026-09-13**, with the scoop shims taken off `PATH` for one
process so `shellcheck` and `go` were absent:

```text
check shellcheck --json   {"problems":0,"schema":"check-shellcheck/1","skipped":"shellcheck is not on PATH"}
check go --json           {"problems":0,"schema":"check-go/1","skipped":"no Go toolchain on PATH"}
check                     ok     shellcheck        ok     go
check --json              "go":0 ... "shellcheck":0, and no skip anywhere
```

## Approach

`skipReason` reads a result's own `skipped` field. `renderGate` and `report` take a
writer, print `skip NAME REASON`, and the verdict says how many of the checks ran.
The JSON gains a `skipped` object only when something was skipped. ⚠ **The exit
code does not change**: a host that cannot run a check is not a defect in the
tree, and the page that promises the report says so.

## Consumers

None. The `check-gate/1` document gains an optional field, which is additive, and
nothing in this tree or in [`../docs/consumers.md`](../docs/consumers.md) parses it.

## Prove

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1
```

A green gate, two cases red when either skip branch is disabled, and the gate
naming both skips on a host without the two tools.

---

## Closing

**Closed 2026-09-13T03:33:20Z.** Driven with the same two tools off `PATH`:

```text
  skip   shellcheck      shellcheck is not on PATH
  skip   go              no Go toolchain on PATH
VERDICT: the tree agrees with itself on 17 of 19 checks. 2 could not run on this host and say nothing about it.
gate exit=0
"skipped":{"go":"no Go toolchain on PATH","shellcheck":"shellcheck is not on PATH"}
  skip   shellcheck: shellcheck is not on PATH
```

```text
  ok       a gate skip printed as a skip                  1 case(s), went red
  ok       a single check skip printed as a skip          1 case(s), went red
```

---

## TOOL-25. The secrets rule could not see a Windows home path

**Source** found on 2026-09-13, when a hardcoded Windows home path in an example
passed a gate whose rule names that shape. Fixed in the same session on the
operator's instruction.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

---

## Problem

[`../docs/public/README.md`](../docs/public/README.md) section 1 forbids an
absolute home path carrying a username, and the gate's `secrets` rule is what holds
it. ⛔ **It could not match the way Windows writes one.** Its pattern spelled the
separator `[\/]`, which in a Go character class is an escaped forward slash and
nothing else. A forward-slash path was found and a backslash path passed, green.

⛔ **The tree had published four of them, with a real username**: one in
[`bsd.md`](bsd.md) and three in [`wsl-toolkit-go.md`](wsl-toolkit-go.md). The
uncommitted `WSL-69` work carried three more.

⚠ **This is `TOOL-10` written again.** That entry fixed the same class in
`check-no-secrets.ps1`; the Go program that replaced both shell halves spelled the
class the same way, and no mutation row covered the rule.

## Premise

⭐ **Measured on 2026-09-13** by running the pattern exactly as it was written:

```text
C:\Users\...\AppData\Local\Zellij\zellij.EXE, backslashes   match=""
C:/Users/.../AppData/Local/Zellij/zellij.EXE, slashes       match="C:/Users/..."
/home/user/.bashrc                                          match="/home/user"
```

(The names are elided, or replaced with a generic one, because this page is scanned
by the rule it describes. The run used a real username and a made-up one.)

## Approach

The separator is `[\\/]+`, which also reaches a JSON-escaped path, where every
backslash is doubled. A name must start with a letter, a digit or an underscore,
so an elided `...` is not taken for one. `genericHome` collapses the separators and
compares in lower case, because Windows compares names that way, and the allowlist
gains `runneradmin`, the GitHub runner account behind `RUNNER~1`. The four published
paths say `USER`, and a test fixture that was a made-up name says `user`.

⚠ **The history still carries the four paths.** A username is a fingerprint, not a
credential, so there is nothing to rotate, and a rewrite of a published history is
the operator's decision, per [`../docs/security/secrets.md`](../docs/security/secrets.md).
The row for `[\/]` in
[`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md)
now records that the port repeated it.

## Consumers

None. No row of [`../docs/consumers.md`](../docs/consumers.md) runs the gate.

## Prove

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1
```

A green gate, both new cases red on the old class, and the corrected rule finding
exactly the paths the old one missed.

---

## Closing

**Closed 2026-09-13T03:33:20Z.** The corrected rule, over the tree before the
redaction, found exactly the five it had been blind to, and nothing else:

```text
  FAIL   TODO/bsd.md:1034: an absolute home path
  FAIL   TODO/wsl-toolkit-go.md:2090: an absolute home path
  FAIL   TODO/wsl-toolkit-go.md:2091: an absolute home path
  FAIL   TODO/wsl-toolkit-go.md:3667: an absolute home path
  FAIL   tools/windows/wsl-toolkit/internal/toolkit/anchor_test.go:44: an absolute home path
secrets: 5 problems
```

After the four redactions and the fixture change, `check secrets` is `ok`. The
match text is left off the lines above for the same reason as in the premise.

```text
  ok       a Windows home path found with either separator      1 case(s), went red
```

## TOOL-26. The job that proves every guard cannot finish the table it proves

**Source** found on 2026-09-27 when CI cancelled the guard job on `c96873b`
after 30 minutes, the job's own limit, and on `a20b4d2` before it.
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

---

## Problem

The CI job that runs `repo mutate` over the whole table is cancelled by its
30-minute limit, so no push proves its guards and every run reads cancelled
rather than red or green.

## Premise

⛔ **Measured on 2026-09-27.** The table held 503 rows. The job started at
11:11:07Z and was cancelled at 11:41:23Z with no verdict. The same job took
19.7, 24.0 and 24.4 minutes over 419 rows, which is `09eea4f`'s measurement,
so the growth from issue 34's rows carried it past the limit.

## Approach

`repo mutate --shard K/N` proves the rows whose place in the table is K modulo
N, and the job becomes three runners, one share each. Every row runs exactly
once, and every module's rows spread over all three. A limit raised to fit was
the alternative, and it makes every push wait on one runner.

## Prove

Exit 0 on every share, in CI, on the commit that carries this change.

---

## Closing

**Closed 2026-09-27T15:26:02Z.** CI ran the three shares on `3290937`, run
`36326850193`. Every share exited 0, and the slowest took 12 minutes 17
seconds, where the single job was cancelled at 30 minutes 16 seconds:

```text
share 1/3   167 of 169 guards proved   11m11s   2 rows skip on Linux
share 2/3   167 of 168 guards proved   11m35s   1 row skips on Linux
share 3/3   168 of 168 guards proved   12m17s
```

The three shares hold 505 rows, which is the whole table, and 502 are proved.
The 3 that are not are rows whose every case skips itself on Linux.

⭐ **The first sharded run went red, and the harness was right.** On `5a667cc`,
run `36325961000`, share 2/3 stopped at 7 minutes 43 seconds on one row:

```text
  THEATRE  wsl-toolkit: a session stop signals the group in a form dash reads   1 case(s), still green
166 of 168 guards proved.
```

⛔ **`3290937`'s message gives the wrong cause.** It says that the dash on
Ubuntu reads `kill -TERM -- "-$pg"`. Measured after the push, with a process
group of two:

| shell | with `--` | without |
| --- | --- | --- |
| dash 0.5.12-12, `golang:1.25` | `Illegal number`, exit 2, 2 processes left | exit 0, 0 left |
| dash 0.5.12-6ubuntu5, `ubuntu:24.04` | `Illegal number`, exit 2, 2 left | exit 0, 0 left |
| dash 0.5.12-2, `debian:bookworm-slim` | `Illegal number`, exit 2, 2 left | exit 0, 0 left |
| busybox, `alpine` | `invalid number '--'`, exit 1, 0 left | exit 0, 0 left |
| bash, `golang:1.25` | exit 0, 0 left | exit 0, 0 left |

The row planted `--` in the TERM alone, and the KILL after the one-second grace
reached the group. So the session ended as stopped either way, and the case
could not see that the TERM had failed. `3290937` made the case read the
payload's background child, which proves the whole group is reached, and it
did not guard the TERM. The case now also reads a TERM trap in the payload, for
a stop and for a deadline, and a row plants `--` in each. The table holds 507
rows, and all three session rows are red in `golang:1.25`:

```text
  ok  wsl-toolkit: a session stop sends its TERM in a form dash reads                1 case(s), went red
  ok  wsl-toolkit: a session deadline sends its TERM in a form dash reads            1 case(s), went red
  ok  wsl-toolkit: a session stop reaches the payload's whole process group          1 case(s), went red
```

## TOOL-27. The Claude desktop app runs one account per user, and a second instance shares everything with the first

**Source** the operator, on 2026-10-08: a PowerShell script that runs separate
Claude desktop profiles from `~/.cc_profiles.json`, safe to run twice, sealed
from the shell that starts it, placed in `~/bin` and committed here. Authored and
built in the same session on that instruction, which is ruling 29.
**Category** tooling, **Priority** P2, **Effort** M, **Status** done

---

## Problem

The Claude desktop app keeps one account per Windows user. A second instance
started with `--user-data-dir` alone still reads Claude Code's `~/.claude` and
`~/.claude.json`, so two accounts mix their settings, memory and session
transcripts.

⛔ **Started from a terminal, the app also belongs to that terminal.** It
inherits the shell's `CLAUDE_*` and `ANTHROPIC_*` variables, prints its log into
the terminal, and ends when the terminal closes. The operator met the last two on
2026-10-10, with an instance started by the bare command this session had given
on 2026-10-08.

## Premise

⭐ **Measured on 2026-10-08**, on Claude 2.26454.2.0, the MSIX package:

- A launch with `--user-data-dir` wrote `Local State`, `logs` and
  `claude_desktop_config.json` into the folder it named, so the app's own data
  moves with the flag.
- The default instance keeps its data under the package's `LocalCache`, and a
  PowerShell outside the package does not see the app's `%APPDATA%\Claude` at
  all. A file the app wrote there was absent from the same path in `pwsh`. So a
  profile folder under `AppData` would be two different folders.
- A second instance logged `Global shortcut registration refused` for
  Ctrl+Alt+Space. `AppxManifest.xml` declares the `claude` protocol, the
  `CoworkVMService` service and the `ClaudeStartup` task once, for every instance.

⚠ **Read, not measured:** the app passes `CLAUDE_CONFIG_DIR` from its own
environment to Claude Code. That is from `app.asar`, which reads
`process.env.CLAUDE_CONFIG_DIR` and falls back to `~/.claude`.

⭐ **Measured on 2026-10-10**, on the operator's own instance: a probe that
attaches to a process's console found it attached to a Windows Terminal tab,
with the tab's `pwsh` and three of the app's own processes. Its log recorded a
component killed with exit code `-1073741510`, `STATUS_CONTROL_C_EXIT`, when an
earlier tab closed. `Claude.exe` contains the string `ELECTRON_NO_ATTACH_CONSOLE`,
the switch Electron reads before it attaches to its parent's console.

## Approach

`scripts/cc_profile/cc_profile.ps1`, one file, with
[`../scripts/cc_profile/README.md`](../scripts/cc_profile/README.md) as its whole
reference and `scripts/cc_profile/tests/cc_profile.tests.ps1` as its suite.

- **The configuration** is hand-editable JSON, and every write goes through one
  door, `Save-ConfigText`, always under a named lock: validation, a refusal of a
  newer schema, a refusal when the file changed since it was read, a backup, an
  atomic replace and a read-back.
- **The launch** builds its environment from the registry, sets
  `CLAUDE_CONFIG_DIR` and `ELECTRON_NO_ATTACH_CONSOLE=1`, and starts the app
  through `cmd /c start` with breakaway, so it has no console and no live parent,
  and no job where the caller's job allows breakaway.
- **Every report on a running instance reads its console attachment**, so an
  instance that a terminal can end is named rather than reported as fine, and
  one whose console cannot be read is named too.

⛔ It does not delete a profile's data, edit `PATH`, touch `~/.claude`, or change
the PowerShell profile unless asked with `-AddToProfile`.

## Decision

**The app's parent is a `cmd.exe` that exits at once**, by default, and
`launch.via = direct` skips that hop. ⚠ The alternative was to name
`explorer.exe` as the parent with `PROC_THREAD_ATTRIBUTE_PARENT_PROCESS`. It
lost because that is the shape endpoint protection reports as parent process
spoofing.

**A launch that cannot detach refuses.** The first version fell back to
`Process.Start`, which hands the app this terminal's console and handles, and
that is the failure the tool exists to prevent.

## Consumers

None. No row of [`../docs/consumers.md`](../docs/consumers.md) fetches anything
under `scripts/cc_profile/`, and the operator's `~/bin` copy is written by the
tool's own `install`.

## Prove

```bash
pwsh -NoProfile -File scripts/cc_profile/tests/cc_profile.tests.ps1 -Live -Mutate
```

Exit 0, with every case passed and every mutation row proved for the reason it
names. ⚠ `-Live` starts the real app, so it needs the app installed and a
desktop to draw on.

---

## Closing

**Closed 2026-10-10T08:22:19Z**, the instant the prove command's last run ended,
exit 0, after 12 minutes 41 seconds. `cc_profile` is in `~/bin`, and `cc_profile`
resolves by name in a new PowerShell and in `cmd.exe`.

```text
  ok    live-launch-survives-a-tree-kill-and-ends-with-the-app (57.8s)
  ok    live-a-kill-on-close-job-that-allows-breakaway-does-not-end-the-app (34.5s)
  ok    live-a-job-that-forbids-breakaway-is-reported (20s)
  ok    live-closing-the-terminal-leaves-the-app-running-and-silent (41.6s)
  ok    live-a-direct-launch-also-survives-closing-the-terminal (38.5s)
  ok    live-an-app-started-from-a-terminal-is-named-and-dies-with-it (41.4s)
cases=67 passed=67 failed=0 skipped=0 mutations=53 proved=53 theatre=0 misfired=0 broken=0 unrun=0
```

⭐ **The operator's question, answered by the running app.** The live tree-kill
case runs the script in-process in a shell, the way an alias does, and the
launch reads its own claims back. Its terminal printed:

```text
Launched 't-live' with Claude 2.26454.2.0.
Verified: pid 1471912 is running with its data in ~/Downloads/ToolKit/.tmp/cc_profile-tests/20261010T080938Z-30952f/live-launch-survives-a-tree-kill-and-ends-with-the-app-cb71/profiles/t-live.
Detached: it has no console and its parent has exited, so closing or killing this terminal does not end it. Quitting the app does.
```

The case then ran `taskkill /T /F` on that terminal, and the app was still
running 3 seconds later. Ending the app ended all its processes, and nothing
started it again in 5 seconds. Two more cases closed a pseudo-console terminal
around a launch, through `cmd` and directly: the app stayed off the console,
printed nothing into it, and outlived the closed tab.

⭐ **The premise, measured both ways.** An app started from a shell with
`ELECTRON_NO_ATTACH_CONSOLE` removed attached to that shell's pseudo-console, the
suite's own probe found it there, and closing the tab ended it. `doctor` failed on
it, `status` showed it ATTACHED, and focusing it warned. Started by `cc_profile`,
the same app had no console.

⛔ **Three reviews before this close found 39 defects and gaps**, and all 39 are
fixed in this change, 37 of them with a case. The two without are the lock, now
keyed on the file a write lands on, which only a symlinked configuration reaches,
and `#Requires -Version 7.5`, which PowerShell enforces before a line runs. The door sweep found 9, among them a purge that
moved the tool's own data folder and one `-Force` that also lifted the refusal to
launch elevated, which is why `-AllowElevated` exists. The guard-strength review
found 14 in the suite. The correctness review found 16, among them a profile block
that stopped the operator's whole PowerShell profile loading.
[`PROGRESS.md`](PROGRESS.md) has the list under "Review findings".

⚠ **The suite's first runs found 5 more, 1 in the script and 4 in the suite, and
all are fixed.** Under `-WhatIf`, the Appx module's first import printed 45 "What
if" lines. Turning off the preference in the function, in the script and globally
did not stop them, and `Set-Variable` honours `-WhatIf` itself. The package query
now runs in a runspace of its own, which prints none; in a scratch script it took
357 ms. In the suite, 2 mutation rows went red for another reason: each case
crashed on a later line before its own assertion. The harness counts that as
misfired rather than proved, and both cases now assert first. A case that counted
every profile `discover` registered also counted the operator's running instance.
And a log an ended app still held kept the run's folder once, so the removal now
retries.

⚠ **What is not measured.** Whether the app outlives a job that forbids
breakaway: the launch says so in `launch.log`, and the suite asserts only that.
A real Windows Terminal window: the suite closes a pseudo-console built the way
Windows Terminal builds one. An elevated instance probed from a normal shell,
which the door sweep suspected answers `unknown` and which `doctor` now reports.

## TOOL-28. A carriage return inside a line passes the control-bytes rule

**Source** found on 2026-10-21 while reading the muse adapter's probe for
[issue 37](https://github.com/Azathothas/ToolKit/issues/37). Filed and fixed in
the session ruling 30 authorised.
**Category** tooling, **Priority** P2, **Effort** S, **Status** done

---

## Problem

`tools/windows/wsl-toolkit/adapters/muse/probe.sh` line 58 reads `tr -d ''` on
screen and in a diff. Between the quotes is a literal carriage return, so the
line does what it means and no reader can see what that is. Its generated copy
under `internal/toolkit/adapters/` carries the same byte, and the gate is green.

## Premise

⭐ **Measured on 2026-10-21**: `git ls-files -z | xargs -0 grep -lP
'\r(?!\n)'` names exactly those two files, and `od -c` shows `'` `\r` `'` at
that line. Read: `ControlBytes` in
[`../tools/check/internal/checks/bytes.go`](../tools/check/internal/checks/bytes.go)
skips every `\r`, because a CRLF file is legitimate here, so it cannot tell a
line ending from a carriage return in the middle of a line.

## Approach

`ControlBytes` refuses a carriage return that no line feed follows, and the
probe writes `'\r'`, which `tr` reads as the same byte. The three agent probes
also write `\n` in place of a line break inside a `printf` quote.

## Consumers

None: the probe's behaviour does not change, and the rule is internal.

## Prove

```bash
sh scripts/common/check.sh control-bytes
```

Exit 0 after the fix, and exit 1 naming the probe's line with the byte put back.

---

## Closing

**Closed 2026-10-21T03:26:51Z.** `ControlBytes` refuses a carriage return that no
line feed follows, at the line it is on. The muse probe writes `'\r'`, and the three
agent probes write `\n` where a `printf` quote held a line break.

```text
sh scripts/common/check.sh control-bytes      exit 0
  ok     control-bytes
the byte put back into adapters/muse/probe.sh:
sh scripts/common/check.sh control-bytes      exit 1
  FAIL   tools/windows/wsl-toolkit/adapters/muse/probe.sh:94: a carriage return inside a line at offset 4073, which a reader cannot see; write it as an escape
```

1 mutation row, red. ⚠ **`gofmt` from Go 1.27 rewrote two single quotes in a doc
comment into a typographic quote**, so the case's comment says what the byte did in
words instead.

## TOOL-29. `PROGRESS.md` grew to 2,776 lines and no check fired

**Source** the operator, 2026-10-21: "why & how did PROGRESS.md get this large, it violates [work-todo.md]"
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

---

## Problem

The model rewrites `PROGRESS.md` every session and gives it no history. Sessions
appended to it from 2026-09-15 on, until it held 116 findings, every review, every
session's measurements and host changes, and 30 rulings. `SUMMARY.md`, which is
overwritten, held four sessions.

## Premise

Measured on 2026-10-21 from git: 93 to 233 lines up to 2026-09-14, 468 on
09-15, 1,128 on 09-17, 2,506 on 09-27, 2,776 at `9784821`, over 162 commits. The record
check read the counts line and the work order, and nothing else of the file.

## Approach

Two rules in `tools/check/internal/checks/record.go`: the record carries the
model's sections and nothing else, under a ceiling of 200 lines, and the summary carries
one session.

## Consumers

None: the gate is this repository's own.

## Prove

```bash
sh scripts/common/check.sh record
```

Exit 0 on the rewritten record, and each of the five new rows red.

---

## Closing

**Closed 2026-10-21T09:46:45Z.** Rules 8 and 9 of the record check: `PROGRESS.md` carries the
State, Baseline, This session, In progress, The work order and Open questions sections
and nothing else, under 200 lines, and `SUMMARY.md` carries one session. A heading in a
code fence is not a section.

```text
sh scripts/common/check.sh record, on the record as it stood     exit 1
  FAIL   TODO/PROGRESS.md is 2776 lines and the ceiling is 200
  FAIL   8 sections the model does not have, "Found, and not filed" among them
  FAIL   TODO/SUMMARY.md carries 4 sessions
record: 10 problems
sh scripts/common/check.sh record, on the rewritten record       exit 0
  ok     record
repo mutate --only 'record: '                                     10 of 10 guards proved
```

5 mutation rows, 5 of 5 red, and a case that runs the whole check so the rule is
proved to be called.


## TOOL-30. The docs check's cited-path rule never ran, and it carried another project's rules

**Source** found on 2026-10-21 while syncing the methodology
**Category** tooling, **Priority** P1, **Effort** S, **Status** done

---

## Problem

`docs/conventions/prose.md` says every cited path exists. The block that checks a
path named in backticks was empty, so a page could cite a file that moved a month ago.
The check also carried another project's tool families, experiment counts, archive
accounting and methodology provenance, none of which applies here, and four numbered
rules that were comments with no code.

## Premise

Read on 2026-10-21: `if pathsBind {}` in `docs.go`, and `citedPaths`,
`citedCommits`, `methodologyLinks` and `countDefects` have no caller. `git log -S`
finds no commit that ever called them: the port to one binary on 2026-09-09 arrived this
way. Armed against the tree, the rule reported 74 problems.

## Approach

Wire the rule for live pages, read a cited line and check the file has it, and
leave out the files that carry history: the changelog, the entries and the reference
sweeps. Delete what never ran. Fix the live pages it finds.

## Consumers

None: the gate is this repository's own.

## Prove

```bash
sh scripts/common/check.sh docs
```

Exit 0, with a case per kind of citation and the rows red.

---

## Closing

**Closed 2026-10-21T09:46:45Z.** The cited-path rule runs for every live page: a path in
backticks headed by one of this tree's directories is tracked, and a `path:N` or
`path:N-M` names a line the file has. The changelog, the entries and the reference
sweeps carry history and are left out, as `docs/HISTORY/` already was. What never ran
is deleted: another project's tool families, experiment and POC numbering and counts,
archive accounting, methodology provenance, an empty loop, and four numbered rules that
were comments. The shell parser's name is reported, as its comment always said.

```text
the rule armed, before any page was fixed                        docs: 74 problems
  history the rule now leaves alone: the changelog 13, the entries 43,
  the reference sweeps 11; the old record 2; the resume file 1
  live pages naming a path that does not exist on purpose: 4, reworded
sh scripts/common/check.sh docs                                   exit 0
go test ./internal/checks/ -run TestACitedPathAndLineMustExist    14 cases, ok
repo mutate --only 'docs: '                                       9 of 9 guards proved
```

7 mutation rows, 7 of 7 red. ⚠ One came back BROKEN first, because removing the call
left `topLevel` unused; it now disables the rule's condition instead.


## TOOL-31. The gate's documented time is 30 seconds, and it takes minutes

**Source** found while measuring the old record's baseline, 2026-10-21
**Category** tooling, **Priority** P3, **Effort** S, **Status** done

---

## Problem

`check-gate.sh` and its twin say the full run is about 30 seconds, and
`docs/AGENTS.md` says under a minute. The measured runs took 131 to 245 seconds.

## Premise

Read on 2026-10-21; the times are in the old record's measurements.

## Approach

The scripts drop the number, and the router points at the measurement in `PROGRESS.md`.

## Consumers

None.

## Prove

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1
```

Exit 0.

---

## Closing

**Closed 2026-10-21T09:46:45Z.** `check-gate.sh` and its twin no longer state a time, and
`docs/AGENTS.md` says a few minutes and points at the measurement in `PROGRESS.md`.

```text
pwsh -NoProfile -File scripts/common/check-gate.ps1     exit 0, 24 of 24, in 654 s
```

⚠ **The 654 s is a loaded machine**, with seven builders compiling beside it, so it is
the reason not to type a number rather than a new number to type.


## TOOL-32. A workflow's own shell has no case

**Source** the old record's finding 33, and its review findings of 2026-09-17
**Category** tooling, **Priority** P2, **Effort** M, **Status** done

---

## Problem

`release.yml` stages, counts and signs the assets, `herdr-nightly.yml` prunes old
prereleases, and the publish reads back what it uploaded. All of it is shell inside YAML,
with no case and no mutation row, and a defect in it has shipped once.

## Premise

Read at `9784821`: `release.yml:165-185` and :249, `herdr-nightly.yml:183-197`.

## Approach

Move that logic into a script with cases and rows, and have the workflows call
it.

## Consumers

None outside the workflows.

## Prove

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1
```

Exit 0, with each new row red.

---

## Closing

**Closed 2026-10-25, merged from `rb/f7-tooling`, for the next release.** The release and nightly shell moved to Go with cases: `release-stage` takes its compiler as a parameter so its staging, `SHA256SUMS` and `GITHUB_ENV` line have a case, and every build is trimmed and CGO-free. The nightly's typed counts moved to `repo nightly-stage`, which writes `BUILD-INFO.json` in jq's shape and `SIGN_COUNT`; the sign step checks it. Built, reviewed twice, repaired once; its rows red on Windows and in golang:1.25; gate 24 of 24 on the merge.

⚠ No workflow has run on a GitHub runner yet: the first real runs are the next release and the next herdr nightly.


## TOOL-33. A Go case that passes only on Windows can reach `main`, because the Linux checks are run by hand

**Source** the old record's finding 42, met twice, and again on 2026-10-21
**Category** tooling, **Priority** P2, **Effort** S, **Status** done

---

## Problem

CI runs the Go tests and the mutation table on Linux. Locally they run on Windows,
and the container runs that copy CI are a step a session can skip. Three cases have
reached `main` green here and red on Linux.

## Premise

Read at `9784821`: `.githooks` holds `commit-msg` alone.

## Approach

A `pre-push` hook runs CI's Linux Go job and ShellCheck in containers through
the installed `wsl-toolkit`, and refuses the push when either fails or cannot run.

## Consumers

None.

## Prove

```bash
git push --dry-run
```

The hook runs both containers and its exit decides the push.

---

## Closing

**Closed 2026-10-25, merged from `rb/f7-tooling`, for the next release.** The pre-push hook runs CI's Linux jobs itself for a tree with no proof: ShellCheck in `ubuntu:24.04`, `check-go.sh` in `golang:1.25`, then every mutation row the push adds. A failure refuses the push; a pass files a proof keyed by the tree, so the next push of that tree is immediate. Each job checks a `git archive` of the commit and compares the copy's tree with the proved one. A Linux host proves natively. Driven with `git push --dry-run` against a throwaway bare remote: a passing tree, a cached tree and a ShellCheck failure.

⚠ Not additive for a developer: every push of a new tree runs the containers, about 94 seconds, and needs Go, `wsl-toolkit` and a working default base. Without them the push exits 2.


## TOOL-34. A PowerShell runner that cannot run can still exit 1

**Source** the old record's finding 34
**Category** tooling, **Priority** P2, **Effort** S, **Status** done

---

## Problem

Under `$ErrorActionPreference = 'Stop'`, `Write-Error` before `exit 2` ends the
script with 1, which means a check failed. `herdr-remote-probe.ps1` still does it, and a
throw during setup in `acceptance.ps1` or `consumer.ps1` ends with 1 as well.

## Premise

Read at `9784821`: `herdr-remote-probe.ps1:397-399`, `acceptance.ps1:262` and :274, `consumer.ps1:370-405`.

## Approach

Each runner exits 2 when it cannot run, a setup throw included, and a
forbidden-patterns row names the trap.

## Consumers

None.

## Prove

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1
```

Exit 0, and each runner's cannot-run path exits 2.

---

## Closing

**Closed 2026-10-21T09:46:45Z.** `herdr-remote-probe.ps1` calls `Exit-Cannot` where it wrote an
error before `exit 2`. `acceptance.ps1` and `consumer.ps1` trap a throw during setup and
turn it into `Exit-Cannot`, and let a throw after setup through unchanged. A
forbidden-patterns row names the trap.

```text
the trap cut from acceptance.ps1, driven in a child pwsh
a throw during setup   exit 2 want 2 ok
a throw after setup    exit 1 want 1 ok
sh scripts/common/check.sh powershell                    exit 0
```

⚠ The first edit put a marker character in two files that were ASCII, and the analyzer
refused both for a missing BOM; the comments are ASCII again.


## TOOL-35. The gate's own list says the analyzer covers `scripts/` alone

**Source** the old record's finding 82
**Category** tooling, **Priority** P3, **Effort** S, **Status** done

---

## Problem

The analyzer reads every tracked `.ps1`, and two strings still say it reads `scripts/` only.

## Premise

Read at `9784821`: `tools/check/main.go:54` and `tools.go:54`.

## Approach

Both strings say what the analyzer reads.

## Consumers

None.

## Prove

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1
```

Exit 0.

---

## Closing

**Closed 2026-10-21T09:46:45Z.** `tools/check/main.go` and `tools.go` say the analyzer reads
every tracked `.ps1`.

```text
sh scripts/common/check.sh powershell                    exit 0
```


## TOOL-36. A changelog stamp later than the clock passes the gate

**Source** the old record's finding 89: five stamps were typed rather than read, and two were in the future
**Category** tooling, **Priority** P3, **Effort** S, **Status** done

---

## Problem

The changelog check reads each stamp's shape and order, never whether it is in the future.

## Premise

Read at `9784821`: no check calls `time.Now`.

## Approach

The changelog check refuses a stamp later than the clock, with a small allowance for skew.

## Consumers

None.

## Prove

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1
```

Exit 0, and a planted future stamp refused.

---

## Closing

**Closed 2026-10-21T09:46:45Z.** Rule 5 of the changelog check refuses a heading stamped later
than the clock, with ten minutes for a clock that runs fast, and a date alone only
after its day. `CHANGELOG.md`'s header lists the fifth rule.

```text
go test ./internal/checks/ -run TestAChangelogStampLaterThanTheClockIsRefused   5 cases, ok
sh scripts/common/check.sh changelog                                            exit 0
repo mutate --only 'changelog: '                                                3 of 3 guards proved
```

3 mutation rows, 3 of 3 red; one came back BROKEN first and was repointed.


## TOOL-37. A nightly whose upload fails loses the whole prerelease

**Source** the old record's finding 103
**Category** tooling, **Priority** P3, **Effort** S, **Status** done

---

## Problem

`gh release create` uploads inside the call that creates, and a failed upload
deletes the release. `release.yml` was fixed; the nightly still uploads that way, and
its read-back never fails.

## Premise

Read at `9784821`: `herdr-nightly.yml:168-171`.

## Approach

The nightly creates empty, uploads each asset, and fails when the read-back is short, as `release.yml` does.

## Consumers

None.

## Prove

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1
```

Exit 0, and the next nightly's run green.

---

## Closing

**Closed 2026-10-25, merged from `rb/f7-tooling`, for the next release.** The nightly's publish step is `repo nightly-publish`, with a fake-gh case and six rows: the create carries no asset, an existing prerelease is not created again, a failed upload stops the run, a short read-back fails it, only a `herdr-nightly-` tag is published, and `--repo` is required. `herdr-nightly.yml` calls it.

⚠ The entry's Prove asks for the next nightly run green, which this host cannot run.

## TOOL-38. The skills check never matches a text-tool call by its full path, and checks no `wsl-toolkit` flag

**Source** the review of the three skills, 2026-10-21
**Category** tooling, **Priority** P3, **Effort** M, **Status** done

---

## Problem

`skills.go:248` writes `"\text-tool.exe"`, where `\t` is a tab, so a call by full
path is never checked. The check holds each `wsl-toolkit` command name to the manual
and none of its flags, so a renamed flag in a skill passes.

## Premise

Read on 2026-10-21 by the review.

## Approach

Fix the string, and check every flag a fenced `wsl-toolkit` line passes against
that command's flags in the generated manual.

## Consumers

None.

## Prove

```bash
sh scripts/common/check.sh skills
```

Exit 0, with a case for a full-path call and one for an unknown flag.

---

## Closing

**Closed 2026-10-21T14:18:03Z.** The suffix is a raw string, so a call by its full
Windows path is read. Every long flag a fenced `wsl-toolkit` line passes after its
command is held to the flags the manual lists under that form, up to the payload: a
dashed word after `-c`, after `--`, or inside quotes is the base command's, not this
tool's. `--help` is accepted on every form. `skills/README.md` says what the check now
reads.

```text
a planted `wsl-toolkit gc --workspace . --bogus-flag` in skills/wsl-toolkit/SKILL.md
  FAIL   skills/wsl-toolkit/SKILL.md:24: passes `--bogus-flag` to `wsl-toolkit gc`,
         which tools/windows/wsl-toolkit/wsl-toolkit.1 does not list for that form
the file restored byte for byte; sh scripts/common/check.sh skills     exit 0
repo mutate --only 'skills: '                                          8 of 8 guards proved
```

4 mutation rows, 4 of 4 red, and 9 new cases.

## TOOL-39. CI takes 23 minutes and a push up to 44, and a mutation row can be proved by a sibling's failure

**Source** the operator, 2026-10-25: "what is so wrong that ci takes this long ... yes please fix it all", after a read-only analysis of CI's step timings
**Category** tooling, **Priority** P1, **Effort** L, **Status** done

---

## Problem

CI run 40156974353 took 1,373 s, its six mutation shards 1,228 to 1,371 s each, and three shards had run out of their 30 minutes on 1,202 rows. The pre-push hook took 370, 886 and 2,651 s. Separately, the harness marked a row ok for any failing exit once a case ran, so a mutation that broke a sibling package proved a guard that never went red.

## Premise

Measured from GitHub's step timings. Each toolkit row recompiles a 57k-line package about twice, rows run one at a time, modulo shards repeat baselines, the gate runs its checks in sequence, and no job caches Go builds. Read: `tools/repo/internal/mutate/mutate.go:300` and `:319-320`.

## Approach

Per-package mutated runs with `-vet=off` and verdicts read from `go test -json`; a worker pool with isolated copies at fixed paths; cost-balanced shards that keep pattern groups whole, 12 of them; a concurrent gate; the probes job's halves at once; Go caches with `GOFLAGS=-count=1`; mutation proofs only for changed modules, with a nightly and a release full run; the `go` job without its duplicate tests; the pre-push hook in one harness process with its three jobs at once.

## Consumers

None outside this repository. A developer's push runs faster.

## Prove

```bash
gh run view RUN --json jobs
```

A push that touches wsl-toolkit finishes CI in about 5 minutes with every row proved, and the verdicts match the old harness except named false-ok rows.

---

## Closing

**Closed 2026-10-25, on `main` at `a0ec0f6`.** The four `perf/*` branches merged as `ba099fb`, `ef86cbd`, `f6d75f5` and `aecf7eb`, and two fixes followed.

- **Harness.** Each row is judged from `go test -json`, and a row is ok only when a named case fails. A sibling's failure proves nothing now. On a 188-row Linux sample, the old and new harness gave the same verdict and case count on every row.
- **CI.** Run 40242199502 proved every `wsl-toolkit` row in 465 s, against 1,373 s in run 40156974353. Its 12 shards took 241 to 421 s, against 1,228 to 1,371 s for six. The probes job took 371 s against 572 s, and the Windows job 206 s against 291 s.
- **Kept proofs.** Run 40238416575 changed `tools/check` alone and finished in 335 s. Its shards took 32 to 69 s.
- **Pre-push.** The push of the four merges proved 173 rows, ShellCheck and the go job in 408 s, against 370 to 2,651 s before. Two later pushes took 246 and 227 s.

⚠ **The Prove's "about 5 minutes" is not met.** A `wsl-toolkit` push takes about 8 minutes, and its slowest shard sets that floor. `TOOL-40` holds the cost of each row that remains.

⚠ **The probes job's halves now run at once, and that exposed two racing cases.** Two runs of `go test` shared the fake scope's mark, so one run's deadline killed the other's child. A detached client's case waited a fixed 300 ms, which a loaded host outran. `a0ec0f6` fixes both, each reproduced first. `8848d92` makes a `--json` check print its findings on stderr, so a disagreement names its case.


## TOOL-40. Each mutation row's cost tracks the size of one 57k-line package and its serial tests

**Source** the operator, 2026-10-25: "what is so wrong that ci takes this long ... yes please fix it all", after a read-only analysis of CI's step timings; step 8 of that plan, left for later
**Category** tooling, **Priority** P3, **Effort** L, **Status** open

---

## Problem

A toolkit row recompiles `internal/toolkit` whole, and its 849 tests run in one process with no `t.Parallel`, 30 of them taking a second or more. Both grow with every change.

## Premise

Measured in the same analysis: 85 of 128 s of the package's tests are in 30 cases; `t.Setenv` in 47 test files blocks `t.Parallel`. After `TOOL-39`, CI run 40242199502's 12 shards took 241 to 421 s, and the slowest set the run's 465 s.

## Approach

Split the compiled test binary across 2 to 4 processes in the `go` check; give embedded shell scripts a test-time override so their rows skip a recompile; consider splitting `internal/toolkit` along its seams. Each step proves the same verdicts before and after.

## Consumers

None outside this repository.

## Prove

```bash
cd tools/windows/wsl-toolkit && go test ./internal/toolkit -count=1
```

The package's tests and a toolkit mutation row each take materially less time, with identical results.

---

## Closing

Open.
