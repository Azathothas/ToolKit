# shell.md

Traps in passing text and reading results between shells, and the rules that
avoid them. Every entry here is a defect that actually happened, and most of
them fail silently, which is why they are worth a document.

The shape of almost all of them is the same: **a payload crosses a boundary and
loses its quoting, or a result crosses a boundary and loses its meaning.**

---

## 1. A prose payload goes through a file. Not through a shell.

⛔ **Write the text to a file with a file-writing tool, then pass the path.**
This applies to a commit message, a document, a script, a JSON body, anything
multi-line, and anything containing an apostrophe, a backtick, a dollar sign or
a backslash.

The reason it is a file and not "better quoting" is that quoting is not
sufficient. Measured on 2026-08-25:

| how the payload travelled | result |
| --- | --- |
| written to a file, then read by the shell | 8657 bytes, byte-exact, exit 0 |
| passed inline to `bash -c` inside a **quoted** heredoc `<<'EOF'` | the backticks in the prose were **executed**: `origin: command not found` |

The second row is the surprising one. A quoted heredoc is supposed to be
literal, and when the payload is handed to a shell as an inline string it is
not reliably so. The minimal reproduction is one line of text containing
`` `backticks` `` and it fires with LF endings and with CRLF endings alike.

⚠ **The way this fails is worse than an error.** The file is written, nothing
returns non-zero, and the damage is a substituted or truncated fragment
somewhere in the middle of a long document. The first sign is usually a commit
subject that reads like a fragment of the body.

Related failures with the same cause:

- A PowerShell here-string written inside a `bash` command is parsed by bash
  first. `@'...'@` is an `@`, a single-quoted string, and an `@`, so it ends at
  the first apostrophe in the text. The phrase "the run's own deadline" turns
  the rest of a commit message into shell commands.
- `python -c` and `python - <<'PY'` are fine for code with no apostrophes and
  no backslashes. A Windows path in a Python string literal has both.
- A backslash escape that survives one hop loses a backslash on the next, and
  the receiving language reads what is left as an escape sequence. `\b` becomes
  a backspace byte and `\f` becomes a form feed. The file is written, nothing
  errors, and a regex that was supposed to end in a word boundary now ends in a
  byte no editor shows.

