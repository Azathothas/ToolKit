# PROGRESS.md

⭐ **The one file every session reads first.** The baseline, what the last
session did, what is in progress, and the work order. Nothing else carries a
work order. [`INDEX.md`](INDEX.md) is the sortable list of open items; this file
says what to do next and why.

⛔ Edited in the same change as the work, never as a report afterwards.

---

## Baseline

**As of 2026-08-27.**

| fact | value |
| --- | --- |
| repository | `Azathothas/ToolKit`, public, 0BSD |
| work model | todo. [`docs/methodology/work-todo.md`](docs/methodology/work-todo.md) |
| push policy | commit and push, to this remote only |
| CI | two hosts, ubuntu and windows. `.github/workflows/ci.yml` |
| gate, locally | the checks in `scripts/common/`, run unpiped |

**Origin.** This repository was bootstrapped from `Azathothas/TEMPLATE` on
2026-08-27, and its first content was `wsl-ephemeral.ps1`, decoupled from that
template so the template keeps only what every project needs. The template now
carries a wrapper that fetches this copy by pinned commit and digest.

---

## The state of the tree

| thing | state |
| --- | --- |
| `scripts/powershell-windows/wsl-ephemeral.ps1` | moved here unchanged from the template. ⛔ It carries ten known defects, none fixed. `wsl-ephemeral.md` lists them and `INDEX.md` tracks them. |
| `scripts/doctor/`, `scripts/common/` | inherited from the template, working, gates green |
| the documents | conventions, methodology and security inherited; `docs/consumers.md` is this repository's own |

⚠ **Nothing here has been through a full gate run on this machine yet beyond
the document and control-byte checks.** The Windows half of CI has not run
against this tree at the time of writing, because the first push is what
triggers it. The first session to work an item confirms CI is green before
opening anything new.

---

## What the last session did

**2026-08-27, bootstrap.**

- Detached from the template remote and initialised this repository.
- Selected what this project keeps from the template and deleted the rest.
- Moved `wsl-ephemeral.ps1` here byte-for-byte, so the move is reviewable as a
  move rather than as a rewrite.
- Wrote `wsl-ephemeral.md`, `docs/consumers.md`, and this record.
- Left the script's ten known defects unfixed on purpose. A bootstrap session
  plans; it does not implement.

---

## The work order

⭐ **Next: `WSL-01`.** It is the only item that can report a false pass, so it
ranks above everything else in the list regardless of size.

Then the rest of the `WSL-*` items in `INDEX.md` by priority. `BSD-01` is
research only and is not started until the operator has approved a written
approach.

---

## Open questions for the operator

| question | why it matters | recommendation |
| --- | --- | --- |
| Should the fixes to `wsl-ephemeral.ps1` land as one change or one per item? | ten items in one commit is unreviewable; ten commits is ten CI runs | one change per item, grouped into at most three pushes |
| Should `WSL-02` change behaviour, or only document it? | carrying the OCI config into the distro changes what an existing caller sees | carry it, behind a switch that defaults off, so no caller changes under them |
