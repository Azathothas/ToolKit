# RULES.md

The part of the record that does not change between sessions.
[`PROGRESS.md`](PROGRESS.md) is what changed since last time and carries the
work order; [`INDEX.md`](INDEX.md) is the list of entries; this is the standing
state and the rules that are this repository's own.

⛔ **A rule is here only if it cost something here.** Everything general lives
in [`../docs/conventions/`](../docs/conventions/) and
[`../docs/methodology/`](../docs/methodology/), and this file links rather than
repeats, so the two cannot fork.

---

## The standing facts

⚠ **Read from the machine or the API, never typed from memory.** Each row names
where it is checked.

| fact | value | where it is read from |
| --- | --- | --- |
| repository | `Azathothas/ToolKit`, public, 0BSD | `gh api repos/Azathothas/ToolKit` |
| what it publishes | the `wsl-toolkit` tool, as a GitHub release on a `wsl-toolkit-v*` tag: from `wsl-toolkit-v3.0.0`, the executable for two Windows architectures, herdr's newest stable release built for four targets, `SHA256SUMS`, and one `.cosign.bundle` per published file. From `wsl-toolkit-v3.1.0`, `text-tool` for four targets as well, so 22 files. ⚠ Every release before `wsl-toolkit-v3.0.0` also carries `wsl-toolkit.ps1` and `launcher.ps1`. And herdr's development branch, as a nightly prerelease on a `herdr-nightly-*` tag, the newest seven kept, by the operator's ruling of 2026-09-15. Nothing else | `gh release view TAG --repo Azathothas/ToolKit --json assets` |
| work model | todo | [`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md) |
| push policy | commit and push, to this remote only, on `main` | [`../docs/conventions/git.md`](../docs/conventions/git.md) section 2 |
| `main` | protected. One approving review, three required status checks, linear history. Force push and deletion refused. Admin bypass is on. | `gh api repos/Azathothas/ToolKit/branches/main/protection` |
| CI | `ci.yml` on every push: five jobs, and two are matrices, the `go` job on two hosts and the mutation job in three shards, so eight runs. `release.yml` on a `wsl-toolkit-v*` tag, and it calls `release-smoke.yml`, which also runs weekly. `remote-items.yml` weekly. | [`../.github/workflows/`](../.github/workflows/), and `gh run view RUN --json jobs` |
| ⚠ what `main` actually REQUIRES | three of those eight: `checks (ubuntu)`, `powershell (windows)`, `the two probes agree`. The `go` matrix and the mutation job are not required, so a red one does not block a merge. | `gh api repos/Azathothas/ToolKit/branches/main/protection` |
| the methodology | `docs/methodology/` was adopted from `Azathothas/TEMPLATE` at `3191d08` and synced with its `a970fe5` on 2026-10-21, ruling 31. A page this repository edited was merged, never overwritten | [`../docs/methodology/template-sync.md`](../docs/methodology/template-sync.md) |
| the record before 2026-10-21 | `PROGRESS.md` as it stood at `9784821`, word for word, with the numbered findings 1 to 116 and rulings 1 to 30 that code, cases and entries cite as `finding N` and `ruling N` | [`../docs/HISTORY/record.md`](../docs/HISTORY/record.md) |
| the local gate | `sh scripts/common/check-gate.sh`, or its `.ps1` twin | [`../scripts/README.md`](../scripts/README.md) |
| the identity a commit carries | the machine's `git config`, per invocation | [`../docs/conventions/git.md`](../docs/conventions/git.md) section 1 |

⛔ **The gate's measured cost belongs in [`PROGRESS.md`](PROGRESS.md), not
here.** It is re-timed on the machine that ran it and it moves, which is the
definition of something that does not belong in this file.

---

## 1. A tool here has callers this tree cannot see

⭐ **This is the rule that makes this repository different from an ordinary
project, and it is the one most likely to be got wrong.** A file here is fetched
by URL by other repositories and by the operator's own scripts. Nothing in this
tree fails when their contract is broken; only they do, later, on a machine
nobody is watching.

[`../docs/consumers.md`](../docs/consumers.md) is the register, the definition
of a break, and what a breaking change owes.

**What it cost.** A copy of this tool, under its old name `wsl-ephemeral.ps1`,
was found in `pkgforge-dev/cross-libc-dlopen` on 2026-08-27, during a reading of
that repository for an unrelated reason. It was not in the register, it fetches
nothing, and it carries both P0 defects this repository has since closed. The
register cannot reach it and neither can a fix.

⚠ **So the register is a lower bound on who is affected, never the complete
set**, and an entry that changes a fetched file says which consumers were
checked rather than assuming the answer.

## 2. The rules are ONE program, and it is not shell

⛔ **Every rule this repository enforces over its own tree lives in
[`../tools/check/`](../tools/check/).** One binary, one tree walk, and it runs
natively on either host. `scripts/common/check-*.sh` and their `.ps1` twins are
wrappers around one named check of it.

**What it cost to learn.** Each rule used to be written twice, in sh and in
PowerShell, because the default host here is Windows and a POSIX check cannot
be assumed to run on it. That is a real hazard: a native PowerShell session
resolves `sort` to `Sort-Object`, which accepts `-u`, compares
case-insensitively, and returned two of four distinct values without erroring.
A missing tool announces itself; an aliased one answers differently and reports
success.

⚠ **But keeping two implementations in step needed a third check that ran both
halves of every pair**, and that check was most of a gate taking about twelve
minutes on this machine. A gate that long is a gate a session skips, and
`--fast` existed to skip exactly it. One implementation has no halves to
compare and nothing to skip.

⛔ **ONE PAIR IS GENUINELY TWO IMPLEMENTATIONS, and it is the doctor probe.**
It RUNS BEFORE YOU KNOW WHAT IS INSTALLED, which is its whole job, so it cannot
require a POSIX layer and cannot require a Go toolchain either: "is there a Go
toolchain" is one of the questions it answers. Everything else that was a pair
is a wrapper now, over [`../tools/check/`](../tools/check/) for the rules and
[`../tools/repo/`](../tools/repo/) for the tools that are not rules.

⛔ **`check-twins.sh` still exists and still matters.** It covers the earned
pair, and it covers the wrapper pairs for a narrower reason: a wrapper that
stops forwarding is drift a schema comparison cannot see, and that has happened
once already. It is no longer in the gate;
[`../scripts/README.md`](../scripts/README.md) says why and where it runs
instead.

## 3. A destructive tool has one deletion, and it reads the state back

⛔ Applies to anything here that removes something on a machine. The compiled
`wsl-toolkit` has one deletion, `RemoveInside` in
[`../tools/windows/wsl-toolkit/internal/toolkit/paths.go`](../tools/windows/wsl-toolkit/internal/toolkit/paths.go).

**What it cost.** `WSL-04`. The predecessor printed that it had deleted a disk
beside a `Remove-Item -ErrorAction SilentlyContinue`, so multi-gigabyte VHDX
files left behind read as disks that had gone.

⚠ **A guard applied at four call sites is a guard that will one day be applied
at three.** The containment check runs inside the deletion helper rather than
beside each caller, and every removal of state reaches that helper.

⚠ **THE LINE IS AROUND STATE, NOT AROUND THE WORD "REMOVE", and it is drawn
where it is for a reason.** The rollback half of a write - `os.Rename` fails and
the same function removes the exact temporary it created two lines above - has
no caller-supplied path to contain, and the only root it could pass is the
file's own directory, which makes the containment check vacuous. A guard that
cannot refuse anything is theatre. `RemoveInside`'s own comment carries the
same sentence, which is where a reader of the code will look.

## 4. FOUR things here are GENERATED, and the tree holds both halves

| generated file | source and regeneration | drift proof |
| --- | --- | --- |
| `tools/windows/wsl-toolkit/internal/toolkit/packages.sh` | the block between the two `shared package table` markers in [`../scripts/common/bootstrap.sh`](../scripts/common/bootstrap.sh); `sh scripts/common/check.sh package-table --fix` | the gate's `package-table` rule regenerates and compares it byte for byte |
| every file under `tools/windows/wsl-toolkit/internal/toolkit/adapters/` | the adapter definitions under [`../tools/windows/wsl-toolkit/adapters/`](../tools/windows/wsl-toolkit/adapters/README.md), the README beside them excepted; `sh scripts/common/check.sh adapters --fix` | the gate's `adapters` rule compares every file byte for byte, and refuses one missing or one with no definition |
| every file under `tools/windows/wsl-toolkit/internal/toolkit/shipped/` | the general-purpose files the executable CARRIES, from [`../scripts/common/`](../scripts/README.md): `bootstrap.sh`, `shell-profile.sh` and `tmux.conf`; `sh scripts/common/check.sh shipped --fix` | the gate's `shipped` rule compares each byte for byte, refuses one missing, and refuses one carried that this repository does not ship |
| `tools/windows/wsl-toolkit/wsl-toolkit.1` | the native command registry and flag bindings; from `tools/windows/wsl-toolkit`, `go run . man --output wsl-toolkit.1` | `TestGeneratedManPageIsCurrent` regenerates and compares it byte for byte |

⚠ **`packages.sh` is read by the base provisioner**, which the executable sends ahead
of `provision.sh`, so an edit to the table's block reaches the published `wsl-toolkit`
as well as every caller of `bootstrap.sh`, in one commit. `WSL-70`.

## 5. The record moves in the same change as the work

Specified in
[`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md), which
also carries the incident that produced it and the arithmetic hazard behind it.
The mechanics here:

