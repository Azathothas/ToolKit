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
total 43  open 0  blocked 0  done 43
```

| priority | open | blocked | done | total |
| --- | --- | --- | --- | --- |
| P0 | 0 | 0 | 3 | 3 |
| P1 | 0 | 0 | 19 | 19 |
| P2 | 0 | 0 | 15 | 15 |
| P3 | 0 | 0 | 6 | 6 |
| **all** | **0** | **0** | **43** | **43** |

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
