# wsl-ephemeral.ps1

Create, use and destroy throwaway WSL2 distros on a Windows host. A distro is
built from an OCI image or a rootfs tarball, a command runs inside it, and it is
removed again.

This page stands alone. An agent that has read only this file can use the script
correctly, from a clone or over the network, without opening the source.

⚠ **Windows only.** It calls `wsl.exe`. On any other host it is not applicable,
and there is no fallback.

---

## Run it

### From a clone

```powershell
pwsh -NoProfile -File scripts/powershell-windows/wsl-ephemeral.ps1 -Action List
```

### From this repository, over the network

⛔ **Pin a commit, never a branch.** `main` moves, and a moved reference runs
code nobody reviewed. Resolve the commit once and keep it:

```bash
gh api repos/Azathothas/ToolKit/commits/main --jq .sha
```

Then fetch that exact revision to a file and run the file. ⚠ Do not pipe a
download straight into a shell: a truncated transfer executes the prefix, and
there is nothing left to inspect afterwards.

```powershell
$ref = 'THE_COMMIT_SHA'
$uri = "https://raw.githubusercontent.com/Azathothas/ToolKit/$ref/scripts/powershell-windows/wsl-ephemeral.ps1"
Invoke-WebRequest -Uri $uri -OutFile "$env:TEMP\wsl-ephemeral.ps1" -UseBasicParsing
pwsh -NoProfile -File "$env:TEMP\wsl-ephemeral.ps1" -Action List
```

⭐ **A caller that wants this done for it should use the wrapper** in
`Azathothas/TEMPLATE` at `scripts/powershell-windows/wsl-ephemeral.ps1`. It
pins the commit, verifies a SHA-256 before executing, caches by ref, and fails
with a message rather than silently when there is no network.

---

## Actions

| `-Action` | what it does |
| --- | --- |
| `New` | create a distro from `-Image` or `-Tarball`, optionally run `-Command`, optionally destroy it again with `-Ephemeral` |
| `Run` | run `-Command` inside an existing ephemeral distro named by `-Name` |
| `List` | list ephemeral distros, and separately list every other distro, which it never touches |
| `Remove` | unregister one ephemeral distro and delete its disk |
| `Purge` | remove every ephemeral distro, prefix-matched only |

## Parameters

| parameter | applies to | meaning |
| --- | --- | --- |
| `-Image` | `New` | OCI reference, for example `alpine:3.22` or `debian:bullseye-slim`. Needs podman or docker. |
| `-Tarball` | `New` | path to a rootfs `.tar` to import instead. Needs no container engine. |
| `-Name` | `New` `Run` `Remove` | distro name. Generated when omitted. The `eph-` prefix is added if missing. |
| `-Command` | `New` `Run` | shell command, run through `/bin/sh -lc` |
| `-User` | `New` `Run` | user inside the distro. Default `root`. |
| `-Ephemeral` | `New` | run `-Command`, then destroy the distro |
| `-Force` | destructive actions | required when the session is non-interactive. Skips the confirmation. |

⛔ `-Image` and `-Tarball` are mutually exclusive, and `New` requires one of
them.

## Exit codes

| code | meaning |
| --- | --- |
| 0 | the action completed |
| non-zero from `Run` | the inner command's own exit code, propagated |
| 1 | the script failed. The message names what. |

⚠ **`New -Command` does not propagate the inner command's exit code today.**
It prints a warning and the script still exits 0. See the limits below before
using `New -Command` as a test gate.

---

## Examples

```powershell
pwsh -NoProfile -File wsl-ephemeral.ps1 -Action New -Image alpine:3.22
```

```powershell
pwsh -NoProfile -File wsl-ephemeral.ps1 -Action Run -Name eph-alpine-3.22-a1b2 -Command "apk add gcc && gcc --version"
```

```powershell
pwsh -NoProfile -File wsl-ephemeral.ps1 -Action Purge -Force
```

---

## The safety model

This script unregisters WSL distros and deletes directories, so removal is
constrained four ways and every destructive path goes through all of them.

1. **A fixed prefix.** Every distro it creates is named `eph-...`.
2. ⛔ **It refuses to remove any distro whose name lacks that prefix.**
3. ⛔ **It refuses to remove any name on the protected list**, prefix or not:
   `podman-machine-default`, `docker-desktop`, `docker-desktop-data`,
   `rancher-desktop`, `rancher-desktop-data`. A mistake here cannot destroy
   your container runtime.
