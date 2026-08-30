# wsl-toolkit

The tool, its sources, and how to change it.

[`wsl-toolkit.md`](wsl-toolkit.md) is what the tool does, for somebody using it.
This is what it is made of, for somebody changing it.

---

## The shape

```text
scripts/windows/wsl-toolkit/
  wsl-toolkit.ps1     ⛔ GENERATED. The product. Tracked and released.
  bundle.manifest     the parts, in the order they are joined
  surface.lock        the CLI surface the product promises
  build.ps1           joins the parts, and proves the result
  release.ps1         verifies, then tags. It does not publish.
  launcher.ps1        fetch, verify and run the product from anywhere
  selftest.ps1        the suite over the product's pure functions
  src/                the entry point: help, the parameter block, the dispatch
  core/               the distro, the command channel, the safety model, the actions
  libs/               helpers that do not know what WSL is
```

⭐ **The single file is the PRODUCT and the parts are the SOURCE.** It is tracked
rather than built on demand because a consumer fetching one raw URL cannot run a
build step, and that one-URL contract is the whole reason the launcher can verify
anything: one URL, one digest, one thing to check.

⛔ **Do not edit `wsl-toolkit.ps1`.** An edit there is lost the next time anything
runs the build, and the gate refuses a bundle that disagrees with its parts, so
it is lost loudly rather than quietly.

⚠ **The join order is not alphabetical and cannot be.** PowerShell requires
`param()` to be the first statement in a script and comment-based help to come
before it, so `src/00-help.ps1` and `src/10-parameters.ps1` lead. Everything
after them is function definitions, which the engine hoists, so their order is
for a reader. `src/99-main.ps1` is last because it is the only part that runs
rather than declares.

---

## Changing it

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/build.ps1
```

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/build.ps1 -Test
```

⭐ **Run `-Test` before every commit.** It is what the local gate and both CI
jobs run, and it is five things rather than one:

| what it proves | how it fails |
| --- | --- |
| every part parses on its own | a broken part names ITSELF, instead of a line number 2,000 lines from where you edited |
| ⭐ no case-shadowed parameter | a local spelled `$state` beside a parameter `$State` IS that parameter, because PowerShell ignores case. That shipped once and only driving a real distro found it |
| the CLI surface matches `surface.lock` | a renamed or retyped parameter fails a gate instead of failing a caller nobody can reach |
| the selftest passes against the built bundle | any of its cases |
| PSScriptAnalyzer is clean over the product | any rule at Error or Warning |

⚠ **The parts are excluded from the analyzer and that loses no coverage.** A
script-scoped `SuppressMessageAttribute` covers only its own file, and all of
this tool's suppressions live in its parameter block, so analysing a fragment
reports every rule those suppressions exist to answer. ⭐ The analyzer runs over
the BUILT bundle, which is every line of every part, and the gate asserts the
bundle is exactly what those parts build.

### Adding a part

1. Write it under `src/`, `core/` or `libs/`.
2. Add it to [`bundle.manifest`](bundle.manifest), in the position a reader
   should meet it.
3. Rebuild.

⛔ **Skipping step 2 is a refusal, not a silent omission.** The build enumerates
every `.ps1` under those three directories and compares that set against the
manifest in both directions: a listed part that does not exist, and an existing
part that is not listed, each stop the build by name.

### Changing the parameter surface

`surface.lock` is the record of what the CLI promised last time.
[`../../../docs/consumers.md`](../../../docs/consumers.md) calls a renamed
parameter and a changed type two of the three things that break a caller who did
nothing wrong; this is what turns them into a failed gate.

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/build.ps1 -Test -UpdateSurface
```

⛔ **Never automatic.** A refresh in the same commit as the rename is the record
that the rename was a decision. A refresh on its own is the record that nobody
looked.

---

## Releasing it

⭐ **Two halves, on purpose.** `release.ps1` verifies and pushes a tag;
[`../../../.github/workflows/release.yml`](../../../.github/workflows/release.yml)
checks that tag out clean, re-runs the same verification and publishes. A release
cut from a developer's machine could come from a dirty tree, an unpushed commit
or a stale bundle, and none of those is visible in the artefact afterwards.

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/release.ps1
```

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/release.ps1 -Publish
```

Without `-Publish` it verifies and prints what it would do. What it refuses:

- a dirty working tree, because the asset would match no commit;
- a bundle that disagrees with its parts;
- a failing `-Test`;
- a tag that already exists, locally or on the remote. ⛔ A moved tag runs code
  nobody reviewed, which is the rule the launcher is built around;
- `HEAD` not on any remote branch, because CI cannot check out a commit the
  remote does not have.

### The version has one home

`$script:ToolkitVersion` in [`src/20-prelude.ps1`](src/20-prelude.ps1). The tag
is `wsl-toolkit-v<version>`, and both `release.ps1` and the workflow read the
version out of the **built bundle** rather than out of the source, so a tag can
never name a version the published file does not carry. ⛔ Nothing else in this
tree may hold a copy of it.

### What a release carries

| asset | what it is |
| --- | --- |
| `wsl-toolkit.ps1` | the product, byte for byte as this tree holds it |
| `launcher.ps1` | the wrapper that fetches and verifies it |
| `SHA256SUMS` | ⭐ computed in CI over the bytes that are uploaded |

⚠ **The digests are computed in the workflow, not here**, and that is not
bureaucracy. A `.ps1` is CRLF in a working tree and LF in the git index, so a
digest taken on a developer's machine is a digest of different bytes from the
one a consumer downloads. `release.ps1` prints the working-tree digests with a
line saying not to copy them anywhere.

⚠ **The release SHA256SUMS proves transport, not authorship.** It comes from the
same release as the asset, so anyone who could replace one could replace the
other. `-LauncherSha256` with a digest the caller holds is the check that proves
authorship, and it applies on top.

---

## The suite

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/selftest.ps1
```

⭐ **It needs no WSL and no container engine**, so it runs on every host with a
PowerShell, which is where its coverage comes from: both CI jobs run it, and one
of them is Ubuntu with a different default culture.

⛔ **It asserts the number of cases it ran.** A table that stopped early exits 0
over a smaller suite, which is the shape a check takes on its way to reporting
nothing.

⚠ **It loads functions out of the built bundle by parsing it**, because the
product calls `exit` at its top level and dot-sourcing it would end the session.
A renamed function is therefore a failure of the suite to follow a rename, and
it says so: fix the list, do not delete the cases.

⛔ **What it does not cover, said plainly:** anything that talks to `wsl.exe`, to
a container engine or to the filesystem. Those are proved by running them, which
is part (b) of [`../../../docs/methodology/gate.md`](../../../docs/methodology/gate.md).
Every defect this session found in the relay was found that way and could not
have been found here.

---

## Related

- [`wsl-toolkit.md`](wsl-toolkit.md), the tool itself
- [`launcher.md`](launcher.md), fetching and verifying it from another project
- [`../../../docs/consumers.md`](../../../docs/consumers.md), who fetches this
  and what breaks them
- [`../../README.md`](../../README.md), the contract every script here is held to
