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
| **A writable `/mnt/c` inside the base** lets a wrong path in a job destroy the real checkout on Windows | the Windows drives mount read only, and the base's verification reads them back. `base.automount` takes `rw` or `off`; explicit `base.mounts` grants are available only with automount and Windows interop both off |

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

⛔ **An instance and its configuration name one distribution.** A configuration
whose `base.name` is not the selected instance's distribution is refused with exit
2, whichever file the search resolved: `wsl-toolkit-muse` belongs to `--instance
muse`, and `wsl-toolkit` to no instance at all. The message names the file, both
distributions, and the `--instance` or `--config` that agrees, and `wsl-toolkit
config` still prints which file won and where it looked.

⛔ **A named instance's own configuration comes before the directory a command runs
in.** When `%LOCALAPPDATA%\wsl-toolkit\instances\NAME\config.json` exists,
`--instance NAME` reads it from any directory, so one base serves every project and a
project's `wsl-toolkit.json` does not change it. The project's file still applies
to the default instance and to a named one with no file of its own, and `--config`
comes before both.

⛔ **A changed `base.automount` is applied, not only reported.** The verification
reads the guest's drives from `/proc/mounts`: every mount at or below a one-letter
directory under `/mnt`. `base status --probe` prints the setting and what the drives
read, `off`, `ro`, `rw`, or `mixed` where a writable mount is beside a read-only one,
and `--json` carries the second as `access.automount_guest`. A base whose drives do
not match the setting answers exit 1 and names what disagrees. `base ensure`
provisions it again, which restarts the distribution and stops what runs in it.
`base shell` reads the same mounts: `--root` names each drive as read-only or
writable, and `--here` into a base with no drive mounted is refused with exit 2 and
`base ensure`.

| measured on 2026-09-14, on a throwaway arch base with interop off | result |
| --- | --- |
| each of the six changes between `off`, `ro` and `rw`, three on a base built with the first value | `base status --probe` exit 1, naming the setting and the drives; `base ensure` exit 0 in 4.3 s to 4.7 s, provisioning again; then all ten drive mounts read-only under `ro`, writable under `rw`, and none under `off` |
| guest root remounts one drive `rw` in an `ro` base | `base status --probe` exit 1, the drives `mixed`, 9 read-only and 1 writable; `base ensure` exit 0 in 4.5 s, and the drive `ro` again |
| `base shell --here` on a base built `off` and set to `ro` | exit 2 naming `base ensure`; after it, the shell started in a directory under `/mnt/c` |

`base exec` is the non-interactive host-to-guest seam. It starts as the
configured account in that account's home unless `--dir` names an absolute
guest path, sends the command or `--script` body framed on stdin with
`/dev/null` as the command's own stdin, forwards output, and returns the guest
exit status. `--root` is an explicit administrative variant.

⚠ **A process `base exec` starts in the background does not outlive the command.**
Measured on 2026-09-14 in a systemd base: `setsid sleep 3600 &` was gone by the next
command, two seconds later. Run anything that has to keep running in a herdr pane.

### Grants that change live

```powershell
wsl-toolkit --instance base base grant --source C:\path\to\project --mode rw
wsl-toolkit --instance base base revoke --target /workspaces/project
```

`base grant` mounts one Windows directory at `/workspaces/` and its own name, or at
`--target`, read-only unless `--mode rw`. `base revoke` unmounts one. Each changes
the running base first, reads every grant back as the account, and only then writes
the configuration file in effect, so a later restart and `base ensure` mount the same
directories. Nothing restarts, so nothing running in the base stops.

⛔ **Neither replaces what it would disturb.** A directory or a target already granted
differently is refused with the revoke to run first, and a grant a process is
standing in stays mounted, with its configuration and fstab entry untouched and exit 1.

