# consumers.md

Who depends on this repository, what they hold, and what they must read before
moving.

⭐ **This is the file that makes ToolKit different from an ordinary project.**
The consumers of a tool here are not in this tree, so nothing in this repository
can fail when their contract is broken. Only they can, later, on a machine
nobody is watching.

---

## The rule

⛔ A consumer owns its migration. Work in ToolKit does not edit another
repository, advance its pin, or preserve a deleted compatibility product.
Before moving, the consumer reads the latest
[`wsl-toolkit` manual](../tools/windows/wsl-toolkit/wsl-toolkit.md) and release
notes directly; copied invocation prose is not an interface.

⚠ **The register is a lower bound on who is affected, never the complete set.**
Operator scripts and machines that are not visible from this repository may also
hold old paths or releases, so a change to a fetched file says which consumers
were checked rather than assuming the answer.

---

## The register

Each row is read from the consumer's own tree, and a change to a fetched contract
reads it again.

| consumer | current relationship | what it must do before moving |
| --- | --- | --- |
| `Azathothas/TEMPLATE`, at `docs/containers.md` and `docs/agent-tooling.md` | describes the tool as a PowerShell product and an executable, with a launcher | rewrite its guide from the latest manual |
| `Azathothas/bit-cli`, at `scripts/wsl-tool.ps1` and `docs/containers.md` | its wrapper runs the deleted launcher at a pinned commit with SHA-256 pins, and its guide fetches `wsl-ephemeral.ps1` by raw URL at a commit it resolves | keep its current pin until it chooses to move, then choose an executable release, verify it, and translate its own invocation from the latest manual |
| `Azathothas/podbox`, at `docs/containers.md`, `docs/agent-tooling.md` and `experiments/` | calls an installed executable with `--instance podbox`: `run --script`, `base status --probe`, `base ensure --probe`, `resources`, `gc --apply --older-than`, `gc --job ID --apply`, `hostaddress`. Reads the three skills from `main` by raw URL | read the latest manual, then replace the wrappers written around a job that outlives its client with `run --detach`, `logs ID --follow`, `wait ID`, `stop ID` and `base exec --detach`, and pass a device and a second file with `--device` and `--input`. Before `wsl-toolkit-v7.0.0`, read its breaks below: `gc --job` exits 2 for an id this instance does not hold, `base ensure` registers QEMU handlers, and printed commands name `--instance podbox` |
| `Azathothas/podssh`, at `scripts/dev-wsl.sh` and `scripts/dev.sh` | bridges every call from Git Bash through PowerShell, keeps seven `--exclude` patterns in a shell string, holds its builds apart with a lock directory of its own, and its `clean` removes nothing, because `gc` took every project's jobs | read the latest manual, then call the tool from Git Bash with `MSYS_NO_PATHCONV=1` and `MSYS2_ARG_CONV_EXCL='*'` exported, pass `--workspace "$PWD"`, `--script -` and `--gitignore`, hold builds with `--exclusive`, and clean with `gc --workspace .` |
| `Azathothas/pg-toolkit`, at `cmd/pg-devenv` | calls an installed executable with `--instance pg-toolkit`: `config --json`, `config --write`, `base ensure`, `base exec --dir`, `base shell`, `base status`, `base remove --yes`, `resources`, `selfupdate --check` and `version` | read the breaks below before `wsl-toolkit-v7.0.0`: its `base ensure` registers QEMU handlers and can exit 2 over them, and a grant whose directory is gone makes `base status` exit 1 where `config` refused before |
| `Azathothas/agent-box`, at `xtask/src/linux.rs` | runs `run --image IMAGE --workspace DIR -c COMMAND`, with `--device /dev/kvm` where asked, from `PATH` | nothing for `wsl-toolkit-v7.0.0`. `--gitignore` can shrink its workspace copy |
| `Azathothas/dash.vpn.ajam.dev`, at `docs/operations/linux.md` | a recipe an operator follows from PowerShell: `run`, `base shell`, `base exec`, `distro new`, `stop` and `gc --apply` | nothing for `wsl-toolkit-v7.0.0`. Its rule against Git Bash can become the two variables the manual names |
| `pkgforge-dev/cross-libc-dlopen`, at `scripts/wsl-ephemeral.ps1` | carries a vendored copy and fetches nothing from ToolKit | nothing here can update it; its owner decides whether to replace the copy |

