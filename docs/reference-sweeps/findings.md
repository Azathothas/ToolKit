# findings.md

The reference sweep behind [`../../TODO/bsd.md`](../../TODO/bsd.md): what was
read, what each one is worth, and the reasoning behind each verdict.

[`usable.md`](usable.md) is the other half, and it is the one a later session
reads. This file carries the verdicts and the argument; that one carries the
commands.

⛔ **Read [`../methodology/references.md`](../methodology/references.md) before
adding to this file.** It is binding on any task whose verb is clone, mine,
survey or investigate.

---

## Provenance

⚠ **Nothing here was cloned**, so there is no commit to record for most rows.
These are published artefacts and a tracker, read over HTTPS on **2026-08-27**.
Where a row has no commit, the date is the only provenance and a later session
re-reads rather than trusting it. [`references.md`](../methodology/references.md)
trap 7: projects move.

| # | reference | reached | depth |
| --- | --- | --- | --- |
| R1 | `docs.freebsd.org` handbook, containers chapter | ✅ | read, one pass |
| R2 | `github.com/orgs/freebsd/packages` | ⚠ partial | ⛔ the HTML page needs `read:packages`, which this token does not carry. Reached the **registry** anonymously instead, which is better evidence: it answers what is actually published rather than what a page lists. |
| R3 | `download.freebsd.org/releases/OCI-IMAGES/` | ✅ | directory listing, three levels |
| R4 | `cbsd/cbsd` `share/docs/general/cbsd_oci.md` | ✅ | read in full, 92 lines |
| R5 | `containers/podman#25230` | ✅ | body **and all 6 comments**, plus the two pull requests it names |

⛔ **What was not done**, stated rather than left to be inferred:

- No repository was cloned, so no source is cited at file and line. Every
  verdict below rests on published artefacts, a tracker, and local measurement.
- `runj` and `ocijail` were checked for **liveness only**, not read. Neither
  verdict below depends on their internals.
- The four-pass reading in `references.md` was not taken over any of these.
  ⚠ Four passes is for a codebase being mined for a mechanism. R1, R3 and R4 are
  documents and R5 is a tracker; a second pass over a directory listing would be
  the same pass written twice. R5 got the tracker pass, which is the one
  `references.md` says gets skipped, and it is where the decisive evidence was.

---

## Verdicts

### R5, `containers/podman#25230`: ⭐ adopt, and it changed the plan

**The single most valuable reference, and it is a tracker, not code.** It
carries three things that appear in no README:

1. ⛔ **The maintainer's refusal, twice.** `Luap99`: "This seems like a
   maintenance burden for us maintainers without much benifit", then "My
   position has not changed." That is a costing no amount of reading the code
   would produce.
2. **`containers/podman#19939`**, `davidchisnall`, +135/-30, **open and
   unmerged**. The "it is only 100 lines" argument in the thread is true and
   irrelevant; the objection is maintenance, not size.
3. ⭐ **`baude`'s escape hatch**, which the operator quoted: a custom machine
   image with Ignition, via `podman machine init --image`. Read in context it is
   "nothing stops you doing it yourself", not a plan. ⛔ It is refused in
   `BSD-01` because FreeBSD has no Ignition.

⚠ `afbjorklund`'s question in the same thread is the one worth keeping: why run
a FreeBSD VM on another OS rather than the existing machine OS? The answer is
the SIGSEGV below, and having the question stated is what makes the answer
worth writing down.

⚠ **The operator's URL was `podman-container-tools/podman`, which resolves.**
The issue was read from `containers/podman`, the upstream. `references.md` trap
3 says resolve a cited reference before repeating it; both point at the same
issue number and the same content.

### R4, `cbsd_oci.md`: ⭐ adopt, as the conceptual correction

Its opening line is the one that reframes the whole task:

> OCI is an image standard, it does not regulate how exactly to work with the
> image.

It states plainly that FreeBSD OCI work assumes a FreeBSD host, that `buildah`
support is **experimental and not for production**, that "there is no known
vendor that makes NATIVE images for FreeBSD for their services", and that Linux
containers on FreeBSD need Linuxulator with "very limited capabilities".

⭐ It also names a FreeBSD-native design the OCI hierarchy does not support: a
read-only base mounted `nullfs` with a read-write overlay, which saves roughly
500 MB against a full base. Worth knowing before assuming layers are the only
model.

### R1, the FreeBSD handbook: **confirms**

Independent confirmation of the architecture, in the project's own words. The
runtime is Podman over jails; the install is
`pkg install -r FreeBSD -y podman-suite`; images load with `podman load -i`.
⭐ The line that matters: the container images **do not include the kernel**.

### R3, `download.freebsd.org/releases/OCI-IMAGES/`: **confirms**, and dates the claim

14.3, 14.4, 14.5 betas, 15.0 and 15.1. ⚠ The three-architecture claim was
checked against the four RELEASE directories, not against the betas. This is
what turns "FreeBSD is working on OCI" into a dated fact.

### R2, `ghcr.io/freebsd`: ⭐ adopt, and it removes a whole track of work

⛔ **The finding that most changes the plan.** `ghcr.io/freebsd/freebsd-runtime`
is published, multi-architecture, with `"os":"freebsd"` in the manifest. The
operator's plan had CI here building BSD images and pushing them to `ghcr.io`;
that work is **already done by the FreeBSD project**, at the registry the plan
was going to push to.

⚠ Recorded honestly: the *page* was not reachable with this token's scopes. The
registry was, anonymously, which answers the real question better than the page
would have.

### `runj` and `ocijail`: **filed elsewhere**

`samuelkarp/runj`, 674 stars, "experimental, proof-of-concept", pushed
2026-08-18. `dfr/ocijail`, 101 stars, pushed 2026-06-21, the one behind the
handbook's `podman-suite`. Both live, neither read. They matter only once a
FreeBSD host exists, and that is `BSD-01` Track A, not this sweep.

---

## The measurement that outranks every reference

⭐ **All five references together do not settle the question. One command
does**, and it is why `references.md` puts measurement above reading.

