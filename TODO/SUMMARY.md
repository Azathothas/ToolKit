# SUMMARY.md

⚠ **A SNAPSHOT, AND [`PROGRESS.md`](PROGRESS.md) IS THE AUTHORITY.** A session
that reads this and acts on it is reading what was true last time.

---

## 2026-09-27T04:41:27Z to 2026-09-27T16:41:19Z, issue 34, and it ends on `wsl-toolkit-v6.0.0`

| row | measured |
| --- | --- |
| Elapsed | **719 minutes**, the doctor's own stamp to the clock |
| Commits | 7 on `main`, `d8b47d7` to this one, all pushed, plus two tags |
| Released | ⭐ **`wsl-toolkit-v5.0.0` and `wsl-toolkit-v6.0.0`**, 22 assets each, 7 of 7 release jobs green each, the consumer smoke included. This host runs 6.0.0 through `selfupdate` |
| Work | ⭐ **13 entries closed**: `WSL-94` to `WSL-101` from issue 34, `WSL-102` to `WSL-104` found on the way, `DOC-08` and `TOOL-26`. 0 deferred, 0 failed. Issue 34 closed; 0 issues and 0 pull requests open |
| Changes | 105 files changed, 11091 insertions(+), 1676 deletions(-), `6ec3fbc` to this commit |
| Size | 116,645 tracked lines to 126,060, **+9,415** |
| Checks | gate **24 of 24** at the start and at the end; Go on Windows and in `golang:1.25`; ShellCheck 0.9.0 over 51 scripts; acceptance **102 of 102** with the installed 6.0.0. CI green on both tagged commits. ⚠ CI was cancelled on `a20b4d2` and `c96873b` at the guard job's limit, which is `TOOL-26`, and red on `5a667cc` on one THEATRE row |
| Guards | **419 mutation rows to 512.** CI on `f9560bb`: 509 proved, 0 THEATRE, 0 BROKEN, 3 that skip on Linux |
| Cost | the guard job: one runner cancelled at 30 minutes, then three runners of 10 to 12 minutes each. ⚠ Runner minutes and bandwidth were not measured. 260.3 MiB of scratch removed |
| Reviews | **6 passes.** Before 5.0.0: the door sweep found 5 gaps, the guard mutation 6 weak rows, the claim audit 2 claims that no drive backed. In the close-out: the door sweep found `WSL-104`; the guard mutation found a TERM that the KILL hid, and a row that `go vet` refused; the claim audit found a commit message's cause false, a page's claim about `base revoke` false, a false CI claim in the draft comment for issue 34, 7 doubled rules and a ruling number used twice |
| Health | 141 of 141 entries done. Tree clean, nothing unpushed. This host runs 6.0.0 |

⛔ **Two things the session got wrong and then caught.** `3290937`'s message
says that the dash on Ubuntu reads `kill -TERM --`, and a measurement in three
images says that no dash does: the KILL after the grace hid the TERM that
failed. And the close-out found a defect in the release it had just published,
so 6.0.0 followed 5.0.0 on the same day.

⚠ **Decided without the operator.** `WSL-104` refuses flags that parsed, so it
is a break and needs a major version. The session made that decision
unattended, under `WSL-100`'s ruling for the same class, and the entry says so.

⚠ **What is not measured.** A Windows job object that refuses the detached
owner's breakaway: the answer warns, and no harness here builds one.

---

## 2026-09-17T14:42:39Z to 2026-09-18, and it ends on a published release

| row | measured |
| --- | --- |
| Elapsed | **234 minutes**, the doctor own stamp to the clock |
| Commits | 5 on `main`, all pushed, plus the tag |
| Released | ⭐ **`wsl-toolkit-v4.0.0`, 22 assets**, all three release jobs green including the consumer smoke |
| Work | **3 entries closed**, **13 findings FIXED** that had a named fix and no entry, and finding 14 closed as a check that cannot exist with the measurement |
| Changes |  58 files changed, 5155 insertions(+), 1179 deletions(-) |
| Checks | gate **24 of 24**; Windows Go over 4 modules; `check-go.sh` in `golang:1.25`; ShellCheck over 51 scripts. CI green on every pushed commit |
| Guards | **419 mutation rows.** Full table: **390 ok, 0 THEATRE, 0 BROKEN**, 29 platform skips |
| Cost | ⭐ **the guard job in CI: 26.4, 26.6, 27.2 min before; 19.7, 24.0, 24.4 after.** ⚠ Three runs each way, so roughly **26.7 to 22.7 on the mean**, and two of the three after sit close to the old range. The firm claim is the per-row one, **6.40 to 2.01 s**, measured on the same 20 rows. 122 MiB reclaimed by a sweep that did not exist |
| Reviews | **17 passes**: 4 on the first half, 4 on the second, 3 more lenses, and 3 each over the documents, the record and the skills |
| Health | 128 of 128 entries done. Tree clean, nothing unpushed |

⛔ **Two things the session got wrong and then caught.** It ended the first half
by handing over a list of findings instead of fixing them, and the operator
refused it. And it wrote five timestamps it had not read, two of them in the
future, which its own claim audit found before printing.

⭐ **The timeout that was asked for.** Every host-engine call goes through one
door with a total ceiling and a STALL deadline beside it, because a ceiling set
for the worst legitimate transfer cannot catch a hang. Driven against a real
child: a stall fires in **3.2 s** against a 2 s limit, and a process that keeps
talking through three stall windows is never touched.

⛔ **The release took two attempts and the first destroyed itself.**
`gh release create` uploads inside the call that creates, and a retried upload
collided with itself and deleted the release. The publish is idempotent and
resumable now and reads back what is actually there.

⛔ **What is still not measured.** The sibling-repair note is proved by its
cases and was never driven against a real stale boot id: podman keeps that state
in its own database and forcing it was not worth corrupting it.
