# SUMMARY.md

⚠ **A SNAPSHOT, AND [`PROGRESS.md`](PROGRESS.md) IS THE AUTHORITY.** A session
that reads this and acts on it is reading what was true last time.

---

## 2026-10-26T20:34:53Z to 2026-10-28T00:11:56Z, ruling 36, and it ends on `wsl-toolkit-v9.0.1`

| row | measured |
| --- | --- |
| Elapsed | **1,657 minutes** of wall clock, paused at the usage limits |
| Commits | 73 on `main` since `c8277ef`, this one included, all pushed |
| Released | ⭐ **`wsl-toolkit-v9.0.0` and `v9.0.1`**, 22 assets each and 19 release jobs green each, 9.0.1's on a second attempt. Both SHA-256 verified here, and `selfupdate` put each on this host |
| Work | ⭐ **267 of 267 entries done**. This window filed `WSL-200` to `WSL-207`, `DOC-16` and `TOOL-41`, and closed them with `WSL-134`, `WSL-135`, `WSL-138`, `WSL-158`, `WSL-159`, `WSL-196` to `WSL-199` and `TOOL-40` |
| Changes | 192 files changed, 21407 insertions(+), 1096 deletions(-), `c8277ef` to this commit |
| Size | 208,797 tracked lines to 229,108, **+20,311** |
| Checks | gate **24 of 24** before every push; the pre-push proof ran each push's added rows on Linux; the full acceptance suite **111 of 111** with the installed 9.0.1 on a fresh instance |
| Guards | **1,821 mutation rows to 2,161**. Each row added went red on Windows and on Linux, or skips where its case cannot run with the reason. Four rows whose cases need `jq` skip in the Linux proof's image and went red in CI. CI ran the whole table green on every push |
| Reviews | **an adversarial pass per group** before it merged. linksafe had two, of 13 findings and then 9, and `TOOL-40` one of 9, each fixed or answered with evidence |
| Health | 0 open, 0 blocked. Tree clean, nothing unpushed. No scratch instance, container or job is left |

⛔ **What the session got wrong.** It handed the operator steps it could take itself: a
WSL shutdown, a rename, a stock-host run. Ruling 36 ended that. Its drive script failed
three times on its own configuration: no schema, automount on, and one source granted
twice. One run was started after a failed edit and stopped before it built anything. A
heredoc fed to Python lost backslashes four times, once putting a backspace byte in the
record, and each was caught before a commit. A skill paragraph over the sentence ceiling
cost one Linux proof, stopped midway.

⚠ **What is not measured.** `TOOL-41`'s retry has not met a reset, so only its first try
ran. `WSL-204`'s deadline rests on cases, not on a stalled mirror. The four `jq` rows are
proved in CI alone. podman's machine disk keeps its 1.55 GB length, since compacting it
needs an elevated shell. pi's key for its startup model is the operator's credential,
`WSL-135`. On this host, typing `omp` reaches scoop's copy before the tool's launcher,
and the PATH order is the operator's.
