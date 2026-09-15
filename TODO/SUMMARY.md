# SUMMARY.md

⚠ **A snapshot of one session, not an authority.**
[`PROGRESS.md`](PROGRESS.md) is what is true now; this is what the session that
wrote it measured on the day. A session that reads this and acts on it is acting
on what was true last time.

---

## 2026-09-15, the two presets that would not build, and three entries closed

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-15T03:12:15Z | ended at the record commit's own time, about two hours |
| Commits | `deea680`, clean `main`, its CI run 34923941438 still in progress | `git log --oneline deea680..HEAD` read **3**: `ef7ad79`, whose CI run 34931576687 failed one mutation row as THEATRE; the record commit; and the fix for that row. CI read after each push |
| Work | 9 open entries; `WSL-87` partial and owing its reviews, `WSL-86` proposed and unruled, `WSL-70` partial on two presets that did not build | **Completed 3:** `WSL-87`, `WSL-86` (filed, ruled and closed in this session) and `WSL-70`. **Partial 1:** `WSL-67`, still waiting on the two BSD downloads. **Deferred 1:** `WSL-71`. **Failed 0.** Entries 122, open 9 to 6, done 113 to 116 |
| Changes | 0 files changed from `deea680` | `git diff --shortstat deea680 HEAD` read 12 files, +1,095 / -53 over the three commits |
| Size | 86,824 text lines in 290 tracked files at `deea680`, `git grep -I -c ''` | 87,866 in 291 files, +1,042 |
| Checks | doctor exit 0 in 25.85 s; gate 20 of 20 in 32.19 s | gate 20 of 20 before every commit; the Windows Go proof with the 8.3 `TEMP` 313 results, 298 passed, 15 skipped, 0 failed; `check-go.sh` exit 0 in `golang:1.25`; ShellCheck 0.9.0 clean over 34 scripts; 3 new mutation rows red in `golang:1.25` and one defect planted by hand red. ⛔ CI failed one of those rows as THEATRE on `ubuntu-latest`, which carries a `getcap` the case needs absent; split into its own case, which skips there |
| Cost | no paid operation authorized; network bytes not measured | no paid operation. Network bytes not measured; known transfers: seven throwaway base builds (2 arch, 2 alpine, 2 debian, 4 fedora attempts), two full 13-image agent matrices with CodeGraph on, and about ten short container runs in `golang:1.25`, `ubuntu:24.04` and `debian:latest`. No BSD image |
| Health | four distributions; two of four base presets did not build | ⭐ all four presets build and verify. ⛔ Found on `main` and not fixed: the automount sweep is a race, recorded. The same four distributions and no throwaway; the FreeBSD image not booted; the SSH configuration unchanged; no tag; CI run 34932875176 green in all six jobs on `12705a1`; tree clean after this commit |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| close `WSL-87` with its three reviews | done. Its door sweep also found and fixed the kernel it never checked, and its guard mutation found a false green in its own method |
| get the ruling on `WSL-86` and act on it | done: approved in chat as recommended, filed, fixed, proved and closed |
| close `WSL-70` on four preset builds | done. Fedora is proved at `toolset none` and the other three at `developer`, and the entry says so |
| drive `pkgin` on NetBSD and `pkg_add` on OpenBSD | ⚠ **not started:** the two downloads were asked for once more at this session's start, in chat and as a file, and not approved |
| then `WSL-71` | not started |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. `WSL-67`'s BSD drives, once the downloads are
approved, then `WSL-71`.


