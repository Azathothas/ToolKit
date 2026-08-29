# usable.md

⭐ **The file a later session acts on.** Commands that were run, outputs that
were seen, and the lessons, so the next session does not re-derive any of it.

[`findings.md`](findings.md) carries the verdicts and the argument. This one
carries the evidence and the recipes.

⚠ Everything here was measured on **one Windows 11 Pro 26200 machine on
2026-08-27**, podman 5.8.6, PowerShell 7.6.5, with a WSL2 podman machine
running Fedora. A measurement carries its conditions or it is not a
measurement.

---

## The one command that settles the design

```bash
podman run --rm ghcr.io/freebsd/freebsd-runtime:15.1 /bin/sh -c 'uname -a'
```

```text
WARNING: image platform (freebsd/amd64) does not match the expected platform (linux/amd64)
```

Exit code **139**. No stdout. 139 is 128 + 11, a **SIGSEGV**.

⛔ **Read that number carefully before designing anything.** It is not
`Exec format error`. The Linux ELF loader accepted a FreeBSD binary and the
binary died on its first syscall. `binfmt_misc` and `qemu-user` are both
irrelevant: they solve a foreign **architecture** presenting **Linux** syscalls,
and this is a native architecture presenting **FreeBSD** syscalls. Linux has no
reverse Linuxulator.

⭐ **A FreeBSD userland requires a FreeBSD kernel.** Everything else in this
file follows from that one line.

---

## Pulling a FreeBSD image at all

The plain pull is refused, and the refusal is correct:

```bash
podman pull ghcr.io/freebsd/freebsd-runtime:15.1
```

```text
Error: ... no image found in image index for architecture "amd64", variant "", OS "linux"
```

⚠ **`--os` is required, and it is separate from `--platform`.** Reaching for
`--platform linux/amd64` out of habit asks for an image that does not exist.

```bash
podman pull --os freebsd --arch amd64 ghcr.io/freebsd/freebsd-runtime:15.1
```

34 MB, and `podman image inspect --format '{{.Os}}/{{.Architecture}}'` reports
`freebsd/amd64`.

⚠ **That pull retags the shared local tag**, which is the trap from
`Azathothas/TEMPLATE` issue 2. Remove the image afterwards, or a later
unqualified pull of the same name is a no-op that serves the FreeBSD copy.

---

## What already exists, so it is not built again

⛔ **Do not build FreeBSD OCI images.** The FreeBSD project publishes them.

```bash
curl -fsSL "https://ghcr.io/token?scope=repository:freebsd/freebsd-runtime:pull&service=ghcr.io" | jq -r .token
```

Use that bearer token against `https://ghcr.io/v2/freebsd/freebsd-runtime/tags/list`.
Tags present on 2026-08-27:

```text
14.4.rc1  14.4  14.5.beta1  14.5.beta2  14.5.beta3
15.1.beta1  15.1.beta2  15.1.beta3  15.1.rc1  15.1.rc2  15.1.rc3  15.1
```

The manifest is an index with `{"architecture":"arm64","os":"freebsd"}` and
`{"architecture":"amd64","os":"freebsd"}`.

The same releases are at `https://download.freebsd.org/releases/OCI-IMAGES/`,
covering 14.3 through 15.1. ⚠ `amd64`, `aarch64` and `riscv64` were confirmed
present for the 14.3, 14.4, 15.0 and 15.1 **RELEASE** builds specifically; the
beta and RC directories were listed but not opened.
⭐ Prefer the download server when a chain of trust matters; the handbook says
so and it is the project's own advice.

---

## The runtime, on a FreeBSD host

From the handbook. ⚠ Not run by this session, because no FreeBSD host existed:

```bash
pkg install -r FreeBSD -y podman-suite
```

```bash
podman load -i=FreeBSD-15.1-RELEASE-amd64-container-image-static.txz
```

The runtime underneath is jails, through `ocijail`. `runj` is the other
implementation and is described by its own author as a proof of concept.

---

## ⭐ The client mechanism that removes the need for any wrapper

This is the most reusable thing the sweep produced, and it is visible on any
machine with podman:

```bash
podman system connection list
```

```text
podman-machine-default  ssh://user@127.0.0.1:53512/run/user/1000/podman/podman.sock
```

⭐ **A podman connection is an ordinary SSH URI to a podman socket.** Nothing
about it is special to `podman machine`. So a podman running anywhere reachable
over SSH, including a FreeBSD guest, is addressable from the Windows client:

```bash
podman system connection add freebsd ssh://user@HOST/var/run/podman/podman.sock
```

```bash
podman -c freebsd run --rm -it ghcr.io/freebsd/freebsd-runtime:15.1 /bin/sh
```

