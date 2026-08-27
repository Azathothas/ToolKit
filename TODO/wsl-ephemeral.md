# TODO: wsl-ephemeral

Entries for `scripts/powershell-windows/wsl-ephemeral.ps1`.
[`INDEX.md`](INDEX.md) is the list; [`PROGRESS.md`](PROGRESS.md) is the order.

⛔ An entry closes **in place**, with its acceptance command actually run and
the output recorded underneath. A premise a measurement disproves keeps its
title and gets the correction written below it.

---

## WSL-01. `New -Command` must propagate the inner exit code

**Source** `Azathothas/TEMPLATE` issue 3, part 3.1. The reporter called it the
one to do first.
**Category** wsl-ephemeral · **Priority** P0 · **Effort** S · **Status** open

**Problem.** A failing command run through `-Action New -Command` reports
success. The script exits 0.

**Premise.** ⭐ **Measured, by reading the source at the commit the entry was
written against.** `Invoke-ActionNew` captures `$rc` from the inner call and
then reaches `Write-Warn "command exited $rc"` and returns; the script's main
`try` completes and exits 0. `Invoke-ActionRun` ends in `exit $rc` and
propagates correctly. The asymmetry is unintentional: the script's own
`PSReviewUnusedParameter` justification asserts "Run exits with the inner
command's code and callers read that", which is true of only one of the two.

**Approach.** Propagate the code from `Invoke-ActionNew`, **after** teardown.

Three things this has to get right, and the third is the one that is easy to
miss:

1. ⛔ `-Ephemeral` must still remove the distro before the script exits.
   Returning the code early leaks a registered distro and a VHDX.
2. ⛔ `$rc` is assigned inside a `try`. Under `Set-StrictMode -Version Latest`
   an unassigned variable throws on read, so the failure path must set it.
3. The `New` and `Run` behaviours go in `.NOTES` together so they cannot drift
   apart again, and the first row of the "Known limits" table in
   `wsl-ephemeral.md` is deleted in the same change. ⛔ Doc and code ship
   together.

**Decision.** Propagate, rather than documenting the asymmetry as intended.
⚠ **This is a breaking change** for any caller relying on the false pass, and
[`../docs/consumers.md`](../docs/consumers.md) governs it. The alternative,
leaving it and documenting it, loses because a step that cannot fail makes
every green result downstream of it meaningless. That is a forbidden pattern in
its own right, and this is the instance that put the row there.

**Prove.** ⛔ Read each exit code from the process that produced it, unpiped.

```powershell
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral.ps1 -Action New -Image alpine:3.22 -Command "exit 7" -Ephemeral -Force
```

Exit code is 7, and `wsl --list --quiet` no longer lists the distro.

```powershell
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral.ps1 -Action New -Image alpine:3.22 -Command "true" -Ephemeral -Force
```

Exit code is 0.

⚠ **Mutation-prove it.** Revert the fix, confirm the first command reports 0,
restore it. A guard nobody has seen fail is theatre.

---

## WSL-02. Carry the image's OCI configuration into the distro

**Source** issue 3, part 3.2.
**Category** wsl-ephemeral · **Priority** P1 · **Effort** M · **Status** open

**Problem.** The distro's environment is not the image's environment. `PATH`,
`WORKDIR`, `ENTRYPOINT` and `USER` are all absent, which makes the distro a
poor stand-in for the thing a person is usually testing.

**Premise.** Read, not measured. `Export-ImageRootfs` uses `create` then
`export`, and `podman export` writes a filesystem with no OCI config by
definition. ⚠ Confirm by comparing `PATH` inside the distro against
`podman run IMAGE sh -lc 'echo $PATH'` before building anything.

**Approach.** Read the config and write it into the distro as
`/etc/profile.d/10-oci-env.sh`.

```bash
podman image inspect IMAGE --format json
```

**Decision.** ⚠ Behind a switch defaulting to **off**. Changing the environment
of an existing distro shape moves every current caller under them for a benefit
they did not ask for. The alternative, on by default, loses on that alone.

