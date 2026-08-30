# launcher.ps1

One file to fetch. It finds
[`wsl-toolkit.ps1`](wsl-toolkit.md), verifies it as far as you allow, makes
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
pwsh -NoProfile -File scripts/windows/wsl-toolkit/launcher.ps1 -Action List
```

The copy beside the launcher is used. Nothing is fetched and nothing is cached.

### From this repository, over the network

⭐ **The short way. It resolves a commit and its digest for you, once, and
records both.**

```powershell
pwsh -NoProfile -File launcher.ps1 -LauncherRef auto -LauncherLock toolkit.lock.json -Action List
```

Every later run reads that lock and asks GitHub nothing. Commit it.

⭐ **The explicit way, when you have reviewed a revision and want that one.**

```powershell
pwsh -NoProfile -File launcher.ps1 -LauncherRef THE_COMMIT_SHA -LauncherSha256 auto -Action List
```

⛔ **There is no default ref and a branch is refused.** `auto` and `latest`
resolve one to a commit before anything is fetched, so what runs is always an
immutable revision.

⚠ **A digest you compute yourself comes from the raw endpoint, never from a
working tree.** This repository stores `.ps1` with CRLF in a checkout and LF in
the index, so a locally computed digest disagrees with what the endpoint serves.
That fails closed, which is safe and takes an hour to work out. ⭐
`-LauncherSha256 auto` removes the step.

---

## What it resolves, and in what order

The first hit wins.

| order | source | when |
| --- | --- | --- |
| 1 | `-LauncherLocal PATH`, or `WSL_EPHEMERAL_LOCAL` | you already have a copy and want that one |
| 2 | `-LauncherRef`, or `WSL_EPHEMERAL_REF` | ⭐ a revision you named. `auto` and `latest` are two of the spellings. |
| 3 | `wsl-toolkit.ps1` **beside the launcher** | a clone. No network, no cache, no digest to keep in step. |

⛔ **An explicit ref wins over the sibling, and it used to be the other way
round.** A caller passing a commit and a digest could get `Using the copy beside
this launcher`, run a stale file and verify nothing. The sibling is what "you
did not say" resolves to, not something that overrides what you did say. This is
a break; [`../../../docs/consumers.md`](../../../docs/consumers.md) records it.

⛔ **There is no default ref.** With no sibling and no ref it exits 1 and says
what to pass. Falling back to a branch would be running code nobody reviewed.

⛔ **It is not a pinned wrapper, and that is deliberate.** A pin inside the
repository that owns the file can only ever name one of its own ancestors, so it
is stale the moment the file it points at changes, and the file sitting next to
it is the newer one.

---

## ⭐ `auto` and `latest`, so nobody pastes a digest by hand

The complaint this answers: a consumer had to resolve a commit, paste forty
characters, compute a digest, paste sixty-four more, and do it again every time
either moved. Every one of those steps is a place to paste the wrong string, and
a wrong digest fails closed in a way that takes an hour to work out.

```powershell
pwsh -NoProfile -File launcher.ps1 -LauncherRef auto -LauncherLock .\toolkit.lock.json -Action List
```

| `-LauncherRef` | resolves | records | warns |
| --- | --- | --- | --- |
| a 40-character commit | nothing | | ⚠ only if no digest was given |
| ⭐ `auto` | `main` to a commit **once**, then reads the lock forever after | ⭐ the commit **and** its digest, in the lock | once, at resolution |
| `latest` | `main` on **every** run | nothing | ⛔ loudly, every run |

⛔ **Neither keyword ever fetches a branch.** Both resolve one to a commit
**first**, so the URL downloaded always names an immutable object. The pin rule
is kept; what is removed is the caller having to paste two long strings.

⭐ **`-LauncherSha256 auto`** reads the digest from the API for whatever ref is
in play and verifies the download against it. ⚠ **That is a transport check, not
a provenance one**: it catches a truncated transfer, a captive portal or a proxy
that rewrote one endpoint, because the digest and the bytes come from different
hosts. A digest a person obtained out of band and reviewed is stronger, and
passing one as `-LauncherSha256 HEX` is still available.

⛔ **`auto` with an explicit `-LauncherSha256` is refused.** The lock owns the
digest, and a second one can only agree or contradict.

### The lock file

```json
{
  "schema": "wsl-toolkit-lock/1",
  "repository": "Azathothas/ToolKit",
  "path": "scripts/windows/wsl-toolkit/wsl-toolkit.ps1",
  "branch": "main",
  "ref": "8efe6e02b1ce",
  "sha256": "ab4f6bd6c040bb9d...",
  "resolved": "2026-08-30T05:27:24Z"
}
```

⚠ **Its default home is the install directory, not the working directory.** A
launcher that wrote a file into whatever directory it was run from has written
into somebody's repository without being asked. ⭐ Pass `-LauncherLock` to put it
beside your project and commit it.

⛔ **A lock naming another repository or another path is refused by name.** Using
its commit would fetch a file whose digest could never match, and that arrives
as a mismatch that reads like an attack.

---

## ⭐ Where the bytes come from, and what happens when a host is down

⛔ **No `gh`, no `curl`, no external tool at all.** `Invoke-WebRequest` ships
with every PowerShell this script supports. A convenience path that needed the
GitHub CLI installed would have replaced one setup step with another.

The download is tried in this order, and the first that answers wins:

| order | host | how |
| --- | --- | --- |
| 1 | `raw.githubusercontent.com` | the raw file at that commit |
| 2 | `api.github.com` | the contents endpoint, `Accept: application/vnd.github.raw` |
| 3 | ⭐ `api.gh.pkgforge.dev` | the same, through [`pkgforge-dev/reverse-proxies`](https://github.com/pkgforge-dev/reverse-proxies) |

⭐ **Measured on 2026-08-30 at commit `8efe6e02`: all three served 96,170 bytes
hashing to `ab4f6bd6c040bb9d...`.**
So a fallback is not a lesser copy; it is the same object over another route,
and the digest check holds it to that whichever one answered.

⚠ **Each host is unusable for a different reason, and none of them makes the
file unavailable.** `raw.githubusercontent.com` is blocked on some corporate
networks. `api.github.com`'s anonymous rate limit is 60 requests an hour per
address; the proxy in front of it reported 5000 remaining on the same day.

⭐ **The host that answered is named on stderr** when it is not the first one. A
fallback nobody can see is a fallback nobody knows fired.

⚠ **The proxy enforces a user-agent allowlist and it is not documentation.**
Measured on 2026-08-30, same URL and same `Accept`, varying only the agent:
`wsl-ephemeral-launcher` and `Mozilla/5.0` both answered **HTTP 420**, while
`curl/8.21.0` and no agent at all answered 200. So the agent this launcher sends
carries a compatibility token beside the tool's real name and its home, rather
than in place of them.

---

## Options

⛔ **Every argument that is not one of these is forwarded to
[`wsl-toolkit.ps1`](wsl-toolkit.md) unchanged**, and this page does not
restate that script's parameters. Restating them is how a wrapper drifts from
the thing it wraps.

⭐ **The `Launcher` prefix is what makes forwarding safe.** The wrapped script
cannot grow a parameter that collides with one of these, whatever it adds later.

| option | environment | meaning |
| --- | --- | --- |
| `-LauncherLocal PATH` | `WSL_EPHEMERAL_LOCAL` | run this file. No network. |
| `-LauncherRef SHA` `auto` `latest` | `WSL_EPHEMERAL_REF` | fetch this revision, or resolve `main` once, or resolve it every run |
| `-LauncherSha256 HEX` `auto` | `WSL_EPHEMERAL_SHA256` | expect this SHA-256, or read one from the API |
| `-LauncherLock PATH` | `WSL_EPHEMERAL_LOCK` | where `auto` keeps what it resolved. Default: the install directory. |
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
`wsl-toolkit.ps1 -Action HostAddress` puts one address there and nothing else:

```powershell
$addr = pwsh -NoProfile -File launcher.ps1 -Action HostAddress 2>$null
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
. .\launcher.ps1 -LauncherAddToPath
```

```text
  * added 'C:\...\wsl-ephemeral\bin' to PATH for THIS session only
```

⚠ **This session only.** Nothing is written to the machine's environment, to a
profile, or to the registry.

⛔ **Dot-sourcing is refused for every other use, and that is not tidiness.**
`wsl-toolkit.ps1` calls `exit`, which ends the **host session** when it is
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
| ⛔ **a digest that is not 64 hex characters is refused by name** | a typo would otherwise arrive later as a mismatch nobody can explain |
| ⭐ **three hosts are tried** | one being blocked, rate-limited or down is not the file being unavailable |
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

Everything [`wsl-toolkit.md`](wsl-toolkit.md) requires applies once the
wrapped script starts running.

---

## Related

- [`wsl-toolkit.md`](wsl-toolkit.md), the tool this launches.
- [`selftest.md`](selftest.md), the test over that
  tool's pure functions.
- [`../../../docs/consumers.md`](../../../docs/consumers.md), for who fetches what
  from this repository and what a rename here breaks out there.
