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
reads it again. ⭐ A row names the commit it was read at, and the file and line of
each call. `DOC-15`

| consumer | current relationship | what it must do before moving |
| --- | --- | --- |
| `Azathothas/TEMPLATE` at `a970fe5`: `docs/containers.md:46-52` and `:85-92`, and `docs/agent-tooling.md:78-83` | names the tool and links the three skills on `main` by URL. It describes no flag, subcommand or version of the tool | nothing for `wsl-toolkit-v8.0.0`. A skill renamed or moved breaks its links |
| `Azathothas/bootroot` at `3ea1187`, `Azathothas/errand-box` at `57231da`, `Azathothas/dht-research` at `a65b63a` and `Azathothas/b-ids` at `8689c1e`, each at `docs/containers.md` and `docs/agent-tooling.md` | carry a copy of TEMPLATE's two pages. errand-box's describe the tool as TEMPLATE's do (`agent-tooling.md:75`), and link the skills on `main` (`containers.md:88-90`). bootroot's link the skills on `main` too (`containers.md:70-71`). Its `experiments/10-reconciled-docs.sh:99-102` and `:147-148` check those links and run `wsl-toolkit --version`. bootroot's, dht-research's and b-ids' pages are older. Each describes a tool that makes a throwaway distribution and removes it, at `agent-tooling.md:73`, `:65` and `:123` in that order. bootroot's `containers.md:70` says the skill runs a job in a throwaway machine | nothing for `wsl-toolkit-v8.0.0`. A skill renamed or moved breaks bootroot's and errand-box's links, and bootroot's check. bootroot, dht-research and b-ids rewrite their two pages from the latest manual |
| `Azathothas/bit-cli` at `cce8131`: `scripts/wsl-tool.ps1:83-141`, `scripts/toolkit-pin.json` and `docs/containers.md:46-50` | its wrapper runs the deleted launcher at the pinned `8efe6e0`, with SHA-256 pins for the launcher and the tool. Its guide moves the pin by resolving `main` and fetching `wsl-ephemeral.ps1` by raw URL there, which answers 404 | keep its current pin until it chooses to move, then choose an executable release, verify it, and translate its own invocation from the latest manual |
| `Azathothas/podbox` at `452d792`, every call with `--instance podbox`: `scripts/windows/run-in-base.sh`, `scripts/dev-lane.sh`, `crates/podbox-gate/src/main.rs:3213-3216` and `dev.rs:622` and `:933-951`, `scripts/windows/kvm-watchdog.py` and `kvm-guest.py`, `experiments/384-windows-lane-v6.sh:26-90`, and `docs/containers.md:47-48`, `:95-97` and `:120-122` | `run --image --workspace . --container-lifecycle ephemeral`, eight `--exclude`, `--script`, `--timeout` and `--tick` (`run-in-base.sh:146-172`). It adds `--input job.sh=FILE` for a caller's job, and `--artifacts DIR` where asked. It exits with the tool's code (`:173-174`). The lane passes the same, with `--max-bytes`, `--user 0:0`, two `--input` and `--json` (`dev-lane.sh:262-285`). It exits with that code too (`:336`). `gc`, `gc --apply` and `gc --job ID --apply`, counting `host dir` lines and the id in `gc`'s text (`dev-lane.sh:92-103`, `:298-299`). `gc --json`, reading `wsl-toolkit-cleanup/1` and `dry_run` (`main.rs`). `base status --probe` (`dev-lane.sh:116`, `dev.rs:933-938`), and `podbox-dev session` runs `base ensure --probe` (`dev.rs:946-951`). `base exec -c`, `base exec --detach --json --script`, `wait ID --timeout`, `stop ID` and `logs ID --follow` (`kvm-watchdog.py:83-262`, `kvm-guest.py:88-121`). Without `--unattended`, `kvm-guest.py:97-100` runs `base exec --timeout 65m --script` and waits on it. Its Windows-lane proof runs three jobs through `run-in-base.sh`, then `gc --job ID --apply` for each, and fails on any exit but 0 (`384-windows-lane-v6.sh:77-85`). `resources` (`containers.md:120`) and `version` (`dev.rs:622`). The watchdog keeps a file of its own in the tool's state directory (`kvm-watchdog.py:65-70`). It no longer reads the skills by URL or calls `hostaddress` | before `wsl-toolkit-v8.0.0`, read the breaks below. `run`, `base exec`, `wait` and `logs ID --follow` answer 125 where the tool could not run the job. `run-in-base.sh` documents 2 for that (`:32`), and now exits 125. So does the lane, once the answer carries a job id (`dev-lane.sh:17`, `:336`). A refusal before an id still exits 2 there (`:296`). `--tool-exit 2` or `WSL_TOOLKIT_TOOL_EXIT=2` keeps 2. An ephemeral `run` now takes its host record with it, so a `gc --job ID --apply` after it exits 2. `dev-lane.sh:298` discards that exit. ❌ The Windows-lane proof does not, so it reports a mismatch and exits 1 (`384-windows-lane-v6.sh:78-84`). `run-in-base.sh:27` and `docs/containers.md:95-97` say the host record stays until `gc`, and `:122` collects it by id. Neither holds for an ephemeral job. From inside a packaged app, a `base ensure --probe` that would re-provision is refused, and it installs no adapter |
| `Azathothas/podssh` at `3ad3203`: `scripts/dev-wsl.sh` and `scripts/dev.sh` | bridges every call from Git Bash through PowerShell, each argument as base64 (`dev-wsl.sh:200-211`). Its builds run `run --image`, with `--workspace` where the tree goes in. They add seven `--exclude`, `--script` or `--command-base64`, two `--env`, `--container-lifecycle ephemeral` and `--timeout 60m` (`dev-wsl.sh:63-97`). Its `images` runs `run --image --command-base64 --container-lifecycle ephemeral --timeout 10m`, with no workspace (`dev.sh:242-243`). It keeps the seven `--exclude` patterns in a shell string (`dev-wsl.sh:50`). It holds its builds apart with a lock directory of its own (`dev.sh:337`). Its `clean` is `gc --older-than 0s`, which removes nothing, because `gc` reaches every project's jobs (`dev.sh:273`) | read the latest manual. Then call the tool from Git Bash with `MSYS_NO_PATHCONV=1` and `MSYS2_ARG_CONV_EXCL='*'` exported. Pass `--workspace "$PWD"`, `--script -` and `--gitignore`, hold builds with `--exclusive`, and clean with `gc --workspace .`. Before `wsl-toolkit-v8.0.0`, read the exit code break below. `run` answers 125 where the tool could not run the job |
| `Azathothas/pg-toolkit`, a private repository, at `cmd/pg-devenv` | calls an installed executable with `--instance pg-toolkit`: `version`, `config --json`, `config --write`, `base ensure` or `base recreate`, `base exec` with `--root` for another account, `base shell`, `base remove --yes`, `base status`, `resources` and `selfupdate --check`. On a failed ensure it prints `base ensure --repair` as the fix. `pg-devenv` exits with the tool's code. Its guest scripts share the tool's run directory and match a distribution named `wsl-toolkit-*` | before `wsl-toolkit-v8.0.0`, read the breaks below. `base exec` and `base shell` answer 125 where the tool could not run the command, and `pg-devenv` passes that on where its contract reads 2. `--tool-exit 2` or `WSL_TOOLKIT_TOOL_EXIT=2` keeps 2. From inside a packaged app, `base remove --yes` and `base recreate` are refused. So is an `up` whose `base ensure` would re-provision, and the `base ensure --repair` it prints. A grant whose directory is gone makes `base status` exit 1 where `config` refused before |
| `Azathothas/agent-box` at `369613d`: `xtask/src/linux.rs`, `xtask/src/linux/args.rs`, `xtask/src/doctor/host.rs` and `docs/development.md` | `cargo xtask linux` starts `wsl-toolkit` from `PATH` with no shell between (`linux.rs:102-103`). Its arguments are `run --image IMAGE --workspace DIR`, five `--exclude`, `--env CARGO_BUILD_JOBS=N`, `--container-lifecycle`, `--timeout 60m` and `--command-base64` (`args.rs:149-176`). It adds `--device /dev/kvm` where asked. It exits with the tool's code (`linux.rs:161-165`). `cargo xtask doctor` runs `version`, and with `--kvm` the same `run` with no workspace (`host.rs:44`, `:180-181`). Its guide cleans with `gc --older-than 24h`, then with `--apply` (`development.md:359-360`) | before `wsl-toolkit-v8.0.0`, read the exit code break below. A refusal it passed on as 2 now reaches its caller as 125. Its guide says 2 means it could not start the tool (`development.md:83`). `--tool-exit 2` or `WSL_TOOLKIT_TOOL_EXIT=2` keeps 2. `--gitignore` can shrink its workspace copy |
| `Azathothas/dash.vpn.ajam.dev`, a private repository, at its operations guide | a recipe an operator follows from PowerShell. It names `run` with `-c`, `--workspace` and `--exclude`, `base shell`, `base exec -c`, `distro new --image --ephemeral -c`, `stop ID`, `gc --apply` and `base remove --yes`. Also `doctor`, `ready`, `ready --smoke`, `base status --probe`, `man --no-pager` and `examples`. Its table names the base distribution `wsl-toolkit` | before `wsl-toolkit-v8.0.0`, read the breaks below. `run`, `base exec`, `base shell` and `distro new -c` answer 125 where the tool could not run the command. A new host's default base is `wsl-toolkit-jobs`, and `base remove --yes` is refused from inside a packaged app. Its rule against Git Bash can become the two variables the manual names |
| `Azathothas/tcp.ts.relay.ajam.dev`, a private repository, at its client build script | prints the version of each Linux binary it builds by running it with `wsl.exe -d` in the distribution `WSL_DISTRO` names, `wsl-toolkit` by default, through automount at `/mnt/`. It runs as the base's default account, `base.user`, not root (`DOC-15`). A failure is silent, and the script prints that the binary is not runnable | on a host whose default base `wsl-toolkit-v8.0.0` built, set `WSL_DISTRO` to the `name` that `base status --json` reads, `wsl-toolkit-jobs`. A base with automount off cannot reach the binaries |
| `pkgforge-dev/cross-libc-dlopen` at `586e61b`: `scripts/wsl-ephemeral.ps1` | carries a vendored copy and fetches nothing from ToolKit | nothing here can update it; its owner decides whether to replace the copy |

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

