# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-09T05:51:14Z
baseline        bf119301, clean main; fast gate: 14 pass, 1 fail, 2 skipped.
entries         total 53  open 8  blocked 0  done 45
gate            17 checks, one binary, 31s, all passing
```

## Active work

None. [WSL-31](issue-6.md) closed on 2026-09-09 with its acceptance output and
its release evidence pasted underneath it, which resolves
[issue 6](https://github.com/Azathothas/ToolKit/issues/6) in full.
[TOOL-13](tooling.md) closed the same day: the gate is one Go binary and runs in
about 30 seconds rather than 13m20s.

## What this session shipped

`tools/windows/wsl-toolkit`, a Go module with no dependencies that carries
`scripts/windows/wsl-toolkit/wsl-toolkit.ps1` inside itself. It owns exactly one
WSL distribution, runs rootless podman in it, executes container jobs against a
COPY of a workspace, runs the twelve-image fleet, surveys the host, and removes
only what it made. `helper serve` is the second permission path, for a caller
that is refused when it calls `wsl.exe` itself.

The PowerShell half gained the per-uid user environment and a second build
product; `build.ps1` writes both and `-Check` compares both against the parts.
`launcher.ps1` runs the executable by default and keeps the previous behaviour
under `-LauncherKind script`. The release train cross compiles both Windows
targets, asserts the asset count and runs the staged binary before publishing.

## Measurements

Read from the machine, on Windows 11 Pro 26200, on 2026-09-09:

```text
transport   a script on wsl.exe stdin arrives byte exact and its exit code
            propagates. The same payload as an ARGUMENT had its dollar name
            expanded, its backtick EXECUTED, and still reported exit 0.
base build  arch 31s / 940 MiB, alpine 28s / 204 MiB, debian 37s / 556 MiB,
            each to a rootless container answering a marker it could not echo.
fleet       12 of 12 catalog images ran, 0 failed, 0 unreached, in 52.1s with
            cold pulls, artifacts returned from every row.
isolation   a container ran `rm -rf /work/*` and the host workspace is
            byte identical afterwards.
acceptance  23 of 23 cases pass against the real base, both routes, both
            accounts, every catalog image.
gate        17 checks, one binary, 31s on this host, including shellcheck,
            PSScriptAnalyzer, a rebuild of both generated products and the Go
            suite. It was 13m20s as eighteen shell checks with a twin comparison.
release     wsl-toolkit-v1.1.0 published by the workflow, five assets, and
            driven from an empty directory in both launcher modes.
```

Sandbox facts that shaped the work, and that a later session should not have to
rediscover:

- WSL enumeration returns `E_ACCESSDENIED` under a sandbox and succeeds through
  the normal approval path. That difference is why the helper exists.
- PowerShell must be started with `-NoProfile`; profile startup costs seconds
  per invocation.
- Admin bypass is enabled on `main`. No branch protection was changed.

## Decisions

The operator chose BOTH permission paths: direct invocation through normal
agent approval, and an opt-in authenticated local helper. ⛔ The two must carry
the same job flags; a flag one honours and the other drops is a job that ran as
somebody else with nothing said, and that defect was found and fixed in review.

Jobs receive workspace copies and never writable host mounts. Artifact export is
a second, explicit act with per-entry validation. The base is `arch` by default
because it is glibc; `alpine`, `debian` and `fedora` are presets and any fully
qualified reference works where a preset id does.

## Work order

Nothing is queued. The eight open entries in [INDEX.md](INDEX.md) are unrelated
to this one and none of them blocks anything here.

⚠ **What a later session should know before touching this tool.** The two
generated products are the trap: `scripts/windows/wsl-toolkit/wsl-toolkit.ps1`
and `tools/windows/wsl-toolkit/internal/script/wsl-toolkit.ps1` are BOTH built
from the parts, and editing either by hand is lost at the next build.
[RULES.md](RULES.md) section 4 owns that. The acceptance runner needs a real
machine and PowerShell 7; `go test ./...` needs neither and covers the guards.
