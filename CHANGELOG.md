# CHANGELOG.md

What shipped, when, and where the evidence is. One entry per shipped unit of
work, pointing at the record that carries the detail.

Four rules, and
[`scripts/common/check-changelog.sh`](scripts/common/check-changelog.sh) holds
all four:

1. ⛔ **Newest first.** A new entry goes at the top of its section.
2. ⛔ **Every heading carries a date**, as an ISO 8601 UTC stamp.
3. ⛔ **Every entry names its record**, the entry or commit carrying the
   evidence.
4. ⛔ **Every entry says whether it deployed.** "No deploy" is a complete and
   common answer. Silence is not.

⛔ Do not tidy this file while shipping something else, and do not delete an
entry. A superseded one is amended in place with a dated note.

---

## 2026-08-27

### 2026-08-27T10:13:41Z: a command channel that survives both shells

**Record:** `WSL-08` in [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), closed
in place with its evidence.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing. The
`Azathothas/TEMPLATE` pin has NOT moved for this change yet.

⭐ **`wsl-ephemeral.ps1` now carries a command as base64 and sources it inside
the distro**, so a quote, a dollar sign, a backtick or a tab arrives byte-exact.
`echo $PATH` works; it used to die with ``syntax error: unexpected "("``.

- **New parameters, both additive:** `-CommandFile` reads a file on this machine
  verbatim, so a multi-line script works; `-CommandB64` takes the command as
  base64. `-Command` stays and is unchanged in spelling. The three are mutually
  exclusive and passing two is refused.
- ⛔ **All three payloads the script sends now go through one function**, which
  **asserts** the transport stays inside the measured alphabet. Two of the three
  used to be hand-written inside that alphabet with nothing enforcing it, which
  is how `WSL-12` shipped.
- ⚠ **One observable difference for an existing caller**, and it is a fix rather
  than a break: `$VAR` in a `-Command` is now expanded by the guest's login
  shell instead of in transit, so with `-OciEnv` it expands to the image's
  value. Nothing renamed, no exit code changed meaning.
- ⚠ **What is still not possible, and it is not this script:** Windows
  PowerShell 5.1 drops a double quote when it builds a child process's argument
  list, so a `-Command` value loses it before this script runs. `-CommandB64` is
  immune and is the documented answer.
- ⭐ **The smoke probe carries `WSL-12`'s exact line again**, brackets and double
  quotes included, and `-Action New` works on 5.1. It is the first thing that
  fails if the channel ever breaks again.

### 2026-08-27T09:20:00Z: the door sweep found New broken on PowerShell 5.1

**Record:** `WSL-12` in [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), filed
and closed in place.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing. The
`Azathothas/TEMPLATE` pin DID move, which is the closest thing to one.

⛔ **`-Action New` was failing outright under Windows PowerShell 5.1**, on a
host `.NOTES` claimed to be tested on. The smoke probe carried a bracket inside
a double-quoted `echo`, the quoting did not survive `wsl.exe`, and every run
reported `Distro imported but /bin/sh did not run` and rolled back.

- Nobody reported it. It came out of part (c) of the gate, the door sweep, run
  against the `WSL-01` to `WSL-05` batch.
- ⚠ **It is host-specific**, which is why it survived: under PowerShell 7.6.5
  the probe runs, and every measurement in this repository until now had been
  taken with `pwsh`.
- The same sweep found a fourth deletion path that did not go through the one
  helper, while `wsl-ephemeral.md` claimed there was one and every path reached
  it. The claim was false for one commit and is true again.
- The claim audit corrected two more published sentences: a limits row saying
  the caller owns `-Command` quoting, which measurement disproved, and a
  pin-state cell and an entry closure that pointed at each other and stated
  nothing.

⭐ **The `Azathothas/TEMPLATE` pin moved** to the head of this batch.
[`docs/consumers.md`](docs/consumers.md) says why: leaving a 5.1 caller pinned
to the old commit protects them from the fix rather than from the break.


### 2026-08-27T08:45:00Z: one command for the gate, a record writer, and a binfmt check

**Record:** `TOOL-02`, `TOOL-01` and `DOC-01` in
[`TODO/tooling.md`](TODO/tooling.md), each closed in place with its evidence.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

Three tools, and each removes something a session was doing by hand.

