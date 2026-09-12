# PROGRESS.md

Current work lives here; [INDEX.md](INDEX.md) owns the entry list and
[RULES.md](RULES.md) owns standing policy. History is in git and the entries.

## State

```text
session started 2026-09-12T11:00:00Z
baseline        d67c1e6, dirty main; gate 19 checks green against inherited
                issue-29 work
entries         total 100  open 3  blocked 0  done 97
gate            19 checks green after the checkpoint edit
head            local changes not yet committed or pushed
```

## Active work

⭐ **[Issue 29](https://github.com/Azathothas/ToolKit/issues/29) is complete in
the tree.** All seven requested features were already represented at `d67c1e6`.
The remaining Linux CI failure was a race in
`TestAFileThatGrowsDoesNotKillTheCopy`: the production copy now writes exactly
the size in the member header through `writeRegularMember`, and the regression
passes an intentionally stale `FileInfo` instead of racing the scheduler. The
full live acceptance runner passed **71 of 71**, cleanup returned to its
baseline, and unrelated distributions were preserved. The remote issue remains
open until this checkpoint is pushed and its CI result is read.

⭐ **[Issue 30](https://github.com/Azathothas/ToolKit/issues/30) is substantially
started, not complete.** `WSL-67` now has the provider-neutral base, explicit
access grants, systemd/developer provisioning, common bootstrap, tmux policy,
and live WSL evidence. `WSL-68` has a measured zero-grant mode for an
unprivileged guest, but remains open because WSL root and network reachability
mean this is not yet a security boundary.

## What was built

- `BaseConfig` gained `interop`, `systemd`, `toolset`, and explicit `mounts`.
  Mount grants require automount and interop off, canonicalize host sources,
  reject unsafe roots and duplicate sources/targets, and confine guest targets
  below `/workspaces`.
- Provisioning owns the grant block in `/etc/fstab`, supports systemd and a
  portable `developer` toolset, and restarts WSL before effective verification.
  A fresh Arch rootfs now uses a full `pacman -Syu`, avoiding partial-upgrade
  failure.
- Verification runs as the unprivileged user and checks effective automount,
  interop, systemd, developer commands, live DrvFS type, requested ro/rw mode,
  and the exact allowlist. WSL's internal read-only driver mount is recognized
  separately.
- `base shell --here` refuses when automount is off. A root shell states that
  root can manually mount more host paths; the feature is not called a sandbox.
- New-base failures before the identity marker now unregister and delete the
  incomplete distro through a fresh bounded rollback context. This was found
  live when the first Arch provisioning attempt failed.
- `examples/common/` installs pinned CodeGraph 1.5.0 npm packages after SHA-512
  verification and carries a tmux configuration; `examples/muse-code/` is the
  end-to-end operator guide. The Muse installer remains an explicit reviewed
  operator step because its official documentation is login-gated.

## Measurements

Read from Windows 11 Pro 26200 on 2026-09-12:

```text
issue-29 acceptance  71/71 passed; cleanup clean; unrelated WSL distros kept
go suites            green after every implementation slice
mutation proof       14 planted cases across 4 new rows, all refused
live provider base   systemd PID 1, Podman 6.1.1, 31 QEMU handlers,
                     developer toolset present, exactly one rw grant
zero-grant transition stale grant refused then removed; no user-directory
                     DrvFS mount; C: mount refused to unprivileged user;
                     interop absent; systemd retained
common bootstrap     CodeGraph 1.5.0, tmux 3.7c; both npm SHA-512 digests
                     verified; remain-on-exit on; exit-empty off
teardown             disposable distro marker-verified and removed; remaining
                     names: podman-machine-default, eph-pgb, wsl-toolkit,
                     wsl-toolkit-podbox
```

## What is left

1. Run the final local gate after this record edit, commit through
   `git-sync.ps1`, push `main`, and post this checkpoint to issues 29 and 30.
2. Read the pushed CI. Close issue 29 only when the new Linux Go run confirms
   the flaky regression fix.
3. For `WSL-67`, add the provider-profile scenarios to the main acceptance
   runner; drive the developer package mapping on non-Arch families; and run
   the Muse installer/authenticated smoke when operator access is available.
4. For `WSL-68`, design and drive the attacking sealed-base probe: network,
   WSL service/init channels, guest root manual mounts, and any Podman/fuse
   consequences. Publish a threat model before using security-boundary terms.
5. Add a focused automated regression for pre-marker base rollback. The live
   failure proved the repair, but it deserves a deterministic unit test.
6. `WSL-59`, the low-level PowerShell adapter, remains unchanged and open.

## Review findings

- **Door sweep:** all new base-config doors converge on validation,
  provisioning and effective verification. It found the pre-marker failure
  path that left a registered disposable distro; rollback was added and the
  leaked probe was removed.
- **Guard mutation:** all four new policy rows were planted independently and
  went red. An accidental broad replacement in unrelated mutation rows was
  detected during the pass and restored before proof.
- **Claim audit:** corrected the stale 23-case acceptance claim to 71, rejected
  the phrase "sealed base", and records the internal WSL driver mount, network
  gap, and root capability rather than claiming isolation.

## Open questions for the operator

None required to resume. Muse credentials/access are needed only for the final
provider-specific smoke; all provider-neutral work can continue unattended.

The existing unrelated host state is untouched: `eph-pgb` and
`wsl-toolkit-podbox` remain registered, as do `podman-machine-default` and the
ordinary `wsl-toolkit` base.