Running the official FreeBSD image on this Windows machine's Linux podman
machine exits **139**, which is 128 + 11, a SIGSEGV.

⛔ **That is a different failure from the one everybody expects.** Not
`Exec format error`, which is what a wrong architecture gives and what
`binfmt_misc` fixes. The Linux ELF loader **accepts** the FreeBSD binary and it
dies on its first syscall. So:

- no `binfmt_misc` change reaches it, and the whole of
  `Azathothas/TEMPLATE` issue 2 is irrelevant here despite looking adjacent;
- `qemu-user` does not help: it emulates a foreign **architecture** presenting
  **Linux** syscalls, and there is no counterpart presenting FreeBSD syscalls on
  a Linux kernel;
- a FreeBSD userland needs a FreeBSD kernel, full stop.

⚠ **A near-miss worth recording.** The adjacent failure in issue 2 was
`Exec format error` and was fixable. This one looks like the same family and is
not. Filing them together would have produced a plan built on the wrong
remedy.

---

## Second sweep, 2026-08-27: pkgforge-dev/docker-archlinux

Read as a **pattern reference only**, at the operator's instruction, for
`pkgforge-dev/docker-bsd`. Tree listed in full (107 blobs) and
`.github/workflows/build-deploy.yml` read (446 lines). ⛔ Not cloned, so nothing
below is cited at file and line beyond that workflow.

### ⭐ adopt: publish by digest, tag in a merge job

The build workflow runs one job per architecture that pushes **by digest and
creates no tag**, then a merge job with `needs:` over the whole matrix creates
every tag from those digests. Its own comment states the reason: a run that lost
one architecture publishes nothing at all.

⭐ **Adopted directly in `docker-bsd`**, with `needs: build` and no
`if: always()`, so a failure in any BSD skips the verify job and publishes a
partial set to nobody.

### ⭐ adopt: a dry run that targets a scratch repository

`dry_run` builds against a separate scratch image so a branch is exercised with
no consumer seeing anything. ⭐ Adopted, and **inverted**: `docker-bsd` defaults
`dry_run` to **true**, so publishing is the deliberate act rather than the
default. Its build script defaults the same way and needs `--push`.

### **confirms**: pinned actions, least privilege, `persist-credentials: false`

Every third-party action pinned to a commit with the tag in a trailing comment,
`permissions: contents: read` at the top with jobs asking for more individually,
and checkout with `persist-credentials: false`. All already required by the
template these repositories share; independent evidence rather than new work.

### ⭐ adopt: the `tests/static` and `tests/image` split

Static tests check the repository and workflow shape with no build; image tests
check a built artefact. ⚠ **Only the static half transfers to `docker-bsd`**,
and the reason is the SIGSEGV: there is no host that can run a BSD image, so an
image-test directory there would either be empty or be theatre. `docker-bsd`
has `tests/run.sh` with the static half and says in its header why the other
half does not exist.

### ⭐ adopt, as a shape: `HISTORY/`

`HISTORY/` holds `references/`, `reviews/` numbered by the reader they imagine,
and named incident files such as `arm-rollback.md` and `tests-seen-to-fail.md`.
⭐ **`tests-seen-to-fail.md` is the strongest single idea in the tree**: a
record of guards actually observed failing, which is the difference between a
test suite and theatre. `docker-bsd` starts with `HISTORY/poc.md` carrying the
measurements; the numbered-review shape is worth adopting when it has enough
history to fill it.

### ⚠ anti-pattern exhibit: the emoji in the workflow name

The build workflow's `name:` is wrapped in a pair of dolphin emoji, and an
older workflow put a check-mark glyph in every commit message. ⚠ **The operator's own issue 3 cited this repository as the evidence that the
template's marker rule was too narrow.** It is kept here
as an exhibit because it is the case that produced the two-tier rule: the fix
was to allow status glyphs in machine output, not to allow decoration in a
workflow name. `docker-bsd` uses plain names.

### ⚠ what did not transfer, and why

- **`Dockerfile`.** `docker-archlinux` builds an image from a base plus
  packages. `docker-bsd` cannot: running `pkg` or `pkg_add` needs a BSD kernel.
  It assembles filesystems instead, which is why it has scripts and no
  Dockerfile.
- **`freshness-*.yml`.** Mirror and keyring freshness are pacman concerns. The
  BSD equivalent is "does the upstream URL still resolve", which `docker-bsd`
  runs as a `continue-on-error` job so an upstream outage reports without
  failing a merge.

⚠ This paragraph describes those glyphs rather than reproducing them, on
purpose. A document that demonstrates the thing a checker looks for makes the
checker fire on correct writing, which is the same rule `docs/README.md` states
about placeholder markers. The check caught the first draft of this very
section.

---

## Third sweep, 2026-08-27: the BSD reference batch, `R6` to `R28`

23 references supplied by the operator, expanded to **28 repositories** because
one row was an organisation query. ⭐ **Every one was reached.** No row is
recorded as gone.

⛔ **Read [`../methodology/references.md`](../methodology/references.md) first.**
This sweep took its tracker step, and the tracker is where most of what follows
came from.

### How it was fetched

Metadata, README, full blob tree, releases, every issue and pull request in
**both states** with their comments, and discussions where the repository has
them. Nothing was cloned, so no line citation below is against a working copy;
the four repositories whose source was read are cited against the blob at the
commit in the table.

⚠ **`gh api` was the only remote verb used, and only for reads.** No issue,
comment, star or fork was created anywhere.
[`../security/remote-ops.md`](../security/remote-ops.md).

### Provenance