⭐ **The published executable CARRIES the three fetched scripts**, so a machine with
the binary needs no clone and no fetch. `wsl-toolkit shipped list` prints each
one's length and SHA-256, `shipped cat` and `shipped write` produce it, and `base
bootstrap` runs the carried bootstrap inside the base with nothing copied anywhere.
⛔ **The URLs stay the contract for a caller outside this tree.** What the carried
copy removes is an operator copying a file into a checkout with no way to say
which version it was. The gate's `shipped` check refuses a carried copy that
disagrees with the file below.

Three files are intended for direct fetching and have no known consumer:
[`bootstrap.sh`](../scripts/common/bootstrap.sh),
[`tmux.conf`](../scripts/common/tmux.conf) and
[`shell-profile.sh`](../scripts/common/shell-profile.sh). Add a row when a
consumer is found. ⚠ `shell-profile.sh` is READ BY A LOGIN SHELL rather than run,
so a caller who fetched it holds a file that every shell on that account starts,
and a change to it is felt on the next login rather than at the next call.
⚠ `bootstrap.sh`'s shared package table is also a build input to the published
executable. The base provisioner resolves its `developer` names through a generated
copy of it, so an edit to that block reaches both in one commit.

The dependency on `pkgforge-dev/docker-bsd` runs the other way: it publishes
BSD images that ToolKit names. It is not a consumer row.

⛔ **The PowerShell product, its launcher and `wsl-toolkit script` are
deleted.** A raw fetch of
`raw.githubusercontent.com/Azathothas/ToolKit/REF/scripts/windows/wsl-toolkit/FILE`
at a commit after the deletion returns 404, and a commit before it still serves it.
No release publishes a script. The `distro` and `hostaddress` commands carry what
the product did. [`HISTORY/consumers.md`](HISTORY/consumers.md) keeps the
pin-state table and the release history.

---

## What counts as a break

A change breaks a caller when a correct existing call behaves differently:

| change | effect |
| --- | --- |
| file removed or moved | its raw URL no longer names a runnable product |
| flag renamed or retyped | the call refuses or binds differently |
| exit meaning changed | automation reads a different verdict |
| output schema or unflagged stream changed | a parser reads different bytes |

Fixing a false pass is still a break and should still be fixed. Record it where
the work closes; do not keep a defective surface solely because a caller may
depend on it.

### ⛔ What the canonical default base name breaks, and what a caller does

`WSL-167`, ruling 33. Not released yet. A break by the table above, so the release
that carries it is a major one. ⭐ A host that already has a default base keeps
using it, under its old name, from every state directory.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| a NEW default base is registered as `wsl-toolkit-jobs`. A base built before keeps its name, `wsl-toolkit` | names the distribution `wsl-toolkit` directly, on a host whose first default base is built after the update | reads the name from `base status --json`, or runs the tool's own commands |
| removal, rebuild, re-provision in place and `--repair` are refused, exit 2, from a state directory that does not own the base, directly or through the helper. The refusal names the disk folder and the command to run from its owner | removed, rebuilt, re-provisioned or repaired the host's base from a scratch `WSL_TOOLKIT_HOME` or `--home`, which 7.0.2 allowed | runs the command the refusal names, from the state directory that owns the base |
| the same set is refused to a process whose writes under `%LOCALAPPDATA%` Windows redirects into a package's copy, such as the Claude desktop app's shell. Its `base ensure` installs no adapter and removes none, and still exits 0 | removed, rebuilt, re-provisioned, repaired or stamped the base from inside the app, or relied on its `base ensure` installing a configured adapter, which 7.0.2 allowed | runs those from PowerShell or a terminal outside the app |

⚠ **Refusals that break no working caller.** Both default names registered is refused
by name, and `base.name` chooses. A new instance named `base` or `jobs` is refused. An
existing one stays reachable. `muse.exe`, `pi.exe` and `omp.exe` reach the default base,
not the instance `base`, which ruling 33 removes.

⚠ **The register's rows list no call that names `wsl-toolkit` as a distribution or
the instance `base`.** Their own trees were not read again for this change, so each row
is read before the release.
### ⚠ What the instance prefix changes, and what a caller does

`WSL-193`. Not released yet. An existing instance keeps its name, so no working call
changes meaning for it.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| a NEW instance named `wsl-NAME` or `wsl-toolkit-NAME` is made as `NAME`, distribution `wsl-toolkit-NAME`, with one warning on stderr. `wsl-`, `wsl-toolkit-`, `wsl-toolkit`, `wsl-base`, `wsl-jobs` and `wsl-auto` are refused | creates such an instance on a new host and then names its distribution `wsl-toolkit-wsl-NAME` directly | reads the name from `base status --json`, or passes `--literal-instance` |

### ⚠ What an ephemeral job's record changes, and what a caller does

`WSL-121`. Not released yet. The help always said an ephemeral job leaves nothing behind;
now its host record goes too, once its answer is written.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| an attached ephemeral job removes its host record when it ends. Its answer is kept for 10 minutes, so `wait ID` and `logs ID --follow` still answer its verdict | reads `logs ID`, `inspect ID` or `gc --job ID` for an ephemeral job long after it ended | reads the answer `run` printed, or runs the job without `--container-lifecycle ephemeral`, which keeps the record |

### ⚠ What an empty `/out` changes, and what a caller does

`WSL-124`. Not released yet.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| `run --artifacts DIR` makes `DIR` with the first entry that arrives, so a job that leaves `/out` empty makes no directory, on the direct route and through the helper | tests that `DIR` exists to learn that the job ran | reads `artifacts` in the `--json` answer, which is 0, or makes `DIR` itself first |

### ⛔ What a project lending every `jobs` key breaks, and what a caller does

`WSL-131`, ruling 34. Not released yet. A break by the table above, so it goes into
`wsl-toolkit-v8.0.0`.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| under a named instance with its own file, a working tree's `wsl-toolkit.json` lends every `jobs` key it writes. An instance whose own file sets `container_lifecycle`, `platform`, `workspace`, `exclude` or `gitignore` now takes the project's value | runs a named instance from a working tree whose file writes one of those keys, and relies on the instance's own value | removes the key from the project's file, or names the file to read with `--config` |
| a key the project writes is taken as written, zero included. A `max_running`, `max_bytes` or `max_entries` of `0`, an `env` of `{}` or a `cache` of `[]` now replaces the instance's own value | writes such an empty value in a project's file and relies on the instance's own cap, limits, environment or caches | removes the key from the project's file, or names the file to read with `--config` |
| a project file that writes only `container_lifecycle`, `platform`, `workspace`, `exclude` or `gitignore` is now read strictly under a named instance. A broken file, or one a newer build wrote, now refuses | runs a named instance from a working tree whose file was ignored before and is broken or newer | fixes the project's file, or names the file to read with `--config` |
| `config --json` drops `project_held` and carries `project_lends`, each key the project lent. `config`, `run` and `matrix` print no ⚠ line about held keys | parses `project_held` or that line | reads `project_lends` |

### ⚠ What a pull's sized stall changes, and what a caller does

`WSL-176`. Not released yet. No exit code changes for a call that names no new
variable.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| a pull may be silent for its largest layer's time at 256 KiB/s, and never under four minutes. Its total ceiling grows past thirty minutes where every layer needs it. A slow pull that was given up now finishes | relied on a slow pull failing within four minutes | sets `WSL_TOOLKIT_PULL_STALL` to the limit it wants |
| `base ensure`, `base recreate` and `distro new --image` read the image's manifest from its registry over HTTPS, with no credential, before the pull. A line on stderr says so first, and the `pulling` line adds the limit and why | treats every stderr line of a build as one it knows | reads the exit code, or `--json` |
| each build sends its image's registry up to three more requests: a token and two manifest reads. Docker Hub counts the two reads as one pull, so a build there counts as two against its anonymous limit | builds from Docker Hub close to that limit | allows for the second pull, or builds from another registry |
| a host that reaches the registry only through the engine, or a private image, keeps four minutes, and the line says why | | sets `WSL_TOOLKIT_PULL_STALL`, or `HTTPS_PROXY` where a proxy is the reason |
| a stall's refusal says silence is not a measured speed, why the limit was what it was, and names `WSL_TOOLKIT_PULL_STALL`. It no longer says the cause is not a slow link. Its last command is the pulling engine's own, such as `docker info` | matches the old refusal's words | matches `and was given up` |
| a `WSL_TOOLKIT_PULL_STALL` that is no positive duration refuses before the engine is asked, and a forced rebuild keeps the base. `distro new --image --dry-run` exits 2 where it exited 0 | reads that exit 0 as a run that would succeed | sets a duration such as `45m`, or removes it |
| `WSL_TOOLKIT_PULL_STALL` names the stall, and the layers still size the ceiling. The ceiling is the longest of thirty minutes and every layer at either rate, so it never shortens | | nothing |
| through the helper, the caller's `WSL_TOOLKIT_PULL_STALL` crosses with `base ensure`, `base recreate`, `ready --ensure` and the ensure before a job. The helper reads no variable of its own | set the variable only where the helper was started | sets it where it calls the tool |
| through the helper, a caller's value that is no duration refuses on the client with exit 2, before the helper is asked | | sets a duration such as `45m`, or removes it |

### ⚠ What watching the host's free space changes, and what a caller does

`WSL-180`. Not released yet.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| `base status --json` adds `host_free_bytes`, and the advisory `host-space-low` under 6 GiB free. `ready` lists the advisory in its notes, and its verdict does not change | | reads them, or ignores them |
| `base ensure --json` and `base recreate --json` add `host_free_bytes` and the advisory too, and their human output prints the `host-space-low` block | treats every line or field of an ensure as one it knows | reads `--json`, and ignores what it does not know |
| `base status` prints the `host-space-low` block under that floor | treats every line of `base status` as one it knows | reads `--json` |
| a job starting under that floor prints a ⚠ line on stderr, through the helper too, and still runs | treats every stderr line of a job as one it knows | reads the exit code, or `--json` |
| `base ensure`, `base recreate`, a `--preset` switch and `distro new --image` refuse before the engine is asked, under the floor. Each refused after the pull before, with the same exit code | | nothing |
| `distro new --image --dry-run` exits 2 under 256 MiB free, where it exited 0 and the real run refused | reads that exit 0 as a run that would succeed | frees space, or reads the refusal |
| `base image load IMAGE` refuses under 256 MiB free, before the engine saves the image into the state directory | loads an image from the engine onto a nearly full volume | frees space, or loads the image from a file |

### ⚠ What reading `--home` the MSYS way changes, and what a caller does

`WSL-187`. Not released yet.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| under Git Bash, `--home /d/x` and `WSL_TOOLKIT_HOME=/d/x` name `D:\x`. They named `x` under the current drive's root, such as `C:\d\x` | keeps jobs, a ledger, locks or grants in that second root | moves what it needs to `D:\x`, or passes the old root's Windows spelling |
| under Git Bash, `/tmp/x` names `x` in the temporary directory, and any other `/...` is under the Git install, such as `C:\Program Files\Git\srv\x`. They named `C:\tmp\x` and `C:\srv\x` | keeps its state under such a root | passes the directory's Windows spelling |
| an agent's launcher, such as `muse.exe`, reads `WSL_TOOLKIT_HOME` the same way, and hands it on absolute | started a launcher from Git Bash and kept its state in the second root | moves what it needs, or passes the Windows spelling |
| `WSL_TOOLKIT_HOME` is handed on absolute, so the helper and a detached child read the directory their parent read | | nothing |

### ⛔ What the tool's own exit code breaks, and what a caller does

`WSL-127`, ruling 35. Not released yet. A break by the table above, so it goes into
`wsl-toolkit-v8.0.0`. A command that answers with a job's own code now answers 125
where this tool could not run the job. So a job's 2 is never read as a refusal.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| a command that answers with a job's code answers 125 where this tool could not run the job, a refusal of a flag included. A job's own 2 is still 2. Those commands are `run`, `wait`, `logs ID --follow` and `bsd run`. They are `base exec`, `base shell`, `base agent` and its launchers, `base herdr` and `base bootstrap`. They are `distro new` given a command, `distro run` and `distro enter` | reads exit 2 from one of them as "could not run", or as a refused flag | reads 125, or passes `--tool-exit 2`, or sets `WSL_TOOLKIT_TOOL_EXIT=2`, which answers what 7.x answered |
| an unreached job's `effective_exit` is 125, and its `exit` stays 2 | reads `effective_exit` 2 as an image that never ran | reads `job_started` or `unreached` |
| a job whose payload started and whose end this tool lost answers 125 with `tool_failed`, where 7.x answered 2 as the job's own | reads that 2 as the job's | reads `tool_failed` |
| a job no process owned when it ended, with no exit its engine holds, ends `wait` and `logs ID --follow` with 125. Its result carries `tool_failed`, `job_started` true and `exit` 2. 7.x answered 1, with `exit` 1 | reads that 1 as a job that ran and disagreed | reads 125, or `tool_failed` |
| `run --via-helper` whose stream broke after the helper took the job on answers the job's result, with `tool_failed` and `job_started` true, and exit 125. 7.x answered 2 and wrote nothing under `--json`. A call cancelled on that route answers 130, where 7.x answered 2 | reads a broken helper stream as a job that never started, or its cancellation as a refusal | reads `tool_failed`, and 130 as a stop |
| a refusal under `--json` writes `wsl-toolkit-refusal/1` on stdout, where it wrote nothing. It does so for `run`, `base exec`, `distro run`, `distro new` given a command, and `bsd run`. Its `command` names the subcommand, such as `base exec`. A job refused after it was claimed writes its own result | treats any stdout under `--json` as a job's answer | reads `schema` |
| `bsd run` past its `--timeout` answers 124, as every deadline this tool holds does, and its `--json` carries `timed_out`. After a guest kernel panic it answers 125. 7.x answered 2 for both | reads 2 there | reads 124 for the deadline and 125 for the panic |
| a job that never ran, or whose end was lost, prints 125 where 7.x printed 2. That is the `exit` of `logs --json` and of `stop --json`, and the exit line of `inspect`. Each is what `wait` answers | reads that 2 as an image that never ran | reads 125, or the job's `job_started` and `tool_failed` |
| a result a 7.x owner recorded carries no `job_started`, so 8.0 `wait --json` reads it as false, for a job that ran too. Its `effective_exit` is resealed to what this `wait` answers | reads `job_started` false as "never ran" on a result 7.x wrote | reads `unreached` as well, on a job 7.x recorded |
| a job's result, each `matrix` row, `wait --json`, `distro run --json`, the `command` of `distro new --json` and `bsd run --json` carry `job_started`. The last three also carry `tool_failed`, and `bsd run` carries `timed_out` | | reads it, or ignores it |
| where 7.x printed `LABEL exited 2 in D`, `run` without `--json` prints `LABEL did not start in D`. A lost end prints `LABEL started, and this tool lost how it ended in D`. The reason `ready --smoke` gives says the same | parses that stderr line | reads the exit code, or `--json` |
| `--tool-exit` given no code, `--tool-exit=` included, is refused. An empty `WSL_TOOLKIT_TOOL_EXIT` still reads as unset | passes an empty `--tool-exit` and relies on 125 | passes a code, or leaves the flag out |
| the helper protocol is `wsl-toolkit-helper/7`, and a 7.x helper and an 8.x client refuse each other | leaves a 7.x helper running after the update | stops it and starts it again: `wsl-toolkit helper stop`, then `wsl-toolkit helper serve --detach`, each with the `--instance` it serves |

⚠ **Every other command keeps 2 for a refusal**, `matrix`, `stop`, `config
validate` and `gc --job` included. None of them answers a job's own code.

⚠ **The rows it reaches:** agent-box and podbox call `run`, dash.vpn calls `run`,
`base exec` and `base shell`, and pg-toolkit calls `base exec` and `base shell`. Their
own trees were not read again for this change, so each is read before the release.

### ⛔ What `text-tool eol` refusing a missing path breaks, and what a caller does

`WSL-166`, ruling 35. Not released yet. `text-tool` ships with `wsl-toolkit`, so it
goes into `wsl-toolkit-v8.0.0`.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| `eol` given a path that does not exist exits 1 and writes nothing, to that path or to any file named beside it, with or without `--dry-run`. 7.x converted the others, said `missing`, and exited 0 | names a path that may not exist and reads exit 0, or relies on the files beside it converting | names only paths that exist, or runs one `eol` per path and reads each exit |
| the stderr line `text-tool: N path(s) do not exist, and eol created nothing there` is gone. The refusal says `N of M path(s) do not exist` | parses that line | reads the exit code, or `missing` in `--json` |

### ⭐ What the base runtime adds, and what a caller reads

`WSL-126`, `WSL-130` and the limits in `WSL-136`. Not released yet. No exit
code changes, and no field a 7.0.2 document carries is removed.

| change | a caller that | the caller does |
| --- | --- | --- |
| `cgroup.delegated` and `cgroup.mechanism` in `base status --probe --json` also count the account's user manager, where podman's manager is `systemd` | read `false` and `none` on such a base while its limits held | reads `limits_enforced` and `stats_usable`, which a container measures |
| beside the `cgroupfs` manager the user manager does not count, so those two fields read as in 7.0.2. The finding names the manager | | nothing |
| `base status --probe --json` adds `cgroup.manager`, `cgroup.account_controllers`, `kvm` and `access.root_engine`, `access.runtime_dir`, `access.kvm` | | reads them, or ignores them |
| `base status` prints its `engines` row only where one of the three settings is on, and its `kvm` row only where `base.kvm` is on | | nothing: an unchanged base prints the rows 7.0.2 printed |
| `base exec --detach` prints a second stderr line that names what a stop reaches. `stop` of a session prints one where it signalled something | treats every stderr line of these two as one it knows | reads `contained_by` and `unit` from `--json` |
| a session's deadline in a scope ends what a command that ended first left in that scope. The session's exit stays its own | relied on a daemon outliving its session's deadline | passes `--timeout 0`, or starts the daemon outside the session |
| `stop` of an ended session ends what it left in its scope. It still exits 0 with no `after`, and `leftovers` says what happened | | reads `leftovers` |
| `gc` ends what an ended session left in its scope before it removes the session. A process that outlives that keeps the session, listed as failed | | runs `stop ID`, then `gc` again |

⚠ **The two stderr lines count as a break by the table above,** because an unflagged
stream changes. Each is added on stderr and leaves stdout and `--json` unchanged.
They ship in `wsl-toolkit-v8.0.0`, a major release by ruling 34.

### ⚠ What the base build changes, and what a caller does

`WSL-137`, `WSL-143`, `WSL-147` and `WSL-150`. Not released yet.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| `base status --json` adds `probed`, `account.passwd`, `account.login_shell` and `account.bash`, and the advisories `account-passwd`, `account-cannot-start` and `login-shell-not-bash` | | reads them, or ignores them |
| an owner's `base ensure` moves the managed account's login shell from `/bin/sh` to bash where the guest has bash | runs scripts in the base that rely on the account's login shell being `sh` | names its shell, as `base exec` scripts already do |
| `bootstrap.sh` puts the account's prefix after the system path and rewrites the line an earlier run put first, so a name that exists in both resolves to the system path's | relies on the account's own copy of a name shadowing the system's | runs that copy by its full path |
| `base ensure` refuses an interrupted build and names `base remove --yes`, where it removed and rebuilt it | expects ensure to recover an interrupted build alone | runs `base remove --yes`, then `base ensure` |

### ⭐ What a restart of WSL changes, and what a caller reads

`WSL-170`, `WSL-178` and `WSL-184`. Not released yet. No field a 7.0.2 document
carries is removed. ⚠ Only a session the base restarted under answers another
code. `wait` answers 1 where it waited or answered 124. `stop` exits 0 where it
signalled another group and exited 1. Both ride in `wsl-toolkit-v8.0.0`.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| a detached session the base restarted under reads as gone without an exit, after `wsl --terminate` as after a restart of WSL's VM. `wait` and `logs --follow` answer 1 at once, `inspect` prints `gone`, and `wait --json` adds `session.lost`, `restart` or `reused` | waited on such a session, which 7.0.2 did until whatever held its old supervisor id ended | reads `lost` |
| a session a 7.x build started is held to its supervisor's own arguments. Where its supervisor id names a process without them, it reads as gone, with `lost` `reused` | waited on, or stopped, such a session after a restart | reads `lost` |
| `stop` of such a session signals nothing and exits 0. Its `--json` reads `before` `ended` and `exit` 1. So does a stop that finds the session lost only as it reaches it, where it said `running` and `stopped` | relied on that stop to end the process group its old id names, which may be another agent's | nothing: that group is not the session's |
| `gc` and `resources` read such a session as ended, where a reused supervisor id read as running. `gc --apply` removes it | kept such a session by its `running` state | nothing: its supervisor is gone |
| every owner's `base ensure` writes `/usr/local/lib/wsl-toolkit/boot.sh` and names it in `/etc/wsl.conf`'s `[boot]`, sealed base or not. WSL runs it from the next start, so nothing restarts for it | edits `/etc/wsl.conf` itself, or expects it to change only at provisioning | leaves the line to this tool. A file this tool did not write, or one naming another boot command, is left as it is |
| a sealed base's `/usr/local/lib/wsl-toolkit/seal-boot.sh` gives way to `boot.sh`, and its log moves from `/run/wsl-toolkit-seal.log` to `/run/wsl-toolkit-boot.log` | checks for `seal-boot.sh`, or reads its log | checks `boot.sh` and reads `/run/wsl-toolkit-boot.log` |
| `base ensure` writes `/usr/local/lib/wsl-toolkit/run-state.sh` and `/etc/tmpfiles.d/wsl-toolkit.conf`, and on a systemd base enables `wsl-toolkit-run-state.service` | expects no unit of this tool's beside its own | leaves the unit to this tool |
| the engine's state is held to a start of the base, in `.wsl-toolkit-run-state.start` beside its lock. A state an earlier start left is cleared at the next start and before each job, as another boot's was. A live `conmon` of the account keeps it | relied on podman reading an earlier start's containers as running after `wsl --terminate` | nothing: every process of that start ended |
| a probe that fails over another boot's record ends its problem with this tool's words naming the boot id | matches the problem's text exactly | reads the `stale-run-state` remediation |
| `examples` and the man page add the nested engine shape. `run` prints a stderr note naming it after a job whose nested podman found no `/dev/fuse` | treats every stderr line of `run` as one it knows | reads the job's own exit |

### ⚠ What a grant that survives a restart changes, and what a caller does

`WSL-190`. Not released yet. A grant that works today keeps working. ⛔ The helper
protocol moves to `wsl-toolkit-helper/7`, a break by the table above, so it ships
in `wsl-toolkit-v8.0.0`, a major release by ruling 34.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| a restart mounts again each grant the guest's ledger gives another configuration file, after `base ensure --repair` too, directly or through the helper | relied on a restart, or on `--repair`, to drop a grant another file made | revokes it with that file: `wsl-toolkit --config FILE base revoke --target TARGET`. A file that is gone is written again, naming the grant, first |
| an ensure refused over another file's live grant says the restart ends that work, where it said the restart drops the grants | parses the refusal's words | reads the targets, files and revokes it names, which are unchanged |
| the helper protocol is `wsl-toolkit-helper/7`, and a 7.x helper and an 8.x client refuse each other. An ensure or a probe through the helper sends the caller's configuration file, and acts for it as the CLI does | leaves a 7.x helper running after the update | stops it and starts it again: `wsl-toolkit helper stop`, then `wsl-toolkit helper serve --detach`, each with the `--instance` it serves |

⚠ **A grant a 7.0.2 build left without its fstab line is not written back.** The
ledger row carries no line, so on such a base the next restart can drop one file's
grant, as 7.0.2 did. The manual's grant section names the two commands of that
file which write the line back.

### ⛔ What `wsl-toolkit-v7.0.0` breaks, and what a caller does

A major version, so a consumer that pins by major is not moved across it silently.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| `binfmt.ready` in `base status --probe --json` reads the base's own QEMU handlers once an ensure of this release has recorded them, and every handler in the kernel until then. `WSL-105` | reads `ready: true` while another distribution's handlers were the only ones | runs `base ensure` once after the update, and reads `binfmt.recorded`, `binfmt.own` and `binfmt.missing` |
| `base ensure` registers the base's QEMU handlers on every base and exits 2 when it cannot. A systemd base gains `wsl-toolkit-binfmt.service`. `WSL-105` | expects exit 0 from a base that verified, or no unit of this tool's beside its own | reads the refusal's words, and leaves the unit to this tool |
| a grant whose Windows directory is gone no longer refuses the configuration. `config` and `config validate` exit 0, and `base status` exits 1 naming the revoke. `WSL-106` | gates on `config validate` failing for a gone grant | reads `access.mounts[].missing`, or `base status` |
| `gc --job ID` refuses an id that is not 16 lowercase hex characters, or that nothing in the instance belongs to, with exit 2 and no plan. `WSL-115` | runs it with a shortened id, or for a job already cleaned up, and reads exit 0 | passes the id `run --json` printed, and reads exit 2 as an id this instance does not hold |
| `gc --images` beside `--job` or `--workspace` is refused, exit 2. `WSL-114` | narrows a cleanup and prunes every image in the same call | runs `gc --images` on its own |
| an empty `gc --job ""`, `gc --workspace ""` or `run --exclusive ""` is refused, exit 2 | passes a variable that can be empty and relies on it reading as not given | passes the flag only with a value |
| under `--instance NAME`, every printed command carries `--instance NAME`, the `follow`, `wait` and `stop` of `wsl-toolkit-detached/1` included. `WSL-116` | parses those strings for a fixed prefix | runs them as printed, or reads `id` |
| the helper protocol is `wsl-toolkit-helper/6`, and a 6.x helper and a 7.x client refuse each other | leaves a 6.x helper running after the update | stops it and starts it again: `wsl-toolkit helper stop`, then `wsl-toolkit helper serve --detach`, each with the `--instance` it serves |
| `base status`'s `binfmt` row names the base's own handlers and the kernel's total | parses the text | reads `--json` |

Everything else in 7.0.0 adds or corrects: `--script -`, `--env-file`, `--gitignore`,
`--exclusive`, `gc --workspace`, `base reclaim`, `wslc` as a host engine, a host path
in MSYS's spelling from Git Bash, an `advisory` mark on a remediation, and the copies
of the state directory `doctor` and `config` name.

### ⛔ What `wsl-toolkit-v6.0.0` breaks, and what a caller does

A major version, so a consumer that pins by major is not moved across it silently.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| a `helper` or `config` subcommand refuses a flag it does not read, exit 2. `WSL-104` | passes a flag the subcommand ignored, such as `helper stop --json` or `config --path FILE` | removes the flag. `config validate --path FILE` reads a named file |
| `helper serve --json` with no `--detach` is refused, exit 2 | passes `--json` to a helper in the foreground | adds `--detach`, or removes `--json` |

Everything else in 6.0.0 adds or corrects: `--help` and the manual list only
the flags a command reads.

### ⛔ What `wsl-toolkit-v5.0.0` breaks, and what a caller does

A major version, so a consumer that pins by major is not moved across it silently.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| a `base` subcommand refuses a flag it does not read, exit 2. `WSL-100` | passes a flag the subcommand ignored, such as `base status --repair` | removes the flag. `base ensure --probe` stays accepted |
| the helper protocol is `wsl-toolkit-helper/5`, and a 4.x helper and a 5.x client refuse each other | leaves a 4.x helper running after the update | stops it and starts it again: `wsl-toolkit helper stop`, then `wsl-toolkit helper serve --detach` |
| `logs` with no id prints kind, state and exit columns | parses the text list | reads `logs --json`, which only gains fields |
| `run` names the job's id on stderr before the job starts | treats every stderr line as the job's own | reads the job's streams from `--json` or from `logs ID` |
| `--redact` reaches the structured answer and the transcript. `WSL-101` | reads a redacted value back from the answer | nothing: the value reaching the answer was the defect |
| `gc` reads a job whose owner process is gone as ended. `WSL-94` | counts on `gc` keeping a killed job's leftovers until its deadline | reads the job with `wait` or `inspect` before `gc --apply` |

Everything else in 5.0.0 adds: `run --detach`, `logs ID --follow`, `wait`,
`stop`, `base exec --detach`, `--device`, `--input`, and `--command-base64` on
every job command.

⚠ **A `text-tool edit --between` needs `--expect N`**, as every other search
does. A build older than `wsl-toolkit-v4.0.0` does not require it. `WSL-93`

---

## Fetching the published product

A release carries:

- `wsl-toolkit-windows-amd64.exe`
- `wsl-toolkit-windows-arm64.exe`
- herdr's newest stable release built by this repository:
  `herdr-VERSION-windows-x86_64.zip`, `herdr-VERSION-windows-aarch64.zip`,
  `herdr-VERSION-linux-x86_64` and `herdr-VERSION-linux-aarch64`
- `text-tool` for both hosts: `text-tool-windows-amd64.exe`,
  `text-tool-windows-arm64.exe`, `text-tool-linux-amd64` and
  `text-tool-linux-arm64`
- `SHA256SUMS`
- one `<asset>.cosign.bundle` per file above

⭐ **`text-tool` writes and edits a file without a shell touching the payload**, and
it is a separate program with no dependency on the rest: take the one asset for your
host and nothing else. [`../skills/text-tool/SKILL.md`](../skills/text-tool/SKILL.md)
is the page to hand an agent, and it stands alone.

⭐ **herdr's development branch is published separately, as a nightly prerelease** on a
`herdr-nightly-YYYYMMDD-SHA12` tag. It carries the same four builds, `BUILD-INFO.json`
naming the herdr commit, `SHA256SUMS`, and a bundle per file that verifies against
`.github/workflows/herdr-nightly.yml`. The newest seven are kept. ⛔ A nightly is
always a prerelease and never a `wsl-toolkit-v*` tag, so a lookup for this tool's
releases skips it.

⚠ **herdr's four builds make a release about 65 MiB larger**, and a consumer that
takes the whole release takes all of it. The steps below take only the executable
for the host's architecture and `SHA256SUMS`, which is what a consumer should do.

Use an immutable `wsl-toolkit-v*` release. Download the executable matching the
host architecture and `SHA256SUMS`, verify the executable's SHA-256, then
verify its keyless bundle against `.github/workflows/release.yml` in
`Azathothas/ToolKit`. A digest proves transport integrity; the signature binds
the asset to this repository's release workflow.

```powershell
$want = (Select-String -Path SHA256SUMS -Pattern 'wsl-toolkit-windows-amd64.exe').Line.Split(' ')[0]
(Get-FileHash wsl-toolkit-windows-amd64.exe -Algorithm SHA256).Hash.ToLowerInvariant() -eq $want
cosign verify-blob --bundle wsl-toolkit-windows-amd64.exe.cosign.bundle --certificate-identity-regexp '^https://github\.com/Azathothas/ToolKit/\.github/workflows/release\.yml@' --certificate-oidc-issuer https://token.actions.githubusercontent.com wsl-toolkit-windows-amd64.exe
```

⛔ **RUN THAT COMMAND FROM POWERSHELL OR cmd ON WINDOWS, NOT FROM GIT BASH.** Git
Bash rewrites an argument that looks like a path, and the identity regex above is
full of `\.`, so it arrives at cosign as `^https://github/.com/...release/.yml@`
and the verification fails with `no matching CertificateIdentity found`. ⚠ **The
signature is fine and the command is right**; the shell changed it in between.
PowerShell answers `Verified OK`, and so does Git Bash with
`MSYS2_ARG_CONV_EXCL='*'` set for the call:

