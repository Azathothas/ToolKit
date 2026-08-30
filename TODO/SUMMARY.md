# SUMMARY.md

⚠ **A snapshot of one session, not an authority.**
[`PROGRESS.md`](PROGRESS.md) is what is true now; this is what the session that
wrote it measured on the day. A session that reads this and acts on it is acting
on what was true last time.

---

## 2026-08-30

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-08-30T07:17:24Z | 2026-08-30T09:35Z, about 2h18m |
| Commits | `2ffa680` | 4 on `main`, and two tags: `wsl-toolkit-v1.0.0` and `v1.0.1` |
| Work | 4 open entries assigned | **8 completed, 0 deferred, 0 failed.** `WSL-21` `WSL-22` `WSL-23` `WSL-24` `TOOL-09` `TOOL-10` `DOC-06` `DOC-07`. 8 new entries filed from a list the operator accepted item by item. |
| Changes | 96 tracked files | 130 tracked files; 69 changed, +9,607 / -614 lines across four commits |
| Size | 28,228 tracked lines | 37,221 tracked lines, +8,993 |
| Checks | 15 passing, `check-twins` skipped by `--fast` | ⭐ **17 of 17 passing on the full run**, `check-twins` included. Two new: `wsl-toolkit bundle` and, inside `build.ps1 -Test`, a case-shadowed-parameter scan. |
| Suite | 63 cases over 15 functions | 117 cases over 30 functions, green on both PowerShell hosts |
| Published | ⛔ nothing, ever | ⭐ two releases, `wsl-toolkit-v1.0.0` and `v1.0.1`, each carrying `wsl-toolkit.ps1`, `launcher.ps1` and a `SHA256SUMS` computed in CI. The workflow succeeded on its first run and on its second. |
| CI | green at `2ffa680` | green at `89397c1`. ⚠ It went red once in between, on the ubuntu job, over a defect the local gate structurally cannot see. |
| Cost | no money, no bandwidth beyond fetches | ~15 distro create-and-destroy cycles, `alpine:3.22` at 8.2 MiB each, all torn down |
| Health | 4 open, 0 blocked, 35 done | 8 open, 0 blocked, 43 done. Tree clean, gate green, `wsl-toolkit-v1.0.0` deployed and driven end to end. |

### ⭐ Defects found, and by which pass

Eleven, and ten of them by driving, measuring or comparing rather than by reading.
Three were in the checks themselves rather than in the code being checked, and
one was found only by CI, on a host the local gate cannot be.

| what | the pass that found it |
| --- | --- |
| a `$state` local shadowing a `$State` parameter, killing the tick mid-run | driving a real distro |
| `-ScriptArg` documented as repeatable and never bindable through `-File` | driving the documented example from the issue comment |
| `[int[]] -TickEscalateSeconds 5,9` binding the single value `59` | instrumenting an escalation that silently never fired |
| `check-no-secrets.ps1` unable to match a Windows home path at all | the FULL gate; `check-twins` named the drift |
| `check-docs.ps1` calling correct three-deep links broken | the two halves of that twin disagreeing |
| `-TimestampProfile raw` reading settings its own branch never built | the door sweep |
| a sink-path refusal that `-DryRun` returned before reaching | the guard-mutation pass |
| a new check reporting one blank finding over a clean tree | reading the finding, not the exit code |
| three of the session's own test cases passing for the wrong reason | the two cases beside them that expected success |
| the vhdx write time advancing while a guest slept | sampling it against a guaranteed-idle guest |
| a Windows-semantics guard answering differently on Linux | CI's ubuntu job, which the local gate cannot be |

### ⚠ What was NOT done, said rather than left to be found

- **The consumer pins have not moved**, and all three rows are broken by the path
  change. That is a change in three other repositories and this one cannot make
  it. `PROGRESS.md` open question 1.
- **The eight new entries are filed and unstarted.** None is a defect; every
  defect found this session was fixed in it.
- **`HUMAN.md` and `SECURITY.md` are still absent**, and publishing an artefact
  strengthens the case for the second one slightly.
