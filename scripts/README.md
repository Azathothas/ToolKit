# scripts

The probe, the checks, and the helpers a project inherits.

| directory | what is in it |
| --- | --- |
| [`doctor/`](doctor/) | ⭐ the environment probe. Two implementations, one schema. Every project keeps this. |
| [`common/`](common/) | the checks and the helpers, and since 2026-09-12 one configuration file a helper installs. ⛔ Every CHECK has a POSIX sh implementation AND a PowerShell twin; a helper and a data file have neither and the twins table below says why. |
| [`../tools/windows/wsl-toolkit/`](../tools/windows/wsl-toolkit/README.md) | the native Windows product, in Go. ⛔ Not a script, so nothing in this file's check contract applies to it; [`common/check-go.sh`](common/) is what the gate runs over it. |
| [`../LICENSES/`](../LICENSES/README.md) | the SPDX texts [`common/fill-license.sh`](common/) reads. ⛔ Not scripts, and four of them must never be edited. |

---

## ⭐ The rules are ONE program, and it is not shell

⭐ **[`../tools/check/`](../tools/check/) holds every rule this repository
enforces over its own tree.** One binary, one tree walk, native on either host.
`common/check-*.sh` and their `.ps1` twins are wrappers around one named check of
it.

⛔ **A POSIX sh check cannot be assumed to run on Windows**, which is why the
rules are not shell. Measured on one Windows 11 machine, 2026-08-25, from a
native PowerShell session with Git Bash NOT on `PATH`:

| tool a shell check needs | native PowerShell resolves it to |
| --- | --- |
| `sed` | ⛔ nothing. Not installed. |
| `sort` | ⚠ PowerShell's own `Sort-Object` alias, not the coreutils binary |
| `awk`, `grep`, `tr`, `comm`, `xargs` | present only because scoop and a coreutils package happen to be installed |

⚠ **The second row is the dangerous one, and this is the measurement to keep.** A
missing tool fails loudly and somebody fixes it. An ALIASED one succeeds and
returns a different answer. Over the five values `b A a B a`:

| | result |
| --- | --- |
| `LC_ALL=C sort -u` | `A B a b` |
| `Sort-Object -u` | ⛔ `A b` |

⛔ **It drops two of the four distinct values**, comparing case-insensitively and
keeping whichever it saw first. A check that deduplicates a file list that way
does not crash, does not warn, reports on a smaller set than it was asked about,
and reports success.

[`../docs/HISTORY/scripts.md`](../docs/HISTORY/scripts.md) carries how the tree
got here, including what the twin gate cost before the port.

### ⛔ Wherever a twin still exists, `check-twins.sh` covers it

[`common/check-twins.sh`](common/) runs BOTH halves of every remaining pair on
one tree and compares the `--json` answer and the exit code.

⭐ **ONE PAIR IS AN IMPLEMENTATION, AND THE REST ARE WRAPPERS.** The probe under
[`doctor/`](doctor/) is genuinely written twice; `git-sync`, `check-binfmt`,
`check-remote-items`, `deslop` and `fill-license` are entry points onto one Go
subcommand each, under [`../tools/repo/`](../tools/repo/). What a wrapper row
proves is narrower: not that two implementations of a rule agree, but that two
entry points FORWARD the same thing. ⚠ That is a real class. A `.ps1` wrapper
once passed `-Json` straight through to a binary that takes `--json`, and this
check is what reported it.

⛔ **THE PROBE CANNOT BECOME A WRAPPER.** It RUNS BEFORE YOU KNOW WHAT IS
INSTALLED, which is its whole job, and "is there a Go toolchain" is one of the
questions it answers. A wrapper that had to build one first could not report that
it was missing.

⚠ **It is not part of the gate.** It costs minutes and catches drift that only
arrives when somebody edits one of those halves; CI runs it on every push.

