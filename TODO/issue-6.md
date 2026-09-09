# Issue 6

## WSL-31. A portable entry point for isolated Linux jobs on Windows

**Source** [Issue 6](https://github.com/Azathothas/ToolKit/issues/6) and the
operator's implementation request on 2026-09-09.
**Category** wsl, **Priority** P1, **Effort** XL, **Status** done

## Problem

Callers build wrappers for guest accounts, tool paths, copied build work,
image matrices and abandoned resources. A writable host mount lets a guest
delete the checkout. Restricted agents hit executable-shim and WSL errors.

## Premise

Measured: the current doctor throws on an inaccessible executable candidate;
sandboxed WSL enumeration is denied while the approved call succeeds. The
existing bundle already supplies base64 transport, user selection, output
relay and guarded distro lifecycle. Extend those existing paths.

The operator-authorized pg-devenv reference supplies useful per-user environment
and mirror ideas. Its writable drive mount and best-effort backup do not provide
the isolation required for this runner.

## Approach

Extend the PowerShell parts and rebuild the bundle. Add a Go command embedding
the product, with native diagnostics, an owned WSL base, validated workspace
archives, isolated container jobs, the twelve-image catalog, bounded fleet
execution, ownership records and cleanup. Direct and helper jobs share code.
Build binaries in the existing release train. Default the launcher to the
binary and retain explicit script mode and the legacy interface.

## Decision

On 2026-09-09 the operator chose both normal agent approval and an opt-in local
helper. The helper authenticates local clients and accepts isolated jobs; host
commands, host paths and arbitrary engine options are outside its protocol.
The two existing machines named by the operator are excluded from all tests.
Workspace copies replace host mounts; exports and deletion validate containment.

## Consumers

Pinned script consumers retain the script interface. Binary-first launching is
a changed default and needs a consumer-register and changelog entry. Other
repositories' pins remain untouched under the operator's repository boundary.

## Prove

Run the rebuilt PowerShell suite and surface lock, native tests and complete
repository gate. Commit an acceptance runner covering both accounts, all twelve
images, direct/helper execution, hostile paths, checkout survival, failures,
deadlines and cleanup. Record actual commands and results before closing.
Download the CI release, verify digests and exercise both launcher modes from
an otherwise empty directory. Complete door, mutation and claim reviews.

## Closing

**Closed 2026-09-09T13:10:00Z.** `tools/windows/wsl-toolkit` is a Go module with
no dependencies that carries `wsl-toolkit.ps1` inside itself, owns one WSL
distribution with a rootless podman in it, runs container jobs against a copied
workspace, runs the twelve-image fleet, surveys the host, and removes what it
made. The PowerShell parts gained the per-uid user environment and the second
build product; the launcher runs the executable by default and keeps the script
under `-LauncherKind script`; the release train cross compiles both Windows
targets, asserts the asset count and runs the staged binary before publishing.

The acceptance runner, on this machine, against the real base:

```text
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1 -Binary .tmp/wsl-toolkit.exe

acceptance: .tmp\wsl-toolkit.exe
scratch:    .tmp\acceptance.33a644fb
untouched:  eph-pgb, podman-machine-default

  ok    the executable reports the version its embedded script declares
  ok    the survey runs, creates nothing, and separates installed from callable
  ok    the catalog is twelve fully qualified references
  ok    the base is registered and can actually run a container
  ok    the presets each carry a measurement or say they carry none
  ok    a container job runs as the image default and returns its output
  ok    a container job runs as another account and can still write /out
  ok    a failing command returns its own exit code, not a flattened one
  ok    a command past its deadline is killed and reports 124
  ok    a container that destroys its workspace leaves the host copy intact
  ok    a hostile artifact name is refused rather than written outside
  ok    a workspace over its ceiling is refused rather than truncated
  ok    an unqualified image reference is refused by name
  ok    this tool refuses to act on a distribution it does not own
  ok    every catalog image runs the same command and returns an artifact
  ok    a selector that matches nothing is a refusal rather than an empty run
  ok    a job runs through the local helper and its artifacts come back
  ok    both paths agree about which account a job runs as
  ok    the helper is gone once it is stopped
  ok    the embedded script runs and reports the distributions it may touch
  ok    the embedded script puts one address on stdout and nothing else
  ok    cleanup removes what this tool made and the counts return to zero
  ok    the two pre-existing distributions are still registered and untouched

acceptance: 23 case(s) passed against a real machine.
```

⚠ **The first full run of that suite failed eight cases and the tool was right
every time.** `Start-Process -ArgumentList` joins the array it is given and
re-quotes it, so `-c 'printf "uid=%s" "$(id -u)"'` arrived as several arguments
and `exit 37` arrived as `-c exit` plus a stray `37`. The runner now passes each
argument through `ProcessStartInfo.ArgumentList`, which needs PowerShell 7 and is
guarded for it. ⛔ A harness defect that reads as a subject defect is worth the
line: eight red cases were about to be debugged in the wrong file.

### What the three review lenses found

**Lens 1, the door sweep.** Every affordance, then every surface that reaches
it. Three findings, each a second door not enforcing what the first did:

- `--user` was on the direct route and dropped by the helper route. The flag was
  accepted, the job ran as root, and nothing said so. It is on the wire now, and
  one acceptance case asks both routes the same question and compares the
  answers, which is the only shape that catches a field dropped between them.
- `launcher.ps1` decided "this was a verification failure, do not fall back"
  by matching the error's WORDING. Any new throw site, or a reworded one, would
  have fallen back to the PowerShell product after a check that had just refused.
  It carries a type now, and both directions are proved.
- A named release lost to a stale `wsl-toolkit.exe` beside the launcher, which
  is the defect the script half already fixed and recorded on 2026-08-30. And
  `-LauncherSha256` was ignored on all three paths that name a file rather than
  fetching one, which is `launcher.md`'s own promise not being kept.

**Lens 2, the guard mutation.** Eleven guards, each with its protection deleted
and the named case run. Ten refused. ⛔ **One was theatre:** deleting the
protected-distribution list left `TestOnlyTheOwnedDistributionCanBeTouched`
green, because the exact-name rule underneath refuses those names anyway. The
list exists so a plausible name's refusal SAYS WHY, and no case asserted the
message. The case now asserts the reason, and refuses without the list.

⚠ The harness for that pass was wrong twice before it was right: a `-run`
pattern that matched no test read as "green with the guard gone", and a mutation
that did not compile read the same way. Both now report the case count and the
build separately, which is the only reason the theatre was found rather than
recorded as a pass.

**A fourth pass, and CI ran it.** The first push went red on three jobs, and
every one of them was a defect this machine structurally cannot see:

- ⛔ **A workspace symlink pointing out of the tree was PACKED, not refused.**
  An absolute link target was joined onto the link's own directory, so `/work`
  plus `/etc/passwd` became `/work/etc/passwd`, which is inside the workspace by
  every containment test there is. The case that covers it skips on Windows,
  where creating a symlink needs a privilege this process may not have, so the
  ubuntu job was the only place it could fire. Fixed, and a second case now
  covers the join itself and runs on every host, which immediately found the
  wider version: on Windows a leading separator with no drive is DRIVE-relative,
  so `filepath.IsAbs` answers false for a target that still names a place
  outside the tree.
- ⚠ **`WindowsPathToGuest` answered differently on Linux**, because
  `filepath.Abs` resolves in the running host's grammar and read a drive-rooted
  path as a relative name. It parses the Windows form itself now.
- ⚠ **A CI runner's temporary directory is the 8.3 short form**, and it failed
  a string comparison against a resolver that was right. The case compares
  canonical paths now.
- ⚠ **shellcheck 0.11.0 here and an older one on `ubuntu-latest` disagree**
  about `cd "$D" && cmd || true`, so the same command over the same files
  answered differently in the two places.
  [`../docs/methodology/gate.md`](../docs/methodology/gate.md) owns what that
  costs and the rule it produced.

**Lens 3, the claim audit.** Every documented flag against the binary's own
help, the preset table against `base presets`, the catalog table against
`images --json`, the release assets against the workflow that stages them. Two
findings: `--user` was in the code and in neither manual, and `build.ps1 -Check`
reported the wrong LINE for every disagreement in the embedded copy, because it
split on newline and left the CRLF product's carriage returns on, so line 1
always differed and the two values printed underneath looked identical. Both
fixed; the differ now names line 2150 for an edit at line 2150.

### The release, driven as a consumer

`wsl-toolkit-v1.1.0` published by the workflow on 2026-09-09, five assets, with
`SHA256SUMS` computed in CI over the bytes that were uploaded. Then, from an
empty directory holding nothing but `launcher.ps1` fetched from that release:

```text
$ pwsh -NoProfile -File ./launcher.ps1 -LauncherRelease wsl-toolkit-v1.1.0 -LauncherInstallDir . version
  * release wsl-toolkit-v1.1.0, asset wsl-toolkit-windows-amd64.exe
  * digest matches the SHA256SUMS in release wsl-toolkit-v1.1.0
1.1.0

$ pwsh -NoProfile -File ./launcher.ps1 -LauncherKind script -LauncherRelease wsl-toolkit-v1.1.0 -LauncherInstallDir . -Action List
  * release wsl-toolkit-v1.1.0
  * digest matches the SHA256SUMS in release wsl-toolkit-v1.1.0
==> Other distros on this system -- never touched by this script
  podman-machine-default   [PROTECTED]
  wsl-toolkit   [PROTECTED]

$ ./wsl-toolkit-windows-amd64.exe run --image alpine --workspace ws --artifacts out -c '...'
  workspace: 1 entries, 17 B copied to /home/toolkit/.wsl-toolkit/jobs/8fa39b8bb07a0154/work
uid=0 ok  alpine exited 0 in 1.776s
  artifact: from-the-release
```

⭐ **The digests were recomputed here and compared against the published
`SHA256SUMS`**, rather than trusting the launcher's own report: all three
downloadable-on-this-host assets match. `-LauncherSha256` with a digest held by
the caller passes on top of that, which is the check that proves authorship
rather than transport.

⚠ **`wsl-toolkit` reads as `[PROTECTED]` to the script now**, which is the
`20-prelude.ps1` change doing its job: the throwaway-distro tool refuses to
touch the distribution the executable owns, and the two tools cannot destroy
each other's.
