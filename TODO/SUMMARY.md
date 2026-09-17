# SUMMARY.md

⚠ **A snapshot of one session, not an authority.**
[`PROGRESS.md`](PROGRESS.md) is what is true now; this is what the session that
wrote it measured on the day. A session that reads this and acts on it is acting
on what was true last time.

---

## 2026-09-17, the release that could not be consumed, and the tool its author would not use

⭐ **Two things happened that only a release could show.** `wsl-toolkit-v3.0.0`
published with all 14 assets and its smoke job FAILED, because `consumer.ps1`
wrote a configuration shape `WSL-74` had made a refusal and nothing could detect
that until a release carried the new binary. And the operator watched this
session edit a file with a Python heredoc and said that was the thing the
repository is trying to prevent - which was true, and the reason was not
forgetfulness: `text-tool` was not reachable from outside a checkout.

| row | before | after |
| --- | --- | --- |
| Elapsed | baseline `1699de9` pushed at 10:03:30Z | last measurement 12:22:40Z, about 2 h 20 m |
| Commits | `1699de9`, tree clean | **10**, by `git log --oneline 1699de9..HEAD`. All pushed except the last, which waits on CI for `19eb9d8` |
| Work | 125 entries, 5 open, 120 done | **127 entries, 3 open, 124 done.** Closed `WSL-68`, `WSL-76`, `WSL-78`, `WSL-90`, `WSL-92`; filed `WSL-91` and `WSL-92` |
| Changes | 0 files from `1699de9` | `git diff --shortstat` reads **62 files, +6,686 / -191** |
| Size | 105,395 text lines in 329 files | **111,890 in 346 files, +6,495**, by `git grep -I -c ''` |
| Checks | gate 21 of 21 | **gate 23 of 23, exit 0 read from the process.** `ste` and `skills` are new checks; `check-go.sh --json` answers `"modules":4` |
| Guards | 338 mutation rows | **392, +54.** Every new row driven and reported red; two came back THEATRE first and both were fixed rather than dropped |
| Findings | 66 | **75.** Findings 67 to 75, and **three of 72 to 75 were found by using `text-tool` on itself** |
| Rulings | 21 | **25.** Ruling 22 amends ruling 6 on the release; 23 clears what was blocked on the operator; 24 refuses a route for `muse serve`; 25 records this release |
| Issues | 2 open | **0.** 30 and 32 closed, each with a comment naming what its entries did NOT deliver |
| Released | `wsl-toolkit-v2.0.2` newest | **`wsl-toolkit-v3.0.0`**, 14 assets, the first release this repository has cut that publishes another project's build |
| Health | 5 distributions | ⭐ **the same 5**, none added or removed; `eph-pgb` untouched. ⛔ **podman would not answer all session**, rejecting its own SSH forward after a clean stop and start |

### What was asked for, and what happened

⭐ **The smoke failure was the job doing its work.** `publish wsl-toolkit`
succeeded and `the published release can be consumed` did not, on two of fourteen
cases. `consumer.ps1` set `base.name` to `wsl-toolkit-consumer` and selected no
instance, which names one distribution while recording into another's state. Its
own comment said "`WSL-43` is the entry that makes an instance a first-class
thing; **until it lands**, the name is set here" - and `WSL-43` had landed. Fixed
by selecting the instance through the environment, beside the state directory
already set that way, and writing the configuration at both paths so the eight
older tags `-Tag` accepts still find one. Driven against the published tag until every case it runs on a runner passed,
and confirmed independently on the runner itself in run 35220237357.

⛔ **No guard was written for it, and the obvious one was designed and thrown
away.** A rule refusing a `.ps1` that names a non-default distribution without
selecting an instance would refuse `acceptance.ps1`, which writes
`wsl-toolkit-nobase` on purpose to drive the refusal. `WSL-91` carries the
structural fix: `consumer.ps1` only ever runs against PUBLISHED binaries, so the
gate cannot reach it.

⭐ **`text-tool` is a product now.** Its own directory, four published assets, a
skill that stands alone, an `eol` mode that is the whole of `dos2unix` and
`unix2dos`, many files edited as one unit, and `--after`/`--before` that keep the
line they match. `check skills` refuses a skill naming a flag the program's own
usage text does not document, and that rule found a real gap on its first run.

⭐ **ONE DIGEST FROM THIRTEEN SHELL INVOCATIONS on two operating systems**, which
is the one claim the Go suite cannot make, because its cases call `Run()` directly
and never cross a shell boundary.

### What this session got wrong

⛔ **I ate an anchor with a substitution three times**, twice orphaning a Go
function's body and once removing a heading. That is already a written note in
this project, which is the evidence that a note is not a fix. The answer was to
add the operation that cannot express the mistake.

⛔ **I read an exit code through a pipe twice** and said so both times, in a
session whose own prompt warns about it in capitals.

