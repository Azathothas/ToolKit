# WSLc 3.0.1 vs wsl-toolkit, and wslc-remote

⭐ **Moved into the tree on 2026-10-21 from the attachment to
[issue 35](https://github.com/Azathothas/ToolKit/issues/35)**, SHA-256
`f7d154c3...6beaa0f`. This file is that text with this note added under its title
and section 10 appended. ⛔ **Nothing else in it is edited in meaning**: it is what
was believed on 2026-09-30, from source, on a host with no `wslc`. Section 10 is
what this host then measured on WSL 3.0.3, written underneath as corrections and
answers. `WSL-110`

⚠ **Six kinds of character this tree does not carry are written out**, so the text
passes the checks every page here passes: the em dash as ` - `, the section sign as
`section`, the empty-set sign as `BY-STRUCTURE`, an arrow as `->`, `Consommé` as
`Consomme`, and the shim's 40-digit commit as its first 12. The issue keeps the
original.

An analysis written on 2026-09-30 for `Azathothas/ToolKit`, to decide whether
Microsoft's newly generally-available **WSL containers** changes what
`wsl-toolkit` is for, and whether the community `wslc-remote` shim belongs
anywhere near it.

This document is standalone. It names its evidence, states what it did not
verify, and carries no claim that survives only because someone read it.

⛔ **This file lives in `.tmp/`, which the repository ignores. A later session
that wants it must move it into the tree.** Two further caveats on the citations,
because "standalone" has to mean more than "readable":

- **wsl-toolkit `file:line` citations are against commit `8a7e03e`**, the tip of
  `main` when this was written, and are identical at the `wsl-toolkit-v6.0.0`
  tag. They will drift as the tree moves. Where a line number is load-bearing,
  the quoted text is given alongside it, so a drifted line is still checkable.
- **wslc `file:line` citations are against the 3.0.1 tag** unless the text says
  master. The source is public and MIT, so any of them is re-checkable with
  `curl https://raw.githubusercontent.com/microsoft/WSL/3.0.1/<path>`. Where this
  document relies on a scratch pull, the URL is given so the pull can be redone.

---

## 0. Scope, and the one constraint that shapes every claim

**The host that wrote this runs WSL 2.7.12.0, kernel 6.18.33.2-2, and has no
`wslc.exe`.** Verified with `wsl.exe --version` and `where wslc` at the time of
writing. WSLc ships in WSL **3.0.1**, published 2026-09-29T17:18:57Z
(`gh release view 3.0.1 --repo microsoft/WSL`).

**No wslc command, and no wslc-remote command, was executed.** Every claim about
wslc is read from an artefact: the source in the public MIT repository
`microsoft/WSL`, the release notes, the GA announcement, and the Microsoft Learn
pages. Every claim about wsl-toolkit is read from this repository's code, with
`file:line`. A reader on a WSL 3.0.1 host can check every wslc claim with
`curl`; that is the main reason this document is written the way it is.

**Provenance of the wslc source read here.** The 62 files under
`.tmp/wslcsrc/` were pulled from `github.com/microsoft/WSL` **master**. For the
load-bearing question - whether `wslc session` exists in the shipped release  - 
master and the **3.0.1 tag** were compared directly, and the answer is in
section 4.2. Where a claim rests on master and not on the tag, it says so.

**Provenance of the wslc-remote source read here.** `craigloewen-msft/WSLc-remote`
at commit **`e53d9a887db5`**, authored
2026-09-29T11:43:50-04:00, message "Fixed install". Shallow clone. Five tracked
files: `wslc` (489 lines), `install.sh` (365), `README.md` (119), `LICENSE` (21),
`.gitignore` (15).

---

## 1. What wslc is, in one page

WSL containers is a **first-party Linux container runtime built into WSL**. It
is not a distribution manager and not a Docker replacement; it is a container
CLI plus a Windows API, delivered by a WSL service.

**The service model.** A client calls `wslservice.exe`, a privileged Windows
service that can create virtual machines through HCS. The service does **not**
retain ownership of the VM: it creates a child process, `wslcsession.exe`, which
**runs on behalf of the calling user** and performs every session operation  - 
creating containers, mounting directories, binding networking ports - in a
process less privileged than the service. Microsoft states the intent as "strong
isolation between sessions" plus "reinforced security boundaries"
(architecture deep dive, `https://devblogs.microsoft.com/commandline/wslc-architecture-deep-dive/`).

**Storage.** Each session has its own VHD holding images, containers, networks
and volumes, under `%AppData%\Local\wslc\sessions\storage.vhdx`
(`WSLCSessionDefaults.h:19-20`: `DefaultStorageSubPath = L"wslc\\sessions"`,
`DefaultStorageVhdName = L"storage.vhdx"`). Two volume kinds:
Windows-path volumes over **virtiofs**, and **VHD volumes** - a real ext4
filesystem on a VHD with a size ceiling
(`wslc volume create --driver vhd -o SizeBytes=200000000`).

**Networking.** A new model Microsoft calls **Consomme**: the VM's traffic leaves
as ethernet frames on a virtio queue, read by a Windows process running as the
user, which then provides DNS, UDP/TCP routing and **port publishing**. Egress
therefore looks like the user's own Windows process, which is what buys VPN and
firewall compatibility.

**The surface.** Read from the 3.0.1 tag's own command tree, not from prose:

| group | subcommands |
| --- | --- |
| `container` | attach, cp, create, exec, export, inspect, kill, list, logs, prune, remove, **restart**, run, start, stats, stop (`ContainerCommand.cpp`, 3.0.1 tag) |
| `image` | build, import, inspect, list, load, prune, pull, push, remove, save, tag |
| `network` | connect, create, disconnect, inspect, list, prune, remove |
| `registry` | login, logout |
| `system` | events, info |
| `volume` | create, inspect, list, prune, remove |
| `settings` | reset |
| `system session` | enter, list, run, shell, terminate - **the invocation is `wslc system session`**, because `SessionCommand` is a child of `SystemCommand` (`SystemCommand.cpp:29`) and `RootCommand.cpp:36-42` registers no `SessionCommand` at top level. See section 4.2 |
| top level | `inspect`, `version` |

The GA additions named in the announcement are each corroborated by a command
file that exists: `container restart`, `container cp`, `system info`, `network
connect`/`disconnect`, `network create` with arbitrary driver options, `wslc
events`, container **health checks**, `--stop-timeout` (including `-1`), `--mount`,
and a configurable storage path for the default session.

**What is absent, and matters.** `wslc compose` **does not exist**; the
announcement calls it the top feature request and says work has started. And
**no argument registered in the CLI table is named `platform` or `device`** - 115
definitions of which 110 are active and 5 are commented out, verified against
both the 3.0.1 tag and master, and the absence was then checked across the whole
3.0.1 tree rather than the CLI table alone (section 6.1). So: no multi-platform
selection, and no device passthrough flag. The word `platform` does appear
in wslc's source, as inspect/list output metadata (`ContainerTasks.cpp:199-200`
sets `entry.Platform.architecture`), so "the string does not occur" would be the
wrong check; the claim is that no *argument* of that name is registered.

**Enterprise.** Microsoft Intune gained "Allow WSL containers access" and a
registry allow list; the Microsoft Defender for Endpoint plugin for WSL was
extended to cover containers. Both are announcement prose with no backing policy
name in any public source I read.

