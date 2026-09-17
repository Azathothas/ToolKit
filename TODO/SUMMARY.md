# SUMMARY.md

⚠ **A SNAPSHOT, AND [`PROGRESS.md`](PROGRESS.md) IS THE AUTHORITY.** A session
that reads this and acts on it is reading what was true last time.

---

## 2026-09-17T14:42:39Z to 2026-09-17T16:30Z

| row | measured |
| --- | --- |
| Elapsed | **107 minutes**, the doctor own stamp to the clock |
| Commits | 3, on `main`, all pushed |
| Work | **3 entries completed** and **9 findings FIXED** that had a named fix and no entry: 1, 12, 13, 15, 23, 31, 32, 37, 41, 59, 67, 84. ⛔ Finding 14 closes as a check that cannot exist, with the measurement |
| Changes | 52 files, **+4,682 / -1,150**. 7 new files |
| Size | about 104,600 tracked lines |
| Checks | gate **24 of 24** throughout. Windows Go over 4 modules exit 0; `check-go.sh` in `golang:1.25` **0 in 36.0 s**; ShellCheck 0.9.0 in `ubuntu:24.04` clean over 51 scripts |
| Cost | no money. One ~700 MiB image pulled; two throwaway distributions and one consumer base built and removed; **122 MiB reclaimed** by the new sweep |
| Health | ⭐ 128 of 128 entries done. **419 mutation rows**, 21 added this session, **20 red and 1 deleted as theatre**. Findings 81 to 99. Tree clean |

⛔ **THE FIRST HALF ENDED BADLY AND THE OPERATOR WAS RIGHT TO REFUSE IT.** The
session was told to finish every open task, closed the two entries, and then
printed a next prompt naming findings with a named fix and no entry. That is
the deferral the instruction forbade. The second half is that work.

⭐ **The timeout the operator asked for.** Every host-engine call goes through
one door with a default ceiling, and a transfer carries a STALL deadline beside
it: the 30-minute ceiling was right and useless against a pull that had simply
stopped. Driven against a real child both ways, a stall fires in **3.2 s**
against a 2 s limit and a process that keeps talking is never touched.

⛔ **Two things this session built were then removed by its own checks**, and
both are recorded rather than quietly kept. A noise filter came back THEATRE
from the mutation harness, because every measured capture makes it unreachable.
An interop check reported the operator own base unusable on its first run, and
`WSL-68` had already measured that signal as misleading in both directions.

⛔ **What is still not measured.** The sibling-repair note is proved by its
cases and was NOT driven against a real stale boot id: podman keeps that state
in its own database and forcing the condition was not worth corrupting it.