⚠ **Some agent harnesses collapse `\\` to `\` before the shell sees it.** Verify
it in the environment you are in rather than assuming either way:

```bash
printf 'literal: C:\\Users and regex \\d+\n'
```

If the output shows one backslash where you wrote two, every literal double
backslash has to go through a file-writing tool instead.

### ⭐ The channel that cannot be reached into: base64

When a payload has to cross a shell at all, **base64 is the one encoding no
shell interprets.** It is `[A-Za-z0-9+/=]` and needs no quoting anywhere: not
in bash, not in PowerShell, not in `cmd`. A quote, a backtick, a dollar sign,
a percent, an emoji and an indented terminator all survive it unchanged.

That makes it the right transport for a helper that writes files:

| channel | when |
| --- | --- |
| ⭐ **base64 argument** | anything with quoting hazards. The bulletproof one. |
| **copy from another file** | the payload already exists on disk |
| **stdin** | ⚠ only behind a pipe, and only from a POSIX shell. See below. |

⛔ **PowerShell's stdin to a native command is NOT byte-exact, and Git Bash's
is.** Measured on one 59-byte fixture: piping it through PowerShell wrote **61**
bytes, because PowerShell's native-command pipe appends a trailing CRLF. The
tail `3e 20 3c 0a` arrived as `3c 0a 0d 0a`. The same file piped through Git
Bash was byte-identical, as were the base64 and copy-from-file paths from
**both** shells.

⚠ A receiving tool cannot tell an intended trailing newline from an added one,
so it must not guess. **From PowerShell, use base64 or copy-from-file. Reserve
stdin for pipes in a POSIX shell.**

⭐ Two properties worth building into any such helper, because both turn a
silent failure into a loud one:

- **Write atomically**: a temp file in the *same* directory, then rename. A
  killed process leaves the old file intact rather than a truncated one. Same
  directory matters: a rename across volumes is a copy and loses the guarantee.
- **Require an expected match count on a substitution.** A replace that matches
  a different number of times than you believed is refused, and the file is
  left untouched. ⛔ A silent no-op that reports success is the failure that
  discipline exists to remove, and it is the exact shape that bit this
  repository twice while it was being written.

---

## 2. An exit code is read from the process that produced it, unpiped

⛔ Piping a check into anything reports the **pipeline's** status, not the
check's, so a guard that failed reads as green.

```bash
node scripts/check-thing.mjs
```

```bash
pwsh -NoProfile -File scripts/check-thing.ps1
```

Not `check | grep`, not `check | Select-String`, not `check | tee`. Run it,
read `$?` or `$LASTEXITCODE`, then look at the output separately if you need to.

The same rule in PowerShell has a second edge: `-ErrorAction SilentlyContinue`
suppresses the error *output* while the cmdlet failure still sets a failing
status. To make a failure genuinely non-fatal, promote it and swallow it:

```powershell
try { Some-Cmdlet -ErrorAction Stop } catch { }
```

---

## 3. stdout and stderr are different streams and merging them is a decision

⛔ **Anything reading a value reads stdout alone and checks the exit code.**
Merging is correct only when the thing you want is on either stream.

The worked example, from this repository's own probe: `git rev-parse
--abbrev-ref HEAD` in a repository with no commits prints `HEAD` to stdout
**and** a three-line fatal to stderr, exiting 128. A version of the probe that
merged the streams put that fatal into a field called `branch`.

The opposite case is equally real: `java -version` prints the version to
**stderr**, so a probe reading stdout alone finds nothing. Merge on purpose
there, and say why in a comment.

---

## 4. A subshell discards the assignment you made in it

⛔ In POSIX shells, a function called inside `$( )` runs in a subshell. Any
variable it sets is gone when it returns.

```sh
collect() { FOUND="$FOUND $1"; printf 'value'; }   # FOUND is lost
x=$(collect a)                                      # ...because of this
```

Signal through the exit code or the output, and let the caller record it:

```sh
x=$(collect a); rc=$?
[ "$rc" = 3 ] && FOUND="$FOUND a"
```

The same shape appears with `while read` on the right of a pipe: the loop body
runs in a subshell and its assignments vanish. A here-document redirect does
not create one, which is why a lookup table is fed to the loop that way.

---

## 5. Line endings

A carriage return in a file `.gitattributes` says is LF is **invisible to git
and visible to everything else.** The index is normalised either way, so
`git diff` shows nothing and a review cannot see it.

It is not invisible to a regex reading the working tree. In .NET, `(?m)^...$`
matches before the newline and leaves the carriage return inside the capture,
so a status cell reads as `done` plus a byte and matches nothing.

⚠ The drift arrives from your own tooling. `Set-Content` writes CRLF on Windows
by default, and so do most editors and most file-writing tools.

Two things fix it and neither is a manual step anybody has to remember:

- a [`.gitattributes`](../../.gitattributes) that states the rule per type;
- a check that compares every tracked file's working-tree endings against what
  `.gitattributes` resolves for it, using git's own answer rather than a second
  table:

```bash
git ls-files --eol
```

⚠ **`.ps1` is the one file type that keeps CRLF.** Windows PowerShell 5.1
mis-parses a here-string whose terminator arrives with a bare LF. The simpler
defence, and the one this template uses, is to write no here-strings in a
`.ps1` at all.

---

## 6. A control byte goes in a file as an escape, never as itself

⛔ A literal control byte makes the file invisible to both review tools. `grep`
calls it binary and skips it, saying so in a line nobody reads, and `git diff`
prints "Binary files differ" so a code review of the file shows no diff at all.

The runtime value is identical either way, so only reviewability is ever at
stake, which is exactly why it survives so long unnoticed.

Write `\0`, `\t`, `\x1f`. Never the byte. Guard it with a check that fails
rather than warns, over every tracked text file.

---

## 7. Windows specifics

- ⭐ **Git Bash rewrites arguments that look like POSIX paths.** Anything with a
  leading slash is converted to a Windows path before the target process sees
  it. When the target is not a Windows program, the rewrite is corruption, it
  is silent, and the error never names the cause. Measured on 2026-08-26:

  ```bash
  gh api /repos/OWNER/NAME/actions/workflows
  ```

  ```text
  invalid API endpoint: "C:/Program Files/Git/repos/OWNER/NAME/actions/workflows".
  ```

  `gh` happens to detect it and say so. Almost nothing else does: a container
  runtime receives the rewritten path as a real argument and acts on it.

  Two variables turn it off, and they cover different things. `MSYS_NO_PATHCONV`
  disables the leading-path heuristic; `MSYS2_ARG_CONV_EXCL` is a per-argument
  exclusion list, and `'*'` excludes everything. ⛔ **Any command whose POSIX
  paths are destined for a non-Windows process carries both:**

  ```bash
  MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*' podman run --rm alpine ls /etc
  ```

  ⚠ This is the root cause behind the reserved-name bullet below, which is why
  the two are next to each other.
- ⛔ **The Windows reserved device names are `CON`, `PRN`, `AUX`, `NUL`, `COM1`
  to `COM9` and `LPT1` to `LPT9`**, in any case and with any extension. Two
  different triggers create one as a real file, and the second is the one nobody
  expects:

  1. **Your own redirect.** `2>/dev/null` under a shell that does not map
     `/dev/null` creates a file called `nul`, which git then tracks, which
     breaks `git stash` outright, and which cannot be deleted by `rm` or by
     Python.
  2. ⚠ **A tool's own argument list.** `podman machine ssh` on Windows passes
     `-o UserKnownHostsFile=NUL` to its own ssh invocation. Under Git Bash that
     is a filename, not the null device, so a 99-byte `NUL` holding an ssh host
     key appears in whatever directory the command ran in. Measured on
     2026-08-27 with `MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*'` already set:
     **the prefix above does not prevent this one**, because the argument never
     looked like a path.

  ⚠ The two differ in recoverability, so do not assume the worse case is the
  only case. The `NUL` written by trigger 2 was removed by `rm` on the same
  machine; the lowercase `nul` from trigger 1 was not. Put the whole reserved
  set in `.gitignore` before any of it happens, because the directory it lands
  in is usually a repository.
