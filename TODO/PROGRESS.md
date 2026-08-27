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
session started 2026-08-27T09:40:00Z
baseline        ci green on both hosts, all three jobs, at ea5d483
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
| the local gate | ⭐ `sh scripts/common/check-gate.sh --fast`, 41s. Full run 208s. |

**Origin.** Bootstrapped from `Azathothas/TEMPLATE` on 2026-08-27. Its first
content was `wsl-ephemeral.ps1`, decoupled from that template so the template
keeps only what every project needs. That template carries a wrapper that
fetches this copy by pinned commit and verified digest.
[`../docs/consumers.md`](../docs/consumers.md) is the state.

---

## The measured baseline

Every gate below was run on this Windows 11 Pro 26200 machine, unpiped.

| gate | result |
| --- | --- |
| `check-gate.sh`, full | ✅ 13 checks, 13 passed, 0 skipped |
| `check-gate.ps1` | ✅ agrees with the sh half |
| `check-twins` | ✅ every pair agrees |
| PSScriptAnalyzer over `scripts/`, Error and Warning | ✅ clean, 13 files |
| `-Action New` end to end, PowerShell 7.6.5 | ✅ |
| `-Action New` end to end, Windows PowerShell 5.1 | ✅ |
| `-Command` byte-exactness, both hosts | ✅ ⭐ guest SHA-256 equals the Windows one |
| `-Systemd` against `almalinux:9`, both hosts | ✅ PID 1 is `systemd` |
| the timeout, against a rootfs whose `/bin/sh` sleeps | ✅ exits 1 within the bound |
| CI, all three jobs | ✅ success at `ea5d483` |

---

## What the last session did

**2026-08-27, the second implementation pass. `wsl-ephemeral.ps1` is finished.**

- ⭐ **`WSL-06` through `WSL-11` closed**, each with its evidence, each
  mutation-proven, each its own commit. ⛔ **No `WSL-*` entry is open.**
- ⭐ **`TOOL-03` filed and closed**, found by using `git-sync.ps1` rather than by
  reading it. It is a P0 and the worst defect of the session.
- **The `Azathothas/TEMPLATE` pin moved** to `ea5d483`, verified end to end from
  both hosts against the live wrapper.

### ⭐ Four premises measurement disproved, all written under their entries

⛔ **None was edited away.** A premise a measurement disproves keeps its title
and gets the correction underneath.

- **`WSL-06`.** "Roughly twice the rootfs size" is not the rule and is not a
  multiple at all. An 8.2 MiB alpine rootfs costs **76 MiB** of VHDX; 74.3 and
  76.9 MiB both cost 172. The cost is a fixed floor, so the entry's estimate was
  wrong in the direction that fails: it would have permitted an import that
  could not finish.
- **`WSL-07`.** The entry described writing `/etc/wsl.conf` and terminating.
  ⛔ Most OCI base images do not ship systemd, so that alone is a flag nothing
  reads: `alpine:3.22`, `ubuntu:24.04` and `fedora:41` have no
  `/usr/lib/systemd/systemd`, and written into ubuntu the flag did nothing and
  said nothing. The switch now verifies `/proc/1/comm` and refuses.
- **`WSL-08`.** The mechanism is expand-**then-re-parse**, not "as though double
  quoted". `$HOME` is harmless and `$PATH` is fatal, because the hazard is what
  the value contains. And a bracket and a single quote DO survive: `WSL-12` was
  the double quote being dropped on 5.1, one character to the left of where it
  was read.
- **`WSL-12`, from the previous session.** Its fix removed brackets from the
  probe. That was treating a symptom. The probe now carries that exact line
  again, brackets and double quotes included, and works on 5.1.

### ⚠ Two defects the work introduced and the gate caught

Both are written into their entries. They are here because they are the shape a
future session should expect to produce.

- ⛔ **`-TimeoutSeconds 15` timed out after 120 seconds**, for one run. A script
  parameter IS a script-scoped variable, so a `$script:TimeoutSeconds = 120` in
  the constants block overwrote whatever the caller passed. ⭐ The refusal was
  correct and the number was a lie, which is the shape a test asserting only
  "it refused" passes over.