⛔ **This is why no `podman` wrapper script should be written.** The original
plan considered shadowing `podman` with a script that manages machines. `-c`
and `podman system connection default` already do it, and a wrapper that shadows
a real binary to reimplement one of its own flags is the kind of rebuilt
machinery
[`../conventions/forbidden-patterns.md`](../conventions/forbidden-patterns.md)
has a row for.

---

## Lessons

| tag | lesson |
| --- | --- |
| `adopt` | ⭐ Read the exit code, not the error text. 139 versus `Exec format error` is the difference between "needs binfmt" and "architecturally impossible". The text looked adjacent to a problem already solved; the number said otherwise. |
| `adopt` | A podman connection is just SSH. Check `podman system connection list` before writing anything that manages podman. |
| `adopt` | ⭐ Read the tracker. The maintainer's two refusals and the stalled pull request are the whole cost picture, and none of it is in any README. `references.md` calls this the step that gets skipped, and it was the highest-value hour of the sweep. |
| `avoid` | Do not build FreeBSD OCI images. The FreeBSD project publishes them at the same registry the plan intended to push to. |
| `avoid` | Do not plan around `containers/podman#19939`. Open, unmerged, and refused twice by the maintainer. |
| `avoid` | Do not follow the `podman machine init --image` suggestion literally. It requires Ignition, which FreeBSD does not have, so it starts with porting a CoreOS provisioning system. |
| `honest-limit` | ⛔ There is no route to a FreeBSD userland from Windows that avoids a hypervisor. The achievable minimum is one, Hyper-V, not nested. Anyone promising otherwise has not run the command at the top of this file. |
| `honest-limit` | "The most popular BSDs" is one BSD. NetBSD and OpenBSD publish no OCI images and have no jail-equivalent OCI runtime. |
| `future` | The FreeBSD-native model that OCI does not express: a read-only base over `nullfs` with a read-write overlay, worth about 500 MB against a full base. Revisit if image size becomes the constraint. |

---

## What this file does not know

⛔ Stated rather than left to be discovered:

- ⭐ **Hyper-V and WSL2 coexisting is MEASURED, and it was the one open
  assumption.** On 2026-08-27, with the WSL2 podman machine running:
  `HypervisorPresent` is `True`, `Get-Service vmms` reports **Running** with
  startup `Auto`, and the `Hyper-V` PowerShell module is present at v2.0.0.0.
  ⚠ `Get-WindowsOptionalFeature` needs elevation, so the definitive feature
  list was not read; a running `vmms` is the stronger evidence anyway.
  [`../../TODO/bsd.md`](../../TODO/bsd.md) carries the probes.
- **No FreeBSD host was available**, so nothing in "The runtime, on a FreeBSD
  host" was executed. It is quoted from the handbook.
- **`runj` and `ocijail` were checked for liveness, not read.** No claim here
  depends on their internals.
- **`github.com/orgs/freebsd/packages` was not reachable** with this token's
  scopes. The registry was queried anonymously instead.

---

# The BSD reference batch, 2026-08-27

⭐ **The commands and recipes from the `R6` to `R28` sweep.**
[`findings.md`](findings.md) carries the verdicts, the ranking and the argument.
This half is what a later session runs.

⚠ **Same machine as everything above**: Windows 11 Pro 26200, podman 5.8.6,
PowerShell 7.6.5, a WSL2 Fedora podman machine, Git Bash. New this session:
the host CPU is a **12th Gen Intel Core i7-12700H**, reported by the registry as
`Intel64 Family 6 Model 154 Stepping 3`. That number matters below.

---

## ⭐ Measured here, and it closes an open caveat

[`../../TODO/bsd.md`](../../TODO/bsd.md) recorded that
`Get-WindowsOptionalFeature` needs elevation, so the definitive feature list was
never read, and that `Microsoft-Hyper-V-All` did not appear in the unelevated
registry view. ⭐ **There is an unelevated runtime check, taken from `R18`, and
it answers the question outright.**

```powershell
$sig = @"
using System;
using System.Runtime.InteropServices;
public static class Whp {
  [DllImport("WinHvPlatform.dll")]
  public static extern int WHvGetCapability(uint code, out int buf, uint bufSize, out uint written);
}
"@
Add-Type -TypeDefinition $sig
$val = 0; $written = 0
$hr = [Whp]::WHvGetCapability(0, [ref]$val, 4, [ref]$written)
"hr=0x{0:X8} value={1} written={2}" -f $hr, $val, $written
```

```text
hr=0x00000000 value=1 written=4
```

⭐ **Two facts in one call, and neither needed elevation.**
`WinHvPlatform.dll` loads only when the optional **Windows Hypervisor Platform**
feature is installed, so the P/Invoke resolving at all proves the feature is
present. Capability code 0 is `WHvCapabilityCodeHypervisorPresent`, and the
32-bit result is `1`, so the Microsoft hypervisor is running.

