# consumers.md

Who depends on this repository, what they hold, and what they must read before
moving.

⭐ **This is the file that makes ToolKit different from an ordinary project.**
The consumers of a tool here are not in this tree, so nothing in this repository
can fail when their contract is broken. Only they can, later, on a machine
nobody is watching.

---

## The rule

⛔ A consumer owns its migration. Work in ToolKit does not edit another
repository, advance its pin, or preserve a deleted compatibility product.
Before moving, the consumer reads the latest
[`wsl-toolkit` manual](../tools/windows/wsl-toolkit/wsl-toolkit.md) and release
notes directly; copied invocation prose is not an interface.

⚠ **The register is a lower bound on who is affected, never the complete set.**
Operator scripts and machines that are not visible from this repository may also
hold old paths or releases, so a change to a fetched file says which consumers
were checked rather than assuming the answer.

---

## The register

Read from each repository on 2026-09-13.

| consumer | current relationship | what it must do before moving |
| --- | --- | --- |
| `Azathothas/TEMPLATE`, at `docs/containers.md` and `docs/agent-tooling.md` | describes the tool as a PowerShell product and an executable, with a launcher | rewrite its guide from the latest manual |
| `Azathothas/bit-cli`, at `scripts/wsl-tool.ps1` and `docs/containers.md` | its wrapper runs the deleted launcher at a pinned commit with SHA-256 pins, and its guide fetches `wsl-ephemeral.ps1` by raw URL at a commit it resolves | keep its current pin until it chooses to move, then choose an executable release, verify it, and translate its own invocation from the latest manual |
| `pkgforge-dev/cross-libc-dlopen`, at `scripts/wsl-ephemeral.ps1` | carries a vendored copy and fetches nothing from ToolKit | nothing here can update it; its owner decides whether to replace the copy |

Two files are intended for direct fetching and have no known consumer:
[`bootstrap.sh`](../scripts/common/bootstrap.sh) and
[`tmux.conf`](../scripts/common/tmux.conf). Add a row when a consumer is found.

The dependency on `pkgforge-dev/docker-bsd` runs the other way: it publishes
BSD images that ToolKit names. It is not a consumer row.

⛔ **The PowerShell product, its launcher and `wsl-toolkit script` are
deleted.** A raw fetch of any file under `scripts/windows/wsl-toolkit/` at a
later commit returns 404, a commit before the deletion still serves it, and
every release from `wsl-toolkit-v3.0.0` publishes only the executables. The
`distro` and `hostaddress` commands carry what the product did.
[`HISTORY/consumers.md`](HISTORY/consumers.md) keeps the pin-state table this
page carried for it.

---

## What counts as a break

A change breaks a caller when a correct existing call behaves differently:

| change | effect |
| --- | --- |
| file removed or moved | its raw URL no longer names a runnable product |
| flag renamed or retyped | the call refuses or binds differently |
| exit meaning changed | automation reads a different verdict |
| output schema or unflagged stream changed | a parser reads different bytes |

Fixing a false pass is still a break and should still be fixed. Record it where
the work closes; do not keep a defective surface solely because a caller may
depend on it.

---

## Fetching the published product

A release carries:

- `wsl-toolkit-windows-amd64.exe`
- `wsl-toolkit-windows-arm64.exe`
- `SHA256SUMS`
- one `<asset>.cosign.bundle` per file above

Use an immutable `wsl-toolkit-v*` release. Download the executable matching the
host architecture and `SHA256SUMS`, verify the executable's SHA-256, then
verify its keyless bundle against `.github/workflows/release.yml` in
`Azathothas/ToolKit`. A digest proves transport integrity; the signature binds
the asset to this repository's release workflow.

```powershell
$want = (Select-String -Path SHA256SUMS -Pattern 'wsl-toolkit-windows-amd64.exe').Line.Split(' ')[0]
(Get-FileHash wsl-toolkit-windows-amd64.exe -Algorithm SHA256).Hash.ToLowerInvariant() -eq $want
cosign verify-blob --bundle wsl-toolkit-windows-amd64.exe.cosign.bundle --certificate-identity-regexp '^https://github\.com/Azathothas/ToolKit/\.github/workflows/release\.yml@' --certificate-oidc-issuer https://token.actions.githubusercontent.com wsl-toolkit-windows-amd64.exe
```

Never pipe a download into a shell. Save it, verify it, then run it.

⚠ **A pinned consumer does not get a fix by its being merged here.** Pinning
protects a consumer from a change it did not review, which is exactly why it also
withholds a fix it would have wanted.

---

## Before changing a fetched contract

1. Read this register and name every affected row in the work record.
2. Update ToolKit's current manual, release checks and consumer smoke in the
   same change.
3. Leave migration to each consumer and tell it to read the latest manual.
4. Preserve prior events only in
   [`HISTORY/consumers.md`](HISTORY/consumers.md), not in this live page.