⚠ **It compares ANSWERS on the tree it is run against, not the rules.** A scope
difference with nothing in the tree to exercise it is invisible: dropping `.py`
from one twin's extension list changes no number here, because this repository
has no `.py` file. ⭐ Prove a scope rule with a fixture, not by trusting the
comparison to notice.

### The things that do NOT have twins, and why

⭐ **The question is whether the JOB exists on the other platform, not whether
the language does.**

| | |
| --- | --- |
| [`../tools/check/`](../tools/check/) | ⛔ **It cannot have one and must not.** It IS the answer to why twins existed: one implementation that runs natively on both hosts. A second one would recreate the drift it removed. |
| [`../tools/repo/`](../tools/repo/) | ⛔ **The same answer, for the tools that are not rules.** `deslop`, `license`, `binfmt`, `remote-items`, `git-sync`, `mutate` and `release` live here as subcommands; the scripts named after the first five are wrappers. ⭐ **`mutate` and `release` have no wrapper and need none**: `repo.sh mutate` and `repo.sh release` reach them. ⚠ It is deliberately NOT `tools/check`: that binary holds what this repository enforces over its own tree, and `check-gate` runs all of it. A commit path and a licence writer are not rules. |
| [`common/set-record.mjs`](common/) and [`common/write-file.mjs`](common/) | ⛔ **Neither needs one.** They are node, and node is the same program on every host: no `sed`, no `sort`, no shell built-ins, no aliases. ⚠ A twin for `set-record` would be a second implementation of table arithmetic, in the one file whose whole job is that the arithmetic is right. ⚠ What they need instead is node, which is the one dependency anything under `scripts/` has. |
| [`common/check-twins.sh`](common/) | ⛔ **It cannot have one.** It works by running both halves of every pair, so it needs a POSIX shell no matter what language it is written in. |
| [`common/bootstrap.sh`](common/), [`common/tmux.conf`](common/) and [`common/shell-profile.sh`](common/) | ⛔ **No twin.** The job is to drive a Unix package manager inside a Unix userland, and to configure a Unix shell. A PowerShell half would have nothing to install and nowhere to install it, and none of the three is a check. |
| [`../tools/windows/wsl-toolkit/`](../tools/windows/wsl-toolkit/README.md) | ⛔ **Not a check and not a script.** It is a Go module, and the gate's `go` check is the check OVER it. Its release refusals are `repo release`, under [`../tools/repo/`](../tools/repo/). |

## The check contract

⛔ **Every check in this repository, and every check a project inherits from it,
satisfies all five.** A script that does not is not a check; it is a script
somebody has to remember to interpret.

1. **A header comment saying what defect it exists to catch.** Not what it
   does: what goes wrong without it. ⭐ This is the field that decides whether a
   future session keeps it, deletes it, or writes a second one that overlaps.
2. **Exit 0 pass, 1 fail, 2 could not run.** ⚠ Those are three different facts.
   "The check failed" and "the check could not run" mean opposite things about
   whether you can ship, and a script that returns 1 for both hides the
   difference.
3. **A json switch**, so a gate runner can consume it.
4. **No dependence on the directory it is run from.** Resolve paths from the
   script's own location.
5. **Read only, unless a fix flag is passed.** A check that repairs things by
   default is a check nobody can use to find out whether something is wrong.

⚠ **A check that measures an open defect must not fail the build for that
defect alone.** Record the count and judge it only past a stated ceiling.
⭐ The other half of that rule is that the exemption comes off when the item
closes. An exemption nobody removes is a check that stopped checking.

---

## ⛔ An exit code is read from the process that produced it, unpiped

```bash
sh scripts/common/check-no-secrets.sh
```

Not `check | grep`, not `check | Select-String`, not `check | tee`. A pipeline
reports the **last** command's status, so a check that failed reads as green.

⚠ This has caught the author of this sentence, in the session that wrote it.

---

## What is here

### `doctor/`

The environment probe. Read [`doctor/README.md`](doctor/README.md) for what it
answers, the schema, and the measured runtimes.

