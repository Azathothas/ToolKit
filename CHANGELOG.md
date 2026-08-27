# CHANGELOG.md

What shipped, when, and where the evidence is. One entry per shipped unit of
work, pointing at the record that carries the detail.

Four rules, and
[`scripts/common/check-changelog.sh`](scripts/common/check-changelog.sh) holds
all four:

1. ⛔ **Newest first.** A new entry goes at the top of its section.
2. ⛔ **Every heading carries a date**, as an ISO 8601 UTC stamp.
3. ⛔ **Every entry names its record**, the entry or commit carrying the
   evidence.
4. ⛔ **Every entry says whether it deployed.** "No deploy" is a complete and
   common answer. Silence is not.

⛔ Do not tidy this file while shipping something else, and do not delete an
entry. A superseded one is amended in place with a dated note.

---

## 2026-08-27

### 2026-08-27T04:58:19Z: repository created, and `wsl-ephemeral.ps1` decoupled from the template

**Record:** [`PROGRESS.md`](PROGRESS.md), the bootstrap entry.
**Deployed:** no deploy. This repository publishes files, not a service.

`Azathothas/ToolKit` was bootstrapped from `Azathothas/TEMPLATE` so that tools
used across many projects have one home, and so the template keeps only what
every project needs.

- `scripts/powershell-windows/wsl-ephemeral.ps1` moved here byte-for-byte from
  the template. ⛔ **No behaviour changed in the move**, so the diff is
  reviewable as a move. Its ten known defects came with it, unfixed, and are
  tracked as `WSL-01` through `WSL-10` in [`INDEX.md`](INDEX.md).
- `scripts/powershell-windows/wsl-ephemeral.md` written, including an explicit
  list of those limits. A limit hidden is a defect filed against the user later.
- ⚠ **`Azathothas/TEMPLATE` now carries a wrapper at the same path**, which
  fetches this copy by pinned commit and verifies a SHA-256 before executing.
  Callers of the old path keep working. [`docs/consumers.md`](docs/consumers.md)
  is the register.
