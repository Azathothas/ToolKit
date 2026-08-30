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
| `pkgforge-dev/cross-libc-dlopen`, at `scripts/wsl-ephemeral.ps1` | ⛔ **nothing. It carries a vendored COPY**, 536 lines against this tree's 1,579 on the day it was found, and 4,184 now | ⛔ not pinned, not fetched, no digest, no reference to this repository | ⚠ nothing here can break it, and nothing here can fix it either |
| `Azathothas/bit-cli`, at `docs/containers.md` | `scripts/windows/wsl-toolkit/wsl-toolkit.ps1`, by `curl` into `.tmp/` | a commit SHA the page tells its reader to resolve, and nothing hardcoded | the path moves, a parameter is renamed, or an exit code changes meaning. ⚠ Its procedure is written out by hand rather than run from a wrapper, so a change to the invocation shape reaches it as prose that is now wrong. |

### ⚠ `Azathothas/bit-cli` was found the same way the vendored copy was

⭐ **Found on 2026-08-29 while reading that repository's `docs/containers.md`**,
which was cited in an issue about this tool for an unrelated reason. It was not
in this register. That is twice now that reading one unrelated page found a
consumer the register did not have, which is the argument for the closing line
of this section rather than a note against it.

⚠ **It is a documentation consumer rather than a code one**, and that is a
different hazard from a pin. Nothing there executes on a schedule, so nothing
there breaks; what happens instead is that a person follows a page whose
commands no longer match the tool, and the page cannot tell them so. ⭐ Its own
measurements agree with this repository's: it records the NAT gateway its distro
saw as `172.23.96.1`, and `-Action HostAddress` on this machine answers the
same.

### ⛔ The vendored copy in `pkgforge-dev/cross-libc-dlopen`

⚠ **Found on 2026-08-27 while reading that repository for an unrelated reason**,
its `experiments/` layout. It was not in this register, and it is the shape the
register exists to catch, from the direction the register does not cover.

⭐ **A pinned consumer is one this repository can reach. A copy is one it cannot.**
The row above is not a rename hazard: a rename here cannot break a file that
never fetches. It is a **drift** hazard, and the drift is already measured.

| checked at `scripts/wsl-ephemeral.ps1` | result |
| --- | --- |
| `-CommandB64`, `-CommandFile`, `-TimeoutSeconds`, `-Systemd` | ⛔ absent, all four |
| `ConvertTo-DistroScriptCommand`, `Assert-EnoughDiskSpace`, `Invoke-ActionEnter` | ⛔ absent |
| the base64 transport | ⛔ absent. `-Command` is passed as an argument to `/bin/sh -lc` |

⛔ **It carries both P0s this repository has closed**, verified by reading its
source rather than inferred from its age:

- **`WSL-01`.** Its `-Action New` path runs the command and then
  `if ($rc -ne 0) { Write-Warn "command exited $rc" }`. It warns and does not
  exit with the code. ⚠ Its `-Action Run` path does `exit $rc` correctly, which
  is exactly the one-gated-door shape `WSL-01` was filed against.
- **`WSL-12`.** Its smoke probe is a here-string passed as an argument, and the
  payload contains both a bracket and a double quote. Windows PowerShell 5.1
  drops the double quote when it builds the child argument list, so
  `-Action New` fails outright there.

⛔ **Not fixed from here.** That repository is read-only to this one, and
[`security/remote-ops.md`](security/remote-ops.md) is absolute about it. It is
that repository's change to make, and the honest options are to take the current
file or to adopt the wrapper `Azathothas/TEMPLATE` already uses.

⚠ **The register's own closing line was already right and is now demonstrated.**
It says to treat the register as a lower bound rather than the complete set. One
reading of one unrelated repository found a consumer it did not have.

⚠ **The dependency also runs the other way, and it is not a consumer row.**
`pkgforge-dev/docker-bsd` publishes the BSD images that `BSD-01` consumes.
Nothing there fetches from here, so a rename here cannot break it; a rename or
a retag **there** breaks anything here that names an image. Tags are pinned by
name in the entry that uses them, never by `latest`.

⚠ The register above is what is known on 2026-08-29. The operator runs these
tools from other machines and other scripts by hand, and those callers are not
listed because they cannot be enumerated from here. Treat the register as the
lower bound on who is affected, never as the complete set.

⛔ **Two of the three rows were added by finding a consumer, not by being told
about one**, on two different days, both while reading something else. That is
the strongest evidence this file has that its lower-bound framing is the correct
one and that a session changing a fetched file should look rather than assume.

---

## What counts as a break

A change is breaking when a caller who did nothing wrong now behaves
differently. The three that actually happen:

| change | why it breaks a caller |
| --- | --- |
| ⛔ renaming or moving a file | the raw URL 404s. The wrapper reports it; a hand-rolled `curl \| pwsh` runs the 404 body. |
| ⛔ renaming a parameter, or changing its type | the call fails, or worse, binds to something else |
| ⛔ changing what an exit code means | a gate that was reading success now reads failure, or the reverse, which is the quiet one |