| # | reference | HEAD read | tracker pulled | depth |
| --- | --- | --- | --- | --- |
| R6 | `BalajeS/WSL-For-FreeBSD` | `5af157a9180f` | 3 items, 1 thread | ⭐ source read: both guest files in full, and the four host commits' patches |
| R7 | `NetBSDfr/smolBSD` | `0824f4ee04fc` | 83 items, 51 threads | README in full, tracker, `#81` and `#66` in full |
| R8 | `vmactions/*` | see below | 250 items, 151 threads | 17 BSD repositories listed, 6 harvested |
| R9 | `cbsd/cbsd` | `88691c73d9b9` | 200 items, 90 threads, 10 discussions | README, tracker, `D#858` and `D#861` |
| R10 | `samuelkarp/runj` | `cb4e1b9919e7` | 70 items, 45 threads | README, tracker titles |
| R11 | `acj/freebsd-firecracker` | `bbec07a288d9` | 2 items | README, releases, limits |
| R12 | `acj/freebsd-firecracker-action` | `e04896d2dd91` | 9 items, 9 threads | ⭐ README and every thread |
| R13 | `acj/netbsd-firecracker` | `d68cf0da7a35` | 0 items | README, releases |
| R14 | `matias-pizarro/freebsd-oci-containers` | `6bb12c20fc52` | 0 items | README |
| R15 | `step-security/freebsd-vm` | `f49184475ccc` | 13 items, 3 threads | README, provenance |
| R16 | `gronke/freebsd-ci` | `ee9770828ffa` | 1 item | README, tree |
| R17 | `no-pictures/freebsd-ci` | `16b835a34634` | 3 items, 3 threads | README, tracker |
| R18 | `anyvm-org/anyvm` | `3924fc60df69` | 70 items, 4 threads | ⭐ source read: `anyvm.py`, 11 821 lines, the accelerator paths |
| R19 | `anyvm-org/freebsd-builder` | `da84b6c9ac9a` | 2 items | README, release assets |
| R20 | `daemonless/daemonless` | `8b548292950c` | 3 items, 8 discussions | README, all discussions |
| R21 | `daemonless/dbuild` | `b1d31d5ed0d9` | 19 items, 6 threads | README |
| R22 | `pavetheway91/tarbsd` | `56ebef933ece` | 22 items | README, tracker |
| R23 | `bitmand/freebsd-info` | `aed35d24313d` | 0 items | README, tree |
| R24 | `gabrielbelli/oci-jails` | `edd1e294940e` | 4 items | README and `CATALOG.md` |
| R25 | `dfr/ocijail` | `84e078d25c28` | 24 items, 22 threads | README, releases |
| R26 | `tsirysndr/bsdkrun` | `b31ff8ebed68` | 43 items | README, tracker |
| R27 | `XaeroVincent/FreeBSD-Gaming-Kernel` | `343c81fccd2e` | 0 items | README in full, 690 bytes |
| R28 | `AkihiroSuda/lsf` | `ff4e43f59c5d` | 2 items | ⭐ README in full |

⚠ **`R8` is 17 repositories, not one.** `vmactions` publishes a `*-vm` action
and a `*-builder` per guest, and the BSD set covers FreeBSD, OpenBSD, NetBSD,
DragonFly, MidnightBSD, HardenedBSD, GhostBSD and NextBSD. Six were harvested in
full: `freebsd-vm`, `openbsd-vm`, `netbsd-vm`, `dragonflybsd-vm`,
`freebsd-builder` and `dragonflybsd-builder`.

### ⚠ Two of these are the same tree

`gronke/freebsd-ci` and `no-pictures/freebsd-ci` share commit `16b835a34634`,
byte for byte. `gronke` carries one commit more. ⛔ **They are counted once
below**, and this line exists so a later session does not mine the second one
believing it is new.

---

## Classification, reviewed twice

Every reference carries exactly one class:

| class | meaning |
| --- | --- |
| **Pre** | useful **before** a BSD userland runs. It helps get one running. |
| **Post** | useful **after**. CI, packaging, images, refinement, hardening. |
| **Misc** | mined, and it offered nothing for this work. ⛔ Recorded anyway, so the hour is not spent again. |

⭐ **The classification was taken twice, as two separate passes, and the second
pass moved four rows.** Stated here because a `Pre` filed as `Post` is an hour
the next session does not spend on the thing that would have unblocked it.

| reference | first pass | second pass | why it moved |
| --- | --- | --- | --- |
| `cbsd/cbsd` | Post | ⭐ **Pre** | The first pass read it as a FreeBSD jail manager, which is a Post concern. `D#858` says CBSD 15.0.6 added **NetBSD** as a fifth platform, which is a direct answer to `BSD-02`'s open question and therefore Pre. |
| `AkihiroSuda/lsf` | Misc | ⭐ **Pre** | The first pass saw a dead 2022 proof of concept and stopped. It is Pre because it **falsifies a claim already written in [`usable.md`](usable.md)**, and a correction to a live document outranks liveness. |
| `acj/freebsd-firecracker-action` | Post | ⚠ **Post, kept** | Considered for Pre because its tracker carries the boot failures. The **artefacts** are `R11`'s; this repository is the CI wrapper around them, so the class stays and the tracker evidence is cited under `R11`. |
| `pavetheway91/tarbsd` | Pre | ⭐ **Post** | The first pass saw "builds a bootable FreeBSD image, converts to `vhdx`" and read it as a route in. ⛔ It needs an **existing FreeBSD host with PHP** to build, so it cannot be the thing that gets you your first BSD. It is excellent once you have one. |

The full classification:

| class | references |
| --- | --- |
| **Pre**, 11 | `R6` `R7` `R8` `R9` `R11` `R13` `R17` `R18` `R19` `R26` `R28` |
| **Post**, 9 | `R10` `R12` `R14` `R16` `R20` `R21` `R22` `R23` `R24` `R25` |
| **Misc**, 2 | `R15` `R27` |

⚠ `R16` and `R17` are one tree, so the Post column lists ten identifiers for
nine distinct references.

---

## ⭐ The ranking

Ranked on what was learnt, the impact on `BSD-01`, and the ideas produced.
⛔ **This ordering is the deliverable, not the list.** A session reading top
down reaches the useful thing first.

