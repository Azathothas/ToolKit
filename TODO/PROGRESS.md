# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-14T12:35:13Z; the record commit's own time is its end
baseline        08e23bc, tree clean, CI run 34837219729 green
entries         total 119  open 10  blocked 0  done 109
closed          WSL-81 at e32791a; WSL-72 at 461269f; WSL-77 at c016d5c
approved        WSL-82 option B; WSL-83 option A; WSL-84 first in the order
head            08e23bc, then this record commit
```

## Active work

⭐ **The operator changed the work order on 2026-09-14.** Finish the open safety
and correctness entries, then finish issue 30, before returning to Muse and herdr.
This authoring session implements none of those entries. `WSL-78`'s entry point,
`base agent` and the `muse.exe` launcher remain built and driven without a sign-in.

**Next: `WSL-84`, then `WSL-83`, then `WSL-82`.** Their decisions are ruled below
and in their entries. No planning question remains before implementation starts.

## The work order, set by the operator on 2026-09-14

Each entry's section in [`wsl-toolkit-go.md`](wsl-toolkit-go.md) carries its
premise, decisions and prove. An issue closes only when every entry mapped to it
is closed, and the issue gets a comment naming the commits.

1. ⭐ **Closed:** `WSL-74`, `WSL-80`, `WSL-79` with issue 33, `WSL-75`, `WSL-81`,
   `WSL-72` and `WSL-77`.
2. **`WSL-84`, the read-only drive verifier.** A base reports the drive mode it has,
   and a changed mode is applied before later base work relies on it.
3. **`WSL-83`, a throwaway FreeBSD overlay per run.** Protect the shared image before
   another BSD proof boots it.
4. **`WSL-82`, the panic detector.** Wait for QEMU to exit, the command marker or the
   60-second bound after the two panic lines appear.
5. **Finish issue 30's existing package and profile work:** `WSL-67`, then `WSL-70`,
   then `WSL-71`. `WSL-67` drives `pkgin` and `pkg_add`, removes Soar, keeps and
   drives Nix, and adds the provider-profile cases to the acceptance runner.
6. **`WSL-68`, the sealed base: a dedicated session of its own.** It closes issue
   30 after the work above, and uses the verifier fixed by `WSL-84`.
7. **Muse and herdr together, one dedicated session.** Build `wsl-toolkit-base` from
   [`../tools/windows/wsl-toolkit/examples/muse-code/wsl-toolkit-base.json`](../tools/windows/wsl-toolkit/examples/muse-code/wsl-toolkit-base.json);
   the operator runs `muse login` in it; then close `WSL-76` and `WSL-78`. Author
   approved entries for `pi` and `omp` before either adapter is built.
8. **`wsl-toolkit-v3.0.0`** is cut after issues 30 and 32 and `WSL-82`, `WSL-83` and
   `WSL-84` are closed, and CI is green on the final commit.

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
8. **2026-09-14: cut `wsl-toolkit-v3.0.0` only after the order above is complete.**
   Issues 30 and 32 and `WSL-82`, `WSL-83` and `WSL-84` are closed first. Run `repo
   release` read-only, and publish only with CI green on the final commit.
9. **2026-09-14: finish existing work before Muse and herdr.** Keep Muse and herdr
   together in one later dedicated session. Complete the sealed base before it. In
   `WSL-76`.
10. **2026-09-14: the three new entries are approved.** `WSL-84` is first.
    `WSL-83` uses option A, a throwaway overlay per BSD run. `WSL-82` uses option B,
    which waits for QEMU's exit, the command marker or 60 seconds.
11. **2026-09-14: remove Soar and keep Nix.** `WSL-67` removes Soar from the
    bootstrap and its live documentation, and drives Nix as the one user-level
    provider.

## Before every push

A green local gate is not CI. Run the Go tests with `TEMP` and `TMP` at an 8.3
short path, then CI's Linux Go job and its ShellCheck in containers.
[`../tools/windows/wsl-toolkit/README.md`](../tools/windows/wsl-toolkit/README.md)
carries all three as commands under "Build and local proof". ⚠ **A heavy BSD run goes
on a fresh image copy**, with `WSL_TOOLKIT_CACHE` under this repository's `.tmp`,
never on the shared image. ⚠ **An untracked script is outside CI's ShellCheck
command**, which lists `git ls-files`, so check a new one by name before it is added.

## Recent work

| commit | what |
| --- | --- |
| `e32791a` | `WSL-81` closed: 8 mutation rows red one at a time; a step the guest never finished no longer prints `(exit 0)`, with a case and a row; the manual's per-run cost measured again on the one-processor build, about 23 s; `WSL-82` filed for a payload that prints a panic's two lines |
| `461269f` | `WSL-72` closed: the prove as written passed on a fresh image copy, exit 0, `absent=` empty, `nim` and `rustc` present, a 10.6 GiB root; 4 rows red. `WSL-83` filed after two panics at poweroff on the shared image, which is restored; the manual, two comments and `WSL-81`'s record corrected where they said such a panic comes after the buffers sync |
| `d8c8328` | `WSL-77` built: the `muse` adapter runs Meta's installer only while its digest is approved, `installer_sha256` carries an operator's approval, `/usr/local/bin/muse` serves `base exec`, and `wsl-toolkit-base.json` is the one base's profile; 3 cases, 6 rows red |
| `c016d5c` | `WSL-77` closed: the prove from a fresh clone on a throwaway base, the planted stop and the approval by configuration; the door sweep's loading row, 7 rows in all; the manual's Muse table, and three sentences the claim audit corrected |
| `360bbde` | `WSL-78` partial: `base agent` and the `muse.exe` launcher, driven on a throwaway base from a granted project, an ungranted directory and a stale build; 6 cases, 7 rows red |
| `08e23bc` | the operator's former order for the next two sessions; `WSL-71`'s starting directory measured; `WSL-84` filed; `WSL-78`'s door sweep and claim audit, and the manual's three sentences they corrected; the record and the summary |
| this record commit | `WSL-82`, `WSL-83` and `WSL-84` approved; the existing work restored ahead of Muse and herdr; Soar set for removal and Nix retained; no entry implemented |

## Measurements

On Windows 11 Pro 26200, WSL 2.7.12, on 2026-09-14:

- **At the start of this authoring session:** the doctor exited 0 at 12:36:05Z;
  the gate passed all 20 checks; the four registered distributions matched the host
  state below; CI run 34837219729 for `08e23bc` was green.
- **For this authoring change:** the Windows Go proof passed with `TEMP` and `TMP`
  at the repository's 8.3 path; the Linux Go proof passed in `golang:1.25`;
  ShellCheck 0.9.0 passed in `ubuntu:24.04`; the final gate passed all 20 checks.
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

This authoring session's door sweep followed the order through the live record, the
entries and the newest summary. It found that the previous summary still routed a
reader to Muse, as that historical snapshot must; the new summary now supersedes it
with `WSL-84`. The guard mutation changed the recorded total from 119 to 118. The
record check exited 1 and named the 119 rows in `INDEX.md`; the true value is restored
and the check is green. The claim audit found that the new summary's first wording
could say Soar was already removed. It now says `WSL-67` requires that future work.

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

None. The operator approved the three entries and their recommended options on
2026-09-14, placed them in the work order, and made the release wait for both P1
entries. The later Muse session still needs the operator to run `muse login`; that is
an action at its stated checkpoint, not a planning question.

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