## 2026-09-15, the provider profiles, the shared table and three findings

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-15T00:39:50Z | ended at the record commit's own time, about two hours and thirty minutes; the operator checkpointed it at `WSL-87` |
| Commits | `45fc7cc`, clean `main`, CI run 34869369358 green | `git log --oneline 45fc7cc..HEAD` read **3**, each pushed and green in CI, runs 34917175480, 34918661319 and 34921726479; then the record commit |
| Work | 7 open entries; `WSL-67` resumed at `pkgin`, `pkg_add` and the acceptance runner's cases, then `WSL-70` and `WSL-71` | **Completed 1:** `WSL-85`, found and approved this session. **Partial 3:** `WSL-67`, its acceptance cases built and its BSD drives waiting for the download approval; `WSL-70`, built, with two presets not building; `WSL-87`, found, approved and built, its reviews owed. **Deferred 1:** `WSL-71`. **Failed 0.** Proposed and not ruled: `WSL-86`. Entries 119, 7 open, to 121, 8 open, 113 done |
| Changes | 0 files changed from `45fc7cc` | `git diff --shortstat 45fc7cc` over the tree before this summary read 21 files, +1,436 / -153; the record commit adds this section |
| Size | 85,510 text lines in 288 files at `45fc7cc`, `git grep -I -c ''` | 86,793 in 290 files, +1,283, before this section |
| Checks | doctor exit 0 in 37.62 s; gate 20 of 20 in 37.47 s | gate 20 of 20 before every commit; the Windows Go proof with the 8.3 `TEMP` and the Linux Go proof in `golang:1.25` exit 0 before every push; ShellCheck 0.9.0 clean over 34 scripts; 7 new mutation rows red and 2 re-pointed rows red; the acceptance runner 96 of 96 |
| Cost | no paid operation authorized; network bytes not measured | no paid operation. Network bytes not measured; known transfers: the packages for 11 throwaway base builds, 6 Arch, 1 Alpine, 2 Debian and 2 Fedora, Fedora's mirror answering in KiB/s; three 13-image agent matrices, two with CodeGraph; npm 7.17.0 and 7.18.0 through `npx`; Rocky 8's nodejs; no BSD image |
| Health | four distributions; the shared FreeBSD image the published one | ⛔ found on `main` and not fixed: the debian and fedora presets do not build, `WSL-86` proposed. ⭐ `bootstrap.sh`'s CodeGraph install works under dash again. The same four distributions and no throwaway; the FreeBSD image's SHA-256 unchanged and not booted; the SSH configuration unchanged; no tag; tree clean after the record commit |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| drive `pkgin` on NetBSD and `pkg_add` on OpenBSD | ⚠ **not started:** the two downloads were asked for in chat at the start, once, and not approved. The boot loader's route to the serial console was measured, and a driver written under `.tmp`, never booted |
| add the provider-profile scenarios to the acceptance runner | done: five cases, red over a planted `WSL-85` defect, and a full run of 96 of 96 |
| close `WSL-67` with its three reviews | not done: it waits for the BSD drives |
| `WSL-70` and `WSL-71` follow | `WSL-70` built and partial, its four-preset prove blocked by two presets that did not build before it either; `WSL-71` not started |
| rule on what driving found | `WSL-85` approved and closed; `WSL-87` approved, built and proved by hand; `WSL-86` proposed and not yet ruled |
| checkpoint here and print the resume prompt | done: the record, this summary and the resume prompt in chat only |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. Close `WSL-87` with its reviews, then ask again
for `WSL-86` and the two downloads, and continue down the order the record gives.

---