```bash
node scripts/common/set-record.mjs status WSL-06 done
```

```bash
sh scripts/common/check-record.sh
```

⛔ **The writer does not grade itself.** `set-record.mjs` moves the numbers and
prints the reader's command; `check-record.sh` asserts independently and runs as
a gate.

## 6. An entry closes on a command, not on a paragraph

An entry is authored from [`ENTRY.md`](ENTRY.md) and closes in place, with the
acceptance command actually run and its real output pasted underneath.

⛔ **Nothing closes as "won't fix" or "somebody else's repository".** A blocked
entry stays open with the blocker named. `BSD-01` is the worked example: the
work left for `pkgforge-dev/docker-bsd` and [`bsd.md`](bsd.md) says where each
part went rather than pretending it finished here.

## 7. What a session owes at its end

Specified in
[`../docs/methodology/sessions.md`](../docs/methodology/sessions.md). What is
this repository's own:

- `RESUME.md` written beside the record at the START, and rewritten whenever
  what is in flight changes. ⚠ It is in `.gitignore`: it exists so a session
  that dies hands something over on this machine, and it is no history;
- [`PROGRESS.md`](PROGRESS.md) rewritten, carrying the state line the record
  check parses, and only the sections the model gives it;
- [`SUMMARY.md`](SUMMARY.md) overwritten with this session's table;
- the gate run, all three parts, and the entry closed with its evidence.