⛔ **I wrote a case whose failure mode was a HANG.** Its fake reader reproduced a
blocking pipe so faithfully that the mutation row waited instead of going red.

⚠ **The container matrix never ran.** Six glibc shells on one distribution is what
was measured, not musl, Void or Alpine's busybox ash, and the base gained four
packages to make even that table.

---
## 2026-09-17, the model and effort every agent starts on, the guide, and a session that stopped without ending

⛔ **Written from artefacts by the session that resumed it**, at 2026-09-17T10:08Z, not
by the session it describes. That session made four commits and then stopped: no
section here, no `Recent work` rows, no measurements of its own, three of its four
commits unpushed, and ⛔ **the sixth thing it was asked for, the release, never cut.**
⚠ **Everything below is read from git, from `PROGRESS.md` and from the tree.** Where it
recorded no number, this says so rather than supplying one.

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-17T05:13:01Z, per its own State block | ⛔ **unknown.** Its last commit `1699de9` is dated 2026-09-17, and it wrote no end instant |
| Commits | `bf7753b`, tree clean | `git log --oneline bf7753b..1699de9` reads **4**. ⛔ **One pushed, three held.** `5eae667` went out and CI 35187005197 was green on it; `e0a239b`, `6192e61` and `1699de9` sat unpushed until the resuming session sent them at 10:03:30Z |
| Work | 5 open entries | **Completed 0.** **Partial 2:** `WSL-76`, whose fifth item it drove, and `WSL-78`, whose guide it wrote and ran. **Failed 0.** Entries 125 to 125, open 5 to 5, done 120 to 120 |
| Changes | 0 files changed from `bf7753b` | `git diff --shortstat bf7753b..1699de9` reads **36 files, +2,661 / -218**, five of them new |
| Size | 102,952 text lines in 324 tracked files | **105,395 in 329 files, +2,443**, by `git grep -I -c ''` |
| Checks | ⛔ **it recorded none of its own** | ⛔ **none it took.** What the resuming session measured over its head `1699de9`: gate **21 of 21** exit 0; Windows Go **377 top-level results, 357 passed, 20 skipped, 0 failed**; `check-go.sh` exit 0 in `golang:1.25`; ShellCheck 0.9.0 clean over **49** scripts. **11 new mutation rows**, 327 to **338**, all 11 reported red by it |
| Cost | ⛔ not recorded | ⛔ **not measurable from artefacts.** Its own record says three agents were started by herdr and answered, so **the operator's subscriptions were spent**; the number of prompts is not written anywhere |
| Health | five distributions | ⭐ the same five, none added or removed. **Findings 53 to 66 opened**, and **ruling 22** recorded. ⛔ **It left the default `wsl-toolkit` base carrying a stale podman boot id**, which the next session's first container check found; finding 67. Tree clean, no tag |

### What it was asked for, and what it did

| asked | outcome |
| --- | --- |
| the guide | ⭐ **done**, and every command in it was RUN before it was written, which found three defects in the guide itself. Finding 61 |
| the default model and effort | ⭐ **done and proved through herdr.** `base.adapters` takes `model` and `effort`, each written where that agent reads it, and each agent's own screen read back. Two silent pi failures found doing it, findings 53 and 54 |
| the errandsh ideas | ⭐ **done.** Three mechanisms into `shell-profile.sh`, driven across 13 images and 28 shells |
| every document plus two skills | ⭐ **done.** `skills/wsl-toolkit` and `skills/wsl-toolkit-agents`, and the manual, `scripts/README.md` and the muse-code guide rewritten |
| four reviews | ⭐ **done**, and each names what it looked at that the others did not. The door sweep found finding 62, the bootstrap PATH line that bypasses every agent wrapper |
| the release | ⛔ **NOT DONE.** Ruling 22, which it recorded itself, authorises it, and it stopped before cutting it |
| the end of the session | ⛔ **NOT DONE.** This section, the `Recent work` rows and the measurements are the debt, paid by the next session |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. What it left: the release, this ending, and
three unpushed commits.

---

