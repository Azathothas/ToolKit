# doctor

What host is this, what is installed, and what is this repo. One read-only
pass, before any of it costs a task.

Two implementations, one schema. Run whichever the host can run.

```bash
sh scripts/doctor/doctor.sh
```

```bash
pwsh -NoProfile -File scripts/doctor/doctor.ps1
```

On Windows prefer the PowerShell one. It needs no POSIX layer, so it answers
on a machine with no Git Bash, no WSL and no msys.

## Why it exists

The defect it catches is an agent that assumes its environment. A session that
assumes node is present writes a node script and finds out at the gate. A
session that assumes Linux reaches for `pkill` on a machine that wants
`taskkill`. A session that takes "most tools are available" on trust plans
around a tool that is not there.

It is also the validator. When the operator states an environment, run this
and compare. A stated fact and a measured one that disagree is the finding.

## What it is not

It is a probe, not a gate. A missing tool is data, not a failure, so it exits
0 whenever it ran. It exits 2 only when it could not run at all. Nothing here
belongs in a gate chain.

It is read-only. No installer, no configuration change, no network call unless the
network flag is passed, and the only file it writes is a temp file it removes.

## Flags

| flag | sh | ps | what it does |
| --- | --- | --- | --- |
| json | `--json` | `-Json` | emit the schema document instead of the report |
| text | `--text` | `-Text` | select the human report explicitly. It is the default. |
| fast | `--fast` | `-Fast` | presence only, skip every version probe |
| net | `--net` | `-Net` | also test outbound reachability |
| group | `--group vcs` | `-Group vcs` | probe one group only |

Groups: `vcs`, `runtime`, `compiler`, `pkg-lang`, `pkg-system`, `container`,
`build`, `quality`, `cli`, `cloud`, `shell`, `agent`.

## What a run costs

A full run spawns one process per tool, so it takes tens of seconds on Windows,
where a spawn costs most. `--fast` and one `--group` take a few seconds. Time it
on the host in front of you rather than quote a number from another.

## The schema, agent-doctor/1

```
schema      the string "agent-doctor/1"
generated   ISO 8601 UTC
probe       impl, fast, group
host        os flavor wsl container kernel arch distro distro_version
            shell writable_tmp network
repo        is_git root branch remote dirty commits
            remote_looks_like_template has_codegraph ecosystems
summary     tools_found tools_missing
tools[]     id group found path version
notes[]     things the probe wants said out loud
```

Field notes that are easy to read wrong:

- `flavor` is the shell environment, not the OS. The sh twin under Git Bash
  reports `msys`; the ps twin on the same machine reports `native`. Both are
  right about the environment they are in.
- `kernel` is a best-effort build identifier and its shape differs by platform.
  Do not parse it.
- `ecosystems` is read from manifest files that are actually present. It is
  evidence, not a guess from a directory name.
- `remote_looks_like_template` is a warning, not a fact about the project. It
  fires when `origin` contains the word template, which is the state a fresh
  clone of this repository is in and must leave before any project work.
- `version` empty with `found` true means the tool answered nothing. That is
  reported in `notes` and it usually means a shim rather than an install.

## The two twins have to agree

Changing a field in one means changing it in the other. The check is to run
both on one machine and compare: same top-level keys, same section keys, same
values for `os`, `arch`, `wsl`, `container`, `distro_version`, and the same
verdict per tool.

⚠ Some disagreement is correct and must not be flattened away. Each twin
reports what its own host can actually reach, and on a Windows machine with
msys installed those differ honestly:

| id | sh sees | ps sees | why |
| --- | --- | --- | --- |
| `bash` | msys bash | the Windows PATH bash | two different binaries |
| `tar` | GNU tar | Windows bsdtar | two different binaries |
| `zsh` | present | absent | msys `/usr/bin/zsh` is not on the native PATH |
| `psscriptanalyzer` | not probed | probed | a PowerShell module, invisible to sh |

## Traps the probe handles

- ⛔ A greedy regex over a version line reports the wrong half of the version,
  and does it confidently: `git version 2.51.0.windows.3` reads as
  `5.0.windows.3`, and `v22.11.0` as `7.0`. So the probe splits the line into
  tokens and takes the first that reads as a version. A wrong number is worse
  than a blank one, because a blank one gets checked.
- ⛔ The name may be joined to the number by a hyphen, as in `jq-1.8.2`, and the
  pattern allows it.
- ⛔ Several tools block for as long as you let them. `kubectl version` without
  `--client` contacts a cluster. Every probe is time-limited at six seconds,
  and a timeout is reported as its own fact rather than as an absent tool.
- ⛔ In the sh twin, `probe_version` runs inside `$( )`, which is a subshell,
  so an assignment inside it is discarded. The caller reads the exit code.
- ⛔ In the ps twin, `Process.Start` with `UseShellExecute` false cannot run a
  `.ps1` or a `.cmd`. On Windows the node ecosystem ships such shims, and
  scoop's are `.ps1`. So the launcher starts a `.ps1` through PowerShell and a
  `.cmd` or `.bat` through `cmd.exe`.
- ⛔ In the ps twin, a value read from merged stdout and stderr can carry a git
  fatal into the `branch` field. A version probe merges the streams on purpose,
  because java prints its version to stderr. Anything that reads a value must not.
- ⛔ `.NET` says `X64` where `uname -m` says `x86_64`. Normalised, or one
  machine reads as two.
- ⚠ `wsl.exe` writes UTF-16LE, which a redirected stdout reads as empty. It is
  probed for presence only.
- ⚠ `Invoke-ScriptAnalyzer` is a cmdlet, not an application, so a PATH lookup
  can never find it. It is checked as a module instead.
