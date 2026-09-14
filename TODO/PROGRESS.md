# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-14T02:01:53Z; the record commit's own time is its end
baseline        e73d7d5, tree clean
entries         total 115  open 12  blocked 0  done 103
closed          WSL-74
filed           WSL-80, found by the acceptance runner's baseline
head            the WSL-74 commit after cf274eb
```

## Active work

⭐ **This session closes issues 30, 32 and 33**, in the order below. Every
decision the entries carried is ruled, and the rulings are in the entries.

## The work order, set by the operator on 2026-09-13

Each entry's section in [`wsl-toolkit-go.md`](wsl-toolkit-go.md) carries its
premise, decisions and prove. An issue closes only when every entry mapped to it
is closed, and the issue gets a comment naming the commits.

1. **`WSL-74`**, first, because `WSL-75` and `WSL-78` stand on it. ⭐ Closed.
2. **`WSL-80`**, inserted here on 2026-09-14 and not part of the three issues: a
   `distro run` that never returns hangs the acceptance runner, which is how
   every later entry here is proved on this host.
3. **`WSL-79`**, issue 33: its line join reports success over commands that
   never ran.
4. **`WSL-72`**: the ruling it needed is made.
5. **`WSL-76`**, then **`WSL-75`**, **`WSL-77`** and **`WSL-78`**: issue 32, in
   that order, because the later ones use herdr and the grants.
6. **Issue 30:** `WSL-67`'s open items, `WSL-68`, `WSL-70` and `WSL-71`.
7. **`wsl-toolkit-base`** is built last, from the machinery, and the operator
   signs Muse in there.
8. **`wsl-toolkit-v3.0.0`** is cut once the three issues are closed and CI is
   green on the final commit.

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
carries all three as commands under "Build and local proof".

## Done this session

| commit | what |
| --- | --- |
| `cf274eb` | the operator's rulings written into `WSL-67`, `WSL-68`, `WSL-75` to `WSL-79` and this record |
| the WSL-74 commit | `WSL-74` closed: a configuration naming another instance's distribution is refused by every command, `ready` included, with 6 mutation rows and 2 acceptance cases; `WSL-80` filed |

## Measurements

On Windows 11 Pro 26200, WSL 2.7.12, on 2026-09-14:

- **At the start:** the probe exit 0 in 41.77 s; the gate exit 0 in 44.68 s,
  19 checks green.
- **WSL networking:** NAT mode, host address `172.23.96.1`, read by `wsl-toolkit
  hostaddress`. Two distributions share one network namespace, in `WSL-68`.
- **herdr:** 0.9.0 is the latest stable release, published 2026-09-07.
  `herdr-linux-x86_64` is 24,644,488 bytes and `herdr-windows-x86_64.zip`
  9,054,745 bytes, each with a published SHA-256 digest.
- ⚠ **The acceptance runner's baseline, on a build of `e73d7d5`**, started by a
  quoting mistake rather than on purpose: `pwsh -Command STRING PATH` appends the
  path to the command text, so a parse check ran `acceptance.ps1`. 91 cases ran
  and 89 passed. One hung for 10m31s and was ended, which is `WSL-80`; one was the
  new `WSL-74` case, failing on a build without the guard; and the count guard
  failed until the declared count moved to 91. Every distribution registered
  before the run was still registered after it.
- **For `WSL-74`:** the Go suites green with `TEMP` at the 8.3 path, 254
  top-level cases, 251 passed, 3 skipped; 6 mutation rows proved on Windows.

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

## Review findings

`WSL-74`'s closing carries its three reviews. The door sweep found `ready` reading
and building from a refused configuration, and the claim audit found the refusal
advising a `--config` file that did not exist; both are fixed in the same commit.

## Open questions for the operator

None. Every decision is ruled.

## Host state

- Registered distributions: `podman-machine-default`, `eph-pgb`, `wsl-toolkit`
  and `wsl-toolkit-podbox`. `eph-pgb` keeps its disk under
  `%LOCALAPPDATA%\wsl-ephemeral` and is not this tool's.
- ⭐ **`wsl-toolkit-muse` is gone.** The operator ran `muse logout` in it and then
  `base remove --yes` on 2026-09-14, and `instances\muse` holds nothing.
- herdr 0.9.0 is installed on Windows by the operator. The shared FreeBSD pkgbase
  guest has not been booted or changed this session.
