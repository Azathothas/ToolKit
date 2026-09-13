# HISTORY: consumers.md

⛔ **Superseded. Nothing here is needed to know who fetches from this repository
or what breaks them.** [`../consumers.md`](../consumers.md) is the live page and
carries the register, the definition of a break, and how to fetch the published
executable.

This holds the narrative that page used to carry beside its facts: how each pin
came to move, and what was measured while moving it, moved here on 2026-08-30
under `DOC-06`; and the advice and pin-state table it carried for the deleted
PowerShell product, moved on 2026-09-13 under `WSL-73`.

---

## The two pin moves on 2026-08-27

⭐ **The pin moved twice that day.** First from the commit that first published
the script to the head of the batch carrying `WSL-01` through `WSL-05`, `WSL-12`
and the tooling work. ⚠ **That move was not for the two `WSL-01` reasons alone.**
It moved because `WSL-12` means every 5.1 caller of the old pin has an
`-Action New` that cannot work at all, and leaving them there to avoid a
behaviour change is protecting them from the fix rather than from the break.

⭐ **Then to `ea5d483`**, the head of the batch carrying `WSL-06` through
`WSL-11`, in `Azathothas/TEMPLATE` as `83f573c`. `WSL-08` is why it moved: a
`-Command` value could not carry a `$`, a backtick or, on 5.1, a double quote,
and now carries anything byte-exact.

⚠ **Three behaviour changes rode along with it, and none was a break by the
definition on the live page.** Nothing was renamed and no exit code changed
meaning. `New` could exit 1 where it used to start an import it could not
finish, exit 1 where it used to hang on a wedged distro, and exit 1 where
`-Systemd` was asked for and could not be given. Each is the tool reporting a
failure it used to paper over.

⭐ **The pin was verified by running it**, not by assuming: the wrapper fetched
`ea5d48310021`, matched the digest, and `-Action Enter`, which did not exist at
the old pin, answered through it from Windows PowerShell 5.1.

## ⚠ The digest that is right and looks wrong

⚠ **Both values were read from the API**, as the wrapper's own `.NOTES` says to.
On the machine that did it the working-tree file hashed to `0fc409a3` and the
raw endpoint served `3c901625`, because the tree is CRLF and the index is LF. A
locally computed digest therefore fails closed, which is safe and takes an hour
to work out.

⭐ **This is why the release publishes a `SHA256SUMS` computed in CI**, over the
exact bytes uploaded: it removes the question rather than documenting it.

## Why the launcher is not a second wrapper

⭐ **A launcher lives beside the tool as of 2026-08-29**, and it is **not** a
second copy of the wrapper in `Azathothas/TEMPLATE`. That one pins a commit and
a digest because it lives in another repository and has to. This one sits beside
the file it runs, so it prefers the sibling and needs no pin at all: a pin inside
the repository that owns the file can only ever name one of its own ancestors.

⚠ **Adding it did not retire the wrapper and did not move any pin.** Which of the
two `Azathothas/TEMPLATE` keeps is that repository's decision.

## What a caller reading a false pass saw

⚠ **A caller that was reading the false pass got a red result the first time it
ran after the pin moved, and the failure it reported was real.** That is the
point of the change, and it is why the break table exists at all: the
alternative is somebody debugging a step that started failing with no record of
why.

## How two of the three consumers were found

⚠ **Neither was reported. Both were found while reading something else**, on two
different days, which is the whole argument for the live page's lower-bound
framing. Moved here on 2026-09-10, because a reader asking "am I affected"
cannot act on any of it.

**`pkgforge-dev/cross-libc-dlopen`, 2026-08-27.** Found while reading that
repository for an unrelated reason, its `experiments/` layout. It carries a
vendored COPY of this tool under its old name, 536 lines against this tree's
1,579 on the day it was found. The drift was measured by reading its source:

| checked at `scripts/wsl-ephemeral.ps1` | result |
| --- | --- |
| `-CommandB64`, `-CommandFile`, `-TimeoutSeconds`, `-Systemd` | absent, all four |
| `ConvertTo-DistroScriptCommand`, `Assert-EnoughDiskSpace`, `Invoke-ActionEnter` | absent |
| the base64 transport | absent. `-Command` is passed as an argument to `/bin/sh -lc` |

