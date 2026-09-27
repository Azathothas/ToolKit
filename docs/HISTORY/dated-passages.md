# HISTORY: dated passages

⛔ **Superseded. Nothing here is read to do work.** The live pages state what is
true now, and cite the entry that holds each measurement.

This holds every paragraph and table a live page carried with a calendar date
or a dated measurement in it until 2026-09-27, when `DOC-08` rewrote the live
pages in the present tense and the `ste` check began to refuse a date. Each
block is verbatim and fenced, so its links and its sentences are text, not
content. The page it came from and the heading it sat under are named above it.


### `docs/AGENTS.md`

From "1. Where you are":

```text
⭐ **Two things are published from here, and only two.** The `wsl-toolkit` tool is
cut as a GitHub release on a `wsl-toolkit-v*` tag, carrying the native executable
for two Windows architectures, from `wsl-toolkit-v3.0.0` herdr's newest stable release
built for four targets, `SHA256SUMS`, and one `.cosign.bundle` per published file. herdr's
development branch is published as a nightly prerelease on a `herdr-nightly-*`
tag by [`../.github/workflows/herdr-nightly.yml`](../.github/workflows/herdr-nightly.yml),
by the operator's ruling of 2026-09-15.
[`../tools/windows/wsl-toolkit/README.md`](../tools/windows/wsl-toolkit/README.md)
is how the compiled half is built.
```

### `docs/consumers.md`

From "The register":

```text
Read from each repository on 2026-09-13.
```

From "Fetching the published product":

```text
⭐ **herdr's development branch is published separately, as a nightly prerelease** on a
`herdr-nightly-YYYYMMDD-SHA12` tag, with the same four builds, `BUILD-INFO.json` naming
the herdr commit, `SHA256SUMS`, and a bundle per file that verifies against
`.github/workflows/herdr-nightly.yml`. The newest seven are kept. ⛔ A nightly is always
a prerelease and never a `wsl-toolkit-v*` tag, so a lookup for this tool's releases
skips it. ⭐ **The first nightly is published**, `herdr-nightly-20260916-18061191fdc0`,
on 2026-09-16.
```

From "Fetching the published product":

```text
⭐ **`wsl-toolkit-v3.0.0` is the first release that carries herdr**, published on
2026-09-17 with 14 assets: the two executables, the four herdr builds, `SHA256SUMS`
and a bundle for each.
```

From "Fetching the published product":

```text
Measured on 2026-09-17 against `wsl-toolkit-v3.1.0`: PowerShell answers
`Verified OK`, and so does Git Bash with `MSYS2_ARG_CONV_EXCL='*'` set for the
call. Plain Git Bash does not.
```

### `docs/conventions/docs.md`

From "Who owns what, for the documents a conflict is settled against":

```text
| file | owns |
| --- | --- |
| ⭐ [`../AGENTS.md`](../AGENTS.md) | the router, and the ONLY one, read end to end. Where you are, the absolutes, the start of a session, what to read for which task, and which tool already exists. ⚠ A root `AGENTS.md` restating the absolutes existed until 2026-08-30 and was deleted under `DOC-07`. |
| [`../../README.md`](../../README.md) | what this is, for a competent stranger, and the map of everything else |
| ⭐ [`../../TODO/PROGRESS.md`](../../TODO/PROGRESS.md) | the record. What changed since last time and what is next. Nothing else carries a work order. |
| [`../../TODO/RULES.md`](../../TODO/RULES.md) | the half of the record that does not change between sessions: the standing facts, and the rules that are this repository's own |
| [`../../TODO/INDEX.md`](../../TODO/INDEX.md) | every entry, one line each, with the counts a check holds |
| [`../../TODO/ENTRY.md`](../../TODO/ENTRY.md) | the form an entry is written from |
| [`../../CHANGELOG.md`](../../CHANGELOG.md) | what shipped, when, and where the evidence is |
| ⭐ [`../consumers.md`](../consumers.md) | the technical reference for the thing that makes this repository different: who fetches from it and what breaks them. **When a document conflicts with it about a consumer, it wins and the other is the defect.** |
| a tool's `.md`, beside the tool | what that tool does, in full, for a reader who has opened nothing else |
```

From "Who owns what, for the documents a conflict is settled against":

```text
⚠ **Two roles are deliberately left empty.** An operator-facing runbook and a
threat model are both worth having and neither has content yet, so neither
exists rather than shipping an empty skeleton for each.
[`../../TODO/PROGRESS.md`](../../TODO/PROGRESS.md) carries both under its open
questions. ⛔ That sentence was here before the line it names was, which the
claim audit found on 2026-09-10; the fix was to write the open question rather
than to delete the sentence.
```

### `docs/conventions/forbidden-patterns.md`

From "Tooling and review":