### ⚠ What the private ranges and herdr's adapters change, and what a caller does

`WSL-159` and `WSL-158`. In `wsl-toolkit-v9.0.0`. The first row fixes a false pass, a
break by the table above. The others change what a base holds, and no flag or exit.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| `base exec --private-net` and `base shell --private-net` refuse `100.64.0.0/10` beside the private ranges. Tailscale gives the Windows host an address there | reaches a Tailscale peer, or another address in that range, from a private namespace | runs that step without `--private-net` |
| a base with the `muse` and `herdr` adapters gets `~/.config/herdr/agent-detection/muse.toml`, which herdr reads before its own Muse rules. herdr then reports Muse panes from it | reads herdr's Muse pane states, and relies on herdr's own rules for them | reads `herdr_detection_rules` in `base status --probe --json`, or replaces the file with one of its own, which is then left alone |
| `base ensure` installs `herdr` before every other adapter, wherever `base.adapters` lists it | reads the order of the adapter lines in the ensure's log | reads each adapter's own line |

⚠ **A base without `private_net` answers what it answered before.** herdr's probe
prints the namespace's facts only where the key is or was used.

### ⛔ What the canonical default base name breaks, and what a caller does

`WSL-167`, ruling 33. In `wsl-toolkit-v8.0.0`. A break by the table above, so the release
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