## 2026-09-17, the last doors a zero-grant base had, and three agents driven through herdr

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-17T02:52:40Z | the record commit's own time is its end |
| Commits | `485252c`, tree **DIRTY** with the last session's two uncommitted wording corrections, its CI run 35070882239 green | **10**, all pushed. ⚠ `c68ca25` went RED on CI over a case of mine that only runs on Windows; `9e1e56e` fixed it and `0c04f98` is green. Finding 42 |
| Work | 7 open entries; `WSL-68` with four remaining items; `WSL-88` and `WSL-89` written and never run | ⭐ **Completed 2**, `WSL-88` and `WSL-89`, every condition driven on the operator's own base. **Partial 3:** `WSL-68` three of four items, the fourth deferred by the operator; `WSL-76` and `WSL-78` both acceptances driven. **Failed 0.** Entries 125 to 125, open 7 to **5**, done 118 to **120**, verified with `check-record.sh` |
| Changes | 0 files changed from `485252c` | `git diff --shortstat 485252c..HEAD` reads **41 files, +4,165 / -75**, eleven of them new |
| Size | 98,840 text lines in 315 tracked files | **102,930 in 324 files, +4,090**, by `git grep -I -c` |
| Checks | doctor exit 0 in 55 s; gate 21 of 21 in 70 s | gate 21 of 21, exit 0, green before every commit. Windows Go **365 top-level results, 345 passed, 20 skipped, 0 failed**; `check-go.sh` exit 0 in `golang:1.25`; ShellCheck 0.9.0 clean over **49** scripts. **15 new mutation rows**, 312 to **327**, all red, each green unmutated first |
| Cost | no paid operation authorized | no paid operation by this session. Sixteen container runs; three throwaway base builds, all removed; the podman machine started and stopped. ⚠ **The operator's own subscriptions were spent**: three agents each ran a prompt through their providers |
| Health | five distributions; `WSL-88` and `WSL-89` written and never run | ⭐ the same five registered, none added or removed. The operator's base gained pi, omp, bubblewrap and bun, and all four adapters read healthy. **Findings 37 to 52 opened**, six rulings recorded, 16 to 21. ⛔ **One of those findings is a defect this session shipped into their live base**: the Muse trust store was written in an invented shape and Muse refused to start until it was restored. Tree clean, no tag |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| resume at `WSL-68`'s four remaining items, none of which needs the operator | ⭐ **all four attacked, three delivered whole.** The probe is a command, the manual says what is not sealed, the shared tmpfs closes, and the private network namespace is measured and shipped as a flag. ⛔ One piece is left and the entry names it |
| the probe as a command rather than a script | ⭐ done. `wsl-toolkit base doors`: 30 doors attacked as the unprivileged account, 13 cases, 6 mutation rows, a manual section, a row in the acceptance sweep, and both refusal paths, one driven and one read |
| the manual paragraph saying what is NOT sealed | ⭐ done, with the measurement beside each door, and `base doors` named as the thing to run instead of trusting the paragraph |
| `muse login`, and the six `--remote` signals in a real window | ⛔ **not done, and they are the operator's.** The ask was re-sent unchanged; the client path in it was verified to still exist on disk |
| validate and reconcile before working | ⭐ done. Doctor, gate, `wsl -l -v`, CI, releases, `INDEX.md` and the entries all read against the record. The work order agreed three ways this time |
| finish all remaining tasks | ⭐ **three of WSL-68's four items are finished, and the fourth you deferred.** ⛔ The fourth is not, and the reason is a measurement rather than a shortfall: no container runs inside the namespace, so wrapping every command would break the base, and an interactive attach cannot be driven from this session |
| the shared tmpfs closed at every start | ⭐ done. `base.shared_tmpfs = "off"`, a boot script WSL runs as root, the resolver written before the door shuts and refreshed before every unmount, and a verifier that refuses either half failing |
| the account in its own network namespace | ⭐ **the mechanism is measured and driven**, `base exec --private-net`: the Windows host refused by an `nft` rule inside the namespace, the internet and DNS up, the shared namespace untouched. ⛔ **A flag, not a default**, because podman inside it takes itself for rootful and cannot run |

### ⛔ Seven defects, and every one needed the thing to be RUN

- ⛔ **A door reported CLOSED over an attempt that never happened.** `interop.run-exe` ran
  a hardcoded `/mnt/c/...` path that does not exist on a base with automount off and read
  the `No such file or directory` as a refusal, in the file whose own header forbids
  exactly that.
- ⛔ **The first driven run took 277 seconds.** bash's `/dev/tcp` carries no deadline and
  two filtered ports ran to the kernel's own timeout. The same run now takes 12 s.
- ⛔ **`--json` answered `"problems": null`** on a base with no problems, so the one answer
  a caller most wants is the one that breaks `.problems.length`, while every failing base
  parses.
- ⛔ **WSL's `WSLInterop` registration does not follow `[interop] enabled`.** Both values
  were observed on the SAME distribution with the same configuration, so
  `base.interop = "off"` now claims only the PATH door. ⚠ The mechanism has not been read.
- ⛔ **A `command=` value wsl.conf cannot parse does not run, and says nothing about it.**
  A value carrying nested quotes was silently ignored; the only evidence was a marker
  file that never appeared. The boot line is a bare path for that reason.
- ⛔ **`base doors` could not say `closed` on a closed door.** It tested for the directory
  and then the write, so on a sealed base the empty root-owned directory made it read
  `readonly` over a door that is shut. That base was the first one it had ever been
  closed on.