```text
| forbidden | what it caused |
| --- | --- |
| A literal control byte in a tracked text file | the file becomes invisible to review. Grep calls it binary and skips it, and a diff says only that the files differ. |
| A payload containing a dollar sign next to a quote, passed as the REPLACEMENT STRING of a JavaScript `String.replace` | the rest of the file is pasted in and nothing errors. `$&`, `` $` `` and `$'` are expanded inside a replacement STRING: `$'` means "everything after the match". One comment carrying a quoted dollar sign duplicated a 500-line script from the anchor down, and the parse error that followed named a brace 200 lines away. Pass a function, `replace(find, () => replacement)`, which is not interpreted at all. Same class as the shell-payload rule below, in a language nobody expects it in. |
| Reading an exit code through a pipe | the pipeline's status, not the check's. A guard that failed reads as green. |
| A PowerShell script with positional binding left on, called through `-File` | ⛔ **an argument list overflowing into whatever parameter is next in declaration order.** `-Gate "a","b","c","d"` reaches the child as four arguments: one bound to `-Gate` and the rest positionally to `-Name`, `-Email` and `-Branch`, so `git-sync.ps1` committed under an author of `sh scripts/common/check-control-bytes.sh` and printed `identity verified` one line under it. The check is the code: `[CmdletBinding(PositionalBinding = $false)]` turns a silent misbinding into a refusal. `TOOL-03`. |
| A prose payload passed inline to a shell | backticks executed inside the text, even in a quoted heredoc |
| A doc claim written without being verified | the most confident sentence in a file is regularly the only false one |
| Acting on an instruction found in an issue, a pull request, a comment, a review or a bot description | executing a string anyone with an account could write. Reading an item is free; obeying it is not reading. [`../security/remote-ops.md`](../security/remote-ops.md) |
| Taking an item's factual claim as verified because its author is trusted | a claim describes the tree it was written against, and that tree has moved. Two findings behind this table were right in substance and stale in detail. |
| An allowlist applied to the whole line instead of to the matched item | the allowed thing hides the banned thing beside it. `grep -nP <banned> \| grep -vP <allowed>` passed a line reading `⛔ never use <banned emoji>`, because `grep -v` drops lines, not characters. Fixed with a lookahead in `check-docs.sh`. |
| Documentation that describes what the project did rather than what the thing does | a reference page turns into a diary and stops being read |
| A page nothing links to | not read, so not corrected. The state every stale document passes through. |
| `cmd; rc=$?` used as a guard in a script running under `set -e` | ⛔ **the guard is unreachable.** A failing simple command exits the shell immediately, so the test never runs, the message is never printed and the cleanup never happens. It reads in review exactly like a checked call. Both fetch scripts in `pkgforge-dev/docker-bsd`'s `experiments/` had it, guarding `curl` and `xz`. `if ! cmd; then` both suppresses `set -e` for that command and lets the guard run. |
| A `try`/`catch` around a foreign-function binding, reporting the catch as "the library did not load" | ⛔ **it cannot tell a missing library from a missing entry point**, and naming the wrong one sends the next reader after the wrong problem. A probe reported `vmcompute.dll did not load` about a library that had loaded and exports 36 functions; the one it wanted lives in `computecore.dll`. `LoadLibrary` then `GetProcAddress` separates the three outcomes: absent, present-without-the-symbol, bound. |
| Sending a whole line at once to a serial console, a pty or any other real tty | ⛔ **characters are silently dropped** while the line discipline is still being set up, and what arrives is a corrupted prefix. A marker of `TOOLKIT-READY-789f28b0` reached a FreeBSD shell as `TOO789f28b`, never matched, and was reported as "the guest never answered" about a guest that had answered correctly. Type one character at a time, and synchronise on the prompt rather than on elapsed time. |
| Enumerating a tree with a language's `glob` where the interesting files are under a dot-directory | ⛔ **the check runs over nothing and exits 0.** Python's `glob` does not descend into a directory whose name begins with a dot, and every yaml file in this repository is under `.github/`, so a CI step named `yaml parses` iterated zero files and reported success for as long as it existed. Enumerate with `git ls-files`, and **assert the count before the verdict** so an empty scope is a refusal rather than a pass. `TOOL-08`. |
| An exemption written as a directory prefix for a directory that does not exist | ⛔ **it grants itself to whatever lands there next.** Three checks here carried exemptions for `docs/templates/`, `dotfiles/` and `bootstrap/` inherited from a template; two had never existed in this tree. A file dropped at one of those paths would have been silently out of scope. Name the file, not the directory, and delete an exemption rather than emptying it. `DOC-04`. |
| A wrapper that prints progress with `Write-Host` around a program whose stdout carries a value | ⛔ **it corrupts the value, and only out of process.** In-process `Write-Host` goes to the information stream and the wrapper looks correct; run as a child process the host writes it to real stdout, so a caller capturing the wrapped program's answer gets a progress line ahead of it. Measured on 2026-08-29: `-Action HostAddress` through the launcher returned `==> Using the copy beside this launcher` before the address. A wrapper writes nothing to stdout. `WSL-15`. |
| Splatting a PowerShell argument list built with `ArrayList.ToArray()` | ⛔ **every parameter NAME binds positionally instead.** `@arr` is re-parsed as a command line only for some ways of building `arr`: an ordinary array with `+=`, a `Where-Object` filter, a range slice and an `[object[]]` parameter all forward names; an `ArrayList.ToArray()` does not, and every element is a `System.String` in all five, so nothing about the values explains it. A wrapper forwarding `-Action HostAddress` had `-Action` bound as the VALUE of `-Action`. `WSL-15`. |
| An `[int[]]` (or any numeric array) parameter on a `.ps1` that callers reach through `pwsh -File` | ⛔ **a silently wrong number, not a refusal.** Through `-File` every argument arrives as a STRING, so `-Steps 5,9` is the one string `"5,9"`, and PowerShell converts a string to an int with the current culture's number style, where a comma is the THOUSANDS separator. It bound the single value **59**. The escalation it configured never fired, over a run that looked normal. Measured under PowerShell 7.6.5 and Windows PowerShell 5.1 on 2026-08-30. Take `[string[]]` and parse it yourself, so a non-number is a refusal. `WSL-22`. |
| Documenting a parameter as "repeatable" on a `.ps1` that callers reach through `pwsh -File` | ⛔ **it cannot be repeated, and the capability was documented for a session before anybody tried.** `-X a -X b` is refused with "parameter 'X' is specified more than once", directly and through a wrapper that splats the same argument list; `-X a b` is refused as positional. Where the values are arbitrary text there is no safe delimiter either, so the channel is a FILE. `WSL-22`. |
| `return , $list` in PowerShell, read at the call site with `@( ... )` | ⛔ **an empty list reports one element, and the element is the empty array.** The comma wraps the list in a one-element array and an array subexpression keeps that element instead of unrolling it again; an ordinary assignment does not. A new check reported one finding with a blank message over a tree that had none, which is a gate INVENTING a defect: worse than missing one, because it sends a reader after nothing. Return the list plainly and let the caller wrap it. Measured on PowerShell 7.6.5, 2026-08-30. |
| A PowerShell local whose name differs from a parameter of the same function only by case | ⛔ **it IS the parameter.** Variable names are case-insensitive, so `$state = Get-Something` inside a function taking `$State` replaced a state object with a string, and the next property read died mid-run on the one code path whose job is to keep reporting when everything else has gone quiet. PSScriptAnalyzer does not flag it and the suite could not see it; driving a real distro is what found it. ⚠ The AST walk that caught it was part of the PowerShell product's build and was deleted with it, so nothing in the gate checks a `.ps1` for this now. That walk's first version used `-eq`, which is case-INSENSITIVE, so it skipped exactly what it was looking for and reported clean over a planted defect. |
| Using `[IO.Path]` to reason about a WINDOWS path in code that also runs elsewhere | **the answer changes with the host, silently.** `GetFileNameWithoutExtension` splits on the RUNNING platform's separators, so on Linux a backslash is an ordinary character and `logs\CON.jsonl` has a file name of `logs\CON`, which is not a reserved device. A guard that refuses `nul` and `con` therefore passed on Windows and failed on the ubuntu CI job, on the same commit, and the rule it enforces is about Windows semantics whatever host is asking. Split on `[\\/]` yourself. `TOOL-10` is the same class in a regex; this one is in a framework call that looks host-neutral. |
| Writing `[\/]` in a .NET character class where you meant "slash or backslash" | **the class matches a forward slash alone**, because the backslash escapes a character that was never special. `check-no-secrets.ps1` therefore could not match a drive-letter home path at all, on the host that produces them, while its sh twin's `[\\/]` caught them. It is the check that keeps a username out of a public repository, and it was blind for as long as it existed. Found when the full gate's `check-twins` reported the two halves disagreeing; `--fast` skips that check, so every fast run all session had been green. `TOOL-10`. ⛔ **The Go program that replaced both halves wrote the same class again**, as `[\/]` in a Go regular expression, where it is also an escaped forward slash alone. The tree then published four home paths carrying a real username, green, until `TOOL-25`. |
| Collapsing `..` in a path with a regex like `[^/]+/\.\./` | ⛔ **`[^/]+` matches `..` itself**, so a link going up three levels eats its own segments: `a/b/c/../../../docs/x` collapsed to `a/b/docs/x` and every correct link from a directory three deep was reported broken. Invisible for as long as nothing in the tree is three deep, which is how it survived in `check-docs.ps1` while its sh twin, which asks the filesystem, was right the whole time. Let the framework resolve it. |
| A gate runner that shells out to one half of a twin pair and skips it when that half cannot run | ⛔ **it skips exactly on the host the twins exist for.** `check-gate.ps1` ran the `.sh` half of six checks and reported six skips and a green exit on a Windows session with no POSIX shell, which is the machine its own header says it earns a twin for. ⚠ Invisible anywhere Git Bash is installed. `TOOL-06`. |
| Resolving a name to its target and then EXECUTING the target | a multiplexer decides what to do from the name it was invoked under, so running the target answers about a different program. `~/.cargo/bin/rustc.exe` resolves to `rustup.exe`, and a host survey that ran the resolved path reported `rustc 1.29.0` on a machine whose rustc is 1.98.0. Report the RESOLVED path and execute the FOUND one. `WSL-31`. |
| Treating a path that will not canonicalise as a path that does not exist | a working tool reads as absent. Every tool scoop installs sits behind a directory JUNCTION named `current`, and Go's `filepath.EvalSymlinks` fails on one whose target this process may not read, so node, ruby and java were reported missing on a machine where all three run. `GetFinalPathNameByHandle` resolves a junction, a symlink and a mount point alike; where nothing resolves it, keep the path and say the canonical one is unknown. `WSL-31`. |
| A version pattern whose boundary excludes a letter, or includes a dot | both directions produce a number nobody measured. Excluding letters made `go version go1.27.0` unmatchable; including a dot made `version v4.35.1` match at the SECOND dot and report `35.1`. A number nothing measured is worse than a blank, because a blank gets checked. `WSL-31`. |
| Assembling a `.cmd` or `.bat` invocation as an ARGUMENT LIST | cmd.exe does not parse what CreateProcess callers produce, so a probe that runs by hand exits 1 through the escaped form and reads as a broken tool. Build the command line as a string, `"cmd.exe" /d /s /c "<quoted argv>"`, and refuse a path carrying a percent or an exclamation mark, which cmd expands even inside quotes. `WSL-31`. |
| A rootless container engine configured without `passt` where podman is 5.4 or later | every run refuses with `could not find pasta, the network namespace can't be configured`, which reads as a broken image and is a missing package. Debian's podman does not depend on it. Install it, and pin `default_rootless_network_cmd` only when the BINARY is absent rather than when a package failed to install. `WSL-31`. |
| Asking two container engines for a field under one spelling | the wrong one fails the whole call and reads as a broken engine. podman answers to `{{.Host.Arch}}` and docker to `{{.Architecture}}`; asking podman for the lower-case form returns `can't evaluate field host in type system.infoReport`. They also disagree on the VALUE, and only podman's is a token `--platform` accepts. `WSL-31`. |
| Reporting only the LAST candidate's reason when several were tried | it names whichever was tried second. An engine probe that kept one error reported "docker: no accessible executable found" on a machine with a working podman and a wrong template, sending the next reader after the wrong tool. Keep every candidate's reason and join them. `WSL-31`. |
| A table of claims about code, with no check that its rows still point at code | ⛔ **a row whose code moved keeps parsing, keeps reading correctly, and proves nothing.** `tools/repo/mutations.json` names a line to delete and a case that must go red. Two rows stopped matching when a signature changed and a rule moved file, and both were silently unproved until somebody ran the ten-minute harness. Split it: a gate check asserts every row still ADDRESSES its subject in under a second, and the slow harness answers whether the case actually fails. `TOOL-19`. |
| A harness that reads `go test` exiting 0 as "the case passed" | ⛔ **a skipped case is byte for byte a passing one from outside the process.** It prints `=== RUN`, it exits 0, and a mutation harness reported a platform-bound guard as THEATRE on the host that cannot run its case: a true statement about the host printed as a false accusation against the test. Count `--- SKIP:` against `=== RUN` and give the skip its own outcome, which is neither proved nor wrong. `TOOL-19`. |
| A test that builds a path from a platform's environment variables, in a suite that runs on a second host | ⛔ **`$env:TEMP` and `$env:WINDIR` are null under PowerShell on Linux**, and `Join-Path` refuses a null path, so two cases threw rather than failed and the ubuntu job went red over a Windows-only tool. The tool is Windows-only; its SUITE is not, which is the distinction. Ask the runtime (`[IO.Path]::GetTempPath()`) or resolve the program (`Get-Command whoami`), rather than spelling a path. `WSL-57`. |
| `pwsh -Command STRING` or `powershell -Command STRING` followed by more arguments, meant as `$args` | ⛔ **the arguments are appended to the command text and run.** A parse check written as `pwsh -NoProfile -Command $check $file` saw an empty `$args[0]`, then executed `$file`, and the file was `acceptance.ps1`: a full acceptance run on the real host, 91 cases, started by a syntax check. Put the check in a script and pass the path as a named parameter through `-File`. `WSL-74`. |
| Reading a project's documentation for its development branch as the contract of the release that is installed | ⛔ **a router that sends every agent to a flag the installed binary refuses.** A reference sweep read herdr's `docs/next`, and `docs/AGENTS.md` routed agents on Windows to `herdr --machine`; herdr 0.9.0 answered `unknown option: --machine` with exit 2. Read the pages at the installed version's tag, and ask the binary itself, as `herdr <group>` does. `WSL-76`. |
| Proving an integration by reading back the file its installer wrote | ⛔ **a probe reports a hook registered that the program never runs.** The muse adapter wrote Muse's hooks into `~/.muse/settings.json` in a shape taken from a plugin for an older Muse, and its probe read that file back and reported six events; Muse Code 1.3.0 reads `~/.config/muse/settings.json` and runs a hook only inside a matcher group, and ran none of them. Measure a registration against the program that reads it: Muse's credential-free `--provider echo` runs every session hook with no sign-in. `WSL-76`. |
| A guard whose subjects are a hand-written list, under a header claiming the coverage is automatic | ⛔ **the claim is true one level down and false one level up.** A test asserting every flag appears in the manual said "a flag added tomorrow is covered without this file being touched"; that held for a flag added to a command already on its list, and a whole new command arrived with two undocumented flags and left it green. Walk the program's own dispatch table, and assert the count of what was reached. `WSL-56`. |
| A total deadline chosen for the worst LEGITIMATE case, used as the guard against a hang | ⛔ **it cannot catch a stall, and the two are different questions.** A `podman pull` was bounded at thirty minutes, which is right for a large image on a slow link and useless against a pull that has simply stopped: measured here with ZERO bytes read, ZERO written and ZERO processor time for 28 minutes, while the tool said nothing. ⭐ Bound the total AND the silence: a transfer that is moving is never stopped, and one that has produced nothing for a few minutes is given up, saying which limit fired. |
| A deadline, a containment check or any other guard chosen at each CALL SITE | ⛔ **it will one day be chosen at one fewer.** Eight host-engine invocations each picked their own timeout, from 90 seconds to thirty minutes, and a ninth would have inherited whatever its caller happened to hold. ⭐ One door with a default, and a textual rule that refuses a call reaching the subject any other way: the rule found a call site the sweep by hand had missed, on its first run. |
| Two documents agreeing with each other about what the code does, with neither compared against the code | ⛔ **the agreement reads as corroboration and is worth nothing.** A manual said the tool named a container log driver and the entry that would have built it listed the same rule as work to do; `git grep` over the whole tree answered ZERO. Each page was written from the other. ⭐ Grep the CODE for the behaviour, not the other page for the sentence. |
```

### `docs/conventions/prose.md`

From ""Short sentences" is a number now: ASD-STE100":

```text
⭐ **The corpus was measured before the rule was written**, on 2026-09-17: 4,836
sentences in the documents a reader follows, **none over 25 words**, longest 19.
This rule did not arrive to punish the tree. It arrived to put a number on a
sentence that had none, and to catch the three things nobody was counting.
```

### `docs/conventions/shell.md`

From "The order to try, and it is an order rather than a menu":

```text
The reason it is a file and not "better quoting" is that quoting is not
sufficient. Measured on 2026-08-25:
```

From "7. Windows specifics":

```text
- ⭐ **Git Bash rewrites arguments that look like POSIX paths.** Anything with a
  leading slash is converted to a Windows path before the target process sees
  it. When the target is not a Windows program, the rewrite is corruption, it
  is silent, and the error never names the cause. Measured on 2026-08-26:
