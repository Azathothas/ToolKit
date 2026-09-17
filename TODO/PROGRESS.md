# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-17T10:01:48Z
baseline        1699de9, tree clean, doctor exit 0, gate 21 of 21, and three commits the previous session left unpushed, pushed at 10:03:30Z on green CI for 5eae667
head            this session's commit
entries         total 125  open 5  blocked 0  done 120
this session    the ending the previous session never wrote, wsl-toolkit-v3.0.0, and WSL-90 closed on the release that proves its step 4
```

## Active work

⛔ **THIS SESSION RESUMED ONE THAT STOPPED WITHOUT ENDING.** The session of
2026-09-17T05:13:01Z was asked for six things - the guide, the default model and
effort, the errandsh ideas, every document plus two skills, four reviews, and the
release - and it delivered the first four. Then it stopped without doing any of what
[`../docs/methodology/sessions.md`](../docs/methodology/sessions.md) says a session
owes: no `SUMMARY.md` section, no `Recent work` rows, no measurements of its own, and
⛔ **the release was never cut.** Three of its four commits were still unpushed.
⚠ **Everything it did record is correct** - the entries, the rulings and findings 51
to 66 were all read against the tree and the artefacts; what was missing was only the
ending. It is written from artefacts below and in `SUMMARY.md`, by the session that
resumed it, which is the second time in three days that a session has owed this.

⭐ **`wsl-toolkit-v3.0.0` IS CUT**, under ruling 22, and it is the first release this
repository has made since `wsl-toolkit-v2.0.2` on 2026-09-10. It carries the two
Windows executables, herdr's newest stable release built for four targets,
`SHA256SUMS` and a keyless signature for each - fourteen files. ⭐ **It is what closes
`WSL-90`**, whose approach step 4 had never run: no `wsl-toolkit` release had ever
carried herdr, so nothing had proved that half of the workflow.

⭐ **The startup model and effort are delivered and PROVED THROUGH herdr.** `base.adapters`
takes `model` and `effort` on an agent adapter, `effort` is `max` when a configuration
names none, and each value is written where that agent itself reads it: pi's settings
file, omp's own `config set`, and - because Muse Code 1.3.0 has no settings key for
either - the wrapper the muse adapter already owns. ⛔ **Not an environment variable, and
that is the whole reason the fields exist**: finding 51. Each agent was started BY herdr
and its own screen read back: Muse `muse-spark-1.3-contributor · max`, pi
`(muse-gateway) muse-spark-1.3-contributor • max`, omp `Muse Spark 1.3 Contributor`.
⛔ **Two silent failures were found doing it**, findings 53 and 54: pi falls back to
another provider's model when its catalogue cannot resolve the one configured, and
clamps `max` to `high` without a `thinkingLevelMap`. Both are now problems
`base status --probe` raises, each proved by planting it.

⭐ **The shell profile gained three mechanisms and was driven across every image.**
`WSL_TOOLKIT_NO_PROFILE` turns all of it off, `PATH` loses its duplicates and its empty
elements for an interactive shell only, and history gets a home where the shell gave it
none. `matrix --images all`, each image twice: **13 images, 28 shells, 0 add a byte to
stderr, 28 de-duplicate, 28 leave a non-interactive shell alone, 13 gained a history
home, 28 honour the switch.** ⛔ **Two defects in the DRIVER found first**, findings 57
and 58: `-ic` never reads `~/.profile`, and Photon has no `tr`.

## The work order, set by the operator on 2026-09-14

Each entry's section in [`wsl-toolkit-go.md`](wsl-toolkit-go.md) carries its
premise, decisions and prove. An issue closes only when every entry mapped to it
is closed, and the issue gets a comment naming the commits.

1. ⭐ **Closed:** `WSL-74`, `WSL-80`, `WSL-79` with issue 33, `WSL-75`, `WSL-81`,
   `WSL-72`, `WSL-77`, `WSL-84`, `WSL-83`, `WSL-82` and `WSL-85`.
2. ⭐ **Closed:** `WSL-70`, `WSL-71` and `WSL-67`, which finish issue 30's existing
   package and profile work. ⚠ **This item said until 2026-09-16 that `WSL-67` still
   had `pkgin` and `pkg_add` to drive and was waiting on two downloads.** Both were
   approved on 2026-09-15, both managers were driven on a NetBSD 11.0 guest, and the
   entry closed at `6f22e39`; the sentence outlived the work by a session and sent a
   resuming session to a finished entry. Finding 29.
3. **`WSL-68`, the sealed base.** It closes issue 30 after the work above, and uses the
   drive verifier `WSL-84` fixed. ⚠ The 2026-09-14 order gave it a dedicated session;
   the operator's instruction of 2026-09-15, to finish everything except the herdr and
   Muse drive, brings it into this one.
4. **Muse and herdr together, one dedicated session.** Build `wsl-toolkit-base` from
   [`../tools/windows/wsl-toolkit/examples/muse-code/wsl-toolkit-base.json`](../tools/windows/wsl-toolkit/examples/muse-code/wsl-toolkit-base.json);
   the operator runs `muse login` in it; then close `WSL-76` and `WSL-78`. Author
   approved entries for `pi` and `omp` before either adapter is built.
5. **`wsl-toolkit-v3.0.0`** is cut after issues 30 and 32 are closed, and CI is green
   on the final commit.

## Rulings in force

1. **2026-09-13: herdr replaces Zellij** as the agents' multiplexer, and tmux
   stays a generic fallback. `WSL-76` quotes the operator.
2. **2026-09-14: one base for every agent.** `wsl-toolkit-base`, the instance
   `base`, configured under the operator's account, with the Linux account
   `herdr`, passwordless sudo and no standing grant. Test grants name directories
   under this repository's `.tmp`. In `WSL-75`.
3. **2026-09-14, the entries' decisions:** `WSL-75` live grants of a project or of
   a parent directory; `WSL-76` OpenSSH through `wsl.exe` with nothing listening
   and only the herdr client's key accepted, and herdr's background checks left
   on; `WSL-77` adapters in the tree with a generated embedded copy; `WSL-78`
   agents run in the base and Windows gets launchers; `WSL-68` the internet only
   with the host refused, and only with no rule in the shared network namespace;
   `WSL-67` drive `pkgin` and `pkg_add`, and remove a manager that cannot be driven
   here.
4. **2026-09-14: one Muse installer digest is approved**, `5196d820…632a0ca`, in
   `WSL-77`.
5. **2026-09-14: host changes the agent may make:** a dedicated SSH key and one
   marked `Host` block in `%USERPROFILE%\.ssh\config`, and agent launchers in
   `%USERPROFILE%\bin`.
6. **2026-09-14: cut `wsl-toolkit-v3.0.0` only after the order above is complete.**
   Issues 30 and 32 are closed first. Run `repo release` read-only, and publish only
   with CI green on the final commit.
7. **2026-09-14: finish existing work before Muse and herdr.** Keep Muse and herdr
   together in one later dedicated session. Complete the sealed base before it. In
   `WSL-76`.
8. **2026-09-14: remove Soar and keep Nix**, and Nix is loaded, set up and never
   installed: an installed Nix is found without sourcing a profile, flakes are set up
   and used for a new profile, the four `NIXPKGS_ALLOW_*` variables are always on
   with more defaults where they help, and a GitHub token reaches Nix through
   `NIX_CONFIG` only. All of it lives in this repository. In `WSL-67`.
9. **2026-09-15: `WSL-85` approved**, "approve WSL-85": filed and fixed in this
   session, before `WSL-70`.
10. **2026-09-15: `WSL-87` approved**, "approve WSL-87": filed and fixed in this
    session.
11. **2026-09-15: `WSL-86` approved** as recommended, "yes i accept your
    recommendation": `nftables` in the provisioner's `apt` arm, and the id-mapping
    check reads the capability rather than the path and restores a dropped one. The
    operator asked in the same message to be asked only for what actually needs them,
    which is why the drive-sweep race found while proving it is recorded under "Found,
    and not filed" rather than proposed as an entry.
12. **2026-09-15: herdr is built here.** "let's build herdr ourself (now locally for
    windows) and if it works, we will create a dedicated nightly builder for it on
    github and publish it on our repo". In `WSL-90`.
13. **2026-09-15: herdr builds are published from ToolKit, as prereleases**, which
    amends `docs/AGENTS.md` section 1's one published thing to two; for Windows
    `x86_64` and `aarch64` and Linux `x86_64` and `aarch64`; and the newest stable herdr
    is published inside each `wsl-toolkit` release. In `WSL-90`.
14. **2026-09-15: the herdr adapter may follow the newest nightly**, over the
    recommended pin by version and digest. In `WSL-90`.
15. **2026-09-15: `WSL-90` approved and implemented in the session that authored it**,
    and the development herdr server swapped into `wsl-toolkit-base` for the
    `--machine` measurement.

16. **2026-09-17: `WSL-88` and `WSL-89` were approved by the work order of 2026-09-14**,
    "Author approved entries for `pi` and `omp` before either adapter is built". Asked
    because neither entry carried an approval line where `WSL-85` to `WSL-87` do, and
    both adapters had already been built under that instruction. Both entries are
    approved as written.
17. ⛔ **2026-09-17: the omp adapter REFUSES a colliding configuration by default AND
    offers an explicit opt-in that separates the directories.** ⚠ **This is not the
    recommendation the entry carried**, which was refuse alone; the operator chose the
    third option. So the separation is new work with its own cases and mutation rows,
    and the refusal stays the default. In `WSL-89`.
18. **2026-09-17: `pi` and `omp` are driven on a throwaway base first**, and installed
    into `wsl-toolkit-base` only once both adapters are known good. Ruling 2 still names
    `wsl-toolkit-base` as the one base for every agent; this is about the order, not the
    destination.
19. ⭐ **2026-09-17: `WSL-68`'s last piece is DEFERRED by the operator**, "defer it for
    now". The entry stays open on the flag for an interactive attach; nothing else in it
    is outstanding. Issue 30 closes when that piece does, and it is the only entry mapped
    to that issue still open.

20. ⭐ **2026-09-17: an adapter prefers the provider's OFFICIAL installer and falls back
    to this repository's own route.** The operator: "we should prefer official installers
    and then fallback to ours". ⚠ **Not implemented, and it is not a small change.** This
    repository never pipes an installer into a shell; the route it permits is the one
    ruling 4 set for Muse - download the installer, read it, run it from the file that
    was read, against an approved digest. Applying that to `pi` and `omp` means a digest
    for each, which is an operator approval per installer. ⛔ Recorded here, filed as the
    next work on the adapters, and NOT silently half-done: today `pi` and `omp` install
    from npm, which is this repository's own route.
    ⭐ **What was read while settling it**, on 2026-09-17: `https://omp.sh/install`, 9,954
    bytes, fetched and READ and never run. Its default mode is `binary`, a prebuilt
    standalone release asset from GitHub, which needs no Bun at all; only `--source`
    installs through Bun, and that path runs `curl -fsSL https://bun.sh/install | bash`,
    a second pipe into a shell. So the official route would also remove this base's need
    for Bun, which makes it worth doing rather than merely worth recording.
21. ⛔ **2026-09-17: bun is NOT in `bootstrap.sh`'s table or the developer toolset**,
    against the expectation that it was. `bootstrap.sh` has seven matches for "bun" and
    every one is `ca-bundle`; a word-boundary search finds none, and `packages.sh` has
    none. The omp adapter installs it from the distribution itself, and does not add it
    to the shared table, which is fetched by URL and is not where a runtime one adapter
    needs belongs - the rule `WSL-88` states for pi.

22. ⭐ **2026-09-17: `wsl-toolkit-v3.0.0` is cut, and this AMENDS ruling 6.** The operator:
    "cut the release finally". ⛔ **Ruling 6 required issues 30 and 32 to be CLOSED first,
    and issue 30 cannot be**: its part 2 is `WSL-68`, which the same operator deferred in
    ruling 19. So the release no longer waits on issue 30. ⚠ **The second half of ruling 6
    still holds and is not amended**: `repo release` runs read-only first, and the publish
    happens only with the gate green and CI green on the final commit. ⭐ **Issue 30 gets a
    comment saying what is left rather than being closed**, and issue 32 closes only if
    every entry mapped to it is done.

## Before every push




A green local gate is not CI. Run the Go tests with `TEMP` and `TMP` at an 8.3
short path, then CI's Linux Go job and its ShellCheck in containers.
[`../tools/windows/wsl-toolkit/README.md`](../tools/windows/wsl-toolkit/README.md)
carries all three as commands under "Build and local proof". ⚠ **A BSD run no longer
writes the shared image**, so a heavy run may boot it; a run that needs a damaged or
altered image goes on a copy, with `WSL_TOOLKIT_CACHE` under this repository's `.tmp`.
⚠ **An untracked file is outside the WHOLE gate, not just ShellCheck.** Every check
reads the tracked set, so a new file is invisible until it is staged and the green gate
you ran before adding it said nothing about it. Met twice: CI's ShellCheck command
lists `git ls-files`, and on 2026-09-16 `herdr-remote-probe.ps1` passed a full gate
while untracked and failed `line-endings` on the very next run, because
`.gitattributes` gives `*.ps1` `eol=crlf` and it had been written with LF. ⭐ **Stage a
new file FIRST, then run the gate.**

