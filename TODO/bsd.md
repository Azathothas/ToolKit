# TODO: bsd

Reaching a real BSD userland from a Windows host through a podman-shaped
interface.

[`INDEX.md`](INDEX.md) is the list; [`PROGRESS.md`](PROGRESS.md) is the order.
The reference sweep is
[`../docs/reference-sweeps/findings.md`](../docs/reference-sweeps/findings.md)
and the part a later session acts on is
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md).

⛔ **This repository publishes no images.** It holds scripts. The images and
their CI live in `pkgforge-dev/docker-bsd`, which was built and pushed on
2026-08-27 and carries its own measurements in `HISTORY/poc.md`. Anything here
that needs an image consumes one from there.

---

## BSD-01. Run a BSD userland from Windows, with the least friction that works

**Source** The operator, 2026-08-27, with five references supplied, plus a
follow-up naming all four BSDs and `pkgforge-dev/docker-bsd`.
**Category** bsd · **Priority** P1 · **Effort** M · **Status** open

### Problem

```bash
podman run --rm -it "example.io/freebsd" -sh
```

from Windows, into a real BSD, without `wsl` inside `linux` inside `qemu`
inside `bsd`.

### Premise, measured

⭐ **The images half is done and is not in this repository.**
`pkgforge-dev/docker-bsd` builds FreeBSD, NetBSD, OpenBSD and DragonFly for
`amd64` and publishes to `ghcr.io`. FreeBSD's upstream archives are verified and
loaded rather than rebuilt; the other three publish no OCI images at all, so
those are genuinely new.

⛔ **The running half has one hard constraint and it is not a bug.** Measured
2026-08-27: FreeBSD's own image on this machine's Linux podman machine exits
**139**, a SIGSEGV. The Linux ELF loader accepts the binary and it dies on its
first syscall. It is **not** `Exec format error`, so `binfmt_misc` and
`qemu-user` are both irrelevant: they solve a foreign *architecture* presenting
*Linux* syscalls, and nothing presents BSD syscalls on a Linux kernel.

⚠ **That is a constraint to route around, not a reason to stop.** A BSD
userland needs a BSD kernel. The only real question is which hypervisor boots
it, and every option below was ranked on the operator's own three criteria:
friction, performance, interop.

### Approach: rank the workarounds, do not declare a blocker

⭐ **All of these give one hypervisor, not nesting.** Windows already runs one
for WSL2, and Hyper-V is the same hypervisor, so a BSD guest beside it is one
level deep.

| option | friction | performance | interop | verdict |
| --- | --- | --- | --- | --- |
| ⭐ **Hyper-V guest from FreeBSD's published `.vhd`, plus `podman system connection add`** | ⭐ lowest. Upstream ships `FreeBSD-15.1-RELEASE-amd64-ufs.vhd.xz`, which is Hyper-V's native disk format, so there is **no installer and no ISO**. | native. Type 1 hypervisor, no emulation. | ⭐ full. `podman -c freebsd run ...` with the real client, no wrapper, no patch. | **recommended** |
| qemu-system on Windows with `-accel whpx` | medium. A qemu install and a boot script. | near native. WHPX is the same Windows hypervisor. | same as above once podman is inside. | ⚠ **fallback.** Worth it only if Hyper-V is unavailable or unwanted. |
| `podman machine init --image` with a FreeBSD machine image | high. Needs Ignition, which FreeBSD does not have. | native. | full, if it ever worked. | ⛔ **refused.** This is `baude`'s suggestion and it starts with porting a CoreOS provisioning system. |
| Wait for `containers/podman#19939` | none, and it never arrives. | n/a | n/a | ⛔ **refused.** Open, unmerged, maintainer refused twice. |
| A `podman` wrapper script | low, and pointless. | n/a | ⚠ worse: it shadows a real binary. | ⛔ **refused.** `podman -c` and `podman system connection default` already do this. |
| Nested qemu inside the WSL machine | low | ⛔ worst. Emulation inside a VM. | fine | ⛔ **refused**, and it is the thing the ask was written to avoid. |

⭐ **The `.vhd` is what makes the recommendation the low-friction one**, and it
was found after the first pass had already concluded otherwise. Without it the
answer is "install FreeBSD from an ISO", which is real friction; with it the
guest is a download, a decompress and a `New-VM`.

**The client half needs nothing built.** Measured: a podman connection is an
ordinary SSH URI to a podman socket.

```text
podman-machine-default  ssh://user@127.0.0.1:53512/run/user/1000/podman/podman.sock
```

So the whole client side is one command, and `podman system connection default`
removes even the `-c`.

### Decision, for the operator

⭐ **Recommended: the Hyper-V `.vhd` guest.** Lowest friction of anything that
can work, native performance, and full podman interop with no wrapper and no
upstream dependency.

⚠ **The honest cost:** a VM the operator keeps, not a container that
disappears. One guest, one disk, stopped when unused.

⚠ **Second-order, worth ruling on at the same time:** provision the guest by
hand once and snapshot it, or script it. Scripted costs more now and is the only
version that survives moving machines, which the intake says happens often.
**Recommend scripted**, and the script belongs in this repository, which is
what this repository is for.

### ⛔ The operator has reopened this, and one of the asks contradicts the table

⚠ **Recorded on 2026-08-27 by the session that closed `WSL-01` to `WSL-05`.**
It did not rule on any of it. This section exists so the next session works from
the ask rather than from a summary of it, and so the contradiction is impossible
to walk past.

**What was asked for, in the operator's terms:**

