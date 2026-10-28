# PROGRESS.md

The record: the state, the measured baseline, what this session did, what is in
progress, the work order and the open questions. ⛔ **Rewritten every session; it
carries no history**, and the record check refuses a section the model does not
have. [INDEX.md](INDEX.md) lists the entries, [RULES.md](RULES.md) holds the
standing rules and the rulings in force, and history is in git, the entries and
`docs/HISTORY/`.

## State

```text
session started 2026-10-21T08:31:38Z, resumed 2026-10-24T01:18:23Z and 2026-10-26T20:40Z; it ends on wsl-toolkit-v9.0.1
baseline        9784821, gate 24 of 24, 139,905 tracked lines, 600 mutation rows
head            this commit: wsl-toolkit 9.0.1 published from 2225272, installed, driven and accepted
entries         total 267  open 0  blocked 0  done 267
this session    ruling 31, then rulings 32 to 36
```

## Baseline

Measured on this host unless a row says read.

| what | value | read from |
| --- | --- | --- |
| the gate | 24 checks, run at once; about 145 s a run on an idle host | `tools/check/main.go`; the last four pushes |
| mutation rows | 2,161 at `2225272` | `tools/repo/mutations.json` |
| CI | 20 jobs, 3 of them required; a `wsl-toolkit` push 752 to 780 s, its 12 shards at most 736 s | `gh run view` on the last five runs |
| the pre-push proof | ShellCheck, the go job and the added rows in containers, one at a time: 183 rows in about 9 minutes, one row in 147 s, none in 104 s. 106 of the 183 rows needed no compile | the last five pushes |
| the acceptance suite | 104 cases quick, 111 full; the full suite 998 s with the installed 9.0.1 on a fresh instance | `acceptance.ps1`; the last run |
| the last release | `wsl-toolkit-v9.0.1`, 22 assets, 19 release jobs green on a second attempt | `gh release view`, `gh run view` |
| this host | Windows 11 Pro 26300, PowerShell 7.6.8, WSL 3.0.3.0, git 2.55.0, Go 1.27.0 | the doctor |

### This host's state

| | what is there, and how to undo it |
| --- | --- |
| the tools | `%USERPROFILE%\bin\wsl-toolkit.exe` 9.0.1, from `selfupdate`, and `text-tool.exe` from the same release, each digest checked against `SHA256SUMS`. No previous copy is kept |
| distributions | `podman-machine-default`, stopped and WSL's default, its engine empty and its disk sparse. `wsl-toolkit-jobs`, the default base, with no grant, no job and no session; its disk sparse, the proof's cache and images removed after the last push. No scratch instance is registered |
| WSL's memory | `.wslconfig` caps the VM at `memory=16GB` and `swap=8GB`, with `autoMemoryReclaim=gradual`. The operator approved the cap after the VM reached 19.5 GB |
| the podman connection | `podman-machine-default`, the rootless one, is the default again |
| the package's copy of the state directory | absent. A Bash shell this desktop app starts recreates it when it runs the tool, `WSL-118` |
| the skills | the three the skills manager installed match `main`, and so does this host's profile copy of `wsl-toolkit` |
| herdr in the default base | one workspace, `work`, in the account's home. The muse-code guide's workspace in the demo grant is closed, and the grant revoked |
| scratch under `.tmp` | `check.exe`, `repo.exe`, `repo-pre-push.exe` and `consumer-wsl-toolkit.exe`, which the gate and the hooks rebuild; the empty `go-tmp`; `bsd67-driver`, kept by `WSL-67` |
| git | `main` alone, and one worktree, this checkout |

## This session

- ⭐ **Two releases**: `wsl-toolkit-v9.0.0`, a major, and `v9.0.1`. Each was published with
  22 assets, self-updated here and driven. 9.0.1's full acceptance suite passed 111 of 111 on
  a fresh instance.
- ⭐ **Ruling 36's leftovers are finished here**, with nothing handed back: `WSL-134`,
  `WSL-135`, `WSL-138`, `WSL-158`, `WSL-159`, `WSL-196` to `WSL-205` and `TOOL-40`. Each group
  was reviewed adversarially and repaired before it merged.
