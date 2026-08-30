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
| `Azathothas/TEMPLATE`, at `scripts/powershell-windows/wsl-ephemeral.ps1` | `scripts/powershell-windows/wsl-ephemeral.ps1` | a commit SHA and a SHA-256 of the file, both hardcoded in the wrapper | the path moves, a parameter is renamed, or an exit code changes meaning |
| `pkgforge-dev/cross-libc-dlopen`, at `scripts/wsl-ephemeral.ps1` | ⛔ **nothing. It carries a vendored COPY**, 536 lines against this tree's 1,579 on the day it was found, and 2,792 now | ⛔ not pinned, not fetched, no digest, no reference to this repository | ⚠ nothing here can break it, and nothing here can fix it either |
| `Azathothas/bit-cli`, at `docs/containers.md` | `scripts/powershell-windows/wsl-ephemeral.ps1`, by `curl` into `.tmp/` | a commit SHA the page tells its reader to resolve, and nothing hardcoded | the path moves, a parameter is renamed, or an exit code changes meaning. ⚠ Its procedure is written out by hand rather than run from a wrapper, so a change to the invocation shape reaches it as prose that is now wrong. |

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

## The rule for a breaking change

1. **Keep the old spelling working** where that is possible at all. An alias for
   a renamed parameter costs one line and removes the whole problem.
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
| 2026-08-30 | `wsl-ephemeral-launcher.ps1`: an explicit `-LauncherRef` now wins over a `wsl-ephemeral.ps1` sitting beside the launcher. It used to be the other way round, so a caller passing a commit and a digest could run a stale sibling and verify nothing. A break by the definition above: a caller who did nothing wrong now behaves differently. | all three rows. `Azathothas/bit-cli` is the one it changes, and it changes in that repository's favour: its `scripts/wsl-tool.ps1` deletes any sibling before every call for exactly this reason, and that workaround is now unnecessary. `Azathothas/TEMPLATE`'s wrapper fetches into a directory with no sibling. The vendored copy fetches nothing. | not moved. Nothing here requires a consumer to move; `bit-cli` may remove its workaround when it chooses. |
| 2026-08-30 | `wsl-ephemeral.ps1`: stdout from `New -Command` and `Run -Command` now carries a prefix. The stream log is on by default and stamps every line with a time and a stream tag. A break: a caller parsing that stdout gets different bytes. `-NoTimestamps` restores the previous shape exactly, byte for byte. | all three rows. `Azathothas/TEMPLATE`'s wrapper forwards arguments and reads no stream. `Azathothas/bit-cli`'s `docs/containers.md` tells a reader that results go to stdout and shows values being taken straight off a command, so that page's examples are affected and its own author decides whether to pass `-NoTimestamps` or cut the prefix. The vendored copy fetches nothing. | not moved. Recorded here the day it was made rather than when somebody notices. |
| 2026-08-29 | `wsl-ephemeral.ps1`: the final `ERROR: ...` line moved from stdout to **stderr**. ⚠ Not a break by the definition above: nothing was renamed and no exit code changed meaning. It is here because it is the one change this session made that a caller could observe. | all three rows. `Azathothas/TEMPLATE`'s wrapper forwards the inner code and reads no stream; `bit-cli`'s page reads exit codes and the command's own output; the vendored copy fetches nothing. | ⚠ **not moved.** Nothing in this batch fixes a defect a consumer is carrying, so there is no reason to ask anyone to move. |
| 2026-08-27 | `wsl-ephemeral.ps1`: `-Action New -Command` now exits with the inner command's code. It used to warn and exit 0. `WSL-01`. | `Azathothas/TEMPLATE`, the only consumer in the register. Its wrapper forwards arguments and propagates the inner code verbatim, so it needs no edit beyond the pin. | ⭐ **moved.** See the note below. |
| 2026-08-27 | `wsl-ephemeral.ps1`: `-Action New` was failing outright on Windows PowerShell 5.1 and now works. `WSL-12`. | the same single consumer. Its wrapper runs the fetched script on whichever host invoked it, so a 5.1 caller was getting the break. | ⭐ **moved**, in the same bump. |

⭐ **The pin moved twice on 2026-08-27.** First from the commit that first
published this script to the head of the batch carrying `WSL-01` through
`WSL-05`, `WSL-12` and the tooling work. ⚠ **That move was not for the two
`WSL-01` reasons alone.** It moved because `WSL-12` means every 5.1 caller of
the old pin has an `-Action New` that cannot work at all, and leaving them there
to avoid a behaviour change is protecting them from the fix rather than from the
break.

⭐ **Then to `ea5d483`**, the head of the batch carrying `WSL-06` through
`WSL-11`, in `Azathothas/TEMPLATE` as `83f573c`. `WSL-08` is why it moved: a
`-Command` value could not carry a `$`, a backtick or, on 5.1, a double quote,
and now carries anything byte-exact.

⚠ **Three behaviour changes ride along with it, and none is a break by the
definition above.** Nothing was renamed and no exit code changed meaning.
`New` can exit 1 where it used to start an import it could not finish, exit 1
where it used to hang on a wedged distro, and exit 1 where `-Systemd` was asked
for and could not be given. Each is the tool reporting a failure it used to
paper over.

⚠ **Both values were read from the API**, as the wrapper's own `.NOTES` says to.
On this machine the working-tree file hashes to `0fc409a3` and the raw endpoint
serves `3c901625`, because the tree is CRLF. A locally computed digest fails
closed, which is safe and takes an hour to work out.

⭐ **The pin was verified by running it**, not by assuming: the wrapper fetched
`ea5d48310021`, matched the digest, and `-Action Enter`, which did not exist at
the old pin, answered through it from Windows PowerShell 5.1.

⭐ **A launcher now lives here too**,
[`../scripts/powershell-windows/wsl-ephemeral-launcher.md`](../scripts/powershell-windows/wsl-ephemeral-launcher.md),
and it is **not** a second copy of the wrapper in `Azathothas/TEMPLATE`. That
one pins a commit and a digest because it lives in another repository and has
to. This one sits beside the file it runs, so it prefers the sibling and needs
no pin at all; a pin inside the repository that owns the file can only ever name
one of its own ancestors.

⚠ **Adding it does not retire the wrapper and does not move any pin.** Which of
the two `Azathothas/TEMPLATE` keeps is that repository's decision.

⚠ **A caller that was reading the false pass gets a red result the first time it
runs after the pin moves, and the failure it reports is real.** That is the
point of the change, and it is also why the row above exists: the alternative is
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
