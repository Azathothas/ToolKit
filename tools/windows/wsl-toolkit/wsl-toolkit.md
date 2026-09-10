# wsl-toolkit

One executable for Linux work on a Windows host. It surveys the machine, owns a
WSL distribution with a container engine in it, runs a command in one container
or in a set of them, and removes what it made.

This page stands alone. An agent that has read only this file can use the tool
correctly, from a release or from a clone, without opening the source.

⚠ **Windows only for anything that touches WSL.** `doctor`, `images`, `config`
and `version` answer on any host; every other command refuses by name off
Windows.

---

## Start here

```powershell
wsl-toolkit doctor
wsl-toolkit base ensure
wsl-toolkit run --image alpine -c 'uname -a'
```

⛔ **Do not call `wsl.exe` directly and do not write another wrapper.** A payload
handed to `wsl.exe` as an argument is expanded before the guest sees it and the
result is parsed again. Measured 2026-09-09 on Windows 11 Pro 26200: a backtick
in an argument was executed and the command still exited 0. This tool sends every
payload on stdin or as a file, and never as an argument.

## Commands

| command | what it does |
| --- | --- |
| `doctor` | what this host is and what is installed, resolved past every shim. Creates nothing. |
| `script` | run the embedded `wsl-toolkit.ps1`, forwarding every argument unchanged |
| `base` | the one WSL distribution this tool owns, which hosts a rootless engine |
| `images` | the container catalog |
| `run` | one command in one container, with a copy of a workspace and no host mount |
| `matrix` | one command across a set of images |
| `resources` | what this tool holds, and what the machine holds that is not its |
| `gc` | remove what this tool made. Reports first; `--apply` acts. |
| `logs` | the complete output a job produced, past whatever its answer kept |
| `helper` | the opt-in local helper, for a caller that cannot reach `wsl.exe` |
| `config` | where the configuration is and what it says |
| `ready` | one answer to whether this agent can run isolated Linux jobs here, and the one command that fixes it if not |
| `selfupdate` | move this executable to a published release, verifying it against `SHA256SUMS` first |
| `artifacts` | retrieve a copy a failed transfer retained. It never re-runs a job |
| `examples` | the canonical command patterns, in the binary rather than only here |
| `version` | the product version, read from the embedded script |

Global flags: `--instance NAME` (also `WSL_TOOLKIT_INSTANCE`), `--home DIR`
(also `WSL_TOOLKIT_HOME`), `--config FILE`, `--quiet`, and `--json` on every
command with a structured answer.

⭐ **Progress goes to stderr. stdout carries the answer alone**, so a caller can
assign it:

```powershell
$addr = wsl-toolkit script -Action HostAddress 2>$null
```

## Exit codes

| code | meaning |
| --- | --- |
| 0 | it ran and it agreed |
| 1 | it ran and it disagreed: a job failed, a fleet row failed, or requested output did not arrive |
| 2 | it could not run: bad usage, a missing base, a refusal |
| 124 | a deadline was reached, as coreutils' `timeout` reports it |
| the container's own code | from `run`, forwarded verbatim |
| the script's own code | from `script`, forwarded verbatim |

---

## `doctor`

```powershell
wsl-toolkit doctor
wsl-toolkit doctor --json
wsl-toolkit doctor --group runtime
wsl-toolkit doctor --fast    # find the tools without asking each one its version
wsl-toolkit doctor --net     # probe outbound HTTPS
```

⛔ **A name on `PATH` is not an executable.** `scripts/doctor/doctor.sh` resolves
a name through a POSIX shell's `PATH` and `doctor.ps1` through Windows'; both are
right and they disagree. This one opens the file and reports what would run,
with the name that was found underneath it:

```text
  yes rustc            1.98.0                          <profile>\.cargo\bin\rustup.exe
      via <profile>\.cargo\bin\rustc.exe
```

⚠ **The version is read by running the path that was FOUND, not the one it
resolves to.** `rustc.exe` resolves to `rustup.exe`, which decides what to do
from the name it was invoked under.

| `kind` | what it means for a caller |
| --- | --- |
| `executable` | hand it to process creation and it runs |
| `scoop-target` | the name on `PATH` is a shim; the resolved path is the real file |
| `command-script` | ⚠ a `.cmd`. Process creation cannot run one; it needs `cmd.exe /d /s /c`. |
| `powershell-script` | ⚠ a `.ps1`. Process creation throws "not a valid application for this OS platform". |
| `app-execution-alias` | ⚠ a zero-byte Windows stub. It may open the Store rather than run anything. |
| `unresolved-link` | the file runs and its canonical path could not be read |

The WSL section answers what a sandboxed caller needs:

```text
WSL
  installed      true
  callable here  NO, this process is refused
  base distro    wsl-toolkit (registered)
  local helper   127.0.0.1:51234
```

⛔ **"installed" and "callable here" are different facts.** A restricted process
gets `E_ACCESSDENIED` on a machine where WSL works. When that happens the report
names both ways forward: the session's own approval path, or the helper.

---

## `base`

```powershell
wsl-toolkit base status --probe
wsl-toolkit base ensure
wsl-toolkit base presets
wsl-toolkit base ensure --preset alpine --save
wsl-toolkit base recreate
wsl-toolkit base remove --yes
wsl-toolkit base shell
wsl-toolkit base shell --root
```

One WSL distribution called `wsl-toolkit`, holding a rootless podman and one
unprivileged account called `toolkit`. Containers run inside it, so nothing a job
does reaches `podman-machine-default` or the images somebody else put there.

