# Provider-base access profiles

Put a profile at the root of the ONE Windows checkout the provider may edit and
name it `wsl-toolkit.json`. A relative mount source is resolved against that
file, not against the shell's current directory.

Each named instance manages one workload account from `base.user`. The account
is UID 1000, owns `/home/<user>`, and receives private persistent
`~/.config`, `~/.cache`, `~/.local/share` and `~/.local/state` directories.
Use separate instances when agents need different names, homes, grants or sudo
authority. Changing `base.user` on an existing instance is refused until the
operator runs `base recreate`, because the old account owns rootless engine and
mount state.

## One read/write checkout

This profile belongs with `--instance muse`, whose distribution is
`wsl-toolkit-muse`:

```json
{
  "schema": "wsl-toolkit-config/1",
  "base": {
    "name": "wsl-toolkit-muse",
    "image": "ghcr.io/pkgforge-dev/archlinux:latest",
    "user": "muse",
    "automount": "off",
    "interop": "off",
    "systemd": true,
    "passwordless_sudo": true,
    "toolset": "developer",
    "mounts": [
      {
        "source": ".",
        "target": "/workspaces/project",
        "mode": "rw"
      }
    ]
  },
  "jobs": {
    "container_lifecycle": "persistent"
  }
}
```

`base.mounts` is accepted only when both ambient doors are closed:

- `automount: "off"` removes the ordinary `/mnt/<drive>` mounts;
- `interop: "off"` removes the Windows executable socket, so a Windows process
  cannot be used to reach an unmounted path;
- `passwordless_sudo: true` lets the trusted Muse agent run `sudo -n` without
  stopping for an operator password;
- the one DrvFS entry above mounts only the resolved checkout, at the fixed
  guest path `/workspaces/project`.

`base ensure` verifies the live mount after restarting WSL. Changing or removing
the grant makes the next ensure reject the stale live mount, rewrite the
tool-owned `/etc/fstab` block, restart, and verify again.

⚠ **Passwordless sudo is deliberate agent authority, not a sandbox.** The
`muse` account can change any guest file and guest root can request a manual
DrvFS mount outside `/workspaces/project`. Use this profile only for an agent
trusted with that authority. `base ensure` validates the tool-owned sudoers
fragment before activation and then proves the account can run `sudo -n true`.

## Low-authority agent with zero host-directory grants

Use a different instance and omit `mounts`:

```json
{
  "schema": "wsl-toolkit-config/1",
  "base": {
    "name": "wsl-toolkit-malaria",
    "image": "ghcr.io/pkgforge-dev/archlinux:latest",
    "user": "malaria",
    "automount": "off",
    "interop": "off",
    "systemd": true,
    "toolset": "developer"
  },
  "jobs": {
    "container_lifecycle": "persistent"
  }
}
```

This produces zero user-directory DrvFS mounts for the ordinary `malaria`
account, and that account cannot mount a Windows drive itself. It is useful for
an experiment that needs a persistent Linux home and no project checkout.
`passwordless_sudo` is omitted, so it defaults to false: `malaria` cannot use
`sudo -n`. Its Linux home and sudoers state are in a separate named distribution
from Muse and it receives no configured Windows path.

⛔ **This is not yet a security boundary and is not called a sealed sandbox.**
WSL exposes `/usr/lib/wsl/drivers` through its own read-only internal mount,
network access is unchanged, and guest root can request manual DrvFS mounts.
`base shell --root` is therefore an administration escape hatch. Use the
ordinary account for the profile above; use a VM or another actual sandbox when
the process must be hostile to guest root or to the network. `WSL-68` tracks the
remaining attack-and-report work.
