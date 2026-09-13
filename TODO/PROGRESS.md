# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-13T02:36:12Z
baseline        e9f0e08 with 19 uncommitted WSL-69 paths; local gate RED,
                3 problems, 42.4s
entries         total 104  open 7  blocked 0  done 97
gate            in progress; see the WSL-69 checkpoint commit
head            e9f0e08 plus this session's commits, not yet pushed
```

## Active work

⭐ **[Issue 30](https://github.com/Azathothas/ToolKit/issues/30) is being finished
unattended**, on the operator's instruction of 2026-09-13 to complete every task
and close the issue properly. The work order is `WSL-69`, then three defects this
session found in the gate itself, then `WSL-72`, `WSL-70`, `WSL-71`, `WSL-67` and
`WSL-68`.

⚠ **`WSL-69` is at a checkpoint and stays open.** The base the guide needs is
built and driven; the three defects and the published fingerprint found in the
uncommitted work are fixed and mutation-proved. The entry's 2026-09-13 amendment
carries the measurements. What remains waits on the operator's Meta account for
two steps, and this session polls for them rather than stopping.

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
local gate            19 checks green in 21s, timed on this host rather than
                      estimated. RULES.md says the gate cost belongs here
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

⭐ **Three passes, three different questions**, run at the end of the session on
the operator's instruction and specified by
[`../docs/methodology/reviews.md`](../docs/methodology/reviews.md). Each names what
it looked at that the other two did not.

**Change under review:** 18 files against `fcca2ba`, +2,741 / -213 as read at
`b524b00`. ⚠ **That figure counts the commit that records it, so it is stale by
construction**; `git diff --shortstat fcca2ba..HEAD` is the live one. Two files added
at the top of the tree, two removed from `examples/`, one catalogue row, one test
assertion, and the record.

### Lens 1, the door sweep: what other door reaches this?

**Looked at what the other two did not:** every surface that can reach the two
moved files and the new catalogue row, enumerated from memory and then **grepped
for the ones the enumeration missed**.

- ⭐ **Found: [`../docs/consumers.md`](../docs/consumers.md) enumerated 11 of the
  bootstrap's 17 flags.** `--no-upstream`, `--tmux-config`, `--no-tmux-config`,
  `--list-providers`, `--list-names` and `--version` were absent, because the
  sentence was written before six of them existed. ⛔ **Fixed by removing the
  enumeration rather than completing it**: a flag list in a second document is a
  list that goes stale, and `--help` is the authority.
- **Found: `scripts/README.md`'s directory table** describes `common/` as checks
  and helpers and gave a reader no way to know a configuration FILE now lives
  there. Fixed.
- ⭐ **Found, and it is why the gate stayed green:** the generated manual does not
  enumerate the catalogue. `grep -c opensuse` over `wsl-toolkit.1` and
  `wsl-toolkit.md` is **0**, so a row is addable without regenerating either, and
  `TestGeneratedManPageIsCurrent` was never at risk. That was luck until it was
  checked.
- **Cleared, each by a grep rather than by reading:** no live reference to either
  old `examples/` path survives - the seven hits are the changelog, the consumers
  break row, the sweep, the README's own account of the move, and the record; the
  PowerShell bundle and launcher reference neither `examples/` nor the scripts; no
  test asserts a `Kind` count, so a `niche` row was safe; `libc:musl` still selects
  three; and `deslop` reports only the four pre-existing methodology files.

### Lens 2, the guard mutation: can the new guard actually fail?

**Looked at what the other two did not:** every guard this session added, by
**planting the defect it exists to catch** and reading the exit code unpiped.

- ⛔ **Found the session's own regression, and it is the important finding.** The
  catalogue count guard was changed from `!= 12` to `< 12` so that adding a row
  would not require editing the assertion. Planting proved that **removing the row
  this session had just added stayed GREEN** - 12 images, `ok` - and only removing
  a second one fired. The old assertion would have caught the first. ⭐ Fixed by
  raising the floor to the current count, 13, with the message saying that adding
  a row means raising it on purpose. Re-proved: unmutated passes, removing the
  newest row now fails at `guards_test.go:492`. The catalogue file was restored
  and `git diff` over it is empty.
- ⭐ **Four digest guards had never been seen to refuse, and all four now have.**
  Each was driven in a container with the defect planted:

  | planted | result |
  | --- | --- |
  | the computed npm digest is wrong, by hashing with sha256 under a sha512 label | exit **1**, "does not match the registry integrity value" |
  | a well-formed but wrong `--expect-integrity` | exit **1**, "does not match the --expect-integrity value" |
  | the computed PowerShell sha256 is wrong, by substituting md5sum | exit **1**, "does not match the digest its own release publishes" |
  | a wrong `--expect-sha256` | exit **1**, "does not match the --expect-sha256 value" |

  ⚠ **The baseline was driven in the same run each time**, because a guard that
  refuses everything is not a guard either: unmutated, the same commands exit 0
  and report two registry matches and one release match.
- **Already driven earlier in the session, and not re-run here:** `--prefix`
  relative and `/`, an absent forced `--user-provider`, a malformed
  `--expect-integrity`, an unknown logical name, an unknown toolset, an unknown
  provider, an unknown flag, and a flag with no value.

### Lens 3, the claim audit: which published sentence has no artefact behind it?

**Looked at what the other two did not:** the numbers in what is being published -
the record, the snapshot, the changelog, the entries and the issue comment -
recomputed from the tree rather than re-read.

- ⭐ **Found four stale numbers in [`SUMMARY.md`](SUMMARY.md)**, every one of them
  a figure that was true when written and was overtaken by later work in the same
  session: the commit count, the files-changed and line counts, the bootstrap's
  line count, and how many matrix and FreeBSD runs it took. All four corrected
  against the tree.
- ⭐ **Found: the run counts were both wrong in the same direction.** "5 full
  matrix runs and 7 FreeBSD sessions" was written from memory; counting the
  invocations gives **6** matrix runs - two `developer`, one tool-availability
  probe, two `agent`, one regression - and **9** `bsd run` sessions plus one `bsd
  status`. ⚠ This is the class `reviews.md` names as a number with the wrong
  denominator, and memory was the denominator.
- **Verified against the tree, not re-read:** 13 catalogue images, 24 tracked
  shell scripts, 19 gate checks, 1,311 lines and 47,297 bytes of `bootstrap.sh`,
  86 lines of `tmux.conf`, 270 tracked files, 7 open entries, and the four new
  entries' priorities and efforts as the index declares them.
- **Also corrected:** the issue comment's reading list said four open entries are
  what the issue wants, which reads as though `WSL-67` were finished. It is open
  too, and the comment now says so where a reader meets the list.

### ⛔ What a pass with no findings would have owed

Every pass fired, so none of the three has to answer that. ⚠ Recorded anyway,
because it is the sentence that proves a pass happened: the door sweep would have
fired had any of the seven surviving `examples/common` references been a live
reference rather than a historical one; the mutation pass would have fired had a
planted digest still exited 0; and the claim audit would have fired had every
recomputed number matched the published one.
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
