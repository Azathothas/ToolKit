# ToolKit

Scripts and tools used across many projects, kept in one place instead of being
copied into the next repository that needs them.

The operator works across many hosts, shells and environments. Every tool here
says which hosts it runs on, and fails with a message on the ones it does not.

**Licence:** 0BSD. Use it however you like.

**Nothing is published from here.** No images, no packages, no releases. This
repository is scripts and their documentation. The BSD container images some of
its history refers to are built by
[`pkgforge-dev/docker-bsd`](https://github.com/pkgforge-dev/docker-bsd).

---

## The tools

| path | what it is |
| --- | --- |
| [`scripts/powershell-windows/wsl-ephemeral.ps1`](scripts/powershell-windows/wsl-ephemeral.md) | create, use and destroy throwaway WSL2 distros on Windows, from an OCI image or a rootfs tarball. Reports what WSL and the container engine are holding, and what address a distro reaches the host at. |
| [`scripts/powershell-windows/wsl-ephemeral-launcher.ps1`](scripts/powershell-windows/wsl-ephemeral-launcher.md) | fetch that script, verify it, make it runnable on Windows, and run it |
| [`scripts/powershell-windows/wsl-ephemeral-selftest.ps1`](scripts/powershell-windows/wsl-ephemeral-selftest.md) | run that script's pure functions against a table of cases. No WSL, no engine, nothing created. |
| [`scripts/doctor/`](scripts/doctor/README.md) | one read-only pass reporting the host, the shell, the installed tools with versions, and the repository state |
| [`scripts/common/`](scripts/README.md) | the checks that hold this repository's gate, plus the helpers that write files, move the record, commit and fill a licence. Each check is an `sh` and PowerShell pair. |
| [`LICENSES/`](LICENSES/README.md) | the SPDX texts `scripts/common/fill-license.sh` reads |

Every tool has a `.md` beside it that stands alone. **Read the tool's own page,
not this one**, before using it.

## The tree

| path | what is in it |
| --- | --- |
| `AGENTS.md` | the entry point for an agent. It points at `docs/AGENTS.md` and says almost nothing else. |
| `README.md` | this page |
| `CHANGELOG.md` | what shipped, when, and where the evidence is |
| `TODO/` | the work: the record, the entry list, the entries themselves, and the standing rules |
| `docs/` | how this repository is worked on. The map is below. |
| `scripts/` | the tools and the checks |
| `LICENSES/` | licence texts, not code |
| `.github/workflows/` | CI. Three jobs on `ci.yml`, across ubuntu and windows, plus a weekly pass over open issues. |

## The documents

| file | answers |
| --- | --- |
| [`docs/AGENTS.md`](docs/AGENTS.md) | the router an agent reads in full: where it is, the absolutes, and which document each task needs |
| [`docs/consumers.md`](docs/consumers.md) | who fetches from this repository, what they pin, and what breaks them |
| [`docs/methodology/gate.md`](docs/methodology/gate.md) | what a unit of work passes before it is done |
| [`docs/methodology/reviews.md`](docs/methodology/reviews.md) | the three review lenses |
| [`docs/methodology/sessions.md`](docs/methodology/sessions.md) | what a session owes at its start and its end |
| [`docs/methodology/authoring.md`](docs/methodology/authoring.md) | how an idea becomes an approved unit of work |
| [`docs/methodology/work-todo.md`](docs/methodology/work-todo.md) | the work model: an index, a record, entries that close in place |
| [`docs/methodology/references.md`](docs/methodology/references.md) | how to study somebody else's project |
| [`docs/methodology/initialize.md`](docs/methodology/initialize.md) | how to start a project that does not exist yet |
| [`docs/conventions/prose.md`](docs/conventions/prose.md) | how documents are written here |
| [`docs/conventions/docs.md`](docs/conventions/docs.md) | which documents exist and what makes them trustworthy |
| [`docs/conventions/git.md`](docs/conventions/git.md) | commit identity, and what may reach a remote |
| [`docs/conventions/code.md`](docs/conventions/code.md) | language-agnostic construction rules |
| [`docs/conventions/forbidden-patterns.md`](docs/conventions/forbidden-patterns.md) | mistakes that shipped, each with what it caused |
| [`docs/conventions/shell.md`](docs/conventions/shell.md) | quoting, exit codes, streams, line endings, and the platform traps |
| [`docs/security/secrets.md`](docs/security/secrets.md) | what never enters the tree |
| [`docs/security/remote-ops.md`](docs/security/remote-ops.md) | the tiers governing action on anything outside this machine |
| [`docs/public/README.md`](docs/public/README.md) | what changes because this repository is public |
| [`docs/reference-sweeps/findings.md`](docs/reference-sweeps/findings.md) | what external repositories were read, and what was true in them |
| [`docs/reference-sweeps/usable.md`](docs/reference-sweeps/usable.md) | which of those findings this repository can use |
| [`docs/HISTORY/README.md`](docs/HISTORY/README.md) | superseded wording and the story of fixes that have shipped. Nothing there is read to do work. |

---

## Using a tool from another project

Fetch it by URL. Nothing here assumes it is being run from a clone.

**Resolve a commit and use that.** A branch moves, and a moved reference runs
code nobody reviewed:

```bash
gh api repos/Azathothas/ToolKit/commits/main --jq .sha
```

**Download to a file, then run the file.** Piping a download into a shell
executes the prefix of a truncated transfer and leaves nothing to inspect.

For `wsl-ephemeral.ps1` the launcher does all of that, verifies a digest, and
forwards the rest of your arguments unchanged:

```powershell
pwsh -NoProfile -File wsl-ephemeral-launcher.ps1 -LauncherRef THE_COMMIT_SHA -Action List
```

[`docs/consumers.md`](docs/consumers.md) is the register of who fetches what,
and what a rename here breaks out there.

---

## Working on this repository

Read [`AGENTS.md`](AGENTS.md), which points at
[`docs/AGENTS.md`](docs/AGENTS.md). [`TODO/PROGRESS.md`](TODO/PROGRESS.md)
carries the current state and the work order, and
[`TODO/RULES.md`](TODO/RULES.md) the part of it that does not change.

Run the probe first, on any machine:

```bash
sh scripts/doctor/doctor.sh
```

```bash
pwsh -NoProfile -File scripts/doctor/doctor.ps1
```

Then the gate, which is every local check in one command:

```bash
sh scripts/common/check-gate.sh --fast
```

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1 -Fast
```

[`docs/methodology/gate.md`](docs/methodology/gate.md) is the rule the gate
implements, and CI runs the same checks on two hosts.
