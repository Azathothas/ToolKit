# wsl-toolkit

One executable for Linux work on a Windows host. It surveys the machine, owns a
WSL distribution with a container engine in it, runs a command in one container
or in a set of them, and removes what it made.

This page stands alone. An agent that has read only this file can use the tool
correctly, from a release or from a clone, without opening the source.

⚠ **Windows only for anything that touches WSL.** `doctor`, `images`, `config`
and `version` answer on any host; every other command refuses by name off
Windows.

---

## Start here

```powershell
wsl-toolkit doctor
wsl-toolkit base ensure
wsl-toolkit run --image alpine -c 'uname -a'
```

⛔ **Do not call `wsl.exe` directly and do not write another wrapper.** A payload
handed to `wsl.exe` as an argument is expanded before the guest sees it and the
result is parsed again. Measured 2026-09-09 on Windows 11 Pro 26200: a backtick
in an argument was executed and the command still exited 0. This tool sends every
payload on stdin or as a file, and never as an argument.

## Commands

| command | what it does |
| --- | --- |
| `doctor` | what this host is and what is installed, resolved past every shim. Creates nothing. |
| `script` | run the embedded `wsl-toolkit.ps1`, forwarding every argument unchanged |
| `base` | the one WSL distribution this tool owns, which hosts a rootless engine |
| `images` | the container catalog |
| `run` | one command in one container, with a copy of a workspace and no host mount |
| `matrix` | one command across a set of images |
| `resources` | what this tool holds, and what the machine holds that is not its |
| `gc` | remove what this tool made. Reports first; `--apply` acts. |
| `helper` | the opt-in local helper, for a caller that cannot reach `wsl.exe` |
| `config` | where the configuration is and what it says |
| `version` | the product version, read from the embedded script |

Global flags: `--home DIR` (also `WSL_TOOLKIT_HOME`), `--quiet`, and `--json` on
every command with a structured answer.

⭐ **Progress goes to stderr. stdout carries the answer alone**, so a caller can
assign it:

```powershell
$addr = wsl-toolkit script -Action HostAddress 2>$null
```

## Exit codes

| code | meaning |
| --- | --- |
| 0 | it ran and it agreed |
| 1 | it ran and it disagreed: a job failed, a fleet row failed |
| 2 | it could not run: bad usage, a missing base, a refusal |
| 124 | a deadline was reached, as coreutils' `timeout` reports it |
| the container's own code | from `run`, forwarded verbatim |
| the script's own code | from `script`, forwarded verbatim |

---

## `doctor`

```powershell
wsl-toolkit doctor
wsl-toolkit doctor --json
wsl-toolkit doctor --group runtime
wsl-toolkit doctor --fast    # find the tools without asking each one its version
wsl-toolkit doctor --net     # probe outbound HTTPS
```

⛔ **A name on `PATH` is not an executable.** `scripts/doctor/doctor.sh` resolves
a name through a POSIX shell's `PATH` and `doctor.ps1` through Windows'; both are
right and they disagree. This one opens the file and reports what would run,
with the name that was found underneath it:

```text
  yes rustc            1.98.0                          <profile>\.cargo\bin\rustup.exe
      via <profile>\.cargo\bin\rustc.exe
```

⚠ **The version is read by running the path that was FOUND, not the one it
resolves to.** `rustc.exe` resolves to `rustup.exe`, which decides what to do
from the name it was invoked under.

| `kind` | what it means for a caller |
| --- | --- |
| `executable` | hand it to process creation and it runs |
| `scoop-target` | the name on `PATH` is a shim; the resolved path is the real file |
| `command-script` | ⚠ a `.cmd`. Process creation cannot run one; it needs `cmd.exe /d /s /c`. |
| `powershell-script` | ⚠ a `.ps1`. Process creation throws "not a valid application for this OS platform". |
| `app-execution-alias` | ⚠ a zero-byte Windows stub. It may open the Store rather than run anything. |
| `unresolved-link` | the file runs and its canonical path could not be read |

The WSL section answers what a sandboxed caller needs:

```text
WSL
  installed      true
  callable here  NO, this process is refused
  base distro    wsl-toolkit (registered)
  local helper   127.0.0.1:51234
```

⛔ **"installed" and "callable here" are different facts.** A restricted process
gets `E_ACCESSDENIED` on a machine where WSL works. When that happens the report
names both ways forward: the session's own approval path, or the helper.

---

## `base`

```powershell
wsl-toolkit base status --probe
wsl-toolkit base ensure
wsl-toolkit base presets
wsl-toolkit base ensure --preset alpine --save
wsl-toolkit base recreate
wsl-toolkit base remove --yes
wsl-toolkit base shell
```

