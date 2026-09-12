# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-12T14:40:00Z
baseline        fcca2ba, clean main; local gate 19 checks green and CI RED on
                that same commit in three jobs
entries         total 104  open 7  blocked 0  done 97
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
freebsd 15.1          driven through bsd run --network plus a fetch of the raw
                      URL. pkg installed bash ca_root_nss coreutils curl git
                      jq node npm ripgrep tmux; requested 18, present 10,
                      skipped 8, absent 0, failures 0. go 1.25.14 and
                      python3 3.12.14 installed; rust and nim filled the
                      4.8 GiB guest disk, and pkg rquery confirms both names
                      (rust 1.96.1, nim 2.2.10). FreeBSD packages powershell
                      7.5.5_1, which the first table did not know
shellcheck            ubuntu:24.04 reports 0.9.0, which is CI's; 24 of 24
                      tracked scripts clean under it
tmux.conf             loaded live on tmux 3.5a and 3.7c with no config error.
                      Every option read back: prefix M-g AND prefix2 C-b, mouse
                      on, history-limit 100000, detach-on-destroy off,
                      remain-on-exit on, exit-empty off, escape-time 10,
                      mode-keys vi, base-index 1, pane-base-index 1. `x` and
                      `&` are unbound, `K` confirms before killing a session,
                      and M-s is in the root table
argument battery      11 cases under dash, which is stricter than bash:
                      unknown name, unknown toolset, unknown provider, bad
                      integrity value, unknown flag, flag with no value,
                      relative prefix, prefix /, --provider override,
                      --list-providers, and a --json object that parses
local gate            19 checks green
```

## What is left, and four of it is now filed

⭐ **The operator ruled every open question on 2026-09-12 and each ruling became an
entry**, so the next session picks up a filed unit of work rather than a decision.
⛔ Authoring is not implementing: none of the four has any code written for it.

| entry | the ruling behind it |
| --- | --- |
| ⭐ `WSL-69` Muse Code installed, authenticated and driven end to end | the operator asked for this as a task of its own. P1, because the guide's last third is written from what the provider documents rather than from what happened |
| `WSL-70` two package maps become one, and the Go module embeds it | **one shared table**. The alternative, leaving both and recording the risk, lost |
| `WSL-71` a portable shell profile this tree owns | **write a portable proper one here**. Pointing at the `devscripts` URL lost because a guest may not reach it; vendoring a copy lost because two homes drift |
| `WSL-72` the BSD guest gets a 10 GiB disk | **grow and allow 10GB** |

What is left and NOT filed:

1. ⛔ **`pkgin` on NetBSD and `pkg_add` on OpenBSD are written and have never been
   run.** No image for either here, and the script header says so rather than
   letting the twelve-manager count imply otherwise.
2. `soar` and `nix` as user-level providers are written and not driven. ⛔ This
   script may not install either, so driving them needs a machine that has one.
3. `WSL-67`'s own remaining list: the provider-profile scenarios in the main
   acceptance runner.
4. Add a deterministic regression for pre-marker base rollback.
5. `WSL-68`: the attacking sealed-base probe and a published threat model.
   ⚠ **Nothing was done to it this session.**
6. `WSL-59`, the low-level PowerShell adapter, remains open and untouched.

## Review findings

- **Driven pass over the two artifacts, not only the script.** The tmux
  configuration was loaded on two tmux versions and every option read back from
  the running server, because a configuration with one unsupported option loads
  PARTIALLY: tmux starts, that line did nothing, and the session looks
  configured. ⭐ It found nothing wrong, which is a result about the file and not
  about the pass.
- **Door sweep over the new script.** Every path that can fail now reaches one of
  `fail`, `warn` or `die`, and the difference between them is stated in the
  header. It found the upstream-failure path reporting one absent tool twice, once
  as skipped and again as not-on-PATH; a forced user provider that is absent being
  attempted once per package instead of refused once; and `--prefix` reaching two
  `rm -rf` calls without being constrained where it is read.
- **Guard mutation, and it found the worst defect in the session.** `fail` inside
  `$( )` increments a counter in a SUBSHELL, so a run whose digest step never
  completed reported `failures=0` and exited 0. ⛔ A guard that cannot make the
  process fail is not a guard. `fetch_verified_npm` answers through a global now.
  The same read found `set --` clobbering that function's own `$2` and `$3`, which
  is what made the digest step fail in the first place.
- **Claim audit, run twice.** The first pass cut three unmeasured claims. ⭐ The
  second had to CORRECT one of its own: everything was labelled "FreeBSD
  installation is not driven" on the strength of a guest with no resolver, and
  `bsd run --network` made that false an hour later. The label is gone and the
  measurement is in its place.
- ⚠ **A pipe hid a red matrix.** The first full run put the script through
  `| tail -40`, so every row reported `tail`'s exit code and twelve of twelve read
  as green. Re-run unpiped: five of twelve. This is
  [`../docs/AGENTS.md`](../docs/AGENTS.md) absolute 5, in the session that read it.
- ⚠ **And one claim made to the operator mid-session was wrong.** Eleven of
  thirteen matrix rows had reported and chimera was read as one of the green ones;
  it was still running and it failed. Corrected in the same conversation. ⛔ A
  partial result table is not a result.

## Open questions for the operator

⭐ **None.** All three that were open were ruled on 2026-09-12 and each is now an
entry; the table above says which ruling produced which. ⚠ `WSL-69` needs the
operator's Meta credentials for two of its steps, and that is a dependency rather
than a question.

Unrelated host state is untouched: `eph-pgb`, `wsl-toolkit-podbox`,
`podman-machine-default` and the ordinary `wsl-toolkit` base all remain
registered. Every container this session ran was ephemeral and removed itself.
⚠ **The FreeBSD guest image is shared state across sessions and this one mutated
it.** What was installed was removed and the root went from 102% to 43%; `pkg info`
counted 564 before and 321 after, so the removal was not total. `wsl-toolkit bsd
fetch` re-downloads a pristine image if an exact baseline is wanted.
