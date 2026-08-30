# wsl-toolkit.ps1

Create, use and destroy throwaway WSL2 distros on a Windows host. A distro is
built from an OCI image or a rootfs tarball, a command runs inside it, and it is
removed again.

This page stands alone. An agent that has read only this file can use the script
correctly, from a clone or over the network, without opening the source.

⚠ **Windows only.** It calls `wsl.exe`. On any other host it is not applicable,
and there is no fallback.

⛔ **This file is BUILT and it must not be edited.** The source is the parts
under [`src/`](src/), [`core/`](core/) and [`libs/`](libs/), joined in the order
[`bundle.manifest`](bundle.manifest) names. Edit a part and run the build; the
gate refuses a bundle that disagrees with its parts, so an edit made here is
lost loudly rather than quietly. [`README.md`](README.md) is how to work on it.

⚠ **It was `wsl-ephemeral.ps1` at `scripts/powershell-windows/` until
2026-08-30, and that path is gone rather than redirected.** A raw fetch of the
old URL now returns 404, which is loud. The alternative was a git symlink, and
that was measured and rejected: `raw.githubusercontent.com` serves a symlink's
own target string with HTTP 200, so the old URL would have answered a
successful-looking 34 bytes of text.
[`../../../docs/consumers.md`](../../../docs/consumers.md) carries the row.

⚠ **Two names did NOT move with the tool**, and that is deliberate: the `eph-`
distro prefix and the `%LOCALAPPDATA%\wsl-ephemeral` state directory. Both name
state that exists on machines right now, and renaming either would make `List`
and `Purge` blind to every distro and tarball created before the rename.

---

## Run it

### From a clone

```powershell
pwsh -NoProfile -File scripts/windows/wsl-toolkit/wsl-toolkit.ps1 -Action List
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
$uri = "https://raw.githubusercontent.com/Azathothas/ToolKit/$ref/scripts/windows/wsl-toolkit/wsl-toolkit.ps1"
Invoke-WebRequest -Uri $uri -OutFile "$env:TEMP\wsl-toolkit.ps1" -UseBasicParsing
pwsh -NoProfile -File "$env:TEMP\wsl-toolkit.ps1" -Action List
```

⭐ **A caller that wants all of that done for it should use the launcher**,
[`launcher.ps1`](launcher.md), which sits beside
this file. It refuses a moving ref by shape, verifies a digest, parses the file
as PowerShell before running it, clears the download mark Windows attaches, and
forwards every other argument here unchanged.

⚠ `Azathothas/TEMPLATE` also carries a wrapper at
`scripts/windows/wsl-toolkit/wsl-toolkit.ps1`, pinned to a commit and a digest
of this file. It is that repository's, and
[`../../../docs/consumers.md`](../../../docs/consumers.md) is where its pin state is
recorded.

---

## Actions

| `-Action` | what it does |
| --- | --- |
| `New` | create a distro from `-Image` or `-Tarball`, optionally run `-Command`, optionally destroy it again with `-Ephemeral` |
| `Run` | run `-Command` inside an existing ephemeral distro named by `-Name` |
| `Enter` | attach an interactive shell to an existing ephemeral distro, as `-User` |
| `List` | list ephemeral distros, every other distro, which it never touches, and any orphaned rootfs tarball |
| `Remove` | unregister one ephemeral distro and delete its disk |
| `Purge` | remove every ephemeral distro, prefix-matched only, and every orphaned rootfs tarball |
| `Resources` | report what WSL and the container engine are holding on this machine, and print the cleanup commands. ⛔ It runs none of them. |
| `HostAddress` | print the address a distro reaches this host at, for the current networking mode. ⭐ It does not create a distro to find out. |
| `Doctor` | ⭐ what this host can and cannot do, before anything is created. Read-only, and every row says how it was obtained. |

## ⭐ `Doctor`, which answers before a failure has to

The defect it exists for is a cryptic failure halfway in. Without it, "the
podman machine is not running" arrives as a pull error, "WSL is in mirrored
mode" arrives as a guest that cannot reach the host, and a console that cannot
print UTF-8 arrives as mojibake in somebody's log. Each is knowable in under a
second and none of them was said.

```powershell
pwsh -NoProfile -File scripts/windows/wsl-toolkit/wsl-toolkit.ps1 -Action Doctor
```

⭐ **Every row carries how it was obtained**, and the vocabulary is three words:

| tag | means |
| --- | --- |
| `obs` | read from an interface that reports it directly |
| `der` | computed from readings, by arithmetic |
| `abs` | ⛔ this machine cannot answer at all |

⛔ **An `abs` row is the tool refusing to fabricate, not the tool failing.** A
machine with no container engine answers `abs` for the engine and the platform,
and `-Tarball` still works there; saying so is the point.

⚠ **The clock-resolution row is measured on the spot, and the two PowerShell
hosts differ by a factor of five thousand.** Measured on one Windows 11 Pro
26200 machine on 2026-08-30, sampling `DateTime.UtcNow` for 40 ms:

| host | smallest gap between distinct readings |
| --- | --- |
| PowerShell 7.6.5 | 100 ns, over 55,490 distinct readings |
| Windows PowerShell 5.1 | 513,600 ns, over 37 distinct readings |

⭐ That is why `-TimestampFormat %9f` is documented as padding rather than
measuring: on 5.1 the last six digits it prints are always zeros this script
wrote.

