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
session started 2026-08-27T11:54:27Z
baseline        ci green on all three jobs at 9503a23, tree clean
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
| the local gate | ⭐ `sh scripts/common/check-gate.sh --fast`. ⚠ 41s and 208s were measured in a previous session and were **not** re-timed in this one. |

**Origin.** Bootstrapped from `Azathothas/TEMPLATE` on 2026-08-27. Its first
content was `wsl-ephemeral.ps1`, decoupled from that template so the template
keeps only what every project needs. That template carries a wrapper that
fetches this copy by pinned commit and verified digest.
[`../docs/consumers.md`](../docs/consumers.md) is the state.

---

## The measured baseline

⚠ **This session wrote documents and ran no code**, so the gate rows below are
the ones it actually ran and the tool rows are inherited from the session that
measured them.

⛔ **Part (c) of the gate, the three deep reviews, was NOT run.** The session
ended on the operator's instruction before them. ⚠ **Treat every claim sourced
from another repository as one pass and re-derive it before acting**; the
measurements taken on this machine are labelled as such and are real.
[`SUMMARY.md`](SUMMARY.md) states the split.

| gate | result |
| --- | --- |
| `check-gate.sh --fast` | ✅ 12 passed, 1 skipped (`check-twins`), run twice |
| `check-docs.sh` | ✅ 41 files, 246 relative links, 88 shell blocks |
| `check-record.sh` | ✅ 18 entries, counts agree with rows |
| `check-changelog.sh` | ✅ 14 entries, in order, each dated with a record and a deploy line |
| CI, all three jobs | ✅ success at `796e40f` |
| `pkgforge-dev/docker-bsd` CI | ✅ both jobs at `878e286` |

### ⭐ Measured on this machine this session, and each answers an open question

| probe | result |
| --- | --- |
| `WHvGetCapability`, capability 0, through `WinHvPlatform.dll` | ⭐ `hr=0`, value `1`, **unelevated**. The Windows Hypervisor Platform is installed and the hypervisor is running |
| host CPU | `Intel64 Family 6 Model 154 Stepping 3`, a 12th Gen Core i7-12700H |
| `qemu-system-x86_64`, `qemu-img`, `oras` on Windows | ❌ all three absent, exit 1 read unpiped |
| `/dev/kvm` inside `podman-machine-default` | ✅ present and **writable**, `kvm_intel.nested` is `Y`, kernel `7.2.0-WSL2-STABLE` |
| `qemu-system-x86_64` inside that machine | ❌ absent |

---

## What the last session did

**2026-08-27, the reference sweep. No code changed.**

- ⭐ **28 repositories mined**, from 23 rows the operator supplied, one of which
  was an organisation query. Every one reached; none recorded as gone. Issues,
  pull requests and discussions in both states, with comments.
- ⭐ **`BSD-02` closed.** Its acceptance was a written answer per BSD, and that
  is what the sweep produced.
- **`BSD-01` corrected in six places**, all underneath the table rather than
  edited into it.
- **`pkgforge-dev/docker-bsd` prepared** for the session that follows: an
  `experiments/` directory with a layout and two working probes, its own
  ignored `.tmp/`, and `TOOLKIT.md` naming what it must carry to stand alone.

### ⭐ Three premises the sweep disproved