⛔ **So `qemu -accel whpx` is available on this machine**, and the
`qemu-system on Windows with -accel whpx` row in `BSD-01`'s table is no longer
resting on an assumption.

⚠ **What is still absent**, checked the same session:

```bash
command -v qemu-system-x86_64 qemu-img oras
```

⭐ **No output, exit code 1, read unpiped.** All three absent on the Windows
host. The WHPX row's friction is an install, not a capability.

## Re-derived, and it agrees with what was recorded

```bash
wsl -d podman-machine-default -u root -- /bin/sh -lc 'ls -l /dev/kvm; cat /sys/module/kvm_intel/parameters/nested; uname -r'
```

```text
crw-rw-rw- 1 root kvm 10, 232 Aug 27 13:12 /dev/kvm
Y
7.2.0-WSL2-STABLE
```

40 threads report `vmx`, and `qemu-system-x86_64` is **absent inside the podman
machine too**, so the nested option is also an install rather than a rebuild.

⚠ **That command fails from Git Bash without the guard**, and the error names
neither the path nor the cause:

```text
/bin/bash: line 1: C:/Program Files/Git/usr/bin/sh: No such file or directory
```

⭐ Git Bash rewrote the guest's `/bin/sh` into a Windows path.
[`../conventions/shell.md`](../conventions/shell.md) section 7 has the rule;
prefix with `MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*'`, or drive `wsl.exe`
from PowerShell.

---

## ⛔ The WHPX trap, and why it lands on THIS machine

From `R18`'s source and `R7`'s issue `#81`, reached independently by two people
on different hardware: **do not hand QEMU `-cpu host` or `-cpu max` under
WHPX.** It can wedge the whole process before the guest runs an instruction.
`R7` reports a fatal privileged instruction with `max` and uses `kvm64-v1`;
`R18` reports a zero-byte serial log and an unresponsive monitor after 12
minutes, and uses named models.

⚠ **`R18` also measured the opposite direction: a named model NEWER than the
host wedges QEMU too.** Its rule keys off the host CPU: Intel family 6 with
model **106 or below** starts at `Icelake-Server-v7`, and anything it cannot
place keeps the newest entry, `GraniteRapids-v2`.

⛔ **This machine reports model 154, so it falls into "cannot place" and would
be offered `GraniteRapids-v2`.** Granite Rapids is a 2024 server part and this
is a 2021 client part, which is the newer-than-host direction `R18` measured as
wedging.

⚠ **That is derived, not measured.** No QEMU is installed here, so nothing was
run to confirm it. `R18` recovers with a dead-VM fast fail in about 2 minutes
and falls back through the list, so the predicted cost is a slow first boot
rather than a failure. ⭐ **A session that installs QEMU here should try
`-cpu Icelake-Server-v7` or `-cpu kvm64-v1` first and record what happens.**
That single measurement is worth taking early.

---

## Boot a BSD without building one

⭐ **Four sources publish artefacts that already boot. None of them is
`pkgforge-dev/docker-bsd`, which publishes a root filesystem nothing can run.**

| source | artefact | for |
| --- | --- | --- |
| `R11` `acj/freebsd-firecracker` | `freebsd-kern.bin`, `freebsd-rootfs.bin.xz`, a `firecracker` binary, an SSH key pair | Firecracker, FreeBSD 15.1 |
| `R13` `acj/netbsd-firecracker` | the same four names, NetBSD 11 | Firecracker |
| `R19` `anyvm-org/freebsd-builder` | `freebsd-15.1.qcow2.zst` plus a `.profile.json`, a `.qemu` file and a key pair, per release and architecture | QEMU |
| `R7` `NetBSDfr/smolBSD` | `rescue-amd64.img.xz` at about 10 MB, `build-amd64.img.xz`, both `amd64` and `evbarm-aarch64` | QEMU microvm, Firecracker |

⚠ **`R19` covers FreeBSD 12.4 through 15.1 across `x86_64`, `aarch64`,
`riscv64` and `powerpc64`**, which is wider than anything this project would
build. Its README footnotes what is missing and why, including an upstream
`13.4` `riscv64` image that is a broken 32-byte stub rather than a disk.

### The lowest-friction FreeBSD guest, from `R17`

⭐ FreeBSD release engineering publishes **BASIC-CI** images that need no
installer and no custom build. They boot with a serial console, DHCP and
`growfs`, and their `sshd` accepts root with an **empty password on first
boot**, so provisioning is plain SSH.

⛔ **Close that door in the same step that opens it.** `R17` installs a per-run
key and disables empty-password login before anything else runs, and binds the
forwarded port to `127.0.0.1` only.