## Parameters

| parameter | applies to | meaning |
| --- | --- | --- |
| `-Image` | `New` | OCI reference, for example `alpine:3.22` or `debian:bullseye-slim`. Needs podman or docker. |
| `-Tarball` | `New` | path to a rootfs `.tar` to import instead. Needs no container engine. |
| `-Name` | `New` `Run` `Enter` `Remove` | distro name. Generated when omitted. The `eph-` prefix is added if missing. ⭐ A **generated** name that collides is drawn again, up to 8 times; a name **you** gave that collides is refused, because silently using a different one would be worse. |
| `-Command` | `New` `Run` | shell command, run through `/bin/sh -lc`. Carried as base64 and sourced in the guest, so quotes, `$`, backticks and tabs arrive byte-exact. |
| `-CommandFile` | `New` `Run` | path to a file **on this machine** whose bytes are the command. Read verbatim, so a multi-line script works. |
| `-CommandB64` | `New` `Run` | the command as base64 of its UTF-8 bytes. ⭐ The one to use from a script, and the only one that survives Windows PowerShell 5.1 when this tool is launched as a child process. |
| `-User` | `New` `Run` `Enter` | user inside the distro. Default `root`. |
| `-Ephemeral` | `New` | run `-Command`, then destroy the distro |
| `-OciEnv` | `New` with `-Image` | carry the image's `ENV` and `WORKDIR` into the distro. Off by default. |
| `-Systemd` | `New` | write `/etc/wsl.conf` enabling systemd, restart the distro, and ⛔ refuse if systemd did not become PID 1. Most base images do not ship systemd; see below. |
| `-Verbatim` | `New` `Run` with `-CommandFile` | send the file's bytes exactly as they are. Off by default, and the default repairs the **copy in transit**. ⛔ The file on disk is never written to either way. |
| `-ScriptArg` | `New` `Run` | `NAME=VALUE`. Prepended as POSIX-quoted shell assignments, so nothing runs `sed` over a payload. `@hostaddress` in a VALUE expands to what `-Action HostAddress` prints. ⛔ **One pair, not repeatable**, when this script is run through `-File`; see below. |
| `-ScriptArgFile` | `New` `Run` | ⭐ a file of `NAME=VALUE` lines, one per line, for more than one pair. Blank lines and `#` lines are skipped, and its bytes get the same repair `-CommandFile` gets. |
| `-TimeoutSeconds` | `New` | how long the script's own questions to a distro may take. Default 120. ⛔ It does not bound `-Command`. |
| `-CommandTimeoutSeconds` | `New` `Run` | a bound on **your** command, in seconds. ⛔ No default. On expiry the distro is terminated and the exit code is 124. |
| `-NoTimestamps` | `New` `Run` | ⭐ the one switch that turns the whole stream log off. Byte-exact passthrough. |
| `-TimestampMode` | `New` `Run` | `Relative` (default), `Delta`, `Wall`, `Iso` or `Epoch`. |
| `-TimestampFormat` | `New` `Run` | a strftime string, with `tss`'s specifiers, for `Relative` and `Wall`. ⛔ Refused with the other three. |
| `-TimestampColumns` | `New` `Run` | ⭐ one or more of `rel,delta,wall,iso,epoch`, comma-separated. Composes, which `-TimestampMode` cannot. ⛔ Passing both is refused. |
| `-TimestampSeparator` | `New` `Run` | what goes between the stamp and the tag. Default one space. |
| `-TimestampProfile` | `New` `Run` | `human` (the default), `ci`, `forensic`, `wall` or `raw`. A starting point: anything passed beside it wins. |
| `-PrefixOnly` | `New` `Run` | the prefix and nothing else, for output that is enormous or must not reach a log. |
| `-Color` | `New` `Run` | `auto` (default), `always` or `never`. ⛔ A file sink never gets colour whatever this says. |
| `-StreamLogPath` | `New` `Run` | append a copy of the rendered log to this file. ⛔ A Windows reserved device name is refused by name. |
| `-StreamLogOverwrite` | `New` `Run` | truncate that file instead of appending. |
| `-EventLog` | `New` `Run` | ⭐ one JSON object per line, one per event, for a caller that is a program. ⛔ Refused with `-NoTimestamps`: with no relay there are no events. |
| `-Redact` | `New` `Run` | a regex, or several comma-separated, replaced with `***` before any sink. A pattern needing a literal comma writes `[,]`. |
| `-MaxLineBytes` | `New` `Run` | truncate the guest's text at this many bytes and say how many went. `0`, the default, never truncates. |
| `-TickSeconds` | `New` `Run` | seconds of **silence** before a heartbeat line. Default 30. `0` turns the heartbeat off and keeps the timestamps. |
| `-TickEscalateSeconds` | `New` `Run` | comma-separated silence thresholds at which the tick says more rather than the same thing again. Default `120,300,900`. |
| `-DryRun` | `New` `Run` `Enter` `Remove` `Purge` | ⭐ print the exact `wsl.exe` command line and the state that would change, then stop. ⛔ Nothing is created, imported, written or removed. Goes to **stdout**, so it can be captured and audited. |
| `-Force` | `New` `Remove` `Purge` | required when the session is non-interactive. Skips the confirmation. |

⛔ `-Image` and `-Tarball` are mutually exclusive, and `New` requires one of
them. ⛔ `-Command`, `-CommandFile` and `-CommandB64` are mutually exclusive
too: they are three spellings of one argument, and passing two is refused
rather than resolved by a precedence nobody would remember.