## Recent work

| commit | what |
| --- | --- |
| `45fc7cc` | `WSL-67` partial: Soar removed from `bootstrap.sh`; the Nix route finds an installed Nix, sets up flakes and installs through the table's `nix` key; the last session's record and summary |
| `953e7b8` | `WSL-85` filed and closed: the verifier refuses passwordless sudo the configuration turned off, `base ensure` provisions such a base again, and a verification failure is named past wsl.exe's own lines; 3 mutation rows |
| `4c573cf` | `WSL-67` partial: the acceptance runner builds `wsl-toolkit-accp` and drives the two provider profiles as five cases, 96 of 96 in a full run |
| `9df2e7b` | `WSL-70` partial: the base provisioner reads the shared package table and its detection; 4 mutation rows; `WSL-87` filed |
| `deea680` | `WSL-87` partial: `bootstrap.sh` installs CodeGraph under dash and names an npm too old for it; the last session's record and summary |
| `53a8f1f` | `WSL-86` filed and closed, `WSL-87` and `WSL-70` closed: the provisioner installs `nftables` and restores the id-mapping capability, `install_codegraph` names a kernel it publishes no package for, and all four presets build; 3 mutation rows and one defect planted by hand |
| `8801301` | `WSL-71` closed: `shell-profile.sh` moves an interactive shell off a Windows drive and leaves a marked or granted one alone, `base shell --here` marks its own shell through `WSLENV`, and `bootstrap.sh` writes its lines to the login file bash actually reads; 2 mutation rows and 5 defects planted by hand |
| `6f22e39` | the last session's record: `WSL-71` and `WSL-67` closed, `WSL-68` attacked, `WSL-88` and `WSL-89` written and never run |
| `f179609` | `WSL-76` and `WSL-78` partial: the four measurements, the muse reporter's four defects fixed with a case and 5 mutation rows, its lifecycle against a real herdr, `--machine` corrected out of every live page, and two failing commands out of the Muse example |
| `062ae9c` | `WSL-90` filed and approved: the release lookup reads past its first page, with 2 cases and 3 mutation rows; `herdr-build.yml`, `herdr-nightly.yml` and `release.yml`'s herdr jobs written |
| `cfa3252` | `WSL-90` partial: the herdr adapter's `nightly` channel, with 4 cases and 5 mutation rows, and the build matrix's first run read and answered |
| `42a7c5b` | the 2026-09-15 checkpoint finished by the session that resumed it: `WSL-90`'s second build matrix run recorded from its logs, 8 of 8 green and both fixes measured; the three closing reviews run, finding 31; five live pages that described a publication none has made; the work order corrected, finding 29; and that session's summary written from artefacts |
| `431417b` | `WSL-90` partial: the first herdr nightly published, its six signatures verified and both refusals proved, the base driven onto the `nightly` channel end to end, and the prune's `created_at` defect found by that first run and fixed |
| `b2203ab` | `WSL-90` partial: the `--remote` probe as a tracked script, driven against both clients, the nightly passing six signals and 0.9.0 failing three; three defects in the probe itself found by driving it; and the gate's `powershell` check, which could not fail for two independent reasons, fixed with 2 cases and 2 mutation rows |
| `69ae23f` | `WSL-90`'s three closing reviews, finding 36 and a line count corrected; the entry stays open on its step 4, which only `wsl-toolkit-v3.0.0` can prove |
| `a19926c` | `WSL-68` steps 3 and 4: `base doors`, the attack as a registered command, with 13 cases, 6 mutation rows, a manual section and a sweep row; three defects it found in itself and one about WSL that narrowed its claims |
| `ef0dd2f` | `WSL-68` step 1's first half: `base.shared_tmpfs = "off"` closes the shared tmpfs at every start and keeps the resolver, with 6 cases and 4 mutation rows, driven from nothing and proved able to fail |
| `0d4d66f` | `WSL-68` step 1's second half: `base exec --private-net`, the account's own network namespace with the Windows host refused by a rule inside it, 7 cases and 3 mutation rows; the engine conflict that makes it a flag |
| `c68ca25` | the teardown of that session's throwaway base |
| `7182b58` | `WSL-88` and `WSL-89` driven: pi 0.85.1 and omp 18.2.3 on one base with herdr, the collision refusal that could never fire made to fire, the operator's opt-in that separates, bun from the distribution, and that session's summary |
| `bf7753b` | ruling 22 recorded: the release stops waiting on issue 30, whose part 2 the operator deferred |
| `5eae667` | `base.adapters` takes `model` and `effort` and each agent is started on them by herdr, with pi's two silent failures raised by the probe; the shell profile's three new mechanisms, driven across 13 images; 6 mutation rows |
| `e0a239b` | the muse-code guide rewritten with every command run before it was written, two standalone skills, `base revoke --help` describing itself, and three defects writing the guide found; 1 mutation row |
| `6192e61` | the door sweep's find: `base bootstrap` puts `~/.local/bin` first and every agent wrapper is bypassed, with all three probes reading the name back on a LOGIN shell; 4 cases and 3 mutation rows |
| `1699de9` | `WSL-76`'s fifth item driven: `PreToolUse` and `PermissionRequest` reported from a real turn, the reporter's log naming the event, and the cancelled approval that leaves an agent `blocked` for ever |
| this record commit | the ending the 05:13Z session never wrote, from artefacts; `wsl-toolkit-v3.0.0` cut and its published assets verified; `WSL-90` closed on approach step 4; issues 30 and 32 commented rather than closed |

## Measurements

On Windows 11 Pro 26200, WSL 2.7.12, on 2026-09-17:

- **At the start of the 10:01Z session:** the doctor exit 0, its report stamped
  `2026-09-17T10:02:53Z`; the gate exit 0, **21 of 21**; `wsl -l -v` five
  distributions, matching the host state below, ⚠ with `podman-machine-default`
  **Running** where that list says Stopped; `repo release` read-only exit 0,
  `wsl-toolkit 3.0.0 -> wsl-toolkit-v3.0.0`, ready. ⛔ **Three commits the previous
  session had committed were never pushed**; CI run 35187005197 was green on
  `5eae667`, so they went at 10:03:30Z as run 35208465569.
- ⛔ **THE DEFAULT BASE'S PODMAN BOOT ID WAS STALE AND NOTHING SAID SO.** The first
  container check of this session exited **2** at `current system boot ID differs from
  cached boot ID; an unhandled reboot has occurred`, from the `wsl-toolkit` base rather
  than `wsl-toolkit-base`. `base ensure --repair` cleared it, exit 0, and both checks
  then passed. ⚠ **It is the same damage the previous session recorded and repaired on
  `wsl-toolkit-base` after running `wsl --shutdown` twice** - and the sibling base,
  which every `run` and `matrix` uses, was left behind. Finding 67.
- **The three pre-push checks, on `1699de9`:** Windows Go with `TEMP` at the 8.3 path,
  **377 top-level results, 357 passed, 20 skipped, 0 failed, exit 0**; `check-go.sh`
  exit 0 in `golang:1.25` in **27.9 s**; ShellCheck 0.9.0 in `ubuntu:24.04` clean over
  **49** tracked scripts, exit 0 in 22.6 s. ⚠ The Windows figure is 377 against the 363
  of the previous session's first commit and the 365 of its last, because
  `agent_default_test.go` and `shell_profile_test.go` are new.
- **The mutation table:** 327 rows at `bf7753b` to **338** at `1699de9`, 11 added by
  the previous session and all 11 reported red by it. ⭐ **`repo mutate --only
  release:` 7 of 7 guards proved on Windows** before the tag was cut, each row's cases
  green unmutated first.
- **For `WSL-59`, the premise re-measured on podman 6.1.1:** every row of the
  2026-09-10 table still holds, and the lifecycle feed carries a trap that returns
  nothing and exits 0. Both are in that entry's amendment of 2026-09-17 with their
  invocations; they are not repeated here.
- ⭐ **A second herdr nightly published at 10:12:44Z**,
  `herdr-nightly-20260917-e7e3dfa60e35`, 6 of 6 jobs green, resolved from a
  development head that had moved since `18061191fdc0`. ⚠ **It is the first time the
  prune has had more than one nightly to keep**, and it keeps seven, so nothing was
  removed. Finding 36, the prune that only runs on the write path, is unchanged by it.
- **At the start of the 02:52Z session:** the doctor exit 0 in 55 s at 02:53Z; the gate exit 0,
  21 of 21 in 70 s; `wsl -l -v` matched the host state, five distributions; CI run
  35070882239 for `485252c` green. ⭐ **A second `herdr-nightly` run, 35078871134, is
  green with build and publish SKIPPED** - herdr's head had not moved, so the workflow's
  idempotency path ran for real for the first time and published nothing. Finding 30
  said a green matrix does not cover what the nightly builds; this is the other half,
  and it worked.
- **For `WSL-68`'s `base doors`, on `wsl-toolkit-base`:** exit 0, 30 doors, 0 problems,
  6 open, **12 s** with the distribution up and 19 s including its start. The first
  driven run took **277 s** and reported a false CLOSED; both are in the entry's
  amendment with their causes. `--json` answers `wsl-toolkit-base-doors/1`.
- **For `WSL-68`'s interop measurement:** readings of
  `/proc/sys/fs/binfmt_misc/WSLInterop` across two distributions and three utility-VM
  lifetimes, with **both values on the same distribution and the same configuration**.
  The table is in the entry's amendment of 2026-09-17 and the manual's safety model.
- **For `WSL-68`'s `base.shared_tmpfs`, on the throwaway arch base `wsl-toolkit-b68b`:**
  `base recreate` from nothing exit 0 in **110 s**, the resolver written with 1
  nameserver before the door shut and the boot script installed; after the restart the
  account found `/mnt/wsl` not mounted, a write refused and `getent hosts` OK; `base
  doors` exit 0 with `fs.mnt-wsl-shared closed`, and **four** doors still open. ⚠ Not a
  delta against the six `wsl-toolkit-base` reports, which differs by more than this
  setting; the claim audit caught that conflation. ⛔ With
  the unmount removed from the boot script by hand, `base doors` exit **1** and `base
  status --probe` exit **1**, `usable false`; restored, both exit 0. ⚠ **The first plant
  was defeated by its own shell**, `false && umount ... || umount -l ...`, which still
  runs the fallback. ⚠ The podman machine was started to export the rootfs, which is
  what a base build from an OCI image needs.
- **For `WSL-68`'s private network namespace, on `wsl-toolkit-b68b`:** inside
  `pasta --config-net` the namespace is its own and `CapEff` is `000001ffffffffff`, so
  the account loads `nft` rules with no privilege; with them the Windows host goes from
  answering ICMP to **refused** while `1.1.1.1:443` and DNS stay up, and the shared
  `net:[4026531833]` is unchanged with a ruleset the account cannot even read. ⛔ **No
  container runs inside**, measured three ways, each failing on a path podman chooses
  because it reads itself as rootful. `base exec --private-net` forwards 0, 7 and 42
  exactly and adds **zero** stderr bytes, 116 against 116.
- **For `WSL-88` and `WSL-89`, on the throwaway arch base `wsl-toolkit-b89`:** herdr on
  the nightly channel, pi and omp together, `base ensure` exit **0**, all three healthy:
  herdr 0.9.0, pi **0.85.1**, omp **omp/18.2.3** with bun **1.4.2** from the distribution.
  `herdr integration status` reads `pi: current (v9)` and `omp: current (v10)` in
  different directories. ⛔ The collision refusal fired for the first time ever, exit 3
  naming both paths and the variable; with `"separate_agent_dir": true` the same
  collision gives exit 0, a wrapper that wins on PATH, and omp resolving to its own
  directory while the account's value and pi are untouched; a second ensure is
  idempotent. ⚠ **`base ensure` cannot be driven from Git Bash**: MSYS translates the
  guest path in herdr's SSH ProxyCommand and it fails with
  `execvpe(C:/Program Files/Git/usr/local/lib/...)`. From PowerShell it works.
- **The three pre-push checks, at the end of the session:** Windows Go with `TEMP` at

  the 8.3 path, **363 top-level results, 343 passed, 20 skipped, 0 failed, exit 0**; `check-go.sh` exit 0 in `golang:1.25` in 29 s; ShellCheck 0.9.0 in
  `ubuntu:24.04` clean over **49** tracked scripts, two more than last session because
  `doors.sh` and `private-net.sh` are new. ⚠ At the first commit the Go figure was 343.