| measured on 2026-09-14, on a throwaway arch base | result |
| --- | --- |
| two `base grant`, then `base revoke` | 0.4 s to 0.5 s each; a process in a herdr pane started before the grants was running after both, and one started before the revoke was running after it |
| `git status --short` through `base exec --dir` in a granted checkout | exit 0 |
| `wsl --terminate`, then `base ensure` | 6.6 s and 10.2 s in two runs, and both grants verified |
| `base revoke` while a pane's process stood in the directory | exit 1 naming `target is busy`, everything else unchanged; after the pane closed, the revoke took 0.4 s |

⚠ `passwordless_sudo: true` gives the configured account unrestricted guest
root. Guest root can manually mount Windows paths, so this setting serves a
trusted agent and is not a containment boundary.

### Adapters, and herdr

A base's `adapters` list names the software `base ensure` installs for its agents,
after provisioning and in order. [`adapters/README.md`](adapters/README.md) is the
contract for writing one.

```json
"adapters": [{ "name": "herdr" }]
```

⭐ **`herdr` puts the agents' multiplexer in the base, and a way in from Windows
that listens on nothing.** In the base: herdr 0.9.0 from its release asset,
digest-checked, at `/usr/local/bin/herdr`; its server as the system unit
`wsl-toolkit-herdr.service`, started with the base and never restarted by an ensure,
because a restart ends every pane; and the tracked configuration from
[`adapters/herdr/config.toml`](adapters/herdr/config.toml), written whole each time.
On this machine: a dedicated key at `%USERPROFILE%\.ssh\wsl-toolkit\id_ed25519`, the
base's host key in `known_hosts` beside it, and one marked `Host` block at the top
of `%USERPROFILE%\.ssh\config`, whose `ProxyCommand` starts `sshd -i` through
`wsl.exe` for one connection.

⛔ **The door accepts that one key, for the base's account, and nothing listens.**
The distribution's own `sshd` units are masked, the key carries `restrict`, and
`base status --probe` fails the adapter over a second key or a listening `sshd`.
Anything that can reach the door could already run `wsl.exe` as this Windows
account.

```powershell
wsl-toolkit --instance base base attach
```

`base attach` prints the commands with the values filled in, and runs none of them:
`herdr --remote wsl-toolkit-base --remote-keybindings server` from Windows, the
`base shell` line for a client inside, and a `base exec` line for an agent.
`--json` answers the same as a document, with exit 1 and the reason when this
machine's half is missing. [`examples/common/herdr.md`](examples/common/herdr.md) is
the guide for the operator and for an agent.

| measured on 2026-09-14, on a throwaway arch base | result |
| --- | --- |
| `base recreate` with the adapter, then `base ensure` | 68.7 s from nothing, the download and its digest included; then 3.7 s, rewriting only the tracked configuration |
| `ssh wsl-toolkit-NAME` through the block | key authentication and a command in 0.2 s |
| herdr's Windows client, `herdr --remote` | connected to the base's server through the block `base ensure` wrote, and detached on prefix then q |
| `wsl --terminate`, then one `ssh` through the block | the distribution started, the unit started the server, and herdr restored its workspaces, in 5.4 s |
| twelve minutes with nothing attached | the base stayed running |
| prefix then x, then prefix then shift+x, in two sessions made alike | herdr's own keys closed a pane, then a tab, each at once with no question; the tracked file closed nothing |

⭐ **`muse` installs Muse Code for the base's account, and runs Meta's installer only
while its digest is approved.** `base ensure` saves the installer Meta serves, prints
its length and SHA-256, and runs it as the account when that digest is the one the
adapter pins, which the operator approved on 2026-09-14, or the `installer_sha256`
the configuration's `muse` entry carries. ⛔ Any other stops the
ensure with exit 2, keeps the file at `/var/lib/wsl-toolkit/muse/install.sh` and
prints how to read it. Add its digest only after reading it:

```json
"adapters": [{ "name": "herdr" }, { "name": "muse", "installer_sha256": "SHA256" }]
```