### ⛔ A parameter the action does not read is refused

The **applies to** column is enforced, not documentation. `-Action List -Image
alpine:3.22` used to do nothing and say nothing, so a caller who typed it
believed something was happening:

```powershell
pwsh -NoProfile -File scripts/windows/wsl-toolkit/wsl-toolkit.ps1 -Action List -Image alpine:3.22
```

```text
ERROR: -Action List ignores parameters you passed, so it would have done
something other than what you asked: -Image is read by -Action New and not by
-Action List.
```

⚠ **This is a break by [`../../../docs/consumers.md`](../../../docs/consumers.md)'s
definition**, and it should be: a caller who was passing a parameter that did
nothing was not getting what they asked for, and nothing told them.

⭐ **`-TimeoutSeconds` on `Run` is the one worth knowing about.** It bounds the
questions this script asks a distro for itself, and `Run` asks none, so it is
refused there with the parameter you actually wanted named:
`-CommandTimeoutSeconds`.

### ⛔ A list parameter takes ONE comma-separated value, and it is not a choice

Measured under PowerShell 7.6.5 and Windows PowerShell 5.1 on 2026-08-30. A
`.ps1` run through `pwsh -File`, which is how every consumer runs this one,
cannot be handed a real array:

| what a caller types | what the script receives |
| --- | --- |
| `-TimestampColumns rel,delta` | ⭐ the one string `rel,delta`, which the script splits itself |
| `-ScriptArg A=1 -ScriptArg B=2` | ⛔ refused: `parameter 'ScriptArg' is specified more than once` |
| `-TickEscalateSeconds 5 9` | ⛔ refused: `a positional parameter cannot be found` |

⛔ **An `[int[]]` parameter is worse than refused, it is silently wrong.**
PowerShell converts a string to an int with the current culture's number style,
where a comma is the **thousands** separator, so `-TickEscalateSeconds 5,9` bound
to the single value `59`. That shipped, the escalation never fired, and nothing
said so. Every list parameter here now takes strings and parses them itself.

⚠ **`-ScriptArg` is the exception and it takes a FILE instead**, because there
is no safe delimiter for it: a VALUE is arbitrary, and a URL query string
carries commas. `-ScriptArgFile` has nothing to be ambiguous about.

## Exit codes

⭐ **`New` and `Run` answer the same way.** Both run `-Command` through one
function inside the script, so there is no second place for a code to be
dropped.

| code | meaning |
| --- | --- |
| 0 | the action completed, and `-Command`, if given, exited 0 |
| the inner command's code | from `New -Command` and from `Run -Command` alike |
| 124 | `-CommandTimeoutSeconds` was reached. The same code coreutils' `timeout` uses. |
| 1 | the script failed. The message names what. |

⚠ **The failure message goes to stderr**, not stdout. An error is not a result,
and `HostAddress` makes that concrete: a caller assigning this script's stdout
to a variable would otherwise get the string `ERROR: ...` where an address goes.

⚠ **With `-Ephemeral` the distro is destroyed before the code is returned**, so
a failing command still leaves nothing registered.

⚠ **A destructive action exits 1 when the disk is still there afterwards.**
`Remove`, `Purge` and `New -Ephemeral` read the directory back and report what
they find. See the safety model below.

---

## Examples

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action New -Image alpine:3.22
```

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action Run -Name eph-alpine-3.22-a1b2 -Command "apk add gcc && gcc --version"
```

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action Purge -Force
```

---

## What this machine is holding, with `Resources`

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action Resources
```

⛔ **It offers and it does not do.** Every cleanup command is printed and none
is run, including the one for the distros this script created. Reclaiming
somebody's disk is their decision, and an agent's job here is to hand them the
numbers and ask.

⚠ **Most of what it reports is not this script's.** The container engine is
shared with everything else on the machine, and the situation that asked for
this action was an agent finding hundreds of images on a host where this script
had never run. So the report is in three parts and says on every line which it
is:

| part | what it covers |
| --- | --- |
| what this script made | each `eph-` distro with the size of its disk, plus any orphaned rootfs tarball. ⭐ The only things `-Action Purge` will ever remove. |
| what else WSL has registered | named, never touched. Their disks are wherever they were imported to, which this script does not know. |
| what the engine is holding | `system df` by type, the dangling image count, and the unused volume count. ⛔ None of it this script's to remove. |

```text
==> What this script made, under %LOCALAPPDATA%\wsl-ephemeral
  eph-toolkit-probe                              76.0 MiB
  * 76.0 MiB held by this script, across 1 distro(s) and 0 tarball(s)
```

⚠ The real output names the expanded path. It is written here in its
environment-variable form because this repository is public and an absolute home
path carries a username. [`../../../docs/public/README.md`](../../../docs/public/README.md)
is the rule, and `check-no-secrets.sh --public` is what caught it.

⚠ **A directory it cannot measure is named and the total is withheld.** A total
that silently counts an unreadable directory as zero is a number somebody acts
on.

⚠ **It cannot tell a leftover from something in use.** A named volume with no
container attached is not garbage: it is how somebody keeps data between runs.
That is why the output is a count and a size rather than a recommendation.

⛔ **`podman system prune -a --volumes` removes every image no *running*
container uses**, which is not the same as unused. The report says so beside the
command rather than in a footnote.

