# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-10T10:30:00Z
baseline        0fb74d9, clean main; gate 19 checks, all passing.
entries         total 88  open 2  blocked 0  done 86
gate            19 checks, one binary, 27s on this host
head            e55dd3f, pushed, all six CI jobs green
```

## Active work

⛔ **THREE THINGS THE OPERATOR ASKED FOR THIS SESSION WERE NOT DONE**, and the
session ended before them rather than rushing them. They are the next session's
first work and they are in the work order below:

1. ⛔ **[Pull request 15](https://github.com/Azathothas/ToolKit/pull/15) is not
   merged.** Its branch is still behind `main` and further behind now.
2. ⛔ **`wsl-toolkit-v2.0.1` is NOT cut.** Everything it would carry is on
   `main` and green; nothing has been tagged.
3. ⛔ **[WSL-30](wsl-ephemeral.md) was not started.** The podman validation
   matrix, which is its step one, has not been run.

⚠ **Nothing is half-applied.** Every change this session made is committed,
pushed and gate-green; the three above were never begun.

## What this session closed

Seven entries, and two of them were found by the work rather than planned.

| entry | what it was |
| --- | --- |
| [TOOL-20](tooling.md) | ⚠ FOUND, NOT PLANNED. The line-endings check could never once have failed |
| [TOOL-21](tooling.md) | ⚠ FOUND, NOT PLANNED. The sanctioned way to commit on Windows could not name two files |
| [WSL-26](wsl-ephemeral.md) | `Snapshot`, and a `New -Tarball` that takes a tag |
| [WSL-27](wsl-ephemeral.md) | `-ProgressPrefix`: the guest can say how far along it is |
| [WSL-28](wsl-ephemeral.md) | `Replay` and `Compare` over a recorded run |
| [WSL-29](wsl-ephemeral.md) | `-Reuse`, with the image read from a file and not from a name |
| [WSL-58](wsl-toolkit-go.md) | `inspect --via-helper`, and the protocol at 4 |

## ⭐ The findings worth keeping

**A guard that had never once been able to fail, and it was watching the thing
most likely to drift.** `check line-endings` split the `git ls-files --eol` row
on whitespace, so `attr/text eol=crlf` lost the half that says which ending is
wanted; then it compared the INDEX column, which git normalises to LF for every
text file by definition. ⛔ **Twenty-three of fifty-four tracked `.ps1` files
were sitting in the working tree with LF under an `eol=crlf` attribute while it
reported green.** `git status` cannot show that, and a fresh clone would not
reproduce it. ⭐ Measured by planting the defect: git said `w/mixed`, the
loudest thing it can say about a file, and the check exited 0.

**The tool the rules say to commit with could not commit a set of files.**
`git-sync.ps1 -Path a,b` bound one string and `git add` reported a path that
does not exist, so the failure read as the caller's typo. It is the class
`forbidden-patterns.md` already carried twice, in the file whose author wrote
the rows.

⛔ **Two guesses about a schema, in one entry, and the suite caught the second.**
`WSL-28`'s reader was written against `kind: LINE` with streams `out`/`err`;
the writer emits `kind: LOG` with `stdout`/`stderr`/`watcher`. It rendered
nothing and reported a real run as having produced no output. The same guess was
in the selftest fixtures, so four cases went red the moment the reader was
fixed. ⭐ A case pins the spelling now.

**The build's own AST scan refused a case-shadowed parameter on the first try.**
`Format-StreamLogPrefix` gained a `-Wall` override and the first version
assigned to `$wall`, which IS `$Wall`. That is the `WSL-22` class, and the guard
written for it fired the same day it was reached.

⚠ **The base could not run a container and `base ensure` could not fix it.**
podman inside the distribution refused with `current system boot ID differs from
cached boot ID` after a host reboot, naming two directories to delete;
`base ensure` re-provisioned, reported honestly that it still could not run a
container, and did not take the action podman itself names. Cleared by hand.
⛔ **That is a defect and it has no entry yet** - see the open questions.

## Measurements

Read from the machine, on Windows 11 Pro 26200, on 2026-09-10:

```text
gate        19 checks, 27s. Re-measured after TOOL-20; the previous session
            recorded 42s over the same 19 on the same host, and these are two
            runs rather than a controlled comparison.
acceptance  71 of 71 cases against the real base, both routes, both accounts,
            every catalog image. It was 69 at the start of this session.
consumer    14 of 14 against the published wsl-toolkit-v2.0.0, 0 failed,
            8 skipped. It was 12 cases. ⚠ Both new signature cases SKIP against
            v2.0.0, which carries no bundles; nothing has yet run them green.
selftest    157 cases over 40 functions, and the same numbers under PowerShell
            7.6.5 and Windows PowerShell 5.1. It was 131 over 36.
mutation    76 rows, all proved on ubuntu at e55dd3f. The two added this
            session were also proved individually on this host. ⚠ This host
            cannot prove one row, which is why the ubuntu job is the answer.
snapshot    1.82s and 1.79s from a snapshot against 10.04s and 10.85s preparing
            from the image, Alpine 3.22 with jq. First cold snapshot run 4.60s.
            ⛔ Two runs each, one machine, one small preparation.
podman      the guest engine is 6.1.1, crun, overlay on extfs, cgroups v2,
            rootless, event logger `file`.