⛔ **This tool acts on `wsl-toolkit` and `wsl-toolkit-<instance>`, and nothing
else.** A configuration naming anything outside that prefix is refused when it is
READ, so a mistyped name cannot become a real distribution. `wsl --shutdown` is
machine-wide and appears nowhere in this tool.

⭐ **Ownership is a proof the guest carries, not a name in a file.** The
distribution holds `/etc/wsl-toolkit-identity.json`, written as root at the end
of provisioning and readable by nothing a job runs as. `base remove` reads it
before unregistering anything, so a distribution this tool did not build cannot
be adopted by editing a configuration file.

⚠ **A base built before that marker existed is ADOPTED, once, and says so.**
`base ensure` recognises an unmarked distribution whose name matches the prefix
and whose record this tool wrote, stamps it, and says which file it wrote.
Anything else is refused with the command that rebuilds it.

⛔ **The guest is the authority and the record on this machine is not.**
`base status --probe` reads the marker and reports `built_from` from it; a
configuration naming a different image is reported as a disagreement and left
alone, and `base ensure` exits 1 rather than rebuilding on its own. ⚠ It USED
to rewrite its own record to match the configuration whenever a health probe
passed, so a distribution built from Arch was relabelled Alpine because an Alpine
CONTAINER ran inside it. The probe proves the engine works and identifies
nothing.

⭐ **A shell starts in the guest account's home**, `/home/toolkit`, and
`--here` is how a caller asks for the Windows directory it was launched from.
⚠ It USED to start there by default: `wsl.exe -d NAME -u USER` inherits the
caller's working directory, mounted WRITABLE under `/mnt/c`, so a root shell
opened from a checkout could write to that checkout. The manual said root was
"inside the distribution" and "not on this machine", which read as the opposite.

⛔ **A Windows drive WSL has mounted is writable from that shell whatever
directory it starts in**, and `--root` now names the drives rather than implying
an isolation the shell does not have.

⭐ **`base ensure` is the recovery path as well as the create path.** A
registered distribution that cannot run a container is re-provisioned in place;
one that will not provision is removed and rebuilt. `base recreate` starts from
nothing. The base is meant to be wrecked.

⚠ **`registered` is not `usable`.** A rootless engine reports a complete
configuration and then fails at the first run. `--probe` runs a real container
and reads back a marker the command could not have echoed; without it, `status`
says the check was not made rather than implying it passed.

| flag | meaning |
| --- | --- |
| `--probe` | run a real container to decide whether the base is usable |
| `--preset` | build from this preset id or fully qualified reference |
| `--save` | make that choice the stored default |
| `--root` | `base shell` attaches as root instead of the `toolkit` account. It is root INSIDE the distribution and not on this machine, and the command prints which Windows drives are mounted and writable from it. |
| `--here` | `base shell` starts in this Windows directory, mounted under `/mnt`, instead of the guest account's home |
| `--yes` | `base remove` does not ask first |
| `--json` | a structured answer |
| `--via-helper` | force the local helper route |

### Presets

A preset id or any fully qualified reference works in the same place.

| preset | libc | measured on Windows 11 Pro 26200, 2026-09-09 |
| --- | --- | --- |
| ⭐ `arch` (default) | glibc | 31s, 940 MiB disk, podman 6.1.1 |
| `alpine` | musl | 28s, 204 MiB disk, podman 5.8.6 |
| `debian` | glibc | 37s, 556 MiB disk, podman 5.4.2 |
| `fedora` | glibc | not measured |

Each figure is `base ensure` to a rootless container returning a marker, with the
image already in the host engine's cache. Arch adds about 38s on a cold pull of
its 540.8 MiB rootfs.

⭐ **The default is glibc, not the fastest.** The base is where a toolchain gets
installed when somebody wants one outside a container. ⚠ It has no bearing on
what a CONTAINER runs, which brings its own userland.

⚠ **Switching preset rebuilds**, because a distribution's rootfs cannot be
changed underneath it. `--save` also makes the choice the stored default; without
it the configuration is unchanged and `base status` reports the disagreement.

---

## `run`

```powershell
wsl-toolkit run --image alpine -c 'apk add gcc && gcc --version'
wsl-toolkit run --image debian --workspace . --artifacts .\out -c 'make && cp build/x /out/'
wsl-toolkit run --image arch --script .\build.sh --timeout 45m
```

⛔ **No host directory is ever mounted into a container.** A job gets a COPY of
its workspace, and what it does to that copy cannot reach this machine.

| the container sees | what it is |
| --- | --- |
| `/work` | a copy of `--workspace`, inside the distribution's own filesystem |
| `/out` | empty. Whatever is left there is fetched to `--artifacts`. |
| `/job.sh` | the command, read-only |

⛔ **Every archive entry is validated before it is written**, in both
directions, against the grammar of the machine it is delivered to rather than the
one checking. Refused by name: an absolute path, a name containing `..`, a link
leaving the tree, a Windows reserved device name, any of `< > : " | ? *`, a
control character, a trailing dot or space, and a second entry whose name differs
from an earlier one only in case.

⭐ **A LINK OUT AND A LINK IN ARE HANDLED DIFFERENTLY, ON PURPOSE.** A caller
CHOOSES what it puts in `/out`, so a refusal there is actionable; a caller often
does not control every entry under a workspace it points at, so failing a job
over one stray link would make the workspace feature unusable.

| the entry | what happens |
| --- | --- |
| an artifact link whose target leaves the tree | ⛔ REFUSED, and the job fails |
| an artifact link that stays inside the tree | recorded as a `.link.txt`, never recreated |
| a workspace link whose target leaves the tree | left out, COUNTED, and named in `workspace_omission` |
| a Windows junction in a workspace | the same: left out, counted and named |