- **The mutation table:** 312 rows to **327**. Every one of the fifteen new rows went
  red, in four runs, and every case was green unmutated first - which finding 1 says
  `repo mutate` does not do for itself. ⚠ Three rows reported `BROKEN, does not
  compile` before they were rewritten; finding 41.
- ⭐ **The throwaway base `wsl-toolkit-b68b` was built under `.tmp` and removed**, with
  its 1.2 GiB, and `podman-machine-default` was started to export a rootfs and stopped
  again. Five registered distributions at the start and five at the end. ⚠ Rebuilding it
  takes 110 s and its configuration is in the entry.
- ⚠ **`wsl --shutdown` was run twice**, to measure the interop handler across utility-VM
  lifetimes. It stopped every distribution, which is what the manual says it does; all
  five were Stopped at the start of this session and none was started by it except
  `wsl-toolkit-base` and `wsl-toolkit`. ⛔ **It invalidated the base's podman boot id**,
  and `base ensure --repair` cleared it, exit 0, the base usable with podman 6.1.1.

On Windows 11 Pro 26200, WSL 2.7.12, on 2026-09-15:

- **At the start of that session:** the doctor exit 0 in 39.71 s at 10:21:35Z; the gate
  exit 0, 21 checks green in 33.81 s at 10:22:19Z; `wsl -l -v` matched the host state
  of the last session; CI run 34955394330 for `6f22e39` green in all six jobs.
- **For `WSL-76` and `WSL-78`, on `wsl-toolkit-base`:** the four ordered measurements,
  the reporter's four defects and its lifecycle, the two herdr builds and the launcher
  from a granted project are in each entry's amendment of 2026-09-15, with their
  conditions. The Windows Go proof with the 8.3 `TEMP`: **324 results, 304 passed, 20
  skipped, 0 failed, exit 0 in 19.1 s**.
- **For `WSL-71`, the shell profile:** `matrix --images all` with a payload that drives
  every shell on the image twice, `--no-shell-profile` then with it, exit 0, **13 ran, 0
  failed, 0 unreached, 0 timed out, in 1m53s**; **28 shells, 0 that add a byte to
  stderr**, and all 28 read the profile. ⚠ The same command against the FIRST driver read
  13 ran, 2 failed, and both failures were the driver: chimera's `wc -c` pads its answer
  with spaces so a count of zero read as non-zero, and photon's own
  `/etc/profile.d/dircolors.sh` writes 66 bytes to every login shell with or without this
  file.
- **For `WSL-71`, on the throwaway arch base `wsl-toolkit-b71`**, automount `rw`, interop
  off, no systemd, the profile installed by `bootstrap.sh` from the mounted checkout: a
  non-interactive login shell on a Windows drive stayed with 0 bytes of stderr, an
  interactive one moved to the home with one 174-byte line, a shell carrying the `--here`
  mark stayed, and one already in the home stayed. `base ensure` exit 0 in 66.07 s. ⚠ **The
  first attempt failed in 57.34 s** at `geo.mirror.pkgbuild.com : Operation too slow`, and
  rolled the distribution back: finding 10 below, met for the third time.
- **For `WSL-71`, the login file bash reads:** `/etc/skel` ships `.bash_profile` and no
  `.profile` on arch, fedora and rocky 8, `.profile` and no `.bash_profile` on debian, and
  neither on alpine. On the arch base, with a fresh account each time: `HEAD` left the
  prefix off a bash login shell's `PATH` and the tree put it there, and both put it on an
  `sh` login shell's. In debian and fedora containers, the same result with a
  `~/.bash_profile` present, and no difference with none.
- **For `WSL-71`, WSLENV measured with `wsl.exe --exec /usr/bin/printenv`:** the variable
  set without being named in `WSLENV` did not reach the guest, in three spellings; named
  as `hereEnv` builds it, and appended to a caller's own list, it did.
- **For `WSL-71`, the five defects planted by hand** in copies cut by `write-file.mjs
  replace --expect 1`: each changed exactly one column of a six-column drive, and the
  restored file matched the unmutated row. 2 Go mutation rows red in `golang:1.25`.
- **For `WSL-68`, on the throwaway zero-grant base `wsl-toolkit-b68`**, arch, automount
  and interop off, no systemd, no sudo, no grant: `base ensure` exit 0 in 40.23 s. Every
  door tried as the account: drives, drvfs mounting, interop and passwordless sudo all
  refused; `/usr/lib/wsl/drivers` present and read-only; the internet reachable; the
  Windows host refused on 445 and 3389 and answering ICMP; `unshare -n` refused and
  `unshare -Un` granted. ⛔ `/mnt/wsl` written from the base and read from
  `wsl-toolkit`, and the reverse. Root's `umount /mnt/wsl` exit 0 in that distribution
  alone, the account's exit 32, and the mount back after `wsl --terminate`.
  `/etc/resolv.conf` resolves to `/mnt/wsl/resolv.conf`, so a new session after the
  umount answered `getent hosts` exit 2. `pasta --config-net` ran the probe in
  `net:[4026532318]` with the shared `net:[4026531833]` unchanged; inside it the internet
  answered and so did a ping to the Windows host, with and without `--no-map-gw`.
- **The suites, on this session's tree:** 315 top-level `wsl-toolkit` results on Windows
  with `TEMP` at the 8.3 path, 299 passed, 16 skipped, 0 failed, exit 0 in 16.5 s;
  `check-go.sh` exit 0 in `golang:1.25` in 32.15 s; ShellCheck 0.9.0 in `ubuntu:24.04`
  clean over **35** tracked scripts, one more than last session because
  `shell-profile.sh` is new. ⚠ It was clean by name BEFORE it was staged, because CI's
  command lists `git ls-files` and an untracked file is outside it.
- **For `WSL-86`, on throwaway instances under `.tmp` with automount and interop off:**
  arch exit 0 in 77.3 s and alpine in 172.9 s with `toolset developer`; debian exit 0 in
  94.0 s, where it had failed at `nft`, with `newuidmap` and `newgidmap` setuid; fedora
  exit 0 in 48.4 s with `toolset none`, its `newuidmap` and `newgidmap` arriving without
  their capability and given it. ⚠ Fedora's `developer` build installed all thirteen
  commands and ran a container in 420 s from a mirror answering in tens of KiB/s, then
  failed verification on `/mnt/e`; a second took the whole 30-minute budget. Three
  mutation rows red in `golang:1.25`, each green unmutated first.
- **For `WSL-87`:** the agent matrix `--images all --toolset agent`, 13 ran, 3 failed, 0
  unreached, 0 timed out, in 1184.45 s: debian, debian 12, ubuntu 22.04 and void-musl each
  `codegraph=1.6.0` and `failures=0`, rocky 8 naming npm 6.14.11 as too old, and chimera
  and gentoo failing as they did before. The kernel guard planted by hand in `golang:1.25`:
  unmutated exit 0, planted exit 1, restored exit 0. ⛔ The first plant read exit 0
  over the removed guard, because `go test` served a cached result for a shell file the
  build cache does not track; `-count=1` made it fire.
- **For `WSL-85`:** on the throwaway `wsl-toolkit-p67`, a sudo drift made `base status
  --probe` exit 1 in 0.5 s and `base ensure` provision again in 15.9 s and 16.0 s; a
  rule the tool did not write made `base ensure` exit 2 in 14.5 s. The Go suites green:
  305 top-level `wsl-toolkit` cases on Windows with `TEMP` at the 8.3 path, 297 passed,
  8 skipped; 304 in `golang:1.25`, 303 passed, 1 skipped. ShellCheck 0.9.0 in
  `ubuntu:24.04` clean over 34 tracked scripts.
- **For `WSL-67`'s provider profiles, on `wsl-toolkit-p67`:** a build from nothing with
  automount and interop off, systemd, the developer toolset and one read-write grant
  under `.tmp` took 67.3 s. As the account: the grant the one DrvFS mount beside WSL's
  own `/usr/lib/wsl/drivers`, a sentinel read and a file written back to Windows, no
  `/mnt` drive, no interop, systemd PID 1, `mount -t drvfs C:` refused. Zero grants: the
  probe exit 1 naming the stale mount, and `base ensure` removed it in 20.4 s. `base
  grant` and `base revoke` 0.31 s and 0.29 s.
- **For `WSL-67`'s acceptance cases:** the five alone 5 of 5 in 105.1 s, and red over a
  build with `WSL-85`'s check taken out; the full runner exit 0, 96 of 96 cases in
  465.3 s from 01:33:26Z.
- **For `WSL-70`:** with the developer toolset, arch built in 51.5 s and alpine in 44.2 s;
  debian failed at the `nft` its podman reaches for, as `4c573cf`'s build did with no
  toolset in 33.6 s, and fedora at `newuidmap`'s missing capability. The agent matrix with
  `--codegraph none`: 13 ran, 2 failed, in 340.6 s; with CodeGraph on, 7 failed in 517.3 s.
- **For `WSL-87`:** npm 6.14.11 and 7.17.0 have no `--pack-destination` and 7.18.0 has;
  both cases green under dash in `golang:1.25` and red with each defect planted by hand;
  over the fix, debian, debian 12, ubuntu 22.04 and void-musl install CodeGraph 1.6.0.
- **For `WSL-67`'s Nix work, on 2026-09-14:** in `docker.io/nixos/nix:latest`, Nix
  2.35.2, a new unprivileged account installed 16 names through flakes in 24 s and again
  in 10 s; a `nix-env` account through channels in 9 s; 25 of 25 table attributes
  evaluate.
- **WSL networking:** NAT mode, host address `172.23.96.1`, read by `wsl-toolkit
  hostaddress`. Two distributions share one network namespace, in `WSL-68`.
- **herdr:** ⭐ **`v0.9.1` is the newest stable release**, published
  2026-09-16T18:40:01Z, read from `repos/herdrdev/herdr/releases/latest` on
  2026-09-17. ⚠ **It supersedes the reading of 2026-09-13**, which had 0.9.0 of
  2026-09-07 as the newest with `herdr-linux-x86_64` at 24,644,488 bytes and
  `herdr-windows-x86_64.zip` at 9,054,745 bytes; those two figures are 0.9.0's and are
  not 0.9.1's. ⭐ **v0.9.1 asks for Zig 0.16.0 and Rust 1.96.1**, read from its own
  `vendor/libghostty-vt/build.zig.zon` and `rust-toolchain.toml` on 2026-09-17, and
  `herdr-build.yml` pins a digest for 0.16.0 on both architectures - the same one the
  nightly built with twice on 2026-09-17. ⛔ **Whether v0.9.1 carries the `#4038`
  repaint fix is NOT measured here**; `herdr-remote-probe.ps1` is what would settle it.
- **Muse:** Meta's installer at `dev.meta.ai` still has the approved digest, 9,314
  bytes, on 2026-09-14; the public channel serves `1.2.1-R2847.1`, a 299,251,896-byte
  Linux x86 build that answers without credentials.
- **For `WSL-71`:** on the throwaway `m71`, with automount off WSL starts a command in
  the account's home; with it on, in the Windows directory.
