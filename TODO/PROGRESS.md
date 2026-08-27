# PROGRESS.md

⭐ **The one file every session reads first.** Where the work is, what is next,
and why. [`INDEX.md`](INDEX.md) is the list of entries; the **order lives here
and nowhere else**. [`SUMMARY.md`](SUMMARY.md) is the last session's table, and
it is a snapshot rather than an authority.

⛔ Rewritten every session. It carries no history: the history is the git log
and the entries themselves. Do not add a "previous sessions" section.

⛔ Edited in the same change as the work, never as a report afterwards.

---

## State

```text
session started 2026-08-27T12:54:52Z
baseline        ci green on all three jobs at a9171be, tree clean
entries         total 18  open 1  blocked 0  done 17
```

⚠ The counts above are checked against
[`INDEX.md`](INDEX.md)'s rows by `scripts/common/check-record.sh`, which runs as
a gate. ⛔ Do not edit them by hand to make a check pass; fix whichever file is
wrong. ⭐ `scripts/common/set-record.mjs` moves them for you.

| fact | value |
| --- | --- |
| repository | `Azathothas/ToolKit`, public, 0BSD |
| work model | todo. [`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md) |
| push policy | commit and push, to this remote only |
| CI | three jobs, ubuntu and windows. `.github/workflows/ci.yml` |
| `main` | protected, admin bypass on. Force push and deletion refused. |
| the local gate | ⭐ `sh scripts/common/check-gate.sh --fast`. **49.8s measured this session**, against the 41s carried since. |

**Origin.** Bootstrapped from `Azathothas/TEMPLATE` on 2026-08-27. Its first
content was `wsl-ephemeral.ps1`, decoupled from that template so the template
keeps only what every project needs. That template carries a wrapper that
fetches this copy by pinned commit and verified digest.
[`../docs/consumers.md`](../docs/consumers.md) is the state.

---

## ⭐ The headline: a BSD userland runs on this machine, with no nesting

⛔ **Measured, not derived.** Unelevated, with the WSL2 podman machine running
throughout:

```text
qemu-system-x86_64 -accel whpx -M q35 -cpu Icelake-Server-v7 -smp 2 -m 2048
  -drive if=none,file=FreeBSD-15.1-RELEASE-amd64-BASIC-CI-ufs.raw,format=raw,id=root0
  -device virtio-blk-pci,drive=root0 -nic none
  -display none -no-reboot -serial stdio
```

```text
FreeBSD freebsd 15.1-RELEASE FreeBSD 15.1-RELEASE releng/15.1-n283562 GENERIC amd64
BSD userland is running as root on FreeBSD
```

⭐ **One hypervisor, the host's own.** That is the operator's ruling satisfied:
nesting was to be the floor, not the target, and this is better than the nested
design and needs no administrator.

---

## The measured baseline

⚠ **This session ran code on three hosts**: the Windows host, the WSL2 podman
machine, and four BSD guests. Everything below was run in this session.

| gate | result |
| --- | --- |
| `check-gate.sh --fast` | ✅ 12 passed, 1 skipped (`check-twins`), **49.8s** |
| ⭐ `check-gate.sh`, the **full** gate | ✅ **all 13 checks passed**, `check-twins` included. ⚠ Run before this file's own last two edits, which are prose; no script in this repository changed at all this session |
| `check-docs.sh` | ✅ 41 files, 269 relative links, 93 shell blocks |
| `check-record.sh` | ✅ counts agree with rows |
| `check-changelog.sh` | ✅ in order, each dated with a record and a deploy line |
| CI, all three jobs | ✅ success at `a9171be`, the session baseline |
| `pkgforge-dev/docker-bsd` CI | ✅ both jobs at `d7e6184` **and** `f2bd4a3` |
| `docker-bsd` `sh tests/run.sh` | ✅ 27 passed, 0 failed |

### ⭐ Measured on this machine this session

| probe | result |
| --- | --- |
| `qemu-system-x86_64` on Windows | ⭐ **installed**, 11.1.0, by `scoop install qemu`. It was absent at the start of this session |
| `-accel help` | `tcg` and ⭐ `whpx`. `microvm` and `q35` both present |
| FreeBSD 15.1 GENERIC under `-accel whpx` | ⭐ **a userland.** `login:` at 117.7s, 117.4s and 113.6s over three boots |
| ⛔ where that time goes | ⛔ **108.2s of 113.6s is device probing**, between the kernel banner and mounting root. Not the loader, not `rc`, not the filesystem, not the network |
| FreeBSD 15.1 under Firecracker, nested KVM | ⭐ `login:` in **1.8s**, shell over SSH at 32.3s |
| ⛔ smolBSD NetBSD under `-accel whpx` | ⛔ **kernel only.** No paravirtual bus, so no disk |
| ⭐ the same under `-accel tcg` | ⭐ **a shell**, 499ms kernel boot |
| ⛔ `-cpu host` and `-cpu max` under WHPX | ⛔ **did NOT wedge.** All five models behaved identically. The published rule does not reproduce here |
| Host Compute System, `computecore.dll` | ⭐ loads unelevated, all seven HCS v2 entry points resolve |
| `HcsEnumerateComputeSystems`, a **read** | ⛔ `0x8037011B`, Hyper-V Administrators only |

---

## What this session did

**2026-08-27, the second BSD session. ⛔ No code changed in this repository.**

The work is **eight experiments** in `pkgforge-dev/docker-bsd` under
`experiments/`, each committed with its result, plus the corrections here.

- ⭐ **A FreeBSD userland was reached on the Windows host's own hypervisor**,
  with no nesting and no elevation, and it answers commands.
- ⭐ **The ruling's order of work was right, and the experiment that FAILED is
  why the one that worked, worked.** smolBSD under WHPX located the cause;
  FreeBSD then printed it in one line: `Hypervisor: Origin = "Microsoft Hv"`.
- ⛔ **Five premises corrected**, each with its wording kept and the correction
  written underneath: the WHPX CPU-model prediction, the untried HCS avenue, the
  relative friction of the Hyper-V route against the WHPX one, "no BSD was
  booted", and a boot time attributed to `growfs`. ⚠ **A sixth correction is of
  this session's own text**, written an hour earlier: the clock explanation.
- ⛔ **Five rows added to
  [`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md)**,
  every one from a defect in this session's own scripts.

