# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-12T14:40:00Z
baseline        fcca2ba, clean main; local gate 19 checks green and CI RED on
                that same commit in three jobs
entries         total 100  open 3  blocked 0  done 97
gate            19 checks green; CI green on bf5c095, all six jobs
head            bf5c095 pushed; the scripts work committed on top
```

## Active work

⭐ **[Issue 29](https://github.com/Azathothas/ToolKit/issues/29) is CLOSED**, on CI
run [`34700281005`](https://github.com/Azathothas/ToolKit/actions/runs/34700281005)
with all six jobs green. ⛔ **Neither of the two failures that had been blocking it
belonged to it**; both were in the issue-30 files, and the record below says what
they were.

⭐ **[Issue 30](https://github.com/Azathothas/ToolKit/issues/30) part 1 is much
further along and still open.** The operator ruled three changes mid-session and
added two requirements; all five are done and driven. `WSL-68`, the sealed-base
work, is untouched.

## What was built

### The two CI failures, and why a green local gate could not see either

- `TestBaseMountPayloadIsEncodedAndEscaped` compared a resolved mount source
  against a raw `t.TempDir()`. The Windows runner's `TEMP` is under
  `C:\Users\RUNNER~1\`, an 8.3 short name that `filepath.EvalSymlinks` expands, so
  the production canonicalization and the typed expectation disagreed there and
  agreed on every long-name host. ⭐ Reproduced locally by pointing `TEMP` at a
  short-name directory: red before the fix, green after.
- `shellcheck` refused `[ ... ] && [ ... ] || die` for SC2015. This host carries
  0.11.0, which permits it; `ubuntu-latest` carries **0.9.0**, which does not.

⭐ **That second one is now measurable here rather than predicted.**
`ubuntu:24.04` in a container is the same binary and version CI installs, so
"will CI's shellcheck agree" is a command rather than a guess.

### The common scripts left `wsl-toolkit`

[`../scripts/common/bootstrap.sh`](../scripts/common/bootstrap.sh) and
[`../scripts/common/tmux.conf`](../scripts/common/tmux.conf), moved out of
`tools/windows/wsl-toolkit/examples/common/` on the operator's ruling. Neither was
ever about that tool. [`../docs/consumers.md`](../docs/consumers.md) carries the
move as a break with its exposure window, and
[`../scripts/README.md`](../scripts/README.md) carries their contract.

### The digests came out, and the replacement says what it proves

The old file pinned CodeGraph 1.5.0 and three SHA-512 values, and the registry was
on **1.6.0** the next day. Version and digest are read from the registry at run
time now. ⚠ **That is weaker, deliberately**: the digest and the bytes come from
one place, which proves transport and not authorship. `--expect-integrity` and
`--expect-sha256` restore the stronger check for a caller who holds a value, and
every run prints what it resolved so a caller can become that one.

### Twelve package managers, three of them BSD, and six languages

apk, apt, dnf, emerge, pacman, tdnf, xbps, yum and zypper; `pkg`, `pkgin` and
`pkg_add`. `soar` and `nix` are user-level providers for an account with no root,
and ⛔ neither is ever installed. The `agent` toolset carries bash, Rust and cargo,
Go, Nim, Python and PowerShell, and PowerShell has an upstream route because only
three of thirteen images package it.

⭐ **The package table is the feature.** One row per logical name, a default, and
only the places that spell it differently. ⚠ An override key is a package manager
OR `os:<ID>`, and `os:` wins, because Alpine, Chimera and Wolfi all use apk and
disagree about half the names.

openSUSE Tumbleweed is the thirteenth catalogue image and the only zypper row. The
guard asserting exactly twelve is a floor now.

## Measurements

Read from Windows 11 Pro 26200 on 2026-09-12:

```text
CI, bf5c095           all six jobs green; issue 29 closed on it
matrix, agent toolset 13 ran, 2 failed, 0 unreached, 0 timed out, 5m45s
  green               alpine arch debian debian12 fedora opensuse photon
                      rocky8 ubuntu2204 void-musl wolfi
  red, both external  gentoo stage3 carries no portage tree; chimera's
                      repository is inconsistent between openssl3-3.6.0-r0 and
                      openssl3-devel-3.6.4-r0