⚠ **Both halves used to be silent.** An escaping artifact link was converted to
an inert `escape.link.txt` and the job exited 0; a junction was skipped by the
walker and the job exited 0 having never seen it. Neither was a traversal hole
and nothing was ever overwritten. The defect was that a caller could not tell its
input or its deliverables were incomplete, which is the same class as a
truncation nobody is told about.

⚠ **`workspace_omitted` is an exact count and `workspace_omission` is the first
twenty**, so a tree full of links does not put thousands of rows in an answer.

⚠ **The colon and the case rule are about data loss, not escape.**
`out.txt:stream` is valid NTFS syntax naming an alternate data stream, so it
creates an empty `out.txt` and puts the payload where nothing reads it;
`Result` and `result` are one file on NTFS, so the second silently replaces the
first. Both are refused with the two names in the message, because a container
knows what it called its files and a tool that renamed them would produce output
a caller cannot predict.

| flag | meaning |
| --- | --- |
| `--image` | a catalog id, or any fully qualified reference |
| `-c` | the command. ⛔ Mutually exclusive with `--script`. |
| `--script` | a file whose bytes are the command. CRLF becomes LF in the copy; the file on disk is never written to. UTF-16 is refused. |
| `--workspace` | a directory on this machine, copied in as `/work` |
| `--artifacts` | a directory on this machine to receive `/out` |
| `--exclude` | a glob to leave out of the copy. Repeatable. |
| `--env` | `NAME=VALUE`. Repeatable. |
| `--timeout` | default 30m. On expiry the container is killed and the code is 124. ⭐ It bounds THE CALLER, not only the container: see below. |
| `--tick` | emit a heartbeat for each running job at this interval. 0 is off; anything under 1s is raised to it |
| `--no-network` | run with no network at all |
| `--user` | what the container runs as: a name, a uid, or `uid:gid`. Empty is the image's default, which for most is root. ⚠ Naming one makes podman re-own the mounts for it, which costs a walk of the copy. |
| `--max-bytes` `--max-entries` | ceilings for a workspace and an artifact set. Default 1 GiB and 200000. |
| `--max-output` | how many bytes of the command's output the ANSWER keeps. Default 8 MiB for stdout and 2 MiB for stderr; 0 uses the default. It does NOT bound the transcript. |
| `--ensure-base` | build the base first when it is missing. On by default. |
| `--via-helper` | force the local helper route |

⚠ **A workspace or an artifact set over a ceiling is refused, never truncated.**

### What a caller sees while a job runs

⭐ **The container's bytes reach this process's own streams as they are
written**, not when the job ends, and that holds on the helper route as well: the
helper answers with a stream of events rather than one object at the end. So a
build that prints for ten minutes prints for ten minutes here.

⛔ **Under `--json` there is no live stdout, and that is deliberate.** This
process's stdout carries the structured answer, so a container writing there too
would produce a document nothing can parse. Live stderr still flows.

⛔ **Nothing is silently shortened.** The complete output of every job is
written to a transcript beside the job's own state, whatever its size. What
`--max-output` bounds is the copy the ANSWER carries, and when that copy is
shorter than the output the result says so:

| field | meaning |
| --- | --- |
| `stdout_bytes` `stderr_bytes` | what the command WROTE, whatever any copy kept |
| `stdout_truncated` `stderr_truncated` | the copy in this answer is shorter than that |
| `transcript` | the directory holding the complete text. `wsl-toolkit logs ID` writes it. |

⛔ **These are the payload's own bytes and nothing else.** The wrapper that
announces the container writes a framing line, and that line is removed without
leaving a byte behind: a command writing nothing to stderr answers
`"stderr_bytes": 0`. ⚠ It answered 1 up to and including `wsl-toolkit-v1.3.0`,
because the framing's own newline was written back whether or not there was a
line for it to end.

### The heartbeat

```powershell
wsl-toolkit run --tick 5s --image alpine -c 'sleep 60'
wsl-toolkit matrix --tick 5s --images all -c 'make'
```

⭐ **A job that is running used to tell a caller almost nothing.** `resources`
reports what exists when asked, and there was no periodic signal, so an agent
watching a long fleet could not tell work in progress from a hang.

`--tick` emits one event per RUNNING job at that interval, on both routes:

| field | why it is there |
| --- | --- |
| `id` `label` `container` | which job, and the engine's name for it so a caller can go and look |
| `elapsed_ms` | how long it has been running |
| `remaining_ms` | what is left of its deadline. ⚠ ABSENT rather than zero where there is no deadline: zero means out of time |
| `stdout_bytes` `stderr_bytes` | ⭐ the heartbeat's real content. Rising counts are work; the same counts for a minute is a stall, and the two are indistinguishable without them |

⛔ **It is not a progress bar and must not become one.** It is a
machine-readable event with a timestamp; whatever renders it belongs to the
caller. Under `--json` it goes to stderr like everything else this program says.

⛔ **It is not a poll loop against the engine.** Everything a tick carries is
already in this process, so the cost is bounded by rows times duration over
interval and does not grow with what the containers are doing.

⚠ **Nothing ticks unless it is asked to**, and a tick can never arrive after the
result: the ticker is stopped and waited for before the answer is built, so a
caller reading events in order never sees a job report that it finished and then
that it is still running.

### What a deadline actually bounds

⭐ **`--timeout` bounds the caller's waiting, not only the container's running.**
An agent that sets one to keep a pipeline moving gets its answer back inside the
deadline plus a bounded grace, and the `duration_ns` it reads is the interval it
actually waited.

