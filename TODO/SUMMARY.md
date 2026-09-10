# SUMMARY.md

⚠ **A snapshot of one session, not an authority.**
[`PROGRESS.md`](PROGRESS.md) is what is true now; this is what the session that
wrote it measured on the day. A session that reads this and acts on it is acting
on what was true last time.

---

## 2026-09-10, the second sitting

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-10T10:30:00Z | one context, ended at its limit |
| Commits | `0fb74d9`, clean `main` | 3 more on `main`, pushed, `e55dd3f` green on all six CI jobs. No tag. |
| Work | 86 entries: 7 open, 0 blocked, 79 done | 88 entries: **2 open**, 0 blocked, 86 done. **7 closed, 2 filed, both closed** |
| Changes | 249 tracked files | 251; 24 changed, +2,961 / -213 lines |
| Gate | 19 checks, 42s | 19 checks, 27s over two runs, which is not a controlled comparison |
| Selftest | 131 cases over 36 functions | 157 over 40, same on 7.6.5 and 5.1 |
| Acceptance | 69 cases | **71 of 71** against the real base |
| Consumer suite | 12 cases | 14. Both new ones skip on an unsigned release |
| Mutation table | 74 rows | 76. Only the two new rows were re-proved here |
| Script surface | 34 parameters, 9 actions | 40 parameters, 12 actions. Lock refreshed in the same commit |
| Helper protocol | `wsl-toolkit-helper/3` | `wsl-toolkit-helper/4`. A v2.0.0 helper refuses a newer client until restarted |
| Published | `wsl-toolkit-v2.0.0`, unsigned | ⛔ **unchanged. `v2.0.1` was not cut.** |
| Open PRs | 1, dependabot, behind `main` | **1, still behind, further behind now** |

### ⭐ What the session actually found

⛔ **Two guards that could not fail, both found by trying to use them, neither
planned.** `check line-endings` split `attr/text eol=crlf` on whitespace and
kept `text`, then compared the index column that git normalises by definition:
23 of 54 tracked `.ps1` files had the wrong working-tree endings while it
reported green. `git-sync.ps1 -Path a,b` bound one string, so the tool
`AGENTS.md` names as the way to commit on Windows could not name two files.

⛔ **A schema guessed rather than read, twice in one entry.** `WSL-28`'s reader
looked for `kind: LINE` with streams `out`/`err`; the writer emits `kind: LOG`
with `stdout`/`stderr`/`watcher`. It reported a real run as producing no output.
The same guess was in the selftest fixtures.

**Two guards fired on the day they were reached.** The build's AST scan
refused a `$wall` local that IS the `$Wall` parameter, and the selftest's own
count assertion refused a suite that had grown by two cases.

### ⛔ What was asked for and not done

Three things, and the session ended before them rather than rushing them:

| asked | state |
| --- | --- |
| merge the dependabot PR | not merged. Branch still behind `main`. |
| cut `wsl-toolkit-v2.0.1` | not cut. The version inside the file still reads `2.0.0`. |
| `WSL-30` | not started. Its validation matrix has not been run. |

⚠ **Nothing is half-applied.** Everything committed is gate-green and pushed;
the three above were never begun.