1. ⭐ **A native path per host.** Hyper-V on Windows, which the table above
   already recommends, **and QEMU on Linux, which the table does not mention at
   all.** ⚠ Every row above is written from a Windows host. The file is
   currently silent on what a Linux machine or a CI runner does, and that is a
   gap rather than a decision.
2. ⭐ **A universal fallback that is identical on both platforms.** The most
   stripped-down Linux kernel and image that can run nothing but a VM layer,
   which then boots the BSD inside it. Deeper nesting, accepted deliberately, in
   exchange for one code path that behaves the same under podman-on-WSL and on a
   native Linux runner, so a user cannot tell which they are on.
3. **Reconcile both with what this file already ranks**, with no contradictions
   left standing.

### ⭐ The operator ruled on this, 2026-08-27. Read this before the table.

⛔ **The ruling has two halves and the second one is the one that gets missed.**

**Half one: correct the refusal and rank both.** The nested row keeps its
wording and gets the measurement written underneath it, and nested-QEMU is then
ranked against Hyper-V on numbers rather than dismissed. That is what the rest
of this section sets up.

⛔ **Half two: nesting is the FLOOR, not the target.** In the operator's words,
nested virtualisation is "a well known well documented technique"; what is
wanted is ⭐ **a novel approach that is better, and that avoids the drawbacks and
limitations** of the nested one. The nested design is what you fall back to
having failed to find one.

⛔ **So the next session does NOT start by building the nested stack.** It
starts by exhausting the alternatives, and it may only reach for nesting once it
can say, in writing, what it tried and why each one failed. A session that opens
by writing a QEMU wrapper has skipped the entire ask.

⚠ **This is a search with a deadline, not an open-ended one.** The nested option
is known to work and is written down, so the search has a floor and cannot fail
to produce something shippable. What it must not do is stop early and call the
known answer a conclusion.

**What "exhaust everything" means concretely.** ⛔ Not a licence to speculate.
Each of these is a claim to be measured or a body of prior art to be read, and
each is recorded with its result whether it wins or loses:

- **A BSD kernel that runs as a Linux process.** Does any usermode or rump-style
  BSD kernel exist that would give a BSD userland a real BSD kernel without a
  VM at all? NetBSD's rump kernels are the obvious prior art and the obvious
  first read.
- **A hypervisor lighter than a full VM.** Firecracker, cloud-hypervisor and
  `bhyve` are not qemu, and boot times differ by an order of magnitude. Does
  any of them take a BSD guest and run under WSL2's nested KVM?
- **The host's own hypervisor, addressed directly.** Windows has WHPX and
  Hyper-V; a Linux host has KVM. ⚠ The universal ask is for one behaviour, not
  necessarily one implementation. A thin layer that presents the same interface
  over two native backends may satisfy it with no nesting anywhere.
- **Whether the WSL2 VM can host the BSD directly**, rather than a VM inside it.
  ⛔ Measured on 2026-08-27 and it cannot: WSL2 runs one Linux kernel and a BSD
  userland on it exits 139. Recorded so nobody re-derives it.
- **What `pkgforge-dev/docker-bsd` already produces.** It builds all four BSDs.
  ⚠ Whether any of those artefacts is bootable rather than merely an image is
  `BSD-02`, and the answer changes what any of this has to build.

⛔ **A negative result is a result.** Each avenue that fails gets a row in this
file with what was tried and what it returned, so the next session does not
repeat it. That is the whole reason the search is being asked for rather than
the answer.

### ⛔ The contradiction, stated plainly

**The table above refuses ask 2 by name.** Its last row is
"Nested qemu inside the WSL machine", verdict ⛔ **refused**, with the reason
"it is the thing the ask was written to avoid". The new ask asks for exactly
that, and for a reason the table never weighed: **uniformity across hosts**, not
performance.

⚠ **Both positions are defensible and they are optimising different things.**

| | the table's position | the new ask's position |
| --- | --- | --- |
| optimises | performance and simplicity on the one host measured | one behaviour on every host, including CI |
| costs | a second, different path for Linux and for CI, which nobody has written yet | emulation inside a VM, and a kernel image to build and keep |
| fails when | the operator moves to a Linux machine, or CI needs to run a BSD | the workload is slow enough that emulation matters |