4. **Directory deletion is confined** to `%LOCALAPPDATA%\wsl-ephemeral\<distro>`.
   The base directory itself, and anything outside it, can never be the target.

`Assert-Removable` is the single choke point for 1 to 3 and
`Assert-InsideBaseDir` for 4. ⭐ **One gate per action.** A new destructive path
calls both or it does not ship.

### ⚠ Two things about WSL that this script cannot protect you from

- ⛔ **`wsl --shutdown` is machine-wide.** It is the command a person reaches
  for after finishing with a throwaway distro, and it stops every distro on the
  machine, including `podman-machine-default`. This script never runs it. If you
  run it by hand, you take your container runtime down with you.
- ⚠ **A distro's lifetime is not the kernel's lifetime.** `--terminate` and
  `--unregister` restart or remove the distro userspace. The WSL2 kernel keeps
  running in the utility VM, so kernel state survives: `binfmt_misc`
  registrations, loaded modules, and superblocks a privileged container pinned.
  Measured on 2026-08-26, a podman machine restarted seconds earlier was running
  a kernel ten hours old:

  ```bash
  podman machine ssh 'uptime -s; systemctl show -p UserspaceTimestamp --value'
  ```

  ⛔ **Only `wsl --shutdown` gives a fresh kernel.** Anyone using an ephemeral
  distro to reproduce a kernel-level condition is reading stale state otherwise,
  and will get a wrong answer confidently.

---

## Known limits

⛔ **These are real and they are not fixed.** They are listed because a limit
hidden is a defect filed against the user later. Each is tracked as an open item
in [`../../INDEX.md`](../../INDEX.md).

| limit | what it means for you |
| --- | --- |
| ⛔ `New -Command` swallows the exit code | a failing command reports success. `Run` propagates correctly; `New` warns and exits 0. Do not use `New -Command` as a CI gate until this is fixed. |
| ⭐ the image's OCI config is dropped | `podman export` writes a filesystem, not a configuration. `ENV`, `WORKDIR`, `ENTRYPOINT` and `USER` are lost, so `PATH` inside the distro is not the image's `PATH`. |
| ⚠ no `--platform` on pull or create | the exported architecture is whatever the local store happened to hold. A previous `--platform` pull retags the shared local tag, so this can silently produce a rootfs that cannot execute. |
| ⚠ a failed delete reports success | `--unregister` releases the VHDX asynchronously, so a delete immediately after can lose the race and leave a multi-gigabyte disk behind while printing that it is gone. |
| ⚠ an interrupted `New` can orphan a tarball | the rootfs `.tar` is cleaned in a `finally`, which a hard interrupt does not always run. Neither `List` nor `Purge` looks for `*.tar`. |
| ⚠ no disk-space preflight | export plus import needs roughly twice the rootfs size on the `%LOCALAPPDATA%` volume. Running out midway leaves a partial VHDX and a registered distro that does not work. |
| ⚠ no systemd | an imported distro has no `/etc/wsl.conf`, so `systemctl` is unavailable and units, timers and services cannot be tested. |
| ⚠ `-Command` has no escaping story | the string crosses PowerShell and then `/bin/sh -lc`, and the caller owns all quoting across both. |
| ⚠ the smoke probe has no timeout | a distro whose init wedges hangs the script with no output. |
| ⚠ `Run` calls `exit` | correct when the script is invoked, fatal to the host session if it is dot-sourced. ⛔ Invoke it, never dot-source it. |

---

## Requirements

| thing | needed for |
| --- | --- |
| Windows 10 2004+ or Windows 11, with WSL2 | everything |
| Windows PowerShell 5.1 or PowerShell 7+ | everything |
| podman or docker | `-Image` only. `-Tarball` needs neither. |

⚠ **On Windows, podman runs inside its own WSL distro**, so `podman machine
start` has to have happened. The script probes for this and says so rather than
failing on a cryptic pull error.

---

## Related

- [`../../docs/conventions/shell.md`](../../docs/conventions/shell.md) section 7,
  for the Windows traps this script is written against: `wsl.exe` emitting
  UTF-16LE, reserved device names, and Git Bash path conversion.
- [`../../docs/consumers.md`](../../docs/consumers.md), for what changing this
  file breaks outside this repository.