**Not corroborated.** The "up to 2x faster" cross-OS file figure appears in
marketing prose only - the announcement and the deep dive. No benchmark, no
release note, no doc page carries a number. **Do not put a 2x in a decision
document.** wslc-remote's parallel "~13x" claim is worse: it cites a
`results/` directory that **does not exist in the repository** (section 7.3).

---

## 2. What wsl-toolkit is, in one page

A single stdlib-only Go executable for Windows. It owns one WSL distribution per
instance (`wsl-toolkit[-NAME]`), runs **rootless podman inside that
distribution** for every container job, and gives each job a **copy** of the
workspace rather than a mount.

The isolation argument is stated in the code and is not a slogan
(`internal/toolkit/engine.go:15-20`):

> Engine is a container engine on the **HOST**, used for exactly one job: turning
> an OCI image into a rootfs archive that `wsl --import` can read.
> ⚠ It is not the engine jobs run in. Jobs run in a rootless podman INSIDE the
> owned distribution, so nothing a job does can reach the machine's own engine
> or the images somebody else put in it.

And on the workspace (`internal/toolkit/job.go:445-447`): no host path reaches
the container; it gets two directories, both inside the distribution's own
filesystem, a copy of the workspace and an empty one to hand things back in.

Around that, wsl-toolkit has capability wslc does not touch at all:

- a **job control plane** - a 16-hex id from `crypto/rand`, an `owner.lock` the
  owner holds for the job's life and writes `result.json` before releasing, a
  JSONL ledger, and `run --detach` / `logs ID --follow` / `wait ID` / `stop ID` /
  `inspect ID` from any process;
- a **relay** - timestamp profiles, a silence heartbeat that reads the thing
  being watched, `--redact` reaching every copy, `--event-log`, progress tokens,
  replay and compare;
- a **grant model** - one Windows directory per `base grant` call, mounted under
  `/workspaces`, applied live, and a configuration that **refuses to validate**
  a grant unless the other two doors are shut (`internal/toolkit/catalog.go`:
  `base.mounts requires base.automount to be "off"` and
  `base.mounts requires base.interop to be "off"`). ⚠ "One at a time" is the
  tool's own usage idiom, not a schema cap: `PlanGrant` appends without a limit
  and `Validate` imposes none, so a configuration may carry several mounts. What
  is enforced is *what kind* of door each one is, not how many;
- automount and interop settings **verified by reading the guest back**, not by
  reading `wsl.conf`;
- `base doors`, which **attacks** its own containment and reports what it can
  open, with the rule that a door it cannot try is reported `unknown` and
  `unknown` is not `closed`;
- adapters that install a multiplexer (herdr) and three coding agents (muse, pi,
  omp) into the base, with per-agent model and effort defaults written to each
  agent's own door, plus an SSH door that listens on nothing;
- `base exec --private-net`, a FreeBSD guest on QEMU, throwaway distros, a helper
  protocol for processes that may not call `wsl.exe`, a fleet `matrix` runner, and
  a `doctor` that resolves past every shim.

Three of wsl-toolkit's own published limits are load-bearing for this analysis
(`tools/windows/wsl-toolkit/wsl-toolkit.md`, "Known limits"):

| limit | kind | the tool's own words |
| --- | --- | --- |
| no per-container resource bounds | host | `--memory` is accepted and not applied, `podman stats` reads `0B`; a job bounded on this base is not bounded. `WSL-60` |
| `podman logs` needs a storing log driver | host | the base default is `journald` and nothing serves a journal, so this tool names `k8s-file` on every job it starts |
| there is no port forwarding | decision | forwarding a port on Windows needs an elevated session and leaves a rule behind |

---

## 3. The matrix

Cells: **PRESENT**, **PARTIAL**, **ABSENT**, or **BY-STRUCTURE** where
the concept is about a Linux distribution and wslc has no such thing. No cell is
blank. wslc cells cite the 3.0.1 tag or master; wsl-toolkit cells cite this tree.

### 3.1 Container lifecycle

| capability | vanilla WSL 2.7.12 | wslc 3.0.1 | wsl-toolkit |
| --- | --- | --- | --- |
| run a container | ABSENT | PRESENT - `container run` | PRESENT - `run`, in-guest `podman` |
| create without starting | ABSENT | PRESENT - `container create` | ABSENT - create and start are one call |
| exec into a running container | ABSENT | PRESENT - `container exec` | ABSENT |
| start / stop / kill | ABSENT | PRESENT - start, `stop --signal --time`, `kill --signal` | PARTIAL - `stop ID` only, TERM then KILL, no raw signal |
| restart | ABSENT | PRESENT - `container restart` | ABSENT |
| rm / prune | ABSENT | PRESENT - `rm -f -v`, `prune -f --filter` | PRESENT - `gc` plans, `gc --apply` carries out |
| list | ABSENT | PRESENT - `container list` with format and quiet flags | PRESENT - `logs` with no id lists jobs and sessions |
| logs | ABSENT | PRESENT - `logs --follow --tail --timestamps --since --until` | PARTIAL - `logs ID --follow`, `--tail`; no `--since`/`--until` |
| inspect | ABSENT | PRESENT - `container inspect` and a top-level `inspect --type` | PRESENT - `inspect ID` |
| export a container | ABSENT | PRESENT - `container export -o` | ABSENT |
| cp in / out | ABSENT | PRESENT - `container cp` (GA) | PARTIAL - `--input NAME=FILE` to `/in`, one way in; `/out` + `--artifacts` is the way back |
| **`container.exe` alias** | ABSENT | PRESENT | n/a |

### 3.2 Images

| capability | vanilla WSL | wslc 3.0.1 | wsl-toolkit |
| --- | --- | --- | --- |
| pull / push | ABSENT | PRESENT | PRESENT - host pull for the rootfs export, in-guest pull for a job |
| **build from a Dockerfile** | ABSENT | PRESENT - `image build`, twelve flags plus a context path | **ABSENT** - nothing in the tree |
| list / inspect | ABSENT | PRESENT | PRESENT - `images`, `images warm` |
| tag / save / load / import | ABSENT | PRESENT | ABSENT |
| registry login | ABSENT | PRESENT - `registry login`, `--password-stdin` | ABSENT |
| **multi-platform selection** | ABSENT | **ABSENT** - no `platform` argument is registered | PRESENT and stricter - `platform` is named on every call |

### 3.3 Limits, health, devices

| capability | vanilla WSL | wslc 3.0.1 | wsl-toolkit |
| --- | --- | --- | --- |
| memory limit | ABSENT | PRESENT - `--memory` | **ABSENT and published as such** |
| cpu limit | ABSENT | PRESENT - `--cpus` | ABSENT, same cause |
| shm / ulimit / tmpfs | ABSENT | PRESENT | ABSENT |
| **health checks** | ABSENT | PRESENT - five `--health-*` flags plus `--no-healthcheck` | ABSENT |
| `--stop-timeout` incl. `-1` | ABSENT | PRESENT | own `--grace`, default 10 s |
| **device passthrough** | ABSENT | **ABSENT** - no `device` argument, in the tag or on master | PRESENT - `--device HOST[:CONTAINER[:PERMS]]` with a pre-flight node check |
| GPU | ABSENT | PRESENT - `--gpus` | ABSENT |
| foreign-architecture execution | ABSENT | ABSENT | PRESENT - QEMU binfmt handlers, restored on VM stop |