### ⛔ The defects this session shipped and then caught

⚠ **Recorded because the reviews caught what running did not.** Each is a class
the repository already names, and one of them printed a false success.

- ⛔ **An experiment reported "a container ran" over a `podman run` that had
  errored.** Its success marker was matched against the guest's **echo of the
  command line that mentioned the marker**, which survived the echo filter
  because the tty had line-wrapped it. ⭐ Fixed twice over: the marker is now
  split so it cannot appear literally in the command, and the echo filter
  compares with whitespace removed.
- ⛔ **`ssh` piped into `sed`, with `$?` read afterwards**, which reads `sed`'s
  status. A failed `ssh` would have read as green.
- ⛔ **`curl` and `xz` guarded by `cmd; rc=$?` under `set -e`**, where the shell
  has already exited before the guard can run. Both guards were decoration.
- ⛔ **A probe reporting `vmcompute.dll did not load` about a library that had
  loaded** and exports 36 functions. A `try`/`catch` around a P/Invoke cannot
  tell a missing library from a missing entry point.
- ⛔ **`network NONE` printed while the guest ran `dhclient`.** QEMU attaches a
  default NIC unless given `-nic none`, and `-display none` says nothing about
  the network. No inbound door was opened, and the header was still false.
- ⛔ **A boot time attributed to `growfs` that `growfs` had nothing to do with.**
  Corrected by making the experiment stamp four boot phases instead.

---

## ⭐ The work order

**1. `BSD-01`, still the only open entry, and the goal it was written around is
met.** ⛔ **What is left is its acceptance command, not its purpose.**

The entry's `Prove` names one command:

```bash
podman -c freebsd run --rm IMAGE /bin/sh -c 'uname -sr'
```

⚠ **That is the client half, and it has not returned 0.**
`41-connect-podman-from-windows.ps1` in `pkgforge-dev/docker-bsd` was **written
and run three times**, and it gets further than that sentence suggests. Every
step below works:

1. a throwaway key generated and installed over the serial console;
2. ⛔ the empty-password ssh door closed **before** the port is forwarded, read
   back from `sshd_config`, and the port bound to `127.0.0.1` only;
3. ssh from Windows into the guest, authenticating;
4. `podman system connection add`, exit 0;
5. the connection removed again afterwards.

⛔ **What does not work is the last hop**, and it is a guest fault rather than a
client one.

⛔ **And one blocker underneath it, which is the real finding.** It was run,
four times, and the tidy explanation did not survive.

`podman info` inside the guest answers `freebsd/amd64 runtime=ocijail`, and a
one-shot `podman run` returns `rc=0` with the container's own stdout. ⛔ **A
long-running `podman system service` panics the guest KERNEL:**

```text
Fatal trap 12: page fault while in kernel mode
current process        = 1546 (podman)
#5 do_wait+0x123   #6 __umtx_op_wait_uint_private+0x54   #7 sys__umtx_op+0x7e
```

