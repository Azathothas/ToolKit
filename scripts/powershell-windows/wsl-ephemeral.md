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
| `List` | list ephemeral distros, every other distro, which it never touches, and any orphaned rootfs tarball |
| `Remove` | unregister one ephemeral distro and delete its disk |
| `Purge` | remove every ephemeral distro, prefix-matched only, and every orphaned rootfs tarball |

## Parameters

| parameter | applies to | meaning |
| --- | --- | --- |
| `-Image` | `New` | OCI reference, for example `alpine:3.22` or `debian:bullseye-slim`. Needs podman or docker. |
| `-Tarball` | `New` | path to a rootfs `.tar` to import instead. Needs no container engine. |
| `-Name` | `New` `Run` `Remove` | distro name. Generated when omitted. The `eph-` prefix is added if missing. |
| `-Command` | `New` `Run` | shell command, run through `/bin/sh -lc`. Carried as base64 and sourced in the guest, so quotes, `$`, backticks and tabs arrive byte-exact. |
| `-CommandFile` | `New` `Run` | path to a file **on this machine** whose bytes are the command. Read verbatim, so a multi-line script works. |
| `-CommandB64` | `New` `Run` | the command as base64 of its UTF-8 bytes. ⭐ The one to use from a script, and the only one that survives Windows PowerShell 5.1 when this tool is launched as a child process. |
| `-User` | `New` `Run` | user inside the distro. Default `root`. |
| `-Ephemeral` | `New` | run `-Command`, then destroy the distro |
| `-OciEnv` | `New` with `-Image` | carry the image's `ENV` and `WORKDIR` into the distro. Off by default. |
| `-Force` | destructive actions | required when the session is non-interactive. Skips the confirmation. |

⛔ `-Image` and `-Tarball` are mutually exclusive, and `New` requires one of
them. ⛔ `-Command`, `-CommandFile` and `-CommandB64` are mutually exclusive
too: they are three spellings of one argument, and passing two is refused
rather than resolved by a precedence nobody would remember.

## Exit codes

⭐ **`New` and `Run` answer the same way.** Both run `-Command` through one
function inside the script, so there is no second place for a code to be
dropped.

| code | meaning |
| --- | --- |
| 0 | the action completed, and `-Command`, if given, exited 0 |
| the inner command's code | from `New -Command` and from `Run -Command` alike |
| 1 | the script failed. The message names what. |

⚠ **With `-Ephemeral` the distro is destroyed before the code is returned**, so
a failing command still leaves nothing registered.

⚠ **A destructive action exits 1 when the disk is still there afterwards.**
`Remove`, `Purge` and `New -Ephemeral` read the directory back and report what
they find, so they can fail where they used to print success. See the safety
model below.

⛔ **`New -Command` used to exit 0 over a failing command.** A caller written
against that is now told the truth, which is a break.
[`../../docs/consumers.md`](../../docs/consumers.md) records it.

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

## The command channel

⭐ **A command is carried as base64 and sourced inside the distro.** Nothing is
quoted for the guest, because quoting does not survive the trip and no caller
can make it.

⛔ **This is not a style choice.** Measured on 2026-08-27 against real Alpine and
Debian distros, under **both** PowerShell 7.6.5 and Windows PowerShell 5.1, with
every hazard already correctly single-quoted for `sh` before it was passed:

| what was sent, POSIX-quoted for `sh` | PowerShell 7.6.5 | Windows PowerShell 5.1 |
| --- | --- | --- |
| `$VAR` | ⛔ expanded in transit, and the **result** is then re-parsed | ⛔ the same |
| a backtick | ⛔ opens a command substitution | ⛔ the same |
| a double quote | arrives | ⛔ `syntax error: unterminated quoted string` |
| a single quote, a bracket, a tab, a space | arrives | arrives |

The first row is the one that bites in ordinary use. `echo $PATH` used to die
with ``syntax error: unexpected "("``, because the value it expands to carries
the bracket in `Program Files (x86)` and that value is parsed again. It works
now.

### What the transport does

```text
mkdir -p /tmp && exec 8>F && exec 9<F && rm -f F && echo B64|base64 -d>&8 && . /dev/fd/9
```

⭐ **The file is unlinked before any content exists in it.** Two open
descriptors keep the inode alive, so the command's text is never a file
anything in the distro can open by name, and no failure can leave one behind.
⚠ That ordering was got wrong first: writing the file and then unlinking it
reads the same, but a redirect creates the file before the decode runs, so a
guest with no `base64` was left holding an empty one.

