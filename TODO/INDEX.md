# INDEX.md

Every entry, one line each, sorted by id. This is a **list**, not a log and not
an order. ⛔ The work order lives in [`PROGRESS.md`](PROGRESS.md) and nowhere
else.

⛔ **The counts below are checked, not typed.** `scripts/common/check-record.sh`
asserts that they agree with the rows, that every row has an entry and every
entry a row, and that no status disagrees between the two. It runs as a gate.

---

## Counts

```text
total 78  open 20  blocked 0  done 58
```

| priority | open | blocked | done | total |
| --- | --- | --- | --- | --- |
| P0 | 0 | 0 | 3 | 3 |
| P1 | 12 | 0 | 29 | 41 |
| P2 | 7 | 0 | 20 | 27 |
| P3 | 1 | 0 | 6 | 7 |
| **all** | **9** | **0** | **54** | **63** |

---

## Entries

| id | pri | eff | status | title | file |
| --- | --- | --- | --- | --- | --- |
| BSD-01 | P1 | M | done | Run a BSD userland from Windows, with the least friction that works | [`bsd.md`](bsd.md) |
| BSD-02 | P3 | S | done | Whether the other three BSDs can be run, not merely built | [`bsd.md`](bsd.md) |
| DOC-01 | P2 | S | done | A `binfmt_misc` check for the podman machine on WSL2 | [`tooling.md`](tooling.md) |
| DOC-02 | P2 | S | done | The tree broke its own character rule in 164 places | [`docs.md`](docs.md) |
| DOC-03 | P2 | S | done | Seventeen sentences had two homes | [`docs.md`](docs.md) |
| DOC-04 | P1 | S | done | The template skeletons go, and TODO/ becomes the shape the model names | [`docs.md`](docs.md) |
| DOC-05 | P1 | M | done | `docs/AGENTS.md`, and what `README.md` is for | [`docs.md`](docs.md) |
| DOC-06 | P1 | M | done | The documents carry the story of their own fixes | [`docs.md`](docs.md) |
| DOC-07 | P2 | S | done | There are two AGENTS.md files | [`docs.md`](docs.md) |
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
| TOOL-11 | P1 | S | open | CI does not run Windows PowerShell 5.1, which is where every P0 has been | [`tooling.md`](tooling.md) |
| TOOL-12 | P2 | S | open | Nothing checks that a published release can be consumed | [`tooling.md`](tooling.md) |
| TOOL-13 | P1 | L | done | The gate took thirteen minutes, and half of it was comparing two copies of every rule | [`tooling.md`](tooling.md) |
| TOOL-14 | P2 | L | done | The last six shell pairs become one program, and `check-twins` goes | [`tooling.md`](tooling.md) |
| TOOL-15 | P1 | M | done | The rule that could only ever speak after the fact | [`tooling.md`](tooling.md) |
| TOOL-16 | P2 | M | done | Evidence that evaporates with the session | [`tooling.md`](tooling.md) |
| TOOL-17 | P1 | L | open | The suite that could not have caught any of them | [`tooling.md`](tooling.md) |
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
| WSL-25 | P2 | M | open | The release digest proves transport, not authorship | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-26 | P2 | M | open | A prepared rootfs is thrown away and paid for again | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-27 | P2 | M | open | The tick can say nothing is happening and never that something is | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-28 | P2 | M | open | A recorded run cannot be re-read or compared | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-29 | P3 | S | open | Every run imports, even when a distro from the same image is registered | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-30 | P2 | XL | open | The mockup's other two thirds: a podman adapter | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
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
| WSL-42 | P1 | L | open | What this tool owns, and how it proves it | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-43 | P1 | L | open | Many agents, many bases | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-44 | P1 | L | open | What a long-lived helper freezes | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-45 | P1 | M | open | A deadline that bounds the caller's wall time | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-46 | P1 | M | open | The answer is exactly what happened | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-47 | P1 | M | open | The boundary the manual promises | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-48 | P1 | M | open | The embedded script tells the truth | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-49 | P1 | L | open | One command to readiness | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-50 | P1 | L | open | Diagnostics and a heartbeat for the base and what runs inside it | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-51 | P1 | L | open | A config the agent does not have to write | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |
| WSL-52 | P2 | L | open | The six commands that make an answer actionable | [`wsl-toolkit-go.md`](wsl-toolkit-go.md) |

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

## ⭐ The argument behind the order these were worked in

Written down so a later session can re-derive it rather than re-argue it.
⚠ **This is a record of an ordering, not a live work order.** Every entry is
closed, and [`PROGRESS.md`](PROGRESS.md) is where the next one will be.

### The 2026-08-29 batch, ordered by what unlocked what

⭐ **`TOOL-04` first, and it is not the most important entry.** It arms the two
checks that MEASURE the defects `DOC-02` and `DOC-03` are about. Working those
two first would have meant fixing what could be seen by reading, declaring it
done, and leaving whatever a reading missed. The instrument comes before the
count.

**`TOOL-05` next because it is small and it was making a whole workflow lie.**
An unread issue was reported as a failed check, so the weekly pass had been red
since the first issue was filed. It is unrelated to everything else in the
batch, which is exactly why it goes early rather than being carried.

**Then `DOC-02`, `DOC-04`, `DOC-03`, in that order.** `DOC-02` is mechanical and
touches every file, so it goes before anything that would have to be written
twice. `DOC-04` removes nine files, and seven of `DOC-03`'s seventeen findings
were in them, so removing first makes the remaining ten the real list rather
than a list of things about to be deleted.

**`TOOL-06`, `TOOL-07` and `TOOL-08` are where they are because each was found
by doing the one before it.** `TOOL-06` came out of wiring `TOOL-04`'s checks
into both halves of the gate; `TOOL-07` restored a twin comparison that had been
removed with the files it compared; `TOOL-08` came out of validating a change to
the workflow `TOOL-04` had just edited. ⚠ None of the three was in the plan, and
each is a P1 or a P2 the plan would not have found.

**`WSL-13`, `WSL-14` and `WSL-15` last**, because they change a file other
repositories fetch. Everything before them is internal, so a mistake there
costs this tree a commit; a mistake here costs a caller nobody can reach.
⭐ `WSL-15` is last of the three: it wraps the script the other two change, and
wrapping a moving target is how a wrapper drifts.

### The 2026-08-27 batch, kept because the reasoning still holds

**`WSL-01` outranked everything regardless of size.** It was the only entry
where the software reported success over a failure. Every other entry costs
someone time; that one cost them a wrong belief, and the documented CI example
was the affected path.

**`TOOL-01` came next despite being infrastructure**, because it protects the
record every other entry is tracked in, and the failure it prevents is the one
`work-todo.md` says was actually paid for.

**`BSD-01` was P1 and sat behind the `WSL-*` work**, not because it was blocked
but because the half of it that was going to be built here belonged elsewhere.
⚠ **`BSD-02` was P3 and small**, written so nobody re-derives the same negative
answer in six months.
