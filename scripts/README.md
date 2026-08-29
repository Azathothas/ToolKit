# scripts

The probe, the checks, and the helpers a project inherits.

| directory | what is in it |
| --- | --- |
| [`doctor/`](doctor/) | ⭐ the environment probe. Two implementations, one schema. Every project keeps this. |
| [`common/`](common/) | the checks and the helpers. ⛔ Every CHECK has a POSIX sh implementation AND a PowerShell twin. |
| [`powershell-windows/`](powershell-windows/) | tools for a job that only exists on Windows. ⛔ Not a twin of anything. Each `.ps1` has a `.md` beside it that stands alone. |
| [`../LICENSES/`](../LICENSES/README.md) | the SPDX texts [`common/fill-license.sh`](common/) reads. ⛔ Not scripts, and four of them must never be edited. |

⚠ **`powershell-windows/` exists because a job in it has no POSIX form, not
because a script was easier to write in PowerShell.** The distinction is the
whole point of the directory. `bash-posix/` does not exist and shipping it
empty would be shipping a phantom: git does not track an empty directory, so a
fresh clone would not have what this table described.

---

## ⭐ Everything in `common/` has two implementations, and here is what that cost

⛔ **A POSIX sh check cannot be assumed to run on Windows.** This was the
template's original position and it was wrong. The reasoning was that `sh`
would be present because Git Bash ships with git, so one implementation was
enough. Measured on one Windows 11 machine, 2026-08-25, from a native
PowerShell session with Git Bash NOT on `PATH`:

| tool the checks need | native PowerShell resolves it to |
| --- | --- |
| `sed` | ⛔ nothing. Not installed. |
| `sort` | ⚠ PowerShell's own `Sort-Object` alias, not the coreutils binary |
| `awk`, `grep`, `tr`, `comm`, `xargs` | present here only because scoop and a coreutils package happen to be installed |

⚠ **The second row is the dangerous one.** A missing tool fails loudly and
somebody fixes it. An ALIASED one succeeds and returns a DIFFERENT ANSWER.
`Sort-Object` even accepts `-u`, which is what makes it convincing. Measured on
the same machine, same day, over the five values `b A a B a`:

| | result |
| --- | --- |
| `LC_ALL=C sort -u` | `A B a b` |
| `Sort-Object -u` | ⛔ `A b` |

⛔ **It dropped two of the four distinct values**, because it compares
case-insensitively and keeps whichever it saw first. A check that deduplicates
a file list that way does not crash and does not warn. It reports on a smaller
set than it was asked about, and reports success.

⭐ **What did NOT reproduce, and is worth writing down so nobody re-derives
it:** git and `gh` behaved identically from both shells on this machine. Same
`git.exe` 2.55.0.windows.3, same `credential.helper manager` from the same
system config, same authenticated `gh`. So the argument for twins here is the
TOOLCHAIN, not credential scoping. A machine that installs git differently per
shell would add a second reason; this one did not have it.

### ⛔ Wherever a twin exists, `check-twins.sh` covers it

That is not advice, it is the rule that keeps two implementations from becoming
two behaviours. [`common/check-twins.sh`](common/) runs BOTH halves of every
pair on one tree and compares the `--json` answer and the exit code.

⚠ **It compares ANSWERS on the tree it is run against, not the rules.** A scope
difference with nothing in the tree to exercise it is invisible: dropping `.py`
from one twin's extension list changed no number here, because this repository
has no `.py` file. Dropping `.md` was caught instantly. ⭐ Prove a scope rule
with a fixture, not by trusting the comparison to notice.

### The five things that do NOT have twins, and why

| | |
| --- | --- |
| [`common/set-record.mjs`](common/) | ⛔ **It does not need one**, and for the same reason as `write-file.mjs` below: it is node. ⚠ What it would cost to give it one is the thing to notice: a twin here means a second implementation of table arithmetic, which is a second place for that arithmetic to be wrong, in the one file whose whole job is that the arithmetic is right. |
| [`common/write-file.mjs`](common/) | ⛔ **It does not need one.** It is node, and node is the same program on every host: no `sed`, no `sort`, no shell built-ins, no aliases. The reason the sh checks needed twins does not apply to it. ⚠ What it needs instead is node itself, which is the one dependency anything under `scripts/` has, and the reason a project may decline this helper rather than inherit it. |
| [`common/check-twins.sh`](common/) | ⛔ **It cannot have one.** It works by running both halves of every pair, so it needs a POSIX shell to run the sh half no matter what language it is written in. A PowerShell twin would still require `sh`, which is the exact dependency a twin exists to remove. It is a maintainer's tool and it runs where both implementations do: this machine, and the CI job that has `pwsh` on an Ubuntu runner. |
| [`powershell-windows/wsl-ephemeral.ps1`](powershell-windows/) | ⛔ **No twin, and it must not get one.** It drives `wsl.exe`, which is a Windows feature. The POSIX "equivalent" would be a container or `systemd-nspawn`: a different tool solving a different problem, sharing no interface and no output. Calling those two a twin would put `check-twins.sh` in the position of comparing two unrelated programs, and the only way to make that pass is to compare nothing. |
| [`powershell-windows/wsl-ephemeral-launcher.ps1`](powershell-windows/) | ⛔ **No twin, for the same reason and one more.** It exists to make the file above runnable on Windows: it clears a Windows file attribute, and a POSIX half would have nothing to launch. |