| rank | reference | class | what it is worth |
| --- | --- | --- | --- |
| **1** | `R18` `anyvm-org/anyvm` | Pre | ⭐ The only source here that has **measured the Windows hypervisor** and written down what breaks. It is also the engine under `R8`. |
| **2** | `R7` `NetBSDfr/smolBSD` | Pre | ⭐ A NetBSD microvm that boots in about 10 ms, and an **open issue reporting it running on Windows under WHPX**. Distributes images through an OCI registry. |
| **3** | `R6` `BalajeS/WSL-For-FreeBSD` | Pre | ⭐ Proof that a **non-Linux guest can drive WSL's own host protocol**, with the wire format readable in 819 lines. Its cost is a rebuilt Windows service. |
| **4** | `R11` `acj/freebsd-firecracker` | Pre | A **published FreeBSD kernel and rootfs pair** that boots under Firecracker in about 12 seconds, plus the CPU faults that took a year to find. |
| **5** | `R26` `tsirysndr/bsdkrun` | Pre | ⭐ Boots an **OCI image as a microVM**. The closest thing to the shape `BSD-01` asks for, on Linux and macOS only. |
| **6** | `R28` `AkihiroSuda/lsf` | Pre | ⛔ It **falsifies a claim this repository has published twice.** Dead since 2022 and worth more than most live things here. |
| **7** | `R8` `vmactions/*` | Pre | The most used BSD CI action there is, and a **library of `qcow2` guests** for four BSDs across four architectures. |
| **8** | `R9` `cbsd/cbsd` | Pre | Runs on five platforms including NetBSD, and holds cloud images for all the BSDs. |
| **9** | `R17` `no-pictures/freebsd-ci` | Pre | The **official BASIC-CI images**, and the `udev` rule that makes a runner's `/dev/kvm` writable. |
| **10** | `R19` `anyvm-org/freebsd-builder` | Pre | Twelve FreeBSD releases as `qcow2.zst`, four architectures, with the failures footnoted. |
| **11** | `R13` `acj/netbsd-firecracker` | Pre | `R11` for NetBSD 11. Same shape, no tracker. |
| **12** | `R24` `gabrielbelli/oci-jails` | Post | ⭐ `CATALOG.md` is a surveyed map of **every** FreeBSD OCI image source. |
| **13** | `R20` `daemonless/daemonless` | Post | The largest FreeBSD image collection in existence, and the sentence that explains why `docker-bsd` should exist. |
| **14** | `R25` `dfr/ocijail` | Post | The runtime under the handbook's `podman-suite`. Live, releasing. |
| **15** | `R10` `samuelkarp/runj` | Post | The other runtime. Its own author calls it a proof of concept. |
| **16** | `R21` `daemonless/dbuild` | Post | A working model for the build tool `docker-bsd` will grow. |
| **17** | `R22` `pavetheway91/tarbsd` | Post | A 40 MB FreeBSD that boots to memory, and `qcow`, `vhdx` and `vmdk` output. |
| **18** | `R14` `matias-pizarro/freebsd-oci-containers` | Post | ⚠ The operator's note said it may offer nothing. It offers a **stated upstream strategy**, which is worth more than its code. |
| **19** | `R12` `acj/freebsd-firecracker-action` | Post | The CI wrapper. Its tracker is cited under `R11`. |
| **20** | `R16` `gronke/freebsd-ci` | Post | `R17` plus one commit. |
| **21** | `R23` `bitmand/freebsd-info` | Post | Machine-readable FreeBSD release data, updated daily. Small and directly usable. |
| **22** | `R15` `step-security/freebsd-vm` | Misc | A hardened repackage of `R8`. Nothing new about BSD. |
| **23** | `R27` `XaeroVincent/FreeBSD-Gaming-Kernel` | Misc | ⚠ The operator asked whether any performance fix applies. **None does.** |

---

## The verdicts

### `R18`, `anyvm-org/anyvm`: ⭐ adopt, and it is the top of the ranking

A single Python file, 11 821 lines, that boots BSD, Illumos, Linux and more
under QEMU on **Linux, macOS and Windows**. MIT, 65 stars, pushed the day this
was read. `winget install anyvm-org.anyvm` then `anyvm --os freebsd`.

⭐ **It is the engine under `vmactions`.** `vmactions/freebsd-vm`, 351 stars,
says "Powered by AnyVM.org" in its README. So the ranking's top entry is not a
hobby project; it is what the most used BSD CI action runs on.

⭐ **What no other reference here has: measured Windows hypervisor behaviour.**

- `-cpu host` and `-cpu max` under WHPX can **wedge the whole QEMU process**.
  Measured on a Zen 5 Ryzen AI MAX+ 395: zero-byte serial log, unresponsive
  monitor, about 2 seconds of CPU time after 12 minutes. Named CPU models skip
  the host-CPUID enumeration path that does it.
- ⛔ **A named model from a generation NEWER than the host wedges QEMU too**,
  before the guest runs an instruction. Measured on **both GitHub Windows runner
  fleets**, 2026-07-29, with the host identities recorded: family 25 model 1
  given `EPYC-Turin-v1`, and family 6 model 106 given `GraniteRapids-v2`.
- ⚠ **Windows QEMU ships WHPX in `qemu-system-x86_64.exe` only.**
  `qemu-system-i386.exe` lists `tcg` alone, and asking a binary for an
  accelerator it lacks is a hard startup error.
- ⭐ `whpx_available()` calls `WHvGetCapability` from `WinHvPlatform.dll` and
  reads the `HypervisorPresent` capability. **That is an unelevated runtime
  check**, and it answers the question `bsd.md` left open when
  `Get-WindowsOptionalFeature` refused without elevation.

⛔ **And one measurement that speaks directly to the nesting question.**
`host_nested_amd_with_avx512()` documents that **nested AMD-V, naming KVM inside
WSL2 or Hyper-V, mishandles the L2 guest's AVX512 XSAVE state**. A guest whose
libc selects the AVX512 string routines then takes random SIGSEGVs across nearly
every dynamically linked binary while the kernel stays up. The workaround is to
drop AVX512 from `-cpu host` in that exact case.