⚠ **Its questions to the engine are bounded by `-TimeoutSeconds`** like every
other question this script asks. On Windows podman talks to a VM, and a machine
that is starting or wedged leaves the client waiting with nothing on either
stream. A timed-out engine is reported and the WSL half of the report still
prints.

---

## Talking to the host from inside a distro, with `HostAddress`

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action HostAddress
```

```text
172.23.96.1
```

⭐ **The address is the only thing on stdout.** Every explanatory line goes to
stderr, so a caller can take the value directly:

```powershell
$addr = pwsh -NoProfile -File wsl-toolkit.ps1 -Action HostAddress 2>$null
```

⛔ **It does not create a distro.** The alternative a caller had was to build a
throwaway VM, read `/proc/net/route` inside it and decode little-endian hex, to
answer a question the host already knows.

| the mode in `%USERPROFILE%\.wslconfig` | the answer |
| --- | --- |
| `mirrored` | `127.0.0.1`. The distro and the host share the loopback address, and a caller's branch disappears. |
| `nat`, which is WSL's default | the host's address on its WSL adapter, read from the interface. |
| `bridged`, or anything else | ⛔ refused, exit 1. The distro is on the LAN and reaches this host at whichever host address is on that switch, which is a choice rather than a lookup. |

⛔ **In NAT mode a host service on `127.0.0.1` is not reachable from the
distro.** This is the trap the action exists for, and it is silent: a fixture
bound to loopback simply never receives a connection, and nothing on either side
says why. Bind it to the address this prints, or to `0.0.0.0` if you accept the
LAN as well.

⚠ **Read the address, never record it.** WSL assigns it and it changes.

⚠ **A commented setting is not a setting.** Real `.wslconfig` files carry the
alternatives commented out above the live one. A parser that grepped for the key
would find `#networkingMode=mirrored` and answer `mirrored` on a host running
NAT, which is wrong in the expensive direction: `127.0.0.1` is a plausible
address that never connects. Only an uncommented key under `[wsl2]` is read, and
the last one wins.

⚠ **In NAT mode the WSL adapter exists only once the utility VM has started.**
Before that, this exits 1 and says so rather than inventing an address.

### ⭐ Measured, on this machine, on 2026-08-29

| | |
| --- | --- |
| mode, from `.wslconfig` | `nat` |
| adapter | `vEthernet (WSL (Hyper-V firewall))` |
| `-Action HostAddress` | `172.23.96.1` |
| `/proc/net/route` inside a real `alpine:3.22` distro | `016017AC`, which is `172.23.96.1` |

⭐ **The two agree**, which is the point: the action answers what the distro
would have said, without the distro. ⚠ The adapter has also been called
`vEthernet (WSL)` on other builds, so it is matched on that prefix rather than
on an exact name.

---

## The bound on the script's own questions

⛔ **Every question this script asks a distro has a hard time limit.** A distro
whose init wedges used to hang it forever with no output at all.

```text
ERROR: TIMED OUT after 15s waiting for the smoke probe in 'eph-x'. It never
answered, which is not the same as it not being installed: the distro is
registered and wsl.exe ran, and nothing came back. Its init is most likely
wedged. The distro has been terminated. Raise the bound with -TimeoutSeconds if
this machine is simply slow.
```

⭐ **"It never answered" and "it is not installed" are different facts and get
different messages.** `wsl.exe` missing is refused by name before anything runs;
a distro whose `/bin/sh` produced the wrong answer says `did not run`; and a
distro that produced no answer at all says `TIMED OUT`.

⛔ **`-Command` is NOT bounded by this, deliberately.** A build that runs for an
hour is a legitimate command, and a tool that kills it at two minutes is broken.
What is bounded is the smoke probe and the `-Systemd` check: the questions the
script asks for itself.

| | |
| --- | --- |
| default | 120 seconds |
| change it with | `-TimeoutSeconds N`, 5 to 3600 |
| when it fires | the distro is terminated, rolled back, and nothing stays registered |

⚠ **Raise it rather than removing it if this machine is slow.** The measured
costs it has to cover on this one: about 11 seconds for systemd to boot under
`-Systemd`, and up to 10 seconds inside the probe itself waiting for the drvfs
automount of `/mnt/c`.

## systemd, with `-Systemd`

An imported distro has no `/etc/wsl.conf`, so WSL starts it with its own `init`
and nothing involving units, timers or `systemctl` can be tested. `-Systemd`
writes the file, restarts the distro so WSL reads it, and ⛔ **refuses if
systemd did not actually become PID 1.**

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action New -Image almalinux:9 -Systemd -Command 'systemctl is-system-running'
```

```text
==> Enabling systemd via /etc/wsl.conf
  * systemd is PID 1
