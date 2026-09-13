# Review passes

Where the deep reviews over a change are recorded, one pass per lens, each
pass obliged to FIND something or verify something, never to summarise. The
method and the lenses live in
[`methodology/reviews.md`](methodology/reviews.md); this page is the record.

⛔ A pass with no finding and no verification is not a pass. A pass that only
says "looks fine" is a reader, not a reviewer.

---

## 2026-09-13: wsl-toolkit implements the compatibility interface natively

The change: `tools/windows/wsl-toolkit` no longer embeds or launches
`wsl-toolkit.ps1`; `internal/compat` implements the same command line in Go.
Five passes, each through one lens, fixes folded in per pass.

*(the five passes are recorded below as they complete)*