One WSL distribution called `wsl-toolkit`, holding a rootless podman and one
unprivileged account called `toolkit`. Containers run inside it, so nothing a job
does reaches `podman-machine-default` or the images somebody else put there.

⛔ **This tool acts on exactly one distribution, by exact name.** Every other
name is refused. `wsl --shutdown` is machine-wide and appears nowhere in it.

⭐ **`base ensure` is the recovery path as well as the create path.** A
registered distribution that cannot run a container is re-provisioned in place;
one that will not provision is removed and rebuilt. `base recreate` starts from
nothing. The base is meant to be wrecked.

⚠ **`registered` is not `usable`.** A rootless engine reports a complete
configuration and then fails at the first run. `--probe` runs a real container
and reads back a marker the command could not have echoed; without it, `status`
says the check was not made rather than implying it passed.

### Presets

A preset id or any fully qualified reference works in the same place.

| preset | libc | measured on Windows 11 Pro 26200, 2026-09-09 |
| --- | --- | --- |
| ⭐ `arch` (default) | glibc | 31s, 940 MiB disk, podman 6.1.1 |
| `alpine` | musl | 28s, 204 MiB disk, podman 5.8.6 |
| `debian` | glibc | 37s, 556 MiB disk, podman 5.4.2 |
| `fedora` | glibc | not measured |

Each figure is `base ensure` to a rootless container returning a marker, with the
image already in the host engine's cache. Arch adds about 38s on a cold pull of
its 540.8 MiB rootfs.

⭐ **The default is glibc, not the fastest.** The base is where a toolchain gets
installed when somebody wants one outside a container. ⚠ It has no bearing on
what a CONTAINER runs, which brings its own userland.

⚠ **Switching preset rebuilds**, because a distribution's rootfs cannot be
changed underneath it. `--save` also makes the choice the stored default; without
it the configuration is unchanged and `base status` reports the disagreement.

---

## `run`

```powershell
wsl-toolkit run --image alpine -c 'apk add gcc && gcc --version'
wsl-toolkit run --image debian --workspace . --artifacts .\out -c 'make && cp build/x /out/'
wsl-toolkit run --image arch --script .\build.sh --timeout 45m
```

⛔ **No host directory is ever mounted into a container.** A job gets a COPY of
its workspace, and what it does to that copy cannot reach this machine.

| the container sees | what it is |
| --- | --- |
| `/work` | a copy of `--workspace`, inside the distribution's own filesystem |
| `/out` | empty. Whatever is left there is fetched to `--artifacts`. |
| `/job.sh` | the command, read-only |

⛔ **Every archive entry is validated before it is written**, in both directions.
Four things are refused by name: an absolute path, a name containing `..`, a link
leaving the tree, and a Windows reserved device name.

| flag | meaning |
| --- | --- |
| `--image` | a catalog id, or any fully qualified reference |
| `-c` | the command. ⛔ Mutually exclusive with `--script`. |
| `--script` | a file whose bytes are the command. CRLF becomes LF in the copy; the file on disk is never written to. UTF-16 is refused. |
| `--workspace` | a directory on this machine, copied in as `/work` |
| `--artifacts` | a directory on this machine to receive `/out` |
| `--exclude` | a glob to leave out of the copy. Repeatable. |
| `--env` | `NAME=VALUE`. Repeatable. |
| `--timeout` | default 30m. On expiry the container is killed and the code is 124. |
| `--no-network` | run with no network at all |
| `--user` | what the container runs as: a name, a uid, or `uid:gid`. Empty is the image's default, which for most is root. ⚠ Naming one makes podman re-own the mounts for it, which costs a walk of the copy. |
| `--max-bytes` `--max-entries` | ceilings for a workspace and an artifact set. Default 1 GiB and 200000. |
| `--ensure-base` | build the base first when it is missing. On by default. |
| `--via-helper` | force the local helper route |

⚠ **A workspace or an artifact set over a ceiling is refused, never truncated.**

---

## `matrix`

```powershell
wsl-toolkit matrix --images all --script .\probe.sh --transcripts .\tr
wsl-toolkit matrix --images libc:musl -c 'ldd --version'
wsl-toolkit matrix --images kind:legacy --workspace . -c 'make'
```

Takes every `run` flag, plus `--images`, `--parallel` and `--transcripts`.

A selector is a catalog id, `libc:musl`, `libc:glibc`, `kind:musl`,
`kind:glibc`, `kind:niche`, `kind:legacy`, or `all`; several are comma
separated. ⛔ **A selector that matches nothing is a refusal**, because a fleet
that ran nothing reads exactly like a fleet where everything agreed.