⚠ **Two rows name `wsl-toolkit` as a distribution, and none names the instance
`base`.** Each row was read again for `DOC-15`. tcp.ts.relay's build script runs its
binaries with `wsl.exe -d wsl-toolkit`. Where no base has that name, it prints that
they are not runnable and goes on. dash.vpn's guide names its base distribution
`wsl-toolkit`.

⚠ **From inside a packaged app, the third row above reaches three rows.**
pg-toolkit's `base remove --yes` and `base recreate`, and dash.vpn's `base remove
--yes`, are refused there. So is a `base ensure` that would re-provision, which
pg-toolkit's `up` and podbox's `podbox-dev session` both run. So is the `base ensure
--repair` that pg-toolkit prints as its fix. No adapter is installed from there.

### ⚠ What the instance prefix changes, and what a caller does

`WSL-193`. In `wsl-toolkit-v8.0.0`. An existing instance keeps its name, so no working call
changes meaning for it.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| a NEW instance named `wsl-NAME` or `wsl-toolkit-NAME` is made as `NAME`, distribution `wsl-toolkit-NAME`, with one warning on stderr. `wsl-`, `wsl-toolkit-`, `wsl-toolkit`, `wsl-base`, `wsl-jobs` and `wsl-auto` are refused | creates such an instance on a new host and then names its distribution `wsl-toolkit-wsl-NAME` directly | reads the name from `base status --json`, or passes `--literal-instance` |

