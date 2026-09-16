# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-16T06:59:37Z, resuming the session of 2026-09-15T10:20:32Z that was checkpointed and ended before it wrote its record
baseline        cfa3252, tree DIRTY with that session's unfinished record, doctor exit 0 in 75.96 s, gate 21 of 21 in 64.8 s
head            42a7c5b, the finished checkpoint, pushed after cfa3252's CI run 34967456693 was green; this record commit follows it
entries         total 125  open 7  blocked 0  done 118
this session    the 2026-09-15 checkpoint finished; WSL-90's first nightly published, verified, driven, and one defect found in it and fixed
```

## Active work

⭐ **This is the attended Muse and herdr session the work order reserved.**
`wsl-toolkit-base` is built from the example profile, saved as the instance's own
configuration, and holds herdr 0.9.0 and Muse Code 1.3.0. The operator's `muse login`
has not happened yet.

⭐ **The four measurements the sweep named are taken**, and three of them overturn the
record. herdr on Windows is 0.9.0 and no published herdr carries `#4038`'s fix; ⛔
**`herdr --machine` does not exist in 0.9.0**, because the sweep read herdr's
unreleased `docs/next`; WSL exposes a foreground process group; and Muse started
through this tool's wrapper keeps its identity to herdr. `WSL-76`'s amendment of
2026-09-15 carries every number, and
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md) the
row-by-row correction.

⭐ **The muse adapter's herdr reporter now runs against a real herdr and a real Muse.**
Its first real run found four defects the stub could not: a temporary file
`fs.protected_regular` refused, the wrong settings file, the wrong hook shape, and a new
file Muse would refuse to start over. All four are fixed with a case and five mutation
rows, and the full lifecycle, a `wait --until working` and resume adoption are measured
in a pane of the base.

⭐ **herdr is built here from its development branch, by the operator's instruction**,
and **`WSL-90` carries it forward**: approved, filed, and implemented in this session by
the operator's rulings 12 to 15. Driven in a Windows pseudo console against the base,
the development client passes every signal `#4176` names and 0.9.0 fails three; with
the development server swapped into `wsl-toolkit-base`, `--machine base agent list`
answers exit 0. Its release lookup is proved, and both build matrix runs are read.

⭐ **The first nightly is published, verified and driven**, on 2026-09-16:
`herdr-nightly-20260916-18061191fdc0`, run `35066661420`, 6 of 6 jobs green, 12 assets.
Every digest passes `sha256sum -c` and every bundle passes `cosign verify-blob` against
`herdr-nightly.yml`'s identity, and both refuse when the identity is wrong or a byte is
flipped. With `"channel": "nightly"` the base installs it in 8.78 s, `base status
--probe` reads the release's own digest and `server-binary-stale no`, and the client
`base attach` prints answers `--machine base agent list` with exit 0.
⛔ **Its first real run found a defect the suite could not**: the prune sorted by
`created_at`, which `--target` sets from the target commit, and would have deleted the
NEWEST nightly. Fixed and driven; finding 33 records that no harness covers it.
⚠ `release.yml`'s herdr jobs have still never run, no `wsl-toolkit-v*` release carries
herdr, and the probe as a tracked script is still a draft outside the tree,
`.tmp\herdr-remote-probe.cs`.

**Resume, in this order:**

1. **`WSL-90`'s last two steps**: the pseudo-console probe as a tracked acceptance
   script, approach step 7, driven against the nightly's Windows client and typing only
   into a workspace of its own; then the three reviews and the closing.
2. **The operator's two steps**, which nothing else here can stand in for: the six
   `--remote` signals in a real Windows Terminal window with the development client, and
   `muse login` in `base shell`.
3. `WSL-76`'s and `WSL-78`'s proves after the sign-in, then `PreToolUse` and
   `PermissionRequest` from a real turn, then the guide.
4. `WSL-68`'s four remaining items, listed in its own entry.
5. `WSL-88` and `WSL-89`, the `pi` and `omp` adapters. ⛔ **Both are WRITTEN AND NEVER
   RUN**.

