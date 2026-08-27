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
entries         total 18  open 0  blocked 0  done 18
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

## ⭐ The headline: the BSD work left this repository

⛔ **`BSD-01` and `BSD-02` moved to
[`pkgforge-dev/docker-bsd`](https://github.com/pkgforge-dev/docker-bsd)**, which
became standalone on 2026-08-27. ⭐ **There are no open entries here.**

⚠ **`BSD-01` is still open.** It is open **there**, with its acceptance command
unchanged. [`bsd.md`](bsd.md) carries the closure and says where each part went.

---

## What this session did

**2026-08-27. Two halves, and the second was not planned.**

1. ⭐ **Nine experiments**, run rather than written, reaching a BSD userland
   from a Windows host. ⛔ They are **not in this tree**: they are in
   `docker-bsd`, because that is where the images and their consumers live.
2. ⭐ **That repository became standalone.** It had been borrowing this
   tree's checks, conventions and methodology; those are now copies living
   there, adapted, so a clone reproduces every measurement with nothing else
   checked out.

⚠ **What this repository keeps:** the tooling those copies came from, and
the record of the sweep that started it, in
[`../docs/reference-sweeps/findings.md`](../docs/reference-sweeps/findings.md).

⛔ **What it no longer owns:** any BSD work. A change to an experiment, a
measurement or an image is a change **there**, made by somebody with that
repository in front of them.

---

## ⭐ The work order

⭐ **Empty. Every entry is closed.**

⛔ **That is a state, not an achievement**, and it is the moment a backlog is
most likely to be refilled with invented work. ⚠ The four items below are
recorded as worth doing and are deliberately **not filed**, because filing an
entry nobody asked for is how a backlog stops meaning anything.

### ⚠ Worth an entry, none of them filed

- ⭐ **`Azathothas/TEMPLATE` carries `git-sync.ps1` with `TOOL-03`'s
  defect.** It is that repository's change to make.
- **The tooling this repository grew is not in the template**: `check-gate`,
  `check-record`, `check-binfmt`, `set-record.mjs`, `check-powershell.ps1`.
- ⚠ **`-Command`, `-CommandFile`, `-OciEnv` and `-Systemd` are silently
  ignored by the actions they do not apply to.** Refusing would be stricter and
  a break.
- ⭐ **`pkgforge-dev/cross-libc-dlopen` carries a vendored
  `wsl-ephemeral.ps1` with `WSL-01` and `WSL-12` in it.** ⛔ Not this
  repository's change to make.
  [`../docs/consumers.md`](../docs/consumers.md) has the evidence.
- ⚠ **New: `check-no-secrets.sh --public` fires on an OCI content digest.**
  `sha256:` and 64 hex characters is a published identifier and can never be a
  credential. ⛔ **The obvious fix is itself a forbidden pattern**: a
  line-level `grep -v` drops the whole line, hiding a real credential beside the
  digest. ⭐ The correct fix is an item-level negative lookbehind in the
  detection pattern.

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