Not breaking, and worth doing freely: adding a parameter with a default, adding
an action, making an error message clearer, fixing a defect that was producing a
wrong answer.

⚠ **Fixing a false pass is a break, and it should still be done.** A caller
depending on a step that could never fail is depending on a defect. Record it as
a break so the caller is told, then fix it.

---

## ⭐ There is now a release, and it is the thing to pin

⛔ **This register was written when nothing was published from here, and that
changed on 2026-08-30.** `wsl-toolkit.ps1` and its launcher are cut as a GitHub
release on a `wsl-toolkit-v*` tag, with a `SHA256SUMS` computed in CI over the
bytes that are uploaded.

**A release is a better pin than a commit for every row above.** A commit names
a TREE, and the file at a path in it is whatever happened to be there. A release
names an ARTEFACT that was built from its sources, tested and published on
purpose, and it carries its own digests. A tag does not move either.

```powershell
pwsh -NoProfile -File launcher.ps1 -LauncherRelease wsl-toolkit-v1.0.0 -Action Doctor
```

⭐ **`wsl-toolkit-v1.0.0` was the first one, published 2026-08-30**, and the path
above was driven from an empty directory holding nothing but `launcher.ps1`: it
resolved the release, downloaded both assets, verified the script against the
published `SHA256SUMS`, created and destroyed a real distro, and returned the
inner command's exit code through both layers.

⚠ **`wsl-toolkit-v1.0.1` supersedes it the same day**, and the reason is worth
naming rather than hiding in a version number: `v1.0.0` carries a guard that
splits a path with the RUNNING host's separators. ⛔ It cannot misbehave on
Windows, which is the only platform this tool supports, so `v1.0.0` is not
withdrawn and a consumer pinned to it is not at risk. It was found by CI's ubuntu
job, which runs the suite on a host the tool never runs on.

⚠ **What the release `SHA256SUMS` proves is transport, not authorship.** It comes
from the same release as the asset, so anyone who could replace one could replace
the other. `-LauncherSha256` with a digest the caller holds is the check that
proves authorship, and it still applies on top.

**The path move above is the reason to do this now rather than later.** A
consumer that moves to a release tag stops caring where in the tree the file
lives, which is the thing that broke them this time.

---

## The rule for a breaking change

1. **Keep the old spelling working** where that is possible at all. An alias for
   a renamed parameter costs one line and removes the whole problem.
   **This session did not manage it for the path move**, and the alternative is
   recorded rather than glossed: a symlink at the old path was measured to serve
   its own target string over a raw fetch, which is a worse failure than a 404.
2. **Where it is not possible**, the change lands with a row in
   [`../CHANGELOG.md`](../CHANGELOG.md) naming what broke, and the item's
   closure says which consumers were checked.
3. ⛔ **Update the pin in every consumer this repository can reach**, in a
   separate change in that repository. Publishing the fix here is half the job.

---

## Breaks that have shipped, and where each pin stands

⛔ **A break gets a row here the moment it is made, not when somebody notices.**
The register above says who could be affected; this says who actually was, and
whether the fix has reached them yet. [`../CHANGELOG.md`](../CHANGELOG.md)
carries the story of each change; this carries only the pin state, which is the
fact a consumer's owner needs.

