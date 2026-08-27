# TODO: tooling

Entries for this repository's own checks and helpers.

[`INDEX.md`](INDEX.md) is the list; [`PROGRESS.md`](PROGRESS.md) is the order.

---

## DOC-01. A `binfmt_misc` check for the podman machine on WSL2

**Source** `Azathothas/TEMPLATE` issue 2, suggestion 1. ⚠ The issue proposed it
for `scripts/doctor/`; that placement is refused below and this is where it
landed instead.
**Category** tooling · **Priority** P2 · **Effort** S · **Status** open

**Problem.** On Windows with a podman machine, cross-architecture containers
fail with `Exec format error` while every visible signal says the machine is
healthy.

**Premise.** ⭐ **Measured on 2026-08-27**, on the reporting machine, and the
diagnosis held. `/proc/sys/fs/binfmt_misc` carries a `binfmt_misc` instance
with a systemd autofs stacked on the same path; reading it returns `ELOOP`.
⛔ `systemd-binfmt.service` reports `status=0/SUCCESS` having registered
nothing. The machine measured here already has the reporter's fix installed:
`podman-binfmt-fix.service` is `enabled` and the qemu handler count is **31**,
the number the issue predicted.

⚠ One claim in the source issue was **not** reproduced and is not treated as
verified: the ten-hour kernel against seconds-old userspace. This machine had
been cold-started, so the two timestamps were 7 seconds apart. That
`podman machine stop` does not restart the WSL2 kernel is architectural and is
documented in `scripts/powershell-windows/wsl-ephemeral.md`; the specific skew
is unmeasured here.

**Approach.** A standalone script in `scripts/`, run on request, following the
contract in [`../scripts/README.md`](../scripts/README.md): a header naming the
defect, exit 0 pass, 1 fail, 2 could not run, a `--json` switch, and no
dependence on the directory it runs from.

It reports the handler count, names the `ELOOP` case specifically rather than
reporting a generic failure, and distinguishes "no podman machine" (exit 2, it
could not run) from "the machine is broken" (exit 1).

**Decision.** ⛔ **Refused for `scripts/doctor/`, which is where the issue asked
for it.**

The probe is read-only, spawns one process per tool, makes no network call
without `--net`, and is inherited by every project started from the template.
Reaching into the machine costs a `podman machine ssh`, which is slow, can hang,
and, ⭐ **measured on 2026-08-27, writes a 99-byte `NUL` file into the working
directory under Git Bash** because it passes `-o UserKnownHostsFile=NUL` to its
own ssh. A probe that litters the repository it is probing is a worse defect
than the one it detects. Most projects starting from that template will never
run a container.

⚠ The alternative considered and rejected: reading `/proc/sys/fs/binfmt_misc`
directly in `doctor.sh` when it runs on Linux. Cheap and honest, but it adds a
field to a schema whose two twins must agree, for an answer only one host can
give, to benefit a case this repository is the only known instance of.

**Prove.** Against a machine in the broken state, exit 1 and the message names
`ELOOP` and the stacked mount. Against this machine, exit 0 and the reported
count is 31.

```bash
podman machine ssh 'ls -1 /proc/sys/fs/binfmt_misc/ | grep -c "^qemu-"'
```

⚠ Run the check from a scratch directory, not from a repository, until it is
confirmed not to leave a `NUL` behind. The defect it exists to document is one
it can commit itself.

---

## TOOL-01. A record checker, so the counts cannot disagree with the rows

**Source** [`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md),
which calls this the model's one mechanical hazard and says to automate it.
**Category** tooling · **Priority** P1 · **Effort** M · **Status** open

⚠ **Half done.** The reader exists as `scripts/common/check-record.sh`; the
writer does not, so the arithmetic is still manual and the reader is what
catches it. The entry stays open until both exist.

**Problem.** Closing one entry moves several numbers: the index totals, the
priority table rows, and the record's own counts. Doing that arithmetic by hand
is how a published record says an entry is open beside an entry saying done.

**Premise.** ⭐ Measured: the reader was written this session and caught a real
disagreement on its first run, before any of it was committed.

**Approach.** Two scripts. ⚠ Only the reader is written.

- **The reader**, and ⭐ it runs as a gate, so a count that disagrees with the
  rows cannot reach a commit. It asserts that every entry has an index row and
  every row an entry, that no status disagrees between the two, and that the
  declared counts match the rows.
- **The writer**, which moves a status and re-derives every count. Not written.
  Until it exists the arithmetic is manual and the reader is what catches it.

**Decision.** Reader first, on purpose. The reader alone turns a silent
inconsistency into a failed gate, which is the whole of the documented damage.
The writer only saves typing, and a writer without a reader would be a second
thing to trust.

**Prove.**

```bash
sh scripts/common/check-record.sh
```

Exit 0 on a consistent tree. ⛔ Mutation-prove it: change one status in
`INDEX.md` without changing the entry, confirm exit 1 naming both files, then
put it back.
