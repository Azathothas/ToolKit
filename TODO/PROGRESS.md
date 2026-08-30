# PROGRESS.md

⭐ **The one file every session reads first.** Where the work is, what is next,
and why. [`INDEX.md`](INDEX.md) is the list of entries and the **order lives
here and nowhere else**. [`RULES.md`](RULES.md) is the half of the record that
does not change between sessions, and [`SUMMARY.md`](SUMMARY.md) is the last
session's table, which is a snapshot rather than an authority.

⛔ Rewritten every session. It carries no history: the history is the git log
and the entries themselves. Do not add a "previous sessions" section.

⛔ Edited in the same change as the work, never as a report afterwards.

---

## State

```text
session started 2026-08-30T07:17:24Z
baseline        gate green at 2ffa680, tree clean, 15 checks passing and
                check-twins skipped by --fast
entries         total 51  open 8  blocked 0  done 43
```

⚠ The counts above are checked against [`INDEX.md`](INDEX.md)'s rows by
`scripts/common/check-record.sh`, which runs as a gate. ⛔ Do not edit them by
hand to make a check pass; fix whichever file is wrong.
⭐ `scripts/common/set-record.mjs recount` moves them for you.

⚠ **The eight open entries were all filed at the END of this session**, from a
list put to the operator interactively and accepted item by item. ⛔ None of them
has been started, and none is a defect: every defect this session found was fixed
in it.

---

## What this session did

**2026-08-30. The tool became a project, and this repository started publishing
something.**

⭐ **`WSL-21` through `WSL-24`, `TOOL-09`, `TOOL-10`, `DOC-06` and `DOC-07` are
closed.**

- **The 2,792-line script is 27 parts and a build.**
  `scripts/powershell-windows/wsl-ephemeral.ps1` is now
  `scripts/windows/wsl-toolkit/wsl-toolkit.ps1`, GENERATED from `src/`, `core/`
  and `libs/`. ⭐ The split was proved byte-identical before anything else
  changed, and the read-only actions were compared between the old file and the
  new bundle: same stdout, same stderr, same exit codes.
- ⛔ **The old path is gone rather than redirected**, on the operator's ruling
  after a measurement: `raw.githubusercontent.com` serves a git symlink's own
  target string with HTTP 200, so a symlink at the old path would have answered
  a successful-looking 34 bytes. A 404 is loud.
- **A release pipeline**, which is the first thing this repository has ever
  published. `release.ps1` verifies and tags; `release.yml` re-verifies on a
  clean checkout and publishes with a `SHA256SUMS` computed there. The launcher
  grew `-LauncherRelease`.
- **Thirteen parameters and an action**, most of them adopted from the mockup
  the issue cited: `Doctor`, `-DryRun`, `-TimestampColumns`,
  `-TimestampProfile`, `-PrefixOnly`, `-Color`, `-StreamLogPath`, `-EventLog`,
  `-Redact`, `-MaxLineBytes`, `-TickEscalateSeconds`, `-ScriptArgFile` and the
  rest.
- **Two new gate checks** and a suite that went from 63 cases to 115.

### ⚠ What was found while doing it, and by which pass

| what | how it was found |
| --- | --- |
| a `$state` local shadowing a `$State` parameter, killing the tick mid-run | ⭐ driving a real distro. The suite could not see it, and PSScriptAnalyzer does not flag it. |
| ⛔ `-ScriptArg` documented as repeatable and never bindable through `-File` | driving the documented example out of the issue comment that closed `WSL-16` |
| ⛔ `[int[]] -TickEscalateSeconds 5,9` binding the single value `59` | instrumenting an escalation that silently never fired |
| ⛔ `check-docs.ps1` reporting correct three-deep links as broken | the two halves of the twin disagreeing about a tree neither had seen before |
| a new check reporting one blank finding over a clean tree | reading the finding rather than the exit code |
| the vhdx write TIME advancing while a guest slept | sampling it against a guaranteed-idle guest before claiming it was a progress signal |
| ⛔ a guard reporting clean over its own planted defect, because `-eq` ignores case | the mutation pass, which is the only thing that could have |
| ⛔ `check-no-secrets.ps1` unable to match a Windows home path at all | the FULL gate. `check-twins` named the drift, and `--fast` skips it. |
| ⛔ `-TimestampProfile raw` reading settings its own branch never built | the door sweep, on "what other door reaches this code" |
| a sink-path refusal a `-DryRun` returned before ever reaching | the guard-mutation pass, planting `-StreamLogPath nul` |
| three of my own new test cases passing for the wrong reason | the two cases in the same block that expected SUCCESS |