- **For `WSL-67`'s BSD drives:** NetBSD 11.0 and OpenBSD 7.9 are the current releases,
  read from the mirrors on 2026-09-14. QEMU 11.1.0 here has no `sga` device; with no
  disk, SeaBIOS's banner and boot messages reached `-serial stdio` under `-M
  q35,graphics=off` and under an `etc/sercon-port` fw_cfg file, and nothing did under a
  plain `-M q35`, on 2026-09-15.

## Found, and not filed

1. ⛔ `repo mutate` never runs a row's cases unmutated, so a case already red
   reports "went red".
2. ⭐ **Fixed on 2026-09-16, and it was worse than this entry said.** The
   separator mismatch was real - the child writes `PARSE|` and the reader looked
   for `PARSE\t` - but repairing it alone left the check still unable to fail,
   because of a **second, independent** defect this entry never named: the file
   list was passed as arguments after `pwsh -Command`, which does not reach
   `$args` at all. Measured: that invocation answers `ARGS_SEEN|0` and echoes the
   file names as output. So the loop ran zero times over zero files and reported
   ok over every broken script in the tree, for as long as the check has existed.
   The list now arrives in the environment, the child returns how many files it
   actually parsed, and the caller refuses any count that is not the number it
   handed over. **2 cases and 2 mutation rows, both red when planted**, and a
   syntax error planted by hand in a tracked `.ps1` takes the check to exit 1
   naming the file. ⚠ The cases need a real PowerShell and are in their own test
   function, so a host without one reports SKIPPED rather than passing.
3. ⛔ `scripts/common/check.ps1` resolves the repository from the working
   directory rather than its own location.
4. `bootstrap.sh --dry-run --toolset agent --codegraph none --json` exits 1 on
   Debian 13; its cause has not been read.
5. A deterministic regression for pre-marker base rollback is still owed. ⚠ **Met again on 2026-09-15:** a fedora build stopped mid-provisioning left `wsl-toolkit-t86fedora` registered, and `base remove` refused it with `carries no wsl-toolkit identity marker`, so `wsl.exe --unregister` was used by hand.
6. ⭐ **CLOSED ON 2026-09-17.** The generated manual printed a one-letter flag as
   `--c`, which the parser accepts and no example writes, so the page was wrong
   in **six** places and every command still worked. `flagDashes` writes one dash
   for a one-character name, the page is regenerated, and the six read `\fB-c\fR`.
   ⛔ **The door sweep on the fix found a SECOND renderer**: `cmd_manual.go` writes
   flags twice, once as roff and once as plain text for the readable manual, and
   fixing the roff half left `    --c` in the other. Both are fixed and the one
   case asserts both, in both directions - a rule that only checked the short
   flags would pass over a generator that dropped a dash from every flag. 1
   mutation row, red when planted.
7. ⭐ **Closed on 2026-09-15.** `examples/common/zellij.md` is deleted, the manual's
   link to it now names `herdr.md`, `examples/muse-code/README.md` is rewritten from
   eleven commands to three, and `examples/common/README.md` no longer tells a reader
   the agents' durable session is tmux. No live document names Zellij; the entries
   that do are recording a decision, and [`ENTRY.md`](ENTRY.md) forbids rewriting a
   title or a premise after the fact.
8. ⚠ **An ephemeral job says its output is kept, and it is not.** `run
   --container-lifecycle ephemeral` against `golang:1.25` printed `the complete
   output is kept: wsl-toolkit logs 4e94faa38b4c9c1c`; `logs` on that id answered
   exit 2, `no transcript`, and `jobs\4e94faa38b4c9c1c` was not in the state
   directory. Measured on 2026-09-14.
   ⚠ **RE-MEASURED ON 2026-09-17 AND IT DOES NOT REPRODUCE.** Two
   `--container-lifecycle ephemeral` runs against `golang:1.25` and `ubuntu:24.04`
   both printed the line, `logs` answered **exit 0** on each with the container's own
   output, and each job directory holds `stdout.log` and `stderr.log`. ⛔ **The
   original id can no longer be checked**: `jobs\4e94faa38b4c9c1c` is still absent, so
   whether it was ever there is unanswerable now. ⭐ **The code was read**, which the
   finding said had not been done: `transcriptHint` in `cmd_run.go` prints the line
   only when `res.Transcript` is non-empty, and ⛔ **that guard has been there since
   `cdb7c9c` on 2026-09-10, four days BEFORE the observation** - so the cause was not
   an unconditional hint, and what it was is unidentified. The finding stays open,
   narrowed to that.
9. ⚠ **A cancelled `bsd run` is reported as a budget that ran out.** Read in
   `bsd.go` on 2026-09-14 and not measured: a cancelled context ends the boot's wait
   with `the guest did not reach a login prompt within` the budget, and a command's
   with `the guest did not finish the command within the budget`, both exit 2, where
   `distro run` answers a cancellation with 130.
10. ⚠ **A base build fails when one Arch mirror stalls, and nothing retries.** Two
    `base ensure` builds from nothing on 2026-09-14 failed in `pacman` with
    `geo.mirror.pkgbuild.com : Operation too slow`, and each rolled its distribution
    back; the next attempt passed.
11. ⚠ **`wsl-toolkit-podbox` has every drive mounted `9p rw`**, read on 2026-09-14,
    and no configuration of its own, so it answers to the default `ro`. Its next
    `base ensure` provisions it again. It is not this session's to change.
12. ⚠ **23 more lines in nine files name an error by the first line of a guest
    command's stderr**, where wsl.exe can write a line of its own first: `adapters.go`,
    `base.go`, `cleanup_run.go`, `engine.go`, `inspect.go`, `job.go`, `matrix.go`,
    `resources.go` and `throwaway.go`, found by `WSL-85`'s door sweep on 2026-09-15.
    `WSL-85` fixed the verifier's; none of the others was measured naming a wsl.exe
    line.
13. A base built from nothing with automount and interop off printed 93 `wsl: Failed
    to translate` lines between provisioning and its restart, measured on 2026-09-15.
    Read, not measured: `appendWindowsPath=false` takes effect at that restart.
14. The verifier's `interop on` checks nothing, so a base configured with interop on
    over a guest without it verifies. Read on 2026-09-15, not measured, and it grants
    no authority.
15. ⚠ **A verification that fails is named by the FIRST guest line, and podman writes a
    warning before its error.** `verifyError` in `base.go` takes `firstLine` of the guest's
    output, so fedora's `newuidmap: write to uid_map failed: Operation not permitted` sat
    behind podman's shared-mount warning and needed a diagnostic build to read, on
    2026-09-15. It is finding 12's family from the other side: not wsl.exe's line but the
    engine's own first one. Found by `WSL-86`'s door sweep.
16. `fetch_verified_npm` in `bootstrap.sh` calls `split_on`, which lives inside the shared
    package table block and is generated into `packages.sh` for the provisioner, so a
    change to `split_on` made for the table changes the npm work directory's name. Read on
    2026-09-15 by `WSL-87`'s door sweep, not measured, and nothing is built on it.
17. `bootstrapSource` in `bootstrap_npm_test.go` reads five levels above its package, which
    is outside the Go module, so a `repo mutate` row naming one of those cases would report
    the file unreadable rather than a guard's verdict. No row names one. Read on 2026-09-15.
18. ⚠ **The provisioner sweeps the automount mount points once, and a Windows drive that
    appears after that sweep is left behind.** Measured on 2026-09-15: this host carries
    ten fixed drives; the arch, alpine and debian builds each removed ten mount points and
    the fedora build, which ran for 420 s, removed **nine**, and its verification then
    refused the base with `/mnt/e exists even though automount is off`. E: is an external
    HDD. The sweep at `provision.sh:494` runs before the restart that applies `automount
    off`, so a drive WSL mounts between the sweep and that restart leaves an empty 0777
    directory the verifier is right to refuse. Found by `WSL-86`'s driven pass.
19. `bootstrap.sh --dry-run --toolset agent --codegraph none --json` exits 0 in
    `golang:1.25` and 1 in `debian:latest`, measured on 2026-09-15, which narrows finding 4
    to what the image carries rather than to Debian.
20. ⚠ **`base ensure` does not install the shell profile, and nothing says it should.**
    `provision.sh` and `verify.sh` write no login file at all, so a base built by `base
    ensure` alone carries no `~/.profile` line and `base shell --here`'s mark has nothing
    to mark for. `bootstrap.sh` run inside the base is what installs it. Found by
    `WSL-71`'s door sweep on 2026-09-15, recorded in the manual, and deliberately not
    changed: merging the two would put a URL-fetched file inside `base ensure`.
21. ⚠ **`--here` and `--root` together mark root's shell, and root has no profile to
    read it.** `base shell --root --here` takes the same branch and gets
    `WSL_TOOLKIT_HERE`, but `bootstrap.sh` installs the profile into the managed
    account's home, not root's. Harmless - the mark is read by nothing - and read rather
    than measured, on 2026-09-15.
22. ⛔ **`/dev/kvm`, `/dev/dxg` and `/dev/vsock` are `crw-rw-rw-` in a zero-grant base**,
    read on 2026-09-15 while attacking `WSL-68`'s doors. None was attacked, so what any
    of them reaches from an unprivileged account is unmeasured; they are named here
    because a sealed base's threat model has to answer for them and this session's
    enumeration did not.
23. ⛔ **The verifier's `automount off` check cannot fire on a guest whose automount
    root is not `/mnt`.** `verify.sh:34` hardcodes `drive_root=/mnt`, while
    `shell-profile.sh`, written this session, reads `[automount] root` from
    `/etc/wsl.conf`. So the two disagree about what "a Windows drive" is. ⚠ A base this
    tool BUILDS is unaffected: `provision.sh:473` writes an `[automount]` block with no
    `root` key, so the default holds. A distribution adopted with a hand-set root would
    have its drives mounted where the verifier does not look. Read on 2026-09-15 by
    `WSL-71`'s door sweep, not measured, and it is a value in two places with no check
    that they agree - they cannot be merged, because one is embedded in the executable
    and the other is fetched by URL and must work with no executable at all.
24. ⚠ **`base status --probe` reports no cgroup delegation on every base built this
    session**, arch at two different configurations, so a memory or cpu limit is accepted
    and not enforced. The remediation says the tool cannot repair it. That is `WSL-60`'s
    known condition and is recorded here only because two more builds met it.
25. ⭐ **Fixed on 2026-09-15:** `base attach` printed `base exec -c 'herdr agent list'` as
    the line for an agent, the quoted shell string `base herdr` exists to avoid. It
    prints `base herdr -- agent list`, and its case asserts the prefix.
26. ⚠ **The muse probe proves the reporter's registration by reading the file the
    install wrote, not by Muse running the hook**, which is the class
    [`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md)
    now has a row for. The registration was measured against Muse by hand on
    2026-09-15; nothing re-measures it when Muse changes its settings file again.
    Muse's `--provider echo` runs `SessionStart` with no credential, which is the
    candidate check.
27. ⚠ **herdr's Muse manifest `2026.08.26.1` matches no rule on Muse Code 1.3.0's input
    screen**, measured on 2026-09-15, so without the reporter herdr shows a Muse pane as
    `idle` whatever it is doing, and `agent prompt --wait` depends on the reporter's
    `working` to pass its five-second activity gate. Upstream's to fix.
28. ⚠ **A build of herdr's development branch answers `herdr 0.9.0`**, because the
    branch has not moved `Cargo.toml`. Any build this repository makes or ships cannot be
    told from the release by `--version`; its digest is the only identity.
29. ⛔ **The work order outlived the work by a session, and misrouted a resuming
    session.** Item 2 above still read "`WSL-67` has left: driving `pkgin` and
    `pkg_add`, which waits for the operator's approval of the two downloads" after
    `6f22e39` closed the entry on a driven NetBSD guest. A session resumed on
    2026-09-16 was sent to re-ask for an approval the operator had already given and
    to re-drive two package managers already driven. ⚠ **`INDEX.md` and the entry both
    read `done` throughout**, so the record disagreed with itself and only the work
    order was wrong; the three-way reconciliation the methodology asks for is what
    caught it.
    ⭐ **CLOSED ON 2026-09-17: the check is written and it is rule 7 of
    `check-record`.** An item marked Closed may not name an entry `INDEX.md` calls
    open, and an item that is not marked Closed must name at least one entry it
    does not call done; an item naming no entry is exempt, because item 5 of the
    real order is a release. ⛔ **It met the defect a second time first**: the
    prompt that opened the session of 2026-09-17T10:01Z sent it to `WSL-68`'s
    "four remaining items, which need nobody" after three had closed and the
    operator had deferred the fourth. 9 cases and **4 mutation rows, 4 of 4 red**,
    each green unmutated first. ⚠ **One case reads the message and not the count**,
    because with the heading guard off a second guard reports one problem too and
    a count could not tell the two apart.
30. ⚠ **A green `herdr-build.yml` matrix does not cover what the nightly builds.** The
    matrix was dispatched against a pinned ref, `052779c4159ed851`, while
    `herdr-nightly.yml` resolves herdr's development head at run time; by 2026-09-16
    that head had moved to `18061191fdc0`. So the first nightly built a commit no
    matrix run had ever built. Read on 2026-09-16, and it is the design working as
    written rather than a defect - recorded because "the matrix is green" is not the
    same sentence as "the nightly will build".
31. ⛔ **`consumer.ps1` downloads every asset a release carries, and from
    `wsl-toolkit-v3.0.0` that includes herdr's four builds.** `Get-Release`'s `gh` path
    is `gh release download` with no `--pattern`, and its no-`gh` fallback reads
    `SHA256SUMS` and fetches every name in it plus a `.cosign.bundle` for each;
    `release.yml` writes herdr's four builds into that same `SHA256SUMS`. So this
    repository's own reference runner, and the weekly release smoke that drives it,
    will fetch four herdr binaries neither uses, while
    [`../docs/consumers.md`](../docs/consumers.md) tells a consumer to download only
    the executable for its architecture and `SHA256SUMS`. Found by `WSL-90`'s door
    sweep on 2026-09-16. ⚠ **Now sized from the first nightly's own assets**, which are
    what `release.yml` would add: 26,235,080 + 24,100,240 + 9,635,803 + 8,348,489 =
    **68,319,612 bytes, about 65 MiB**, plus four bundles. `release-smoke.yml` runs
    `consumer.ps1` every Monday at 07:00 UTC, so that is the recurring cost from
    `wsl-toolkit-v3.0.0`. ⚠ It is a cost and a page disagreeing with a script, not a
    consumer break: no consumer in the register fetches `consumer.ps1`.
