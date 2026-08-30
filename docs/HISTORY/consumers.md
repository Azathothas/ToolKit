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
