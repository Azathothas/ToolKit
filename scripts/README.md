# scripts

The probe, the checks, and the helpers a project inherits.

| directory | what is in it |
| --- | --- |
| [`doctor/`](doctor/) | ⭐ the environment probe. Two implementations, one schema. Every project keeps this. |
| [`common/`](common/) | the checks and the helpers. ⛔ Every CHECK has a POSIX sh implementation AND a PowerShell twin. |
| [`powershell-windows/`](powershell-windows/) | tools for a job that only exists on Windows. ⛔ Not a twin of anything. |

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

### The three things that do NOT have twins, and why

| | |
| --- | --- |
| [`common/write-file.mjs`](common/) | ⛔ **It does not need one.** It is node, and node is the same program on every host: no `sed`, no `sort`, no shell built-ins, no aliases. The reason the sh checks needed twins does not apply to it. ⚠ What it needs instead is node itself, which is the one dependency anything under `scripts/` has, and the reason a project may decline this helper rather than inherit it. |
| [`common/check-twins.sh`](common/) | ⛔ **It cannot have one.** It works by running both halves of every pair, so it needs a POSIX shell to run the sh half no matter what language it is written in. A PowerShell twin would still require `sh`, which is the exact dependency a twin exists to remove. It is a maintainer's tool and it runs where both implementations do: this machine, and the CI job that has `pwsh` on an Ubuntu runner. |
| [`powershell-windows/wsl-ephemeral.ps1`](powershell-windows/) | ⛔ **No twin, and it must not get one.** It drives `wsl.exe`, which is a Windows feature. The POSIX "equivalent" would be a container or `systemd-nspawn`: a different tool solving a different problem, sharing no interface and no output. Calling those two a twin would put `check-twins.sh` in the position of comparing two unrelated programs, and the only way to make that pass is to compare nothing. |

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

What host is this, what is installed, and what is this repo. Read
[`doctor/README.md`](doctor/README.md) for the schema and the measured
runtimes.

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
checks. These three are held to the header rule and the exit-code rule, and
deliberately not to "read only": writing is what they are for.

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