| what the grace covers | ceiling |
| --- | --- |
| `wsl.exe` returning after its process tree is killed | about 2s measured; the process wait is bounded at 1s past the kill |
| stopping the container and removing the job's guest directory | 10s, shared between the two rather than one ceiling each |

Measured on the development host on 2026-09-10: a `--timeout 2s` over a payload
sleeping 8 seconds returned in **5.3s**, and over one sleeping 60 seconds in
**5.2s**. ⭐ **The number does not grow with the payload**, which is the property
that makes a deadline worth setting.

⚠ **It answered in about 15 seconds up to and including `wsl-toolkit-v1.3.0`,
and reported about 4.** Two things caused it and both are fixed: the reported
duration stopped at the point the container died rather than at the point the
caller got an answer, and `podman rm -f` was waiting out podman's own ten second
SIGTERM grace for a payload that had already been told its time was up.

### What the answer says happened

⛔ **`exit` is the CONTAINER's own code and `effective_exit` is what this
process returned.** They differ whenever the container succeeded and something
around it did not, and a caller that wants "did this work" reads the second:

| field | meaning |
| --- | --- |
| `exit` | the payload's own status, forwarded verbatim |
| `effective_exit` | what the process exited with: the payload's code, 124 for a deadline, 2 for a container that never started, 1 for output that did not arrive |
| `artifacts` | entries DELIVERED to the directory named |
| `artifacts_attempted` | entries the guest offered, delivered or not |
| `retained_kind` `retained` | where a copy that could not be delivered still is: `guest` with a path inside the distribution, or `helper` with the artifact set id the helper is still holding |

⚠ **`artifacts` used to mean entries encountered**, so a refused transfer
answered with a positive count beside its own failure. It means delivered from
`wsl-toolkit-v2.0.0`, and `artifacts_attempted` carries the old number under a
name that says what it is.

### When output cannot be delivered

⛔ **A job whose requested artifacts do not arrive does not exit 0.** The
command's own exit code wins when it is nonzero; a command that succeeded and
whose output could not be fetched exits 1, and a fleet counts that row as failed.

⭐ **The copy that could not be delivered is kept, and the answer names it.**
On the direct route the guest job directory stays, as `guest_dir` and as
`retained`; on the helper route the helper does not acknowledge the artifact
set, so it is still there and `retained` carries its id. `gc` collects either
later under its own age policy, so the output is recoverable rather than
destroyed on the way out.

⛔ **An image that could not be acquired is `unreached`, not a job that
failed.** The payload is entered through a wrapper that announces itself from
inside the container, so a pull that failed, a container that could not be
created and a `--user` that does not exist are all distinguishable from a program
that itself exited 125. The classification is never inferred from an exit
status.

---

## `matrix`

```powershell
wsl-toolkit matrix --images all --script .\probe.sh --transcripts .\tr
wsl-toolkit matrix --images libc:musl -c 'ldd --version'
wsl-toolkit matrix --images kind:legacy --workspace . -c 'make'
```

Takes every `run` flag, plus `--images`, `--parallel` and `--transcripts`.

A selector is a catalog id, `libc:musl`, `libc:glibc`, `kind:musl`,
`kind:glibc`, `kind:niche`, `kind:legacy`, or `all`; several are comma
separated. ⛔ **A selector that matches nothing is a refusal**, because a fleet
that ran nothing reads exactly like a fleet where everything agreed.

The workspace is sent once and copied per row inside the distribution. Each row
gets its own copy.

```text
  alpine    ok            1.927s  docker.io/library/alpine:latest
  arch      ok           26.039s  ghcr.io/pkgforge-dev/archlinux:latest
  gentoo    ok           48.633s  docker.io/gentoo/stage3:latest

  12 ran, 0 failed, 0 unreached, 0 timed out, in 52s
```

⛔ **Three counts, because "the image could not be pulled" and "the subject is
broken" need different next moves.**

| exit | meaning |
| --- | --- |
| 0 | every row ran and every row agreed |
| 1 | at least one row disagreed, or could not be reached |
| 2 | ⛔ nothing ran |

⚠ **Four rows at a time by default.** Every row is a container inside one WSL
distribution sharing one utility VM's memory. `--parallel` past four is a
decision about that machine.

`--transcripts DIR` writes one file per row.

---

## The catalog

```powershell
wsl-toolkit images
wsl-toolkit images --select libc:musl
```

⛔ **Every reference is fully qualified.** An engine resolves an unqualified name
through its own alias table, so the same string is two different images on two
machines. The base distribution sets `short-name-mode = "enforcing"`, so a short
name typed inside it is refused too.

| id | libc | kind | reference |
| --- | --- | --- | --- |
| `alpine` | musl | musl | `docker.io/library/alpine:latest` |
| `void-musl` | musl | musl | `ghcr.io/void-linux/void-musl:latest` |
| `chimera` | musl | musl | `docker.io/chimeralinux/chimera:latest` |
| `arch` | glibc | glibc | `ghcr.io/pkgforge-dev/archlinux:latest` |
| `debian` | glibc | glibc | `docker.io/library/debian:latest` |
| `fedora` | glibc | glibc | `registry.fedoraproject.org/fedora:latest` |
| `gentoo` | glibc | niche | `docker.io/gentoo/stage3:latest` |
| `wolfi` | glibc | niche | `cgr.dev/chainguard/wolfi-base:latest` |
| `photon` | glibc | niche | `docker.io/library/photon:latest` |
| `rocky8` | glibc | legacy | `quay.io/rockylinux/rockylinux:8` |
| `ubuntu2204` | glibc | legacy | `docker.io/library/ubuntu:22.04` |
| `debian12` | glibc | legacy | `docker.io/library/debian:12-slim` |

