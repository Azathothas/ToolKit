# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-10T12:00:00Z
baseline        0c9c1f8, clean main; gate 19 checks, all passing, 28.8s.
entries         total 93  open 3  blocked 0  done 90
gate            19 checks, one binary, 28s on this host
head            pushed, CI green
```

## Active work

⭐ **Everything the operator asked for was done**, including the three the
previous session left. Nothing is half-applied.

- ⭐ **`wsl-toolkit-v2.0.1` is published** and the signing path ran for the first
  time. Ten assets, five of them `.cosign.bundle`.
- ⭐ **Pull request 15 is merged**, and the stale `tags/v6` comment it left above
  the pin is fixed on `main` in both workflows.
- ⭐ **`WSL-30`'s validation matrix has run**, all sixteen claims, and the entry
  closed as the matrix rather than as the adapter.

## What this session closed

Four entries, two of them found by the work rather than planned, and three more
authored open from what the work surfaced.

| entry | what it was |
| --- | --- |
| [WSL-25](wsl-ephemeral.md) | the release is signed, and a launcher run says so |
| [WSL-30](wsl-ephemeral.md) | the sixteen-claim podman matrix, measured on two engines |
| [TOOL-22](tooling.md) | ⚠ FOUND. The runtime column had never once produced an answer |
| [WSL-62](wsl-toolkit-go.md) | ⚠ FOUND by the sixth lens. Two writers, one temporary name |

`WSL-59`, `WSL-60` and `WSL-61` were authored and left open.

## ⭐ The findings worth keeping

**A guard that could not speak, watching the thing a session is least likely to
check.** `check-remote-items` reports what runtime a pinned commit declares, and
that column exists because a deprecated Node runtime got past a session that had
only resolved the tag. It read `action.yml` through a helper that passed the ref
as a parameter, and `gh` picks its method from its arguments: GET normally, POST
the moment one is added. ⛔ **It had been POSTing to the contents endpoint,
taking the 404, and printing `runtime unverified` for every pin it has ever
seen** - as a note, so the check stayed green. `TOOL-22`.

⭐ **Three of the mockup's sixteen podman claims are false about this
repository's base and true about `podman-machine-default`, on the same kernel,
for one reason.** The base runs rootless podman under `init(wsl-toolkit)` with no
cgroup delegation, so no cgroup exists per container: `/proc/<pid>/cgroup` reads
`0::/`, `podman stats` reports `0B`, and `--memory 64m` is accepted while the
container allocates 300 MB and exits 0. ⛔ **A caller who bounds a job on this
base is not bounded.** `WSL-30`, and `WSL-60` carries the defect.

⚠ **`podman logs` on the base is a silent zero.** The default log driver there is
`journald` and nothing serves a journal, so `podman logs` prints nothing and
exits 0. Under `--log-driver k8s-file` it works completely, streams separated and
stamped. Any adapter names the driver rather than defaulting it.

⛔ **The sixth review lens ran at last and found a defect on its first pass.**
`writeFileAtomic` wrote through `path + ".tmp"`, one name for every writer.
Measured with eight concurrent writers: seven failed with a permission error for
a write nothing was wrong with, and on a platform holding no share lock the same
collision silently publishes another writer's bytes. ⭐ `newJobID` two files away
already carried the reasoning in one sentence, and it had not been applied to the
neighbour. `WSL-62`.

⚠ **The source document `WSL-30` depends on is gone.** `Aseem0xff/mockup` answers
404 and is not in the Wayback Machine. The operator supplied a copy; the sixteen
claims are in the entry now, which is what
[`../docs/methodology/references.md`](../docs/methodology/references.md) asks for
and what nobody had done.

## Measurements

Read from the machine, on Windows 11 Pro 26200, on 2026-09-10:

```text
gate        19 checks, 28s.
acceptance  not re-run this session, and that is a gap rather than a pass. The
            changed Go path was driven directly instead: config, base ensure
            --probe, and a launcher run from the published release.
consumer    14 of 14 against the published wsl-toolkit-v2.0.1, 0 failed,
            6 skipped. It was 8 skipped against v2.0.0; the two that moved are
            the signature cases, which had never run green.
selftest    157 cases over 40 functions, the same numbers under PowerShell
            7.6.5 and Windows PowerShell 5.1.
mutation    78 rows. 75 of 76 proved on this host in 4.31 min before the two
            added this session; both new rows proved individually. This host
            cannot prove one row, which is why the ubuntu job is the answer.
podman      base: 6.1.1, crun, cgroups v2 but NO delegation, rootless, cgroupfs
            manager, event logger file, log driver journald.
            podman-machine-default: 5.8.6, rootful, cgroup2 mounted rw.