- ⚠ **`/tmp` is not one directory.** Git Bash resolves it inside the msys root;
  a native Windows Python or PowerShell resolves it somewhere else entirely, or
  not at all. A file written by one and read by the other is not found. Use a
  repository-relative scratch directory, or an absolute path both agree on.
  This document's author hit it while testing the probe.
- ⚠ **A shim is not an executable.** On Windows the node ecosystem ships shims,
  and scoop's are `.ps1`. `Process.Start` with `UseShellExecute` false throws
  "not a valid application for this OS platform" on a `.ps1` and refuses a
  `.cmd`. Route a `.ps1` to a PowerShell host and a `.cmd` to `cmd.exe`.
- ⚠ **`wsl.exe` writes UTF-16LE**, which a redirected stdout reads as empty or
  as mojibake. `WSL_UTF8=1` fixes it.
- ⛔ **A payload handed to `wsl.exe -- /bin/sh -lc` does NOT keep its quoting,
  and the caller cannot fix it by quoting harder.** Measured on 2026-08-27
  against real Alpine and Debian distros, under **both** PowerShell hosts, with
  every hazard **already correctly single-quoted for `sh`** before it was
  passed:

  | POSIX-quoted for sh | PowerShell 7.6.5 | Windows PowerShell 5.1 |
  | --- | --- | --- |
  | `$VAR` | ⛔ expanded in transit, and the **result** is then re-parsed | ⛔ the same |
  | a backtick | ⛔ opens a command substitution | ⛔ the same |
  | a double quote | arrives | ⛔ `syntax error: unterminated quoted string` |
  | a bracket, a single quote, a tab, a space | arrives | arrives |

  ⭐ **The mechanism is expand-then-re-parse**, which is not what "as though it
  were double-quoted" would predict: a double-quoted expansion does not re-parse
  its result. `printf %s a$HOME` answers `a/root` and `printf %s a$PATH` dies
  with ``syntax error: unexpected "("``, because WSL appends
  `/mnt/c/Program Files (x86)/...`. ⛔ **So the hazard is not the `$`. It is
  whatever the value happens to contain**, which is why no alphabet a caller
  keeps to is safe.

  ⭐ **The fix is section 1's fix**, reached from a different direction: send the
  payload as base64 and decode it in the guest. A worked implementation is
  `ConvertTo-DistroScriptCommand` in
  [`../../scripts/windows/wsl-toolkit/wsl-toolkit.ps1`](../../scripts/windows/wsl-toolkit/wsl-toolkit.ps1),
  which also **asserts** that the skeleton it builds stays inside the measured
  alphabet, because a payload hand-written inside a safe alphabet is a
  constraint nothing enforces.

  ⚠ **Create the file, open it, unlink it, and only then decode into it.**
  Writing it first and unlinking after reads the same in a diff and is not: a
  redirect creates the file before the decode runs, so a guest with no `base64`
  is left holding an empty one that nothing removes.