==> Running command as 'root'
running
```

### ⛔ Most OCI base images do not ship systemd, and that is the catch

⚠ **This is the thing to know before reaching for the switch.** Measured on
2026-08-27:

| image | `/usr/lib/systemd/systemd` | what `-Systemd` does |
| --- | --- | --- |
| `almalinux:9` | ⭐ present | PID 1 becomes `systemd`, `is-system-running` says `running` |
| `alpine:3.22` | absent | ⛔ refused, and nothing is left registered |
| `ubuntu:24.04` | absent | ⛔ refused |
| `fedora:41` | absent | ⛔ refused |

⛔ **The refusal is the feature.** Written into an image with no systemd the
flag would be a setting nothing reads, and a caller would come away believing
they had systemd. So the switch verifies its own effect by reading
`/proc/1/comm` and fails loudly when the effect is absent.

⚠ **`wsl: Failed to start the systemd user session for 'root'` is not the
failure.** It appears on a working systemd distro too. It is about the per-user
session, not the system manager, and `systemctl is-system-running` answering
`running` is the fact that matters.

⚠ **The restart is not optional and it is not free.** `--terminate` is what
makes WSL re-read `wsl.conf`; without it the switch would appear to work and
only the next session would get it. The first command afterwards waits for
systemd to boot, measured at about 11 seconds on this machine.

⚠ **It works with `-Tarball` too**, unlike `-OciEnv`: the file is written into
the imported distro and needs no image to inspect.

## The disk-space preflight

`New` measures free space on the volume that will hold the disk, before
`--import`, and **refuses** when there is not enough. Running out midway leaves
a partial VHDX and a registered distro that does not work, and unpicking that is
a job the user did not ask for.

```text
  * space: 272 MiB needed, 438,292 MiB free