- `scripts/common/check-gate.sh` and its twin run every local gate in one
  command. ⛔ Not a second set of rules: every line delegates to a check that
  already exists. `--fast` skips `check-twins` alone, measured at 41s against
  208s for the full run.
- `scripts/common/check-powershell.ps1` holds the two PowerShell assertions CI
  had inline. ⚠ **PSScriptAnalyzer is optional and saying so is the point**: a
  machine without it reports SKIPPED and exits 0, and CI installs it and then
  asserts it was not skipped.
- `scripts/common/set-record.mjs` moves an entry's status and re-derives every
  count. It does not run the reader and report green.
- `scripts/common/check-binfmt.sh` and its twin read the kernel rather than a
  unit's exit code. ⭐ **No `podman machine ssh`**, so nothing is written into
  the directory it runs from.

⭐ **The gate found three defects, two of them its own**, and they are written
into `TOOL-02`: an infinite recursion with `check-twins` that left twenty stray
shells holding their own files open; a skipped analyzer reported as a passed
check, which is the forbidden pattern its own header cites; and a `.ps1`
shipped with no UTF-8 BOM, caught by the analyzer it had just wired in.


### 2026-08-27T07:32:10Z: `New -Command` propagates the inner exit code

**Record:** `WSL-01` in [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md),
closed in place with its evidence.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

⛔ **This is a breaking change and it is the point of the change.**
`wsl-ephemeral.ps1 -Action New -Command` used to print a warning over a failing
command and exit 0. It now exits with the command's own code, the same way
`-Action Run` always has.

- **Who it breaks.** Anything reading `New -Command` as a gate and getting a
  pass because it could not fail. The failure such a caller now sees is real,
  and it was there before: the step was reporting green over it.
- **Who was checked.** `Azathothas/TEMPLATE`, the only consumer in the
  register. Its wrapper forwards arguments and propagates the inner code, so it
  needs no edit beyond its pin.
  [`docs/consumers.md`](docs/consumers.md) carries the pin state.
- ⚠ **A pinned consumer does not get this until its pin moves**, which is a
  separate change in that repository.

The asymmetry is gone structurally rather than by convention: both actions run
the caller's command through one function, so there is no second place for a
code to be dropped.


### 2026-08-27T06:24:28Z: the BSD research, and the record moved into TODO

**Record:** [`TODO/PROGRESS.md`](TODO/PROGRESS.md).
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

- The record follows the todo model's own shape now: `TODO/` holding
  `PROGRESS.md`, `INDEX.md` and the entries by category, rather than two files
  at the root with every entry inlined into the index.
- `check-record` and its twin assert that the counts agree with the rows, in
  **both** the index and the record. ⚠ The record half was a gap a review found
  after the check had already reported clean once over exactly that drift.
- ⭐ The `BSD-01` research produced a shape different from the one requested,
  and the images half moved out entirely: `pkgforge-dev/docker-bsd` now builds
  FreeBSD, NetBSD, OpenBSD and DragonFly for amd64. Only FreeBSD publishes OCI
  images upstream, so three of the four are new.
- What is left here is one scripted VM guest, ranked against every alternative
  on friction, performance and interop in [`TODO/bsd.md`](TODO/bsd.md).


### 2026-08-27T04:58:19Z: repository created, and `wsl-ephemeral.ps1` decoupled from the template

**Record:** [`TODO/PROGRESS.md`](TODO/PROGRESS.md), the bootstrap entry.
**Deployed:** no deploy. This repository publishes files, not a service.

`Azathothas/ToolKit` was bootstrapped from `Azathothas/TEMPLATE` so that tools
used across many projects have one home, and so the template keeps only what
every project needs.

- `scripts/powershell-windows/wsl-ephemeral.ps1` moved here byte-for-byte from
  the template. ⛔ **No behaviour changed in the move**, so the diff is
  reviewable as a move. Its eleven known findings came with it, unfixed, and are
  tracked as `WSL-01` through `WSL-11` in [`TODO/INDEX.md`](TODO/INDEX.md).
- `scripts/powershell-windows/wsl-ephemeral.md` written, including an explicit
  list of those limits. A limit hidden is a defect filed against the user later.
- ⚠ **`Azathothas/TEMPLATE` now carries a wrapper at the same path**, which
  fetches this copy by pinned commit and verifies a SHA-256 before executing.
  Callers of the old path keep working. [`docs/consumers.md`](docs/consumers.md)
  is the register.