- ⛔ **No container runs inside the private network namespace.** pasta puts the payload
  in a user namespace where it is uid 0, podman then takes itself for rootful and cannot
  write the paths it chooses. Measured three ways. It is why `--private-net` is a flag
  and not how every command in the base starts.

### And then the adapters

| asked | outcome |
| --- | --- |
| ask and settle open questions before working | ⭐ done. Four rulings recorded, 16 to 21: the two entries approved by the 2026-09-14 work order, the omp collision settled as refuse-plus-opt-in, throwaway-first, and official-installers-first |
| finish the herdr/muse/pi/omp work | ⭐ **pi and omp are DRIVEN**, on one base with herdr. ⛔ **Muse is not**: `WSL-76` and `WSL-78` need `muse login`, which is yours, and nothing in them moved |
| `WSL-89`'s collision refusal | ⛔ **it could never fire**, on any base, ever. It read the account's environment under `env -i`, which clears it. It fires now, and the opt-in separates instead when asked |

### And then the agents, once the operator signed in

| asked | outcome |
| --- | --- |
| install and set up pi and omp here, then drive muse via pi and omp through herdr | ⭐ **done.** All four adapters healthy in the operator's base; all three agents answered  through .  and  closed |
| stop asking my agents about workspace trust | ⭐ done, for the account's home and each configured grant and nothing else. ⛔ **It shipped broken first** and Muse would not start until the store was restored; finding 47 |
| `base shell` should use bash with a proper login shell | ⭐ done, three arms, all driven. ⚠ Only `base shell` chooses; the account's shell is still `/bin/sh`, so herdr panes and `base exec` still land in sh. Finding 44 |
| why does `--instance base base shell` say base twice | answered: an instance name meeting a command group, with `WSL_TOOLKIT_INSTANCE` as the documented way out. Finding 45 |
| ensure future agents do not fall into the PATH trap | ⭐ done: a wrapper on the system PATH, a probe that reads the name back on a LOGIN shell, and the contract in `adapters/README.md`. ⛔ The first guard was theatre. Finding 49 |
| adopt the issue 30 bashrc list, and errandsh | ⛔ **not done, and not started.** `errandsh` is a Python file rather than a shell rc, so adopting it means extracting its ideas into `scripts/common/shell-profile.sh` under `WSL-71`'s constraints. It is the next unit of work |

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. ⭐ **Nothing waits on the operator any more.**
The shell profile work they asked for is unstarted; `WSL-76` and `WSL-78` each owe one
small thing; `WSL-68` is deferred and `WSL-90` waits on ruling 6.

---

## 2026-09-16, a checkpoint finished, the first nightly published, and three checks that could not fail



| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-16T06:59:37Z | the record commit's own time is its end, about one hour |
| Commits | `cfa3252`, tree **DIRTY** with the previous session's unfinished record, its CI run 34967456693 green | `git log --oneline cfa3252..HEAD` reads **5**. ⚠ **Three pushes, not five:** `42a7c5b` after `cfa3252`'s CI was green, `431417b` after `42a7c5b`'s, then `b2203ab`, `69ae23f` and `485252c` **together** after `431417b`'s. So CI ran on the head of each push and not on every commit, which is what a branch push does and is stated here rather than implied away |
| Work | 7 open entries; a checkpoint claimed and not made | **Completed 0.** **Partial 1:** `WSL-90`, whose every step but one is now proved and driven. **Failed 0.** ⛔ **`WSL-90` does NOT close**, and not on the operator: its step 4 needs a `wsl-toolkit-v*` release that ruling 6 forbids cutting yet. Entries 125 to 125, open 7 to 7, done 118 to 118 |
| Changes | 0 files changed from `cfa3252` | `git diff --shortstat cfa3252..HEAD` reads **14 files, +1,193 / -52**, two of them new |
| Size | 97,648 text lines in 313 tracked files at `cfa3252`, `git grep -I -c ''` | **98,789 in 315 files, +1,141** |
| Checks | doctor exit 0 in 75.96 s; gate 21 of 21 in 64.8 s | gate 21 of 21, green before every commit. Windows Go **330 top-level results, 310 passed, 20 skipped, 0 failed, exit 0 in 15.25 s**, and `tools/check` and `tools/repo` green beside it; `check-go.sh` exit 0 in `golang:1.25`; ShellCheck 0.9.0 clean over **47** scripts. **2 new mutation rows**, 310 to **312**, both red when planted |
| Cost | no paid operation authorized | no paid operation. One `herdr-nightly.yml` run, 6 jobs, about 11 minutes of GitHub runner time across four architectures; **68 MiB downloaded** and verified, kept under `.tmp`; six container runs |
| Health | five distributions; the base running a hand-swapped development herdr | ⭐ the same five, none added or removed. The base now runs a **published** artefact, `herdr-nightly-20260916-18061191fdc0`, over the hand-swapped build. ⭐ Findings 2, 29 and 31 to 36 opened or closed; **finding 2 closed and corrected** - it named half a defect. Tree clean, no tag, no BSD image, the operator's herdr workspace `w2` untouched |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| resume at `WSL-67`'s `pkgin` and `pkg_add` drives, then `WSL-71` | ⛔ **the prompt was stale and this was not done.** Both entries closed at `6f22e39` on 2026-09-15; `INDEX.md` and both entries read `done`. Only `PROGRESS.md`'s work order still said otherwise, and it is what misrouted the prompt. Finding 29 |
| ask the operator to approve two BSD downloads | ⛔ **not asked, deliberately.** They were approved on 2026-09-15, downloaded, driven and removed. Asking a fourth time for something already given is the opposite of the instruction |
| validate and reconcile actual progress before working | ⭐ done, and it is the reason anything else here is right. The doctor, the gate, `wsl -l -v`, the CI API and `INDEX.md` were read against the record, and the record lost |
| finish what the previous session left | ⭐ done. Its checkpoint claimed an amendment that did not exist, owed three reviews it had not run, and had written no summary. All three delivered, and its summary written from artefacts |
| `WSL-90`'s first nightly | ⭐ **published, verified and driven.** 6 of 6 jobs, 12 assets, every digest and signature checked and both proved able to refuse, and the base driven onto the channel end to end |
| the probe as a tracked script | ⭐ done, and it reproduces the entry's premise from a command rather than from a session's memory |
| `muse login`, and the six `--remote` signals in a real window | ⛔ **not done, and they are the operator's.** Asked once, with exact commands, and the `--remote` ask revised to the nightly's own client when the base moved to it |