⚠ **`SUMMARY.md` is a snapshot and `PROGRESS.md` is the authority.** A session
that reads the first and acts on it is reading what was true last time.

⛔ **The record check holds the shape, because asking did not.** From
2026-09-15 to 2026-10-21 each session appended to `PROGRESS.md` instead of
rewriting it, until it was 2,776 lines, and `SUMMARY.md` held four sessions.
Rules 8 and 9 of `tools/check/internal/checks/record.go` now refuse a section
the model does not have, a record over 200 lines, and a second session in the
summary. `TOOL-29`.

## 8. The operator's rulings that still govern

Moved here from `PROGRESS.md` on 2026-10-21, word for word and under the same
numbers, so a `ruling N` cited anywhere still resolves. A ruling that has been
carried out is in [`../docs/HISTORY/record.md`](../docs/HISTORY/record.md)
instead. ⛔ **A new ruling is added here, numbered on from the last.**

**1.** **2026-09-13: herdr replaces Zellij** as the agents' multiplexer, and tmux
stays a generic fallback. `WSL-76` quotes the operator.

**2.** **2026-09-14: one base for every agent.** `wsl-toolkit-base`, the instance
`base`, configured under the operator's account, with the Linux account
`herdr`, passwordless sudo and no standing grant. Test grants name directories
under this repository's `.tmp`. In `WSL-75`.

**3.** **2026-09-14, the entries' decisions:** `WSL-75` live grants of a project or of
a parent directory; `WSL-76` OpenSSH through `wsl.exe` with nothing listening
and only the herdr client's key accepted, and herdr's background checks left
on; `WSL-77` adapters in the tree with a generated embedded copy; `WSL-78`
agents run in the base and Windows gets launchers; `WSL-68` the internet only
with the host refused, and only with no rule in the shared network namespace;
`WSL-67` drive `pkgin` and `pkg_add`, and remove a manager that cannot be driven
here.

**4.** **2026-09-14: one Muse installer digest is approved**, `5196d820…632a0ca`, in
`WSL-77`.

