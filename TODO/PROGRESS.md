# PROGRESS.md

The record: the state, the measured baseline, what this session did, what is in
progress, the work order and the open questions. ⛔ **Rewritten every session; it
carries no history**, and the record check refuses a section the model does not
have. [INDEX.md](INDEX.md) lists the entries, [RULES.md](RULES.md) holds the
standing rules and the rulings in force, and history is in git, the entries and
`docs/HISTORY/`.

## State

```text
session started 2026-10-21T08:31:38Z, on the operator's request to rewrite the record to the model, sync the methodology, refresh the skills and finish all WSL work
baseline        9784821, gate 24 of 24, 139,905 tracked lines, 600 mutation rows
head            this commit: the record rewritten, the methodology synced, 58 entries filed and 9 closed; seven builders running
entries         total 216  open 44  blocked 5  done 167
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
| the tool | `%USERPROFILE%\bin\wsl-toolkit.exe` 7.0.0, from `selfupdate`; the next `selfupdate` replaces it |
| distributions | `podman-machine-default`, stopped and WSL's default; `wsl-toolkit`, the jobs base, 58.4 GB; `wsl-toolkit-base`, the operator's base, 1.9 GB, rebuilt on 2026-10-21 with the herdr adapter alone. ⚠ Neither base disk is sparse |
| the podman connection | `podman-machine-default-root`, because the machine's rootless socket is dead (finding 76 of the old record). Undo with `podman system connection default podman-machine-default` once that socket works |
| the package's copy of the state directory | `%LOCALAPPDATA%\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Local\wsl-toolkit`, `WSL-118`: 503 job directories, a configuration, a `base.json` and a ledger. Ruling 31 empties it this session. The job that explains finding 8 is copied to `.tmp/finding8-4e94faa38b4c9c1c` |
| `wsl-toolkit-base` | runs `wsl-toolkit-binfmt.service` and the operator's own `qemu-binfmt-restore.service`; ruling 31 retires the second. It holds the test grant `.tmp/wsl78/proj` at `/workspaces/proj` |
| `wslc` | holds `archlinux:latest` and `alpine:latest` from `WSL-110`'s drive, beside two images that are the operator's. Remove the two by name; `wslc image prune` would take the operator's too |
| scratch under `.tmp` | herdr's source and builds and a verified nightly, about 1.1 GB, kept from 2026-09-17; each removable by its literal path |

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

- The methodology sync from TEMPLATE, `DOC-09`, by a background pass in this tree.
- Seven builders, each in its own worktree branch from `9784821`, by the table in
  the work order below. Each branch is merged, gated and driven here before its
  entries close.

## The work order, set by the operator on 2026-10-21

Ruling 31. The argument for the order is in [INDEX.md](INDEX.md).

1. **The record and the checks that hold it**, first, because every later commit
   is checked by them: `DOC-10`, `TOOL-29`, `TOOL-30`, `DOC-09`, `TOOL-31`,
   `TOOL-35`, `DOC-13`, `DOC-11` and `DOC-12`.
2. **The survey's asks**, in parallel by code area: jobs `WSL-120`, `WSL-121`,
   and the cap in `WSL-136` (F1); `run` and the configuration `WSL-122`,
   `WSL-123`, `WSL-128`, `WSL-129`, `WSL-131`, `WSL-161` (F2); `base exec`
   `WSL-124`, `WSL-125`, `WSL-133`, `WSL-146` (F4); the base's runtime
   `WSL-126`, `WSL-130` and the limits in `WSL-136` (F5a); the base's build
   `WSL-137`, `WSL-140`, `WSL-143`, `WSL-147`, `WSL-150`, `WSL-153`, `WSL-155`
   (F5b).
3. **The old record's open findings**: adapters `WSL-139`, `WSL-141`, `WSL-144`,
   `WSL-157` (F6a); fixes `WSL-149`, `WSL-151`, `WSL-152`, `WSL-154`, `WSL-156`,
   `WSL-160`, `WSL-162`, `WSL-163` (F6b); and here `WSL-132`, `WSL-142`,
   `WSL-145` and `WSL-148`.
4. **The tooling**: `TOOL-32`, `TOOL-33`, `TOOL-34`, `TOOL-36` and `TOOL-37`.
5. **The skills**, reviewed against what the tool does once the builders are
   merged, and installed.
6. **`wsl-toolkit-v7.1.0`**: every branch merged and gated, CI's Linux jobs in
   containers, three reviews, CI green, published, this host moved to it by
   `selfupdate`, the new behaviour driven, and the acceptance suite on a fresh
   instance.
7. **This host**, ruling 31: the package's copy emptied, the operator's
   `qemu-binfmt-restore.service` retired, the installed skills updated.
8. **Not this release**, ruling 31: `WSL-127`, the distinct exit code, which is a
   break.
9. **Blocked, each on what its entry names**: `WSL-134`, `WSL-135`, `WSL-138`,
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
