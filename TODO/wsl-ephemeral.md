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
**Category** wsl-ephemeral · **Priority** P0 · **Effort** S · **Status** done

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
**Category** wsl-ephemeral · **Priority** P0 · **Effort** S · **Status** done

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
**Category** wsl-ephemeral · **Priority** P1 · **Effort** M · **Status** done

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
**Category** wsl-ephemeral · **Priority** P1 · **Effort** S · **Status** done

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
**Category** wsl-ephemeral · **Priority** P1 · **Effort** S · **Status** done

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

⚠ **This is a behaviour change for `Remove`, `Purge` and `New -Ephemeral`:**
they can now exit non-zero where they used to exit 0. They were not succeeding
before; they were reporting. Not a consumer break in the
[`../docs/consumers.md`](../docs/consumers.md) sense, because no caller can
have been depending on a disk being left behind, but it is written into
`wsl-ephemeral.md` beside the exit codes.

---

## WSL-05. Report and purge orphaned rootfs tarballs

**Source** issue 3, part 3.7.
**Category** wsl-ephemeral · **Priority** P2 · **Effort** S · **Status** done

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
**Category** wsl-ephemeral · **Priority** P2 · **Effort** S · **Status** done

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
**Category** wsl-ephemeral · **Priority** P2 · **Effort** M · **Status** done

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