### ⛔ Three things were green and could not have been otherwise

- ⛔ **The gate's `powershell` check had never checked anything.** Finding 2 blamed a
  separator mismatch; fixing that alone changed nothing, because the file list was passed
  as arguments after `pwsh -Command`, which does not reach `$args`. It parsed **zero**
  files and reported ok for its whole life, while CI's `powershell` job ran the gate and
  trusted it. It now returns how many files it read and refuses any count that is not the
  number handed to it.
- ⛔ **A signal called `repaint` passed the client that cannot repaint.** Written as "drew
  more than zero bytes", it passed herdr 0.9.0 on 331 bytes of terminal setup - inside the
  probe written to catch exactly that. What separates the builds is the SHARE of the paint
  arriving before any focus, 98 per cent against 9.
- ⛔ **`herdr-nightly.yml`'s prune would have deleted the newest nightly.**
  `gh release create --target SHA` dates a release from the target commit, so
  `sort_by(.created_at)` ordered nightlies by when `main` last moved. The one it would
  have deleted is the one the adapter resolves. Nothing published was at risk, because
  the prune keeps seven and there is one.

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. The operator's two steps, then `WSL-68`'s four
remaining items, none of which needs them.

---

## 2026-09-15, the attended Muse and herdr session, and a builder for somebody else's program

⚠ **Written on 2026-09-16 by the session that resumed this one.** The session below was
checkpointed at the operator's request and ended before it wrote its own summary, so every
figure here is read from git, the CI API and the entries rather than from its narrative.

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-15T10:20:32Z | its last work commit `cfa3252` at 12:05:05Z, about one hour forty-five; the record commit that closes it is the next day's |
| Commits | `6f22e39`, clean `main`, its CI run 34955394330 green in all six jobs | `git log --oneline 6f22e39..HEAD` reads **3**, each pushed with the previous one's CI green first; `cfa3252`'s own CI run 34967456693 is green |
| Work | 7 open entries | **Completed 0.** **Partial 3:** `WSL-76` and `WSL-78`, whose four ordered measurements were taken and whose muse reporter was driven against a real herdr; and `WSL-90`, filed, approved by ruling and implemented the same session. **Failed 0.** Entries 124 to 125, open 6 to 7, done 118 to 118 |
| Changes | 0 files changed from `6f22e39` | `git diff --shortstat 6f22e39` reads **37 files, +2,712 / -299**, five of them new |
| Size | 95,369 text lines in 308 tracked files at `6f22e39`, `git grep -I -c ''` | **97,782 in 313 files, +2,413** |
| Checks | doctor exit 0 in 39.71 s; gate 21 of 21 in 33.81 s | gate 21 of 21, green before every commit. ⚠ Re-measured on the finished tree on 2026-09-16, because the session recorded its Go proof mid-way and then added six cases: the Windows proof with the 8.3 `TEMP` reads **330 top-level results, 310 passed, 20 skipped, 0 failed, exit 0 in 15.25 s**, where `WSL-76`'s amendment quotes 324 and 304; `check-go.sh` exit 0 in `golang:1.25` in 31.52 s; ShellCheck 0.9.0 in `ubuntu:24.04` clean over **47** tracked scripts in 24.12 s. **8 new mutation rows**, 3 for the release lookup and 5 for the nightly channel |
| Cost | no paid operation authorized | no paid operation. Rust 1.96.1 installed through rustup for herdr's build; two local herdr builds, 227 s for Windows and 223.4 s for Linux; four dispatched `herdr-build.yml` runs on GitHub's runners, two of them failures |
| Health | five distributions; `wsl-toolkit-base` holding pinned herdr 0.9.0 | the same five, none added or removed. ⚠ **`wsl-toolkit-base` left running herdr's DEVELOPMENT server** by the operator's ruling 15, with 0.9.0 kept beside it; the next `base ensure` puts the pinned digest back. ⛔ Tree left **dirty** across the session boundary, and the record commit was not written |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| the four measurements the reference sweep named | ⭐ done, and **three of the four overturned the record**. herdr on Windows is 0.9.0, no published herdr carries `#4038`'s fix, and ⛔ **`herdr --machine` does not exist in 0.9.0** - the sweep had read herdr's unreleased `docs/next` |
| the muse adapter's herdr reporter, against a real herdr and a real Muse | ⭐ done, and its first real run found **four defects the stub could not**: a temporary file `fs.protected_regular` refused, the wrong settings file, the wrong hook shape, and a new file Muse would refuse to start over |
| build herdr here, and a nightly builder if it works | ⭐ built locally for Windows and Linux, and `WSL-90` filed, approved and implemented in the same session by the operator's rulings 12 to 15 |
| `muse login` | ⛔ **not done.** The credential is the operator's and was asked for in chat |
| the six `--remote` signals in a real window | ⛔ **not done.** A pseudo console measured four; only a real window carries a real focus event |