## 2026-09-14, the safety entries and the Nix route

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-14T14:15:42Z | ended at the record commit's own time, about two hours and fifteen minutes; the operator checkpointed it at `WSL-67` |
| Commits | `7122216`, clean `main`, CI run 34846522263 green | `git log --oneline 7122216..HEAD` read **3**, each pushed and green in all six CI jobs, runs 34860407687, 34863266828 and 34865346058; then the record commit |
| Work | 10 open entries; `WSL-84`, `WSL-83`, `WSL-82`, then issue 30's `WSL-67`, `WSL-70` and `WSL-71` assigned | **Completed 3:** `WSL-84`, `WSL-83`, `WSL-82`. **Partial 1:** `WSL-67`, Soar removed and Nix driven, with `pkgin`, `pkg_add` and the acceptance runner's cases open. **Deferred 2** by the checkpoint: `WSL-70`, `WSL-71`. **Failed 0.** Entries 119, open 10 to 7, done 109 to 112 |
| Changes | 0 files changed from `7122216` | `git diff --shortstat 7122216` over the tree before this summary read 20 files, +2,049 / -424; the record commit adds this section |
| Size | 83,853 text lines in 288 files at `7122216`, `git grep -I -c ''` | 85,478 in 288 files, +1,625, before this section |
| Checks | doctor exit 0 in 21.17 s; gate 20 of 20 in 46.33 s | gate 20 of 20 before every commit; the Windows Go proof with the 8.3 `TEMP` and the Linux Go proof in `golang:1.25` exit 0 before every push; ShellCheck 0.9.0 clean over 34 scripts; 29 new or changed mutation rows red in `golang:1.25` |
| Cost | no paid operation authorized; network bytes not measured | no paid operation. Network bytes not measured; known transfers: `docker.io/nixos/nix:latest`, 161,500,661 bytes compressed, once, and Nix packages for four accounts; Arch packages for 15 throwaway base builds, 2 of them stopped by a stalled mirror, and 21 provisioning runs over a built one; 196,079,616 bytes of the NetBSD image and 53,981,184 of the OpenBSD image before the checkpoint stopped both |
| Health | four distributions; the shared FreeBSD image restored and unbooted | ⭐ the shared FreeBSD image booted eight times through overlays with its SHA-256 unchanged; the same four distributions; no throwaway, image copy, partial download or Nix image left; the SSH configuration unchanged; no tag; tree clean after the record commit |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| read `docs/AGENTS.md` and reconcile the last session's unfinished authoring | done: `7122216` was pushed and green and carried the order and the rulings; only its next prompt had not been printed |
| execute the work order in its order | `WSL-84`, `WSL-83` and `WSL-82` closed, each driven on this host, reviewed and green in CI; `WSL-67` reached its Nix work |
| cover the operator's own Nix setup, and do it better | built as ruled in chat: an installed Nix is found and used, flakes are set up, the allow variables always on, a token through `NIX_CONFIG` only; `install_nix.sh` read and not run |
| approve the BSD image and Nix image downloads | approved in chat; the Nix image used and removed; both BSD downloads stopped at the checkpoint and removed |
| finish the task in flight, run the end of the session and print a resume prompt | done: the Nix work committed with the record, and the resume prompt in chat only |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. Resume `WSL-67` at driving `pkgin` and
`pkg_add`, after asking the operator again in chat for the two image downloads, then
the acceptance runner's provider-profile cases and the entry's closing.

---

## 2026-09-14, work-order correction

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-14T12:35:13Z | ended at the record commit's own time |
| Commits | `08e23bc`, clean `main`, CI run 34837219729 green | one record commit, pushed after this table is written |
| Work | 10 open entries; Muse and herdr first in the recorded order; three entries waiting for rulings | **Completed 1 authoring task, deferred 10 open entries, failed 0.** `WSL-82`, `WSL-83` and `WSL-84` approved. `WSL-84`, `WSL-83`, `WSL-82`, issue 30, the sealed base, Muse and herdr, then the release is the new order. No entry was implemented. `WSL-67` now requires removal of Soar; Nix remains and is driven |
| Changes | 0 files changed from `08e23bc` | 3 files, +133 / -94 |
| Size | 83,814 tracked text lines at `08e23bc` | 83,853, +39 |
| Checks | doctor exit 0; baseline gate 20 of 20; CI green | focused docs, markers, record and one-home checks green; Windows Go proof green with the 8.3 temporary path; Linux Go proof green in `golang:1.25`; ShellCheck 0.9.0 green in `ubuntu:24.04`; final gate 20 of 20 |
| Cost | no paid operation authorized; network bytes not measured | no paid operation; network bytes not measured |
| Health | tree clean; four registered distributions matched the record; the shared FreeBSD image remained restored and unbooted | no WSL distribution or base changed; no tag; tree clean after the record commit and CI checked after its push |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| restore the unfinished safety and correctness work ahead of Muse and herdr | done in the record and entries; Muse and herdr remain one later session |
| settle the pending decisions | done: `WSL-84` first, `WSL-83` option A, `WSL-82` option B, and the release waits for both P1 entries |
| remove Soar and keep Nix | recorded in `WSL-67`; implementation is deliberately deferred with the entry |
| update and push the plan without implementing it | done by this record commit after the required gates |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. Implement `WSL-84` and close it with its
prove and three reviews. Continue through the work order only after that commit is
green.

---

