# SUMMARY.md

⚠ **A SNAPSHOT, AND [`PROGRESS.md`](PROGRESS.md) IS THE AUTHORITY.** A session
that reads this and acts on it is reading what was true last time.

---

## 2026-09-17T14:42:39Z to 2026-09-17T17:05Z

| row | measured |
| --- | --- |
| Elapsed | about 2h 23m, from the recorded start instant |
| Commits | 1, on `main`, over `105dfdc..HEAD` |
| Work | **3 completed, 0 deferred, 0 failed.** `WSL-59` and `WSL-91` were the two open entries; `WSL-93` was filed and closed in the same session |
| Changes | 33 files, **+2,949 / -275**. 4 new files |
| Size | 102,379 tracked lines, **+2,472** |
| Checks | gate **24 of 24** at both ends, 35.9 s then 32.5 s. ⚠ The recorded baseline said 21 of 21 and the gate had 23 checks at that commit |
| Cost | no money. One container image pulled to the host engine, about 700 MiB; two throwaway distributions built and purged; one consumer base built and removed |
| Health | ⭐ **0 entries open, 128 of 128 done.** 13 mutation rows added, **13 of 13 red**. 8 findings recorded, 81 to 88. Tree clean, nothing unpushed |

⭐ **What closed.** The observation layer reaches a container: one relay, an
`Observer` seam, and each adapter describing its own kind of thing. The gate
drives the released-binary contract on every commit, so finding 70's shape is
caught before a tag rather than after one.

⛔ **What the session found that it was not looking for.** A published binary
with a silent no-op, `text-tool --between`. A manual asserting a behaviour the
tool did not have, with the entry that would have built it agreeing. An analyzer
walking one directory of three, where its only real finding was. A file git
cannot see blocking this repository's own pre-push check.

⛔ **What was NOT explained.** A `podman pull` stalled dead for 28 minutes and
the same pull then took 221 seconds, with zero bytes and zero processor time
measured while it was stuck. The cause is unknown and is written as unknown.
The silence around it was ours and is fixed.
