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
session started 2026-08-30T04:56:24Z
baseline        gate green at 8efe6e0, tree clean, 14 checks passing and
                check-twins skipped by --fast
entries         total 40  open 4  blocked 0  done 35
```

⚠ The counts above are checked against [`INDEX.md`](INDEX.md)'s rows by
`scripts/common/check-record.sh`, which runs as a gate. ⛔ Do not edit them by
hand to make a check pass; fix whichever file is wrong.
⭐ `scripts/common/set-record.mjs recount` moves them for you.

⚠ **The gate grew a sixteenth entry this session**, `wsl-ephemeral selftest`,
in both halves and in both CI jobs. It is the first test in this tree as
distinct from a check, and it found a defect on its first run.

---

## What this session did

**2026-08-30. Issue 5, in full, and the three things a consumer had to do by
hand.**

⭐ **`WSL-16` through `WSL-20` are closed.** Every complaint in the issue is
answered, and two of them are answered more completely than they were asked.

- **The stream log**, on by default. A timestamp on every line a command
  produces, a stream tag, a heartbeat while there are none, and a carriage
  return treated as a line terminator so a progress meter is visible instead of
  being twenty minutes of nothing. `-NoTimestamps` turns all of it off and is
  byte-exact.
- **`-CommandFile` repairs the copy it sends**, rather than warning and sending
  it anyway: CRLF to LF, a byte order mark removed, UTF-16 refused by name.
  `-ScriptArg NAME=VALUE` passes values in without anybody running `sed` over a
  payload, and `@hostaddress` resolves through the same code `-Action
  HostAddress` uses.
- **`-CommandTimeoutSeconds`**, opt-in with no default, exit 124.
- **`-LauncherRef auto` and `latest`, `-LauncherSha256 auto`, and a lock file.**
  A consumer never pastes a commit or a digest again, and neither keyword ever
  fetches a branch: both resolve to a commit first.
- **Three hosts are tried for the bytes**, with `api.gh.pkgforge.dev` behind
  `api.github.com`. ⛔ No `gh`, no `curl`, no external tool at all.
- **A test**, `wsl-ephemeral-selftest.ps1`, 63 cases over 15 functions, wired
  into both halves of the gate and both CI jobs.

### ⚠ What was found while doing it

| what | how it was found |
| --- | --- |
| a byte-array concatenation that threw on every `-ScriptArg` | the selftest, on its first run, before any distro existed to hit it |
| a tick advancing the delta clock, so a five-second gap read as `+0.619` | ⭐ driving a real distro. The suite could not have seen it. |
| a PowerShell hashtable folding `%m` into `%M` | the parser refused it, which is the loud version |
| `Invoke-WebRequest`'s `.Content` arriving as a byte array | the shape check on the resolved commit |
| the proxy enforcing a user-agent allowlist | measuring it rather than reading its page |

Each is in
[`../docs/HISTORY/wsl-ephemeral.md`](../docs/HISTORY/wsl-ephemeral.md), which is
where that kind of text goes from now on.

### ⛔ What was NOT done, and it was assigned

The session pivoted at the operator's instruction with about a third of its
budget left, to finish the issue rather than half-finish everything.

- **`DOC-06` is `partial`, not done.** `docs/HISTORY/` exists, the check exempts
  it in both halves, and the WSL tool page was purged. `scripts/README.md`,
  `docs/consumers.md`, `docs/conventions/docs.md` and `prose.md` were not.
- **`DOC-07` was not started.** Both `AGENTS.md` files are still there. The
  entry records what was measured about removing the root one, including the
  check exemption that has to move with it.
- **The interactive task list was not presented.** The entries below were
  written from the session's own findings instead, and `WSL-21` carries the
  language and file-splitting question with a recommendation rather than a
  ruling.

---

## ⭐ The work order

⭐ **Four open entries. Take them in this order and the reason is written down.**

1. ⭐ **`DOC-06`**, the documentation purge, because it is half applied and a
   half-applied convention is worse than an unapplied one: a reader cannot tell
   which pages follow it. Four files, named in the entry's closing section.
2. **`DOC-07`**, the second `AGENTS.md`, because it is small, it is the second
   time it has been asked for, and `DOC-06` will have touched the same pages.
3. **`WSL-23`**, parameters ignored by the actions they do not apply to. It is a
   break and it is the last honest gap in the surface this session doubled.
4. ⚠ **`WSL-21` needs a ruling, not work.** Its recommendation is to stay in
   PowerShell and not split the file, and the acceptance is to produce the
   numbers rather than to make the change.

⚠ **`WSL-22` is deliberately last and may never be done.** It records six `tss`
flags that were not implemented, so nobody implements them because the list
exists.

---

## Open questions for the operator

Each carries a recommendation, so agreeing costs nothing. None blocks work.

### 1. ⭐ Is PowerShell still the right language, and should the file be split?

**Recommendation: stay in PowerShell, and do not split yet.** The full argument,
including what would change the answer, is in `WSL-21`. The short form: the
script drives a Windows binary and reads Windows state, so a compiled rewrite
buys a single binary and pays for it with a release pipeline this repository
does not have and does not want. And a single file is what makes the launcher's
contract possible: one URL, one digest, one thing to verify.

⚠ **This is the one question this session was asked to put to the operator and
did not get to interactively.** It is here rather than in chat because the
session ended before that exchange.

### 2. Should `check-binfmt` join the gate?

**Recommendation: no**, unchanged. It needs a running podman machine, so on a
machine without one it is a permanent skip, and a check that is always skipped
is one nobody reads.

### 3. `HUMAN.md` and `SECURITY.md` are still not written

**Recommendation:** leave them until there is something to put in them. An empty
skeleton outlives the session that wrote it.

### 4. ⚠ Two paths shipped this session are reasoned rather than measured

- The no-POSIX-shell branch of `check-gate.ps1`, carried from last session.
- ⛔ **The stream log has not been run under Windows PowerShell 5.1.** It uses
  `Task.WaitAny`, `StreamReader.ReadAsync` and `[string]::new(char[],int,int)`,
  all of which exist on .NET Framework 4.5, and it parses and analyses clean
  under both hosts. **That is a reasoned claim and not a measured one**, and
  5.1 is exactly the host `WSL-12` shipped a P0 against. **Recommendation:**
  run one `-Action Run -Command` under `powershell.exe` early next session and
  record the result here.