### ⛔ What it left behind, and what the resuming session found

- ⛔ **The checkpoint was claimed and not made.** Its own record row said the build
  matrix's second run was "read in `WSL-90`'s last amendment". No such amendment existed;
  the runs had happened and were green, and nothing recorded them. Written on 2026-09-16
  from the run logs.
- ⛔ **The work order outlived the work by a session.** It still sent a reader to drive
  `pkgin` and `pkg_add` for `WSL-67` and to ask the operator for two downloads, after
  `6f22e39` had closed that entry on a driven NetBSD guest and the operator had approved
  the downloads. `INDEX.md` and the entry both read `done`, so only the work order was
  wrong - and a session resumed on 2026-09-16 was misrouted by it. Finding 29.
- ⛔ **The three closing reviews were not run.** Run on 2026-09-16; the door sweep found
  that `consumer.ps1` downloads every asset a release carries, the guard mutation drove
  `herdr-nightly.yml`'s guards for the first time, and the claim audit found an
  enumeration that was right by luck and a correction with no date.
- ⭐ **The four measurements and the reporter's four defects are real and recorded**, each
  with its conditions, in `WSL-76`'s and `WSL-78`'s amendments of 2026-09-15. Nothing in
  this session's measured work was found wanting.

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. `WSL-90`'s first nightly, then the operator's two
steps, then `WSL-76`'s and `WSL-78`'s proves.

---

