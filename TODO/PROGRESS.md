# PROGRESS.md

⭐ **The one file every session reads first.** Where the work is, what is next,
and why. [`INDEX.md`](INDEX.md) is the list of entries and the **order lives
here and nowhere else**. [`RULES.md`](RULES.md) is the half of the record that
does not change between sessions, and [`SUMMARY.md`](SUMMARY.md) is the last
session's table, which is a snapshot rather than an authority.

⛔ Rewritten every session. It carries no history: the history is the git log
and the entries themselves. Do not add a "previous sessions" section.

⛔ Edited in the same change as the work, never as a report afterwards.

---

## State

```text
session started 2026-08-29T14:26:11Z
baseline        ci green on all three jobs at 7127ff7, tree clean, and the
                gate was 13 checks passing in 49.8s
entries         total 30  open 0  blocked 0  done 30
```

⚠ The counts above are checked against [`INDEX.md`](INDEX.md)'s rows by
`scripts/common/check-record.sh`, which runs as a gate. ⛔ Do not edit them by
hand to make a check pass; fix whichever file is wrong.
⭐ `scripts/common/set-record.mjs recount` moves them for you.

⛔ **The baseline was green and two of its checks did not exist.** The gate this
session started from ran thirteen entries and it now runs fifteen. The two that
were added found **167 marker problems and 17 two-home sentences** in the tree
that baseline called clean. A green baseline is a statement about the checks
that ran and nothing more.

⭐ **Re-measured on this machine on 2026-08-29**, because two more twin pairs
made the figures the scripts carried stop describing this tree. ⚠ These are
separate runs on a machine doing other things, so they do not add up.

| measurement | this session | before, 2026-08-27 |
| --- | --- | --- |
| the fast gate, sh half | 66s, 15 checks | 49.8s, 13 checks |
| the fast gate, PowerShell half | 41s, 15 checks | not comparable: it skipped six |
| the full gate | 379s | 208s |
| `check-twins` alone | 270s | 171s |
| twin pairs it compares | 13 | 10 |

---

## What this session did

**2026-08-29. The four open issues, and six defects found while doing them.**

⭐ **Every issue open against this repository is answered.** Twelve entries
filed and closed: `DOC-02` through `DOC-05`, `TOOL-04` through `TOOL-08`, and
`WSL-13` through `WSL-15`.

### The half that was asked for

- **`wsl-ephemeral.ps1` gains two read-only actions.** `-Action Resources` says
  what WSL and the container engine are holding and prints the cleanup commands
  without running one of them. `-Action HostAddress` answers what a distro
  reaches this host at, which previously took building a throwaway VM to find
  out. `-PortForward` was refused, with the reason recorded in the entry.
- **A launcher**, `wsl-ephemeral-launcher.ps1`, so fetching and running this
  tool from another project is one command rather than five careful steps.
- **`docs/templates/` is gone**, and `TODO/` is the shape
  [`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md) names:
  a record, an index, the entries, [`RULES.md`](RULES.md), and one form at
  [`ENTRY.md`](ENTRY.md).
- **`docs/AGENTS.md` exists** and is written to be read end to end. `README.md`
  is for a person and carries the map; the root `AGENTS.md` is a door.
- **Four helpers moved here from `Azathothas/TEMPLATE`** with the licence texts
  one of them reads, and two drifted checks were refreshed from it.

### ⭐ The half nobody asked about, and it is the more useful half

⛔ **Six defects, four of them reporting success over a failure.** Each is an
entry, and each has a row in
[`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md).

| what | how long it had been true |
| --- | --- |
| a CI step named `yaml parses` iterated zero files and exited 0 | since it was written. Python's `glob` skips dot-directories and every yaml file here is under `.github/` |
| `check-remote-items` exited 1 for an item that only needed reading | since the first issue was filed. The weekly workflow was red the whole time |
| its two modes disagreed on the same tree, text 1 and json 0 | the same |
| `check-gate.ps1` skipped six checks on the host it exists for | since it grew a twin. Invisible on any machine with Git Bash |
| both halves of `deslop` printed the count they had planned to remove | in the repository it was imported from |
| 164 characters outside the five, in 28 files, all in scripts | since `check-docs.sh` was written to read markdown alone |

---

## ⭐ The work order

⭐ **Empty. Every entry is closed and nothing is outstanding from the issues.**

⛔ **That is a state rather than an achievement**, and it is the moment a
backlog is most likely to be refilled with invented work. The items below are
recorded as worth doing and are deliberately **not filed**.

### ⚠ Worth an entry, none of them filed

- **`Azathothas/TEMPLATE` carries `git-sync.ps1` with `TOOL-03`'s defect.** It
  is that repository's change to make, and it has been told in a comment on its
  issue 9.
- **The tooling this repository grew is still not in that template**:
  `check-gate`, `check-record`, `check-binfmt`, `set-record.mjs`,
  `check-powershell.ps1`, and now the way `check-gate.ps1` runs a twin.
- **Four parameters are silently ignored by the actions they do not apply to.**
  Refusing would be stricter and a break. ⚠ The two new actions join that list:
  they ignore every parameter.
- **`pkgforge-dev/cross-libc-dlopen` carries a vendored `wsl-ephemeral.ps1`**
  with `WSL-01` and `WSL-12` in it. ⛔ Not this repository's change to make.
- **`check-no-secrets.sh --public` fires on an OCI content digest.** `sha256:`
  and 64 hex characters is a published identifier and can never be a credential.
  ⛔ The obvious fix is itself a forbidden pattern: a line-level `grep -v` drops
  the whole line and hides a real credential beside the digest. The correct fix
  is an item-level negative lookbehind in the detection pattern.
- **`Azathothas/bit-cli`'s `docs/containers.md` predates the two new actions.**
  It tells a reader to create a distro and decode `/proc/net/route`, which is
  now one command. ⛔ That repository's change, not this one's.

---

## Open questions for the operator

Each carries a recommendation, so agreeing costs nothing. None blocks work.

### 1. Should `check-binfmt` join the gate?

**Recommendation: no**, unchanged from last session. It needs a running podman
machine, so on a machine without one it is a permanent skip, and a check that is
always skipped is one nobody reads. It is a diagnostic, run on request.

### 2. `HUMAN.md` and `SECURITY.md` are still not written

⭐ **`RULES.md` was the third of that set and it now exists**, because the work
model names it and there was something to put in it. The other two are not part
of that shape. **Recommendation:** leave them until there is. An empty skeleton
outlives the session that wrote it, which is nine instances of what `DOC-04`
removed.

### 3. One path shipped this session is reasoned rather than measured

⚠ **Written down because a reasoned path and a measured one are different
things.** The no-POSIX-shell branch of `check-gate.ps1`. It needs a Windows
session with no `sh`, no `bash` and no Git for Windows anywhere `Get-PosixShell`
looks, and producing one on this machine costs more than the branch is worth.
**Recommendation:** leave it, and measure the first time a machine that already
has that shape is in front of somebody. `TOOL-06` says what would have had to be
true.

⭐ **The two `-Action HostAddress` branches that started this question are now
measured**, against fixture `.wslconfig` profiles rather than by reconfiguring
this machine. `WSL-14` carries the four rows.