⭐ It is a **probe, not a gate**: a missing tool is data, so it exits 0 whether
or not anything is missing. Nothing here belongs in a gate chain.

### `common/check-no-secrets.sh`

Does any file in this tree carry something that must not be published.

⚠ **Tracked plus untracked-but-not-ignored, not tracked alone.** A file that
has never been staged is exactly when a new file is likeliest to carry a
credential, and exactly what the next `git add -A` would take.

⛔ **It finds the shapes it knows, and a green run is not a clearance.** It
cannot find a password that looks like a word or a page of correct-looking
examples that happens to describe a real system.

`--public` adds the rules that only matter for a repository that will be
public: emails, absolute home paths, long hex identifiers. In a private project
those are legitimate content, which is why they are not the default.

### `common/check-placeholders.sh`

Did a template placeholder survive into a real file. Run at the end of a
bootstrap, and as a gate afterwards.

### `common/check-docs.sh`

Do the documents still resolve, and are they written the way this
repository writes documents. Relative links, fenced shell blocks that
parse, shell-unsafe placeholders, control bytes, em dashes, and the three
defined markers.

⚠ The template directories are exempt from the **link** check only: their
links are written relative to where the file will live in a project. The
prose rules still apply to them.

### `common/check-markers.sh`

Are the only characters outside ASCII in this tree the five this repository
defines, and does any one page carry so many of them that they have stopped
meaning anything.

⛔ **It covers every tracked text file, not markdown alone**, which is the whole
reason it exists beside `check-docs.sh` rather than inside it: that one reads
markdown, and every finding the first armed run produced was in a `.ps1` or a
`.sh`. The count is in
[`../docs/HISTORY/scripts.md`](../docs/HISTORY/scripts.md).

⭐ **The density ceiling is 30 markers per 100 non-blank lines**, and it is a
constant rather than a flag: a ceiling anybody can raise from a command line is
a ceiling that gets raised instead of met. Three files here were over it.

⚠ **A specimen inside a code span or a fenced block is permitted in markdown.**
Without that, a page that bans a character cannot show a reader which one.

### `common/check-one-home.sh`

Does any sentence of twelve words or more appear in two documents.

⭐ [`../docs/conventions/prose.md`](../docs/conventions/prose.md) owns the rule
that one fact lives in one document; this is what enforces it. What the first
armed run found is in
[`../docs/HISTORY/scripts.md`](../docs/HISTORY/scripts.md).

⛔ **It carries no router exemption, and it used to.** `AGENTS.md` and
`docs/AGENTS.md` each stated the absolutes in full, so the pair was exempt from
each other by name; the root file was deleted on 2026-08-30 and the exemption
went with it. ⭐ An exemption for a file that no longer exists grants itself to
whatever lands at that path next, so it is deleted rather than emptied.

⚠ **It compares sentences**, so a fact restated in different words passes here
and fails a review instead. That is the same split every other prose rule has.

### `common/check-twins.sh`

Do the two probe implementations still answer the same way. It runs both on
one machine and compares the schema, the section keys, and the host and repository
facts that describe that machine.

⚠ It compares the SHAPE and the FACTS, not the tool-by-tool verdicts. Each
twin reports what its own host can reach, and on a Windows machine with msys
installed `bash`, `tar` and `zsh` genuinely differ between them.

⭐ **It also compares the CLI surface, which the schema cannot show.** Every
comparison above reads what the probes OUTPUT; none of them reads what the
probes ACCEPT. `doctor.sh --text` exited 0 while `doctor.ps1 -Text` exited 1
with a parameter-binding error, and every other comparison in the file passed
the whole time that was true.

### `common/check-remote-items.sh`

What is open against the repository, and does it say anything that survives
being checked. For every pinned action a pull request proposes: the commit
exists in the repository the ref names, the tag comment resolves to that same
commit, and ⭐ the runtime it DECLARES is not one the platform has deprecated.

⛔ **It never merges, closes, comments or approves.** It reports, and deciding
is the operator's.