⚠ **Pipe scripts into `sh -s` on a FreeBSD guest.** Root's login shell is
`csh`, so a command string passed as an SSH argument is parsed by `csh` rather
than by `sh`. Same class as
[`../conventions/shell.md`](../conventions/shell.md) section 7, different shell.

---

## `R7`, smolBSD: a NetBSD microvm with a Docker-shaped front end

```bash
git clone --depth 1 https://github.com/NetBSDfr/smolBSD.git smolBSD
```

```bash
bmake SERVICE=rescue build
```

```bash
./startnb.sh -k kernels/netbsd-SMOL -i images/rescue-amd64.img
```

Boots through the **PVH entry point** into QEMU's `microvm` machine or into
Firecracker in about 10 milliseconds. `bmake kernfetch` downloads the kernel, or
take it directly from `https://smolbsd.org/assets/netbsd-SMOL`.

The Docker-shaped half builds from a file that reads like a `Dockerfile`, where
`FROM` names NetBSD sets rather than an image:

```bash
./smoler.sh build smolerfiles/Dockerfile.caddy
```

```bash
./smoler.sh run bsdshell-amd64:latest -P -m 1024 -c 2
```

⚠ `-P` allocates a real pty instead of QEMU's `stdio`, which anything
full-screen needs.

### ⭐ Images distributed through an OCI registry, as raw bootable disks

```bash
./smoler.sh push myimage-amd64:latest
```

```bash
./smoler.sh pull myimage-amd64:latest
```

