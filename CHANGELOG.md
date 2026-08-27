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

### 2026-08-27T06:24:28Z: the BSD research, and the record moved into TODO

**Record:** [`TODO/PROGRESS.md`](TODO/PROGRESS.md).
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

- The record follows the todo model's own shape now: `TODO/` holding
  `PROGRESS.md`, `INDEX.md` and the entries by category, rather than two files
  at the root with every entry inlined into the index.
- `check-record` and its twin assert that the counts agree with the rows, in
  **both** the index and the record. ⚠ The record half was a gap a review found
  after the check had already reported clean once over exactly that drift.
- ⭐ The `BSD-01` research produced a shape different from the one requested,
  and the images half moved out entirely: `pkgforge-dev/docker-bsd` now builds
  FreeBSD, NetBSD, OpenBSD and DragonFly for amd64. Only FreeBSD publishes OCI
  images upstream, so three of the four are new.
- What is left here is one scripted VM guest, ranked against every alternative
  on friction, performance and interop in [`TODO/bsd.md`](TODO/bsd.md).


### 2026-08-27T04:58:19Z: repository created, and `wsl-ephemeral.ps1` decoupled from the template

**Record:** [`TODO/PROGRESS.md`](TODO/PROGRESS.md), the bootstrap entry.
**Deployed:** no deploy. This repository publishes files, not a service.

`Azathothas/ToolKit` was bootstrapped from `Azathothas/TEMPLATE` so that tools
used across many projects have one home, and so the template keeps only what
every project needs.

- `scripts/powershell-windows/wsl-ephemeral.ps1` moved here byte-for-byte from
  the template. ⛔ **No behaviour changed in the move**, so the diff is
  reviewable as a move. Its eleven known findings came with it, unfixed, and are
  tracked as `WSL-01` through `WSL-11` in [`TODO/INDEX.md`](TODO/INDEX.md).
- `scripts/powershell-windows/wsl-ephemeral.md` written, including an explicit
  list of those limits. A limit hidden is a defect filed against the user later.
- ⚠ **`Azathothas/TEMPLATE` now carries a wrapper at the same path**, which
  fetches this copy by pinned commit and verifies a SHA-256 before executing.
  Callers of the old path keep working. [`docs/consumers.md`](docs/consumers.md)
  is the register.