**Prove.** A distro built from an image with a non-default `PATH` reports that
`PATH` from `wsl -d DISTRO -- /bin/sh -lc 'echo $PATH'` with the switch, and the
old value without it.

---

## WSL-03. Pass `--platform` to pull and create

**Source** issue 3, part 3.3, and issue 2's tag-overwrite trap.
**Category** wsl-ephemeral · **Priority** P1 · **Effort** S · **Status** open

**Problem.** The exported architecture is whatever the local store happened to
hold, so the import can silently produce a rootfs that cannot execute. It
surfaces only as the `/bin/sh did not run` smoke failure, with no hint of cause.

**Premise.** ⭐ **Measured, in the reporting issue.** A single
`podman run --platform linux/riscv64 alpine` retags the shared local
`alpine:latest` to the riscv64 image, so a later unqualified pull is a no-op and
the wrong architecture is exported.

**Approach.** `Export-ImageRootfs` already runs
`podman info --format '{{.Host.Arch}}'` as a readiness probe and discards the
value. Keep it and pass `--platform linux/ARCH` to both `pull` and `create`.

**Decision.** Default to the host architecture rather than adding a
`-Platform` parameter now. A parameter with no caller is machinery nothing asked
for; add it when something asks.

**Prove.**

```bash
podman pull --platform linux/riscv64 alpine
```

Then `-Action New -Image alpine` on this x86_64 host still produces a distro
whose `uname -m` is `x86_64`.

---

## WSL-04. A failed delete must not report success

**Source** issue 3, part 3.6.
**Category** wsl-ephemeral · **Priority** P1 · **Effort** S · **Status** open

**Problem.** `Remove-EphemeralDistro` prints `deleted DIR` whether or not the
directory went, so a multi-gigabyte VHDX can be left behind and reported gone.

**Premise.** Read. `Remove-Item -LiteralPath $dir -Recurse -Force
-ErrorAction SilentlyContinue` is followed by an unconditional `Write-Ok`.
`--unregister` releases the VHDX asynchronously, so the delete immediately
after can lose the race. ⚠ The race is asserted, not reproduced; reproduce it
by holding a handle on the directory.

**Approach.** `Test-Path` after the delete, a short bounded retry, an honest
message and a non-zero exit on failure.

**Prove.** With a handle held open on the distro directory, the command exits
non-zero and the message names the path. With no handle, it exits 0.

---

## WSL-05. Report and purge orphaned rootfs tarballs

**Source** issue 3, part 3.7.
**Category** wsl-ephemeral · **Priority** P2 · **Effort** S · **Status** open

**Problem.** An interrupted `New` can leave a rootfs `.tar` of several hundred
MiB in `%LOCALAPPDATA%`, and nothing reports it.

**Premise.** Read. The temp tarball is cleaned in a `finally`, which does not
run on every hard interrupt, and neither `Invoke-ActionList` nor
`Invoke-ActionPurge` looks at `*.tar`.

**Approach.** `List` reports orphans with their sizes; `Purge` offers to remove
them, under `Assert-InsideBaseDir` like every other deletion.

**Decision.** ⛔ Orphan removal goes through the same containment guard as
distro removal. A second deletion path with its own checks is how one of them
ends up without any. [`../docs/conventions/code.md`](../docs/conventions/code.md),
one write path.

**Prove.** Place a `.tar` in the base directory; `List` names it and `Purge
-Force` removes it. A `.tar` outside the base directory is refused.

---

## WSL-06. Disk-space preflight before import

**Source** issue 3, part 3.8.
**Category** wsl-ephemeral · **Priority** P2 · **Effort** S · **Status** open

**Problem.** Running out of space midway leaves a partial VHDX and a registered
distro that does not work.

**Premise.** Read. Export plus import needs roughly twice the rootfs size on the
`%LOCALAPPDATA%` volume. ⚠ The factor of two is an estimate and the entry says
so; measure it once on a real import before writing it into a message.

**Approach.** Compare free space on the target volume against the tarball size
before `--import`, and refuse cleanly when it is short.

**Decision.** Refuse rather than warn. A clean refusal is recoverable; a partial
VHDX and a broken registration is a state the user has to be told how to unpick.

