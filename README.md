# ToolKit

Scripts and tools that are used across many projects, kept in one place instead
of being copy-pasted into the next repository that needs them.

The operator works across many hosts, shells and environments. Every tool here
says which hosts it runs on, and fails with a message on the ones it does not.

**Licence:** 0BSD. Use it however you like.

⛔ **Nothing is published from here.** No images, no packages. This repository
is scripts and their documentation. The BSD OCI images referred to below are
built and published by `pkgforge-dev/docker-bsd`.

---

## What is here

| path | what it is |
| --- | --- |
| [`scripts/powershell-windows/wsl-ephemeral.ps1`](scripts/powershell-windows/wsl-ephemeral.ps1) | create, use and destroy throwaway WSL2 distros on Windows. [Docs.](scripts/powershell-windows/wsl-ephemeral.md) |
| [`scripts/doctor/`](scripts/doctor/README.md) | one read-only pass reporting the host, the shell, the installed tools and their versions, and the repository state |
| [`scripts/common/`](scripts/README.md) | the checks that hold this repository's gate, each as an sh and PowerShell pair |

Every tool has a `.md` beside it that stands alone. ⭐ **Read the tool's own
page, not this one**, before using it.

---

## Using a tool from another project

Fetch it by URL. Nothing here assumes it is being run from a clone.

⛔ **Pin a commit, never a branch.** `main` moves, and a moved reference runs
code nobody reviewed:

```bash
gh api repos/Azathothas/ToolKit/commits/main --jq .sha
```

⚠ **Download to a file, then run the file.** Piping a download into a shell
executes the prefix of a truncated transfer and leaves nothing to inspect.

[`docs/consumers.md`](docs/consumers.md) is the register of who fetches what,
and what a rename here breaks out there.

---

## Working on this repository

Read [`AGENTS.md`](AGENTS.md). It routes each kind of task to the files that
task needs, and [`TODO/PROGRESS.md`](TODO/PROGRESS.md) carries the current state and the
work order.

Run the probe first, on any machine:

```bash
sh scripts/doctor/doctor.sh
```

```bash
pwsh -NoProfile -File scripts/doctor/doctor.ps1
```

The gate is the checks in `scripts/common/`, run unpiped, plus CI on two hosts.
[`docs/methodology/gate.md`](docs/methodology/gate.md) is the rule.
