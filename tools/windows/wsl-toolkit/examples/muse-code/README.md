# Muse Code in one named WSL base

⭐ **One profile, three commands, and then the agent.** This is the concrete
provider example for the one-checkout profile in
[`../common/access-profiles.md`](../common/access-profiles.md): a persistent
Linux home for Muse with systemd, rootless Podman, the developer toolset and
herdr, which reaches a Windows checkout only when that checkout is granted.

---

## The whole of it

[`wsl-toolkit-base.json`](wsl-toolkit-base.json) is the profile. ⭐ **It becomes the
instance's own configuration**, which `--instance base` reads from any directory, so
one base serves every project:

```powershell
New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\wsl-toolkit\instances\base" | Out-Null
Copy-Item wsl-toolkit-base.json "$env:LOCALAPPDATA\wsl-toolkit\instances\base\config.json"
```

⛔ **Do not point `--config` at the file in this directory.** `base grant` writes the
configuration in effect, so it would edit the example.

Then, from the checkout to work on:

```powershell
wsl-toolkit --instance base base ensure
wsl-toolkit --instance base base grant --source . --mode rw
muse exec --json "Answer with only the number of files git tracks here."
```

| the command | what it does |
| --- | --- |
| `base ensure` | builds the distribution, provisions it, and installs the `herdr` and `muse` adapters the profile names. A second run reconciles rather than rebuilds |
| `base grant` | mounts **this** Windows directory under `/workspaces`, live, with no restart |
| `muse exec` | `muse.exe`, the launcher the adapter writes into `%USERPROFILE%\bin`, runs Muse in the base at the guest path this directory is granted at. Every argument is Muse's, and the exit code is Muse's |

⛔ **A directory no grant covers is refused** with exit 2 and the `base grant` line
that would cover it. That refusal is the profile working, not a failure.

⛔ **Muse's own screen needs a terminal, so `muse` with no argument, and `muse
resume`, answer exit 2 from Windows.** The screen runs in a herdr pane in the base:
attach with the line `base attach` prints, then in a pane `cd` to the guest path and
run `muse`. [`../common/herdr.md`](../common/herdr.md) is that guide.

---

## Before the first run, once

```powershell
wsl-toolkit --instance base config
```

It names the instance's own configuration as the file in effect, and prints `systemd
true`, `passwordless sudo true`, and the two adapters. ⚠ **The profile carries no
standing grant on purpose**: the base is built once and each project is granted when
it is worked on, so a base left running reaches no checkout at all.

⛔ **Signing in is the operator's, once:** `wsl-toolkit --instance base base shell`,
then `muse login`.

After `base ensure`, make it prove its live state rather than trusting the
configuration:

```powershell
wsl-toolkit --instance base base status --probe --json
```

⛔ **`registered` is not `usable`.** `--probe` runs a container and reads the
adapters back: Muse's version, whether a credential file is present, and the events
its herdr reporter is registered for.

---

## What the profile chose, and why

| choice | reason |
| --- | --- |
| `arch` | the preset both adapters are measured on; a configuration naming either on another preset is refused |
| `automount: off`, `interop: off` | no Windows drive is reachable and no Windows executable runs. A grant is then the only door, and it is explicit |
| `passwordless_sudo: true` | a package install does not stop for a password nobody is there to type. ⚠ It is a **trust** decision about the agent, not a containment one |
| `systemd: true` | herdr's server is a system unit, so it starts with the base |
| `toolset: developer` | the thirteen commands the base itself installs as root, `jq` among them, which the herdr reporter reads its events with |
| `adapters: herdr, muse` | installed by `base ensure` and read back by `base status --probe` |

---

## The account's own tools

The base's toolset is installed as root into the image. ⭐ **The bootstrap is the
other half**, and it adds the languages and CodeGraph into the account's home,
which is what persists with the base:

```powershell
wsl-toolkit --instance base base bootstrap -- --toolset agent
```

⭐ **Nothing is copied and nothing is fetched.** The executable carries the
bootstrap; `wsl-toolkit shipped list` prints its length and SHA-256, and
`base bootstrap` sends those exact bytes into the base as a payload and runs them
as the account.

⛔ **Not as root.** Provider state, authentication and CodeGraph belong to the
unprivileged account. [`../common/README.md`](../common/README.md) carries what
each toolset name resolves to.

---

## Watching it

[`../common/herdr.md`](../common/herdr.md) is the operator and agent guide:
attaching from Windows, `wsl-toolkit base herdr` for scripted watching with nothing
open, and the traps a Windows-to-WSL shape walks into.

⭐ **Muse reports its own lifecycle to herdr.** The `muse` adapter registers a hook
for six of Muse's events in `~/.config/muse/settings.json`, and the hook reports
`idle`, `working` and `blocked` for the pane Muse runs in. ⚠ herdr's own Muse screen
rules were written against an older Muse: on Muse Code 1.3.0's input screen none of
them matched and herdr fell back to `idle`, and `herdr agent wait --until working`
returned only because the hook reported the turn.
