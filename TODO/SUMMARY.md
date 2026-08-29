# SUMMARY.md

⭐ **The last session's summary table, saved as well as printed**, per
[`../docs/methodology/sessions.md`](../docs/methodology/sessions.md). It is the
fastest orientation into what the last session actually did.
[`PROGRESS.md`](PROGRESS.md) is what to read next, and it is the authority.

⛔ Overwritten each session. It is a snapshot, not a log.

---

## 2026-08-29, the session that closed the issue list and found six defects doing it

| row | before | after |
| --- | --- | --- |
| **Elapsed** | probe at 2026-08-29T14:26:11Z | see the printed table for the closing instant |
| **Commits** | `7127ff7` | 1, squashed, pushed to `main` |
| **Work** | 4 issues open, 0 entries open | ⭐ **4 issues answered, 12 entries filed and closed, 0 deferred, 0 failed** |
| **Changes** | | 82 files, **+7,078 / -2,021**. 27 added, 10 removed, and one of the removals is `docs/README.md` becoming `docs/AGENTS.md` |
| **Size** | 19,611 lines, 75 files | 25,260 lines, 92 files, **+5,649**. ⚠ **1,488 of those are SPDX licence texts**, which are data rather than work |
| **Checks** | 13 gate entries, 12 passed 1 skipped, 49.8s fast | ⭐ **15 gate entries, all 15 passed**, 379s full and 66s fast. ⛔ The two added found **167 marker problems and 17 two-home sentences** in the tree the baseline called clean |
| **Twins** | 10 pairs compared, and the PowerShell gate half skipped six checks | 13 pairs, all agreeing, plus the licence texts byte-for-byte. The PowerShell half now runs 15 and skips none |
| **Cost** | | no money. Two `alpine:3.22` pulls of 8.2 MiB and two 76 MiB probe distros, one at a time, all four removed. ⚠ **Nothing left on disk**: `podman system df` back to zero on all three rows, and no distro registered |
| **Health** | 18 entries, 0 open | 30 entries, 0 open. 6 defects cleared, ⚠ **0 introduced that a check can see**, and **1 branch** shipped reasoned rather than measured and named as such |

### ⛔ The line that matters most in that table

**A green baseline is a statement about the checks that ran.** This session
started from a gate that passed thirteen checks in 49.8 seconds against a tree
carrying 164 characters that break its own prose rule, three pages over its own
marker ceiling, and 17 sentences with two homes. Nothing was wrong with the
gate's arithmetic. What was missing was two checks.

---

## What each issue asked for, and what it got

| issue | the ask | the answer |
| --- | --- | --- |
| 1 | report consumed resources, offer cleanup rather than doing it | `-Action Resources`. Three sections, every cleanup command printed and none run |
| 1 | `-Action HostAddress`, so a caller stops creating a distro to read `/proc/net/route` | done, and verified against a real distro's own answer: both `172.23.96.1` |
| 1 | `-PortForward`, **or** documentation that NAT needs a non-loopback bind | ⛔ the flag refused, with the reason. The action says the bind address every time it runs in NAT mode |
| 1 | a wrapper, improved, that fetches and makes the tool runnable | `wsl-ephemeral-launcher.ps1`, with its own page |
| 2 | the template skeletons quote placeholders verbatim | `docs/templates/` deleted. `TODO/` is the shape the work model names |
| 3 | `docs/AGENTS.md` is missing and `docs/README.md` is unusable | `docs/AGENTS.md` written to be read in full. `README.md` is for a person |
| 4 | bring the non-essential scripts across and iterate on all of them | 4 helpers and the licence texts moved, 2 drifted checks refreshed, `mine-repo` deliberately left |

---

## ⛔ Six defects, and four of them were reporting success

