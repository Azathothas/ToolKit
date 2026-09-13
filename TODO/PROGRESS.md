# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-13T02:36:12Z; the record commit's own time is its end
baseline        e9f0e08 with 19 uncommitted WSL-69 paths; local gate RED,
                3 problems, 42.4s
entries         total 108  open 7  blocked 0  done 101
gate            20 checks green in 31.6s on the working tree at 04:57:21Z
head            2656a0d pushed and CI green on all six jobs; the commit that
                carries this record sits on top of it
```

## Active work

⛔ **[Issue 30](https://github.com/Azathothas/ToolKit/issues/30) is NOT finished,
and this session stopped on the operator's instruction.** The operator first asked
for the issue to be finished unattended, then redirected the same day: close the
task in flight, checkpoint the rest, file
[pull request 31](https://github.com/Azathothas/ToolKit/pull/31) as a task, and
end the session. What follows is a resume point.

## The work order, set by the operator on 2026-09-13

⛔ **The next session trusts nothing, including this file, and validates
everything before building on it.** Every number below is a claim until a command
re-measures it.

1. ⭐ **`WSL-73`, and nothing else first.** Review pull request 31 as untrusted
   input, adopt only what survives, iterate and improve it, delete the PowerShell
   product entirely, and close the pull request. Consumers get no migration work
   and are told to read the latest docs directly. That unit is done when the port
   is complete, the pull request is closed, every suite and the gate pass, the
   docs are updated and bloat-free with no narrative history, the tree is clean,
   CI is green, and three deep reviews are recorded. The entry carries the
   measured premise and the rulings.
2. **Read issues [30](https://github.com/Azathothas/ToolKit/issues/30),
   [32](https://github.com/Azathothas/ToolKit/issues/32) and
   [33](https://github.com/Azathothas/ToolKit/issues/33) in full, comments
   included, for understanding only.** ⛔ Implement none of them. Then reconcile
   the open entries against them: `WSL-59`, `WSL-67`, `WSL-68`, `WSL-70`, `WSL-71`
   and `WSL-72`, and the unfiled findings below. Author the new entries that will
   resolve the three issues, each with its `INDEX.md` row in the same change.
3. **Review the docs, the repository and CI again**, after the two steps above
   have changed them.
4. **End with a kickoff prompt for the session after**: detailed, correct and
   deep-reviewed, written for an agent with no prior context or memory, that will
   close those issues properly.

## Done this session

| commit | what |
| --- | --- |
| `622f46a` | `WSL-69` checkpoint: the previous session's uncommitted Muse base, with three defects and a published username path found in it and fixed |
| `508b429` | `TOOL-23`, `TOOL-24`, `TOOL-25` filed and closed: three gate rules that could not fail the way they were written |
| `e8948d5` | ⭐ `WSL-69` closed. The operator installed Muse and signed in; the agent drove it headless and through its interactive screen; the negative pass found a sixth defect, empty drive mount points under `automount off` |
| `2656a0d` | `WSL-72` checkpoint: the BSD disk grows to 10 GiB and nim is the one name left. `WSL-70` checkpoint: the package table has one home and a checked copy that nothing reads yet |
| the record commit | `WSL-73` filed for pull request 31, with the operator's rulings |

## Measurements

Read on Windows 11 Pro 26200 on 2026-09-13:

```text
CI            622f46a, 508b429, e8948d5 and 2656a0d: all six jobs green
clean clone   the gate green on a fresh clone of e8948d5 and of 2656a0d, once
              core.hooksPath was set in it
shellcheck    ubuntu:24.04's 0.9.0, which is CI's: 25 of 25 tracked scripts
              clean at 2656a0d
8.3 TEMP      tools/check, tools/repo and tools/windows/wsl-toolkit suites
              green on the clean clone with TEMP at a short-name directory
mutation      113 rows at 2656a0d against 90 at e9f0e08. Every row added or
              re-anchored this session was planted alone and went red, and CI's
              mutation job ran all of them green
muse          Muse Code 1.1.1; headless smoke 52 s, exit 0; the interactive
              answer on screen 10 s after ENTER; acceptance: version 0, push 0
              with local and remote equal, /mnt/c /mnt/d and System32 exit 2,
              cmd.exe not on PATH
bsd           10.0 GiB disk, 8.7 GiB root. The languages toolset: exit 1 with
              absent=nim alone. Five sessions of 128.6 s to 287.2 s
bootstrap     HEAD and the refactored file gave identical output for five
              invocations on alpine 3.22 and debian 13
tree          85,893 text lines in 279 files at 2656a0d, against 83,285 in 270
              at e9f0e08, counted with git grep -I -c
