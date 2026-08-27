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
