# wsl-toolkit

Run an isolated Linux container job on a Windows host. The tool owns one WSL
distribution, runs rootless Podman inside it, and gives a job a COPY of a
workspace rather than a mount.

⭐ **This is the only page you need to use it, and it is deliberately short.**
The flags live in the CLI, which generates its own manual, so nothing here can
drift from the binary. Read the three commands under
[Finding a flag](#finding-a-flag) before reading anything else.

⚠ **Windows only.** It drives `wsl.exe`. On any other host the commands refuse
with a message rather than half-working.

---

## Finding a flag

⛔ **Do not look for a flag reference on this page.** There is none, on purpose:
a hand-written one was 1,217 lines, and a flag it did not mention was still a
flag the binary accepted.

```powershell
wsl-toolkit man --no-pager     # the complete manual, generated from the CLI
wsl-toolkit COMMAND --help     # one command
wsl-toolkit examples           # the canonical commands, ready to paste
```

`wsl-toolkit man` alone opens a pager for a person. [`wsl-toolkit.1`](./wsl-toolkit.1)
is the same manual as a tracked roff page for `man`; it is generated, and a test
refuses it when it disagrees with the registered commands and flags.

---

## ⭐ The five things a caller gets wrong

Each of these was a real workaround in somebody's wrapper before it was a
feature here. ⛔ **You no longer need to write any of them.**

| what used to bite | what happens now |
| --- | --- |
| **NTFS carries no executable bit**, so every script in a Windows checkout arrived unrunnable and the first one to run failed `Permission denied` | the copy reads the git index, and a file the index marks `100755` arrives executable. A file that is in no index and starts with `#!` arrives executable too. The count is reported. Data is never marked executable |
| **A file that grew while the workspace copied** killed the whole copy with `archive/tar: write too long`, naming the archiver and not the file | the copy is bounded by the size in the header. The file travels as the prefix that was declared, and the result names it. A background index or log no longer fails a job |
| **A payload written on Windows carries CRLF**, and `/bin/sh` reads the carriage return as part of the last word | `-c` and `--script` both repair the copy that is sent. The file on disk is never written to |
| **`--workspace .` resolved against the working directory**, which a sandbox can reset, so a job copied a tree nobody meant | a relative path is resolved against the project configuration when there is one, the resolved host path is printed, and a filesystem root, a home directory or a system directory is refused outright |
| **`/mnt/c` was writable inside the base**, so a wrong path in a job destroyed the real checkout on Windows | the Windows drives mount read only. `base.automount` takes `rw` or `off` where a caller wants something else |

---

## Operating model

- The tool owns the `wsl-toolkit` distribution and its state directory, and
  manages nothing else. `podman-machine-default` is somebody else's.
- ⛔ **A job gets a copy of its workspace, never a mount.** Getting anything back
  out is a second explicit act, through `/out` and `--artifacts`.
- Progress goes to stderr. The answer goes to stdout, alone. Every command that
  has a structured answer takes `--json`.
- `resources` reports what the tool is holding. `gc` prints a plan and
  `gc --apply` carries it out.

⛔ **Do not call `wsl.exe` with a job payload, and do not write a wrapper for
it.** A command handed to `wsl.exe` as an ARGUMENT is expanded before the guest
sees it and the result is parsed a second time: measured on 2026-09-09, a
payload's backtick was EXECUTED and the command still reported exit 0 over the
failure. This tool sends payloads on stdin or as a file.

---

## Container lifecycle

⭐ **The default is `persistent`**, so a job keeps its stopped container and its
guest job directory after it finishes. That is what makes a failed job something
you can go back and look at.

⚠ **Persistent means retained, not reused.** A later job creates a new
container. Nothing is carried from one job into the next.

```powershell
wsl-toolkit inspect JOB-ID            # what the job left
wsl-toolkit resources --job JOB-ID    # what is still held
wsl-toolkit gc --job JOB-ID --apply   # remove it
```

Pass `--container-lifecycle ephemeral` when a job must leave nothing, which is
normally the right choice in CI. `jobs.container_lifecycle` sets the default for
a machine or a project.

A timeout stops a persistent container and removes an ephemeral one, and exits
124. A user cancellation does the same and exits 130.

---

## Container platform

```powershell
wsl-toolkit run --image alpine --platform linux/arm64 -c 'uname -m'
```

Architecture shorthand such as `arm64` is accepted. The base registers QEMU
binfmt handlers for a foreign architecture and restores a handler that WSL
discarded when its VM stopped.

⛔ **Every job names a platform, including one that asked for nothing.** An
unnamed platform used to mean no `--platform` on the podman command line, and
podman then ran whatever variant of the image the local store already held: after
one `linux/arm64` run, a later native run of the same image answered `aarch64` on
an x86_64 host, with nothing but a podman warning to say so. The resolved value
is on the result.

⚠ **A shared kernel carries handlers this tool did not register.** The WSL2
kernel outlives any one distribution, so a foreign-architecture rootfs may RUN
rather than fail, which is the worse outcome. Measured 2026-08-27: 31 `qemu-*`
handlers were visible from a freshly imported Alpine containing no emulator, and
a riscv64 rootfs booted and answered `riscv64`. That is why the architecture is
named at pull and create time rather than checked afterwards.

---

## Relative paths, and the directory you are actually in

With a project configuration in play, a relative `--workspace`, `--script`,
`--artifacts` or matrix transcript path resolves against the directory holding
that configuration. A relative `jobs.workspace` always resolves against its own
configuration file.

Without one, a relative path resolves against the current directory, and the
resolved value is printed when the workspace is copied.

```powershell
wsl-toolkit config              # which file was chosen, and every place looked at
wsl-toolkit config --effective  # what the tool will actually use
```

⛔ **A filesystem root, a home directory or a system directory is refused**, not
corrected. Guessing which tree a caller meant is how a tool acts on the wrong one
and reports success.

---

## ⭐ A BSD userland, with no nesting

```powershell
wsl-toolkit bsd status
wsl-toolkit bsd fetch                       # about 635 MB, digest checked before use
wsl-toolkit bsd run -c 'uname -a; sysctl -n hw.model'
```

FreeBSD 15.1-RELEASE runs on this host's OWN hypervisor, through
`qemu-system-x86_64 -accel whpx`, which is the same hypervisor WSL2 uses. ⛔ **One
level deep, and no elevation.** The guest sits beside the podman machine rather
than inside it.

⛔ **A BSD binary cannot run on a Linux kernel, and binfmt does not help.**
FreeBSD's own image under Linux podman exits 139, a SIGSEGV on its first syscall,
rather than `Exec format error`. `binfmt_misc` and `qemu-user` solve a foreign
ARCHITECTURE presenting LINUX syscalls, and nothing presents BSD syscalls on a
Linux kernel. A BSD userland needs a BSD kernel.

⚠ **A boot costs about two minutes and it is paid per run.** Measured here:
113.6 s, 117.4 s and 117.7 s to a login prompt over three boots, of which 108 s
is device probing between the kernel banner and mounting root.

⛔ **This reaches a BSD SHELL and not a BSD container endpoint.** A long-running
`podman system service` inside the guest panics the guest kernel in `_umtx_op`.
[`pkgforge-dev/docker-bsd`](https://github.com/pkgforge-dev/docker-bsd) carries
the images and that experiment.

⚠ The guest gets no network unless `--network` is passed, and nothing is ever
forwarded inward. This image's root account has an empty password, which is what
makes it usable with no installer, so the console stays this process's own pipe.

---

## The low-level PowerShell interface

`wsl-toolkit.ps1` creates and destroys THROWAWAY distributions from any image or
rootfs tarball, which is what you want in order to test what a distribution
itself does. The executable carries it and forwards to it:

```powershell
wsl-toolkit script -Action New -Image alpine:3.22 -Command 'uname -a'
```

⚠ **It is the low-level compatibility interface and it is not the route to
reach for.** Other repositories fetch that file by raw URL, so its path,
parameters and exit codes do not change.
[`scripts/windows/wsl-toolkit/README.md`](../../../scripts/windows/wsl-toolkit/README.md)
is how it is built and released, and the script's own comment-based help is its
parameter reference:

```powershell
Get-Help .\scripts\windows\wsl-toolkit\wsl-toolkit.ps1 -Full
```

---

## The safety model

Both products here remove things, so removal is constrained and every
destructive path goes through the same gate.

1. Every throwaway distribution the script creates is named `eph-...`, and it
   refuses to remove a name without that prefix.
2. ⛔ **It refuses a name on the protected list, prefix or not**:
   `podman-machine-default`, `docker-desktop`, `docker-desktop-data`,
   `rancher-desktop`, `rancher-desktop-data`. A mistake cannot destroy your
   container runtime.
3. Directory deletion is confined to the tool's own state directory.
4. ⭐ **The containment check runs INSIDE the deletion helper**, not beside each
   caller. A guard applied at four call sites is a guard that will one day be
   applied at three.

⛔ **A delete that did not happen is reported as one.** `wsl --unregister`
releases a disk asynchronously, so the state is read back after removal and a
path that is still there exits non-zero naming it.

### ⚠ Two things about WSL that neither product can protect you from

- ⛔ **`wsl --shutdown` is machine-wide.** It is what a person reaches for after
  finishing with a throwaway distribution, and it stops every distribution on the
  machine, including the podman machine. Neither product ever runs it.
- ⚠ **A distribution's lifetime is not the kernel's.** `--terminate` and
  `--unregister` restart or remove the userspace; the WSL2 kernel keeps running,
  so `binfmt_misc` registrations, loaded modules and pinned superblocks survive.
  Measured 2026-08-26: a podman machine restarted seconds earlier was running a
  kernel ten hours old. Only `wsl --shutdown` gives a fresh one, and anyone
  reproducing a kernel-level condition without it reads stale state and gets a
  confident wrong answer.

---

## Known limits

⛔ **A limit hidden is a defect filed against the user later.**

| limit | kind | what it means |
| --- | --- | --- |
| the base enforces no per-container resource bounds | host | rootless podman under `init` with no cgroup delegation means no cgroup per container. `--memory` is accepted and not applied, and `podman stats` reads `0B`. ⭐ `base status` reports this. A caller who bounds a job on this base is not bounded |
| `podman logs` on the base is a silent zero | host | the default log driver is `journald` and nothing serves a journal. The tool names `k8s-file` on the runs it owns; a caller driving podman directly should too |
| `bsd run` boots for about two minutes per call | host | device probing in the guest. `bsd` carries the measurement |
| no BSD container endpoint | open | a long-running podman service panics the FreeBSD guest kernel. Tracked in [`../../../TODO/bsd.md`](../../../TODO/bsd.md) |
| `-OciEnv` carries `ENV` and `WORKDIR` only | decision | `USER` and `ENTRYPOINT` are not carried and will not be |
| there is no `-PortForward` | decision | forwarding a port on Windows needs an elevated session and leaves a rule behind. `HostAddress` answers the question it was wanted for: bind the host service to that address |

---

## Requirements

| thing | needed for |
| --- | --- |
| Windows 10 2004+ or Windows 11, with WSL2 | everything |
| PowerShell 7+, or Windows PowerShell 5.1 for the script | everything |
| a container engine on the host | `base ensure` only, to export the base rootfs once |
| `qemu-system-x86_64` and the Windows Hypervisor Platform | `bsd` only |
| `xz` | `bsd fetch` only |

---

## Examples

```powershell
wsl-toolkit ready --smoke                          # the whole route, including one container
wsl-toolkit run --image alpine -c 'uname -a'       # one job, container retained
wsl-toolkit gc --job JOB-ID --apply                # and removed again
```

```powershell
wsl-toolkit run --image debian --workspace . --artifacts .\out -c 'make && cp build/x /out/'
wsl-toolkit matrix --images all --container-lifecycle ephemeral -c 'cc --version'
wsl-toolkit helper serve --detach                  # when the calling process cannot reach WSL
```

`wsl-toolkit examples` prints these from the binary, so they cannot go stale.

---

## Related

- [`README.md`](./README.md) is how the executable is built and tested.
- [`scripts/windows/wsl-toolkit/README.md`](../../../scripts/windows/wsl-toolkit/README.md)
  is the script's build, surface lock and release pipeline.
- [`scripts/windows/wsl-toolkit/launcher.md`](../../../scripts/windows/wsl-toolkit/launcher.md)
  is the one-fetch launcher and its verification table.
- [`docs/consumers.md`](../../../docs/consumers.md) is who fetches from here and
  what breaks them.
- [`docs/HISTORY/wsl-toolkit.md`](../../../docs/HISTORY/wsl-toolkit.md) is closed
  defects. ⛔ Nothing there is read to do work.