- ⛔ **The first transport left an empty file behind whenever the decode
  failed**, because a redirect creates the file before the decode runs. The
  mutation that planted a missing decoder is what found it. Both orderings read
  the same in a diff.

---

## ⭐ The work order

**1. `BSD-01` and `BSD-02`.** ⚠ **They are all that is left**, and they are a
separate session with its own kickoff. ⛔ **Read the ruling at the top of
[`bsd.md`](bsd.md) before anything else in that file.** The operator ruled twice
on 2026-08-27: correct the nested-QEMU refusal and rank it honestly, **and**
treat nesting as the floor rather than the target, because it is a well
documented technique and what is wanted is a better one that avoids its limits.
⛔ A session that opens by building the nested stack has skipped the ask.

**2. ⚠ Nothing else is open.** `wsl-ephemeral.ps1` has no open entry. If the
next session is not the BSD one, the honest options are to work an intake into
new entries per
[`../docs/methodology/authoring.md`](../docs/methodology/authoring.md), or to
take one of the three suggestions below, none of which is filed.

### ⚠ Three things worth an entry, none of them filed

⛔ **Not filed, because filing an entry nobody asked for is how a backlog stops
meaning anything.** Recorded so they are not re-derived.

- ⭐ **`Azathothas/TEMPLATE` carries `git-sync.ps1` with `TOOL-03`'s defect**,
  because that is where this copy came from. A commit made there can still be
  authored by a gate string. It is that repository's change to make.
- **The tooling this repository grew is not in the template**: `check-gate`,
  `check-record`, `check-binfmt`, `set-record.mjs`, `check-powershell.ps1`.
  Backporting is the template's decision, and issues proposing it were opened
  there in the first session.
- ⚠ **`-Command`, `-CommandFile`, `-OciEnv` and `-Systemd` are silently ignored
  by the actions they do not apply to**, which the parameter table documents.
  Refusing instead would be stricter and would be a break.

---

## Open questions for the operator

⛔ These block work. Each carries a recommendation, so agreeing costs nothing.

### 1. ⭐ RULED, 2026-08-27. Not open. Read it before touching `bsd.md`.

The operator ruled on the nested-QEMU question and the ruling has two halves.
⛔ **The second half is the one a reader skims past**: nesting is the floor to
fall back to, not the thing to build. It is a well documented technique, and
what is wanted is a better approach that avoids its limits. A session that opens
by building the nested stack has skipped the ask.

⛔ The full ruling, with what "exhaust everything" means concretely, is at the
top of [`bsd.md`](bsd.md). It is not restated here, so the two cannot fork.

⭐ **What is measured and does not need re-deriving:** a FreeBSD userland on
this machine's Linux podman machine exits **139**; Hyper-V's `vmms` runs
alongside the WSL2 podman machine, so the two coexist; and `/dev/kvm` is present
inside the WSL2 utility VM with `kvm_intel.nested=Y`, so nesting there would be
accelerated rather than emulated.

### 2. ⭐ RULED by the work, 2026-08-27. `WSL-08` kept `-Command`.

It is closed. The answer the measurement forced is worth keeping: `-Command` is
correct for anything now, not merely for simple commands, because the payload no
longer crosses a shell that can reach into it. ⚠ **The residual is one layer
up**: Windows PowerShell 5.1 drops a double quote when it builds a child
process's argument list, so a scripted 5.1 caller wants `-CommandB64`.

### 3. Are `RULES.md`, `HUMAN.md` and `SECURITY.md` wanted?

Not written. **Recommendation:** leave them until there is something to put in
them. An empty skeleton is honest; a fabricated one outlives the session that
wrote it.

### 4. Should `check-binfmt` join the gate?

**Recommendation: no.** It needs a running podman machine, so on a machine
without one it would be a permanent SKIP, and a check that is always skipped is
one nobody reads. It is a diagnostic, run on request.
