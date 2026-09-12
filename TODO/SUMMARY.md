# SUMMARY.md

⚠ **A snapshot of one session, not an authority.**
[`PROGRESS.md`](PROGRESS.md) is what is true now; this is what the session that
wrote it measured on the day. A session that reads this and acts on it is acting
on what was true last time.

---

## 2026-09-12 checkpoint

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-12T11:00:00Z | checkpoint recorded 2026-09-12T14:18:33Z; final commit time follows from git |
| Commits | `d67c1e6`, dirty `main` with inherited issue-29 work | checkpoint changes still uncommitted when this snapshot was written |
| Work | issue 29 at its final flaky Linux test; issue 30 authored only | issue 29 locally complete; issue 30 provider-neutral base substantially built; zero-grant containment partially measured |
| Changes | inherited working tree | final file/line count measured after the record edit and reported in chat |
| Size | not measured at session start | not measured; no number invented |
| Checks | 19-check gate green at the inherited baseline | 19-check final gate green; live acceptance 71/71 and all Go suites green |
| Cost | no paid operation measured | network bytes not measured; no cost number invented |
| Health | dirty tree; issue 29 remotely open | disposable WSL instance removed; four unrelated distros preserved; tree awaits checkpoint commit/push |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. The code and live provider-base proof
are complete through the one-grant, zero-grant and common-bootstrap transitions.
The immediate remaining work is the final gate, commit/push, issue comments and
CI confirmation. After that, continue `WSL-67` with acceptance automation and
provider authentication, and `WSL-68` with an attacking threat-model probe.

---

## 2026-09-10, the third sitting

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-10T12:00:00Z | one context |
| Commits | `0c9c1f8`, clean `main`, no tag | 7 on `main`, pushed, CI green, and **two** releases: `wsl-toolkit-v2.0.1` and `wsl-toolkit-v2.0.2` |
| Work | 88 entries: 2 open, 0 blocked, 86 done | 93 entries: 1 open, 0 blocked, 92 done. 6 closed, 5 filed, 4 of those closed |
| Changes | 251 tracked files | 255; 35 changed, +2,617 / -366 lines |
| Size | 76,414 tracked lines | 77,914 at the end, +1,500. ⚠ The doc pass took 26 lines out of `scripts/README.md` and put 21 in `docs/HISTORY/scripts.md`; the record, the entries and two rulings' worth of code grew by far more than the manuals shrank. |
| Gate | 19 checks, 28.8s, green | 19 checks, green |
| Selftest | 157 over 40 functions | 157 over 40, same on 7.6.5 and 5.1. Unchanged: nothing in the script moved but its version. |
| Mutation | 76 rows, 75 proved on this host | 86 rows, every one of the ten new ones proved individually. ⚠ Two were THEATRE on the first attempt and the harness said so. |
| Selfupdate | never driven this session | ⭐ driven from the PUBLISHED 2.0.1 to 2.0.2, digest checked independently against the published sums, `--check` changed nothing |
| Consumer | 14 cases, 8 skipped against v2.0.0 | 14 cases, 0 failed, 6 skipped against v2.0.1. The two that moved are the signature cases, which had never run green. |
| Acceptance | 71 of 71, last session | ⛔ **not re-run.** The changed Go path was driven directly instead. That is a gap, named rather than counted as a pass. |
| Release | `wsl-toolkit-v2.0.0`, five assets, none signed | `wsl-toolkit-v2.0.1`, ten assets, five signature bundles, verified in the release job and from outside by the consumer suite |
| Health | 3 debts owed: PR 15, the tag, `WSL-30` | all three cleared. 3 filed and 2 ruled by the operator and built the same session; `WSL-59` is the one entry left. ⚠ One fix is on `main` and not in `v2.0.2`. Tree clean. |

## What was asked, and what happened

| asked | outcome |
| --- | --- |
| finish the remaining TODO work, including debts only alluded to | done. `WSL-25` and `WSL-30` closed; `TOOL-22` and `WSL-62` found and closed; `WSL-61` gave a two-session-old open question its entry at last |
| update and merge pull request 15, then fix the stale `tags/v6` comment | done. Squashed with a clean message, and both comments now name `v7.0.0` and the date they were re-resolved |
| three deep reviews, then cut the fat from the docs | done, and a fourth. The door sweep, the guard mutation and the claim audit each found something; the concurrency lens three sessions had owed found `WSL-62` |
| start `WSL-30` by running its matrix, designing nothing first | done. Sixteen claims measured on two engines, nothing designed, and the split its own decision pre-authorised was taken |
| bump to 2.0.1, rebuild, cut the release, close `WSL-25` | done. The signing path ran for the first time and did not go red |
| end with the release, lean docs, clean repo, green CI | done |
| rule `WSL-60` toward whatever covers the most, given a systemd or KVM future | done. Neither seam as written: the mechanism is measured and NAMED, so a base that later gains systemd, a rootful engine or a whole virtual machine answers through the same row |
| take the `WSL-61` recommendation and extend it so agents are told loudly | done. `--repair`, an ALREADY EXISTS line, and every condition a value carrying an id, a cost and the exact command, in `--json` as well as in the terminal |

## What each review pass found

⛔ **Three headings over one sweep is not three passes.** Each of these names
what it looked at that the others did not.

| lens | looked at | found |
| --- | --- | --- |
| door sweep | every caller of the changed helper, then every other outbound HTTP door in the tree | the fix reaches all five callers, and nothing else in the tree can silently become a write. It also found a pin example still naming the unsigned release, on the page that says signing exists |
| guard mutation | the new guards, by removing what each protects | both went red. One case was weaker than its name, failing on an index shift rather than on aliasing, and was rewritten |
| claim audit | every standing fact and count in the live pages, against the API and the machine | `repo mutate` carried a duration with no conditions, off by more than half; a README claimed "the ten already here" against 60 rows; the router said four assets against a release of ten |
| concurrency, the sixth | what two of this tool do to one state directory | `WSL-62`, on the first pass, measured rather than argued |
| ⭐ the driven pass, on the rulings | the repair actually running in the guest | a defect INSIDE the fix: it read podman's run directories from `$XDG_RUNTIME_DIR`, which is WSLg's here, and would have removed nothing and reported success |
| second source | every capability claim, re-derived by a different route than the one that produced it | all four held. ⚠ And a trap for later: the cgroup tree is mounted `rw,nsdelegate` and is still `root:root 555`, so both cheap reads say "delegated" and both are wrong |
| failure paths | what happens when each new thing fails | the capability probe was an unbounded second container on the health path, and `ready` answered a failed `base ensure` with `base ensure` |
| reachability | what was added that nothing reaches | every symbol has callers; what it found was a producer with no consumer, the helper dropping the state on an error on BOTH sides |
| ⭐ running `ready` | the command an agent runs first | ⛔ a regression this session had just introduced: a working machine reported `not-ready` |
| ⭐ running `selfupdate` | 2.0.1 to 2.0.2, for real | it works, and it makes one claim it does not keep about removing the superseded copy |
