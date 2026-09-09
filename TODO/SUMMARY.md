# SUMMARY.md

⚠ **A snapshot of one session, not an authority.**
[`PROGRESS.md`](PROGRESS.md) is what is true now; this is what the session that
wrote it measured on the day. A session that reads this and acts on it is acting
on what was true last time.

---

## 2026-09-09

| row | before | after |
| --- | --- | --- |
| Elapsed | started 2026-09-09T05:51:14Z | about 16 hours, across one resumed context |
| Commits | `bf11930` | 5 on `main`, and one tag: `wsl-toolkit-v1.1.0` |
| Work | issue 6, unstarted | **2 entries closed, 0 deferred, 0 failed.** `WSL-31` resolves the issue in full; `TOOL-13` was filed and closed inside the session |
| Changes | 130 tracked files | 201 tracked files; 124 changed, +19,310 / -4,441 lines |
| Checks | 18, as sh and PowerShell pairs, 13m20s | ⭐ **17 in one Go binary, 31s**, all passing. The twin comparison left the gate and stayed in CI |
| Suite | 123 PowerShell cases | 123 PowerShell cases, plus 34 Go cases and a 23-case acceptance runner against a real machine |
| Published | `wsl-toolkit-v1.0.1`, two assets | ⭐ `wsl-toolkit-v1.1.0`, five assets, including the executable for two Windows architectures. The workflow succeeded on its first run |
| CI | green at `bf11930` | green at `87b7775`. ⚠ It went red once, on three jobs, over three defects the local gate structurally cannot see |
| Cost | no money | 12 catalog images pulled, one 4 GiB WSL distribution built and kept, ~20 containers commissioned and destroyed |
| Health | 52 entries: 9 open, 0 blocked, 43 done | 53 entries: 8 open, 0 blocked, 45 done. Tree clean, gate green, release driven end to end |

### ⭐ Defects found, and by which pass

Sixteen. ⛔ **Four were found by CI or by a driven run and could not have been
found by reading**, and three were in the checks rather than in the code.

| what | the pass that found it |
| --- | --- |
| the helper route accepted `--user` and ran the job as root | the door sweep |
| the launcher decided "verification failed" from the wording of an error | the door sweep |
| a named release lost to a stale executable beside the launcher | the door sweep |
| `-LauncherSha256` ignored on every path that names a file | the door sweep |
| `-LauncherLocal` and `-LauncherRef` still went to the network for a binary | driving the launcher from an empty directory |
| the protected-distribution list could be deleted with the suite still green | the guard mutation |
| `--user` was in the code and in neither manual | the claim audit |
| `build.ps1 -Check` named line 1 for every difference in the embedded copy | the claim audit |
| `gc` exited 0 over a list of things it could not remove | reading the gc path after a leftover |
| a workspace symlink pointing out of the tree was packed, not refused | ⛔ CI's ubuntu job; the case cannot run on Windows |
| `WindowsPathToGuest` answered differently on Linux | ⛔ CI's ubuntu job |
| a runner's short-form temporary directory failed a path comparison | ⛔ CI's windows job |
| shellcheck 0.11.0 and 0.8.0 disagree about one line | ⛔ CI, and reproduced in a container afterwards |
| two banned adjectives in live pages, with nothing enforcing the rule | the Go port reading a rule nothing had read |
| the ported tree loader dropped untracked files from every check's scope | planting a credential and watching nothing report it |
| eight acceptance cases failing over `Start-Process -ArgumentList` | running the acceptance runner for the first time |

### ⚠ What was NOT done, said rather than left to be found

- **`check-twins` still takes 5m35s** and now runs outside the gate. The six
  pairs left are genuinely two implementations; porting the doctor probe would
  remove most of that, and nothing here depends on it.
- **The `go` and `checks` CI jobs are not required status checks.** `main`
  requires three, and the compiled half is not one of them. Adding it is a
  branch-protection change and the operator's to make.
- **The consumer pins have not moved**, and this release breaks three things
  they may rely on. [`../docs/consumers.md`](../docs/consumers.md) carries a row
  for each; moving a pin is a change in another repository.