⚠ **It does not bite on this machine**, whose 40 threads report `vmx` rather
than `AuthenticAMD`. It is recorded because it is a measured **correctness**
fault of the nested configuration, not a performance one, and
[`../../TODO/bsd.md`](../../TODO/bsd.md) ranked nesting on performance alone.

### `R7`, `NetBSDfr/smolBSD`: ⭐ adopt

A NetBSD microvm builder. 710 stars, BSD-2-Clause, `smolbsd.org`. Boots
NetBSD/amd64 through the **PVH entry point** into QEMU's `microvm` machine or
into Firecracker in about **10 milliseconds**. Builds from any Linux, NetBSD or
macOS host with no NetBSD installation.

Three mechanisms worth taking:

1. ⭐ **A `Dockerfile`-compatible build file.** `FROM base,etc` names NetBSD
   sets rather than an image; `RUN`, `CMD`, `EXPOSE` and `LABEL` work.
   `smoler.sh build`, `run`, `images`, `push`, `pull`.
2. ⭐ **Images distributed through an OCI registry with `oras`**, default
   `ghcr.io/netbsdfr/smolbsd`. A raw bootable disk image, carried as a registry
   artefact. ⚠ This is the model `docker-bsd` does not have: it publishes a
   rootfs nothing can run, where this publishes a thing that boots.
3. **A bare-metal and non-PVH path** through `BIOSBOOT=y`, which trades the
   sub-second boot for `bhyve` and real hardware.

⭐ **`#81`, still open, is the single most valuable tracker item in this sweep.**
`@donno` reports getting smolBSD to run **on a Windows host**, far enough to
serve HTTP from Caddy inside the guest, and records exactly where it stops:

- `-accel whpx` with `cputype="kvm64-v1"`, because `-cpu host` needs KVM or HVF
  and `max` gives a fatal privileged instruction. ⭐ **Independently the same
  finding as `R18`'s**, reached by a different person on different hardware.
- ⚠ `oras` pulling on Windows writes a **zero-byte file**, because the `:` in
  `name:latest` is not a legal NTFS filename and `oras` neither errors nor
  sanitises. Worked around by pulling in a container and renaming.
- Git Bash reports `uname -s` as `mingw64_nt.<version>`; Busybox for Windows
  reports `windows_nt`.
- Building needs `bmake` and disk tooling, so the reporter built in WSL2, and
  `startnb.sh` has **no flag to disable acceleration**, which forces nested
  virtualisation for a build that does not need speed.
- The WSL2-built image then panicked on Windows with
  `ffs_newvnode: dup alloc`.

⭐ **`#66` carries the maintainer's own costing**, which is the kind of line a
README never has. `@iMilnb`: FreeBSD and OpenBSD are unsupported because their
hypervisors, `bhyve` and `vmm`, have **no QEMU bindings**, and QEMU is the base
for the `microvm` machine type and VirtIO-MMIO. Support is planned, not
refused. A commenter names an in-progress FreeBSD GSoC branch adding bhyve QEMU
bindings.

### `R6`, `BalajeS/WSL-For-FreeBSD`: ⭐ adopt the protocol, ⛔ refuse the patch

420 stars, MIT. ⚠ **Not a GitHub fork**: a copy of `microsoft/WSL` whose history
ends at upstream `39b4cd887`, with FreeBSD work committed on top.

⭐ **The finding: WSL's host side talks to its guest over Hyper-V sockets, and
the guest end is 819 lines of ordinary C.** `FreeBSD/hvinit.c` and
`FreeBSD/hvbridge.c` are the whole guest implementation, and between them they
document the wire format:

- the guest **connects out** three times to `AF_HYPERV` port **50000** and
  sends `LX_INIT_GUEST_CAPABILITIES` with a kernel version string;
- it answers with `LX_MINI_INIT_CREATE_INSTANCE_RESULT`, naming a port to be
  called back on, then a configuration response and a session response;
- it **listens** on `AF_HYPERV` port **60000**, accepts an init socket, an
  initial socket and five more, and reads
  `LX_INIT_CREATE_PROCESS_UTILITY_VM`, which carries rows and columns followed
  by byte offsets for filename, working directory, command line and
  environment. ⭐ **That structure is `wsl.exe`'s argument list on the wire.**
- sockets 0 to 2 are standard input, output and error; the guest calls
  `forkpty` and runs `/bin/sh` against them.

⛔ **The host side is patched, and that is the cost the README does not
state.** Four files, about +248/-13, every one under
`src/windows/service/exe/`, which is `wslservice.exe`. The patch:

- replaces `GenerateConfigJson()` with a **string literal** HCS document
  carrying a hard-coded `vhdx` path and a hard-coded user SID in the hvsocket
  security descriptor;
- boots UEFI from a SCSI attachment rather than the WSL kernel path;
- assigns over the loaded configuration to disable GPU support, host filesystem
  access, kernel modules, swap and the system distro, and comments out
  `ReadGuestCapabilities()`, `ValidateNetworkingMode()`, the early
  configuration message, `InitializeGuest()`, disk mounts and the telemetry
  agent.

⛔ **Adopting it means replacing the Windows service that also runs this
machine's podman machine.** That is refused: `BSD-01`'s recommendation rests on
Hyper-V and WSL2 coexisting, and this would put the thing they coexist through
into an experimental build.

⭐ **What survives the refusal is better than the patch.** The literal HCS
document shows that WSL's own VM is created by `CreateComputeSystem()` over a
JSON object naming a UEFI boot, a SCSI disk and an hvsocket configuration.
⭐ **Reaching the Host Compute System directly, or Hyper-V's own module, needs
no WSL patch at all**, and that is the "host's own hypervisor, addressed
directly" avenue `bsd.md` lists and nobody had costed.

### ⚠ `R6` anti-pattern exhibit: three defects readable in the source

Kept on purpose, per [`../methodology/references.md`](../methodology/references.md).
Each is a row this repository's own table already has:

- `WslCoreInstance.cpp` declares `ULONG port = 600000;`. ⛔ **Six digits.** The
  guest binds **60000**.
