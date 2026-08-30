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
**Category** wsl-ephemeral, **Priority** P0, **Effort** S, **Status** done

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

### Closed 2026-08-27

**What changed.** `Invoke-InDistro` is now the one function that runs a
caller's command inside a distro, and `Invoke-ActionNew` and `Invoke-ActionRun`
both call it. `Invoke-ActionNew` ends in `exit $rc`, placed after the
`-Ephemeral` teardown and after the `finally` that removes the temp tarball.

⚠ **The approach was widened, deliberately, and here is the reason.** The entry
asked for the two behaviours to be documented together so they could not drift
apart again. Documenting them together leaves two copies of the code that has to
agree. Extracting the one path makes the agreement structural instead, which is
[`../docs/conventions/code.md`](../docs/conventions/code.md), one write path.
The cost is fifteen lines and one indirection.

⭐ **The exit code comes back through a `[ref]` parameter, not as a return
value.** The inner command's stdout flows out of the function's success stream
so the caller can see it, so `$rc = Invoke-InDistro ...` would capture that
output into `$rc` instead of the code. That is the trap this shape avoids, and
it is why the obvious signature is the wrong one.

**Mutation proof.** ⛔ Run against the unmodified script, before the fix, on
this Windows 11 Pro 26200 machine with `podman-machine-default` running. Not a
simulated revert: the shipped code.

```powershell
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral.ps1 -Action New -Image alpine:3.22 -Command "exit 7" -Ephemeral -Force
```

```text
==> Running command as 'root'
  ! command exited 7
==> -Ephemeral set: tearing down 'eph-alpine-3.22-6ytu'
  * unregistered eph-alpine-3.22-6ytu
EXITCODE=0
```

**Acceptance, after the fix.** Every code read from the process that produced
it, unpiped.

| command | exit | also asserted |
| --- | --- | --- |
| `-Action New -Image alpine:3.22 -Command "exit 7" -Ephemeral -Force` | 7 | `wsl --list --quiet` lists only `podman-machine-default` afterwards |
| `-Action New -Image alpine:3.22 -Command "true" -Ephemeral -Force` | 0 | |
| `-Action New -Image alpine:3.22 -Name eph-w01 -Force` | 0 | the summary prints and the distro stays registered |
| `-Action Run -Name eph-w01 -Command "exit 42"` | 42 | `Run` still propagates, through the shared path now |
| `-Action Run -Name eph-w01 -Command "echo hello; exit 0"` | 0 | `hello` reached the console, so the `[ref]` shape did not swallow stdout |

**Consumers checked.** `Azathothas/TEMPLATE` is the only row in
[`../docs/consumers.md`](../docs/consumers.md). Its wrapper declares no
parameters and forwards `@args`, then exits with `$LASTEXITCODE` from the
inner script, so the new code reaches a caller unchanged and no wrapper edit is
needed. ⚠ The break is recorded in that file and in
[`../CHANGELOG.md`](../CHANGELOG.md).

**Pin state.** ⭐ Held in [`../docs/consumers.md`](../docs/consumers.md), which
is the one home for that fact. ⛔ The pin is not bumped to this commit: it would
name a tree that does not yet carry the rest of the `WSL-*` batch, and a
consumer stepping onto a mid-batch commit gets a version nobody ran the gate
against as a whole.

---

## WSL-12. `-Action New` fails outright on Windows PowerShell 5.1

**Source** ⭐ **This repository's own door sweep**, part (c) of the gate, run
against the `WSL-01` to `WSL-05` batch. It was not in issue 3 and nobody had
reported it.
**Category** wsl-ephemeral, **Priority** P0, **Effort** S, **Status** done

**Problem.** Every `-Action New` run from Windows PowerShell 5.1 failed. The
distro imported, the smoke probe was refused by the guest shell, the script
reported `Distro imported but /bin/sh did not run`, rolled the distro back and
exited 1. ⛔ **On a host `.NOTES` claimed to be tested on.**

**Premise.** ⭐ **Measured, not read.** The smoke probe carried this line:

```text
if [ ! -d /mnt/c ]; then echo "note: no /mnt/c (Windows drives not mounted)"; fi
```

The quoting does not survive `wsl.exe`, so the bracket reached the guest as
syntax:

```text
/bin/sh: syntax error: unexpected "("
```

⚠ **It is host-specific, which is why it survived.** Under PowerShell 7.6.5 the
double quote does arrive and the probe runs; under 5.1 it does not. Every
measurement in this repository until this session had been taken with `pwsh`.

**Approach.** Rewrite the probe payload inside the alphabet that arrives intact:
no double quote, no bracket, no dollar sign, no backtick. ⛔ Not a fix for the
transport, which is `WSL-08`. A payload change is what makes the tool work
today; the transport is what stops the next payload doing this again.

**Decision.** Fix the payload now rather than waiting for `WSL-08`. The
alternative loses badly: `WSL-08` is an M behind three other entries, and until
it lands the tool does not run at all on one of its two documented hosts.

**Prove.** `-Action New` from **both** hosts, each exit code read from the
process that produced it.

### Closed 2026-08-27

**Mutation proof.** The defect was measured on the shipped code before the fix,
which is the strongest form of it: not a simulated revert, the real thing.

```text
==> Importing as WSL2 distro 'eph-51'
  ! creation failed; rolling back
ERROR: Distro imported but /bin/sh did not run. Output: /bin/sh: syntax error: unexpected "("
5.1 EXITCODE=1
```

**Acceptance, after the fix.**

| host | command | exit |
| --- | --- | --- |
| Windows PowerShell 5.1 | `-Action New -Image alpine:3.22 -Command "uname -m" -Ephemeral -Force` | 0, and `x86_64` printed |
| PowerShell 7.6.5 | `-Action New -Image alpine:3.22 -Command "exit 5" -Ephemeral -Force` | 5 |

⭐ `wsl --list --quiet` afterwards shows only `podman-machine-default`, and the
base directory is empty, so neither run leaked.

⚠ **What this does NOT fix.** `-Command` still cannot carry a `$`, a backtick
or, on 5.1, a double quote. That is `WSL-08` and it is still open. The limits
table in `wsl-ephemeral.md` now says so in those terms rather than the
"the caller owns all quoting" it said before, which measurement disproved.

**Consumers.** ⛔ This is the reason the `Azathothas/TEMPLATE` pin moved in this
session. Its wrapper runs the fetched script on whichever host invoked it, so
every 5.1 caller pinned to the old commit had an `-Action New` that could not
work. [`../docs/consumers.md`](../docs/consumers.md) carries the pin state.

---

## WSL-02. Carry the image's OCI configuration into the distro

**Source** issue 3, part 3.2.
**Category** wsl-ephemeral, **Priority** P1, **Effort** M, **Status** done

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

### ⚠ The prove command does not work, and finding out why is a separate finding

⛔ **`echo $PATH` is exactly the command that cannot be sent.** Measured on
2026-08-27 against a real distro:

```text
wsl -d eph-w02-on -u root -- /bin/sh -lc 'echo $PATH'
/bin/sh: syntax error: unexpected "("
```

The single quotes do not survive the trip, so `$PATH` expands, and the value it
expands to contains `/mnt/c/Program Files (x86)/...` because WSL appends the
Windows path. The `(` then reaches the shell as syntax. ⭐ **That is `WSL-08`,
seen in the most ordinary command anybody would type**, and it is why the
acceptance below uses `printenv PATH`, which needs no `$`.

### ⚠ The premise's suggested check is misleading, and it was measured

The premise says to confirm by comparing against
`podman run IMAGE sh -lc 'echo $PATH'`. ⛔ **That comparison answers the wrong
question**, because `sh -l` runs `/etc/profile` inside the container too, and
Alpine's resets `PATH`. Measured, same image, same machine:

| where | `PATH` |
| --- | --- |
| `podman run --rm python:3.13-alpine sh -c 'printenv PATH'` | `/usr/local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin` |
| `podman run --rm python:3.13-alpine sh -lc 'printenv PATH'` | ⚠ `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin` |

⭐ The first is the image's `PATH`; the second is `/etc/profile`'s. The
authority is `podman image inspect --format '{{json .Config}}'`, which is what
the code reads.

### Closed 2026-08-27

**What changed.** `-OciEnv`, defaulting off, as the entry decided. It reads
`{{json .Config}}` from the image and writes `/etc/profile.d/10-oci-env.sh`
into the distro, which a login shell sources, and `-Command` runs through
`/bin/sh -lc`.

⭐ **`Write-DistroFile` is the seam, and it exists because of the measurement
above.** It carries the body as base64, because base64 is the only alphabet that
survives `wsl.exe`. ⚠ It also **refuses a path** it cannot carry safely rather
than carrying it unsafely, since the path is not quotable either. `WSL-08`
reuses it.

⛔ **`USER` and `ENTRYPOINT` are not carried, and will not be.** The entry's
Problem statement names all four. WSL fixes the login user at import and
`-User` selects it per call, so a `USER` line in a profile script is a setting
nothing reads; a login shell has no entrypoint to run. Both would be a value
the engine reads that nobody can set, from the other direction. That is written
into `wsl-ephemeral.md` rather than left as an omission.

**Acceptance.** Two distros from `python:3.13-alpine`, whose image `PATH` is
non-default, one built with the switch and one without.

```powershell
wsl -d eph-w02-off -u root -- /bin/sh -lc 'printenv PATH'
```

| | `PATH` | `PYTHON_VERSION` |
| --- | --- | --- |
| without `-OciEnv` | `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin` | unset |
| with `-OciEnv` | ⭐ `/usr/local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin` | `3.13.15` |
| the image itself | `/usr/local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin` | `3.13.15` |

⭐ The with-switch row equals the image row exactly. The file arrived with mode
`0644` and the values single-quoted:

```text
export PATH='/usr/local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'
export GPG_KEY='7169605F62C751356D054A26A821E680E5FA6305'
export PYTHON_VERSION='3.13.15'
export PYTHON_SHA256='1e66a794...eea4a76'
```

⚠ **What the acceptance does NOT show.** `command -v python3` resolves in both
distros, because `/usr/local/bin` is on Alpine's default profile `PATH` anyway.
A reader could mistake that for the switch not mattering. It matters for the
order of `PATH` and for every other variable, which is what the table shows.

**Mutation proof.** Not a guard, so there is nothing to plant. The negative case
is the without-switch column above, run from the same tree in the same minute:
it reports the old value, so the switch is what moves it. ⛔ If the switch were
a no-op both rows would read the same, and they do not.

---

## WSL-03. Pass `--platform` to pull and create

**Source** issue 3, part 3.3, and issue 2's tag-overwrite trap.
**Category** wsl-ephemeral, **Priority** P1, **Effort** S, **Status** done

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

### ⛔ The premise was half wrong, and the correction matters more than the fix

⛔ **Written underneath rather than edited into the premise above.** Measured on
podman 5.8.6, Windows 11 Pro 26200, kernel 7.2.0-WSL2-STABLE, 2026-08-27.

**1. An unqualified `pull` is NOT a no-op on this engine.** The premise says a
later unqualified pull is a no-op, so the wrong architecture is exported. It is
not:

| step | `podman image inspect alpine:3.22 --format '{{.Architecture}}'` |
| --- | --- |
| after `podman rmi alpine:3.22` | absent |
| after `podman pull --platform linux/riscv64 alpine:3.22` | `riscv64` |
| after an unqualified `podman pull alpine:3.22` | ⭐ `amd64` |

podman re-pulls the host-architecture image and repoints the tag. ⚠ Do not read
that as "the trap is not real": it says the trap does not fire through `pull`
on **this** version, which is a narrower claim than the entry made.

**2. The trap is real, and it lives in `create`.** With the tag left pointing
at riscv64 and no pull in between, an unqualified `podman create alpine:3.22`
exits 0 and produces a container whose image is the riscv64 one. That is the
operation this script uses to materialise a rootfs, and it was the unqualified
one.

**3. ⛔ The stated symptom is wrong on this machine, in the dangerous
direction.** The entry says a wrong-architecture rootfs "surfaces only as the
`/bin/sh did not run` smoke failure". It does not surface at all. With
`create` mutated to ask for `linux/riscv64`, the distro imported, the smoke
test passed, and:

```text
==> Running command as 'root'
riscv64
EXITCODE=0
```

⭐ **The cause is the shared kernel.** 31 `qemu-*` handlers are registered in
this WSL2 kernel's `binfmt_misc`, and an imported distro sees every one of
them because `--import` gives it a new userspace and not a new kernel:

```bash
wsl -d eph-w01 -u root -- /bin/sh -lc 'cat /proc/sys/fs/binfmt_misc/qemu-riscv64'
```

```text
enabled
interpreter /usr/bin/qemu-riscv64-static
flags: POCF
```

⚠ The `F` flag is what makes it reach a distro that has no emulator installed:
the kernel opens the interpreter when the handler is registered and holds that
file open, so the binary does not have to exist inside the mount namespace that
runs. ⛔ **So the failure this entry was written against is not a failure here.
It is a distro that is silently the wrong architecture and runs emulated**,
which is the worse of the two outcomes and the one no smoke test can catch.

### Closed 2026-08-27

**What changed.** The readiness probe's answer is used instead of discarded, and
`--platform linux/ARCH` is passed to `pull` and to both `create` calls.
`ConvertTo-OciArch` normalises what the engines report onto the names
`--platform` accepts.

⚠ **Two things the entry did not anticipate, both found by writing it:**