⚠ It cannot tell you whether a change is a good idea. It checks the facts an
item asserts about the world; whether you want the change is a reading.

⭐ It exists because this repository was pinned to an action targeting a Node
runtime GitHub had deprecated, and the warning sat in a log nobody read. A
dependency bot is right almost every time, and that is precisely what makes
the wrong one expensive.

### `common/check-control-bytes.sh`

Is there a literal control byte in any text file in the tree.

⭐ **It covers every text file, not only markdown.** The rule used to live in
`check-docs.sh` and scanned `.md` alone, which left every `.ts`, `.py`, `.rs`,
`.sh` and `.yml` unchecked for the one defect that makes a file invisible to
both review tools at once: `grep` calls it binary and skips it, and `git diff`
prints "Binary files differ" so a code review shows no diff at all.

⚠ The runtime value is identical either way, so only reviewability is ever at
stake. That is exactly why it survives unnoticed.

### `common/check-go.sh`

Does the `wsl-toolkit` executable still build, vet and pass its own tests.

⭐ **Four steps, one exit code**, and the failing one is named: gofmt, `go vet`,
`go build`, `go test`. ⛔ It is one check rather than four inlined commands in two
gate runners, so both halves run the same thing and `check-twins.sh` compares
their answers.

⚠ **No `go` on PATH is exit 2, not exit 0.** A machine without a toolchain has
not proved this tree builds.

⛔ **The module is not at the repository root** and this resolves it. `go build
./...` from the root finds no module and exits 1, which reads as a broken build
and is a wrong directory.

### `common/check-gate.sh`

⭐ **Run every local gate this host can run, in one command.** Part (a) of
[`../docs/methodology/gate.md`](../docs/methodology/gate.md) is a list, and a
list run by hand is run in the order somebody recalls it, missing whichever
entry was added last.

```bash
sh scripts/common/check-gate.sh
```

⛔ **It is not a second set of rules.** It builds
[`../tools/check/`](../tools/check/) and runs it, so there is no list of checks
here to fall out of step with the list there. When it and
`.github/workflows/ci.yml` disagree about what runs, CI gates the push and this
one is the defect.

⚠ **A skipped check is not a passed check.** `shellcheck`, `pwsh` and
PSScriptAnalyzer are not on every machine. A missing one is reported with what
was missing rather than counted as agreement, and the exit code is still 0,
because "this host cannot run that one" is not a failure of the tree.

⛔ **`--fast` IS GONE, AND A CALLER PASSING IT IS TOLD SO.** It skipped
`check-twins` and nothing else. The rules are one program now, so there are no
halves to compare and nothing worth skipping. Silently accepting the flag and
doing something different is how a caller comes to believe they ran less than
they did.

### `common/check-powershell.ps1`

Does every tracked `.ps1` parse, and is PSScriptAnalyzer clean over `scripts/`
at Error and Warning.

⚠ **The analyzer is a module, not part of PowerShell.** Without it this reports
`SKIPPED` and exits 0. ⛔ **It never installs it**: a check that installs
software changes the machine it is measuring, and this one runs before a commit.
CI installs it explicitly and then asserts it was not skipped.

⭐ Its last line is a fixed `analyzer=clean|skipped|issues:N`, which is what
`check-gate` reads. ⛔ Parse that, never the prose above it.

### `common/check-binfmt.sh`

Are `binfmt_misc` handlers actually registered in the kernel containers run
against, and can that directory be read at all.

⭐ **It reads the kernel, not a unit's exit code**, because the unit is the thing
that lied: `systemd-binfmt.service` reported `status=0/SUCCESS` having
registered zero handlers, with an autofs stacked on the mount so every read
returned `ELOOP`. Green unit, complete configuration, installed emulators, and
cross-architecture execution had never once worked.