⛔ **None was edited away.** Each keeps its wording and carries the correction
underneath, per
[`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md).

- **[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md).**
  It said there is no counterpart presenting FreeBSD syscalls on a Linux kernel.
  `AkihiroSuda/lsf` is one. ⚠ It is a 2022 proof of concept that crashes and has
  one commit, so the conclusion holds and the reasoning was wrong. ⭐ It also
  explains the **139**: the Linux kernel does not validate an ELF binary's OSABI
  on `execve`, which is why the loader accepts a FreeBSD binary at all.
- **`BSD-02`.** "Not yet runnable anywhere" conflated two questions. Three BSDs
  have no jail-equivalent OCI runtime, which is what the entry meant; all four
  are runnable as guests and two have been for years.
- **`BSD-01`'s WHPX row**, rated a fallback worth taking only if Hyper-V is
  unavailable. ⭐ Hyper-V and WHPX are both available here, measured, and the
  caveat that said the feature list could not be read without elevation was
  wrong about the method rather than the answer.

### ⚠ What the sweep found that nothing had asked for

- ⛔ **An unregistered consumer**, found while reading
  `pkgforge-dev/cross-libc-dlopen` for its `experiments/` layout. It carries a
  **vendored copy** of `wsl-ephemeral.ps1`, 536 lines against this tree's 1,579,
  with no pin and no digest. ⭐ **It carries both P0s this repository has
  closed**, verified by reading its source.
  [`../docs/consumers.md`](../docs/consumers.md) has the row and the evidence.
  ⚠ Not fixable from here; that repository is read-only to this one.
- ⛔ **Two defects in the probes written for `docker-bsd`, both found by running
  them on all three hosts rather than on one.** `grep -i microsoft /proc/version`
  answers "not WSL" inside a machine running a custom WSL2 kernel, and
  `Add-Type` on Windows PowerShell 5.1 fails outright when `LIB` holds a stale
  directory, because it shells out to `csc.exe` and compiles
  warnings-as-errors.

---

## ⭐ The work order

**1. `BSD-01`, and it is the only open entry.** ⛔ **Read the ruling at the top
of [`bsd.md`](bsd.md) before anything else in that file**, then the six
corrections underneath the table, then
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md).

⭐ **The entry now names what to try and in what order**, and the order is not
what it was:

1. a smolBSD rescue image under `qemu -accel whpx`, with a named CPU model;
2. `acj`'s FreeBSD kernel and root filesystem under Firecracker, inside the
   podman machine;
3. the Host Compute System API directly, which is the untried avenue;
4. ⚠ the Hyper-V `.vhd` guest, which stays the fallback that is known to work.

⛔ **A session that opens by building the nested stack has still skipped the
ask**, and it now has three better things to try first.

**2. ⚠ The next session works across two directories**, `Azathothas/ToolKit`
and `pkgforge-dev/docker-bsd`. ⛔ **Not `Azathothas/TEMPLATE`.** The operator
authorised read and write on `docker-bsd` on 2026-08-27; every other remote
stays read-only.

### ⚠ Four things worth an entry, none of them filed

⛔ **Not filed, because filing an entry nobody asked for is how a backlog stops
meaning anything.** Recorded so they are not re-derived. The first three are
unchanged; the fourth is new.

- ⭐ **`Azathothas/TEMPLATE` carries `git-sync.ps1` with `TOOL-03`'s defect.**
  It is that repository's change to make.
- **The tooling this repository grew is not in the template**: `check-gate`,
  `check-record`, `check-binfmt`, `set-record.mjs`, `check-powershell.ps1`.
- ⚠ **`-Command`, `-CommandFile`, `-OciEnv` and `-Systemd` are silently ignored
  by the actions they do not apply to.** Refusing would be stricter and a break.
- ⭐ **New: `pkgforge-dev/cross-libc-dlopen`'s vendored copy of
  `wsl-ephemeral.ps1` carries `WSL-01` and `WSL-12`.** ⛔ Not this repository's
  change to make. Recorded in [`../docs/consumers.md`](../docs/consumers.md)
  rather than filed here, because filing it would be filing work this repository
  cannot do.

---

## Open questions for the operator

⛔ These block work. Each carries a recommendation, so agreeing costs nothing.

### 1. ⭐ RULED, 2026-08-27. Not open. Read it before touching `bsd.md`.

The ruling has two halves and the second is the one a reader skims past: nesting
is the floor to fall back to, not the thing to build. ⛔ The full ruling is at
the top of [`bsd.md`](bsd.md) and is not restated here, so the two cannot fork.

⭐ **The sweep has now given that ruling something to work with.** A BSD microvm
on the host's own hypervisor is one level deep, boots in about 10 milliseconds
for NetBSD and about 12 seconds for FreeBSD under Firecracker, and has published
artefacts for both. That is a better answer than nesting rather than an argument
against it.

### 2. ⚠ NEW. Should `docker-bsd` publish something bootable?

⛔ **Not a decision this session took.** `docker-bsd` publishes a root
filesystem, and for three of its four BSDs nothing exists that can run one. The
sweep found two projects solving the same distribution problem differently:
smolBSD pushes a raw bootable disk to `ghcr.io` through `oras`, and `acj`
publishes a kernel and a root filesystem as release assets.

**Recommendation: not yet.** ⚠ It is a shape question that `BSD-01` will answer
by finding out what actually boots on this machine. Deciding it now would be
choosing a format before knowing what consumes it. Recorded in `BSD-02`'s
closure so it is not lost.

### 3. Are `RULES.md`, `HUMAN.md` and `SECURITY.md` wanted?

Not written. **Recommendation:** leave them until there is something to put in
them. An empty skeleton is honest; a fabricated one outlives the session that
wrote it.

### 4. Should `check-binfmt` join the gate?

**Recommendation: no.** It needs a running podman machine, so on a machine
without one it would be a permanent SKIP, and a check that is always skipped is
one nobody reads. It is a diagnostic, run on request.
