# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-10T08:30:00Z
baseline        af3de74, clean main; gate 18 checks, all passing.
entries         total 86  open 7  blocked 0  done 79
gate            19 checks, one binary, 42s on this host
```

## Active work

None in flight. ⭐ **`wsl-toolkit-v2.0.0` is published**, the thirteen issues a
consumer filed against `v1.3.0` are closed against it, and nothing is open on
the remote except one dependency bump the operator has to approve.

## What this session closed

Six entries, and two of them were found by the work rather than planned.

| entry | what it was |
| --- | --- |
| [TOOL-11](tooling.md) | CI runs both PowerShell hosts now, and compares the counts |
| [TOOL-12](tooling.md) | a published release is fetched and driven, after every publish and weekly |
| [TOOL-19](tooling.md) | three of the guards had stopped being proved and nothing said so |
| [WSL-56](wsl-toolkit-go.md) | `inspect`: what a failed job ran on |
| [WSL-57](wsl-toolkit-go.md) | two selftest cases were written against one host's environment |
| [WSL-58](wsl-toolkit-go.md) | ⚠ FILED, NOT CLOSED. `inspect` is the one report the helper cannot serve |

## ⭐ The findings worth keeping

**Four commits sat unpushed, so the second host had not looked at any of them.**
The first CI run that saw them went red on ubuntu and stayed green here: two
selftest cases built a path out of `$env:TEMP` and `$env:WINDIR`, which are null
under PowerShell on Linux. ⭐ **The finding worth more than the fix is that the
ubuntu half is reachable from this Windows host in about ten seconds**, in a
container, so that class no longer has to be found by pushing and waiting.

**The instrument that proves the guards had stopped proving three of them.**
`repo mutate` reported 58 of 61. Two rows matched nothing because the code moved
under them; a third was called theatre against a case that had never run,
because a skipped case is byte for byte a passing one from outside the process.
⛔ **That is the harness's own defect class, in the harness**: its header says
three outcomes and not two because a previous version collapsed three answers
into one, and it had then collapsed a fourth.

**Putting the consumer suite in CI found two defects in the suite, immediately.**
A strict-mode property read that threw on the host it was written to tolerate,
and a hand-rolled readiness probe that was wrong twice: `base status` was wrong
on a runner, and `ready`'s `route.wsl_callable` was also wrong, because
`wsl.exe` answers there and a distribution still cannot be built. ⭐ The probe is
`base ensure` now, which is the thing itself rather than a signal that
correlates with it.

**WSL-56's stated obstacle did not hold, and measuring it first was the ruling
that paid.** The entry said `run --rm` removes the container before anything can
read its last state. podman's event journal outlives the container and carries
`ContainerExitCode`, so `--rm` stays and the fallbacks the entry offered were
not needed.

⛔ **The claim audit caught a false claim that had already been published.** A
comment closing issue 19 asserted that the mutation table proved a guard. It did
not; there was no row. The row exists now, it was run, and the comment carries a
correction rather than a quiet edit.

## Measurements

Read from the machine, on Windows 11 Pro 26200, on 2026-09-10:

```text
gate        19 checks, 42s. `check mutations` alone is 556, 560 and 390 ms over
            three runs, most of it loading the tracked file list the gate
            already shares.
acceptance  69 of 69 cases against the real base, both routes, both accounts,
            every catalog image. It was 67 at the start of this session.
            ⚠ This line read 71 until the run finished and was compared
            against it. A number written before the measurement is a
            fabrication whatever it turns out to be.
consumer    12 of 12 against the published wsl-toolkit-v2.0.0, downloaded and
            digest-verified, from a temp directory with no repository present.
            On a GitHub windows runner the same suite is 12 cases, 0 failed,
            6 skipped.
selftest    131 cases over 36 functions, and the same numbers under PowerShell 7
            on Windows, Windows PowerShell 5.1, and PowerShell 7 on Linux.
mutation    66 rows, all proved on ubuntu. On this host one is SKIPPED, because
            creating a symbolic link here needs a privilege this process may
            not have.
podman      the guest engine is 6.1.1, crun, overlay on extfs, cgroups v2,
            rootless, event logger `file`.