⭐ **Nine of those ten were found by driving, measuring or comparing rather than
by reading**, which is parts (b) and (c) of the gate restated with this
session's own evidence. ⚠ The tenth, the last row, is the one that matters most
about the suite: three cases named for a guard were green because the function
died before reaching it.

### ⭐ Two questions the last session left open are now answered

1. **The stream log under Windows PowerShell 5.1** was carried as reasoned
   rather than measured. It is measured now: a real Alpine distro, driven under
   `powershell.exe`, with the carriage-return redraw, the delta column and the
   exit code all correct.
2. **Whether to split the file** carried a recommendation not to. The operator
   ruled to split, and `WSL-21`'s closing corrects the half of that reasoning
   which said it was not possible.

---

## ⭐ The work order

⭐ **Eight open entries. Take them in this order and the reason is written down.**

1. ⭐ **`TOOL-11`**, Windows PowerShell 5.1 in CI, first and it is the smallest.
   Both P0 defects this tool has ever had lived on 5.1, this session and the last
   one both had to check it by hand, and every entry below ships code that 5.1
   has to run. ⛔ Doing it first means the rest are covered as they land rather
   than checked afterwards.
2. **`TOOL-12`**, the release smoke and its weekly re-check. Second because the
   release path now has exactly one proof, taken by hand on one day, and three of
   the entries below change what a release contains.
3. **`WSL-25`**, signing. Third because it is the last thing standing between a
   consumer and a pin they can trust without holding a digest, and because
   `TOOL-12` is what will prove the signed release is still consumable.
4. ⭐ **`WSL-28`**, replay and compare. It is pure functions over a file the tool
   already writes, the suite can cover all of it with no WSL, and it is the
   cheapest of the four feature entries.
5. **`WSL-27`**, the progress protocol, then **`WSL-26`**, the rootfs cache. Both
   change what a long quiet run looks like, and `WSL-27` is the one that makes
   `WSL-26`'s saving measurable rather than asserted.
6. **`WSL-29`**, `-Reuse`, is small and last of the feature work because it
   overlaps `WSL-26`: if the cache lands first, reuse may be the wrong shape.
7. ⚠ **`WSL-30`, the podman adapter, is XL and is genuinely two entries.** Its
   first step is the mockup's validation matrix, sixteen unmeasured claims, and
   the entry says in as many words that more than a couple of absent feeds should
   split it rather than push it through.

---

## Open questions for the operator

Each carries a recommendation, so agreeing costs nothing.

### 1. ⭐ The consumer pins have not moved, and this session broke all three rows

⛔ **`scripts/powershell-windows/wsl-ephemeral.ps1` 404s now.**
`Azathothas/TEMPLATE` and `Azathothas/bit-cli` both name it. Their existing pins
keep working, because a pinned commit still has the old path in its tree; they
break on their next bump.

**Recommendation:** move each to a release tag rather than to a new commit,
because a release stops them caring where in the tree the file lives, which is
the thing that broke them this time. ⛔ That is a change in those repositories
and this one cannot make it.

### 2. Should `check-binfmt` join the gate?

**Recommendation: no**, unchanged. It needs a running podman machine, so on a
machine without one it is a permanent skip, and a check that is always skipped
is one nobody reads.

### 3. `HUMAN.md` and `SECURITY.md` are still not written

**Recommendation:** leave them until there is something to put in them. An empty
skeleton outlives the session that wrote it. ⚠ The argument for `SECURITY.md`
got slightly stronger: `docs/public/README.md` names it as where a finder looks,
and this repository now publishes a runnable artefact rather than only a tree.

### 4. ⭐ The release pipeline ran, and it is no longer a reasoned claim

`wsl-toolkit-v1.0.0` was tagged and the workflow published it on its first run.
The whole path was then driven the way a consumer meets it: a bare `launcher.ps1`
in an empty directory, no sibling, pointed at the release.

```text
  * release wsl-toolkit-v1.0.0
  * digest matches the SHA256SUMS in release wsl-toolkit-v1.0.0
  ! that proves the bytes arrived intact, not who published them.
```

A real distro was created, run and destroyed through it, and the inner command's
exit code reached the caller through both layers.

⚠ **One thing is worth knowing before the next release.** The published
`launcher.ps1` is 55,185 bytes and the working tree's is 54,078: the workflow
uploads what it checks out, which is CRLF, and the tree here is what an editor
last wrote. That is why `release.ps1` prints its own digests with a line saying
not to copy them anywhere, and why `SHA256SUMS` is computed in CI.

### 5. The no-POSIX-shell branch of `check-gate.ps1` is still reasoned

Carried unchanged from two sessions ago. Nothing this session did touched it.
