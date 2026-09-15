# Muse Code in one named WSL base

⭐ **Three commands, and the third one is the agent.** This is the concrete
provider example for the one-checkout profile in
[`../common/access-profiles.md`](../common/access-profiles.md): a persistent
Linux home for Muse with systemd, rootless Podman, the developer toolset,
CodeGraph and herdr, and exactly one Windows checkout reachable from it.

⚠ **This page used to carry eleven steps, two files copied by hand and a
multiplexer that has been replaced.** What removed them was not a shorter page:
`base.adapters` installs the software during `base ensure`, and `base agent` runs
the agent in the granted directory. The page is short because the tool does the
work.

---

## The whole of it

[`wsl-toolkit-base.json`](wsl-toolkit-base.json) is the profile. Save it at the
checkout's root as `wsl-toolkit.json`, or point `--config` straight at this file.

```powershell
wsl-toolkit --instance base --config .\wsl-toolkit.json base ensure
wsl-toolkit --instance base base grant . --mode rw
wsl-toolkit --instance base base agent muse
```

| the command | what it does |
| --- | --- |
| `base ensure` | builds the distribution, provisions it, and installs the `herdr` and `muse` adapters named in the profile. Idempotent: a second run reconciles rather than rebuilds |
| `base grant` | mounts **this** Windows directory under `/workspaces`, live, with no restart |
| `base agent muse` | runs Muse in the guest directory this Windows directory is granted at, with its screen in a herdr pane. Every argument after the name is Muse's |

⭐ **The adapter also writes `muse.exe`** into the account's bin directory, so the
same run is `muse.exe` from the checkout once that directory is on `PATH`.

⛔ **A directory no grant covers is refused**, with the `base grant` line that
would cover it. That refusal is the profile working, not a failure.

---

## Before the first run, once

```powershell
wsl-toolkit --instance base --config .\wsl-toolkit.json config validate
```

It must print `passwordless sudo true`, `systemd true`, and the two adapters.
⚠ **The profile carries no standing grant on purpose**: the base is built once
and each project is granted when it is worked on, so a base left running reaches
no checkout at all.

After `base ensure`, make it prove its live state rather than trusting the
configuration:

```powershell
wsl-toolkit --instance base base status --probe --json
```

⛔ **`registered` is not `usable`.** `--probe` runs a container and reads the
adapters back; without it the answer is what this machine believes.

---

## What the profile chose, and why

| choice | reason |
| --- | --- |
| `arch` | glibc. ⛔ herdr's 0.9.0 Linux server aborts in a **musl** malloc check and takes every pane child with it, `herdrdev/herdr#4174` |
| `automount: off`, `interop: off` | no Windows drive is reachable and no Windows executable runs. A grant is then the only door, and it is explicit |
| `passwordless_sudo: true` | a package install does not stop for a password nobody is there to type. ⚠ It is a **trust** decision about the agent, not a containment one |
| `systemd: true` | herdr's server is a system unit, so it starts with the base |
| `toolset: developer` | the thirteen commands the base itself installs as root |
| `adapters: herdr, muse` | installed by `base ensure` and read back by `base status --probe` |

---

## The account's own tools

The base's toolset is installed as root into the image. ⭐ **The bootstrap is the
other half**, and it adds the languages and CodeGraph into the account's home,
which is what persists with the base:

```powershell
wsl-toolkit --instance base base exec -c 'sh /workspaces/project/.wsl-toolkit/common/bootstrap.sh --toolset agent'
```

⛔ **Not as root.** Provider state, authentication and CodeGraph belong to the
unprivileged account. [`../common/README.md`](../common/README.md) carries what
each toolset name resolves to.

---

## Watching it

[`../common/herdr.md`](../common/herdr.md) is the operator and agent guide:
attaching from Windows, `herdr --machine` for scripted watching with nothing
open, and the four traps a Windows-to-WSL shape walks into.

⚠ **herdr detects Muse already.** It ships a Muse screen-detection manifest, so a
Muse pane is classified `idle`, `working` and `blocked` with no integration at
all. What it does not have is lifecycle authority or session identity; `WSL-76`
owns that gap and
[`../../../../../docs/reference-sweeps/usable.md`](../../../../../docs/reference-sweeps/usable.md)
carries the contract it is built from.