### ⚠ What an ephemeral job's record changes, and what a caller does

`WSL-121`. In `wsl-toolkit-v8.0.0`. The help always said an ephemeral job leaves nothing behind;
now its host record goes too, once its answer is written.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| an attached ephemeral job removes its host record when it ends. Its answer is kept for 10 minutes, so `wait ID` and `logs ID --follow` still answer its verdict | reads `logs ID`, `inspect ID` or `gc --job ID` for an ephemeral job long after it ended | reads the answer `run` printed, or runs the job without `--container-lifecycle ephemeral`, which keeps the record |

⚠ **The row it reaches, read again for `DOC-15`.** podbox runs each `run` ephemeral,
then `gc --job ID --apply`, which now exits 2. Its lane discards that exit, and its
Windows-lane proof fails on it. agent-box and podssh run ephemeral jobs too, and read
nothing of one after it ends.

### ⚠ What an empty `/out` changes, and what a caller does

`WSL-124`. In `wsl-toolkit-v8.0.0`.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| `run --artifacts DIR` makes `DIR` with the first entry that arrives, so a job that leaves `/out` empty makes no directory, on the direct route and through the helper | tests that `DIR` exists to learn that the job ran | reads `artifacts` in the `--json` answer, which is 0, or makes `DIR` itself first |

⚠ **No row is reached, each read again for `DOC-15`.** podbox passes `--artifacts`
from two scripts, and each makes the directory first (`run-in-base.sh:158`,
`dev-lane.sh:266`). No other row passes it.

### ⛔ What a project lending every `jobs` key breaks, and what a caller does