- ⛔ **`wsl.exe` is one of the commands the path-conversion rule above applies
  to**, which is not obvious because `wsl.exe` is itself a Windows program.
  From Git Bash, `wsl -d D -- /bin/sh -lc ...` has `/bin/sh` rewritten to
  `C:/Program Files/Git/bin/sh`, and the distro reports as unstartable on a
  machine where it is running fine.
- ⚠ **Windows PowerShell 5.1 drops a double quote when it builds a CHILD
  PROCESS's argument list**, one layer above `wsl.exe`. A `-Command` value of
  ``a'b"c`d$e`` reaches a script spawned as `powershell -File s.ps1 -Command ...`
  as ``a'bc`d$e``. In-process it arrives intact, and PowerShell 7.6.5 is fine
  either way. ⛔ Nothing the spawned script does can recover it, so a scripted
  5.1 caller passes base64 rather than text.
- ⚠ **A machine-wide install is not under the user's home.** Checking only
  `~/scoop` reports a tool as absent on a machine that has it under
  `C:\ProgramData\scoop`. Look in both.
- ⚠ **A release binary left running holds its own executable open**, and the
  next build fails on a locked file with an error naming neither. Kill stray
  processes before rebuilding.
- ⛔ **Python on Windows cannot print this repository's own markers.** stdout
  defaults to cp1252, which has no ⛔, no ⭐ and no ⚠. Measured on 2026-08-27,
  Python 3.13.15:

  ```bash
  python -c "print('⛔')"
  ```

  ```text
  UnicodeEncodeError: 'charmap' codec can't encode character '⛔'
  in position 0: character maps to undefined
  ```

  ⭐ Note what the error itself does: it names the character as a codepoint,
  because it cannot print it either.

  ⚠ The failure is at print time, so it passes every test that captures output
  and fails the moment a person runs it at a console. Any script that echoes a
  marker sets `PYTHONIOENCODING=utf-8` or calls
  `sys.stdout.reconfigure(encoding='utf-8')` before printing. Where the encoding
  is not yours to control, print the codepoint instead of the character.