### 3.4 Storage

| capability | vanilla WSL | wslc 3.0.1 | wsl-toolkit |
| --- | --- | --- | --- |
| named volumes | ABSENT | PRESENT | ABSENT |
| Windows path into a container | ABSENT | PRESENT - virtiofs | **INVERTED - a copy, never a mount** |
| **VHD volume, sized** | ABSENT | PRESENT - `--driver vhd -o SizeBytes=` | ABSENT |
| `--mount` long form | ABSENT | PRESENT | ABSENT |
| configurable session storage path | ABSENT | PRESENT | PARTIAL - the tool's own state dir, but it is a distro disk |
| per-unit storage VHD | PARTIAL | PRESENT | PARTIAL - one distro per instance |

### 3.5 Networking

| capability | vanilla WSL | wslc 3.0.1 | wsl-toolkit |
| --- | --- | --- | --- |
| publish a port | ABSENT | PRESENT - `-p`, `-P` | **ABSENT by decision** |
| user-defined networks | ABSENT | PRESENT, with driver options | ABSENT |
| connect / disconnect a network | ABSENT | PRESENT | ABSENT |
| network isolation mode | ABSENT | PRESENT - `--network` | PRESENT - `--no-network` |
| VPN / firewall-compatible egress | PARTIAL | PRESENT - Consomme | n/a - the guest uses WSL's own stack |
| **private network namespace with the host refused** | ABSENT | **ABSENT** - no such flag | PRESENT - `base exec --private-net`, `pasta` plus in-namespace `nft` |
| proxy support | PARTIAL | not verified - #40981 (`wslc pull` behind a corporate proxy) and #40945 (PAC/Zscaler) are both open | n/a |

### 3.6 The distribution, the base, the agent - the layer wslc does not have

This is the asymmetry that decides the answer. Twenty of the rows below are
BY-STRUCTURE: the concept is about a managed Linux distribution, and wslc's only
distribution-shaped object is a **session**, which is not one.

| capability | vanilla WSL | wslc 3.0.1 | wsl-toolkit |
| --- | --- | --- | --- |
| own a distribution, import a rootfs | PRESENT - `wsl --import` | **BY-STRUCTURE** | PRESENT |
| build a distro rootfs from an OCI image | ABSENT | **BY-STRUCTURE** | PRESENT |
| reconcile on second run, not rebuild | ABSENT | **BY-STRUCTURE** | PRESENT |
| an unprivileged account with a home and XDG dirs | ABSENT | **BY-STRUCTURE** | PRESENT |
| systemd as PID 1 | PRESENT - `wsl.conf` | **BY-STRUCTURE** | PRESENT |
| a toolset installed as root | ABSENT | **BY-STRUCTURE** | PRESENT |
| a `base` health probe that runs a real thing | ABSENT | PARTIAL - `container run` is its own primitive | PRESENT |
| an identity marker demanded before removal | ABSENT | **BY-STRUCTURE** | PRESENT |
| refuse `wsl --shutdown` anywhere in the binary | ABSENT | n/a | PRESENT |
| **one Windows directory granted, live** | ABSENT | **BY-STRUCTURE** - `-v` is a container volume, no registry, no persistence | PRESENT |
| refuse a grant whose other doors are open | ABSENT | **BY-STRUCTURE** | PRESENT - `catalog.go:612-617`, the two `base.mounts requires …` errors |
| "target is busy" when a process stands in the mount | ABSENT | **BY-STRUCTURE** | PRESENT |
| drive automount off/ro/rw, **read back from the guest** | PRESENT as a setting | **BY-STRUCTURE** | PRESENT |
| Windows interop off, **measured** | PRESENT as a setting | **BY-STRUCTURE** | PRESENT - reads the binfmt handler, not the file |
| passwordless sudo as a verified state | ABSENT | **BY-STRUCTURE** | PRESENT - both arms read from `sudo -n true` |
| close `/mnt/wsl`, the shared tmpfs | ABSENT | **BY-STRUCTURE** | PRESENT |
| a resolver that survives closing that tmpfs | ABSENT | **BY-STRUCTURE** | PRESENT |
| **`base doors`: attack your own containment** | ABSENT | **ABSENT - no analogue at all** | PRESENT |
| `unknown` is not `closed` | ABSENT | ABSENT | PRESENT |
| a private netns for one command, host refused | ABSENT | **ABSENT** | PRESENT |
| install agents into a base | ABSENT | **BY-STRUCTURE** | PRESENT - herdr, muse, pi, omp |
| per-agent model and effort defaults | ABSENT | **BY-STRUCTURE** | PRESENT |
| an SSH door that listens on nothing | ABSENT | **BY-STRUCTURE** | PRESENT |
| a login shell chosen in three arms | ABSENT | ABSENT | PRESENT |
| `base exec` as a framed host-to-guest seam | ABSENT | PARTIAL - `system session run` forwards a command into a session; the default session is persistent, `session enter`'s is not | PRESENT |
| `hostaddress`: read the host address without starting anything | ABSENT | ABSENT | PRESENT |
| a FreeBSD guest on the host hypervisor | ABSENT | ABSENT - a container runtime cannot boot a BSD kernel | PRESENT |
| throwaway distros with replay and compare | PRESENT by hand | **BY-STRUCTURE** | PRESENT |
| a helper protocol for a caller that may not call `wsl.exe` | ABSENT | **BY-STRUCTURE** - wslc's own model *is* a service client the user reaches by design | PRESENT |
| `doctor` past every shim | PARTIAL | PARTIAL - `system info` is wslc's own state, not a host survey | PRESENT |
| the executable carries its own scripts | ABSENT | ABSENT | PRESENT |
| a manual that cannot drift from the binary | ABSENT | ABSENT | PRESENT, with a test that refuses drift |

### 3.7 Governance and platform

| capability | vanilla WSL | wslc 3.0.1 | wsl-toolkit |
| --- | --- | --- | --- |
| Intune enable/disable, registry allow list | n/a | PRESENT | ABSENT |
| MDE plugin covers containers | n/a | PRESENT | ABSENT |
| a Windows app API for containers | ABSENT | PRESENT - NuGet `Microsoft.WSL.Containers`, C# stable, C++/WinRT **in preview** | ABSENT - no FFI surface, stdlib only |
| compose | ABSENT | **ABSENT at GA** | ABSENT |
| a Docker-compatible `DOCKER_HOST` endpoint | ABSENT | **ABSENT** - #40976, open since 2026-08-25 | ABSENT |

---

## 4. Redundancy, settled

### 4.1 What wslc makes redundant in wsl-toolkit: one thing, and it is load-bearing

**The copy-not-mount rule, and the hazards it exists to survive.**

wsl-toolkit forbids a host path reaching a container and copies the workspace
instead (`job.go:444-447`). The copy is not a policy preference; it is what
repairs the exec bit from the git index, bounds a file that grows mid-copy,
defeats `aux.c`-style device names, and keeps a wrong path in a job from
destroying the real checkout. wslc removes the underlying limitation - virtiofs
Windows-path volumes, and a VHD volume with a size ceiling.

So wslc makes the *reason* for the copy obsolete. It does not make the copy
wrong: wsl-toolkit would still copy, and the four mitigations would still hold,
because a copy has no per-I/O cost and a mount has no copy cost. **The honest
framing is that wslc removed a constraint, not a capability.**