`WSL-131`, ruling 34. In `wsl-toolkit-v8.0.0`. A break by the table above, so it goes into
`wsl-toolkit-v8.0.0`.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| under a named instance with its own file, a working tree's `wsl-toolkit.json` lends every `jobs` key it writes. An instance whose own file sets `container_lifecycle`, `platform`, `workspace`, `exclude` or `gitignore` now takes the project's value | runs a named instance from a working tree whose file writes one of those keys, and relies on the instance's own value | removes the key from the project's file, or names the file to read with `--config` |
| a key the project writes is taken as written, zero included. A `max_running`, `max_bytes` or `max_entries` of `0`, an `env` of `{}` or a `cache` of `[]` now replaces the instance's own value | writes such an empty value in a project's file and relies on the instance's own cap, limits, environment or caches | removes the key from the project's file, or names the file to read with `--config` |
| a project file that writes only `container_lifecycle`, `platform`, `workspace`, `exclude` or `gitignore` is now read strictly under a named instance. A broken file, or one a newer build wrote, now refuses | runs a named instance from a working tree whose file was ignored before and is broken or newer | fixes the project's file, or names the file to read with `--config` |
| `config --json` drops `project_held` and carries `project_lends`, each key the project lent. `config`, `run` and `matrix` print no ⚠ line about held keys | parses `project_held` or that line | reads `project_lends` |

### ⚠ What a pull's sized stall changes, and what a caller does

`WSL-176`. In `wsl-toolkit-v8.0.0`. No exit code changes for a call that names no new
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

`WSL-180`. In `wsl-toolkit-v8.0.0`.

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

`WSL-187`. In `wsl-toolkit-v8.0.0`.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| under Git Bash, `--home /d/x` and `WSL_TOOLKIT_HOME=/d/x` name `D:\x`. They named `x` under the current drive's root, such as `C:\d\x` | keeps jobs, a ledger, locks or grants in that second root | moves what it needs to `D:\x`, or passes the old root's Windows spelling |
| under Git Bash, `/tmp/x` names `x` in the temporary directory, and any other `/...` is under the Git install, such as `C:\Program Files\Git\srv\x`. They named `C:\tmp\x` and `C:\srv\x` | keeps its state under such a root | passes the directory's Windows spelling |
| an agent's launcher, such as `muse.exe`, reads `WSL_TOOLKIT_HOME` the same way, and hands it on absolute | started a launcher from Git Bash and kept its state in the second root | moves what it needs, or passes the Windows spelling |
| `WSL_TOOLKIT_HOME` is handed on absolute, so the helper and a detached child read the directory their parent read | | nothing |

### ⚠ What reading `WSL_TOOLKIT_CACHE` the MSYS way changes, and what a caller does

`WSL-195`. In `wsl-toolkit-v8.0.0`.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| under Git Bash, `WSL_TOOLKIT_CACHE=/d/x` names `D:\x`. It named `x` under the current drive's root, such as `C:\d\x`. `/tmp/x` and any other `/...` move as `WSL-187` says for the home | keeps a fetched image, such as the BSD guest, in that second directory | moves it to `D:\x`, or passes the old directory's Windows spelling |
| `WSL_TOOLKIT_CACHE` is handed on absolute, so the helper and a detached child read the directory their parent read | | nothing |

### ⚠ What reading the host it runs on changes, and what a caller does