```

## Found, and not filed

1. ⛔ **`bsd run` joins a payload's lines with `; `, so a comment line silently
   drops every command after it and the run still exits 0.** Measured in the
   FreeBSD guest: `sh -c 'echo joined-a; # a comment; echo joined-b'` printed only
   `joined-a` and exited 0. A blank line becomes `; ;`, which is a syntax error.
   `bsd.go` says the joined payload is "what a shell already understands".
   Recommended: P1, because the answer is silently wrong.
2. `bootstrap.sh --dry-run --toolset agent --codegraph none --json` exits 1 on
   debian 13, under HEAD and the refactor alike. The cause was not read.
3. `pkgin` on NetBSD and `pkg_add` on OpenBSD are written and have never run.
4. `soar` and `nix` as user-level providers are written and not driven.
5. A deterministic regression for pre-marker base rollback is still owed.

## Review findings

Three passes over `e9f0e08..2656a0d` and this record, per
[`../docs/methodology/reviews.md`](../docs/methodology/reviews.md). Each names
what it looked at that the other two did not.

### Lens 1, the door sweep

**Looked at:** every surface that reaches `bsd run --disk` and its JSON fields,
`check package-table --fix`, the moved block of the fetched bootstrap, and what
pull request 31 deletes.

- **Found:** `scripts/README.md`'s bootstrap section gave someone editing the
  table no sign that the block has a generated copy. It points at `RULES.md`
  section 4 now.
- **Found:** `RULES.md` section 4 said two generated files, and there are three.
- **Found:** `--disk 0` was refused only after the boot banner printed. It is
  refused before any output now, with a test.
- **Found:** pull request 31 leaves `.gitattributes` line 58 and `RULES.md`
  section 4 naming a file it deletes. Recorded in `WSL-73` rather than fixed,
  because the file still exists on `main`.
- **Found:** `check.sh` and `check.ps1` each carried a sentence with a word
  missing. Fixed.
- **Cleared:** the acceptance runner's JSON sweep parses `bsd status --json`, and
  the drive parsed the new `disk_default_bytes` field; the manual carries
  `--disk`; `check.ps1` passes `--fix` through unchanged.

### Lens 2, the guard mutation

**Looked at:** every guard this session added, by planting what it catches.

- Twenty-three rows added, each planted alone and red: nine in the `WSL-69`
  checkpoint, four for `TOOL-23` to `TOOL-25`, three for the drive mount points,
  four for the BSD disk and boot rules, and three for the package table.
- ⛔ **Found:** the umask row from the checkpoint stopped matching once `cd /` was
  inserted beside its anchor, and the gate refused it. Re-anchored and re-proved.
- The package table check driven against a planted edit to each side alone: exit
  1 both times, naming line 49, and exit 0 once restored.
- ⭐ **Found by driving rather than planting:** the first boot failure rule matched
  `panic: ` anywhere, and a healthy boot stopped on a console line reporting an
  EARLIER panic. It is anchored now, and that line is a regression case.

### Lens 3, the claim audit

**Looked at:** every sentence and number published this session, against the
artefact behind it.

- **Found:** the generated copy's banner, the bootstrap comment and the checker's
  comment all said the provisioner RUNS the copy. Nothing reads it; all three say
  so now.
- **Found:** the boot failure error said "The image is damaged" for any match,
  a panic included. It says "If the image is damaged" now.
- **Found:** the `WSL-69` closing carried a full SHA-256 and two full commit ids,
  which the tree refuses. Shortened, with the reason beside them.
- **Found:** the changelog called every `WSL-69` defect mutation-proved; the
  username's guard is `TOOL-25`'s row, and the sentence says so.
- **Found:** `WSL-72` asks for a root of at least 9 GiB, which a 10 GiB disk
  cannot give on an image carrying a 1 GiB swap partition. Recorded as a
  correction and asked below rather than quietly met.
- **Verified:** the damaged BSD image was not this session's grow. The first
  failing boot ran before any grow, on an image at the published size.
- **Verified against the artefact:** 978 nim files; a 171,601,920-byte core; 500,
  499 and 492 packages; 31 new files and 11 tests in pull request 31, 55 files
  at +9,204 / -5,610; 113 mutation rows; 20 checks.

## Open questions for the operator

1. `WSL-72`: accept an 8.7 GiB root on the ruled 10 GiB disk as the grow reaching
   the filesystem? Recommended yes: the swap partition is the image's, and the
   entry forbids a larger default.

## Host state

- `wsl -l -v` reads what it read at 02:45:39Z: `podman-machine-default`,
  `wsl-toolkit`, `wsl-toolkit-muse` and `wsl-toolkit-podbox` running, and
  `eph-pgb` stopped. `wsl-toolkit-muse` was rebuilt in between for the `muse`
  account. It holds the operator's Muse install and credential, whose file
  was checked for existence and never read. No Zellij session is left and no
  drive mount point.
- The ordinary `wsl-toolkit` base was re-provisioned in place by `base ensure`.
- The FreeBSD image is 10.0 GiB with the published 500 packages; `WSL-72` names
  its residue.
- Every container ran ephemeral and removed itself.
- `.tmp/wsl69-e2e/project` is the throwaway checkout the Muse base's one grant
  points at, and it stays while that base does.