32. ⚠ **A killed `repo mutate` leaves its whole staged copy behind, and nothing sweeps
    it.** `.tmp\g\mutate-432424907`, **127 MiB**, has been there since 2026-09-14.
    `mutate.go:157` stages with `os.MkdirTemp("", "mutate-")` and `mutate.go:162` removes
    it with a `defer`, which a killed process never runs, so the copy survives in
    whatever `TEMP` named at the time - here `.tmp\g`, the 8.3 directory that session
    used. It is the same class as the killed matrix `wsl-toolkit gc` exists for, and
    `gc` does not cover it because the directory is not this tool's job state. Found on
    2026-09-16; the directory is left in place as evidence and is safe to delete by
    literal path.
33. ⛔ **A workflow's own logic has no harness here, and a real defect shipped inside
    one.** `herdr-nightly.yml`'s prune sorted by the wrong date field and would have
    deleted the newest nightly; it is fixed and driven by hand against a crafted list,
    and it has **no case and no mutation row**, because nothing in this tree runs a
    workflow's shell the way `repo mutate` runs Go and `check.sh` runs the tree. The
    gate's `mutations` check cannot point at a `jq` expression inside a YAML `run:`
    block. `consumer.ps1` has the same gap for the same reason - the amendment of
    2026-09-15 already noted it has no case harness - so this is one gap with two known
    occupants, and both are load-bearing: one publishes, one deletes. Found on
    2026-09-16 by `WSL-90`'s driven pass.
34. ⚠ **A `.ps1` under `$ErrorActionPreference = 'Stop'` cannot exit through
    `Write-Error`.** The preference makes it a terminating error, so an `exit 2` written
    after it never runs and pwsh ends **1** - which silently merges "could not run" into
    "a check failed". Met on 2026-09-16 in `herdr-remote-probe.ps1` and fixed there with
    `[Console]::Error.WriteLine`. ⚠ **Not swept for elsewhere**: `acceptance.ps1`,
    `consumer.ps1` and the `check-*.ps1` wrappers set the same preference and document a
    distinct exit 2, and none was read for this shape.
35. ⚠ **A guard that reads a field can be defeated by the read itself throwing.**
    `herdr-remote-probe.ps1` read a workspace id from the wrong property; StrictMode
    made the missing property a terminating error, which jumped past the assignment, so
    the id the cleanup path needed was exactly what had failed to be read, and the
    workspace leaked. Fixed by reading through a helper that returns null rather than
    throwing. ⚠ It is a general shape under StrictMode, not a fact about this file:
    anywhere a cleanup handler depends on a value assigned from a property read, the
    read's own failure disarms the handler.
36. ⚠ **The old herdr client is pruned only when a new one is WRITTEN.** The loop that
    removes every other `herdr-nightly-*` directory lives inside the function that
    installs a client, so a `base ensure` that finds the current client already matching
    its digest short-circuits and never reaches it. Driven on 2026-09-16: a decoy
    `herdr-nightly-20260901-aaaaaaaaaaaa` and an unrecognised `not-a-nightly-dir` were
    planted under the instance's herdr directory, `base ensure` exited 0 printing
    `herdr ... is installed and matches its pinned digest` and no `removed the herdr
    client` line, and **both survived**. ⚠ **The amendment of 2026-09-15 says the host
    half "removes older ones"**, which is true of the write path and not of an ensure;
    the correction is here rather than in that premise. ⚠ It self-corrects at the next
    nightly, each directory is about 9.6 MiB extracted, and the loop is deliberately
    conservative - it only ever removes a name matching the nightly tag, so the
    unrecognised directory surviving is correct behaviour and not part of the finding.
    Both decoys were removed by literal path afterwards.

37. ⚠ **A case's verdict depends on what ran before it.** `jsonSurfaces` in
    `sweep_test.go` reads `flagSets.byName`, a package-level registry that
    `collectManualFlagSets` resets and repopulates, and it adds to whatever is
    already there rather than building its own. So `go test . -run
    TestEveryJSONSurfaceReachesTheSweep` reports four `sweptElsewhere` rows as stale
    - `base grant`, `base revoke`, `bsd fetch`, `bsd run` - and a full package run
    reports none, because a manual case ran first and registered every HelpForm. Met
    on 2026-09-17 while adding `base doors`: the isolated run's message sent this
    session looking for a defect that a full run does not have. ⚠ The case is right in
    a full run, which is how CI and the gate call it; it is the isolated invocation
    that lies. Not fixed, and nothing is built on it.
38. ⛔ **`base doors` reports `/dev/kvm`, `/dev/dxg` and `/dev/vsock` and still does
    not attack them**, which is finding 22 carried forward rather than closed. All
    three read `crw-rw-rw-` again on 2026-09-17. What an unprivileged account reaches
    through any of them is unmeasured, and the manual's safety model says so in the
    row rather than leaving the reader to infer it.

39. ⛔ **`baseUsage` and the manual's `HelpForms` disagree about what `base` has, in
    BOTH directions, and no check asserts they agree.** Found on 2026-09-17 by
    `WSL-68`'s door sweep, grepping for every base subcommand named anywhere rather
    than reading either list. `cmd_base.go`'s usage text names `herdr` and `bootstrap`,
    which `main.go:119` does not, so neither reaches `wsl-toolkit.1` or `man` and
    neither has its flags documented anywhere; `main.go:119` names `agent`, which the
    usage text does not, so a reader of `base` with no arguments never learns it
    exists. ⚠ Three subcommands, one tool, two lists written from memory at different
    times. `base doors` was added to both, which is how the disagreement was seen at
    all. Not fixed: documenting three more commands is its own unit of work with its
    own proof, and this is where it is tracked.

40. ⚠ **`base agent`, `base herdr` and `shipped write` build their own request as the
    account and none takes `--private-net`.** So an agent started through `base agent`
    runs in the shared network namespace whatever a sealed base intends. Found on
    2026-09-17 by `WSL-68`'s door sweep, grepping every builder of an `ExecRequest`
    rather than reading the two the task named. ⛔ Not closed, because closing it is
    the same design question the operator is asked in question 3: whether a base may
    declare the namespace for ALL of the account's processes and give up running
    containers as that account.
41. ⛔ **A `repo mutate` row that stops the module compiling reports `BROKEN`, which is
    neither red nor green.** Met three times on 2026-09-17: deleting a guard left the
    variable it read unused, and the row read `does not compile`. ⚠ **It is easy to
    read as proved** in a run where other rows say `ok`, and the exit code is 1 for
    both a broken row and a guard that failed to go red. A mutation that keeps the
    variable referenced, by comparing it against a value the setting never takes, goes
    red properly. The table has no rule that a row must compile, and nothing checks it.

42. ⛔ **A case that only runs on one host was written, passed here, and PUSHED RED.**
    `TestTheStatusReportCarriesTheSharedTmpfsSetting` built a `Base` to read a setting
    back, and `NewBase` binds to the host through `FindWsl`, which on Linux answers
    `wsl.exe was not found on this host: this host is linux`. It passed on Windows, the
    local gate ran `go test` on Windows and passed, and `check-go.sh` in `golang:1.25`
    is the check that would have caught it - which was run BEFORE that case existed and
    not again after. It went out in `0d4d66f`. ⚠ **The case never needed a host at all**:
    it asserts a normalizer and a JSON tag, both facts about the types, and it does that
    now. ⛔ **The rule this breaks is already written**: PROGRESS's own "Before every
    push" says a green local gate is not CI, and the three container checks exist for
    exactly this. Running them once at the start of a change and not again at the end is
    the gap, and nothing enforces the order.

43. ⛔ **THE `wsl-toolkit` ON THE OPERATOR'S PATH CANNOT READ THE BASE THIS TREE
    WRITES.** `%USERPROFILE%\bin\wsl-toolkit.exe` is **2.0.2**, built 2026-09-11, the
    last cut release; the configuration carries `automount`, added after it, and the
    loader uses `DisallowUnknownFields`, so every command the operator types as
    `wsl-toolkit ...` exits 2 with `json: unknown field "automount"`. Met on 2026-09-17
    when the operator ran `wsl-toolkit --instance base base shell` to sign in to Muse
    and could not. ⚠ **Neither half is wrong**: ruling 6 forbids cutting `v3.0.0` yet,
    so the released binary is correctly old, and the tree is correctly ahead of it. ⛔
    **What is wrong is that every page and entry writes the command as `wsl-toolkit`**,
    which is the binary that fails, rather than the build under `.tmp`. A reader
    following this repository's own documented commands on this host gets an error about
    their configuration being unreadable, which is the opposite of what is true.
    ⚠ Not fixed. The narrow fix is a sentence; the real one is `v3.0.0`, which ruling 6
    gates on issues 30 and 32.

44. ⛔ **`base shell` attached to `/bin/sh` with no profile read, on a base that has
    bash.** `wsl.exe -d N -u U` runs the account's passwd shell and the provisioner
    leaves that at `/bin/sh`, so an interactive attach had no line editing, no history
    and no login profile. Measured 2026-09-17 on `wsl-toolkit-base`: passwd shell
    `/bin/sh`, bash at `/usr/sbin/bash`, `BASH_VERSION 5.3.15`. Reported by the operator.
    ⭐ Fixed in `base shell`: the guest is asked, in the same call, to exec bash as a
    LOGIN shell and to fall back to `/bin/sh -l`, so a guest with no bash still gains the
    login half. ⚠ **The deeper fix is not done**: the provisioner still creates the
    account with `/bin/sh`, so every other route in - a herdr pane, `base exec`, an SSH
    session - still lands in sh. Changing the account's shell is a change to a base that
    already exists and is its own unit of work.
45. ⚠ **`wsl-toolkit --instance base base shell` says "base" twice, and both are right.**
    `--instance base` names the instance and `base` names the command group; the clash is
    only that ruling 2 named the instance the same word as the group. ⭐ **There is a way
    out and it is documented but unused**: `WSL_TOOLKIT_INSTANCE=base` makes it
    `wsl-toolkit base shell`. Raised by the operator on 2026-09-17. Not changed: renaming
    either half now would break ruling 2 or every page that names the command.
46. ⚠ **herdr's `tab create --cwd PATH` is ignored by the published nightly.** Measured
    2026-09-17 on `herdr-nightly-20260916-18061191fdc0`: a tab created with
    `--cwd /workspaces/proj` and another with `--cwd /tmp` both reported `cwd` and
    `foreground_cwd` as the account's home. ⛔ It is upstream's, not this tree's, and it
    is why `WSL-76`'s drive could not place the agent's pane in the granted project.

47. ⛔ **A FILE'S SHAPE WAS INVENTED FROM A TRUNCATED READ, AND IT BROKE THE
    OPERATOR'S AGENT.** The muse adapter's new trust step wrote `{"workspaces":{}}`.
    Muse's store is `{"schema_version": 1, "projects": {...}}` - **two** things wrong,
    the key and the required field - and Muse then refused to start at all:
    `malformed trust store at ~/.config/muse/trust.json: missing field schema_version`.
    The operator hit it in their own terminal. ⛔ **The cause is exactly the claim-audit
    lens's own subject**: the shape was taken from a `cat` whose output had been cut by a
    `Select-Object -Last`, so the parent key was never actually seen and was assumed.
    ⭐ **Fixed and driven three ways**: an existing store is EDITED with `jq` so
    `schema_version` and every other project survive; a store that does not parse as one
    stops the adapter, exit 3, `it is left exactly as it is`, proved by planting the
    exact malformed file and reading it back unchanged; and a store that is absent is
    created in Muse's own shape. ⚠ **The operator's file was restored from the backup the
    step itself had taken**, which is the only reason this cost minutes rather than a
    re-authentication.
48. ⛔ **Muse ENFORCES A SANDBOX AND THE BASE HAD NONE, so a signed-in agent ran
    nothing.** Measured 2026-09-17: with no `bwrap`, every command Muse was asked to run
    came back `The execution environment is broken: the command was never started ...
    sandbox enforcement unavailable (muse-bin under writable root, no usable bwrap)`, and
    `herdr agent prompt --wait` returned **exit 0** with that refusal as the answer. ⭐ **A
    green run where the work never happened**, which is the shape this repository hunts.
    Arch carries `bubblewrap 0.12.0`; the adapter installs it now and the probe reports
    `sandbox bwrap 0.12.0` as a fact, so a base without it is a problem rather than a
    healthy base that refuses everything. ⚠ **The check is made at Muse's startup**, so an
    agent already running does not pick it up and has to be started again.

49. ⛔ **AN AGENT npm PUTS IN `$HOME/.local/bin` CANNOT BE STARTED BY herdr AT ALL, AND
    THE ADAPTER'S OWN CHECK COULD NOT SEE IT.** herdr launches an agent by running its
    canonical name in a pane, and that pane is a LOGIN shell whose PATH is
    `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:...` with no `~/.local/bin`;
    `/etc/profile` appends `/usr/local/bin` and nothing else. Measured 2026-09-17 on the
    operator's base: `pi` and `omp` both installed healthy, both answered a version, and
    `herdr agent start` timed out on each because the pane said `command not found`. ⭐
    `muse` was the only one that worked, because it has had a root-owned wrapper at
    `/usr/local/bin/muse` since it was built. Both adapters write one now.
    ⛔ **And the first guard written for it was THEATRE.** It read the name back through
    `as_account`, whose PATH puts the npm prefix first, so it resolved `~/.local/bin/pi`
    and would have passed with no wrapper at all. It uses `runuser -l`, a real login
    shell, which answers `/usr/local/bin/pi` - proved by removing the wrapper and
    watching `base status --probe` go to exit **1** naming it, and back to 0 restored.
    ⭐ The contract is in `adapters/README.md` so the next adapter does not repeat it.