- ⭐ **The drives and the acceptance run found four more, and each is closed**: `WSL-206`,
  `DOC-16`, `TOOL-41` and `WSL-207`.
- ⭐ **This host is cleaned**: every scratch instance is removed, and so are the default base's
  jobs, sessions, demo grant, images and proof cache. Both WSL disks are sparse and trimmed.

## In progress

None. Every entry this session took on is closed, and the release that carries it is
published and installed here.

## The work order, set by the operator on 2026-10-21

Ruling 31. The argument for the order is in [INDEX.md](INDEX.md).

1. ⛔ **Hotfix `wsl-toolkit-v7.0.1`**: `WSL-168`, `WSL-169` and the first half of
   `WSL-137`. **Closed:** 7.0.1 shipped.
2. ⛔ **`wsl-toolkit-v7.0.2`, the audit's data-loss fixes**: `WSL-169`, `WSL-171` to
   `WSL-175`, `WSL-177`, `WSL-179`, `WSL-181`, `WSL-182` and `WSL-185`. **Closed:** 7.0.2
   shipped.
3. **The record and the checks that hold it**: `DOC-10`, `TOOL-29`, `TOOL-30`, `DOC-09`,
   `TOOL-31`, `TOOL-35`, `DOC-13`, `DOC-11` and `DOC-12`. **Closed:** all done.
4. **The survey's asks**: `WSL-120` to `WSL-126`, `WSL-128` to `WSL-131`, `WSL-133`,
   `WSL-136`, `WSL-137`, `WSL-140`, `WSL-143`, `WSL-146`, `WSL-147`, `WSL-150`, `WSL-153`,
   `WSL-155` and `WSL-161`. **Closed:** all in 8.0.0.
5. **The old record's open findings and the audit's rows**: `WSL-170`, `WSL-176`,
   `WSL-178`, `WSL-180`, `WSL-183` to `WSL-188`, `DOC-15`, `WSL-139`, `WSL-141`, `WSL-144`,
   `WSL-157`, `WSL-149`, `WSL-151`, `WSL-152`, `WSL-154`, `WSL-156`, `WSL-160`, `WSL-162` to
   `WSL-164`, `WSL-132`, `WSL-142`, `WSL-145` and `WSL-148`. **Closed:** all in 8.0.0.
6. **The tooling**: `TOOL-32`, `TOOL-33`, `TOOL-37`, `TOOL-38` and `WSL-165`.
   **Closed:** all done.
7. **The skills**, `DOC-14`, reviewed against the merged tool and installed.
   **Closed:** done and installed.
8. **The release**, planned as 7.1.0 and made 8.0.0 by ruling 34: every branch merged
   and gated, CI green, published, `selfupdate`, driven, the acceptance suite on a fresh
   instance. **Closed:** `wsl-toolkit-v8.0.0`, 104 of 104.
9. **This host**, ruling 31: the package's copy emptied, the operator's
   `qemu-binfmt-restore.service` retired with `wsl-toolkit-base`, the installed skills
   updated. **Closed:** all three.
10. **The exit codes**, ruling 35: `WSL-127` and `WSL-166`. **Closed:** both in 8.0.0.
11. **One base under a canonical name**, ruling 32: `WSL-167`, then `WSL-193`.
    **Closed:** both in 8.0.0.
12. **Unblocked by ruling 36**: `WSL-134`, `WSL-135`, `WSL-138`, `WSL-158` and `WSL-159`.
    **Closed:** all five, merged for 9.0.0.
13. **The 9.0.0 release**: `WSL-196` to `WSL-205` and `TOOL-40`, with the five above.
    `WSL-200` is a break, so the release is a major one. **Closed:** all of them, in 9.0.0.
14. **What the 9.0.0 drive and the acceptance run found**: `WSL-206`, `DOC-16`, `TOOL-41`
    and `WSL-207`. **Closed:** `WSL-206` and `DOC-16` in 9.0.1, `TOOL-41` by a dispatched
    herdr build, `WSL-207` by the full suite.

## Open questions for the operator

None. Ruling 36 settled the seven that were here, and each entry it touches carries its
decision. ⛔ Only a credential waits on the operator: the three sign-ins of `WSL-135`.
