# SUMMARY.md

⚠ **A SNAPSHOT, AND [`PROGRESS.md`](PROGRESS.md) IS THE AUTHORITY.** A session
that reads this and acts on it is reading what was true last time.

---

## 2026-10-21T02:36:37Z to 2026-10-21T08:15:05Z, issues 35 and 37 and pull request 36, and it ends on `wsl-toolkit-v7.0.0`

| row | measured |
| --- | --- |
| Elapsed | **338 minutes** of wall clock, the session's start to the clock |
| Commits | 6 on `main`, this one included, all pushed. `d81910f` is pull request 36, squashed |
| Released | ⭐ **`wsl-toolkit-v7.0.0`**, 22 assets, 8 of 8 release jobs green. Its digest and cosign bundle verified here, and `selfupdate` put it on this host |
| Work | ⭐ **16 entries filed and closed**: `WSL-105` to `WSL-119` and `TOOL-28`. Issues 35 and 37 closed with their comments, pull request 36 merged. 0 deferred, 0 failed |
| Changes | 123 files changed, 8846 insertions(+), 472 deletions(-), `d52040b` to this commit |
| Size | 131,500 tracked lines to 139,874, **+8,374** |
| Checks | gate **24 of 24** at the start and at the end; `check-go` and CI's Linux jobs in containers before every push; the acceptance suite **102 of 102** with a build of `4361680` and **102 of 102** with the installed 7.0.0 |
| Guards | **512 mutation rows to 600**. Each row added went red here, and this session's 89 rows ran in `golang:1.25` with none THEATRE. CI ran the whole table green on `4361680` and `d81910f` |
| Reviews | **3 passes**, run as agents over the whole change set: correctness found 7, safety 4, and the contract review 6 claims of the manual, a `ready` defect and the breaks table. All fixed, each with a case where a case could hold it |
| Health | 158 of 158 entries done. Tree clean, nothing unpushed |

⛔ **What the session got wrong.** Its rows were proved on Windows alone, and CI's
Linux run of `54547dd` found two that stayed green there, because their cases asserted
on Windows only. The reviews then found 11 defects in entries already closed, so their
closings carry dated amendments. The acceptance suite first ran on `s110`, whose own
configuration outranks the files three cases write. A later run failed its last guard
because `s110` was removed while it ran.

⚠ **What is not measured.** A `stop` sent to a job waiting for its `--exclusive`
name is proved by the wait loop's case, not by a real waiting job. A helper upload that
carries executable bits is proved by cases, not by a running helper. The verifier's
allowance for a gone grant's stale mount ran as its extracted block, not on a base
whose directory was deleted. `base reclaim` making a disk sparse needs `wsl --shutdown`,
the operator's step, so only its refusal was driven.