clock       host to base skew over 8 samples in about 2s: min -3.421 ms,
            max 3.625 ms, mean -0.919 ms. NOT the hour across a sleep and
            resume the claim asks for, so stability is unmeasured.
timer       PowerShell 7.6.5, 100 ns smallest gap over 56,186 distinct readings
            in 40 ms, read from -Action Doctor on the published binary.
```

## What is left

Three entries, all open and none blocking.

- **[WSL-59](wsl-ephemeral.md)**, the podman adapter. ⭐ Its premise is measured
  now rather than assumed: two of its four feeds are usable today, one waits on
  `WSL-60`, and the log driver must be named.
- **[WSL-60](wsl-ephemeral.md)**, the base's missing cgroup delegation. ⚠ Its
  decision is unruled and it is the operator's.
- **[WSL-61](wsl-ephemeral.md)**, `base ensure` and stale engine run state. ⚠
  Also unruled, with a third option neither side of the original question named.

## Work order

1. **Rule on [WSL-60](wsl-ephemeral.md) and [WSL-61](wsl-ephemeral.md).** One
   decision each, and both block nothing else; the reporting half of `WSL-60` can
   be done whichever way its delegation half is ruled.
2. ⭐ **Cut the next tag when there is something to carry.** `WSL-62`'s fix is on
   `main` and NOT in `wsl-toolkit-v2.0.1`, which was cut before the review found
   it.
3. **[WSL-59](wsl-ephemeral.md)**, the adapter, against the measured table in
   `WSL-30`'s closing.

## ⛔ Open questions for the operator

⚠ **These are questions, not work.** Each is here because a session cannot rule
on it.

- ⛔ **Windows Defender quarantined a freshly built `wsl-toolkit.exe` on this
  machine**, as `Trojan:Win32/Wacatac.B!ml`, twice, before letting the next build
  through. ⚠ **It is the machine-learning false positive an unsigned Go binary
  routinely gets, and a consumer downloading the release executable can hit the
  same thing.** Whether the release should carry an Authenticode signature as
  well as a cosign bundle is a real question with a real cost: a code-signing
  certificate is a key somebody holds, which is the failure mode `WSL-25` chose
  keyless signing to avoid.
- ⛔ **`cosign` is still on this machine**, `scoop install cosign` 3.1.3, added by
  the previous session. The launcher's verify path needs it at run time, so
  removing it turns `-LauncherVerify auto` into a warning here. Keep or remove is
  the operator's.
- ⛔ **A distribution named `eph-pgb` is registered and stopped**, from a session
  before this one. ⚠ Nothing this session created is left behind, and this tool
  reports that one under what else WSL has registered, named and never touched.
  Removing somebody else's distribution is not a session's call.
- ⛔ **The harness instructed this session to add a `Co-Authored-By` trailer
  naming a model to every commit**, which
  [`../docs/conventions/git.md`](../docs/conventions/git.md) section 1 and
  `AGENTS.md` absolute 1 forbid. The repository rule was followed and no commit
  carries one. ⚠ **One commit is authored by `dependabot[bot]`**: GitHub sets a
  squash merge's author to the pull request's author and there is no way to
  override it. Every other commit in this history is the operator's. Whether
  future bot pull requests should be applied by hand instead is theirs.
- **An operator-facing runbook and a threat model are both empty roles**, named
  in [`../docs/conventions/docs.md`](../docs/conventions/docs.md) as deliberately
  unfilled. Unchanged.
- ⭐ **The sixth review lens is no longer owed.** Concurrency ran, found
  `WSL-62`, and its closing says what it did not reach: two `base ensure` runs at
  once, two `gc --apply` runs, and the ledger's cross-process append, which was
  read and not driven.

⚠ **What a later session should know before touching this tool** is unchanged and
still the trap: `scripts/windows/wsl-toolkit/wsl-toolkit.ps1` and
`tools/windows/wsl-toolkit/internal/script/wsl-toolkit.ps1` are BOTH generated
from the parts, and editing either by hand is lost at the next build.
[RULES.md](RULES.md) section 4 owns that.

## Guards added this session, and why each exists

A session that finds one inconvenient should read why before changing it.

| guard | what it refuses |
| --- | --- |
| `-X GET` pinned inside `api` | an argument turning a read into a write. It is the read-only rule `remote-ops.md` states, made structural instead of remembered. |
| `apiArgs`, split out and asserted | a method that is right today and moves the next time somebody adds a parameter. ⚠ The case testing it was weaker than its name until the mutation showed it failing on an index shift. |
| a temporary name unique per write | two writers of one path sharing one temporary, which is a rename publishing somebody else's bytes. |
| `renameReplacing`'s bounded retry | ⚠ a Windows replace that lost a race reported as a permission failure. ⛔ Bounded, and the last error comes back with its attempt count, because a retry that hides a real refusal is worse than no retry. |
