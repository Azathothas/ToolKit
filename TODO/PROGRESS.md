# PROGRESS.md

The record: the state, the measured baseline, what this session did, what is in
progress, the work order and the open questions. ⛔ **Rewritten every session; it
carries no history**, and the record check refuses a section the model does not
have. [INDEX.md](INDEX.md) lists the entries, [RULES.md](RULES.md) holds the
standing rules and the rulings in force, and history is in git, the entries and
`docs/HISTORY/`.

## State

```text
session started 2026-10-21T08:31:38Z, resumed 2026-10-24T01:18:23Z: critical fixes first, as the operator asked, then the rest of ruling 31
baseline        9784821, gate 24 of 24, 139,905 tracked lines, 600 mutation rows
head            this commit: the record rewritten, the methodology synced, 63 entries filed and 11 closed; the builders paused on wip/ branches
entries         total 224  open 50  blocked 5  done 169
this session    ruling 31
```

## Baseline

Measured on this host unless a row says read.

| what | value | read from |
| --- | --- | --- |
| the gate | 24 checks; 131 to 245 s a run | `tools/check/main.go`; the last three runs |
| `check-go.ps1` | 72 to 75 s | the last two runs |
| CI's Linux jobs in containers | Go 68 to 82 s, ShellCheck 0.9.0 over 52 scripts 30 to 43 s | the last two runs |
| mutation rows | 600 at `9784821` | `tools/repo/mutations.json` |
| CI | 8 runs, 3 of them required; one guard shard 9 to 17 min | `gh run view` on `9784821` and `d81910f` |
| the acceptance suite | 102 cases quick, 109 full; 399 s with the installed 7.0.0 | `acceptance.ps1`; the last run |
| the last release | `wsl-toolkit-v7.0.0`, 22 assets, 7 release jobs green | `gh release view`, `gh run view` |
| this host | Windows 11 Pro 26300, PowerShell 7.6.7, WSL 3.0.3.0, git 2.55.0, Go 1.27.0 | the doctor |

### This host's state

| | what is there, and how to undo it |
| --- | --- |
| the tool | `%USERPROFILE%\bin\wsl-toolkit.exe` 7.0.0, from `selfupdate`; the next `selfupdate` replaces it. `gc --include-live --apply` and `gc --images --apply` on 2026-10-21 left the jobs base with no job, session or image of this work |
| distributions | `podman-machine-default`, stopped and WSL's default; `wsl-toolkit`, the jobs base, 58.4 GB; `wsl-toolkit-base`, the operator's base, 1.9 GB, rebuilt on 2026-10-21 with the herdr adapter alone. All three are stopped: the operator asked for `wsl --shutdown` at the pause. ⚠ Neither base disk is sparse |
| the podman connection | `podman-machine-default-root`, because the machine's rootless socket is dead (finding 76 of the old record). Undo with `podman system connection default podman-machine-default` once that socket works |
| the package's copy of the state directory | removed on 2026-10-21 under ruling 31, with its 503 job directories, `WSL-118`. A Bash shell this desktop app starts recreates it when it runs the tool. The job that explains finding 8 is kept at `.tmp/finding8-4e94faa38b4c9c1c` |
| `wsl-toolkit-base` | runs `wsl-toolkit-binfmt.service` and the operator's own `qemu-binfmt-restore.service`; ruling 31 retires the second. It holds the test grant `.tmp/wsl78/proj` at `/workspaces/proj` |
| `wslc` | the Arch and Alpine images this work pulled are removed. The `debian`, `wgcf-connector` and `rust` images are not this work's and stay |
| scratch under `.tmp` | emptied on 2026-10-21, 2.9 GB, except `finding8-4e94faa38b4c9c1c`, `bsd67-driver` (kept by `WSL-67`), the test grant `wsl78` and the empty `go-tmp` |

## This session

- ⭐ **Asked the operator four questions and recorded the answers as ruling 31**
  in `RULES.md`.
- ⭐ **The record is rewritten to the model.** The old `PROGRESS.md`, the index's
  ordering argument and four session tables are in `docs/HISTORY/record.md` word
  for word. Five read-only passes sorted its 116 findings, 30 rulings, reviews and
  host notes against the tree, and every finding still open is an entry.
- ⭐ **58 entries filed**, `WSL-120` to `WSL-163`, `TOOL-29` to `TOOL-37` and
  `DOC-09` to `DOC-13`. The rulings still in force are `RULES.md` section 8.
- ⭐ **The record check holds the shape**, `TOOL-29`, and the docs check's
  cited-path rule runs for the first time, `TOOL-30`.

## In progress

⛔ **Paused by the operator on 2026-10-21**, because the parallel builds kept Defender
scanning. Every builder is stopped, its worktree removed, and its work committed on a
local branch, unpushed. None is merged or gated yet:

| branch | entries | state |
| --- | --- | --- |
| `wip/f6a-adapters` | `WSL-139`, `WSL-141`, `WSL-144`, `WSL-157`, `DOC-12` | finished and driven on a scratch base, 13 rows red; merge, gate, close |
| `wip/f1-jobs` | `WSL-120`, `WSL-121`, the cap in `WSL-136` | unfinished |
| `wip/f2-run-config` | `WSL-122`, `WSL-123`, `WSL-128`, `WSL-129`, `WSL-131`, `WSL-161`, `--cap-add` | unfinished |
| `wip/f4-base-exec` | `WSL-123`'s `base exec`, `WSL-124`, `WSL-125`, `WSL-133`, `WSL-146` | unfinished, rows being run |
| `wip/f5a-base-runtime` | `WSL-126`, `WSL-130`, the limits in `WSL-136` | unfinished |
| `wip/f5b-base-build` | `WSL-137`, `WSL-140`, `WSL-143`, `WSL-147`, `WSL-150`, `WSL-153`, `WSL-155` | unfinished, at its combined drive |
| `wip/f6b-fixes` | `WSL-149` to `WSL-164`, F6b's share | unfinished, at `check-go` |
| `wip/f7-tooling` | `TOOL-32`, `TOOL-33` | unfinished, wiring `main.go` |

⚠ **Resume one or two builders at a time**, not seven: each runs Go builds and a scratch
base, and seven together loaded the machine. A builder's brief is the session's
`specs.md`, `seams.md` and `specs-f7.md`; a fresh session reads the entry instead.
The scratch distributions `wsl-toolkit-f4`, `-f5a` and `-f5b` are unregistered.

## The work order, set by the operator on 2026-10-21

Ruling 31. The argument for the order is in [INDEX.md](INDEX.md).

1. ⛔ **Hotfix `wsl-toolkit-v7.0.1` first**, because workloads are blocked today: `WSL-168`,
   which stops every Debian base with systemd at `base ensure`; `WSL-169`, which can
   destroy a base; and the half of `WSL-137` that clears a stale run state in `run`,
   `matrix` and `base exec`, which every consumer of the jobs base meets after a reboot.
2. **The record and the checks that hold it**, first, because every later commit
   is checked by them: `DOC-10`, `TOOL-29`, `TOOL-30`, `DOC-09`, `TOOL-31`,
   `TOOL-35`, `DOC-13`, `DOC-11` and `DOC-12`.
3. **The survey's asks**, in parallel by code area: jobs `WSL-120`, `WSL-121`,
   and the cap in `WSL-136` (F1); `run` and the configuration `WSL-122`,
   `WSL-123`, `WSL-128`, `WSL-129`, `WSL-131`, `WSL-161` (F2); `base exec`
   `WSL-124`, `WSL-125`, `WSL-133`, `WSL-146` (F4); the base's runtime
   `WSL-126`, `WSL-130` and the limits in `WSL-136` (F5a); the base's build
   `WSL-137`, `WSL-140`, `WSL-143`, `WSL-147`, `WSL-150`, `WSL-153`, `WSL-155`
   (F5b).
4. **The old record's open findings**: adapters `WSL-139`, `WSL-141`, `WSL-144`,
   `WSL-157` (F6a); fixes `WSL-149`, `WSL-151`, `WSL-152`, `WSL-154`, `WSL-156`,
   `WSL-160`, `WSL-162`, `WSL-163`, `WSL-164` (F6b); and here `WSL-132`, `WSL-142`,
   `WSL-145` and `WSL-148`.
5. **The tooling**: `TOOL-32`, `TOOL-33`, `TOOL-37` and `TOOL-38`, and `text-tool`'s
   `WSL-165`.
6. **The skills**, `DOC-14`, reviewed against what the tool does once the builders are
   merged, and installed.
7. **`wsl-toolkit-v7.1.0`**: every branch merged and gated, CI's Linux jobs in
   containers, three reviews, CI green, published, this host moved to it by
   `selfupdate`, the new behaviour driven, and the acceptance suite on a fresh
   instance.
8. **This host**, ruling 31: the package's copy emptied, the operator's
   `qemu-binfmt-restore.service` retired, the installed skills updated.
9. **Not this release**, ruling 31: `WSL-127` and `WSL-166`, exit codes that change, each a
   break.
10. **One base under a canonical name**, ruling 32: `WSL-167`, which removes
   `wsl-toolkit-base`. First after the pause, before the builders' merges.
11. **Blocked, each on what its entry names**: `WSL-134`, `WSL-135`, `WSL-138`,
   `WSL-158` and `WSL-159`.

## Open questions for the operator

1. **`WSL-159`, the old record's question 3.** Should the herdr service, and so
   every agent it starts, run in a private network namespace, at the cost of
   rootless containers for the base account? Nothing else waits on it.
2. **`WSL-138`, ruling 20.** Each official installer for `pi` and `omp` needs its
   digest approved, as ruling 4 did for Muse. The session measures each and asks
   with the numbers.
3. **`WSL-134` and `WSL-135`** need the operator at the desktop: a real terminal
   window, and three sign-ins.
4. **The base disks are not sparse.** `base reclaim` needs `wsl --shutdown`
   first, which stops every distribution, so it is the operator's step.
