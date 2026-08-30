# SUMMARY.md

⚠ **A snapshot of one session, not an authority.**
[`PROGRESS.md`](PROGRESS.md) is what a session reads first, and it is the file
that carries the work order. This is the table the last session printed, kept so
it survives the chat scrolling away.

---

## 2026-08-30

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-08-30T04:56:24Z | 1h11m to `c3e0359`, read from the commit's own timestamp |
| Commits | `8efe6e0` | two. `c3e0359` is the work, squashed as asked. A second carries what the closing review pass corrected, because [`../docs/conventions/git.md`](../docs/conventions/git.md) section 5 forbids amending a commit that has left the machine and no force push was needed. |
| Work | 4 assigned areas | **2 done, 2 partial.** Issue 5 is closed in full with five entries; the gate, the reviews and CI are done. The documentation purge is `partial` and the second `AGENTS.md` was not started; the future task list was filed as entries rather than presented interactively. All three gaps are open entries or open questions. |
| Changes | | 24 files, 4 of them new. 3,328 insertions, 449 deletions. `wsl-ephemeral.ps1` 1,980 to 2,792 lines. |
| Size | 30 entries | 40 entries: 5 closed, 5 filed open or partial |
| Checks | 14 passed, 1 skipped | ⭐ **16 entries in the gate**, 15 passed and `check-twins` skipped by `--fast`. Both halves run it. |
| Cost | | 2 distros created and removed, one `alpine:3.22` image pulled and removed. ⚠ The engine and WSL are as they were found. |
| Health | tree clean | tree clean, gate green in both halves, ⛔ **the stream log is unmeasured under Windows PowerShell 5.1** and `PROGRESS.md` question 4 says so |

### What the four review passes found

| lens | what it looked at that the others did not | finding |
| --- | --- | --- |
| door sweep | every caller of the nine changed functions, and every reference to the four that were renamed or removed | none. `Invoke-InDistro` has exactly two callers and one branch; `Enter` bypasses it deliberately; no dangling reference survived. |
| guard mutation | seven planted defects, each read unpiped | ⛔ **one guard was theatre.** A case named for the transport alphabet was satisfied by an earlier pattern check, so disabling the assert it was named for left the suite green. Renamed, and the real guard proved by mutating the skeleton instead. |
| claim audit | every number about to be published, against the artefact that produced it | ⛔ **a wrong line count.** `WSL-21` said 2,300 lines; the file is 2,792. Corrected in the entry, the index row and `consumers.md`. |
| the closing re-audit | this table itself, and the CI logs, after the push | ⛔ **two more wrong numbers, both mine, both in this file.** Elapsed said 2h30m against a commit stamp of 1h11m, and the change count said 21 files against 24. The pass that audits the audit is the one that found them. |

⚠ **A fourth thing was found by neither**, and by the tooling instead: a
JavaScript `String.replace` expanded a dollar sign inside a replacement string
and duplicated a 500-line script. It has a row in
[`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md).
