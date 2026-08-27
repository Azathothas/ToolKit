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

⚠ **The dependency also runs the other way, and it is not a consumer row.**
`pkgforge-dev/docker-bsd` publishes the BSD images that `BSD-01` consumes.
Nothing there fetches from here, so a rename here cannot break it; a rename or
a retag **there** breaks anything here that names an image. Tags are pinned by
name in the entry that uses them, never by `latest`.

⚠ The register above is what is known on 2026-08-27. The operator runs these
tools from other machines and other scripts by hand, and those callers are not
listed because they cannot be enumerated from here. Treat the register as the
lower bound on who is affected, never as the complete set.

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

⚠ **`wsl-ephemeral.ps1` now has no open entry**, so the next pin move will be
for something not yet written.

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
