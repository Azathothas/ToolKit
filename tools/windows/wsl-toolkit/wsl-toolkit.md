# wsl-toolkit

Run an isolated Linux container job on a Windows host. The tool owns one WSL
distribution, runs rootless Podman inside it, and gives a job a COPY of a
workspace rather than a mount. It also makes, runs and removes throwaway WSL
distributions, and boots a FreeBSD guest.

⭐ **This is the only page you need to use it, and it is deliberately short.**
The flags live in the CLI, which generates its own manual, so nothing here can
drift from the binary. Read the three commands under
[Finding a flag](#finding-a-flag) before reading anything else.

⚠ **Windows only.** It drives `wsl.exe`. On any other host the commands refuse
with a message rather than half-working.

---

## Finding a flag

⛔ **Do not look for a flag reference on this page.** There is none, on purpose:
a hand-written reference drifts, and a flag it does not mention is still a flag
the binary accepts.

```powershell
wsl-toolkit man --no-pager     # the complete manual, generated from the CLI
wsl-toolkit COMMAND --help     # one command
wsl-toolkit examples           # the canonical commands, ready to paste
```

`wsl-toolkit man` alone opens a pager for a person. [`wsl-toolkit.1`](./wsl-toolkit.1)
is the same manual as a tracked roff page for `man`; it is generated, and a test
refuses it when it disagrees with the registered commands and flags.

---

## ⭐ Five hazards a caller does not have to handle

| the hazard | what the tool does |
| --- | --- |
| **NTFS carries no executable bit**, so a script in a Windows checkout arrives unrunnable and fails `Permission denied` | the copy reads the git index, and a file the index marks `100755` arrives executable. A file that is in no index and starts with `#!` arrives executable too. The count is reported. Data is never marked executable |
| **A file that grows while the workspace copies** would fail the copy with `archive/tar: write too long`, naming the archiver and not the file | the copy is bounded by the size in the header. The file travels as the prefix that was declared, and the result names it |
| **A payload written on Windows carries CRLF**, and `/bin/sh` reads the carriage return as part of the last word | every spelling of a command has the copy that is sent repaired. The file on disk is never written to |
| **`--workspace .` resolves against the working directory**, which a sandbox can reset | a relative path is resolved against the project configuration when there is one, the resolved host path is printed, and a filesystem root, a home directory or a system directory is refused outright |
| **A writable `/mnt/c` inside the base** lets a wrong path in a job destroy the real checkout on Windows | the Windows drives mount read only. `base.automount` takes `rw` or `off`; explicit `base.mounts` grants are available only with automount and Windows interop both off |

---

## Operating model

- The tool owns the `wsl-toolkit` distribution, the throwaway distributions whose
  disks are in its state directory, and that directory. It manages nothing else,
  and `podman-machine-default` is somebody else's.
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

## Dedicated provider bases

A named instance can be a persistent Linux home for a provider CLI. A project
profile can enable systemd, select the `developer` toolset, opt a trusted agent
into passwordless sudo, disable ambient Windows drive mounts and executable
interop, and grant exactly one checkout at `/workspaces/project`:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base ensure
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base status --probe --json
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base shell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec --dir /workspaces/project -c 'git status --short'
```

`base exec` is the non-interactive host-to-guest seam. It starts as the
configured account in that account's home unless `--dir` names an absolute
guest path, sends the command or `--script` body framed on stdin with
`/dev/null` as the command's own stdin, forwards output, and returns the guest
exit status. `--root` is an explicit administrative variant.

⚠ `passwordless_sudo: true` gives the configured account unrestricted guest
root. Guest root can manually mount Windows paths, so this setting serves a
trusted agent and is not a containment boundary.

[`examples/muse-code/README.md`](examples/muse-code/README.md) is the complete
worked example. [`examples/common/access-profiles.md`](examples/common/access-profiles.md)
carries both the one-checkout profile and the zero-grant profile, including the
boundary they do not claim against guest root or the network.
[`examples/common/zellij.md`](examples/common/zellij.md) is the operator and
agent guide for the same durable session, including native Windows attachment.

---

## Container platform

```powershell
wsl-toolkit run --image alpine --platform linux/arm64 -c 'uname -m'
```

Architecture shorthand such as `arm64` is accepted. The base registers QEMU
binfmt handlers for a foreign architecture and restores a handler that WSL
discarded when its VM stopped.

⛔ **Every job names a platform, including one that asked for nothing.** With no
`--platform`, podman runs whatever variant of the image its local store holds:
after one `linux/arm64` run, a native run of the same image answered `aarch64` on
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

⭐ **The guest disk is 10 GiB, and the root filesystem follows it.** The published
image is 6.0 GiB with a 4.8 GiB root, and a toolchain install fills that. `bsd run`
grows the image file to `--disk` GiB, 10 by default, before it boots, then extends
the partition and the filesystem with FreeBSD's own `gpart` and `growfs` before the
payload runs. Measured on 2026-09-13: a 10.0 GiB disk with an 8.7 GiB root. `bsd
status` prints the disk, and the run's last line prints both sizes.

⛔ **It never shrinks.** Every session on this host shares the image, and a shorter
file cuts off the filesystem inside it, so a `--disk` smaller than the image is
refused. `bsd fetch --force` goes back to the published image.

⛔ **This reaches a BSD SHELL and not a BSD container endpoint.** A long-running
`podman system service` inside the guest panics the guest kernel in `_umtx_op`.
[`pkgforge-dev/docker-bsd`](https://github.com/pkgforge-dev/docker-bsd) carries
the images and that experiment.

⚠ The guest gets no network unless `--network` is passed, and nothing is ever
forwarded inward. This image's root account has an empty password, which is what
makes it usable with no installer, so the console stays this process's own pipe.

---

## Throwaway distributions

A throwaway distribution is a whole WSL distribution imported from an image or a
rootfs archive, for a job whose subject is the distribution itself: its init, its
`/etc/wsl.conf`, what a login shell sees. Containers in the base answer every
other question.

```powershell
wsl-toolkit distro new --image alpine -c 'cat /etc/os-release' --ephemeral
wsl-toolkit distro new --image debian --name build-box
wsl-toolkit distro run --name build-box -c 'uname -a'
wsl-toolkit distro list
wsl-toolkit distro remove --name build-box --yes
```

- Every name starts with `eph-`, and every `--name` adds the prefix when it is
  left off. A name in the wrong case is refused rather than lowered.
- `distro new --image` needs a host engine, podman or docker, and pulls for this
  host's platform. The import is refused before it starts when the volume lacks
  256 MiB beyond twice the archive. `--probe-timeout` bounds the questions the
  tool asks the new distribution for itself: the smoke probe, whether PID 1 is
  systemd, and the image configuration `--oci-env` carries. A file the tool writes
  into the distribution has its own two-minute bound.
- `--systemd` refuses an image whose PID 1 is not systemd after the restart.
  `--oci-env` writes the image's `ENV` and `WORKDIR` to `/etc/profile.d`.
  `--reuse` runs in the newest distribution built from the same `--image` and
  says so.
- `distro snapshot --name NAME --tag TAG` exports one, and `distro new --tarball
  TAG` imports it again. Without `--force` it never replaces an archive under the
  tag, including one another run wrote while this export ran. ⚠ A snapshot is an
  unencrypted archive of whatever the distribution held, a credential a command
  left behind included.
- A refusal answers 2 and changes nothing. A removal or an export that was
  attempted and did not finish answers 1.

⛔ **The tool acts only on a distribution whose disk WSL registered inside this
state directory's `distros` folder.** The prefix proves nothing, so a
distribution another run or another tool made under it is listed as elsewhere and
refused by `run`, `enter`, `remove` and `snapshot`. `distro purge` prints a plan,
and `--apply` removes every owned distribution and what failed creations left. A
running distribution, and one another run is still creating, is kept unless
`--include-live` is passed. A snapshot is always kept.

### The command

- ⭐ **The command travels framed on stdin, and its own stdin is `/dev/null`.** A
  command that reads stdin cannot consume the rest of its script, and a prompt
  waiting for input reads end of file rather than waiting. `distro enter` is the
  interactive path.
- It runs as a login shell, as `--user`, and its exit code is the answer.
  `--timeout` terminates the distribution and answers 124, and a cancellation
  answers 130.
- `-c`, `--command-base64` and `--script` are three spellings of one command.
  The copy in transit has CRLF turned into LF and a byte order mark dropped, and
  the tool says when it did; `--verbatim` sends the bytes exactly. UTF-16 and a
  NUL byte are refused. From PowerShell, pass a command carrying quotes as
  `--command-base64`.
- `--env-file`, then `--env`, assign `NAME=VALUE` before the command, single
  quoted and in that order. `@hostaddress` inside a value becomes what
  `wsl-toolkit hostaddress` answers. `--user-env` first prepares a private
  `XDG_RUNTIME_DIR`, a `TMPDIR` and a deduplicated `PATH`, so a value passed with
  `--env` wins over it.
- `--dry-run` refuses what the real run would refuse, a snapshot tag the state
  directory does not hold, a taken `--name` and a missing host engine included,
  and prints the plan from the same selections the real run reads: the command's
  size and digest and the variables' names, never their values. Nothing in WSL
  changes and no log file is opened.

### Watching and recording a command

⭐ **With no option below, the command's streams are forwarded unchanged.**

| to get | pass |
| --- | --- |
| a timestamp and a stream tag on every line | `--log-profile human`, `ci` (rel and delta, no colour), `forensic` (wall and rel to the microsecond, and delta) or `wall`, or `--timestamp-column rel,delta`. A flag passed beside a profile wins over it |
| a heartbeat while a command is silent | `--tick 30s`, which a rendering profile turns on. It reads the distribution's state and disk, and says more at each `--tick-escalate` threshold, 2m, 5m and 15m by default |
| progress a command reports | `--progress-prefix TOKEN`. A line `TOKEN 42 unpacking` is consumed, and the heartbeat carries the last one and its age |
| a copy of the rendered lines | `--stream-log FILE`, which is never coloured |
| a record a program reads | `--event-log FILE`, one `wsl-toolkit-event/1` object per line, appended |
| a secret kept out of every sink | `--redact REGEX`, whose matches become `***` before any sink sees a line. Repeat it or pass a comma list; `[,]` matches a literal comma |
| a bound on a line | `--max-line-bytes N`, cut at a character boundary, saying how many bytes went |

- A prefix is the timestamp columns, the separator, then a fixed four-character
  tag: `out`, `err`, `tick` or `note`. A `~` in the tag marks a line that had not
  ended: a carriage return redrew it, or it sat unterminated for two seconds and
  was shown early.
- The command's stdout stays on stdout. Its stderr, the heartbeat and the notes go
  to stderr, and a nonzero exit gets a note on what the number can mean.
- ⚠ **A rendered line is not the guest's bytes.** A prefix, a redaction, a line
  bound or a progress token relays the live streams line by line, re-terminated
  with a newline. A sink or a heartbeat alone leaves the live bytes exact. Either
  way the command writes into a pipe, so a program that block-buffers off a
  terminal shows its lines late.
- ⚠ A redaction matches within one line, so a secret split across a
  carriage-return redraw is two lines and matches neither.
- `distro replay --from FILE` renders a recorded log again, stamped from each
  record's own wall clock. `distro compare --before A --after B` sets two runs
  side by side: elapsed time, time to first output, longest silence, the lines
  and bytes the record holds, which are counted after redaction and the line
  bound, and exit code, with `-` where a run measured nothing. It reports and does
  not judge. An appended log holds one run per command: `--run`, `--before-run`
  and `--after-run` pick one, and `compare` takes each file's last by default.

⚠ The helper does not serve these commands. A process that may not call `wsl.exe`
makes them through the path its session uses to approve it.

`wsl-toolkit doctor` reports what a throwaway distribution would meet: free space
against the import floor, what the state directory holds, the networking mode
and host address, the host engine, and the clock's resolution. `resources` lists
the owned distributions, leftovers and snapshots, and `resources --host-engine`
adds what the host's own engine holds, printing the commands that would free it
and running none.

## The address a distribution reaches this host at

```powershell
$addr = wsl-toolkit hostaddress
```

The address is the only thing on stdout. It reads `%USERPROFILE%\.wslconfig` and
this host's network adapters and starts nothing. ⛔ **In NAT mode a host service
bound to 127.0.0.1 is not reachable from a distribution**: bind it to the address
printed. Mirrored mode answers 127.0.0.1, and any other mode is refused.

---

## The safety model

Removal is constrained, and every destructive path goes through the same gate.

1. The base is a distribution named `wsl-toolkit` or `wsl-toolkit-INSTANCE`, and
   removing one that was ever usable also demands the identity marker its
   provisioning wrote inside the guest.
2. A throwaway distribution is this tool's only when its registered disk is
   inside the state directory's `distros` folder.
3. ⛔ **A container runtime's own distribution is refused by name**:
   `podman-machine-default`, `docker-desktop`, `docker-desktop-data`,
   `rancher-desktop`, `rancher-desktop-data`.
4. ⭐ **Every removal of state goes through one deletion, and the containment
   check runs INSIDE it**, not beside each caller. A guard applied at four call
   sites is a guard that will one day be applied at three.

⛔ **A delete that did not happen is not reported as one.** `wsl --unregister`
releases a disk asynchronously, so the state is read back after removal and a
path that is still there exits non-zero naming it.

### ⚠ Two things about WSL that this tool cannot protect you from

- ⛔ **`wsl --shutdown` is machine-wide.** It is what a person reaches for after
  finishing with a throwaway distribution, and it stops every distribution on the
  machine, including the podman machine. This tool never runs it.
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
| `--oci-env` carries `ENV` and `WORKDIR` only | decision | `USER` and `ENTRYPOINT` are not carried and will not be: WSL fixes the login account per call, and a login shell has no entrypoint |
| a throwaway distribution's command gets no stdin | decision | its stdin is `/dev/null`, because a pipe that carries the script cannot also carry input. `distro enter` is interactive |
| there is no port forwarding | decision | forwarding a port on Windows needs an elevated session and leaves a rule behind. `hostaddress` answers the question it was wanted for: bind the host service to that address |

---

## Requirements

| thing | needed for |
| --- | --- |
| Windows 10 2004+ or Windows 11, with WSL2 | everything |
| a container engine on the host | `base ensure`, once, to export the base rootfs, and `distro new --image` |
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
- [`docs/consumers.md`](../../../docs/consumers.md) is who fetches from here and
  what breaks them.
- [`docs/HISTORY/wsl-toolkit.md`](../../../docs/HISTORY/wsl-toolkit.md) is closed
  defects. ⛔ Nothing there is read to do work.
