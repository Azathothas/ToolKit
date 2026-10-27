---
name: wsl-toolkit
description: "Build and operate a named Linux base on Windows with wsl-toolkit. A base is one WSL distribution the tool owns, with rootless Podman, and it can have systemd and a toolset. Windows directories are granted to it one at a time. Use when the user asks to make, inspect, repair, grant a directory to, or attack a wsl-toolkit base. Use it to run a job or a base command that outlives the call, then follow, wait on or stop it. Use it when the user names wsl-toolkit, a wsl-toolkit instance, or the default base wsl-toolkit-jobs. Do not use for ordinary WSL work that does not involve this tool."
---

# wsl-toolkit

`wsl-toolkit` is a single Windows executable. It owns one WSL distribution per
**instance**, plus the `eph-` distributions that `distro` makes. It does nothing to
any other distribution.

⛔ **Never act on a WSL distribution this tool did not create.** `wsl --unregister`
deletes the distribution it names, with no question. ⛔ **Only `wsl --shutdown`
reaches every distribution on the machine**, including the user's own work. ⭐ Where
the user has said nobody else uses WSL on that machine, run it yourself when a step
needs it, and say what it stopped. Otherwise it is the user's step. Never hand back a
step the user has already given you. Use the tool's own commands.

---

## 1. Get the tool, and keep it current

⛔ **Do not assume a version.** Ask:

```powershell
wsl-toolkit --version
```

If the command is not found, the executable is not on `PATH`. The user downloads
`wsl-toolkit-windows-amd64.exe`, or `wsl-toolkit-windows-arm64.exe` on ARM, from the
newest `wsl-toolkit-v*` release of `Azathothas/ToolKit`. They rename it to
`wsl-toolkit.exe` and put it in a directory on `PATH`, usually `%USERPROFILE%\bin`.

To move it to the newest release:

```powershell
wsl-toolkit selfupdate
```

⚠ **A configuration field the tool does not know is refused**, as
`FILE is not readable as wsl-toolkit-config/1: json: unknown field "X"`. A field in
the wrong place gives the same message. Compare the field with `config --effective`.
If its name and place are right, run `selfupdate`.

---

## 2. Read the manual before using a flag

⛔ **Never guess a flag, and never trust a flag list written on a page.** The manual
is generated from the commands the executable really has, so it is current for your
copy:

```powershell
wsl-toolkit man --no-pager
```

For one command:

```powershell
wsl-toolkit base --help
wsl-toolkit base exec --help
```

⚠ **Asking for help is not a failure.** Every `--help` answers 0. `wsl-toolkit base` or
`wsl-toolkit helper` with no subcommand answers 2, because nothing was asked.

⭐ **Read the refusal first.** Most name the file, the field and the fix.

---

## 3. Two rules that hold on every call

⛔ **FROM GIT BASH, EXPORT TWO VARIABLES FIRST.** MSYS rewrites a guest path such as
`--dir /workspaces/proj` into `C:/Program Files/Git/workspaces/proj` before the tool
starts, and the tool refuses the rewrite. `export MSYS_NO_PATHCONV=1
MSYS2_ARG_CONV_EXCL='*'` stops it, and the tool then reads a host path such as
`"$PWD"` in MSYS's spelling, `--home`, `WSL_TOOLKIT_HOME` and `WSL_TOOLKIT_CACHE`
included. Every example here is PowerShell.

⛔ **READ AN EXIT CODE FROM THE PROCESS, NOT THROUGH A PIPE.** Piped into another
program, such as `findstr`, `$LASTEXITCODE` is that program's code, not the tool's.
Read it on the line after the command, with nothing piped.

⭐ **A job's code and the tool's are apart.** A command that runs a job answers the
job's own code. Where the tool could not run the job, or lost how it ended, it answers
125. Those commands are:

- `run`, `wait`, `logs ID --follow` and `bsd run`
- `base exec`, `base shell`, `base agent`, `base herdr` and `base bootstrap`
- `distro new` given a command, `distro run` and `distro enter`

Every other command answers 2 for a refusal, `stop` for an unknown id included.
Under `--json`, `job_started` says whether the job started, because a job can exit
125 itself.

---

## 4. The shape of a base

⭐ **Every example here acts on the default base, so none passes `--instance`.** The
default base is the agents' base too. Its canonical name is `wsl-toolkit-jobs`. ⚠ An
older default base may still be named `wsl-toolkit`, and it keeps working under that
name. `base status` says which name it has, and how to move. ⛔ Moving deletes the
base's home, so it is the user's choice.