**5.** **2026-09-14: host changes the agent may make:** a dedicated SSH key and one
marked `Host` block in `%USERPROFILE%\.ssh\config`, and agent launchers in
`%USERPROFILE%\bin`.

**6.** **2026-09-14: cut `wsl-toolkit-v3.0.0` only after the order above is complete.**
Issues 30 and 32 are closed first. Run `repo release` read-only, and publish only
with CI green on the final commit.

**8.** **2026-09-14: remove Soar and keep Nix**, and Nix is loaded, set up and never
installed: an installed Nix is found without sourcing a profile, flakes are set up
and used for a new profile, the four `NIXPKGS_ALLOW_*` variables are always on
with more defaults where they help, and a GitHub token reaches Nix through
`NIX_CONFIG` only. All of it lives in this repository. In `WSL-67`.

**11.** **2026-09-15: `WSL-86` approved** as recommended, "yes i accept your
recommendation": `nftables` in the provisioner's `apt` arm, and the id-mapping
check reads the capability rather than the path and restores a dropped one. The
operator asked in the same message to be asked only for what actually needs them,
which is why the drive-sweep race found while proving it is recorded under "Found,
and not filed" rather than proposed as an entry.

**13.** **2026-09-15: herdr builds are published from ToolKit, as prereleases**, which
amends `docs/AGENTS.md` section 1's one published thing to two; for Windows
`x86_64` and `aarch64` and Linux `x86_64` and `aarch64`; and the newest stable herdr
is published inside each `wsl-toolkit` release. In `WSL-90`.

**14.** **2026-09-15: the herdr adapter may follow the newest nightly**, over the
recommended pin by version and digest. In `WSL-90`.

**17.** ⛔ **2026-09-17: the omp adapter REFUSES a colliding configuration by default AND
offers an explicit opt-in that separates the directories.** ⚠ **This is not the
recommendation the entry carried**, which was refuse alone; the operator chose the
third option. So the separation is new work with its own cases and mutation rows,
and the refusal stays the default. In `WSL-89`.

**20.** ⭐ **2026-09-17: an adapter prefers the provider's OFFICIAL installer and falls back
to this repository's own route.** The operator: "we should prefer official installers
and then fallback to ours". ⚠ **Not implemented, and it is not a small change.** This
repository never pipes an installer into a shell; the route it permits is the one
ruling 4 set for Muse - download the installer, read it, run it from the file that
was read, against an approved digest. Applying that to `pi` and `omp` means a digest
for each, which is an operator approval per installer. ⛔ Recorded here, filed as the
next work on the adapters, and NOT silently half-done: today `pi` and `omp` install
from npm, which is this repository's own route.

**21.** ⛔ **2026-09-17: bun is NOT in `bootstrap.sh`'s table or the developer toolset**,
against the expectation that it was. `bootstrap.sh` has seven matches for "bun" and
every one is `ca-bundle`; a word-boundary search finds none, and `packages.sh` has
none. The omp adapter installs it from the distribution itself, and does not add it
to the shared table, which is fetched by URL and is not where a runtime one adapter
needs belongs - the rule `WSL-88` states for pi.

**22.** ⭐ **2026-09-17: `wsl-toolkit-v3.0.0` is cut, and this AMENDS ruling 6.** The operator:
"cut the release finally". ⛔ **Ruling 6 required issues 30 and 32 to be CLOSED first,
and issue 30 cannot be**: its part 2 is `WSL-68`, which the same operator deferred in
ruling 19. So the release no longer waits on issue 30. ⚠ **The second half of ruling 6
still holds and is not amended**: `repo release` runs read-only first, and the publish
happens only with the gate green and CI green on the final commit. ⭐ **Issue 30 gets a
comment saying what is left rather than being closed**, and issue 32 closes only if
every entry mapped to it is done.

**24.** ⭐ **2026-09-17: `muse serve`'s stdio protocol gets NO route through this tool**,
cleared with ruling 23 and taking the recommendation. The reasoning is in
`WSL-78`'s closing and is not repeated here.

