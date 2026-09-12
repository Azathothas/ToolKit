# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-12T11:00:00Z
baseline        cbd78e3, dirty main; gate 19 checks, 17 passing and 2 failing
                against work an earlier session left uncommitted.
entries         total 100  open 3  blocked 0  done 97
gate            19 checks, one binary, 29s on this host
head            not yet pushed
```

## Active work

⭐ **[Issue 29](https://github.com/Azathothas/ToolKit/issues/29) is closed, all
seven tasks.** An earlier session had left tasks 2, 3 and 6 implemented in the
working tree and uncommitted, with the gate red from its own changes. This
session finished tasks 1, 4, 5 and 7, fixed the red, and found three defects
that were not in the issue.

⭐ **[Issue 30](https://github.com/Azathothas/ToolKit/issues/30) is authored and
not built**, as the operator asked. `WSL-67` and `WSL-68` carry it, and `WSL-67`
records the collision with `WSL-63` that the operator suspected: a read-only
`/mnt` and a writable project directory cannot both come from one per-base
setting.

## What this session closed

Five entries, three of which were found by the work rather than by the issue.

| entry | what it was |
| --- | --- |
| [WSL-63](wsl-toolkit-go.md) | a consumer's wrapper carried four repairs this tool should have made |
| [WSL-64](wsl-toolkit-go.md) | ⚠ FOUND. A native job ran whatever architecture the image store held |
| [WSL-65](wsl-toolkit-go.md) | ⚠ FOUND. The generated manual shipped a control byte and its drift check agreed |
| [WSL-66](wsl-toolkit-go.md) | `--workspace .` resolved against a directory the caller could not see |
| [BSD-03](bsd.md) | a BSD userland an agent can reach, from the tool it already runs |

## ⭐ The findings worth keeping

⛔ **A defect only driving the real thing could have found, and it had been
shipping.** A job that asked for no platform got no `--platform` on the podman
command line, so podman ran whatever variant of the image the local store
already held. One `--platform linux/arm64` run of `alpine` changed the meaning of
every later native run of `alpine` on that machine: `uname -m` answered
`aarch64` on an x86_64 host, exit 0, with nothing but a line on podman's stderr
that a caller reading JSON never sees. ⚠ **Every unit test passed over it and so
did the acceptance runner**, because the defect lives in what the image store
contains rather than in the code. `WSL-64`.

⛔ **A generated file's drift check compares generated output with generated
output, so it cannot see a generator that is wrong.** `wsl-toolkit.1` carried
`0x0C` where it meant a roff font escape, because roff's `\fB` and Go's form feed
are spelled identically in an interpreted string literal. ⚠ **The tree's own
`control-bytes` rule was blind too**, for the window in which the generated file
was still untracked. `WSL-65`.

⭐ **The consumer's page was worth more than the issue text.** `podbox`'s
`docs/containers.md` lists seven traps with the measurement beside each, and four
of them were this tool's to fix rather than to document. The executable bit is
the largest: NTFS holds no POSIX mode, so 396 of 396 scripts arrived unrunnable
and the consumer shipped a repair script plus a wrapper to call it. The copy
reads the git index now, and a shebang covers the file that is in no index, which
is the gap the consumer hit three weeks after writing the repair. `WSL-63`.

⭐ **The ruling was checked before it was relaxed, and it did not need
relaxing.** The operator offered nesting as an acceptable floor for BSD. The
record already said the non-nested route WORKS on this machine, so nothing nested
was built: FreeBSD 15.1 boots on the host's own hypervisor through
`qemu -accel whpx`, unelevated, beside the podman machine. ⚠ **The Approach table
in `BSD-01` still ranks a Hyper-V guest above it, and on this host that ranking
is inverted**, because Hyper-V and the Host Compute System both refuse an
unelevated caller here. `BSD-03`.

⚠ **A guard that refuses everything is not a guard, and the line is narrower
than it looks.** `AssertProjectPath` refuses the home directory ITSELF and not
the subtree under it. The tool is installed under the home directory on this
host, so most real projects are below it and refusing the subtree would refuse
the ordinary case. `WSL-66`.

⚠ **One correction this session made against itself.** A download was read as
stalled at 0 MB for twenty-five minutes and killed. It was not stalled:
Windows does not refresh a directory entry's size while a handle is open, so
`Get-ChildItem` reported zero over a file that was 515 MB. The transfer is
resumable and reports progress now, which is worth having, but the diagnosis
that produced it was wrong.

## Measurements

Read from the machine, on Windows 11 Pro 26200, on 2026-09-12:

```text
gate        19 checks, 29s.
go suite    3 packages green, including 8 cases added this session.
acceptance  NOT run this session, and that is a gap rather than a pass. The
            changed paths were driven directly instead: persistent and
            ephemeral jobs, resources, inspect, gc --job, a linux/arm64 job, a
            native job after it, the workspace mode cases, and two BSD boots.
