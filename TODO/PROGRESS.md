# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-13T05:24:47Z; the record commit's own time is its end
baseline        05c1703 clean; local gate green, 20 checks, 36.0s at 05:26:51Z
entries         total 108  open 7  blocked 0  done 101
gate            20 checks green in 39.2s on the staged checkpoint at 06:16:23Z
head            the checkpoint commit on top of 05c1703
```

## Active work

⛔ **`WSL-73` is OPEN at a checkpoint, and the operator stopped the session
there.** Pull request 31 was reviewed trusting nothing and measured, and the
operator asked for it to be closed with the review's findings and decisions on it,
naming this checkpoint. The native replacement is written and unit-proved and has
NOT run on a real host. The PowerShell product is still in the tree. The entry's
checkpoint section is the resume point, in order.

## The work order, set by the operator on 2026-09-13

⛔ **The next session trusts nothing, including this file, and validates
everything before building on it.** Every number below is a claim until a command
re-measures it.

1. ⭐ **Finish `WSL-73`**, from its checkpoint section in
   [`wsl-toolkit-go.md`](wsl-toolkit-go.md): drive the native `distro` and
   `hostaddress` commands and compare them with the script on this host, delete
   the PowerShell product, give the version one home in Go, move the release
   refusals to `tools/repo`, update the docs with no narrative history, pass every
   gate, get CI green, record three reviews, and name the final commits in a
   comment on the closed pull request 31.
2. **File and fix the two gate defects found below.** The gate every later unit
   leans on has to be able to fail.
3. **Read issues [30](https://github.com/Azathothas/ToolKit/issues/30),
   [32](https://github.com/Azathothas/ToolKit/issues/32) and
   [33](https://github.com/Azathothas/ToolKit/issues/33) in full**, comments
   included. Reconcile `WSL-59`, `WSL-67`, `WSL-68`, `WSL-70`, `WSL-71`, `WSL-72`
   and the unfiled findings against them, author the entries that resolve them,
   each with its `INDEX.md` row, and then work those entries until the three
   issues can be closed on evidence.
4. **Review the docs, the repository and CI again**, including what the session
   before this one added to the `wsl-toolkit` manual and the Muse guide.
5. **End the way [`../docs/methodology/sessions.md`](../docs/methodology/sessions.md)
   requires.**

## Done this session

| commit | what |
| --- | --- |
| the checkpoint commit | `WSL-73` checkpoint: the review of pull request 31 measured and recorded, the decisions, and the native `distro` and `hostaddress` commands with 15 unit cases and 11 mutation rows |

## Measurements

Read on Windows 11 Pro 26200 on 2026-09-13:

```text
probe         doctor.ps1 exit 0 in 16.5 s
gate          20 checks green in 36.0 s on 05c1703
pr 31 go      Windows: internal/compat FAIL, 10 cases. Linux, golang:1.25 with
              go 1.25.14: all three packages ok
pr 31 gate    run from its worktree on Windows: 2 problems, secrets and go
pr 31 answers List, HostAddress and a refused parameter matched the embedded
              script on this host
native go     Windows and Linux (golang:1.25): every package ok, gofmt and vet
              clean on both GOOS targets
8.3 TEMP      tools/windows/wsl-toolkit, tools/check and tools/repo suites green
              with TEMP and TMP at the 8.3 short form of a directory under .tmp
shellcheck    ubuntu:24.04's 0.9.0, which is CI's: 25 of 25 scripts clean
mutation      11 new rows, each red: mutate --only throwaway 9 of 9 and
              --only hostaddress 2 of 2. The whole table was not re-run
tree          85,989 text lines in 279 files at 05c1703, git grep -I -c
```

## Found, and not filed

1. ⛔ **The gate's `powershell` check cannot fail.** Its PowerShell snippet prints
   `PARSE|file|message` and `LINT|file|message`, and the Go reader matches
   `PARSE` and `LINT` followed by a TAB, so a parse error or an analyzer finding
   is never reported. Planted: a `.ps1` with an unclosed brace added to the index
   of a scratch worktree, `check powershell --json` exited 0 with `problems: 0`
   over 58 scripts. ⚠ **Until it is fixed, parse every edited `.ps1` by hand.**
2. ⛔ **`scripts/common/check.ps1` resolves the repository from the working
   directory**, with `git rev-parse --show-toplevel`, rather than from its own
   location, which the check contract's point 4 forbids. Running pull request 31's
   worktree copy of `check-gate.ps1` from `main`'s directory checked `main` and
   printed 20 greens. `check.sh` was not read for the same defect.
3. `bsd run` joins a payload's lines with `; `. ⭐ **A comment on
   [issue 33](https://github.com/Azathothas/ToolKit/issues/33#issuecomment-5651331707)**,
   by the operator's ruling, and step 3 reconciles it.
4. `bootstrap.sh --dry-run --toolset agent --codegraph none --json` exits 1 on
   debian 13. The cause was not read.
5. `pkgin` on NetBSD and `pkg_add` on OpenBSD are written and have never run.
6. `soar` and `nix` as user-level providers are written and not driven.
7. A deterministic regression for pre-marker base rollback is still owed.

## Review findings

⛔ **No review pass has run over this checkpoint.** The three lenses are part of
closing `WSL-73` and are owed there.

## Open questions for the operator

⭐ **None blocking.** The review's decisions in `WSL-73`'s checkpoint section are
recorded for the operator to overrule: the native form, ownership by disk
location, what is not carried from the script, version `3.0.0`, and the launcher
going with the script.

## Host state

- `wsl -l -v` reads `podman-machine-default`, `wsl-toolkit`, `wsl-toolkit-muse`
  and `wsl-toolkit-podbox` running, and `eph-pgb` stopped, as at the session's
  start. ⛔ **`eph-pgb` is not this tool's**: its disk is under
  `%LOCALAPPDATA%\wsl-ephemeral`, and the script's `Purge` would unregister it.
- `wsl-toolkit-muse` holds the operator's Muse credential and was not touched.
- The base's rootless engine holds `docker.io/library/golang:1.25`, pulled for the
  Linux test runs, which the next session needs again. Every container ran
  ephemeral and removed itself.
- No throwaway distribution was created, and no BSD guest was booted.