- The line under it reassigns the **local pointer parameter**,
  `ConnectPort = &port;`, after the value has already been written through it.
  It cannot reach the caller. A dead write, in the commit titled "Fixed interop
  port". That is *a setting or flag that no code reads*.
- `hvinit.c` terminates a receive buffer using the length from an **earlier**
  receive on a different socket rather than the one read.

⚠ And the tracker's own shape is the finding: three items, all open, two of them
asking **how to build it** and **how to test it**, unanswered since 2025-10-19.
⛔ There are no build instructions. A reader who takes the README's roadmap
ticks at face value would not learn that.

### `R11` and `R13`, `acj/freebsd-firecracker` and `netbsd-firecracker`: ⭐ adopt

⭐ **These publish what `docker-bsd` does not: a kernel and a root filesystem
that boot.** Each release carries `freebsd-kern.bin`, `freebsd-rootfs.bin.xz`,
a `firecracker` binary and an SSH key pair. FreeBSD 15.1 and NetBSD 11,
Firecracker 1.16.1, Intel and AMD, **about 12 seconds** to a running VM in
GitHub Actions.

⚠ **Small kernel patches are needed** to boot FreeBSD in Firecracker on the
runners' CPUs. The repository exists to hold them.

⛔ **Its stated limit answers a question `bsd.md` calls the highest-value
unknown**: `x86_64` only, "due to the need for PVH direct boot and lack of
nested virtualization support in the GitHub Actions runners (an Azure
limitation)".

⭐ **The tracker is the cost picture**, and its heavy consumer is `astral-sh/uv`:

- `TSC not initialized` panic during LAPIC init **on every VM start**, on Intel
  runners whose CPU brand string omits a frequency. Firecracker cannot derive
  an early guest TSC frequency from those hosts and the FreeBSD Firecracker
  kernel skips early calibration. Fixed by supplying a base frequency through
  CPUID leaf 0x16.
- `Open tap device failed: Resource busy` on retry, because the next process
  started before the previous one finished releasing `tap0`.
- Spurious `iptables` and `No route to host` failures that the maintainer could
  not reproduce with logging attached, worked around with a fast retry.

⚠ **Read together, those are the friction column for the Firecracker row**, and
they are all in the host CPU and the network teardown rather than in FreeBSD.

### `R26`, `tsirysndr/bsdkrun`: ⭐ adopt as the shape

A microVM launcher built on **libkrun**, so a VM is a process with no daemon.
Boots FreeBSD and NetBSD three ways: UEFI firmware, direct kernel, and
⭐ **straight from an OCI image**, which it pulls from any registry, extracts,
and boots the way `docker run` would.

⭐ **That last one is the shape `BSD-01` describes.** `podman run` against a BSD
is refused by the kernel; `bsdkrun linux alpine` shows the same gesture working
when the thing underneath is a microVM rather than a namespace.

⛔ **macOS on Apple Silicon and Linux on amd64 or arm64. No Windows.** On this
host it would have to live inside the WSL2 machine, which is nesting, so it is
the floor rather than the target. ⚠ It is a very good floor: under WSL2's
measured nested KVM it would behave identically to a native Linux runner, which
is what the operator's second ask wanted.

⚠ FreeBSD on Linux/amd64 needs their **PVH-enabled libkrun fork**. Same PVH
dependency as `R7`, reached independently.

`#24` adds a `bsdkrun kvm` subcommand that checks `/dev/kvm` before booting
rather than failing inside `krun_create_ctx` with a bare errno, and states that
**GitHub's arm64 runners have no `/dev/kvm`**, so the success path is asserted
in an x86_64 job instead.

### `R28`, `AkihiroSuda/lsf`: ⭐ adopt, as a correction to this repository

"Linux Subsystem for FreeBSD". 180 stars, Apache-2.0. ⛔ **One commit, dated
2022-08-29, never touched since.** Its own status section: proof of concept,
crashes very frequently, many syscalls unimplemented, amd64 only.

⛔ **It falsifies a sentence [`usable.md`](usable.md) and
`pkgforge-dev/docker-bsd` both publish.** Both say `qemu-user` does not help
because it emulates a foreign architecture presenting **Linux** syscalls and
"there is no counterpart presenting FreeBSD syscalls on a Linux kernel".
⭐ **A counterpart was built.** `lsf` traps syscalls with `PTRACE_SYSCALL`,
rewrites the syscall number in `RAX`, translates the structures that differ, and
sets the carry flag on error because FreeBSD processes expect it.

⭐ **It also explains the 139.** Its own words: the Linux kernel does not
validate the OSABI of an ELF binary on `execve`. That is precisely why a FreeBSD
binary is **accepted** by the loader and dies at its first syscall, instead of
being refused with `Exec format error`.

⚠ **The conclusion does not change and the reasoning does.** A reverse
Linuxulator exists, it was abandoned four years ago, and it crashes. The honest
statement is that one was attempted rather than that none can exist, and the
correction is written under the claim rather than over it.

### `R8`, `vmactions/*`: ⭐ adopt

17 BSD repositories. `freebsd-vm` at 351 stars is the most used way to run a BSD
in GitHub Actions; `openbsd-vm`, `netbsd-vm` and `dragonflybsd-vm` cover the
other three, and `nextbsd`, `midnightbsd`, `hardenedbsd` and `ghostbsd` exist
too. Every one is QEMU with SSH and two-way folder sync, and every one is
⭐ **powered by `R18`**.

⭐ **The `*-builder` repositories publish the guests as release assets**:
`freebsd-13.4.qcow2.zst`, a matching libvirt XML, and a generated SSH key pair,
per release and per architecture. ⚠ A ready-made bootable BSD with no installer,
which is the same friction argument `bsd.md` makes for FreeBSD's `.vhd` and
reaches further, because it covers four BSDs rather than one.

Two tracker findings:

- `freebsd-vm#138`, closed. `@neilpang`: nested virtualisation needs the host
  and guest architectures to match, so an arm64 guest needs an
  `ubuntu-24.04-arm` runner, and ⛔ **that fleet has "low performance, and kvm
  disabled"**.