## The work order, set by the operator on 2026-09-14

Each entry's section in [`wsl-toolkit-go.md`](wsl-toolkit-go.md) carries its
premise, decisions and prove. An issue closes only when every entry mapped to it
is closed, and the issue gets a comment naming the commits.

1. ⭐ **Closed:** `WSL-74`, `WSL-80`, `WSL-79` with issue 33, `WSL-75`, `WSL-81`,
   `WSL-72`, `WSL-77`, `WSL-84`, `WSL-83`, `WSL-82` and `WSL-85`.
2. ⭐ **Closed:** `WSL-70`, `WSL-71` and `WSL-67`, which finish issue 30's existing
   package and profile work. ⚠ **This item said until 2026-09-16 that `WSL-67` still
   had `pkgin` and `pkg_add` to drive and was waiting on two downloads.** Both were
   approved on 2026-09-15, both managers were driven on a NetBSD 11.0 guest, and the
   entry closed at `6f22e39`; the sentence outlived the work by a session and sent a
   resuming session to a finished entry. Finding 29.
3. **`WSL-68`, the sealed base.** It closes issue 30 after the work above, and uses the
   drive verifier `WSL-84` fixed. ⚠ The 2026-09-14 order gave it a dedicated session;
   the operator's instruction of 2026-09-15, to finish everything except the herdr and
   Muse drive, brings it into this one.
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
12. **2026-09-15: herdr is built here.** "let's build herdr ourself (now locally for
    windows) and if it works, we will create a dedicated nightly builder for it on
    github and publish it on our repo". In `WSL-90`.
13. **2026-09-15: herdr builds are published from ToolKit, as prereleases**, which
    amends `docs/AGENTS.md` section 1's one published thing to two; for Windows
    `x86_64` and `aarch64` and Linux `x86_64` and `aarch64`; and the newest stable herdr
    is published inside each `wsl-toolkit` release. In `WSL-90`.
14. **2026-09-15: the herdr adapter may follow the newest nightly**, over the
    recommended pin by version and digest. In `WSL-90`.
15. **2026-09-15: `WSL-90` approved and implemented in the session that authored it**,
    and the development herdr server swapped into `wsl-toolkit-base` for the
    `--machine` measurement.

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
| `53a8f1f` | `WSL-86` filed and closed, `WSL-87` and `WSL-70` closed: the provisioner installs `nftables` and restores the id-mapping capability, `install_codegraph` names a kernel it publishes no package for, and all four presets build; 3 mutation rows and one defect planted by hand |
| `8801301` | `WSL-71` closed: `shell-profile.sh` moves an interactive shell off a Windows drive and leaves a marked or granted one alone, `base shell --here` marks its own shell through `WSLENV`, and `bootstrap.sh` writes its lines to the login file bash actually reads; 2 mutation rows and 5 defects planted by hand |
| `6f22e39` | the last session's record: `WSL-71` and `WSL-67` closed, `WSL-68` attacked, `WSL-88` and `WSL-89` written and never run |
| `f179609` | `WSL-76` and `WSL-78` partial: the four measurements, the muse reporter's four defects fixed with a case and 5 mutation rows, its lifecycle against a real herdr, `--machine` corrected out of every live page, and two failing commands out of the Muse example |
| `062ae9c` | `WSL-90` filed and approved: the release lookup reads past its first page, with 2 cases and 3 mutation rows; `herdr-build.yml`, `herdr-nightly.yml` and `release.yml`'s herdr jobs written |
| `cfa3252` | `WSL-90` partial: the herdr adapter's `nightly` channel, with 4 cases and 5 mutation rows, and the build matrix's first run read and answered |
| `42a7c5b` | the 2026-09-15 checkpoint finished by the session that resumed it: `WSL-90`'s second build matrix run recorded from its logs, 8 of 8 green and both fixes measured; the three closing reviews run, finding 31; five live pages that described a publication none has made; the work order corrected, finding 29; and that session's summary written from artefacts |
| this record commit | `WSL-90` partial: the first herdr nightly published, its six signatures verified and both refusals proved, the base driven onto the `nightly` channel end to end, and the prune's `created_at` defect found by that first run and fixed |