⛔ **Do not resolve this by rewriting the table's refusal into an acceptance
without saying so.** The refusal was written against a measured constraint and
it keeps its wording; if the new ask wins, the row gets a correction underneath
it in the way [`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md)
requires of any premise a decision overturns.

### ⭐ Measured 2026-08-27: nested KVM is available inside WSL2, and that
### disproves the table's stated reason for refusing nesting

⛔ **The refusal row says "⛔ worst. Emulation inside a VM." On this machine it
would not be emulation.** Read from inside `podman-machine-default`, kernel
`7.2.0-WSL2-STABLE`:

```bash
wsl -d podman-machine-default -u root -- /bin/sh -lc 'ls -l /dev/kvm; grep -c vmx /proc/cpuinfo; cat /sys/module/kvm_intel/parameters/nested'
```

```text
crw-rw-rw- 1 root kvm 10, 232 /dev/kvm
40
Y
```

| fact | value |
| --- | --- |
| `/dev/kvm` inside the WSL2 VM | ⭐ present, mode `crw-rw-rw-` |
| CPU threads reporting `vmx` | 40 |
| `kvm_intel.nested` | `Y` |
| `qemu-system-x86_64` inside that VM | absent, so it is an install and not a rebuild |

⭐ **A BSD guest under QEMU inside the podman machine would run KVM-accelerated,
not emulated.** ⛔ The refusal's premise is therefore false on this host, and the
row keeps its wording with this correction underneath it rather than being
quietly rewritten.

⚠ **What this does NOT settle.** Nested KVM being *available* is not the same as
it being *fast enough*, and none of the following is measured:

- BSD boot time under nested KVM against the Hyper-V guest, which is the
  comparison the table's performance column claims to rank on;
- whether a GitHub-hosted Linux runner exposes `/dev/kvm` at all. ⛔ **Do not
  assume it does.** If it does not, the universal option is emulated in exactly
  the place it was meant to make uniform, and the whole argument inverts.
- whether `podman machine` survives a second hypervisor running inside its own
  VM under load.

### ⚠ What is NOT yet measured, and must be before either is ranked

⛔ Nothing below is a claim. It is the list of things the next session has to
turn into numbers, because the current table ranks on friction, performance and
interop and neither new option has any of the three measured.

- **What does a BSD boot cost under nested KVM, against the Hyper-V guest?**
  ⭐ This is now the deciding number, and the nesting measurement above is what
  makes it worth taking.
- **Does a CI Linux runner have `/dev/kvm`?** ⛔ The single highest-value
  unknown, because the universal option exists to make CI and the laptop behave
  the same, and the answer decides whether it can.
- **What is the smallest kernel and initramfs that boots and runs qemu?** The
  ask floats jemalloc; ⚠ that is an allocator, not a kernel or an image, so the
  intent needs restating before it can be measured.
- **What does a Linux host do today?** `BSD-02` already measured that a BSD
  userland on a Linux kernel exits 139. The native-Linux answer is therefore
  QEMU or nothing, and its friction has never been compared against the Hyper-V
  row.

### Prove

⛔ **Written after the ruling**, because the acceptance depends on which option
is approved and an acceptance for an unapproved design is a paragraph pretending
to be a gate. For the recommended one:

```bash
podman -c freebsd run --rm ghcr.io/pkgforge-dev/freebsd:15.1-static-amd64 /bin/sh -c 'uname -sr'
```

Exit code 0, read unpiped, stdout reading `FreeBSD 15.1`.

⭐ **The blocking question is now answered, and it was measured, not assumed.**
On 2026-08-27, with the WSL2 podman machine running:

| probe | result |
| --- | --- |
| `(Get-CimInstance Win32_ComputerSystem).HypervisorPresent` | `True` |
| `Get-Service vmms` | ⭐ **Running**, startup `Auto` |
| `Get-Module -ListAvailable Hyper-V` | present, v2.0.0.0 |

`vmms` is Hyper-V's virtual machine management service. It is running on this
machine **at the same time as** the WSL2 podman machine, so the coexistence the
recommendation rests on is a fact here rather than an expectation.

⚠ Two caveats, stated rather than glossed. `Microsoft-Hyper-V-All` did not
appear in the unelevated registry view, so the management GUI may not be
installed even though the platform is; that costs nothing, since `New-VM` is
PowerShell. And `Get-WindowsOptionalFeature` needs elevation, so the definitive
feature list was not read. ⛔ Neither changes the finding: a running `vmms` is
stronger evidence than a feature flag.

### ⭐ The reference sweep of 2026-08-27 corrected four things and added three options

⛔ **Nothing above was edited.** The table keeps its wording and its verdicts;
this section is what 28 references say about them. The evidence is
[`../docs/reference-sweeps/findings.md`](../docs/reference-sweeps/findings.md),
ranked, and the commands are in
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md).

#### 1. ⭐ The `-accel whpx` row is no longer a fallback resting on an assumption

The table calls it "⚠ **fallback.** Worth it only if Hyper-V is unavailable or
unwanted", and rates its friction medium because it needs a QEMU install and a
boot script. ⭐ **Measured 2026-08-27, unelevated**: `WinHvPlatform.dll` loads
and `WHvGetCapability` reports `HypervisorPresent` as `1`. The Windows
Hypervisor Platform feature is installed on this machine and the hypervisor is
running.

⭐ **That also closes the caveat under `Prove` below**, which recorded that
`Microsoft-Hyper-V-All` was invisible in the unelevated registry view and that
`Get-WindowsOptionalFeature` needs elevation. It never needed elevation; it
needed the right call.

⚠ **The friction estimate stands and gets a number.** `qemu-system-x86_64`,
`qemu-img` and `oras` are all absent here, and so is QEMU inside the podman
machine. Every option below starts with an install.

#### 2. ⛔ Under WHPX, `-cpu host` and `-cpu max` wedge QEMU

⚠ **A trap nothing in this file anticipated, and the row that recommends WHPX
would have walked into it.** Two projects measured it independently on different
hardware: `-cpu max` gives a fatal privileged instruction, and `-cpu host` can
hang the whole QEMU process with a zero-byte serial log and an unresponsive
monitor. The fix is a named CPU model, and the model must be **no newer than the
host**, because that direction wedges too.

⛔ **This machine is `Intel64 Family 6 Model 154`, an i7-12700H**, which the
published rule cannot place and would therefore hand the newest model. ⚠ That
is the wedging direction. Derived, not measured, and
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md) says
which model to try first and asks for the result.

#### 3. ⭐ Three options the table does not contain

| option | friction | performance | interop | verdict |
| --- | --- | --- | --- | --- |
| ⭐ **A BSD microvm: smolBSD for NetBSD, or `acj`'s Firecracker kernel and rootfs for either** | ⭐ low. Upstream publishes a kernel and a root filesystem that boot; nothing is built. | ⭐ best measured here. About **10 ms** for smolBSD through PVH, about **12 s** for FreeBSD under Firecracker in CI. | ⚠ not podman. SSH, or a Docker-shaped wrapper the project ships. | ⭐ **the strongest new candidate.** It is what the ask wanted: better than nesting, and not a full VM. |
| **The Host Compute System API, or the Hyper-V module, driven directly** | ⚠ unknown. Nobody here has tried it. | native. It is the hypervisor WSL2 already runs on. | none until something is built on top. | ⚠ **untried, and the cheapest untried thing.** See below. |
| **`libkrun`, through `bsdkrun`** | medium on Linux, ⛔ **unavailable on Windows** | native under KVM. A VM as a process, no daemon. | ⭐ **boots an OCI image directly**, which is the gesture `BSD-01` opens with | ⚠ **the best shape, on the wrong host.** Inside the WSL2 machine it is nesting, so it is the floor rather than the target. |

⭐ **The microvm row is the answer to the operator's second ruling**, which asked
for something better than nesting rather than the nested design itself. A
microvm on the host's own hypervisor is one level deep, boots in milliseconds
rather than seconds, and has published artefacts for both FreeBSD and NetBSD.

#### 4. ⭐ WSL itself will drive a non-Linux guest, and the cost is too high

`BalajeS/WSL-For-FreeBSD` boots FreeBSD as the WSL2 utility VM. The guest half
is 819 lines of C speaking WSL's own protocol over `AF_HYPERV` sockets, and
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md) has
the full message sequence.

⛔ **Refused, and the reason is specific.** The host half is a patch to four
files in `src/windows/service/exe/`, which is `wslservice.exe`. Adopting it
means running an experimental build of the Windows service that also runs this
machine's podman machine, and the whole recommendation above rests on those two
coexisting.

⭐ **What survives is the finding underneath it.** The patch replaces WSL's
generated configuration with a literal document handed to
`CreateComputeSystem()`, naming a UEFI boot from a SCSI disk and an hvsocket
configuration. ⛔ **So the Host Compute System takes a JSON document and boots
an arbitrary UEFI disk, and reaching it needs no WSL patch at all.** That is the
"host's own hypervisor, addressed directly" avenue in the list below, and it is
now the cheapest one nobody has tried.

#### 5. ⭐ The highest-value unknown is answered

The list below calls "Does a CI Linux runner have `/dev/kvm`?" the single
highest-value unknown, because it decides whether the universal option can
exist. ⚠ **Not measured here, and answered by four references that run on those
runners:**

| runner | `/dev/kvm` |
| --- | --- |
| `x86_64`, GitHub-hosted | ✅ present. A `udev` rule is applied to make it **writable**, which is what QEMU tests for |
| arm64, GitHub-hosted | ❌ absent. One maintainer's words: "low performance, and kvm disabled" |

⭐ **The universal option is possible on `x86_64` and impossible on arm64.** It
does not invert the argument, it bounds it. ⚠ And the unaccelerated fallback has
a cost on record: a FreeBSD first boot under TCG overran a ten-minute budget.

#### 6. ⚠ A second reason nesting stays the floor, and it is not performance

The correction above this one showed nested KVM is available and accelerated
here, which removed the table's stated reason for refusing it. ⛔ **A different
reason has now been measured by somebody else**: nested AMD-V mishandles the L2
guest's AVX512 XSAVE state, so a guest whose libc picks the AVX512 string
routines takes random SIGSEGVs across nearly every dynamically linked binary
while its kernel stays up.

⚠ **It does not apply to this machine**, which is Intel. It is recorded because
it is a **correctness** fault of nesting rather than a speed one, and this file
had ranked nesting on speed alone.

#### ⭐ What this changes about the Approach

⛔ **The recommendation is not overturned and the order of work is.** The
Hyper-V `.vhd` guest is still the lowest-friction thing that certainly works,
and nothing found contradicts it. ⚠ It was also not re-checked this session.

⭐ **What the next session should do first, and why it is not the `.vhd`:**

1. **Install QEMU and boot a smolBSD rescue image under `-accel whpx`**, with a
   named CPU model rather than `host`. It is about 10 MB, it is published, and
   it is the shortest path from nothing to a BSD kernel running on this
   machine's own hypervisor. Record which CPU model worked.
2. **Then `acj`'s FreeBSD kernel and rootfs**, which is the same question for
   the BSD that matters most and has a runtime story.
3. **Then the Host Compute System directly**, because it is the only untried
   avenue with a native path and no third-party dependency.
4. ⚠ **The `.vhd` guest stays the fallback that is known to work**, and it is
   what gets built if the three above fail.

⛔ **A session that opens by building the nested stack has still skipped the
ask**, and it now has three better things to try before it gets there.
---

## BSD-02. Whether the other three BSDs can be *run*, not merely built

**Source** Derived from BSD-01 and the `docker-bsd` work.
**Category** bsd · **Priority** P3 · **Effort** S · **Status** done

**Problem.** `docker-bsd` builds images for all four. Only FreeBSD has a
documented OCI runtime to run them on.

**Premise.** Read, not measured. FreeBSD has jails plus `ocijail` and `runj`,
and a `podman-suite` package. NetBSD, OpenBSD and DragonFly have no
jail-equivalent OCI runtime that this sweep could find. ⚠ Their images are
therefore publishable and, as far as is known, not yet runnable anywhere.

**Approach.** A written answer per BSD: what runtime exists, or none, with the
evidence. ⛔ It does not close as "out of scope"; it closes with the answer even
when the answer is that there is no route.

**Prove.** A section in
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md)
naming, per BSD, the runtime and its state, or stating that none exists with
what was checked.

### Closed 2026-08-27

**What changed.** No code. The answer, which is what this entry asked for: a
per-BSD table in
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md)
naming the runtime and its state, backed by 28 references read in
[`../docs/reference-sweeps/findings.md`](../docs/reference-sweeps/findings.md).

### ⛔ The premise was wrong, and it was wrong by conflating two questions

⛔ **Kept above, uncorrected, per
[`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md).** The
premise says the other three are "publishable and, as far as is known, not yet
runnable anywhere". ⚠ **"Runnable" is two questions and the answer differs
between them.**