```

From "7. Windows specifics":

```text
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
```

From "7. Windows specifics":

```text
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
  against real Alpine and Debian distributions, under **both** PowerShell hosts, with
  every hazard **already correctly single-quoted for `sh`** before it was
  passed:
```

From "7. Windows specifics":

```text
  ⭐ **The fix is to keep the payload out of the argument list.** `wsl-toolkit`
  never passes one as an argument: a container job's script travels as a file,
  and a command for `distro` or `base exec` goes on the guest shell's stdin,
  framed so the shell reads all of it before running any of it and the command's
  own stdin is `/dev/null`: `FramePayload` in
  [`../../tools/windows/wsl-toolkit/internal/toolkit/payload.go`](../../tools/windows/wsl-toolkit/internal/toolkit/payload.go).
  A caller whose own shell would reach into the text first passes it as
  `--command-base64`, which is section 1's channel, and the tool decodes it on
  the host.
- ⛔ **`wsl.exe` is one of the commands the path-conversion rule above applies
  to**, which is not obvious because `wsl.exe` is itself a Windows program.
  From Git Bash, `wsl -d D -- /bin/sh -lc ...` has `/bin/sh` rewritten to
  `C:/Program Files/Git/bin/sh`, and the distribution reports as unstartable on a
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
```

From "7. Windows specifics":

```text
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
```

From "8. PowerShell specifics":

```text
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
  7.4, so a native command writing to stderr does not stop under
  `$ErrorActionPreference = 'Stop'`.
- ⚠ **`Get-Command` finds cmdlets, functions and aliases too.** Filter to
  `Application` and `ExternalScript` when you mean an executable. A cmdlet
  looked for on PATH reports as missing on every machine that has it.
