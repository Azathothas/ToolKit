# INDEX.md

Every entry, one line each, sorted by id. This is a **list**, not a log and not
an order. ⛔ The work order lives in [`PROGRESS.md`](PROGRESS.md) and nowhere
else.

⛔ **The counts below are checked, not typed.** `scripts/common/check-record.sh`
asserts that they agree with the rows, that every row has an entry and every
entry a row, and that no status disagrees between the two. It runs as a gate.

⭐ **And since 2026-09-17 it asserts that [`PROGRESS.md`](PROGRESS.md)'s work
order agrees with these rows**: an item marked Closed may not name an entry this
list calls open, and an item that is not marked Closed must name at least one
entry this list does not call done. ⛔ **That rule exists because the work order
outlived the work twice**, and both times this list and the entry were right and
only the order was wrong, so nothing fired.

---

## Counts

```text
total 221  open 47  blocked 5  done 169
```

| priority | open | blocked | done | total |
| --- | --- | --- | --- | --- |
| P0 | 0 | 0 | 4 | 4 |
| P1 | 3 | 0 | 80 | 83 |
| P2 | 30 | 2 | 73 | 105 |
| P3 | 14 | 3 | 12 | 29 |
| **all** | **47** | **5** | **169** | **221** |

---

## Entries

| id | pri | eff | status | title | file |
| --- | --- | --- | --- | --- | --- |
| BSD-01 | P1 | M | done | Run a BSD userland from Windows, with the least friction that works | [`bsd.md`](bsd.md) |
| BSD-02 | P3 | S | done | Whether the other three BSDs can be run, not merely built | [`bsd.md`](bsd.md) |
| BSD-03 | P1 | M | done | A BSD userland an agent can actually reach, from the tool it already runs | [`bsd.md`](bsd.md) |
| DOC-01 | P2 | S | done | A `binfmt_misc` check for the podman machine on WSL2 | [`tooling.md`](tooling.md) |
| DOC-02 | P2 | S | done | The tree broke its own character rule in 164 places | [`docs.md`](docs.md) |
| DOC-03 | P2 | S | done | Seventeen sentences had two homes | [`docs.md`](docs.md) |
| DOC-04 | P1 | S | done | The template skeletons go, and TODO/ becomes the shape the model names | [`docs.md`](docs.md) |
| DOC-05 | P1 | M | done | `docs/AGENTS.md`, and what `README.md` is for | [`docs.md`](docs.md) |
| DOC-06 | P1 | M | done | The documents carry the story of their own fixes | [`docs.md`](docs.md) |
| DOC-07 | P2 | S | done | There are two AGENTS.md files | [`docs.md`](docs.md) |
| DOC-08 | P2 | L | done | The live pages carry dated measurements and the story of their own work again | [`docs.md`](docs.md) |
| DOC-09 | P2 | M | done | The methodology is a month behind TEMPLATE | [`docs.md`](docs.md) |
| DOC-10 | P1 | S | done | The record carries history, a findings list and every session's measurements | [`docs.md`](docs.md) |
| DOC-11 | P3 | S | open | Lessons the old record held have no live page | [`docs.md`](docs.md) |
| DOC-12 | P3 | S | open | The muse-code guide starts an agent named `muse` and then prompts `work` | [`docs.md`](docs.md) |
| DOC-13 | P3 | S | done | The router and the rules count the release and CI wrong | [`docs.md`](docs.md) |
| DOC-14 | P2 | M | open | The skills tell an agent things the tool does not do | [`docs.md`](docs.md) |
| TOOL-01 | P1 | M | done | A record checker, so the counts cannot disagree with the rows | [`tooling.md`](tooling.md) |
| TOOL-02 | P1 | S | done | One command that runs the whole local gate | [`tooling.md`](tooling.md) |
| TOOL-03 | P0 | S | done | `git-sync.ps1` bound a gate string to the author identity | [`tooling.md`](tooling.md) |
| TOOL-04 | P1 | M | done | Two rules the conventions state and nothing checked | [`tooling.md`](tooling.md) |
| TOOL-05 | P1 | S | done | `check-remote-items` reported red for an item that only needed reading | [`tooling.md`](tooling.md) |
| TOOL-06 | P1 | S | done | `check-gate.ps1` skipped six checks on the host it exists for | [`tooling.md`](tooling.md) |
| TOOL-07 | P2 | M | done | The helpers `Azathothas/TEMPLATE` is dropping move here | [`tooling.md`](tooling.md) |
| TOOL-08 | P1 | S | done | The CI step that parses the workflows had never parsed one | [`tooling.md`](tooling.md) |
| TOOL-09 | P1 | S | done | `check-docs.ps1` collapsed `..` with a regex that matches `..` | [`tooling.md`](tooling.md) |
| TOOL-10 | P1 | S | done | `check-no-secrets.ps1` could not match a Windows home path at all | [`tooling.md`](tooling.md) |
| TOOL-11 | P1 | S | done | CI does not run Windows PowerShell 5.1, which is where every P0 has been | [`tooling.md`](tooling.md) |
| TOOL-12 | P2 | S | done | Nothing checks that a published release can be consumed | [`tooling.md`](tooling.md) |
| TOOL-13 | P1 | L | done | The gate took thirteen minutes, and half of it was comparing two copies of every rule | [`tooling.md`](tooling.md) |
| TOOL-14 | P2 | L | done | The last six shell pairs become one program, and `check-twins` goes | [`tooling.md`](tooling.md) |
| TOOL-15 | P1 | M | done | The rule that could only ever speak after the fact | [`tooling.md`](tooling.md) |
| TOOL-16 | P2 | M | done | Evidence that evaporates with the session | [`tooling.md`](tooling.md) |
| TOOL-17 | P1 | L | done | The suite that could not have caught any of them | [`tooling.md`](tooling.md) |
| TOOL-18 | P1 | S | done | The row of the counts that was typed | [`tooling.md`](tooling.md) |
| TOOL-19 | P1 | M | done | The instrument that proves the guards had stopped proving three of them | [`tooling.md`](tooling.md) |
| TOOL-20 | P1 | S | done | The check for the endings could never once have failed | [`tooling.md`](tooling.md) |
| TOOL-21 | P2 | S | done | The sanctioned way to commit on Windows could not name two files | [`tooling.md`](tooling.md) |
| TOOL-22 | P1 | S | done | the runtime column had never once produced an answer | [`tooling.md`](tooling.md) |
| TOOL-23 | P1 | S | done | The bundle rule rebuilt the products it was meant to compare | [`tooling.md`](tooling.md) |
| TOOL-24 | P1 | S | done | The gate printed a check that could not run as a pass | [`tooling.md`](tooling.md) |
| TOOL-25 | P1 | S | done | The secrets rule could not see a Windows home path | [`tooling.md`](tooling.md) |
| TOOL-26 | P1 | S | done | The job that proves every guard cannot finish the table it proves | [`tooling.md`](tooling.md) |
| TOOL-27 | P2 | M | done | The Claude desktop app runs one account per user, and a second instance shares everything with the first | [`tooling.md`](tooling.md) |
| TOOL-28 | P2 | S | done | A carriage return inside a line passes the control-bytes rule | [`tooling.md`](tooling.md) |
| TOOL-29 | P1 | S | done | `PROGRESS.md` grew to 2,776 lines and no check fired | [`tooling.md`](tooling.md) |
| TOOL-30 | P1 | S | done | The docs check's cited-path rule never ran, and it carried another project's rules | [`tooling.md`](tooling.md) |
| TOOL-31 | P3 | S | done | The gate's documented time is 30 seconds, and it takes minutes | [`tooling.md`](tooling.md) |
| TOOL-32 | P2 | M | open | A workflow's own shell has no case | [`tooling.md`](tooling.md) |
| TOOL-33 | P2 | S | open | A Go case that passes only on Windows can reach `main`, because the Linux checks are run by hand | [`tooling.md`](tooling.md) |
| TOOL-34 | P2 | S | done | A PowerShell runner that cannot run can still exit 1 | [`tooling.md`](tooling.md) |
| TOOL-35 | P3 | S | done | The gate's own list says the analyzer covers `scripts/` alone | [`tooling.md`](tooling.md) |
| TOOL-36 | P3 | S | done | A changelog stamp later than the clock passes the gate | [`tooling.md`](tooling.md) |
| TOOL-37 | P3 | S | open | A nightly whose upload fails loses the whole prerelease | [`tooling.md`](tooling.md) |
| TOOL-38 | P3 | M | done | The skills check never matches a text-tool call by its full path, and checks no `wsl-toolkit` flag | [`tooling.md`](tooling.md) |
| WSL-01 | P0 | S | done | `New -Command` must propagate the inner exit code | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-02 | P1 | M | done | Carry the image's OCI configuration into the distro | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-03 | P1 | S | done | Pass `--platform` to pull and create | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-04 | P1 | S | done | A failed delete must not report success | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-05 | P2 | S | done | Report and purge orphaned rootfs tarballs | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-06 | P2 | S | done | Disk-space preflight before import | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-07 | P2 | S | done | Optional systemd via `/etc/wsl.conf` | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-08 | P2 | M | done | A `-Command` channel that survives two shells | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-09 | P3 | S | done | Bound the smoke probe with a timeout | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-10 | P3 | S | done | Retry a generated name on collision | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-11 | P3 | S | done | An `Enter` action | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-12 | P0 | S | done | `-Action New` fails outright on Windows PowerShell 5.1 | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-13 | P2 | M | done | Report what the machine is holding, and offer rather than act | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-14 | P2 | M | done | Answer what a distro reaches the host at, without creating a distro | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-15 | P2 | M | done | A launcher, so one fetch is enough | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-16 | P1 | M | done | The file channel makes a consumer normalise and encode by hand | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-17 | P1 | M | done | The launcher makes every consumer resolve a commit and a digest by hand | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-18 | P1 | L | done | A command that prints nothing is indistinguishable from one that has died | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-19 | P2 | S | done | Nothing bounds the caller's command, so a hung run ends in a kill | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-20 | P2 | S | done | One API host is a single point of failure | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-21 | P2 | L | done | `wsl-ephemeral.ps1` is 2,792 lines in one file | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-22 | P3 | S | done | The stream log has no sink, no colour and no prefix-only mode | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-23 | P3 | M | done | Parameters are silently ignored by the actions they do not apply to | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-24 | P1 | S | done | A list parameter cannot be repeated, and an int list binds a wrong number | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-25 | P2 | M | done | The release digest proves transport, not authorship | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-26 | P2 | M | done | A prepared rootfs is thrown away and paid for again | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-27 | P2 | M | done | The tick can say nothing is happening and never that something is | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-28 | P2 | M | done | A recorded run cannot be re-read or compared | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-29 | P3 | S | done | Every run imports, even when a distro from the same image is registered | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-30 | P2 | XL | done | The mockup's other two thirds: a podman adapter | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-31 | P1 | XL | done | A portable entry point for isolated Linux jobs on Windows | [`issue-6.md`](issue-6.md) |
| WSL-32 | P1 | M | done | Helper routing asks whether `wsl.exe` resolves, not whether it answers | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-33 | P1 | M | done | A failed artifact transfer exits 0 and the output is then destroyed | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-34 | P1 | M | done | Two artifact names that differ only in case become one file | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-35 | P1 | L | done | Output is bounded without saying so and arrives only at the end | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-36 | P1 | M | done | `gc` removes every container before it checks any age | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-37 | P2 | M | done | The helper never releases an upload or an artifact set | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-38 | P2 | S | done | The CLI accepts a trailing word and silently drops what follows it | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-39 | P2 | M | done | An image that could not be pulled counts as a job that ran and failed | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-40 | P1 | M | done | What a second reading of the core found | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-41 | P1 | M | done | What a failure is allowed to hide | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-42 | P1 | L | done | What this tool owns, and how it proves it | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-43 | P1 | L | done | Many agents, many bases | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-44 | P1 | L | done | What a long-lived helper freezes | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-45 | P1 | M | done | A deadline that bounds the caller's wall time | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-46 | P1 | M | done | The answer is exactly what happened | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-47 | P1 | M | done | The boundary the manual promises | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-48 | P1 | M | done | The embedded script tells the truth | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-49 | P1 | L | done | One command to readiness | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-50 | P1 | L | done | Diagnostics and a heartbeat for the base and what runs inside it | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-51 | P1 | L | done | A config the agent does not have to write | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-52 | P2 | L | done | The six commands that make an answer actionable | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-53 | P1 | M | done | The tool updates itself, and readiness says whether it should | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-54 | P2 | S | done | The three answers a diagnostic has to tell apart | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-55 | P2 | S | done | A report that creates the thing it is describing | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-56 | P2 | M | done | What a failed job leaves a reader to ask the machine by hand | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-57 | P1 | S | done | Two selftest cases were written against one host's environment | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-58 | P2 | M | done | `inspect` is the one report the helper route cannot serve | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-59 | P2 | L | done | the podman adapter, on what the matrix says it can be built | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-60 | P2 | M | done | the base accepts a memory limit and does not enforce it | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-61 | P2 | S | done | `base ensure` cannot recover a base whose engine has stale run state | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-62 | P2 | S | done | two of this tool sharing one state directory corrupt each other's writes | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-63 | P1 | M | done | A consumer wrote a wrapper for four things this tool should have done | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-64 | P1 | S | done | A native job ran whatever architecture the image store happened to hold | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-65 | P2 | S | done | The generated manual carried a control byte, and its drift check agreed with it | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-66 | P1 | M | done | `--workspace .` resolved against a directory the caller could not see | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-67 | P2 | L | done | A provider's Linux-only CLI, run from Windows as if it were native | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-68 | P2 | M | done | A base that can reach nothing on the host at all | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-69 | P1 | M | done | Muse Code installed, authenticated and driven end to end | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-70 | P2 | L | done | Two package maps become one, and the Go module embeds it | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-71 | P2 | M | done | A portable shell profile this tree owns | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-72 | P2 | S | done | The BSD guest gets a 10 GiB disk, and the languages install on it | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-73 | P1 | XL | done | The PowerShell product retires, and pull request 31 is reviewed before any of it lands | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-74 | P1 | S | done | A named instance acts on whatever base the nearest project file names | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-75 | P2 | L | done | One named base serves every project, and a grant changes without a restart | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-76 | P2 | L | done | herdr replaces Zellij, and the operator watches the agents from Windows | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-77 | P2 | L | done | A provider base rebuilt from a clone in one command, with herdr and Muse as its first adapters | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-78 | P2 | L | done | Muse from any Windows project, and a guide for someone who has never used a coding agent | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-79 | P1 | M | done | A BSD run pays two minutes before its first command, and a comment line ends its script early | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-80 | P1 | M | done | A throwaway command that has finished can leave `distro run` waiting forever | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-81 | P1 | M | done | A FreeBSD guest that panics mid-run leaves `bsd run` waiting out its budget | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-82 | P2 | S | done | A payload that prints a FreeBSD panic's two lines has its `bsd run` ended as a kernel panic | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-83 | P1 | M | done | A FreeBSD kernel panic at poweroff leaves the shared guest image unchecked, and the next run panics on it | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-84 | P1 | M | done | A base reconfigured to read-only drives keeps writable ones, and reports read-only | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-85 | P1 | S | done | A base reconfigured from passwordless sudo to none keeps it, and reports none | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-86 | P1 | M | done | The debian and fedora presets build a base that cannot run a container | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-87 | P1 | S | done | bootstrap.sh's CodeGraph install fails wherever /bin/sh is dash | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-88 | P2 | M | done | The pi adapter, and herdr's first lifecycle authority in this base | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-89 | P2 | M | done | The omp adapter, and the directory collision herdr refuses | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-90 | P2 | L | done | herdr built nightly from its development branch, published here, and followed by the herdr adapter | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-91 | P2 | S | done | The consumer smoke can only break after a release | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-92 | P1 | M | done | text-tool becomes a product an agent downloads, not a script in a checkout | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-93 | P1 | S | done | `--between` is the one edit operation with no required count, and it deletes the most | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-94 | P1 | L | done | A job whose client is gone can be neither stopped, followed nor waited on | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-95 | P2 | M | done | A long base drive has to be held in a foreground client | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-96 | P2 | S | done | A job container cannot be given a host device | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-97 | P2 | S | done | A job takes one payload file, so a second one travels through the workspace and collides | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-98 | P2 | S | done | A workspace copy that fails does not always name the file | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-99 | P2 | S | done | Only `distro` has a channel no shell can reach into, and Git Bash rewrites the rest | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-100 | P2 | S | done | The `base` group accepts flags its subcommands never read | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-101 | P1 | S | done | `--redact` keeps a secret off the screen and writes it to the answer and the transcript | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-102 | P0 | S | done | `gc --via-helper --job ID` plans and applies a cleanup of every job | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-103 | P1 | S | done | Under an instance, `helper serve --detach` never answers, and a detached run would not either | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-104 | P2 | S | done | `--help` and the manual name flags a subcommand refuses, and `helper` and `config` still ignore flags | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-105 | P1 | M | done | Another distribution empties the QEMU handler table, and only a job gets its handler back | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-106 | P1 | S | done | A grant whose Windows directory is gone refuses every command, `base revoke` included | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-107 | P2 | S | done | `base ensure` stops at the first adapter whose read-back finds a problem, and a probe calls an adapter installed that never was | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-108 | P1 | S | done | `shipped cat NAME` and `shipped write NAME` can never succeed | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-109 | P2 | M | done | The space a base frees comes back only with `base recreate` | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-110 | P2 | M | done | WSL's own container tool is on this host, and this tool does not know it | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-111 | P2 | S | done | A Git Bash caller bridges every call through PowerShell | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-112 | P2 | S | done | A workspace copy takes what git ignores, so each project keeps its own exclude list | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-113 | P2 | S | done | Nothing stops two jobs of one project from running at once | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-114 | P2 | S | done | `gc` cannot be limited to one project's jobs | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-115 | P2 | S | done | `gc --job` with an id nothing here belongs to exits 0, having removed nothing | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-116 | P2 | S | done | A printed command drops `--instance`, so it reaches the default instance | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-117 | P1 | S | done | `gc --apply` cannot remove a job's host directory when Windows redirects the state directory | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-118 | P1 | S | done | A caller Windows redirects keeps a second state directory, and nothing says so | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-119 | P1 | S | done | git runs in a tree a container or a helper client wrote, and a setting there runs a program | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-120 | P2 | M | open | A job id is hard to use: a prefix, the container name and one-line JSON are all refused | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-121 | P1 | M | open | An ephemeral job keeps its host record, and a killed caller leaves its job running | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-122 | P2 | M | open | Nothing is cached between jobs, so each job installs its toolchain again | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-123 | P2 | M | open | A job cannot run as an account its image lacks, and `base exec` cannot run as another account | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-124 | P2 | M | open | `base exec` lacks the inputs, artifacts, account and stdin `run` has, and its `--dir` takes guest paths only | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-125 | P2 | M | open | A 9P grant is slow, and nothing copies a checkout into the base | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-126 | P2 | M | open | `stop` does not reach a session's child that left the process group | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-127 | P2 | M | open | The tool's own exit 2 cannot be told from a job's exit 2 | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-128 | P2 | S | open | An input file keeps CRLF and no executable bit | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-129 | P2 | S | open | A project cannot say which version of the tool it needs | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-130 | P2 | L | open | An engine inside a base is set up by hand | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-131 | P2 | M | open | Job defaults are repeated in every wrapper | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-132 | P2 | S | open | Two acceptance cases are not isolated from what they run beside | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-133 | P3 | S | open | `base shell` has no `--private-net` | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-134 | P3 | S | blocked | A real window's focus event was never produced for herdr's `--remote` signals | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-135 | P3 | S | blocked | The muse-code guide was never run as one ordered pass from an empty host | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-136 | P2 | M | open | A memory or cpu limit is accepted in a base and enforced by nothing, and nothing caps how many jobs run | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-137 | P2 | M | open | A base stops running containers after a host restart until someone repairs it | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-138 | P2 | M | blocked | An adapter installs from npm where the provider publishes an official installer | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-139 | P3 | S | open | `base ensure` leaves older herdr nightly clients on disk | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-140 | P2 | S | open | A base build fails when one package mirror stalls, and nothing retries | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-141 | P2 | S | open | The herdr adapter installs 0.9.0 when this repository publishes 0.9.5 | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-142 | P3 | S | open | `base doors --via-helper`'s refusal was never run against a live helper | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-143 | P3 | S | open | The base account's login shell is `/bin/sh` | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-144 | P2 | S | open | An agent pane cannot be started in the project directory | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-145 | P2 | S | open | The adapter contract's example probe passes with no wrapper | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-146 | P3 | S | open | No channel gives an agent an environment variable | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-147 | P1 | S | open | After `base bootstrap`, agents bypass this tool's wrappers | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-148 | P2 | S | open | After a cancelled approval, herdr refuses every prompt, and no guide says how out | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-149 | P2 | S | open | `bootstrap.sh --dry-run` reports a failure on an image without curl | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-150 | P2 | M | open | A base build killed before its marker leaves a distribution only `wsl --unregister` can remove | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-151 | P2 | S | open | A cancelled `bsd run` says its budget ran out and exits 2 | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-152 | P2 | S | open | Four guest errors are named by the wrong line | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-153 | P3 | S | open | An automount-off build logs one translate failure per Windows PATH entry | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-154 | P3 | S | open | bootstrap.sh's cases read it from outside the module, so no mutation row can hold one | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-155 | P2 | S | open | A drive WSL mounts during a long build fails the new base | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-156 | P2 | M | open | `base doors` lists three world-writable device nodes and tries none of them | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-157 | P2 | S | open | The muse probe calls the herdr reporter registered without Muse ever running it | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-158 | P3 | S | blocked | Without the reporter, herdr shows a Muse pane as idle | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-159 | P2 | M | blocked | An agent started through herdr runs in the shared network namespace | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-160 | P3 | S | open | Three command groups keep two lists of their subcommands that nothing compares | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-161 | P1 | S | open | A configuration that leaves out `systemd` turns systemd off | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-162 | P3 | S | open | Nothing holds a job container's log driver | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-163 | P3 | S | open | `resources --host-engine` calls the engine outside the deadline door | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-164 | P2 | S | open | `helper`, `distro`, `bsd`, `shipped` and `artifacts` refuse their own `--help` | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-165 | P2 | S | done | `text-tool eol` creates an empty file at a path that does not exist | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-166 | P3 | S | open | `text-tool eol` answers 0 for a path that does not exist | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |

---

## Priorities and effort

Defined once, here, and meant.

| priority | means |
| --- | --- |
| P0 | breaks correctness, loses data, or takes the process down |
| P1 | a documented capability does not work, or a flag does nothing |
| P2 | worth doing; nothing is wrong without it |
| P3 | worth recording so it is not rediscovered |

| effort | means |
| --- | --- |
| S | under a day |
| M | a few days |
| L | a week |
| XL | ⚠ almost always two entries pretending to be one |

---

## ⭐ The argument behind the current order

Written down so a later session can re-derive it rather than re-argue it. The
order itself is in [`PROGRESS.md`](PROGRESS.md), ruling 31's. The orderings of
earlier batches are in
[`../docs/HISTORY/record.md`](../docs/HISTORY/record.md).

⭐ **The record and its checks first** (`DOC-10`, `TOOL-29`, `TOOL-30`,
`DOC-09`). Every later commit is checked by them, so a rule armed late lets the
commits before it through unchecked. `TOOL-30` armed a rule that had never run,
and it found live pages citing paths that had moved.

⭐ **The survey's asks in parallel, split by code area rather than by priority.**
Seven builders work at once, each in its own worktree, and a split by file keeps
their merges mechanical. Two builders that must touch one file keep their changes
in separate hunks.

⚠ **`WSL-121`, `WSL-147` and `WSL-161` are P1 inside that work.** Each is a
documented behaviour the tool does not have today, and `WSL-161` takes systemd away
from a base whose configuration leaves the key out. A builder takes them first
within its share.

⚠ **The tooling after the asks**, because `TOOL-33`'s pre-push hook makes every
later push slower, and the builders' branches are merged before it lands.

⛔ **`WSL-127` waits for a major**, by ruling 31, and the five blocked entries wait
on what each one names. None of them blocks anything else.