`--instance NAME` acts on another base, the distribution `wsl-toolkit-NAME`, with its
own state. `doctor --json` lists every distribution in `host.wsl_distros`. ⛔ Where the
instance does not exist, `base ensure` builds a new one, so use the name the user has.
⛔ **`base` and `jobs` are reserved instance names.** A new instance under either is
refused, and an existing one stays reachable.
⚠ **A new instance named `wsl-NAME` or `wsl-toolkit-NAME` is made as `NAME`.** Its
distribution is `wsl-toolkit-NAME`, and stderr warns once. An existing instance keeps
its name. `--literal-instance` keeps the name as given.

One base is one JSON configuration. When the instance's own `config.json` exists in
its state directory, `--instance NAME` reads it from any directory. ⚠ The nearest
`wsl-toolkit.json` from the working directory up still lends it every `jobs` key that
file writes. Otherwise the search starts at `wsl-toolkit.json` in the working
directory and above it. `config` names the file that won, and each key it lent:

```powershell
wsl-toolkit config
```

That prints the file in effect, the places it searched, and what it resolves to.
Start there whenever something is unexpected.

| field under `base` | what it means |
| --- | --- |
| `image` | the OCI image the distribution is built from. An adapter is refused on any image but the `arch` preset |
| `user` | the unprivileged Linux account everything runs as |
| `systemd` | whether systemd runs as the first process. It is off by default, and the `herdr` adapter needs it on |
| `passwordless_sudo` | whether the account can elevate with no password. It is off by default. With it on, the account can mount a Windows path nobody granted |
| `automount` / `interop` | whether Windows drives are mounted and Windows executables run. Grants need both `off`. Other doors stay open: run `base doors` |
| `shared_tmpfs` | `off` unmounts the shared `/mnt/wsl`. It needs `automount` `off` too |
| `toolset` | what the base installs as root. It is `none` by default |
| `mounts` | the grants. One Windows directory each |
| `adapters` | the software `base ensure` installs, in order. Each installs with `pacman -Syu --needed`, which upgrades every package in the base |

| field under `jobs` | what it means |
| --- | --- |
| `exclude` | globs a workspace copy leaves out, for every job of the project |
| `gitignore` | `true` leaves out of a workspace copy what git ignores |

To edit it, write the effective configuration to the state directory's own `config.json`,
change that file, and validate it:

```powershell
wsl-toolkit config --write
wsl-toolkit config validate
```

⚠ **Where `config` names a `wsl-toolkit.json` instead, that file wins in its
directory and below.** Change that one, because the state directory's copy then
changes nothing.

---

## 5. The commands that do the work

```powershell
wsl-toolkit base ensure
```

Builds, provisions, installs every adapter, and prints the state. A second run
reconciles rather than rebuilding, so run it after a configuration change.
⛔ **It can restart the base, which stops what runs in it.** A base that does not
verify, such as one whose provisioned setting changed, is provisioned again and
restarted. Where that fails, the base is kept, and the refusal names `base recreate`.
Ask the user before you run it on a base that holds work.

⚠ Building a base needs a container engine on Windows: podman, docker or `wslc`,
tried in that order.

⛔ **Only the state directory that owns the base changes it.** `base status` names
the owner. Another one is refused a rebuild, a re-provision, `--repair` and
`base remove`. So is a shell whose `%LOCALAPPDATA%` Windows redirects into an app's
copy. From either, `base ensure` installs no adapter, says so in one line, and
exits 0. Run those from PowerShell or a terminal outside the app.

```powershell
wsl-toolkit base status --probe
```

⛔ **`registered` is not `usable`.** Without `--probe` this reads the configuration.
With it, the tool runs a container and reads every adapter back from the machine.
Exit 0 means healthy; exit 1 lists each problem, and each problem names its fix.

⛔ **A grant needs `automount` and `interop` both `off`.** On a base with either one
on, `base grant` is refused.

```powershell
wsl-toolkit base grant --source . --mode rw
```

Grants **one** Windows directory, live, under `/workspaces`. Use `--mode ro` when the
work must not change it. ⛔ `base agent NAME` refuses a directory no grant covers, with
exit 125 and the `base grant` line that would cover it. **That refusal is the design
working. Do not widen a grant to make a step pass.** Report it instead.

To take one away:

```powershell
wsl-toolkit base revoke --target /workspaces/NAME
```

⛔ **`revoke` takes `--target` alone**, the path the base sees it at, not the Windows
directory. The mount goes at once and an empty directory is left behind, reaching
nothing.

