# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-13T09:20:27Z; the record commit's own time is its end
baseline        97c80f2, with the second WSL-73 checkpoint staged and unpushed
entries         total 108  open 7  blocked 0  done 101
checkpoint      WSL-73 third checkpoint: parity surface driven, two reviews owed
head            the checkpoint commit; WSL-73 remains open
```

## Active work

⛔ **`WSL-73` is OPEN at its third checkpoint**, which the operator asked for
mid-session. Pull request 31 stays closed and unmerged. The entry's newest
section in [`wsl-toolkit-go.md`](wsl-toolkit-go.md) lists the 18 defects this
session found in the second checkpoint and fixed, what was measured, and what
is left, in order.

What is not done: the guard mutation and claim audit reviews, a full acceptance
run after finding 16, the whole `repo mutate` table, the changelog entry, the
removal of the two comparison distributions, CI green on a final commit, and
the second comment on pull request 31.

## The work order, set by the operator on 2026-09-13

1. ⭐ **Finish `WSL-73` and nothing before it.** Its newest checkpoint section is
   the list. Done only when the port is complete, parity is reached and improved
   on, every suite passes, the docs are updated and bloat-free with no narrative
   history, the tree is clean, CI is green, and three deep reviews are recorded.
2. **Only after WSL-73 is done**, read issues 30, 32 and 33 in full, comments
   included, for understanding only. Reconcile existing open entries against
   them and author the entries that resolve them, each with its INDEX row in the
   same change. Do not implement them.
3. Review the docs, the repository and CI again, including the wsl-toolkit
   manual and the Muse guide.
4. End per [`../docs/methodology/sessions.md`](../docs/methodology/sessions.md):
   the gate, the record, `TODO/SUMMARY.md`, the chat summary table, and a deeply
   reviewed zero-context kickoff prompt for the session that closes issues 30,
   32 and 33.

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

| checkpoint | what |
| --- | --- |
| the current commit | `WSL-73`'s third checkpoint: 18 defects in the second checkpoint fixed with guards, the command channel framed, the relay and event log on one contract, the manual and harnesses rewritten, `WSL-59` amended for the deletion |

## Measurements

The entry's checkpoint section carries the table. In short, on Windows 11 Pro
26200 on 2026-09-13: the probe exit 0 in 41.86 s; the opening gate over the
staged tree exit 1 with 3 problems; every Go module green on Windows and in
`golang:1.25`; ShellCheck 0.9.0 clean over 25 scripts; 35 mutation rows added
and each proved; acceptance 85 of 87 before the stale catalog count was fixed;
the consumer smoke 14 of 14 against `v2.0.2`; the gate 19 of 19 on the
checkpoint tree.

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

## Review findings

The door sweep ran over the whole change and found two defects, both fixed:
`distro run`, `enter` and `snapshot` acted on a distribution another run was
still creating, and `distro purge --apply` deleted a snapshot export still being
written. ⛔ **The guard mutation and claim audit lenses have not run.**

## Open questions for the operator

⭐ **None blocking.**

## Host state

- Distributions registered at session start: `podman-machine-default`,
  `eph-pgb`, `wsl-toolkit`, `wsl-toolkit-muse`, `wsl-toolkit-podbox`, and the
  two comparison distributions below. `eph-pgb` keeps its disk under
  `%LOCALAPPDATA%\wsl-ephemeral` and is not this tool's.
- The acceptance runner created `wsl-toolkit-acc` and removed it again; its last
  case confirmed the pre-existing distributions untouched.
- `eph-wsl73n-main`, under `.tmp/wsl73-native-baseline/distros`, and
  `eph-wsl73s-main`, under `.tmp/wsl73-script-baseline`, remain for removal by
  the exact commands in the entry. Remove only those two state directories
  after them.
- `wsl-toolkit-muse` holds the operator's signed-in Muse credential. Never read,
  recreate or remove it.
- The base holds no job, container or open record from this session.
- The shared FreeBSD pkgbase guest was not booted or changed.