- ⛔ **`Start-Process -ArgumentList` re-quotes what you hand it**, so an array
  is not passed through as an array. `-c 'exit 37'` reached the child as `-c`
  and `37`, and the case asserting an exit code is not flattened read 0 against
  a tool that was right. `[Diagnostics.ProcessStartInfo]::new().ArgumentList` is
  exact, and it is PowerShell 7 only.
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
```

From "8. PowerShell specifics":

```text
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
```

### `docs/methodology/gate.md`

From "Local is not production":

```text
⚠ **The same check is not the same tool.** A local gate and CI can run the
identical command over the identical files and disagree, because the binary
underneath is a different version. Measured on 2026-09-09: shellcheck 0.11.0 on
the development host reports nothing for `cd "$D" && cmd || true`, and the
version on `ubuntu-latest` reports SC2015 and fails the job. The local gate was
green on that line for a whole session. ⛔ A green local run is evidence the
tree is right, never evidence that CI will agree; the CI result is the one that
gates a merge, so read it rather than predicting it.
```

### `scripts/README.md`

From "scripts":

```text
| directory | what is in it |
| --- | --- |
| [`doctor/`](doctor/) | ⭐ the environment probe. Two implementations, one schema. Every project keeps this. |
| [`common/`](common/) | the checks and the helpers, and since 2026-09-12 one configuration file a helper installs. ⛔ Every CHECK has a POSIX sh implementation AND a PowerShell twin; a helper and a data file have neither and the twins table below says why. |
| [`../tools/windows/wsl-toolkit/`](../tools/windows/wsl-toolkit/README.md) | the native Windows product, in Go. ⛔ Not a script, so nothing in this file's check contract applies to it; [`common/check-go.sh`](common/) is what the gate runs over it. |
| [`../LICENSES/`](../LICENSES/README.md) | the SPDX texts [`common/fill-license.sh`](common/) reads. ⛔ Not scripts, and four of them must never be edited. |
```

From "⭐ The rules are ONE program, and it is not shell":

```text
⛔ **A POSIX sh check cannot be assumed to run on Windows**, which is why the
rules are not shell. Measured on one Windows 11 machine, 2026-08-25, from a
native PowerShell session with Git Bash NOT on `PATH`:
```

From "common/check-one-home.sh":

```text
⛔ **It carries no router exemption, and it used to.** `AGENTS.md` and
`docs/AGENTS.md` each stated the absolutes in full, so the pair was exempt from
each other by name; the root file was deleted on 2026-08-30 and the exemption
went with it. ⭐ An exemption for a file that no longer exists grants itself to
whatever lands at that path next, so it is deleted rather than emptied.
```

From "common/check-powershell.ps1":

```text
⛔ **IT ANALYSED `scripts/` ALONE UNTIL 2026-09-17, and the one real finding in
the tree was in the half it did not look at.** The parse loop always covered
every tracked file; the analyzer took one directory. `consumer.ps1` held a
non-ASCII byte with no byte order mark, which is the exact rule
`PSUseBOMForUnicodeEncodedFile` exists for and which that file's own header
claimed it did not need. It also found `$args` assigned inside a function in
`shell-matrix.ps1`, which
[`../docs/conventions/shell.md`](../docs/conventions/shell.md) section 8
forbids, and a variable computed and never asserted in `acceptance.ps1` whose
case was named for the half it dropped. A guard on one of several paths into the
same thing is the commonest hole there is.
```

From "consumer, the gate's drive of the released-binary contract":

```text
⛔ **It ran ONLY against published binaries until 2026-09-17, so a refusal added
to the tool could not be detected until a tag was cut.** It fired for real:
`wsl-toolkit-v3.0.0` published green and its smoke job then failed, because
`consumer.ps1` wrote a configuration shape `WSL-74` had made a refusal months of
commits earlier. `WSL-91`.
```

From "common/text-tool.sh and common/text-tool.ps1":

```text
⭐ **What it keeps, measured on 2026-09-17:** a file's CRLF endings, a file with no
trailing newline, bytes that are not UTF-8, and the file's mode. The same base64
given from bash and from PowerShell produced byte-identical files.
```

From "common/set-record.mjs":

```text
⚠ **Since 2026-09-17, closing an entry can also mean editing the work order by
hand.** `check-record`'s rule 7 refuses a work order item that is not marked
Closed when every entry it names is done, and this writer moves the numbers
alone - it does not touch prose and is not going to start. So the gate is what
tells you the order is now behind the work, which is the whole point of the
rule.
```

From "common/tmux.conf":

```text
⛔ **The one exception is behind a version test, and without it an agent cannot
type a newline.** tmux strips modifier information by default, so `Shift+Enter`
arrives as plain `Enter` - and pi binds `Enter` to submit and `Shift+Enter` to
insert a newline, so under an unconfigured tmux the second one submits. The fix is
`extended-keys on` with `extended-keys-format csi-u`, and the second needs tmux
**3.5**. The file tests the version in the shell's own `case`, because `sort -V`
and `awk` are not on every image this repository installs into. Driven on
2026-09-15: on tmux 3.5a both options read back set, tmux started with exit 0 and
**empty stderr**, and the rest of the configuration still applied; the test skips
2.9 through 3.4b and sets 3.5, 3.5a, 3.6, **3.10** and 4.0, which a numeric
comparison would get wrong.
```

From "common/shell-profile.sh":

```text
| measured on 2026-09-17, `matrix --images all`, each image driven twice - without the profile, then with it | result |
| --- | --- |
| images, and shells found on them | 13 and **28** |
| shells that add a byte to stderr, login or interactive | ⭐ **0**. ⚠ The assertion is the DELTA, because Photon's own `dircolors.sh` writes 66 bytes either way |
| shells that de-duplicate a planted repeat for an interactive shell | **28 of 28** |
| shells that leave a non-interactive shell's `PATH` exactly as it was | **28 of 28** |
| shells that gained a history home | **13**, every `sh`, `dash` and `ash`; the other 15 kept their own |
| shells that honour `WSL_TOOLKIT_NO_PROFILE` | **28 of 28** |
```

### `scripts/doctor/README.md`

From "Measured runtime":

```text
The numbers are from this machine on 2026-08-25 and are here so a session
knows what to expect, not as a claim about any other host. Windows is the slow
case: a process spawn costs more there than anywhere else, and this spawns one
per tool. Re-measure rather than quote these if the answer matters.
```

### `skills/text-tool/SKILL.md`

From "5. The operations":

```text
⛔ **`--between` NEEDS `--expect` FROM `wsl-toolkit-v4.0.0`, AND IT REPORTS THE
LINES IT TOOK.** ⚠ On a 3.1.0 binary the flag is optional there, so a reader
holding an older build will not see the refusal this section describes;
`text-tool --help` names what your copy really requires. It is the
widest operation here - it deletes a whole region rather than one line - and it
was the only search without a required count until 2026-09-17: with no
`--expect` it wrote nothing and exited **0**, both when two ranges matched and
when none did. ⚠ **A count is still not enough on its own.** An anchor that also
appears earlier in the file pairs the FIRST copy with the closing anchor, which
is exactly one match, so `--expect 1` is satisfied and a far bigger region goes.
That happened here, to a 43 KB script: 745 lines, reported as `1 match(es)`. The
report now names the span - `lines 2-9 (8 line(s))` - so read it.
```

### `skills/wsl-toolkit-agents/SKILL.md`

From "4. Start an agent":

```text
⚠ **The flags above were measured on 2026-09-17 and this page is not their
authority.** herdr moves. If one is refused, ask the CLI, as the section before
this one says, and use what it answers.
```

From "6. ⭐ Read back what the agent is really on":

```text
Two silent failures found this way on 2026-09-17, both of which every file said were
fine:
```

### `tools/windows/wsl-toolkit/README.md`

From "Release":

```text
⭐ **From `wsl-toolkit-v3.0.0`, the release also carries herdr's newest stable release**, which
[`herdr-build.yml`](../../../.github/workflows/herdr-build.yml) builds for Windows
`x86_64` and `aarch64` and Linux `x86_64` and `aarch64`, and `release.yml` covers with
its `SHA256SUMS` and signs. The same workflow builds herdr's development branch for
[`herdr-nightly.yml`](../../../.github/workflows/herdr-nightly.yml), which publishes a
prerelease on a `herdr-nightly-*` tag and keeps the newest seven. `WSL-90`. ⭐ **The
first nightly is published**, `herdr-nightly-20260916-18061191fdc0`, on 2026-09-16, with
all four builds green. ⚠ `herdr-build.yml` publishes nothing by design and is dispatched
by hand. ⭐ **`release.yml`'s herdr jobs ran for the first time on 2026-09-17**, cutting
`wsl-toolkit-v3.0.0`. From `wsl-toolkit-v3.1.0` the same workflow also builds
`text-tool` for Windows and Linux on both architectures, and runs the staged Windows
one before publishing it.
[`../../../docs/consumers.md`](../../../docs/consumers.md) lists what each release
carries; this page does not repeat it.
```

### `tools/windows/wsl-toolkit/adapters/README.md`

From "⭐ How a version moves without an edit to this tree":

```text
⭐ **`pi` and `omp` are the next two, and both now have entries rather than a
sentence.** `WSL-88` and `WSL-89` in
[`../../../../TODO/wsl-toolkit-go.md`](../../../../TODO/wsl-toolkit-go.md) carry
what each installs, costed from a reference sweep on 2026-09-15: both are npm
packages installed with `--ignore-scripts` and **neither needs a piped installer**,
both already have official herdr integrations, and both give herdr **lifecycle
authority** rather than the screen detection Muse gets. ⛔ `WSL-89` also carries
the one trap: herdr **refuses** the omp integration when pi and omp resolve to the
same extension directory, and `PI_CODING_AGENT_DIR` is read by both.
```

From "⛔ An agent has to be on the PATH a PANE has":

```text
⛔ **`$HOME/.local/bin` is NOT on it**, and `/etc/profile` appends `/usr/local/bin`
and nothing else. So an adapter that installs through `npm -g` into the account's
prefix produces an agent that works from `base exec`, works from the adapter's own
script, and **cannot be started by herdr at all**: the pane answers `command not
found` and `herdr agent start` times out on an agent that was never going to appear.
Measured 2026-09-17 for `pi` and `omp`, both of which installed cleanly and neither
of which herdr could launch.
```

From "⛔ An agent has to be on the PATH a PANE has":

```text
⛔ **AND IT HAS TO RESOLVE TO THE WRAPPER, NOT MERELY RESOLVE.** `bootstrap.sh` writes
`export PATH="$HOME/.local/bin:$PATH"` into the account's profile, so after
`base bootstrap` a login shell finds the vendor's own launcher first and the wrapper on
the system path is never reached. Measured on 2026-09-17 by planting that one line: all
three agent names moved. ⭐ **So an agent adapter's probe reads the resolved file and
refuses one that does not carry this tool's marker**, and it reads it through
`runuser -l`. ⚠ The muse probe already checked the wrapper and could not see this,
because it asked `as_account`, whose PATH is curated: a guard proved on a path nobody
uses, for the second time.
```

### `tools/windows/wsl-toolkit/examples/common/README.md`

From "⛔ Which multiplexer, and it is not a preference":

```text
⛔ **tmux inside a herdr pane hides the agent from herdr.** herdr's own agents
page says detection does not inspect a tmux session launched inside a pane, so
herdr sees `tmux` as the pane process and the agent behind it becomes invisible:
no `idle`, no `working`, no `blocked`, no notification. ⚠ **A shell framework that
auto-enters tmux therefore breaks the whole point of running agents under herdr.**
Read on 2026-09-15 and recorded in
[`../../../../../docs/reference-sweeps/usable.md`](../../../../../docs/reference-sweeps/usable.md).
```

### `tools/windows/wsl-toolkit/examples/common/herdr.md`

From "herdr: one server in the base, two ways in":

```text
⚠ **What herdr does on this host is measured where a line says so, on 2026-09-15,
with herdr 0.9.0 on both sides.** The rest is read from herdr's documentation for
0.9.0. [`../../../../../docs/reference-sweeps/usable.md`](../../../../../docs/reference-sweeps/usable.md)
carries the sweep and its commits; this page carries only what an operator does.
```

From "⛔ Two things to know before the first attach":

```text
1. ⛔ **herdr 0.9.0's Windows `--remote` client is reported to repaint only on
   window activation and to apply no prefix command.** That is
   `herdrdev/herdr#4176`, closed `not_planned` as a duplicate of `#4038`, which herdr
   closed as fixed on its development branch. ⚠ **The newest stable release is
   `v0.9.1`**, published 2026-09-16T18:40:01Z and read on 2026-09-17; ⛔ **whether it
   carries that fix is not measured here**, and `herdr-remote-probe.ps1` is what would
   settle it. ⭐ **What this repository ships and drives is the nightly**, built from
   the development branch where herdr closed `#4038` as fixed. ⭐
   **Measure the repaint before concluding anything about SSH**: a client that
   repaints only on window activation looks exactly like a connection that is not
   working.