⛔ **Every payload this tool sends goes through that one function**, including
the smoke probe and the file `-OciEnv` writes, and it **asserts** that the
skeleton stays inside the alphabet the measurement cleared. Before, two of the
three payloads were hand-written inside that alphabet with nothing enforcing
it, which is how `-Action New` came to fail outright on 5.1.

### The requirement this puts on an image

⚠ **The guest needs `base64` and `/dev/fd`.** Measured present in both distros
this was tested against: busybox supplies `base64` in Alpine 3.22, coreutils
supplies it in Debian bookworm, and in both `/dev/fd` is a symlink to
`/proc/self/fd` that WSL puts there at import. ⛔ Neither is asserted of every
image, and a rootfs built from scratch may have neither. `New` finds out at
creation, because the smoke probe uses the same channel, and says so rather
than failing later inside somebody's command.

### ⚠ The one thing 5.1 still cannot do, and it is not this script

⛔ **Windows PowerShell 5.1 drops a double quote when it builds a child
process's argument list.** Measured: a `-Command` value of ``a'b"c`d$e`` reaches
a script spawned as `powershell -File script.ps1 -Command ...` as ``a'bc`d$e``.
The quote is gone before this script runs, so nothing this script does can
recover it. In-process, `& .\wsl-ephemeral.ps1 -Command $v`, it arrives intact,
and PowerShell 7.6.5 is fine either way.

⭐ **`-CommandB64` is immune, and that is what it is for.** Base64 has no
character any shell or argument parser touches.

```powershell
$b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($script))
pwsh -NoProfile -File wsl-ephemeral.ps1 -Action Run -Name eph-x -CommandB64 $b64
```

⚠ **`-CommandFile` is read verbatim, CRLF included.** A file with Windows line
endings makes `/bin/sh` read the carriage return as part of the last word on
each line. The tool warns and does not rewrite the file: silently editing
somebody's payload is the failure this whole channel exists to remove.

## Orphaned rootfs tarballs

