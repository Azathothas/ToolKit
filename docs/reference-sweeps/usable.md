# usable.md

⭐ **The file a later session acts on.** Commands that were run, outputs that
were seen, and the lessons, so the next session does not re-derive any of it.

[`findings.md`](findings.md) carries the verdicts and the argument. This one
carries the evidence and the recipes.

⚠ Everything here was measured on **one Windows 11 Pro 26200 machine on
2026-08-27**, podman 5.8.6, PowerShell 7.6.5, with a WSL2 podman machine
running Fedora. A measurement carries its conditions or it is not a
measurement.

---

## The one command that settles the design

```bash
podman run --rm ghcr.io/freebsd/freebsd-runtime:15.1 /bin/sh -c 'uname -a'
```

```text
WARNING: image platform (freebsd/amd64) does not match the expected platform (linux/amd64)
```

Exit code **139**. No stdout. 139 is 128 + 11, a **SIGSEGV**.

⛔ **Read that number carefully before designing anything.** It is not
`Exec format error`. The Linux ELF loader accepted a FreeBSD binary and the
binary died on its first syscall. `binfmt_misc` and `qemu-user` are both
irrelevant: they solve a foreign **architecture** presenting **Linux** syscalls,
and this is a native architecture presenting **FreeBSD** syscalls. Linux has no
reverse Linuxulator.

⭐ **A FreeBSD userland requires a FreeBSD kernel.** Everything else in this
file follows from that one line.

---

## Pulling a FreeBSD image at all

The plain pull is refused, and the refusal is correct:

```bash
podman pull ghcr.io/freebsd/freebsd-runtime:15.1
```

```text
Error: ... no image found in image index for architecture "amd64", variant "", OS "linux"
```

⚠ **`--os` is required, and it is separate from `--platform`.** Reaching for
`--platform linux/amd64` out of habit asks for an image that does not exist.

```bash
podman pull --os freebsd --arch amd64 ghcr.io/freebsd/freebsd-runtime:15.1
```

34 MB, and `podman image inspect --format '{{.Os}}/{{.Architecture}}'` reports
`freebsd/amd64`.

⚠ **That pull retags the shared local tag**, which is the trap from
`Azathothas/TEMPLATE` issue 2. Remove the image afterwards, or a later
unqualified pull of the same name is a no-op that serves the FreeBSD copy.

---

## What already exists, so it is not built again

⛔ **Do not build FreeBSD OCI images.** The FreeBSD project publishes them.

```bash
curl -fsSL "https://ghcr.io/token?scope=repository:freebsd/freebsd-runtime:pull&service=ghcr.io" | jq -r .token
```

Use that bearer token against `https://ghcr.io/v2/freebsd/freebsd-runtime/tags/list`.
Tags present on 2026-08-27:

```text
14.4.rc1  14.4  14.5.beta1  14.5.beta2  14.5.beta3
15.1.beta1  15.1.beta2  15.1.beta3  15.1.rc1  15.1.rc2  15.1.rc3  15.1
```

The manifest is an index with `{"architecture":"arm64","os":"freebsd"}` and
`{"architecture":"amd64","os":"freebsd"}`.

The same releases are at `https://download.freebsd.org/releases/OCI-IMAGES/`,
covering 14.3 through 15.1. ⚠ `amd64`, `aarch64` and `riscv64` were confirmed
present for the 14.3, 14.4, 15.0 and 15.1 **RELEASE** builds specifically; the
beta and RC directories were listed but not opened.
⭐ Prefer the download server when a chain of trust matters; the handbook says
so and it is the project's own advice.

---

## The runtime, on a FreeBSD host

From the handbook. ⚠ Not run by this session, because no FreeBSD host existed:

```bash
pkg install -r FreeBSD -y podman-suite
```

```bash
podman load -i=FreeBSD-15.1-RELEASE-amd64-container-image-static.txz
```

The runtime underneath is jails, through `ocijail`. `runj` is the other
implementation and is described by its own author as a proof of concept.

---

## ⭐ The client mechanism that removes the need for any wrapper

This is the most reusable thing the sweep produced, and it is visible on any
machine with podman:

```bash
podman system connection list
```

```text
podman-machine-default  ssh://user@127.0.0.1:53512/run/user/1000/podman/podman.sock
```

⭐ **A podman connection is an ordinary SSH URI to a podman socket.** Nothing
about it is special to `podman machine`. So a podman running anywhere reachable
over SSH, including a FreeBSD guest, is addressable from the Windows client:

```bash
podman system connection add freebsd ssh://user@HOST/var/run/podman/podman.sock
```

```bash
podman -c freebsd run --rm -it ghcr.io/freebsd/freebsd-runtime:15.1 /bin/sh
```

⛔ **This is why no `podman` wrapper script should be written.** The original
plan considered shadowing `podman` with a script that manages machines. `-c`
and `podman system connection default` already do it, and a wrapper that shadows
a real binary to reimplement one of its own flags is the kind of rebuilt
machinery
[`../conventions/forbidden-patterns.md`](../conventions/forbidden-patterns.md)
has a row for.

---

## Lessons

| tag | lesson |
| --- | --- |
| `adopt` | ⭐ Read the exit code, not the error text. 139 versus `Exec format error` is the difference between "needs binfmt" and "architecturally impossible". The text looked adjacent to a problem already solved; the number said otherwise. |
| `adopt` | A podman connection is just SSH. Check `podman system connection list` before writing anything that manages podman. |
| `adopt` | ⭐ Read the tracker. The maintainer's two refusals and the stalled pull request are the whole cost picture, and none of it is in any README. `references.md` calls this the step that gets skipped, and it was the highest-value hour of the sweep. |
| `avoid` | Do not build FreeBSD OCI images. The FreeBSD project publishes them at the same registry the plan intended to push to. |
| `avoid` | Do not plan around `containers/podman#19939`. Open, unmerged, and refused twice by the maintainer. |
| `avoid` | Do not follow the `podman machine init --image` suggestion literally. It requires Ignition, which FreeBSD does not have, so it starts with porting a CoreOS provisioning system. |
| `honest-limit` | ⛔ There is no route to a FreeBSD userland from Windows that avoids a hypervisor. The achievable minimum is one, Hyper-V, not nested. Anyone promising otherwise has not run the command at the top of this file. |
| `honest-limit` | "The most popular BSDs" is one BSD. NetBSD and OpenBSD publish no OCI images and have no jail-equivalent OCI runtime. |
| `future` | The FreeBSD-native model that OCI does not express: a read-only base over `nullfs` with a read-write overlay, worth about 500 MB against a full base. Revisit if image size becomes the constraint. |

---

## What this file does not know

⛔ Stated rather than left to be discovered:

- ⭐ **Hyper-V and WSL2 coexisting is MEASURED, and it was the one open
  assumption.** On 2026-08-27, with the WSL2 podman machine running:
  `HypervisorPresent` is `True`, `Get-Service vmms` reports **Running** with
  startup `Auto`, and the `Hyper-V` PowerShell module is present at v2.0.0.0.
  ⚠ `Get-WindowsOptionalFeature` needs elevation, so the definitive feature
  list was not read; a running `vmms` is the stronger evidence anyway.
  [`../../TODO/bsd.md`](../../TODO/bsd.md) carries the probes.
- **No FreeBSD host was available**, so nothing in "The runtime, on a FreeBSD
  host" was executed. It is quoted from the handbook.
- **`runj` and `ocijail` were checked for liveness, not read.** No claim here
  depends on their internals.
- **`github.com/orgs/freebsd/packages` was not reachable** with this token's
  scopes. The registry was queried anonymously instead.