## Measurements

On Windows 11 Pro 26200, WSL 2.7.12, on 2026-09-15:

- **At the start of this session:** the doctor exit 0 in 39.71 s at 10:21:35Z; the gate
  exit 0, 21 checks green in 33.81 s at 10:22:19Z; `wsl -l -v` matched the host state
  of the last session; CI run 34955394330 for `6f22e39` green in all six jobs.
- **For `WSL-76` and `WSL-78`, on `wsl-toolkit-base`:** the four ordered measurements,
  the reporter's four defects and its lifecycle, the two herdr builds and the launcher
  from a granted project are in each entry's amendment of 2026-09-15, with their
  conditions. The Windows Go proof with the 8.3 `TEMP`: **324 results, 304 passed, 20
  skipped, 0 failed, exit 0 in 19.1 s**.
- **For `WSL-71`, the shell profile:** `matrix --images all` with a payload that drives
  every shell on the image twice, `--no-shell-profile` then with it, exit 0, **13 ran, 0
  failed, 0 unreached, 0 timed out, in 1m53s**; **28 shells, 0 that add a byte to
  stderr**, and all 28 read the profile. ⚠ The same command against the FIRST driver read
  13 ran, 2 failed, and both failures were the driver: chimera's `wc -c` pads its answer
  with spaces so a count of zero read as non-zero, and photon's own
  `/etc/profile.d/dircolors.sh` writes 66 bytes to every login shell with or without this
  file.
- **For `WSL-71`, on the throwaway arch base `wsl-toolkit-b71`**, automount `rw`, interop
  off, no systemd, the profile installed by `bootstrap.sh` from the mounted checkout: a
  non-interactive login shell on a Windows drive stayed with 0 bytes of stderr, an
  interactive one moved to the home with one 174-byte line, a shell carrying the `--here`
  mark stayed, and one already in the home stayed. `base ensure` exit 0 in 66.07 s. ⚠ **The
  first attempt failed in 57.34 s** at `geo.mirror.pkgbuild.com : Operation too slow`, and
  rolled the distribution back: finding 10 below, met for the third time.
- **For `WSL-71`, the login file bash reads:** `/etc/skel` ships `.bash_profile` and no
  `.profile` on arch, fedora and rocky 8, `.profile` and no `.bash_profile` on debian, and
  neither on alpine. On the arch base, with a fresh account each time: `HEAD` left the
  prefix off a bash login shell's `PATH` and the tree put it there, and both put it on an
  `sh` login shell's. In debian and fedora containers, the same result with a
  `~/.bash_profile` present, and no difference with none.
- **For `WSL-71`, WSLENV measured with `wsl.exe --exec /usr/bin/printenv`:** the variable
  set without being named in `WSLENV` did not reach the guest, in three spellings; named
  as `hereEnv` builds it, and appended to a caller's own list, it did.
- **For `WSL-71`, the five defects planted by hand** in copies cut by `write-file.mjs
  replace --expect 1`: each changed exactly one column of a six-column drive, and the
  restored file matched the unmutated row. 2 Go mutation rows red in `golang:1.25`.