- **The engines spell the field differently.** `podman info` has `.Host.Arch`
  and answers `amd64`; `docker info` has `.Architecture` and answers
  `x86_64`. The probe asked both engines for `{{.Host.Arch}}`, so on a docker
  machine it was failing and being reported as "docker is installed but not
  responding". That is fixed here because this entry is the one that made the
  value load-bearing.
- **`x86_64` is not a name `--platform` accepts.** Reading the engine's answer
  and passing it straight through would have produced `linux/x86_64` on docker.

**Mutation proof.** `create` was changed to ask for `linux/riscv64` and the
run above is the result: a riscv64 distro, reported as up, exit 0. Restored, and
with the local tag deliberately left pointing at riscv64:

```powershell
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral.ps1 -Action New -Image alpine:3.22 -Command "uname -m" -Ephemeral -Force
```

```text
==> Platform: linux/amd64
==> Running command as 'root'
x86_64
EXITCODE=0
```

⭐ The local tag read `riscv64` before the run and `amd64` after it, so the
named pull is doing the work rather than the tag happening to be right.

**Follow-on.** ⚠ The binfmt finding above is what `DOC-01` is about, and it
changes that entry's shape: handlers are registered here and reach an ephemeral
distro, so a check has something to assert rather than a gap to report.

---

## WSL-04. A failed delete must not report success

**Source** issue 3, part 3.6.
**Category** wsl-ephemeral, **Priority** P1, **Effort** S, **Status** done

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

### Closed 2026-08-27

**What changed.** `Remove-PathWithRetry` is now the only deletion in the
script. It deletes, reads the state back, retries five times over about three
seconds, and throws naming the path when the path is still there.

⭐ **All three deletion paths go through it**, including the rollback after a
failed create, which previously had its own `Remove-Item` and its own guard
call. ⛔ **The containment guard moved inside the helper**, because a guard
applied at three call sites is a guard that will one day be applied at two, and
that is the most recurring hole in this whole methodology.

⚠ **The `-Ephemeral` teardown moved out of `Invoke-ActionNew`'s `try`.** It
had to. Inside the try, a teardown that could not delete the disk was caught by
the rollback handler and announced as `creation failed; rolling back`, which is
false and sends the reader to the wrong half of the function: the distro was
created, the command ran, and the only thing wrong was several gigabytes still
on disk. It still runs before the exit, so `WSL-01`'s requirement holds.

**The premise's race was asserted, not reproduced. It still is.** ⚠ The entry
says so and it was right to. What is reproduced below is the **guard**, against
the fault it exists to catch, injected deliberately: a `FileShare::None` handle
held on a file inside the directory, which is what a live VHDX looks like to
`Remove-Item`. ⛔ Whether `--unregister`'s async release actually loses that
race on this machine is still unmeasured, and no run in this session lost it.

**Mutation proof.** The helper call was reverted to the original
`Remove-Item -ErrorAction SilentlyContinue` plus an unconditional `Write-Ok`,
with the same handle held:

```text
  ! eph-w04 was not registered
  * deleted C:\...\wsl-ephemeral\eph-w04
EXITCODE=0
still there: True
```

⛔ **Reported deleted, exit 0, still on disk.** That is the defect, seen.

**Acceptance, after restoring the fix.**

```powershell
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral.ps1 -Action Remove -Name eph-w04 -Force
```

| condition | exit | message |
| --- | --- | --- |
| handle held | 1 | `FAILED to delete the disk for 'eph-w04' at 'C:\...\eph-w04'. It is STILL THERE after 5 attempts.` and the path is still there |
| handle released | 0 | `* deleted C:\...\eph-w04`, and the path is gone |

### ⚠ 2026-08-27, later the same day: the two removal paths did not agree

⛔ **Written under the closure rather than edited into it.** A door sweep for
the `WSL-06` to `WSL-11` batch asked which paths reach one operation. Both
removals go through `Remove-PathWithRetry`, which this entry made the only
deletion, but they did not agree on what happens first:

| path | before |
| --- | --- |
| `Remove-EphemeralDistro` | `--terminate`, then `--unregister`, then delete |
| ⛔ the rollback in `Invoke-ActionNew`'s catch | `--unregister`, then delete |

⭐ The rollback was the one MORE exposed to the async release this entry is
about, and it is the path that runs when something has already gone wrong. It
now terminates first, like the other one.

⚠ **This is not a measured bug fix and it is not presented as one.** The race
this entry describes has still never been reproduced on this machine, exactly as
the closure above says. What is fixed is two ways of doing one thing, which is
how one of them ends up wrong.

⚠ **This is a behaviour change for `Remove`, `Purge` and `New -Ephemeral`:**
they can now exit non-zero where they used to exit 0. They were not succeeding
before; they were reporting. Not a consumer break in the
[`../docs/consumers.md`](../docs/consumers.md) sense, because no caller can
have been depending on a disk being left behind, but it is written into
`wsl-ephemeral.md` beside the exit codes.

---

## WSL-05. Report and purge orphaned rootfs tarballs

**Source** issue 3, part 3.7.
**Category** wsl-ephemeral, **Priority** P2, **Effort** S, **Status** done

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

### Closed 2026-08-27

**What changed.** `Get-OrphanTarball` scans the base directory for `*.tar`.
`List` reports each with its size and last-written time; `Purge` removes them
through `Remove-PathWithRetry`, which is the same deletion and the same
containment guard `WSL-04` made the only one. The two classes share **one**
confirmation rather than prompting twice: two prompts over one `-Force` is how
somebody learns to pass `-Force` without reading either.

⚠ **`List` prints a time, not a verdict.** A `New` that is executing right now
has its tarball in exactly that directory, and nothing can tell that apart from
an orphan. Claiming to would be a synthetic status. The warning says to read the
time.

**Acceptance.** A 2 MiB `.tar` planted in the base directory, and a 1 MiB decoy
planted outside it.

```text
==> Orphaned rootfs tarballs in C:\...\wsl-ephemeral
  eph-orphan-w05.tar   2.0 MiB   written 2026-08-27 08:04:15Z
  ! 2.0 MiB total. Remove them with -Action Purge.
  ! a New running right now also has a .tar here: check the time before purging.
```

| after `-Action Purge -Force` | |
| --- | --- |
| exit | 0 |
| the orphan inside the base directory | gone |
| the decoy outside it | ⭐ still there, and never named |

### ⭐ The mutation found a second defect, and it is fixed here

⛔ **`Purge` was exiting 0 over a deletion it had refused.** The scan was
mutated to read the base directory's **parent**, a file called
`DO-NOT-DELETE-w05.tar` was planted there, and `Purge -Force` was run:

```text
  ! skip DO-NOT-DELETE-w05.tar: REFUSING to delete '...': outside ...\wsl-ephemeral.
EXITCODE=0
```

⭐ The containment guard fired and the file survived, which is the result the
mutation was looking for. **The exit code is the defect.** `Purge` caught the
refusal, warned, and returned success, which is the exact shape `WSL-04` had
just removed from the single delete, reappearing one level up in the loop that
calls it. ⚠ It was pre-existing: the original `Purge` did the same for distros.

⛔ **It also made a sentence written in `WSL-04`'s change false**, and that
sentence was already in `wsl-ephemeral.md`: "Remove, Purge and New -Ephemeral
read the directory back and report what they find, so they can fail where they
used to print success". `Purge` could not.

`Invoke-ActionPurge` now counts failures and throws at the end. ⭐ It counts
rather than throwing on the first, so one stuck item does not hide the state of
everything after it. Re-run against the same mutation:

```text
  ! skip DO-NOT-DELETE-w05.tar: REFUSING to delete '...': outside ...\wsl-ephemeral.
ERROR: 1 of 1 item(s) were NOT removed. Each is named in a warning above.
EXITCODE=1
```

⚠ **This is a second behaviour change to `Purge`,** on top of it now removing
tarballs. It can exit non-zero where it used to exit 0. Same reasoning as
`WSL-04`: it was not succeeding, it was reporting.

**Both happy paths re-run after the fix**, to confirm the tally did not turn a
working purge red: `Purge -Force` with nothing to do exits 0 and says
`nothing to purge`; with a real orphan it exits 0 and the orphan is gone.

---

## WSL-06. Disk-space preflight before import

**Source** issue 3, part 3.8.
**Category** wsl-ephemeral, **Priority** P2, **Effort** S, **Status** done

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

### Closed 2026-08-27

**What changed.** `Assert-EnoughDiskSpace` runs between the export and the
`--import`, and throws when the volume is short. `Get-VolumeFreeBytes` reads
`AvailableFreeSpace` rather than `TotalFreeSpace`, because a quota'd volume can
have plenty of the second and none of the first, and it is the first the import
spends.

### ⛔ The premise's factor of two is wrong, and it is not a multiple at all

⛔ **Written underneath, not edited in.** The entry said an import needs
"roughly twice the rootfs size" and flagged the two as an estimate to be
measured before it went into a message. Measured on this machine on
2026-08-27, VHDX size **on disk** against the tarball that produced it:

| image | rootfs `.tar` | VHDX on disk | ratio |
| --- | --- | --- | --- |
| `alpine:3.22` | 8.2 MiB | 76.0 MiB | ⛔ **9.27x** |
| `python:3.13-alpine` | 45.4 MiB | 140.0 MiB | 3.08x |
| `debian:bookworm-slim` | 74.3 MiB | 172.0 MiB | 2.31x |
| `ubuntu:24.04` | 76.9 MiB | 172.0 MiB | 2.24x |

⭐ **The cost is dominated by a fixed floor.** An 8 MiB rootfs costs 76 MiB,
and the two 75-ish MiB rootfs images cost the same 172 MiB as each other. A
rule written as "twice the rootfs" would have asked for 16 MiB where 76 was
needed, which is the direction that fails: it would have let an import start
that could not finish, which is the exact outcome this entry exists to prevent.

⚠ **Two alpine imports measured 76.0 MiB each**, so the floor is reproducible
rather than one sample. Sizes are read with `GetCompressedFileSizeW`, because a
VHDX is sparse and its logical length is not what the volume loses. On these
five files the two agreed exactly.

**The requirement is `256 MiB + 2 x tarball`.** ⛔ Set above every row rather
than fitted to them. A preflight that is tight refuses an import that would
have worked, and that is a worse failure than the one being prevented.

**Decision kept.** Refuse, not warn, as the entry decided. ⚠ **No parameter was
added** to override the threshold. `WSL-03` set the precedent: a parameter with
no caller is machinery with a maintenance cost and no benefit. The mutation
below is how the refusal was driven instead.

**Mutation proof.** The floor was raised above the volume's free space, which
is the same arithmetic a full volume produces:

```text
  * rootfs: 8.2 MiB
  * space: 1,000,015 MiB needed, 438,292 MiB free
  ! creation failed; rolling back
  * deleted C:\...\wsl-ephemeral\eph-w06full
  * deleted C:\...\wsl-ephemeral\eph-w06full.tar
ERROR: NOT ENOUGH DISK SPACE to import 'C:\...\eph-w06full'. Need about
1,000,015 MiB and 438,292 MiB is free on the volume holding C:\. Nothing has
been imported and nothing is registered. Free some space, or point LOCALAPPDATA
at a volume that has it, and run this again.
  exit=1
```

| asserted afterwards | |
| --- | --- |
| `wsl --list --quiet` | ⭐ byte-identical to before the run |
| the distro directory | gone |
| the rootfs tarball | gone |

**The third branch is proven too.** `Get-VolumeFreeBytes` returning `$null` is
"I could not measure", which is neither of the other two answers. Mutated to
return `$null`:

```text
  ! could not read free space for 'C:\...\eph-alpine-3.22-x9gw'; importing
    without the preflight. If the volume is full, the import will leave a
    partial disk.
ran-anyway
  exit=0
```

⭐ It says so and imports anyway. A preflight that skipped is not a preflight
that passed, and refusing on an unreadable volume would break the tool on a
machine that was fine.

**Acceptance, the happy path, both hosts.** Each code read from the process
that produced it.

| host | run | exit |
| --- | --- | --- |
| PowerShell 7.6.5 | `New -Image alpine:3.22 -Command 'echo ok' -Ephemeral -Force` | 0, `space: 272 MiB needed, 438,304 MiB free` |
| Windows PowerShell 5.1 | the same | 0, `space: 272 MiB needed, 438,298 MiB free` |

⚠ **What is NOT reproduced: a genuinely full volume.** The guard is proven to
fire, to fire before `--import`, and to leave nothing behind. Whether a real
out-of-space `--import` fails the way the Problem statement describes is still
unmeasured here, and filling a 428 GiB volume to find out would be a poor
trade. Same shape as `WSL-04`'s race, and it is said rather than implied.

---

## WSL-07. Optional systemd via `/etc/wsl.conf`

**Source** issue 3, part 3.9.
**Category** wsl-ephemeral, **Priority** P2, **Effort** S, **Status** done

**Problem.** An imported distro has no `/etc/wsl.conf`, so systemd does not
start and nothing involving units, timers or `systemctl` can be tested. That
rules out a large share of what a throwaway distro is useful for.

**Premise.** Read.

**Approach.** A `-Systemd` switch writing `[boot]` and `systemd=true`.
⚠ It needs a `--terminate` afterwards to take effect, and that belongs next to
the switch in the documentation rather than in a release note.

**Prove.** With the switch, `systemctl is-system-running` answers from inside
the distro. Without it, the behaviour is unchanged.

### Closed 2026-08-27

