# SUMMARY.md

⭐ **The last session's summary table, saved as well as printed**, per
[`../docs/methodology/sessions.md`](../docs/methodology/sessions.md). It is the
fastest orientation into what the last session actually did.
[`PROGRESS.md`](PROGRESS.md) is what to read next, and it is the authority.

⛔ Overwritten each session. It is a snapshot, not a log.

---

## 2026-08-27, the BSD reference sweep

| row | before | after |
| --- | --- | --- |
| **Elapsed** | probe at 2026-08-27T11:54:27Z | 2026-08-27T12:37:50Z, **43m** |
| **Commits** | `9503a23` | **2 here**, plus **1** in `pkgforge-dev/docker-bsd` (`878e286`) |
| **Work** | 1 assigned: mine the references, correct the entries, prepare `docker-bsd` | ⭐ **completed, 0 deferred, 0 failed.** Plus `BSD-02` closed unassigned, and an unregistered consumer found and recorded |
| **Changes** | | 8 files here, **+1,495 / -131**. 6 files in `docker-bsd`, **+627**. ⛔ No code changed in either |
| **Size** | 18,090 lines, 75 files | 19,454 lines, 75 files, **+1,364**. ⚠ Measured before this table's own final edit, so both figures are short by a few lines. No file added or removed here |
| **Checks** | `check-gate --fast`: 12 passed, 1 skipped | ⭐ same, run twice. ⚠ The **full** gate was not run in this session, so `check-twins` was skipped at both ends and this row has no 13-of-13 behind it |
| **Cost** | | no money. ⚠ **No BSD was booted and nothing was built.** ⛔ The API call count is **not given**: the hourly limit reset mid-session, so nothing measured it |
| **Health** | 18 entries, 2 open, 16 done | 18 entries, **1 open**, 17 done. Both trees clean. CI green: three jobs at `796e40f`, two jobs at `878e286` |

### ⛔ CAUTION: this session did not finish the gate

⛔ **Part (c) of [`../docs/methodology/gate.md`](../docs/methodology/gate.md),
the three deep reviews, was NOT run.** The session ran out of budget and the
operator ended it deliberately. Parts (a) and (b) were run: the local gate
twice, and both probes driven on all three hosts.

⚠ **So treat the sweep as one pass, not as reviewed work.** Specifically:

- ⭐ **The measurements taken on this machine are real** and are labelled as
  such: the WHPX capability, the absent tooling, the host CPU identity, the
  re-derived nested KVM facts, and the two probe defects found by running them.
- ⛔ **Everything attributed to another repository is one reading of it.**
  Verdicts, rankings, quoted numbers and the Pre/Post/Misc classification were
  reviewed twice for classification only, not audited for accuracy. ⚠ Some of
  it may be wrong. Re-derive any claim before building on it, which
  [`../docs/security/remote-ops.md`](../docs/security/remote-ops.md) requires
  anyway.
- ⚠ **The per-BSD table that closed `BSD-02` is the same kind of claim.** It
  closed because its acceptance was a written answer with its evidence, and the
  evidence is cited. It did not close because anything was run.

### What moved

| | |
| --- | --- |
| ⭐ the sweep | **28 repositories** from the operator's 23 rows, one of which was an organisation query. Every one reached, none gone. `R6` to `R28`, classified, reviewed twice, and ranked |
| ⭐ `BSD-02` | **closed.** Its acceptance was a written answer per BSD; that is what the sweep produced |
| ⭐ `BSD-01` | six corrections underneath its table, and a changed order of work. It keeps every word it had |
| ⭐ `docker-bsd` | an `experiments/` layout with two probes that were run, its own ignored `.tmp/`, and `TOOLKIT.md` |
| ⛔ `consumers.md` | a **second consumer**, unregistered until now, carrying a vendored copy with both closed P0s in it |

### ⭐ The five findings that change what the next session does

1. ⛔ **`x86_64` GitHub runners have `/dev/kvm` and arm64 runners do not.** Four
   references agree. `BSD-01` called this its single highest-value unknown.
2. ⛔ **Under WHPX, `-cpu host` and `-cpu max` wedge QEMU**, and so does a named
   model newer than the host. Two projects measured it independently.
3. ⭐ **A BSD microvm is the option the table did not have**: about 10 ms for
   NetBSD through PVH, about 12 s for FreeBSD under Firecracker, with published
   kernels and root filesystems for both.
4. ⭐ **WSL's own host will drive a non-Linux guest**, and the guest half is 819
   lines of C over Hyper-V sockets. ⛔ Refused: the cost is a rebuilt
   `wslservice.exe`, which runs this machine's podman machine. ⭐ What survives
   is that the Host Compute System takes a JSON document and boots an arbitrary
   UEFI disk, needing no patch at all.
5. ⛔ **A reverse Linuxulator exists and this repository said it did not.**

### Debts introduced, named rather than left

- ⛔ **`pkgforge-dev/cross-libc-dlopen` carries `wsl-ephemeral.ps1` with
  `WSL-01` and `WSL-12` in it.** Not fixed: that tree is read-only to this one.
  Recorded in [`../docs/consumers.md`](../docs/consumers.md).
- ⚠ **`docker-bsd` publishes a root filesystem that three of its four BSDs have
  nothing to run.** Whether it should publish something bootable instead is an
  open question in [`PROGRESS.md`](PROGRESS.md), deliberately not decided here.

### What was NOT measured, so it is not claimed

- ⛔ **No BSD was booted.** Not one, on any host. Every boot time in the sweep
  is somebody else's, attributed where it appears.
- ⛔ **No QEMU exists on either side of this machine**, so the WHPX CPU-model
  prediction for this host's Model 154 CPU is **derived** from another project's
  rule and its measurements. It is not a measurement of this machine.
- ⚠ **The `/dev/kvm` answer for CI runners is sourced, not measured.** This
  session has no runner to probe.
- ⚠ **`oras` is absent here**, so the zero-byte pull on Windows is a report from
  smolBSD's tracker and was not reproduced.
- ⚠ **The gate timings in [`PROGRESS.md`](PROGRESS.md), 41s and 208s, were not
  re-timed** in this session and are carried from the one that measured them.
