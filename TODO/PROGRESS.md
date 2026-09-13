# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-13T14:24:42Z; the record commit's own time is its end
baseline        fc30c6c, with 13 uncommitted paths another session had left
entries         total 108  open 6  blocked 0  done 102
closed          WSL-73: the port finished, driven, reviewed and recorded
head            the WSL-73 closing commit
```

## Active work

⭐ **`WSL-73` is closed.** Its closing in [`wsl-toolkit-go.md`](wsl-toolkit-go.md)
carries the parity map, the four reviews and the acceptance output. Step 1 of the
work order ends when CI is green on the closing commit and pull request 31 carries
the comment naming it.

## The work order, set by the operator on 2026-09-13

1. ⭐ **Finish `WSL-73`**: closed in this session, CI and the pull request comment
   owed on the closing commit.
2. **Then** read issues 30, 32 and 33 in full, comments included, for
   understanding only. Reconcile existing open entries against them and author
   the entries that resolve them, each with its INDEX row in the same change.
   Do not implement them.
3. Review the docs, the repository and CI again, including the wsl-toolkit manual
   and the Muse guide.
4. End per [`../docs/methodology/sessions.md`](../docs/methodology/sessions.md):
   the gate, the record, `TODO/SUMMARY.md`, the chat summary table, and a deeply
   reviewed zero-context kickoff prompt for the session that closes issues 30, 32
   and 33.

Rulings to carry into step 2: the FreeBSD guest's default disk rises to the
smallest whole number of GiB, 12 or 13, that gives a true 10 GiB root
filesystem measured with `df -k /`, recorded in the BSD disk entry and not
implemented; and the `bsd run` line join is not an entry, because the operator
had it posted on issue 33.

## Before every push

A green local gate is not CI. Run the Go tests with `TEMP` and `TMP` at an 8.3
short path, then CI's Linux Go job and its ShellCheck in containers.
[`../tools/windows/wsl-toolkit/README.md`](../tools/windows/wsl-toolkit/README.md)
carries all three as commands under "Build and local proof".

## Done this session

| commit | what |
| --- | --- |
| the closing commit | `WSL-73` closed: the inherited review pass proved and kept, 27 review findings fixed, 24 mutation rows added, 3 acceptance cases added, the manual, the maintainer README, `shell.md`, `consumers.md` and `RULES.md` corrected, and the two comparison distributions removed |

## Measurements

The entry's closing carries each with its conditions. In short, on Windows 11 Pro
26200 on 2026-09-13: the probe exit 0 in 19.56 s; the opening gate exit 1 in
40.88 s with 2 problems; the Go suites green on Windows with `TEMP` at the 8.3
path and in `golang:1.25`; the acceptance runner 90 of 90; the mutation table
189 of 192 rows proved on Windows, the other 3 skipped there and proved on Linux; the gate 19 of 19 checks.

## Found, and not filed

1. ⛔ `repo mutate` never runs a row's cases unmutated, so a case already red
   reports "went red". File against tooling in step 2.
2. ⛔ The gate's `powershell` check cannot fail: its producer uses pipe
   separators and its Go reader looks for tabs. Until fixed, parse every edited
   PowerShell file independently.
3. ⛔ `scripts/common/check.ps1` resolves the repository from the working
   directory rather than its own location.
4. The `bsd run` line join defect belongs to issue 33 by operator ruling and is
   deliberately not an entry.
5. `bootstrap.sh --dry-run --toolset agent --codegraph none --json` exits 1 on
   Debian 13; its cause has not been read.
6. `pkgin` on NetBSD, `pkg_add` on OpenBSD, `soar` and `nix` remain undriven.
7. A deterministic regression for pre-marker base rollback is still owed.
8. For step 3: [`../docs/AGENTS.md`](../docs/AGENTS.md) says a naive licence
   fill corrupts four of the twelve texts, and
   [`../LICENSES/README.md`](../LICENSES/README.md) says five.
9. The generated manual prints a one-letter flag as `--c`, which the parser
   accepts and no example writes.

## Review findings

The entry's closing carries all four reviews: the door sweep, the guard
mutation, the claim audit and the driven pass, each with what it looked at that
the others did not.

## Open questions for the operator

⭐ **Whether to cut `wsl-toolkit-v3.0.0`.** The latest release is
`wsl-toolkit-v2.0.2`, which still carries the PowerShell product, and the manual
on `main` names commands it lacks. A release is the operator's to ask for. Once
CI is green on `main`:

```powershell
pwsh -NoProfile -File scripts/common/repo.ps1 release
```

```powershell
pwsh -NoProfile -File scripts/common/repo.ps1 release --publish
```

## Host state

- Registered distributions: `podman-machine-default`, `eph-pgb`, `wsl-toolkit`,
  `wsl-toolkit-muse` and `wsl-toolkit-podbox`, the five registered before
  `WSL-73` began. `eph-pgb` keeps its disk under `%LOCALAPPDATA%\wsl-ephemeral`
  and is not this tool's.
- This session created and removed `eph-s1-probe`, removed the comparison
  distributions `eph-wsl73n-main` and `eph-wsl73s-main`, and deleted their two
  state directories under `.tmp`. The acceptance runner created and removed its
  own; its last case read every pre-existing distribution back.
- `wsl-toolkit-muse` holds the operator's signed-in Muse credential. Never read,
  recreate or remove it.
- The shared FreeBSD pkgbase guest was not booted or changed.