**What changed.** `-Systemd` writes `/etc/wsl.conf` through `Write-DistroFile`,
runs `wsl --terminate` so WSL re-reads it, and then **checks that systemd is
actually PID 1**. `Get-DistroOutput` is new: it runs a payload through the same
transport as everything else and returns what it printed, which is what the
check needs. ⭐ The smoke probe now uses it too, so the script has one capture
path rather than a second hand-rolled `& $wsl ... 2>&1`.

### ⛔ The switch that only writes the file would have been a lie

⛔ **Measured before the check was written, and it is why the check exists.**
The entry describes writing `[boot]` and `systemd=true` and terminating. Done
exactly that and no more, on 2026-08-27:

| image | after the write and `--terminate` |
| --- | --- |
| `ubuntu:24.04` | ⛔ PID 1 still `init(...)`. **No error, no delay, no message.** |
| `alpine:3.22` | ⛔ 20 seconds, then `wsl: Failed to start the systemd user session`, PID 1 still `init` |
| `almalinux:9` | ⭐ PID 1 `systemd`, `is-system-running` says `running` |

⭐ **Most OCI base images do not ship systemd at all**, which the entry did not
anticipate and which turns its Approach into a defect: `/usr/lib/systemd/systemd`
is absent from `alpine:3.22`, `ubuntu:24.04` and `fedora:41`, and present in
`almalinux:9`. A switch that writes the file into any of the first three is
`forbidden-patterns.md`'s "a setting or flag that no code reads", and the
caller comes away believing they have systemd.

So the switch verifies its own effect and refuses when the effect is absent.

**Mutation proof.** The verification was disabled and the same run repeated on
`alpine:3.22`:

```text
  * systemd is PID 1
init
NO-SYSTEMCTL
  C exit=0
```

⛔ **It printed `systemd is PID 1` over a distro whose PID 1 is `init` and
which has no `systemctl` at all**, and exited 0. That is the synthetic status
this guard removes, seen.

Restored, the same command:

```text
==> Enabling systemd via /etc/wsl.conf
  ! creation failed; rolling back
ERROR: -Systemd was asked for and this distro is NOT running systemd: PID 1 is
'init'. The likeliest cause is an image that does not ship systemd, and most do
not: measured on 2026-08-27, alpine:3.22, ubuntu:24.04 and fedora:41 have no
/usr/lib/systemd/systemd, and almalinux:9 has. Nothing is left registered. Use
an image that ships systemd, or drop -Systemd.
  B exit=1
```

⭐ `wsl --list --quiet` was byte-identical before and after that refusal.

**Acceptance.** Both hosts, each code read from the process that produced it.

| host | run | result |
| --- | --- | --- |
| pwsh 7.6.5 | `New -Image almalinux:9 -Systemd -Command 'systemctl is-system-running; cat /proc/1/comm; systemctl list-units ...'` | ⭐ exit 0, `running`, `systemd`, and real units listed: `dbus-broker.service`, `ldconfig.service` |
| Windows PowerShell 5.1 | `New -Image almalinux:9 -Systemd -Command '...; exit 4' -Ephemeral` | ⭐ exit **4**, `running`, `systemd`, distro unregistered and the disk gone |
| both | `New -Image alpine:3.22 -Systemd` | exit 1, refused, nothing registered |
| both | `New -Image alpine:3.22` with no switch | unchanged, exit 0 |

⚠ **The 5.1 row is doing double duty**: it shows the teardown of a distro
running systemd works, and that `WSL-01`'s exit-code guarantee survives the
restart the switch introduces.

### ⚠ Two things the entry did not say, both measured

- ⚠ **`wsl: Failed to start the systemd user session for 'root'` appears on a
  WORKING systemd distro.** It is about the per-user session, not the system
  manager, and treating it as the failure signal would have refused
  `almalinux:9`. `/proc/1/comm` is the fact that decides.
- ⚠ **The restart costs about 11 seconds** on this machine before the first
  command answers, because systemd is booting. The entry said `--terminate`
  "belongs next to the switch in the documentation rather than in a release
  note", and it is there, with the price attached.

**Consumers.** Adding a switch with a default of off. Not a break.

---

## WSL-08. A `-Command` channel that survives two shells

**Source** issue 3, part 3.11.
**Category** wsl-ephemeral, **Priority** P2, **Effort** M, **Status** done

**Problem.** `-Command` crosses PowerShell and then `/bin/sh -lc`, and the
caller owns all quoting across both.

**Premise.** Read, and the repository already has the rule:
[`../docs/conventions/shell.md`](../docs/conventions/shell.md) section 1 is
emphatic that this is where payloads lose their meaning, and names base64 as the
one channel no shell interprets.

### ⭐ Measured on 2026-08-27, and it is worse than the entry assumed

⛔ **The premise says the caller owns all quoting across both shells. The caller
cannot own it, because the quoting is destroyed in transit.** Measured on this
machine against a real Alpine distro, each hazard already correctly
single-quoted for `sh` before being passed:

| character, POSIX-quoted for sh | PowerShell 7.6.5 | Windows PowerShell 5.1 |
| --- | --- | --- |
| plain `abc` | ok | ok |
| double quote | ok | ⛔ `syntax error: unterminated quoted string` |
| backtick | ⛔ `syntax error: EOF in backquote substitution` | ⛔ same |
| dollar sign | ⛔ expanded: `a$b` arrived as 1 byte, not 3 | ⛔ same |
| tab, backslash, single quote, caret, percent | ok | ok |

⭐ **The single quotes are not reaching the guest.** That is why `$` expands and
a backtick opens a substitution: the payload is re-parsed on the far side as
though it were inside double quotes.

⚠ **The ordinary case is broken, not an exotic one.**
`wsl -d D -u root -- /bin/sh -lc 'echo $PATH'` fails with
`syntax error: unexpected "("`, because `$PATH` expands to a value containing
`/mnt/c/Program Files (x86)/...`.

### ⭐ The channel that does work, measured end to end

An alphabet of letters, digits and `+ / = | > ; . -` survives both hosts
intact. A payload base64-encoded on the Windows side, written to a file in the
guest and sourced there, arrived **byte-exact**: the SHA-256 of the string on
Windows equalled the SHA-256 of the decoded file in the guest, with a single
quote, a double quote, a backtick, a dollar sign and a tab all present.

```text
d305a338...10321970  (the string on Windows)
d305a338...10321970  /tmp/.c   (the decoded file in the guest)
```

⭐ **`Write-DistroFile` already exists and already does this**, added by
`WSL-02`. It carries a body as base64 and refuses a path it cannot carry
safely. ⚠ **The transport half of this entry is therefore mostly written**; what
is left is routing `Invoke-InDistro` through it and adding `-CommandFile` and
`-CommandB64`.

⛔ **Two payloads inside this script must move to that channel at the same
time**, and a reader who fixes only `-Command` will miss them: the smoke probe
in `Invoke-ActionNew` (see `WSL-12`, which is exactly this defect firing) and
the script `Write-DistroFile` itself sends. ⚠ Both are currently written inside
the safe alphabet by hand, which is a constraint no check enforces.

**Approach.** `-CommandFile PATH` or `-CommandB64 STRING`, consistent with the
answer the tree already ships in `scripts/common/write-file.mjs`.

**Decision.** Both, and keep `-Command`. Removing the simple form to force the
safe one is a break for every existing caller for no gain: the simple form is
correct for simple commands.

**Prove.** A command containing a single quote, a double quote, a backtick, a
dollar sign and a tab arrives byte-exact inside the distro.

### Closed 2026-08-27

**What changed.** One function, `ConvertTo-DistroScriptCommand`, builds the
transport, and **every** payload this script sends now goes through it: the
caller's command, the smoke probe in `Invoke-ActionNew`, and the script
`Write-DistroFile` sends. `Invoke-InDistro` takes **bytes** rather than a
string, because `-CommandFile` is read verbatim from disk and a parameter typed
as text would re-encode it. `-CommandFile` and `-CommandB64` are new;
`-Command` stays, as the entry decided.

```text
mkdir -p /tmp&&exec 8>F&&exec 9<F&&rm -f F&&echo B64|base64 -d>&8&&. /dev/fd/9
```

⭐ **The alphabet constraint is now enforced by a machine.** The entry said the
two internal payloads were "hand-written inside the safe alphabet, which is a
constraint no check enforces". They are not hand-written any more, and the
function asserts that the skeleton it built carries nothing outside the
measured alphabet. That assertion is the third mutation below.

### ⭐ The premise's measurement re-run, and two things it adds

⛔ **Written underneath, not edited in.** Re-measured on 2026-08-27 against a
real Alpine and a real Debian distro, from both hosts, because the whole design
rests on it.

**1. The mechanism is expand-then-re-parse, not "as though double-quoted".**
The entry said the payload "is re-parsed on the far side as though it were
inside double quotes". Measured, the distinction matters:

| sent | result |
| --- | --- |
| `printf %s a$HOME` | `a/root`. Expanded, and the result is fine. |
| `printf %s a$PATH` | ⛔ ``syntax error: unexpected "("`` |

⭐ A double-quoted expansion does not re-parse its result; this does. `$HOME`
is harmless because its value has no metacharacter in it, and `$PATH` is not
because WSL appends `/mnt/c/Program Files (x86)/...`. So the hazard is not the
`$`, it is **whatever the value happens to contain**, which is why no alphabet
a caller keeps to can be safe.

**2. A bracket and a single quote DO arrive.** The entry's table did not list
them separately, and `WSL-12` was read here as "the bracket did not survive".
It did:

| POSIX-quoted for sh | 7.6.5 | 5.1 |
| --- | --- | --- |
| `'a(b'` | arrives | ⭐ arrives |
| `'a"b'` | arrives | ⛔ `unterminated quoted string` |

⚠ So `WSL-12` was the **double quote** being dropped on 5.1, which left the
bracket bare and the shell then choked on it. The bracket was never the
problem. That matters because the fix at the time removed brackets from the
probe, which was treating a symptom one character to the left of the cause.

### ⛔ Mutation proof, three of them, each read unpiped

**M1, the defect this entry removes.** `Invoke-InDistro` reverted to sending
the caller's bytes raw, which is the pre-fix transport:

```text
/bin/sh: syntax error: EOF in backquote substitution
  M1 hazard  exit=2
/bin/sh: syntax error: unexpected "("
  M1 echo PATH exit=2
```

Restored, the same two commands give the matching SHA-256 and exit 0.

**M2, the alphabet assertion.** A literal `$` planted in the skeleton:

```text
ERROR: Transport skeleton carries '$', which is outside the alphabet measured
to survive PowerShell to wsl.exe to /bin/sh. Re-measure before widening it.
  M2c exit=1
```

⚠ **And with the assertion disabled, the same mutation exits 0** having built a
skeleton that silently did the wrong thing. That is the pair that makes it a
guard rather than a comment.

**M3, the `&&` after the decode.** Replaced with `;`, and the decoder renamed
to one that does not exist:

| skeleton | exit | what the command did |
| --- | --- | --- |
| `...\|nob64here -d>F;exec 9<F...` | ⛔ **0** | never ran. `THIS-NEVER-RAN` was not printed. |
| `...\|nob64here -d>F&&exec 9<F...` | 127 | never ran, and said so |

⭐ The first row is the forbidden pattern in one line: a step that exits 0
having done nothing it was asked to do.

### ⭐ The mutation found a defect in the fix itself, and it is fixed here

⛔ **The first skeleton left an empty file behind whenever the decode failed.**
M3 is what exposed it: with `echo B64|base64 -d>F&&exec 9<F&&rm -f F`, the
redirect **creates F before the decode runs**, so a guest with no `base64`
short-circuits at the `&&` and never reaches the `rm`. Three zero-byte
`/tmp/.wsl-eph-*` files were sitting in the test distro, timestamped to the
minutes the mutations ran.

The order is now create, open, **unlink**, then decode through the open
descriptor. Measured on both distros and both hosts, decoder present and
decoder missing:

| skeleton | decoder | exit | residue |
| --- | --- | --- | --- |
| unlink first | `base64` | 3 | GONE |
| unlink first | missing | 127 | ⭐ GONE |
| write first | `base64` | 3 | GONE |
| write first | missing | 127 | ⛔ STILL-THERE |

⚠ **Both orders read the same in a diff.** That is why this is written down
rather than quietly corrected.

**Acceptance.** Hazard string `a'b"c` + backtick + `$e` + TAB + `f(g)h`, whose
SHA-256 on Windows is `0f090de5...df553c`. Every code read from the process
that produced it.

| what | pwsh 7.6.5 | Windows PowerShell 5.1 |
| --- | --- | --- |
| `-CommandB64` writes the hazards to a guest file, `sha256sum` in the guest | ⭐ `0f090de5...df553c`, exit 0 | ⭐ the same digest, exit 0 |
| `-Command` carrying the hazards inline | same digest, exit 0 | ⚠ see the residual limit below |
| `-Command 'echo $PATH'` | prints `PATH`, exit 0 | prints `PATH`, exit 0 |
| `-Command 'exit 42'` | 42 | 42 |
| `-CommandFile`, multi-line, quotes and tabs on every line | exit 7, output correct | exit 7, output correct |
| two command switches at once | refused, exit 1 | refused, exit 1 |
| `-CommandB64 'not!base64!'` | refused, exit 1 | refused, exit 1 |
| `.wsl-eph-*` left in the guest `/tmp` afterwards | 0 | 0 |

**End to end on a fresh distro**, which is the way a user reaches it: `New`
with `-CommandB64`, then `-OciEnv` with `-Ephemeral`, then a failing command.

