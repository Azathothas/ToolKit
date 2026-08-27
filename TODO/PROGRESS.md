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
session started 2026-08-27T07:24:43Z
baseline        ci green on both hosts, all three jobs
entries         total 17  open 8  blocked 0  done 9
```

⚠ The counts above are checked against
[`INDEX.md`](INDEX.md)'s rows by `scripts/common/check-record.sh`, which runs as
a gate. ⛔ Do not edit them by hand to make a check pass; fix whichever file is
wrong. ⭐ `scripts/common/set-record.mjs` moves them for you now.

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
fetches this copy by pinned commit and verified digest, and ⭐ **that pin moved
in this session**. [`../docs/consumers.md`](../docs/consumers.md) is the state.

---

## The measured baseline

Every gate below was run on this Windows 11 Pro 26200 machine, unpiped.

| gate | result |
| --- | --- |
| `check-gate.sh`, full | ✅ 13 checks, 13 passed, 0 skipped |
| `check-gate.ps1`, full | ✅ identical json to the sh half |
| `check-twins` | ✅ every pair agrees |
| PSScriptAnalyzer over `scripts/`, Error and Warning | ✅ clean, 12 files |
| `-Action New` end to end, PowerShell 7.6.5 | ✅ exit code propagates |
| `-Action New` end to end, Windows PowerShell 5.1 | ✅ ⭐ fixed this session, `WSL-12` |
| CI, all three jobs | ✅ success |

---

## What the last session did

**2026-08-27, the first implementation pass. Nine entries closed.**

- ⭐ **`WSL-01`, `WSL-02`, `WSL-03`, `WSL-04`, `WSL-05`** closed with evidence,
  each mutation-proven, each its own commit.
- ⭐ **`WSL-12` filed and closed**, found by the door sweep rather than by any
  issue: `-Action New` was failing outright on Windows PowerShell 5.1, on a host
  `.NOTES` claimed to be tested on.
- **`TOOL-01`, `TOOL-02`, `DOC-01`** closed. Four new scripts: `check-gate` and
  its twin, `check-powershell.ps1`, `set-record.mjs`, `check-binfmt` and its
  twin.
- **The `Azathothas/TEMPLATE` pin moved**, and issues were opened there
  proposing the backports.

⛔ **It did not finish.** `WSL-06` through `WSL-11` are untouched, and the
`bsd.md` work the operator asked for has not started. See the work order.

### ⚠ Three premises measurement disproved, all written under their entries

⛔ **None was edited away.** A premise a measurement disproves keeps its title
and gets the correction underneath.

- **`WSL-03`.** An unqualified `podman pull` is **not** a no-op over a
  foreign-architecture tag on podman 5.8.6; it re-pulls the host's. The trap is
  real in `create`. ⭐ And the stated symptom was wrong in the dangerous
  direction: this kernel carries 31 `qemu-*` `binfmt_misc` handlers with the `F`
  flag, so a riscv64 rootfs **boots and runs emulated** rather than failing.
- **`WSL-02`.** Its own prove command cannot be sent at all, and its suggested
  comparison against a login shell in the container answers the wrong question.
- **`WSL-08`.** The caller does not "own all quoting"; the quoting is destroyed
  in transit. Measured per character, on both hosts.

---

## ⭐ The work order

**1. `WSL-06`, `WSL-07`, `WSL-09`, `WSL-10`, `WSL-11`.** The straightforward
tail. All S, all independent of each other.

**2. `WSL-08`, last of the batch and the one that matters most.** It is an M and
it is where the transport defect behind `WSL-12` actually gets fixed.
⭐ **Read its premise first**: the measurement is already there,
`Write-DistroFile` already implements the working channel, and ⛔ **two payloads
inside the script have to move to that channel in the same change** or the next
one repeats `WSL-12`.

**3. `BSD-01` and `BSD-02`.** ⚠ A separate session with its own kickoff. The
operator has asked for [`bsd.md`](bsd.md) to be extended with a native path and
a universal nested-VM fallback, and reconciled with what it already ranks,
**before** any of it is built.

---

## Open questions for the operator

⛔ These block work. Each carries a recommendation, so agreeing costs nothing.

### 1. `BSD-01`: which workaround for the missing BSD kernel?

⭐ **The constraint, measured:** a FreeBSD image on this machine's Linux podman
machine exits **139**, a SIGSEGV. A BSD userland needs a BSD kernel.

⚠ **This question is being reopened deliberately.** The operator has asked for
two more approaches to be written up and reconciled with the existing ranking: a
native path per host, and a universal nested one. ⛔ **Do not rule on it from
the old text alone**; that is the next `bsd.md` session's first task.

⭐ **What is settled and does not need re-deriving:** Hyper-V's `vmms` is
Running on this machine alongside the WSL2 podman machine, so the two coexist.
[`bsd.md`](bsd.md) carries the probes.

### 2. Should `WSL-08` keep `-Command` working for simple commands?

⭐ **Recommendation: yes, and it is now more than a convenience.** The entry
already decided to keep it. The measurement changes the reason: `-Command` is
not merely awkward, it is wrong for any payload with a `$` or a backtick, so
"keep it for simple commands" has to ship with the safe alphabet written down.

### 3. Are `RULES.md`, `HUMAN.md` and `SECURITY.md` wanted?

Not written. **Recommendation:** leave them until there is something to put in
them. An empty skeleton is honest; a fabricated one outlives the session that
wrote it.

### 4. Should `check-binfmt` join the gate?

**Recommendation: no.** It needs a running podman machine, so on a machine
without one it would be a permanent SKIP, and a check that is always skipped is
one nobody reads. It is a diagnostic, run on request.
