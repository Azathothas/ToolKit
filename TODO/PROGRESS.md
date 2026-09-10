# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-10T04:15:00Z
baseline        f7eabfe, clean main; gate 18 checks, all passing, 43s.
entries         total 85  open 7  blocked 0  done 78
gate            18 checks, one binary, all passing
```

## Active work

None in flight. **Every entry the last session filed is closed**, and so are the
two the operator added and the four this session found while closing them.

`wsl-toolkit` is at **2.0.0 in the tree and the tag is not pushed.** That is the
one thing left and it is the operator's:

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/release.ps1 -Publish
```

⛔ **Nothing here pushed a tag.**
[`../docs/security/remote-ops.md`](../docs/security/remote-ops.md) puts any push
behind the project's push policy, and this repository's policy names commits to
`main` rather than tags. `release.ps1` without `-Publish` verifies and prints
what it would do.

## What this session closed

Sixteen entries. Twelve were the ones the last session filed from a consumer
agent's thirteen defects against the published `v1.3.0`; two the operator added
mid-session; four came out of doing the work.

| entry | what it was |
| --- | --- |
| [TOOL-17](tooling.md) | the suite could not have caught any of the thirteen. Four capabilities and a consumer harness |
| [TOOL-18](tooling.md) | one row of the index's counts was typed, and the gate was green over it |
| [WSL-42](wsl-toolkit-go.md) | what this tool owns, and how it proves it |
| [WSL-43](wsl-toolkit-go.md) | many agents, many bases |
| [WSL-44](wsl-toolkit-go.md) | what a long-lived helper freezes |
| [WSL-45](wsl-toolkit-go.md) | a deadline that bounds the caller's wall time |
| [WSL-46](wsl-toolkit-go.md) | the answer is exactly what happened |
| [WSL-47](wsl-toolkit-go.md) | the boundary the manual promises |
| [WSL-48](wsl-toolkit-go.md) | the embedded script tells the truth |
| [WSL-49](wsl-toolkit-go.md) | one command to readiness |
| [WSL-50](wsl-toolkit-go.md) | a heartbeat for a job that is running |
| [WSL-51](wsl-toolkit-go.md) | a config the agent does not have to write |
| [WSL-52](wsl-toolkit-go.md) | the six commands that make an answer actionable |
| [WSL-53](wsl-toolkit-go.md) | `selfupdate`, and readiness says whether it should |
| [WSL-54](wsl-toolkit-go.md) | the three answers a diagnostic has to tell apart |
| [WSL-55](wsl-toolkit-go.md) | a report that creates the thing it is describing |

## ⭐ The findings worth keeping

**A guard built this session caught a defect written this session.** `TOOL-17`
added a sweep asserting every `--json` surface emits exactly one parsable object.
Hours later it caught `artifacts retry` putting nothing on stdout, in a command
written after the same defect had been fixed in `base ensure`. ⭐ That is the
strongest evidence this tree has that a named guard beats another pass of care.

**The mutation pass caught a NEW test being theatre.** A `WSL-54` case asserted
that an engine which answered nonzero was reported with its own code. Collapsing
the three branches back into one left it GREEN, because the fallback message
APPENDS the underlying error and the substring appears in both. It asserts the
negative now.

**Three defects came from DRIVING rather than reading**, and none was visible in
the code: a missing identity marker read as "could not read the marker", one
disagreement was reported twice under two names, and `selfupdate --check` on a
build ahead of the newest release offered a downgrade.