## 2026-09-14, the third sitting

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-14T09:18:24Z | ended at the record commit's own time, about two hours; the operator checkpointed it at about 11:00Z |
| Commits | `eff07c5`, clean `main`, CI run 34820474937 green | `git log --oneline eff07c5..360bbde` read **5**, all pushed, CI green in all six jobs on the first four and `360bbde`'s mutation job still running at the record commit; then the record commit |
| Work | the order's open items: `WSL-81` partial, `WSL-72`'s prove, `WSL-76`, `WSL-77` and `WSL-78` of issue 32, the four of issue 30, the base and the release | **Completed 3:** `WSL-81`, `WSL-72`, `WSL-77`. **Partial 2:** `WSL-78`, its entry point built and driven, its prove and guide waiting for a signed-in Muse; `WSL-71`, its premise measured and nothing built. **Deferred 6** by the operator's checkpoint: `WSL-76`'s closing, `WSL-67`'s open items, `WSL-68`, `WSL-70`, the base and the release. **Failed 0.** Filed 3: `WSL-82`, `WSL-83`, `WSL-84`. Entries 116 with 10 open to 119 with 10 open |
| Changes | 0 files changed from `eff07c5` | `git diff --shortstat eff07c5 360bbde` read 24 files, +2,370 / -163; the record commit adds its own |
| Size | 81,440 text lines in 280 files at `eff07c5`, `git grep -I -c ''` | 83,647 in 288 files at `360bbde`, +2,207, before the record commit |
| Checks | doctor exit 0; gate 20 of 20 in 46.78 s | gate 20 of 20 before every commit and on the record commit; Go suites green on Windows with the 8.3 `TEMP` and in `golang:1.25` before every push; ShellCheck 0.9.0 clean. Mutation rows 235 to 250, each new one red after its case passed unmutated, and `WSL-81`'s 7 and `WSL-72`'s 4 proved again one at a time |
| Cost | no paid operation authorized; network bytes not measured | no paid operation and no Muse prompt. Network bytes not measured; the largest transfers known: Muse's 299,251,896-byte build twice, into `m77` and `m78`; the `languages` packages once, into a FreeBSD copy; ShellCheck's packages in six containers |
| Health | the shared FreeBSD image at 12 GiB with its 500-package baseline; four distributions; no throwaway | ⛔ the shared image panicked twice at poweroff and is the published 6.0 GiB image again; `WSL-83` and `WSL-84` filed at P1; the same four distributions, no throwaway, and the operator's SSH configuration and `bin` directory unchanged; no tag; tree clean after the record commit |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| resume unattended, and work the order through issue 32, issue 30, the base and the release | ⚠ **partly.** `WSL-81`, `WSL-72` and `WSL-77` closed, `WSL-78` built as far as a sign-in allows; issue 30 reached `WSL-71`'s measurement before the operator's checkpoint |
| when a step needs the operator, print exact commands, keep working and poll | three rulings asked for in chat, `WSL-82`, `WSL-83` and the release, and none answered; no credential needed |
| wrap up, make Muse and herdr together the next session's one task, and the sealed base a session after it | written into `WSL-76` and the order, with the reviews run and the rest of the protocol |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. The next session builds `wsl-toolkit-base`,
has the operator sign Muse in, and closes `WSL-76` and `WSL-78` with Muse used
directly, through herdr, and through `pi` or `omp` in herdr.

---

## 2026-09-14, resumed session

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-14T06:44:07Z | ended at the checkpoint commit's own time, after the operator stopped the session for budget |
| Commits | `24fd159`, clean `main`, CI run 34814120051 still finishing | no intervening commit; this checkpoint commit is pushed after the table is written. Run 34814120051 finished green in all six jobs |
| Work | issues 30 and 32 open; `WSL-81`'s one-vCPU runs unstarted | **Completed 0, partial 1, deferred 9 open entries, failed 0.** `WSL-81`: empirical proof and default complete; seven narrow guard mutations, claim audit and closing remain. Issues 30 and 32 remain open; no release |
| Changes | 0 files changed from `24fd159` | 9 files, +138 / -26 before this same-line measurement replaced its placeholder |
| Size | 81,328 tracked text lines at `24fd159` | 81,440, +112 before this same-line measurement replaced its placeholder |
| Checks | doctor exit 0; start gate 20 of 20 outside the filesystem sandbox in about 76 s | Windows Go proof green with the 8.3 temporary path; Linux Go proof green in `golang:1.25`; ShellCheck 0.9.0 clean in `ubuntu:24.04`. The end gate first refused two full digests in tracked records; they were abbreviated, then all 20 checks passed; the exact-byte rerun also passed |
| Cost | no paid operation authorized; network bytes not measured | no paid operation and no Muse prompt. Network bytes not measured: five FreeBSD language package transactions and two container proofs ran. Five image-copy directories totalling 67,755,936,860 bytes were deleted; the shared image grew by 6,408,263,680 bytes |
| Health | the shared FreeBSD image was the restored 6,476,638,208-byte published image; four registered distributions; no throwaway | shared image healthy at 12,884,901,888 bytes with its 500-package baseline intact; the same four distributions; every image copy removed; no QEMU or toolkit process left. WSL-81 remains explicitly partial; tree clean after the checkpoint commit |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| resume unattended from the record and complete the remaining work | ⚠ **partly.** Startup obligations were completed. `WSL-81`'s five-run hypothesis and shared-image proof completed, and the safer default was built. The operator then requested a checkpoint because the session budget ended |
| when a step needs the operator, print exact commands, keep working and poll | no operator action or credential was needed in this resumed session |
| end with the gate, record, saved and printed summary, and a resume prompt printed only in chat | done by this checkpoint; the resume prompt is not written to the tree |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. Finish `WSL-81`'s seven narrow guard
mutations and claim audit, close it on a green gate, then continue at `WSL-72`.