`WSL-188`. In `wsl-toolkit-v8.0.0`.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| the native platform is the machine's. The amd64 build on an ARM64 Windows runs a job that names no platform as `linux/arm64`, and `selfupdate` installs the arm64 build | runs the amd64 build on ARM64 and reads amd64 results | passes `--platform linux/amd64` |
| a host engine that answers for another architecture, through a remote connection or context, is passed over with its host named. So is docker in Windows-containers mode. Where no other engine answers, the build exits 2 | built through such an engine, whose rootfs ran nothing | points the engine at this machine, or switches docker to Linux containers |
| each engine but the last one installed has 20 seconds for `info`, not 90. That is podman while docker or `wslc` is installed, and docker while `wslc` is. `wslc` ships with WSL 3.0.1 and later | starts a docker daemon or a podman machine and builds at once | starts it first, or builds again |
| `base ensure --json`, `base recreate --json` and `ready --ensure --json` add `built_with`, the engine a build used and the host it answered from. Their human output adds a `built with` line. The helper's answer carries both | treats every field or line as one it knows | ignores what it does not know |
| `doctor --json` reports the machine's `host.arch`. The amd64 build on an ARM64 Windows reports `aarch64`, where it reported `x86_64` | reads `host.arch` as the build it runs | reads the executable's own architecture elsewhere |
| a build's export container carries `wsl-toolkit.export=1`. `resources --host-engine --json` adds `export_containers`, and the command list adds a `container prune` | treats every field or command as one it knows | ignores what it does not know |
| the podman hint tells a stopped machine to start, and says nothing of a restart where `podman machine list` did not answer | matches the old hint's words | matches podman's own words |
| `doctor --json` adds `host.wsl_state`. An empty machine reads `wsl_access` true with no distribution, and only a refusal adds the approval-flow note | reads `wsl_access` false as an empty machine | reads `wsl_state` |
| an empty machine in another language lists nothing, where the listing failed | | nothing |
| `base doors` reads `[automount] root` for the drive, `cmd.exe` and PATH doors | reads a hand-set root's drives as closed | nothing |
| `root = /` is kept, so its drives are found at `/c`, where `/mnt` was searched. The verifier counts them, and an adopted base with such drives no longer passes `automount: off` | adopted a base with `root = /` and set `automount: off` over its mounted drives | lets `base ensure` turn the drives off, or sets `automount` to what they are |
| `interop.windows-path` is open only for a directory on a drive, one letter below the root. Another directory below the root on PATH reads closed | reads such a directory as a Windows one | nothing |
| each preset's figure in `base preset list` names the `.wslconfig` it was measured under | matches the figures' words | reads the id and the reference |
| an adapter installs a missing package with `pacman -Syu --needed`, which upgrades the whole base | relies on an adapter leaving the base's other packages at their versions | pins what it needs in the image |
| the account takes the next free uid where the image holds 1000, and its `/etc/subuid` and `/etc/subgid` ranges start past every other. A build that refused now succeeds | | nothing |
| a grant's fstab line names the account's own ids, and the verifier refuses a grant mounted for another uid. An adopted base whose account is not 1000 is re-provisioned at its next ensure, and restarted | holds such a base with live grants | runs `base ensure --repair` when a restart is acceptable |
| a live grant is read back through `su` where there is no `runuser`, as on Alpine. A read-back that runs no shell as the account refuses the grant in either mode, and names the tool that ran and its exit | relies on a read-only grant passing where nothing could read it back, or matches the refusal's words | matches `could not be read back` |
| a read-only grant on Alpine is remounted read-only after its mount, where it was refused | | nothing |
| provisioning's engine settings are drop-ins, and the main files are left. The account's `containers.conf` an earlier release wrote goes | edited that tool-written file by hand | moves its settings to the main file or a drop-in of its own |
| a sealed base keeps `/mnt/wsl` mounted for up to ten seconds at each start, while it waits for WSL's shared resolver. The verifier waits up to fifteen seconds for the unmount, because WSL does not wait for a `[boot]` command | relies on `/mnt/wsl` being closed the moment a start returns | waits for the boot log's `shared tmpfs unmounted` line, or for `fs.mnt-wsl-shared closed` in `base doors` |
| a sealed base that cannot resolve `docker.io` is restarted by `base ensure`, and not re-provisioned. One that still cannot resolve after the restart exits 2, named by its resolver, and is kept | relies on that re-provisioning | runs `base ensure` again once the host's network answers |

### ⛔ What the tool's own exit code breaks, and what a caller does

`WSL-127`, ruling 35. In `wsl-toolkit-v8.0.0`. A break by the table above, so it goes into
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

⚠ **The rows it reaches, each read again for `DOC-15`.** agent-box, podbox and podssh
call `run`, and podbox also `base exec`, `wait` and `logs ID --follow`. dash.vpn's
guide calls `run`, `base exec`, `base shell` and `distro new`. pg-toolkit calls `base
exec` and `base shell`. ⚠ agent-box, podbox, podssh and pg-toolkit exit with the tool's
code, so their own callers read 125 too. Four of their contracts read 2 as could not
run: agent-box's `development.md:83`, podbox's `run-in-base.sh:32` and
`dev-lane.sh:17`, and pg-toolkit's.

