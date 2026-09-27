# wsl-toolkit

Run an isolated Linux container job on a Windows host. The tool owns one WSL
distribution, runs rootless Podman inside it, and gives a job a COPY of a
workspace rather than a mount. It also makes, runs and removes throwaway WSL
distributions, and boots a FreeBSD guest.

⭐ **It also keeps a base for coding agents.** `base.adapters` installs a
multiplexer and the agents during `base ensure`. `base agent NAME` runs one in
the directory this Windows directory is granted at, and `base herdr` reaches
the multiplexer that watches them. The executable also carries the
general-purpose scripts, so a machine with the binary needs no clone:
`wsl-toolkit shipped list`.

⭐ **This is the only usage page.** The flags live in the CLI, which generates
its own manual, so nothing here can drift from the binary. Read the three
commands under [Finding a flag](#finding-a-flag) first.

⚠ **Windows only.** It drives `wsl.exe`. On any other host the commands refuse
with a message.

---

## Finding a flag

⛔ **This page has no flag reference.** A hand-written reference drifts, and a
flag it does not mention is still a flag the binary accepts.

```powershell
wsl-toolkit man --no-pager     # the complete manual, generated from the CLI
wsl-toolkit COMMAND --help     # one command
wsl-toolkit examples           # the canonical commands, ready to paste
```

`wsl-toolkit man` alone opens a pager for a person. [`wsl-toolkit.1`](./wsl-toolkit.1)
is the same manual as a tracked roff page for `man`. It is generated, and a test
refuses it when it disagrees with the registered commands and flags.

---

## ⭐ Hazards a caller does not have to handle

| the hazard | what the tool does |
| --- | --- |
| **NTFS carries no executable bit**, so a script in a Windows checkout arrives unrunnable and fails `Permission denied` | the copy reads the git index, and a file the index marks `100755` arrives executable. A file in no index that starts with `#!` arrives executable too. The count is reported. Data is never marked executable |
| **A file that grows while the workspace copies** makes an archiver fail and name itself rather than the file | the copy is bounded by the size in the header. The file travels as the prefix that was declared, and the result names it |
| **A file named like a Windows device**, such as `aux.c` or `con.txt`, opens the device through a plain path | the copy opens every file by its whole `\\?\` path, so the file travels with its bytes. `WSL-98` |
| **A payload written on Windows carries CRLF**, and `/bin/sh` reads the carriage return as part of the last word | every spelling of a command has the copy that is sent repaired. The file on disk is never written to |
| **Git Bash rewrites an argument that begins with `/`** into a Windows path before this program starts | a guest path or guest text that MSYS rewrote is refused, naming the rewrite and the two ways past it. `--command-base64` carries a command no shell can reach. `WSL-99` |
| **`--workspace .` resolves against the working directory**, which a sandbox can reset | a relative path resolves against the project configuration when there is one, and the resolved host path is printed. A filesystem root, a home directory or a system directory is refused |
| **A writable `/mnt/c` inside the base** lets a wrong path in a job destroy the real checkout | the Windows drives mount read only, and the base's verification reads them back. `base.automount` takes `rw` or `off`. Explicit `base.mounts` grants need automount and Windows interop both off |

---

## Operating model

- The tool owns the `wsl-toolkit` distribution, the throwaway distributions whose
  disks are in its state directory, and that directory. It manages nothing else,
  and `podman-machine-default` is somebody else's.
- ⛔ **A job gets a copy of its workspace, never a mount.** Getting anything back
  out is a second explicit act, through `/out` and `--artifacts`.
- Progress goes to stderr. The answer goes to stdout, alone. Every command that
  has a structured answer takes `--json`.
- `resources` reports what the tool holds. `gc` prints a plan, and `gc --apply`
  carries it out.

⛔ **Do not call `wsl.exe` with a job payload, and do not write a wrapper for
it.** A command given to `wsl.exe` as an ARGUMENT is expanded before the guest
sees it, and the result is parsed a second time. A backtick in the payload runs,
and the command can still report exit 0. This tool sends a payload on stdin or
as a file.

---

## Container lifecycle

⭐ **The default is `persistent`.** A job keeps its stopped container and its
guest job directory after it ends, so a failed job is something you can go back
and look at.

⚠ **Persistent means retained, not reused.** A later job creates a new
container. Nothing is carried from one job into the next.

```powershell
wsl-toolkit inspect JOB-ID            # what the job left
wsl-toolkit resources --job JOB-ID    # what is still held
wsl-toolkit gc --job JOB-ID --apply   # remove it
```

Pass `--container-lifecycle ephemeral` when a job must leave nothing, which is
normally the right choice in CI. `jobs.container_lifecycle` sets the default for
a machine or a project.

A timeout stops a persistent container, removes an ephemeral one, and exits
124. A cancellation does the same and exits 130.

---

## ⭐ A job from a second process

A job has an id from its first second, and any process on this machine reaches
it by that id. The process that runs a job is its OWNER.

```powershell
$id = wsl-toolkit run --detach --image alpine -c 'make test'
wsl-toolkit logs $id --follow         # both streams as the job writes them, then its exit code
wsl-toolkit wait $id --timeout 30m    # the exit code alone
wsl-toolkit stop $id                  # TERM, then KILL after --grace
wsl-toolkit logs                      # every job and session, with its state
```

| command | what it answers |
| --- | --- |
| `run --detach` | the job's id on stdout, at once. A detached copy of this program owns the job and writes its own progress to `owner.log` in the job's directory. `--json` answers `wsl-toolkit-detached/1` |
| `logs ID --follow` | the job's stdout on stdout and its stderr on stderr, as they are written, from the start or from `--tail N` lines. It exits with the job's own exit code |
| `wait ID` | the exit code `run` would have answered. `--timeout` bounds the wait: it then answers 124, and the job keeps running. `--json` answers `wsl-toolkit-job-end/1` |
| `stop ID` | exit 0 once the job no longer runs, 1 when it could not be stopped, 2 for an id this machine does not know. `--json` answers `wsl-toolkit-stop/1` |
| `logs` | each job and session, newest first: its kind, its state, its exit code and its bytes |
| `inspect ID` | the state, the recorded exit, and what the machine was running |

The state is one of four words:

| state | meaning |
| --- | --- |
| `running` | a process owns the job now |
| `ended` | the owner recorded the end, and `wait` answers from that record |
| `detached` | a base session, which the base runs with no owner on Windows |
| `no owner` | the owner ended without a record. The engine is asked |

⭐ **The owner's lock is the liveness answer.** The owner holds `owner.lock` in
the job's directory for the whole job, and Windows lets it go when the process
ends however it ends. The owner writes `result.json` before it lets the lock go.
So a free lock and no result is an owner that is gone, and `gc` reads such a job
as ended. `WSL-94`

⛔ **A stop is recorded before the engine is asked.** The owner reads the record
before its container starts, when the container announces itself, and after the
container exits. A job still being prepared never starts its container. A job
whose image is still pulling is stopped as its container starts, so its payload
can run for a moment. A stopped job answers exit 130 with `stopped` set.

⚠ **A payload that handles TERM keeps its own exit code.** Only an engine TERM or
KILL, exit 143 or 137, with a recorded stop reads as stopped.

⚠ **A payload that does not handle TERM waits for the KILL.** The payload's shell is
the container's first process, and a first process ignores a signal it has no
handler for. So such a stop takes the whole `--grace`, 10 seconds by default.

⭐ **A job whose owner was killed still ends and still answers.** The engine holds
the job's deadline too, 30 seconds after the owner's own. `wait` and
`logs --follow` read the container's log and exit code from the engine, report
`orphaned`, and record the end so a second reader gets the same answer.

⚠ **Windows can refuse to release the owner.** `run --detach` starts the owner
outside the job object this process runs in. A job object that forbids that
keeps the owner inside it, and the owner then ends when that object closes. The
answer says so, and `--json` carries it as `warning`. A base session does not
depend on it.

⛔ **`--detach` does not cross the helper.** A job through the helper ends with
its client, so `run --detach --via-helper` is refused. `stop --via-helper` stops
a job the helper runs.

---

## ⭐ A command in the base that outlives its client

```powershell
$sid = wsl-toolkit base exec --detach -c './serve.sh'
wsl-toolkit logs $sid --follow
wsl-toolkit stop $sid
```

`base exec --detach` answers with a session id and returns. A supervisor in the
base, started with `setsid`, runs the command in a process group of its own. It
keeps the two streams and the exit code in `~/.wsl-toolkit/sessions/ID` in the
guest, and holds `--timeout` itself. Nothing on Windows has to stay alive.
`WSL-95`

- The command is the one `base exec` would send, `--private-net` and `--root`
  included. `--dir` is where it starts.
- `logs ID --follow`, `wait ID`, `stop ID`, `inspect ID` and `gc` reach it by id.
- Its deadline sends TERM, then KILL after 10 seconds, and answers 124. A stop
  answers 130. `stop --grace` sets the time between the two signals.
- `--json` answers `wsl-toolkit-detached/1` with `kind` `session`. Without
  `--detach`, `--json` is refused, because `base exec` forwards the command's
  own streams.
- ⛔ The helper protocol does not carry `base exec`, detached or not.
- A session whose distribution is no longer registered is gone. `gc` removes its
  record, and `wait` says the guest holds nothing for it.

---

## Devices and input files

```powershell
wsl-toolkit run --image alpine --device /dev/kvm -c 'ls -l /dev/kvm'
wsl-toolkit run --image alpine --input app.toml=.\app.toml --input seed/data.bin=.\data.bin -c 'ls -lR /in'
```

⭐ **`--device HOST[:CONTAINER[:PERMS]]` passes a node in the base into the
container.** The node must be under `/dev`, and the base's account must read and
write it, because a rootless container has the account's own access. A node that
is absent, is not a device, or is not open to the account is refused before the
container starts, and the refusal names the node. `WSL-96`

⭐ **`--input NAME=FILE` puts one file from this machine at `/in/NAME`, read-only
and byte for byte.** It is repeatable, and each job has its own `/in`, so two
inputs never collide in the workspace. NAME is a relative path of plain parts.
The same NAME twice is refused. Through the helper, the inputs of one job are at
most 2 MiB together. `WSL-97`

`run`, `matrix` and the helper take both. `run`, `matrix`, `base exec`,
`bsd run` and `distro` take `--command-base64`.

---

## ⭐ Watching and recording a command

⭐ **With no option below, the command's streams are forwarded unchanged.**

⭐ **`run`, `distro run` and `distro new` all take these.** One relay renders,
records and reports for each, and an ADAPTER behind it answers about the thing
the command runs in. A container and a distribution share a kernel and nothing
else, so each is described in its own words.

| to get | pass |
| --- | --- |
| a timestamp and a stream tag on every line | `--log-profile human`, `ci` (rel and delta, no colour), `forensic` (wall and rel to the microsecond, and delta) or `wall`, or `--timestamp-column rel,delta`. A flag passed beside a profile wins over it |
| a heartbeat while a command is silent | `--tick 30s`, which a rendering profile turns on. It reads the watched thing's state, and says more at each `--tick-escalate` threshold, 2m, 5m and 15m by default |
| progress a command reports | `--progress-prefix TOKEN`. A line `TOKEN 42 unpacking` is consumed, and the heartbeat carries the last one and its age |
| a copy of the rendered lines | `--stream-log FILE`, which is never coloured |
| a record a program reads | `--event-log FILE`, one `wsl-toolkit-event/1` object per line, appended |
| a secret kept out of every copy | `--redact REGEX`. A match becomes `***` before the screen, the sinks, the structured answer or the job's transcript receives the line. Repeat it or pass a comma list; `[,]` matches a literal comma. `WSL-101` |
| a bound on a line | `--max-line-bytes N`, cut at a character boundary, saying how many bytes went |

- A prefix is the timestamp columns, the separator, then a fixed four-character
  tag: `out`, `err`, `tick` or `note`. A `~` in the tag marks a line that had not
  ended: a carriage return redrew it, or it sat unterminated for two seconds and
  was shown early.
- The command's stdout stays on stdout. Its stderr, the heartbeat and the notes go
  to stderr, and a nonzero exit gets a note on what the number can mean.
- ⚠ **A rendered line is not the guest's bytes.** A prefix, a redaction, a line
  bound or a progress token relays the live streams line by line, ended with a
  newline. A sink or a heartbeat alone leaves the live bytes exact. Either way
  the command writes into a pipe, so a program that block-buffers off a terminal
  shows its lines late.
- ⚠ A redaction matches within one line, so a secret split across a
  carriage-return redraw is two lines and matches neither. Under a redaction, the
  answer and the transcript hold the redacted lines, and the byte counts still
  count what the command wrote.
- `distro replay --from FILE` renders a recorded log again, stamped from each
  record's own wall clock. `distro compare --before A --after B` sets two runs
  side by side: elapsed time, time to first output, longest silence, lines,
  bytes and exit code, with `-` where a run measured nothing. It reports and
  does not judge. `--run`, `--before-run` and `--after-run` pick one run of an
  appended log, and `compare` takes each file's last by default.

### ⛔ What the heartbeat can and cannot measure, per feed

⛔ **A feed that does not exist reports ABSENT, and never a zero.** `WSL-59`

| feed | a container, through `run` | a distribution, through `distro` |
| --- | --- | --- |
| state | the engine's own word: `running`, `exited`, or `gone` where the container is not there | WSL's word: `running`, `stopped` or `not registered` |
| output | present. The bytes arrive attached, on the pipe the engine was started on, so no log driver can lose them | present |
| disk | ⛔ **absent.** A container has no disk of its own this side can size | present: the distribution's `ext4.vhdx`, and whether it grew since the last tick |
| resources | ⛔ **absent on a base with no cgroup delegation**, with the reason on the line | ⛔ **absent.** WSL does not account for a distribution separately |
| exit | present, read from the engine as well as from the process | present |

⚠ **`podman stats` answers on such a base, and the answer is not a
measurement.** It reports a processor share over 100 and a memory use of `0B`,
and both parse. So the resource column says absent and names the reason. It is
the same finding `base status --probe` reports as the cgroup capability.

⚠ **The heartbeat asks the engine once per silent tick, and never otherwise.** A
talkative job costs the engine nothing.

⛔ **`matrix` does not take these.** A fleet renders rows, and many relays
interleaving timestamped lines into one terminal cannot be read. A fleet's
per-row record is `--transcripts DIR`.

⛔ **`run --via-helper` refuses them.** The job runs on the helper's machine, so
its container cannot be watched from here. Read it afterwards with
`wsl-toolkit inspect JOB`.

⚠ The helper does not serve `distro replay` or `distro compare` either. A process
that may not call `wsl.exe` makes them through the path its session uses to
approve it.

---

## Dedicated provider bases

A named instance can be a persistent Linux home for a provider CLI. A project
profile can enable systemd, select the `developer` toolset, give a trusted agent
passwordless sudo, disable Windows drive mounts and executable interop, and
grant exactly one checkout at `/workspaces/project`:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base ensure
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base status --probe --json
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base shell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec --dir /workspaces/project -c 'git status --short'
```

⛔ **An instance and its configuration name one distribution.** A configuration
whose `base.name` is not the selected instance's distribution is refused with exit
2, whichever file the search resolved. `wsl-toolkit-muse` belongs to `--instance
muse`, and `wsl-toolkit` to no instance at all. The message names the file, both
distributions, and the `--instance` or `--config` that agrees. `wsl-toolkit
config` still prints which file won and where it looked.

⛔ **A named instance's own configuration comes before the directory a command runs
in.** When `%LOCALAPPDATA%\wsl-toolkit\instances\NAME\config.json` exists,
`--instance NAME` reads it from any directory. So one base serves every project,
and a project's `wsl-toolkit.json` does not change it. The project's file still
applies to the default instance and to a named one with no file of its own.
`--config` comes before both.

⭐ **A process this tool starts resolves the instance once.** `helper serve
--detach` and `run --detach` start a copy of this program with the instance's own
home, and that copy uses it as it is. `WSL-103`

⛔ **A changed `base.automount` is applied, not only reported.** The verification
reads the guest's drives from `/proc/mounts`: every mount at or below a one-letter
directory under `/mnt`. `base status --probe` prints the setting and what the drives
read: `off`, `ro`, `rw`, or `mixed` where a writable mount is beside a read-only
one. `--json` carries the second as `access.automount_guest`. A base whose drives
do not match the setting answers exit 1 and names what disagrees. `base ensure`
provisions it again, which restarts the distribution and stops what runs in it.
`WSL-84`

`base shell` reads the same mounts. `--root` names each drive as read-only or
writable, and `--here` into a base with no drive mounted is refused with exit 2
and `base ensure`.

## ⭐ Reaching herdr, and through it the agents

Two routes for a command, and a terminal UI for a person.

```powershell
wsl-toolkit base herdr -- agent list            # one herdr command, its arguments as arguments
wsl-toolkit base exec -c 'herdr agent list'     # the general channel
```

⛔ **The herdr the base pins, 0.9.0, has no `--machine` prefix.** Its Windows client
exits 2 on the flag, `unknown option: --machine`. A build of herdr's development
branch has it, and refuses a 0.9.0 server with `remote Herdr does not support
machine API forwarding`, so the prefix needs a newer herdr on both sides.
`WSL-90`

⭐ **`base herdr` exists because the second line is a quoted shell string.** A
herdr prompt is prose, and prose carries quotes, dollar signs and backticks.
`base herdr` takes herdr's arguments as arguments and quotes each one once, on
the path `base agent` uses.

⛔ **It is a channel, not a wrapper.** It knows no herdr subcommand, adds no
default and parses no answer. So a herdr release that adds or renames a command
reaches the caller unchanged. It refuses one thing: a command that wants a
terminal, a bare `herdr` or any `attach`, because this path has no screen. It
names `base attach` instead.

⭐ **`base shell --here` marks its own shell, and the profile this tree ships reads
the mark.** [`../../../scripts/common/shell-profile.sh`](../../../scripts/common/shell-profile.sh)
moves an INTERACTIVE shell off a Windows drive to the account's home. That is
right for a shell that inherited the directory and wrong for one that asked for
it. `--here` sets `WSL_TOOLKIT_HERE` and names it in `WSLENV`, which is the only
way a variable crosses into a guest, and a caller's own `WSLENV` is extended.
⚠ `bootstrap.sh` installs the profile, not `base ensure`, so a base that has
never run it has nothing to mark for. `WSL-71`

| the shell, under bash, dash or busybox ash | where it starts |
| --- | --- |
| a NON-interactive login shell on `/mnt/c` | stays there, and writes nothing to stderr. `distro run -c` and `matrix -c` are login shells, so moving one would change every caller's working directory |
| an interactive shell on `/mnt/c` | the account's home, with one line on stderr |
| an interactive shell with `WSL_TOOLKIT_HERE` set | stays on `/mnt/c` |
| an interactive shell in `/workspaces/project` or `/mnt/wsl` | stays. Only one level under the automount root is a Windows drive |
| an interactive shell on `/mnt/c` with `root = /windows/` in `/etc/wsl.conf` | stays, and one under `/windows/c` moves. The root is read, not assumed |

⛔ **A changed `base.passwordless_sudo` is applied too, and read from what `sudo`
answers.** The verification runs `sudo -n true` as the account, which must succeed
under `true` and be refused under `false`. `base status --probe` answers exit 1
naming a base that disagrees. `base ensure` provisions it again, which writes or
removes the rule this tool owns, `/etc/sudoers.d/wsl-toolkit-ACCOUNT`. ⚠ Any
other rule that grants it stays, and the ensure then answers exit 2 with the same
refusal. `WSL-85`

`base exec` is the non-interactive host-to-guest seam. It starts as the
configured account in that account's home unless `--dir` names an absolute guest
path. It sends the command or the `--script` body framed on stdin, with
`/dev/null` as the command's own stdin, forwards output, and returns the guest
exit status. `--root` is an explicit administrative variant. `--detach` runs the
command apart from its client, as [the section above](#-a-command-in-the-base-that-outlives-its-client)
says.

### Grants that change live

```powershell
wsl-toolkit --instance base base grant --source C:\path\to\project --mode rw
wsl-toolkit --instance base base revoke --target /workspaces/project
```

`base grant` mounts one Windows directory at `/workspaces/` and its own name, or at
`--target`, read-only unless `--mode rw`. `base revoke` unmounts one. Each changes
the running base first, reads every grant back as the account, and then writes the
configuration file in effect, so a later restart and `base ensure` mount the same
directories. Nothing restarts, so nothing running in the base stops. `WSL-75`

⛔ **Neither replaces what it would disturb.** A directory or a target already
granted differently is refused with the revoke to run first. A grant a process
stands in stays mounted, with its configuration and fstab entry untouched, and
the revoke answers exit 1 naming `target is busy`.

### The shell `base shell` gives you

⭐ **bash, as a LOGIN shell, where the guest has one.** `wsl.exe -d NAME -u USER`
runs the account's passwd shell, which provisioning leaves at `/bin/sh`. So the
guest is asked in the same call, and answers in three arms, each a login shell:

| the guest has | `base shell` runs |
| --- | --- |
| bash on `PATH` | `bash -l` |
| no bash, and the account has its own shell | that shell, `-l` |
| neither | `/bin/sh -l` |

⚠ **A missing `.bashrc` or profile is not a failure.** `bash -l` with no
`/etc/profile`, `~/.bash_profile`, `~/.bash_login` or `~/.profile` starts normally
and reads nothing.

⛔ **Only `base shell` chooses.** The account's configured shell is still `/bin/sh`,
so a herdr pane, `base exec` and an SSH session all land in `sh`.

⚠ `passwordless_sudo: true` gives the configured account unrestricted guest root.
Guest root can mount Windows paths by hand, so this setting serves a trusted agent
and is not a containment boundary.

### Adapters, and herdr

A base's `adapters` list names the software `base ensure` installs for its agents,
after provisioning and in order. [`adapters/README.md`](adapters/README.md) is the
contract for writing one.

```json
"adapters": [{ "name": "herdr" }]
```

⭐ **`herdr` puts the agents' multiplexer in the base, and a way in from Windows
that listens on nothing.** In the base: herdr 0.9.0 from its release asset,
digest-checked, at `/usr/local/bin/herdr`. Its server runs as the system unit
`wsl-toolkit-herdr.service`, started with the base and never restarted by an
ensure, because a restart ends every pane. The tracked configuration from
[`adapters/herdr/config.toml`](adapters/herdr/config.toml) is written whole each
time. On this machine: a dedicated key at `%USERPROFILE%\.ssh\wsl-toolkit\id_ed25519`,
the base's host key in `known_hosts` beside it, and one marked `Host` block at the
top of `%USERPROFILE%\.ssh\config`. Its `ProxyCommand` starts `sshd -i` through
`wsl.exe` for one connection. `WSL-76`

⛔ **The door accepts that one key, for the base's account, and nothing listens.**
The distribution's own `sshd` units are masked, the key carries `restrict`, and
`base status --probe` fails the adapter over a second key or a listening `sshd`.
Anything that can reach the door could already run `wsl.exe` as this Windows
account.

```powershell
wsl-toolkit --instance base base attach
```

`base attach` prints the commands with the values filled in, and runs none of them:
`herdr --remote wsl-toolkit-base --remote-keybindings server` from Windows, the
`base shell` line for a client inside, and a `base herdr` line for an agent.
`--json` answers the same as a document, with exit 1 and the reason when this
machine's half is missing. [`examples/common/herdr.md`](examples/common/herdr.md) is
the guide for the operator and for an agent.

| what a base with the adapter does | result |
| --- | --- |
| `ssh wsl-toolkit-NAME` through the block | key authentication and a command |
| herdr's Windows client, `herdr --remote` | connects to the base's server through the block `base ensure` wrote |
| `wsl --terminate`, then one `ssh` through the block | the distribution starts, the unit starts the server, and herdr restores its workspaces |
| nothing attached | the base stays running |
| prefix then x, or prefix then shift+x | herdr's own keys close a pane or a tab at once. The tracked file closes nothing |

⭐ **`"channel": "nightly"` makes the herdr adapter follow this repository's herdr
nightlies** instead of the release it pins:

```json
"adapters": [{ "name": "herdr", "channel": "nightly" }]
```

`base ensure` resolves the newest `herdr-nightly-*` prerelease. It installs the
Linux build only when the digest matches the one that nightly's `SHA256SUMS`
publishes, and writes the Windows client of the same build to
`herdr\TAG\herdr.exe` under the instance's state directory. `base attach` then
prints that client, which is the one with `--machine`, and `base remove` takes it
away. `WSL-90`

- ⚠ **A nightly's digest proves transport, not authorship.** `SHA256SUMS` ships
  beside the files it covers. This tool does not check the keyless bundle beside
  each file, which verifies against `herdr-nightly.yml`.
- ⛔ A channel beside a `version` or `sha256` is refused.
- ⚠ `base ensure` never restarts a running server, so a newer build serves nothing
  until the server next starts. `base status --probe` reports it as
  `server-binary-stale`, with the installed `release` and `sha256`.
- With no published nightly, `base ensure` refuses the channel with `this
  repository has published no herdr nightly`.

⭐ **`muse` installs Muse Code for the base's account, and runs Meta's installer only
while its digest is approved.** `base ensure` saves the installer Meta serves, prints
its length and SHA-256, and runs it as the account when that digest is the one the
adapter pins, or the `installer_sha256` the configuration's `muse` entry carries.
⛔ Any other digest stops the ensure with exit 2, keeps the file at
`/var/lib/wsl-toolkit/muse/install.sh`, and prints how to read it. Add its digest
only after reading it: `WSL-77`

```json
"adapters": [{ "name": "herdr" }, { "name": "muse", "installer_sha256": "SHA256" }]
```

A base whose Muse answers a version does not fetch the installer again. ⚠ Muse's own
launcher looks for a newer launcher and a newer Muse, at most hourly.
`/usr/local/bin/muse` puts it on `PATH` for `base exec`, whose shell reads no
profile, and refuses every other account. ⛔ **Signing in is the operator's:** `base
shell`, then `muse login`. `base status --probe` reports the version, where `muse`
resolves, and whether a credential file is present, never what it holds.

⭐ **With the `herdr` adapter beside it, `muse` also reports Muse's lifecycle to
herdr.** It installs a hook at `~/.local/share/wsl-toolkit/herdr-agent-state.sh` and
registers it in `~/.config/muse/settings.json`, the file Muse reads. The events are
`SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `Stop` and
`SessionEnd`, merged into what the file already holds. The hook finds the herdr pane
Muse runs in, reports `idle`, `working` or `blocked` for it, and releases the pane
when the session ends. ⛔ A settings file that is not a JSON object carrying
`schema_version` stops the ensure with the file untouched, because Muse refuses to
start over one. `base status --probe` reports the events the hook is registered for
and runs its self-test. `WSL-88`

| what Muse does with a hook | result |
| --- | --- |
| the hook in `~/.config/muse/settings.json`, `~/.muse/settings.json` and a workspace's `.muse/settings.json` | it runs from the first only. Muse ignores the other two and says nothing |
| the command straight under an event, or inside `{"matcher":"","hooks":[...]}` | it runs only inside the matcher group |
| a settings file with no `schema_version` | Muse exits 1, `malformed settings file ... missing field schema_version` |
| a turn submitted with `herdr agent prompt` | the pane reads `idle`, `working`, `idle`, and `herdr agent wait --until working` returns when the hook reports |
| `/exit`, then `muse resume` and a prompt | the pane reads `idle` and is released. A resumed session sends no `SessionStart`, and the hook adopts the pane again |

⭐ **`pi` and `omp` install the other two agents from npm, and take herdr's own
integration.** Both are npm packages, so there is no installer to approve and no
digest to pin: npm checks a package against the registry's own integrity value.
`version` on either entry pins a release. Each writes a wrapper at
`/usr/local/bin/NAME`, because npm installs into `~/.local/bin` and ⛔ **a herdr pane
does not have that on `PATH`**. Each adapter reads the name back through
`runuser -l`, which is the PATH a pane has, and refuses the install when a login
shell still cannot find it. `WSL-88`, `WSL-89`

⚠ **omp is a Bun program and the adapter installs Bun from the distribution.** npm's
own `bun` package leaves `Bun's postinstall script was not run`, so the adapter takes
the distribution's, which `bootstrap.sh` does not carry.

⛔ **A base carrying both pi and omp is REFUSED when they resolve to one extension
directory.** Both read `PI_CODING_AGENT_DIR`, so one exported for pi redirects omp
onto pi's directory, and herdr then refuses the omp integration. The refusal names
both paths and the variable. `separate_agent_dir` gives omp a directory of its own,
and the adapter writes a wrapper so the separation holds at run time as well as at
install time.

```json
"adapters": [{ "name": "pi" }, { "name": "omp", "separate_agent_dir": true }]
```

#### ⭐ The model and the reasoning effort a new session starts on

An agent adapter takes `model` and `effort`, and writes each where that agent reads
it.

```json
"adapters": [
  { "name": "muse", "model": "muse-spark-1.3-contributor", "effort": "max" },
  { "name": "pi",   "model": "muse-gateway/muse-spark-1.3-contributor", "effort": "max" },
  { "name": "omp",  "model": "muse-spark-1.3-contributor", "effort": "max" }
]
```

⛔ **Not an environment variable, and that is why these fields exist.** An agent
herdr starts inherits the herdr **service's** environment, not a login shell's, so
a default exported in `~/.profile` never reaches it.

| field | what it does |
| --- | --- |
| `model` | the model a session nobody passed a flag to begins on. Empty leaves the agent's own default alone, which a base with no provider serving a named model needs. A value carrying a slash is `provider/id`, for an agent that reaches a model through a named provider |
| `effort` | the reasoning effort that session begins at. **`max` when a configuration names none**, because all three agents take it |

⛔ **`base bootstrap` puts the account's prefix ahead of the wrapper.**
`bootstrap.sh` writes `export PATH="$HOME/.local/bin:$PATH"` into the account's
profile, so a login shell then finds each agent's own launcher first and
`/usr/local/bin/NAME` is not reached. ⛔ Muse then starts with neither the model nor
the effort, because it has no settings key for either. ⭐ Every agent probe reads
the name back on a LOGIN shell and refuses a resolution that is not this tool's
wrapper, so `base status --probe` says so.

⚠ **The effort vocabulary is each agent's own**, read from its own `--help`, and a
word an agent does not take is refused rather than sent to it:

| agent | takes |
| --- | --- |
| `muse` | `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`, `ultra` |
| `pi` | `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max` |
| `omp` | `minimal`, `low`, `medium`, `high`, `xhigh`, `max`, `auto` |

**Where each one is written**, which is also where to change it by hand:

| agent | the door |
| --- | --- |
| `pi` | `~/.pi/agent/settings.json`: `defaultProvider`, `defaultModel`, `defaultThinkingLevel`. Merged, so every other setting is kept |
| `omp` | `omp config set modelRoles` for the `default` role, and `omp config set defaultThinkingLevel`. omp's own command writes them, so nothing here parses YAML |
| `muse` | ⛔ **Muse Code 1.3.0 has no settings key for either**, so `/usr/local/bin/muse` passes `--model` and `--reasoning-effort` instead |

⛔ **The Muse wrapper reads the line before it adds anything**, because of how
Muse Code 1.3.0 parses one:

| what is run | what Muse answers |
| --- | --- |
| `muse --model M config status` | exit 2, `invalid TUI options: unknown argument config`. A root option in front of a subcommand makes the whole line a TUI line |
| `muse --model a --model b` | exit 2, `the argument '--model <MODEL>' cannot be used multiple times`. The last one does not win |
| `muse how do I use exec mode` | exit 2, `unknown argument how`. Muse takes exactly one positional, so a prompt is one argument |

So the wrapper puts them in front for the TUI, after `exec` or `resume` for those
two, and **adds nothing to any other subcommand**: `muse login` reaches Muse exactly
as typed. ⭐ **It reads the subcommand list from `muse --help` at install time**, so
a subcommand Muse adds later is known the next time `base ensure` runs. A flag the
caller passed is never joined by a second.

⛔ **A startup model pi cannot resolve is not an error to pi.** It falls back to its
own built-in default, on a different provider, and says nothing. A gateway serving
a model and pi's catalogue knowing it are two different facts. ⚠ **`max` clamps to
`high` without a `thinkingLevelMap`**, which pi's own documentation states. `base
status --probe` reports `startup_model` and `startup_effort`, and raises a problem
for either case. `WSL-88`

⚠ **All four adapters are driven on the `arch` preset, herdr with systemd, and a
configuration naming any of them on anything else is refused.** Removing herdr from
a configuration takes this machine's half away on the next ensure and leaves the
base's half. Removing any of them leaves what it installed in the base, and `base
recreate` removes that. `base remove` takes this machine's half away and keeps the
key, which is this tool's and not the base's. `WSL_TOOLKIT_SSH_DIR` names a
directory to write this machine's half into instead of `%USERPROFILE%\.ssh`, which
the acceptance runner uses; herdr's own client does not read it.

### Agents from Windows

```powershell
wsl-toolkit --instance base base grant --source C:\path\to\project --mode rw
Set-Location C:\path\to\project
muse --version
```

⭐ **An agent runs in the base, and Windows reaches it from the project it stands
in.** The `muse` adapter writes `muse.exe` into `%USERPROFILE%\bin` for the instance
`base`, and `muse-NAME.exe` for any other instance, as a copy of this executable.
Started under that name it is `wsl-toolkit --instance base base agent muse -- ARGS`.
It finds the grant that covers the working directory, runs `muse ARGS` as the base's
account at the guest path that directory is granted at, through the framed channel
`base exec` uses, and answers Muse's own exit code. Each argument is single-quoted
for the guest's shell, so none is read as shell syntax, and Muse's stdin is
`/dev/null`. `WSL-78`

⛔ **A directory no grant covers is refused** with exit 2 and the `base grant` line
for it. A grant covers its directory and what is beneath it, and never a sibling
whose name starts the same way. ⛔ **Muse's own screen needs a terminal**, so `muse`
alone and `muse resume` answer exit 2 with the herdr route.

⚠ **The launcher is a copy, so an update of this tool leaves it behind.** `base status
--probe` names a launcher that is another build, and `base ensure` rewrites it. It
never writes over, or removes, a file that is not a build of this tool, which it
reads from the file's Go build information without running it. `WSL_TOOLKIT_BIN_DIR`
names a directory to write it into instead, which a throwaway base uses. `base
remove --yes` removes the launcher with the distribution.

[`examples/muse-code/README.md`](examples/muse-code/README.md) is the complete
worked example. [`examples/common/access-profiles.md`](examples/common/access-profiles.md)
carries both the one-checkout profile and the zero-grant profile, including the
boundary they do not claim against guest root or the network.
[`examples/common/herdr.md`](examples/common/herdr.md) is the operator and agent
guide for the same durable session, including how it is reached from Windows.

---

## Container platform

```powershell
wsl-toolkit run --image alpine --platform linux/arm64 -c 'uname -m'
```

Architecture shorthand such as `arm64` is accepted. The base registers QEMU
binfmt handlers for a foreign architecture, and restores a handler that WSL
discarded when its VM stopped.

⛔ **Every job names a platform, including one that asked for nothing.** With no
`--platform`, podman runs whatever variant of the image its local store holds.
After one `linux/arm64` run, a native run of the same image answers `aarch64` on
an x86_64 host, with only a podman warning to say so. The resolved value is on
the result. `WSL-64`

⚠ **A shared kernel carries handlers this tool did not register.** The WSL2
kernel outlives any one distribution, so a foreign-architecture rootfs may RUN
rather than fail, which is the worse outcome. A freshly imported Alpine with no
emulator sees the `qemu-*` handlers, and a riscv64 rootfs boots in it. So the
architecture is named at pull and create time rather than checked afterwards.
`WSL-03`

---

## Relative paths, and the directory you are actually in

With a project configuration in play, a relative `--workspace`, `--script`,
`--artifacts`, `--input` or matrix transcript path resolves against the directory
holding that configuration. A relative `jobs.workspace` always resolves against
its own configuration file.

Without one, a relative path resolves against the current directory, and the
resolved value is printed when the workspace is copied.

```powershell
wsl-toolkit config              # which file was chosen, and every place looked at
wsl-toolkit config --effective  # what the tool will actually use
```

⛔ **A filesystem root, a home directory or a system directory is refused**, not
corrected. Guessing which tree a caller meant is how a tool acts on the wrong one
and reports success.

---

## ⭐ A BSD userland, with no nesting

```powershell
wsl-toolkit bsd status
wsl-toolkit bsd fetch                       # about 635 MB, digest checked before use
wsl-toolkit bsd run -c 'uname -a; sysctl -n hw.model'
```

FreeBSD 15.1-RELEASE runs on this host's OWN hypervisor, through
`qemu-system-x86_64 -accel whpx`, which is the hypervisor WSL2 uses. ⛔ **One level
deep, and no elevation.** The guest sits beside the podman machine rather than
inside it.

⛔ **A BSD binary cannot run on a Linux kernel, and binfmt does not help.**
FreeBSD's own image under Linux podman exits 139, a SIGSEGV on its first syscall.
`binfmt_misc` and `qemu-user` solve a foreign ARCHITECTURE presenting LINUX
syscalls, and nothing presents BSD syscalls on a Linux kernel. A BSD userland needs
a BSD kernel.

⚠ **Every run boots the guest and powers it off, and pays about 23 seconds for it.**
Nothing keeps a guest running between runs. ⚠ Every run is the image's first boot,
because no run writes the image: FreeBSD grows its root to the disk and generates
its SSH host keys before each login. ⚠ The guest is not shown the host's hypervisor
signature. A FreeBSD kernel that sees it waits for a Hyper-V VMBus that QEMU does not
provide, before it mounts root. `WSL-79`, `BSD-02`

⛔ **A run never writes the image.** QEMU boots the guest from a qcow2 overlay that
the run makes beside the image, reads the image through it read-only, and the run
removes the overlay when QEMU exits. ⚠ **Nothing a payload installs survives its
run**, so a payload installs what it needs in the run that uses it. A later run
removes an overlay or a payload disk that a killed run left, once it is an hour old.
`qemu-img`, which ships with QEMU, makes the overlay. `WSL-83`

⭐ **The default is one vCPU.** A fresh-image toolchain install completes with one
processor, where the same guest panics under every two-processor CPU model and
memory size tried. ⚠ **One processor lowers the rate of panics and does not end
it.** `--cpus` overrides the default. `WSL-81`

⭐ **A script reaches the guest as a file, byte for byte.** `-c` and `--script` travel
on a second read-only disk and run from a copy in `/tmp`, so a comment, a blank line
and a command split across lines run as written. Its stdin is `/dev/null`, and the
exit code is the script's own.

⭐ **The guest disk is 12 GiB, which gives a root filesystem of about 10.6 GiB.**
The published image is 6.0 GiB with a 4.8 GiB root, and a toolchain install fills
that. `bsd run` gives the run's overlay a size of `--disk` GiB, 12 by default, then
extends the partition and the filesystem with FreeBSD's own `gpart` and `growfs`
before the payload runs. ⚠ A boot partition, an EFI partition and 1 GiB of swap sit
ahead of root, so the root is smaller than the disk. `bsd status` prints the disk,
and the run's last line prints both sizes. `WSL-72`

⛔ **A `--disk` smaller than the image is refused**, because a shorter disk cuts off
the filesystem inside it. An image an earlier build grew keeps its size until `bsd
fetch --force` goes back to the published image.

⛔ **The guest kernel can panic.** `bsd run` ends a run with exit 2 and the panic line
when QEMU exits after the console shows a panic's two lines, `panic: ` and `cpuid = `
under it, or 60 seconds after them. What the panic wrote goes with the run's
overlay. ⚠ The payload's output is on the same console. So a payload that prints
those two lines itself and ends within the 60 seconds answers with its own exit and
output, and one that runs on past them is ended as a panic. `WSL-82`

⚠ **A panic while the guest powers off leaves the payload's exit standing.** The run
warns and carries it as `shutdown_panic`. It can come before the buffers sync, which
leaves a root filesystem that was not properly dismounted, and that goes with the
overlay too. ⚠ A boot that finds the image's own root not properly dismounted is
named on the run's result as `root_not_dismounted`, with a warning, because every
run boots on it until `bsd fetch --force` restores the published image. When the
archive that command keeps is whole, the restore is a digest check and an
expansion, with no download. `WSL-83`

⛔ **This reaches a BSD SHELL and not a BSD container endpoint.** A long-running
`podman system service` inside the guest panics the guest kernel in `_umtx_op`.
[`pkgforge-dev/docker-bsd`](https://github.com/pkgforge-dev/docker-bsd) carries
the images and that experiment.

⚠ The guest gets no network unless `--network` is passed, and nothing is ever
forwarded inward. This image's root account has an empty password, which makes it
usable with no installer, so the console stays this process's own pipe.

---

## Throwaway distributions

A throwaway distribution is a whole WSL distribution imported from an image or a
rootfs archive, for a job whose subject is the distribution itself: its init, its
`/etc/wsl.conf`, what a login shell sees. Containers in the base answer every
other question.

```powershell
wsl-toolkit distro new --image alpine -c 'cat /etc/os-release' --ephemeral
wsl-toolkit distro new --image debian --name build-box
wsl-toolkit distro run --name build-box -c 'uname -a'
wsl-toolkit distro list
wsl-toolkit distro remove --name build-box --yes
```

- Every name starts with `eph-`, and every `--name` adds the prefix when it is
  left off. A name in the wrong case is refused rather than lowered.
- `distro new --image` needs a host engine, podman or docker, and pulls for this
  host's platform. The import is refused before it starts when the volume lacks
  256 MiB beyond twice the archive. `--probe-timeout` bounds the questions the
  tool asks the new distribution for itself: the smoke probe, whether PID 1 is
  systemd, and the image configuration `--oci-env` carries. A file the tool writes
  into the distribution has its own two-minute bound.
- `--systemd` refuses an image whose PID 1 is not systemd after the restart.
  `--oci-env` writes the image's `ENV` and `WORKDIR` to `/etc/profile.d`.
  `--reuse` runs in the newest distribution built from the same `--image` and
  says so.
- `distro snapshot --name NAME --tag TAG` exports one, and `distro new --tarball
  TAG` imports it again. Without `--force` it never replaces an archive under the
  tag, including one another run wrote while this export ran. ⚠ A snapshot is an
  unencrypted archive of whatever the distribution held, a credential a command
  left behind included.
- A refusal answers 2 and changes nothing. A removal or an export that was
  tried and did not finish answers 1.

⛔ **The tool acts only on a distribution whose disk WSL registered inside this
state directory's `distros` folder.** The prefix proves nothing, so a
distribution another run or another tool made under it is listed as elsewhere, and
`run`, `enter`, `remove` and `snapshot` refuse it. `distro purge` prints a plan,
and `--apply` removes every owned distribution and what failed creations left. A
running distribution, and one another run is still creating, is kept unless
`--include-live` is passed. A snapshot is always kept.

### The command

- ⭐ **The command travels framed on stdin, and its own stdin is `/dev/null`.** A
  command that reads stdin cannot consume the rest of its script, and a prompt
  waiting for input reads end of file. `distro enter` is the interactive path.
- It runs as a login shell, as `--user`, and its exit code is the answer.
  `--timeout` stops the distribution and answers 124, and a cancellation
  answers 130.
- `-c`, `--command-base64` and `--script` are three spellings of one command.
  The copy in transit has CRLF turned into LF and a byte order mark dropped, and
  the tool says when it did; `--verbatim` sends the bytes exactly. UTF-16 and a
  NUL byte are refused. From PowerShell, pass a command carrying quotes as
  `--command-base64`.
- `--env-file`, then `--env`, assign `NAME=VALUE` before the command, single
  quoted and in that order. `@hostaddress` inside a value becomes what
  `wsl-toolkit hostaddress` answers. `--user-env` first prepares a private
  `XDG_RUNTIME_DIR`, a `TMPDIR` and a deduplicated `PATH`, so a value passed with
  `--env` wins over it.
- `--dry-run` refuses what the real run would refuse, a snapshot tag the state
  directory does not hold, a taken `--name` and a missing host engine included.
  It prints the plan from the same selections the real run reads: the command's
  size and digest and the variables' names, never their values. Nothing in WSL
  changes and no log file is opened.

`wsl-toolkit doctor` reports what a throwaway distribution would meet: free space
against the import floor, what the state directory holds, the networking mode
and host address, the host engine, and the clock's resolution. `resources` lists
the owned distributions, leftovers and snapshots. `resources --host-engine` adds
what the host's own engine holds, and prints the commands that would free it
without running them.

## The address a distribution reaches this host at

```powershell
$addr = wsl-toolkit hostaddress
```

The address is the only thing on stdout. It reads `%USERPROFILE%\.wslconfig` and
this host's network adapters and starts nothing. ⛔ **In NAT mode a host service
bound to 127.0.0.1 is not reachable from a distribution**: bind it to the address
printed. Mirrored mode answers 127.0.0.1, and any other mode is refused.

---

## ⭐ The doors, attacked rather than read

```powershell
wsl-toolkit --instance base base doors
wsl-toolkit --instance base base doors --json
```

`base doors` runs a probe inside the base, **as its unprivileged account**, that
TRIES each way out and reports what got through. ⛔ **No row is a setting read
back.** A door is `open` because something got through, `closed` because something
refused it, `unknown` because this guest carries no way to try it, and `info` where
a value is worth printing and nothing was tried.

⛔ **`unknown` is not `closed`.** A probe that reports a door shut because it could
not reach the handle is the failure this command refuses. So a door the
configuration claims to close, and that could not be tried, is a problem.

Exit 0 means every door **this base's own configuration claims to close** is
closed. ⛔ **It does not mean the base reaches nothing**, and the report ends with
the doors that are open and that no setting here closes. Exit 1 is a claim the base
does not keep, and exit 2 is a probe that could not run. Every network route in the
probe has a deadline. `WSL-68`

Four settings make a claim, and only four: `base.automount = off` claims
`fs.windows-drives`, `base.interop = "off"` claims `interop.windows-path`,
`base.passwordless_sudo = false` claims `priv.passwordless-sudo`, and
`base.shared_tmpfs = "off"` claims `fs.mnt-wsl-shared`.

⚠ **`base.interop = "off"` claims only the PATH door**, because the other two are
not this tool's to promise. The `WSLInterop` `binfmt_misc` registration does **not**
follow a distribution's own `[interop] enabled` setting. Both values are seen on one
distribution with one configuration across utility-VM lifetimes. So
`interop.exec-pe` is reported and never claimed. `interop.run-exe` reports `unknown`
on a base with no Windows path reachable, because there is no executable to try.

| on a base with automount off, interop off and passwordless sudo on | `base doors` reports |
| --- | --- |
| closed, each by an attempt | `fs.windows-drives`, `fs.mount-drvfs` (`must be superuser to use mount`), `interop.windows-path`, `net.windows-host-smb`, `net.windows-host-rdp`, `priv.unshare-netns` |
| read-only | `fs.wsl-drivers`, 9p, a write refused |
| ⛔ open, and no setting here closes them | `fs.mnt-wsl-shared`, `net.windows-host-icmp`, `net.internet`, `priv.passwordless-sudo`, `priv.unshare-userns`, `priv.unshare-user-plus-net`, and `interop.exec-pe` whenever the handler is live |

⛔ **This is a report, not a boundary.** Nothing in this tree shows a WSL
distribution to be a security boundary. This command exists so that a sentence
about a base is made of what was tried rather than of settings.

---

### ⭐ A network namespace of the account's own

```powershell
wsl-toolkit --instance base base exec --private-net -c 'curl -sS https://example.com'
```

`--private-net` runs one payload in a network namespace the **account** makes, with
no privilege, through `pasta --config-net`. An `nft` rule **inside** that namespace
refuses the Windows host and the private ranges. `WSL-68`

⭐ **The rule is not in the shared namespace.** Every WSL distribution on a host
shares one network namespace, so a rule written there would change the network of
the podman machine and of every other base.

⛔ **No container can run inside it.** `pasta --config-net` puts the payload in a
user namespace where it is uid 0, so podman takes itself for rootful and chooses
system paths it cannot write. Plain, with `--root` and `--runroot` pointed at the
account's own directories, or with `_CONTAINERS_USERNS_CONFIGURED` set, podman
refuses on `/var/lib/containers` or `/run/libpod`. ⭐ **So this is a flag on one
command and not how every command in the base starts**: the base exists to run
containers as that account.

⛔ **`--private-net` and `--root` are refused together.** Guest root can leave the
namespace it is put in, so the confinement would be a claim rather than a fact.

| | plain | `--private-net` |
| --- | --- | --- |
| the Windows host, ICMP | open | ⭐ **closed** |
| the internet, `1.1.1.1:443` | open | open |
| DNS | answers | answers |
| the namespace | the one every distribution shares | ⭐ its own |
| the shared namespace's own ruleset, after | empty | ⭐ **empty, and unreadable to the account** |
| the payload's exit code | its own | ⭐ its own |
| bytes the wrapper adds to stderr | | ⭐ **none** |

⚠ **`base shell` does not take this flag.** The wrapper delivers its payload as a
script, and an interactive attach through `pasta` is not proved, so it is not
offered.

### ⭐ Closing the shared tmpfs, and what it costs

```json
{ "base": { "automount": "off", "shared_tmpfs": "off" } }
```

⛔ **`/mnt/wsl` is one `tmpfs`, common to every distribution in the WSL2 utility
VM, mounted `drwxrwxrwt`, with uids not namespaced across it.** A base with no
grants writes a file there that another distribution reads. It is the last door a
zero-grant base has, and `base.shared_tmpfs` shuts it. The provisioner installs
`/usr/local/lib/wsl-toolkit/seal-boot.sh`, which WSL runs as root at every start.
`WSL-68`

⚠ **It costs DNS, and that is why it is opt-in and why the order matters.**
`/etc/resolv.conf` is a symlink INTO that directory, so a base that unmounts it and
does nothing else cannot resolve a name. So, in this order and no other:

1. WSL is told to stop generating the symlink.
2. The provisioner writes a real `/etc/resolv.conf` from the shared file while it
   is still reachable.
3. The boot script refreshes that file at every start, before it unmounts. A
   refresh that reads short leaves the working file alone.

⛔ **`shared_tmpfs: "off"` requires `automount: "off"`** and is refused otherwise.
Closing the tmpfs while the Windows drives are mounted costs the resolver and seals
nothing, because the drives are a wider channel than the tmpfs.

⭐ **The verifier reads the result, not the intention.** It refuses a base whose
`/mnt/wsl` is still mounted, whose `/etc/resolv.conf` is still a symlink or carries
no nameserver, or which cannot resolve a name. ⚠ The last one matters because a
`command=` value that wsl.conf's parser rejects does not run and says nothing. So
the boot line is a bare path with no shell in it.

| a base with no grants and `shared_tmpfs: "off"` | result |
| --- | --- |
| the account, after a restart | `/mnt/wsl` not mounted, a write refused, and `getent hosts` answers |
| `base doors` | exit 0, `fs.mnt-wsl-shared closed`, `no tmpfs mounted at /mnt/wsl`, claimed by the setting |
| the doors that stay open | `net.windows-host-icmp`, `net.internet`, `priv.unshare-userns`, `priv.unshare-user-plus-net` |
| ⛔ the unmount taken out of the boot script by hand | `base doors` exit 1 naming the claim, and `base status --probe` exit 1, `usable false`, naming the setting |

## The safety model

Removal is constrained, and every destructive path goes through the same gate.

1. The base is a distribution named `wsl-toolkit` or `wsl-toolkit-INSTANCE`.
   Removing one that was ever usable also demands the identity marker its
   provisioning wrote inside the guest.
2. A throwaway distribution is this tool's only when its registered disk is
   inside the state directory's `distros` folder.
3. ⛔ **A container runtime's own distribution is refused by name**:
   `podman-machine-default`, `docker-desktop`, `docker-desktop-data`,
   `rancher-desktop`, `rancher-desktop-data`.
4. ⭐ **Every removal of state goes through one deletion, and the containment
   check runs INSIDE it**, not beside each caller.
5. ⛔ **A container is stopped only when it carries this tool's label**, and a
   session's directory is removed only at the one layout a session has.

⛔ **A delete that did not happen is not reported as one.** `wsl --unregister`
releases a disk asynchronously, so the state is read back after removal, and a
path that is still there exits non-zero naming it.

### ⛔ What a zero-grant base does NOT seal

⭐ **Run `base doors` and read its answer rather than this list**, which is what
that command is for. These are the doors it finds open on a base built with
`automount off`, `interop off` and no grant. `WSL-68` holds each measurement.

| door | what it means |
| --- | --- |
| ⭐ `/mnt/wsl` | one world-writable `tmpfs` shared by every distribution in the utility VM, with uids not namespaced across it. ⭐ **`base.shared_tmpfs: "off"` closes it at every start**, and the section above says what that costs. ⚠ It is off by default, so a base that does not ask for it still shares the directory |
| ⛔ the internet | `1.1.1.1:443` connects from a base with no grants. Nothing in this tool closes it |
| ⛔ the Windows host | `445` and `3389` are refused and **ICMP answers**, so the host is reachable. The address is `hostaddress`'s answer |
| ⭐ a private network namespace | `unshare -n` is refused and `unshare -Un` SUCCEEDS, so the account can make one with no privilege. ⚠ `pasta --config-net` alone still reaches the internet and the Windows host. ⭐ **An `nft` rule INSIDE that namespace refuses the host and the private ranges while the internet and DNS stay up**, which is what `base exec --private-net` runs |
| ⛔ a PE handler this tool cannot remove | the `WSLInterop` `binfmt_misc` registration does not follow `[interop] enabled`. `base doors` reports `interop.exec-pe` and never claims it |
| ⚠ `/dev/kvm`, `/dev/dxg`, `/dev/vsock` | all three are `crw-rw-rw-` in a zero-grant base. **None is attacked by this probe**, so what an unprivileged account reaches through them is unmeasured |
| ⚠ passwordless sudo, where it is on | guest root can mount a Windows path the configuration never granted. It is authority, not containment, and `base doors` reports it open |

⛔ **So the shape this tool builds is called zero grants and never a sandbox or a
security boundary**, and no page here says otherwise.

### ⚠ Four things about WSL that this tool cannot protect you from

- ⛔ **Every distribution on this machine shares one writable directory**,
  `/mnt/wsl`, as the table above says. So a zero-grant base is not a sandbox.
- ⛔ **`wsl --shutdown` is machine-wide.** It stops every distribution on the
  machine, the podman machine included. This tool never runs it.
- ⚠ **A distribution's lifetime is not the kernel's.** `--terminate` and
  `--unregister` restart or remove the userspace, and the WSL2 kernel keeps
  running. So `binfmt_misc` registrations, loaded modules and pinned superblocks
  survive. Only `wsl --shutdown` gives a fresh kernel, and a condition reproduced
  without it reads stale state.
- ⚠ **A distribution's `binfmt_misc` is not its own.** Anything that decides
  whether interop is available by reading that file gets a confident wrong answer.
  Run a Windows executable instead, which is what `base doors` does.

---

## Known limits

⛔ **A limit hidden is a defect filed against the user later.**

| limit | kind | what it means |
| --- | --- | --- |
| the base enforces no per-container resource bounds | host | rootless podman under `init` with no cgroup delegation makes no cgroup per container. `--memory` is accepted and not applied, and `podman stats` reads `0B`. ⭐ `base status` reports this. A job bounded on this base is not bounded. `WSL-60` |
| `podman logs` needs a log driver that stores | host | the engine's default driver on the base is `journald`, and nothing serves a journal there. This tool names `k8s-file` on each job it starts, so `podman logs` answers for them. The first line podman shows is this tool's own start marker, and `logs --follow` strips it. A caller driving podman directly names a driver too |
| `bsd run` boots and powers off a guest per call, about 23 seconds each | host | nothing keeps a guest running between runs |
| nothing a payload installs survives its `bsd run` | decision | a run writes to an overlay it removes, so no panic can damage the shared image |
| no BSD container endpoint | open | a long-running podman service panics the FreeBSD guest kernel. [`../../../TODO/bsd.md`](../../../TODO/bsd.md) tracks it |
| `--oci-env` carries `ENV` and `WORKDIR` only | decision | `USER` and `ENTRYPOINT` are not carried: WSL fixes the login account per call, and a login shell has no entrypoint |
| a throwaway distribution's command gets no stdin | decision | its stdin is `/dev/null`, because a pipe that carries the script cannot also carry input. `distro enter` is interactive |
| there is no port forwarding | decision | forwarding a port on Windows needs an elevated session and leaves a rule behind. `hostaddress` answers the question it was wanted for |
| an agent behind a launcher may be invisible to herdr | host | herdr reads a pane's foreground process, and `base agent` and a `NAME.exe` launcher are wrappers. herdr's own remedy is `HERDR_AGENT=<agent>` set on the wrapper command **on the herdr side**. This is herdr's documentation, not a measurement here |
| a base carrying both pi and omp can be configured so herdr refuses one | decision | the omp adapter refuses first, naming both paths and the variable |
| a detached owner inside a job object that forbids a breakaway | host | the owner ends when that object closes. The answer warns, and a base session does not depend on it |
| ⛔ a base is not a sandbox | host | `/mnt/wsl` closes with `base.shared_tmpfs: "off"`. The internet answers, the Windows host answers ICMP, and the account can make a private network namespace with no privilege. Run `base doors` for this host's own list. `WSL-68` |

---

## Requirements

| thing | needed for |
| --- | --- |
| Windows 10 2004+ or Windows 11, with WSL2 | everything |
| a container engine on the host | `base ensure`, once, to export the base rootfs, and `distro new --image` |
| `qemu-system-x86_64` and the Windows Hypervisor Platform | `bsd` only |
| `xz` | `bsd fetch` only |

---

## Examples

```powershell
wsl-toolkit ready --smoke                          # the whole route, including one container
wsl-toolkit run --image alpine -c 'uname -a'       # one job, container retained
wsl-toolkit gc --job JOB-ID --apply                # and removed again
```

```powershell
wsl-toolkit run --image debian --workspace . --artifacts .\out -c 'make && cp build/x /out/'
wsl-toolkit matrix --images all --container-lifecycle ephemeral -c 'cc --version'
wsl-toolkit helper serve --detach                  # when the calling process cannot reach WSL
```

`wsl-toolkit examples` prints these from the binary, so they cannot go stale.

---

## Related

- [`README.md`](./README.md) is how the executable is built and tested.
- [`docs/consumers.md`](../../../docs/consumers.md) is who fetches from here and
  what breaks them.
- [`docs/HISTORY/wsl-toolkit.md`](../../../docs/HISTORY/wsl-toolkit.md) is closed
  defects. ⛔ Nothing there is read to do work.
