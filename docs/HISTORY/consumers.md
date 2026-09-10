# HISTORY: consumers.md

⛔ **Superseded. Nothing here is needed to know who fetches from this repository
or what breaks them.** [`../consumers.md`](../consumers.md) is the live page and
carries the register, the definition of a break, the pin state of every row, and
the release that is now the thing to pin.

This holds the narrative that page used to carry beside its facts: how each pin
came to move, and what was measured while moving it. Moved here on 2026-08-30,
under `DOC-06`, because a reader asking "am I affected" cannot act on any of it.

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