| step | pwsh | 5.1 |
| --- | --- | --- |
| `New -Image alpine:3.22 -CommandB64 ...` | 0, guest digest matches | 0, guest digest matches |
| `.wsl-eph-*` in that distro's `/tmp` | 0 | 0 |
| `New -Image python:3.13-alpine -OciEnv -Ephemeral` | 0, `PYTHON_VERSION=3.13.15`, image `PATH` | the same |
| `New -Command 'exit 33' -Ephemeral` | ⭐ 33 | ⭐ 33 |

⭐ **The smoke probe carries `WSL-12`'s exact line again**, brackets, double
quotes and all, and `New` works on 5.1. That is the strongest statement the
transport can make about itself, and it is now the thing that fails first if
the channel ever breaks again.

### ⚠ What is NOT fixed, and it is one layer above this script

⛔ **Windows PowerShell 5.1 drops a double quote when it builds a child
process's argument list.** Measured with a probe script that reports the
parameter it was handed, so the answer is about the argv boundary and not about
`wsl.exe`:

| how the script was invoked | `-Command` value received |
| --- | --- |
| `powershell -File probe.ps1 -Command a'b"c` + backtick + `d$e` | ⛔ ``a'bc`d$e`` |
| `& .\probe.ps1 -Command $v`, in process | ``a'b"c`d$e`` |
| `pwsh -File probe.ps1 ...` | ``a'b"c`d$e`` |

The quote is gone before this script runs, so nothing in it can recover the
value. ⭐ `-CommandB64` is immune and that is what it is for; it produced the
matching digest from 5.1 in the table above. This is written into
`wsl-ephemeral.md` as a limit of the host rather than an open item, because
there is nothing here to fix.

**Consumers.** ⚠ **Not a break** by
[`../docs/consumers.md`](../docs/consumers.md)'s definition: no path, parameter
or exit code changed meaning, and two parameters were added with defaults. What
changed is that a command which used to be mangled now runs. ⚠ **One observable
difference is worth naming**: `$VAR` in a `-Command` is now expanded by the
guest's login shell rather than in transit, so with `-OciEnv` it expands to the
image's value. That is the correct value and it was not reachable before.

**Pin state.** Held in [`../docs/consumers.md`](../docs/consumers.md).

---

## WSL-09. Bound the smoke probe with a timeout

**Source** issue 3, part 3.13.
**Category** wsl-ephemeral, **Priority** P3, **Effort** S, **Status** done

**Problem.** A distro whose init wedges hangs the script with no output.

**Premise.** Read. The probe absorbs a drvfs race with a bounded sleep loop
**inside** the guest, but the outer `& $wsl ...` call has no time limit.

**Approach.** A bounded wait with a clear message on expiry.
[`../docs/conventions/shell.md`](../docs/conventions/shell.md) section 9.
⚠ Distinguish "it never answered" from "it is not installed"; they are different
facts and belong in different messages.

**Prove.** Against a distro whose init blocks, the script exits non-zero within
the bound with a message naming the timeout.

### Closed 2026-08-27

**What changed.** `Invoke-WslBounded` runs `wsl.exe` with a hard limit and
returns what it printed; `Get-DistroOutput` uses it, so **every question this
script asks a distro** is bounded. `-TimeoutSeconds` defaults to 120.

⚠ **That function is called `Invoke-BoundedProcess` now.** `WSL-13` gave it a
`-FilePath` so the container engine could be asked bounded questions through the
same code, and the name stopped being true. Nothing else about it changed.

⚠ **The scope was widened from "the smoke probe" to "the script's own
questions", deliberately.** `WSL-07` added a second capture-style probe in the
same session, and a bound applied to one of two identical calls is the
one-gated-door pattern this repository keeps finding. ⛔ The caller's
`-Command` is deliberately NOT bounded: a build that runs for an hour is a
legitimate command.

⚠ **A parameter WAS added here, where `WSL-06` declined one.** The difference is
what the number depends on. `WSL-06`'s threshold is derived from the tarball, so
the script can compute it; a timeout depends on the machine, and a user on a
slow disk whose import legitimately takes three minutes has no way to say so
except by telling it. `ValidateRange(5, 3600)` keeps it honest at both ends.

### ⛔ The fix shipped broken for one run, and the acceptance is what caught it

⛔ **`-TimeoutSeconds 15` timed out after 120 seconds.** The constant was
written as `$script:TimeoutSeconds = 120` in the constants block, and **a script
parameter IS a script-scoped variable**, so that line overwrote whatever the
caller passed, on every run, before any function read it.

```text
ERROR: TIMED OUT after 120s waiting for the smoke probe in 'eph-w09wedge'. ...
  exit=1 after 121s
```

⭐ **The refusal was correct and the number was a lie**, which is the shape that
survives review: a test asserting only "it refused" passes over it. The
acceptance asserted the bound it asked for, and that is the only reason this is
a paragraph rather than a defect. The constant is gone and the comment on the
parameter says why there must never be one of that name.

**Mutation proof, against a REAL wedged init rather than a simulated one.** A
rootfs was built whose `/bin/sh` is:

```text
#!/bin/busybox sh
exec /bin/busybox sleep 900
```

| the wait | result |
| --- | --- |
| `WaitForExit([int]::MaxValue)`, the pre-fix shape | ⛔ **STILL RUNNING after 75s**, no output, killed by the harness |
| `WaitForExit($TimeoutSeconds * 1000)` | exit 1 at 16s, on both hosts |

**Acceptance.** Every code read from the process that produced it.

| host | run | result |
| --- | --- | --- |
| pwsh 7.6.5 | `New -Tarball w09-wedged.tar -TimeoutSeconds 15` | ⭐ exit 1 after **16s**, `TIMED OUT after 15s waiting for the smoke probe` |
| Windows PowerShell 5.1 | the same | ⭐ exit 1 after **16s**, same message |
| both | `wsl --list --quiet` after each | byte-identical to before, and the disk directory is gone |
| pwsh 7.6.5 | `New -Image alpine:3.22 -Command 'echo normal-ok' -Ephemeral` | exit 0, unchanged |

### ⚠ Two implementation facts worth keeping

- ⛔ **`ProcessStartInfo.ArgumentList` is .NET Core only.** Windows PowerShell
  5.1 has only the single `Arguments` string, so the join is done in
  `ConvertTo-NativeArgumentString`, which **refuses** an argument containing a
  quote or a backslash rather than escaping one. Every argument this script
  passes is one it built, and the transport skeleton has already been checked
  against an alphabet with no quote in it. Writing an escape for a case that
  cannot occur is how a quoting bug gets written and never exercised.
- ⚠ **The streams are read before the wait.** `docs/conventions/shell.md`
  section 8: `WaitForExit` first deadlocks any child that fills the pipe
  buffer. `ReadToEndAsync` is started on both streams first, and it exists on
  .NET Framework and .NET Core alike.

⚠ **Killing `wsl.exe` on this side does not stop the process in the guest**, so
the timeout path runs `wsl --terminate` before it throws. The distro is then
rolled back by the normal path.

**Consumers.** Adding a parameter with a default. Not a break. ⚠ `New` can now
exit 1 where it used to hang, which is the point.

### ⛔ The door sweep found the bound on one of two identical calls

⛔ **Written under the closure, in the same session, because it is this entry's
scope and it was wrong.** Part (c) of the gate asked "which paths reach the same
operation, and is the guard on all of them". `Write-DistroFile` was on the wrong
side of the line:

| the call | before the sweep | after |
| --- | --- | --- |
| the smoke probe | bounded | bounded |
| the `-Systemd` check | bounded | bounded |
| ⛔ the file write `-OciEnv` and `-Systemd` both use | **unbounded** | bounded |
| the caller's `-Command` | unbounded, deliberately | unchanged |

⭐ **It is a question the SCRIPT asks**, so by this entry's own rule it should
always have been bounded. And it is reached BEFORE the `-Systemd` check that is
bounded, so a distro that wedged after the smoke probe hung there forever.
`Write-DistroFile` now uses `Get-DistroOutput`, which also puts the guest's own
complaint into the error message instead of leaving it loose on the console.

⚠ **What is still not bounded, named rather than implied:** `--import`,
`--terminate` and `--unregister`. They are host-side calls to the WSL service
rather than execution inside a guest, none was observed to hang in this session,
and bounding `--import` would be wrong outright: a five-minute import of a large
rootfs is legitimate.

---

## WSL-10. Retry a generated name on collision

**Source** issue 3, part 3.10.
**Category** wsl-ephemeral, **Priority** P3, **Effort** S, **Status** done

**Problem.** A generated name that collides throws instead of drawing again.

**Premise.** Read. `New-DistroName` appends four characters from a 36-symbol
alphabet, so the space is 36^4 = 1,679,616. For a caller-supplied `-Name`,
throwing is correct behaviour and stays.

**Approach.** Retry a bounded number of times for a **generated** name only.

**Prove.** With the generator stubbed to a constant, the first call succeeds and
the second draws again rather than throwing; after the retry bound it throws
with a message saying so.

### Closed 2026-08-27

**What changed.** `Resolve-NewDistroName` is the one place that decides what a
collision means, and the two cases are genuinely different: a name the **caller**
gave still throws, and a name the **script** drew is drawn again, up to 8 times.

⭐ **The existence check moved into it.** `Invoke-ActionNew` used to resolve a
name and then test it against the registered list itself, which is why the two
cases could not be told apart: by the time the check ran, the information about
where the name came from was gone.

**Acceptance.** The generator stubbed, as the entry asks. Each code read from
the process that produced it, with `eph-w10taken` registered throughout.

| stub | result |
| --- | --- |
| draws `eph-w10taken`, then `eph-w10free` | ⭐ `! generated name 'eph-w10taken' is already taken; drawing again (1 of 8)`, then `* 'eph-w10free' is up`, command ran, **exit 0** |
| draws the constant `eph-w10taken` | 8 warnings, then `ERROR: Could not draw an unused distro name in 8 attempts.`, **exit 1** |

**Mutation proof.** The retry bound was set to 1, which is the pre-fix
behaviour of drawing once and giving up, against the same sequence stub that
succeeds with the retry:

```text
  ! generated name 'eph-w10taken' is already taken; drawing again (1 of 1)
ERROR: Could not draw an unused distro name in 1 attempts. ...
  mutation exit=1
```

⭐ The identical scenario exits 0 with the retry in place and 1 without it, so
the retry is what moves it.

**The caller-supplied case is unchanged, and that was checked rather than
assumed.** Both hosts:

| host | `New -Image alpine:3.22 -Name eph-w10taken` | exit |
| --- | --- | --- |
| pwsh 7.6.5 | `ERROR: Distro 'eph-w10taken' already exists. Choose another -Name or remove it first.` | 1 |
| Windows PowerShell 5.1 | the same message | 1 |
| Windows PowerShell 5.1 | a normal generated name, `New -Command 'echo generated-ok' -Ephemeral` | 0 |

⚠ **The registered list is read once, not once per draw.** Re-reading would
cost a `wsl.exe` call per attempt to defend against a distro appearing in the
microseconds between two draws, and `--import` refuses that case anyway.

**Consumers.** No parameter, path or exit code changed. A command that used to
fail on a one-in-1.68-million collision now succeeds. Not a break.

---

## WSL-11. An `Enter` action

**Source** issue 3, part 3.12. ⚠ Missed on the first pass through the issue and
added on review; the fourteen findings map to eleven entries and three
documentation notes, and this was briefly neither.
**Category** wsl-ephemeral, **Priority** P3, **Effort** S, **Status** done

**Problem.** The summary after `New` tells the user to run `wsl -d DISTRO` by
hand, so the interactive path is the only one that is not first class.

**Premise.** Read. `ValidateSet` is `New, Run, List, Remove, Purge`.

**Approach.** `-Action Enter`, honouring `-User`, giving one place to add
further interactive handling later.

**Prove.** `-Action Enter -Name eph-...` attaches an interactive shell as
`-User`, and a name that is not registered is refused with a clear message.

### Closed 2026-08-27

**What changed.** `Enter` joins the `ValidateSet` and `Invoke-ActionEnter` hands
`wsl.exe` the distro and the user **and nothing else**. The summary `New` prints
now says `-Action Enter -Name ...` instead of `wsl -d ...`, which is the half of
the Problem statement that was about the tool telling users to leave it.

⛔ **Sending no command is the design, not an omission.** Routing this through
`Invoke-InDistro` would produce a shell reading a script, and a shell reading a
script ignores the terminal. `-TimeoutSeconds` does not apply either: a person
sitting in a shell is not a wedged init.

### ⚠ What the acceptance can and cannot reach

⛔ **This harness has no terminal to allocate, so a human TTY session was not
driven.** Said plainly rather than implied. What was driven is everything the
TTY sits on top of: stdin belongs to the guest's shell, `-User` selects the
account, and the shell's exit code comes back.

⭐ **stdin came from a file, not a PowerShell pipe**, and the first attempt is
worth recording because it failed for a reason this repository already
documents. Piping `exit 9` from PowerShell produced:

```text
-sh: exit: line 1: Illegal number: 9
```

⚠ That is `docs/conventions/shell.md` section 1: PowerShell's native-command
pipe is not byte-exact and appends CRLF, so the guest read `exit 9<CR>`. Not a
defect in `Enter`; a defect in the first acceptance, and it would have been
recorded as a mysterious exit 2 by anyone who did not know that rule.

**Acceptance.** Both hosts, stdin from an LF file, each code read from the
process that produced it.