Default repository `ghcr.io/netbsdfr/smolbsd`, overridable with `SMOLREPO`. The
transport is [`oras`](https://oras.land).

⛔ **The Windows trap, from `#81`: `oras` pull writes a zero-byte file.** A tag
contains a colon, `name:latest`, and that is not a legal NTFS filename. `oras`
neither errors nor sanitises. The reporter pulled inside a container and
renamed. ⚠ **This is the same family as the reserved-device-name rule already in
this repository's `.gitignore`**, and it will bite `docker-bsd` the moment it
distributes anything by `oras` on Windows.

⚠ **Two more from `#81`, both open.** Building needs `bmake` and disk tooling,
so the reporter built under WSL2 and copied the image out, and `startnb.sh` has
no flag to disable acceleration, which forces nested virtualisation for a build
that does not need it. The WSL2-built image then panicked on Windows with
`ffs_newvnode: dup alloc`. ⛔ **Building on Linux and running on Windows is not
known to work.** Nothing here has reproduced it either way.

### Why smolBSD is NetBSD only

⭐ The maintainer's own costing, `#66`: `bhyve` and `vmm` have **no QEMU
bindings**, and QEMU is what provides the `microvm` machine type and
VirtIO-MMIO. Support for FreeBSD and OpenBSD is planned rather than refused, and
a commenter names an in-progress FreeBSD branch adding bhyve QEMU bindings.

---

## `R26`, bsdkrun: an OCI image booted as a microVM

⭐ **The closest thing anyone has built to what `BSD-01` describes.**

```bash
bsdkrun freebsd -- uname -a
```

```bash
bsdkrun linux alpine
```

The second pulls an OCI image from any registry with no daemon, extracts the
root filesystem, and boots it as a microVM. Built on `libkrun`, so a VM is a
process rather than a service.

⛔ **macOS on Apple Silicon and Linux on amd64 or arm64 only. No Windows.** On
this host it lives inside the WSL2 machine, which is nesting. ⚠ Under the
measured nested KVM above that is the floor rather than the target, and it is a
good floor: it would behave the same there as on a native Linux runner, which is
what the operator's universal ask wanted.

⚠ FreeBSD on Linux amd64 needs their **PVH-enabled `libkrun` fork**, so this is
not a plain `cargo install`. NetBSD direct-boots its kernel everywhere.

---

## `R6`: what WSL's host expects from a guest, if anyone ever writes one

⛔ **Do not adopt the patch.** It rebuilds `wslservice.exe`, which is the service
running this machine's podman machine. The protocol is the part worth keeping,
and it is small enough to restate.

| step | port | message |
| --- | --- | --- |
| 1 | guest connects out to **50000** | `LX_INIT_GUEST_CAPABILITIES`, type 1, with a kernel version string |
| 2 | two more connects to **50000** | the notify and init sockets |
| 3 | guest sends | `LX_MINI_INIT_CREATE_INSTANCE_RESULT`, type 33, `Pid` 1, naming its callback port |
| 4 | guest answers | `LX_INIT_CONFIGURATION_INFORMATION_RESPONSE`, type 6 |
| 5 | guest answers | `LX_INIT_CREATE_SESSION_RESPONSE`, type 3 |
| 6 | guest **listens** on **60000** | accepts init, initial, then five more sockets |
| 7 | guest reads | `LX_INIT_CREATE_PROCESS_UTILITY_VM`: rows, columns, then byte offsets for filename, working directory, command line and environment |
| 8 | sockets 0 to 2 | standard input, output and error. `forkpty`, then `/bin/sh` |

⭐ **The socket family is `AF_HYPERV` with a `sockaddr_hvs` carrying
`sa_len`, `sa_family` and `hvs_port`.** FreeBSD has that already through its
Hyper-V support; nothing in the guest half needed a kernel change.

⭐ **The more useful half is what the host patch revealed.** WSL creates its own
VM by calling `CreateComputeSystem()` with a JSON document naming a UEFI boot
from a SCSI attachment and an `HvSocket` configuration. ⚠ **Reaching the Host
Compute System directly, or Hyper-V's own PowerShell module, requires no WSL
patch at all.** That is the untried avenue with the lowest cost of the ones
`BSD-01` lists, and nothing here has attempted it.

---

## ⛔ The `/dev/kvm` question on CI runners, answered

⚠ **Not measured here.** This session has no runner to probe. Four references
that do run on them agree, which is a sourced claim rather than an assumption.

| runner | `/dev/kvm` | source |
| --- | --- | --- |
| `x86_64`, GitHub-hosted | ✅ present, and a `udev` rule is applied to make it **writable** | `R17` |
| `arm64`, GitHub-hosted | ❌ absent. "low performance, and kvm disabled" | `R8`, `R26`, `R11` |

⭐ **So a universal Linux path is possible on `x86_64` and impossible on
arm64.** `BSD-01` called this the single highest-value unknown because it
decides whether the uniform option can exist. It can, on one architecture.

⚠ **The unaccelerated fallback has a measured cost**: `R17` records a full
FreeBSD first boot under TCG overrunning a ten-minute budget.

---

## ⛔ A correction to this file

The section at the top of this document says `qemu-user` cannot help because it
emulates a foreign architecture presenting Linux syscalls, and that **"there is
no counterpart presenting FreeBSD syscalls on a Linux kernel"**.

⛔ **A counterpart was built.** `R28`, `AkihiroSuda/lsf`, does exactly that job
with `PTRACE_SYSCALL`; [`findings.md`](findings.md) carries how.

Its README's own route, quoted rather than adapted, and ⚠ **not run here**:
this Windows host has no `docker`, and `lsf` needs a Linux kernel of 5.6 or
newer, so it would have to run inside the podman machine.

```bash
docker build -t lsf .
```

```bash
docker run -it --rm --security-opt seccomp=unconfined lsf
```

Its README shows `uname -a` inside that container reporting
`FreeBSD 13.1-RELEASE-p1` while the kernel underneath is Linux.

⚠ **The conclusion does not move and the reason does.** One commit, dated
2022-08-29, never touched since. Its own status section says proof of concept,
crashes very frequently, many syscalls unimplemented, amd64 only. ⭐ **The
honest statement is that a reverse Linuxulator was attempted and abandoned, not
that none can exist.**

⭐ **It also explains the 139 at the top of this file.** In its own words, the
Linux kernel does not validate the OSABI of an ELF binary on `execve`. That is
why the loader **accepts** a FreeBSD binary and it dies at its first syscall,
rather than being refused with `Exec format error`.

---

## `BSD-02`, answered per BSD

⛔ The entry asks for a written answer per BSD, with the evidence, and it does
not close as out of scope. Here is what the sweep found. ⚠ **"Runnable" is
split**, because the entry's premise conflated two questions.

| BSD | as an OCI container | as a bootable guest |
| --- | --- | --- |
| FreeBSD | ✅ `ocijail` (`R25`, `v0.6.0`, behind `podman-suite`) and `runj` (`R10`, proof of concept, refuses production use). Needs a FreeBSD host. | ✅ Firecracker (`R11`), QEMU (`R8`, `R18`, `R19`), bhyve, `libkrun` (`R26`), and upstream BASIC-CI and `.vhd` images |
| NetBSD | ⚠ no jail-equivalent OCI runtime. ⭐ CBSD (`R9`) manages NetBSD from 15.0.6 and is OCI-aware on FreeBSD | ✅ ⭐ smolBSD (`R7`) as a 10 ms microvm, Firecracker (`R13`), QEMU (`R8`), `libkrun` (`R26`) |
| OpenBSD | ❌ none found | ✅ QEMU through `R8` and `R18` |
| DragonFly | ❌ none found | ✅ QEMU through `R8` and `R18` |

⭐ **The premise that needed correcting**: `BSD-02` says the other three are
"publishable and, as far as is known, not yet runnable anywhere". They are all
runnable as guests, and two of them have been for years. What none of the three
has is a **jail-equivalent runtime**, which is a narrower and still-true claim.

---

## Lessons

| tag | lesson |
| --- | --- |
| `adopt` | ⭐ Read the tracker. Again. The `/dev/kvm` answer, the WHPX wedge, the psshfs corruption and the smolBSD Windows report are all in trackers and none is in a README. This is the second sweep in a row where the tracker outweighed the code. |
| `adopt` | ⛔ Under WHPX, never `-cpu host` and never `-cpu max`. Two independent projects measured it wedging QEMU. Use a named model no newer than the host. |
| `adopt` | ⭐ `WHvGetCapability` through `WinHvPlatform.dll` answers "can this Windows run a VM" unelevated, in one call. Prefer it to a feature query that needs elevation. |
| `adopt` | Publish artefacts somebody can boot. Four references publish a kernel and a root filesystem; the thing that made them useful was not the image format. |
| `adopt` | ⭐ An OCI registry will carry a raw bootable disk as an artefact, through `oras`. The registry is a distribution channel, not only a container format. |
| `avoid` | ⛔ Do not patch `wslservice.exe`. It runs the podman machine that everything else here depends on. |
| `avoid` | Do not build FreeBSD guest images. `R19` publishes twelve releases across four architectures already. |
| `avoid` | ⚠ Do not read `R6`'s roadmap ticks as a working product. Its two open issues ask how to build it and how to test it, and both are unanswered since 2025-10-19. |
| `honest-limit` | ⛔ arm64 CI runners have no `/dev/kvm`. Any uniform-across-hosts design is `x86_64` only, or it is emulated in exactly the place uniformity was meant to help. |
| `honest-limit` | ⚠ NetBSD's `mount_psshfs` caches attributes for 30 seconds over the page cache and serves short reads under a parallel build. Object files came back `file too short` ten seconds after being compiled. Use NFS or a local disk with copy-back. |
| `honest-limit` | ⚠ Nested AMD-V mishandles the L2 guest's AVX512 XSAVE state, so a modern guest takes random SIGSEGVs while its kernel stays up. Intel hosts are unaffected, this one included. |
| `future` | The Host Compute System API takes a JSON document naming a UEFI boot and an hvsocket, and is what WSL itself calls. A BSD guest through HCS or the Hyper-V module needs no patched service. Untried. |

---

## ⛔ What this half does not know

- **No BSD was booted.** Not here, not anywhere in this sweep. Every boot time
  quoted is somebody else's, attributed where it appears.
- **No QEMU is installed on either side**, so the WHPX prediction for this
  machine's Model 154 CPU is derived from `R18`'s rule and its measurements, and
  is explicitly not a measurement of this host.
- ⚠ **`oras` is absent here**, so the zero-byte pull on Windows is `R7`'s
  report and has not been reproduced.
- ⚠ **The FreeBSD `.vhd` route that `BSD-01` recommends was not re-checked this
  session.** Nothing found contradicts it, and nothing found confirms it either.

---

# ⭐ Measured 2026-08-27, second session: QEMU installed, and BSDs booted

⛔ **Nothing above is edited.** Everything in this section was run on the same
Windows 11 Pro 26200 machine, with **QEMU 11.1.0 installed** by
`scoop install qemu`, which is the install the section above records as absent.
The experiments are committed in `pkgforge-dev/docker-bsd` under
`experiments/`, and each names what it measures.

⚠ **Conditions, because a measurement carries them or it is not one.** QEMU
11.1.0 (`v11.1.0-12130-ge470268ff4`), host CPU `Intel64 Family 6 Model 154
Stepping 3`, a 12th Gen Core i7-12700H, unelevated, with the WSL2 podman
machine running throughout.

---

## ⛔ Correction 1: the WHPX CPU-model prediction was wrong for this host

The section **"The WHPX trap, and why it lands on THIS machine"** above says
this machine's model 154 falls into `R18`'s "cannot place" branch, would be
handed `GraniteRapids-v2`, and that this is the newer-than-host direction
measured as wedging. It is careful to label that **derived, not measured**, and
asks a later session to try `Icelake-Server-v7` or `kvm64-v1` first and record
what happens.

⭐ **It was run. Nothing wedged, including the two models the advice forbids.**

| accel | `-cpu` | QEMU started | serial log | verdict | CPU seconds at 35 s |
| --- | --- | --- | --- | --- | --- |
| whpx | `Icelake-Server-v7` | ✅ | 3,315 B | kernel ran, no disk | 15.41 |
| whpx | `kvm64-v1` | ✅ | 3,233 B | kernel ran, no disk | 15.73 |
| whpx | `qemu64` | ✅ | 3,300 B | kernel ran, no disk | 14.52 |
| whpx | ⛔ `host` | ✅ | 3,321 B | kernel ran, no disk | 15.34 |
| whpx | ⛔ `max` | ✅ | 3,321 B | kernel ran, no disk | 15.44 |
| tcg | `qemu64` | ✅ | 3,492 B | ⭐ **booted to a shell** | 3.98 |

⚠ **This does not falsify `R18` or `R7`.** Their measurements were taken on
QEMU 9.x, one on a Zen 5 AMD part and one on GitHub's Windows runner fleets.
⛔ **What it falsifies is the prediction this repository wrote about this
machine**, which said the published rule would hand it a wedging model. On
QEMU 11.1.0 on an i7-12700H, no model in the set wedged, and a zero-byte serial
log never appeared.

⭐ **The advice to prefer a named model stands anyway**, and the reason is now
cheaper to state: it costs nothing, and the failure it avoids is a twelve-minute
hang with no output.

---

## ⭐ Correction 2: why smolBSD boots under TCG and not under WHPX

The sweep treated the CPU model as the open question. ⛔ **It is not the CPU
model.** Every `-cpu` above produced the same NetBSD boot and the same dead end,
and the difference is the **accelerator**:

```text
tcg :  pv0 at mainbus0 -> qemufwcfg0 -> virtio0 (viommio @0xfeb00e00) -> ld0
       -> dk0 at ld0: "rescueroot" -> root on dk0 -> a shell
whpx:  mainbus0, cpu0, ioapic0, isa0, com0.  And nothing else, ever.
```

⛔ **NetBSD's paravirtual bus never attaches under WHPX.** With no `pv0` there
is no `qemufwcfg`, with no firmware-config device there is no virtio-mmio
enumeration, and with no virtio there is no disk. The kernel is healthy
throughout: it prints its banner, sizes memory, attaches `com0` and then sits at
`root device:` forever, once per second, until it is killed.

⭐ **FreeBSD, booted under the same accelerator, prints the reason in one
line:**

```text
Hypervisor: Origin = "Microsoft Hv"
```

and `sysctl -n kern.vm_guest` answers `hv`, where the same FreeBSD under
Firecracker answers `kvm`.

⛔ **Under WHPX the guest sees the HOST's hypervisor signature, not QEMU's.**
FreeBSD has Hyper-V support and carries on; NetBSD's `pv` bus is looking for
QEMU, does not find it, and concludes it is on bare metal.

⚠ **So this is a guest-side limitation, specific to kernels that reach their
disk only through a paravirtual bus.** A kernel with ordinary PCI drivers is
unaffected, which is exactly why the FreeBSD result below works with the same
accelerator, the same machine type and the same CPU model.

⭐ **smolBSD is therefore not out of reach on Windows. It is out of reach
accelerated.** Under `-accel tcg` it boots to a NetBSD shell with a **499 ms**
kernel boot time, from a 4.2 MB compressed image.

---

## ⭐ Correction 3: the Host Compute System, costed

The `future` lesson above says the Host Compute System "takes a JSON document
naming a UEFI boot and an hvsocket, and is what WSL itself calls. A BSD guest
through HCS or the Hyper-V module needs no patched service. **Untried.**"

⭐ **Tried. Half of it holds and the half that matters does not.**

| probe | result |
| --- | --- |
| `computecore.dll` loads unelevated | ✅ and all seven wanted HCS v2 entry points resolve |
| `vmcompute.dll` loads unelevated | ✅ but it exports 36 `Hcs*` functions and **not** `HcsCreateOperation`; it is the older surface |
| `HcsEnumerateComputeSystems`, a **read** | ⛔ `0x8037011B`, "Only administrators or users that are members of the Hyper-V Administrators user group" |
| `Get-VM` | ⛔ refused, the same way |

⭐ **The API is genuinely reachable with no patched service and no third
party**, which is the finding the sweep extracted from `R6`, and it is correct.
⛔ **Every useful call is privileged.** If enumeration is refused, creation
certainly is, so this route is closed to an unelevated session however elegant
the API is.

⚠ **And the honest comparison is the damning part.** HCS yields a virtual
machine with **no way to talk to it**: it has no serial pipe of its own, and
WSL reaches its own guest over `AF_HYPERV` sockets whose guest half is the 819
lines of C this repository refuses to adopt. QEMU, on the very same hypervisor,
yields a serial console on an ordinary pipe and needs no elevation at all.

---

## ⭐ The recipe that works, on this machine, today

⛔ **Two commands and a wait.** No installer, no ISO, no elevation, no nesting.

```bash
sh experiments/21-fetch-freebsd-ci.sh
```

```powershell
pwsh -NoProfile -File experiments/33-boot-freebsd-whpx.ps1
```

The QEMU line underneath, stated so it does not have to be extracted from the
script:

```text
qemu-system-x86_64 -accel whpx -M q35 -cpu Icelake-Server-v7 -smp 2 -m 2048
  -drive if=none,file=FreeBSD-15.1-RELEASE-amd64-BASIC-CI-ufs.raw,format=raw,id=root0
  -device virtio-blk-pci,drive=root0
  -nic none
  -display none -no-reboot -serial stdio
  -rtc base=utc,clock=host,driftfix=slew
```

What it answers, read back off the console rather than scraped from a boot log:

```text
FreeBSD freebsd 15.1-RELEASE FreeBSD 15.1-RELEASE releng/15.1-n283562 GENERIC amd64
15.1-RELEASE
Intel Xeon Processor (Icelake)
hv
/dev/gpt/rootfs    4.8G    2.5G    2.0G    56%    /
BSD userland is running as root on FreeBSD
```

| number | value |
| --- | --- |
| image, compressed | 666,285,484 B, verified against the published `CHECKSUM.SHA256` |
| image, expanded | 6.03 GiB |
| login prompt | **117.7 s**, and **117.4 s** on a second, independent boot |
| total, boot to shutdown | 133 s, and 142.6 s |
| nesting | ⭐ **none** |
| elevation | ⭐ **none** |

⚠ **Three traps that cost real time here**, none of which is about
virtualisation:

- ⛔ **`Start-Process -ArgumentList` joins the array with spaces and quotes
  nothing.** A QEMU `-append` value of `console=com root=NAME=rescueroot -z`
  arrives as four separate arguments and QEMU dies on `-z: invalid option`.
  Use `ProcessStartInfo.ArgumentList`, which escapes.
- ⛔ **A serial console drops input typed faster than the tty accepts it.** A
  marker of `TOOLKIT-READY-789f28b0` reached the shell as `TOO789f28b`. Type
  one character at a time with a few milliseconds between, and wait for the
  prompt rather than for elapsed time.
- ⛔ **`-serial stdio`, not `-serial mon:stdio`.** The monitor multiplexed onto
  the same pipe puts its own banner into the stream being parsed.
- ⛔ **`-display none` does not mean no network, and QEMU attaches a DEFAULT
  NIC unless told otherwise.** The first version of the experiment printed
  `network NONE` in its own header while the guest brought up `em0`, ran
  `dhclient` and took a lease on 10.0.2.15. ⚠ No inbound door was opened,
  because user mode networking forwards nothing without `hostfwd`, but the
  header was false and it was a header about a security property. `-nic none`
  is what makes it true.

---

## ⭐ FreeBSD under Firecracker, on the nested KVM, measured

The nested option is the floor rather than the target, and it is a good floor.
Inside `podman-machine-default`, using `acj`'s published `v0.11.0` kernel and
root filesystem:

| number | value |
| --- | --- |
| console reaches `login:` | ⭐ **1.8 s** |
| shell over SSH | 32.3 s |
| `kern.vm_guest` | `kvm` |
| root filesystem | `/dev/vtbd0`, 4.8G |

⛔ **The 30-second gap is not FreeBSD booting.** `sshd` accepts the TCP
connection immediately and stalls reverse-resolving the client, because this
experiment deliberately sets up no NAT and the guest's DNS therefore goes
nowhere. `acj`'s own CI never sees it, because it masquerades the guest onto
the runner's network.

⚠ **That distinction is worth more than the number.** The first version of the
experiment asked only SSH, waited 60 s, and reported **"boot FAILED"** about a
FreeBSD that had been up for 295 seconds. A probe that cannot tell "did not
boot" from "booted, and the door was slow" sends the next reader after the
wrong defect.

---

## ⚠ What this section still does not know

⛔ Stated rather than left to be discovered.

- ⚠ **Every number here is one machine.** The CPU-model result in particular
  disagrees with two published reports taken on other hardware and older QEMU,
  and one sample does not settle that.
- ⛔ **`oras` is still absent**, so the zero-byte pull on Windows remains `R7`'s
  report and is still not reproduced.
- ⛔ **The FreeBSD `.vhd` and Hyper-V route is still not re-checked.**
  ⭐ Experiment 32 does bound it: Hyper-V needs elevation this session did not
  have, and the WHPX route needs none, so the `.vhd` route now has a cost the
  recommended one does not.
- ⛔ **The 117 s is NOT explained, and the first write-up of this guessed.**
  It said the time was `growfs` on first boot. The console says otherwise:
  there is no `growfs` line anywhere, the root filesystem comes up
  `FILE SYSTEM CLEAN; SKIPPING CHECKS`, and a second boot took **117.4 s**,
  within 0.3 s of the first, so it is not a one-time first-boot cost either.
  ⭐ `33-boot-freebsd-whpx.ps1` now stamps the loader handoff, the kernel
  banner, the root mount and the start of `rc`, so the next run reports where
  the time goes instead of attributing it.
- ⚠ **Nothing here tested arm64**, and the artefacts used are `amd64` only.