```

## What is left, and none of it is urgent

Seven entries, and the shape of the list is worth naming rather than leaving to
be inferred.

- ⚠ **[WSL-25](wsl-ephemeral.md) through [WSL-30](wsl-ephemeral.md) are the
  2026-08-30 `wsl-toolkit.ps1` backlog, and NOTHING HAS RE-DERIVED THEM AGAINST
  THE TREE SINCE.** They were written when that script was the only product
  here. The compiled tool has since taken over the same ground from a different
  direction: `base ensure` keeps one long-lived Linux host, `--tick` is a
  heartbeat, `run` is podman-native, and `inspect` reads a job back. ⛔ **That
  does not close any of them** - the script is still what a consumer fetching
  one raw URL gets, and `RULES.md` section 6 refuses a "won't fix" - but the
  next session to pick one up should measure whether the problem it names is
  still the problem, and write the answer into the entry either way.
- **[WSL-25](wsl-ephemeral.md)** is the one that is certainly still real and is
  not about the script at all: the release digest proves transport and not
  authorship, and the launcher says so out loud on every fetch. It is ruled:
  sigstore keyless, verification optional in the change that adds it.
- **[WSL-58](wsl-toolkit-go.md)** is this session's own door sweep: `inspect`
  has no `--via-helper` and every other report does. Its cost is a helper
  protocol version, which is why it did not land with the command.

## Work order

1. ⭐ **[WSL-25](wsl-ephemeral.md)**, signing. It is the only open entry that
   changes what a consumer can verify, and the release it applies to is now
   published and unsigned.
2. **[WSL-58](wsl-toolkit-go.md)**, the helper's `inspect`. It is a protocol
   bump, so it wants to travel with any other protocol change rather than alone.
3. **Re-derive [WSL-26](wsl-ephemeral.md) to [WSL-30](wsl-ephemeral.md) against
   the tree**, oldest first, writing the answer into each entry. ⚠ `WSL-30` is
   XL and its own decision section pre-authorises the split: the validation
   matrix is one entry and the adapter is the other.

## ⛔ Open questions for the operator

⚠ **These are questions, not work.** Each is here because a session cannot rule
on it.

- ⛔ **[Pull request 15](https://github.com/Azathothas/ToolKit/pull/15) is ready
  and needs an approving review, which is the operator's.** `actions/setup-go`
  6.5.0 to 7.0.0. Verified on 2026-09-10: the pinned commit
  `b7ad1dad31e0` belongs to `actions/setup-go`, the tag `v7.0.0` resolves to it,
  and its `action.yml` declares `node24`, which is current. ⚠ Its `checks
  (ubuntu)` is red on a stale base: the branch is BEHIND `main` and predates the
  `WSL-57` fix. Updating the branch is expected to make it green.
- **An operator-facing runbook and a threat model are both empty roles**, named
  in [`../docs/conventions/docs.md`](../docs/conventions/docs.md) as
  deliberately unfilled. ⚠ That page said this file carried them as an open
  question and it did not, which the claim audit found; this line is that
  sentence being made true rather than deleted.
- ⚠ **A SIXTH REVIEW LENS IS STILL OWED AND CONCURRENCY IS STILL THE
  CANDIDATE.** Two sessions have now named it and neither has run it. What two
  of this tool running at once do to ONE state directory is unproven: instances
  give two agents separate stores, and nothing yet proves two processes sharing
  one store behave.

⚠ **What a later session should know before touching this tool** is unchanged
and still the trap: `scripts/windows/wsl-toolkit/wsl-toolkit.ps1` and
`tools/windows/wsl-toolkit/internal/script/wsl-toolkit.ps1` are BOTH generated
from the parts, and editing either by hand is lost at the next build.
[RULES.md](RULES.md) section 4 owns that.

## Guards added this session, and why each exists

A session that finds one inconvenient should read why before changing it.

| guard | what it refuses |
| --- | --- |
| `check mutations` | a mutation row that no longer reaches its subject. It found three the day it was written, one of them created by a rename made while writing it. |
| the `_test.go` refusal inside it | a row that mutates the CASE rather than the guard. Breaking an assertion turns the case red and the harness prints `ok`, which is theatre with the harness's own seal on it. |
| `SKIPPED` in `repo mutate` | a skipped case counted as proved, or accused of being theatre. Neither is what a skip means. |
| the ubuntu `mutations` job | a row this host cannot prove going unproved anywhere. Nothing skips there. |
| the 5.1 selftest step | a construct Windows PowerShell 5.1 cannot parse reaching a release with every check green. Planted and measured. |
| `TestEveryJSONSurfaceReachesTheSweep` | a `--json` surface that never reaches the sweep built to check them. It refused `inspect` the moment it existed, and refused an exemption naming a surface that does not exist. |
| the dispatch table `manual_test.go` walks | a whole new command arriving with undocumented flags. The hand-written list it replaced had exactly that hole. |
| `release-smoke.yml` | a release that publishes and cannot be fetched or run. It has already been red twice, both times about the suite rather than the release. |
