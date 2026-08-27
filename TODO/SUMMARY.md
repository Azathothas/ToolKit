# SUMMARY.md

⭐ **The last session's summary table, saved as well as printed**, per
[`../docs/methodology/sessions.md`](../docs/methodology/sessions.md). It is the
fastest orientation into what the last session actually did.
[`PROGRESS.md`](PROGRESS.md) is what to read next, and it is the authority.

⛔ Overwritten each session. It is a snapshot, not a log.

---

## 2026-08-27, the session that booted a BSD

| row | before | after |
| --- | --- | --- |
| **Elapsed** | probe at 2026-08-27T12:54:52Z | 2026-08-27T14:25Z, about **90m** |
| **Commits** | `a9171be` here, `878e286` in `pkgforge-dev/docker-bsd` | **1 here**, **2** in `docker-bsd` (`d7e6184`, `f2bd4a3`), both pushed |
| **Work** | 1 assigned: reach a BSD userland from Windows, trying the ranked avenues in order | ⭐ **the goal is reached.** ⚠ `BSD-01` stays **open** on its own acceptance, and PROGRESS says exactly what is left |
| **Changes** | | ⛔ **no code changed here.** 6 files, **+987 / -200**, docs and record only. 10 files in `docker-bsd`, **+2,382 / -26**, of which **8 are new experiments** and one a shared console library |
| **Size** | 19,454 lines, 75 files | **20,241 lines**, 75 files, **+787**. ⚠ Measured before this table's own final edit, so it is short by a few lines. No file added or removed here |
| **Checks** | `check-gate --fast` 12 passed 1 skipped, carried at 41s | ⭐ same, **re-timed at 49.8s**, plus the **full** gate with `check-twins`. `docker-bsd` `tests/run.sh` 27 passed 0 failed |
| **Cost** | | no money. ⚠ **about 1.0 GB downloaded**: QEMU 197 MB, FreeBSD BASIC-CI 666 MB, `acj`'s Firecracker set about 130 MB, smolBSD 13.6 MB, plus `podman-suite` inside a guest, which was not measured separately. ⚠ **Disk left behind: 6.8 GB** on `C:` and **603 MB** inside the podman machine, both in ignored scratch |
| **Health** | 18 entries, 1 open, 17 done | 18 entries, **1 open**, 17 done, unchanged. Both trees clean and pushed. ⚠ CI: three jobs green at `a9171be` and two at `d7e6184`; the runs for this session's own commits are named in the closing report rather than guessed at here |

### ⭐ What the session was for, and whether it did it

⛔ **The ask was a BSD that boots on this machine, not a plan for one.**

```text
FreeBSD freebsd 15.1-RELEASE FreeBSD 15.1-RELEASE releng/15.1-n283562 GENERIC amd64
BSD userland is running as root on FreeBSD
```

⭐ **On the Windows host's own hypervisor, through `qemu -accel whpx`, with no
nesting and no elevation.** That is the operator's ruling satisfied: nesting was
to be the floor rather than the target, and this is better than the nested
design on both counts the ruling named.

⭐ **And a container runs inside it**, `rc=0`, with `podman info` reporting
`freebsd/amd64 runtime=ocijail`.

### ⭐ The five findings that change what the next session does

1. ⭐ **WHPX presents the HOST's hypervisor identity to the guest**, not QEMU's.
   `Hypervisor: Origin = "Microsoft Hv"`. Everything else here follows from
   that one line.
2. ⛔ **Go binaries die in that guest, and the tidy explanation is wrong.**
   FreeBSD picks its Hyper-V timecounter at quality 3000, every Go binary dies
   of `SIGFPE` in the garbage collector, and
   `sysctl kern.timecounter.hardware=ACPI-fast` moves `podman run` to `rc=0`.
   ⛔ **But the clock then measurably works** (`delta_ns=1002101384` across a
   one-second sleep) and a long-running Go daemon **panics the guest kernel** in
   `_umtx_op`. ⚠ The timecounter change moved the symptom; it did not explain
   it. This session published the tidy version first and corrected it.
3. ⭐ **And NetBSD's paravirtual bus never attaches**, so smolBSD boots a kernel
   under WHPX and never finds its disk. The same image under `-accel tcg` boots
   to a shell in 499 ms of kernel time.
4. ⛔ **`-cpu host` and `-cpu max` did NOT wedge QEMU here**, against published
   advice and against this repository's own prediction for a Model 154 host.
   All five models behaved identically on QEMU 11.1.0.
5. ⛔ **The Host Compute System is reachable and closed.** `computecore.dll`
   binds unelevated, and `HcsEnumerateComputeSystems`, a **read**, returns
   `0x8037011B`: Hyper-V Administrators only.

### ⛔ Six defects this session shipped and then caught

⚠ **Every one is a class this repository already names, and the reviews caught
what running did not.** Five new rows in
[`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md).

- ⛔ **A false success.** An experiment printed "a container ran" over a
  `podman run` that had exited with an error, because its marker matched the
  guest's **echo of the command line that mentioned the marker**.
- ⛔ `ssh` piped into `sed` with `$?` read after, which reads `sed`'s status.
- ⛔ `curl` and `xz` guarded by `cmd; rc=$?` under `set -e`, where the shell has
  already exited before the guard can run.
- ⛔ A probe reporting `vmcompute.dll did not load` about a library that had
  loaded, because a `try`/`catch` cannot tell a missing library from a missing
  entry point.
- ⛔ `network NONE` printed while the guest ran `dhclient`: QEMU attaches a
  default NIC unless given `-nic none`.
- ⛔ A boot time attributed to `growfs` that `growfs` had nothing to do with.
  Replaced with four measured boot phases, which located it: **108 s of 114 s
  is device probing**.

### ⚠ What was NOT measured, so it is not claimed

- ⛔ **Steady-state performance under WHPX.** Only boot time was measured. A
  slow device probe hints that IO exits are expensive, and nothing here tested
  whether that follows the guest into ordinary work.
- ⛔ **The two FreeBSD boot times are not a hypervisor comparison.** 1.8 s under
  Firecracker is a patched minimal kernel with almost nothing to probe; 113.6 s
  under WHPX is stock GENERIC on an emulated q35. Two variables moved at once.
- ⚠ **The Hyper-V `.vhd` route was still not built or re-checked.** What is now
  known is that it needs elevation the WHPX route does not.
- ⚠ **Every number is one machine.** The CPU-model result in particular
  disagrees with two published reports taken on other hardware and older QEMU.
- ⛔ **The clock fix improved the Go failure; it did not abolish it.** A
  separate `podman pull` still died in the same run, with a different fatal
  error. The remaining fault is intermittent and is not diagnosed.
