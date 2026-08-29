# wsl-ephemeral-launcher.ps1

One file to fetch. It finds
[`wsl-ephemeral.ps1`](wsl-ephemeral.md), verifies it as far as you allow, makes
it runnable on Windows, and runs it with the arguments you gave.

This page stands alone. An agent that has read only this file can use the
launcher correctly, from a clone or over the network, without opening the
source.

⚠ **Windows only.** It clears a Windows file attribute and it runs a tool that
drives `wsl.exe`. On any other host neither applies.

---

## Run it

### From a clone, which needs no network at all

```powershell
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral-launcher.ps1 -Action List
```

The copy beside the launcher is used. Nothing is fetched and nothing is cached.

### From this repository, over the network

⛔ **Resolve a commit first. There is no default and a branch is refused.**

```bash
gh api repos/Azathothas/ToolKit/commits/main --jq .sha
```

```powershell
pwsh -NoProfile -File wsl-ephemeral-launcher.ps1 -LauncherRef THE_COMMIT_SHA -Action List
```

⭐ **Add the digest and the bytes are verified before anything executes.**

```bash
curl -sSL "https://raw.githubusercontent.com/Azathothas/ToolKit/THE_COMMIT_SHA/scripts/powershell-windows/wsl-ephemeral.ps1" | sha256sum
```

```powershell
pwsh -NoProfile -File wsl-ephemeral-launcher.ps1 -LauncherRef THE_COMMIT_SHA -LauncherSha256 THE_DIGEST -Action List
```

⚠ **Read the digest from the raw endpoint, not from a working tree.** This
repository stores `.ps1` with CRLF in a checkout and LF in the index, so a
locally computed digest disagrees with what the endpoint serves. That fails
closed, which is safe and takes an hour to work out.

---

## What it resolves, and in what order

The first hit wins.

| order | source | when |
| --- | --- | --- |
| 1 | `-LauncherLocal PATH`, or `WSL_EPHEMERAL_LOCAL` | you already have a copy and want that one |
| 2 | ⭐ `wsl-ephemeral.ps1` **beside the launcher** | a clone. No network, no cache, no digest to keep in step. |
| 3 | `-LauncherRef SHA`, or `WSL_EPHEMERAL_REF` | fetched from `Azathothas/ToolKit` at that exact revision |

⛔ **There is no default ref.** With no sibling and no ref it exits 1 and prints
the command that resolves one. Falling back to a branch would be running code
nobody reviewed.

⛔ **It is not a pinned wrapper, and that is deliberate.** A pin inside the
repository that owns the file can only ever name one of its own ancestors, so it
is stale the moment the file it points at changes, and the file sitting next to
it is the newer one.

---

## Options

⛔ **Every argument that is not one of these is forwarded to
[`wsl-ephemeral.ps1`](wsl-ephemeral.md) unchanged**, and this page does not
restate that script's parameters. Restating them is how a wrapper drifts from
the thing it wraps.

⭐ **The `Launcher` prefix is what makes forwarding safe.** The wrapped script
cannot grow a parameter that collides with one of these, whatever it adds later.

| option | environment | meaning |
| --- | --- | --- |
| `-LauncherLocal PATH` | `WSL_EPHEMERAL_LOCAL` | run this file. No network. |
| `-LauncherRef SHA` | `WSL_EPHEMERAL_REF` | fetch this revision |
| `-LauncherSha256 HEX` | `WSL_EPHEMERAL_SHA256` | expect this SHA-256 of the fetched bytes |
| `-LauncherAllowMovingRef` | `WSL_EPHEMERAL_ALLOW_MOVING_REF=1` | permit a branch or a tag. Warns every time. |
| `-LauncherInstallDir DIR` | `WSL_EPHEMERAL_CACHE` | where a fetched copy is kept. Default `%LOCALAPPDATA%\wsl-ephemeral\bin`. |
| `-LauncherAddToPath` | | put the directory on `PATH` and ⛔ run nothing |
| `-LauncherHelp` | | print the built-in help and stop |

⛔ **An unknown `-Launcher*` argument is refused, never forwarded.** Forwarded,
it would reach the wrapped script, which would complain about a parameter it
does not have, and you would go looking in the wrong file.

---

## Exit codes and streams

| code | meaning |
| --- | --- |
| the wrapped script's code | forwarded verbatim, which is the whole point |
| 1 | the launcher itself failed: no ref, a refused ref, a digest mismatch, a file that is not PowerShell |

