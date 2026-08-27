# INDEX.md

Every open item, sortable. [`PROGRESS.md`](PROGRESS.md) says what to do next
and why; this file is the list.

⛔ **An entry is closed by its acceptance command, not by a claim.** The command
is in the entry, it is run unpiped, and its output is the evidence.
[`docs/methodology/work-todo.md`](docs/methodology/work-todo.md) is the model.

Priority: `P0` blocks something, `P1` next, `P2` wanted, `P3` someday.

---

## Open

| id | pri | title | status |
| --- | --- | --- | --- |
| `WSL-01` | P0 | `New -Command` must propagate the inner exit code | open |
| `WSL-02` | P1 | carry the image's OCI config into the distro | open |
| `WSL-03` | P1 | pass `--platform` to pull and create | open |
| `WSL-04` | P1 | a failed delete must not report success | open |
| `WSL-05` | P2 | report and purge orphaned rootfs tarballs | open |
| `WSL-06` | P2 | disk-space preflight before import | open |
| `WSL-07` | P2 | optional systemd via `/etc/wsl.conf` | open |
| `WSL-08` | P2 | a `-Command` channel that survives two shells | open |
| `WSL-09` | P3 | bound the smoke probe with a timeout | open |
| `WSL-10` | P3 | retry a generated name on collision | open |
| `DOC-01` | P2 | a `binfmt_misc` check for the podman machine on WSL2 | open |
| `BSD-01` | P1 | run BSD under podman on Windows without nested qemu | research |

---

## The entries

### `WSL-01` P0. `New -Command` must propagate the inner exit code

**What is wrong.** `Invoke-ActionNew` runs the command, captures `$rc`, and then
calls `Write-Warn` and returns. The script exits 0. `Invoke-ActionRun` ends in
`exit $rc` and propagates correctly, so the two paths disagree.

⛔ **Why it is P0.** The documented CI example uses `New -Command`, so a failing
command in CI reports success. Every green result from that path means nothing
until this is fixed. It is the same shape as the defect in
`Azathothas/TEMPLATE` issue 2, where `systemd-binfmt.service` exited 0 having
registered no handlers.

**What done looks like.** `New` propagates the inner code after teardown, so
`-Ephemeral` still removes the distro before the script exits. The asymmetry
between `New` and `Run` is stated in `.NOTES` so it cannot drift back.

⚠ Read [`docs/consumers.md`](docs/consumers.md) first. This is a behaviour
change for any caller that was relying on the false pass.

**Acceptance.** Both must hold, and the exit code is read unpiped:

```powershell
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral.ps1 -Action New -Image alpine:3.22 -Command "exit 7" -Ephemeral -Force
```

Exit code is 7, and `wsl --list --quiet` no longer lists the distro.

```powershell
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral.ps1 -Action New -Image alpine:3.22 -Command "true" -Ephemeral -Force
```

Exit code is 0.

### `WSL-02` P1. Carry the image's OCI config into the distro

`Export-ImageRootfs` uses `create` plus `export`, which writes a filesystem and
no configuration, so `ENV`, `WORKDIR`, `ENTRYPOINT` and `USER` are lost. The
distro's `PATH` is therefore not the image's `PATH`, which makes the distro a
poor stand-in for the thing being tested.

Read the config and write it into the distro:

```bash
podman image inspect IMAGE --format '{{json .Config.Env}}'
```

⚠ Behaviour change. Recommend a switch defaulting to off, so no existing caller
moves under them.

**Acceptance.** A distro built from an image with a non-default `PATH` reports
that `PATH` from `sh -lc 'echo $PATH'`.

### `WSL-03` P1. Pass `--platform` to pull and create

Neither call names a platform, so the exported architecture is whatever the
local store happened to hold. ⚠ A single `--platform` pull rewrites the shared
local tag, so an unrelated earlier command can silently produce a rootfs that
cannot execute, surfacing only as the `/bin/sh did not run` smoke failure.

`Export-ImageRootfs` already probes `podman info --format '{{.Host.Arch}}'` for
readiness and discards the value. Use it.

**Acceptance.** After a deliberate `podman pull --platform linux/riscv64 alpine`,
a `New -Image alpine` on an x86_64 host still produces a working distro.

### `WSL-04` P1. A failed delete must not report success

`Remove-EphemeralDistro` calls `Remove-Item -ErrorAction SilentlyContinue` and
then prints `deleted` unconditionally. `--unregister` releases the VHDX
asynchronously, so a delete immediately after can lose the race and leave a
multi-gigabyte disk behind while reporting it gone.

