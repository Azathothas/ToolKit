# SUMMARY.md

⚠ **A SNAPSHOT, AND [`PROGRESS.md`](PROGRESS.md) IS THE AUTHORITY.** A session
that reads this and acts on it is reading what was true last time.

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
| Cost | ⭐ **the guard job 26.4 to 19.7 minutes in CI**, and 6.40 to 2.01 s per row locally; 122 MiB reclaimed by a sweep that did not exist |
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