- ⛔ **A byte class is not a character class, and the wrong one is silently
  wrong.** `grep -o '[^\x00-\x7F]'` returns per-byte fragments, so a three-byte
  marker counts as three separate entries and the total is wrong in a way that
  looks like real output. Measured on 2026-08-27 over a file holding exactly one
  ⛔ and one ⚠:

  | tool | answer |
  | --- | --- |
  | `grep -o '[^\x00-\x7F]'` | 6 fragments, none of them a character |
  | `rg -o '[^\x00-\x7F]'` | `1 ⚠`, `1 ⛔` |

  ⚠ **Setting `LC_ALL=C` does not rescue the first row**, and assuming it does
  is the trap. On the measured machine `LANG` was already empty, so `LC_ALL=C`
  changed nothing at all. The fix is choosing the right tool, not the locale.

  ⛔ **A check states which of the two jobs it is doing**, because the same
  expression is correct for one and quietly wrong for the other. Counting bytes
  is byte-oriented and belongs to a byte tool. Counting characters is
  character-oriented and needs a Unicode-aware one. This matters most to
  [`check-control-bytes.sh`](../../scripts/common/check-control-bytes.sh),
  whose whole subject is bytes that review tools misreport.

---

## 8. PowerShell specifics

- ⛔ **`[int]` on a double rounds.** `[int](2.65)` is 3, so a 2h39m session
  prints as 3h39m and the number goes straight into a report. Use
  `[math]::Floor`.
- ⛔ **`-match` is case-insensitive**, so `'FAILED'` matches `"0 failed"` in a
  summary line and a failing test's name is lost exactly when it is needed.
  Use `-cmatch` when case is the signal, and filter on the per-test line rather
  than the summary.
- ⛔ **`$args` inside a function is an automatic variable** and silently
  swallows a parameter of that name. Variable names are case-insensitive, so
  `$Args` collides too. Name locals so they cannot.
- ⚠ **`$PSNativeCommandUseErrorActionPreference` defaults to false** from pwsh
  7.4, so a native command writing to stderr does not terminate under
  `$ErrorActionPreference = 'Stop'`.
- ⚠ **`Get-Command` finds cmdlets, functions and aliases too.** Filter to
  `Application` and `ExternalScript` when you mean an executable. A cmdlet
  looked for on PATH reports as missing on every machine that has it.
- ⚠ **Read the child's streams before waiting on it.** Calling `WaitForExit`
  first deadlocks any child that fills the pipe buffer: the child blocks on
  write, the parent blocks on the wait, and neither moves until the timeout.
- ⚠ **`ConvertTo-Json` defaults to depth 2**, and renders anything deeper as
  the literal text `System.Collections.Hashtable`. Pass `-Depth`.
- ⛔ **A `.ps1` containing any non-ASCII byte needs a UTF-8 BOM if Windows
  PowerShell 5.1 has to run it.** 5.1 decodes a BOM-less file as the system ANSI
  code page, so every non-ASCII character is mis-decoded. PowerShell 7 defaults
  to UTF-8 and does not care, which is exactly why this is easy to miss: the
  file works on the machine it was written on and breaks on the one it was
  written for. `PSUseBOMForUnicodeEncodedFile` is the analyzer rule, and it
  caught this repository's own probe.
  ⚠ The alternative is to keep every `.ps1` ASCII-only. That is also defensible;
  what is not defensible is non-ASCII with no BOM and a claim of 5.1 support.