```powershell
wsl-toolkit base exec -c 'COMMAND'
wsl-toolkit base exec --script LOCAL-FILE.sh
wsl-toolkit base shell
```

`base exec` runs one command as the account and returns its exit code, or 125 where
it could not run the command. `--script`
sends a local file's bytes, which avoids every quoting problem, and `--script -` reads
them from stdin. `--env-file FILE` exports its `NAME=VALUE` lines to the command, and
no shell reads them. `base shell` opens an interactive login shell. It is for a
person, and an agent uses `base exec`.

⚠ **`base exec` runs `/bin/sh`, which reads no profile.** To see what a login shell
sees, ask one, where ACCOUNT is `base.user`:

```powershell
wsl-toolkit base exec --root -c "runuser -l ACCOUNT -c 'COMMAND'"
```

A base with no `runuser`, as on Alpine, takes `su` instead, its options before the
account:

```powershell
wsl-toolkit base exec --root -c "su -l -c 'COMMAND' ACCOUNT"
```

```powershell
wsl-toolkit base doors
```

Attacks the base from the unprivileged account and reports every door it finds open.
Exit 0 means every door the configuration claims to close is closed, and other doors
can still be open. Exit 1 names a claim the base does not keep, and exit 2 is a probe
that could not run. ⛔ **Run this rather than believing any page about what a base
cannot reach.** It is refused through the helper with exit 2, so run it through the
session's WSL approval path.

```powershell
wsl-toolkit base reclaim
```

Makes the base's disk sparse, then trims what the guest freed. ⚠ WSL makes a disk
sparse only while its VM is stopped. While any distribution runs, `base reclaim`
exits 1 and names `wsl --shutdown`. Who runs it is the rule at the top.

---

## 6. Work that outlives the call

⭐ **A job, or a command in the base, can run apart from the client that started it**,
and any later call reaches it by its id:

```powershell
$id = wsl-toolkit run --detach --image alpine --workspace . -c 'make test'
wsl-toolkit logs $id --follow
wsl-toolkit wait $id --timeout 30m
wsl-toolkit stop $id
```

`--workspace` puts a copy of the directory in the container at `/work`. Nothing comes
back unless the job writes it to `/out` and the run passes `--artifacts DIR`.

`run --detach` answers with the job's id at once. `logs ID --follow` writes both
streams as they are written and exits with the job's own code. `wait ID` answers that
code alone. When `wait --timeout` runs out, it answers 124 and the job keeps running.
`stop ID` ends the job. `wait` then answers 130, unless the job traps TERM and keeps
its own code. `logs ID --follow` and `wait ID` answer 125 where they could not read the
job, such as an id this instance does not hold. `stop ID` answers 2 for that id.

```powershell
$sid = wsl-toolkit base exec --detach --timeout 0 -c './serve.sh'
```

`base exec --detach` runs the command in the base with no client attached, and
answers with a session id that `logs`, `wait`, `stop` and `inspect` take. It holds
`--timeout` itself, 30 minutes by default, so a server takes `--timeout 0`.
⛔ **Do not wrap a long command in `nohup` or `setsid` inside `base exec -c`**: nothing
then keeps its output or its exit code, and nothing can stop it by id.
A session the base restarted under is gone without an exit: `wait` answers 1 at
once, and `stop` signals nothing.

A job runs a nested podman rootless, and adds no capability. A job whose nested
podman found no `/dev/fuse` is told this shape after its exit:

```powershell
wsl-toolkit run --image quay.io/podman/stable --user podman --device /dev/fuse -c 'podman run --rm docker.io/library/alpine echo nested'
```

`wsl-toolkit logs` lists every job and session of that instance with
its state: `running`, `ended`, `detached` or `no owner`. With no `--instance`, it
lists the default base's jobs. A job gets a device node with `--device /dev/kvm`, and
a file from Windows at `/in/NAME` with `--input NAME=FILE`. A script to run by its
shebang goes as `--input-exec NAME=FILE`, which arrives with LF line endings and mode
0755. `base exec` takes both, and places them where `WSL_TOOLKIT_IN` names.

`--gitignore` leaves out of the workspace copy what git ignores, and `--exclude GLOB`
leaves out what the glob matches. They add to `jobs.gitignore` and `jobs.exclude`, and
`--gitignore=false` turns the project's rule off for one job.

`--platform linux/arm64` runs a job for another architecture through the base's QEMU
binfmt handlers. Every `base ensure` registers them and records which are the base's
own. On a base with systemd, `wsl-toolkit-binfmt.service` registers them again when
another distribution empties the table. ⚠ Where `base status --probe --json` shows
`binfmt.recorded` false, run `base ensure` once.

