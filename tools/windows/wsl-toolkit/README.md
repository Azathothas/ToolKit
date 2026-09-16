# wsl-toolkit, the executable

`wsl-toolkit` is the only product published from this repository. Run
`wsl-toolkit man` for the generated manual, `wsl-toolkit man --no-pager` for
plain text, or `wsl-toolkit examples` for canonical commands.
[`wsl-toolkit.md`](wsl-toolkit.md) is the operator guide.

This page is for maintainers.

## Source shape

```text
tools/windows/wsl-toolkit/
  go.mod                    dependency-free module
  main.go                   command registry, global flags and output contract
  cmd_*.go                  thin command surfaces
  helper_route.go           direct/helper routing decision
  internal/toolkit/         WSL, distros, containers, the relay, state and safety
  internal/toolkit/testdata a log the retired PowerShell product recorded
  adapters/                 what base ensure installs for agents; see adapters/README.md
  internal/toolkit/adapters generated copy of adapters/, which the gate compares
  internal/toolkit/shipped  generated copy of the scripts/common/ files the
                            executable CARRIES, which the gate compares
  acceptance.ps1            real-Windows acceptance runner
  consumer.ps1              published-release smoke runner
  wsl-toolkit.1             generated man page
```

⭐ **Two of those directories are generated and neither is edited by hand.**
`sh scripts/common/check.sh adapters --fix` and `sh scripts/common/check.sh
shipped --fix` write them, and the gate's `adapters` and `shipped` rules compare
each file byte for byte. [`../../../TODO/RULES.md`](../../../TODO/RULES.md)
section 4 lists all four generated halves.

⛔ Keep the module on the Go standard library. There is no `go.sum`, no
dependency graph to audit, and no network fetch required to build it.

The product version has one home:
[`internal/toolkit/version.go`](internal/toolkit/version.go). Release tags,
helper compatibility and both version outputs read that constant.

## Build and local proof

```powershell
Set-Location tools/windows/wsl-toolkit
go build -trimpath -o ../../../.tmp/wsl-toolkit.exe .
go test ./...
go run . man --output wsl-toolkit.1
```

The repository gate runs formatting, vet, build and tests on every Go module:

```powershell
pwsh -NoProfile -File scripts/common/check-go.ps1
```

⚠ **On Windows, run the Go tests with `TEMP` and `TMP` at the 8.3 short form of
a directory under `.tmp`.** From the repository root:

```powershell
New-Item -ItemType Directory -Force .tmp\go-tmp | Out-Null
$env:TEMP = (New-Object -ComObject Scripting.FileSystemObject).GetFolder((Resolve-Path .tmp\go-tmp).Path).ShortPath
$env:TMP = $env:TEMP
```

⚠ **A green local run is not CI.** Before a push, run the Linux half of the
suite and CI's ShellCheck in containers on the base, which carries the same
images CI's jobs use:

```powershell
.tmp/wsl-toolkit.exe run --image docker.io/library/golang:1.25 --container-lifecycle ephemeral --timeout 30m --workspace . --exclude .tmp --exclude .codegraph -c 'git config --global --add safe.directory "*" && cd /work && sh scripts/common/check-go.sh'
.tmp/wsl-toolkit.exe run --image docker.io/library/ubuntu:24.04 --container-lifecycle ephemeral --timeout 30m --workspace . --exclude .tmp --exclude .codegraph -c 'apt-get update -qq && apt-get install -y -qq shellcheck git >/dev/null && git config --global --add safe.directory "*" && cd /work && shellcheck --version && git ls-files -z "*.sh" | xargs -0 shellcheck -s sh'
```

Both run from the repository root, with the executable built above rather than
an installed release, which may not carry every flag they pass. The first is
CI's `go` job on its Linux host, over every module, and the second is the
ShellCheck CI's `checks` job installs.