2. ⚠ **herdr's Linux release binary is static and carries its own musl allocator,
   whatever the base's libc is.** `herdrdev/herdr#4174`, open, is a heap corruption
   that allocator detected inside a 0.9.0 server, so a glibc base does not avoid it.
   The base is `arch` because the adapters are measured on that preset alone.
```

From "⭐ Watch the agents without opening anything":

```text
| measured on 2026-09-15, herdr 0.9.0 on Windows and in the base | result |
| --- | --- |
| `herdr --machine base agent list` | ⛔ exit 2, `unknown option: --machine`. 0.9.0 has no such prefix |
| a build of herdr's development branch, `--machine base agent list`, against the 0.9.0 server | exit 1, `remote Herdr does not support machine API forwarding` |
| `herdr machine add wsl-toolkit-base --label base` | exit 0 in 2.7 s. It saved the profile in `%LOCALAPPDATA%\herdr\client\endpoints.json`, installed nothing in the base, started no second server, and created a workspace on a server that had none |
| a development build on both sides, `--machine base agent list` | exit 0 in 1.3 s, with no terminal UI open |
```

From "⭐ Watch the agents without opening anything":

```text
⭐ **`--machine` needs a development build on both sides.** Set the base's herdr adapter
to `"channel": "nightly"`, as the manual's herdr adapter section says, and `base attach`
prints the Windows client of the build the base runs. ⭐ **Driven on 2026-09-16**: with
that channel set, `base ensure` installed `herdr-nightly-20260916-18061191fdc0` in 8.78 s,
`base attach` printed the nightly's own client, and that client answered `--machine base
agent list` with exit 0 in 2.1 s.
```

From "⭐ Starting an agent, and reading what it says":

```text
⛔ **Read it back rather than trusting the configuration.** Both pi failures found on
2026-09-17 - a model it could not resolve, and an effort it clamped - looked correct in
every file and wrong in that one line.
```

### `tools/windows/wsl-toolkit/examples/muse-code/README.md`

From "Agents in one named WSL base":

```text
Every command below was run on 2026-09-17, on Windows 11 Pro 26200 with WSL 2.7.12.
```

### `tools/windows/wsl-toolkit/examples/windows-repo/README.md`

From "A GitHub repository on Windows, an agent inside the base":

```text
Every command and every reading on this page was driven on Windows 11 Pro 26200,
WSL 2.7.12, against `wsl-toolkit-base`, on 2026-09-17.
```

From "3. Start the agent from Windows, and it runs inside the base":

```text
⛔ **Measured on this host on 2026-09-17: `omp` resolved to a native Windows
install and not to the launcher.** `C:\ProgramData\scoop\persist\bun\bin` sat at
`PATH` position 3 and `%USERPROFILE%\bin` at position 66, and both files exist. The
native program answered `omp/18.1.19` while the base holds `omp/18.2.3`. A native
program cannot see the grant and cannot see the base. Run the launcher by its full
path when the name is taken:
```

From "⚠ Can a native Windows agent run its commands in the base instead?":

```text
⭐ **The seam exists.** A native Windows omp takes `shellPath`, and its shell is
called as `SHELL -c "COMMAND"`, which is the shape `base exec -c` already takes. A
shim that forwards one to the other, after moving to the guest path that matches
the Windows directory, was written and driven on 2026-09-17:
```

From "⚠ Can a native Windows agent run its commands in the base instead?":

```text
| driven | result |
| --- | --- |
| a relative command | exit 0. `pwd` answered `/workspaces/proj` and `uname -s` answered `Linux` |
| a command carrying a Windows absolute path | ⛔ exit 1, and the path arrived as `C:UsersAjamX...` with every backslash eaten as a shell escape |
```

From "⛔ The one sharp edge: the agent cannot commit":

```text
An agent that runs `git commit` **inside** the base fails. Measured on the
operator's base on 2026-09-17:
```

From "What the base can and cannot reach":

```text
| reading, 2026-09-17 | result |
| --- | --- |
| the granted directory, written by the base's account | writes succeed |
| who owns the checkout, as the base sees it | the base's own account, so git raises no ownership refusal |
| `git --version` in the base | 2.55.0 |
| `git ls-remote https://github.com/...` from the base | exit 0, so the base reaches GitHub |
| `user.name`, `user.email`, `credential.helper` in the base | none of the three |
| `/mnt` in the base | `wsl` and `wslg`, and no Windows drive |
```

### `tools/windows/wsl-toolkit/wsl-toolkit.md`

From "Operating model":

```text
⛔ **Do not call `wsl.exe` with a job payload, and do not write a wrapper for
it.** A command handed to `wsl.exe` as an ARGUMENT is expanded before the guest
sees it and the result is parsed a second time: measured on 2026-09-09, a
payload's backtick was EXECUTED and the command still reported exit 0 over the
failure. This tool sends payloads on stdin or as a file.
```

From "⛔ What the heartbeat can and cannot measure, per feed":

```text
⛔ **A feed that does not exist reports ABSENT, and never a zero.** Each row was
measured on this base on 2026-09-17, against podman 6.1.1.
```

From "Dedicated provider bases":

```text
| measured on 2026-09-14, on a throwaway arch base with interop off | result |
| --- | --- |
| each of the six changes between `off`, `ro` and `rw`, three on a base built with the first value | `base status --probe` exit 1, naming the setting and the drives; `base ensure` exit 0 in 4.3 s to 4.7 s, provisioning again; then all ten drive mounts read-only under `ro`, writable under `rw`, and none under `off` |
| guest root remounts one drive `rw` in an `ro` base | `base status --probe` exit 1, the drives `mixed`, 9 read-only and 1 writable; `base ensure` exit 0 in 4.5 s, and the drive `ro` again |
| `base shell --here` on a base built `off` and set to `ro` | exit 2 naming `base ensure`; after it, the shell started in a directory under `/mnt/c` |
```

From "⭐ Reaching herdr, and through it the agents":

```text
⛔ **The herdr the base pins, 0.9.0, has no `--machine` prefix.** Its Windows client
exits 2 on the flag, `unknown option: --machine`, measured on 2026-09-15. A build of
herdr's development branch has it, and refuses a 0.9.0 server with `remote Herdr does
not support machine API forwarding`, so the prefix needs a newer herdr on both sides.
```

From "⭐ Reaching herdr, and through it the agents":

```text
⭐ **`base herdr` exists because the second line is a quoted shell string.** A herdr
prompt is prose, and prose carries quotes, dollar signs and backticks; measured on
2026-09-09 against a real distribution, a payload's backtick was EXECUTED and the
command still reported exit 0. `base herdr` takes herdr's arguments as arguments
and quotes each one once, on the same path `base agent` uses.
```

From "⭐ Reaching herdr, and through it the agents":

```text
| measured on 2026-09-15, driven under bash, dash and busybox ash | result |
| --- | --- |
| a NON-interactive login shell on `/mnt/c` | stays there, and writes **0 bytes** to stderr. `distro run -c` and `matrix -c` are login shells, so moving one would change every caller's working directory |
| an interactive shell on `/mnt/c` | starts in the account's home, with one line of 147 bytes on stderr, identical in all three shells |
| an interactive shell with `WSL_TOOLKIT_HERE` set | stays on `/mnt/c` |
| an interactive shell in `/workspaces/project` or `/mnt/wsl` | stays. Only one level under the automount root is a Windows drive |
| an interactive shell on `/mnt/c` with `root = /windows/` in `/etc/wsl.conf` | stays, and one under `/windows/c` moves. The root is read, not assumed |
```

From "⭐ Reaching herdr, and through it the agents":

```text
| measured on 2026-09-15, on a throwaway arch base with interop off | result |
| --- | --- |
| set from `false` to `true`, then from `true` to `false` | each time `base status --probe` exit 1 naming the account's sudo; `base ensure` exit 0 in 15.9 s and 16.0 s, provisioning again; then `sudo -n true` granted, then refused, and the probe exit 0 |
| under `false`, a second rule that is not this tool's granting the account sudo | `base ensure` exit 2 in 14.5 s, `re-provisioned and it still does not verify`, naming the account's sudo; with that rule removed, exit 0 in 2.5 s |
```