`run --exclusive NAME` refuses a second job with the same NAME, and
`--exclusive-wait 30m` queues it instead. ⚠ The lock is in the state directory, so a
caller that Windows redirects holds the name only against callers on its own side.

`gc --workspace . --apply` removes this project's jobs and no other project's. It
skips a job that recorded no workspace, and `gc --job ID --apply` removes that one.
It is refused through the helper.

---

## 7. When it is broken

⛔ **`base recreate` and `base remove --yes` delete the base's home, images and
credentials.** Run either only when the user asks for it, whatever a refusal or a
remediation names.

| what you see | what to do |
| --- | --- |
| a configuration field the tool does not know | compare it with `config --effective`. If its name and place are right, run `wsl-toolkit selfupdate` |
| an adapter reports a problem | read the line. It names the file and the command |
| the base is registered and nothing works | run `base status --probe`. Each problem it lists names its fix. ⛔ `base ensure` can restart the base, and `--repair` restarts it over another configuration's grants too, so ask the user first |
| a refusal naming both `wsl-toolkit` and `wsl-toolkit-jobs` as registered | the tool does not guess which one is the default base. Ask the user, then set `base.name` to that one in the configuration |
| Windows restarted WSL under it | a job, `base exec` and each start of the base clear the engine state another boot or an earlier start left. An owner's `base ensure` installs that check, and installing it restarts nothing. Where podman still refuses, `base ensure --repair` clears it. ⛔ **`wsl --shutdown` breaks EVERY instance holding an engine.** The remediation names the other instances, unprobed. Run `base status --probe` with each one's `--instance` before any repair |
| a Windows launcher says it is another build | `base ensure` rewrites it |
| `no such job` for a job another shell started | the other shell may name another `--instance`, or hold another state directory. `config` and `doctor` name a second copy. Call the tool from one kind of shell, or set `WSL_TOOLKIT_HOME` outside `%LOCALAPPDATA%` |
| a helper refusal reading `and this build speaks` | a helper from another release is still running. Run `wsl-toolkit helper stop`, then `wsl-toolkit helper serve --detach`, each with the `--instance` that helper serves |
| a line saying `pull ... and was given up` | ⭐ **silence, and a slow link can be silent too.** A pull prints nothing while one layer downloads, so its limit is that layer's time at 256 KiB/s. On a slower link, set `WSL_TOOLKIT_PULL_STALL` to a duration such as `45m` where you call the tool, and run it again. The line says where else to look |
| a refusal reading `the wsl-toolkit identity marker could not be read`, or a remediation `identity-unread` | a damaged disk, or one a virus scanner or a backup holds. Nothing was changed. `ready` names `base recreate` for it. ⛔ **Ask the user first**, as the line above the table says |
| an advisory `built-from-local-copy` | the pull failed, and the base was built from the image the host engine already held. It may be older than the registry's. The advisory names its digest, through the helper too. Once the registry answers, `base recreate` builds from its image, when the user asks for it |
| a line saying `export ... and was given up` | the export writes to this machine and touches no registry. Read the free space and the engine machine. `WSL_TOOLKIT_PULL_STALL` does not change its limit |
| an advisory `host-space-low` | the volume the base's disk grows into has under 6 GiB free. Jobs still run, and a full volume fails the guest's writes. Free space there. `wsl-toolkit gc` lists what the tool holds |

⭐ **The tool keeps a complete transcript of every job it runs**, until `gc` removes
the job:

```powershell
wsl-toolkit logs JOB-ID
```

⚠ **An ephemeral job that ran attached keeps no transcript.**
`--container-lifecycle ephemeral` removes its record once its answer is written, and
`logs ID` then finds no job. Read the answer the run printed.

---

## 8. What this tool does not promise

⛔ **A base with no grants is not a sandbox.** `base doors` lists what stays open.
Read its answer before telling anyone a base is sealed.

⚠ **A setting and a flag narrow it further, and both cost something.** The setting
`base.shared_tmpfs: "off"` closes the shared `/mnt/wsl`, and it needs
`base.automount: "off"`. It also takes the resolver WSL shares, so the base keeps its
own copy, refreshed at each start. The flag `base exec --private-net` gives one
command its own network namespace, with the Windows host refused. It is refused
beside `--root`, and ⛔ **no container runs inside that namespace**.

---

## 9. Where the detail lives

`base doors` is the measurement. The manual is the authority for commands and flags,
and it ships with the executable you are running:

```powershell
wsl-toolkit man --no-pager
```