The workspace is sent once and copied per row inside the distribution. Each row
gets its own copy.

```text
  alpine    ok            1.927s  docker.io/library/alpine:latest
  arch      ok           26.039s  ghcr.io/pkgforge-dev/archlinux:latest
  gentoo    ok           48.633s  docker.io/gentoo/stage3:latest

  12 ran, 0 failed, 0 unreached, 0 timed out, in 52s
```

⛔ **Three counts, because "the image could not be pulled" and "the subject is
broken" need different next moves.**

| exit | meaning |
| --- | --- |
| 0 | every row ran and every row agreed |
| 1 | at least one row disagreed, or could not be reached |
| 2 | ⛔ nothing ran |

⚠ **Four rows at a time by default.** Every row is a container inside one WSL
distribution sharing one utility VM's memory. `--parallel` past four is a
decision about that machine.

`--transcripts DIR` writes one file per row.

---

## The catalog

```powershell
wsl-toolkit images
wsl-toolkit images --select libc:musl
```

⛔ **Every reference is fully qualified.** An engine resolves an unqualified name
through its own alias table, so the same string is two different images on two
machines. The base distribution sets `short-name-mode = "enforcing"`, so a short
name typed inside it is refused too.

| id | libc | kind | reference |
| --- | --- | --- | --- |
| `alpine` | musl | musl | `docker.io/library/alpine:latest` |
| `void-musl` | musl | musl | `ghcr.io/void-linux/void-musl:latest` |
| `chimera` | musl | musl | `docker.io/chimeralinux/chimera:latest` |
| `arch` | glibc | glibc | `ghcr.io/pkgforge-dev/archlinux:latest` |
| `debian` | glibc | glibc | `docker.io/library/debian:latest` |
| `fedora` | glibc | glibc | `registry.fedoraproject.org/fedora:latest` |
| `gentoo` | glibc | niche | `docker.io/gentoo/stage3:latest` |
| `wolfi` | glibc | niche | `cgr.dev/chainguard/wolfi-base:latest` |
| `photon` | glibc | niche | `docker.io/library/photon:latest` |
| `rocky8` | glibc | legacy | `quay.io/rockylinux/rockylinux:8` |
| `ubuntu2204` | glibc | legacy | `docker.io/library/ubuntu:22.04` |
| `debian12` | glibc | legacy | `docker.io/library/debian:12-slim` |

⚠ **Where a maintainer publishes somewhere other than Docker Hub, that is the
row**, because Docker Hub's anonymous pull limit is the likeliest cause of a red
row that has nothing to do with the subject.

⭐ All twelve ran on Windows 11 Pro 26200 on 2026-09-09: 12 ran, 0 failed, 0
unreached, 52.1s with cold pulls, artifacts returned from every row.

### Changing it

```powershell
wsl-toolkit config
wsl-toolkit config --write
```

`--write` puts the effective configuration on disk to edit, carrying the current
catalog. `images` in it REPLACES the built-in list; `matrix` names the ids a
fleet uses when the caller names none. ⛔ **A file this build cannot read is a
refusal, not a fall back to the defaults.**

---

## `resources` and `gc`

```powershell
wsl-toolkit resources
wsl-toolkit gc
wsl-toolkit gc --apply --older-than 24h
wsl-toolkit gc --apply --images     # also prune images in the base
```

⛔ **The report is in two parts and says on every line which it is:** what this
tool made, and what the machine holds that is not its. Only the first is ever
removed.

⛔ **`gc` reports by default; `--apply` acts.** It finishes both loops before it
reports, so one item it cannot remove does not stop it removing the rest.

⚠ **It matches on a label this tool stamped, never on a name pattern.**

⭐ **A killed run still leaves a record.** Every resource is written to
`ledger.jsonl` before it exists, so an interrupted run leaves an open record with
no close, and that is what `gc` looks for.

---

## `script`

```powershell
wsl-toolkit script -Action List
wsl-toolkit script -Action New -Image docker.io/library/alpine:latest -Ephemeral -Command 'echo hi'
wsl-toolkit script -Action Doctor
```

The embedded `wsl-toolkit.ps1`, with every argument forwarded unchanged and its
exit code returned.
[`../../../scripts/windows/wsl-toolkit/wsl-toolkit.md`](../../../scripts/windows/wsl-toolkit/wsl-toolkit.md)
is its page.

⭐ **Throwaway distributions and the owned base are different tools.** `script
-Action New` builds a distribution from any image and destroys it; `base`
maintains one that persists and holds an engine. Neither can remove the other's.

---

## The local helper

```powershell
wsl-toolkit helper serve --detach
wsl-toolkit helper status
wsl-toolkit helper stop
```