| what | pwsh 7.6.5 | Windows PowerShell 5.1 |
| --- | --- | --- |
| stdin reaches the shell: `id -un`, `cat /proc/1/comm`, `exit 9` | `root`, `init(eph-w10tak...)`, **exit 9** | identical |
| `-User tester` honoured, `id -un` | `tester`, exit 0 | identical |
| held 12s past a `-TimeoutSeconds 5` bound | ⭐ `still-here`, **exit 3 after 12s** | exit 3 after 13s |
| `-Name eph-does-not-exist` | exit 1, message names `-Action List` and `-Action New` | |
| `-Name podman-machine-default` | ⭐ exit 1: prefix-forced to `eph-podman-machine-default`, which is not registered | |
| `-Name` omitted | exit 1, `Action Enter requires -Name.` | |

⭐ The third row is the one that proves the timeout exclusion, and the fifth is
the safety property: `Enter` cannot reach a distro this script did not create.

**Mutation proof, both guards.**

| planted | result |
| --- | --- |
| the registration check removed | ⛔ WSL's own `Wsl/Service/WSL_E_DISTRO_NOT_FOUND`, and **exit -1**, which is not one of this script's documented codes |
| `Enter` sends `-- /bin/sh -lc :`, the shape `Run` has | ⛔ **stdin ignored entirely**: nothing printed, **exit 0** instead of 9 |

⭐ The second row is the one worth keeping. "It sends no command" reads like a
detail and it is the whole feature: with a command, everything the user types
goes nowhere and the session exits immediately, successfully.

⚠ **`-Ephemeral` was NOT added to `Enter`.** Attach-then-destroy is a plausible
thing to want and nobody has asked for it. `WSL-03` set the precedent for
machinery with no caller; it is one line to add when something needs it.

**Consumers.** Adding an action to a `ValidateSet`. Nothing renamed, no exit
code changed meaning. Not a break.

---

## WSL-13. Report what the machine is holding, and offer rather than act

**Source** Issue 1, request 1. An agent found 554 images at 31.35 GB, an exited
container holding 5.08 GB and five orphaned volumes on a machine where it had
not run a container, and had to assemble that picture from six podman commands
by hand.
**Category** wsl-ephemeral, **Priority** P2, **Effort** M, **Status** done

**Problem.** The tool reports on the distros it made and nothing else. The
resources an agent actually trips over are the container engine's, and the
engine is shared with everything else on the machine. There was no one command
that said what is here, so every agent that wondered wrote its own sequence.

**Premise.** ⛔ **The numbers in the request are not re-derived and are not
claimed here.** They describe that machine on the day it was written, and this
one had been pruned before this session started: `podman system df` answered
zero on every row. What was verified is that every command the action runs works
on this host and that the report is correct on a machine with one distro
registered.

⚠ **The mechanism behind the request's larger claim is checkable and was
checked.** This engine's `system df` prints exactly three rows, Images,
Containers and Local Volumes, and no build-cache row, so a prune can free more
than the figure it reports. That is stated; the specific gigabytes are not.

**Approach.** `-Action Resources`, read-only, in three parts that say on every
line which is which: what this script made, with the size of each distro's disk;
what else WSL has registered, named and never touched; and what the engine is
holding, from `system df` plus a dangling-image count and an unused-volume
count.

⛔ **It offers and it does not do.** Every cleanup command is printed and none is
run, including this script's own `-Action Purge`. Reclaiming somebody's disk is
their decision and an agent's job is to hand them the numbers and ask.

⛔ **The engine calls are bounded.** `Invoke-WslBounded` becomes
`Invoke-BoundedProcess` and takes a `-FilePath`, because podman on Windows talks
to a VM and a machine that is starting or wedged leaves the client waiting with
nothing on either stream. One bounded runner rather than two: a second
implementation of the read-before-wait ordering is a second place to get it
wrong. A wedged engine cannot stop the half of the
report this script actually owns.

⚠ **An unreadable directory produces a dash rather than a zero**, and the total
is withheld with the count of what could not be read beside it. The tool page
says why.

**Decision.** No `--json`. This tool has no machine-readable surface anywhere,
and adding one for a single action is a half-surface that reads as a promise.
⭐ The action that a caller does consume a value from is `WSL-14`, and it emits
one line rather than a document.

**Consumers.** Adding an action to a validate-set. Nothing renamed and no exit
code changed meaning. Not a break.

**Prove.**

```bash
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral.ps1 -Action Resources
```

Exit 0, three sections, and a distro's disk size that agrees with what this
tool's own page measured for the same image.

---

### Closing

**Closed 2026-08-29T15:18:27Z.** Driven against a real `alpine:3.22` distro
created for the purpose and removed afterwards.

```text
==> What this script made, under %LOCALAPPDATA%\wsl-ephemeral
  eph-toolkit-probe                              76.0 MiB
  * 76.0 MiB held by this script, across 1 distro(s) and 0 tarball(s)
```

⭐ **76.0 MiB is the figure `wsl-ephemeral.md` already carried** for an
`alpine:3.22` VHDX, measured independently on 2026-08-27 for the disk-space
preflight. Two measurements taken for different reasons agreeing is what makes
this one evidence rather than an echo.

⛔ **A format-string defect was caught by the code refusing rather than
printing.** The first version wrote alignment as `{1,>6}`, which .NET does not
accept, and the action exited 1 naming the offset instead of printing a wrong
table.

⚠ **Teardown verified by counting.** The probe distro was removed and the image
this session pulled was removed with it; `podman system df` returned to zero
across all three rows.

---

## WSL-14. Answer what a distro reaches the host at, without creating a distro

**Source** Issue 1, request 2, raised by a consumer whose page is
`Azathothas/bit-cli` at `docs/containers.md`.
**Category** wsl-ephemeral, **Priority** P2, **Effort** M, **Status** done

**Problem.** A caller that needs to talk from a distro back to the host has to
build a throwaway VM, read `/proc/net/route` inside it and decode little-endian
hex, to answer a question the host already knows. In mirrored mode the answer is
the loopback address and every caller's branch disappears.

⛔ **And the failure it guards against is silent.** In NAT mode a host service
bound to `127.0.0.1` is not reachable from the distro. A fixture on loopback
simply never receives a connection, and nothing on either side says why.

**Premise.** ⭐ **Measured on this machine on 2026-08-29.** `.wslconfig` sets
`networkingMode=NAT`; the host has one WSL adapter, `vEthernet (WSL (Hyper-V
firewall))`, at `172.23.96.1`; and a real `alpine:3.22` distro read `016017AC`
from `/proc/net/route`, which is that address.

⚠ **The consumer's page recorded the same address on its own machine**, which is
the same machine, so the two agree rather than corroborate.

**Approach.** `-Action HostAddress`. Read the mode from `%USERPROFILE%\.wslconfig`
and answer from it: loopback for mirrored, the WSL adapter's IPv4 address for
NAT, and a refusal for bridged.

⛔ **The parse refuses a commented line, checks the section, and takes the last
key.** Getting any of those wrong answers `mirrored` on this host, which runs
NAT, and the wrong answer here is the expensive one: loopback is plausible and
never connects. The tool page carries the shape of a real configuration file
that makes each of the three necessary.

⛔ **Bridged is refused rather than guessed.** The distro is on the LAN and
reaches the host at whichever host address is on that switch, which is a choice
rather than a lookup.

⭐ **The address is the only thing on stdout**, so a caller can assign it.
⚠ Getting that right needed two changes that look like style and are not. Write-Host
is invisible in-process and lands on stdout out of process, so the explanatory
lines go to stderr through a helper; and the script's final error line moved to
stderr too, because a caller assigning stdout would otherwise get the string
beginning `ERROR:` where an address goes.

**Decision.** ⛔ **No `-PortForward`, and the request offered documentation as
the alternative.** Forwarding a port on Windows means `netsh interface
portproxy`, which needs an elevated session and leaves a rule on the machine
after the tool exits. This tool creates nothing it cannot remove and asks for no
elevation. ⭐ `HostAddress` answers the question the port forward was wanted for:
bind the host service to that address instead of to loopback, which the action
says in as many words every time it runs in NAT mode.

**Consumers.** Adding an action, plus the error line moving to stderr. Neither
is a break by the definition in
[`../docs/consumers.md`](../docs/consumers.md); the stderr move is recorded
there because it is the one change a caller could observe.

**Prove.**

```bash
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral.ps1 -Action HostAddress
```

Exit 0, and stdout is one address that equals what a real distro reads from
`/proc/net/route`.

---

### Closing

**Closed 2026-08-29T15:18:27Z.** Verified against a real distro rather than
against the host alone.

```text
guest /proc/net/route -> 172.23.96.1 (016017AC)
HostAddress          -> 172.23.96.1
MATCH
```

⭐ **The stdout property was verified both ways**, because the two disagree by
default: captured in-process and captured through `pwsh -File`, both gave
`172.23.96.1` and nothing else.

⭐ **All four parse branches were exercised, by pointing the script at fixture
profiles rather than by reconfiguring this machine.** Switching a host's WSL
networking mode restarts every distro on it, including the podman machine, which
is not a thing to do to somebody's session to test a branch. The parse reads
`%USERPROFILE%\.wslconfig`, so a scratch `USERPROFILE` exercises it end to
end. ⛔ Exit codes read unpiped:

```text
live NAT under two commented alternatives   nat        172.23.96.1   rc=0
live mirrored under a commented NAT         mirrored   127.0.0.1     rc=0
live bridged                                bridged    (refused)     rc=1
the key under [experimental] only           nat        172.23.96.1   rc=0
```

⭐ **The third row is the guard doing its job** and the fourth is the section
check: a key in the wrong section is not read, and the answer falls back to the
documented default with `Source` saying so.

⚠ **What is still NOT measured is WSL's own behaviour under mirrored mode**, that
a host service on loopback is reachable from a distro there. That is a claim
about WSL rather than about this script, it is what Microsoft documents, and
this machine cannot be put into that mode to check it without restarting every
distro on it.

---

## WSL-15. A launcher, so one fetch is enough

**Source** Issue 1, request 3, which asks whether the wrapper in
`Azathothas/TEMPLATE` should be copied here and improved to justify existing
beside the real script.
**Category** wsl-ephemeral, **Priority** P2, **Effort** M, **Status** done

**Problem.** Running this tool from another project takes five steps that are
easy to get subtly wrong: resolve a commit, fetch that exact revision to a file,
do not pipe it into a shell, clear the mark Windows puts on a downloaded file,
and then run it. The page said all of that and a reader had to do all of it.

**Premise.** Read in a clone of `Azathothas/TEMPLATE` at `6eaf4b5`. Its wrapper
pins a commit and a digest of this repository's script, verifies before
executing, caches by ref, parses the file as PowerShell first, and forwards
every argument.

⛔ **Copying it verbatim is wrong here and the reason is structural.** A pin
inside the repository that owns the file can only ever name one of its own
ancestors, so it is stale the moment the file it points at changes, and the file
sitting next to it is the newer one.

**Approach.** A launcher that resolves in three steps and takes the first hit:
an explicit local file, the sibling beside it, then a revision the caller named.
⛔ There is no default revision: with no sibling and no ref it refuses and prints
the command that resolves one.

What it adds over a download and a `pwsh`: a moving ref refused by shape, a
digest mismatch as a hard stop rather than a warning, the file parsed as
PowerShell before it can run, the `Zone.Identifier` stream cleared, a cache keyed
by ref, and an implausibly small download refused.

⭐ **`-LauncherAddToPath` is the request's third item and it needs dot-sourcing.**
A child process cannot change the environment of the session that ran it, so run
normally the launcher installs and prints the line; dot-sourced, it does the
assignment in the caller's session. Claiming to have changed PATH from a child
process would be reporting a result it never read. ⛔ Dot-sourcing is refused for
every other use, because the wrapped script calls `exit`, which would end the
host session.

**Decision.** ⛔ **Every option it understands is prefixed `-Launcher`.** A
wrapper that took `-Install` would break the day the wrapped script grew an
`-Install`, and this one must never restate that script's parameter list.

**Consumers.** ⚠ It does not replace the wrapper in `Azathothas/TEMPLATE` and
does not move any pin. Which of the two that repository keeps is its decision.

**Prove.**

```bash
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral-launcher.ps1 -Action HostAddress
```

Exit 0, forwarding a named parameter to the sibling, with stdout carrying the
address and nothing else.

---

### Closing

**Closed 2026-08-29T15:18:27Z.** Driven on all four paths: sibling, fetch with a
matching digest, fetch with a wrong digest, and no sibling with no ref.

```text
sibling, -Action HostAddress        stdout = 172.23.96.1            rc=0
fetch 7127ff707be9 + digest         digest matches, -Action List    rc=0
fetch with a wrong digest           DIGEST MISMATCH, copy deleted   rc=1
a branch as the ref                 refused by shape                rc=1
no sibling and no ref               refused, prints the gh command  rc=1
```

⛔ **Two defects were found by driving it and neither was visible in review.**

1. **Splatting the forward list passed every argument positionally.** `-Action`
   bound as the VALUE of `-Action` and the script refused it against its own
   validate-set. Measured across five ways of building the array: an ordinary
   array with `+=`, a `Where-Object` filter, a range slice and an `[object[]]`
   parameter all forward parameter NAMES; an `ArrayList.ToArray()` does not.
   Every element is a string in all five, so nothing about the values explains
   it. The measurement is in the launcher's own page.
2. **The launcher's progress lines corrupted the value.** Write-Host from a child
   process reaches the caller's stdout, so capturing
   `-Action HostAddress` through the launcher returned a progress line ahead of
   the address. Every line the launcher prints now goes to stderr and it writes
   nothing to stdout at all.

