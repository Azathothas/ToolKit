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
entries         total 43  open 0  blocked 0  done 43
```

⚠ The counts above are checked against [`INDEX.md`](INDEX.md)'s rows by
`scripts/common/check-record.sh`, which runs as a gate. ⛔ Do not edit them by
hand to make a check pass; fix whichever file is wrong.
⭐ `scripts/common/set-record.mjs recount` moves them for you.

⭐ **The backlog is empty for the first time.** Every entry is closed. That is a
statement about the entry list and not about the tool: what a next session should
do is below, and it is a list the operator rules on rather than a work order.

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

⛔ **There is none, because there are no open entries.** The next session's first
job is to agree one.

⭐ **A candidate list was put to the operator interactively at the end of this
session.** Whatever they accepted is filed as entries and appears in
[`INDEX.md`](INDEX.md); if the index shows none open, nothing was accepted and
the next session asks rather than inventing work.

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

### 4. ⚠ One path shipped this session is reasoned rather than measured

⛔ **The release workflow had never run end to end when it was written.** Its
YAML parses, and every command in it was run by hand in some form, but a
workflow is proved by a run. ⚠ The first release is therefore the test of the
release pipeline. **Recommendation:** cut the first tag and watch it, which is
what this session does last; whatever that run reports is recorded here by the
session that reads it.

### 5. The no-POSIX-shell branch of `check-gate.ps1` is still reasoned

Carried unchanged from two sessions ago. Nothing this session did touched it.