| BSD | runnable as an OCI container | runnable as a booted guest |
| --- | --- | --- |
| FreeBSD | ✅ `ocijail` `v0.6.0`, behind the handbook's `podman-suite`; `runj` as a proof of concept. Needs a FreeBSD host. | ✅ Firecracker, QEMU, bhyve, `libkrun`, and upstream `.vhd` and BASIC-CI images |
| NetBSD | ⚠ no jail-equivalent runtime. ⭐ CBSD manages NetBSD from 15.0.6 | ✅ ⭐ smolBSD as a 10 ms microvm, Firecracker, QEMU, `libkrun` |
| OpenBSD | ❌ none found | ✅ QEMU, through `vmactions` and `anyvm` |
| DragonFly | ❌ none found | ✅ QEMU, through `vmactions` and `anyvm` |

⭐ **So the narrow claim holds and the broad one does not.** Three BSDs have no
jail-equivalent OCI runtime, which is what the entry meant. All four are
runnable as guests, and two of them have been for years. ⛔ The sentence as
written would have told a future session that a NetBSD image cannot be run,
which is false.

### ⭐ What this changes for `pkgforge-dev/docker-bsd`

⚠ **Not this repository's change to make**, and recorded because that repository
is where it lands. `docker-bsd` publishes a root filesystem for four BSDs.
⭐ **For three of them, nothing can run what it publishes**, and the sweep found
two projects that solve the same distribution problem differently:

- `smolBSD` pushes a **raw bootable disk image** to `ghcr.io` through `oras`,
  so the registry carries something that boots;
- `acj`'s Firecracker repositories publish a **kernel and a root filesystem**
  as release assets, which is what a microvm consumes.

⛔ **That is a shape question for `docker-bsd`, not a defect in it.** An OCI
rootfs is the right artefact for FreeBSD, where a runtime exists. For the other
three it is currently a container image with no container to run it in.

### What was NOT done

- ⛔ **No BSD was run.** Not one. Every runtime above was established from its
  own repository, its releases and its tracker, and none was executed here.
- ⚠ **`ocijail` and `runj` were read, not built.** Their release tags and their
  documented spec coverage are the evidence; neither was compiled.
- ⚠ **"None found" for OpenBSD and DragonFly is the state of one search.** It is
  a negative result over 28 references, not a proof that nothing exists.

### ⭐ 2026-08-27, second session: a BSD userland runs on this Windows host

⛔ **Nothing above is edited.** Every premise this section disproves keeps its
wording and gets the correction underneath it, per
[`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md).

**What changed.** ⛔ **No code in this repository.** The work is seven
experiments in `pkgforge-dev/docker-bsd` under `experiments/`, each committed
with its result, and the corrections here and in
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md).

---

#### ⭐ The answer, and it is measured

```text
qemu-system-x86_64 -accel whpx -M q35 -cpu Icelake-Server-v7 -smp 2 -m 2048
  -drive if=none,file=FreeBSD-15.1-RELEASE-amd64-BASIC-CI-ufs.raw,format=raw,id=root0
  -device virtio-blk-pci,drive=root0 -nic none
  -display none -no-reboot -serial stdio
  -rtc base=utc,clock=host,driftfix=slew
```

Run on this machine, unelevated, with the WSL2 podman machine running
throughout. What the guest answered, read back off its own console by running
commands in it rather than by scraping a boot log:

```text
FreeBSD freebsd 15.1-RELEASE FreeBSD 15.1-RELEASE releng/15.1-n283562 GENERIC amd64
15.1-RELEASE
Intel Xeon Processor (Icelake)
hv
BSD userland is running as root on FreeBSD
```

| the ask | delivered |
| --- | --- |
| a BSD userland from Windows | ⭐ FreeBSD 15.1-RELEASE, GENERIC |
| least friction | one fetch script, one boot script. ⚠ 666 MB, verified against the published `CHECKSUM.SHA256` |
| ⛔ **not** `wsl` inside `linux` inside `qemu` inside `bsd` | ⭐ **no nesting at all.** One hypervisor, the host's own, through WHPX |
| ⚠ and unstated, but it turned out to matter | ⭐ **no elevation** |

⭐ **That is the ruling's second half satisfied.** Nesting was to be the floor,
not the target, and something better than nesting was wanted. This is one level
deep on the machine's own hypervisor, which is strictly better than the nested
design, and it needs no administrator.

---

#### ⭐ The order of work in the ruling was right, and the first step is why

⚠ **The first experiment failed and it is the reason the third one worked.**
The ruling said to try a smolBSD rescue image under `-accel whpx` first. It
booted a NetBSD kernel and never found its disk, on every CPU model tried. The
control run under `-accel tcg`, identical in every other respect, booted to a
shell. That contrast located the cause, and the cause is one line of FreeBSD's
own boot output:

```text
Hypervisor: Origin = "Microsoft Hv"
```

⛔ **Under WHPX the guest sees the HOST's hypervisor signature, not QEMU's.**
NetBSD's paravirtual bus is looking for QEMU, does not find it, and concludes
it is on bare metal, so `qemufwcfg` is never probed and virtio-mmio is never
enumerated. FreeBSD has Hyper-V support and simply carries on.

⭐ **So the failure was not "QEMU under WHPX does not work". It was "this
guest reaches its disk only through a bus that WHPX does not present".** A
kernel with ordinary PCI drivers has no such dependency, which is the whole
change between the experiment that stopped short and the one that worked: same
accelerator, same machine type, same CPU model, different guest.

---

#### ⛔ Correction: the WHPX CPU-model prediction was wrong for this machine

Section 2 of the reference sweep above says `-cpu host` and `-cpu max` wedge
QEMU under WHPX, that a named model newer than the host wedges it too, and that
⛔ **"this machine reports model 154, so it falls into 'cannot place' and would
be offered `GraniteRapids-v2`"**, which is the wedging direction. It labels
that **derived, not measured**, and asks for the measurement.

⭐ **Measured. Nothing wedged, including the two the advice forbids.**

| accel | `-cpu` | verdict | qemu CPU seconds at a 35 s timeout |
| --- | --- | --- | --- |
| whpx | `Icelake-Server-v7` | kernel ran, no disk | 15.41 |
| whpx | `kvm64-v1` | kernel ran, no disk | 15.73 |
| whpx | `qemu64` | kernel ran, no disk | 14.52 |
| whpx | ⛔ `host` | kernel ran, no disk | 15.34 |
| whpx | ⛔ `max` | kernel ran, no disk | 15.44 |
| tcg | `qemu64` | ⭐ booted to a shell | 3.98 |

⚠ **This does not falsify the sources.** `R18` and `R7` measured QEMU 9.x, on a
Zen 5 AMD part and on GitHub's Windows runner fleets. ⛔ **It falsifies the
prediction this repository wrote about this host**, on QEMU 11.1.0 on an
i7-12700H. No zero-byte serial log ever appeared.

⭐ **Prefer a named model anyway.** It costs nothing, and the failure it avoids
is a twelve-minute hang with no output. `Icelake-Server-v7` is what the working
recipe uses.

---

#### ⛔ Correction: the untried avenue is now tried, and it is closed

The sweep calls the Host Compute System **"the untried avenue with the lowest
cost"** and says reaching it "needs no WSL patch at all". ⭐ **The API half is
correct.** `computecore.dll` loads in an ordinary unelevated process and every
HCS v2 entry point wanted resolves.

⛔ **The privilege half closes it.** `HcsEnumerateComputeSystems`, which is a
**read**, returns `0x8037011B`:

> Insufficient privileges. Only administrators or users that are members of the
> Hyper-V Administrators user group are permitted to access virtual machines or
> containers.

`Get-VM` is refused identically. If reading is privileged, creating is.

⚠ **And even with elevation it is the worse option**, which the sweep could not
have known. HCS yields a virtual machine with **no way to talk to it**: no
serial pipe of its own, and WSL reaches its own guest over `AF_HYPERV` sockets
whose guest half is the 819 lines of C this repository refused to adopt. QEMU,
on the very same hypervisor, yields a serial console on an ordinary pipe and
needs no elevation.

---

#### ⚠ Correction: the recommended option now has a cost the table does not show

The Approach table above rates the **Hyper-V `.vhd` guest** as ⭐ lowest
friction and rates `qemu -accel whpx` as ⚠ **"fallback. Worth it only if
Hyper-V is unavailable or unwanted."**

⛔ **On this machine the ranking inverts, and the reason is not performance.**
Experiment 32 measured that Hyper-V and the Host Compute System both refuse an
unelevated caller here. The WHPX route needs no elevation at all. ⚠ **So the
`.vhd` route's true friction includes an administrator prompt, and the
"fallback" is the only one of the two this session could run.**

⚠ **The `.vhd` route was still not re-checked**, and this does not claim it
fails. It claims its cost was understated and the fallback's overstated.

---

#### ⛔ Correction: "no BSD was booted" no longer holds

[`SUMMARY.md`](SUMMARY.md) and the sweep both record, correctly at the time,
that **no BSD was booted, on any host, and every boot time quoted was somebody
else's.** ⭐ **Four BSD kernels have now been started here, three of them
reaching a userland**, and the numbers below are this machine's own:

| what | where | measured |
| --- | --- | --- |
| NetBSD 11.0, smolBSD rescue | QEMU `-accel tcg`, Windows | ⭐ a shell. 499 ms of kernel boot, by the kernel's own clock |
| NetBSD 11.0, smolBSD rescue | QEMU `-accel whpx`, Windows | ⛔ **kernel only.** No disk, so no userland |
| FreeBSD 15.1, ⚠ `acj`'s patched FIRECRACKER kernel | Firecracker on the nested KVM in the podman machine | ⭐ `login:` in **1.8 s**, shell over SSH at 32.3 s |
| FreeBSD 15.1-RELEASE, ⭐ stock GENERIC | ⭐ QEMU `-accel whpx`, Windows, no nesting | `login:` at **117.7 s**, **117.4 s** and **113.6 s**, three independent boots |

⚠ **The two FreeBSD rows are different kernels and different root filesystems**,
so the times rank the whole stack rather than the two hypervisors. The
correction below says what the phase table can and cannot support.

---

#### ⛔ Correction: the boot time was attributed to `growfs`, and that was a guess

⚠ **This session's own first write-up said the 117 s was `growfs` on first
boot. It is not**, and the correction is here rather than as an edit because
the same rule applies to a premise this session wrote an hour ago. The console
reports the root filesystem `FILE SYSTEM CLEAN; SKIPPING CHECKS`, carries no
`growfs` line at all, and a second boot landed within 0.3 s of the first.

⭐ **The experiment now stamps four boot phases, so the cost is located rather
than attributed.** Measured with `-nic none`:

| phase, from the QEMU process starting | at | cost of this phase |
| --- | --- | --- |
| loader hands off to the kernel | 4.3 s | 4.3 s |
| kernel banner | 4.3 s | 0 s |
| ⛔ **root mounted** | **112.5 s** | ⛔ **108.2 s** |
| `rc` starts | 112.8 s | 0.3 s |
| login prompt | 113.6 s | 0.8 s |

⛔ **108 of the 114 seconds are between the kernel banner and mounting root**,
which is device probing. ⚠ Not the loader, not `rc`, not the filesystem, and
not the network: removing the NIC entirely changed the total by under four
seconds.

⚠ **What that costs, stated against the alternative, and stated carefully.**
FreeBSD 15.1 under Firecracker on the nested KVM reaches a login prompt in
**1.8 s**, against 113.6 s here. ⛔ **That is not a like-for-like comparison of
two hypervisors and must not be quoted as one.** The Firecracker guest is
`acj`'s **patched FIRECRACKER kernel** and a purpose-built root filesystem with
almost no devices to probe; this one is stock **GENERIC** on an emulated q35
with a full PCI and ISA bus behind it. ⚠ Two variables moved at once, and
nothing here separates them.

⭐ **What can be said from the phase table alone**, with no second guest
involved: the cost is device probing, it is 108 s, and it is 95 percent of the
boot. Whether a leaner guest under WHPX would pay it is the obvious next
measurement and was not taken.

⛔ **Steady-state performance was NOT measured.** A slow device probe suggests
WHPX handles the exits behind port and memory-mapped IO expensively, and
whether that follows the guest into ordinary work is a different question that
nothing here answers. ⚠ Do not read the boot number as a throughput number.

---

#### ⭐ A container runs inside that guest, which is the gesture this entry opens with

⛔ **The entry's `Problem` is one line**, and it is now runnable end to end on
this machine. Inside the FreeBSD guest, on the Windows host's own hypervisor:

```text
$ podman run --rm ghcr.io/freebsd/freebsd-runtime:15.1 /bin/sh -c 'uname -sr; echo ...'
rc=0
Trying to pull ghcr.io/freebsd/freebsd-runtime:15.1...
Getting image source signatures
Copying blob sha256:78d645ce98ae...
Writing manifest to image destination
FreeBSD 15.1-RELEASE
```

⭐ **`podman info` reports `freebsd/amd64 runtime=ocijail`**, so the runtime
underneath is jails, exactly as the handbook and `BSD-02` describe. ⚠ **The
image is FreeBSD's own published one**, consumed rather than rebuilt, which is
what `pkgforge-dev/docker-bsd`'s README requires.

⚠ **Three corrections were needed and each looked like a different problem than
it was.** Two are podman's own defaults; the third is the interesting one.

---

#### ⭐ The finding this session would keep if it could keep only one

⛔ **Under WHPX, FreeBSD selects a clock that does not work, and every Go
binary in the guest dies of it.**

It follows directly from the `Microsoft Hv` result above. The guest sees the
host's hypervisor signature, so FreeBSD offers and then trusts a paravirtual
timecounter that QEMU is only pretending to provide:

```text
TSC-low(-100) i8254(0) ACPI-fast(900) HPET(950) Hyper-V-TSC(3000) Hyper-V(2000)
was: Hyper-V-TSC
```

Go's garbage collector divides by a rate derived from the monotonic clock, so
every Go binary, podman included, dies before it does any work:

```text
SIGFPE: floating-point exception
runtime.deductSweepCredit(0x2000, 0x0)
        /usr/local/go125/src/runtime/mgcsweep.go:948
```

⭐ **One sysctl, and nothing in that stack trace would ever point at a clock:**

```bash
sysctl kern.timecounter.hardware=ACPI-fast
```

⚠ **Honest limit, and it is the half that gets dropped when this is retold.**
⛔ **This improved the failure; it did not abolish it.** With `ACPI-fast`
selected, `podman run` pulls its image and runs a container correctly with
`rc=0`. In the **same run**, a separate `podman pull` still died, with
`fatal error: releasep: invalid p state` rather than `SIGFPE`. ⚠ So the Go
runtime is still not entirely happy on this guest and the remaining fault is
intermittent. Two datapoints are not a diagnosis, and this is recorded as an
open behaviour rather than a solved one.

⚠ **A second observation, one occurrence, cause unknown.** FreeBSD 15.1 GENERIC
page-faulted **in the kernel** during `rc.shutdown`, in `vget_finish`, on the
boot that had run podman. ⛔ The boots that did not run podman shut down
cleanly. Recorded so it is not a surprise; not attributed to anything.

---

#### ⛔ Correction to the section immediately above, from a later measurement

⚠ **The section above is this session's own, written an hour before this one,
and it keeps its wording** because the rule applies to a premise however recently
it was written. ⛔ **It said the clock was the cause. That is not supported.**

⭐ **Everything it MEASURED still holds.** The guest does see `Microsoft Hv`.
FreeBSD does select `Hyper-V-TSC` at quality 3000. Go binaries do die with
`SIGFPE` in the garbage collector. Setting `ACPI-fast` did change `podman run`
from failing to returning `rc=0`. All four were run.

⛔ **What does not hold is the "therefore".** Two later measurements:

1. ⭐ **With `ACPI-fast` selected the clock is demonstrably correct.** Two reads
   of `date +%s%N` a second apart give `delta_ns=1002101384`, which is 1.0021
   seconds for a 1 second sleep. ⛔ A clock that good does not make anything
   divide by zero.
2. ⛔ **A long-running Go daemon does something worse than `SIGFPE`: it takes
   the guest KERNEL down.** `podman system service` panics FreeBSD, with
   `ACPI-fast` already selected:

```text
Fatal trap 12: page fault while in kernel mode
current process        = 1546 (podman)
#5 0xffffffff80bade73 at do_wait+0x123
#6 0xffffffff80bab814 at __umtx_op_wait_uint_private+0x54
#7 0xffffffff80ba8f9e at sys__umtx_op+0x7e
```

`_umtx_op` is FreeBSD's userspace-mutex syscall, and it is what Go's scheduler
parks threads on.

⚠ **So the honest reading is that something about this guest under WHPX is
wrong at a level below the timecounter.** The timecounter change moved the
symptom, twice, reproducibly; it did not explain it. ⛔ **Calling it the cause
would be a story rather than a measurement**, and this repository has a table
row for a number on a report that was not measured.

⚠ **And the shutdown page fault now has company.** FreeBSD page-faulted in the
kernel at `vget_finish` during `rc.shutdown` on one boot, and at
`do_wait`/`_umtx_op` on another. ⛔ **Both were boots that ran a multithreaded
Go program; the boots that did not shut down cleanly.** Two kernel faults at
different sites under the same condition is a pattern, and it is not diagnosed
here.

⭐ **What this changes for anyone acting on this entry.** A BSD userland on
WHPX is real, reproducible and useful for interactive work. ⛔ **A long-running
multithreaded Go service in that guest is not, today**, and that is the whole
distance between this entry's purpose and its acceptance command.

---

### ⚠ Where this entry stands, 2026-08-27

⛔ **The purpose is met and the acceptance is not, and those are different
sentences.** Both are stated here so nobody has to infer which.

**The `Problem` this entry opens with:**

```bash
podman run --rm -it "example.io/freebsd" -sh
```

> from Windows, into a real BSD, without `wsl` inside `linux` inside `qemu`
> inside `bsd`.

| the clause | state |
| --- | --- |
| a real BSD | ⭐ **met.** FreeBSD 15.1-RELEASE, stock GENERIC, answering commands |
| from Windows | ⭐ **met.** On the host's own hypervisor through WHPX |
| ⛔ without the nesting | ⭐ **met.** One hypervisor. And unelevated, which was not even asked for |
| `podman run`, in the guest | ⭐ **met.** `rc=0`, `runtime=ocijail`, the container's own stdout read back |
| `podman run`, **from the Windows client** | ⛔ **NOT met.** This is the `Prove` clause, and it is what keeps the entry open |

**The `Prove` clause, unchanged:**

```bash
podman -c freebsd run --rm IMAGE /bin/sh -c 'uname -sr'
```

⚠ **Everything underneath that command works and the command does not.**
`41-connect-podman-from-windows.ps1` in `pkgforge-dev/docker-bsd` gets further
than the summary above suggests, and it is worth being precise about how far,
because the next session should not redo it:

| step | state |
| --- | --- |
| a throwaway key installed over the serial console | ⭐ works |
| ⛔ empty-password ssh closed **before** the port is forwarded | ⭐ works. `PermitEmptyPasswords no`, `PasswordAuthentication no`, `PermitRootLogin prohibit-password`, read back from the file |
| the port forwarded, bound to `127.0.0.1` only | ⭐ works |
| ssh from Windows into the guest | ⭐ **works.** The client authenticates |
| the podman API socket existing | ⭐ works, at `/var/run/podman/podman.sock` |
| ⛔ **the podman API service staying up** | ⛔ **this is the blocker** |
| `podman system connection add` | ⭐ works, exit 0 |
| `podman -c freebsd run ...` | ⛔ `ssh: rejected: connect failed (open failed)`, because there is nothing behind the socket |

⛔ **The blocker, stated exactly.** `podman system service` is a long-running Go
daemon, and it dies of `SIGFPE` inside the Go runtime seconds after starting,
leaving the socket file behind with nothing listening. ⭐ **It is the same fault
as the clock finding above**, and the correction that is enough for a
short-lived `podman run` is **not** enough for a daemon: more GC cycles, more
chances to divide by zero.

⚠ **So the honest shape of what is left is one question, not one command:**
can a Go daemon be kept alive in a FreeBSD guest under WHPX? ⭐ Three things
worth trying, in order, and none was tried here:

1. **A different timecounter, chosen for the daemon rather than for a
   command.** `HPET`, `TSC-low` and `i8254` all exist in the guest and only
   `ACPI-fast` was used.
2. **Set it at boot rather than at runtime**, through `/etc/sysctl.conf` or a
   loader tunable, so no Go binary ever runs while `Hyper-V-TSC` is selected.
3. ⚠ **Or sidestep it: the client does not need the daemon in that guest.**
   A podman connection is only an SSH URI to a socket, so a `podman system
   service` running anywhere reachable satisfies the acceptance. That is a
   weaker result and it should be labelled as one if it is taken.

⛔ **Status stays `open`.** ⚠ **Not because the work failed**: the goal the
operator stated, a BSD that boots on this machine, is reached, measured and
reproducible. It stays open because this entry's acceptance names a command
that has not returned 0, and closing an entry on a command that was never run
is exactly the "fake anything" class this repository has a table for.