⛔ **It does not use `podman machine ssh`**, which is what the reporting issue
assumed. On Windows that command passes `-o UserKnownHostsFile=NUL` to its own
ssh, and under Git Bash `NUL` is a filename rather than the null device, so it
writes a 99-byte file called `NUL` into the directory you ran it from. ⭐ It is
also unnecessary: every WSL2 distribution shares one kernel, so `wsl -d DISTRO` reads
the same handlers with nothing written anywhere.

⚠ **`--require N` is what turns it from a report into an assertion.** Without
it, zero handlers is reported and exits 0, because a machine that never wanted
cross-architecture execution is not broken.

### `common/check-changelog.sh`

Does `CHANGELOG.md` still obey the four rules a machine can hold: newest first,
every heading dated, every entry naming its record, every entry saying whether
it deployed.

⭐ It exists because [`../docs/conventions/docs.md`](../docs/conventions/docs.md)
stated those four rules, said in as many words that each was mechanical enough
to check, and nothing checked them.

⚠ **No `CHANGELOG.md` is exit 2, not exit 0.** A project without one has
neither broken these rules nor satisfied them, and reporting green over an
absent file is how a check quietly stops applying.

---

## The helpers, which are not checks

⚠ **A helper writes; a check reports.** The five-point contract above is for
checks. The ones below are held to the header rule and the exit-code rule, and
deliberately not to "read only": writing is what they are for.

⚠ **`deslop` and `fill-license` are documented above, among the checks, because
that is where a reader looking for them will be.** Neither is a check by this
contract: `deslop` writes under `--apply` and `fill-license` writes a licence.
⛔ Both refuse rather than writing when they are unsure, which is the property
that matters more than which list they appear in.

### `common/write-file.mjs`

Write, append to, or patch a file without the shell touching the payload.

⭐ **The payload channel is base64**, which is the one encoding no shell
interprets: not bash, not PowerShell, not `cmd`. A quote, a backtick, a dollar
sign, a percent and an emoji all survive it unchanged.

⛔ **A substitution whose match count differs from the number you declared is
REFUSED and the file is left untouched.** A silent no-op reporting success is
the failure this exists to remove. It fired twice while this template was
being maintained, once on a CRLF file whose LF search string matched nothing.

⚠ It needs `node`. That is the only thing under `scripts/` that does, and it
is the reason this is a helper a project may decline rather than a check every
project inherits. [`../docs/conventions/shell.md`](../docs/conventions/shell.md)
section 1 is the reasoning, measured.

### `common/set-record.mjs`

