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
total 15  open 15  blocked 0  done 0
```

| priority | open | blocked | done | total |
| --- | --- | --- | --- | --- |
| P0 | 1 | 0 | 0 | 1 |
| P1 | 5 | 0 | 0 | 5 |
| P2 | 5 | 0 | 0 | 5 |
| P3 | 4 | 0 | 0 | 4 |
| **all** | **15** | **0** | **0** | **15** |

---

## Entries

| id | pri | eff | status | title | file |
| --- | --- | --- | --- | --- | --- |
| BSD-01 | P1 | M | open | Run a BSD userland from Windows, with the least friction that works | [`bsd.md`](bsd.md) |
| BSD-02 | P3 | S | open | Whether the other three BSDs can be run, not merely built | [`bsd.md`](bsd.md) |
| DOC-01 | P2 | S | open | A `binfmt_misc` check for the podman machine on WSL2 | [`tooling.md`](tooling.md) |
| TOOL-01 | P1 | M | open | A record checker, so the counts cannot disagree with the rows | [`tooling.md`](tooling.md) |
| WSL-01 | P0 | S | open | `New -Command` must propagate the inner exit code | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-02 | P1 | M | open | Carry the image's OCI configuration into the distro | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-03 | P1 | S | open | Pass `--platform` to pull and create | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-04 | P1 | S | open | A failed delete must not report success | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-05 | P2 | S | open | Report and purge orphaned rootfs tarballs | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-06 | P2 | S | open | Disk-space preflight before import | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-07 | P2 | S | open | Optional systemd via `/etc/wsl.conf` | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-08 | P2 | M | open | A `-Command` channel that survives two shells | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-09 | P3 | S | open | Bound the smoke probe with a timeout | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-10 | P3 | S | open | Retry a generated name on collision | [`wsl-ephemeral.md`](wsl-ephemeral.md) |
| WSL-11 | P3 | S | open | An `Enter` action | [`wsl-ephemeral.md`](wsl-ephemeral.md) |

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

## ⭐ The argument behind the current ordering

Written down so a later session can re-derive it rather than re-argue it.

**`WSL-01` outranks everything regardless of size.** It is the only entry where
the software reports success over a failure. Every other entry costs someone
time; this one costs them a wrong belief, and the documented CI example is the
affected path. It is an S.

**`TOOL-01` is next despite being infrastructure**, because it protects the
record every other entry is tracked in, and the failure it prevents is the one
`work-todo.md` says was actually paid for: a published record saying entries
were open beside entries saying done.

**Then `WSL-03`, `WSL-04`, `WSL-02`**, in that order rather than by priority
alone. `WSL-03` and `WSL-04` are both S and both remove a silent wrong answer.
`WSL-02` is the same priority and an M, and it changes behaviour, so it wants a
ruling recorded before it starts.

**`BSD-01` is P1 and sits behind the `WSL-*` work**, not because it is
blocked but because the half of it that was going to be built here turned out
to belong elsewhere: `pkgforge-dev/docker-bsd` publishes the images, and what
is left here is one scripted VM guest. ⚠ It carries an operator decision with a
recommendation attached, and the first step is a measurement, not a build.

⚠ **`BSD-02` is P3 and could be done any time.** It is small, it is written, and
it exists so nobody re-derives the same negative answer in six months.

**The `WSL-05` through `WSL-11` tail is ordered by effort, not by value.** They
are close enough in value that ordering them any other way would be inventing a
distinction.