For a process that is refused when it calls `wsl.exe`. Start it once through the
approval path the session offers; it holds the access and later commands hand it
job data.

⭐ **Nothing has to know it is there.** A command that can reach `wsl.exe` uses it
directly. One that cannot finds the helper and says so once. `--via-helper`
forces the route.

⛔ **It is a lifetime boundary, not a privilege boundary.** It listens on
loopback, runs as whoever started it, and its token lives in that user's own
state directory. It buys one approval instead of one per command; it adds no
permission anybody did not already have.

⛔ **What the protocol will not carry:**

| | |
| --- | --- |
| a host path | a workspace is uploaded as an archive and artifacts are downloaded as one, so the helper never opens a file the caller named |
| a host command | there is no endpoint that runs a program on this machine |
| a raw engine option | podman's flags are not reachable, so a mount cannot be asked for |
| a WSL lifecycle call | it builds and repairs the base and can unregister nothing. `base remove` and `base shell` are not on the protocol. |

⭐ **What it does carry is every job flag the direct path has**, `--user`
included. A flag one route honours and the other drops is a job that ran as
somebody else with nothing said; the acceptance runner asks both routes the same
question and compares the answers.

⚠ The token is compared in constant time and the endpoint file is written `0600`.
On Windows those mode bits are not an access control list; what protects the file
is that `%LOCALAPPDATA%` is that user's own directory.

---

## Where things live

| | |
| --- | --- |
| state | `%LOCALAPPDATA%\wsl-toolkit`, or `WSL_TOOLKIT_HOME`, or `--home DIR` |
| the base distribution's disk | `<state>\base\ext4.vhdx` |
| the configuration | `<state>\config.json` |
| the ownership record | `<state>\ledger.jsonl` |
| the helper's endpoint | `<state>\helper.json` |
| the extracted script | `<state>\script\wsl-toolkit-<digest>.ps1` |

⚠ **It is not the script's directory.** `wsl-toolkit.ps1` keeps ephemeral
distributions under `%LOCALAPPDATA%\wsl-ephemeral` and its `Purge` removes every
distribution it finds there. The base has to survive a purge.

---

## Getting it

```powershell
pwsh -NoProfile -File launcher.ps1 -Action List
pwsh -NoProfile -File launcher.ps1 doctor
```

The launcher fetches and verifies it and defaults to the executable.
[`../../../scripts/windows/wsl-toolkit/launcher.md`](../../../scripts/windows/wsl-toolkit/launcher.md)
is its page, including `-LauncherKind script`.

Or take the release asset directly: `wsl-toolkit-windows-amd64.exe` or
`wsl-toolkit-windows-arm64.exe`, with the `SHA256SUMS` published beside it.

⚠ **The release `SHA256SUMS` proves transport, not authorship.** It comes from
the same release as the asset.

---

## Known limits

| limit | what it means for you |
| --- | --- |
| ⛔ a job's stdin is not the caller's | the command travels as a file, so nothing reads this process's stdin. An interactive container is `base shell` plus `podman run -it` inside it. |
| ⚠ the fleet's rows share one utility VM | memory and disk are one machine's |
| ⚠ a link in an artifact set is recorded, not recreated | a `.link.txt` beside where it would have been |
| ⚠ `base status` without `--probe` cannot say a base is usable | running a container is the only thing that answers it |
| ⛔ there is no `--platform` | a container runs this host's architecture. A foreign rootfs on this kernel may RUN rather than fail, through a `binfmt_misc` handler something else registered. |
| ⚠ a musl base runs glibc containers | the base's libc bounds what is installed IN THE BASE only |

---

## Requirements

| thing | needed for |
| --- | --- |
| Windows 10 2004+ or Windows 11, with WSL2 | `base`, `run`, `matrix`, `resources`, `gc`, `script` |
| podman or docker on the host | building the base only. Jobs use the engine inside it. |
| Windows PowerShell 5.1 or PowerShell 7+ | `script` |
| about 6 GiB free, plus twice the rootfs | the base refuses an import the volume cannot hold |

---

## Related

- [`README.md`](README.md), how this executable is built, tested and released
- [`../../../scripts/windows/wsl-toolkit/wsl-toolkit.md`](../../../scripts/windows/wsl-toolkit/wsl-toolkit.md),
  the PowerShell product this one carries
- [`../../../scripts/windows/wsl-toolkit/launcher.md`](../../../scripts/windows/wsl-toolkit/launcher.md),
  fetching and verifying either product
- [`../../../docs/consumers.md`](../../../docs/consumers.md), who fetches from
  this repository and what a change here breaks
- [`../../../CHANGELOG.md`](../../../CHANGELOG.md), what shipped and the defects
  behind each change
