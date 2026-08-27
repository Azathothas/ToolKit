# SUMMARY.md

⭐ **The last session's summary table, saved as well as printed**, per
[`../docs/methodology/sessions.md`](../docs/methodology/sessions.md). It is the
fastest orientation into what the last session actually did.
[`PROGRESS.md`](PROGRESS.md) is what to read next, and it is the authority.

⛔ Overwritten each session. It is a snapshot, not a log.

---

## 2026-08-27, the `WSL-06` to `WSL-11` batch

| row | before | after |
| --- | --- | --- |
| **Elapsed** | probe at 2026-08-27T09:46:21Z | 2026-08-27T11:31:12Z, 1h 45m |
| **Commits** | `4837470` | `13bb310`, **8 commits** here, plus **1** in `Azathothas/TEMPLATE` (`83f573c`) |
| **Work** | 6 assigned: `WSL-06` `WSL-07` `WSL-08` `WSL-09` `WSL-10` `WSL-11` | ⭐ **6 completed, 0 deferred, 0 failed.** Plus `TOOL-03` filed and closed unassigned, and 2 door-sweep fixes recorded under existing entries |
| **Changes** | | 10 files, **+1996 / -181**. No file added or removed |
| **Size** | 16,180 lines, 74 files | 17,995 lines, 74 files, **+1,815**. `wsl-ephemeral.ps1` 979 to 1,579 |
| **Checks** | `check-gate --fast`: 12 passed, 1 skipped | ⭐ `check-gate` **full: 13 of 13, 0 skipped**. The `.ps1` twin agrees. ⚠ The full gate was **not** run at the start, only `--fast`; the 13/13 has no before to compare against |
| **Cost** | | no money. 5 OCI images pulled: rootfs sizes 8.2, 45.4, 74.3, 76.9 and 190.7 MiB. ⚠ **Download volume was not measured** and no number is given for it |
| **Health** | 17 entries, 8 open, 9 done | 18 entries, **2 open**, 16 done. Tree clean. CI green on all three jobs at both ends |

### What moved

| | |
| --- | --- |
| ⭐ `wsl-ephemeral.ps1` | **no open entry left.** Every `WSL-*` item is closed |
| ⭐ the command channel | base64 through one checked function; `-CommandFile` and `-CommandB64` added |
| ⭐ the consumer pin | `Azathothas/TEMPLATE` moved to `ea5d483`, verified by running the wrapper on both hosts |
| ⛔ `TOOL-03` | a P0 in `git-sync.ps1`: it committed under an author of `sh scripts/common/check-control-bytes.sh` and printed `identity verified` under it |

### Debts introduced, named rather than left

- ⛔ **`Azathothas/TEMPLATE` carries `git-sync.ps1` with `TOOL-03`'s defect.**
  Not fixed there: that tree is read-only to this one except for the pin.
- ⚠ **Three things worth an entry are unfiled**, listed in
  [`PROGRESS.md`](PROGRESS.md) rather than invented as work nobody asked for.

### What was NOT measured, so it is not claimed

- ⚠ **A real out-of-space `--import`.** The `WSL-06` guard is proven to fire and
  to leave nothing registered; filling a 428 GiB volume to see the fault itself
  was not done.
- ⚠ **The `--unregister` release race.** Still unreproduced, exactly as `WSL-04`
  recorded. The rollback now terminates first, which is two paths agreeing
  rather than a measured fix.
- ⚠ **A human TTY session for `-Action Enter`.** This harness cannot allocate a
  terminal. Everything under the TTY was driven: stdin, `-User`, the exit code.