- **For `WSL-68`, on the throwaway zero-grant base `wsl-toolkit-b68`**, arch, automount
  and interop off, no systemd, no sudo, no grant: `base ensure` exit 0 in 40.23 s. Every
  door tried as the account: drives, drvfs mounting, interop and passwordless sudo all
  refused; `/usr/lib/wsl/drivers` present and read-only; the internet reachable; the
  Windows host refused on 445 and 3389 and answering ICMP; `unshare -n` refused and
  `unshare -Un` granted. ⛔ `/mnt/wsl` written from the base and read from
  `wsl-toolkit`, and the reverse. Root's `umount /mnt/wsl` exit 0 in that distribution
  alone, the account's exit 32, and the mount back after `wsl --terminate`.
  `/etc/resolv.conf` resolves to `/mnt/wsl/resolv.conf`, so a new session after the
  umount answered `getent hosts` exit 2. `pasta --config-net` ran the probe in
  `net:[4026532318]` with the shared `net:[4026531833]` unchanged; inside it the internet
  answered and so did a ping to the Windows host, with and without `--no-map-gw`.
- **The suites, on this session's tree:** 315 top-level `wsl-toolkit` results on Windows
  with `TEMP` at the 8.3 path, 299 passed, 16 skipped, 0 failed, exit 0 in 16.5 s;
  `check-go.sh` exit 0 in `golang:1.25` in 32.15 s; ShellCheck 0.9.0 in `ubuntu:24.04`
  clean over **35** tracked scripts, one more than last session because
  `shell-profile.sh` is new. ⚠ It was clean by name BEFORE it was staged, because CI's
  command lists `git ls-files` and an untracked file is outside it.
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
7. ⭐ **Closed on 2026-09-15.** `examples/common/zellij.md` is deleted, the manual's
   link to it now names `herdr.md`, `examples/muse-code/README.md` is rewritten from
   eleven commands to three, and `examples/common/README.md` no longer tells a reader
   the agents' durable session is tmux. No live document names Zellij; the entries
   that do are recording a decision, and [`ENTRY.md`](ENTRY.md) forbids rewriting a
   title or a premise after the fact.
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
20. ⚠ **`base ensure` does not install the shell profile, and nothing says it should.**
    `provision.sh` and `verify.sh` write no login file at all, so a base built by `base
    ensure` alone carries no `~/.profile` line and `base shell --here`'s mark has nothing
    to mark for. `bootstrap.sh` run inside the base is what installs it. Found by
    `WSL-71`'s door sweep on 2026-09-15, recorded in the manual, and deliberately not
    changed: merging the two would put a URL-fetched file inside `base ensure`.
21. ⚠ **`--here` and `--root` together mark root's shell, and root has no profile to
    read it.** `base shell --root --here` takes the same branch and gets
    `WSL_TOOLKIT_HERE`, but `bootstrap.sh` installs the profile into the managed
    account's home, not root's. Harmless - the mark is read by nothing - and read rather
    than measured, on 2026-09-15.
22. ⛔ **`/dev/kvm`, `/dev/dxg` and `/dev/vsock` are `crw-rw-rw-` in a zero-grant base**,
    read on 2026-09-15 while attacking `WSL-68`'s doors. None was attacked, so what any
    of them reaches from an unprivileged account is unmeasured; they are named here
    because a sealed base's threat model has to answer for them and this session's
    enumeration did not.
23. ⛔ **The verifier's `automount off` check cannot fire on a guest whose automount
    root is not `/mnt`.** `verify.sh:34` hardcodes `drive_root=/mnt`, while
    `shell-profile.sh`, written this session, reads `[automount] root` from
    `/etc/wsl.conf`. So the two disagree about what "a Windows drive" is. ⚠ A base this
    tool BUILDS is unaffected: `provision.sh:473` writes an `[automount]` block with no
    `root` key, so the default holds. A distribution adopted with a hand-set root would
    have its drives mounted where the verifier does not look. Read on 2026-09-15 by
    `WSL-71`'s door sweep, not measured, and it is a value in two places with no check
    that they agree - they cannot be merged, because one is embedded in the executable
    and the other is fetched by URL and must work with no executable at all.
24. ⚠ **`base status --probe` reports no cgroup delegation on every base built this
    session**, arch at two different configurations, so a memory or cpu limit is accepted
    and not enforced. The remediation says the tool cannot repair it. That is `WSL-60`'s
    known condition and is recorded here only because two more builds met it.