Add a test for every refusal and a row in
[`../../repo/mutations.json`](../../repo/mutations.json) for every new guard
whose removal could make the suite pass falsely. `repo mutate` proves those
rows by deleting one guard at a time in a copy; a row whose test skips on this
host is reported as skipped, and CI's ubuntu job proves it.

## What needs a real host

Unit tests cannot prove calls to `wsl.exe`, a real distribution, or the
container engine. Build a temporary executable and run:

```powershell
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1 -Binary .tmp/wsl-toolkit.exe
```

The runner inventories pre-existing distributions, uses isolated state, and
checks the names again at teardown. Its throwaway-distribution cases import and
remove their own distribution under a state directory of their own, and its
provider-profile cases build `wsl-toolkit-accp` the same way, granting it a
checkout under the runner's `.tmp` scratch directory, and remove it. Never
point it at `wsl-toolkit-muse`, `podman-machine-default`, or another
distribution it did not create.

For a focused distro change, use a separately named `eph-*` distribution and an
isolated `--home`. Exercise success, failure, deadline, ownership refusal,
snapshot and reuse, and cleanup. When the command channel or the relay changes,
also prove on a real distribution:

- a command that reads stdin, and every quoting hazard through `--command-base64`;
- raw and rendered live output, the stdout and stderr split, colour, and the
  uncoloured stream log;
- redaction before the live, text and event sinks;
- progress consumption, heartbeats with the distribution's state and disk, and
  the escalation notes;
- replay and compare over `wsl-toolkit-event/1`, including the log in
  `internal/toolkit/testdata`;
- that `--dry-run` leaves the registration, the state directory and the sink
  paths unchanged.

## Release

```powershell
pwsh -NoProfile -File scripts/common/repo.ps1 release
pwsh -NoProfile -File scripts/common/repo.ps1 release --publish
```

The default is read-only. It refuses a failing gate, a dirty tree, a checkout
not on `main`, a version declared other than exactly once, a remote whose fetch
or push URL is not `github.com/Azathothas/ToolKit` or carries a credential, a
HEAD that is not that remote's live `main`, and a tag that already exists
locally or remotely. `--publish` creates and pushes the annotated tag only after
those checks pass, then reads the tag back from the remote: a push that errored
after the remote accepted it counts as published, and the local tag is removed
only when the read-back proves the remote has none.

[`release.yml`](../../../.github/workflows/release.yml) checks the tag in a
clean checkout, runs the shared Go proof, cross-compiles amd64 and arm64 Windows
executables with `-trimpath`, runs the staged amd64 binary, writes
`SHA256SUMS`, signs every published file, verifies every bundle, and publishes
the release. [`release-smoke.yml`](../../../.github/workflows/release-smoke.yml)
downloads that release into a temporary directory and drives it as a consumer,
and does it again weekly against whichever release is the latest.

The executables are Windows only because the operational commands drive
`wsl.exe`. The module still builds and tests on Linux so host-dependent path
logic is caught before release.

⭐ **From `wsl-toolkit-v3.0.0`, the release also carries herdr's newest stable release**, which
[`herdr-build.yml`](../../../.github/workflows/herdr-build.yml) builds for Windows
`x86_64` and `aarch64` and Linux `x86_64` and `aarch64`, and `release.yml` covers with
its `SHA256SUMS` and signs. The same workflow builds herdr's development branch for
[`herdr-nightly.yml`](../../../.github/workflows/herdr-nightly.yml), which publishes a
prerelease on a `herdr-nightly-*` tag and keeps the newest seven. `WSL-90`. ⚠ Neither
had published anything as of 2026-09-15: `herdr-build.yml` is dispatched by hand only,
and it publishes nothing by design.

## Related

- [`wsl-toolkit.md`](wsl-toolkit.md): operator guide
- [`../../../docs/consumers.md`](../../../docs/consumers.md): external contract
- [`../../../docs/methodology/gate.md`](../../../docs/methodology/gate.md):
  completion gate
- [`../../../docs/methodology/reviews.md`](../../../docs/methodology/reviews.md):
  three review lenses
