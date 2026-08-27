# PROGRESS.md

⭐ **The one file every session reads first.** Where the work is, what is next,
and why. [`INDEX.md`](INDEX.md) is the list of entries; the **order lives here
and nowhere else**.

⛔ Rewritten every session. It carries no history: the history is the git log
and the entries themselves. Do not add a "previous sessions" section.

⛔ Edited in the same change as the work, never as a report afterwards.

---

## State

```text
session started 2026-08-27T04:33:44Z
baseline        ci green on both hosts, all three jobs
entries         total 15  open 13  blocked 0  done 2
```

⚠ The counts above are checked against
[`INDEX.md`](INDEX.md)'s rows by `scripts/common/check-record.sh`, which runs as
a gate. ⛔ Do not edit them by hand to make a check pass; fix whichever file is
wrong.

| fact | value |
| --- | --- |
| repository | `Azathothas/ToolKit`, public, 0BSD |
| work model | todo. [`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md) |
| push policy | commit and push, to this remote only |
| CI | three jobs, ubuntu and windows. `.github/workflows/ci.yml` |
| `main` | protected, admin bypass on. Force push and deletion refused. |

**Origin.** Bootstrapped from `Azathothas/TEMPLATE` on 2026-08-27. Its first
content was `wsl-ephemeral.ps1`, decoupled from that template so the template
keeps only what every project needs. The template now carries a wrapper that
fetches this copy by pinned commit and verified digest.

---

## The measured baseline

Every gate below was run on this Windows 11 Pro 26200 machine, unpiped, and CI
ran the same set on ubuntu and windows runners.

| gate | result |
| --- | --- |
| `check-docs`, `check-placeholders`, `check-control-bytes` | ✅ exit 0, both twins |
| `check-record` | ✅ exit 0, both twins, JSON identical |
| `check-changelog` | ✅ exit 0 |
| `check-no-secrets --public` | ✅ exit 0, both twins |
| `check-twins` | ✅ exit 0, 30 pairs agree |
| PSScriptAnalyzer over `scripts/`, Error and Warning | ✅ clean |
| CI, all three jobs | ✅ success |

---

## What the last session did

**2026-08-27, bootstrap and the first research pass.**

- Bootstrapped this repository and moved `wsl-ephemeral.ps1` here byte-for-byte
  from the template, leaving a verified wrapper behind.
- Filed the eleven `WSL-*` entries from `Azathothas/TEMPLATE` issue 3, and
  `DOC-01` from issue 2.
- ⭐ **Did the `BSD-01` reference sweep**, which changed the shape of that work.
  See below.
- Wrote `check-record.sh` and its twin, and wired both into the gate.
  ⚠ It caught a real count error in `INDEX.md` on its first run.

⛔ **Nothing was implemented.** No entry has been worked.

---

## ⭐ The work order

**1. `WSL-01`.** The only entry where the software reports success over a
failure, so it outranks everything regardless of size. ⚠ A copy of its kickoff
was left at `.tmp/PROMPT.md`, which is **gitignored and local to one machine**.
The entry in [`wsl-ephemeral.md`](wsl-ephemeral.md) is the authority; the prompt
is rebuilt from it.

**2. `TOOL-01`.** Finish the record checker by writing the writer. The reader
exists and is a gate; the writer is what stops the arithmetic being manual.

**3. `WSL-03`, then `WSL-04`, then `WSL-02`.** Reasoning in
[`INDEX.md`](INDEX.md) under the ordering argument.

⚠ **`BSD-01` sits after those.** Its images half is done and lives in
`pkgforge-dev/docker-bsd`; what is left here is one scripted VM guest, and its
first step is a measurement, not a build. It carries a decision for the
operator, below.

---

## Open questions for the operator

⛔ These block work. Each carries a recommendation, so agreeing costs nothing.

### 1. `BSD-01`: which workaround for the missing BSD kernel?

⭐ **The constraint, measured:** a FreeBSD image on this machine's Linux podman
machine exits **139**, a SIGSEGV, not `Exec format error`. A BSD userland needs
a BSD kernel and no `binfmt_misc` or `qemu-user` work changes that. It is a
constraint to route around, and every option is ranked on friction, performance
and interop in [`bsd.md`](bsd.md).

**Recommendation:** a Hyper-V guest built from FreeBSD's **published `.vhd`**,
addressed with `podman system connection add`. The `.vhd` is the point: it is
Hyper-V's native disk format, so there is no installer and no ISO, which makes
this the lowest-friction option as well as the fastest and the most
interoperable. `podman -c freebsd run ...` then works with the real client.

⚠ The honest cost is a VM the operator keeps.

⭐ **The step that was going to block it is done.** Hyper-V's `vmms` service is
Running on this machine alongside the WSL2 podman machine, so the coexistence
the recommendation depends on is measured. `bsd.md` carries the probes.

### 2. Do the `WSL-*` fixes land as one change or one per entry?

**Recommendation:** one change per entry, grouped into at most three pushes.
Eleven fixes in one commit is unreviewable; eleven pushes is eleven CI runs.

### 3. Should `WSL-02` change behaviour or only document it?

**Recommendation:** carry the OCI config, behind a switch defaulting to off, so
no existing caller moves under them.

### 4. Are `RULES.md`, `HUMAN.md` and `SECURITY.md` wanted?

Not written. ⚠ `RULES.md` is named in the todo model's shape and is a stub
pointing at the conventions; the other two have nothing true in them yet.
**Recommendation:** leave them until there is something to put in them. An empty
skeleton is honest; a fabricated one outlives the session that wrote it.