⚠ **Where a maintainer publishes somewhere other than Docker Hub, that is the
row**, because Docker Hub's anonymous pull limit is the likeliest cause of a red
row that has nothing to do with the subject.

⭐ All twelve ran on Windows 11 Pro 26200 on 2026-09-09: 12 ran, 0 failed, 0
unreached, 52.1s with cold pulls, artifacts returned from every row.

### Changing it

```powershell
wsl-toolkit config
wsl-toolkit config --write
```

`--write` puts the effective configuration on disk to edit, carrying the current
catalog. `images` in it REPLACES the built-in list; `matrix` names the ids a
fleet uses when the caller names none. ⛔ **A file this build cannot read is a
refusal, not a fall back to the defaults.**

---

## `resources` and `gc`

```powershell
wsl-toolkit resources
wsl-toolkit gc
wsl-toolkit gc --apply --older-than 24h
wsl-toolkit gc --apply --images       # also prune images in the base
wsl-toolkit gc --apply --include-live # also remove work that is running now
```

⛔ **The report is in two parts and says on every line which it is:** what this
tool made, and what the machine holds that is not its. Only the first is ever
removed.

⛔ **`gc` reports by default; `--apply` acts.** It finishes both loops before it
reports, so one item it cannot remove does not stop it removing the rest.

⚠ **It matches on a label this tool stamped, never on a name pattern.**

⭐ **A killed run still leaves a record.** Every resource is written to
`ledger.jsonl` before it exists, so an interrupted run leaves an open record with
no close, and that is what `gc` looks for.

⛔ **`gc` does not touch work that is running.** A container the engine
reports as running, and anything belonging to a job whose record is still open,
is spared and listed under `Kept` with the reason. `--include-live` is the only
way to remove it, and it has to be typed.

⛔ **`--older-than` applies to containers as well as directories.** A
container's age is its job directory's, because both are named for the same job
and made in the same moment. One whose directory is already gone has no age to
read and is not running, which is what abandoned means, so it is collected.

⭐ **The plan and the apply consume one filtered set.** What a dry run
lists is exactly what `--apply` removes; they were computed separately, and only
one of them applied the age.

⚠ **The helper's own directories are part of what is held.** An uploaded
workspace is released by the job that named it and an artifact set when the
client says it arrived; anything nobody acknowledged appears in `resources` and
is collected by age like everything else.

---

## `logs`

```powershell
wsl-toolkit logs                      # what is still here, newest first
wsl-toolkit logs 8fa39b8bb07a0154     # that job's output
wsl-toolkit logs 8fa39b8bb07a0154 --both --tail 50
```

| flag | meaning |
| --- | --- |
| `--stderr` | write the error stream instead of the output stream |
| `--both` | write both, the output first |
| `--tail N` | only the last N lines |
| `--json` | list as structured data |

⭐ **It is the other half of the truncation rule.** An answer carries a
bounded copy and says when it is bounded; this is how the complete text is read
back. `--tail` reads a window from the end rather than the whole file, so a
transcript larger than memory costs one small read.

⛔ **A job id is the FIRST argument or there is none.** `logs --tail 5`
asks for the last five lines of nothing, not for a job called `5`.

⭐ **It answers on both routes.** A job run through the helper has its
transcript written by the helper, under the helper's own state directory; the
client writes the same bytes as they arrive, so the path on the result is one
this machine can open whether or not the two share a state directory.

⚠ **A transcript is removed by `gc` under the same age policy as
everything else**, so an id that ran long enough ago will not be here.

⭐ **`run` tells you the id.** A job that kept its output ends with the
command that reads it back, so the id does not have to be found by listing first.
A run whose output was cut at the capture limit says that instead, with the path,
because saying the same thing twice in two shapes reads as two facts.

⛔ **An empty machine and an unreadable one are different answers.** A
machine that has not run a job yet exits 0 and says so. A `jobs` path that cannot
be READ is a refusal at exit 2 naming the path, because "no transcripts on this
machine yet" is a true-sounding answer to a question this tool could not
answer.

---

## `script`

```powershell
wsl-toolkit script -Action List
wsl-toolkit script -Action New -Image docker.io/library/alpine:latest -Ephemeral -Command 'echo hi'
wsl-toolkit script -Action Doctor
```

The embedded `wsl-toolkit.ps1`, with every argument forwarded unchanged and its
exit code returned.
[`../../../scripts/windows/wsl-toolkit/wsl-toolkit.md`](../../../scripts/windows/wsl-toolkit/wsl-toolkit.md)
is its page.

⭐ **Throwaway distributions and the owned base are different tools.** `script
-Action New` builds a distribution from any image and destroys it; `base`
maintains one that persists and holds an engine. Neither can remove the other's.

---

## The local helper

```powershell
wsl-toolkit helper serve --detach
wsl-toolkit helper status
wsl-toolkit helper stop
```

For a process that is refused when it calls `wsl.exe`. Start it once through the
approval path the session offers; it holds the access and later commands hand it
job data.

⭐ **Nothing has to know it is there.** A command that can reach `wsl.exe` uses
it directly. One that cannot finds the helper and says so once. `--via-helper`
forces the route.