From "⭐ Reaching herdr, and through it the agents":

```text
⚠ **A process `base exec` starts in the background does not outlive the command.**
Measured on 2026-09-14 in a systemd base: `setsid sleep 3600 &` was gone by the next
command, two seconds later. Run anything that has to keep running in a herdr pane.
```

From "Grants that change live":

```text
| measured on 2026-09-14, on a throwaway arch base | result |
| --- | --- |
| two `base grant`, then `base revoke` | 0.4 s to 0.5 s each; a process in a herdr pane started before the grants was running after both, and one started before the revoke was running after it |
| `git status --short` through `base exec --dir` in a granted checkout | exit 0 |
| `wsl --terminate`, then `base ensure` | 6.6 s and 10.2 s in two runs, and both grants verified |
| `base revoke` while a pane's process stood in the directory | exit 1 naming `target is busy`, everything else unchanged; after the pane closed, the revoke took 0.4 s |
```

From "The shell base shell gives you":

```text
⚠ **A missing `.bashrc` or profile is not a failure.** `bash -l` with no `/etc/profile`,
`~/.bash_profile`, `~/.bash_login` or `~/.profile` starts normally and reads nothing;
measured 2026-09-17 with an empty `HOME`, exit 0.
```

From "Adapters, and herdr":

```text
| measured on 2026-09-14, on a throwaway arch base | result |
| --- | --- |
| `base recreate` with the adapter, then `base ensure` | 68.7 s from nothing, the download and its digest included; then 3.7 s, rewriting only the tracked configuration |
| `ssh wsl-toolkit-NAME` through the block | key authentication and a command in 0.2 s |
| herdr's Windows client, `herdr --remote` | connected to the base's server through the block `base ensure` wrote, and detached on prefix then q |
| `wsl --terminate`, then one `ssh` through the block | the distribution started, the unit started the server, and herdr restored its workspaces, in 5.4 s |
| twelve minutes with nothing attached | the base stayed running |
| prefix then x, then prefix then shift+x, in two sessions made alike | herdr's own keys closed a pane, then a tab, each at once with no question; the tracked file closed nothing |
```

From "Adapters, and herdr":

```text
`base ensure` resolves the newest `herdr-nightly-*` prerelease, installs its Linux build
only when the digest matches the one that nightly's `SHA256SUMS` publishes, and writes
the Windows client of the same build to `herdr\TAG\herdr.exe` under the instance's state
directory. `base attach` then prints that client, which is the one with `--machine`, and
`base remove` takes it away. ⚠ **A nightly's digest proves transport, not authorship**:
`SHA256SUMS` ships beside the files it covers, and the keyless bundle beside each file,
which verifies against `herdr-nightly.yml`, is not checked by this tool. ⛔ A channel
beside a `version` or `sha256` is refused. ⚠ `base ensure` never restarts a running
server, so a newer build serves nothing until the server next starts; `base status
--probe` reports it as `server-binary-stale`, with the installed `release` and `sha256`.
⭐ **Driven on 2026-09-16, against the first published nightly.** With `"channel":
"nightly"` on `wsl-toolkit-base`, `base ensure` exited 0 in 8.78 s and installed
`herdr-nightly-20260916-18061191fdc0` over the digest its `SHA256SUMS` publishes;
`base status --probe` then read `release`, that tag, `sha256`
`213580fc…f92f14a1`, and `server-binary-stale no`. ⚠ Until a nightly exists `base
ensure` refuses the channel with `this repository has published no herdr nightly`.
```

From "Adapters, and herdr":

```text
⭐ **`muse` installs Muse Code for the base's account, and runs Meta's installer only
while its digest is approved.** `base ensure` saves the installer Meta serves, prints
its length and SHA-256, and runs it as the account when that digest is the one the
adapter pins, which the operator approved on 2026-09-14, or the `installer_sha256`
the configuration's `muse` entry carries. ⛔ Any other stops the
ensure with exit 2, keeps the file at `/var/lib/wsl-toolkit/muse/install.sh` and
prints how to read it. Add its digest only after reading it:
```

From "Adapters, and herdr":

```text
A base whose Muse answers a version does not fetch the installer again. ⚠ Muse's own
launcher, as read on 2026-09-14, looks for a newer launcher and a newer Muse at most
hourly after that. `/usr/local/bin/muse` puts it on `PATH` for `base exec`, whose shell reads no
profile, and refuses every other account. ⛔ **Signing in is the operator's:** `base
shell`, then `muse login`. `base status --probe` reports the version, where `muse`
resolves, and whether a credential file is present, never what it holds.
```

From "Adapters, and herdr":

```text
| measured on 2026-09-14, on a throwaway arch base from a fresh clone | result |
| --- | --- |
| `base ensure` from nothing, with `herdr` and `muse` | 77.6 s; Muse Code 1.2.1 installed as the account, approved by the pinned digest |
| a build whose pin differs, over a base with no launcher | exit 2 in 4.4 s, the installer saved and not run, its digest and the approving key printed |
| the same build, with that digest as `installer_sha256` | exit 0 in 5.6 s, approved by the configuration |
| `base ensure` over an installed Muse | exit 0 in 3.8 s, the installer not fetched |
| `muse --version` through `base exec`, then as root | `Muse Code 1.2.1 (1.2.1-R2847.1)`, then exit 126 |
```

From "Adapters, and herdr":

```text
| measured on 2026-09-15, herdr 0.9.0 and Muse Code 1.3.0 with `--provider echo`, in a pane of the base | result |
| --- | --- |
| the same hook in `~/.config/muse/settings.json`, `~/.muse/settings.json` and a workspace's `.muse/settings.json` | it ran from the first only; the other two were ignored without a word |
| the command straight under an event, then inside `{"matcher":"","hooks":[...]}` | it ran only inside the matcher group |
| a settings file with no `schema_version` | Muse exit 1, `malformed settings file ... missing field schema_version` |
| a turn submitted with `herdr agent prompt` | reported `idle`, `working`, `idle`; `herdr agent wait --until working` returned when the hook reported, with no screen rule matching |
| `/exit`, then `muse resume` and a prompt | reported `idle` and released; the resumed session sent no `SessionStart`, and the hook adopted the pane at a sequence above the release |
```

From "Adapters, and herdr":

```text
⭐ **`pi` and `omp` install the other two agents from npm, and take herdr's own
integration.** Both are npm packages, so there is no installer to approve and no
digest to pin: npm checks a package against the registry's own integrity value.
`version` on either entry pins a release. Each writes a wrapper at
`/usr/local/bin/NAME`, because npm installs into `~/.local/bin` and ⛔ **a herdr pane
does not have that on `PATH`** - measured on 2026-09-17, a pane answered `command not
found` and `herdr agent start` waited for an agent that could never appear. Each
adapter reads the name back through `runuser -l`, which is the PATH a pane really
has, and refuses the install when a login shell still cannot find it.
```

From "⭐ The model and the reasoning effort a new session starts on":

```text
⛔ **NOT AN ENVIRONMENT VARIABLE, AND THAT IS THE WHOLE REASON THESE FIELDS EXIST.**
An agent herdr starts inherits the herdr **service's** environment, not a login
shell's - measured on 2026-09-17 by reading `/proc/PID/environ` for every pane
process - so a default exported in `~/.profile` never reaches it.
```

From "⭐ The model and the reasoning effort a new session starts on":

```text
⛔ **`base bootstrap` PUTS THE ACCOUNT'S PREFIX AHEAD OF THE WRAPPER.**
`bootstrap.sh` writes `export PATH="$HOME/.local/bin:$PATH"` into the account's profile,
so after it runs a login shell finds each agent's own launcher first and
`/usr/local/bin/NAME` is never reached. Measured on 2026-09-17 by planting that one
line: all three names moved. ⛔ Muse then starts with neither the model nor the effort,
because it has no settings key for either. ⭐ Every agent probe reads the name back on a
LOGIN shell and refuses a resolution that is not this tool's wrapper, so
`base status --probe` says so rather than calling that base healthy.
```

From "⭐ The model and the reasoning effort a new session starts on":

```text
⚠ **The effort vocabulary is each agent's own**, read from its own `--help` on
2026-09-17, and a word an agent does not take is refused rather than sent to it:
```

From "⭐ The model and the reasoning effort a new session starts on":

```text
⛔ **A startup model pi cannot resolve is not an error to pi**: it falls back to its
own built-in default, on a different provider, and says nothing. Measured on
2026-09-17, a correct `defaultModel` gave `(anthropic) claude-opus-4-8` because
`models.json` declared four ids and not that one - the gateway serving a model and
pi's catalogue knowing it are two different facts. ⚠ **And `max` clamps to `high`
without a `thinkingLevelMap`**, which pi's own documentation states. `base status
--probe` reports `startup_model` and `startup_effort` and raises a problem for either
case, so neither is silent again.
```

From "⭐ The model and the reasoning effort a new session starts on":