Everything else on the redundancy list is smaller:

- **Image pull.** wsl-toolkit pulls in-guest (`job.go:1250`) and needs the base
  booted to pull anything. wslc pulls into the session VHD - read from the deep
  dive's storage section, not from a cited line.
- **The engine-call discipline for the host import.** wsl-toolkit builds a
  two-deadline model for it - a total ceiling and a separate stall deadline,
  because a hang and a slow transfer are different questions
  (`internal/toolkit/enginecall.go:14-19`, `136-159`). wslc's `--stop-timeout`
  is a first-class equivalent, and `wslc` has no stall case of its own that
  wsl-toolkit would inherit.
- **The network stack wsl-toolkit configures** for the base: rootless pasta and
  slirp4netns setup in `provision.sh`. Consomme replaces it for the container
  path.

### 4.2 What wslc does not touch at all

The job control plane, the relay, the grant model, `base doors`, the adapters,
the helper protocol, distros, BSD, `matrix`, `doctor`, `shipped`, the generated
manual, and the entire verified-automount-and-interop layer. Twenty rows of
section 3.6 are structurally out of wslc's reach because they are about a **managed
distribution**, and **wslc's session is not a distribution**.

That last point was worth settling precisely rather than repeating, because the
agents disagreed about it. **`wslc system session` does exist in the 3.0.1
tag.** The `commands/` directory at ref `3.0.1` contains `SessionCommand.cpp`,
`SessionEnterCommand.cpp`, `SessionListCommand.cpp`, `SessionRunCommand.cpp`,
`SessionShellCommand.cpp` and `SessionTerminateCommand.cpp`; `SessionCommand.cpp`
at the tag is byte-identical to master (1415 bytes each). The invocation is
`wslc system session …`, **not** `wslc session …`: `SystemCommand.cpp:29` makes
`SessionCommand` a child of `SystemCommand`, and `RootCommand.cpp:36-42`
registers `SystemCommand` without registering `SessionCommand` at top level. The
independent confirmation is a third party that has it right - wslc-remote calls
`"$WSLC" system session run "$@"` (`wslc:85`).

**But it is not a distribution**, and that is the part that matters:

- it has **no** account provisioning, **no** toolset, **no** `wsl.conf`,
  **no** systemd, **no** adapters, **no** `wsl.conf [automount]` or `[interop]`;
  the argument table has no such argument, and a full sweep of the 3.0.1 tree
  finds no `wsl.conf`, no `useradd`/`adduser` and no `systemd` anywhere;
- the session VM's init is `/bin/sh -c` (`WSLCSession.cpp:667,779`) with no PID-1
  supervisor, and the session's only writable root is the storage device, mounted
  ext4 at `/var/lib/docker` (`WSLCSession.cpp:650`, with tmpfs over it for an
  ephemeral session at `:584`; the constant is `WSLCSessionDefaults.h:26`).
  **A distribution analogue would need everything else.**

**On persistence, the honest answer is that `session enter` and the default
session behave differently, and the CLI is what makes them differ.**

- `session enter` is non-persistent by one argument. The 3.0.1 string table says
  it outright: *"Creates a non-persistent session with the given storage path and
  opens a shell into it. **The session is deleted when the shell exits.** If no
  name is provided, a GUID is generated and printed to stderr"*
  (`localization/strings/en-US/Resources.resw:3081`, at the tag). The code
  agrees: the enter path calls `CreateSession(…, WSLCSessionFlagsNone, …)`
  (`WSLCSessionManager.cpp:466`), and `WSLCSessionFlagsNone = 0` against
  `WSLCSessionFlagsPersistent = 1, // Session remains active after its COM
  reference is released` (`WSLCShared.idl:250-252`).
- **The default session is persistent.** The same manager sets
  `Flags = WSLCSessionFlagsOpenExisting | WSLCSessionFlagsPersistent` for it
  (`WSLCSessionManager.cpp:200`), it appears in `session list`, and the
  existence of a `session terminate` command is itself proof that something
  needs terminating.
- And a session is reachable **without the CLI at all**. `WslcCreateSession` is a
  public API taking settings and returning a session, and the C# projection
  models a session as an object with `Start()` and `Terminate()`. A named,
  explicitly started and explicitly terminated session is one line away in the
  stable projection today.

⛔ **So "non-persistent" is a property of `session enter`, not of wslc sessions
in general, and an earlier draft of this document over-generalised it.** The
conclusion is unchanged, and the reason it survives is the specific list above:
persistence is not what wsl-toolkit's `base` layer is. The thing a session does
not have is **provisioning** - no account, no toolset, no `wsl.conf`, no
adapters, no grant model, no `base doors`, no automount read-back. A session that
grew account provisioning and a toolset would still have none of the twenty BY-STRUCTURE
rows in section 3.6 that are about containment and verification.

**The form of the absorption argument that survives** is therefore the specific
one, not a blanket claim: **if wslc shipped a provisionable session - an
account, a toolset, and something to attach the adapters and the grant model to
 -  the `base` layer would come under serious pressure.** It ships none of those,
and the "not yet" is a product decision visible in the command tree, not only an
absence from a roadmap.

### 4.3 What wslc does that wsl-toolkit cannot do today

| capability | why wsl-toolkit cannot |
| --- | --- |
| real port publishing | a published decision; the stated reason is now **factually obsolete**, see section 6.3 |
| resource limits that bind | `--memory` is accepted and not applied, by the tool's own admission |
| container health checks | absent |
| `image build` | absent from the tree entirely |
| volumes that outlive a container, with a size ceiling | no volume primitive; "persistent means retained, not reused" |
| multi-user / network-attached container addressing | no user-defined networks |
| a Windows-native API for containers | stdlib-only Go, no FFI |
| Intune and MDE governance | out of scope |
| compose | absent from **both** - a shared gap, not an advantage |

**wslc does not, contrary to the announcement's framing of a container
platform, add `--device` or `--platform`.** Both were reported as present by a
first reading of the source; the tag check in section 6 settles it.

---

## 5. Can wsl-toolkit wrap wslc

**Answer: it can call `wslc.exe` today, cheaply, and it cannot make wslc its
engine without a rewrite. And wrapping is the wrong frame, because wsl-toolkit
is a Windows process and wslc is a Windows client.**

### 5.1 The call is easy; the swap is not

Three facts from this tree:

- `FindWsl` refuses outright on any non-Windows host and resolves the binary
  once into a handle whose whole job is "one place that knows how to talk to it
  and one place a denial is recognised" (`internal/toolkit/wsl.go:50-67`).
- The tool **already executes a Windows binary with a guest command and reads
  its streams**: the herdr adapter's SSH `ProxyCommand` is literally
  `wsl.exe -d <distro> -u root --exec …/sshd-stdio`
  (`internal/toolkit/adapters.go:941`).
- The same stdin-framing discipline applies to any payload: the script is
  written to the child's **stdin** and never as an argument, with the
  measurement that produced that rule recorded in the code
  (`internal/toolkit/wsl.go:382-393`: a payload carrying a dollar sign, a
  backtick, both quotes, a tab and a parenthesis arrived intact on stdin and
  propagated exit 7, while the same payload as an argument had its backtick
  **executed** and reported exit 0).