⛔ **The route is chosen by ASKING WSL, not by finding `wsl.exe` on the
path.** A resolvable executable says nothing about whether this process may talk
to it, and deciding on the path meant a sandboxed caller took the direct route,
met `E_ACCESSDENIED`, and was then advised to start the helper that was already
listening. The decision now runs one read-only enumeration, which is the same
call the work would have failed on, and the answer is cached for the life of the
process so a fleet pays for it once.

⚠ **A probe that fails for any other reason does not route.** A timeout, a
machine with no distributions and a refusal are three different answers, and only
the third is what a helper fixes. Anything else takes the direct path, which
reports the real reason itself.

⛔ **It is a lifetime boundary, not a privilege boundary.** It listens on
loopback, runs as whoever started it, and its token lives in that user's own
state directory. It buys one approval instead of one per command; it adds no
permission anybody did not already have.

⛔ **What the protocol will not carry:**

| | |
| --- | --- |
| a host path | a workspace is uploaded as an archive and artifacts are downloaded as one, so the helper never opens a file the caller named |
| a host command | there is no endpoint that runs a program on this machine |
| a raw engine option | podman's flags are not reachable, so a mount cannot be asked for |
| a WSL lifecycle call | it builds and repairs the base and can unregister nothing. `base remove` and `base shell` are not on the protocol. |

⭐ **What it does carry is every job flag the direct path has**, `--user` and
`--include-live` included. A flag one route honours and the other drops is a job
that ran as somebody else with nothing said; the acceptance runner asks both
routes the same question and compares the answers.

⭐ **`run` and `matrix` answer with a stream, not with one object at the
end.** Each line is a JSON event: a chunk of the command's stdout or stderr, a
fleet row that has just finished, or the final result. So the restricted route
shows a build as it happens, which is the route with no alternative to it.

⚠ **A stream that ends without a final event is an error, not an empty
answer.** That is a helper that died mid-job, and a client that returned what
had arrived so far would report a killed run as a finished one.

⚠ The token is compared in constant time and the endpoint file is written `0600`.
On Windows those mode bits are not an access control list; what protects the file
is that `%LOCALAPPDATA%` is that user's own directory.

---

## `ready`

```powershell
wsl-toolkit ready
wsl-toolkit ready --json
wsl-toolkit ready --smoke --ensure
```

⭐ **One answer to "can this agent run isolated Linux jobs here, and if not,
what is the one command that fixes it".** Everything it reports is something
`doctor`, `helper status`, `base status --probe`, `run` and `logs` already
answered; what was missing was a command that runs them in order and reports one
verdict.

| flag | meaning |
| --- | --- |
| `--smoke` | run one tiny container proving the whole path end to end |
| `--ensure` | build or repair the base if it is not usable |
| `--no-update` | do not ask whether a newer release exists |
| `--json` | one stable object |

⛔ **`--ensure` is OFF by default, and that is deliberate.** A readiness check
that builds a distribution has changed the thing it was asked to measure, and a
first run that silently spent four minutes pulling a rootfs is not an answer.

⛔ **It never stops at the first problem.** An agent that learns three things are
wrong in one pass fixes them in one pass, so every part runs and `problems`
carries all of them.

⛔ **When it cannot proceed it names ONE exact command and does not pretend it
can self-elevate.** A restricted process that cannot start a helper says so and
prints the approval command, which is the whole reason the helper exists.
`remediation[0]` is the command to run now.

### What `--smoke` proves

Six facts, one container, and each is a separate line so a partial answer says
which one was missing:

| fact | why it is in the list |
| --- | --- |
| a uid | the container ran at all |
| a kernel release | it reached the WSL kernel rather than something emulated |
| `/work` is writable | a job can build |
| an artifact came back from `/out` | a job can deliver |
| the transcript reads back | `wsl-toolkit logs` will find it |
| no host mount | ⛔ ASKED, not inferred from what was left off the command line |

⚠ **The last one is a question rather than an assumption.** A header asserting a
property the command line does not enforce is a claim that can simply be false,
so the container is asked whether `/mnt/c` exists rather than being trusted not
to have one.

### The verdict

| `verdict` | what it means |
| --- | --- |
| `ready` | a job can run here now |
| `no-route` | this process cannot call `wsl.exe` and no helper is listening |
| `no-base` | the route works and the distribution is not usable |
| `not-ready` | something else is wrong, and `problems` says what |

Exit 0 when ready, 1 otherwise.

---

## `selfupdate`

```powershell
wsl-toolkit selfupdate --check
wsl-toolkit selfupdate
wsl-toolkit selfupdate --tag wsl-toolkit-v2.0.0
```

⭐ **It resolves the newest release, verifies the download against the published
`SHA256SUMS`, and replaces this executable.** A consumer that fetched this binary
had no other way off it than knowing the URL scheme, resolving the latest tag,
picking the asset for its architecture and checking a digest by hand.

| flag | meaning |
| --- | --- |
| `--check` | report whether a newer release exists and change nothing |
| `--tag` | a specific `wsl-toolkit-vX.Y.Z`, so a caller can move DOWN as well as up |
| `--json` | a structured answer |

⛔ **A digest that does not match is a refusal and the running executable is
untouched.** Nothing is replaced before the replacement has been verified.

⛔ **It reads no credential.** The release assets are public, so the fetch is
anonymous and a `GITHUB_TOKEN` in the environment is deliberately not read.

⛔ **Nothing updates without being asked.** `ready` reports; this acts. An
auto-updater changes the binary under a running pipeline, which is the one thing
an agent cannot recover from.

⚠ **The previous executable is renamed aside, not deleted.** Windows will not
let a running program delete itself, so the old copy becomes
`.wsl-toolkit-previous-<version>.exe` beside the new one and the NEXT run removes
it.