A base whose Muse answers a version does not fetch the installer again. ⚠ Muse's own
launcher, as read on 2026-09-14, looks for a newer launcher and a newer Muse at most
hourly after that. `/usr/local/bin/muse` puts it on `PATH` for `base exec`, whose shell reads no
profile, and refuses every other account. ⛔ **Signing in is the operator's:** `base
shell`, then `muse login`. `base status --probe` reports the version, where `muse`
resolves, and whether a credential file is present, never what it holds.

| measured on 2026-09-14, on a throwaway arch base from a fresh clone | result |
| --- | --- |
| `base ensure` from nothing, with `herdr` and `muse` | 77.6 s; Muse Code 1.2.1 installed as the account, approved by the pinned digest |
| a build whose pin differs, over a base with no launcher | exit 2 in 4.4 s, the installer saved and not run, its digest and the approving key printed |
| the same build, with that digest as `installer_sha256` | exit 0 in 5.6 s, approved by the configuration |
| `base ensure` over an installed Muse | exit 0 in 3.8 s, the installer not fetched |
| `muse --version` through `base exec`, then as root | `Muse Code 1.2.1 (1.2.1-R2847.1)`, then exit 126 |

⚠ **Both adapters are driven on the `arch` preset, herdr with systemd, and a
configuration naming either on anything else is refused.** Removing herdr from a
configuration takes this machine's half away on the next ensure and leaves the base's
half, and removing either leaves what it installed in the base; `base recreate`
removes that. `base remove` takes this machine's half away and keeps the key, which
is this tool's and not the base's. `WSL_TOOLKIT_SSH_DIR` names a directory to write
this machine's half into instead of `%USERPROFILE%\.ssh`, which the acceptance
runner uses; herdr's own client does not read it.

### Agents from Windows

```powershell
wsl-toolkit --instance base base grant --source C:\path\to\project --mode rw
Set-Location C:\path\to\project
muse --version
```

⭐ **An agent runs in the base, and Windows reaches it from the project it stands
in.** The `muse` adapter writes `muse.exe` into `%USERPROFILE%\bin` for the instance
`base`, and `muse-NAME.exe` for any other instance, as a copy of this executable.
Started under that name it is `wsl-toolkit --instance base base agent muse -- ARGS`:
it finds the grant that covers the working directory, runs `muse ARGS` as the base's
account at the guest path that directory is granted at, through the framed channel
`base exec` uses, and answers Muse's own exit code. Each argument is single-quoted for
the guest's shell, so none is read as shell syntax, and Muse's stdin is `/dev/null`.

⛔ **A directory no grant covers is refused** with exit 2 and the `base grant` line
for it. A grant covers its directory and what is beneath it, and never a sibling
whose name starts the same way. ⛔ **Muse's own screen needs a terminal**, so `muse`
alone and `muse resume` answer exit 2 with the herdr route, rather than start it
where it cannot draw.

⚠ **The launcher is a copy, so an update of this tool leaves it behind.** `base status
--probe` names a launcher that is another build, and `base ensure` rewrites it. It
never writes over, or removes, a file that is not a build of this tool, which it
reads from the file's Go build information without running it. `WSL_TOOLKIT_BIN_DIR`
names a directory to write it into instead, which a throwaway base uses.

| measured on 2026-09-14, on a throwaway arch base with the muse adapter | result |
| --- | --- |
| `base ensure` from nothing, the instance's own configuration | 76.6 s, and the launcher written |
| `muse-m78.exe --version` in the granted project, then in a directory beneath it | `Muse Code 1.2.1 (1.2.1-R2847.1)` and exit 0, both |
| the same in a directory no grant covers | exit 2, with the `base grant` line |
| `muse-m78.exe` with no argument | exit 2, naming the herdr route |
| `muse-m78.exe --definitely-not-a-flag` | exit 2, Muse's own, which `base exec` also read |
| a launcher from another build | the probe exit 1 naming it; `base ensure` rewrote it, and the probe exit 0 |
| `base remove --yes` | the launcher removed with the distribution |

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

⚠ **Every run boots the guest and powers it off, so every run pays about 23
seconds.** Measured on 2026-09-14 over three runs of `bsd run -c true` with the
one-vCPU default: a login prompt at 8.5 s, 8.3 s and 8.5 s, the command finished at
16.4 s, 16.2 s and 16.4 s, and the process gone at 23.3 s, 23.0 s and 23.3 s. Nothing
keeps a guest running between runs. ⚠ The first run after `bsd fetch` pays the
image's first boot: FreeBSD grows its root to the disk and generates its SSH host
keys before the login, which came at 29 s on 2026-09-14. ⚠ The guest is not shown
the host's hypervisor signature, because a FreeBSD kernel that sees it waits about
105 seconds before mounting root, for a Hyper-V VMBus QEMU does not provide.

⭐ **The default is one vCPU.** Five fresh-image toolchain installs completed
without a kernel panic with one processor on 2026-09-14, where the same guest
panicked under every two-processor CPU model and memory size measured. ⚠ **One
processor lowers the rate and does not end it:** the same day, two read-only runs in
a row on the shared image panicked while powering off. `--cpus` overrides the
default.

⭐ **A script reaches the guest as a file, byte for byte.** `-c` and `--script` travel
on a second read-only disk and run from a copy in `/tmp`, so a comment, a blank line
and a command split across lines run as written. Its stdin is `/dev/null`, and the
exit code is the script's own.

⭐ **The guest disk is 12 GiB, which gives a 10 GiB root filesystem.** The published
image is 6.0 GiB with a 4.8 GiB root, and a toolchain install fills that. `bsd run`
grows the image file to `--disk` GiB, 12 by default, before it boots, then extends
the partition and the filesystem with FreeBSD's own `gpart` and `growfs` before the
payload runs. ⚠ A boot partition, an EFI partition and 1 GiB of swap sit ahead of
root, so the root is smaller than the disk: measured on 2026-09-14 by `df -k /`, an
11 GiB disk gives a root of 10,110,092 KiB and a 12 GiB disk 11,138,540 KiB. `bsd
status` prints the disk, and the run's last line prints both sizes.

⛔ **It never shrinks.** Every session on this host shares the image, and a shorter
file cuts off the filesystem inside it, so a `--disk` smaller than the image is
refused. `bsd fetch --force` goes back to the published image.

⛔ **The guest kernel can panic, and a panic can leave the shared image unable to
boot.** Measured on 2026-09-14: `bootstrap.sh --toolset languages` in a 2048 MiB
guest panicked in the page daemon while `pkg` installed, and the next boot stopped
at a filesystem check that needs a single-user shell. `bsd run` ends a run as soon
as the console shows a kernel panic or QEMU exits, with exit 2 and the panic line,
and a later boot that stops at that check is named as it happens. ⚠ The payload's
output is on the same console, so a payload that prints a FreeBSD panic's own two
lines, `panic: ` and `cpuid = ` under it, can end its run the same way. `bsd fetch
--force` restores the published image. When the archive it keeps is whole, that is a
digest check and an expansion, measured at 84 s and at 13.1 s on this host, with no
download.

⚠ **A panic while the guest powers off leaves the payload's exit standing**, and the
run warns and carries it as `shutdown_panic`. ⛔ **It can come before the buffers
sync, and then the image's filesystem is left unchecked.** Measured on 2026-09-14
with one processor: a run's poweroff panicked in `pmap_remove_pages` while
`rc.shutdown` stopped processes, before `Syncing disks`. The next boot printed `/ was
not properly dismounted`, saved a 173,228,032-byte core into `/var/crash`, and set its
filesystem check for 60 seconds after boot, which a short run never reaches. That
run's own poweroff then panicked inside the filesystem, in `initiate_write_filepage`.
`bsd fetch --force` put the published image back.

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
| `bsd run` boots and powers off a guest per call, about 23 seconds each | host | nothing keeps a guest running between runs. The BSD section carries the measurement |
| a FreeBSD kernel panic at poweroff can leave the shared guest image's filesystem unchecked | open | the next run boots on it, and can panic on it. The BSD section carries the measurement, and `bsd fetch --force` puts the published image back. Tracked in [`../../../TODO/wsl-toolkit-go.md`](../../../TODO/wsl-toolkit-go.md) |
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