25. ⭐ **Fixed on 2026-09-15:** `base attach` printed `base exec -c 'herdr agent list'` as
    the line for an agent, the quoted shell string `base herdr` exists to avoid. It
    prints `base herdr -- agent list`, and its case asserts the prefix.
26. ⚠ **The muse probe proves the reporter's registration by reading the file the
    install wrote, not by Muse running the hook**, which is the class
    [`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md)
    now has a row for. The registration was measured against Muse by hand on
    2026-09-15; nothing re-measures it when Muse changes its settings file again.
    Muse's `--provider echo` runs `SessionStart` with no credential, which is the
    candidate check.
27. ⚠ **herdr's Muse manifest `2026.08.26.1` matches no rule on Muse Code 1.3.0's input
    screen**, measured on 2026-09-15, so without the reporter herdr shows a Muse pane as
    `idle` whatever it is doing, and `agent prompt --wait` depends on the reporter's
    `working` to pass its five-second activity gate. Upstream's to fix.
28. ⚠ **A build of herdr's development branch answers `herdr 0.9.0`**, because the
    branch has not moved `Cargo.toml`. Any build this repository makes or ships cannot be
    told from the release by `--version`; its digest is the only identity.
29. ⛔ **The work order outlived the work by a session, and misrouted a resuming
    session.** Item 2 above still read "`WSL-67` has left: driving `pkgin` and
    `pkg_add`, which waits for the operator's approval of the two downloads" after
    `6f22e39` closed the entry on a driven NetBSD guest. A session resumed on
    2026-09-16 was sent to re-ask for an approval the operator had already given and
    to re-drive two package managers already driven. ⚠ **`INDEX.md` and the entry both
    read `done` throughout**, so the record disagreed with itself and only the work
    order was wrong; the three-way reconciliation the methodology asks for is what
    caught it. ⚠ **No check asserts that the work order agrees with `INDEX.md`**, and
    `check-record.sh` did not fire. That check is the fix and is not written.
30. ⚠ **A green `herdr-build.yml` matrix does not cover what the nightly builds.** The
    matrix was dispatched against a pinned ref, `052779c4159ed851`, while
    `herdr-nightly.yml` resolves herdr's development head at run time; by 2026-09-16
    that head had moved to `18061191fdc0`. So the first nightly built a commit no
    matrix run had ever built. Read on 2026-09-16, and it is the design working as
    written rather than a defect - recorded because "the matrix is green" is not the
    same sentence as "the nightly will build".
31. ⛔ **`consumer.ps1` downloads every asset a release carries, and from
    `wsl-toolkit-v3.0.0` that includes herdr's four builds.** `Get-Release`'s `gh` path
    is `gh release download` with no `--pattern`, and its no-`gh` fallback reads
    `SHA256SUMS` and fetches every name in it plus a `.cosign.bundle` for each;
    `release.yml` writes herdr's four builds into that same `SHA256SUMS`. So this
    repository's own reference runner, and the weekly release smoke that drives it,
    will fetch four herdr binaries neither uses, while
    [`../docs/consumers.md`](../docs/consumers.md) tells a consumer to download only
    the executable for its architecture and `SHA256SUMS`. Found by `WSL-90`'s door
    sweep on 2026-09-16. ⚠ **Now sized from the first nightly's own assets**, which are
    what `release.yml` would add: 26,235,080 + 24,100,240 + 9,635,803 + 8,348,489 =
    **68,319,612 bytes, about 65 MiB**, plus four bundles. `release-smoke.yml` runs
    `consumer.ps1` every Monday at 07:00 UTC, so that is the recurring cost from
    `wsl-toolkit-v3.0.0`. ⚠ It is a cost and a page disagreeing with a script, not a
    consumer break: no consumer in the register fetches `consumer.ps1`.
32. ⚠ **A killed `repo mutate` leaves its whole staged copy behind, and nothing sweeps
    it.** `.tmp\g\mutate-432424907`, **127 MiB**, has been there since 2026-09-14.
    `mutate.go:157` stages with `os.MkdirTemp("", "mutate-")` and `mutate.go:162` removes
    it with a `defer`, which a killed process never runs, so the copy survives in
    whatever `TEMP` named at the time - here `.tmp\g`, the 8.3 directory that session
    used. It is the same class as the killed matrix `wsl-toolkit gc` exists for, and
    `gc` does not cover it because the directory is not this tool's job state. Found on
    2026-09-16; the directory is left in place as evidence and is safe to delete by
    literal path.
33. ⛔ **A workflow's own logic has no harness here, and a real defect shipped inside
    one.** `herdr-nightly.yml`'s prune sorted by the wrong date field and would have
    deleted the newest nightly; it is fixed and driven by hand against a crafted list,
    and it has **no case and no mutation row**, because nothing in this tree runs a
    workflow's shell the way `repo mutate` runs Go and `check.sh` runs the tree. The
    gate's `mutations` check cannot point at a `jq` expression inside a YAML `run:`
    block. `consumer.ps1` has the same gap for the same reason - the amendment of
    2026-09-15 already noted it has no case harness - so this is one gap with two known
    occupants, and both are load-bearing: one publishes, one deletes. Found on
    2026-09-16 by `WSL-90`'s driven pass.

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

⭐ **2026-09-15, `WSL-71`.** Its door sweep found that a `base.mounts` grant is a DrvFS
mount exactly like an automounted drive, which is why the profile's guard is a path test
and not a filesystem-type test: the type test would have moved every shell out of the one
directory the caller was granted. It also found the `PATH` line that never reached a bash
login shell, fixed in the same change. ⛔ **Its guard mutation found a defect in its own
method for the second session running.** The first planting script used `awk`'s `sub()`,
whose first argument is an ERE, against anchors full of `*`, `$`, `{`, `?` and `|`; every
substitution silently failed, and all five rows read identically to the unmutated one -
which looks exactly like five guards that do nothing. Printing the unmutated row FIRST is
what made it legible, and `write-file.mjs replace --expect 1` cannot fail that way. ⛔ Its
claim audit rejected the entry's own first passing condition: "writes nothing to stderr"
is unmeetable on photon, whose `/etc/profile.d` writes 66 bytes to every login shell with
or without the file, so the assertion is now the DELTA between a run without the profile
and one with it. It also put the conditions back on a byte count that had been quoted as
though it were a constant.

⭐ **2026-09-15, `WSL-68`'s driven pass**, which is a fourth lens and started from the
attacker rather than from the code. It found the shared `/mnt/wsl` the entry's own door
list did not contain, and it found two of its own readings to be wrong before they were
written down - both because a check read a PIPELINE's status instead of the process's,
which is the absolute this repository states first and the one a scratch probe keeps
breaking. Its door sweep found finding 23, that this tree now holds two disagreeing
definitions of "a Windows drive under /mnt". `WSL-68` is a checkpoint and owes its three
closing reviews.

⭐ **2026-09-16, `WSL-90`'s checkpoint reviews**, owed by the session that was
checkpointed on 2026-09-15 and run by the one that resumed it.

**The door sweep** enumerated every release lookup in the tracked tree rather than the
two the entry named, with `git ls-files | xargs grep -l releases`: `LatestRelease` in
`release.go`, the nightly's in `herdr_nightly.go`, `Resolve-LatestTag` and `Get-Release`
in `consumer.ps1`, `remote.go`, `bootstrap.sh`, `release.yml` and `herdr-nightly.yml`.
⭐ Two of those are not exposed and the entry never said why: `bootstrap.sh` and
`remote.go` both read `/releases/latest`, which the API defines as excluding
prereleases, so no nightly can reach them. The nightly's own lookup is the mirror image
of `LatestRelease` - same hundred-a-page, same ten-page bound, and
`^herdr-nightly-[0-9]{8}-[0-9a-f]{12}$`, which is exactly the tag
`herdr-nightly.yml` builds - so the symmetric hazard, this tool's releases hiding the
nightlies, is closed too. ⛔ **It found one the entry's Consumers section missed:
`consumer.ps1` downloads EVERY asset a release carries.** Its `gh` path is `gh release
download` with no `--pattern`, and its fallback walks `SHA256SUMS` and fetches each
name plus a bundle. `release.yml` stages herdr's four builds into that same
`SHA256SUMS`, so from `wsl-toolkit-v3.0.0` this repository's own reference runner will
fetch four herdr binaries it never uses, where `consumers.md` tells a consumer to
download only "the executable matching the host architecture and `SHA256SUMS`". The
entry called the herdr files "additive", which is true of the release and not of the
fetch. Filed as finding 31.

**The guard mutation** took the guards in `herdr-nightly.yml`, which had never been
driven at all, and planted the defect each exists to catch, unmutated row printed
first: the development head's shape refused an empty string, 39 characters, uppercase,
a shell payload and an API error string; the prune refused this tool's own release,
an upstream tag, and a release standing behind a nightly; the staging refused a missing
target and a `SHA256SUMS` one line short. **14 rows, 14 as wanted, 0 red.** ⚠ **It
proves the logic and not the deployed step**: the harness transcribes each guard's
control flow into a function returning 1 where the workflow exits 1, so it is a reading
of the workflow rather than the workflow. The dispatched nightly is what drives the
steps themselves, and only their passing half.

**The claim audit** read the five corrected pages against `gh run list` and `gh release
list` rather than against the checkpoint's description of them, and the corrections
hold: `herdr-nightly` appears in no run listing and no `herdr-nightly-*` tag exists.
⛔ **The enumeration "five live pages" was right by luck.** A grep for the claim found
**six** files; the sixth, `CHANGELOG.md:28`, is a dated `**Deployed:**` line that the
changelog's own rule 4 requires and its "do not delete an entry" rule protects, so it
is history and correctly untouched - but nothing in the checkpoint showed it had been
looked at. ⛔ **The README's correction carried no date** where the other four carry
`read on 2026-09-15`, which is a claim quoted without its conditions; it now names the
date and says `herdr-build.yml` publishes nothing by design. ⛔ **And four of the five
corrections are false by the end of this session**, because it publishes the first
nightly: they are committed as the previous session's record, dated, and rewritten in
the nightly's own commit rather than left to rot.

## Open questions for the operator

⭐ The operator asked on 2026-09-15 to be asked only for what actually needs them: a
ruling this repository's own rules require, a download, a credential, or Windows
software. Two are open, each asked in chat on 2026-09-15:

1. **`muse login`**, the credential, in `wsl-toolkit --instance base base shell`.
2. **The six `--remote` signals in a real Windows Terminal window**, with the
   development client: a pseudo console measured all four input signals, and only a
   real window carries a real focus event.

Answered on 2026-09-15 and recorded as rulings 13 to 15: whether herdr builds are
published here, the targets, how the adapter takes a nightly, and where the
development server ran for `--machine`.

## Host state

- Registered distributions: `podman-machine-default`, `eph-pgb`, `wsl-toolkit`,
  `wsl-toolkit-podbox`, and ⭐ **`wsl-toolkit-base`, the operator's one base for every
  agent**, built on 2026-09-15 and kept. `eph-pgb` keeps its disk under
  `%LOCALAPPDATA%\wsl-ephemeral` and is not this tool's.
- ⭐ **`instances\base\config.json`** is `examples/muse-code/wsl-toolkit-base.json` plus
  one test grant, `.tmp\wsl78\proj` read-write at `/workspaces/proj`, a throwaway git
  project of three files.
- ⭐ **This machine's half of the herdr adapter is written**, as ruling 5 allows: the
  marked `Host wsl-toolkit-base` block at the top of `%USERPROFILE%\.ssh\config`, which
  now reads SHA-256 `72693CCF…E626AF`, one line in the dedicated `known_hosts`, and
  `%USERPROFILE%\bin\muse.exe`.
- ⚠ **`herdr machine add` saved a profile** `base`, id `e70f5d617fc95616…`,
  in `%LOCALAPPDATA%\herdr\client\endpoints.json`, a directory it created, and created
  the workspace `w2` in the base's herdr server.
- ⚠ **Rust 1.96.1 is installed through rustup for herdr's build, and rustup updated
  itself from 1.29.0 to 1.29.1** while doing it, which is its default and was not asked
  for. `.tmp\herdr` holds herdr's source at `052779c4159ed851` and its Windows build
  tree; `.tmp\herdr-master-052779c4` and `.tmp\herdr-linux-052779c4` hold the two built
  binaries.
- ⭐ **`wsl-toolkit-base` runs the published nightly's server**, since 2026-09-16.
  `instances\base\config.json` now carries `{"name": "herdr", "channel": "nightly"}`, and
  `base ensure` installed `herdr-nightly-20260916-18061191fdc0`, SHA-256
  `213580fc…f92f14a1`, which is that release's own `SHA256SUMS` line for
  `herdr-linux-x86_64`. It replaced the development server ruling 15 swapped in by hand,
  `978fde51…9827d75`. The configuration as it was before is kept at
  `.tmp\base-config-before-nightly.json`. ⚠ The base's own `--version` still answers
  `herdr 0.9.0`, finding 28, so the digest above is its only identity.
- ⭐ **The nightly's Windows client is under the instance's state directory**,
  `instances\base\herdr\herdr-nightly-20260916-18061191fdc0\herdr.exe`, written by
  `base ensure` and printed by `base attach`. ⚠ **It, not `.tmp\herdr-master-052779c4`,
  is the client that matches the server the base now runs**, and it is what the
  operator's `--remote` test should use.
- ⚠ **A verified copy of the nightly is under `.tmp\nightly-verify`**, 12 files and about
  68 MiB, kept as the evidence behind this session's verification and safe to remove by
  literal path.
- ⚠ **Running herdr's Windows clients in a pseudo console created
  `%APPDATA%\herdr`**, which did not exist: a 19-byte `config.toml` reading
  `onboarding = false`, written by the development client, and `herdr-client.log`. Kept
  so the operator's own `--remote` test is not met by the onboarding overlay that
  swallowed the probe's keys.
- ⚠ **`%LOCALAPPDATA%\herdr\remote` holds three `ssh-PID-0` directories and three
  `.sock` files**, left by the three pseudo-console runs that were killed, at 11:19,
  11:20 and 11:21Z; the two runs that detached with exit 0 left none.
- The shared `/mnt/wsl` was not written to this session.
- ⚠ **A scratch serial-console driver for the NetBSD and OpenBSD guests is at
  `.tmp\bsd67-driver`**, untracked: it boots through an overlay, answers the boot loader and
  the OpenBSD installer over SeaBIOS's serial console, and runs commands from a spool with
  files served over QEMU's own TFTP. It has never booted a guest; read it before using it.
- `instances` under `%LOCALAPPDATA%\wsl-toolkit` holds `acc`, `muse`, `nobase`,
  `podbox` and `podbox-migrate`, as at the start. `instances\muse` holds nothing.
- No BSD image is downloaded. The shared FreeBSD image is the published one,
  6,476,638,208 bytes, SHA-256 `12807CE7…921663BF`, not booted this session.
- herdr 0.9.0 is installed on Windows by the operator, through scoop, and is the `herdr`
  on `PATH`; the `wsl-toolkit` on `PATH` is `%USERPROFILE%\bin\wsl-toolkit.exe`.
- The dedicated SSH key the ruling allows is at
  `%USERPROFILE%\.ssh\wsl-toolkit\id_ed25519`, with the one-line `known_hosts` above
  beside it, and stays for `wsl-toolkit-base`.