50. ⚠ **`herdr agent prompt` answers `agent_prompt_stalled` for an agent with no
    credential**, `no observed working or blocked state within 5000 ms; current status is
    idle`. Met on 2026-09-17 for `pi` and `omp`, which are installed, integrated and
    started but reach no model. ⭐ It is the right answer and not a defect: the agent
    never moved, and herdr says so rather than waiting out the full timeout.

51. ⛔ **AN AGENT herdr STARTS INHERITS THE herdr SERVER'S ENVIRONMENT, NOT A LOGIN
    SHELL'S.** So an `export` in the account's `~/.profile` never reaches it. Measured
    2026-09-17 on the operator's base: the variable was set and 51 characters long in a
    login shell, `pi auth check` answered `ready` under `runuser -l`, and **not one**
    shell in a herdr pane had it - read from `/proc/PID/environ` for every pane process.
    herdr runs as `wsl-toolkit-herdr.service`, a systemd unit, which reads no profile at
    all, and its panes inherit from it. ⛔ **So a credential an agent needs cannot be
    delivered by a shell startup file**, and the guide must not tell anyone to try. It
    belongs in the agent's own credential store - `~/.pi/agent/auth.json` for pi, which
    pi reads whatever the environment is. ⚠ **This tree has no way to give an agent an
    environment variable**, and `base agent` carries none either; that is the gap behind
    both, and it is not filled.
52. ⚠ **pi picks a built-in model when one is not named, and sends the configured key to
    it.** Started with no `--model`, pi chose `claude-opus-4-8` and answered
    `401 authentication_error: API key is invalid` - it had used the `anthropic` entry in
    `auth.json` against the real Anthropic API. Started with `--model
    muse-gateway/spark-max` the status line reads `spark-max` and the provider is the
    gateway. ⭐ Not a defect, and worth writing down: an agent adapter that configures a
    custom provider has to pin the model too, or the agent silently uses another one.

53. ⛔ **A STARTUP MODEL PI CANNOT RESOLVE IS NOT AN ERROR TO PI, and this is finding 52
    made worse rather than the same finding.** `~/.pi/agent/settings.json` carried
    `defaultProvider muse-gateway` and `defaultModel muse-spark-1.3-contributor`, both
    correct and both what the operator asked for, and pi's own status line still read
    `(anthropic) claude-opus-4-8`. ⭐ **The cause: pi resolves a startup model against
    its own catalogue, and `models.json` declared four `spark-*` ids and not that one.**
    The gateway serving a model and pi's catalogue knowing it are two different facts -
    `GET /v1/models/muse-spark-1.3-contributor` answers HTTP 200 on the gateway, and pi
    could not select it. ⚠ **Nothing is printed when this happens**, which is why
    `base status --probe` now reads the startup model back through `pi --list-models`
    and raises a problem. Proved by removing the id and watching the probe go to exit 1.
54. ⛔ **`max` CLAMPS TO `high` WITH NOTHING SAID, unless the model declares a
    `thinkingLevelMap` that exposes it.** pi's own `docs/models.md`: with the map
    omitted, "Standard levels through `high` use the provider's default mapping;
    extended `xhigh` and `max` levels are unsupported". So a base configured for `max`
    started every session at `high`, and the status line said `high` while the settings
    file said `max`. ⭐ Adding `"thinkingLevelMap": {"xhigh": "xhigh", "max": "max"}` to
    the gateway's models made the same session read `max`. The probe raises a problem
    for this too.
55. ⭐ **`spark-max` WAS NOT AN INVENTED ID.** The lens "what did I measure but never
    verify" went after last session's `models.json`, because the gateway's `/v1/models`
    listing does not contain `spark-max`, `spark-hi`, `spark-lo` or `spark-min`.
    ⚠ Asking for each one by name answers **HTTP 200**, so they are aliases the gateway
    serves and does not list. The entry stands; what was missing was the check.
56. ⛔ **MUSE REFUSES A ROOT OPTION IN FRONT OF A SUBCOMMAND, AND REFUSES A REPEATED
    ONE.** `muse --model M config status` exits 2 with `unknown argument config`,
    because a root option makes the whole line a TUI line; `muse --model a --model b`
    exits 2 with `cannot be used multiple times`, so "the last one wins" is false here.
    ⚠ And `muse how do I use exec mode` exits 2 with `unknown argument how`: Muse takes
    exactly one positional. ⭐ All three together are why the wrapper reads the line
    before it adds anything, rather than putting the defaults in front of every call.
57. ⛔ **AN INTERACTIVE NON-LOGIN SHELL NEVER READS `~/.profile`.** The first driver for
    the profile's new mechanisms used `sh -ic` and reported **six of thirteen images as
    failing** over code that had not run: `-ic` reads `~/.bashrc` or `$ENV`. ⚠ **And a
    `PATH` planted in the environment cannot be read back through `/etc/profile`**,
    which every image but arch replaces `PATH` in, so the count read 0 whatever the
    profile did. The driver sources the file inside the shell instead, which tests the
    file rather than the plumbing.
58. ⛔ **PHOTON CARRIES NO `tr`.** A counter written as `tr ':' '\n' | grep -c` answered
    0 for every case on that image - including the cases with the profile switched OFF,
    which is what gave it away, because a real failure could not have made the switch
    stop working too. ⚠ This is the same shape as chimera's padded `wc`, which the last
    driver hit: **the assertion must not need a program the image may not have.**
    Splitting on the colon with parameter expansion needs none.
59. ⭐ **THE MUTATION HARNESS CAUGHT REDUNDANT CODE, not a weak case.** A row that
    deleted `tk_model=; tk_effort=;` from the Muse wrapper's non-agent-subcommand arm
    stayed green, and the case was right: `tk_after=none` alone already stops the
    injection, so the two assignments could never change an outcome. ⚠ The fix was to
    delete the redundancy rather than to strengthen the case. A guard that cannot be
    made to matter is not a guard.