```

⛔ **A refusal happens before anything is registered.** The message names what
is needed, what is free, and which volume:

```text
ERROR: NOT ENOUGH DISK SPACE to import '...\eph-x'. Need about 1,000,015 MiB
and 438,292 MiB is free on the volume holding C:\. Nothing has been imported
and nothing is registered. Free some space, or point LOCALAPPDATA at a volume
that has it, and run this again.
```

### ⚠ The requirement is a floor plus a multiple, and the floor is what matters

⛔ **"Roughly twice the rootfs size" is not the rule.** Measured on 2026-08-27
on this machine, VHDX size on disk against the rootfs tarball that produced it:

| image | rootfs `.tar` | VHDX on disk | ratio |
| --- | --- | --- | --- |
| `alpine:3.22` | 8.2 MiB | 76.0 MiB | ⛔ 9.27x |
| `python:3.13-alpine` | 45.4 MiB | 140.0 MiB | 3.08x |
| `debian:bookworm-slim` | 74.3 MiB | 172.0 MiB | 2.31x |
| `ubuntu:24.04` | 76.9 MiB | 172.0 MiB | 2.24x |

⭐ **An 8 MiB rootfs costs 76 MiB**, so the cost is dominated by a fixed floor
rather than by a multiple of the input. The check asks for **256 MiB plus twice
the tarball**, which is above every row above with room to spare. ⚠ It is
deliberately not fitted to them: a preflight that is tight refuses an import
that would have worked, which is a worse failure than the one it prevents.

⚠ **A volume whose free space cannot be read is imported anyway, and says so.**
"I could not measure" is a third answer, and treating it as either of the other
two would be a lie in one direction or a needless refusal in the other.

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
⚠ The order of those five is the point, and reversing any of it is a silent
change: a redirect creates the file before the decode runs, so a guest with no
`base64` would be left holding an empty one.

⛔ **Every payload this tool sends goes through that one function**, including
the smoke probe and the file `-OciEnv` writes, and it **asserts** that the
skeleton stays inside the alphabet the measurement cleared. ⛔ A payload
hand-written inside that alphabet is a constraint nothing enforces.

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
recover it. ⚠ It survives `& .\wsl-toolkit.ps1 -Command $v` in the same
process, and PowerShell 7 does not have the fault at all;
[`../../../docs/conventions/shell.md`](../../../docs/conventions/shell.md) section 8
carries the measurement.

⭐ **`-CommandB64` is immune, and that is what it is for.** Base64 has no
character any shell or argument parser touches.

```powershell
$b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($script))
pwsh -NoProfile -File wsl-toolkit.ps1 -Action Run -Name eph-x -CommandB64 $b64
```

⚠ **`-CommandFile` repairs the copy it sends and never the file.** The file
channel section below is what it does and what `-Verbatim` turns off.

---

## ⭐ The stream log: a timestamp on every line, and a heartbeat when there are none

⛔ **On by default. `-NoTimestamps` turns all of it off.**

The failure it exists for: a command prints nothing for twenty minutes, and a
caller reading a pipe cannot tell that from a command that has died. An agent
waits on a matcher that never fires, somebody eventually kills the run, and the
only evidence left is `exit 137`. **Silence has to be a line, or it says nothing
at all.**

```text
00:00:00.145 out  hello from stdout
00:00:02.300 out~ Continue? [y/N]
00:00:05.121 out~ 10%
00:00:06.123 out  100%
00:00:09.339 tick 3s silent | elapsed 9s | out 7 lines 89 B | err 1 lines 18 B | distro Running
00:00:13.122 out  after the quiet
```

### The line

`<stamp> <tag> <detail>`, and the tag is a **fixed four-character field** so a
downstream `awk` can cut on it.

| tag | what it is | which stream it is written to |
| --- | --- | --- |
| `out ` | the guest's stdout | stdout |
| `err ` | the guest's stderr | stderr |
| `tick` | the watcher's heartbeat | ⭐ stderr, because it is not the command's output |
| `note` | the watcher saying something once: silence ending, what the readings are consistent with, what an exit code could mean | stderr, for the same reason |
| `out~` `err~` | ⚠ the line had **not ended** when it was printed | as above |

⛔ **The guest's bytes are never touched.** Everything added is to the left of
`<detail>`: nothing is re-wrapped, re-quoted, truncated or coloured.

⭐ **The tick goes to stderr on purpose.** A caller capturing stdout to read a
result must not find a heartbeat in it.

### ⭐ A carriage return ends a line, which is what makes a progress bar visible

`curl`, `apt` and every layer-progress meter redraw **one** line with a carriage
return and emit no newline for minutes. A line-oriented reader shows nothing the
whole time, and that silence is indistinguishable from a deadlock.

⚠ **An unterminated line that has sat still for two seconds is shown early**,
marked with the trailing character above. A prompt waiting on stdin that will
never arrive is exactly that shape, and it is the one case where showing nothing
means waiting forever.

⚠ **A trailing carriage return is held, never emitted.** It may be the first
half of a CRLF split across two reads. The consequence is worth knowing: a guest
that emits a carriage return and then a newline is indistinguishable from a CRLF
line ending, so CRLF damage in a payload is **less** visible in the log than it
is in the guest.

### The heartbeat

⚠ **It fires on silence, not on a timer.** A command printing a line a second
produces no ticks at all. A heartbeat that beats through a conversation is one
people filter out.

⛔ **Nothing is injected into the guest to produce it.** Every figure is one the
host already holds, plus one read-only `wsl --list --verbose`. An image with no
shell and no coreutils ticks exactly as well as a full userspace.

| the tick says | why it is on the line |
| --- | --- |
| how long it has been silent | the number the caller is actually waiting on |
| elapsed | how far into the run this is |
| lines and bytes per stream | busy-but-quiet and idle-and-quiet are different |
| ⭐ the distro's state | the distro being gone and the command being quiet look identical otherwise |

### Timestamps

| `-TimestampMode` | example | default format |
| --- | --- | --- |
| `Relative` ⭐ default | `00:01:04.882` | `%H:%M:%S.%3f` |
| `Delta` | `+64.001` | fixed |
| `Wall` | `2026-08-30 11:04:18` | `%Y-%m-%d %H:%M:%S`, which is `tss`'s |
| `Iso` | `2026-08-30T11:04:19.464+05:45` | fixed |
| `Epoch` | `1788067160` | fixed |

⛔ **`Delta` measures since the previous LINE, and a tick is not a line.** A tick
advancing that clock made a five-second gap read as `+0.619` against the last
tick, on a stream where the ticks are not even present.

`-TimestampFormat` takes `tss`'s specifiers, which are `%Y %m %d %H %M %S %3f
%6f %9f %z %Z` and a doubled percent sign. ⛔ An unknown one is refused by name,
and so is one with no meaning in the mode: a year is not a thing a duration has.
The format is rendered once at startup and thrown away, so a typo is refused
before a distro is built rather than after.

⚠ **`%9f` pads and does not measure.** The .NET tick is 100ns, so the ninth
digit is a zero this script wrote, and the Windows clock's own resolution is
coarser still.

⚠ **A literal colon survives a host whose culture would replace it.** The
specifiers are substituted directly rather than through a .NET custom format
string, where a colon and a slash are culture-dependent placeholders.

### ⚠ What the log costs, and why the off switch is not decoration

⛔ **Relaying means the guest's stdout is a pipe rather than an inherited
handle.** Three consequences, and all three are absent under `-NoTimestamps`:

| | |
| --- | --- |
| an application that block-buffers when it is not on a terminal **will** buffer | its lines then carry the time this script received them, which can be much later than when they were written. `stdbuf -oL` and `PYTHONUNBUFFERED=1` are the guest's fix; this script cannot do it for you. |
| lines are terminated with the **host's** newline | a guest line that was LF-terminated arrives CRLF-terminated |
| ⛔ **stdout now carries a prefix** | a break. A caller parsing `Run -Command` output passes `-NoTimestamps` or cuts the prefix. [`../../../docs/consumers.md`](../../../docs/consumers.md) records it. |

---

## ⭐ What the tick says as the silence grows

A line repeated forty times is a line a reader stops reading, so the tick says
**more** at each threshold in `-TickEscalateSeconds` rather than the same thing
again. A real run, with the thresholds pulled in to seconds so they fire:

```text
00:00:00.206 +0.001 out  going quiet
00:00:03.433 +3.226 tick 3s silent | elapsed 3s | out 3 lines 93 B | err 0 lines 0 B | distro Running | disk 76.0 MiB
00:00:06.648 +6.441 tick 6s silent | elapsed 6s | out 3 lines 93 B | err 0 lines 0 B | distro Running | disk 76.0 MiB (unchanged)
00:00:06.651 +6.444 note this process relays the guest through a pipe rather than a terminal, so an application that block-buffers off a tty is buffering
00:00:06.653 +6.446 note after 6s of silence: NOTHING is ruled out. because: the distro disk did not grow between the last two ticks
00:00:08.365 +8.158 note output resumed after 8s of silence
```

⭐ **"Output resumed" is a line because its absence is ambiguous.** A run that
recovered at four minutes and a run that never recovered are the same picture in
a log that reports only the alarm.

### ⚠ The disk figure, and what three measurements showed it is worth

⛔ **Nothing is injected into the guest to produce any of this.** Every figure is
one this process already holds, plus one read-only `wsl --list --verbose` and
one file length on this host's own disk. Four candidate signals were sampled on
one Windows 11 Pro 26200 machine on 2026-08-30, every two seconds, while a guest
wrote 480 MiB:

| candidate | result |
| --- | --- |
| `vmmemWSL` `TotalProcessorTime` | ⛔ `0.00` throughout. Not a signal at all. |
| `vmmemWSL` `WorkingSet64` | 1228 to 1822 to 1582 MiB. ⛔ Machine-wide: every distro shares one utility VM, and it tracks page cache rather than work. |
| `ext4.vhdx` `LastWriteTimeUtc` | ⛔ **useless, and it looked useful.** It advanced every one to three seconds while the guest wrote AND while the guest sat in `sleep 14`. WSL touches the disk on its own. |
| `ext4.vhdx` `Length` | ⭐ 79,691,776 to 583,008,256 while the guest allocated, flat while it did not. ⚠ Coarse: a second run writing 120 MiB showed no change six seconds later. |

⭐ **So growth is evidence and flatness is not**, and the escalation says exactly
that. A computation that writes no files, a lock, a prompt waiting on stdin, and
a guest writing hard whose disk has not been extended yet all read identically
from out here. ⛔ The strongest thing this tool will say about a quiet command is
what the readings are consistent with, tagged as an inference, with the readings
it was drawn from named on the same line.

### A non-zero exit gets a reading, not just a number

`exit 137` is the code from the report this whole layer was built for, and on
its own it means "killed by signal 9" and stops:

```text
00:03:42.201 note exit 137 is 128+9, which is SIGKILL and nothing more. It is
             produced by: the kernel out-of-memory killer inside the utility VM,
             which every WSL distro shares; something outside this run sending a
             signal; wsl --shutdown, or the utility VM going away, which takes
             every distro at once. What can be ruled on here: the distro is still
             Running, so the whole utility VM did not go away. ⛔ This script did
             not send it: its own timeout reports 124.