`--check` exits 0 when this executable is current, 1 when a newer release exists,
and 2 when the question could not be asked. ⛔ A network that could not be
reached is not "up to date": answering 0 there would make a pipeline that gates
on this stop updating the day GitHub was unreachable.

### What `ready` says about it

`ready` carries an `update` section, and it is a section rather than a separate
command because an agent asking "am I ready" wants the same answer.

⛔ **A network that cannot be reached is NOT a machine that is not ready.** The
check is bounded at eight seconds and its failure is reported as
`update.checked: false` with a reason; a tool that refused to run isolated Linux
jobs because GitHub was down would have invented a dependency it does not have.

⚠ **`update.checked` is always present**, so a caller reading
`update.available` can tell "there is no newer release" from "nobody looked".

---

## The six commands that make an answer actionable

⭐ **`ready` can tell an agent it is not ready.** These are the commands behind
the six things it might have to say, so the answer names a next step rather than
a subsystem. None of them needs a concept this tool did not already have.

### `helper status` says whose configuration it is running

`helper status --json` carries `config_fingerprint`, the fingerprint of the
configuration the helper started with, beside `client_config_fingerprint` and
`config_matches_client`.

⚠ **A difference is information rather than a fault.** Every request has carried
the client's own configuration since `wsl-toolkit-v2.0.0`, so a helper that
started with another one still acts on yours; what the fingerprint answers is
"which configuration did this process come up with", which was previously
invisible.

### `config validate` and `config --effective`

```powershell
wsl-toolkit config validate
wsl-toolkit config validate --path .\wsl-toolkit.json
wsl-toolkit config --effective > wsl-toolkit.json
```

| | |
| --- | --- |
| `config validate` | refuses a configuration the loader refuses, and WRITES NOTHING |
| `--path` | validate that file instead of the one the search resolves |
| `--effective` | print the configuration that WOULD be used, as JSON, and write nothing |

⛔ **Neither writes.** A caller checking a configuration before using it must not
have the check create the file it was asking about. `config --write` is the one
that writes, and it always writes the state directory's own file.

⭐ **`--effective` is the whole configuration and nothing around it**, so it can
be piped to a file, edited, and passed back with `--config`. `config --json`
carries the report AROUND the configuration; this is the configuration.

Exit 0 valid, 1 not.

### `artifacts retry`

```powershell
wsl-toolkit artifacts retry JOB-ID --to .\recovered
```

Fetches a copy that was RETAINED because its transfer failed. The id is a job id
for a copy kept in the guest, or an artifact set id for one the helper is still
holding; any result that retained a copy names it in `retained`.

⛔ **It does not re-run the job.** Producing the output again is a different act
with a different cost. When nothing was retained it says so.

⛔ **It does not remove the guest copy afterwards.** `gc` collects it under its
own age policy, so a partial retrieval has not destroyed the only remaining copy.

### `resources --job` and `gc --job`

```powershell
wsl-toolkit resources --job JOB-ID
wsl-toolkit gc --job JOB-ID --apply
```

One job instead of the whole store.

⚠ **Naming a job is not a way of saying `--include-live`.** A job that is still
running is spared exactly as it would be without `--job`.

⚠ **`resources --job` withholds the byte totals rather than recomputing them.**
They measure the whole state directory, and a total printed beside one job's rows
would look like it belonged to the job.

### `images warm` and `images pull`

```powershell
wsl-toolkit images warm
wsl-toolkit images pull --select libc:musl
```

| | |
| --- | --- |
| `warm` | which references this machine already holds, and which it does not |
| `pull` | fetch the ones it does not |

⛔ **It exists so a fleet fails FIRST rather than slowly.** An image is pulled
when a job needs it, so a twelve-row matrix on a slow link discovers an
unreachable reference on row nine, an hour in.

⚠ **`warm` does not go to a registry.** Not cached is not unreachable, and a
report that quietly downloaded a gigabyte would not be a report. `pull` is the
one that reaches out.

Exit 1 when any reference could not be reached.

### `examples`

```powershell
wsl-toolkit examples
wsl-toolkit examples --json
```

The canonical command patterns, in the binary rather than only here. ⚠ It is a
deliberate copy of what this page shows: an agent holding the executable and no
manual can still be told the shape of a correct call.

---

## Instances: several agents on one machine

```powershell
wsl-toolkit --instance two base ensure
wsl-toolkit --instance two run --image alpine -c 'id -u'
wsl-toolkit --instance auto base ensure
```

⭐ **An instance is a distribution AND a state directory, together.** One flag
moves both, and nothing moves one without the other. `--instance two` is the
distribution `wsl-toolkit-two` with its state under `<state>\instances\two`: its
own base disk, ledger, transcripts, artifacts and helper endpoint.

| you pass | distribution | state |
| --- | --- | --- |
| nothing | `wsl-toolkit` | `<state>` |
| `--instance two` | `wsl-toolkit-two` | `<state>\instances\two` |
| `--instance auto` | the lowest free `wsl-toolkit-<N>` | `<state>\instances\<N>` |
| `--home D --instance two` | `wsl-toolkit-two` | `D\instances\two` |

⛔ **`--home` sets the ROOT and an instance still gets its own directory under
it.** Two instances sharing one state directory would be two distributions
sharing one ledger, one helper endpoint and one transcript directory, while each
caller believed it was isolated.

⛔ **It is not a scheduler.** No lock, no pool, no allocation service. Two
agents that both ask for `--instance two` get the same base, and that is correct.
`auto` picks the lowest free one, where free means BOTH no registered
distribution and no state directory.