alpine, full          requested 25, present 25, skipped 0, absent 0,
                      codegraph 1.6.0, failures 0
debian, upstream      powershell 7.6.6 from its GitHub release, sha256
                      ddbc4a2d...103bc matched against the release's own file
freebsd 15.1          pkg at /usr/sbin/pkg, ID=freebsd, sha256 and openssl
                      present, no bash. Detection driven; install NOT driven
shellcheck            ubuntu:24.04 reports 0.9.0, which is CI's; 24 of 24
                      tracked scripts clean under it
local gate            19 checks green
```

## What is left

1. ⛔ **Drive the FreeBSD install path.** The `os:freebsd` package names are
   written from the ports naming convention and not from a machine. ⚠ `bsd run
   --script` flattens a script into one `;`-joined console line, so a 44 KB file
   does not survive it, and `bsd run -c` is bounded by the console line length.
   ⭐ **The route that will work is `bsd run --network` plus a `fetch` of the raw
   URL now that this is pushed**, and that also exercises the consumer path.
2. `pkgin` on NetBSD and `pkg_add` on OpenBSD are written and not driven. No image
   for either here.
3. `soar` and `nix` as user-level providers are written and not driven.
4. ⚠ **`provision.sh` is a SECOND package map** and still knows six families
   rather than twelve. Not merged: that one runs as root during `base ensure` and
   installs the engine. ⭐ Merging means the Go module embedding a file consumers
   also fetch by URL, which is a decision for the operator rather than a refactor.
5. `WSL-67`'s own remaining list: the acceptance runner, and the Muse smoke.
6. Add a deterministic regression for pre-marker base rollback.
7. `WSL-68`: the attacking sealed-base probe and a published threat model.
8. `WSL-59`, the low-level PowerShell adapter, remains open and untouched.

## Review findings

- **Door sweep over the new script.** Every path that can fail now reaches one of
  `fail`, `warn` or `die`, and the difference between them is stated in the
  header. It found the upstream-failure path reporting one absent tool twice, once
  as skipped and again as not-on-PATH.
- **Guard mutation, and it found the worst defect in the session.** `fail` inside
  `$( )` increments a counter in a SUBSHELL, so a run whose digest step never
  completed reported `failures=0` and exited 0. ⛔ A guard that cannot make the
  process fail is not a guard. `fetch_verified_npm` answers through a global now.
  The same read found `set --` clobbering that function's own `$2` and `$3`, which
  is what made the digest step fail in the first place.
- **Claim audit.** Three claims were cut for being unmeasured: FreeBSD package
  names are labelled as taken from the ports convention rather than the machine,
  `pkgin`/`pkg_add` and `soar`/`nix` are labelled written-and-not-driven, and the
  two red matrix rows are attributed to the distributions rather than called
  expected failures.
- ⚠ **A pipe hid a red matrix.** The first full run put the script through
  `| tail -40`, so every row reported `tail`'s exit code and twelve of twelve read
  as green. Re-run unpiped: five of twelve. This is
  [`../docs/AGENTS.md`](../docs/AGENTS.md) absolute 5, in the session that read it.

## Open questions for the operator

1. ⭐ **Does a `bashrc` belong in this tree?** The reference sweep read the
   operator's own, which lives in `pkgforge/devscripts` and re-fetches itself in
   place. A copy here would be a second home for one file of personal
   configuration, which the one-home rule is against. ⛔ Nothing was written.
2. **Should `provision.sh` and `bootstrap.sh` share one package map?** Item 4
   above has the trade.

Unrelated host state is untouched: `eph-pgb`, `wsl-toolkit-podbox`,
`podman-machine-default` and the ordinary `wsl-toolkit` base all remain
registered. Every container this session ran was ephemeral and removed itself.
