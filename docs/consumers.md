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
deleted.** A raw fetch of any file under `scripts/windows/wsl-toolkit/` at a
commit after the deletion returns 404, and a commit before it still serves it.
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