**25.** ⭐ **2026-09-17: `wsl-toolkit-v3.1.0` is cut, and it carries `text-tool`.** The
operator: "cut the release finally", in the same instruction that asked for
`text-tool` to be published alongside the rest. ⚠ **Ruling 22's second half still
governs how**: `repo release` runs read-only first, and the publish happens only
with the gate green and CI green on the final commit. ⭐ **Ruling 6's first half is
satisfied outright this time rather than amended**: issues 30 and 32 are both
CLOSED, each with a comment naming what its entries did not deliver, so the
condition ruling 22 had to work around no longer applies. ⛔ **A minor version and
not a patch**, because the release publishes four assets no previous release
carried, and a consumer's asset list changes.

**27.** ⭐ **2026-09-17: `wsl-toolkit-v4.0.0` is a MAJOR, and the rule decided it
rather than a preference.** `docs/consumers.md` calls a changed exit meaning
a break, and `text-tool edit --between` now exits 2 where it exited 0. ⚠ **The
break is in `text-tool` and the version is `wsl-toolkit`s**, because one tag
publishes both; a consumer pinning `wsl-toolkit` by major is what the bump
protects. ⛔ **Ruling 22 second half still governs how**: `repo release` runs
read-only first, and the publish happens only with the gate green and CI
green on the final commit.

**28.** **2026-09-27: issue 34's four forks, each the recommended option.** The
operator answered a structured question before any code was written.
- **Detach:** `run --detach` through a detached copy of the tool, and
  `base exec --detach` through the guest. Both use one id with
  `logs --follow`, `wait`, `stop` and `inspect`. `WSL-94`, `WSL-95`.
- **Devices:** `run --device` takes any node under `/dev` that the base
  account can read and write. `WSL-96`.
- **Documents:** a dated measurement moves to the entry that produced it, and
  a live page cites the entry. `DOC-08`.
- **Version:** `wsl-toolkit-v5.0.0`, because `WSL-100` refuses flags that
  used to parse. `base ensure --probe` stays accepted.

**31.** ⭐ **2026-10-21: the record is rewritten to the model, the methodology is
synced, and the WSL work is finished.** The operator: "why & how did
PROGRESS.md get this large, it violates [work-todo.md] / clean this up and
follow TODO properly from now on / and have the skill files been
updated/cleaned / ask me any open decisions/questions and finish all WSL
related work". Four forks, answered before any change:
- **The record:** rewritten to the model, with every finding still open
  filed as an open entry and the old text moved word for word to
  `docs/HISTORY/`. The rulings still in force are this section.
- **The methodology:** all of TEMPLATE's current pages, by its own sync
  procedure, so a page this repository edited is merged rather than
  replaced.
- **The WSL work:** every survey ask and every open finding is built,
  EXCEPT the distinct exit code for the tool's own refusals, P14, which is
  a break. So the release is a MINOR, `wsl-toolkit-v7.1.0`, and P14 stays
  an open entry.
- **This host:** the package's copy of the state directory is emptied, the
  operator's own `qemu-binfmt-restore.service` is retired in
  `wsl-toolkit-base`, and the installed skills are updated from the tree.

## 9. Before every push

⭐ **A green local gate is not CI.** Run the Go tests with `TEMP` and `TMP` at an
8.3 short path, then CI's Linux Go job and its ShellCheck in containers.
[`../tools/windows/wsl-toolkit/README.md`](../tools/windows/wsl-toolkit/README.md)
carries all three as commands under "Build and local proof". ⚠ **A mutation row
written on Windows runs on Linux before the push**, in `golang:1.25`: a case that
asserts on Windows alone stays green there, and CI reports the row THEATRE.

⚠ **Stage a new file before running the gate.** The checks read the tracked files
and the untracked ones git does not ignore, but two do not: `line-endings` reads
`git ls-files --eol`, and CI's ShellCheck lists `git ls-files "*.sh"`. On
2026-09-16 a new `.ps1` passed a full gate untracked, and failed `line-endings` on
the next run: `.gitattributes` gives `*.ps1` CRLF, and it was written with LF.

⛔ **A file git cannot see can stop the container checks dead.** A file named
`NUL` in the root took `run --workspace .` to `workspace refused: NUL shrank
while it was being read`, and `git status --untracked-files=all` listed nothing,
because git cannot stat a Windows reserved device name. Read the refusal's own
words, and remove such a file by its literal path with the `\\?\` prefix.

⚠ **A BSD run that needs a damaged or altered image goes on a copy**, with
`WSL_TOOLKIT_CACHE` under this repository's `.tmp`. A plain run no longer writes
the shared image.