⭐ **The question to ask is whether the JOB exists on the other platform, not
whether the language does.** `wsl-ephemeral` fails that test. Every check in
`common/` passes it, which is why every one of them has two halves.
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
reason it exists beside `check-docs.sh` rather than inside it. ⚠ Measured here
on 2026-08-29, before it was armed: **164 characters across 28 files**, every
one of them in a script's comment banner, with `check-docs.sh` reporting the
tree clean throughout.

⭐ **The density ceiling is 30 markers per 100 non-blank lines**, and it is a
constant rather than a flag: a ceiling anybody can raise from a command line is
a ceiling that gets raised instead of met. Three files here were over it.

⚠ **A specimen inside a code span or a fenced block is permitted in markdown.**
Without that, a page that bans a character cannot show a reader which one.

### `common/check-one-home.sh`

Does any sentence of twelve words or more appear in two documents.

⭐ [`../docs/conventions/prose.md`](../docs/conventions/prose.md) has always said
one fact lives in one document, and nothing checked it. ⚠ Measured here on
2026-08-29, before it was armed: **17 sentences with two homes**, seven of them
involving a skeleton this repository had copied from a template and never
filled in.

⛔ **The two entry-point routers are exempt from each other and only from each
other.** `AGENTS.md` and `docs/AGENTS.md` each state the absolutes in full on
purpose, because a session may be handed exactly one of them. A sentence shared
between a router and any other file is still refused.

⚠ **It compares sentences**, so a fact restated in different words passes here
and fails a review instead. That is the same split every other prose rule has.

### `common/check-twins.sh`

Do the two probe implementations still answer the same way. It runs both on
one machine and compares the schema, the section keys, and the host and repo
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

### `common/check-gate.sh`

⭐ **Run every local gate this host can run, in one command.** Part (a) of
[`../docs/methodology/gate.md`](../docs/methodology/gate.md) is a list, and a
list run by hand is run in the order somebody recalls it, missing whichever
entry was added last.

```bash
sh scripts/common/check-gate.sh --fast
```

⛔ **It is not a second set of rules.** Every line delegates to a check that
already exists and reads that check's own exit code. When it and
`.github/workflows/ci.yml` disagree about what runs, CI gates the push and this
one is the defect.

⚠ **A skipped check is not a passed check.** `shellcheck`, `jq`, `pwsh` and
PSScriptAnalyzer are not on every machine. A missing one is reported as `SKIP`,
counted separately, named in the summary and carried in `--json` as
`skipped`. The exit code is still 0, because "this host cannot run that one" is
not a failure of the tree.

⛔ **The analyzer and the parse are scored separately**, because they can have
different answers and `check-powershell` exits 0 either way. One verdict for
both is how a skipped analyzer reads as a passed check, which is what it did
here once.

⚠ **`--fast` skips `check-twins` and nothing else.** Measured on one Windows 11
machine, 2026-08-27: the full run took 208s and `check-twins` was 171s of it.
That is the right price before a push and the wrong one before each of eleven
commits.

⛔ **It runs `check-twins`, which runs it.** A recursion guard breaks the cycle;
without it the pair hung for ten minutes and left twenty stray shells holding
their own files open.

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
returned `ELOOP`. Green unit, complete config, installed emulators, and
cross-architecture execution had never once worked.

⛔ **It does not use `podman machine ssh`**, which is what the reporting issue
assumed. On Windows that command passes `-o UserKnownHostsFile=NUL` to its own
ssh, and under Git Bash `NUL` is a filename rather than the null device, so it
writes a 99-byte file called `NUL` into the directory you ran it from. ⭐ It is
also unnecessary: every WSL2 distro shares one kernel, so `wsl -d DISTRO` reads
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
git config, and if neither has one it refuses rather than guessing.

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

### `powershell-windows/wsl-ephemeral.ps1`

Create, use and destroy throwaway WSL2 distros, from an OCI image or a local
rootfs tarball.

⭐ **It exists because the default host here is Windows and agents constantly
need a Linux userspace.** Without it, an agent that needs one improvises, asks,
or gives up.

⛔ **Removal is constrained four ways and all four are load-bearing**: a fixed
name prefix; refusal to remove anything lacking it; an explicit protected list
covering the container runtimes; and directory deletion confined to one base
path. Destructive actions require `-Force` when non-interactive.

⚠ **Asking it to remove a protected distro by name does not remove it.** The
name is prefix-forced first, so `-Action Remove -Name podman-machine-default
-Force` targets `eph-podman-machine-default`, which does not exist. Verified on
a machine that had the real one registered; it survived.

⭐ **Two of its actions are read-only reports and neither creates a distro.**
`-Action Resources` says what WSL and the container engine are holding and
prints the cleanup commands without running one of them; `-Action HostAddress`
answers what a distro would reach this host at, which a caller previously had to
build a throwaway VM to find out.

### `powershell-windows/wsl-ephemeral-launcher.ps1`

Resolve the script above, verify it as far as the caller allows, make it
runnable on Windows, and run it with everything else forwarded unchanged.

⭐ **It prefers the copy beside it**, so from a clone it touches no network and
keeps no pin. ⛔ **With no sibling it refuses a moving ref by shape** and refuses
to guess a default, because a branch moves and a moved reference runs code
nobody reviewed.

⚠ **Every line it prints goes to stderr and it writes nothing to stdout.** A
wrapper that writes to the wrapped program's stdout corrupts it, and
`-Action HostAddress` puts one address there and nothing else.

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