---

## 2026-09-14

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-14T02:01:53Z | ended at the record commit's own time, about four hours and forty minutes, one context and one summarised continuation. The operator checkpointed it |
| Commits | `e73d7d5`, clean `main` | `git log --oneline e73d7d5..0143318` read **7**, all pushed; CI green on the first six, the seventh running at the record commit; then the record commit |
| Work | issues 30, 32 and 33: ten entries, the base and the release | **Completed 4:** `WSL-74`, `WSL-80`, `WSL-79` with issue 33 closed, and `WSL-75`. **Partial 3:** `WSL-72`, built with its prove unrun because the guest panics; `WSL-76`, built and driven, its closing waiting for Muse; `WSL-81`, built, its one-vCPU runs unstarted. **Not started 6:** `WSL-77`, `WSL-78`, `WSL-67`, `WSL-68`, `WSL-70`, `WSL-71`; `wsl-toolkit-base` not built and `wsl-toolkit-v3.0.0` not cut. Failed: 0. Entries 114, 12 open, to 116, 10 open, 106 done |
| Changes | `e73d7d5` | `git diff --shortstat e73d7d5 0143318` read 48 files, +5,386 / -272. ⚠ Stamped; the record commit adds to it |
| Size | 76,179 text lines in 262 files at `e73d7d5`, `git grep -I -c ''` | 81,293 in 280 files at `0143318`, +5,114 |
| Checks | the gate exit 0, 19 checks, 44.68 s | the gate exit 0, 20 checks with the new `adapters` rule, 48.9 s at 06:30Z. Go suites green on Windows with an 8.3 `TEMP` and in `golang:1.25`; ShellCheck 0.9.0 in `ubuntu:24.04` clean; acceptance 91 of 91 at `5e66e44`, not re-run since. Mutation rows 192 to 234, each new one red when planted |
| Cost | not measured | no paid operation, and no Muse prompt. Network bytes not measured: herdr 0.9.0's 24.6 MB Linux asset downloaded three times into a throwaway base; about 16 BSD runs each fetched the `languages` packages, which one run's cache measured at 335,828 KiB; container images and the FreeBSD archive were already local |
| Health | issues 30, 32 and 33 open; `wsl-toolkit-muse` present | ⭐ issue 33 closed with the commits named. `WSL-81` filed and open: the FreeBSD guest panicked 8 times in 14 heavy runs and the prove, under every CPU model tried. ⭐ `wsl -l -v` reads the start's distributions less `wsl-toolkit-muse`, which the operator removed; every throwaway, test directory and image copy removed; the user's SSH configuration byte for byte as found. No tag. Tree clean after the record commit |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| settle every decision only the operator can make, then work unattended | done first: every ruling written into its entry, `cf274eb` |
| close issues 30, 32 and 33 properly, each entry driven, reviewed and closed with evidence | ⚠ **partly.** Issue 33 closed; `WSL-74` and `WSL-75` of issue 32 closed; `WSL-76`'s machinery built; issue 30 not started |
| end with the gate, the record, a summary and a resume prompt, and cut `wsl-toolkit-v3.0.0` | the gate green, this record, and the resume prompt in chat; ⛔ the release not cut, because the ruling waits for the three issues to close |
| checkpoint here and prepare a resume prompt | done |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. `WSL-81`'s one-vCPU runs come next,
because `WSL-72`'s prove depends on a guest that does not panic.