```bash
MSYS2_ARG_CONV_EXCL='*' cosign verify-blob --bundle ASSET.cosign.bundle --certificate-identity-regexp '^https://github\.com/Azathothas/ToolKit/\.github/workflows/release\.yml@' --certificate-oidc-issuer https://token.actions.githubusercontent.com ASSET
```

⚠ **The identity ends at the `@` on purpose.** A release cut by pushing a tag
signs as `release.yml@refs/tags/wsl-toolkit-vX.Y.Z` and one republished by
workflow dispatch signs as `release.yml@refs/heads/main`. Both are this
repository's own workflow; pinning the ref would refuse half of its own releases.

Never pipe a download into a shell. Save it, verify it, then run it.

⚠ **The manual on `main` describes the executable built from `main`.**
`wsl-toolkit version` names the release you hold, and `wsl-toolkit man --no-pager`
is that release's own manual: a command the page names that your release does not
register arrived in a later one.

⚠ **A pinned consumer does not get a fix by its being merged here.** Pinning
protects a consumer from a change it did not review, which is exactly why it also
withholds a fix it would have wanted.

---

## Before changing a fetched contract

1. Read this register and name every affected row in the work record.
2. Update ToolKit's current manual, release checks and consumer smoke in the
   same change.
3. Leave migration to each consumer and tell it to read the latest manual.
4. Preserve prior events only in
   [`HISTORY/consumers.md`](HISTORY/consumers.md), not in this live page.