`_umtx_op` is FreeBSD's userspace-mutex syscall, and it is what Go's scheduler
parks threads on.

⚠ **The clock was the hypothesis and it is NOT the answer.** Under WHPX the
guest does see `Microsoft Hv` and does select its Hyper-V timecounter, and
setting `kern.timecounter.hardware=ACPI-fast` did move `podman run` from failing
to `rc=0`. ⛔ **But with `ACPI-fast` selected the clock measurably works**:
`delta_ns=1002101384` across a one-second sleep. The timecounter change moved
the symptom; it did not explain it, and
[`bsd.md`](bsd.md) carries the correction under the section that claimed
otherwise.

⭐ **So what is left is one question rather than one command:** can a
multithreaded Go daemon stay alive in a FreeBSD guest under WHPX. `bsd.md` lists
three untried things, and a fourth is now obvious: FreeBSD's `podman_service` rc
script exists beside `podman`, and only `podman` was ever started.

**2. ⚠ The next session works across two directories**, `Azathothas/ToolKit`
and `pkgforge-dev/docker-bsd`. ⛔ **Not `Azathothas/TEMPLATE`.** Every other
remote stays read-only.

### ⚠ Worth an entry, none of them filed

⛔ **Not filed, because filing an entry nobody asked for is how a backlog stops
meaning anything.** The first four are unchanged from the last session.

- ⭐ **`Azathothas/TEMPLATE` carries `git-sync.ps1` with `TOOL-03`'s defect.**
- **The tooling this repository grew is not in the template.**
- ⚠ **`-Command`, `-CommandFile`, `-OciEnv` and `-Systemd` are silently ignored
  by the actions they do not apply to.**
- ⭐ **`pkgforge-dev/cross-libc-dlopen`'s vendored copy of `wsl-ephemeral.ps1`
  carries `WSL-01` and `WSL-12`.** In
  [`../docs/consumers.md`](../docs/consumers.md).
- ⚠ **New: FreeBSD 15.1 GENERIC page-faulted in the kernel during
  `rc.shutdown`** under WHPX, in `vget_finish`, on the boot that had run
  podman. ⛔ It did **not** happen on the boots that did not, which shut down
  cleanly. One occurrence, cause unknown, recorded so it is not a surprise.
- ⚠ **New: `check-no-secrets.sh --public` fires on an OCI content digest.**
  `sha256:` followed by 64 hex characters is a published, public identifier and
  can never be a credential, and this repository is about container images, so
  it will recur. ⛔ **Not fixed inline, and the reason is the interesting part.**
  The obvious fix, another `grep -vE` on the line, is itself a row in
  [`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md):
  `grep -v` drops **lines**, not matched items, so an allowlist for the digest
  would hide a real credential that happened to sit on the same line. ⭐ The
  correct fix is an item-level negative lookbehind in the detection pattern, the
  way `check-docs.sh` already does it. ⚠ It is a change to a tool other
  repositories fetch, so it wants its own entry rather than a drive-by edit.
  This session elided the digest instead.

---

## Open questions for the operator

⛔ These block work. Each carries a recommendation, so agreeing costs nothing.

### 1. ⭐ RULED, 2026-08-27, and now SATISFIED

Nesting was the floor, not the target, and something better was wanted. ⭐ **A
FreeBSD userland on the host's own hypervisor, one level deep, unelevated, is
that.** The full ruling is at the top of [`bsd.md`](bsd.md) and is not restated
here so the two cannot fork.

### 2. ⚠ Should `docker-bsd` publish something bootable?

⭐ **Recommendation changed, and it is now answerable.** The previous session
said "not yet, because `BSD-01` will tell us what actually boots". It has:
**a raw disk image with a stock GENERIC kernel boots on the Windows host's own
hypervisor with no installer.** ⛔ An OCI rootfs does not, on three of the four
BSDs. **Recommend publishing a bootable raw image alongside the rootfs**, the
way smolBSD does through `oras`. ⚠ Still the operator's call, and it is a
`docker-bsd` decision rather than this repository's.

### 3. Are `RULES.md`, `HUMAN.md` and `SECURITY.md` wanted?

Not written. **Recommendation:** leave them until there is something to put in
them. An empty skeleton is honest; a fabricated one outlives the session that
wrote it.

### 4. Should `check-binfmt` join the gate?

**Recommendation: no.** It needs a running podman machine, so on a machine
without one it would be a permanent SKIP, and a check that is always skipped is
one nobody reads. It is a diagnostic, run on request.
