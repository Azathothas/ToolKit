# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-14T09:18:24Z; the record commit's own time is its end
baseline        eff07c5, tree clean, CI run 34820474937 green
entries         total 118  open 10  blocked 0  done 108
closed          WSL-81 at e32791a; WSL-72
filed           WSL-82, found by WSL-81's claim audit; WSL-83, found by WSL-72's baseline read
head            e32791a, then this commit
```

## Active work

⭐ **`WSL-81` and `WSL-72` are closed.** `WSL-72`'s prove ran as written from pushed
`main` and passed on a fresh copy of the published image. ⛔ **`WSL-83` is filed at
P1:** two read-only runs on the shared image panicked while powering off, with one
processor, and the second panicked on the filesystem the first left unchecked. The
shared image is restored, and the decision is the operator's, below.

**Next: issue 32.** `WSL-76` waits for `wsl-toolkit-base` and the operator's sign-in,
so `WSL-77` comes first: the Muse adapter, driven on a throwaway instance, then
`WSL-78`'s launchers and guide as far as they go without a signed-in Muse.

## The work order, set by the operator on 2026-09-13

Each entry's section in [`wsl-toolkit-go.md`](wsl-toolkit-go.md) carries its
premise, decisions and prove. An issue closes only when every entry mapped to it
is closed, and the issue gets a comment naming the commits.

1. **`WSL-74`**, first, because `WSL-75` and `WSL-78` stand on it. ⭐ Closed.
2. **`WSL-80`**, inserted here on 2026-09-14 and not part of the three issues: a
   `distro run` that never returns hangs the acceptance runner, which is how
   every later entry here is proved on this host. ⭐ Closed.
3. **`WSL-79`**, issue 33: its line join reports success over commands that
   never ran. ⭐ Closed, and issue 33 is closed with a comment naming `5e66e44`
   and `cf274eb`.
4. **`WSL-72`**, and **`WSL-81`** inserted before its prove, because the first run of
   that prove panicked the guest. ⭐ Both closed.
5. **`WSL-76`**, then **`WSL-75`**, **`WSL-77`** and **`WSL-78`**: issue 32, in
   that order, because the later ones use herdr and the grants. `WSL-76`'s
   machinery is built and driven without Muse; its closing needs Muse through herdr,
   so it waits for `wsl-toolkit-base` and the operator's sign-in. ⭐ `WSL-75` is
   closed.
6. **Issue 30:** `WSL-67`'s open items, `WSL-68`, `WSL-70` and `WSL-71`.
7. **`wsl-toolkit-base`** is built last, from the machinery, and the operator
   signs Muse in there.
8. **`wsl-toolkit-v3.0.0`** is cut once the three issues are closed and CI is
   green on the final commit.

⚠ **`WSL-82` and `WSL-83` are in no issue and not in this order.** Each waits for the
operator's approval and ruling, below. The release condition does not name them.

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

## Before every push

A green local gate is not CI. Run the Go tests with `TEMP` and `TMP` at an 8.3
short path, then CI's Linux Go job and its ShellCheck in containers.
[`../tools/windows/wsl-toolkit/README.md`](../tools/windows/wsl-toolkit/README.md)
carries all three as commands under "Build and local proof". ⚠ **A heavy BSD run goes
on a fresh image copy**, with `WSL_TOOLKIT_CACHE` under this repository's `.tmp`,
never on the shared image.

## Done this session

| commit | what |
| --- | --- |
| `e32791a` | `WSL-81` closed: 8 mutation rows red one at a time; a step the guest never finished no longer prints `(exit 0)`, with a case and a row; the manual's per-run cost measured again on the one-processor build, about 23 s; `WSL-82` filed for a payload that prints a panic's two lines |
| this commit | `WSL-72` closed: the prove as written passed on a fresh image copy, exit 0, `absent=` empty, `nim` and `rustc` present, a 10.6 GiB root; 4 rows red. `WSL-83` filed after two panics at poweroff on the shared image, which is restored; the manual, two comments and `WSL-81`'s record corrected where they said such a panic comes after the buffers sync |

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
- **For `WSL-81`'s closing:** the Go suites green on Windows with `TEMP` at the 8.3
  path, 280 top-level `wsl-toolkit` cases, 276 passed, 4 skipped; in `golang:1.25`,
  279, 278 passed, 1 skipped; ShellCheck 0.9.0 in `ubuntu:24.04` clean over every
  tracked script. 8 mutation rows red on Windows, each after its cases passed
  unmutated. Three `bsd run -c true` runs with one processor: a login at 8.3 s to
  8.5 s and the process gone at 23.0 s to 23.3 s, with no panic.
- **For `WSL-72`'s closing:** the prove exit 0 in 178.8 s on a fresh copy, whose
  first boot logged in at 29 s; 4 mutation rows red. `bsd fetch --force` restored
  the shared image in 13.1 s, and an image copy expanded in 24 s.
- ⛔ **For `WSL-83`:** two read-only runs on the shared image, one processor, both
  exit 0, both panicked at poweroff: `bad pte va 389278400000 pte 0` before `Syncing
  disks`, then, on the next boot's unchecked root, `initiate_write_filepage: dir inum
  0 != new 160513`.

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

Each closed entry carries its three reviews. What they changed: `WSL-74`'s door
sweep found `ready` reading and building from a refused configuration, and its claim
audit found the refusal advising a `--config` file that did not exist. `WSL-80`'s
claim audit found its own premise blaming a pipe the stack dumps cleared. `WSL-79`'s
door sweep found the guest keeping script copies in a shared `/tmp` after a failed
run. `WSL-81`'s claim audit found a grow a panic ended printing `(exit 0)`, the
manual's per-run cost measured on the two-processor build, and a payload's copy of a
panic read as the kernel's, which is `WSL-82`. `WSL-72`'s claim audit found the
manual's per-run cost silent about a fresh image's first boot, and its restore time
stale. Each is fixed or filed in the entry's own commit.

## Open questions for the operator

1. **`WSL-83`: approve the entry, and rule A, B or C.** What protects the shared FreeBSD
   image from a panic at poweroff: **A, a throwaway overlay per run, recommended**,
   which ends a guest one session configures for the next; B, the image writable,
   with `sync` before `poweroff` and a run refused on a boot that shows its root not
   properly dismounted; C, a warning only.
2. **`WSL-82`: approve the entry, and rule A, B or C.** What ends a command's wait
   once the console shows a panic's two lines: A, at once, as now; **B, QEMU's exit,
   the command's closing marker, or 60 seconds, recommended**; C, QEMU's exit alone.
3. **Does `wsl-toolkit-v3.0.0` wait for `WSL-83`?** Recommended: no. The ruling names
   the three issues, the manual's known limits carry the panic, and `bsd fetch
   --force` restores the image in seconds.

## Host state

- Registered distributions: `podman-machine-default`, `eph-pgb`, `wsl-toolkit`
  and `wsl-toolkit-podbox`. `eph-pgb` keeps its disk under
  `%LOCALAPPDATA%\wsl-ephemeral` and is not this tool's.
- ⭐ **`wsl-toolkit-muse` is gone.** The operator ran `muse logout` in it and then
  `base remove --yes` on 2026-09-14, and `instances\muse` holds nothing.
- herdr 0.9.0 is installed on Windows by the operator.
- The dedicated SSH key the ruling allows is at
  `%USERPROFILE%\.ssh\wsl-toolkit\id_ed25519`, with an empty `known_hosts` beside it,
  and stays for `wsl-toolkit-base`. `%USERPROFILE%\.ssh\config` reads SHA-256
  `18FC11BE…E93F4`, as the previous session left it.
- ⛔ **The shared FreeBSD image is the published one again**, 6,476,638,208 bytes,
  restored by `bsd fetch --force` at 09:46:42Z after `WSL-83`'s panics, and not booted
  since. The next run grows it to 12 GiB. No image copy remains under `.tmp`.
