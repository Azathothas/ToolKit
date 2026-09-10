# SUMMARY.md

⚠ **A snapshot of one session, not an authority.**
[`PROGRESS.md`](PROGRESS.md) is what is true now; this is what the session that
wrote it measured on the day. A session that reads this and acts on it is acting
on what was true last time.

---

## 2026-09-10

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-09T21:00:00Z | about 9 hours, across two resumed contexts |
| Commits | `450b380` | 13 on `main`, counting the one that carries this number, and two tags: `wsl-toolkit-v1.2.0` and `wsl-toolkit-v1.3.0` |
| Work | 8 issues filed against the published `v1.1.0`, none started | **14 entries closed, 11 filed and not started, 0 deferred, 0 failed.** `WSL-32` to `WSL-39` resolve issues 7 to 14; `TOOL-14` ports five shell pairs; `WSL-40` and `WSL-41` are the core pass and the review after it; `TOOL-15` and `TOOL-16` are what the operator's question produced |
| Changes | 201 tracked files | 234 tracked files; 78 changed, +10,885 / -2,789 lines |
| Go modules | 2, `tools/check` and `tools/windows/wsl-toolkit` | 3. `tools/repo` is the tool box for what is NOT a gate check, and `check-gate` still runs only rules |
| Suite | 39 Go cases | 129 Go cases, all three modules clean under `-race`. ⚠ `tools/check` had **zero** before this session and has 5 |
| Acceptance | 23 cases against a real machine | ⭐ **39 cases.** The 14 issue cases each fail against `v1.1.0`; the two newest cover the helper route's own transcript and the job id |
| Mutation | 23 guards proved, by a script under `.tmp/` | ⭐ **48 proved**, by `repo mutate` in the tree. Each one deleted, the named case run, the case count and the build status reported separately |
| Published | `wsl-toolkit-v1.1.0` | `wsl-toolkit-v1.3.0`, five assets, digests recomputed from the downloaded files |
| Checks | 17 in one Go binary, 31s | 18, 43s. Five shell pairs became a Go program and none of them became a gate check; the one that was added is `hooks`, for the reason below |
| Health | 53 entries: 8 open, 0 blocked, 45 done | 78 entries: 20 open, 0 blocked, 58 done. Tree clean, gate green, CI green, release verified as a consumer |

### ⭐ Defects found, and by which pass

Sixteen, and ⛔ **eight of them were found with nothing reported broken**, in a
tree that had just closed eight consumer-filed issues and passed 37 acceptance
cases.

| what | the pass that found it |
| --- | --- |
| `--max-output` honoured on the direct route and dropped by the helper | the door sweep. ⚠ Second time a job flag has drifted |
| `base --root` in the code and in no manual | the claim audit |
| `io.MultiWriter` in the client spool would abort a helper job over a local disk | reading the sink before writing the test |
| `ClientSpool.Finish("")` leaked one staging directory per job with no id | the same reading |
| `boundedBuffer.truncated` read without the lock | the same reading |
| `logs --tail 5` read `5` as the job id | writing the case for the flag |
| a kept guest directory was spared by gc FOREVER, because its ledger record stayed open | the acceptance run, on a real failure |
| `closeStaleRecords` looked at `GuestDir` only, so helper records never closed | reading why the above happened |
| `prefixWriter` raced: one instance, two copiers, one unlocked slice | the core pass |
| a catalog id of `..` passed `isImageID`, and `matrix --artifacts` writes `out/<id>` | the core pass |
| `Ledger.Compact` read with no lock and then took the lock to write | the core pass |
| `NewClientSpool` had four silent returns where the direct route logs | the fifth review |
| `Finish` returned nothing while holding a complete transcript | the fifth review |
| `logs` reported every read failure as "no transcripts yet" at exit 0 | the fifth review |
| `helper stop` discarded the reason nothing answered | the fifth review |
| the job id was nowhere in the human output, so `logs ID` was untypeable | the fifth review |
| the refusal for a bad image id described a rule the input satisfied | driving the PUBLISHED v1.3.0 binary as a consumer |

### ⛔ Three process failures worth keeping

**I committed over a red gate.** I chained `&&` off `tail -8` rather than reading
the gate's own exit code, which is this repository's own oldest rule and the one
written down in [RULES.md](RULES.md). The commit had not been pushed, so the
finding was fixed and the commit amended; the broken form never left the machine.
Every gate run since reads `$?` from the gate with no pipe in front of it.

**I put a tool credit in a commit message, and it reached a protected `main`.**
The harness asks for that trailer and re-asserts the request continuously; the
repository's rule against it is read once, at orientation. That asymmetry is the
mechanism, and it had already produced eighteen such commits in an earlier
session. ⛔ **The gate could not have caught it and still cannot.** Its
`commits` rule reads `git log`, and the written procedure is run the gate, then
commit, so at the moment the gate runs the commit does not exist. CI caught it
eleven minutes after the push, on a branch whose protection forbids a force push,
and undoing it cost the operator turning that protection off and on again.
`TOOL-15` is the fix: the rule now runs from a `commit-msg` hook, on the message,
before the commit exists, and an eighteenth gate check refuses a checkout that is
not running the hook.

**I wrote a claim I had not measured.** A comment said the race detector was what
made a lock's removal VISIBLE. Measured ten runs each way, the broken version went
red in 6 of 10 plain runs and 10 of 10 under `-race`, so the detector buys
determinism and not visibility. The comment, the record and the harness all carry
the numbers now. ⚠ The lesson is the cheaper one: the measurement took four
minutes and the claim would have stood for as long as the file did.

### ⛔ A closed entry was carrying a false claim

The sixth review read the twelve filed entries as a session with no memory of
writing them would, and checked every reference rather than every sentence. Six
`file:line` seams all resolved to the line they name. Every cross-reference
resolved to an entry that exists. Two citations of PRIOR entries did not.

`WSL-44` credited a probe cache to `WSL-19`, which is the timeout entry; it is
`WSL-32`. `WSL-46` credited the marker framing to `WSL-38`, which is the
positional-arguments entry; it is `WSL-39`. ⭐ **Both survived the gate,
because its link check proves a linked FILE exists and cannot know whether the
entry inside it is the one meant.**

Following the second one found the thing worth keeping. `WSL-39`, closed and
shipped, says the marker is stripped before any sink sees it. It is not: the
newline in front of it is written back, which is
[issue 23](https://github.com/Azathothas/ToolKit/issues/23), filed against a
release this repository cut believing that sentence. The entry is amended in
place rather than corrected, because a test named for a property the code does
not have is the more useful half of that record.

⚠ **A third citation was missing rather than wrong.** `WSL-19` bounded a
hung run by bounding the CHILD. `WSL-45` is the discovery that bounding the child
does not bound the caller, and it did not mention the entry it supersedes.

### ⚠ One test was theatre and is now labelled

`TestLedgerCompactDoesNotLoseAConcurrentAppend` went red in **0 of 10 runs**
against the defect it was written for, because `Open` took the lock too and an
append almost never lands in the gap. It was renamed to
`TestLedgerSurvivesConcurrentUse` and its comment states what it proves and what
it does not. The invariant is held by structure: `Compact` takes the lock once and
calls `openLocked`, which is checkable by reading nine lines.

⛔ **A test that passes ten times out of ten against the defect is worse than
no test**, because it is read as coverage. The mutation harness carries a comment
where that row would go, saying why there is no row.