```

## ⭐ The event log, with `-EventLog`

One JSON object per line, one per event. ⭐ **The rendered log is a view over
this**: anything the terminal shows that this does not carry would be a renderer
knowing something the record does not.

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action Run -Name eph-x -Command 'make' -EventLog run.jsonl
```

| field | always | notes |
| --- | --- | --- |
| `schema` | yes | `wsl-toolkit-event/1`. ⛔ Versioned, because the reader is a program. |
| `seq` | yes | ⭐ monotonic and gapless. A gap means records were dropped, which is itself a finding. |
| `t_rel`, `t_wall` | yes | seconds since the command started, and the wall clock |
| `kind` | yes | `LOG`, `TICK`, `TICK_FACTS`, `NOTE`, `EXIT` |
| `prov` | yes | `obs` read from an interface, `der` computed, `inf` a judgement that can be wrong |
| `stream` | on a line | `stdout`, `stderr` or `watcher` |
| `text`, `partial` | on a line | the guest's bytes, and whether the line had ended |

⛔ **Refused with `-NoTimestamps`.** That switch turns the relay off entirely, so
there are no events to record, and writing an empty file would be the quietest
possible lie.

## ⭐ Bounding your own command, with `-CommandTimeoutSeconds`

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action Run -Name eph-x -CommandTimeoutSeconds 900 -Command 'make'
```

⛔ **No default, and there will not be one.** `-TimeoutSeconds` bounds the
questions this script asks for itself; a build that runs for an hour is a
legitimate command and a tool that kills it at two minutes is broken. This
exists because the caller sometimes knows a bound the script cannot.

On expiry the child is killed, the distro is terminated so the work in the guest
stops too, and the exit code is **124**, which is what coreutils' `timeout`
reports.

⚠ **It needs the stream log**, so it is refused with `-NoTimestamps` rather than
silently ignored.

---

## ⭐ Passing values in, with `-ScriptArg`

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action New -Image alpine:3.22 -CommandFile probe.sh -ScriptArg "URL=https://@hostaddress:8443/"
```

⭐ **Values are assigned and exported ahead of your script, never substituted
into it.** The thing this replaces is a caller running `sed` over their own file
to inject a URL, where a value carrying a slash, an ampersand or a quote breaks
the edit and the corruption lands in the middle of a script nobody reads again.

| | |
| --- | --- |
| the name | must be a POSIX shell variable name. ⛔ Anything else is refused. |
| the value | single-quoted, so it can contain anything at all |
| `@hostaddress` | replaced with what `-Action HostAddress` prints, resolved without creating a distro |
| ⚠ the first line of your file | is no longer the first line the shell sees. That costs nothing: the body is **sourced**, so a leading shebang was already a comment. |

⛔ **`-ScriptArg` with no command is refused**, not ignored.

### ⛔ More than one pair needs `-ScriptArgFile`

This was documented as repeatable and it never was. Measured under both
PowerShell hosts on 2026-08-30, directly and through the launcher, which splats
the same argument list:

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action New -Image alpine:3.22 -Command 'echo $A $B' -ScriptArg 'A=1' -ScriptArg 'B=2'
```

```text
ERROR: Cannot bind parameter because parameter 'ScriptArg' is specified more
than once.
```

⭐ **A file has no delimiter to be ambiguous about**, which a comma-separated
list would: a VALUE is arbitrary and a URL query string carries commas.

```text
# args.env -- blank lines and # lines are skipped
URL=https://@hostaddress:8443/a,b,c
CFT=https://example.invalid/x.zip?a=1,2
```

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action New -Image alpine:3.22 -CommandFile probe.sh -ScriptArgFile args.env
```