So `wslc.exe run …` from wsl-toolkit is the shape of a call it already makes,
with the same denial handling and the same framing.

**The swap is a different matter, and the code is blunt about it.**
`internal/toolkit/job.go:994-1003` builds the container invocation as a generated
POSIX shell script:

```go
var b strings.Builder
b.WriteString("set -u\n")
b.WriteString(deviceCheckScript(spec.Devices))
b.WriteString("exec podman")
for _, a := range args {
    b.WriteString(" ")
    b.WriteString(shellQuote(a))
}
```

There is **no interface, no registry, no dispatch**. `podman` is a literal in a
`strings.Builder` that is piped to `/bin/sh` inside a distribution, and it
reappears in the same shape across `internal/toolkit` - 68 raw occurrences in
the six files that hold the call sites, of which 13 are in `_test.go`, and 6
more in the embedded `verify.sh`. Measured on this tree, not estimated:
`job.go` 16, `jobctl.go` 8, `observe.go` 17, `inspect.go` 18, `resources.go` 4,
`cleanup_run.go` 5. **`doors.sh` and `grants.sh` contain no occurrence of the
string at all**, which matters: those two are attack and grant surfaces, and
they are engine-independent. `provision.sh` installs the engine.

**The one thing that looks like a seam is not one.** `engine.go:21-25` defines
`type Engine struct{ Name, Path, Arch }` - and its own doc comment says it is a
**host** engine whose one job is producing a rootfs tar for `wsl --import`. The
only per-engine variation is the architecture field spelling for podman versus
docker. Swapping *that* type changes image-to-rootfs provisioning, not job
execution.

**Where a real seam does exist, and it is one layer above the podman call:**
`internal/toolkit/observe.go` defines an `Observer` with two implementations,
`DistroObserver` and `ContainerObserver`, and `ContainerObserver` is constructed
with an injected `Ask`. The relay consumes `Observer` and names no command. So
the *relay* is engine-agnostic while the *adapter* filling it is podman-shaped. A
wslc-backed `Observer` would slot in; a wslc-backed job path would not.

### 5.2 What would gain, what would be bypassed

**Gain:** `--stop-timeout` including `-1`; resource limits that bind, retiring
`WSL-60`; health checks; `image build`; `--cpus`; a real per-container disk with
a ceiling; `container exec` into a running job; a Windows-native API if ever
wanted.

**Bypassed - i.e. the base stops being the container host:** the provisioned
rootless-podman base, the guest runtime prologue, the binfmt registration, the
whole `doors.sh` attack surface, the grant model, `base exec --private-net`, the
adapters, the helper protocol, distros, BSD. Roughly the majority of
`internal/toolkit` by file, and all of section 3.6.

**Two frictions that are not flag-renames:**

1. **The container-id lifecycle differs.** wsl-toolkit keys jobs by its own id and
   stamps containers with a label of its own, so a stop cannot touch a foreign
   container. wslc returns its own ids in a separate namespace. The mapping has
   to be held, and the label that makes `stop` safe has no wslc equivalent.
2. **`observe.go` parses `podman inspect --type container` into wsl-toolkit's own
   structs.** wslc's inspect output is a different JSON shape. That is a
   rewrite, not a flag rename.

**One correction to a claim in an earlier draft of this section.** It said the
wsl-toolkit label "has no wslc equivalent". wslc has `--label`
(`ArgumentDefinitions.h`, `Kind::Value`, key/value pair), so the flag exists.
**What is not verified is the part that matters**: whether a wslc `stop` filters
by label at all. That behaviour would be service-side, in `ContainerTasks.cpp`,
which is outside the pulled CLI tree. Until it is read, "a stop cannot touch a
foreign container" is a property wsl-toolkit holds and wslc is not known to
hold.

### 5.3 The honest counter-argument

The parts of wsl-toolkit that matter most are the parts wslc cannot do.
Adopting wslc as the job engine would leave the tool with two disconnected
halves: a container runtime it no longer owns, and a distro/agent/door layer
with nothing to attach it to. **The win is that containers gain capabilities
wsl-toolkit does not have - resource limits that bind, health checks, `image
build` - and the loss is the thing that makes a base trustworthy.** Note that
section 1 declines to put a number on wslc's file performance, so "faster" is not part
of this argument: the case for wslc rests on capability, not on speed.

---

## 6. Verification notes, and the corrections they produced

Everything in this section was checked by hand against an artefact after the
first reading produced something wrong or unsupported. They are recorded because
the corrections are the point: a first reading of a 157-file source tree is not
evidence.

### 6.1 `--device` and `--platform` are NOT in wslc 3.0.1

A first reading reported `--device` present in wslc's argument table and cited
`ArgumentDefinitions.h:87`. Line 87 is `--input`. **No argument named `device`
or `platform` is registered**, and the absence was then checked against the whole
3.0.1 tree rather than the CLI table alone: the strings occur in exactly three
places under `src/`, all of them comments - a `// TODO: List specific GPU
devices.` and a `// TODO: Platforms: a json array of OCI platform strings.` in
`wslc.idl`, and one about `wsldevicehost` in a comment. The word `platform` does
appear as inspect/list output metadata (`ContainerTasks.cpp:199-200`), which is
why the claim is scoped to registered arguments.

Settled against the shipped tag rather than master: `ArgumentDefinitions.h` at
the 3.0.1 tag is byte-identical to master, `diff` of the two identifier lists is
empty, and 115 definitions resolve to 110 active with 5 commented out. The
SDK header is also clean: `WSLCCreateContainerOptions` carries volumes, ports,
labels, DNS and tmpfs and **no device or platform field**, and a case-insensitive
search for `privileged` across the CLI files, `wslc.idl` and `WSLCShared.idl`
returns nothing. There is no `--privileged`, no `--cap-add`/`--cap-drop`, no
`--security-opt`, no `--read-only`.

**Three tracker entries make the direction of travel legible, and they are the
evidence a reader deciding adoption actually needs:**

| item | state | what it says |
| --- | --- | --- |
| **#40988** | open, 2026-07-03 | "wslc preview: Missing `--device` flag and `/dev/kfd` CDI mapping breaks AMD ROCm GPU passthrough" - first-party, names the missing flag, predates GA |
| **#41161** | **closed unmerged** | `feat(wslc): add privileged container support (--privileged flag)` - written and **rejected** |
| **#41597** | **closed unmerged** | `wslc: add USB device passthrough (--usb, --privileged)` - also rejected, and it states *why* device work is blocked: `wslc run -v /dev/ttyUSB0:/dev/ttyUSB0` "just creates an empty directory in the container, since the node doesn't exist in the container VM for it to bind to" |

**And one shipped artefact that contradicts the released capability:**
`intune/WSL.admx:190` at the tag defines an Intune policy named
`AllowWSLContainerPrivileged`, beside `AllowWSLContainer` (`:179`) and
`WSLContainerRegistryAllowlist` (`:201`). A privileged-container policy ships
while no privileged mode does. That is either a Microsoft bug or an imminent
feature, and an adoption plan should assume it resolves upward.

**Consequence: wsl-toolkit's `--device /dev/kvm` has no wslc equivalent to wrap,
and wslc's platform story has no upstream to defer to.** Note also that #41597
independently corroborates section 4.1 from the other direction: a bind of a device
node into a wslc container does not work, which is the same gap wsl-toolkit's
copy-not-mount rule exists to close.

