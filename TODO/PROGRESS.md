# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-14T09:18:24Z; the record commit's own time is its end
baseline        eff07c5, tree clean, CI run 34820474937 green
entries         total 119  open 10  blocked 0  done 109
closed          WSL-81 at e32791a; WSL-72 at 461269f; WSL-77 at c016d5c
filed           WSL-82 by WSL-81's claim audit; WSL-83 by WSL-72's baseline read; WSL-84 by WSL-71's measurement
head            360bbde, then this record commit
```

## Active work

⭐ **The operator checkpointed this session on 2026-09-14 at about 11:00Z**, and set
the next one. `WSL-81`, `WSL-72` and `WSL-77` are closed. `WSL-78`'s entry point,
`base agent` and the `muse.exe` launcher, is built and driven without a sign-in, and
stays open. `WSL-71` has its premise measured and nothing built. `WSL-82`, `WSL-83`
and `WSL-84` are filed and wait for rulings.

**Next: one dedicated session, Muse and herdr together**, as the operator set it in
the ruling below. `WSL-76` carries the operator's words.

## The work order, set by the operator on 2026-09-13 and 2026-09-14

Each entry's section in [`wsl-toolkit-go.md`](wsl-toolkit-go.md) carries its
premise, decisions and prove. An issue closes only when every entry mapped to it
is closed, and the issue gets a comment naming the commits.

1. ⭐ **Closed:** `WSL-74`, `WSL-80`, `WSL-79` with issue 33, `WSL-75`, `WSL-81`,
   `WSL-72` and `WSL-77`.
2. **Muse and herdr together, one dedicated session.** Build `wsl-toolkit-base` from
   [`../tools/windows/wsl-toolkit/examples/muse-code/wsl-toolkit-base.json`](../tools/windows/wsl-toolkit/examples/muse-code/wsl-toolkit-base.json);
   the operator runs `muse login` in it; then `WSL-76`'s and `WSL-78`'s open items,
   and `pi` and `omp` driving Muse through herdr, authored before they are built.
3. **`WSL-68`, the sealed base: a dedicated session of its own**, after 2.
4. **The rest of issue 30:** `WSL-67`'s open items, `WSL-70` and `WSL-71`.
5. **`wsl-toolkit-v3.0.0`** is cut once issues 30 and 32 are closed and CI is green
   on the final commit.

⚠ **`WSL-82`, `WSL-83` and `WSL-84` are in no issue and not in this order.** Each waits
for the operator's approval and ruling, below.

## Rulings in force

1. **2026-09-13: herdr replaces Zellij** as the agents' multiplexer, and tmux
   stays a generic fallback. `WSL-76` quotes the operator.
2. **2026-09-13: the FreeBSD guest's default disk** rises to the smallest whole
   number of GiB, 12 or 13, that gives a true 10 GiB root, measured with `df -k /`.
   In `WSL-72`.
3. **2026-09-13: the `bsd run` line join is a task in `WSL-79`**, not an entry.
4. **2026-09-14: one base for every agent.** `wsl-toolkit-base`, the instance
   `base`, configured under the operator's account, with the Linux account
   `herdr`, passwordless sudo and no standing grant. Test grants name directories
   under this repository's `.tmp`. In `WSL-75`.
5. **2026-09-14, the entries' decisions:** `WSL-75` live grants of a project or of
   a parent directory; `WSL-76` OpenSSH through `wsl.exe` with nothing listening
   and only the herdr client's key accepted, and herdr's background checks left
   on; `WSL-77` adapters in the tree with a generated embedded copy; `WSL-78`
   agents run in the base and Windows gets launchers; `WSL-79` document the boot
   cost, and keep a guest running only if tuning leaves more than 30 seconds;
   `WSL-68` the internet only with the host refused, and only with no rule in the
   shared network namespace; `WSL-67` drive `pkgin` and `pkg_add`, and remove a
   manager that cannot be driven here.
6. **2026-09-14: one Muse installer digest is approved**, `5196d820…632a0ca`, in
   `WSL-77`.
7. **2026-09-14: host changes the agent may make:** a dedicated SSH key and one
   marked `Host` block in `%USERPROFILE%\.ssh\config`, and agent launchers in
   `%USERPROFILE%\bin`.
8. **2026-09-14: cut `wsl-toolkit-v3.0.0` at the end of this session**, after
   `repo release` passes read-only, and only with CI green on the final commit.
   ⚠ Its condition was not met by the session it named: issues 30 and 32 are open.
9. **2026-09-14: Muse and herdr together are the next session's one task**, and the
   sealed base a dedicated session after it. In `WSL-76`.

## Before every push

A green local gate is not CI. Run the Go tests with `TEMP` and `TMP` at an 8.3
short path, then CI's Linux Go job and its ShellCheck in containers.
[`../tools/windows/wsl-toolkit/README.md`](../tools/windows/wsl-toolkit/README.md)
carries all three as commands under "Build and local proof". ⚠ **A heavy BSD run goes
on a fresh image copy**, with `WSL_TOOLKIT_CACHE` under this repository's `.tmp`,
never on the shared image. ⚠ **An untracked script is outside CI's ShellCheck
command**, which lists `git ls-files`, so check a new one by name before it is added.

## Done this session

| commit | what |
| --- | --- |
| `e32791a` | `WSL-81` closed: 8 mutation rows red one at a time; a step the guest never finished no longer prints `(exit 0)`, with a case and a row; the manual's per-run cost measured again on the one-processor build, about 23 s; `WSL-82` filed for a payload that prints a panic's two lines |
| `461269f` | `WSL-72` closed: the prove as written passed on a fresh image copy, exit 0, `absent=` empty, `nim` and `rustc` present, a 10.6 GiB root; 4 rows red. `WSL-83` filed after two panics at poweroff on the shared image, which is restored; the manual, two comments and `WSL-81`'s record corrected where they said such a panic comes after the buffers sync |
| `d8c8328` | `WSL-77` built: the `muse` adapter runs Meta's installer only while its digest is approved, `installer_sha256` carries an operator's approval, `/usr/local/bin/muse` serves `base exec`, and `wsl-toolkit-base.json` is the one base's profile; 3 cases, 6 rows red |
| `c016d5c` | `WSL-77` closed: the prove from a fresh clone on a throwaway base, the planted stop and the approval by configuration; the door sweep's loading row, 7 rows in all; the manual's Muse table, and three sentences the claim audit corrected |
| `360bbde` | `WSL-78` partial: `base agent` and the `muse.exe` launcher, driven on a throwaway base from a granted project, an ungranted directory and a stale build; 6 cases, 7 rows red |
| this record commit | the operator's order for the next two sessions; `WSL-71`'s starting directory measured; `WSL-84` filed; `WSL-78`'s door sweep and claim audit, and the manual's three sentences they corrected; this record and the summary |

## Measurements

On Windows 11 Pro 26200, WSL 2.7.12, on 2026-09-14:

- **At the start:** the doctor exit 0 in 45.08 s at 09:19:45Z; the gate exit 0,
  20 checks green in 46.78 s at 09:20:48Z. `wsl -l -v` matched the host state
  below. CI run 34820474937 for `eff07c5` finished green.
- **WSL networking:** NAT mode, host address `172.23.96.1`, read by `wsl-toolkit
  hostaddress`. Two distributions share one network namespace, in `WSL-68`.
- **herdr:** 0.9.0 is the latest stable release, published 2026-09-07, read on
  2026-09-13. `herdr-linux-x86_64` is 24,644,488 bytes and
  `herdr-windows-x86_64.zip` 9,054,745 bytes, each with a published SHA-256 digest.
- **Muse:** Meta's installer at `dev.meta.ai` still has the approved digest, 9,314
  bytes, on 2026-09-14; the public channel serves `1.2.1-R2847.1`, a 299,251,896-byte
  Linux x86 build that answers without credentials.
- **For `WSL-81`'s closing:** the Go suites green on Windows with `TEMP` at the 8.3
  path, 280 top-level `wsl-toolkit` cases, 276 passed, 4 skipped; in `golang:1.25`,
  279, 278 passed, 1 skipped; ShellCheck 0.9.0 in `ubuntu:24.04` clean over every
  tracked script. Three `bsd run -c true` runs with one processor: a login at 8.3 s
  to 8.5 s and the process gone at 23.0 s to 23.3 s, with no panic.
- **For `WSL-72`'s closing:** the prove exit 0 in 178.8 s on a fresh copy, whose
  first boot logged in at 29 s. `bsd fetch --force` restored the shared image in
  13.1 s.
- **For `WSL-77`'s closing:** from a fresh clone at `d8c8328` on the throwaway
  `m77`: `base ensure` from nothing exit 0 in 77.6 s; a differing pin exit 2 in
  4.4 s; approved by configuration exit 0 in 5.6 s; over an installed Muse exit 0 in
  3.8 s; `base remove --yes` exit 0 in 10.5 s.
- **For `WSL-78`'s entry point:** on the throwaway `m78`, `base ensure` exit 0 in
  76.6 s writing a 14,200,320-byte launcher; the launcher exit 0 in the granted
  project and beneath it, exit 2 elsewhere and with no argument, and Muse's own exit
  2 forwarded; a stale launcher named by the probe and rewritten by `base ensure`.
- ⛔ **For `WSL-83`:** two read-only runs on the shared image, one processor, both
  exit 0, both panicked at poweroff: `bad pte va 389278400000 pte 0` before `Syncing
  disks`, then, on the next boot's unchecked root, `initiate_write_filepage: dir inum
  0 != new 160513`.
- ⛔ **For `WSL-84` and `WSL-71`:** on the throwaway `m71`, a base reconfigured from
  `rw` to `ro` kept `/mnt/c` mounted `9p rw` and reported `ro`; with automount off,
  WSL starts a command in the account's home; with it on, in the Windows directory.
  The default `wsl-toolkit` base reads `9p ro`.

## Found, and not filed

1. ⛔ `repo mutate` never runs a row's cases unmutated, so a case already red
   reports "went red".
2. ⛔ The gate's `powershell` check cannot fail: its producer uses pipe
   separators and its Go reader looks for tabs. Until fixed, parse every edited
   PowerShell file independently.
3. ⛔ `scripts/common/check.ps1` resolves the repository from the working
   directory rather than its own location.
4. `bootstrap.sh --dry-run --toolset agent --codegraph none --json` exits 1 on
   Debian 13; its cause has not been read.
5. A deterministic regression for pre-marker base rollback is still owed.
6. The generated manual prints a one-letter flag as `--c`, which the parser
   accepts and no example writes.
7. `examples/muse-code/README.md` and `examples/common/zellij.md` describe Zellij
   and `wsl-toolkit-muse`, which changes with `WSL-76`.
8. ⚠ **An ephemeral job says its output is kept, and it is not.** `run
   --container-lifecycle ephemeral` against `golang:1.25` printed `the complete
   output is kept: wsl-toolkit logs 4e94faa38b4c9c1c`; `logs` on that id answered
   exit 2, `no transcript`, and `jobs\4e94faa38b4c9c1c` was not in the state
   directory. Measured on 2026-09-14, and not yet read in the code.
9. ⚠ **Every `bsd run` leaves its console lines in the shared image.** The guest's
   root shell keeps history: `/root/.sh_history` read 40,215 bytes in one run on
   2026-09-14 and 42,825 bytes two runs later.
10. ⚠ **A cancelled `bsd run` is reported as a budget that ran out.** Read in
    `bsd.go` on 2026-09-14 and not measured: a cancelled context ends the boot's wait
    with `the guest did not reach a login prompt within` the budget, and a command's
    with `the guest did not finish the command within the budget`, both exit 2, where
    `distro run` answers a cancellation with 130.

## Review findings

Each closed entry carries its three reviews. What this session's changed: `WSL-81`'s
claim audit found a grow a panic ended printing `(exit 0)`, the manual's per-run cost
measured on the two-processor build, and a payload's copy of a panic read as the
kernel's, which is `WSL-82`. `WSL-72`'s claim audit found the manual silent about a
fresh image's first boot. `WSL-77`'s door sweep found no case holding a field inside
a stored adapter across a load, and its claim audit found the manual saying the
operator had read the installer. `WSL-78`'s claim audit found the manual promising
every argument arrives as written, which no drive reached end to end. Each is fixed
or filed in the entry's own commit, `WSL-78`'s in this record commit.

## Open questions for the operator

1. **`WSL-84`, P1: approve the entry, and say where it goes in the order.**
   Recommended: before `WSL-68`, because a base whose drives drift from `rw` to `ro`
   stays writable while it reports read-only, and the sealed base's proof reads the
   same verifier.
2. **`WSL-83`: approve the entry, and rule A, B or C.** What protects the shared FreeBSD
   image from a panic at poweroff: **A, a throwaway overlay per run, recommended**,
   which ends a guest one session configures for the next; B, the image writable,
   with `sync` before `poweroff` and a run refused on a boot that shows its root not
   properly dismounted; C, a warning only.
3. **`WSL-82`: approve the entry, and rule A, B or C.** What ends a command's wait
   once the console shows a panic's two lines: A, at once, as now; **B, QEMU's exit,
   the command's closing marker, or 60 seconds, recommended**; C, QEMU's exit alone.
4. **Does `wsl-toolkit-v3.0.0` wait for `WSL-83` or `WSL-84`?** Recommended: for
   `WSL-84`, which is a read-only promise the release makes, and not for `WSL-83`,
   whose limit the manual carries and `bsd fetch --force` repairs in seconds.

## Host state

- Registered distributions: `podman-machine-default`, `eph-pgb`, `wsl-toolkit`
  and `wsl-toolkit-podbox`, as at the session's start. `eph-pgb` keeps its disk
  under `%LOCALAPPDATA%\wsl-ephemeral` and is not this tool's.
- ⭐ **No throwaway is left.** `wsl-toolkit-m77`, `-m78` and `-m71` were removed with
  their instance directories, and `instances` holds `acc`, `muse`, `nobase`, `podbox`
  and `podbox-migrate`, as at the start. `instances\muse` holds nothing.
- herdr 0.9.0 is installed on Windows by the operator. `%USERPROFILE%\bin` holds no
  `muse.exe`: every launcher this session wrote went under `.tmp`, and is removed.
- The dedicated SSH key the ruling allows is at
  `%USERPROFILE%\.ssh\wsl-toolkit\id_ed25519`, with an empty `known_hosts` beside it,
  and stays for `wsl-toolkit-base`. `%USERPROFILE%\.ssh\config` reads SHA-256
  `18FC11BE…E93F4`, unchanged through the session.
- ⛔ **The shared FreeBSD image is the published one**, 6,476,638,208 bytes, restored
  by `bsd fetch --force` at 09:46:42Z after `WSL-83`'s panics, and not booted since.
  The next run grows it to 12 GiB. No image copy remains under `.tmp`.