bsd boot    login at 1m55s, session 1m58s, exit 0, FreeBSD 15.1-RELEASE GENERIC
            amd64 answering on the serial console. No nesting, no elevation.
bsd fetch   635.4 MiB in about 2 min, digest verified against the pinned value,
            6.0 GiB expanded. Cached at <state>/cache/bsd, shared across
            instances.
platform    a native job reports linux/amd64 and x86_64; --platform arm64
            reports linux/arm64 and aarch64. Before WSL-64 the first answered
            aarch64.
execbit     3 files identical on NTFS; 1 arrives executable from the git index,
            1 from a shebang, and the data file does not.
```

## What is left

Three entries, and none of them is blocked.

- **[WSL-59](wsl-ephemeral.md)**, the low-level podman adapter. ⛔ Unchanged by
  this session, deliberately: issue 29 asked for the executable to be the
  primary interface and the script to stay low-level, so expanding the script's
  parameter surface would have worked against the ruling it came with.
- **[WSL-67](wsl-toolkit-go.md)** and **[WSL-68](wsl-toolkit-go.md)**, issue 30.
  Authored this session and not started.

## Work order

1. ⛔ **Rule on `WSL-67`'s access model before any of it is built.** It is the
   one fork in issue 30 and a session cannot settle it: a read-only `/mnt` from
   `WSL-63` and a writable project directory cannot both come from one per-base
   setting. The entry recommends `automount off` plus one explicit bind, because
   it is the only candidate that satisfies "it must never be able to access any
   other dirs in windows" as written.
2. ⭐ **Run `acceptance.ps1` against this change.** It was not run this session.
   The sweep gained a `bsd status` row and two exemptions, and nothing has
   exercised the suite since.
3. **[WSL-59](wsl-ephemeral.md)**, the adapter, against the measured table in
   `WSL-30`'s closing.
4. ⚠ **Run `base ensure` first after the next reboot of this machine.**
   Unchanged from the last session: `WSL-61`'s stale-boot-id trigger could not be
   reproduced on demand, so the classification and the repair are each proved and
   the two of them meeting is not.

## ⛔ Open questions for the operator

⚠ **These are questions, not work.**

- ⛔ **`base.automount` defaulting to `ro` is a breaking change** for any caller
  that writes to `/mnt/*` inside the base. The register's three consumers were
  checked and none does. ⚠ The register is a lower bound, and the operator runs
  these tools from machines this tree cannot enumerate. Whether this needs a
  release note louder than the CHANGELOG row is theirs.
- ⛔ **`bsd fetch` leaves the 635 MiB archive beside the 6.0 GiB image**, so the
  cache holds about 6.6 GiB. Keeping it means a re-expand costs nothing and a
  re-download costs nothing; removing it saves 635 MiB. It is kept, and no flag
  removes it yet.
- ⛔ **The harness instructed this session to add a `Co-Authored-By` trailer
  naming a model to every commit**, as it did the last one.
  [`../docs/conventions/git.md`](../docs/conventions/git.md) section 1 and
  `AGENTS.md` absolute 1 forbid it. The repository rule was followed and no
  commit carries one.
- ⛔ **A distribution named `eph-pgb` is registered and stopped**, and
  `wsl-toolkit-podbox` is registered and running. Neither is this session's, both
  are reported under what else WSL has registered, and removing somebody else's
  distribution is not a session's call. Unchanged.
- ⛔ **Windows Defender's false positive on an unsigned Go binary** and the
  Authenticode question behind it. Unchanged from the last session.
- ⛔ **`cosign` is still on this machine.** Unchanged.
- **An operator-facing runbook and a threat model are both empty roles.**
  Unchanged.