### ⛔ What `text-tool eol` refusing a missing path breaks, and what a caller does

`WSL-166`, ruling 35. In `wsl-toolkit-v8.0.0`. `text-tool` ships with `wsl-toolkit`, so it
goes into `wsl-toolkit-v8.0.0`.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| `eol` given a path that does not exist exits 1 and writes nothing, to that path or to any file named beside it, with or without `--dry-run`. 7.x converted the others, said `missing`, and exited 0 | names a path that may not exist and reads exit 0, or relies on the files beside it converting | names only paths that exist, or runs one `eol` per path and reads each exit |
| the stderr line `text-tool: N path(s) do not exist, and eol created nothing there` is gone. The refusal says `N of M path(s) do not exist` | parses that line | reads the exit code, or `missing` in `--json` |

### ⭐ What the base runtime adds, and what a caller reads

`WSL-126`, `WSL-130` and the limits in `WSL-136`. In `wsl-toolkit-v8.0.0`. No exit
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

`WSL-137`, `WSL-143`, `WSL-147` and `WSL-150`. In `wsl-toolkit-v8.0.0`.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| `base status --json` adds `probed`, `account.passwd`, `account.login_shell` and `account.bash`, and the advisories `account-passwd`, `account-cannot-start` and `login-shell-not-bash` | | reads them, or ignores them |
| an owner's `base ensure` moves the managed account's login shell from `/bin/sh` to bash where the guest has bash | runs scripts in the base that rely on the account's login shell being `sh` | names its shell, as `base exec` scripts already do |
| `bootstrap.sh` puts the account's prefix after the system path and rewrites the line an earlier run put first, so a name that exists in both resolves to the system path's | relies on the account's own copy of a name shadowing the system's | runs that copy by its full path |
| `base ensure` refuses an interrupted build and names `base remove --yes`, where it removed and rebuilt it | expects ensure to recover an interrupted build alone | runs `base remove --yes`, then `base ensure` |

### ⭐ What a restart of WSL changes, and what a caller reads

`WSL-170`, `WSL-178` and `WSL-184`. In `wsl-toolkit-v8.0.0`. No field a 7.0.2 document
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

`WSL-190`. In `wsl-toolkit-v8.0.0`. A grant that works today keeps working. ⛔ The helper
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

### ⚠ What a base whose marker cannot be read changes, and what a caller does

`WSL-183`. In `wsl-toolkit-v8.0.0`. A removal the caller
asked for replaces a refusal, and a refusal replaces a stamp.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| `base remove --yes` removes an owned base whose identity marker cannot be read, on WSL's registration of its disk in the instance's `base` folder. 7.x refused it, and only `wsl --unregister` removed it | relied on that refusal to keep such a base | runs `base remove --yes` only on a base it means to delete |
| `base recreate`, and a `--preset` switch that rebuilds, rebuild such a base. 7.x refused both | relied on those refusals to keep it | runs neither on a base it means to keep |
| `cat` failing on the marker for any reason but a missing file reads as an unread marker. `base ensure` refuses such a base with exit 2, where 7.x stamped it from this machine's record | relied on that ensure adopting the base | frees the disk from the program holding it, or runs `base recreate` |
| that refusal, and the `base status --probe` problem, add the likely causes and name `base remove --yes` and `base recreate` | matches the problem's words exactly | reads the exit code, or matches `could not be read` |
| the state that refusal prints with `--json`, and `base status --probe --json`, add the remediation `identity-unread`, naming `base recreate` and what it deletes | | reads it, or ignores it |
| `ready` over such a base names `base recreate` first, with `--ensure` or without it. 7.x named `base ensure`, which refused it | runs `ready`'s first remediation without asking | reads the remediation's `costs`, and asks before it deletes a home |
| a probe while another process holds the base's lock names that process, and names neither command | matches the probe's words exactly | runs the probe again once that process ends |
| the marker is read through `/usr/bin/env LC_ALL=C`, so a missing marker reads as missing in any guest language | built a base from an image with no `/usr/bin/env` | adds it to the image, as coreutils and BusyBox do |