It carries both P0 defects this repository has closed, verified by reading
rather than inferred from its age. Its `-Action New` path runs the command and
then warns without exiting with the code, which is `WSL-01`; its `-Action Run`
path does `exit $rc` correctly, which is exactly the one-gated-door shape
`WSL-01` was filed against. Its smoke probe is a here-string passed as an
argument whose payload holds a bracket and a double quote, which is `WSL-12`.

⛔ **Not fixed from here.** That repository is read-only to this one. It is that
repository's change to make, and the honest options are to take the current file
or to adopt the wrapper `Azathothas/TEMPLATE` already uses.

**`Azathothas/bit-cli`, 2026-08-29.** Found while reading that repository's
`docs/containers.md`, which was cited in an issue about this tool for an
unrelated reason. It is a documentation consumer rather than a code one, which
is a different hazard from a pin: nothing there executes on a schedule, so
nothing there breaks; what happens instead is that a person follows a page whose
commands no longer match the tool, and the page cannot tell them so. Its own
measurements agreed with this repository's: it records the NAT gateway its
distro saw as `172.23.96.1`, and `-Action HostAddress` answered the same here.

## The first two releases, 2026-08-30

⭐ **`wsl-toolkit-v1.0.0` was the first**, and the path was driven from an empty
directory holding nothing but `launcher.ps1`: it resolved the release,
downloaded both assets, verified the script against the published `SHA256SUMS`,
created and destroyed a real distro, and returned the inner command's exit code
through both layers.

⚠ **`wsl-toolkit-v1.0.1` superseded it the same day**, and the reason is worth
naming rather than hiding in a version number: `v1.0.0` carries a guard that
splits a path with the RUNNING host's separators. It cannot misbehave on
Windows, which is the only platform this tool supports, so `v1.0.0` was not
withdrawn and a consumer pinned to it was not at risk. It was found by CI's
ubuntu job, which runs the suite on a host the tool never runs on.

⚠ **The register was written when nothing was published from here.** That
changed on 2026-08-30, and the live page's advice to pin a release rather than a
commit dates from then.

## What the live page carried until 2026-09-13

Moved here when the PowerShell product, its launcher and `wsl-toolkit script` were deleted under `WSL-73`. The live page now carries the register as it stands, what counts as a break, how to fetch the executable, and what a change to a fetched contract owes. What follows is the advice and the pin-state table that page carried for the deleted product, kept as it was written.

### What to pin, and how

⭐ **A release, not a commit.** A commit names a TREE, and the file at a path in
it is whatever happened to be there. A release names an ARTEFACT that was built
from its sources, tested and published on purpose, and it carries its own
digests. A tag does not move either.

```powershell
pwsh -NoProfile -File launcher.ps1 -LauncherRelease wsl-toolkit-v2.0.1 -Action Doctor
```

A release carries `wsl-toolkit.ps1`, `launcher.ps1`, the `wsl-toolkit`
executable for two Windows architectures, a `SHA256SUMS` computed in CI over the
bytes that are uploaded, and one `.cosign.bundle` per asset.

⚠ **What that `SHA256SUMS` proves is transport, not authorship.** It comes from
the same release as the asset, so anyone who could replace one could replace the
other.

⭐ **Since `wsl-toolkit-v2.0.1` every asset also carries a keyless signature
bundle**, `<asset>.cosign.bundle`, made by this repository's release workflow and
verifiable against that workflow's OIDC identity. `launcher.ps1` checks the one
belonging to the file it fetched and reports four outcomes rather than going
quiet; `-LauncherVerify require` turns an absent bundle or an absent `cosign`
into a refusal too.
`scripts/windows/wsl-toolkit/launcher.md`, deleted with the launcher,
owns the table. `-LauncherSha256` with a digest the caller holds is the older
answer and still applies on top of both. `WSL-25`.

#### The launcher runs the executable by default

| you want | pass |
| --- | --- |
| ⭐ the executable, which carries the script and adds to it | nothing. It is the default. |
| the script exactly as before | `-LauncherKind script`, or `WSL_TOOLKIT_KIND=script` |
| an executable you already hold | `-LauncherBinary PATH` |

⚠ **A consumer that fetches `wsl-toolkit.ps1` by raw URL is unaffected**,
because it never goes through the launcher. The path, the parameters and the
exit codes of that file are unchanged.

---

### The rule for a breaking change

