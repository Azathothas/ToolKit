# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-13T14:24:42Z; the record commit's own time is its end
baseline        fc30c6c, with 13 uncommitted paths another session had left
entries         total 114  open 12  blocked 0  done 102
closed          WSL-73 at 02ea58d, CI run 34767124644 green on all six jobs
filed           WSL-74 to WSL-79 for issues 32 and 33, none implemented
head            the record commit after 02ea58d
```

## Active work

Nothing is in flight. ⭐ **The next session's job is to close issues 30, 32 and
33**, in the order below.

## The work order, set by the operator on 2026-09-13

Each entry's section in [`wsl-toolkit-go.md`](wsl-toolkit-go.md) carries its
premise, decisions and prove. An issue closes only when every entry mapped to it
is closed, and the issue gets a comment naming the commits.

1. **`WSL-74`**, first, because `WSL-75` and `WSL-78` stand on it.
2. **`WSL-79`**, issue 33: its line join reports success over commands that
   never ran.
3. **`WSL-72`**: the ruling it needed is made.
4. **`WSL-76`**, then **`WSL-75`**, **`WSL-77`** and **`WSL-78`**: issue 32, in
   that order, because the later ones use herdr and the grants.
5. **Issue 30:** `WSL-67`'s open items, `WSL-68`, `WSL-70` and `WSL-71`.

⚠ **`WSL-75` to `WSL-79` each carry a decision with a recommendation.** Ask the
operator before building past one, and record the ruling in the entry.

## Rulings in force

1. **2026-09-13: herdr replaces Zellij** as the agents' multiplexer. It
   supersedes the ruling that made Zellij 0.45.1 Muse's durable session, and tmux
   stays a generic fallback. `WSL-76` quotes the operator and carries the swap.
2. **The FreeBSD guest's default disk** rises to the smallest whole number of
   GiB, 12 or 13, that gives a true 10 GiB root, measured with `df -k /`. In
   `WSL-72`, not implemented.
3. **The `bsd run` line join is not an entry of its own.** It is part of issue
   33, so it is a task in `WSL-79`.
4. **Issues 32 and 33 were amended in place** on the operator's instruction: the
   original text kept, and a dated section appended mapping each item to its
   entry.

## Before every push

A green local gate is not CI. Run the Go tests with `TEMP` and `TMP` at an 8.3
short path, then CI's Linux Go job and its ShellCheck in containers.
[`../tools/windows/wsl-toolkit/README.md`](../tools/windows/wsl-toolkit/README.md)
carries all three as commands under "Build and local proof".

## Done this session

| commit | what |
| --- | --- |
| `02ea58d` | `WSL-73` closed: the inherited review pass proved and kept, 27 review findings fixed, 24 mutation rows added, 3 acceptance cases added, the manual, the maintainer README, `shell.md`, `consumers.md` and `RULES.md` corrected, and the two comparison distributions removed. Pull request 31's comment amended, and a second comment names the commits |
| the record commit | `WSL-74` to `WSL-79` filed and `WSL-67` reconciled against issues 30, 32 and 33; issues 32 and 33 amended; `WSL-73`'s closing now names the build its acceptance ran on; `docs/AGENTS.md` and `examples/common/README.md` corrected |

## Measurements

On Windows 11 Pro 26200 on 2026-09-13:

- **At the start:** the probe exit 0 in 19.56 s; the gate exit 1 in 40.88 s with
  2 problems.
- **For `WSL-73`**, with conditions in its closing: the Go suites green on
  Windows with `TEMP` at the 8.3 path and in `golang:1.25`; the acceptance runner
  90 of 90 on a build of `02ea58d`; the mutation table 189 of 192 rows proved on
  Windows, the other 3 skipped there and proved on Linux; CI green on all six
  jobs.
- **While authoring**, each in its entry: `--instance muse config --json`
  resolving another base from a project file, exit 0; Muse Code 1.1.1's own help
  through `base exec`, read-only; `https://dev.meta.ai/install.ps1` read and not
  run.
- **Before the record push:** the Go suites green with `TEMP` at the 8.3 path;
  ShellCheck 0.9.0 in `ubuntu:24.04` clean over every tracked script.

## Found, and not filed

1. ⛔ `repo mutate` never runs a row's cases unmutated, so a case already red
   reports "went red".
2. ⛔ The gate's `powershell` check cannot fail: its producer uses pipe
   separators and its Go reader looks for tabs. Until fixed, parse every edited
   PowerShell file independently.
3. ⛔ `scripts/common/check.ps1` resolves the repository from the working
   directory rather than its own location.
4. `bootstrap.sh --dry-run --toolset agent --codegraph none --json` exits 1 on
   Debian 13; its cause has not been read.
5. A deterministic regression for pre-marker base rollback is still owed.
6. The generated manual prints a one-letter flag as `--c`, which the parser
   accepts and no example writes.
7. ⚠ **Step 3 of this session's order, the second review of the docs, the
   repository and CI, was cut to two fixes** when the operator ended the session
   for budget. `examples/muse-code/README.md` and `examples/common/zellij.md`
   describe Zellij, which is true today and changes with `WSL-76`.

## Review findings

`WSL-73`'s closing carries its four reviews: the door sweep, the guard mutation,
the claim audit and the driven pass. The entries filed this session were
reviewed against the code they cite, and each premise says whether it was read
or measured.

## Open questions for the operator

1. ⭐ **Whether to cut `wsl-toolkit-v3.0.0`.** The latest release is
   `wsl-toolkit-v2.0.2`, which still carries the PowerShell product, and the
   manual on `main` names commands it lacks. Once CI is green on `main`:

   ```powershell
   pwsh -NoProfile -File scripts/common/repo.ps1 release
   ```

   ```powershell
   pwsh -NoProfile -File scripts/common/repo.ps1 release --publish
   ```

2. **The decisions in the new entries**, each recommended in its section:
   `WSL-75` live grants per project; `WSL-76` OpenSSH through `wsl.exe` for the
   Windows herdr client; `WSL-77` adapters declared in the configuration and
   applied by `base ensure`; `WSL-78` Muse kept in the base behind a Windows entry
   point rather than Meta's Windows installer; `WSL-79` document the boot cost in
   every outcome, and keep a guest running only if tuning leaves more than 30
   seconds.

## Host state

- Registered distributions: `podman-machine-default`, `eph-pgb`, `wsl-toolkit`,
  `wsl-toolkit-muse` and `wsl-toolkit-podbox`, the five registered before
  `WSL-73` began. `eph-pgb` keeps its disk under `%LOCALAPPDATA%\wsl-ephemeral`
  and is not this tool's.
- `wsl-toolkit-muse` holds the operator's signed-in Muse credential. Never read,
  recreate or remove it. This session ran only `muse --version` and help pages in
  it.
- herdr is installed nowhere. The shared FreeBSD pkgbase guest was not booted or
  changed.
