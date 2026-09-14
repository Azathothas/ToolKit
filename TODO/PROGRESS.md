# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-14T14:15:42Z; the record commit's own time is its end
baseline        7122216, tree clean, CI run 34846522263 green
entries         total 119  open 7  blocked 0  done 112
closed          WSL-84 at 191062a; WSL-83 at b9a8ea2; WSL-82 in its own commit
approved        none waiting
head            b9a8ea2, then the WSL-82 commit
```

## Active work

⭐ **The session works the operator's order unattended.** `WSL-84`, `WSL-83` and
`WSL-82` are closed. **Next: issue 30's `WSL-67`**, then `WSL-70` and `WSL-71`. Their
decisions are ruled below and in their entries. No planning question remains.

`WSL-78`'s entry point, `base agent` and the `muse.exe` launcher remain built and
driven without a sign-in.

## The work order, set by the operator on 2026-09-14

Each entry's section in [`wsl-toolkit-go.md`](wsl-toolkit-go.md) carries its
premise, decisions and prove. An issue closes only when every entry mapped to it
is closed, and the issue gets a comment naming the commits.

1. ⭐ **Closed:** `WSL-74`, `WSL-80`, `WSL-79` with issue 33, `WSL-75`, `WSL-81`,
   `WSL-72`, `WSL-77`, `WSL-84`, `WSL-83` and `WSL-82`.
2. **Finish issue 30's existing package and profile work:** `WSL-67`, then `WSL-70`,
   then `WSL-71`. `WSL-67` drives `pkgin` and `pkg_add`, removes Soar, keeps and
   drives Nix, and adds the provider-profile cases to the acceptance runner.
3. **`WSL-68`, the sealed base: a dedicated session of its own.** It closes issue
   30 after the work above, and uses the drive verifier `WSL-84` fixed.
4. **Muse and herdr together, one dedicated session.** Build `wsl-toolkit-base` from
   [`../tools/windows/wsl-toolkit/examples/muse-code/wsl-toolkit-base.json`](../tools/windows/wsl-toolkit/examples/muse-code/wsl-toolkit-base.json);
   the operator runs `muse login` in it; then close `WSL-76` and `WSL-78`. Author
   approved entries for `pi` and `omp` before either adapter is built.
5. **`wsl-toolkit-v3.0.0`** is cut after issues 30 and 32 are closed, and CI is green
   on the final commit.

## Rulings in force

1. **2026-09-13: herdr replaces Zellij** as the agents' multiplexer, and tmux
   stays a generic fallback. `WSL-76` quotes the operator.
2. **2026-09-14: one base for every agent.** `wsl-toolkit-base`, the instance
   `base`, configured under the operator's account, with the Linux account
   `herdr`, passwordless sudo and no standing grant. Test grants name directories
   under this repository's `.tmp`. In `WSL-75`.
3. **2026-09-14, the entries' decisions:** `WSL-75` live grants of a project or of
   a parent directory; `WSL-76` OpenSSH through `wsl.exe` with nothing listening
   and only the herdr client's key accepted, and herdr's background checks left
   on; `WSL-77` adapters in the tree with a generated embedded copy; `WSL-78`
   agents run in the base and Windows gets launchers; `WSL-68` the internet only
   with the host refused, and only with no rule in the shared network namespace;
   `WSL-67` drive `pkgin` and `pkg_add`, and remove a manager that cannot be driven
   here.
4. **2026-09-14: one Muse installer digest is approved**, `5196d820…632a0ca`, in
   `WSL-77`.
5. **2026-09-14: host changes the agent may make:** a dedicated SSH key and one
   marked `Host` block in `%USERPROFILE%\.ssh\config`, and agent launchers in
   `%USERPROFILE%\bin`.
6. **2026-09-14: cut `wsl-toolkit-v3.0.0` only after the order above is complete.**
   Issues 30 and 32 are closed first. Run `repo release` read-only, and publish only
   with CI green on the final commit.
7. **2026-09-14: finish existing work before Muse and herdr.** Keep Muse and herdr
   together in one later dedicated session. Complete the sealed base before it. In
   `WSL-76`.
8. **2026-09-14: remove Soar and keep Nix.** `WSL-67` removes Soar from the
   bootstrap and its live documentation, and drives Nix as the one user-level
   provider.

## Before every push

A green local gate is not CI. Run the Go tests with `TEMP` and `TMP` at an 8.3
short path, then CI's Linux Go job and its ShellCheck in containers.
[`../tools/windows/wsl-toolkit/README.md`](../tools/windows/wsl-toolkit/README.md)
carries all three as commands under "Build and local proof". ⚠ **A BSD run no longer
writes the shared image**, so a heavy run may boot it; a run that needs a damaged or
altered image goes on a copy, with `WSL_TOOLKIT_CACHE` under this repository's `.tmp`.
⚠ **An untracked script is outside CI's ShellCheck command**, which lists `git
ls-files`, so check a new one by name before it is added.

## Recent work

| commit | what |
| --- | --- |
| `d8c8328` | `WSL-77` built: the `muse` adapter runs Meta's installer only while its digest is approved, and `wsl-toolkit-base.json` is the one base's profile |
| `c016d5c` | `WSL-77` closed: the prove from a fresh clone on a throwaway base |
| `360bbde` | `WSL-78` partial: `base agent` and the `muse.exe` launcher, driven on a throwaway base |
| `08e23bc` | the operator's former order for the next two sessions; `WSL-71`'s starting directory measured; `WSL-84` filed |
| `7122216` | `WSL-82`, `WSL-83` and `WSL-84` approved; the existing work restored ahead of Muse and herdr; Soar set for removal and Nix retained |
| `191062a` | `WSL-84` closed: the verifier reads the drives and prints what they are, `base ensure` provisions a drifted base again, and `base status --probe` prints both; `base shell --root` and `--here` read the same mounts; 16 rows red |
| `b9a8ea2` | `WSL-83` closed: a BSD run boots from a qcow2 overlay it removes, the image is never written, and a root not properly dismounted is named; 6 rows new and 4 changed |
| the `WSL-82` commit | `WSL-82` closed: after a panic's two lines a command's wait ends at QEMU's exit, the closing marker or 60 seconds; driven on a real guest; 4 rows new and 2 changed |

## Measurements

On Windows 11 Pro 26200, WSL 2.7.12, on 2026-09-14:

- **At the start of this session:** the doctor exit 0 in 21.17 s at 14:16:33Z; the
  gate exit 0, 20 checks green in 46.33 s at 14:17:30Z; `wsl -l -v` matched the host
  state below; CI run 34846522263 for `7122216` was green.
- **For `WSL-84`'s closing:** the Go suites green on Windows with `TEMP` at the 8.3
  path, 296 top-level `wsl-toolkit` cases, 289 passed, 7 skipped; in `golang:1.25`,
  295, 294 passed, 1 skipped; ShellCheck 0.9.0 in `ubuntu:24.04` clean over 34
  tracked scripts. On the throwaway `m84`, a build from nothing took 32.7 s to 74.1 s,
  and each `base ensure` over drifted drives 4.3 s to 4.7 s.
- **WSL networking:** NAT mode, host address `172.23.96.1`, read by `wsl-toolkit
  hostaddress`. Two distributions share one network namespace, in `WSL-68`.
- **herdr:** 0.9.0 is the latest stable release, published 2026-09-07, read on
  2026-09-13. `herdr-linux-x86_64` is 24,644,488 bytes and
  `herdr-windows-x86_64.zip` 9,054,745 bytes, each with a published SHA-256 digest.
- **Muse:** Meta's installer at `dev.meta.ai` still has the approved digest, 9,314
  bytes, on 2026-09-14; the public channel serves `1.2.1-R2847.1`, a 299,251,896-byte
  Linux x86 build that answers without credentials.
- **For `WSL-84`'s CI:** run 34860407687 for `191062a` green in all six jobs.
- **For `WSL-82`'s closing:** the prove exit 0 on Windows and in `golang:1.25`; 303
  top-level `wsl-toolkit` cases on Windows, 296 passed, 7 skipped; 302 in
  `golang:1.25`, 301 passed, 1 skipped. A real guest's payload printing the two panic
  lines exit 7 with its whole output, where the build before answered exit 2.
- **For `WSL-83`'s closing:** on the shared image, `bsd run -c true` three times with
  the overlay: a login at 8.6 s to 8.8 s and the process gone at 23.4 s to 23.6 s; the
  image's SHA-256 `12807CE7…921663BF` unchanged across six runs. The Go suites green
  on Windows, 300 top-level `wsl-toolkit` cases, 293 passed, 7 skipped; in
  `golang:1.25`, 299, 298 passed, 1 skipped; ShellCheck 0.9.0 clean over 34 scripts.
- **For `WSL-71`:** on the throwaway `m71`, with automount off WSL starts a command in
  the account's home; with it on, in the Windows directory.

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
9. ⚠ **A cancelled `bsd run` is reported as a budget that ran out.** Read in
   `bsd.go` on 2026-09-14 and not measured: a cancelled context ends the boot's wait
   with `the guest did not reach a login prompt within` the budget, and a command's
   with `the guest did not finish the command within the budget`, both exit 2, where
   `distro run` answers a cancellation with 130.
10. ⚠ **A base build fails when one Arch mirror stalls, and nothing retries.** Two
    `base ensure` builds from nothing on 2026-09-14 failed in `pacman` with
    `geo.mirror.pkgbuild.com : Operation too slow`, and each rolled its distribution
    back; the next attempt passed.
11. ⚠ **`wsl-toolkit-podbox` has every drive mounted `9p rw`**, read on 2026-09-14,
    and no configuration of its own, so it answers to the default `ro`. Its next
    `base ensure` provisions it again. It is not this session's to change.

## Review findings

Each closed entry carries its three reviews. `WSL-84`'s door sweep found `base shell
--root` calling read-only drives writable and `base shell --here` trusting the setting
over a guest with no drive, and both now read the mounts; its claim audit corrected
two sentences in the manual. `WSL-83`'s door sweep found no second writer of the
image; its claim audit corrected a panic error, a warning and `bsd status`, which each
described the image a run used to write.

## Open questions for the operator

None. The later Muse session still needs the operator to run `muse login`; that is an
action at its stated checkpoint, not a planning question.

## Host state

- Registered distributions: `podman-machine-default`, `eph-pgb`, `wsl-toolkit`
  and `wsl-toolkit-podbox`, as at the session's start. `eph-pgb` keeps its disk
  under `%LOCALAPPDATA%\wsl-ephemeral` and is not this tool's.
- ⭐ **No throwaway is left.** `wsl-toolkit-m84` was removed with its instance
  directory, and `instances` holds `acc`, `muse`, `nobase`, `podbox` and
  `podbox-migrate`, as at the start. `instances\muse` holds nothing.
- herdr 0.9.0 is installed on Windows by the operator. `%USERPROFILE%\bin` holds no
  `muse.exe`.
- The dedicated SSH key the ruling allows is at
  `%USERPROFILE%\.ssh\wsl-toolkit\id_ed25519`, with an empty `known_hosts` beside it,
  and stays for `wsl-toolkit-base`. `%USERPROFILE%\.ssh\config` reads SHA-256
  `18FC11BE…E93F4`, unchanged through the session.
- ⭐ **The shared FreeBSD image is the published one**, 6,476,638,208 bytes, SHA-256
  `12807CE7…921663BF`, booted six times through overlays and unchanged. No run grows
  or writes it now. The copy `WSL-83` damaged under `.tmp` is deleted, and no QEMU
  process is left.
