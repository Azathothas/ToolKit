# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-15T03:12:15Z; the record commit's own time is its end
baseline        deea680, tree clean, doctor exit 0 in 25.85 s, gate 20 of 20 in 32.19 s
head            12705a1, CI run 34932875176 green in all six jobs
entries         total 122  open 6  blocked 0  done 116
closed          WSL-86, WSL-87 and WSL-70 in this session's work commit
partial         WSL-67, waiting on the two BSD downloads
```

## Active work

⭐ **`WSL-86`, `WSL-87` and `WSL-70` are closed**, each with its prove and its three
reviews. The base provisioner installs the `nft` its podman shells out to and reads the
id-mapping privilege rather than the path; `bootstrap.sh` installs CodeGraph under dash
and names a kernel it publishes no package for. All four base presets build and verify.
**Resume, in this order:**

1. `WSL-67`'s `pkgin` and `pkg_add`, once the two downloads are approved: ask again once,
   then its closing.
2. `WSL-71`.
3. `WSL-68`, the sealed base, in a dedicated session, which closes issue 30.

⚠ **The two BSD image downloads still wait for the operator's approval in chat.** Asked
again on 2026-09-15 at this session's start, in chat and as a file, naming each file, its
source and its size, and not answered. The operator's reply that day approved `WSL-86` and
asked to be asked only for what actually needs them; a 1.3 GiB download from two mirrors
is one of those, so it is asked once more and no further. ⭐ QEMU 11.1.0 here puts
SeaBIOS's screen on the serial console with `-M q35,graphics=off`, measured with no disk on
2026-09-15, which is how a BSD boot loader's prompt is reached with no `sga` device. The
scratch driver under `.tmp\bsd67-driver` has still never booted a guest.

⛔ **Found while proving `WSL-86`, and not filed: the automount sweep is a race.** A
Windows drive that appears between the provisioner's sweep and the restart that applies
`automount off` leaves an empty mount point, and the base then fails verification.
Measured on 2026-09-15 and recorded under "Found, and not filed" below, with the numbers.

`WSL-78`'s entry point, `base agent` and the `muse.exe` launcher remain built and
driven without a sign-in.

## The work order, set by the operator on 2026-09-14

Each entry's section in [`wsl-toolkit-go.md`](wsl-toolkit-go.md) carries its
premise, decisions and prove. An issue closes only when every entry mapped to it
is closed, and the issue gets a comment naming the commits.

1. ⭐ **Closed:** `WSL-74`, `WSL-80`, `WSL-79` with issue 33, `WSL-75`, `WSL-81`,
   `WSL-72`, `WSL-77`, `WSL-84`, `WSL-83`, `WSL-82` and `WSL-85`.
2. **Finish issue 30's existing package and profile work:** `WSL-67`, then `WSL-70`,
   then `WSL-71`. `WSL-67` has left: driving `pkgin` and `pkg_add`, which waits for
   the operator's approval of the two downloads, and its closing.
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
8. **2026-09-14: remove Soar and keep Nix**, and Nix is loaded, set up and never
   installed: an installed Nix is found without sourcing a profile, flakes are set up
   and used for a new profile, the four `NIXPKGS_ALLOW_*` variables are always on
   with more defaults where they help, and a GitHub token reaches Nix through
   `NIX_CONFIG` only. All of it lives in this repository. In `WSL-67`.
9. **2026-09-15: `WSL-85` approved**, "approve WSL-85": filed and fixed in this
   session, before `WSL-70`.
10. **2026-09-15: `WSL-87` approved**, "approve WSL-87": filed and fixed in this
    session.
11. **2026-09-15: `WSL-86` approved** as recommended, "yes i accept your
    recommendation": `nftables` in the provisioner's `apt` arm, and the id-mapping
    check reads the capability rather than the path and restores a dropped one. The
    operator asked in the same message to be asked only for what actually needs them,
    which is why the drive-sweep race found while proving it is recorded under "Found,
    and not filed" rather than proposed as an entry.

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
| `45fc7cc` | `WSL-67` partial: Soar removed from `bootstrap.sh`; the Nix route finds an installed Nix, sets up flakes and installs through the table's `nix` key; the last session's record and summary |
| `953e7b8` | `WSL-85` filed and closed: the verifier refuses passwordless sudo the configuration turned off, `base ensure` provisions such a base again, and a verification failure is named past wsl.exe's own lines; 3 mutation rows |
| `4c573cf` | `WSL-67` partial: the acceptance runner builds `wsl-toolkit-accp` and drives the two provider profiles as five cases, 96 of 96 in a full run |
| `9df2e7b` | `WSL-70` partial: the base provisioner reads the shared package table and its detection; 4 mutation rows; `WSL-87` filed |
| `deea680` | `WSL-87` partial: `bootstrap.sh` installs CodeGraph under dash and names an npm too old for it; the last session's record and summary |
| this session's work commit | `WSL-86` filed and closed, `WSL-87` and `WSL-70` closed: the provisioner installs `nftables` and restores the id-mapping capability, `install_codegraph` names a kernel it publishes no package for, and all four presets build; 3 mutation rows and one defect planted by hand |

## Measurements

On Windows 11 Pro 26200, WSL 2.7.12, on 2026-09-15:

- **At the start of this session:** the doctor exit 0 in 25.85 s at 03:12:24Z; the gate
  exit 0, 20 checks green in 32.19 s at 03:12:49Z; `wsl -l -v` matched the host state
  below; CI run 34923941438 for `deea680` was still running.
- **For `WSL-86`, on throwaway instances under `.tmp` with automount and interop off:**
  arch exit 0 in 77.3 s and alpine in 172.9 s with `toolset developer`; debian exit 0 in
  94.0 s, where it had failed at `nft`, with `newuidmap` and `newgidmap` setuid; fedora
  exit 0 in 48.4 s with `toolset none`, its `newuidmap` and `newgidmap` arriving without
  their capability and given it. ⚠ Fedora's `developer` build installed all thirteen
  commands and ran a container in 420 s from a mirror answering in tens of KiB/s, then
  failed verification on `/mnt/e`; a second took the whole 30-minute budget. Three
  mutation rows red in `golang:1.25`, each green unmutated first.
- **For `WSL-87`:** the agent matrix `--images all --toolset agent`, 13 ran, 3 failed, 0
  unreached, 0 timed out, in 1184.45 s: debian, debian 12, ubuntu 22.04 and void-musl each
  `codegraph=1.6.0` and `failures=0`, rocky 8 naming npm 6.14.11 as too old, and chimera
  and gentoo failing as they did before. The kernel guard planted by hand in `golang:1.25`:
  unmutated exit 0, planted exit 1, restored exit 0. ⛔ The first plant read exit 0
  over the removed guard, because `go test` served a cached result for a shell file the
  build cache does not track; `-count=1` made it fire.
- **The suites, on this session's tree:** 313 top-level `wsl-toolkit` results on Windows
  with `TEMP` at the 8.3 path, 298 passed, 15 skipped, 0 failed, exit 0 in 16.8 s;
  `check-go.sh` exit 0 in `golang:1.25`; ShellCheck 0.9.0 in `ubuntu:24.04` clean over 34
  tracked scripts.
- **For `WSL-85`:** on the throwaway `wsl-toolkit-p67`, a sudo drift made `base status
  --probe` exit 1 in 0.5 s and `base ensure` provision again in 15.9 s and 16.0 s; a
  rule the tool did not write made `base ensure` exit 2 in 14.5 s. The Go suites green:
  305 top-level `wsl-toolkit` cases on Windows with `TEMP` at the 8.3 path, 297 passed,
  8 skipped; 304 in `golang:1.25`, 303 passed, 1 skipped. ShellCheck 0.9.0 in
  `ubuntu:24.04` clean over 34 tracked scripts.
- **For `WSL-67`'s provider profiles, on `wsl-toolkit-p67`:** a build from nothing with
  automount and interop off, systemd, the developer toolset and one read-write grant
  under `.tmp` took 67.3 s. As the account: the grant the one DrvFS mount beside WSL's
  own `/usr/lib/wsl/drivers`, a sentinel read and a file written back to Windows, no
  `/mnt` drive, no interop, systemd PID 1, `mount -t drvfs C:` refused. Zero grants: the
  probe exit 1 naming the stale mount, and `base ensure` removed it in 20.4 s. `base
  grant` and `base revoke` 0.31 s and 0.29 s.
- **For `WSL-67`'s acceptance cases:** the five alone 5 of 5 in 105.1 s, and red over a
  build with `WSL-85`'s check taken out; the full runner exit 0, 96 of 96 cases in
  465.3 s from 01:33:26Z.
- **For `WSL-70`:** with the developer toolset, arch built in 51.5 s and alpine in 44.2 s;
  debian failed at the `nft` its podman reaches for, as `4c573cf`'s build did with no
  toolset in 33.6 s, and fedora at `newuidmap`'s missing capability. The agent matrix with
  `--codegraph none`: 13 ran, 2 failed, in 340.6 s; with CodeGraph on, 7 failed in 517.3 s.
- **For `WSL-87`:** npm 6.14.11 and 7.17.0 have no `--pack-destination` and 7.18.0 has;
  both cases green under dash in `golang:1.25` and red with each defect planted by hand;
  over the fix, debian, debian 12, ubuntu 22.04 and void-musl install CodeGraph 1.6.0.
- **For `WSL-67`'s Nix work, on 2026-09-14:** in `docker.io/nixos/nix:latest`, Nix
  2.35.2, a new unprivileged account installed 16 names through flakes in 24 s and again
  in 10 s; a `nix-env` account through channels in 9 s; 25 of 25 table attributes
  evaluate.
- **WSL networking:** NAT mode, host address `172.23.96.1`, read by `wsl-toolkit
  hostaddress`. Two distributions share one network namespace, in `WSL-68`.
- **herdr:** 0.9.0 is the latest stable release, published 2026-09-07, read on
  2026-09-13. `herdr-linux-x86_64` is 24,644,488 bytes and
  `herdr-windows-x86_64.zip` 9,054,745 bytes, each with a published SHA-256 digest.
- **Muse:** Meta's installer at `dev.meta.ai` still has the approved digest, 9,314
  bytes, on 2026-09-14; the public channel serves `1.2.1-R2847.1`, a 299,251,896-byte
  Linux x86 build that answers without credentials.
- **For `WSL-71`:** on the throwaway `m71`, with automount off WSL starts a command in
  the account's home; with it on, in the Windows directory.
- **For `WSL-67`'s BSD drives:** NetBSD 11.0 and OpenBSD 7.9 are the current releases,
  read from the mirrors on 2026-09-14. QEMU 11.1.0 here has no `sga` device; with no
  disk, SeaBIOS's banner and boot messages reached `-serial stdio` under `-M
  q35,graphics=off` and under an `etc/sercon-port` fw_cfg file, and nothing did under a
  plain `-M q35`, on 2026-09-15.

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
5. A deterministic regression for pre-marker base rollback is still owed. ⚠ **Met again on 2026-09-15:** a fedora build stopped mid-provisioning left `wsl-toolkit-t86fedora` registered, and `base remove` refused it with `carries no wsl-toolkit identity marker`, so `wsl.exe --unregister` was used by hand.
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
12. ⚠ **23 more lines in nine files name an error by the first line of a guest
    command's stderr**, where wsl.exe can write a line of its own first: `adapters.go`,
    `base.go`, `cleanup_run.go`, `engine.go`, `inspect.go`, `job.go`, `matrix.go`,
    `resources.go` and `throwaway.go`, found by `WSL-85`'s door sweep on 2026-09-15.
    `WSL-85` fixed the verifier's; none of the others was measured naming a wsl.exe
    line.
13. A base built from nothing with automount and interop off printed 93 `wsl: Failed
    to translate` lines between provisioning and its restart, measured on 2026-09-15.
    Read, not measured: `appendWindowsPath=false` takes effect at that restart.
14. The verifier's `interop on` checks nothing, so a base configured with interop on
    over a guest without it verifies. Read on 2026-09-15, not measured, and it grants
    no authority.
15. ⚠ **A verification that fails is named by the FIRST guest line, and podman writes a
    warning before its error.** `verifyError` in `base.go` takes `firstLine` of the guest's
    output, so fedora's `newuidmap: write to uid_map failed: Operation not permitted` sat
    behind podman's shared-mount warning and needed a diagnostic build to read, on
    2026-09-15. It is finding 12's family from the other side: not wsl.exe's line but the
    engine's own first one. Found by `WSL-86`'s door sweep.
16. `fetch_verified_npm` in `bootstrap.sh` calls `split_on`, which lives inside the shared
    package table block and is generated into `packages.sh` for the provisioner, so a
    change to `split_on` made for the table changes the npm work directory's name. Read on
    2026-09-15 by `WSL-87`'s door sweep, not measured, and nothing is built on it.
17. `bootstrapSource` in `bootstrap_npm_test.go` reads five levels above its package, which
    is outside the Go module, so a `repo mutate` row naming one of those cases would report
    the file unreadable rather than a guard's verdict. No row names one. Read on 2026-09-15.
18. ⚠ **The provisioner sweeps the automount mount points once, and a Windows drive that
    appears after that sweep is left behind.** Measured on 2026-09-15: this host carries
    ten fixed drives; the arch, alpine and debian builds each removed ten mount points and
    the fedora build, which ran for 420 s, removed **nine**, and its verification then
    refused the base with `/mnt/e exists even though automount is off`. E: is an external
    HDD. The sweep at `provision.sh:494` runs before the restart that applies `automount
    off`, so a drive WSL mounts between the sweep and that restart leaves an empty 0777
    directory the verifier is right to refuse. Found by `WSL-86`'s driven pass.
19. `bootstrap.sh --dry-run --toolset agent --codegraph none --json` exits 0 in
    `golang:1.25` and 1 in `debian:latest`, measured on 2026-09-15, which narrows finding 4
    to what the image carries rather than to Debian.

## Review findings

Each closed entry carries its three reviews. `WSL-85`'s door sweep found 23 more
error messages named by a guest's first stderr line, recorded above; its claim audit
corrected the manual's first draft, which said a disagreeing base "answers exit 1" and
named no command. `WSL-67`'s checkpoint: the claim audit found the Nix version probe
outside `nix_run`, where the documentation said every Nix command gets the settings, and
the probe now goes through it. ⭐ **2026-09-15:** `WSL-87`'s guard mutation found a
defect in its own method - a hand-planted defect in a file outside the Go module read as a
green pass, because `go test` served a cached result and only `-count=1` made the guard
fire; `repo mutate` already passes it. Its claim audit found `presets.go` publishing a
build figure for a preset that had not built since, now corrected with both dates and both
sets of conditions. `WSL-86`'s driven pass found the automount sweep race, and its door
sweep found that a verification failure is named by the engine's first line. ⛔ **CI
then failed one of its own mutation rows as THEATRE**, because the case proving it can only
be staged where no `getcap` exists and `ubuntu-latest` carries one: the row is now its own
case, which skips there and goes red in `golang:1.25`. `WSL-67` still owes its closing
reviews.

## Open questions for the operator

⚠ **One, and it is the BSD download approval above.** `WSL-86` was ruled on
2026-09-15 and is closed. The later Muse session still needs the operator to run `muse
login`. ⭐ The operator asked on 2026-09-15 to be asked only for what actually needs
them: a ruling this repository's own rules require, a download, a credential, or Windows
software. Everything else is decided here and recorded.

## Host state

- Registered distributions: `podman-machine-default`, `eph-pgb`, `wsl-toolkit` and
  `wsl-toolkit-podbox`, as at the session's start. `eph-pgb` keeps its disk under
  `%LOCALAPPDATA%\wsl-ephemeral` and is not this tool's.
- ⭐ **No throwaway is left.** The seven preset builds this session made under
  `.tmp\wtk86` were each removed by `base remove`. ⚠ **One needed `wsl.exe
  --unregister` by hand:** a fedora build whose mirror stalled was stopped before
  provisioning wrote the identity marker, and `base remove` refuses a distribution that
  carries none - the pre-marker rollback case the record has owed a regression for since
  it was first seen. Its name was typed in full and no other distribution was touched.
- ⚠ **A scratch serial-console driver for the NetBSD and OpenBSD guests is at
  `.tmp\bsd67-driver`**, untracked: it boots through an overlay, answers the boot loader and
  the OpenBSD installer over SeaBIOS's serial console, and runs commands from a spool with
  files served over QEMU's own TFTP. It has never booted a guest; read it before using it.
- `instances` under `%LOCALAPPDATA%\wsl-toolkit` holds `acc`, `muse`, `nobase`,
  `podbox` and `podbox-migrate`, as at the start. `instances\muse` holds nothing.
- No BSD image is downloaded. The shared FreeBSD image is the published one,
  6,476,638,208 bytes, SHA-256 `12807CE7…921663BF`, not booted this session.
- herdr 0.9.0 is installed on Windows by the operator. `%USERPROFILE%\bin` holds no
  `muse.exe`.
- The dedicated SSH key the ruling allows is at
  `%USERPROFILE%\.ssh\wsl-toolkit\id_ed25519`, with an empty `known_hosts` beside it,
  and stays for `wsl-toolkit-base`. `%USERPROFILE%\.ssh\config` reads SHA-256
  `18FC11BE…E93F4`, as at the last session's end.