⚠ **A name is 1 to 32 characters of lower-case letters, digits, dash or
underscore.** Lower case is not tidiness: WSL compares distribution names
case-insensitively and a Windows path preserves case, so `-A` and `-a` would be
one distribution with two state directories.

⭐ **A working tree can point at one.** `.wsl-toolkit/instance.json` beside a
checkout, or in any parent, names the instance every call made from inside it
uses. It holds a POINTER and never state: a checkout deleted mid-job loses a file
naming an instance, not a running job's ledger.

```json
{ "schema": "wsl-toolkit-pointer/1", "instance": "two" }
```

⚠ **A flag and the environment both beat a pointer**, in that order, so a file
in a checkout cannot silently decide which machine an agent talks to.

---

## Which configuration is in effect

⭐ **`wsl-toolkit config` prints the file it resolved and every path it looked
at.** The order:

| | looked at | if found |
| --- | --- | --- |
| 1 | `--config PATH` | used. ⛔ A named file that is not there is an error, never a fallback. |
| 2 | `wsl-toolkit.json` in the working directory, then each parent | ⛔ the NEAREST one wins WHOLE |
| 3 | `<state>\config.json` | used |
| 4 | nothing | the compiled-in defaults |

⛔ **The nearest file wins outright rather than being merged.** A partially
merged configuration is one nobody can reason about from any single file.

⛔ **A `wsl-toolkit.toml` found during that search is REFUSED by name.** This
tool reads JSON and has no dependencies, and silently skipping a file somebody
wrote as configuration is how it would lie about which one won.

⚠ **`config --write` always writes `<state>\config.json`**, never the file the
search resolved, so it cannot overwrite a tracked file in somebody's checkout.

---

## Where things live

| | |
| --- | --- |
| state | `%LOCALAPPDATA%\wsl-toolkit`, or `WSL_TOOLKIT_HOME`, or `--home DIR` |
| an instance's state | `<state>\instances\<name>` |
| the base distribution's disk | `<state>\base\ext4.vhdx` |
| the configuration | `<state>\config.json`, or the nearest `wsl-toolkit.json` |
| the ownership record | `<state>\ledger.jsonl` |
| the identity marker | `/etc/wsl-toolkit-identity.json`, INSIDE the distribution |
| the helper's endpoint | `<state>\helper.json` |
| the extracted script | `<state>\script\wsl-toolkit-<digest>.ps1` |
| a working tree's pointer | `.wsl-toolkit/instance.json`, beside a checkout |

---

## Getting it

```powershell
pwsh -NoProfile -File launcher.ps1 -Action List
pwsh -NoProfile -File launcher.ps1 doctor
```

The launcher fetches and verifies it and defaults to the executable.
[`../../../scripts/windows/wsl-toolkit/launcher.md`](../../../scripts/windows/wsl-toolkit/launcher.md)
is its page, including `-LauncherKind script`.

Or take the release asset directly: `wsl-toolkit-windows-amd64.exe` or
`wsl-toolkit-windows-arm64.exe`, with the `SHA256SUMS` published beside it.

⚠ **The release `SHA256SUMS` proves transport, not authorship.** It comes from
the same release as the asset.

---

## Known limits

| limit | what it means for you |
| --- | --- |
| ⛔ a job's stdin is not the caller's | the command travels as a file, so nothing reads this process's stdin. An interactive container is `base shell` plus `podman run -it` inside it. |
| ⚠ the fleet's rows share one utility VM | memory and disk are one machine's |
| ⚠ a link in an artifact set is recorded, not recreated | a `.link.txt` beside where it would have been, whose own name goes through the same collision check. One whose target LEAVES the tree is refused instead. |
| ⚠ a junction in a workspace does not travel | it is left out, counted and named, rather than refused or dropped in silence |
| ⚠ an artifact name a Windows path cannot hold is refused | including `< > : " | ? *`, a trailing dot or space, and a case-only duplicate. Rename it in the container. |
| ⚠ a fleet does not forward each row's own output live | twelve containers interleaved on one stream is unreadable. Rows are announced as they finish and each row's complete text is in its transcript. |
| ⚠ `base status` without `--probe` cannot say a base is usable | running a container is the only thing that answers it |
| ⛔ there is no `--platform` | a container runs this host's architecture. A foreign rootfs on this kernel may RUN rather than fail, through a `binfmt_misc` handler something else registered. |
| ⚠ a musl base runs glibc containers | the base's libc bounds what is installed IN THE BASE only |

---

## Requirements

| thing | needed for |
| --- | --- |
| Windows 10 2004+ or Windows 11, with WSL2 | `base`, `run`, `matrix`, `resources`, `gc`, `script` |
| podman or docker on the host | building the base only. Jobs use the engine inside it. |
| Windows PowerShell 5.1 or PowerShell 7+ | `script` |
| about 6 GiB free, plus twice the rootfs | the base refuses an import the volume cannot hold |

---

## Related

- [`README.md`](README.md), how this executable is built, tested and released
- [`../../../scripts/windows/wsl-toolkit/wsl-toolkit.md`](../../../scripts/windows/wsl-toolkit/wsl-toolkit.md),
  the PowerShell product this one carries
- [`../../../scripts/windows/wsl-toolkit/launcher.md`](../../../scripts/windows/wsl-toolkit/launcher.md),
  fetching and verifying either product
- [`../../../docs/consumers.md`](../../../docs/consumers.md), who fetches from
  this repository and what a change here breaks
- [`../../../CHANGELOG.md`](../../../CHANGELOG.md), what shipped and the defects
  behind each change