60. ⛔ **`base revoke --help` DESCRIBED ITSELF IN `base grant`'S WORDS.** Both subcommands
    register one flag set, so revoke's help offered `--source: the Windows directory to
    grant` and `--mode: ro or rw` over a command that REFUSES both. ⚠ **It was found by
    writing the guide from the help and then running what the guide said**, which is the
    claim audit working on a page that had not been published yet: the command answered
    `base revoke takes --target alone`. ⭐ The flags stay registered, so that refusal
    still fires rather than becoming an unknown-flag error; only the wording is revoke's
    now. 1 case, 1 mutation row, red when planted.
61. ⭐ **THE GUIDE ITSELF FOUND THE DEFECTS, because every command in it was RUN.** Three
    things it says were wrong when first written: pi's credential type is `api_key` and
    not `api`, a grant is taken away with `base revoke --target` rather than by hand, and
    `printf '\n'` inside a generated block lost its escape. ⚠ **None would have been
    caught by reading.** A guide whose commands have not been run is a list of guesses.
62. ⛔ **`base bootstrap` SILENTLY BYPASSES EVERY AGENT WRAPPER, AND THE PROBE CALLED
    THAT HEALTHY.** `bootstrap.sh` writes `export PATH="$HOME/.local/bin:$PATH"` into the
    account's profile, so after it runs a LOGIN shell finds the vendor's own launcher
    first and `/usr/local/bin/NAME` is never reached. Measured on 2026-09-17 by planting
    that one line: all three names moved from `/usr/local/bin/*` to
    the account's own `~/.local/bin`, and the resolved `muse` was Meta's own bash launcher.
    ⛔ **Muse Code has no settings key for a model or an effort**, so what is lost is
    BOTH, on every session, with nothing said. ⚠ pi and omp keep theirs, which live in
    their own configuration; what they lose is the account guard.
    ⛔ **And the muse probe's existing wrapper check could not see it**: it asked
    `as_account`, whose PATH is curated and carries no `$HOME/.local/bin`. That is
    finding 49's shape a second time, in the guard written to answer finding 49.
    ⭐ All three probes now read the name back on a LOGIN shell and refuse a resolution
    that is not this tool's wrapper. Driven both ways: with the line planted the probe
    exits 1 and names the cause for each of the three; with it gone and `base ensure`
    run, exit 0.
63. ⚠ **`$LASTEXITCODE` AFTER A PIPE IS THE PIPELINE'S, and it said 0 over a probe that
    had just exited 1.** Seen while proving finding 62, in the same session whose own
    rules say to read an exit code from the process. The reading was redone unpiped.

64. ⭐ **`PreToolUse` AND `PermissionRequest` ARE DRIVEN FROM A REAL TURN**, which is
    `WSL-76`'s fifth outstanding item and had been registered and never fired, because the
    echo provider calls no tool. On the operator's base on 2026-09-17, with Muse on
    `muse-spark-1.3-contributor` at `max`:
    `reported working on UserPromptSubmit`, `reported working on PreToolUse`,
    `reported idle on Stop`; and for a command outside the workspace,
    `reported working on PreToolUse` then `reported blocked on PermissionRequest`, with
    the pane showing the approval request and the exact command.
    ⭐ **The log names the event now.** It used to print only the state, so two reports of
    `working` in one turn could not be told apart and `PreToolUse` could not be shown to
    have fired at all.
65. ⛔ **A CANCELLED APPROVAL LEAVES THE AGENT `blocked` FOR EVER, AND herdr THEN REFUSES
    EVERY PROMPT.** Measured 2026-09-17: the approval was denied with `escape`, the pane
    printed `tool approval cancelled`, the file was never written - and Muse fired **no
    hook at all**, so the reporter's last word stayed `blocked` and
    `herdr agent prompt` answered `agent_blocked`. ⚠ **Two more hook events were tried and neither
    fired**; `WSL-76`'s amendment of the same date names them and what they did. ⭐ **The recovery is to submit through the PANE
    rather than through the agent**: `pane send-text` plus `pane send-keys enter` reached
    Muse, herdr's own screen detection moved the agent to `working`, and `agent prompt`
    was accepted again. ⚠ **That recovery came from herdr and not from the hook** - the
    reporter logged nothing new - which is the screen-derived fallback herdr keeps even
    under full lifecycle authority.
66. ⚠ **`agent wait --until idle` timed out over an agent that was ALREADY idle.** The
    turn finished before the wait began. Not a defect, and worth knowing before treating
    a timeout as a failure.

67. ⛔ **`wsl --shutdown` INVALIDATES THE PODMAN BOOT ID IN EVERY BASE, AND ONLY ONE
    WAS REPAIRED.** The previous session ran it twice, met the damage on
    `wsl-toolkit-base`, repaired that one with `base ensure --repair`, and recorded
    exactly that. ⚠ **The default `wsl-toolkit` base was never touched**, and it is the
    one every `run` and `matrix` uses, so the first container check of 2026-09-17T10:07Z
    exited **2** on `current system boot ID differs from cached boot ID` before anything
    else could run. `base ensure --repair` cleared it in one command. ⛔ **The record
    said the base was repaired, and a reader would take that to mean the host was** -
    the host state names five distributions and does not say which of them carry an
    engine. ⭐ **The mechanical half is cheap and is not written**: a `wsl --shutdown`
    is a host-wide event and the tool knows which instances it manages, so `base status`
    could answer for all of them rather than for the one named. Not built, and tracked
    here.

68. ⛔ **`bootstrap.sh` AND `remote.go` READ NO RELEASE OF THIS REPOSITORY AT ALL, and
    `WSL-90`'s door sweep said they did.** That sweep, on 2026-09-16, listed both as
    reading `/releases/latest` and reasoned that "the API defines latest as excluding
    prereleases, so no nightly can reach them". ⚠ **The conclusion is true and the
    reason is not.** `bootstrap.sh` has exactly two release URLs and both are
    `github.com/PowerShell/PowerShell`; the only owner it names anywhere is PowerShell.
    `latestRelease` in `remote.go` takes an `OWNER/NAME` argument and is called to pin
    third-party GitHub Actions, never for `Azathothas/ToolKit`. So a nightly cannot
    reach either of them because **neither looks**, not because of what `latest` means.
    ⛔ **It is a reason that would survive a change that broke it**: if either file ever
    did read this repository's releases, the recorded argument would still read as
    settled. Found on 2026-09-17 by re-running that sweep with
    `git ls-files | xargs grep -l` rather than reading the sentence.

## Review findings

⭐ **2026-09-17, this session's FOUR closing reviews.** Each pass names what it looked at
that the others did not.

**Pass 1, the door sweep** - "what other door reaches an agent's startup model?" It
enumerated every route that starts an agent by name: `base agent`, the Windows launcher
`NAME.exe`, `herdr agent start`, a person typing the name in `base shell`, and
`base exec -c`. ⭐ All five resolve through `/usr/local/bin/NAME`, measured on both
`base exec`'s cleared environment and a login shell's. ⛔ **Then it grepped for the door
it had not enumerated, and found one**: `bootstrap.sh` writes
`export PATH="$HOME/.local/bin:$PATH"` into the account's profile, so after
`base bootstrap` every one of those names resolves to the VENDOR's launcher and the
wrapper is never reached. Muse then starts with neither the model nor the effort, and
`base status --probe` called that base healthy. Finding 62. ⚠ **The guard that should
have caught it was finding 49's shape a second time**: the muse probe did check the
wrapper, through `as_account`, whose PATH is curated and carries no `$HOME/.local/bin`.
All three probes now read the name back on a LOGIN shell and refuse a resolution that is
not this tool's wrapper; driven both ways, exit 1 planted and exit 0 restored.

**Pass 2, the guard mutation** - "can each new guard actually fail?" 11 rows, **11 of 11
red**, and each green unmutated first. ⛔ **Its first real find was redundant code rather
than a weak case**: a row deleting `tk_model=; tk_effort=;` from the Muse wrapper stayed
green, because `tk_after=none` alone already stops the injection. The redundancy was
deleted rather than the case strengthened. Finding 59. ⭐ **And it found a gap no row
could cover**: the shell profile's three new mechanisms were proved only by
`matrix --images all`, which nobody runs on a commit, so a regression would have reached
a release with the gate green. Four cases now drive the SHIPPED copy under `sh`, and
three rows prove them.

**Pass 3, the claim audit** - "which sentence about to be published is not backed by an
artefact?" It read the guide, the two skills, the manual's new sections and
`scripts/README.md` against the tree and the live base. ⛔ **Four sentences failed.**
`base ensure takes about 80 seconds` was carried from a 2026-09-14 measurement of a base
with TWO adapters and was not re-measured, so the number is gone and the manual's
measured table is named instead; pi's credential type is `api_key` and the guide said
`api`; a grant is taken away with `base revoke --target` and the guide said to edit the
file by hand, which came from `base revoke --help` describing itself in `base grant`'s
words, finding 60; and a generated block had lost a backslash. ⭐ **The numbers that
passed were recounted from the artefacts**: 13 transcripts, 28 `shells_seen`, 28
`deduplicated`, 13 `gained-a-home`, and `base doors --json` reporting 30 doors.

**Pass 4, what was measured but never verified** - naturally distinct from pass 3,
because it starts from a number taken on trust rather than from a sentence. ⭐ **Three
results.** `spark-max` and its three siblings are absent from the gateway's `/v1/models`
listing, which looked like last session had invented them; asking for each by name
answers **HTTP 200**, so they are aliases the gateway serves and does not list, and the
entry stands. Finding 55. Each agent's effort vocabulary had been read from its `--help`
and never exercised: driven word by word, pi refuses `ultra` and `auto`, omp refuses
`ultra` and `off`, muse refuses `auto` and takes `ultra`, and all three take `max`.
⛔ **And pi only WARNS on a level it does not know and continues at exit 0**, so an
unvalidated effort would be silently ignored rather than refused - which is why this
tool validates the word before sending it.









⭐ **2026-09-17, `WSL-88`'s and `WSL-89`'s closing reviews**, run over the two adapters
together because they were driven together on one base.

**The door sweep** went after the setting rather than the scripts: every reader of
`separate_agent_dir`, and every place a configuration field can be lost. ⛔ **It found
that the field is inside `BaseAdapter`, which the reflect walk guarding the rest cannot
see.** `TestEveryStoredBaseFieldSurvivesLoading` says so in its own comment - it walks
`BaseConfig` and stops at the adapter - and `base.adapters` itself was once decoded,
validated as empty and dropped. A case now writes the setting out, reads it back through
`LoadConfig` and asserts it survived. ⚠ It also confirmed the flag is refused on every
adapter but omp, which is the rule `installer_sha256` and `channel` already keep.

**The guard mutation** ran two new rows, 2 of 2 red. ⭐ **But the lens's real find came
before any row existed**: the collision refusal this entry was written around **could
never fire**. `account_env` read the account's variables under `env -i`, which clears the
environment on purpose, so every variable read as unset and the refusal was dead code on
every base that has ever existed. ⚠ **A row could not have caught it**: the guard was
reachable in Go, correct in the shell, and answered a question nobody could make true. It
took exporting the variable on a real base, in a login shell, to see the two disagree.
⭐ Three more defects came out of the same driving, each named in `WSL-89`'s amendment:
bun, the unconditional override, and a wrapper guard that read a path where npm's own
shim lives.

**The claim audit** read the two amendments against the runs. ⛔ **It caught a claim in a
CASE rather than a page**: `TestBunComesFromTheDistributionAndNotFromNpm` asserted the
installer does not run `npm install -g bun`, and failed - over the COMMENT that explains
why it must not. A negative assertion that reads comments fails on the sentence stating
the rule; it strips comment lines and reads what runs. ⚠ It also rejected a first draft
of `WSL-88`'s amendment that called the entry's premise "correct": correct is what it
was, and worth stating only because `WSL-89`'s premise, written the same day from the
same sweep, was wrong about exactly the thing that mattered. ⛔ **And it caught six
absolute home paths** carrying an account name in the amendments and two scripts, which
the gate's `secrets` rule then refused; they are the class `examples/common/zellij.md`
published three times once.

⚠ **What a pass with nothing to report would have needed.** None of these three was
quiet. The one that came closest was the door sweep on `pi`, which found nothing to
change: pi is a Node program, `--ignore-scripts` is what its own project documents, and
its premise held in every particular. It would have fired if pi had needed a runtime the
base lacks, which is exactly what omp turned out to need.

⭐ **2026-09-17, `WSL-68`'s reviews for approach step 1**, run over the shared tmpfs and
the private network namespace together.

**The door sweep** grepped for every builder of an `ExecRequest` that runs as the
account rather than trusting the two the task named, and for every reader of the new
setting. ⛔ **It found that `base status` could not report `base.shared_tmpfs` at all**:
`BaseAccessState` carried automount, interop, systemd, sudo and toolset and nothing
else, so a caller could read the configuration of a base and not learn whether it
closes the directory every distribution shares. `base doors` answers about the DOOR and
`base status` about the CONFIGURATION, and a base that should close it and does not is
only visible by comparing them. Added, with a case. ⚠ **And it found a gap that is
left**: `base agent`, `base herdr` and `shipped write` each build their own request as
the account and none takes `--private-net`, so an agent started through `base agent` is
not in a private namespace. That is named in the entry rather than closed, because
closing it is the same design question the operator is asked.

**The guard mutation** ran seven new rows through `repo mutate`, 7 of 7 red, each case
green unmutated first. ⚠ **Two had to be rewritten before they could go red at all**:
deleting a guard left its variable unused and the row reported `BROKEN, does not
compile`, which is neither red nor green and would have been easy to read as proved.
⭐ **And two live plants on a real base, which no row can reach:**

- the unmount taken out of the boot script: `base doors` **exit 1** naming the claim,
  `base status --probe` **exit 1**, `usable false`, naming the setting. ⚠ **The FIRST
  plant was defeated by its own shell** - `false && umount ... || umount -l ...` still
  runs the fallback, the door stayed closed, and the row would have read as a guard
  that does nothing. Printing what the script actually contained is what caught it;
- the ruleset flushed inside the namespace: the Windows host went from `closed` back to
  **open** while the internet stayed open. ⭐ So it is the RULE that refuses the host,
  not pasta and not the namespace, which is the thing the entry's premise rests on and
  the one a reading could not have settled.

**The claim audit** read every number in the two amendments, the manual and this record
against the run that produced it. ⛔ **It caught a conflation**: "four open doors, down
from six" put a figure from `wsl-toolkit-base` under a table headed by the throwaway
base, and those two differ by passwordless sudo and by the tmpfs as well as by this
setting. ⛔ **The honest reading is worse than the correction**: the throwaway base with
its tmpfs OPEN and the CORRECTED probe was never measured at all, because the probe was
corrected first, so no before-and-after for that door on that base exists. Both pages
say so now. ⚠ It also confirmed what the entry may claim: the ruling's condition, "no
rule in the shared namespace", is backed by three readings - the namespace id differs,
the shared id is unchanged before and after, and the shared ruleset is not readable to
the account at all.

⭐ **The fourth lens, "what did the driven pass show that the suite could not":** all
four defects in the shared-tmpfs work and the whole engine conflict came from running
it. The suite could not have found any of them - a `command=` wsl.conf silently ignores,
a resolver that reads empty that early, a door that cannot say `closed`, and podman
taking itself for rootful inside a user namespace are all facts about WSL and podman
rather than about this code.

⭐ **2026-09-17, `WSL-68`'s three closing reviews for steps 3 and 4.**


**The door sweep** did not read either list of `base` subcommands; it grepped for every
one named anywhere, in `cmd_base.go`, `main.go` and `helper.go`. ⛔ **That found finding
39**: the usage text and the manual registry disagree in both directions, so `base herdr`
and `base bootstrap` are in no manual and `base agent` is in no usage text. It also
enumerated the reaches into the new code rather than trusting the task list - the
dispatch, the `--json` surface, the acceptance sweep, the man page registry, the helper
route, and `shipped`, which does not carry `doors.sh` and should not. ⭐ **Two refusals
were driven, not read**: an unregistered instance exits 2 naming `base ensure`, and it
created no state directory, which is `NewBase`'s rule holding. ⚠ **The helper refusal was
NOT driven**: no helper is running on this host, so `--via-helper` exits 2 at
`no helper has been started` before the refusal in `cmd_base_doors.go` is reached. It is
read, not measured, and saying so is the point.

**The guard mutation** ran the six new rows through `repo mutate`, 6 of 6 red, each case
green unmutated first. ⭐ **And then the one the rows cannot reach**: the contract between
`doors.sh` and its reader is two files, so the defect was planted in the SHELL half - the
row separator changed from `|` to `\t`, the executable rebuilt, and the command driven
against the live base. Unmutated first: exit 0, 30 doors. Mutated: **exit 2, `the probe
says it reported 30 doors and 0 were understood here`**, which is finding 2's whole
defect class refused out loud instead of an empty table and a zero. Restored and verified
byte-identical, exit 0 again. ⛔ **`TestEveryRequiredDoorIsEmittedByTheEmbeddedProbe`
stayed GREEN over that mutation**, correctly - it asserts the emitting calls exist, not
the format - so the Go suite alone would not have caught it and the live count comparison
is what did.

**The claim audit** read every number in the amendment, the manual and this record
against the artefact that produced it, and **three were invented**: "12 cases" where
`grep -c '^func Test'` reads **13**; "a reader with four guards" where nothing counted
four; and "six readings" of the interop handler where the session took **seventeen**
across two distributions and three utility-VM lifetimes. All three are corrected, and
the interop claim now carries the table rather than a total. ⚠ It also caught
`wsl-toolkit.md` placing the new section under **"Reaching herdr, and through it the
agents"**, where `base doors` is not a herdr topic; it is a top-level section before the
safety model now. ⛔ **And it caught the record contradicting itself twice**: a
`PROGRESS.md` line still calling the `--remote` probe "a draft outside the tree" three
paragraphs above the one saying it is tracked, and `WSL-68`'s own 2026-09-15 "Still open"
list still naming items 3 and 4 as open above the amendment that closes them. Both are
finding 29's shape inside one file, both corrected in place.

⚠ **What a fourth lens would have had to be.** The driven pass is not listed separately
here because it is not separable: this whole unit was built by driving it, and the three
defects in the probe and the one in WSL all came from runs rather than from reading. The
lens with nothing to report is the suite's own - no case in `doors_test.go` failed at any
point after it was written, which means every one of them was written against a defect
already understood, and the things that were NOT understood were found by the command
running against a real base.

Each closed entry carries its three reviews. `WSL-85`'s door sweep found 23 more
error messages named by a guest's first stderr line, recorded above; its claim audit
corrected the manual's first draft, which said a disagreeing base "answers exit 1" and
named no command. `WSL-67`'s checkpoint: the claim audit found the Nix version probe
outside `nix_run`, where the documentation said every Nix command gets the settings, and
the probe now goes through it. ⭐ **2026-09-15:** `WSL-87`'s guard mutation found a
defect in its own method - a hand-planted defect in a file outside the Go module read as a
green pass, because `go test` served a cached result and only `-count=1` made the guard
fire; `repo mutate` already passes it. Its claim audit found `presets.go` publishing a
build figure for a preset that had not built since, now corrected with both dates and both
sets of conditions. `WSL-86`'s driven pass found the automount sweep race, and its door
sweep found that a verification failure is named by the engine's first line. ⛔ **CI
then failed one of its own mutation rows as THEATRE**, because the case proving it can only
be staged where no `getcap` exists and `ubuntu-latest` carries one: the row is now its own
case, which skips there and goes red in `golang:1.25`. `WSL-67` still owes its closing
reviews.

⭐ **2026-09-15, `WSL-71`.** Its door sweep found that a `base.mounts` grant is a DrvFS
mount exactly like an automounted drive, which is why the profile's guard is a path test
and not a filesystem-type test: the type test would have moved every shell out of the one
directory the caller was granted. It also found the `PATH` line that never reached a bash
login shell, fixed in the same change. ⛔ **Its guard mutation found a defect in its own
method for the second session running.** The first planting script used `awk`'s `sub()`,
whose first argument is an ERE, against anchors full of `*`, `$`, `{`, `?` and `|`; every
substitution silently failed, and all five rows read identically to the unmutated one -
which looks exactly like five guards that do nothing. Printing the unmutated row FIRST is
what made it legible, and `write-file.mjs replace --expect 1` cannot fail that way. ⛔ Its
claim audit rejected the entry's own first passing condition: "writes nothing to stderr"
is unmeetable on photon, whose `/etc/profile.d` writes 66 bytes to every login shell with
or without the file, so the assertion is now the DELTA between a run without the profile
and one with it. It also put the conditions back on a byte count that had been quoted as
though it were a constant.

⭐ **2026-09-15, `WSL-68`'s driven pass**, which is a fourth lens and started from the
attacker rather than from the code. It found the shared `/mnt/wsl` the entry's own door
list did not contain, and it found two of its own readings to be wrong before they were
written down - both because a check read a PIPELINE's status instead of the process's,
which is the absolute this repository states first and the one a scratch probe keeps
breaking. Its door sweep found finding 23, that this tree now holds two disagreeing
definitions of "a Windows drive under /mnt". `WSL-68` is a checkpoint and owes its three
closing reviews.

⭐ **2026-09-16, `WSL-90`'s checkpoint reviews**, owed by the session that was
checkpointed on 2026-09-15 and run by the one that resumed it.

**The door sweep** enumerated every release lookup in the tracked tree rather than the
two the entry named, with `git ls-files | xargs grep -l releases`: `LatestRelease` in
`release.go`, the nightly's in `herdr_nightly.go`, `Resolve-LatestTag` and `Get-Release`
in `consumer.ps1`, `remote.go`, `bootstrap.sh`, `release.yml` and `herdr-nightly.yml`.
⭐ Two of those are not exposed and the entry never said why: `bootstrap.sh` and
`remote.go` both read `/releases/latest`, which the API defines as excluding
prereleases, so no nightly can reach them. The nightly's own lookup is the mirror image
of `LatestRelease` - same hundred-a-page, same ten-page bound, and
`^herdr-nightly-[0-9]{8}-[0-9a-f]{12}$`, which is exactly the tag
`herdr-nightly.yml` builds - so the symmetric hazard, this tool's releases hiding the
nightlies, is closed too. ⛔ **It found one the entry's Consumers section missed:
`consumer.ps1` downloads EVERY asset a release carries.** Its `gh` path is `gh release
download` with no `--pattern`, and its fallback walks `SHA256SUMS` and fetches each
name plus a bundle. `release.yml` stages herdr's four builds into that same
`SHA256SUMS`, so from `wsl-toolkit-v3.0.0` this repository's own reference runner will
fetch four herdr binaries it never uses, where `consumers.md` tells a consumer to
download only "the executable matching the host architecture and `SHA256SUMS`". The
entry called the herdr files "additive", which is true of the release and not of the
fetch. Filed as finding 31.

**The guard mutation** took the guards in `herdr-nightly.yml`, which had never been
driven at all, and planted the defect each exists to catch, unmutated row printed
first: the development head's shape refused an empty string, 39 characters, uppercase,
a shell payload and an API error string; the prune refused this tool's own release,
an upstream tag, and a release standing behind a nightly; the staging refused a missing
target and a `SHA256SUMS` one line short. **14 rows, 14 as wanted, 0 red.** ⚠ **It
proves the logic and not the deployed step**: the harness transcribes each guard's
control flow into a function returning 1 where the workflow exits 1, so it is a reading
of the workflow rather than the workflow. The dispatched nightly is what drives the
steps themselves, and only their passing half.

**The claim audit** read the five corrected pages against `gh run list` and `gh release
list` rather than against the checkpoint's description of them, and the corrections
hold: `herdr-nightly` appears in no run listing and no `herdr-nightly-*` tag exists.
⛔ **The enumeration "five live pages" was right by luck.** A grep for the claim found
**six** files; the sixth, `CHANGELOG.md:28`, is a dated `**Deployed:**` line that the
changelog's own rule 4 requires and its "do not delete an entry" rule protects, so it
is history and correctly untouched - but nothing in the checkpoint showed it had been
looked at. ⛔ **The README's correction carried no date** where the other four carry
`read on 2026-09-15`, which is a claim quoted without its conditions; it now names the
date and says `herdr-build.yml` publishes nothing by design. ⛔ **And four of the five
corrections are false by the end of this session**, because it publishes the first
nightly: they are committed as the previous session's record, dated, and rewritten in
the nightly's own commit rather than left to rot.

## Open questions for the operator

⭐ The operator asked on 2026-09-15 to be asked only for what actually needs them: a
ruling this repository's own rules require, a download, a credential, or Windows
software.

1. ⭐ **ANSWERED on 2026-09-17.** `muse login` was done in the operator's own base,
   Muse drove a real turn through herdr, and findings 47 to 51 and 64 to 66 are what
   that sign-in made measurable. ⛔ **Do not ask for it again.**
2. **The six `--remote` signals in a real Windows Terminal window.** A pseudo console
   measured every input signal; only a real window carries a real focus event, so the
   probe reports signal 7 as `operator` rather than pretending. ⚠ **The client to pass
   is the one `base attach` prints**, which changes with each nightly; asked again with
   exact commands on 2026-09-17T10:05Z. It is `WSL-76`'s only remaining item and
   `WSL-90`'s second.

3. ⭐ **New on 2026-09-17, and it is a ruling rather than work.** `WSL-68`'s amendment
   of that date measured that a base cannot both put the account's processes in their
   own network namespace and run containers as that account: `pasta --config-net` puts
   them in a user namespace where podman takes itself for rootful and cannot write the
   paths it then chooses, measured three ways. The 2026-09-14 ruling made the private
   network a property of a sealed base. What is delivered is
   `base exec --private-net`, a flag, which leaves the engine working everywhere else.
   **Whether a base should instead be able to declare the namespace for ALL of the
   account's processes, and give up running containers as that account, is the
   operator's to decide.** Nothing waits on the answer; the flag is shipped either way.

4. ⭐ **New on 2026-09-17T10:05Z: a from-nothing transcript of the muse-code guide, in
   its own order.** `WSL-78`'s prove asks for one and every command in the guide has
   been run, ⛔ **but never as one ordered pass from an empty host**, because step 5 is
   three sign-ins and each is the operator's credential. ⚠ **The alternative was offered
   in the same message**: say so and the entry closes with the condition recorded as
   met-except-for-the-ordered-pass, naming which commands were run separately and when.
   Either answer closes it; silence is what leaves it open.
5. ⭐ **New on 2026-09-17T10:05Z, and it is a ruling: does `muse serve`'s stdio protocol
   get a route through this tool?** `WSL-78` item 3, open and undecided since
   2026-09-15. ⚠ **The recommendation sent was no** - herdr already reaches every agent,
   and a stdio bridge is a second protocol surface with one caller and no consumer in
   the register. A yes means an entry, authored before anything is built.

Answered on 2026-09-15 and recorded as rulings 13 to 15: whether herdr builds are
published here, the targets, how the adapter takes a nightly, and where the
development server ran for `--machine`.

## Host state

- Registered distributions: `podman-machine-default`, `eph-pgb`, `wsl-toolkit`,
  `wsl-toolkit-podbox`, and ⭐ **`wsl-toolkit-base`, the operator's one base for every
  agent**, built on 2026-09-15 and kept. `eph-pgb` keeps its disk under
  `%LOCALAPPDATA%\wsl-ephemeral` and is not this tool's.
- ⭐ **`instances\base\config.json`** is `examples/muse-code/wsl-toolkit-base.json` plus
  one test grant, `.tmp\wsl78\proj` read-write at `/workspaces/proj`, a throwaway git
  project of three files.
- ⭐ **This machine's half of the herdr adapter is written**, as ruling 5 allows: the
  marked `Host wsl-toolkit-base` block at the top of `%USERPROFILE%\.ssh\config`, which
  now reads SHA-256 `72693CCF…E626AF`, one line in the dedicated `known_hosts`, and
  `%USERPROFILE%\bin\muse.exe`.
- ⚠ **`herdr machine add` saved a profile** `base`, id `e70f5d617fc95616…`,
  in `%LOCALAPPDATA%\herdr\client\endpoints.json`, a directory it created, and created
  the workspace `w2` in the base's herdr server.
- ⚠ **Rust 1.96.1 is installed through rustup for herdr's build, and rustup updated
  itself from 1.29.0 to 1.29.1** while doing it, which is its default and was not asked
  for. `.tmp\herdr` holds herdr's source at `052779c4159ed851` and its Windows build
  tree; `.tmp\herdr-master-052779c4` and `.tmp\herdr-linux-052779c4` hold the two built
  binaries.
- ⭐ **`wsl-toolkit-base` runs the published nightly's server**, since 2026-09-16.
  `instances\base\config.json` now carries `{"name": "herdr", "channel": "nightly"}`, and
  `base ensure` installed `herdr-nightly-20260916-18061191fdc0`, SHA-256
  `213580fc…f92f14a1`, which is that release's own `SHA256SUMS` line for
  `herdr-linux-x86_64`. It replaced the development server ruling 15 swapped in by hand,
  `978fde51…9827d75`. The configuration as it was before is kept at
  `.tmp\base-config-before-nightly.json`. ⚠ The base's own `--version` still answers
  `herdr 0.9.0`, finding 28, so the digest above is its only identity.
- ⭐ **The nightly's Windows client is under the instance's state directory**,
  `instances\base\herdr\herdr-nightly-20260916-18061191fdc0\herdr.exe`, written by
  `base ensure` and printed by `base attach`. ⚠ **It, not `.tmp\herdr-master-052779c4`,
  is the client that matches the server the base now runs**, and it is what the
  operator's `--remote` test should use.
- ⚠ **A verified copy of the nightly is under `.tmp\nightly-verify`**, 12 files and about
  68 MiB, kept as the evidence behind this session's verification and safe to remove by
  literal path.
- ⚠ **Running herdr's Windows clients in a pseudo console created
  `%APPDATA%\herdr`**, which did not exist: a 19-byte `config.toml` reading
  `onboarding = false`, written by the development client, and `herdr-client.log`. Kept
  so the operator's own `--remote` test is not met by the onboarding overlay that
  swallowed the probe's keys.
- ⚠ **`%LOCALAPPDATA%\herdr\remote` holds three `ssh-PID-0` directories and three
  `.sock` files**, left by the three pseudo-console runs that were killed, at 11:19,
  11:20 and 11:21Z; the two runs that detached with exit 0 left none.
- ⚠ **The shared `/mnt/wsl` was written to and cleaned up**, once per `base doors` run:
  each run writes `/mnt/wsl/.doors-<12 hex>` and removes it, and the row reports whether
  its own file went away. Every run this session reported `own file removed`, and the
  directory holds `resolv.conf` alone.
- ⚠ **`wsl --shutdown` was run twice on 2026-09-17** and the base was repaired with
  `base ensure --repair` afterwards. `eph-pgb` was Stopped throughout and was not
  touched; no distribution was added or removed.
- ⚠ **A scratch serial-console driver for the NetBSD and OpenBSD guests is at
  `.tmp\bsd67-driver`**, untracked: it boots through an overlay, answers the boot loader and
  the OpenBSD installer over SeaBIOS's serial console, and runs commands from a spool with
  files served over QEMU's own TFTP. It has never booted a guest; read it before using it.
- `instances` under `%LOCALAPPDATA%\wsl-toolkit` holds `acc`, `muse`, `nobase`,
  `podbox` and `podbox-migrate`, as at the start. `instances\muse` holds nothing.
- No BSD image is downloaded. The shared FreeBSD image is the published one,
  6,476,638,208 bytes, SHA-256 `12807CE7…921663BF`, not booted this session.
- herdr 0.9.0 is installed on Windows by the operator, through scoop, and is the `herdr`
  on `PATH`; the `wsl-toolkit` on `PATH` is `%USERPROFILE%\bin\wsl-toolkit.exe`.
- The dedicated SSH key the ruling allows is at
  `%USERPROFILE%\.ssh\wsl-toolkit\id_ed25519`, with the one-line `known_hosts` above
  beside it, and stays for `wsl-toolkit-base`.