### 6.2 `wslc system session` IS in 3.0.1, and it is more capable than first read

The opposite correction, in the other direction. `SessionCommand.cpp` exists at
the tag and is byte-identical to master, **and the default session is
persistent**. Two errors are corrected here: the invocation path, which is
`wslc system session` and not `wslc session`, and the over-general "the session
is deleted when the shell exits", which is true of `session enter` and false of
the default session. Both agents reviewing an earlier draft missed the first and
one caught the second. The full reading, with the flag that makes the difference,
is in section 4.2.

### 6.3 wsl-toolkit's port-forwarding row now rests on a contestable reason

`wsl-toolkit.md:1106` says, under Known limits, that there is no port
forwarding because "forwarding a port on Windows needs an elevated session and
leaves a rule behind". Consomme does port mapping from a Windows process running
as the user, with no elevation.

**The row is still true of wsl-toolkit, and the decision may well stand - the
tool may prefer not to publish a port at all. But an earlier draft of this
document claimed the stated reason was "no longer a fact about Windows", and
that is too strong.** wslc publishes from a Windows process, so the tooling
still has to leave something behind for anything to reach the port; what changes
is that it no longer needs an *elevated* session to do it. The accurate claim is
narrower: **the justification is contestable, not falsified.**

Two facts that keep the decision defensible, and that a reader should have:

- wslc's port publishing is **not a strict superset**. #40999 is open - UDP
  publishing rewrites the reply source port and breaks connected UDP sockets,
  which is QUIC and games. A web server port works; a QUIC service does not.
- wslc has a real setting for the binding address,
  `session.defaultBindingAddress` (`WSLCUserSettings.h:106`), so "which
  interfaces does this listen on" is a separate question from "does it publish",
  and the tool would have to answer it.

This is a documentation finding, not a code one, and it is small. It is filed
here rather than fixed, because a live page in this tree is edited in the
session that owns it, with its changelog entry, not from a `.tmp` analysis.

### 6.4 Two wslc defects wsl-toolkit would inherit

Read from the tracker, both open at the time of writing:

- **#41691** - a published port's wildcard listener accepts TCP and relays zero
  bytes. That is the port-publishing path, broken, in the first release.
- **#41722** - "Failed to configure network (networkingMode Consomme), falling
  back to networkingMode None." On a host that falls back, container networking
  is off.

Neither was independently re-derived from source; both are tracker reports and
are marked as such here.

### 6.5 A wsl-toolkit question raised but not answered