⭐ **Everything the launcher prints goes to stderr, and it writes nothing to
stdout.** A wrapper that writes to the wrapped program's stdout corrupts it, and
`wsl-ephemeral.ps1 -Action HostAddress` puts one address there and nothing else:

```powershell
$addr = pwsh -NoProfile -File wsl-ephemeral-launcher.ps1 -Action HostAddress 2>$null
```

⚠ Measured on 2026-08-29: with the progress lines on stdout, that assignment
captured `==> Using the copy beside this launcher` ahead of the address.

---

## `-LauncherAddToPath`, and why dot-sourcing is the whole story

⛔ **A child process cannot change the environment of the session that ran it.**
Run the launcher normally and it resolves the script, reports where it is, and
**prints** the line that would put it on `PATH`. It does not claim to have done
it, because it has not.

⭐ **Dot-source it and the assignment happens in your session**, which is what
you wanted:

```powershell
. .\wsl-ephemeral-launcher.ps1 -LauncherAddToPath
```

```text
  * added 'C:\...\wsl-ephemeral\bin' to PATH for THIS session only
```

⚠ **This session only.** Nothing is written to the machine's environment, to a
profile, or to the registry.

⛔ **Dot-sourcing is refused for every other use, and that is not tidiness.**
`wsl-ephemeral.ps1` calls `exit`, which ends the **host session** when it is
reached through a dot-source rather than an invocation. The launcher refuses
with a message instead of taking your shell down.

---

## What it does that a download and a `pwsh` do not

Each of these is a refusal or a repair that a hand-rolled fetch does not have.

| | |
| --- | --- |
| ⛔ **a moving ref is refused by shape** | a 40-character commit, or an explicit `-LauncherAllowMovingRef`, and nothing in between |
| ⛔ **a digest mismatch is a hard stop** | the fetched copy is deleted and nothing runs. Never a warning. |
| ⛔ **the file is parsed as PowerShell before it runs** | a captive portal or a 404 body arriving with HTTP 200 cannot reach the execution path |
| ⭐ **the download mark is cleared** | a file fetched on Windows can carry a `Zone.Identifier` stream, and an execution policy that would run a local script refuses the same bytes with that stream on them. ⚠ The error names the policy, not the stream, which is what makes this worth doing rather than explaining. |
| **the cache is keyed by ref** | changing the ref cannot serve the old copy |
| **an implausibly small download is refused** | under 1 KiB is a redirect page, not this script |
| ⚠ **a fetch failure falls back only to a VERIFIED cache** | with no digest there is nothing to verify against, so there is no fallback and it says so |

⚠ **Verified means verified against what you gave it.** With no
`-LauncherSha256` the launcher says out loud that the bytes were not checked. A
commit cannot be pushed over, so a pinned ref alone is far from nothing; it is
still not the same as verified bytes.

---

## ⚠ How arguments are forwarded, which looks like style and is not

A splatted array is re-parsed as a command line, so `-Action` inside it binds as
a **parameter name**. That property does not survive every way of building an
array. Measured on this machine on 2026-08-29, forwarding `-Action a` to a
script with a `ValidateSet` on `-Action`:

| how the array was built | result |
| --- | --- |
| an ordinary array with `+=` | ✅ `Action=[a]` |
| `@($args \| Where-Object { $true })` | ✅ `Action=[a]` |
| a range slice, `$args[0..($args.Count - 1)]` | ✅ `Action=[a]` |
| passed through an `[object[]]` parameter | ✅ `Action=[a]` |
| `(New-Object System.Collections.ArrayList).ToArray()` | ❌ refused. `-Action` bound **positionally**, as the value of `-Action`. |

Every element is a `System.String` in all five, so nothing about the values
explains it. ⛔ The first version of this launcher used the last row and no
argument reached the wrapped script correctly.

---

## Requirements

| thing | needed for |
| --- | --- |
| Windows PowerShell 5.1 or PowerShell 7+ | everything |
| network access to `raw.githubusercontent.com` | resolution order 3 only |

Everything [`wsl-ephemeral.md`](wsl-ephemeral.md) requires applies once the
wrapped script starts running.

---

## Related

- [`wsl-ephemeral.md`](wsl-ephemeral.md), the tool this launches.
- [`../../docs/consumers.md`](../../docs/consumers.md), for who fetches what
  from this repository and what a rename here breaks out there.