---

## 2026-09-13, the third session

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-13T14:24:42Z | ended at the record commit's own time, about two and a half hours, one context and one summarised continuation. The operator ended it for budget |
| Commits | `fc30c6c`, with 13 uncommitted paths inherited | `02ea58d`, pushed, CI run 34767124644 green on all six jobs; then the record commit |
| Work | four steps assigned | **Completed 2:** step 1, `WSL-73` closed; step 2, `WSL-74` to `WSL-79` filed and `WSL-67` reconciled. **Cut short by the operator, 1:** step 3, two doc fixes of a full review. **Step 4:** this record. Failed: 0. Entries 108, 6 open, to 114, 12 open, 102 done |
| Changes | `fc30c6c` | `git diff --shortstat fc30c6c 02ea58d` read 41 files, +2,280 / -343. The record commit's `git diff --shortstat`, before this row was written, read 6 files, +728 / -80. ⚠ Stamped |
| Size | 73,594 text lines at `fc30c6c`, `git grep -I -c ''` | 75,531 at `02ea58d`, +1,937, before the record commit |
| Checks | the gate exit 1, 2 problems over 19 checks, 40.88 s | the gate 19 of 19 in 26.5 s at 16:42:08Z on the record change, after one `one-home` fix. Go suites green with an 8.3 `TEMP`; ShellCheck 0.9.0 in `ubuntu:24.04` clean; acceptance 90 of 90 on a build of `02ea58d` |
| Cost | not measured | no paid operation. Network bytes not measured: container images, herdr's documentation and Meta's two installer scripts read, nothing of either installed |
| Health | pull request 31 closed with `WSL-73` checkpointed, and issues 30, 32 and 33 with no entries | ⭐ `WSL-73` closed and pull request 31's comments name the commits. Issues 32 and 33 amended in place, and each item mapped to an entry. `wsl -l -v` reads the same five distributions. ⛔ `wsl-toolkit-v3.0.0` not cut, the operator's call. Tree clean after the record commit |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| finish pull request 31's work as `WSL-73`: parity and better, suites green, docs lean, three deep reviews, CI green | done: four reviews, 27 findings fixed, 90 of 90 acceptance, CI green |
| read issues 30, 32 and 33, reconcile the open entries and author the entries that resolve them, implementing none | done: six entries, and `WSL-67` reconciled. One defect found while grounding, `WSL-74`, was measured with a read-only command |
| swap Zellij for herdr in issue 32 | recorded as a ruling in `WSL-76` and carried by `WSL-77` and `WSL-78`; ⚠ not implemented, as step 2 required |
| amend issues 32 and 33, update the docs, and end the session | done, with step 3's review cut to two fixes |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. Its work order closes issues 30, 32 and
33, and five of the new entries wait on a decision the operator has not ruled.

---

