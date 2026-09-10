# consumers.md

Who fetches from this repository, what they pin, and what breaks them.

⭐ **This is the file that makes ToolKit different from an ordinary project.**
The consumers of a tool here are not in this tree, so nothing in this repository
can fail when their contract is broken. Only they can, later, on a machine
nobody is watching.

---

## The register

⛔ **Every external consumer of a file here gets a row.** A consumer nobody
wrote down is a consumer nobody checks before a rename.

| consumer | fetches | how it is pinned | breaks if |
| --- | --- | --- | --- |
| `Azathothas/TEMPLATE`, at `scripts/windows/wsl-toolkit/wsl-toolkit.ps1` | `scripts/windows/wsl-toolkit/wsl-toolkit.ps1` | a commit SHA and a SHA-256 of the file, both hardcoded in the wrapper | the path moves, a parameter is renamed, or an exit code changes meaning |
| `Azathothas/bit-cli`, at `docs/containers.md` | `scripts/windows/wsl-toolkit/wsl-toolkit.ps1`, by `curl` into `.tmp/` | a commit SHA the page tells its reader to resolve, and nothing hardcoded | the path moves, a parameter is renamed, or an exit code changes meaning. Its procedure is written out by hand rather than run from a wrapper, so a change to the invocation shape reaches it as prose that is now wrong. |
| `pkgforge-dev/cross-libc-dlopen`, at `scripts/wsl-ephemeral.ps1` | nothing. It carries a vendored COPY | not pinned, not fetched, no digest, no reference to this repository | ⚠ nothing here can break it, and nothing here can fix it either. It carries both P0 defects this repository has closed, `WSL-01` and `WSL-12`. |

⚠ **The register is a lower bound on who is affected, never the complete set.**
Two of its three rows were added by finding a consumer while reading an
unrelated page, on two different days, rather than by being told about one. The
operator also runs these tools from other machines and other scripts by hand,
and those callers cannot be enumerated from here. A change to a fetched file
says which consumers were checked rather than assuming the answer.
[`HISTORY/consumers.md`](HISTORY/consumers.md) carries how each was found.

⚠ **The dependency also runs the other way, and it is not a consumer row.**
`pkgforge-dev/docker-bsd` publishes the BSD images `BSD-01` consumes. Nothing
there fetches from here, so a rename here cannot break it; a rename or a retag
**there** breaks anything here that names an image. Tags are pinned by name in
the entry that uses them, never by `latest`.

---

## What counts as a break

A change is breaking when a caller who did nothing wrong now behaves
differently. The three that actually happen:

| change | why it breaks a caller |
| --- | --- |
| renaming or moving a file | the raw URL 404s. The wrapper reports it; a hand-rolled `curl \| pwsh` runs the 404 body. |
| renaming a parameter, or changing its type | the call fails, or worse, binds to something else |
| changing what an exit code means | a gate that was reading success now reads failure, or the reverse, which is the quiet one |

Not breaking, and worth doing freely: adding a parameter with a default, adding
an action, making an error message clearer, fixing a defect that was producing a
wrong answer.

⚠ **Fixing a false pass is a break, and it should still be done.** A caller
depending on a step that could never fail is depending on a defect. Record it as
a break so the caller is told, then fix it.

---

## What to pin, and how

⭐ **A release, not a commit.** A commit names a TREE, and the file at a path in
it is whatever happened to be there. A release names an ARTEFACT that was built
from its sources, tested and published on purpose, and it carries its own
digests. A tag does not move either.

```powershell
pwsh -NoProfile -File launcher.ps1 -LauncherRelease wsl-toolkit-v2.0.0 -Action Doctor
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
[`../scripts/windows/wsl-toolkit/launcher.md`](../scripts/windows/wsl-toolkit/launcher.md)
owns the table. `-LauncherSha256` with a digest the caller holds is the older
answer and still applies on top of both. `WSL-25`.

### The launcher runs the executable by default

| you want | pass |
| --- | --- |
| ⭐ the executable, which carries the script and adds to it | nothing. It is the default. |
| the script exactly as before | `-LauncherKind script`, or `WSL_TOOLKIT_KIND=script` |
| an executable you already hold | `-LauncherBinary PATH` |

⚠ **A consumer that fetches `wsl-toolkit.ps1` by raw URL is unaffected**,
because it never goes through the launcher. The path, the parameters and the
exit codes of that file are unchanged.

---

## The rule for a breaking change

1. **Keep the old spelling working** where that is possible at all. An alias for
   a renamed parameter costs one line and removes the whole problem.
2. **Where it is not possible**, the change lands with a row in
   [`../CHANGELOG.md`](../CHANGELOG.md) naming what broke, and the entry's
   closure says which consumers were checked.
3. ⛔ **Update the pin in every consumer this repository can reach**, in a
   separate change in that repository. Publishing the fix here is half the job.

⚠ **A path move cannot be softened with a symlink.** Measured:
`raw.githubusercontent.com` serves a symlink's own target string with HTTP 200,
so the old URL answers a successful-looking 34 bytes of text that no `pwsh` can
run and no digest check would explain. A 404 is loud; that is not.

---

## Breaks that have shipped, and where each pin stands

⛔ **A break gets a row here the moment it is made, not when somebody notices.**
The register says who could be affected; this says who actually was, and whether
the fix has reached them yet. [`../CHANGELOG.md`](../CHANGELOG.md) carries the
story of each change; this carries the pin state, which is the fact a consumer's
owner needs.

| date | what broke | consumers checked | pin state |
| --- | --- | --- | --- |
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
in [`HISTORY/consumers.md`](HISTORY/consumers.md).** This page carries the pin
STATE, which is the fact a consumer's owner needs.

⚠ **A caller that was reading a false pass gets a red result the first time it
runs after a pin moves, and the failure it reports is real.** That is the point
of such a change, and it is why the table above exists: the alternative is
somebody debugging a step that started failing with no record of why.

---

## ⚠ A pinned consumer does not get your fix

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