- `netbsd-vm#21`, closed. A 20 percent CI failure rate traced not to QEMU but to
  **NetBSD's `mount_psshfs`**, a PUFFS filesystem over SFTP that caches
  directory and node attributes for 30 seconds over the page cache. Under a
  parallel build it served short reads: object files linked ten seconds after
  compiling came back `file too short`, and a `build.ninja` read back nine
  seconds after being written came back corrupted. ⭐ The maintainer's answer
  names the fix and the reason the other BSD legs stayed clean: they use FUSE
  sshfs, and only NetBSD uses psshfs.

⚠ **That last one is a trap for anything that shares a workspace into a BSD
guest**, which is what every option in `bsd.md` will end up doing.

### `R9`, `cbsd/cbsd`: ⭐ adopt, and it changes `BSD-02`

774 stars, BSD-2-Clause, pushed the day it was read. 200 tracker items and 10
discussions.

⭐ **`D#858`, 2026-03-29: CBSD added NetBSD as a fifth platform**, alongside
FreeBSD, DragonFly, Linux and XigmaNAS, from CBSD 15.0.6 with NetBSD 11.
⛔ `BSD-02`'s premise says the other three BSDs have no jail-equivalent OCI
runtime "that this sweep could find". One of the three now has a management
layer that is OCI-aware and runs on it.

`D#861`, "FreeBSD containers in GitHub Actions", carries the maintainer's
position on CI: VMs fit small cases, self-hosted runners with Garm are what
`olevole` uses, and ⚠ native FreeBSD support in GitHub's own runner has been
open for over seven years. ⭐ And the line worth keeping: **CBSD already
provides cloud images for all the BSDs.**

### `R17` and `R16`, the `freebsd-ci` pair: **adopt one mechanism**

⭐ **FreeBSD release engineering publishes official BASIC-CI images.** They boot
with a serial console, DHCP and `growfs`, and their `sshd` accepts root with an
empty password on first boot, so a plain SSH provisioning step works with no
custom image to maintain. Provisioning installs a per-run key and closes the
empty-password door in the same step, and the forwarded port binds to
`127.0.0.1` only.

⭐ **`#2` answers the runner question from the other side**: QEMU autodetects KVM
by looking for a **writable** `/dev/kvm`, falls back to TCG, and `setup-vm`
applies a **`udev` rule on runners** to make it writable. ⚠ And the honest half:
the same pull request records that a full first boot **under TCG** overran a
ten-minute budget, which is what an unaccelerated fallback costs.

⚠ **One detail transfers to any BSD work**: scripts are piped into `sh -s` on
the guest because **root's login shell on FreeBSD is `csh`**, so a command
string passed as an SSH argument is parsed by `csh`. That is the same class as
[`../conventions/shell.md`](../conventions/shell.md) section 7.

### `R24`, `gabrielbelli/oci-jails`: ⭐ adopt `CATALOG.md`

A catalogue of every source of FreeBSD-platform OCI images its author could find
and check, with the `skopeo` and `curl` commands that checked them, dated
2026-08-21 against a FreeBSD 15.1 host. ⭐ **Three tiers, and the third is
empty**:

| tier | who | how many |
| --- | --- | --- |
| 1 | the FreeBSD Project itself | 5 base images, no applications |
| 2 | community collections repackaging `pkg` software over tier 1 | 8 sources, one of which is larger than the rest combined |
| 3 | upstream projects publishing a `freebsd` platform in their own manifest | ⛔ **zero**, verified by survey |

⚠ **Its warning is the one `docker-bsd` should carry too**: an OCI container on
FreeBSD is a **jail on your kernel** with no user-namespace remapping, so uid 80
inside is uid 80 outside, and a careless image is a host problem rather than a
sandbox problem.

⚠ **`pkgforge-dev/docker-bsd` is not in the catalogue**, which is correct: the
catalogue was surveyed on 2026-08-21 and `docker-bsd` was first pushed on
2026-08-27.

### `R20` and `R21`, `daemonless`: **confirms**, with one sentence worth quoting

`daemonless` publishes native FreeBSD OCI images, **92 image repositories
listed in its README**, with s6 supervision and PUID/PGID mapping, and `dbuild` is the
Python tool that builds, tests, generates an SBOM and pushes them. Both were
pushed within two days of this read. `R24` calls it an order of magnitude larger
than every other community source combined.

⭐ **`D#5` is the reason `docker-bsd` exists, written by somebody in the FreeBSD
container ecosystem rather than by us**: FreeBSD has OCI, and what it does not
have is images. The same comment names the jail managers that implement images,
AppJail, CBSD, `pot` and `vessel`, and notes that only CBSD and AppJail are
OCI-compatible, so an OCI image is the one format all of them can consume.

⭐ **`dbuild`'s command split is a shape worth copying**: `detect` produces the
build matrix, then `build`, `test`, `sbom`, `push` and `manifest`. The CI
workflow stays generic and the platform knowledge lives in the tool.

### `R25` and `R10`, `ocijail` and `runj`: **filed elsewhere**, now with detail

Both were liveness-only in the first sweep. Read this time:

- `dfr/ocijail`, C++, 101 stars, `v0.6.0` on 2026-06-21, allows runtime-spec
  1.3.x. This is the one behind the handbook's `podman-suite`.
- `samuelkarp/runj`, Go, 674 stars, last release `v0.2.0` in 2025-11.
  ⛔ **Its own README refuses production use**, has not been evaluated for
  security, and is stated to be a personal project. Its implemented surface is
  create, delete, start, state and kill, plus root path, args, environment,
  terminal, hostname, mounts and two hooks. `docs/spec-coverage.md` is the
  checklist.

⭐ Together they answer `BSD-02` for FreeBSD precisely: one production-intent
runtime and one proof of concept, with the coverage written down.

### `R22`, `pavetheway91/tarbsd`: **adopt later**

Builds a FreeBSD image that boots to memory with most of the system in a
zstd-19 tar mounted at `/usr` rather than extracted, so it needs far less RAM
than a traditional memory-root image and can come in under 40 MB. In the ports
tree as `sysutils/tarbsd-builder`.