## 2026-09-15, the BSD drives, the reference sweep, and a binary that carries its scripts

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-15T06:16:51Z | ended at the record commit's own time, about three hours and twenty minutes |
| Commits | `53a8f1f`, clean `main`, its CI run 34934199765 green in all six jobs | `git log --oneline 53a8f1f..HEAD` reads **6**, each pushed with the previous one's CI green first |
| Work | 6 open entries; `WSL-67` blocked on two downloads, `WSL-71` and `WSL-68` untouched | **Completed 2:** `WSL-71` and `WSL-67`. **Partial 1:** `WSL-68`, its door enumeration closed and four items named. **Authored 2:** `WSL-88` and `WSL-89`, whose adapters are written and never run. **Deferred 2:** `WSL-76` and `WSL-78`, the interactive session the operator reserved. **Failed 0.** Entries 122 to 124, open 6 to 6, done 116 to 118 |
| Changes | 0 files changed from `53a8f1f` | `git diff --shortstat 53a8f1f` reads **57 files, +7,834 / -385**, one file deleted and eleven new |
| Size | 87,866 text lines in 291 tracked files at `53a8f1f`, `git grep -I -c ''` | **95,315 in 308 files, +7,449** |
| Checks | doctor exit 0 in 27.79 s; gate 20 of 20 in 33.09 s | ⭐ gate **21 of 21**, the new one being `shipped`; green before every commit. The Windows Go proof with the 8.3 `TEMP` **323 results, 304 passed, 19 skipped, 0 failed**; `check-go.sh` exit 0 in `golang:1.25`; ShellCheck 0.9.0 clean over **45** tracked scripts, up from 34. **21 new mutation rows** proved, four of them in `golang:1.25` because they need a `dash` this host has not got; **10 defects planted by hand** in two shell files, each changing exactly one column |
| Cost | no paid operation authorized; network bytes not measured | no paid operation. ⭐ **1.25 GiB of BSD images, approved in chat and removed after use**, with about 6.5 GiB of images and overlays freed. Fourteen repositories cloned shallow for the sweep. Two full thirteen-image matrices, five `base ensure` builds, and roughly forty container runs |
| Health | four distributions; `WSL-67` blocked for the fourth session | ⭐ `WSL-67` closed on a real NetBSD guest; three defects in a URL-fetched file fixed. ⛔ Recorded and not fixed: `/mnt/wsl` is shared and writable across every distribution, so a zero-grant base is not a sandbox, and the manual now says so. The same four distributions, no throwaway, no BSD image, the shared tmpfs as it was found, no tag, tree clean |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| `WSL-67`, once the downloads are approved | ⭐ **approved in chat after four sessions, and closed.** NetBSD 11.0 booted under QEMU drove **both** arms: `pkgin` and, with pkgin hidden, the base `pkg_add`. Three defects fixed, each invisible in the source. OpenBSD 7.9 was installed and booted; the operator then ruled that one driven system is enough |
| then `WSL-71` | ⭐ done. A portable shell profile, the `--here` mark through `WSLENV`, and a `PATH` line that had never reached a bash login shell on this tool's own default base |
| `WSL-68` | ⚠ **partial by design.** Approach step 1 is closed - every door attacked on a live zero-grant base - and it found one the entry did not list |
| mine fourteen references for `WSL-76` and `WSL-78` | ⭐ done, with commits and **the trackers**. Recorded in `docs/reference-sweeps/` |
| amend the stale documents | ⭐ done. The replaced multiplexer's page deleted, its links moved, and ten documents amended against what is now true |
| collapse the examples | ⭐ the muse-code example went from **eleven commands and two files copied by hand to three commands**, because the tool already did the work the page described by hand |
| our own herdr-muse plugin | ⭐ built, taking what the references got right and none of what they got wrong. Driven against a stub herdr; ⛔ never against a real herdr or a real Muse |
| the pi and omp adapters | ⛔ **written, never run.** Both entries and the adapters README say so in those words |
| resilience, no hardcoded values | ⭐ an adapter's version and digests move from the configuration, either alone refused; no adapter summary carries a version; the omp adapter refuses the directory collision before herdr does |
| embed the scripts in the binary | ⭐ `shipped list/cat/write` and `base bootstrap`, with a `shipped` gate rule and a carried script that needs no `mktemp` |
| leave the interactive test | ⭐ left. `WSL-76` and `WSL-78` name the first measurement each |

### ⛔ What the reviews caught, and one of them was wrong in five places

- ⛔ **`herdrdev/herdr#4176` is closed `not_planned`, not closed as fixed.** This
  session had written "closed, so a later release carries the fix" and repeated it in
  five files. It would have sent the next session to update herdr and read a green
  result as proof the door worked. Corrected everywhere.
- ⛔ **The muse probe never read its own reporter back**, while the pi and omp probes
  both read theirs. Found by the door sweep; it now reports the reporter, runs its
  self-test, and lists the events the settings register.
- ⛔ **A carried script's temporary file needed `mktemp`**, which a base at
  `toolset none` has not got, and the fallback would have been a predictable path
  under `/tmp`. It reads from a file descriptor instead.
- ⛔ **A mutation planter that planted nothing**, twice: `awk`'s `sub()` takes a
  regular expression and the anchors were full of metacharacters, so five guards read
  as doing nothing. Printing the unmutated row first is what made it legible.
- ⛔ **A `TK_GROUP` added at the top of a script for a feature used at the bottom**
  ended every path above it under `set -e`. Found by CI's Linux job, not by Windows.

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. `WSL-68`'s four remaining items, then the
Muse and herdr session the operator reserved, which is attended and interactive.

---