**Prove.** With a deliberately low threshold, the command refuses before
`--import` runs and registers nothing. `wsl --list --quiet` is unchanged.

---

## WSL-07. Optional systemd via `/etc/wsl.conf`

**Source** issue 3, part 3.9.
**Category** wsl-ephemeral · **Priority** P2 · **Effort** S · **Status** open

**Problem.** An imported distro has no `/etc/wsl.conf`, so systemd does not
start and nothing involving units, timers or `systemctl` can be tested. That
rules out a large share of what a throwaway distro is useful for.

**Premise.** Read.

**Approach.** A `-Systemd` switch writing `[boot]` and `systemd=true`.
⚠ It needs a `--terminate` afterwards to take effect, and that belongs next to
the switch in the documentation rather than in a release note.

**Prove.** With the switch, `systemctl is-system-running` answers from inside
the distro. Without it, the behaviour is unchanged.

---

## WSL-08. A `-Command` channel that survives two shells

**Source** issue 3, part 3.11.
**Category** wsl-ephemeral · **Priority** P2 · **Effort** M · **Status** open

**Problem.** `-Command` crosses PowerShell and then `/bin/sh -lc`, and the
caller owns all quoting across both.

**Premise.** Read, and the repository already has the rule:
[`../docs/conventions/shell.md`](../docs/conventions/shell.md) section 1 is
emphatic that this is where payloads lose their meaning, and names base64 as the
one channel no shell interprets.

**Approach.** `-CommandFile PATH` or `-CommandB64 STRING`, consistent with the
answer the tree already ships in `scripts/common/write-file.mjs`.

**Decision.** Both, and keep `-Command`. Removing the simple form to force the
safe one is a break for every existing caller for no gain: the simple form is
correct for simple commands.

**Prove.** A command containing a single quote, a double quote, a backtick, a
dollar sign and a tab arrives byte-exact inside the distro.

---

## WSL-09. Bound the smoke probe with a timeout

**Source** issue 3, part 3.13.
**Category** wsl-ephemeral · **Priority** P3 · **Effort** S · **Status** open

**Problem.** A distro whose init wedges hangs the script with no output.

**Premise.** Read. The probe absorbs a drvfs race with a bounded sleep loop
**inside** the guest, but the outer `& $wsl ...` call has no time limit.

**Approach.** A bounded wait with a clear message on expiry.
[`../docs/conventions/shell.md`](../docs/conventions/shell.md) section 9.
⚠ Distinguish "it never answered" from "it is not installed"; they are different
facts and belong in different messages.

**Prove.** Against a distro whose init blocks, the script exits non-zero within
the bound with a message naming the timeout.

---

## WSL-10. Retry a generated name on collision

**Source** issue 3, part 3.10.
**Category** wsl-ephemeral · **Priority** P3 · **Effort** S · **Status** open

**Problem.** A generated name that collides throws instead of drawing again.

**Premise.** Read. `New-DistroName` appends four characters from a 36-symbol
alphabet, so the space is 36^4 = 1,679,616. For a caller-supplied `-Name`,
throwing is correct behaviour and stays.

**Approach.** Retry a bounded number of times for a **generated** name only.

**Prove.** With the generator stubbed to a constant, the first call succeeds and
the second draws again rather than throwing; after the retry bound it throws
with a message saying so.

---

## WSL-11. An `Enter` action

**Source** issue 3, part 3.12. ⚠ Missed on the first pass through the issue and
added on review; the fourteen findings map to eleven entries and three
documentation notes, and this was briefly neither.
**Category** wsl-ephemeral · **Priority** P3 · **Effort** S · **Status** open

**Problem.** The summary after `New` tells the user to run `wsl -d DISTRO` by
hand, so the interactive path is the only one that is not first class.

**Premise.** Read. `ValidateSet` is `New, Run, List, Remove, Purge`.

**Approach.** `-Action Enter`, honouring `-User`, giving one place to add
further interactive handling later.

**Prove.** `-Action Enter -Name eph-...` attaches an interactive shell as
`-User`, and a name that is not registered is refused with a clear message.