| date | what broke | consumers checked | pin state |
| --- | --- | --- | --- |
| 2026-08-30 | ⛔ **THE FILE MOVED AND THE OLD PATH IS GONE.** `scripts/powershell-windows/wsl-ephemeral.ps1` is now `scripts/windows/wsl-toolkit/wsl-toolkit.ps1`, and the launcher moved with it. A raw fetch of either old URL returns **404**. A git symlink at the old path was considered and REJECTED on a measurement: `raw.githubusercontent.com` serves a symlink's own target string with HTTP 200, so the old URL would have answered a successful-looking 34 bytes of text that no `pwsh` can run and no digest check would explain. A 404 is loud; that is not. | all three rows. `Azathothas/TEMPLATE` and `Azathothas/bit-cli` both name the old path and both stop working on their next run after their pin moves past this commit; the vendored copy fetches nothing. | ⛔ **not moved, and moving it is a rewrite rather than a bump.** The operator holds this: they said consumers migrate at their own pace. Until each does, its existing pin keeps working, because a pinned commit still has the old path in its tree. |
| 2026-08-30 | ⛔ **A parameter an action does not read is now REFUSED.** `-Action List -Image alpine:3.22` used to do nothing and say nothing; it now exits 1 naming the parameter and the actions that do read it. `-TimeoutSeconds` on `Run` is the one most likely to bite: it bounds the script's own questions, `Run` asks none, and the refusal names `-CommandTimeoutSeconds` instead. `WSL-23`. | all three rows. `Azathothas/bit-cli`'s `docs/containers.md` shows `-Action New` and `-Action Run` invocations with parameters those actions do read, so nothing on that page is refused. `Azathothas/TEMPLATE`'s wrapper forwards whatever it is given. | not moved. |
| 2026-08-30 | ⛔ **`-ScriptArg` was documented as repeatable and never was.** Measured under both PowerShell hosts, directly and through the launcher: a second `-ScriptArg` is refused with "specified more than once", because a `.ps1` run through `-File` cannot have a parameter repeated. `-ScriptArgFile` is the fix and takes a file of `NAME=VALUE` lines. Fixing a documented capability that did not work is a break by the definition above, and it is here for that reason. | all three rows. Nobody could have been using the repeated form, because it never bound; a caller passing ONE `-ScriptArg` is unaffected. | not moved. |
| 2026-08-30 | `launcher.ps1`: an explicit `-LauncherRef` now wins over a `wsl-toolkit.ps1` sitting beside the launcher. It used to be the other way round, so a caller passing a commit and a digest could run a stale sibling and verify nothing. A break by the definition above: a caller who did nothing wrong now behaves differently. | all three rows. `Azathothas/bit-cli` is the one it changes, and it changes in that repository's favour: its `scripts/wsl-tool.ps1` deletes any sibling before every call for exactly this reason, and that workaround is now unnecessary. `Azathothas/TEMPLATE`'s wrapper fetches into a directory with no sibling. The vendored copy fetches nothing. | not moved. Nothing here requires a consumer to move; `bit-cli` may remove its workaround when it chooses. |
| 2026-08-30 | `wsl-toolkit.ps1`: stdout from `New -Command` and `Run -Command` now carries a prefix. The stream log is on by default and stamps every line with a time and a stream tag. A break: a caller parsing that stdout gets different bytes. `-NoTimestamps` restores the previous shape exactly, byte for byte. | all three rows. `Azathothas/TEMPLATE`'s wrapper forwards arguments and reads no stream. `Azathothas/bit-cli`'s `docs/containers.md` tells a reader that results go to stdout and shows values being taken straight off a command, so that page's examples are affected and its own author decides whether to pass `-NoTimestamps` or cut the prefix. The vendored copy fetches nothing. | not moved. Recorded here the day it was made rather than when somebody notices. |
| 2026-08-29 | `wsl-toolkit.ps1`: the final `ERROR: ...` line moved from stdout to **stderr**. ⚠ Not a break by the definition above: nothing was renamed and no exit code changed meaning. It is here because it is the one change this session made that a caller could observe. | all three rows. `Azathothas/TEMPLATE`'s wrapper forwards the inner code and reads no stream; `bit-cli`'s page reads exit codes and the command's own output; the vendored copy fetches nothing. | ⚠ **not moved.** Nothing in this batch fixes a defect a consumer is carrying, so there is no reason to ask anyone to move. |
| 2026-08-27 | `wsl-toolkit.ps1`: `-Action New -Command` now exits with the inner command's code. It used to warn and exit 0. `WSL-01`. | `Azathothas/TEMPLATE`, the only consumer in the register. Its wrapper forwards arguments and propagates the inner code verbatim, so it needs no edit beyond the pin. | ⭐ **moved.** See the note below. |
| 2026-08-27 | `wsl-toolkit.ps1`: `-Action New` was failing outright on Windows PowerShell 5.1 and now works. `WSL-12`. | the same single consumer. Its wrapper runs the fetched script on whichever host invoked it, so a 5.1 caller was getting the break. | ⭐ **moved**, in the same bump. |

**How each of those pins came to move, and what was measured while moving it,
is in [`HISTORY/consumers.md`](HISTORY/consumers.md).** This page carries the pin
STATE, which is the fact a consumer's owner needs; the story of how it got there
is not something anybody can act on.

⚠ **A caller that was reading a false pass gets a red result the first time it
runs after a pin moves, and the failure it reports is real.** That is the point
of such a change, and it is why the table above exists: the alternative is
somebody debugging a step that started failing with no record of why.

---

## ⚠ A pinned consumer does not get your fix

This is the trap, and it runs in the opposite direction from the one people
expect. Pinning protects a consumer from a change it did not review, which is
exactly why it also withholds a fix it would have wanted.

So a fix here is not deployed anywhere by the act of merging it. An item that
fixes a consumer-visible defect is not closed on the strength of this repository
being green. It is closed when the pins that matter have moved, or when the
entry says explicitly which ones have not and why.

⭐ **The wrapper in `Azathothas/TEMPLATE` documents its own refresh commands**
in its `.NOTES` block, so bumping it does not require reading this file. Read
the digest from the API rather than typing it: a hand-copied digest that is
wrong fails closed, which is safe and takes an hour to work out.