⚠ **One reported cause did not reproduce.**
[Issue 27](https://github.com/Azathothas/ToolKit/issues/27) says
`-Action Doctor` fails with a `StandardOutputEncoding` exception. Measured three
ways on this host and it does not; both places this tree sets that property set
redirection first. The half that IS this tool's defect was fixed, and the
non-reproduction is written under `WSL-48` rather than into it.

## Measurements

Read from the machine, on Windows 11 Pro 26200, on 2026-09-10:

```text
acceptance  67 of 67 cases pass against the real base, both routes, both
            accounts, every catalog image, and two instances built in one run.
            It was 39 at the start of this session.
consumer    12 cases against the published wsl-toolkit-v1.3.0, downloaded and
            digest-verified, run from a temp directory with no repository
            present. 11 pass and 1 fails, which is correct: it tests a
            published artifact and that artifact has the defect WSL-45 fixes.
selftest    131 cases over 36 functions of the built bundle.
gate        18 checks, one binary, all passing.
deadline    a 2s timeout over a 60s payload returns in 5.3s and no longer grows
            with the payload. It was 15.2s, reported as 4.4s.
tick        a twelve-row matrix with 5s ticks: 95.9s wall and 72 events,
            against 97.2s and 0 without. The estimate in WSL-50 was high by
            about three times.
podman rm   10.63s on a running container, 0.56s with -t 0. That difference is
            where WSL-45's missing seconds were.
```

## What is left, and none of it is urgent

Nine entries, none of them from this session's subject:

- **[WSL-56](wsl-toolkit-go.md)** is the half of `WSL-50` that was not built: a
  deeper inspection surface for a job that has already failed. ⚠ It has an
  obstacle worth measuring first, which the entry names: `run --rm` removes the
  container before anything can read its last state.
- **[TOOL-11](tooling.md)**: CI does not run Windows PowerShell 5.1, which is
  where every P0 has been. **[TOOL-12](tooling.md)** is now half answered:
  `consumer.ps1` exists and nothing runs it after a release.
- **[WSL-25](wsl-ephemeral.md)** through **[WSL-30](wsl-ephemeral.md)**, the
  older `wsl-toolkit.ps1` backlog.
- **The doctor pair is still the last shell twin**, 646 lines across two files
  asking DIFFERENT questions of different hosts, so porting it is a behaviour
  decision. [TOOL-14](tooling.md) says why it was left, and when it goes
  `check-twins.sh` goes with it.

## Work order

⭐ **Cut the release first**, because everything below is measured against a
published artifact and the published one now carries thirteen known defects. The
command is at the top of this file.

Then, in rough order of what unblocks the most:

1. **[TOOL-12](tooling.md)**, which is now one line of `release.ps1` rather than
   a project: run `consumer.ps1` against the tag CI just published, and refuse
   to report success until it has. `TOOL-17` settled that it is not in CI.
2. **[WSL-56](wsl-toolkit-go.md)**, the inspection surface.
3. **[TOOL-11](tooling.md)**, the 5.1 job.
4. The `wsl-ephemeral` backlog, oldest first.

⚠ **A SIXTH REVIEW LENS IS STILL OWED, and concurrency is still the candidate.**
The last session named it and this one did not run it, which is worth saying
plainly rather than leaving to be inferred. What two of this tool running at once
do to one state directory is now a LARGER question than it was: instances give
two agents separate stores, and nothing yet proves two processes sharing ONE
store behave.

⚠ **What a later session should know before touching this tool** is unchanged
and still the trap: `scripts/windows/wsl-toolkit/wsl-toolkit.ps1` and
`tools/windows/wsl-toolkit/internal/script/wsl-toolkit.ps1` are BOTH generated
from the parts, and editing either by hand is lost at the next build.
[RULES.md](RULES.md) section 4 owns that.

## Guards added this session, and why each exists

A session that finds one inconvenient should read why before changing it.

| guard | what it refuses |
| --- | --- |
| the `--json` sweep in `acceptance.ps1` | a surface that advertises `--json` and puts nothing, or two documents, on stdout. It has already caught one command written after it. |
| `Test-Case -MaxSeconds` | a wall-time claim nothing compares. Every case was timed and none asserted on it. |
| `Show-Bytes` | a substring test over an invented byte. `stderr_bytes` was one too many for two releases. |
| `New-StateHome` and `Set-StateConfig` | a suite that can only build state before the first invocation, so nothing a long-lived process caches can be reached. |
| the `Resolve-DistroListing` cases | a refusal rendered as an empty machine. Mutation-proved: deleting the exit-code branch turns two of them red. |
| `TestABuildAheadOfTheReleaseIsNotAnUpdate` | a version check that compares strings for inequality and offers a downgrade. |
| the read-only state case | a report that creates the directory it is describing. Four commands had it. |
