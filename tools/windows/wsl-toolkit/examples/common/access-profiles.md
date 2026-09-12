# Provider-base access profiles

Put a profile at the root of the ONE Windows checkout the provider may edit and
name it `wsl-toolkit.json`. A relative mount source is resolved against that
file, not against the shell's current directory.

## One read/write checkout

This profile belongs with `--instance muse`, whose distribution is
`wsl-toolkit-muse`:

```json
{
  "schema": "wsl-toolkit-config/1",
  "base": {
    "name": "wsl-toolkit-muse",
    "image": "ghcr.io/pkgforge-dev/archlinux:latest",
    "user": "toolkit",
    "automount": "off",
    "interop": "off",
    "systemd": true,
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
- the one DrvFS entry above mounts only the resolved checkout, at the fixed
  guest path `/workspaces/project`.

`base ensure` verifies the live mount after restarting WSL. Changing or removing
the grant makes the next ensure reject the stale live mount, rewrite the
tool-owned `/etc/fstab` block, restart, and verify again.

## Zero configured host-directory grants

Use a different instance and omit `mounts`:

```json
{
  "schema": "wsl-toolkit-config/1",
  "base": {
    "name": "wsl-toolkit-sealed",
    "image": "ghcr.io/pkgforge-dev/archlinux:latest",
    "user": "toolkit",
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

This produces zero user-directory DrvFS mounts for the ordinary `toolkit`
account, and that account cannot mount a Windows drive itself. It is useful for
an experiment that needs a persistent Linux home and no project checkout.

⛔ **This is not yet a security boundary and is not called a sealed sandbox.**
WSL exposes `/usr/lib/wsl/drivers` through its own read-only internal mount,
network access is unchanged, and guest root can request manual DrvFS mounts.
`base shell --root` is therefore an administration escape hatch. Use the
ordinary account for the profile above; use a VM or another actual sandbox when
the process must be hostile to guest root or to the network. `WSL-68` tracks the
remaining attack-and-report work.