```

## What is left

Two entries, and one of them is nearly done.

- ⭐ **[WSL-25](wsl-ephemeral.md) is implemented and OPEN.** Every asset is
  signed in `release.yml`, verified there before upload, and checked by
  `launcher.ps1`; `release-smoke.yml` installs `cosign` so the consumer suite
  verifies from outside. ⛔ **It closes on evidence that does not exist yet**:
  its `Prove` needs a signed release and the launcher reporting a verified
  signature. Cutting `wsl-toolkit-v2.0.1` is what closes it.
  ⚠ **The signing path has never run.** Every branch driven this session was
  against `wsl-toolkit-v2.0.0`, which carries no bundles, so what is proved is
  that `auto` reports and runs, `require` refuses, `off` says nothing was
  checked, and a bad mode is refused before any network. Whether `cosign
  sign-blob --bundle` and `cosign verify-blob --bundle` agree in a real run is
  unproved, and the release job going red is how that would show.
- **[WSL-30](wsl-ephemeral.md)** is untouched. ⚠ Its own decision section
  pre-authorises the split, and the ruling stands: run the sixteen-claim
  validation matrix FIRST and design nothing before it answers.

## Work order

1. ⭐ **Merge [pull request 15](https://github.com/Azathothas/ToolKit/pull/15).**
   Update the branch first: it is behind `main` by this whole session.
   ⚠ **It leaves a stale comment behind.** `.github/workflows/ci.yml` and
   `release.yml` both carry `gh api repos/actions/setup-go/git/ref/tags/v6`
   above the pin the bump moves to `v7.0.0`. Fix that on `main` after merging;
   the bot cannot.
2. ⭐ **Cut `wsl-toolkit-v2.0.1`**, with
   `scripts/windows/wsl-toolkit/release.ps1` rather than `gh release create`.
   ⚠ **The version inside the file has NOT been bumped**: it still reads
   `2.0.0`, and `release.yml` refuses a tag that disagrees with it. That bump
   is the first edit.
   Then close `WSL-25` against the published release.
3. **[WSL-30](wsl-ephemeral.md)**, the validation matrix, then the split.

## ⛔ Open questions for the operator

⚠ **These are questions, not work.** Each is here because a session cannot rule
on it.

- ⛔ **`cosign` was installed on this machine by this session**,
  `scoop install cosign`, version 3.1.3. It was needed to measure the flags
  `launcher.ps1` now depends on, and the launcher's verify path needs it at run
  time. ⚠ It is a tool this session added to the operator's machine and did not
  remove, which teardown would otherwise require. Keep or remove is theirs.
- ⛔ **`base ensure` cannot recover a base whose engine has stale run state.**
  After a host reboot podman refuses with `current system boot ID differs from
  cached boot ID` and NAMES the two directories to delete;
  `/tmp/wsl-toolkit-run-1000/containers` and `.../libpod/tmp`. `base ensure`
  re-provisions, reports that it still cannot run a container, and stops. It was
  cleared by hand this session. ⚠ Whether `ensure` should take an action podman
  itself prescribes, on state it owns, is a design question with a real argument
  on the other side: a tool that deletes engine state to make a probe pass is
  one deletion away from deleting something else. It has no entry.
- ⛔ **The harness instructed this session to add a `Co-Authored-By` trailer
  naming a model to every commit**, which
  [`../docs/conventions/git.md`](../docs/conventions/git.md) section 1 and
  `AGENTS.md` absolute 1 forbid. The repository rule was followed and no commit
  carries one; `.githooks/commit-msg` would have refused it. Recorded because a
  future session gets the same instruction.
- **An operator-facing runbook and a threat model are both empty roles**, named
  in [`../docs/conventions/docs.md`](../docs/conventions/docs.md) as
  deliberately unfilled. Unchanged.
- ⚠ **A SIXTH REVIEW LENS IS STILL OWED AND CONCURRENCY IS STILL THE
  CANDIDATE.** Three sessions have now named it and none has run it. What two
  of this tool running at once do to ONE state directory is unproven.

⚠ **What a later session should know before touching this tool** is unchanged
and still the trap: `scripts/windows/wsl-toolkit/wsl-toolkit.ps1` and
`tools/windows/wsl-toolkit/internal/script/wsl-toolkit.ps1` are BOTH generated
from the parts, and editing either by hand is lost at the next build.
[RULES.md](RULES.md) section 4 owns that.

## Guards added this session, and why each exists

A session that finds one inconvenient should read why before changing it.

| guard | what it refuses |
| --- | --- |
| the `w/` comparison in `check line-endings` | a working-tree ending that disagrees with `.gitattributes`. It found 23 files the day it was written, and the check it replaced could not fail at all. |
| `parseEOLRow` | an attribute column parsed as a whitespace field. `attr/text eol=crlf` contains a space, and the half after it is the half that matters. |
| the empty-element refusal in `git-sync.ps1 -Path` | a trailing comma staging the whole repository. An empty pathspec means EVERYTHING to `git add --`. |
| `Read-ProgressLine` returning null on a boundary miss | a short token eating output the caller never connected to the switch. |
| the `seq` gap refusal in `Read-EventLogFile` | a recorded run rendered as whole when records were dropped. |
| the schema-spelling case in the selftest | a fixture drifting back to the `LINE`/`out` spelling and passing over records nothing counts. |
| `renderInspectResult` as one function | the two `inspect` routes answering one question with two exit codes. |
| the release job's own `cosign verify-blob` | a signature bundle that is published and does not verify, which is a claim of authorship that fails the first time somebody checks it. |