Move an entry's status and re-derive every count from the rows.
[`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md) calls the
counts the model's one mechanical hazard and says to automate **both** halves;
`check-record` is the reader and this is the writer it names.

```bash
node scripts/common/set-record.mjs status WSL-06 done
```

Closing one entry moves seven numbers: the index count line, the priority
table's four figures for that priority, that table's **all** row, and the
record's own count line.

⛔ **It does not run `check-record` and report green.** A writer that grades its
own work is one bug away from hiding the bug, and the reader has to assert
independently. It prints the command; `check-gate` runs it.

⚠ **Since 2026-09-17, closing an entry can also mean editing the work order by
hand.** `check-record`'s rule 7 refuses a work order item that is not marked
Closed when every entry it names is done, and this writer moves the numbers
alone - it does not touch prose and is not going to start. So the gate is what
tells you the order is now behind the work, which is the whole point of the
rule.

⚠ **It needs `node`, and has no PowerShell twin for the same reason
`write-file.mjs` has none.** A second implementation of table arithmetic is a
second place for that arithmetic to be wrong.

### `common/git-sync.sh`

Commit and push with the rules in
[`../docs/conventions/git.md`](../docs/conventions/git.md) enforced rather than
remembered.

⭐ **It arrived as a 674-line PowerShell script and now exists as both**: a
POSIX sh implementation so every Linux and macOS project can run it, and a
PowerShell twin because on Windows the sh one needs a POSIX layer that a native
session may not have. ⚠ On Windows prefer the `.ps1`: it drives the native
`git.exe` rather than one inside an msys layer.

⛔ **An AI-attribution line is refused, never stripped.** Rewriting somebody's
commit message is worse than declining it: the author never learns the rule.

⛔ **A CI-skip marker is refused unless the flag was passed.** A message that
merely mentions one skips CI, because the platform does not read the sentence
around it.

⚠ **It knows nothing about who you are.** Identity comes from the flags or from
`git config`, and if neither has one it refuses rather than guessing.

### `common/deslop.sh`

Which files in a tree address a reader as an agent.

⭐ **An inventory, not a gate**, and it exits 0 whether it finds twenty such
files or none. ⛔ **It is aimed at ANOTHER tree.** Run with `--apply` here it
would remove this repository's own router and methodology, which are content it
wants rather than content it regrets.

⛔ **It never touches history, never deletes without `--apply`, and `--apply`
refuses on a dirty tree.** ⭐ It reads the state back after removing and reports
what is actually gone: the version this repository inherited printed the number
it had planned to remove, which is a delete reporting success it never checked.

### `common/fill-license.sh`

Write `LICENSE` from one of the texts in [`../LICENSES/`](../LICENSES/), with
the holder filled in.

⛔ **Four of the twelve are refused rather than filled**, and that refusal is
the feature. The GPL, AGPL and LGPL texts open with the Free Software
Foundation's copyright on the licence document itself; SPDX's ISC text is a
licence instance carrying Internet Systems Consortium's own notice. Rewriting
any of those attributes your software to somebody else.

⚠ **Compared on its OUTPUT by `check-twins.sh`, not on a status line**, because
a corrupted licence exits 0. The over-replacement that produced that rule wrote
a valid-looking file with a mangled warranty clause.

### `common/bootstrap.sh`

Bring a Unix userland up to a named set of tools, and report what it actually
resolved.

⭐ **It is here rather than under one tool's `examples/` directory, and that move
is the point of it.** It began as `tools/windows/wsl-toolkit/examples/common/`,
where a caller had to know that a particular provider example existed before they
could find a general-purpose bootstrap. Nothing in it is about `wsl-toolkit`: it
runs in a container, in CI, on a laptop, in a WSL guest, and in a BSD guest.

⭐ **Twelve package managers.** apk, apt, dnf, emerge, pacman, tdnf, xbps, yum and
zypper on Linux; `pkg` on FreeBSD and DragonFly, `pkgin` on NetBSD, `pkg_add` on
OpenBSD. `nix` is the user-level provider when an account has neither root nor
passwordless sudo. ⛔ **It never installs nix**, because installing it means piping
a remote script into a shell.

⭐ **An installed Nix is found and used whole.** A Nix outside the shell's `PATH`, in
the account's profile or the daemon's default profile, is put on `PATH` without
sourcing any profile script. A new profile installs with `nix profile install` and
flakes, and one `nix-env` made stays on its channel. The account's own `nix.conf`
gains `experimental-features = nix-command flakes` when it names no experimental
features. Every Nix command the run starts gets the four `NIXPKGS_ALLOW_*`
variables, a source build when a substitute fails, bounded network waits, and
`GITHUB_TOKEN` as `access-tokens` through `NIX_CONFIG` when it is set, never
printed or written. A caller's own `NIX_CONFIG` comes last and wins. The table's
`nix` key names each nixpkgs attribute.

⭐ **The package table is the feature, and adding a distribution changes no
code.** One row per LOGICAL name, a default package name, then only the places
that spell it differently. ⚠ An override key may be a package manager OR
`os:<ID>`, and `os:` wins, because Alpine, Chimera and Wolfi all use `apk` and
disagree about half the names.

⚠ **The table sits between two marker lines because a generated copy of that
block lives in `wsl-toolkit`'s provisioner package**, and the base provisioner
resolves its `developer` names through it. The gate's `package-table` check refuses
the two disagreeing, and `sh scripts/common/check.sh package-table --fix` rewrites
the copy. [`../TODO/RULES.md`](../TODO/RULES.md) section 4 carries the rule.

⛔ **IT DEPENDS ON THE SHELL AND THE PACKAGE MANAGER AND ALMOST NOTHING ELSE.**
Not `awk`, `tr`, `find`, `grep`, `sed`, `install` or `dirname`. Measured over the
image catalogue: Photon has neither `awk` nor `tr`, openSUSE has neither `awk` nor
`find`, and Void and Rocky 8 have no `find`. ⚠ **A bootstrap whose job is to
install the missing tools cannot require them to be there already**, and the first
draft of this file did, and reported that it had no table row for ten names on
Photon as a result.

⚠ **NO DIGEST IS WRITTEN INTO IT, and that is weaker on purpose.** Where it
verifies a download the expected value is read from the same registry as the bytes
at run time, which proves transport rather than authorship. The version it
replaced pinned CodeGraph 1.5.0 and three SHA-512 values, and the registry was on
1.6.0 the following day. ⭐ `--expect-integrity` and `--expect-sha256` put the
stronger check back for a caller who holds a value; the run prints every version
and digest it resolved so that caller can. ⚠ CodeGraph is fetched with `npm pack
--pack-destination`, which needs npm 7.18.0 or later, and an older npm is named as
that rather than tried. ⚠ It publishes a Linux package only, so a BSD is named as
that before any fetch rather than offered the linux-x64 one.

⛔ **The report is read from the machine.** A name that was asked for, whose
install command exited 0, and that is not on `PATH` afterwards is a failure and
exit 1. ⚠ **One transaction first, then one package at a time**: a bulk install
that fails installs nothing and names nothing, and six of twelve images once
failed over one absent package each while reporting all eighteen as missing.

Exit codes: 0 done, 1 something asked for could not be installed, 2 could not run.

```sh
sh scripts/common/bootstrap.sh --toolset agent --dry-run
```

### `common/tmux.conf`

A long-running session that survives its terminal closing and cannot be quit by
one wrong key. `bootstrap.sh` installs it as `~/.tmux.conf` when it finds it
beside itself.

⭐ **Both `M-g` and `C-b` are the prefix**, so whichever one a reader remembers
works. ⭐ **The three keys that matter are on the status line**, which is the whole
answer to not wanting to learn a multiplexer.

⛔ **`x` and `&` are unbound.** Both are one key away from keys used all day and
both end something that took hours; the deliberate kill is a capital letter and
still asks. ⚠ `remain-on-exit` keeps a pane whose command has ended, so `prefix R`
respawns one.

⚠ **Every option in it is one tmux 3.0 accepts, and window and server options are
spelled `setw -g` and `set -s`.** A bare `set -g` over a window option is an error
on an older tmux, and the configuration then loads PARTIALLY: tmux starts, that
line did nothing, and the session looks configured.

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

⛔ **tmux is the fallback, not the agents' multiplexer.** herdr's agent detection
does not inspect a tmux session launched inside one of its panes: it sees `tmux`
as the pane process and the agent behind it becomes invisible. A shell framework
that auto-enters tmux therefore breaks agent state entirely.
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md) carries
the sweep that read it.

### `common/shell-profile.sh`

A login-shell profile that does three things, and every one of them only for an
**interactive** shell. `bootstrap.sh` installs it under the prefix and adds one line
that reads it to `~/.profile`, and to `~/.bash_profile` or `~/.bash_login` where the
account already has one.

| what | when |
| --- | --- |
| a shell whose working directory is a Windows drive WSL mounted moves to the account's home, and says so once | the directory is one level under the automount root |
| duplicate entries come out of the `PATH` it inherited | the list has a repeat, or an empty element |
| shell history gets a home that survives | the shell itself gave it none |

⛔ **Interactive, not login, and the difference is the whole guard.** `wsl-toolkit
distro run -c` and `matrix -c` send every command to a LOGIN shell, so a profile
that changed one would silently change what every command any caller runs sees.
`$-` carries `i` only for a shell a person is typing at.

⭐ **`WSL_TOOLKIT_NO_PROFILE=1` turns all of it off**, which is the one named way.

⛔ **NOTHING HERE ADDS A DIRECTORY TO `PATH`.** `bootstrap.sh` writes the line for
the prefix and one fact has one home. This file only removes a repeat of something
already there, keeping the first occurrence in its place so which binary wins does
not change. ⚠ **An empty element is dropped**, because `PATH=/bin:` searches the
current directory for every command typed; that is the one change here that alters
what a command resolves to, and it is deliberate.

⚠ **History is four plain variables and nothing else.** `HISTFILE`, `HISTSIZE`,
`HISTFILESIZE` and `HISTCONTROL`, each set only where nothing set it, so the
account's own choice always wins. `shopt -s histappend` is bash's and is not POSIX,
so it is not here. bash already has a history file and keeps it; the sh, dash and ash
family had none and gain one.

⛔ **A granted directory is not a Windows drive for this purpose.** The guard
matches one level under the automount root - `/mnt/c`, and the root itself is read
from `/etc/wsl.conf` rather than assumed - so `/workspaces/project` and `/mnt/wsl`
are left alone. A guard that matched the whole of `/mnt` would move a shell out of
the directory it was granted.

⭐ **`wsl-toolkit base shell --here` marks its own shell** with `WSL_TOOLKIT_HERE`,
named in `WSLENV`, and the profile leaves a marked shell where it started. From
inside the guest a shell that asked for the Windows directory and one that merely
inherited it are otherwise identical.

⛔ **It fetches nothing and has no aliases and no prompt.** A profile that updates
itself puts a network fetch in front of every shell start and makes its own content
untrackable; an alias for a tool the toolset did not install is an error on every
shell start.

⚠ **The `.sh` extension is the constraint, not a label.** CI runs `shellcheck -s
sh` over every tracked `*.sh`, which is exactly what this file needs: no arrays, no
`local`, no `[[`.

| measured on 2026-09-17, `matrix --images all`, each image driven twice - without the profile, then with it | result |
| --- | --- |
| images, and shells found on them | 13 and **28** |
| shells that add a byte to stderr, login or interactive | ⭐ **0**. ⚠ The assertion is the DELTA, because Photon's own `dircolors.sh` writes 66 bytes either way |
| shells that de-duplicate a planted repeat for an interactive shell | **28 of 28** |
| shells that leave a non-interactive shell's `PATH` exactly as it was | **28 of 28** |
| shells that gained a history home | **13**, every `sh`, `dash` and `ash`; the other 15 kept their own |
| shells that honour `WSL_TOOLKIT_NO_PROFILE` | **28 of 28** |

⛔ **`-ic` is not the flag to drive this with.** An interactive NON-login shell reads
`~/.bashrc` or `$ENV` and never `~/.profile`, so a driver using it reported six
images as failing over code that had not run. ⛔ **And a planted `PATH` cannot be
read back through `/etc/profile`**, which every image but arch replaces `PATH` in.
---

## Adding one

1. **Name the defect first.** If you cannot say what goes wrong without this
   script, it is not a check.
2. **Follow the contract**, all five points.
3. ⭐ **Mutation-prove it.** Plant the defect it exists to catch, run it, and
   read the exit code unpiped. **A guard that has never been seen to refuse is
   a guard nobody knows works.**

   This is not optional advice. While building this repository, a licence
   filler reported success over a licence whose warranty clause it had
   corrupted, because its check only ever asked whether a placeholder
   *survived*, never whether the substitution had reached too far. The mutation
   test is what found it.

4. **Wire it into the gate**, if it can fail.
5. **Document it**: here, and in the project's own tool table.

⚠ **A script that lives only in a transcript is re-derived every session.**
When a scratch helper does something a future session will also need, promote
it: write it into `scripts/` with the contract above, document it where agents
are told to look, and wire it into the gate if it is a check rather than a
one-off.