⭐ **It emits `qcow`, `qcow2`, `vdi`, `vmdk`, `vhdx`, `vpc` and `parallels`**
from one build when QEMU is present. `vhdx` is Hyper-V's native format.

⛔ **It needs an existing FreeBSD system with PHP 8.2 or newer to run**, which
is why it moved from Pre to Post in the second classification pass. It is what
you use once you have a BSD, to make a smaller one.

### `R14`, `matias-pizarro/freebsd-oci-containers`: **adopt the strategy**

⚠ **The operator's note said it may offer nothing and asked for it to be mined
anyway. It offers a strategy rather than a mechanism**, and the strategy is the
part worth having: track the generation logic of the official docker-library
projects, patch them to emit FreeBSD variants while staying merge-compatible
with upstream, and aim at `FROM postgres:17` working on FreeBSD, with
`FROM freebsd/postgres:17` as the intermediate step.

⚠ Empty tracker, no discussions, last pushed 2026-03-24, and `R24` reaches
further on the same ground.

### `R23`, `bitmand/freebsd-info`: **adopt, small**

Machine-readable FreeBSD release information, updated the day before this read.
Directly usable by `docker-bsd`'s `scripts/sources`, which pins one release per
BSD by hand.

### `R15`, `step-security/freebsd-vm`: **Misc**

A `step-security` repackage of `R8` with pinned actions and Harden-Runner. The
BSD half is `R8`'s and the release matrix is identical. ⚠ Its supply-chain
practice is already what
[`findings.md`](findings.md)'s second sweep recorded as **confirms**, so there
is nothing new in either half.

### `R27`, `XaeroVincent/FreeBSD-Gaming-Kernel`: **Misc**, and the question is answered

⚠ **The operator asked whether any performance fix here applies. None does.**
The whole repository is 690 bytes of README and 14 blobs against
`freebsd-src` 15.1.0-p2, and every patch is a **desktop and gaming
compatibility** change rather than a performance one: a hidraw ioctl handler
for game controllers, ignoring `EPOLLEXCLUSIVE`, linsysfs nodes for Mesa, an
Android compatibility runtime, and Syscall User Dispatch for anti-tamper DRM.

⚠ **One of them is interesting for an unrelated reason.** Syscall User Dispatch
is the mechanism `R28` explicitly says it does **not** use, having chosen
`PTRACE_SYSCALL` instead. Seeing it turn up in a FreeBSD kernel for DRM is a
reminder that the primitive exists on both sides.

---

## What this sweep corrects

⛔ **Written here rather than edited into the claims**, per
[`../methodology/work-todo.md`](../methodology/work-todo.md).

| the claim | where | the correction |
| --- | --- | --- |
| "there is no counterpart presenting FreeBSD syscalls on a Linux kernel" | [`usable.md`](usable.md), and `docker-bsd`'s README | ⛔ One was built: `R28`. It is a dead 2022 proof of concept that crashes, so the conclusion holds and the reason must change. |
| "NetBSD, OpenBSD and DragonFly have no jail-equivalent OCI runtime that this sweep could find" | `BSD-02` | ⚠ Still true for a **jail-equivalent** runtime. ⛔ Not true that they cannot be run: `R7` and `R13` boot NetBSD as a microvm, and `R9` manages NetBSD as of CBSD 15.0.6. |
| "their images are publishable and, as far as is known, not yet runnable anywhere" | `BSD-02` | ⛔ Overtaken. `R26` boots an OCI image as a microVM, and `R7` distributes bootable images through an OCI registry with `oras`. |
| nested QEMU is "⛔ worst. Emulation inside a VM." | `BSD-01`'s table | ⚠ Already corrected once by the measured nested KVM. `R18` adds a second and different reason to keep it as the floor: a measured **correctness** fault under nested AMD-V, not a performance one. |
| "Does a CI Linux runner have `/dev/kvm`? ⛔ The single highest-value unknown" | `BSD-01` | ⭐ Answered by three independent references. See the row below. |

### ⭐ The highest-value unknown, answered three times

⚠ **Not measured here.** This session has no GitHub runner to probe. What it
has is three references that run on them and say so, which is the difference
between an assumption and a sourced claim.

| source | what it says |
| --- | --- |
| `R11` | `x86_64` only, because arm64 needs PVH direct boot and the runners lack nested virtualisation, named as an Azure limitation |
| `R26` | tests avoid a real `/dev/kvm` so they pass on **GitHub's arm64 runners, which have none**; the success path is asserted in an `x86_64` job |
| `R8` | the arm64 fleet has "low performance, and kvm disabled" |
| `R17` | on `x86_64`, QEMU finds KVM when `/dev/kvm` is **writable**, and a `udev` rule is applied on runners to make it so |

⭐ **So: `x86_64` GitHub-hosted runners expose `/dev/kvm` and need a `udev` rule
to make it writable; arm64 runners do not have it at all.** ⛔ That is the
answer `BSD-01` said would decide whether the universal option can exist, and it
says the universal option is possible on `x86_64` and impossible on arm64.

---

## ⛔ What this sweep did not do

- **Nothing was cloned.** Four repositories had source read through the contents
  API at the commit in the provenance table; the rest were read as README,
  tree, releases and tracker.
- **Nothing was run.** No image was booted, no VM started, no `qemu` invoked.
  Every number above is somebody else's measurement, attributed to them.
- ⚠ **The four-pass reading was taken over two references only**, `R6` and
  `R18`, which are the two whose source was read in depth. For the others a
  second pass over a README and a tracker would be the same pass written twice,
  which [`../methodology/references.md`](../methodology/references.md) says is
  worse than admitting three.
- ⚠ **`cbsd`'s tracker was capped at 200 items** and its 90 comment threads are
  the most recent. A repository with 774 stars and eight years of history has
  more in it than one pass finds.
- ⚠ **The 11 remaining `vmactions` BSD repositories were listed, not
  harvested.** They are the same generated shape as the six that were.