```text
| measured on 2026-09-17, on the operator's own base, each agent started BY herdr and its own screen read back | what the agent said |
| --- | --- |
| `muse` | `Model set to muse-spark-1.3-contributor`, status line `muse-spark-1.3-contributor · max · ~` |
| `pi` | `(muse-gateway) muse-spark-1.3-contributor • max` |
| `omp` | `Muse Spark 1.3 Contributor` |
```

From "Agents from Windows":

```text
| measured on 2026-09-14, on a throwaway arch base with the muse adapter | result |
| --- | --- |
| `base ensure` from nothing, the instance's own configuration | 76.6 s, and the launcher written |
| `muse-m78.exe --version` in the granted project, then in a directory beneath it | `Muse Code 1.2.1 (1.2.1-R2847.1)` and exit 0, both |
| the same in a directory no grant covers | exit 2, with the `base grant` line |
| `muse-m78.exe` with no argument | exit 2, naming the herdr route |
| `muse-m78.exe --definitely-not-a-flag` | exit 2, Muse's own, which `base exec` also read |
| a launcher from another build | the probe exit 1 naming it; `base ensure` rewrote it, and the probe exit 0 |
| `base remove --yes` | the launcher removed with the distribution |
```

From "Container platform":

```text
⚠ **A shared kernel carries handlers this tool did not register.** The WSL2
kernel outlives any one distribution, so a foreign-architecture rootfs may RUN
rather than fail, which is the worse outcome. Measured 2026-08-27: 31 `qemu-*`
handlers were visible from a freshly imported Alpine containing no emulator, and
a riscv64 rootfs booted and answered `riscv64`. That is why the architecture is
named at pull and create time rather than checked afterwards.
```

From "⭐ A BSD userland, with no nesting":

```text
⚠ **Every run boots the guest and powers it off, so every run pays about 23
seconds.** Measured on 2026-09-14 over three runs of `bsd run -c true` with the
one-vCPU default and the run's overlay: a login prompt at 8.6 s, 8.6 s and 8.8 s, the
command finished at 16.5 s, 16.5 s and 16.8 s, and the process gone at 23.4 s, 23.4 s
and 23.6 s. Nothing keeps a guest running between runs. ⚠ Every run is the image's
first boot, because no run writes the image: FreeBSD grows its root to the disk and
generates its SSH host keys before each login, and the three runs above include it.
⚠ The guest is not shown the host's hypervisor signature, because a FreeBSD kernel
that sees it waits about 105 seconds before mounting root, for a Hyper-V VMBus QEMU
does not provide.
```

From "⭐ A BSD userland, with no nesting":

```text
| measured on 2026-09-14, on the shared image | result |
| --- | --- |
| `bsd run -c 'touch /root/tk-overlay-probe'`, then `bsd run -c 'test ! -e /root/tk-overlay-probe'` | both exit 0; the image's SHA-256 the same before the first run, after the second and after three more; no overlay left |
```

From "⭐ A BSD userland, with no nesting":

```text
⭐ **The default is one vCPU.** Five fresh-image toolchain installs completed
without a kernel panic with one processor on 2026-09-14, where the same guest
panicked under every two-processor CPU model and memory size measured. ⚠ **One
processor lowers the rate and does not end it:** the same day, two read-only runs in
a row on the shared image panicked while powering off. `--cpus` overrides the
default.
```

From "⭐ A BSD userland, with no nesting":

```text
⭐ **The guest disk is 12 GiB, which gives a 10 GiB root filesystem.** The published
image is 6.0 GiB with a 4.8 GiB root, and a toolchain install fills that. `bsd run`
gives the run's overlay a size of `--disk` GiB, 12 by default, then extends the
partition and the filesystem with FreeBSD's own `gpart` and `growfs` before the
payload runs. ⚠ A boot partition, an EFI partition and 1 GiB of swap sit ahead of
root, so the root is smaller than the disk: measured on 2026-09-14 by `df -k /`, an
11 GiB disk gives a root of 10,110,092 KiB and a 12 GiB disk 11,138,540 KiB. `bsd
status` prints the disk, and the run's last line prints both sizes.
```

From "⭐ A BSD userland, with no nesting":

```text
⛔ **The guest kernel can panic.** Measured on 2026-09-14: `bootstrap.sh --toolset
languages` in a 2048 MiB guest panicked in the page daemon while `pkg` installed.
`bsd run` ends a run with exit 2 and the panic line when QEMU exits after the console
shows a panic's two lines, `panic: ` and `cpuid = ` under it, or 60 seconds after
them, and what the panic wrote goes with the run's overlay. ⚠ The payload's output is
on the same console, so a payload that prints those two lines itself and finishes
within the 60 seconds answers with its own exit and output; one that runs on past
them is ended as a panic.
```

From "⭐ A BSD userland, with no nesting":