1. **Keep the old spelling working** where that is possible at all. An alias for
   a renamed parameter costs one line and removes the whole problem.
2. **Where it is not possible**, the change lands with a row in
   [`../../CHANGELOG.md`](../../CHANGELOG.md) naming what broke, and the entry's
   closure says which consumers were checked.
3. ⛔ **Update the pin in every consumer this repository can reach**, in a
   separate change in that repository. Publishing the fix here is half the job.

⚠ **A path move cannot be softened with a symlink.** Measured:
`raw.githubusercontent.com` serves a symlink's own target string with HTTP 200,
so the old URL answers a successful-looking 34 bytes of text that no `pwsh` can
run and no digest check would explain. A 404 is loud; that is not.

---

### Breaks that have shipped, and where each pin stands

⛔ **A break gets a row here the moment it is made, not when somebody notices.**
The register says who could be affected; this says who actually was, and whether
the fix has reached them yet. [`../../CHANGELOG.md`](../../CHANGELOG.md) carries the
story of each change; this carries the pin state, which is the fact a consumer's
owner needs.

| date | what broke | consumers checked | pin state |
| --- | --- | --- | --- |
| 2026-09-13 | ⚠ **A BASE BUILT BEFORE THIS CHANGE REPORTS UNUSABLE UNDER `--probe` UNTIL IT IS RE-PROVISIONED.** Verification now also proves the managed account owns writable `~/.config`, `~/.cache`, `~/.local/share` and `~/.local/state`, and, where `base.passwordless_sudo` is set, that `sudo -n true` works. Measured on an ordinary `wsl-toolkit` base built 2026-09-10: `base status --probe` answered `does not own a writable XDG directory at /home/toolkit/.cache`, and `base ensure` re-provisioned it in place in 13 s and answered healthy. The status JSON's `engine` field also changes for a base whose podman warns on stderr: it carried the warning and now carries the version. ⚠ A base with `automount: "off"` must also have no `/mnt/<letter>` directory left: measured on the rebuilt Muse base, nine empty mount points from the fresh import's first start made it report unusable, and `base ensure` removed them in place in 20.6 s and answered healthy. `base exec`, `base.passwordless_sudo` and `access.passwordless_sudo` are additive. `WSL-69`. | all three rows, and none is affected: none of them runs the executable's base. | not moved. |
| 2026-09-12 | ⛔ **TWO FILES MOVED AND THE OLD PATHS ARE GONE.** `tools/windows/wsl-toolkit/examples/common/bootstrap.sh` and `.../tmux.conf` are now [`../../scripts/common/bootstrap.sh`](../../scripts/common/bootstrap.sh) and [`../../scripts/common/tmux.conf`](../../scripts/common/tmux.conf). A raw fetch of either old URL returns **404**. ⚠ The bootstrap also gained a surface, where the old file took no arguments at all. ⛔ **It is not enumerated here, and that is deliberate:** a flag list in a second document is a list that goes stale, and this one already had. A door sweep on 2026-09-12 found it naming eleven flags of the seventeen the file has, because six were added after the sentence was written. `sh bootstrap.sh --help` is the authority. Its default toolset installs what the old file did not, so a caller who wants only the old behaviour passes `--toolset minimal --codegraph latest`. | all three rows, and none is affected: none of them fetches anything under `examples/`. ⚠ **The exposure window is one commit.** Both files were added in `fcca2ba` and moved here, so the only caller who could hold the old URL is one who copied it out of that commit on the same day. Neither path has ever been in a release. | not moved, and nothing needs to move. |
| 2026-09-10 | ⚠ **A release now carries ten assets rather than five, and the five new ones are signatures.** Each published file gets a `<name>.cosign.bundle` beside it. Not a break: nothing is renamed, no exit code moves, and a consumer that fetches by name finds the same names. ⭐ `launcher.ps1` gains `-LauncherVerify`, defaulting to `auto`, which REPORTS rather than refuses when a release has no bundle or the machine has no `cosign`. A consumer pinned to any earlier release sees one extra warning line and nothing else. `WSL-25`. | all three rows. None parses the asset list by count; `Azathothas/TEMPLATE` and `Azathothas/bit-cli` both reach the script, and the vendored copy fetches nothing. ⚠ A consumer that wants the stricter reading opts in with `-LauncherVerify require`, which is the one thing here that can turn a working call into a refusal, and only when they ask for it. | not moved. |
| 2026-09-10 | ⛔ **A `base.name` OUTSIDE THE PREFIX IS NOW REFUSED, and ownership is a proof the guest carries.** `base.name` was editable and the guard compared it against itself, so `base ensure` would create any syntactically valid distribution and `base remove --yes` would unregister it, while the manual said every other name was refused. A configuration is now accepted only for `wsl-toolkit` or `wsl-toolkit-<instance>`, and the irreversible call additionally reads an identity marker the tool wrote inside the distribution. A base built before the marker existed is ADOPTED on the next `base ensure` where this tool's own record describes it, and says so. `WSL-42`. | all three rows. None sets `base.name`: `Azathothas/TEMPLATE` and `Azathothas/bit-cli` both reach the script rather than the executable's configuration, and the vendored copy fetches nothing. | not moved. A caller that never named a base is unaffected. |
| 2026-09-10 | ⛔ **`base ensure` NO LONGER RELABELS A BASE TO MATCH ITS CONFIGURATION.** It called `writeRecord()` on the registered-and-healthy path, so a distribution built from Arch with the config since changed to Alpine was recorded as Alpine because a health probe ran an Alpine CONTAINER successfully. The probe proves the engine works and identifies nothing. The record now follows the GUEST, the disagreement is reported, and `ensure` exits 1 over a base that does not match what was asked for rather than 0 over one it has renamed. `WSL-42`. | all three rows, none of which reads `base.json` or the `built_from` field. | not moved. |
| 2026-09-10 | **`--instance`, `--config` and a `.wsl-toolkit/` pointer are additive**, and a caller naming none of them behaves exactly as before: distribution `wsl-toolkit`, the same state directory, the same configuration file. ONE thing changes with no flag: a `wsl-toolkit.json` in the working directory OR ANY PARENT is now read in preference to the state directory's file, and a `wsl-toolkit.toml` found in that search is REFUSED by name rather than ignored. `wsl-toolkit config` prints the file it resolved and every path it looked at. `WSL-43`, `WSL-51`. | all three rows. None ships a `wsl-toolkit.json`, and none runs the executable from a tree that has one. This is the row most likely to reach a consumer later: a repository that adds such a file changes which configuration its own calls use. | not moved. |
| 2026-09-10 | **`stderr_bytes` IS ONE SMALLER, and `artifacts` means delivered.** The container wrapper's framing newline was written back into the payload's stream, so a job writing no error output reported one byte. `artifacts` counted entries ENCOUNTERED and now counts entries DELIVERED; `artifacts_attempted` carries the old number. `effective_exit`, `retained_kind` and `retained` are new fields. `WSL-46`. | all three rows. None reads a job's JSON: two run the script, which has no such answer, and the vendored copy fetches nothing. Anything golden-comparing a transcript sees a diff, and that diff is the fix. | not moved. |
| 2026-09-10 | **`duration_ns` MEANS SOMETHING DIFFERENT.** It was the interval up to the point the container stopped and is now the interval the caller waited, which includes cleanup. On a timed-out job the two differ by seconds: measured here, 4.57s reported against 15.56s waited before, and about 5.3s for both after. Anything comparing a duration against a previous run sees the change. `WSL-45`. | all three rows, none of which reads `duration_ns`. | not moved. |
| 2026-09-09 | ⛔ **`launcher.ps1` NOW RUNS THE EXECUTABLE BY DEFAULT.** It used to resolve `wsl-toolkit.ps1` and nothing else; it now looks for `wsl-toolkit.exe` beside itself, then the release asset for this architecture, and falls back to the script only when none can be had. A caller passing `-Action ...` still works: an argument list beginning with a dash is forwarded through the executable's `script` command unchanged. `-LauncherKind script` restores the previous behaviour exactly, and `WSL_TOOLKIT_KIND=script` does the same from the environment. | all three rows. `Azathothas/TEMPLATE`'s wrapper forwards whatever it is given, so it reaches the executable and the arguments still bind. `Azathothas/bit-cli`'s `docs/containers.md` fetches `wsl-toolkit.ps1` by raw URL and runs it directly, so it does not go through this launcher at all and is unaffected. The vendored copy fetches nothing. | not moved. Nothing here requires a consumer to move; a caller that wants the old behaviour passes one flag. |
| 2026-09-09 | ⚠ **A release now carries four assets rather than two.** `wsl-toolkit-windows-amd64.exe` and `-arm64.exe` join `wsl-toolkit.ps1` and `launcher.ps1`, and `SHA256SUMS` covers all four. Not a break: a consumer reading the `SHA256SUMS` line for a name it already fetched finds the same shape. A consumer that parsed the file expecting exactly two lines would see four. | all three rows. None parses `SHA256SUMS` by line count; the launcher looks a name up in it. | not moved. |
| 2026-09-09 | **`-LauncherLocal` and `-LauncherRef` select the script, so the new default does not reach a caller who named one.** Without this, a call passing a commit and a digest would have gone to the network for an executable, compared the caller's digest -- which is their SCRIPT's -- against the downloaded binary, and refused. Driven from an empty directory before this release: `-LauncherRef <sha> -LauncherSha256 auto` fetches and verifies the script and never asks for a binary. `-LauncherKind binary` together with either is refused by name rather than resolved. | all three rows. `Azathothas/bit-cli` is the one this protects: its `scripts/wsl-tool.ps1` passes a ref and a digest. | not moved, and no move is needed. |
| 2026-09-09 | **`-LauncherSha256` IS NOW CHECKED AGAINST A FILE YOU NAMED.** `-LauncherLocal`, `-LauncherBinary` and a copy sitting beside the launcher return a file rather than fetching one, and the digest was ignored on all three: a caller passing one was told nothing and verified nothing. It now compares, and a mismatch is a refusal that prints both digests. `-LauncherSha256 auto` with a named file is refused by name, because `auto` reads a digest for a REF. A break by the definition above: a caller passing a digest that never matched used to run. | all three rows. None passes `-LauncherSha256` with `-LauncherLocal`; `Azathothas/bit-cli` passes a digest with `-LauncherRef`, which was already checked and is unchanged. | not moved. A caller whose digest is right sees one extra line saying so. |
| 2026-09-09 | **A named release now wins over an executable beside the launcher.** The script half already worked this way; the binary half did not, so a caller passing `-LauncherRelease` with a stale `wsl-toolkit.exe` beside the launcher would have run the stale one. Not a break against any published version, because the binary half ships for the first time in `wsl-toolkit-v1.1.0`. | all three rows, none of which has an executable beside a launcher today. | not moved. |
| 2026-09-09 | ⚠ **`wsl-toolkit.ps1` gained `-StateDir` and `-UserEnv`, and neither changes an existing call.** Both default to what the script did before: `-StateDir` reads `WSL_TOOLKIT_STATE_DIR` and falls back to `%LOCALAPPDATA%\wsl-ephemeral`, and `-UserEnv` is off. One behaviour did change with no flag: a read-only action no longer CREATES the state directory, and `New` refuses an existing per-distro state directory rather than importing over it. | all three rows. A caller running `-Action List` on a machine where the directory did not exist used to leave one behind and now does not, which no row depends on. | not moved. |
| 2026-08-30 | ⛔ **THE FILE MOVED AND THE OLD PATH IS GONE.** `scripts/powershell-windows/wsl-ephemeral.ps1` is now `scripts/windows/wsl-toolkit/wsl-toolkit.ps1`, and the launcher moved with it. A raw fetch of either old URL returns **404**. | all three rows. `Azathothas/TEMPLATE` and `Azathothas/bit-cli` both name the old path and both stop working on their next run after their pin moves past this commit; the vendored copy fetches nothing. | ⛔ **not moved, and moving it is a rewrite rather than a bump.** The operator holds this: they said consumers migrate at their own pace. Until each does, its existing pin keeps working, because a pinned commit still has the old path in its tree. |
| 2026-08-30 | ⛔ **A parameter an action does not read is now REFUSED.** `-Action List -Image alpine:3.22` used to do nothing and say nothing; it now exits 1 naming the parameter and the actions that do read it. `-TimeoutSeconds` on `Run` is the one most likely to bite: it bounds the script's own questions, `Run` asks none, and the refusal names `-CommandTimeoutSeconds` instead. `WSL-23`. | all three rows. `Azathothas/bit-cli`'s `docs/containers.md` shows `-Action New` and `-Action Run` invocations with parameters those actions do read, so nothing on that page is refused. `Azathothas/TEMPLATE`'s wrapper forwards whatever it is given. | not moved. |
| 2026-08-30 | ⛔ **`-ScriptArg` was documented as repeatable and never was.** Measured under both PowerShell hosts, directly and through the launcher: a second `-ScriptArg` is refused with "specified more than once", because a `.ps1` run through `-File` cannot have a parameter repeated. `-ScriptArgFile` is the fix and takes a file of `NAME=VALUE` lines. Fixing a documented capability that did not work is a break by the definition above, and it is here for that reason. | all three rows. Nobody could have been using the repeated form, because it never bound; a caller passing ONE `-ScriptArg` is unaffected. | not moved. |
| 2026-08-30 | `launcher.ps1`: an explicit `-LauncherRef` now wins over a `wsl-toolkit.ps1` sitting beside the launcher. It used to be the other way round, so a caller passing a commit and a digest could run a stale sibling and verify nothing. A break by the definition above: a caller who did nothing wrong now behaves differently. | all three rows. `Azathothas/bit-cli` is the one it changes, and it changes in that repository's favour: its `scripts/wsl-tool.ps1` deletes any sibling before every call for exactly this reason, and that workaround is now unnecessary. `Azathothas/TEMPLATE`'s wrapper fetches into a directory with no sibling. The vendored copy fetches nothing. | not moved. Nothing here requires a consumer to move; `bit-cli` may remove its workaround when it chooses. |
| 2026-08-30 | `wsl-toolkit.ps1`: stdout from `New -Command` and `Run -Command` now carries a prefix. The stream log is on by default and stamps every line with a time and a stream tag. A break: a caller parsing that stdout gets different bytes. `-NoTimestamps` restores the previous shape exactly, byte for byte. | all three rows. `Azathothas/TEMPLATE`'s wrapper forwards arguments and reads no stream. `Azathothas/bit-cli`'s `docs/containers.md` tells a reader that results go to stdout and shows values being taken straight off a command, so that page's examples are affected and its own author decides whether to pass `-NoTimestamps` or cut the prefix. The vendored copy fetches nothing. | not moved. Recorded here the day it was made rather than when somebody notices. |
| 2026-08-29 | `wsl-toolkit.ps1`: the final `ERROR: ...` line moved from stdout to **stderr**. Not a break by the definition above: nothing was renamed and no exit code changed meaning. It is here because it is the one change that session made that a caller could observe. | all three rows. `Azathothas/TEMPLATE`'s wrapper forwards the inner code and reads no stream; `bit-cli`'s page reads exit codes and the command's own output; the vendored copy fetches nothing. | not moved. Nothing in that batch fixes a defect a consumer is carrying, so there is no reason to ask anyone to move. |
| 2026-08-27 | `wsl-toolkit.ps1`: `-Action New -Command` now exits with the inner command's code. It used to warn and exit 0. `WSL-01`. | `Azathothas/TEMPLATE`, the only consumer in the register at the time. Its wrapper forwards arguments and propagates the inner code verbatim, so it needed no edit beyond the pin. | ⭐ **moved.** |
| 2026-08-27 | `wsl-toolkit.ps1`: `-Action New` was failing outright on Windows PowerShell 5.1 and now works. `WSL-12`. | the same single consumer. Its wrapper runs the fetched script on whichever host invoked it, so a 5.1 caller was getting the break. | ⭐ **moved**, in the same bump. |

**How each of those pins came to move, and what was measured while moving it, is
in the sections above.** This page carries the pin
STATE, which is the fact a consumer's owner needs.

⚠ **A caller that was reading a false pass gets a red result the first time it
runs after a pin moves, and the failure it reports is real.** That is the point
of such a change, and it is why the table above exists: the alternative is
somebody debugging a step that started failing with no record of why.

---

### ⚠ A pinned consumer does not get your fix

This is the trap, and it runs in the opposite direction from the one people
expect. Pinning protects a consumer from a change it did not review, which is
exactly why it also withholds a fix it would have wanted.

So a fix here is not deployed anywhere by the act of merging it. An entry that
fixes a consumer-visible defect is not closed on the strength of this repository
being green. It is closed when the pins that matter have moved, or when the
entry says explicitly which ones have not and why.

**The wrapper in `Azathothas/TEMPLATE` documents its own refresh commands** in
its `.NOTES` block, so bumping it does not require reading this file. Read the
digest from the API rather than typing it: a hand-copied digest that is wrong
fails closed, which is safe and takes an hour to work out.