## 2026-09-13, the second session

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-13T05:24:47Z | ended at the record commit's own time, about one hour; the operator stopped the session at the checkpoint |
| Commits | `05c1703`, clean `main` | `37b1f89`, pushed, CI green on all six jobs with 124 of 124 mutation rows proved; then the record commit |
| Work | step 1 of four assigned: `WSL-73` | **Completed 0. Checkpointed 1:** `WSL-73`, reviewed and measured, native commands written and unit-proved, not driven. **Deferred by the operator:** steps 2 to 4, to the next session. Failed: 0. Entries unchanged at 108: 7 open, 0 blocked, 101 done |
| Changes | 279 tracked files | `git diff --shortstat 05c1703 37b1f89` read 20 files, +3,080 / -167. ⚠ Stamped at `37b1f89`; the record commit adds to it |
| Size | 85,989 text lines in 279 files at `05c1703`, `git grep -I -c ''` | 88,902 in 286 files at `37b1f89`, +2,913 |
| Checks | 20 checks green in 36.0 s | 20 checks green in 39.2 s at 06:16:23Z on the staged checkpoint. Go suites green on Windows with `TEMP` at an 8.3 path and on Linux in `golang:1.25`. CI's ShellCheck 0.9.0 in `ubuntu:24.04`: 25 of 25 clean. 11 new mutation rows red when planted. ⛔ The gate's `powershell` check was found unable to fail, so `acceptance.ps1` and `consumer.ps1` were parsed by hand: 0 errors |
| Cost | not measured | no paid operation. `docker.io/library/golang:1.25` pulled into the base, bytes not measured |
| Health | pull request 31 open and unreviewed | ⭐ pull request 31 closed unmerged at 06:19:43Z with the review and decisions in [a comment](https://github.com/Azathothas/ToolKit/pull/31#issuecomment-5651629772). Two gate defects found and not filed. ⛔ Native `distro` and `hostaddress` on `main` undriven. `wsl -l -v` as at the start; no distribution created; scratch worktree removed. Tree clean after the record commit |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| review pull request 31 trusting nothing, adopt what survives, delete the PowerShell product, close the pull request | ⚠ **partly.** Reviewed and measured; nothing merged as written; the native replacement written on `internal/toolkit`; the deletion not started |
| checkpoint here, close the pull request with the current decisions, and print a prompt for a session that continues and handles the rest of the issues | done: `37b1f89` on green CI, the pull request closed with the decisions on it, and the resume prompt printed in chat |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first, then `WSL-73`'s checkpoint section in
[`wsl-toolkit-go.md`](wsl-toolkit-go.md).

---

## 2026-09-13

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-13T02:36:12Z | ended at the record commit's own time; one context and one summarised continuation |
| Commits | `e9f0e08`, with 19 uncommitted `WSL-69` paths inherited | `git log --oneline e9f0e08..HEAD` read **4** at `2656a0d`, all pushed and CI green on all six jobs each, then the record commit. ⚠ Stamped, because a record edit is itself a commit |
| Work | 104 entries: 7 open, 0 blocked, 97 done | 108 entries: 7 open, 0 blocked, 101 done. **Completed 4:** `WSL-69`, `TOOL-23`, `TOOL-24`, `TOOL-25`, three of them found and filed this session. **Checkpointed 2 and still open:** `WSL-72`, `WSL-70`. **Filed 1:** `WSL-73`. **Not started, deferred by the operator's redirect:** `WSL-71`, `WSL-67`'s remaining list, `WSL-68`. `WSL-59` untouched. Failed: 0 |
| Changes | 270 tracked files | 279. `git diff --shortstat e9f0e08 2656a0d` read 47 files, +2,795 / -187. ⚠ Stamped at `2656a0d`; the record commit adds to it |
| Size | 83,285 text lines | 85,893 at `2656a0d`, +2,608, counted with `git grep -I -c ''` at each revision |
| Checks | local gate RED: 3 problems over 19 checks, 42.4 s | 20 checks green in 31.6 s. CI's ShellCheck 0.9.0 in a container: 25 of 25 scripts clean. The Go suites green with an 8.3 `TEMP` on a clean clone. Mutation rows 90 to 113, each new row planted alone and red |
| Cost | not measured | No paid operation by this repository. ⚠ Muse's two driven prompts ran on the operator's Meta account, and their cost was not measured. Downloads: Muse's installer reported 273 MB; FreeBSD packages, four container images and one BSD image restore were not measured in bytes. ⭐ **At least 8 `bsd run` sessions**, counted from the console logs saved, so a run with no saved log is missing from it |
| Health | the Muse base built for `toolkit`, sudo not live, no Muse | ⭐ Muse installed, signed in and driven; the base healthy with no drive mount points and no Zellij session left. The FreeBSD image restored to its published 500 packages on a 10 GiB disk, residue named in `WSL-72`. ⭐ `wsl -l -v` reads the same five distributions in the same states as at 02:45:39Z, with `wsl-toolkit-muse` rebuilt in between. Tree clean after the record commit. ⛔ Issue 30 still open |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| read `docs/AGENTS.md` and issue 30 in full, then work unattended until every task is done | ⚠ **partly.** `WSL-69` closed and three gate defects closed; the operator then redirected the session to checkpoint and stop |
| poll until the Muse CLI is authenticated, and give the operator exact commands | done. The operator installed and signed in; the agent drove everything else |
| close the task in flight, update the record and amend the issue 30 comment in place | done: `WSL-69` closed on green CI, and the issue comment amended |
| file pull request 31 for the next session, which reviews it trusting nothing, adopts what is useful, deletes the PowerShell product, closes the pull request, then reads issues 30, 32 and 33 and turns them into entries | filed as `WSL-73`, and [`PROGRESS.md`](PROGRESS.md)'s work order carries the rest in the operator's order |
| ask the two open questions and record the answers | done. `WSL-72`: the default disk rises to whatever whole GiB, 12 or 13, gives a true 10 GiB root, measured. The `bsd run` line join: posted as a comment on issue 33 instead of an entry |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. Its work order is the operator's, and it
starts with pull request 31.

---

## 2026-09-12, the second sitting

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-12T14:40:00Z | one context |
| Commits | `fcca2ba`, clean `main`, ⛔ CI RED on it in three jobs | `git log --oneline fcca2ba..HEAD | wc -l` read **12** at `bfb4c5a`, all pushed. ⚠ Stamped rather than current, for the reason in the row below: a record edit is itself a commit, so only a stamped count stays true. ⭐ Every run that completed is green; ⚠ two were CANCELLED, which is GitHub dropping a PENDING run superseded by a newer push, not a failure |
| Work | 100 entries: 3 open, 0 blocked, 97 done | 104 entries: 7 open, 0 blocked, 97 done. ⭐ Issue 29 CLOSED on green CI; `WSL-67` amended, not closed; `WSL-69` to `WSL-72` AUTHORED from four operator rulings and ⛔ none of them implemented |
| Changes | 270 tracked files | 270. Two files moved out of `examples/`, two added at the root; 18 files changed. ⚠ **A count of a session's own diff is stale the moment the record carrying it is committed**, so the command is the answer and the figure is stamped: `git diff --shortstat fcca2ba..HEAD` read +2,741 / -213 at `b524b00` |
| Size | `examples/common/bootstrap.sh`, 88 lines, one package manager, three digests written in | `scripts/common/bootstrap.sh`, 1,311 lines, twelve package managers, no digest written in. `tmux.conf` 86 lines |
| Checks | 19-check gate green and CI red, which is the finding | 19-check gate green, and ⭐ CI's own shellcheck 0.9.0 run in a container over all 24 scripts |
| Cost | not measured | ⭐ **6** full matrix runs and **9** `bsd run` sessions, counted from the invocations rather than recalled; the first figures here said 5 and 7. The final matrix: 13 images, 7m2s. The gate: 21s, timed. No paid operation; network bytes not measured and no number invented |
| Health | tree clean, two CI failures unfixed; WSL held `podman-machine-default` running, `eph-pgb` stopped, `wsl-toolkit` and `wsl-toolkit-podbox` running | tree clean; every container ephemeral and self-removed; ⚠ the FreeBSD guest image was mutated and restored from 102% to 43% disk. ⭐ **`wsl -l -v` is identical to the baseline**: the same four distributions in the same states, none created and none removed |

### The one number worth keeping

| | |
| --- | --- |
| `matrix --images all`, `agent` toolset, 25 logical names | **13 ran, 2 failed** in 7m2s, and the same 11 green after the last three fixes |
| the two red rows | Gentoo stage3 has no portage tree; Chimera's repository disagrees with itself about `openssl3`. ⛔ Neither is worked around |
| Alpine, full | requested 25, present 25, absent 0, CodeGraph 1.6.0, failures 0 |
| Debian, upstream route | PowerShell 7.6.6, digest matched against the release's own file |
| FreeBSD 15.1 | `pkg` installed 10, skipped the 8 its base provides, failures 0 |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first, and then
[issue 30](https://github.com/Azathothas/ToolKit/issues/30), whose single comment
is a cold-start orientation written for a session with no memory of this one.

⭐ **Nothing is waiting on the operator.** All three open questions were ruled on
2026-09-12 and each became an entry. ⭐ **The next unit of work is `WSL-69`**,
Muse Code installed, authenticated and driven end to end, because it is the ask
this issue was filed for and the only part of it never run. Two of its steps need
the operator's Meta credentials and they have said they will help.

---

## 2026-09-12 checkpoint, superseded by the sitting above

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