```text
| measured on 2026-09-14, on the shared image | result |
| --- | --- |
| a script printing `panic: page fault` and `cpuid = 0`, then sleeping 2 s, printing a line and exiting 7 | exit 7 with all three lines, the session 22.9 s; the build before this rule answered exit 2, `the guest's kernel panicked`, and cut the output after the two lines |
```

From "⭐ A BSD userland, with no nesting":

```text
| measured on 2026-09-14, on a throwaway copy of the image | result |
| --- | --- |
| an earlier build killed its guest mid-command, then a run booted the copy | exit 0, `root_not_dismounted` `WARNING: / was not properly dismounted` and the warning; the copy's SHA-256 the same after the run |
```

From "⭐ The doors, attacked rather than read":

```text
⚠ **`base.interop = "off"` deliberately claims only the PATH door**, because the
other two are not this tool's to promise. Measured on 2026-09-17, across two
distributions and three utility-VM lifetimes: the `WSLInterop` `binfmt_misc`
registration does **not** follow a distribution's own `[interop] enabled` setting,
and both values were observed on the same distribution with the same configuration.
`wsl-toolkit-base`, configured `enabled=false`, was found with the handler live,
accepting a PE image and handing it to `/init`; `wsl-toolkit`, configured
`enabled=true`, was found with none and could not execute a Windows binary. ⚠ The mechanism has not been read. So
`interop.exec-pe` is reported and never claimed, and `interop.run-exe` reports
`unknown` on a base with no Windows path reachable, because there is no executable
to try and saying so is the honest answer.
```

From "⭐ The doors, attacked rather than read":

```text
| measured on 2026-09-17, on `wsl-toolkit-base`: automount off, interop off, passwordless sudo on | result |
| --- | --- |
| `base doors` | exit 0, 12 s with the distribution already running, 19 s including its start |
| closed, and each by an attempt | `fs.windows-drives`, `fs.mount-drvfs` (`must be superuser to use mount`), `interop.windows-path`, `net.windows-host-smb`, `net.windows-host-rdp`, `priv.unshare-netns` |
| read-only | `fs.wsl-drivers`, 9p, a write refused |
| ⛔ open, and no setting here closes them | `fs.mnt-wsl-shared`, `net.windows-host-icmp`, `net.internet`, `priv.passwordless-sudo`, `priv.unshare-userns`, `priv.unshare-user-plus-net`, and `interop.exec-pe` whenever the handler is live |
| ⚠ the first driven run took **277 s** | two filtered ports, connected through bash's `/dev/tcp`, which carries no deadline. Every route now has one and the bash route is not used at all without `timeout` |
```

From "⭐ A network namespace of the account's own":

```text
⭐ **The rule is not in the shared namespace**, which is the condition the operator's
ruling of 2026-09-14 carries. Every WSL distribution on a host shares one network
namespace, so a rule written there would change the network of the podman machine
and of every other base.
```

From "⭐ A network namespace of the account's own":

```text
⛔ **No container can run inside it.** `pasta --config-net` puts the payload in a
user namespace where it is uid 0, so podman takes itself for rootful and chooses
system paths it cannot write. Measured 2026-09-17 three ways: plain, with `--root`
and `--runroot` pointed at the account's own directories, and with
`_CONTAINERS_USERNS_CONFIGURED` set. Each failed, the refusal moving between
`/var/lib/containers` and `/run/libpod`. ⭐ **That is why this is a flag on one
command and not how every command in the base starts**: the base exists to run
containers as that account.
```

From "⭐ A network namespace of the account's own":

```text
| measured on 2026-09-17, on a throwaway arch base with no grants | plain | `--private-net` |
| --- | --- | --- |
| the Windows host, ICMP | open | ⭐ **closed** |
| the internet, `1.1.1.1:443` | open | open |
| DNS | ok | ok |
| the namespace | `net:[4026531833]`, shared with every distribution | ⭐ `net:[4026532297]`, its own |
| the shared namespace's own ruleset, after | empty | ⭐ **empty, and unreadable to the account** |
| a payload exiting 0, 7 and 42 | 0, 7, 42 | ⭐ 0, 7, 42 |
| bytes the wrapper adds to stderr | | ⭐ **zero**: 116 against 116 for the same command |
```

From "⭐ Closing the shared tmpfs, and what it costs":

```text
⛔ **`/mnt/wsl` is one `tmpfs`, common to every distribution in the WSL2 utility
VM, mounted `drwxrwxrwt`, with uids not namespaced across it.** A base with no
grants at all wrote a file there that another distribution read, measured
2026-09-15. It is the last door a zero-grant base has, and `base.shared_tmpfs`
is what shuts it: the provisioner installs
`/usr/local/lib/wsl-toolkit/seal-boot.sh`, which WSL runs as root at every start.
```

From "⭐ Closing the shared tmpfs, and what it costs":

```text
⭐ **The verifier reads the result, not the intention.** It refuses a base whose
`/mnt/wsl` is still mounted, whose `/etc/resolv.conf` is still a symlink or
carries no nameserver, or which cannot resolve a name. ⚠ That last one matters
because a `command=` value wsl.conf's parser rejects does not run and says
nothing about it: measured 2026-09-17, a value carrying nested quotes was
silently ignored, which is why the boot line is a bare path with no shell in it.
```

From "⭐ Closing the shared tmpfs, and what it costs":

```text
| measured on 2026-09-17, on a throwaway arch base with no grants | result |
| --- | --- |
| `base recreate` from nothing with `shared_tmpfs: "off"` | exit 0 in **110 s**, `wrote /etc/resolv.conf, 1 nameserver(s)`, boot script installed, verification passed |
| the account, after the restart | `/mnt/wsl` **not mounted**, a write refused, `getent hosts` **OK** |
| `base doors` | exit 0, `fs.mnt-wsl-shared closed`, `no tmpfs mounted at /mnt/wsl`, claimed by the setting |
| the open doors that remain | `net.windows-host-icmp`, `net.internet`, `priv.unshare-userns`, `priv.unshare-user-plus-net`. ⚠ **Four, and the comparison is not with the six `wsl-toolkit-base` reports**: that base has passwordless sudo on and its tmpfs open, so the two numbers differ by more than this setting. This base with the tmpfs open and the corrected probe was never measured, and saying so beats a delta nothing took |
| ⛔ with the unmount removed from the boot script by hand | `base doors` **exit 1** naming the claim, and `base status --probe` **exit 1**, `usable false`, naming the setting. Restored, both green again |
| three consecutive restarts before the setting existed | the earlier hand-driven form: tmpfs closed and DNS up on every one |
```

From "⛔ What a zero-grant base does NOT seal, with the measurement beside each":

```text
| door | measured | what it means |
| --- | --- | --- |
| ⭐ `/mnt/wsl`, **and this one now has a remedy** | 2026-09-15 both ways; closed and driven 2026-09-17 | one world-writable `tmpfs` shared by every distribution in the utility VM, with uids not namespaced across it. A zero-grant account wrote a file another distribution read. ⭐ **`base.shared_tmpfs: "off"` closes it at every start**, and the section above carries what that costs and what was measured. ⚠ It is off by default, so a base that does not ask for it still shares the directory |
| ⛔ the internet | 2026-09-15 and 2026-09-17 | `1.1.1.1:443` connects from a base with no grants. Nothing in this tool closes it |
| ⛔ the Windows host | 2026-09-15 and 2026-09-17 | `445` and `3389` were refused and **ICMP answered**, so the host is reachable. The address is `hostaddress`'s answer |
| ⭐ a private network namespace, **and this one now has a use** | 2026-09-15; refused from inside 2026-09-17 | `unshare -n` is refused and `unshare -Un` SUCCEEDS, so the account can make one with no privilege. ⚠ `pasta --config-net` alone still reached the internet and the Windows host, with and without `--no-map-gw`; ⭐ **an `nft` rule INSIDE that namespace refuses the host and the private ranges while the internet and DNS stay up**, which is what `base exec --private-net` runs. ⛔ No container runs inside it |
| ⛔ a PE handler this tool cannot remove | 2026-09-17, two distributions, three utility-VM lifetimes | the `WSLInterop` `binfmt_misc` registration does not follow `[interop] enabled`, and both values were seen on the same distribution with the same configuration. `base doors` reports `interop.exec-pe` and never claims it |
| ⚠ `/dev/kvm`, `/dev/dxg`, `/dev/vsock` | 2026-09-15, read; 2026-09-17, read again | all three are `crw-rw-rw-` in a zero-grant base. **None has been attacked**, so what an unprivileged account reaches through them is unmeasured, and a threat model has to answer for them |
| ⚠ passwordless sudo, where it is on | by configuration | guest root can mount a Windows path the configuration never granted. It is authority, not containment, and `base doors` reports it open |
```

From "⚠ Four things about WSL that this tool cannot protect you from":

```text
- ⛔ **Every distribution on this machine shares one writable directory.**
  `/mnt/wsl` is a single `tmpfs`, mounted `drwxrwxrwt`, common to every
  distribution in the WSL2 utility VM. Measured on 2026-09-15: a base with **no
  grants at all** wrote a file there that another distribution read, and read one
  that distribution's root wrote; uids are not namespaced across it, so one inode
  lists under two different account names. ⛔ **So a zero-grant base is not a
  sandbox**, and this page never calls one that. Root can unmount it in one
  distribution, at every start, and doing so costs DNS because `/etc/resolv.conf`
  resolves into it. `WSL-68` owns the work.
- ⛔ **`wsl --shutdown` is machine-wide.** It is what a person reaches for after
  finishing with a throwaway distribution, and it stops every distribution on the
  machine, including the podman machine. This tool never runs it.
- ⚠ **A distribution's lifetime is not the kernel's.** `--terminate` and
  `--unregister` restart or remove the userspace; the WSL2 kernel keeps running,
  so `binfmt_misc` registrations, loaded modules and pinned superblocks survive.
  Measured 2026-08-26: a podman machine restarted seconds earlier was running a
  kernel ten hours old. Only `wsl --shutdown` gives a fresh one, and anyone
  reproducing a kernel-level condition without it reads stale state and gets a
  confident wrong answer.
- ⚠ **A distribution's `binfmt_misc` is not its own**, and that is the row
  above from the other side. Measured 2026-09-17: the
  `WSLInterop` registration was present in a distribution configured `[interop]
  enabled=false` and absent in one configured `enabled=true`, and both values were
  observed on the SAME distribution with the same configuration across three
  utility-VM lifetimes. The mechanism has not been read. Anything deciding whether
  interop is available by reading that file gets a confident wrong answer; run a
  Windows executable instead, which is what `base doors` does.
```

From "Known limits":

```text
| limit | kind | what it means |
| --- | --- | --- |
| the base enforces no per-container resource bounds | host | rootless podman under `init` with no cgroup delegation means no cgroup per container. `--memory` is accepted and not applied, and `podman stats` reads `0B`. ⭐ `base status` reports this. A caller who bounds a job on this base is not bounded |
| `podman logs` on the base is a silent zero | host | the default log driver is `journald` and nothing serves a journal. ⛔ **This page claimed the tool named `k8s-file` before the tool did**; it does from 2026-09-17, and a job's container answered 6 bytes and 52 where the same call answered 0 and 0. ⚠ The first line podman shows is this tool's own start marker. A caller driving podman directly should name a driver too |
| `bsd run` boots and powers off a guest per call, about 23 seconds each | host | nothing keeps a guest running between runs. The BSD section carries the measurement |
| nothing a payload installs survives its `bsd run` | decision | a run writes to an overlay it removes, so no panic can damage the shared image. A payload installs what it needs in the run that uses it |
| no BSD container endpoint | open | a long-running podman service panics the FreeBSD guest kernel. Tracked in [`../../../TODO/bsd.md`](../../../TODO/bsd.md) |
| `--oci-env` carries `ENV` and `WORKDIR` only | decision | `USER` and `ENTRYPOINT` are not carried and will not be: WSL fixes the login account per call, and a login shell has no entrypoint |
| a throwaway distribution's command gets no stdin | decision | its stdin is `/dev/null`, because a pipe that carries the script cannot also carry input. `distro enter` is interactive |
| there is no port forwarding | decision | forwarding a port on Windows needs an elevated session and leaves a rule behind. `hostaddress` answers the question it was wanted for: bind the host service to that address |
| an agent behind a launcher may be invisible to herdr | host | herdr reads a pane's foreground process, and `base agent` and a `NAME.exe` launcher are both wrappers. herdr's own remedy is `HERDR_AGENT=<agent>` set on the wrapper command **on the herdr side**; it cannot see one set inside the guest. ⚠ Read from herdr's documentation on 2026-09-15 and not yet measured here |
| a base carrying both pi and omp can be configured so herdr refuses one | decision | `PI_CODING_AGENT_DIR` is read by both, so one exported for pi redirects omp onto pi's extension directory and herdr refuses the omp install. The omp adapter refuses first, naming both paths and the variable |
| ⛔ a base is not a sandbox, and the network is now the reason | host | `/mnt/wsl`, the world-writable `tmpfs` every WSL distribution shares, ⭐ **closes with `base.shared_tmpfs: "off"`**, driven 2026-09-17. What does not close: the internet answers, the Windows host answers ICMP, and the account can make a private network namespace with no privilege. Run `base doors` for this host's own list. `WSL-68` |
```