- ⛔ **PowerShell's `2>` on a native command is NOT byte-faithful, and it will
  make you diagnose a defect that is not there.** It captures the child's stderr
  as error records and re-renders them, so escape sequences are dropped.
  Measured on 2026-08-30, running one script that writes an ANSI-coloured line
  to each stream:

  | how stderr was captured | escape bytes on stdout | escape bytes on stderr |
  | --- | --- | --- |
  | `pwsh -File s.ps1 > out 2> err` | 3 | ⛔ **0** |
  | `cmd /c "pwsh -File s.ps1 > out 2> err"` | 3 | 4 |

  ⚠ **The first row reads exactly like a bug in the program**: stdout is
  coloured, stderr is not, and the code that writes them is one function. An
  hour went into the wrong file before the same line was captured a second way.
  ⭐ To check what a child actually put on stderr, redirect with `cmd /c`, or
  have the child write the file itself. Section 3's rule about the two streams
  being different is about which one to READ; this is about the capture
  changing what is there.
- ⛔ **A `.ps1` run through `-File` CANNOT be handed an array, and the failure
  modes differ by type.** Measured on 2026-08-30 under PowerShell 7.6.5 and
  Windows PowerShell 5.1, both identical, against a script declaring
  `[int[]]$Ints` and `[string[]]$Strs`:

  | what the caller types | what binds |
  | --- | --- |
  | `-Strs a,b` | one value, the literal string `a,b` |
  | `-Ints 5,9` | ⛔ **one value, `59`** |
  | `-Strs a -Strs b` | ⛔ refused: `parameter 'Strs' is specified more than once` |
  | `-Ints 5 9` | ⛔ refused: `a positional parameter cannot be found` |
  | in-process `& .\s.ps1 -Ints @(5,9)` | two values, correctly |

  ⭐ **Row two is the dangerous one and it is not obvious.** Every argument
  arrives as a string, and PowerShell converts a string to an int with the
  CURRENT CULTURE's number style, where a comma is the thousands separator. So
  the parameter binds a plausible number that is not the one anybody typed, and
  nothing warns.

  ⚠ **A wrapper does not rescue it.** `wsl-toolkit`'s launcher splats the same
  argument list into the inner script, and the inner binding is identical.

  ⭐ **So a list parameter takes `[string[]]` and splits its own value**, which
  makes a bad element a refusal. ⛔ Where the values are arbitrary text there is
  no safe delimiter either, and the channel is a FILE: a URL query string carries
  commas, and splitting on one corrupts the value it was passing.
- ⚠ **An empty `catch {}` is refused by PSScriptAnalyzer**, and it should be:
  it is indistinguishable from an accidentally swallowed error. Where swallowing
  is genuinely the design, say so in code rather than by omission. `$null = $_`
  discards the error explicitly and reads as a decision.

---

## 9. A long-running command needs a hard time limit

⛔ Several tools block for as long as you let them. `kubectl version` without
`--client` contacts a cluster, `gradle --version` starts a daemon, a cloud CLI
sits on an update check. A script that shells out to unknown tools without a
limit is a script that hangs, and a script that hangs is one nobody runs twice.

```bash
timeout 6 some-tool --version
```

`timeout` is absent on stock macOS. `gtimeout` is there when coreutils is
installed, and the fallback is to run in the background and kill on a counter.
Exit 124 is the coreutils verdict and 137 is a kill; **both mean "it never
answered", which is a different fact from "it is not installed"** and belongs in
a different field.

---

## 10. Waiting inside an agent session

⚠ **Do not end a turn to wait for something.** The conversation idles, the
harness times out, and the session dies mid-operation with state half-changed.

Hold in the foreground with a loop that ticks, and keep the tick under four
minutes so progress is visible and nothing looks hung:

```bash
i=0
while [ $i -lt 8 ]; do
  sleep 45; i=$((i + 1))
  printf 'tick %s  %s  %s\n' "$i" "$(date -u +%H:%M:%SZ)" "$(some_cheap_check)"
  if some_done_condition; then printf 'complete\n'; break; fi
done
```

A background job and a foreground hold loop are not alternatives. Use both: the
job's notification tells you the instant it finished, and the hold loop is what
keeps the session alive long enough to receive it.

⚠ The ceiling is per tick, not per wait. A forty-five minute operation is ten
holds that each print progress, never one forty-five minute sleep.