**Acceptance.** `Test-Path` after the delete, an honest message on failure, and
a short bounded retry. A test that holds the directory open sees a non-zero exit
and a message naming the path.

### `WSL-05` P2. Report and purge orphaned rootfs tarballs

The export target is `BaseDir\<distro>.tar`, cleaned in a `finally`, which a
hard interrupt does not always run. Neither `List` nor `Purge` looks at `*.tar`,
so a few hundred MiB can sit in `%LOCALAPPDATA%` unnoticed.

**Acceptance.** `List` reports orphans; `Purge` offers to remove them under
`Assert-InsideBaseDir`.

### `WSL-06` P2. Disk-space preflight before import

Export plus import needs roughly twice the rootfs size on the `%LOCALAPPDATA%`
volume. Running out midway leaves a partial VHDX and a registered distro that
does not work.

**Acceptance.** Free space is compared against the tarball size before
`--import`, and an insufficient volume is a clean refusal, not a half-state.

### `WSL-07` P2. Optional systemd via `/etc/wsl.conf`

An imported distro has no `/etc/wsl.conf`, so systemd does not start and nothing
involving units, timers or `systemctl` can be tested. Add a switch writing
`[boot]` and `systemd=true`.

⚠ It needs a `--terminate` afterwards to take effect, and that belongs next to
the switch in the documentation.

**Acceptance.** With the switch, `systemctl is-system-running` answers from
inside the distro.

### `WSL-08` P2. A `-Command` channel that survives two shells

`-Command` crosses PowerShell and then `/bin/sh -lc`, and the caller owns all
quoting across both. [`docs/conventions/shell.md`](docs/conventions/shell.md)
section 1 is emphatic that this is where payloads lose their meaning, and this
repository already ships the answer: base64, or a file.

**Acceptance.** A command containing a single quote, a double quote, a backtick
and a dollar sign runs unchanged.

### `WSL-09` P3. Bound the smoke probe with a timeout

The probe absorbs a drvfs race with a sleep loop inside the guest, but the outer
call has no time limit, so a distro whose init wedges hangs the script with no
output. [`docs/conventions/shell.md`](docs/conventions/shell.md) section 9.

### `WSL-10` P3. Retry a generated name on collision

`New-DistroName` adds a four-character suffix and `Invoke-ActionNew` throws if
the name exists. For a caller-supplied `-Name` that is correct. For a generated
one, failing on a 1-in-1,679,616 collision is worse than drawing again.

### `DOC-01` P2. A `binfmt_misc` check for the podman machine on WSL2

On Windows with a podman machine, cross-architecture containers fail with
`Exec format error` because `/proc/sys/fs/binfmt_misc` has a `binfmt_misc`
instance and a systemd autofs stacked on the same path. Reading it returns
`ELOOP`, and ⛔ `systemd-binfmt.service` reports success having registered
nothing.

⚠ **This does not belong in `scripts/doctor/`**, which was the original
suggestion. The probe is read-only, spawns one process per tool and makes no
network call, and reaching into the machine costs a `podman machine ssh`, which
is slow, can hang, and on 2026-08-27 was measured writing a 99-byte `NUL` file
into the working directory under Git Bash. A probe that litters a repository is
a worse defect than the one it detects.

It belongs here instead, as its own script, run on request.

**Acceptance.** The check reports the handler count and names the `ELOOP` case
specifically. Against a machine in the broken state it exits non-zero; against a
fixed one it exits 0 and reports 31 handlers.

### `BSD-01` P1. Run BSD under podman on Windows without nested qemu

⛔ **Research only. Nothing is implemented until the operator has approved a
written approach.**

The goal is `podman run --rm -it SOME_IMAGE sh` dropping into a real BSD
userland from a Windows host, without `wsl` inside `linux` inside `qemu` inside
`bsd`, and with CI here building images for the common BSDs and pushing them to
`ghcr.io`.

The write-up answers, with evidence rather than from a README: what the FreeBSD
OCI images actually are and what runs them; whether a Linux-kernel container
runtime can execute a FreeBSD userland at all, and if not, what the honest
alternative is on a Windows host; what a podman machine image has to provide;
and what the wrapper would have to do if podman assumes something it cannot
change.

References to work from are in the operator's intake and belong in the
write-up's own reference list under
[`docs/methodology/references.md`](docs/methodology/references.md).