### ⭐ What recording the digest adds, and what a caller reads

`WSL-186`. In `wsl-toolkit-v8.0.0`. No exit code changes, and no field is removed.

| change | a caller that | the caller does |
| --- | --- | --- |
| `base status --json`, and the state `base ensure --json` prints, add `digest`. Under `--probe` it is the guest marker's, and otherwise this machine's record's | | reads it, or ignores it |
| `base status` prints a `digest` row naming the image at that digest, where a build of this release recorded one | treats every line of `base status` as one it knows | reads `--json` |
| the guest's `/etc/wsl-toolkit-identity.json` and the instance's `base.json` add `digest` | reads either file | reads it, or ignores it |
| a build prints a `digest:` line, or a ⚠ line saying none is recorded | treats every stderr line of a build as one it knows | reads the exit code, or `--json` |
| a build whose digest could not be read carries the advisory `digest-unrecorded` in its state, with the reason. `--json`, the helper's answer and `ready`'s notes carry it | treats every remediation as one it knows | ignores an advisory it does not know |
| the digest is the repository's, which the registry names for the tag. Where podman also names its platform manifest's, that one is not kept | pins a base by the platform manifest's digest | reads the `digest` row, which names the image at the repository's digest |

### ⚠ What a `--preset` switch through the helper changes, and what a caller does

`WSL-191`. In `wsl-toolkit-v8.0.0`. The helper protocol stays `wsl-toolkit-helper/7`, which
no release speaks yet, and its `base/ensure` request gains `switch`.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| `base ensure --preset` through the helper rebuilds only where the base, read under its lock, is not built from the preset. 7.x rebuilt it every time | relied on a switch through the helper to rebuild a base already built from the preset | runs `base recreate` |
| `--preset` without `--save` through the helper builds the preset's image. 7.x sent the file's image, and built that | relied on the helper building the file's image beside a `--preset` | drops `--preset` |
| a request to `/v1/base/ensure` with both `force` and `switch` is refused with HTTP 400, and nothing changes | sends its own requests to the helper | sends one of the two |
| the client sends `switch` only for a switch, so a plain ensure still reaches a helper built before the field | | needs to do nothing |
| through the helper, a `--preset` switch's stderr line says it asks the helper to switch the base. 7.x said the base rebuilds | treats every stderr line as one it knows | reads the exit code, or `--json` |

### ⚠ What building from the engine's own copy changes, and what a caller does

`WSL-192`. In `wsl-toolkit-v8.0.0`. A build that failed can succeed. ⚠ A rebuild whose pull
fails no longer exits 2 where the engine holds the image, a change by the table above.

| change | it breaks a caller that | the caller does |
| --- | --- | --- |
| a build whose pull fails exports the copy the host engine already holds for this machine's platform. Its state carries the advisory `built-from-local-copy`, naming the copy's digest. 7.x exited 2 and kept the base | relied on a failed pull to stop a rebuild, so a base is only built from the registry's current image | removes the engine's copy before the rebuild, or reads the advisory after it |
| that advisory reaches `--json`, the helper's answer and `ready`'s notes. `run --ensure-base` through the helper prints it, and a direct build prints two ⚠ lines | treats every stderr line, or every remediation, as one it knows | reads the exit code, and ignores an advisory it does not know |
| a refusal for a failed pull adds why the engine's own copy was not used: it holds none, or one for another platform | matches the refusal's words exactly | reads the exit code |

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