`New -Image` writes a rootfs `.tar` into `%LOCALAPPDATA%\wsl-ephemeral\` and
removes it in a `finally`. ⚠ **A `finally` does not run on every hard
interrupt**, so a cancelled run can leave several hundred MiB behind.

`List` reports each one with its size and the time it was last written.
`Purge` removes them, in the same confirmation as the distros and through the
same deletion and the same containment guard.

```powershell
pwsh -NoProfile -File wsl-ephemeral.ps1 -Action List
```

⛔ **A `New` that is running right now has its tarball in that directory too**,
and nothing can tell that apart from an orphan. That is why `List` prints the
time rather than a verdict, and why the warning says to read it. Purging while
a `New` is mid-export takes that run's rootfs out from under it.

⚠ **Only the base directory is scanned.** A `.tar` anywhere else is neither
reported nor removed, and the deletion refuses a path outside that directory
even if something managed to hand it one.

---

## The image's configuration, with `-OciEnv`

⛔ **A rootfs is a filesystem and not a configuration.** `podman export` writes
the container's files; `ENV`, `WORKDIR`, `ENTRYPOINT` and `USER` live in the
image's OCI config and are not in it. So by default the distro's `PATH` is
WSL's, not the image's, and a distro built from a toolchain image does not have
that toolchain on `PATH`.

`-OciEnv` reads the config and writes `/etc/profile.d/10-oci-env.sh`, which a
login shell sources. `-Command` runs through `/bin/sh -lc`, so it gets it.

```powershell
pwsh -NoProfile -File wsl-ephemeral.ps1 -Action New -Image python:3.13-alpine -OciEnv -Command 'echo $PATH'
```

⚠ **It is off by default and it stays off by default.** Turning it on changes
`PATH` inside every distro this script makes, and every existing caller is
written against the shape they have. Changing that under them buys them
something they did not ask for.

⛔ **`USER` and `ENTRYPOINT` are not carried, and that is a decision.** WSL
fixes the login user when the distro is imported and `-User` selects it per
call, so writing `USER` into a profile script would be a setting nothing reads.
A login shell has no entrypoint to run. Both would look like they worked.

⚠ **The switch does nothing with `-Tarball`** and says so: a tarball has no
image to inspect.

⚠ **With `-OciEnv` the image's `PATH` replaces the inherited one**, which
includes the Windows directories WSL appends. If you need `explorer.exe` on
`PATH` inside the distro, do not use the switch, or add them back yourself.

---

## The architecture is named, not inherited

⭐ **Every `pull` and every `create` passes `--platform linux/ARCH`**, where
`ARCH` is this host's own architecture read from the engine.

The reason is a trap in the engine rather than in this script. ⛔ **The local
image store is keyed by tag and not by architecture.** One
`podman pull --platform linux/riscv64 alpine` repoints the shared local
`alpine:latest` at the riscv64 image, and every later unqualified
`pull alpine` is a no-op that hands back the riscv64 one. Imported into WSL,
that rootfs registers cleanly and then nothing in it executes.

⚠ **There is no `-Platform` parameter.** Building a distro whose architecture
this host cannot run is not a thing anybody has asked for, and a parameter with
no caller is machinery with a maintenance cost and no benefit. Add it when
something asks.

⚠ **The engines disagree on the spelling.** `podman info` answers `amd64` and
`docker info` answers `x86_64`, and only the first is a name `--platform`
accepts, so the script normalises. A value it does not recognise is passed
through rather than guessed at, and the engine refuses it by name.

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

⭐ **There is one deletion in the script and every path reaches it**, including
the rollback after a failed create. It runs the containment guard itself rather
than trusting each caller to, because a guard applied at four call sites is a
guard that will one day be applied at three.

### A delete that did not happen is reported as a delete that did not happen

⛔ **The script reads the state back after deleting and reports what it finds.**
`wsl --unregister` releases the disk asynchronously, so a delete immediately
afterwards can lose the race. It retries five times over about three seconds,
and if the path is still there it exits non-zero with a message naming it.

⚠ **This means `Remove`, `Purge` and `New -Ephemeral` can now fail where they
used to print success.** They were not succeeding before; they were reporting.

⭐ **`Purge` finishes both loops before it reports.** One item it cannot remove
does not stop it removing the rest; it warns per item, names each one, and then
exits non-zero with the count. Stopping at the first failure would hide the
state of everything after it.

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
- ⛔ **A foreign-architecture rootfs may run rather than fail, and that is the
  worse outcome.** The shared kernel above carries whatever `binfmt_misc`
  handlers anything on the machine has registered, and an imported distro sees
  all of them. Measured on 2026-08-27, kernel `7.2.0-WSL2-STABLE`: 31
  `qemu-*` handlers were registered and visible from a freshly imported Alpine
  distro that contains no emulator of its own.

  ```bash
  wsl -d DISTRO -u root -- /bin/sh -lc 'cat /proc/sys/fs/binfmt_misc/qemu-riscv64'
  ```

  ```text
  enabled
  interpreter /usr/bin/qemu-riscv64-static
  flags: POCF
  ```

  ⚠ The `F` flag is why the emulator does not need to exist inside the distro:
  the kernel opens the interpreter at registration time and holds it open. So a
  riscv64 rootfs on this x86_64 host boots, passes the smoke test and answers
  `riscv64` to `uname -m`. ⭐ That is why the architecture is named on every
  pull and create rather than checked afterwards: there is no afterwards to
  check in.

---

## Known limits

⛔ **These are real.** They are listed because a limit hidden is a defect filed
against the user later.

⚠ **Three different things are in this table and the difference matters.** Some
rows are tracked as **open** items in
[`../../TODO/INDEX.md`](../../TODO/INDEX.md) and will go. The `-OciEnv` row is a
**settled decision** about what that switch carries, and it stays. The 5.1
quoting row is neither: it is a **limit of the host**, one layer above this
tool, and it stays because a user hits it and needs to be told what to do
instead. ⛔ The intro used to claim every row was an open item, which stopped
being true the moment one of them was closed as a decision.

| limit | what it means for you |
| --- | --- |
| ⚠ `-OciEnv` carries `ENV` and `WORKDIR` only | `USER` and `ENTRYPOINT` are not carried and will not be. See the section on it above for why. |
| ⚠ no disk-space preflight | export plus import needs roughly twice the rootfs size on the `%LOCALAPPDATA%` volume. Running out midway leaves a partial VHDX and a registered distro that does not work. |
| ⚠ no systemd | an imported distro has no `/etc/wsl.conf`, so `systemctl` is unavailable and units, timers and services cannot be tested. |
| ⚠ on Windows PowerShell 5.1, a `-Command` value loses its double quotes when this tool is launched as a child process | 5.1 drops them building the child's argument list, before this script sees anything, so nothing here can recover them. ⭐ Use `-CommandB64`. Not an open item: it is 5.1's argument handling, one layer above this tool. See the command channel section. |
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
