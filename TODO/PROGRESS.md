# PROGRESS.md

The record: the state, the measured baseline, what this session did, what is in
progress, the work order and the open questions. ⛔ **Rewritten every session; it
carries no history**, and the record check refuses a section the model does not
have. [INDEX.md](INDEX.md) lists the entries, [RULES.md](RULES.md) holds the
standing rules and the rulings in force, and history is in git, the entries and
`docs/HISTORY/`.

## State

```text
session started 2026-10-21T08:31:38Z, resumed 2026-10-24T01:18:23Z; it ends on wsl-toolkit-v8.0.0
baseline        9784821, gate 24 of 24, 139,905 tracked lines, 600 mutation rows
head            this commit: wsl-toolkit 8.0.0 published from 5e0bdcf, installed and driven
entries         total 261  open 4  blocked 0  done 257
this session    ruling 31, then rulings 32 to 35
```

## Baseline

Measured on this host unless a row says read.

| what | value | read from |
| --- | --- | --- |
| the gate | 24 checks, run at once; 280 to 600 s a run with two builders loading the host | `tools/check/main.go`; the last eight runs |
| mutation rows | 1,821 at `5e0bdcf` | `tools/repo/mutations.json` |
| CI | 8 jobs, 3 of them required; a `wsl-toolkit` push 465 to 658 s, its 12 shards at most 526 s; a push of the record alone 358 s | `gh run view` on the last nine runs |
| the pre-push proof | ShellCheck, the go job and the added rows in containers, one at a time: 88 rows in 894 s, 173 in 408 s, none in about 230 s | the last eight pushes |
| the acceptance suite | 104 cases quick, 111 full; 750 s quick with the installed 8.0.0 on a fresh instance | `acceptance.ps1`; the last run |
| the last release | `wsl-toolkit-v8.0.0`, 22 assets, 19 release jobs green | `gh release view`, `gh run view` |
| this host | Windows 11 Pro 26300, PowerShell 7.6.7, WSL 3.0.3.0, git 2.55.0, Go 1.27.0 | the doctor |

### This host's state

| | what is there, and how to undo it |
| --- | --- |
| the tools | `%USERPROFILE%\bin\wsl-toolkit.exe` 8.0.0, from `selfupdate`, beside `.wsl-toolkit-previous-7.0.2.exe`. `text-tool.exe` from the same release, beside `.text-tool-previous.exe`. The next `selfupdate` removes the old tool |
| distributions | `podman-machine-default`, stopped and WSL's default; `wsl-toolkit`, the default base under its older name, 7.3 GiB, its disk not sparse. `wsl-toolkit-base` is unregistered with no copy, as ruling 33 says |
| WSL's memory | `.wslconfig` caps the VM at `memory=16GB` and `swap=8GB`, with `autoMemoryReclaim=gradual`. The operator approved the cap after the VM reached 19.5 GB |
| the podman connection | `podman-machine-default-root`, because the machine's rootless socket is dead (finding 76 of the old record). Undo with `podman system connection default podman-machine-default` once that socket works |
| the package's copy of the state directory | absent. A Bash shell this desktop app starts recreates it when it runs the tool, `WSL-118` |
| the skills | the three the skills manager installed match `main`, and so does this host's profile copy of `wsl-toolkit` |
| scratch under `.tmp` | build outputs and label files of this work, the empty `go-tmp`, `bsd67-driver` (kept by `WSL-67`) and `finding8-4e94faa38b4c9c1c` |

## This session

- ⭐ **Three releases**: `wsl-toolkit-v7.0.1` and `v7.0.2`, the critical and data-loss
  fixes, then `v8.0.0`, a major. Each was published, self-updated here and driven.
- ⭐ **8.0.0 merged nine groups of builders** after the F-groups, `WSL-167`, `WSL-193`
  and `WSL-131`. Each group was built in its own worktree, reviewed adversarially and
  repaired. Every break has its section in `docs/consumers.md`.
- ⭐ **CI proves every row in under 11 minutes**, where it took 23, `TOOL-39`. Its probes
  job then exposed three cases that two `go test` runs at once broke, and each is fixed.
- ⭐ **Rulings 32 to 35** settled the canonical base, its name, the major version and the
  exit codes. `WSL-196` to `WSL-199` are filed from the last drives.

## In progress

Ruling 36, from 2026-10-27: every leftover of 8.0.0 is finished here, with nothing handed
back. This host is cleaned: podman's machine pruned and compacted, the old default base
removed with its state, and `wsl-toolkit-jobs` built by the muse-code guide in order.
`WSL-197` and `WSL-134` are done. The guide's ordered pass found four faults, filed in
`WSL-135` and built with `WSL-138` for 9.0.0. Every branch but `TOOL-40` is merged.

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
13. **Next**: `WSL-197` on a stock host, then `WSL-198`, `WSL-199`, `WSL-196`, `WSL-200`
    `TOOL-40`, `WSL-201` and `WSL-202`, released as `wsl-toolkit-v9.0.0` with the five above. `WSL-200` is a
    break, so the release is a major one.

## Open questions for the operator

None. Ruling 36 settled the seven that were here, and each entry it touches carries its
decision. ⛔ Only a credential waits on the operator: the three sign-ins of `WSL-135`.