⚠ **The file's bytes get the same repair `-CommandFile` gets**: CRLF becomes LF
in the copy, a byte order mark is dropped, UTF-16 is refused by name. ⛔ The file
on disk is never written to. The pairs from the file are applied first, then
`-ScriptArg` if one was also given.

---

## The file channel, with `-CommandFile`

⭐ **`-CommandFile` is the spelling to reach for**, and it now does the whole
chain a caller used to do by hand. Write the file, pass the path.

| what the file has | what happens |
| --- | --- |
| CRLF line endings | ⭐ turned into LF **in the copy being sent**, with a line saying how many |
| a UTF-8 byte order mark | left out of the copy |
| UTF-16 | ⛔ refused by name. Its bytes carry a NUL after nearly every character and `/bin/sh` stops at the first one, so the command would do nothing and say nothing. |
| a lone carriage return | ⚠ kept. It is a deliberate byte, and turning it into a newline would edit the payload rather than repair it. |

⛔ **The file on disk is never written to.** The distinction, and it is the whole
design, is between **the file**, which is the caller's, and **the copy in
transit**, which is this script's to make correct.

⚠ **`-Verbatim` turns all three off** and warns about what it is about to send.

```text
  * guest.sh has 15 CRLF line ending(s); the copy being sent uses LF.
  * the file on disk was NOT modified. Pass -Verbatim to send its bytes exactly.
```

---

## Attaching a shell, with `Enter`

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action Enter -Name eph-alpine-3.22-a1b2
```

Leave it with `exit` or Ctrl-D. The shell's exit code is the script's.

⛔ **`Enter` sends no command at all**, and that is the whole difference from
`Run`. No `--`, no `/bin/sh -lc`, no base64 transport: `wsl.exe` is handed the
distro and the user and nothing else, so the guest's login shell owns the
terminal. ⚠ Route it through the command channel and it becomes a shell reading
a script, which ignores anything you type.

⛔ **`-TimeoutSeconds` does not apply to it.** A person sitting in a shell is not
a wedged init.

⚠ **It reaches only distros this script created.** The name is prefix-forced
like everywhere else, so `-Action Enter -Name podman-machine-default` asks for
`eph-podman-machine-default` and is refused as unregistered.

| situation | what you get |
| --- | --- |
| the distro is registered | a login shell as `-User`, and its exit code |
| the distro is not registered | exit 1, and the message names `-Action List` and `-Action New` |
| `-Name` omitted | exit 1, `Action Enter requires -Name.` |

## Orphaned rootfs tarballs

`New -Image` writes a rootfs `.tar` into `%LOCALAPPDATA%\wsl-ephemeral\` and
removes it in a `finally`. ⚠ **A `finally` does not run on every hard
interrupt**, so a cancelled run can leave several hundred MiB behind.

`List` reports each one with its size and the time it was last written.
`Purge` removes them, in the same confirmation as the distros and through the
same deletion and the same containment guard.

```powershell
pwsh -NoProfile -File wsl-toolkit.ps1 -Action List
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
pwsh -NoProfile -File wsl-toolkit.ps1 -Action New -Image python:3.13-alpine -OciEnv -Command 'echo $PATH'
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
[`../../../TODO/INDEX.md`](../../../TODO/INDEX.md) and will go. The `-OciEnv` row is a
**settled decision** about what that switch carries, and it stays. The 5.1
quoting row is neither: it is a **limit of the host**, one layer above this
tool, and it stays because a user hits it and needs to be told what to do
instead. ⛔ The intro used to claim every row was an open item, which stopped
being true the moment one of them was closed as a decision.

| limit | what it means for you |
| --- | --- |
| ⚠ `-OciEnv` carries `ENV` and `WORKDIR` only | `USER` and `ENTRYPOINT` are not carried and will not be. See the section on it above for why. |
| ⚠ on Windows PowerShell 5.1, a `-Command` value loses its double quotes when this tool is launched as a child process | 5.1 drops them building the child's argument list, before this script sees anything, so nothing here can recover them. ⭐ Use `-CommandB64`. Not an open item: it is 5.1's argument handling, one layer above this tool. See the command channel section. |
| ⚠ `Run` calls `exit` | correct when the script is invoked, fatal to the host session if it is dot-sourced. ⛔ Invoke it, never dot-source it. |
| ⛔ there is no `-PortForward` | asked for, and refused. Forwarding a port on Windows means `netsh interface portproxy`, which needs an elevated session and leaves a rule on the machine after the tool exits. This tool creates nothing it cannot remove and asks for no elevation. ⭐ `HostAddress` answers the question the port forward was wanted for: bind the host service to that address instead of to loopback. |

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

- [`../../../docs/conventions/shell.md`](../../../docs/conventions/shell.md) section 7,
  for the Windows traps this script is written against: `wsl.exe` emitting
  UTF-16LE, reserved device names, and Git Bash path conversion.
- [`selftest.md`](selftest.md), the test that holds
  the parts of this script deciding what a caller sees.
- [`../../../docs/consumers.md`](../../../docs/consumers.md), for what changing this
  file breaks outside this repository.
- [`../../../docs/HISTORY/wsl-toolkit.md`](../../../docs/HISTORY/wsl-toolkit.md),
  for the defects this script shipped and closed. ⛔ Nothing there is needed to
  use it.