`cmd_base.go` registers `shell` as reading `via-helper`
(`cmd_base.go:296-303`), and the helper branch at `cmd_base.go:111-130` handles
only `status`, `ensure` and `recreate`, refusing everything else with "Removing a
distribution and attaching a terminal are not job data". So `base shell
--via-helper` **reads the flag and then refuses the route**. Whether that
satisfies or violates the rule `WSL-104` states - a command must never publish a
flag it does not read - was not settled here, and is unrelated to wslc. Recorded
as a finding, not fixed, not in scope.

---

## 7. wslc-remote

### 7.1 What it is

A three-file shell project whose own header is the most honest sentence in it:
"VS Code Remote-WSL-style fast file sharing for wslc containers" (`wslc:3`).

`wslc.exe` is a Windows-side client, so a command typed in a distro has to cross
the Windows boundary; a `-v /host/dir:/ctr` bind crosses a second boundary into a
separate runtime VM over virtiofs. wslc-remote's answer: keep the data in the
distro's own ext4 and reach it from the VM over **loopback NFS** instead.

Per host bind, the mechanism is (`wslc:168-231`):

1. start an `unfsd` NFSv3 server in the distro, exporting the directory **on
   loopback only**, with `no_root_squash` on read-write shares because a
   container's volume-init chown must succeed (`wslc:120-128`);
2. `wslc volume create --driver guest` and NFS-mount that server onto the
   volume's `_data` backing store **inside the runtime VM** (`wslc:213-220`);
3. rewrite `-v /host/dir:/ctr` into `-v <volume>:/ctr` and `exec` the real
   `wslc` (`wslc:286-293`, `318-338`).

It adds exactly two commands, `wslc _check` and `wslc _help`, both
underscore-prefixed so they cannot collide with a current or future subcommand.

### 7.2 What is genuinely good about it

Read the code, not the README:

- **`_check` tests the capability, not the configuration.** It does not read
  `.wslconfig` or trust a mode name; it starts a throwaway `unfsd`, exports a
  scratch dir read-only, and has **the runtime VM itself** open a TCP connection
  to `127.0.0.1:<port>` (`wslc:411-435`). It checks unfsd, the CLI, the session
  and the VM's NFS client separately first, so a failure names which link broke.
- **Read-only is preserved twice over.** `ro` in the exports file *and* `:ro`
  preserved in the rewritten spec, plus the read-only flag folded into the
  volume-name hash so a read-write and a read-only share of the same directory
  get separate volumes and separate servers and can never reuse each other's
  state (`wslc:91-95`, `:123`, `:292`). This is the kind of detail that usually
  gets missed.
- **The port-collision race is closed.** `share()` waits five seconds and aborts
  the **instant its own** unfsd dies (`wslc:196-209`), because otherwise a
  *different* process already on that port would satisfy the liveness check and
  the volume would mount against the wrong export. A real bug class, closed.
- **A self-reference guard** refuses to run if the resolved client is the shim
  itself (`wslc:57-69`).
- **`unfsd` installation is careful**: distro package first, a documented
  refusal where only AUR exists, a source-build fallback, PATH symlinking for
  the `/usr/sbin` case.

### 7.3 What is wrong with it, and what the README does not say

The README states almost no limitations. These are the real ones.

**Maturity is the headline.** Zero issues, zero pull requests, zero comments,
ever - verified three ways (`state=all` returned length 0; `search/issues`
`total_count` 0; the API rate limit was untouched, so it was not throttling).
Eight stars, one author, six commits, and **all three substantial ones landed in
a 90-minute window on 2026-09-29 - the day wslc went GA**, with the last push at
15:43:54Z, *before* the 17:18:57Z GA timestamp. The code was written against a
pre-GA build and has never been touched since. There is no test, no CI, no tag,
no release, no checksum.

**A shell injection through a bind source path, reproduced.** `wslc:216`
interpolates the directory **unquoted** into a string handed to `sh -c` inside
the runtime VM:

```
grep -q \" \$T nfs \" /proc/mounts || mount -i -t nfs -o $(nfs_opts "$port") :$dir \"\$T\"
```

Run on this host's Git Bash, in a scratch directory under `.tmp`, with
`dir="/tmp/my proj; touch /tmp/PWNED; x"`, that command reported
`sh: line 4: x: command not found` and `/tmp/PWNED` was created. `;`, `$(…)`,
backticks and newline all inject. It requires a path you already control, so it
is self-inflicted rather than a privilege boundary - and the reproduction ran as
this host's user, not as root inside a runtime VM, which bounds what it
demonstrates. Still: a `git clone` into a directory with a `;` in its name
silently executes the rest of the name as root-side commands in the runtime VM,
and the offending path is produced by `wslc:172`
(`dir="$(cd "$1" && pwd)"`), which preserves it intact. It is a plain quoting
bug, not a race.

**A path with a space breaks the state file.** `wslc:222-228` writes `DIR=$dir`
with no quoting into a file that `wslc:180` and `:238` **`source`**. Sourcing
such a file with a space in the path leaves `$DIR` unbound, and the script's
`set -euo pipefail` (`wslc:33`) turns that into exit 127. Reproduced, with the
quoted control running clean. Spaces in directory names are not exotic, so this
one fires on ordinary input while the injection needs an unusual name.

⛔ **Ranking, corrected.** An earlier draft called the injection "the most
serious defect in the project". That is not defensible, and the two defects below
are worse for the operator who is actually running the thing: they need no
attacker and no unusual filename. The injection is a real bug and belongs in
this list; it does not belong at the top.

**No cleanup on failure, and no `trap`.** `stop_share` is reachable from exactly
two places - stale-share replacement and an explicit `volume rm` - and **nothing
calls it on container teardown**. If the exec'd `wslc` exits nonzero, every
share created for that invocation stays: server running, guest volume created,
export live.

**`--rm` is misleading for this tool, and this is the worst of the three.** The
README's own examples use `--rm` (three of them, at `README.md:60,63,66`). It
removes the container. The share, the volume and the `unfsd` all survive - so
what is left behind is **a standing unauthenticated export of the directory,
with `no_root_squash`, live after the workload that wanted it has finished**.
`grep -n "trap " wslc` returns nothing, so there is no exit-time cleanup on any
path. The documented happy path leaks.

**Claims not corroborated by the repository:**

| claim | status |
| --- | --- |
| "~13x faster for builds" | **uncorroborated, and worse than stated** - the figure appears only in a source comment at `wslc:26`, not in the README at all, and the `results/` directory it cites exists neither in the clone nor in any commit of the history |
| "wslc has no `--mount` flag" (`wslc:296`) | **contradicted** - `--mount` is registered at `ArgumentDefinitions.h:99` at the tag and on master, and taken by `ContainerRun.cpp:56` and `ContainerCreate.cpp` |
| "read-only binds stay read-only" | **corroborated twice over**, see section 7.2 |
| "both `mirrored` and `virtioproxy` are supported" | not verifiable from this repo; it is a WSL behaviour claim. The *design* it implies - test the capability - is corroborated in code |
| the distro support matrix | **corroborated, weakly** - every README row is satisfied by *some* installer branch, so the check cannot fail. It is a weak test presented as a pass, and should not be read as "every row is individually exercised" |

### 7.4 What wslc-remote would mean for wsl-toolkit

**Verdict: refused, and the reasoning is recorded so a future session does not
re-derive it.**

1. **It solves a problem wsl-toolkit does not have.** Its purpose is
   accelerating a host bind mount. wsl-toolkit's job mounts are guest paths
   inside the distribution, and the workspace arrives by **copy** through tar.
   The cost it optimises is not a cost wsl-toolkit pays.
2. **The direction is wrong.** The shim exists because `wslc.exe` is a
   Windows-side client. wsl-toolkit is a Windows process. Using the shim would
   mean Windows process -> `wsl.exe` -> distro -> bash shim -> `wslc.exe` -> back:
   two process hops, a bash dependency and a state directory, to make a call
   that is already local.
3. **On a sealed base it cannot work.** The shim's fallback client path is
   `/mnt/c/Program Files/WSL/wslc.exe` (`wslc:56`), and a base with
   `automount: off` has no `/mnt/c` - that is the setting's meaning, and it is
   verified by reading the guest back. A base with `interop: off` cannot execute
   a PE image at all. The obvious escape is an explicit `WSLC=` pointing inside a
   **granted** directory, and the grant model closes it: `base.mounts` requires
   `interop` off (`catalog.go:616-617`, quoted verbatim and verified), because
   interop is precisely what lets a PE image run. A granted directory is a DrvFS
   mount inside the guest, but `exec` of a PE image is governed by interop, not
   by where the file happens to sit - so `WSLC=/workspaces/proj/wslc.exe` is
   refused by the same rule, and symlinking it back to `/mnt/c` only moves the
   failure. **The shim is unusable on a sealed base, and the sealed base's own
   validation is what makes it so.**
4. **It is incompatible with `--private-net`.** The transport is TCP to
   `127.0.0.1` across WSL localhost forwarding; `private-net.sh` drops the
   private ranges on the output path. And the namespace makes the payload uid 0,
   which is exactly why **no container runs inside it** - a wslc container is a
   container, so the existing measured failure for podman transfers directly.
5. **Installing it in a base would make the tool's central promise
   unenforceable.** A share exports whatever the account can already reach, over
   a path that never passes through `base grant`, never appears in `fstab`, and
   is never read back by `grants.sh` or `verify.sh`. The containers it starts
   land in a store at `/var/lib/docker` that is in no wsl-toolkit ledger, no
   `result.json`, and no JSONL record - and `base doors` would not see it.

**The honest counterweight, because it is the strongest argument against this
verdict:** on a base with `passwordless_sudo: true` the account can already
mount a Windows path by itself, and the tool says so in its own code. So the
shim does not open a hole that was closed. **What it changes is the shape of the
reach** - it turns a `mount` line an agent must know to write into a transparent
shim that does the work, and converts the reach into a *container's* reach,
which is the one thing the job path promises cannot happen. The correct claim is
not "this opens a hole". It is "this makes the promise unenforceable by the
checks this tool actually has".

**Cost if it were ever wanted:** a new adapter with a real dependency problem  - 
on the only preset every existing adapter has been driven on, `arch`, `unfsd` is
AUR-only, so the adapter would have to build UNFS3 from source and would be the
only adapter in the tree that modifies third-party source. And the probe would
have to report the loopback reachability `unknown` on any base with interop off,
because testing it means running a Windows binary. That is an adapter whose
install is refused on a sealed base and whose probe is permanently `unknown`
there. **The cheaper route to the same capability is a wslc engine seam beside
the existing `Engine`, driven from Windows** - which is what section 5.1 concludes, and
which is not a shim.

---

## 8. Recommendations, in the order a session should take them

1. **Adopt nothing now.** wslc is one day old. Its own tracker carries open
   defects in the exact paths an adoption would lean on - #41691 and #41722 both
   predate 3.0.1 - it has no `--platform` and no `--device`, and the version
   floor is WSL 3.0.1 while this host runs 2.7.12. Record the refusal with section 4.2
   and section 7.4 as the reason.
2. **Annotate the port-forwarding row** in
   `tools/windows/wsl-toolkit/wsl-toolkit.md:1106`. The limit stands; its
   justification is now contestable rather than settled, and UDP publishing is
   not a clean substitute (#40999). One line, in the session that owns the page,
   with its changelog entry - **not** from a `.tmp` analysis.
3. **If and when wslc is reconsidered, the first work is a seam, not a
   feature.** The honest first step is a `JobEngine` interface over the
   generated-script call site at `job.go:994-1003`, with the podman path kept as
   one implementation, plus a `WSLEngine` behind it - and the `Observer` seam at
   `observe.go`, which is already engine-agnostic. Prove the podman path is
   unchanged before writing a line of wslc. ⛔ **A note against my own
   recommendation**: an interface with one implementation is the
   speculative-abstraction case, and this repository's own `research.md` says
   to adopt mechanisms rather than architectures. The mitigation is the guard in
   the sentence above, and the mitigation is load-bearing.
4. **Do not build an adapter for wslc-remote.** section 7.4 is the record.
5. **Two things are worth watching, and neither is work today:** whether a wslc
   session ever becomes **provisionable** - an account, a toolset, something to
   attach the adapters and the grant model to, which is the part section 4.2 shows is
   missing and the part that would put the `base` layer under real pressure - and
   whether `wslc compose` ships. A session that merely became persistent would
   change nothing: the default session is already persistent (section 4.2).

---

## 9. What this document did not do, and what would settle each gap

| not verified | what would settle it |
| --- | --- |
| any wslc behaviour at runtime | a WSL 3.0.1 host: `wslc --help`, `wslc system session --help`, `wslc container run --help`. ⛔ **Note that the one wslc CLI reference on Microsoft Learn is a 404**, so `--help` on a real host is the only route |
| the "2x" cross-OS file figure | a published benchmark. The deep dive's version is a *different* claim - "Compared to plan9, virtiofs is about twice as fast" - so the baseline is plan9, not unstated performance. Nothing in the repository, the release notes or Learn carries a number |
| Consomme in practice | issue #41722 suggests it falls back to `none` on some hosts, and the trigger is unknown |
| wslc-remote's NFS path on a `nat` host | a live run. This host has WSL 2.7.12 and no `wslc.exe`, so no measurement exists |
| whether a wslc session mounts Windows drives at all | `mount` inside `wslc system session shell` |
| whether a wslc `stop` filters by label | `ContainerTasks.cpp`, which is outside the pulled CLI tree. This is the one claim in section 5.2 that would decide whether wsl-toolkit's "a stop cannot touch a foreign container" survives a swap |
| the meaning of the shipped `AllowWSLContainerPrivileged` policy | an Intune surface, or a Microsoft bug. section 6.1 records the policy at `intune/WSL.admx:190` with no privileged mode behind it |
| the MDE container telemetry claim | the linked MDE plugin page, which was found but not read |
| wslc's `--secret` on `image build` | present in the argument table, with a tracker issue saying it may not function |
| whether the 62 wslc files on master are byte-identical to the tag outside those compared | nine command files, four leaf files, `ArgumentDefinitions.h` and `WSLCSessionDefaults.h` were each compared; the remaining leaf command bodies were read from master only |
| the wslc-remote commit history diff-by-diff | a bisect of "Improved startup" and "Improved install" would likely show which design decisions are corrections to observed failures. That is the highest-value unmined material in that repository |
| tracker issues #41691, #41722, #40999, #40961, #40976, #41545, #41666, #40979, #40981, #40945 | read from the tracker, not re-derived from source. #40981 (`wslc pull` behind a corporate proxy) and #40945 (PAC/Zscaler) are both open and both wslc-specific, which is a little better supported than section 3.5's "not verified" admits |

**Nothing in this document is a measurement of wslc.** It is a reading of
artefacts, and on a host with WSL 3.0.1 most of section 9 becomes an afternoon's work.

---

## 10. Measured on WSL 3.0.3, on 2026-10-21

The host now runs WSL **3.0.3.0**, and `wslc version` answers **`wslc 3.0.3.0`**
from `C:\Program Files\WSL\wslc.exe`, on Windows 10.0.26300. Every row below was
run on it, from a shell at Medium integrity, which is not elevated. The work this
section closed is `WSL-110` in [`../../TODO/wsl-toolkit-go.md`](../../TODO/wsl-toolkit-go.md).

### 10.1 What section 9 left open, answered

| section 9 row | measured |
| --- | --- |
| any wslc behaviour at runtime | ⭐ `wslc run --help` registers `--memory`, `--cpus`, `--publish`, `--mount` and `--gpus`, and no `--platform`, `--device`, `--privileged`, `--cap-add`, `--security-opt` or `--read-only`. **Section 6.1 holds at 3.0.3.** `wslc pull --help` and `wslc create --help` name no `--platform` either |
| whether a wslc session mounts Windows drives at all | ⭐ **No drive is mounted.** `wslc system session run cat /proc/mounts` in the default session shows the root as an overlay, `/var/lib/docker` as the storage disk, ext4 with `discard`, and the WSL libraries over virtiofs, with one virtiofs mount at `/run/wsl/virtiofs-mounts/drvfs`. Nothing under `/mnt` is a Windows drive |
| the "2x" cross-OS file figure | not measured |
| Consomme in practice | not measured |
| wslc-remote's NFS path | not measured, and section 7.4's verdict does not depend on it |
| whether a wslc `stop` filters by label | not measured |

⭐ **A wslc session is its own VM, with its own kernel.** `uname -r` in the session
answers `6.18.54.1-microsoft-standard-WSL2`, the stock WSL kernel, while this host's
WSL distributions run a 7.2.0 kernel that `.wslconfig` names. So the `binfmt_misc`
table a session sees is not the one WSL distributions share, which is the table
`WSL-105` is about.

### 10.2 The export a base needs, driven

`wsl-toolkit` needs one thing from a host engine: an image turned into a root
filesystem archive that `wsl --import` reads. With `docker.io/library/alpine:latest`:

| step | measured |
| --- | --- |
| `wslc pull REF` | exit 0 in 14 s |
| `wslc image inspect --format json REF` | one JSON object carrying `"Architecture": "amd64"` and `"Os": "linux"`, which is how an engine with no `--platform` is checked |
| `wslc create REF` | exit 0, and the container id alone on stdout |
| `wslc export ID` to stdout, and `wslc export -o FILE ID` | both exit 0; 8,705,536 bytes and 520 entries each |
| `wsl --import` of that archive | registers, and `/etc/os-release` answers `Alpine Linux` on `x86_64` |
| `wslc remove -f ID` | exit 0, and `wslc list --all` is empty |

⭐ **So `wslc` is now this tool's third host engine**, tried after podman and docker,
under ruling 30. A scratch base built with the Podman machine stopped answered `host
engine: wslc, platform linux/amd64`, exported a 544.1 MiB root filesystem, and was
usable in 119 seconds.

### 10.3 Section 6.3, corrected: publishing a port needs no elevation

`wslc run -d -p 18080:8080` with a `nc` responder inside: `127.0.0.1:18080` was
listening, owned by `dllhost.exe`, a COM surrogate running as the user, and a request
answered `200` with the responder's text. `wslc container inspect` named `HostIp`
`127.0.0.1`.

⛔ **So for wslc's route the reason this tool's limit gives is false**: no elevated
session was needed. What stays true is that something has to keep running for the
port to answer: the listener belonged to a process outside the container. The manual's
limit now says both.

⚠ **A false start, recorded so it is not repeated**: the first attempt served with
`httpd`, which Alpine's busybox does not carry. The container exited 127, nothing
listened, and the refusal measured the responder, not the publishing.

⚠ **#41691 was not reproduced, and this run could not have**: the port bound
loopback, and the tracker describes a wildcard listener.

### 10.4 What the recommendations in section 8 became

| recommendation | now |
| --- | --- |
| 1, adopt nothing now | ⭐ **superseded for one use by ruling 30**: wslc is a host engine for the root filesystem export. It is still not an engine jobs run in, for section 5's reasons |
| 2, annotate the port-forwarding row | done, from section 10.3's measurement rather than a reading |
| 3, a seam before a feature | still the order, and still not started: nothing here runs a job in wslc |
| 4, no adapter for wslc-remote | stands |
| 5, two things to watch | stands |