## 2026-09-15, the shell profile, and the doors a zero-grant base still has

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-15T06:16:51Z | ended at the record commit's own time, about forty minutes. Much of it ran in parallel: two full thirteen-image matrices, four base builds and about a dozen container runs overlapped with the writing |
| Commits | `53a8f1f`, clean `main`, its CI run 34934199765 green in all six jobs | `git log --oneline 53a8f1f..HEAD` reads **2**: `8801301`, `WSL-71`, pushed with CI run 34938687647; and this record commit |
| Work | 5 open entries after `WSL-71` was counted; `WSL-67` partial and waiting on two downloads, `WSL-68` untouched since 2026-09-14 | **Completed 1:** `WSL-71`, with its prove, five hand-planted mutations and three reviews. **Partial 1:** `WSL-68`, its Approach step 1 closed and four items named. **Blocked 1:** `WSL-67`, asked a fourth time and not answered. **Deferred 2:** `WSL-76` and `WSL-78`, the herdr and Muse drive, excluded by the operator for this session. **Failed 0.** Entries 122, open 6 to 5, done 116 to 117 |
| Changes | 0 files changed from `53a8f1f` | `git diff --cached --shortstat 53a8f1f` with this whole change staged reads **14 files, +1,018 / -72**, one of them new |
| Size | 87,866 text lines in 291 tracked files at `53a8f1f`, `git grep -I -c ''` | **88,812 in 292 files, +946**, this section included |
| Checks | doctor exit 0 in 27.79 s; gate 20 of 20 in 33.09 s | gate 20 of 20 before every commit; the Windows Go proof with the 8.3 `TEMP` 315 results, 299 passed, 16 skipped, 0 failed, exit 0 in 16.5 s; `check-go.sh` exit 0 in `golang:1.25` in 32.15 s; ShellCheck 0.9.0 in `ubuntu:24.04` clean over **35** tracked scripts; 2 new mutation rows red and 5 defects planted by hand in the shell file, each changing exactly one column |
| Cost | no paid operation authorized; network bytes not measured | no paid operation, and ⛔ **no BSD image: the download was asked for a fourth time and not approved.** Network bytes not measured; known transfers: two full thirteen-image matrices, 26 rows, with `--toolset minimal`; **four `base ensure` invocations** - one refused on an unqualified image reference before building anything, one arch build rolled back by a stalled mirror, and two arch builds completing; and about fifteen single container runs in `debian`, `ubuntu:24.04`, `golang:1.25`, `fedora`, `rockylinux:8` and `alpine`, one of which reached its 12-minute deadline in dnf |
| Health | four distributions; `WSL-71` and `WSL-68` both untouched this session | ⭐ `WSL-71` closed and a defect it found fixed in the same change. ⛔ Found on `main` and NOT fixed: `/mnt/wsl` is one shared tmpfs every distribution can write to, recorded in `WSL-68` with the attack that proved it. The same four distributions and no throwaway; both test bases removed with their disks; the shared tmpfs left as it was found; the FreeBSD image not booted; the SSH configuration unchanged; no tag; tree clean after this commit |

### What was asked, and what happened

| asked | outcome |
| --- | --- |
| `WSL-67`'s `pkgin` and `pkg_add` once the downloads are approved, then its closing | ⛔ **not started.** The two images were asked for in chat and as a file at the session's start, with a denial offered as a complete outcome. No answer came, so nothing was downloaded and nothing was worked around. Four sessions have now asked |
| then `WSL-71` | ⭐ **done and pushed.** A new `scripts/common/shell-profile.sh`, `base shell --here` marking its own shell through `WSLENV`, and the `PATH` line `bootstrap.sh` writes reaching the login file bash actually reads |
| finish everything except the herdr and Muse drive | `WSL-68` was brought into this session by that instruction, against the 2026-09-14 order that gave it a session of its own. Its Approach step 1 is closed - every door attacked on a live zero-grant base - and the remaining four items are named. It does not close here, and the entry says why |

### What the work found

- ⛔ **`bootstrap.sh`'s `PATH` line has never reached a bash login shell on this tool's
  own default base.** bash reads the first of `~/.bash_profile`, `~/.bash_login` and
  `~/.profile` and stops, and arch's `/etc/skel` ships `.bash_profile` and no `.profile`,
  as fedora's and rocky 8's do. Measured on an arch base built at this tool's defaults, and
  fixed in the same commit.
- ⛔ **A zero-grant base is not sealed, and the reason is not in `WSL-68`'s own list.**
  `/mnt/wsl` is one `tmpfs` mounted `drwxrwxrwt` and shared by every distribution in the
  utility VM. The base wrote a file another distribution read, and read one another
  distribution wrote. Closing it needs root, at every start, and costs DNS.
- ⚠ **Two readings were wrong before they were corrected**, both because a check read a
  pipeline's status instead of the process's: the podman sockets in that tmpfs are
  zero-byte regular files that `curl` cannot connect to, and the first DNS answer after
  the unmount was `head`'s status, not `getent`'s.
- ⛔ **A mutation pass reported five guards as doing nothing, and the planter was the
  defect.** `awk`'s `sub()` takes a regular expression, and the anchors were full of
  metacharacters. Printing the unmutated row first is what made it readable as a claim
  about the planter.

### Resume point

Read [`PROGRESS.md`](PROGRESS.md) first. `WSL-67`'s BSD drives if and only if the
downloads are approved, then `WSL-68`'s four remaining items, then the herdr and Muse
session.


## 2026-09-15, the two presets that would not build, and three entries closed

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-15T03:12:15Z | ended at the record commit's own time, two hours and thirty-five minutes. About half of it was spent waiting on a Fedora mirror answering in tens of KiB/s and on two full agent matrices |
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
