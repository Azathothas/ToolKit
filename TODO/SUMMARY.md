# SUMMARY.md

⚠ **A snapshot of one session, not an authority.**
[`PROGRESS.md`](PROGRESS.md) is what is true now; this is what the session that
wrote it measured on the day. A session that reads this and acts on it is acting
on what was true last time.

---

## 2026-09-10, the afternoon

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-10T08:30:00Z | one context |
| Commits | `af3de74`, four of them unpushed | 6 on `main`, and one tag: `wsl-toolkit-v2.0.0` |
| Work | 85 entries: 8 open, 0 blocked, 77 done | 86 entries: 7 open, 0 blocked, 79 done. **5 closed, 3 filed, 1 of those filed and left open** |
| Changes | 245 tracked files | 249; 40 changed, +2,700 / -320 lines |
| Open issues | 13, every one against the published `v1.3.0` | ⭐ **0** |
| Published | `wsl-toolkit-v1.3.0`, carrying thirteen known defects | `wsl-toolkit-v2.0.0`, driven as a consumer at 12 of 12 |
| Checks | 18 in one Go binary | 19. The new one is `mutations` |
| Acceptance | 67 cases | 69 |
| Mutation table | 61 rows, 58 proved, 2 broken, 1 misreported | 66 rows, all 66 proved on ubuntu |
| CI | 5 jobs, none of which had seen the 2.0.0 commits | 6 jobs, all green, plus a release smoke that runs after a publish and weekly |

### ⭐ What each of the three review lenses found

⛔ **Three passes reporting nothing is a weaker result than one pass reporting a
defect**, so each is written up by what it looked at that the others did not.

**The door sweep** enumerated every affordance this session added, then grepped
for the ones the enumeration missed. Four findings, three fixed:

- ⛔ **`inspect --json` was not in the sweep that checks every `--json`
  surface.** `TOOL-17` built that sweep because commands were being added with
  that defect faster than cases were written, and the very next `--json` surface
  was not in its hand-typed list. The list is walked from the binary's real flag
  sets now, and an omission has to be written down as an exemption with a reason.
- ⛔ **`inspect` gave no answer at all on a host with no `wsl.exe`**, including
  for the transcript and the ledger record, which sit on this machine's own disk
  and which `logs` reads with no runner whatever. Split and tested.
- **`cmd_reach.go` contained no `reach` command** and never had. Renamed.
- ⚠ **`inspect` has no `--via-helper` and every other report does.** Not fixed:
  it is a helper protocol version. `WSL-58`.

**The guard mutation** planted the defect each new guard exists to catch and read
the exit code unpiped. One finding, and it is about the harness rather than the
code:

- ⛔ **Nothing stopped a mutation row from mutating the TEST instead of the
  code.** Breaking an assertion makes the named case go red, the harness prints
  `ok`, and the guard the row claims to prove was never touched. That is theatre
  with the harness's own seal on it, which is worse than an unproved guard
  because it reads as proof. Refused now, planted, and the refusal is itself a
  mutation row.
- Every other new guard was seen to refuse: `check mutations` against the real
  stale row, the sweep guard against `inspect` taken back out, the 5.1 CI step
  against a planted ternary, and six new rows in the table.

**The claim audit** re-read what was about to be published against the artefacts.
Three findings:

- ⛔ **A false claim that had ALREADY BEEN PUBLISHED.** The comment closing
  issue 19 said the mutation table proved a guard. There was no row for it. The
  row exists and was run; the issue carries a correction rather than a quiet
  edit.
- ⚠ **"The cost did not move" over two uncontrolled runs.** The gate was 43s at
  18 checks last session and 42s at 19 now, on the same host, which supports
  "inside the noise" and not what was written. Both places now carry the numbers
  and their conditions.
- ⚠ **`docs/conventions/docs.md` said `PROGRESS.md` carried the runbook and
  threat-model roles as an open question, and it did not.** Made true rather
  than deleted.
- ⛔ **AND THE PASS CAUGHT ITSELF.** `PROGRESS.md` was written with the
  acceptance count typed as 71 while the run was still going, and the run
  reported 69. The line carries both the number and that sentence, because a
  figure written before its measurement is a fabrication whichever way it lands.

### ⛔ What the second host found that this one could not

Four commits were made on 2026-09-10 and none was pushed, so the ubuntu job had
not run since `f7eabfe`. The first run that saw them went red there and stayed
green here: two selftest cases composed a path from `$env:TEMP` and
`$env:WINDIR`, which are null under PowerShell on Linux.

⭐ **The durable half is that the ubuntu answer is now available from this
Windows host in about ten seconds**, in a container, so the same class does not
have to be found by pushing and waiting.

### ⚠ One entry's premise was disproved by measuring it

`WSL-56` said `run --rm` removes the container before anything can read its last
state, named that as the obstacle, and asked whoever took it to measure first.
The measurement says otherwise: podman's event journal outlives the container
and the `died` and `remove` events both carry `ContainerExitCode`. ⭐ So `--rm`
stays, and the entry's fallback options were not needed. Two of its requirements
described things that do not exist here, and both are written into the closing
rather than dropped.