⚠ **None of these was in any issue.** Each was found by running something, and
each has an entry and a row in
[`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md).

- **`yaml parses` in CI had never parsed a file.** `glob` skips dot-directories.
  Fixed by enumerating with git and asserting the count. `TOOL-08`.
- **`check-remote-items` called an unread issue a failed check**, and its two
  modes disagreed on the same tree. `TOOL-05`.
- **`check-gate.ps1` skipped six checks on the one host it exists for.**
  `TOOL-06`.
- **`deslop` printed the count it planned to remove**, in both halves.
  `TOOL-07`.
- **164 characters and 17 duplicated sentences.** `DOC-02`, `DOC-03`.

⭐ **Two more were found by driving the launcher rather than reading it**, and
neither was visible in review: a splatted argument list that passed every
parameter name positionally, and a wrapper's progress output landing in the
value its caller was capturing.

---

## ⚠ What was NOT measured, so it is not claimed

- ⛔ **WSL's own behaviour under mirrored mode**, that a host service on loopback
  is reachable from a distro there. ⭐ The script's mirrored branch IS measured,
  against a fixture profile; what is not is the claim about WSL that makes the
  answer useful.
- ⛔ **The no-POSIX-shell branch of `check-gate.ps1`.** What was verified is that
  every check now runs through its twin here and that the two gate halves agree.
- ⚠ **The download mark the launcher clears was never present.**
  `Invoke-WebRequest` did not attach one, so that path reported nothing to
  clear, which is what it is written to do.
- ⚠ **The resource figures in issue 1 were not re-derived.** They describe a
  machine on the day they were written and this one had been pruned before the
  session began. The mechanism behind them was checked; the gigabytes were not,
  so they are not repeated anywhere in the tree.
- ⛔ **CI is green at the pushed commit or it is not**, and the closing report
  names the run rather than predicting it.

---

## The three review passes, and what each found

⭐ **Three different questions, not one sweep written up three times.** Each pass
names what it looked at that the others did not.
[`../docs/methodology/reviews.md`](../docs/methodology/reviews.md) is the
specification.

### Lens 1, the door sweep: what other door reaches this code?

Enumerated every affordance the session added, then grepped for the callers and
surfaces that were not on the list.

- ⛔ **`Get-DirectorySizeBytes` had no containment guard**, while every writing
  path in the same file had two. It builds a path from a distro name `wsl.exe`
  reported and walks it recursively, so a name carrying a traversal would send
  the walk outside the base directory. It only reads, which is why it looked
  fine. Fixed: `Assert-InsideBaseDir` runs first, and the legitimate path was
  re-driven afterwards to prove the guard does not refuse it.
- ⚠ **`WSL-09`'s closure named `Invoke-WslBounded` in the present tense** about a
  function `WSL-13` had renamed. Corrected underneath rather than edited away.
- ⚠ **The launcher was the only `.ps1` in the tree with LF line endings.** The
  index was right so no check fired, and a fresh clone would have produced CRLF
  anyway; normalised so the working tree matches every sibling.

⚠ **What did NOT fire:** the sweep for a second path into a destructive action
found none. Both new actions are read-only and neither reaches the deletion
helper, which was confirmed by listing every caller of it rather than by reading
the two new functions.

### Lens 2, the guard mutation: can the new guards actually fail?

Planted the defect each guard exists to catch and read the exit code unpiped.

| guard | planted | result |
| --- | --- | --- |
| the yaml step's empty scope | a pattern matching nothing | ✅ exit 1, refused |
| the launcher's digest check | a digest of all zeros | ✅ exit 1, fetched copy deleted |
| the launcher's ref shape | a branch name | ✅ exit 1, refused |
| the launcher's missing ref | no sibling, no ref | ✅ exit 1, printed the resolving command |
| `deslop --apply` on a dirty tree | the tree, as it was | ✅ exit 1 in both halves, nothing removed |
| the `.wslconfig` parse | four fixture profiles | ✅ all four, including a refusal for bridged |
| `check-markers`, `check-one-home` | the tree, as it was | ✅ 167 and 17, then 0 and 0 |

⭐ **The last row is the strongest of them.** Both checks were seen to refuse a
real tree before they were seen to pass one, which is the only way to know a
check is not theatre.

⚠ **Two branches shipped without a mutation.** `Invoke-PsCheck`'s missing-file
skip and `check-gate.ps1`'s no-shell path. What would have had to be true for
either to fire is written into `TOOL-06`.

### Lens 3, the claim audit: which sentence is not backed by an artefact?

Re-read every number and every cited path against the tree and the run logs.

- ⛔ **A count of 181 problems that was never measured.** It was the sum of two
  numbers done in the head, and the two numbers are 167 and 17. Replaced with
  both figures rather than a total.
- ⛔ **A breakdown of the 164 characters that used the SECOND measurement**, taken
  after the session had itself added two offending lines. Replaced with the
  first run's five codepoint counts, and labelled as lines rather than
  characters, which is what the check reports.
- ⛔ **Three size figures that had moved** while the session kept working: the
  file count, the line delta and the added-file count. Re-taken last.
- ⛔ **A table of thirteen documents in `docs/conventions/docs.md`, seven of which
  have never existed here**, and one of the seven was named as the authority a
  conflict between two documents is settled against. Rewritten to the set this
  repository has, with the two deliberately empty roles named as empty.
- ⛔ **`docs/public/README.md` said `SECURITY.md` carries the security contact.**
  There is no such file here, and a rule naming a file that does not exist reads
  as a rule that is being followed.
- ⚠ **A consumer row calling the vendored copy "536 lines against this tree's
  1,579"** when this tree's file is now 1,980. Both numbers kept, with their
  dates.
- ⭐ **And one claim that was too pessimistic.** The record said the mirrored and
  bridged branches were reasoned rather than measured. Lens 2 had by then
  measured both against fixture profiles, so the record was corrected in the
  other direction: what remains unmeasured is WSL's own behaviour under mirrored
  mode, which is a claim about WSL rather than about this code.
