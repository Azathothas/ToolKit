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

---

## BSD-02. Whether the other three BSDs can be *run*, not merely built

**Source** Derived from BSD-01 and the `docker-bsd` work.
**Category** bsd · **Priority** P3 · **Effort** S · **Status** open

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