⚠ **The download mark was not observed on this host.** `Invoke-WebRequest` did
not attach a `Zone.Identifier` stream to the fetched file, so the clearing path
reported nothing to clear, which is what it is written to do. ⭐ What would have
had to be true for it to fire: a copy fetched by a browser, by
`Start-BitsTransfer`, or unpacked from a downloaded archive, pointed at with
`-LauncherLocal`.

---

## WSL-16. the file channel makes a consumer normalise and encode by hand

**Source** [issue 5](https://github.com/Azathothas/ToolKit/issues/5), complaint 1.
**Category** wsl-ephemeral, **Priority** P1, **Effort** M, **Status** done

---

## Problem

A consumer wanting to run a script in a distro had to write it to a file, check
its line endings, convert it with `base64 -w0`, and pass the result as
`-CommandB64`. The issue's own transcript shows all four steps, plus two `sed`
passes to inject a URL into the script and an `od -c | grep -c` to count
carriage returns.

## Premise

⭐ **Measured, not read.** `-CommandFile` already existed and already read a file
verbatim, so three of those four steps were already unnecessary and nobody knew.
What it did with CRLF was warn and send the bytes anyway, so the one step that
actually mattered was the one it did not do.

## Approach

`Resolve-CommandBytes` gains `ConvertFrom-CommandFileBytes`, which repairs the
**copy in transit** and never the file: CRLF to LF, a UTF-8 byte order mark
removed, and UTF-16 refused by name because `/bin/sh` stops at the first NUL.
`-Verbatim` turns all three off. `-ScriptArg NAME=VALUE` prepends POSIX-quoted
assignments so nothing runs `sed` over a payload, and `@hostaddress` in a value
resolves through the same function `-Action HostAddress` uses.

⛔ It must not rewrite the file on disk. That distinction is the whole design.

## Consumers

`Azathothas/bit-cli`'s `docs/containers.md` tells its reader to do the base64
step by hand. Nothing breaks: `-CommandB64` is unchanged. The page is now longer
than it needs to be, which is that repository's to decide.

## Prove

```bash
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral-selftest.ps1
```

Exit 0, with the eight `ConvertFrom-CommandFileBytes` cases and the eight
`ConvertTo-ScriptArgPrologue` cases passing.

---

## Closing

**Closed 2026-08-30T06:00:00Z.** Both parameters shipped, with sixteen cases in
the selftest and one end-to-end run against a real `alpine:3.22` distro using a
15-CRLF file and `-ScriptArg "URL=https://@hostaddress:8443/"`.

```text
  * .tmp/guest-drive.sh has 15 CRLF line ending(s); the copy being sent uses LF.
  * the file on disk was NOT modified. Pass -Verbatim to send its bytes exactly.
  * -ScriptArg URL : @hostaddress resolved to 172.23.96.1.
00:00:06.123 out  SCRIPTARG URL=https://172.23.96.1:8443/
```

---

## WSL-17. the launcher makes every consumer resolve a commit and a digest by hand

**Source** [issue 5](https://github.com/Azathothas/ToolKit/issues/5), complaint 2.
**Category** wsl-ephemeral, **Priority** P1, **Effort** M, **Status** done

---

## Problem

`-LauncherRef` and `-LauncherSha256` both had to be resolved by the caller, with
`gh` and `sha256sum`, and re-resolved every time either moved. The issue asks
why a consumer cannot choose to trust the launcher instead.

## Premise

⚠ **The pin rule is right and the cost of it was real.** "Pin a commit, never a
branch" prevents running code nobody reviewed. What it does not require is that
the CALLER be the one who types the commit.

## Approach

`-LauncherRef auto` resolves the branch to a commit once, records the commit and
its digest in a lock file, and reads the lock forever after. `-LauncherRef
latest` re-resolves every run and warns every run. `-LauncherSha256 auto` reads
a digest from the API for whatever ref is in play.

⛔ Neither keyword may fetch a branch. Both resolve to a commit first, so the URL
downloaded always names an immutable object.

⛔ No `gh`. `Invoke-WebRequest` ships with every supported PowerShell, and a
convenience path needing another tool installed replaces one setup step with
another.

## Decision

**An explicit `-LauncherRef` now wins over the sibling beside the launcher.**
Ruled 2026-08-30. The alternative was leaving the order alone and documenting
the trap, which is what `Azathothas/bit-cli` had to do: it deletes the sibling
before every call and wrote the workaround into its own page. A caller who names
a revision means that revision. Recorded as a break in `../docs/consumers.md`.

## Consumers

All three rows checked. `bit-cli`'s workaround becomes unnecessary;
`Azathothas/TEMPLATE`'s wrapper fetches into a directory with no sibling; the
vendored copy fetches nothing.

## Prove

```bash
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral-launcher.ps1 -LauncherRef auto -LauncherInstallDir DIR -Action HostAddress
```

Exit 0, a lock written at `DIR`, and the address on stdout. A second run reads
the lock and resolves nothing.

---

## Closing

**Closed 2026-08-30T06:00:00Z.** Run against `main` on this machine. The digest
`auto` computed is `ab4f6bd6c040bb9d...`,
⭐ which is the same value the issue's own transcript shows a consumer having
pasted by hand.

```text
  * Azathothas/ToolKit@main is 8efe6e02b1ce
  * recorded in ...\wsl-ephemeral.lock.json. Later runs read it and ask GitHub nothing.
  ! you have just trusted whatever 'main' pointed at. Nobody reviewed it on your behalf.
==> Fetching Azathothas/ToolKit@8efe6e02b1ce
  * digest matches
```

Both guards were mutation-proved: a lock with a zeroed digest produced
`DIGEST MISMATCH` and exit 1, and a lock naming another repository was refused
by name.

---

## WSL-18. a command that prints nothing is indistinguishable from one that has died

**Source** [issue 5](https://github.com/Azathothas/ToolKit/issues/5), complaint 3, and the mockup it links.
**Category** wsl-ephemeral, **Priority** P1, **Effort** L, **Status** done

---

## Problem

⭐ **The expensive one.** A consumer ran a script in a distro, it produced no
output for a long time, an agent waited on a matcher that never fired, and the
run ended in `Exit code 137` and a manual kill. Nothing in the output
distinguished a slow download, a progress bar redrawing with a carriage return,
a deadlock, a dead distro, or a prompt waiting on stdin that would never arrive.

## Premise

The issue proposes a `ts`-style timestamp on every line and a tick, emitted by
the watcher, with nothing injected into the guest. ⚠ **The mockup it links is
explicitly a mockup**: it says in its own banner that no code exists and every
transcript in it is hand-written. So it was read as a design to select from
rather than a specification to implement.

## Approach

`Invoke-InDistro` gains one branch. With the log on, the child's streams are
relayed rather than inherited, each line is stamped and tagged, a carriage
return terminates a line, an unterminated line is flushed after two seconds, and
`-TickSeconds` of silence produces a heartbeat. `-NoTimestamps` restores the
inherited-handle path byte for byte.

⛔ It must not design a ceiling: `-TimestampMode` and `-TimestampFormat` cover
`tss`'s surface rather than inventing one.
⛔ It must not build the rest of the mockup. No JSONL, no dashboard, no
capability probe, no `podman` adapter. Those are a different tool.

## Consumers

⛔ **A break.** stdout now carries a prefix. Recorded in `../docs/consumers.md`
with `-NoTimestamps` named as the restore.

## Prove

```bash
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral.ps1 -Action New -Image alpine:3.22 -Name eph-tk-drive -Ephemeral -Force -CommandFile .tmp/guest-drive.sh -TickSeconds 3
```

Exit 3, the command's own code. Ticks on stderr during the quiet stretch, and
`out~` lines for the carriage-return progress meter and the unterminated prompt.

---

## Closing

**Closed 2026-08-30T06:00:00Z.** Driven against a real `alpine:3.22` distro.

```text
00:00:00.145 out  hello from stdout
00:00:02.300 out~ Continue? [y/N]
00:00:05.121 out~ 10%
00:00:06.123 out  100%
00:00:09.339 tick 3s silent | elapsed 9s | out 7 lines 89 B | err 1 lines 18 B | distro Running
00:00:13.122 out  after the quiet
EXIT=3
```

⛔ **Two defects were found while building this and both are in
`../docs/HISTORY/wsl-ephemeral.md`:** a byte-array concatenation that threw, and
a tick advancing the delta clock so a five-second gap read as `+0.619`. The
first was found by the selftest on its first run; the second only by driving a
real distro, which is why part (b) of the gate exists.

⚠ **What was deliberately not built** is in `WSL-22`.

---

## WSL-19. nothing bounds the caller's command, so a hung run ends in a kill

**Source** [issue 5](https://github.com/Azathothas/ToolKit/issues/5), the transcript's `Exit code 137`.
**Category** wsl-ephemeral, **Priority** P2, **Effort** S, **Status** done

---

## Problem

`-TimeoutSeconds` bounds the questions the script asks a distro for itself and
deliberately not `-Command`. That is right as a default and it left the caller
with no bound at all, so step 6 of the reported failure is a person killing the
session by hand.

## Approach

`-CommandTimeoutSeconds`, opt-in, no default. On expiry the child is killed, the
distro is terminated so the work in the guest stops too, and the exit code is
124.

⛔ It must not acquire a default. A build that runs for an hour is a legitimate
command.

## Consumers

None: a new parameter with no default cannot change an existing call.

## Prove

```bash
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral.ps1 -Action Run -Name eph-tk-modes -CommandTimeoutSeconds 5 -TickSeconds 2 -Command 'echo starting; sleep 600; echo never'
```

Exit 124, in about five seconds rather than ten minutes.

---

## Closing

**Closed 2026-08-30T06:00:00Z.**

```text
rc=124
elapsed=5s
00:00:05.031 tick TIMED OUT after 5s: -CommandTimeoutSeconds 5 was reached.
             Terminating 'eph-tk-modes'. The exit code is 124, which is what
             coreutils' timeout reports.
```

---

## WSL-20. one API host is a single point of failure

**Source** the operator, 2026-08-30, while `WSL-17` was being built.
**Category** wsl-ephemeral, **Priority** P2, **Effort** S, **Status** done

---

## Problem

`WSL-17`'s convenience path asks api.github.com two questions. Unauthenticated
that endpoint allows 60 requests an hour per address, and on some networks it or
`raw.githubusercontent.com` is not reachable at all. A convenience that fails
closed on a rate limit is a convenience nobody relies on.

## Premise

⭐ **Measured on 2026-08-30**, not assumed. `raw.githubusercontent.com`,
`api.github.com` and `api.gh.pkgforge.dev` each served 96,170 bytes for commit
`8efe6e02`, all hashing to the same SHA-256. The proxy's own `rate_limit`
reported 5000 core requests remaining against the anonymous 60.

## Approach

Every API question and the file download try each host in order and stop at the
first that answers. The host that answered is named on stderr when it is not the
first.

⛔ A bad answer is not tried past: the caller checks the shape of what came back,
so a proxy returning a login page fails the same way the source would.

## Consumers

None. The URL a caller passes has not changed; only what happens when it cannot
be reached.

## Prove

```bash
pwsh -NoProfile -File LAUNCHER_WITH_A_DEAD_FIRST_HOST -LauncherRef auto -LauncherInstallDir DIR -Action HostAddress
```

Exit 0, with the fallback named, and the same commit and digest the healthy path
produces.

---

## Closing

**Closed 2026-08-30T06:00:00Z.** Mutation-proved: a copy of the launcher with
`api.github.invalid` as the first host resolved through the proxy and produced
an identical lock.

```text
  ! api.github.invalid did not answer; this came from api.gh.pkgforge.dev instead.
  * Azathothas/ToolKit@main is 8efe6e02b1ce
  * digest matches
```

⚠ **The proxy enforces a user-agent allowlist**, which was found by measuring
rather than by reading its page: `wsl-ephemeral-launcher` and `Mozilla/5.0` both
answered HTTP 420 while `curl/8.21.0` answered 200. The agent sent now carries a
compatibility token beside the tool's real name.

---

## WSL-21. `wsl-ephemeral.ps1` is 2,792 lines in one file

**Source** the operator, 2026-08-30: is PowerShell still the right language, and should the file be split?
**Category** wsl-ephemeral, **Priority** P2, **Effort** L, **Status** done

---

## Problem

The script is one file of 2,792 lines, counted on 2026-08-30. It was 1,980 on
2026-08-29, so this session grew it by 41 percent. A reader looking for the
stream log reads past the safety model, the OCI config and the disk preflight to
reach it.

## Premise

⚠ **Read rather than measured.** Nothing has been shown to be slower, harder to
change or more defect-prone because of the length. The two defects this session
found were both logic, not structure.

## Decision

⭐ **Recommendation: stay in PowerShell, and do NOT split the file yet.**

**On the language.** It is the right one and the reasons are not preference.
`wsl.exe` is a Windows binary; the script reads `.wslconfig`, enumerates network
interfaces through .NET, clears an alternate data stream, and drives a process
with two redirected pipes. A Rust or Go rewrite buys a single binary and pays
for it with a build step, a release pipeline, a platform matrix and a download
before anything runs. ⛔ This repository publishes no artefacts, so a compiled
tool would be the first thing it had to publish, and that is a much larger
change than the one being considered.

**On splitting.** A single file is what makes the launcher's whole contract
possible: one URL, one digest, one thing to verify. Splitting into modules means
a consumer fetches N files and verifies N digests, or this repository starts
publishing a concatenated release, which is the artefact it does not publish.
⚠ **The honest reading is that the file is long and not yet unmanageable**, and
the cost of the split lands entirely on consumers.

⭐ **What would change the answer**, so this is re-derivable rather than
re-arguable: a second Windows tool needing the same helpers, at which point a
shared module has two callers instead of one and stops being speculative.

## Prove

---

## Closing

**Closed 2026-08-30T08:40:00Z.** ⭐ **The operator ruled on 2026-08-30: stay in
PowerShell, and DO split the file.** Half the recommendation was accepted and
half was overruled, and the half that was overruled is the interesting one.

**What was built.** The 2,792-line file is now 27 parts under
`scripts/windows/wsl-toolkit/{src,core,libs}`, joined by `build.ps1` in the order
`bundle.manifest` names, into a tracked product at
`scripts/windows/wsl-toolkit/wsl-toolkit.ps1`. The tool was renamed from
`wsl-ephemeral` in the same change, at the operator's instruction.

⭐ **The split was proved byte-identical before anything else changed**, which is
what made everything after it a normal diff rather than a rewrite nobody could
review:

```text
rejoined 139470  original 139470
BYTE-IDENTICAL: the 23 parts rejoin to the original exactly
```

```text
bundle minus BOM and banner: 139467   original minus BOM: 139467
PROVED: the built bundle is the original file, plus a 14-line generated banner and nothing else
```

**And it was proved behaviourally too**, before the rename, by running both
files:

```text
List         exit 0/0  stdout same  stderr same
HostAddress  exit 0/0  stdout same  stderr same
Resources    exit 0/0  stdout same  stderr same
```

### The acceptance: lines per responsibility

⛔ **The entry's acceptance was to produce the numbers rather than to make the
change, and the numbers are what the ruling changed.** They are here because the
entry asked for them and because they are the evidence the split was worth doing.

| | before | after |
| --- | --- | --- |
| files | 1 | 27 parts, plus the product |
| lines in the largest thing a reader opens | 2,792 | 592 (`core/stream-log.ps1`) |
| median file | 2,792 | 133 |
| lines total | 2,792 | 4,170 across the parts, 4,184 built |

```text
src/                          core/                          libs/
  229  00-help.ps1              109  action-address.ps1         72  bounded-process.ps1
  186  10-parameters.ps1        133  action-doctor.ps1          27  console.ps1
   63  20-prelude.ps1           204  action-new.ps1            146  events.ps1
  101  99-main.ps1               57  action-purge.ps1           27  native-args.ps1
                                220  action-resources.ps1       52  process.ps1
                                 95  action-run.ps1            125  redact.ps1
                                152  applicability.ps1         224  stamp.ps1
                                246  command-channel.ps1
                                118  disk.ps1
                                141  distro-exec.ps1
                                 87  distro-file.ps1
                                339  distro-run.ps1
                                149  rootfs.ps1
                                164  safety.ps1
                                592  stream-log.ps1
                                 75  wsl-host.ps1
```

⚠ **The total grew by 48 percent and that is not the split's cost.** This session
also added thirteen parameters, an action, an event log, a redaction layer and an
applicability table. The split itself added the manifest, the builder and the
banner.

### ⛔ What was WRONG in the recommendation above, corrected here

The Decision section argued that splitting was impossible because a single file
is what makes the launcher's contract possible: one URL, one digest, one thing to
verify. ⭐ **That premise was right about the CONSTRAINT and wrong about the
CONCLUSION.** The constraint holds completely; what does not follow is that the
sources have to be the artefact. A build step separates them, and the product is
tracked so a consumer fetching one raw URL still needs no build.

⛔ **The second half of the argument was also wrong, and it was wrong about this
repository rather than about PowerShell:** "this repository publishes no
artefacts, so a compiled tool would be the first thing it had to publish". It now
publishes one, the release pipeline took an afternoon rather than a project, and
the thing published is a script rather than a binary. ⚠ That does not reopen the
language question: PowerShell is still right for the reasons the Decision gives,
which are about what the tool talks to, and none of those changed.

### What makes the split safe rather than merely tidy

| the failure | what stops it |
| --- | --- |
| a part edited and never rebuilt | `check-gate`'s `wsl-toolkit bundle`, in both halves and in CI: it rebuilds and compares bytes |
| the product edited by hand | the same check, from the other direction |
| a part added and never listed | the build enumerates the three directories and refuses a set that disagrees with the manifest, both ways |
| a part moved above `param()` | the build asserts the result parses and has a param block |
| a renamed parameter reaching a consumer | `surface.lock`, compared on every `-Test` |
| a fragment losing the suppressions that live in the param block | the analyzer runs over the PRODUCT, and the bundle check is what makes that cover every part |

```text
wsl-toolkit build: <the tool directory; the absolute path is elided, this repository is public>
  ok    wrote wsl-toolkit.ps1 (209,922 bytes, 27 parts)
  parts parse: ok
  no case-shadowed parameter: ok
  surface: ok (33 entries)
  selftest: ok
  analyzer: clean
build ok
```

⭐ **The split immediately paid for itself in a way the entry did not predict.**
`build.ps1 -Test` grew a check for a local whose name differs from a parameter's
only by case, because one of those shipped in this very session and only driving
a real distro found it. That check exists because there was a place to put it.

---

## WSL-22. the stream log has no sink, no colour and no prefix-only mode

**Source** deferred from `WSL-18`, 2026-08-30.
**Category** wsl-ephemeral, **Priority** P3, **Effort** S, **Status** done

---

## Problem

`tss` has `-o FILE`, `--force-overwrite`, `--color`, `--prefix-only`,
`--separator` and `--buffered`. The stream log implements the timestamp modes
and the format string and none of those six.

## Premise

⭐ **Deliberate, and recorded so it is not read as an oversight.** A caller
redirects to get a file, and none of the six was asked for by the issue. A knob
with no caller is machinery with a maintenance cost and no benefit.

## Approach

⛔ **Do not implement these because the list exists.** Add one when something
asks for it, and say in the entry what asked.

## Prove

The command that asked for it, in a session transcript or an issue.

---

## Closing

**Closed 2026-08-30T08:45:00Z.** ⭐ **The thing that asked for it is the
operator, on 2026-08-30**, instructing this session to adopt as many useful ideas
and quality-of-life improvements as possible from the issue's comments and its
references. That is the command this entry was waiting for, and the entry's own
rule was followed: five of the six were built, one was refused, and the refusal
carries its reason.

| `tss` flag | what it became |
| --- | --- |
| `-o FILE` | `-StreamLogPath` |
| `--force-overwrite` | `-StreamLogOverwrite` |
| `--color` | `-Color auto\|always\|never`. ⛔ A file sink never gets colour whatever it says: escape sequences in a log make a later grep answer wrongly. |
| `--prefix-only` | `-PrefixOnly` |
| `--separator` | `-TimestampSeparator` |
| `--buffered` | ⛔ **refused, and this is the reason.** Buffering trades latency for throughput on a finished log, and latency is the entire point here: a tick stuck in a buffer is worse than no tick, because it is late AND authoritative. A caller who wants a buffered log has one already, by redirecting. |

**And more than the six**, because the mockup the issue cited carries ideas the
six do not: `-TimestampColumns` composing `rel,delta`, `-TimestampProfile` with
`human`, `ci`, `forensic`, `wall` and `raw`, `-Redact`, `-MaxLineBytes`,
`-EventLog` writing one JSON object per line, `-TickEscalateSeconds`, a
silence-ended line, an exit-code reading, `-DryRun`, and `-Action Doctor`.

```text
selftest: 117 case(s) passed over 30 function(s) loaded from wsl-toolkit.ps1.
```

### ⛔ A defect this entry's own work shipped, and the class behind it

⚠ **`-TickEscalateSeconds` was declared `[int[]]` and it silently bound the wrong
number.** A `.ps1` run through `pwsh -File` receives every argument as a STRING,
so `-TickEscalateSeconds 5,9` arrives as the one string `"5,9"`, and PowerShell
converts a string to an int with the current culture's number style, where a
comma is the THOUSANDS separator. It bound the single value **59**. The
escalation never fired and nothing said so; it was found by instrumenting the
function, not by reading it.

Measured the same day, under PowerShell 7.6.5 and Windows PowerShell 5.1, both
identical:

```text
=== pwsh -File, comma form ===
Ints  count=1  values=[59]
Strs  count=1  values=[a,b]
=== pwsh -File, repeated parameter ===
Cannot bind parameter because parameter 'Strs' is specified more than once.
=== pwsh -File, space-separated ===
A positional parameter cannot be found that accepts argument '9'.
=== in-process call (a real array) ===
Ints  count=2  values=[5|9]
```

⭐ **So every list parameter here takes `[string[]]` and splits its own value**,
which turns a non-number into a refusal instead of a plausible wrong answer.
`docs/conventions/shell.md` section 8 and
`docs/conventions/forbidden-patterns.md` both carry it, and `WSL-24` carries the
second half of the same discovery.

---

## WSL-23. parameters are silently ignored by the actions they do not apply to

**Source** carried from the 2026-08-29 record, extended 2026-08-30.
**Category** wsl-ephemeral, **Priority** P3, **Effort** M, **Status** done

---

## Problem

`-Image`, `-Tarball`, `-OciEnv` and `-Systemd` are read only by `New`, and
passing them to `List` or `Purge` does nothing and says nothing. This session
added seven more parameters with the same property.

## Premise

⚠ **Partly addressed and not closed, which is why this stays open.** The
combinations that are wrong *among the new parameters* are refused by name:
`-NoTimestamps` with any of the four it disables, `-TimestampFormat` with a mode
that has no format, `-Verbatim` without `-CommandFile`, `-ScriptArg` with no
command. What is not refused is a parameter passed to an action that ignores it.

## Approach

A table of which parameter applies to which action, checked once in `Main`.

⛔ **This is a break**, and it should be: a caller passing a parameter that does
nothing believes something is happening.

## Prove

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/wsl-toolkit.ps1 -Action List -Image alpine:3.22
```

Exit 1, naming both the parameter and the action.

---

## Closing

**Closed 2026-08-30T08:50:00Z.** A table in `core/applicability.ps1` says which
parameters each action reads, checked once in `Main` before anything else runs.

```text
ERROR: -Action List ignores parameters you passed, so it would have done something other than what you asked: -Image is read by -Action New and not by -Action List.
exit=1
```

⭐ **The table is derived from where each variable is READ, not from what the
help says**, and one row is worth naming: `-TimeoutSeconds` reaches
`Get-DistroOutput` and `Get-EngineAnswer` and nothing else, so it belongs to
`New` and `Resources` and is refused on `Run`. The refusal names the parameter
the caller actually wanted:

```text
-TimeoutSeconds bounds the questions this script asks a distro for itself. The
bound on YOUR command is -CommandTimeoutSeconds.
```

⛔ **A parameter with no row is refused on every action rather than allowed on
every action.** Forgetting to add a row then fails loudly on first use instead of
quietly reinstating the exact defect this closes. ⭐ And the selftest asserts the
table against the parameter block in BOTH directions, so a parameter without a
row and a row without a parameter each fail a gate:

```text
ok    every parameter in the block has a row, and every row a parameter
```

**Consumers.** ⛔ This is a break by `docs/consumers.md`'s definition and the row
is there. It is the right kind: a caller passing a parameter that did nothing was
not getting what they asked for, and nothing told them.

---

## WSL-24. a list parameter cannot be repeated, and an int list binds a wrong number

**Source** found on 2026-08-30 while driving `WSL-22`'s escalation, which never fired.
**Category** wsl-ephemeral, **Priority** P1, **Effort** S, **Status** done

---

## Problem

Two documented capabilities did not work, and neither said so.

`-ScriptArg NAME=VALUE` was documented as repeatable, and the issue comment that
closed `WSL-16` shows the two-argument form as the replacement for a consumer's
`sed` passes. It is refused:

```text
ERROR: Cannot bind parameter because parameter 'ScriptArg' is specified more than once.
```

`-TickEscalateSeconds 5,9` was declared `[int[]]` and bound the single value
`59`, so the escalation configured by it never fired and the run looked normal.

## Premise

⭐ **Measured, not read, and on both hosts.** A `.ps1` run through `pwsh -File`,
which is how every consumer runs this one, cannot be handed an array:

| what a caller types | what binds |
| --- | --- |
| `-Strs a,b` | one value, the literal string `a,b` |
| `-Ints 5,9` | ⛔ one value, `59` |
| `-Strs a -Strs b` | ⛔ refused: specified more than once |
| `-Ints 5 9` | ⛔ refused: positional |
| in-process `& .\s.ps1 -Ints @(5,9)` | two values, correctly |

⛔ **A wrapper does not rescue it.** The launcher splats the same argument list
into the inner script, and the inner binding is identical. Verified by running
the documented example through both entry points.

⚠ **Row two is the dangerous one and it is not obvious from the type.** Every
argument arrives as a string; PowerShell converts a string to an int with the
CURRENT CULTURE's number style, where a comma is the thousands separator.

## Approach

Two different answers, because the values are different kinds of thing.

**Where a comma cannot occur inside a value**, the parameter takes `[string[]]`
and splits its own value: `-TimestampColumns`, `-TickEscalateSeconds`, and
`-Redact`, where a pattern needing a literal comma writes the character class
`[,]`, which matches the same thing. `Split-DelimitedArgument` in
`libs/redact.ps1` is the one implementation, and an in-process caller passing a
real array lands there too.

⛔ **Where the value is arbitrary text there is no safe delimiter**, and
`-ScriptArg` is exactly that: a URL query string carries commas, and splitting on
one would corrupt the value it was passing, which is the failure `-ScriptArg`
exists to remove. So it takes a FILE: `-ScriptArgFile`, one `NAME=VALUE` per
line, through the same byte repair `-CommandFile` gets.

## Consumers

⛔ **A break, and it is in `docs/consumers.md`.** Nobody could have been using the
repeated form, because it never bound; a caller passing one `-ScriptArg` is
unaffected.

## Prove

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/selftest.ps1
```

Exit 0, with cases asserting that a comma-separated value becomes several, that a
real array still works, that a non-integer threshold is refused, and that a pairs
file and the flag land in one list.

---

## Closing

**Closed 2026-08-30T08:55:00Z.** Driven against a real distro with two pairs that
both carry commas in their values, which is the case a delimiter would have
corrupted:

```text
00:00:00.156 +0.156 out  URL is https://172.23.96.1:8443/a,b,c
00:00:00.205 +0.048 out  CFT is https://example.invalid/x.zip?a=1,2
```

and with the escalation that never fired now firing:

```text
00:00:06.648 +6.441 tick 6s silent | ... | distro Running | disk 76.0 MiB (unchanged)
00:00:06.653 +6.446 note after 6s of silence: NOTHING is ruled out. because: the distro disk did not grow between the last two ticks
00:00:08.365 +8.158 note output resumed after 8s of silence
```

```text
selftest: 117 case(s) passed over 30 function(s) loaded from wsl-toolkit.ps1.
```

⚠ **What this did not fix, said rather than left to be found.** An in-process
caller can still pass a real array to `-ScriptArg` and get every element, and
that path is not reachable through `-File`. The help says so instead of pretending
the two are the same.

---

## WSL-25. the release digest proves transport, not authorship

**Source** the operator, 2026-08-30, accepting it from a list put to them at the end of that session. The gap was written down by the work that created it.
**Category** wsl-ephemeral, **Priority** P2, **Effort** M, **Status** open

---

## Problem

`SHA256SUMS` ships in the same release as the asset it describes, so anyone who
could replace one could replace the other. The launcher says exactly that, out
loud, on every release fetch:

```text
  ! that proves the bytes arrived intact, not who published them.
```

⚠ **Saying it is better than not saying it and is not a fix.** A consumer who
reads that line and wants authorship has one answer today, `-LauncherSha256`
with a digest they hold, and holding a digest per release is the manual work the
release was supposed to remove.

## Premise

⭐ **Measured on 2026-08-30**, by publishing `wsl-toolkit-v1.0.0` and fetching it
from an empty directory: the launcher verifies the asset against the sums file
from the same release and reports a match. Nothing in that chain is signed, and
nothing in it is tied to the workflow that produced it.

## Approach

Sigstore keyless, ruled by the operator over minisign.

- `release.yml` signs each asset with `cosign sign-blob --yes`, producing a
  bundle beside it. ⛔ The job needs `id-token: write`, which is a permission
  raise and belongs to that job alone, exactly as `contents: write` already does.
- The launcher gains an OPTIONAL verify step: when `cosign` is present and a
  bundle asset exists, verify it against the workflow identity and say so; when
  it is not, say that too rather than skipping silently.

⛔ **Verification must not become mandatory in the same change.** A launcher that
suddenly requires a tool nobody has installed breaks every consumer to close a
gap none of them asked about. Make it report, then decide.

## Decision

⭐ **Ruled 2026-08-30: sigstore keyless, not minisign.** Keyless ties the
signature to the workflow identity, so there is no key for the operator to hold
or lose, which is the failure mode that actually ends signing schemes. ⚠ What it
costs is a dependency on the public transparency log at verify time, and an
offline verifier cannot check it. Minisign has the opposite trade and lost on the
key-custody half.

## Consumers

All three rows of [`../docs/consumers.md`](../docs/consumers.md). ⛔ Not breaking
while verification is optional; it becomes breaking on the day it is required,
and that day is its own entry.

## Prove

```bash
gh release view TAG --repo Azathothas/ToolKit --json assets --jq '[.assets[].name]'
```

The bundle assets present, and a launcher run reporting a verified signature
naming the workflow, plus a run on a host with no `cosign` reporting that it
could not verify rather than reporting success.

---

## WSL-26. a prepared rootfs is thrown away and paid for again

**Source** the operator, 2026-08-30. The workload behind issue 5 is the worked example.
**Category** wsl-ephemeral, **Priority** P2, **Effort** M, **Status** open

---

## Problem

`-Action New` always pulls, exports and imports. The incident that produced the
whole stream log was a 14-minute silent `apt` install; the second and third runs
of that workload pay for it again, and the tool offers nothing between "throw
everything away" and "manage a long-lived distro by hand".

## Premise

⚠ **Read, not measured**, and the measurement is cheap: nothing in this tree has
timed a repeated run of the same prepared workload. ⭐ What IS measured is the
import cost this session saw repeatedly, around 8 to 12 seconds for an 8 MiB
Alpine rootfs, which is the floor rather than the interesting number. The
interesting number is the guest-side preparation, and that is the caller's.

## Approach

`-Action Snapshot -Name <distro> -As <tag>` exports a registered distro back to a
rootfs tarball under the existing base directory, and `-Action New -Tarball <tag>`
takes it. The seam already exists: `Export-ImageRootfs` writes a tarball and
`Invoke-ActionNew` imports one, and this is the third caller of the same path.

⛔ **Nothing about the safety model changes.** The tarball lands under the one
base directory, `List` and `Purge` already report and remove loose tarballs
there, and a snapshot must not become a thing those two are blind to.

⚠ **A snapshot carries whatever the last command left in it**, including a
credential a caller passed with `-ScriptArg`. Say so where the tag is named, not
only on this page.

## Consumers

None break: this adds an action and a switch. ⚠ `Purge` starts removing
snapshots as orphans unless they are distinguished, and a caller who thought a
snapshot was durable would lose it. That is the one design question this entry
must answer before it writes anything.

## Prove

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/wsl-toolkit.ps1 -Action Snapshot -Name eph-x -As probe-ready
```

A second `New -Tarball probe-ready` that reaches the prepared state, timed
against the same preparation done from the image, both numbers recorded with the
machine and the date.

---

## WSL-27. the tick can say nothing is happening and never that something is

**Source** the operator, 2026-08-30, accepting it from the list. The idea is the mockup's section 48, question 4.
**Category** wsl-ephemeral, **Priority** P2, **Effort** M, **Status** open

---

## Problem

The tick reports silence, the distro's state and whether its disk grew. It cannot
report that a command is 60 percent through, because nothing inside the guest can
tell it anything, and the three host-side signals that were measured say only
whether something is happening rather than how much is left.

## Premise

⭐ **The mockup argues a stdout prefix is the right channel precisely because it
needs nothing**: no injection, no mount, no named pipe, no agent in the image.
⚠ Its own caveat is that a prefix can collide with application output, which is
why the token has to be the caller's choice rather than a constant.

## Approach

`-ProgressPrefix TOKEN`, off by default. A relayed line whose text begins with
the token is CONSUMED rather than relayed, parsed as a percentage and an optional
label, and reported by the tick and in the event log. The seam is
`Write-StreamLogLine`'s caller in `Invoke-InDistroLogged`, where a line is
already classified before it is written.

⛔ **Off by default and the token is never a default.** A tool that silently
swallows a line beginning with some chosen string is a tool that eats somebody's
output.

⛔ **The tick reports the last progress and when it arrived. It does not compute
an estimate.** A remaining-time figure from one sample is the fabricated number
`prose.md` forbids.

## Consumers

None break: a caller who never passes `-ProgressPrefix` sees exactly what they
see now, and that is the property to assert in the suite.

## Prove

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/selftest.ps1
```

Cases proving a prefixed line is consumed, an unprefixed one is relayed
unchanged, a malformed prefixed line is relayed rather than swallowed, and that
with no `-ProgressPrefix` every line is relayed byte for byte.

---

## WSL-28. a recorded run cannot be re-read or compared

**Source** the operator, 2026-08-30, accepting it from the list. Possible only because `-EventLog` now exists.
**Category** wsl-ephemeral, **Priority** P2, **Effort** M, **Status** open

---

## Problem

`-EventLog` writes every event as JSON, and nothing reads it. A caller who
recorded a run cannot re-render it in another timestamp shape without running it
again, and cannot compare two runs at all.

## Premise

⭐ **The schema is versioned and the renderer is already a pure function of the
event fields**, which is what makes this small: `Format-StreamLogPrefix` needs a
clock reading and a tag, and both are in every record.

⚠ **What is NOT in the records is the settings the run used.** A replay in a
different shape is the point, so that is fine; a replay claiming to reproduce the
original bytes is not, and this entry must not claim it.

## Approach

Two actions, both pure functions over a file, so the suite covers all of it with
no WSL and no engine.

- `-Action Replay -EventLog FILE` re-renders with any `-Timestamp*` settings.
- `-Action Compare -EventLog A -Against B` diffs duration, longest silence, time
  to first output, line and byte counts per stream, and the exit code.

⭐ **Longest silence is the figure that earns this.** A run whose result stayed
green while its longest gap grew fifteen times has a regression that no exit code
reports.

⛔ **A gap in `seq` is reported, not smoothed over.** The field is documented as
gapless, so a gap means records were dropped and that is a finding about the
recording rather than about the run.

## Consumers

None. Two new actions, and `WSL-23`'s applicability table gains rows for them.

## Prove

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/selftest.ps1
```

Cases over a fixture event log: a replay in three timestamp shapes, a comparison
naming the longer silence, and a refusal on a log with a gap in `seq`.

---

## WSL-29. every run imports, even when a distro from the same image is registered

**Source** the operator, 2026-08-30, accepting it from the list.
**Category** wsl-ephemeral, **Priority** P3, **Effort** S, **Status** open

---

## Problem

A caller running ten commands against one image either pays the pull, export and
import ten times, or manages a distro name by hand across ten invocations.

## Premise

⚠ **Read, not measured.** The import cost seen repeatedly this session is 8 to 12
seconds for an 8 MiB Alpine rootfs on one machine, which bounds what this saves
for a small image and says nothing about a large one.

## Approach

`-Reuse` on `New`: if a registered ephemeral distro was created from the same
image reference, run in it rather than importing. It must SAY which it did.

⛔ **It cannot be the default and it cannot be silent.** A reused distro carries
whatever the last command left in it, including files a previous caller wrote,
and a caller who did not ask for that is owed the warning every time.

⚠ **The image reference has to be recorded somewhere the tool can read back.**
Deriving it from the generated distro name is a value re-parsed out of a mutable
name, which `conventions/code.md` names as the wrong answer.

## Consumers

None break: a caller who never passes `-Reuse` gets today's behaviour exactly.

## Prove

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/wsl-toolkit.ps1 -Action New -Image alpine:3.22 -Reuse -Command 'true'
```

Twice in a row: the first imports, the second names the distro it reused and its
age, and both exit 0.

---

## WSL-30. the mockup's other two thirds: a podman adapter

**Source** the operator, 2026-08-30, choosing to build the adapter over measuring the feeds first.
**Category** wsl-ephemeral, **Priority** P2, **Effort** XL, **Status** open

---

## Problem

The timestamp layer, the tick, the event log and the exit-code reading are all
container-agnostic, and none of them can watch a container. A caller running a
podman workload gets none of it, which is the two thirds of the mockup this
repository deliberately did not build.

## Premise

⛔ **EVERY FEED THIS NEEDS IS UNMEASURED**, and the mockup says so about itself:
its section 47 is sixteen claims, each marked as a hypothesis, over a document
whose own banner says nothing in it has been run.

⚠ **This tool's WSL signals, by contrast, are all measured, and three of the four
candidates were REJECTED by measurement.** That ratio is the reason this entry
starts where it does.

## Approach

⭐ **Step one is the validation matrix, and nothing is designed before it
answers.** Run the mockup's sixteen claims against real podman on this host and
record which feeds exist, which are absent, and which answer differently from
what it assumed. That produces sixteen facts and makes every later decision
cheap.

Then, and only then, an adapter behind the existing rendering layer: lifecycle
from `podman events`, output from `podman logs`, resources from `podman stats`,
and the exit code from `podman wait`. ⛔ The observation layer must not name a
command; the adapter decides, and a feed that does not exist reports absent
rather than zero.

⛔ **A distro and a container are not the same kind of thing.** The mockup is
explicit about it and refuses to pretend otherwise, and so must this: they share
a kernel and nothing else, and one tool that blurs them produces nonsense about
both.

## Decision

⭐ **Ruled 2026-08-30: build the adapter**, over the smaller option of measuring
the feeds and stopping.

⚠ **The effort is XL, and the index says an XL is almost always two entries
pretending to be one. It is.** The ruling is recorded as given; the honest note is
that if the validation matrix comes back with more than a couple of absent feeds,
the right response is to split this rather than to push through, and the split is
already drawn: the matrix is one entry and the adapter is the other.

## Consumers

None today. ⚠ If it lands as new actions on this script rather than as a second
tool, every consumer's parameter surface grows, and `surface.lock` is what will
say so.

## Prove

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/wsl-toolkit.ps1 -Action Doctor
```

First, a capability table with a measured answer for each of the mockup's sixteen
claims. Then a container run whose output carries the same stamps, whose silence
produces the same tick, and whose exit code passes through unchanged.
