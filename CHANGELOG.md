# CHANGELOG.md

What shipped, when, and where the evidence is. One entry per shipped unit of
work, pointing at the record that carries the detail.

Four rules, and
[`scripts/common/check-changelog.sh`](scripts/common/check-changelog.sh) holds
all four:

1. ⛔ **Newest first.** A new entry goes at the top of its section.
2. ⛔ **Every heading carries a date**, as an ISO 8601 UTC stamp.
3. ⛔ **Every entry names its record**, the entry or commit carrying the
   evidence.
4. ⛔ **Every entry says whether it deployed.** "No deploy" is a complete and
   common answer. Silence is not.

⛔ Do not tidy this file while shipping something else, and do not delete an
entry. A superseded one is amended in place with a dated note.

---

## 2026-09-17

### 2026-09-17T17:30:08Z: the guard job stops paying for the same copy 419 times, and 4.0.0

**Record:** findings 100 to 102 in [`TODO/PROGRESS.md`](TODO/PROGRESS.md).
**Deployed:** ⭐ **yes**, as `wsl-toolkit-v4.0.0`.

⛔ **A MAJOR AND NOT A MINOR, because a published binary changed how it exits.**
`text-tool edit --between` now requires `--expect`, as every other search
operation already did: with no count it exited **0** and wrote nothing, both
when two ranges matched and when none did.
[`docs/consumers.md`](docs/consumers.md) calls that a break in its own words,
so a consumer pinning by major is not moved across it silently.

⭐ **The mutation job was 26 minutes on every push and the copy was 17 of them.**
Measured: one row in the biggest module is 6.40 s, of which the module copy is
3.13 s, and that module holds 332 of the table 419 rows. One staged copy per
module with a verified byte-exact restore, and the duplicate compile dropped
because the unmutated baseline already proves the module builds: **6.40 to
2.01 s per row**.

⛔ **Removing the duplicate compile made a branch unreachable and CI caught it
as THEATRE**, which is the harness working on itself. The dead branch is gone
and the row points at the guard that decides it now.

### 2026-09-17T16:29:52Z: a deadline on every engine call, and the findings that had been listed rather than fixed

**Record:** findings 90 to 99 in [`TODO/PROGRESS.md`](TODO/PROGRESS.md).
**Deployed:** no. The next `wsl-toolkit-v*` tag carries it.

⛔ **The session before this entry ended by handing the operator a list.** It had
been told to finish every open task and it closed the entries, then printed a
next prompt naming findings that had a named fix and no entry. That is the
deferral the instruction forbade, and this entry is the work it should have
done.

⭐ **Every host-engine call is bounded at one door.** The deadline used to be
chosen at each of seven call sites, and the largest was 30 minutes: a ceiling
nobody would sit through, which is why a stalled `podman pull` cost 28 minutes.
`engineCall` adds a STALL deadline beside the total, so a transfer that is
moving is never stopped and one that has produced nothing for four minutes is
given up, naming the stall.

⭐ **The mutation harness asks what a case did BEFORE the mutation.** It saw red
and reported "went red", so a case that was already failing certified every
guard it was named by. That is finding 1, the oldest in the record.

⛔ **One guard written this session was DELETED by the harness that proves
them**, and one check was removed because driving it contradicted a measurement
this repository already had. Both are recorded as what they are.

### 2026-09-17T15:43:52Z: the last two open entries close, and the tool watches a container

**Record:** `WSL-59` in [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md),
`WSL-91` and `WSL-93` in
[`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), findings 81 to 88 in
[`TODO/PROGRESS.md`](TODO/PROGRESS.md).
**Deployed:** no. Nothing is published by this unit; the next `wsl-toolkit-v*`
tag carries it.

⭐ **The observation layer reaches a container.** The timestamp layer, the
silence heartbeat, the event log and the exit reading were container-agnostic
and could only watch a distribution, so a podman workload got a byte counter and
nothing else. An `Observer` seam now answers about the thing the command is in,
and `run` takes the same flags `distro run` does.

⛔ **A feed that does not exist reports ABSENT, never zero.** `podman stats` on a
container running `sleep` answers `35048.23%` of a processor and `0B / 33.44GB`.
Both parse, neither is a measurement, and the resource column names its reason
instead of printing them.

⭐ **The gate drives the released-binary contract on every commit.**
`consumer.ps1` ran only against published binaries, so a refusal added to the
tool could not be found until a tag was cut - which is how
`wsl-toolkit-v3.0.0` published green and failed its own smoke. It takes `-Exe`
now and the gate runs it in 2.4 seconds.

⛔ **`text-tool --between` was the one edit operation with no required count.**
With no `--expect` it wrote nothing and exited **0**, both when two ranges
matched and when none did; with `--expect 1` and an anchor that appeared twice
it replaced the wrong region and reported `1 match(es)`. It took 745 lines out
of a 43 KB file in this session before it was seen. It is a breaking change to a
published binary, and every call it breaks was already doing nothing.

⛔ **The gate analysed one directory of three.** PSScriptAnalyzer ran over
`scripts/` alone, so the eight `.ps1` files under `tools/` were parsed and never
analysed - and the one real finding in the tree was in that half.


### 2026-09-17T14:10:00Z: wsl-toolkit-v3.1.0, and the first release whose smoke is green

**Record:** `WSL-90`'s closing and `WSL-92` in
[`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), findings 78 to 80 in
[`TODO/PROGRESS.md`](TODO/PROGRESS.md).
**Deployed:** ⭐ **yes**, as `wsl-toolkit-v3.1.0`, **22 assets, 11 files each with
a signature bundle**: the two `wsl-toolkit` executables, herdr 0.9.1 for four
targets, the four new `text-tool` builds, and `SHA256SUMS`. ⭐ **Both jobs green**,
`publish wsl-toolkit` and `the published release can be consumed`, which
`wsl-toolkit-v3.0.0` could not manage.

⛔ **It took three attempts and each failure was ours.** The first ended the
staged-binary step holding the exit code of a refusal it had just proved on
purpose. The second failed at `expected 7 assets to sign and found 11`, because
the number of published assets was written in three places and one was updated.
Neither published anything, so no consumer saw either.

⭐ **Driven from a consumer's side afterwards**, not read from a job summary: the
published `text-tool-windows-amd64.exe` downloaded, its digest compared against
`SHA256SUMS`, its keyless bundle verified, installed to `%USERPROFILE%\bin`, and
all 22 commands its skill shows run green against it, with the installed
`wsl-toolkit` moved from 2.0.2 by its own `selfupdate`. That pass found finding 80: this
repository's own documented verification command fails in Git Bash on Windows,
because the shell rewrites the identity regex before cosign sees it.

### 2026-09-17T12:16:46Z: text-tool becomes a product an agent downloads

**Record:** `WSL-92` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), and
findings 71 to 75 in [`TODO/PROGRESS.md`](TODO/PROGRESS.md).
**Deployed:** ⭐ **yes**, in `wsl-toolkit-v3.1.0`, which adds
`text-tool-windows-amd64.exe`, `text-tool-windows-arm64.exe`,
`text-tool-linux-amd64` and `text-tool-linux-arm64`, each with a signature
bundle.

`tools/text` is `tools/text-tool`, and the reason it moved is that it was not
reachable: it was built on demand inside a checkout, so the agents it was
written for could not use it. `eol` is a mode now and is the whole of `dos2unix`
and `unix2dos`; every mode takes as many files as it is given and all of them
change or none do; `--after` and `--before` insert beside a matched line and KEEP
that line. `skills/text-tool/SKILL.md` stands alone, and `check skills` refuses a
skill naming a `text-tool` flag its own usage text does not document.

⭐ **One digest from thirteen shell invocations on two operating systems**, by
`shell-matrix.sh` and `shell-matrix.ps1`: four shells under Git Bash, six in the
base, and pwsh, powershell 5.1 and cmd on Windows. That is the one claim the Go
suite cannot make, because its cases never cross a shell boundary.

⛔ **Four defects, all found by using the tool on itself**: `--between` cut its
anchors at the first comma, `edit --delete` read stdin and hung on an inherited
pipe, a substitution that retypes its own anchor deleted that anchor three times
in one session, and the line list capped at twenty silently.

### 2026-09-17T11:46:20Z: the smoke of the first released tag could not consume it

**Record:** finding 70 and `WSL-91` in [`TODO/PROGRESS.md`](TODO/PROGRESS.md) and
[`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md).
**Deployed:** ⛔ **no deploy.** On `main` as `f6ab39b`; it changes a test runner
and no shipped code.

`wsl-toolkit-v3.0.0` published with all 14 assets and its `release-smoke` job
FAILED. `consumer.ps1` wrote a configuration with `base.name` set and no instance
selected, which `WSL-74` had made a refusal, so the first release cut since that
rule landed was the one that carried it to a consumer. Fixed by selecting the
instance through the environment, beside the state directory that was already set
that way, and writing the configuration at both paths so the eight older tags
`-Tag` accepts still find one. Measured back to 8 passed, 0 failed against the
published tag, and green on the runner in run 35220237357.

### 2026-09-17T09:12:00Z: wsl-toolkit-v3.0.0, the first release carrying herdr

**Record:** `WSL-90` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md).
**Deployed:** ⭐ **yes**, as `wsl-toolkit-v3.0.0`, 14 assets: the two executables,
herdr 0.9.1 for Windows and Linux on both architectures, `SHA256SUMS` and a
signature bundle for each. It is the first release this repository has cut that
publishes another project's build, which is `WSL-90`'s approach step 4 and the
only thing that could prove it.

---
## 2026-09-16

### 2026-09-16T07:15:16Z: the first herdr nightly is published from this repository

**Record:** `WSL-90`'s amendment "the first nightly is published, verified and driven,
and the prune deletes the wrong release" in
[`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md).
**Deployed:** ⭐ **yes, and it is the first thing this repository has published other
than `wsl-toolkit` itself.** The prerelease `herdr-nightly-20260916-18061191fdc0`, from
[`.github/workflows/herdr-nightly.yml`](.github/workflows/herdr-nightly.yml) run
35066661420, 6 of 6 jobs green. No `wsl-toolkit-v*` tag was cut.
**Closes:** nothing. `WSL-90` stays open for its probe script and its closing.

⭐ **Twelve assets**: herdr built from its development branch at `18061191fdc0` for
Windows `x86_64` and `aarch64` and Linux `x86_64` and `aarch64` musl, `BUILD-INFO.json`,
`SHA256SUMS`, and a keyless `.cosign.bundle` for each. A downloaded copy passes
`sha256sum -c` and passes `cosign verify-blob` against this workflow's identity, and both
refuse a flipped byte and a wrong workflow identity.

⭐ **The herdr adapter's `nightly` channel is driven for the first time.** `base ensure`
on `wsl-toolkit-base` installed the nightly in 8.78 s over the digest its own
`SHA256SUMS` publishes, and the client `base attach` prints answered `--machine base
agent list` with exit 0.

⛔ **The first real run found a defect the suite could not.** `gh release create --target`
dates a release from the target commit, so the prune's `sort_by(.created_at)` ordered
nightlies by when this repository's `main` last moved and would have deleted the NEWEST
nightly once eight existed - the one the adapter resolves. It sorts by `published_at`
now. Nothing published was ever at risk, because the prune keeps seven and there is one.

## 2026-09-15

### 2026-09-15T12:01:11Z: the herdr adapter can follow the nightlies, and the build matrix's first run is read

**Record:** `WSL-90`'s amendment "the build matrix's first run, and step 5 written" in
[`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md).
**Deployed:** no deploy, and no tag. No nightly is published.
**Closes:** nothing.

⭐ **`"channel": "nightly"` on the herdr adapter** installs the newest herdr nightly's
Linux build over the digest its `SHA256SUMS` publishes, writes the Windows client of the
same build under the instance's state directory, and makes `base attach` print it. Four
cases and five mutation rows hold the channel's refusals.

⚠ **The first run of the build matrix built six of eight**: Zig 0.16.0 crashed on
Windows on Arm, and Zig 0.15.2 asserted building herdr `v0.9.0` for Windows `x86_64`.
The workflow now answers both, and has not yet run again.

### 2026-09-15T11:46:43Z: herdr gets a builder here, and this tool's release lookup stops reading one page

**Record:** `WSL-90` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), filed and
approved on 2026-09-15.
**Deployed:** no deploy, and no tag. No nightly has run.
**Closes:** nothing.

⭐ **The operator ruled that this repository builds and publishes herdr.** Two
workflows are written and not yet run: `herdr-build.yml` builds one herdr ref for
Windows `x86_64` and `aarch64` and Linux `x86_64` and `aarch64`, and
`herdr-nightly.yml` publishes herdr's development branch as a signed prerelease when it
moved and herdr's own CI passed, keeping seven. `release.yml` now also builds herdr's
newest stable release into each `wsl-toolkit` release. `docs/AGENTS.md` names two
published things.

⛔ **`wsl-toolkit`'s update check read one page of thirty releases**, so daily nightlies
would soon have hidden every `wsl-toolkit` release from it. It reads as many pages as
it takes, with two cases and three mutation rows, and the consumer smoke lists releases
without prereleases.

### 2026-09-15T11:16:41Z: Muse's herdr reporter meets a real herdr, and herdr's unreleased pages leave the router

**Record:** the amendments of 2026-09-15 to `WSL-76` and `WSL-78` in
[`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md).
**Deployed:** no deploy, and no tag.
**Closes:** nothing. `WSL-76` and `WSL-78` are open and wait for the operator's sign-in.

⛔ **The muse adapter registered its herdr reporter where Muse Code 1.3.0 never looks,
in a shape it never runs, and failed to install at all on a systemd base.** Measured
with Muse's credential-free echo provider: Muse reads `~/.config/muse/settings.json`,
runs a hook only inside a matcher group, and refuses to start over a file with no
`schema_version`; and `fs.protected_regular` refused root's write into a temporary
file the script had already given to the account. The adapter now writes the file,
shape and version Muse runs, and `TestTheHerdrReporterIsRegisteredWhereMuseReadsIt`
with five mutation rows holds each. Driven in a pane against herdr 0.9.0, the hook
reports every turn, releases the pane at the end, and adopts a resumed session.

⛔ **`herdr --machine` left every live page.** The reference sweep read herdr's pages
for its unreleased version, and herdr 0.9.0 refuses the flag with exit 2. `base herdr`
is the route, `base attach` prints it for an agent, and
[`docs/reference-sweeps/usable.md`](docs/reference-sweeps/usable.md) carries what the
measurement corrected. The Muse example lost two commands that failed as written.

### 2026-09-15T09:45:00Z: the executable carries the scripts, and the agents get adapters of their own

**Record:** `WSL-88` and `WSL-89` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md),
and the amendments to `WSL-76` and `WSL-78`.
**Deployed:** no deploy, and no tag.
**Closes:** nothing. `WSL-88` and `WSL-89` are open and their adapters are built but
not driven.

⭐ **A single binary now needs no clone and no fetch.** `wsl-toolkit shipped list`
names each general-purpose file the executable carries with its length and SHA-256,
`shipped cat` and `shipped write` produce one, and `base bootstrap` runs the carried
bootstrap inside the base with nothing copied anywhere. ⛔ The URLs in
[`docs/consumers.md`](docs/consumers.md) are unchanged and stay the contract for a
caller that does not hold the binary; what this removes is an operator copying
`bootstrap.sh` into a checkout with no way afterwards to say which version it was. A
`shipped` gate rule refuses the carried copy disagreeing with the file this repository
publishes, refuses one missing, and refuses one carried that is not shipped.

⭐ **Muse reports its own lifecycle to herdr.** The `muse` adapter installs a reporter
of this repository's own and merges it into Muse's `settings.json` for six events.
herdr ships a Muse detection manifest and no Muse integration, and the one proposed
upstream was closed unmerged on 2026-09-15, so a third-party hook is the only route.
⛔ It never writes stdout, because Muse reads a hook's stdout and it can influence the
agent; it always exits 0; and it never guesses a pane.

⭐ **`pi` and `omp` are adapters.** Both are npm packages installed with
`--ignore-scripts`, and both use **herdr's own** integration rather than anything of
this repository's, because herdr has one for each. ⛔ The omp adapter refuses when pi
and omp resolve to one extension directory, naming both paths and the variable that
caused it.

⭐ **An adapter's version moves without an edit to this tree.** `version` and `sha256`
on an adapter's entry pin a release this executable has never heard of, and ⛔ either
alone is refused. No adapter summary carries a version number any more.

⭐ **`base herdr` runs one herdr command in the base** with every argument herdr's
own, for an agent that cannot run herdr on Windows. It is a channel and not a wrapper:
it knows no subcommand and parses no answer.

⚠ **`tmux.conf` is a fallback and the pages now say so.** herdr's agent detection does
not inspect a tmux session launched inside one of its panes, so a shell framework that
auto-enters tmux makes the agent invisible.

### 2026-09-15T08:55:00Z: NetBSD drives both of bootstrap.sh's BSD package managers, and neither worked before

**Record:** `WSL-67` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the images, the digests, the drives, the five driver defects and the
three reviews.
**Deployed:** no deploy, and no tag. ⚠
[`scripts/common/bootstrap.sh`](scripts/common/bootstrap.sh) is fetched by URL, so this
reached a caller of `main` when it was pushed.
**Closes:** `WSL-67`.

⛔ **A STOCK NetBSD HAD NO PACKAGE MANAGER AT ALL, AND THE MESSAGE NAMED THE ONE IT WAS
STANDING ON.** `pkgin` is not installed on NetBSD 11.0 and `pkg_add` is in base, so the
run exited 2 with `no package manager found; looked for ... pkg_add pkgin ...` on a
system where `command -v pkg_add` answers. NetBSD now tries `pkgin`, then the base
`pkg_add`.

⛔ **NetBSD's `pkg_add` has no default repository and OpenBSD's has one.** With no
`PKG_PATH` exported, `pkg_add -I jq` answered `no pkg found for 'jq', sorry.` and a
five-name run reported five failures. `PKG_PATH` is now set from `uname -m` and
`uname -r`, for NetBSD only, and ⚠ a value the caller already exported is left alone.

⛔ **Eight table rows asked pkgsrc for what NetBSD base already provides.** `tar`,
`procps`, `openssh`, `npm`, `file`, `less`, `unzip` and `xz`; four are not in the
repository at all, so a run exited 1 with `absent=` empty - a report saying everything
was present while failing. Each row gained `os:netbsd=-`, confirmed by `pkg_info -Fe`.

⭐ **Driven on NetBSD 11.0 booted under QEMU on the maintainer's host**, after its
SHA512 was checked: `pkgin` exit 0 with `failures=0`, and with `pkgin` hidden the base
`pkg_add` exit 0 with `failures=0`. No flag changed and no exit code changed for any
Linux or FreeBSD caller.

### 2026-09-15T08:55:00Z: the agents' multiplexer is herdr everywhere a live page says so

**Record:** the sweep in
[`docs/reference-sweeps/findings.md`](docs/reference-sweeps/findings.md) and
[`docs/reference-sweeps/usable.md`](docs/reference-sweeps/usable.md), and the amendments
to `WSL-76` and `WSL-78`.
**Deployed:** no deploy, and no tag. ⚠
[`scripts/common/tmux.conf`](scripts/common/tmux.conf) is fetched by URL and changed.
**Closes:** nothing. `WSL-88` and `WSL-89` are new and open.

⛔ **`examples/common/zellij.md` is deleted.** The multiplexer it documents was replaced
on 2026-09-13 and the page had outlived the decision. The manual's link now names
`herdr.md`, and `examples/muse-code/README.md` went from eleven commands and two files
copied by hand to **three commands**, because `base.adapters` and `base agent` already do
the work the page was describing by hand.

⛔ **`examples/common/README.md` told a reader the agents' durable session was tmux, and
herdr cannot see an agent behind one.** herdr's agent detection does not inspect a tmux
session launched inside a pane: it sees `tmux` as the pane process and the agent becomes
invisible. tmux is the fallback for a shell outside herdr, and all three example pages now
say so.

⚠ **`tmux.conf` gained `extended-keys on` and `extended-keys-format csi-u`, behind a
version test.** Without them tmux strips modifier information and `Shift+Enter` arrives
as plain `Enter`, so an agent that binds `Enter` to submit cannot type a newline. The
option needs tmux 3.5 and the file targets 3.0, so a `case` on `tmux -V` sets it from 3.5
upward - including 3.10, which a numeric comparison gets wrong - and skips 3.0 to 3.4b
with empty stderr.

### 2026-09-15T06:37:22Z: a portable shell profile this tree owns, and the login file bash actually reads

**Record:** `WSL-71` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the drives, the three reviews and the conditions each figure was
taken under.
**Deployed:** no deploy, and no tag. ⚠
[`scripts/common/bootstrap.sh`](scripts/common/bootstrap.sh) is fetched by URL and
[`scripts/common/shell-profile.sh`](scripts/common/shell-profile.sh) is a new file
beside it, so both reached a caller of `main` when they were pushed.
**Closes:** `WSL-71`.

⭐ **`shell-profile.sh` is new, and it does one thing.** An INTERACTIVE login shell
whose working directory is a Windows drive WSL mounted for it moves to the account's
home and says so once. `bootstrap.sh --shell-profile PATH` and `--no-shell-profile`
select it, and the default is the file beside the script, as `tmux.conf` already was.
⛔ It fetches nothing, defines no alias and sets no prompt, and `PATH` for the prefix
stays `bootstrap.sh`'s line.

⛔ **A PATH LINE WRITTEN ONLY TO `~/.profile` DOES NOTHING ON THE RHEL FAMILY.** bash
reads the FIRST of `~/.bash_profile`, `~/.bash_login` and `~/.profile` and stops.
Measured on 2026-09-15: Fedora's `/etc/skel` ships `.bash_profile` and no `.profile`,
and with one present, `HEAD`'s `bootstrap.sh` left `$PREFIX/bin` off the PATH of a bash
login shell while reporting that it had added it. It is written to the other two where
they already exist, and ⚠ neither is created, because creating `~/.bash_profile` would
itself stop bash reading `~/.profile`.

⭐ **`base shell --here` now marks its own shell**, with `WSL_TOOLKIT_HERE` named in
`WSLENV`, so the profile leaves a shell that ASKED for the Windows directory where it
started. A caller's own `WSLENV` is extended rather than replaced. No flag changed and
no exit code changed.

### 2026-09-15T05:07:50Z: the debian and fedora presets build, and codegraph names a kernel it publishes no package for

**Record:** `WSL-86` and `WSL-87` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md),
whose closings carry the four preset builds, the agent matrix and the reviews.
**Deployed:** no deploy, and no tag. ⚠
[`scripts/common/bootstrap.sh`](scripts/common/bootstrap.sh) is fetched by URL, so its
half of this reached a caller of `main` when it was pushed.
**Closes:** `WSL-86`, `WSL-87` and `WSL-70`.

⚠ **TWO OF THE FOUR BASE PRESETS BUILD WHERE THEY DID NOT.** `base ensure` from the
`debian` preset ended in `netavark: nftables error: unable to execute nft: No such file
or directory`, because the provisioner's `apt` arm installed no firewall package; it
installs `nftables`. From the `fedora` preset it built and then failed verification,
because an imported rootfs's `newuidmap` carries neither the setuid bit nor
`cap_setuid`; the provisioner now reads the privilege rather than the path, restores a
dropped capability with `setcap`, and refuses with the capability named when it cannot.

⚠ **A BSD IS NAMED BEFORE A FETCH.** `bootstrap.sh --toolset agent` read the
architecture and not the kernel, so an amd64 FreeBSD, NetBSD or OpenBSD host resolved
codegraph's `linux-x64` package and fetched it. Such a run now fails codegraph with the
kernel's name and fetches nothing.

### 2026-09-15T03:03:22Z: bootstrap.sh installs CodeGraph where /bin/sh is dash, and names an npm too old for it

**Record:** `WSL-87` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
amendment of this date carries the measurements and the cases planted by hand.
**Deployed:** no deploy, and no tag. ⚠
[`scripts/common/bootstrap.sh`](scripts/common/bootstrap.sh) is fetched by URL, so this
reached a caller of `main` when it was pushed.
**Closes:** nothing. `WSL-87` stays open for its reviews.

⚠ **AN EXIT CHANGES MEANING ON DEBIAN, UBUNTU AND VOID.** `--toolset agent`, and any run
with `--codegraph` other than `none`, exited 1 with `npm did not write exactly one archive
into ` after every package installed, because the fetch's directory came back empty under
dash. It installs CodeGraph now, and exits 0 when nothing else failed.

⚠ **An npm older than 7.18.0 is named, and not tried.** CodeGraph is fetched with `npm pack
--pack-destination`, which npm 6 and npm 7.17 do not have; such a run fails CodeGraph with
the npm's version, where it said `npm could not fetch`.

### 2026-09-15T01:19:47Z: a base configured without passwordless sudo is refused while its account still has it

**Record:** `WSL-85` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the drive on a throwaway base, the cases and the reviews.
**Deployed:** no deploy. This is `main` only, and no tag was cut.
**Closes:** `WSL-85`.

⚠ **AN EXIT CHANGES MEANING FOR A BASE WHOSE SUDO DISAGREES WITH ITS CONFIGURATION.**
Under `passwordless_sudo: false`, `base status --probe` and `base ensure` answered exit 0
over an account that could still use sudo without a password. The probe now answers exit
1 naming it, and `base ensure` provisions the base again, which restarts it and removes
the rule this tool wrote. Where another rule grants it, the ensure answers exit 2.

⭐ **A verification that fails is named by the guest's own line.** A line wsl.exe writes
about itself, beginning `wsl: `, no longer stands in for the verifier's refusal or the
engine's error.

---

## 2026-09-14

### 2026-09-14T16:26:40Z: bootstrap.sh drops Soar, and finds, sets up and uses an installed Nix

**Record:** `WSL-67` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
amendment of this date carries the operator's rulings and the drives.
**Deployed:** no deploy, and no tag. ⚠
[`scripts/common/bootstrap.sh`](scripts/common/bootstrap.sh) is fetched by URL, so
this reached a caller of `main` when it was pushed.
**Closes:** nothing. `WSL-67` stays open for `pkgin`, `pkg_add` and the acceptance
runner.

⛔ **`--user-provider soar` IS REFUSED, WITH EXIT 2.** Soar is no longer detected or
used. `nix` and `none` are the values.

⚠ **THE NIX ROUTE INSTALLS DIFFERENTLY.** Names resolve to nixpkgs attributes
through the table's new `nix` key, where they were installed under their logical
names and two of four failed. A new profile installs with `nix profile install
--impure` and flakes, and a `nix-env` profile with `nix-env -f '<nixpkgs>' -iA`.
Every Nix command gets the four `NIXPKGS_ALLOW_*` variables, and the account's
`nix.conf` gains the flakes line when it names no experimental features.

⭐ **A Nix outside `PATH` is found**, in the account's or the daemon's profile, and a
`GITHUB_TOKEN` reaches Nix through `NIX_CONFIG` only. The report and `--json` carry
`nix_route` and `nix_flakes_config`.

### 2026-09-14T15:45:01Z: a BSD payload that prints a panic's two lines is no longer ended as a kernel panic

**Record:** `WSL-82` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the cases, the drive on a real guest and the reviews.
**Deployed:** no deploy. This is `main` only, and no tag was cut.
**Closes:** `WSL-82`.

⚠ **AN EXIT CHANGES MEANING FOR ONE KIND OF RUN.** A `bsd run` whose payload printed
`panic: ` and `cpuid = ` under it exited 2 as a kernel panic, with its guest killed
and its output cut. After those two lines a run now waits for QEMU's exit, the
command's closing marker or 60 seconds, so such a payload answers with its own exit
when it finishes within 60 seconds. A real panic still exits 2 with its line, after
its dump and reboot rather than when it prints, and a guest that hangs after one ends
at 60 seconds.

### 2026-09-14T15:31:34Z: a BSD run writes to an overlay, and the shared image is never written

**Record:** `WSL-83` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the runs on the shared image, the per-run cost and the reviews.
**Deployed:** no deploy. This is `main` only, and no tag was cut.
**Closes:** `WSL-83`.

⛔ **NOTHING A PAYLOAD INSTALLS SURVIVES ITS `bsd run`, WHICH BREAKS A CALLER THAT
INSTALLS IN ONE RUN AND USES IT IN THE NEXT.** A run boots from a qcow2 overlay over
the image, and removes the overlay when QEMU exits, so a kernel panic can no longer
leave the shared image unchecked or unable to boot. `bsd run` no longer grows the
image file: `--disk` sizes the overlay, and a `--disk` smaller than the image is still
refused. `qemu-img`, which ships with QEMU, is now required, and `bsd status` names it.

⭐ **A run costs what it did**: three `-c true` runs took 23.4 s to 23.6 s, each doing
the image's first-boot work.

⚠ **`bsd run --json` carries `root_not_dismounted`** when a boot finds the image's
own root not properly dismounted, with a warning to run `bsd fetch --force`.

### 2026-09-14T15:04:56Z: a base's Windows drives are read back, and a changed automount is applied

**Record:** `WSL-84` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the drive on a throwaway base, what driving it found and the reviews.
**Deployed:** no deploy. This is `main` only, and no tag was cut.
**Closes:** `WSL-84`.

⛔ **A BASE WHOSE DRIVES DISAGREE WITH `base.automount` IS PROVISIONED AGAIN, WHICH
RESTARTS IT.** The verification checked nothing for `ro` or `rw` and printed the
setting as its result, so a base changed from `rw` to `ro` kept `/mnt/c` writable and
reported `ro` and usable. It now reads every mount at or below a one-letter directory
under `/mnt`. `base ensure` over such a base exited 0 and changed nothing; it now
provisions the base again and restarts the distribution, which stops what runs in it.
`base status --probe` answers exit 1 until then.

⭐ **`base status --probe` prints what the drives read beside the setting**, and
`--json` carries it as `access.automount_guest`: `off`, `ro`, `rw`, or `mixed`.

⚠ **`base shell` reads the same mounts.** `--root` names each drive as read-only or
writable, where it called every drive writable. `--here` into a base with no drive
mounted is refused with exit 2, where it started the shell in the account's home.

### 2026-09-14T10:31:57Z: a provider base is rebuilt from a clone in one command, with herdr and Muse as adapters

**Record:** `WSL-77` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the prove from a fresh clone and the reviews.
**Deployed:** no deploy. This is `main` only, and no tag was cut.
**Closes:** `WSL-77`, in [issue 32](https://github.com/Azathothas/ToolKit/issues/32).

⭐ **`base.adapters` takes `muse`.** `base ensure` installs Muse Code for the base's
account from Meta's own installer, and runs that installer only while its SHA-256 is
the one the operator approved or the `installer_sha256` the configuration carries.
Any other stops the ensure with exit 2 before it runs, and says how to read the file.
`/usr/local/bin/muse` puts Muse on `PATH` for `base exec` and runs it only as that
account. Signing in stays the operator's.

⭐ **[`examples/muse-code/wsl-toolkit-base.json`](tools/windows/wsl-toolkit/examples/muse-code/wsl-toolkit-base.json)
is the one base every agent shares**, with the `herdr` and `muse` adapters. The new
configuration key is additive, and an adapter that runs no installer refuses it.

### 2026-09-14T09:54:56Z: the FreeBSD guest's disk grows to 12 GiB, and bootstrap.sh puts FreeBSD's nim on PATH

**Record:** `WSL-72` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the prove's output and the reviews.
**Deployed:** no deploy. This is `main` only, and no tag was cut. ⚠
[`scripts/common/bootstrap.sh`](scripts/common/bootstrap.sh) is fetched by URL, so
its change reached a caller of `main` when it was pushed.
**Closes:** `WSL-72`.

⚠ **THE GUEST DISK IS 12 GiB BY DEFAULT, AND THE PUBLISHED IMAGE IS 6.** `bsd run`
grows the image file to `--disk` GiB before it boots, then grows the partition and
the filesystem in the guest before the payload runs, which gives a 10.6 GiB root. The
file never shrinks: a smaller `--disk` is refused, and `bsd fetch --force` goes back to
the published image. `bsd status` prints the disk.

⭐ **`bootstrap.sh --toolset languages` completes on FreeBSD 15.1.** Rust fits, and
`nim`, which FreeBSD installs under `/usr/local/nim/bin` with nothing on `PATH`, is
linked into `~/.local/bin`.

### 2026-09-14T09:36:07Z: a FreeBSD guest that panics or stops ends its run at once, and a run uses one processor

**Record:** `WSL-81` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the panic-rate runs, the one-processor runs and the reviews.
**Deployed:** no deploy. This is `main` only, and no tag was cut.
**Closes:** `WSL-81`.

⭐ **A run no longer waits out its budget for a guest that has gone.** A guest whose
kernel panicked, or whose QEMU exited, cost its run the rest of `--timeout` and an
error naming a timeout. The run now ends when the console shows the panic or closes,
with exit 2 and the panic's line. A panic while the guest powers off leaves the
payload's exit standing, and is carried on the result as `shutdown_panic`.

⚠ **THE DEFAULT PROCESSOR COUNT IS ONE, WHERE IT WAS TWO.** With two, the guest
panicked in 8 of 15 heavy runs, under every CPU model and memory size tried; with
one, in none of 5. `--cpus 2` restores the old count. A `-c true` run measured about
23 seconds either way.

⚠ A payload that prints a FreeBSD panic's own two lines can end its run the same way.
`WSL-82` carries it.

⚠ **Amended 2026-09-14T09:54:56Z: one processor lowers the rate and does not end
it.** Two read-only runs in a row on the shared image panicked while powering off,
the first before the buffers synced, and the second on the filesystem the first left
unchecked. `WSL-83` carries it.

### 2026-09-14T06:30:00Z: one base serves every project, and a grant changes without a restart

**Record:** `WSL-75` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the prove on a throwaway base, what driving it found and the reviews.
**Deployed:** no deploy. This is `main` only, and no tag was cut.
**Closes:** `WSL-75`, in [issue 32](https://github.com/Azathothas/ToolKit/issues/32).

⛔ **A BREAK FOR ONE COMBINATION.** A named instance whose own `config.json` exists
now reads that file before a `wsl-toolkit.json` in the working directory. A command
run from inside a project with `--instance NAME` used the project's file before, and
now uses the instance's. `--config` still comes first, and `config` says which file
won and why.

⭐ **`base grant` and `base revoke` change one Windows directory grant with no
restart**, so nothing running in the base stops. The base changes first and the
configuration file second, and a grant a process is standing in stays as it was.
Provisioning now writes the fstab block through the same script.

### 2026-09-14T04:11:39Z: a BSD run takes seconds, and its script runs as written

**Record:** `WSL-79` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the boot series, the named cause and the acceptance output.
**Deployed:** no deploy. This is `main` only, and no tag was cut.
**Closes:** `WSL-79`, and with it [issue 33](https://github.com/Azathothas/ToolKit/issues/33).

⭐ **The boot went from about 115 seconds to a login to about 9.** QEMU under WHPX
showed the guest the host's hypervisor signature, and FreeBSD's Hyper-V VMBus driver
held root mount for about 105 seconds waiting on a VMBus that does not exist. The guest
is no longer shown the signature, and boots with no default devices.

⛔ **A SCRIPT'S LINES ARE NO LONGER JOINED, WHICH CHANGES WHAT RUNS.** A payload was
typed at the console with its newlines turned into `; `, so a comment line ended it
while the run exited 0, and a blank line or a split `if` did not parse. A script now
reaches the guest on its own read-only disk and runs as a file: a payload that relied
on the join runs every line, `-c` has no console line limit, and a script's stdin is
`/dev/null` where it was the console.

### 2026-09-14T04:11:39Z: a finished throwaway command no longer waits forever

**Record:** `WSL-80` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the stack dumps, the runs before and after, and the reviews.
**Deployed:** no deploy. This is `main` only, and no tag was cut.
**Closes:** `WSL-80`.

⛔ **`distro run` with a heartbeat could relay its command's last line and never
return**, in 6 of 17 runs on this host. The heartbeat's loop re-read a channel that
the end of the run cleared, and waited on nothing while the command sat finished. A
caller now gets its command's exit code, in 30 of 30 runs.

### 2026-09-14T02:49:38Z: an instance and its configuration name one distribution

**Record:** `WSL-74` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
closing carries the prove output and the three reviews.
**Deployed:** no deploy. This is `main` only, and no tag was cut.
**Closes:** `WSL-74`, the first entry of
[issue 32](https://github.com/Azathothas/ToolKit/issues/32).

⛔ **A BREAK FOR ONE SHAPE OF CONFIGURATION.** A file whose `base.name` is not the
selected instance's distribution was accepted, so `--instance muse` run from a checkout
whose file named `wsl-toolkit` acted on that distribution while it wrote muse's state.
Every command that reads a configuration now refuses it with exit 2, naming the file,
both distributions and the selection that agrees. A caller relying on the mismatch gets
2 where it got 0.

⚠ **`ready` changed with it.** A refused configuration was a problem in its report
and then still the configuration it read the base from, ran `--smoke` through, and
with `--ensure` built. It now checks no base, runs no smoke, names no base, and answers
`not-ready`.

## 2026-09-13

### 2026-09-13T15:53:39Z: the PowerShell product is gone, and the executable does everything it did

**Record:** `WSL-73` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md): its three
checkpoints, and its closing, which carries the parity map, the reviews and the
acceptance output.
**Deployed:** no deploy. This is `main` only. The product version is `3.0.0` and no
tag was cut, so the latest release is still `wsl-toolkit-v2.0.2`, which carries the
PowerShell product.
**Closes:** `WSL-73`. [Pull request 31](https://github.com/Azathothas/ToolKit/pull/31)
was reviewed, nothing in it was merged, and it is closed with the review on it.

⛔ **A BREAK THE OPERATOR RULED.** `wsl-toolkit.ps1`, its launcher, the copy the
executable carried and `wsl-toolkit script` are deleted: a raw fetch of the script at
`main` answers 404, and a commit before the deletion still serves it.
[`docs/consumers.md`](docs/consumers.md) says so, and each consumer migrates on its
own from the manual.

⭐ **Every capability the script had is a native command.** Its 12 actions and 40
parameters are `wsl-toolkit distro list|new|run|enter|remove|purge|snapshot|replay|compare`,
`hostaddress`, `doctor` and `resources`, with `--json`, a generated manual and this
tool's exit codes. A throwaway distribution is this tool's when WSL registered its
disk inside the state directory, not when its name carries `eph-`.

⛔ **A defect the script carried did not come across.** A command that read stdin
consumed the lines of the script after it, and the run exited 0 over commands that
never ran. Commands now travel framed, with `/dev/null` as their stdin, through
`distro` and `base exec` alike.

⚠ **Reviewing and driving the finished port changed more than it added.** A dry run
passed three things the real run refuses; a removal asked for confirmation before
refusing; a closed stdout ended the event log before its exit; a second run on one
state directory could fail the first by removing its own marker; and a snapshot could
replace one another run wrote under the same tag. Each is fixed with a case that went
red when its guard was removed.

### 2026-09-13T04:19:42Z: Muse Code installed, signed in, and driven by an agent from Windows

**Record:** `WSL-69` in [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose
2026-09-13 amendment carries the rulings and the defects and whose closing carries
the acceptance output.
**Deployed:** no deploy. This is `main` only; no tag was cut, and the executable
that carries `base exec` ships in the next release.
**Closes:** `WSL-69`. [Issue 30](https://github.com/Azathothas/ToolKit/issues/30)
stays open for the entries that remain.

⭐ **THE PROVIDER CLI RUNS IN ITS OWN NAMED BASE, AND AN AGENT DRIVES IT.** A
managed `muse` account with a private persistent home, opt-in passwordless sudo
validated by `visudo` before activation, one Windows checkout granted read-write,
and `wsl-toolkit base exec` as the one non-interactive seam. Muse read, wrote,
committed and pushed headless in 52 seconds, and answered through its interactive
screen in a Zellij pane ten seconds after the agent typed into it.

⛔ **SIX DEFECTS WERE FOUND BY BUILDING IT, NOT BY READING IT.** A restrictive
umask leaked through the provisioner and would have locked the account out of its
checkout; the verifier called that unreachable mount absent; `base exec` dropped a
failure to start `wsl.exe`; the verifier published a podman warning as the engine
version; an example carried a username in a path; and `automount off` left nine
empty drive mount points, so `ls /mnt/c` succeeded. Each is fixed, and each has a
guard that was planted and went red; the username's guard is `TOOL-25`'s.

⚠ **Verification asks more of every base**, so an older one reports unusable until
`base ensure` re-provisions it. [`docs/consumers.md`](docs/consumers.md) carries the
measurement.

### 2026-09-13T03:33:20Z: three gate rules that could not fail the way they were written

**Record:** `TOOL-23`, `TOOL-24` and `TOOL-25` in
[`TODO/tooling.md`](TODO/tooling.md), each closed with the planted defect and the
driven output.
**Deployed:** no deploy. This is `main` only; the rules check this repository's own
tree and ship in no release.
**Closes:** nothing on its own; found while finishing
[issue 30](https://github.com/Azathothas/ToolKit/issues/30).

⛔ **THE `bundle` RULE REBUILT THE PRODUCTS IT WAS MEANT TO COMPARE.** It ran
`build.ps1 -Test`, which writes both products before testing them, so a stale part
or a hand-edited product exited 0 with the tree rewritten underneath the gate. It
runs `-Check -Test` now, and a planted stale part fails it in a clone of the tree.

⚠ **A CHECK THAT COULD NOT RUN WAS PRINTED AS A PASS.** Both gate outputs read only
the problem count, so a host with no shellcheck and no Go printed `ok` for both.
The gate prints `skip` with the reason, the verdict says how many checks ran, and
the JSON carries a `skipped` object. The exit code is unchanged.

⛔ **THE SECRETS RULE COULD NOT SEE A WINDOWS HOME PATH.** Its separator class was
`[\/]`, which in Go matches a forward slash alone, the `TOOL-10` defect written
again in the port. The tree had published four home paths carrying a real
username; they say `USER` now, and the history still holds them.

## 2026-09-12

### 2026-09-12T16:20:00Z: the bootstrap leaves one tool's examples directory, and learns twelve package managers

**Record:** `WSL-67` in
[`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), whose amendment carries the
matrix result and the six defects the drive found, and sweep 2 in
[`docs/reference-sweeps/findings.md`](docs/reference-sweeps/findings.md) and
[`docs/reference-sweeps/usable.md`](docs/reference-sweeps/usable.md) for the seven
scripts read to get here.
**Deployed:** no deploy. This is `main` only; no tag was cut, and nothing in a
release changed.
**Closes:** nothing. [Issue 30](https://github.com/Azathothas/ToolKit/issues/30)
stays open: `WSL-68` and the provider-specific smoke are untouched by this.

⛔ **TWO FILES MOVED AND THE OLD PATHS 404.**
`tools/windows/wsl-toolkit/examples/common/bootstrap.sh` and `.../tmux.conf` are
now [`scripts/common/bootstrap.sh`](scripts/common/bootstrap.sh) and
[`scripts/common/tmux.conf`](scripts/common/tmux.conf).
[`docs/consumers.md`](docs/consumers.md) carries the break, and the exposure
window: both existed at the old path for one commit and neither has ever been in
a release.

⛔ **THE HARDCODED DIGESTS CAME OUT, AND THE REPLACEMENT IS WEAKER ON PURPOSE.**
The old file pinned CodeGraph 1.5.0 and three SHA-512 values; the registry was on
1.6.0 the next day. Version and digest are now read from the registry at run time,
which proves transport rather than authorship - the same property this
repository's own `SHA256SUMS` has. `--expect-integrity` and `--expect-sha256` put
the stronger check back for a caller who holds a value, and every run prints what
it resolved.

⭐ **Twelve package managers rather than one**, and three of them are BSD: apk,
apt, dnf, emerge, pacman, tdnf, xbps, yum, zypper, `pkg`, `pkgin`, `pkg_add`.
⭐ **FreeBSD 15.1 is driven**, through `bsd run --network` and a `fetch` of the raw
URL, which is also the consumer path: `pkg` installed ten packages, the eight
names its base already provides were skipped, and the report read every version
back off the machine. ⛔ `pkgin` and `pkg_add` are written and have never been
run, and the script header says so rather than letting the count imply otherwise.
`soar` and `nix` are used as user-level providers for an account with no root, and
⛔ neither is ever installed. A new `agent` toolset carries bash, Rust and cargo,
Go, Nim, Python and PowerShell; PowerShell has an upstream route because only
three of thirteen images package it.

⛔ **NO `awk`, `tr`, `find`, `grep`, `sed`, `install` OR `dirname`.** Photon
carries neither of the first two; openSUSE carries neither the first nor the third.
The first draft looked its table up with `awk`, and reported no row for ten logical
names on Photon as a result. [`scripts/README.md`](scripts/README.md) has the rule.

⚠ **`openSUSE` Tumbleweed joins the image catalogue**, which is the thirteenth row
and the only `zypper` one. The guard that asserted the catalogue held exactly
twelve images is a floor now: it still catches a row disappearing, and it no
longer has to be edited to add one.

**2026-09-12T18:10:00Z amendment, from the three review passes.** ⛔ **The
catalogue count guard had a hole this change put there.** Relaxing it from exactly
twelve to a floor of twelve meant that removing the row this change ADDED stayed
green; only removing a second fired. The floor is the current count now, 13, and
the message says that adding a row means raising it deliberately. Planted and
re-proved both ways. ⭐ **Four digest guards were also driven for the first time**,
each with the defect planted: a wrong computed npm digest, a wrong
`--expect-integrity`, a wrong computed PowerShell sha256, and a wrong
`--expect-sha256`. All four exit 1 with the right message, and the unmutated
baseline exits 0 in the same run. A flag enumeration in `docs/consumers.md` that
had gone stale was removed rather than completed.

**Driven:** `wsl-toolkit matrix --images all` with the 25-name `agent` toolset:
**13 ran, 2 failed** in 5m45s. Both failures are outside the script and neither is
worked around - Gentoo's stage3 has no portage tree, and Chimera's repository is
momentarily inconsistent between `openssl3` and `openssl3-devel`. Alpine reached
25 of 25 with CodeGraph 1.6.0 verified; Debian installed PowerShell 7.6.6 from
upstream with its published digest checked. `ubuntu:24.04` in a container reports
shellcheck **0.9.0**, which is CI's binary rather than this host's 0.11.0, and all
24 tracked scripts are clean under it.

### 2026-09-12T14:50:00Z: the two CI failures a green local gate could not see

**Record:** [`TODO/PROGRESS.md`](TODO/PROGRESS.md) and commit `bf5c095`.
**Deployed:** no deploy. `main` only.
**Closes:** [issue 29](https://github.com/Azathothas/ToolKit/issues/29), on CI run
`34700281005` with all six jobs green.

⛔ **The local gate was green on `fcca2ba` and CI was red on it in three jobs**,
and neither failure belonged to the issue being closed.

- `TestBaseMountPayloadIsEncodedAndEscaped` compared a resolved mount source
  against a raw `t.TempDir()`. The Windows runner puts `TEMP` under
  `C:\Users\RUNNER~1\`, an 8.3 short name `filepath.EvalSymlinks` expands, so the
  production canonicalization and the typed expectation disagreed there and agreed
  on every long-name host. Reproduced by pointing `TEMP` at a short-name
  directory: red before, green after.
- `shellcheck` refused `[ ... ] && [ ... ] || die` for SC2015. A development host
  carries 0.11.0, which does not report it; `ubuntu-latest` carries 0.9.0, which
  does.

⭐ **The second one is measurable here now rather than predicted**: `ubuntu:24.04`
in a container is the same shellcheck CI installs.

### 2026-09-12T13:30:00Z: the consumer's wrapper becomes features, and a BSD userland gets a command

**Record:** [`TODO/PROGRESS.md`](TODO/PROGRESS.md), and `WSL-63` to `WSL-66` in
[`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md) and `BSD-03` in
[`TODO/bsd.md`](TODO/bsd.md) for the measurements.
**Deployed:** no deploy. This is `main` only; no tag was cut.
**Closes:** [issue 29](https://github.com/Azathothas/ToolKit/issues/29), all
seven tasks. [Issue 30](https://github.com/Azathothas/ToolKit/issues/30) is
tracked as `WSL-67` and `WSL-68`.

**2026-09-12T14:18:33Z checkpoint correction:** issue 29's full acceptance
runner subsequently passed 71 of 71. Its Linux CI failure was a flaky test that
raced a growing file; the production writer and regression now share an
explicit stale header size. Issue 30 is no longer merely authored: the
provider-neutral base, explicit access grants, systemd/developer provisioning,
common bootstrap and live one-grant/zero-grant evidence are implemented.
Provider authentication and the security-boundary work remain open.

⛔ **BREAKING: `base.automount` defaults to `ro`.** WSL mounts every fixed drive
under `/mnt` inside the base, and a job could WRITE there, so a wrong path in one
destroyed the real checkout on the Windows host. That is the one thing this
tool's copy-never-mount rule exists to make impossible, reachable through a door
nobody opened on purpose. Reading still works. ⚠ **A caller that WRITES to
`/mnt/*` inside the base breaks**, and sets `base.automount` to `rw` to keep the
old behaviour, or `off` to remove the mount entirely. The three consumers in
[`docs/consumers.md`](docs/consumers.md) were checked and none writes there.

⭐ **Four repairs a consumer was carrying by hand are now the tool's.** Read from
`Azathothas/podbox`, whose `run-in-base.sh` is 151 lines and whose
`docs/containers.md` lists seven traps with the measurement beside each:

- **the executable bit.** NTFS holds no POSIX mode, so 396 of 396 scripts in a
  Windows checkout arrived unrunnable and the first to run failed
  `Permission denied`, naming the script and not the transfer. The copy reads
  the git index now, and a shebang covers a file that is in no index, which is
  the gap the consumer hit three weeks after writing its own repair. ⛔ Data is
  never marked executable, and the count is announced rather than applied in
  silence.
- **a file that grows while it copies.** The copy was unbounded against a header
  whose size was already written, so a background index or log killed the whole
  job with `archive/tar: write too long` in 475 ms. It is bounded now, the file
  travels as the prefix that was declared, and the result names it.
- **CRLF.** `--script` repaired it and `-c` did not, which is one gate on one
  path and none on its sibling. Both repair the copy that is sent.
- **`/mnt/c`.** Above.

⛔ **A defect that had been shipping, found by driving the tool rather than by
reading it.** A job that asked for no platform got no `--platform` on the podman
command line, so podman ran whatever variant of the image the local store already
held. After a single `--platform linux/arm64` job, `alpine` meant something else
to every later job on that host that named no architecture: `uname -m` answered
`aarch64` on an x86_64 machine, exit 0. ⚠ **The only sign was a line on podman's stderr**, which a
caller reading the JSON answer never sees, and a green suite could not have seen
it at all because it lives in what the image store contains. Every job names a
platform now, and the resolved value is on the result. `WSL-64`.

⛔ **The generated manual carried a control byte and its own drift check agreed
with it.** roff's font escape and Go's form feed are spelled identically in an
interpreted string literal, so `wsl-toolkit.1` shipped `0x0C`. A check that
compares generated output with generated output cannot see a generator that is
wrong, and the tree's `control-bytes` rule was blind for as long as the file
stayed untracked. `WSL-65`.

⭐ **`wsl-toolkit bsd` runs a command in a FreeBSD userland, with no nesting and
no elevation.** `qemu-system-x86_64 -accel whpx` puts FreeBSD 15.1-RELEASE on the
host's own hypervisor, beside the podman machine rather than inside it. Measured
here: login at 1m54s, session 1m58s, exit 0, the guest answering on its serial
console. The image is pinned and its digest verified before use, and it lives in
a cache SHARED across instances so a second agent does not fetch 635 MiB again.
⛔ **It reaches a BSD shell and not a BSD container endpoint**: a long-running
podman service panics the guest kernel. `BSD-03`.

⚠ **The nesting the ask offered was not needed.** The operator accepted nested
virtualisation as a floor. The record already said the non-nested route works on
this machine, so it was checked first and nothing nested was built.

⭐ **`--workspace .` no longer resolves against a directory nobody chose.** A
relative path resolves against the project configuration when there is one, the
resolved host path is printed, and a filesystem root, home directory or system
directory is refused outright. ⛔ Refused rather than corrected, because a tool
that picks a tree for you can pick the wrong one and still report success.
`WSL-66`.

⛔ **`scripts/windows/wsl-toolkit/wsl-toolkit.md` is REMOVED**, and
[`tools/windows/wsl-toolkit/wsl-toolkit.md`](tools/windows/wsl-toolkit/wsl-toolkit.md)
is the only usage page for both products. It was 1,374 lines of hand-written
parameter reference beside a script that carries complete comment-based help, and
a page that restates a binary's flags is a page that goes stale without anybody
touching it. ⚠ **The script itself is untouched**: its path, parameters and exit
codes are what consumers fetch and none of them moved.
`scripts/windows/wsl-toolkit/README.md` remains the build and release pipeline.

⭐ **Other things a caller can now see:** `config` reports the job defaults it
never showed, which is how a persistent-container setting can exist and no agent
know about it; `bsd fetch` resumes a partial download and reports progress; and
a large asset is cached per user rather than per instance.

---
## 2026-09-10

### 2026-09-10T17:20:00Z: wsl-toolkit 2.0.2, and three more review lenses

**Record:** [`TODO/PROGRESS.md`](TODO/PROGRESS.md), and
[`TODO/SUMMARY.md`](TODO/SUMMARY.md) for what each pass looked at.
**Deployed:** ⭐ **yes**, as `wsl-toolkit-v2.0.2`, ten assets. It carries what
`wsl-toolkit-v2.0.1` was cut before: `WSL-62`'s concurrent write path and
`WSL-60` and `WSL-61`'s capability and repair work.

⛔ **The three passes found three defects and one of them was a regression this
session had just introduced.** `ready` reported a working machine as `not-ready`
because a capability finding went into `problems`, and `r.Ready` is computed from
`len(problems)`. Only running the command found it; the reasoning that put it
there was wrong in a way reading could not see.

⚠ **The other two are the same shape as each other.** `ready` answered a failed
`base ensure` with `base ensure`, the command that had just refused; and the
helper dropped the state on an error, on both sides, so a refusal carrying the
exact command to run next crossed the protocol as a bare string.

⭐ **Two cases were theatre and the mutation harness said so.** Both asserted a
rule against a value written by hand, so removing the guard at the call site left
them green. One is a call-site case now and the other is a round trip: what the
refusal produces is sent back through the classifier that will read it.

### 2026-09-10T16:30:00Z: the base says what it can account for, and what to run

**Record:** [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), entries `WSL-60`
and `WSL-61`, both ruled by the operator in the session that authored them.
**Deployed:** ⛔ **no deploy.** On `main` and not in `wsl-toolkit-v2.0.1`. The
next tag carries this and `WSL-62`.

⭐ **The ruling on `WSL-60` was to take the answer that covers the most**, on the
grounds that the base may later be a systemd container or a full virtual machine
under KVM. That rules out both seams as written, because each answers "does THIS
base delegate" and the base is the thing that changes. What ships instead is a
measured CAPABILITY: `base status --probe` reports the mechanism by name, one of
`systemd`, `delegated`, `rootful`, `none` or `unknown`, and derives whether
limits are enforced from a container that asked for one.

⛔ **`unknown` is a real answer and is not `none`.** A container on this base has
`/sys/fs/cgroup` mounted and no `memory.max` in it, because it sits in the root
cgroup, which has no limit file by definition. The first version of the probe
called that unmeasured; it is the strongest evidence available that the limit was
not applied.

⭐ **`WSL-61` was ruled as the third option and extended**: `--repair` takes the
deletion, the unflagged `ensure` prints the command, and every condition is now a
`Remediation` an agent can act on rather than read - an id, what it costs, the
exact command, and whether this tool can take it. `base ensure` against a
registered distribution says ALREADY EXISTS before anything else.

⛔ **Driving it found a defect inside the fix.** The repair derived podman's run
directories from `$XDG_RUNTIME_DIR`, which in this base is WSLg's directory and
not podman's run root. It would have removed two directories that do not exist
and reported success. The root is asked of `podman info` now.

### 2026-09-10T14:50:00Z: four review lenses, and the sixth one that was owed

**Record:** [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md) entry `WSL-62`,
and [`TODO/SUMMARY.md`](TODO/SUMMARY.md) for what each pass looked at.
**Deployed:** ⛔ **no deploy.** `WSL-62`'s fix is on `main` and is NOT in
`wsl-toolkit-v2.0.1`, which was cut before the review found it. The next tag
carries it.

⭐ **The concurrency lens ran for the first time after three sessions named it,
and found a defect on its first pass.** `writeFileAtomic` wrote through
`path + ".tmp"`, one name for every writer, so two of this tool sharing one state
directory collided on it. Eight concurrent writers, forty rounds: seven failed
with a permission error for a write nothing was wrong with, and a unique
temporary alone was not enough because Windows refuses a replace while another
replace of the same target is in flight. `WSL-62`.

⚠ **The claim audit found three numbers that had drifted**, all of them the kind
a check cannot hold: a duration with no conditions that was off by more than
half, a count of guards that had grown six-fold, and an asset count from before
the release was signed.

### 2026-09-10T14:05:00Z: wsl-toolkit 2.0.1, and the first release that is signed

**Record:** [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), entry `WSL-25`,
closed against the published release rather than against the workflow that
would produce it.
**Deployed:** ⭐ **yes**, as `wsl-toolkit-v2.0.1`, ten assets: the script, the
launcher, the executable for two Windows architectures, `SHA256SUMS`, and one
`.cosign.bundle` per file.

⭐ **The signing path had never run.** `WSL-25` landed in the previous session
with every branch of the launcher's verify path driven against
`wsl-toolkit-v2.0.0`, which carries no bundles: what was proved was that `auto`
reports, `require` refuses, `off` says nothing was checked, and a bad mode is
refused before any network. Whether `cosign sign-blob --bundle` and
`cosign verify-blob --bundle` agree in a real run was unproved until this tag.

⚠ **Nothing in the product changed with the version.** 2.0.1 carries the same
surface as 2.0.0, forty entries in `surface.lock`; the bump exists because
`release.yml` refuses a tag that disagrees with the version inside the file, and
because the release is the artefact `WSL-25` closes against.



### 2026-09-10T13:55:00Z: the runtime check that never ran, and sixteen claims measured

**Record:** [`TODO/tooling.md`](TODO/tooling.md) entry `TOOL-22`, and
[`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md) entry `WSL-30`, with `WSL-59`,
`WSL-60` and `WSL-61` authored from what the matrix found.
**Deployed:** ⛔ **no deploy.** Nothing here reaches a published asset; the
workflow comments, the record and one Go helper are all internal.

⛔ **A guard that had never once been able to speak.** `check-remote-items`
reports what runtime a pinned commit declares, and every pin it has ever seen
printed `runtime unverified` instead, as a note rather than as a failure. The
cause and the measurement are in `TOOL-22`.

⭐ **Ten of the mockup's sixteen podman claims hold, three are absent, and the
three share one cause.** The base runs rootless podman under `init(wsl-toolkit)`
with no cgroup delegation, so no cgroup exists per container:
`/proc/<pid>/cgroup` reads `0::/`, `podman stats` reports `0B`, and `--memory` is
accepted and not enforced. `podman-machine-default`, rootful on the same kernel
with a read-write cgroup tree, answers all three. `WSL-30`, and `WSL-60` carries
the defect.

⚠ **The document those claims came from is 404 now**, and was never archived.
The claims are copied into the entry, which is what
[`docs/methodology/references.md`](docs/methodology/references.md) asks for and
what nobody had done.

### 2026-09-10T13:45:00Z: actions/setup-go moves to v7.0.0

**Record:** commit `295151a`, and the pull request it squashed.
**Deployed:** ⛔ **no deploy.** A workflow pin change takes effect on the next
run and reaches no published asset.

⚠ **The bump's own claims were re-derived rather than read.** The commit is what
`refs/tags/v7.0.0` resolves to, and `action.yml` at it declares `node24`. The
comments above two of the five pins still cited the `v6` ref they were resolved
against; both now name `v7.0.0` and the date they were re-resolved.

### 2026-09-10T13:40:00Z: the 2026-08-30 script backlog, re-derived and closed

**Record:** [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), entries `WSL-26`,
`WSL-27`, `WSL-28` and `WSL-29`, each closed in place with the output of its own
acceptance command.
**Deployed:** ⛔ **no deploy.** Nothing is published until the next release tag;
`scripts/windows/wsl-toolkit/wsl-toolkit.ps1` is fetched by raw URL from `main`
by two consumers, both of which pin a commit, so neither sees this until they
move their pin.

⭐ **They were re-derived against the tree first**, which the record asked for
and which nothing had done since they were written. All four seams the entries
named still existed at the files and lines they named.

| entry | what shipped |
| --- | --- |
| `WSL-26` | `-Action Snapshot -Name D -As TAG`, and `New -Tarball TAG` reading a tag back. ⛔ `Purge` never removes a snapshot. |
| `WSL-27` | `-ProgressPrefix TOKEN`. A prefixed line is consumed and the tick reports the last progress and its AGE. |
| `WSL-28` | `-Action Replay -From LOG` and `-Action Compare -From A -Against B`. |
| `WSL-29` | `-Reuse` on `New`, with the image reference read from an `origin.json` rather than out of the distro name. |

⭐ **`WSL-26`'s one design question is answered by structure, not by a name.**
Snapshots live in a subdirectory the orphan sweep does not enumerate, so `Purge`
cannot see one. A `snap-*.tar` convention was rejected: it is one rename away
from making every existing snapshot an orphan again.

⛔ **The `WSL-28` premise was right about the renderer and wrong about the
schema, and that is the story worth keeping.** The reader was first written
against `kind: LINE` with streams `out` and `err`; `Write-StreamLogLine` writes
`kind: LOG` with `stdout`, `stderr` or `watcher`. It rendered nothing and
reported a real run as having produced no output at all. The same guess was in
the selftest fixtures, so four cases went red the moment the reader was
corrected, and there is a case pinning the spelling now.

⚠ **The measurement `WSL-26` asked for, with its conditions.** Windows 11 Pro
26200, PowerShell 7.6.5, Alpine 3.22 with `jq` as the preparation: 1.82s and
1.79s from a snapshot against 10.04s and 10.85s preparing from the image each
time, with the first cold snapshot run at 4.60s. ⛔ Two runs each on one machine
with a small preparation. They do not support a ratio for a workload whose
preparation is minutes.

The surface lock is refreshed in the same commit, which is the record that the
six new parameters and the three new actions were a decision.

### 2026-09-10T13:10:00Z: `inspect` goes through the helper, and the protocol is 4

**Record:** [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md), entry `WSL-58`,
closed with the acceptance run.
**Deployed:** ⛔ **no deploy.** It ships in the next release tag.

`inspect` was the one report the helper route could not serve, so a restricted
client got the host half of the answer and not the machine half while its
sibling report `resources` had the door. That is the one-gated-door class in the
tool that keeps finding it.

⛔ **Adding a method is a break, and it is the whole cost.** The protocol moves
to `wsl-toolkit-helper/4`, and a client and a helper that disagree about the
version refuse each other by design, so a helper left running from
`wsl-toolkit-v2.0.0` refuses a newer client until `helper stop` and
`helper serve --detach` restart it. The manual owns what a consumer does about
that.

⭐ **Both routes turn a report into an exit code in one function**, and the
unknown-job refusal survives the wire as the same typed error, so the two routes
cannot answer one question with two exit codes.

### 2026-09-10T12:20:00Z: every release asset is signed, and the launcher checks it

**Record:** [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), entry `WSL-25`,
⚠ **still open**: it closes against a signed release, and none exists yet.
**Deployed:** ⛔ **no deploy.** The signing runs on the next `wsl-toolkit-v*`
tag.

`SHA256SUMS` ships in the same release as the asset it describes, so anyone who
could replace one could replace the other. The launcher has said exactly that on
every release fetch since it existed, and saying it is better than not saying it
and is not a fix.

`release.yml` signs every staged asset, `SHA256SUMS` included, with a keyless
sigstore signature tied to this workflow's OIDC identity, and ⛔ **verifies every
bundle it wrote before anything is uploaded**: a bundle that exists and does not
verify is a claim of authorship that fails the first time somebody checks it, on
a release that has already shipped.

`launcher.ps1` gains `-LauncherVerify auto|require|off`. ⛔ **Four outcomes and
never silence**: verified, no bundle in that release, no `cosign` on this
machine, or refused. Only ABSENCE is tolerated by `auto`; a bundle that fails is
a hard stop under every mode. Verification does not become mandatory in the
change that adds it, which is what the entry rules.

⚠ **The identity is anchored on the repository and the workflow, deliberately
not on the ref**, because `release.yml` also runs on `workflow_dispatch` and
pinning the ref would refuse such a release while telling the caller their asset
was unsigned.

[`docs/consumers.md`](docs/consumers.md) carries the row: a release now carries
ten assets rather than five, which is additive, and `-LauncherVerify require` is
the one thing here that can turn a working call into a refusal, only when a
caller asks for it.

### 2026-09-10T11:58:00Z: two guards that could not fail

**Record:** [`TODO/tooling.md`](TODO/tooling.md), entries `TOOL-20` and
`TOOL-21`, both closed with their mutation and driven evidence.
**Deployed:** ⛔ **no deploy.** Both are internal tooling.

⛔ **`check line-endings` could not fail on any text file in the tree.** Two
defects, and the first hid the second. `git ls-files --eol` writes its attribute
column as `attr/text eol=crlf`, with a space in it, so splitting the row on
whitespace put `eol=crlf` in a field of its own and the parse kept only `text`.
The comparison then read the INDEX column, and git normalises a text file to LF
in the index by definition, so what it asserted was a tautology.

⚠ **Twenty-three of fifty-four tracked `.ps1` files were sitting in the working
tree with LF under an `eol=crlf` attribute while it reported green**, which
`git status` cannot show and a fresh clone would not reproduce. Measured by
planting five CRLF into a file declared `eol=lf`: git reported `w/mixed`, the
loudest thing it can say about a file, and the check exited 0.

⛔ **`git-sync.ps1` could not stage a named set of files**, which is what
`docs/AGENTS.md` section 5 names as the way to commit on Windows. A `.ps1`
reached through `-File` cannot have a parameter repeated and its comma form
binds one string, so the failure named a path and read as the caller's typo.
`-Path` splits its own value now, and an empty element is refused rather than
forwarded: an empty pathspec means EVERYTHING to `git add --`.

⚠ **Both are the class [`docs/conventions/forbidden-patterns.md`](docs/conventions/forbidden-patterns.md)
already carried**, in files the rule had not been applied to. A rule recorded
against one file does not reach the next one on its own.

### 2026-09-10T11:20:00Z: the documents say what is true now

**Record:** [`TODO/SUMMARY.md`](TODO/SUMMARY.md) carries the pass; there is no
entry, because this is the claim audit applied to the documents rather than a
unit of work.
**Deployed:** ⛔ **no deploy.** Nothing a consumer fetches changed.

Six contradictions and eight stale facts, each fixed in the page that owns it.

| page | what it said | what is true |
| --- | --- | --- |
| [`README.md`](README.md) | each check is an `sh` and PowerShell pair | ⛔ the rules are one Go program and the scripts are thin wrappers. It contradicted `TODO/RULES.md` section 2 directly |
| [`README.md`](README.md) | `tools/` is the compiled half of `wsl-toolkit` | three modules: `check`, `repo`, and the tool |
| [`README.md`](README.md) | CI is three jobs plus a weekly pass | four workflows, and `RULES.md` carries the counts |
| [`docs/security/remote-ops.md`](docs/security/remote-ops.md) | an API call is read-only, no POST or PATCH, full stop | ⛔ that forbade this project's own release procedure. The carve-out is written and it is narrow: this repository's remote, under the push policy, for an action the operator asked for in the session |
| [`TODO/RULES.md`](TODO/RULES.md) | the gate takes about 30s, in the file whose own rule says a measured cost belongs in the record | the row carries no number now |
| [`TODO/RULES.md`](TODO/RULES.md) | the deletion rule applies to the PowerShell script | both products remove things. The line is drawn around STATE rather than around the word "remove", and `RemoveInside`'s own comment carries the same sentence |
| [`docs/conventions/docs.md`](docs/conventions/docs.md) | `PROGRESS.md` carries the runbook and threat-model roles as an open question | it did not. The open question is written rather than the sentence deleted |
| `scripts/windows/wsl-toolkit/selftest.md` | 63 cases over 15 functions, run by the gate "as `wsl-toolkit selftest`" | 131 over 36, on three hosts, and that command has not existed for a while |

[`docs/consumers.md`](docs/consumers.md) loses two discovery narratives and a
release story to [`docs/HISTORY/consumers.md`](docs/HISTORY/consumers.md), which
is where [`docs/conventions/prose.md`](docs/conventions/prose.md) says the story
of a fix goes. The register, the break definition, the pin state and the trap
stay; nothing was dropped.

⭐ **[`docs/conventions/forbidden-patterns.md`](docs/conventions/forbidden-patterns.md)
gains the four classes this session found**, and loses twelve stacked markers.
Its own prose rule says a page where every paragraph carries one has no markers
at all, and adding four rows had put it over the density ceiling the gate holds.
### 2026-09-10T10:55:00Z: `wsl-toolkit-v2.0.0` is published, and what three review lenses found after it

**Record:** [`TODO/PROGRESS.md`](TODO/PROGRESS.md) and
[`TODO/SUMMARY.md`](TODO/SUMMARY.md); [`TODO/tooling.md`](TODO/tooling.md)
carries `TOOL-12`, [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md) carries
`WSL-58`.
**Deployed:** ⭐ **yes.** `wsl-toolkit-v2.0.0`, five assets, digests computed in
CI over the bytes that were uploaded. Driven as a consumer at 12 of 12 from a
temp directory with no repository present, and again on a GitHub runner.

⭐ **The thirteen issues a consumer agent filed against `v1.3.0` are closed
against the published `v2.0.0`**, each re-derived against that binary rather
than against the tree that fixes it. Nothing is open on the remote except one
dependency bump, which needs an approving review.

**What the three review passes found, after the release:**

| lens | the finding it fired on |
| --- | --- |
| the door sweep | ⛔ `inspect --json` was not in the sweep that checks every `--json` surface, which is the guard `TOOL-17` built for exactly that class. The list is walked from the binary's flag sets now. |
| the door sweep | `inspect` gave no answer at all where `wsl.exe` is absent, including for the transcript and the ledger record, which are on this machine's own disk |
| the guard mutation | ⛔ nothing stopped a mutation row from mutating the TEST instead of the code, which turns the case red and prints `ok` over a guard nobody touched |
| the claim audit | ⛔ a false claim that had already been published, in a comment closing an issue. Corrected in place, on the issue. |

⚠ **`WSL-58` is filed and left open.** `inspect` has no `--via-helper` and every
other report does; the cost is a helper protocol version, which is not a thing
to bump inside the change that added the command.
### 2026-09-10T10:30:00Z: what a failed job ran on, and the obstacle that was not there

**Record:** [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md) carries `WSL-56`.
**Deployed:** ⚠ **not yet.** It ships in the next release.

`wsl-toolkit inspect [JOB]` answers what a failed job ran on: the engine and its
version, the OCI runtime, the storage driver and the filesystem under it, the
cgroup version and manager, whether the engine is rootless, how much disk the
distribution has left, and the container's own lifecycle with its last exit.

⭐ **The obstacle the entry named does not hold, and measuring it first was the
right call.** It said `run --rm` removes the container before anything can read
its last state, and asked whoever took it to find out whether podman retained
enough afterwards. It does: the event logger is `file`, and the `died` and
`remove` events both carry `ContainerExitCode` long after the container is gone.
So `--rm` stays, which is what stops a failed fleet leaving twelve containers
behind.

Two measurements kept in the code because the next reader would re-derive them:
`podman events --until 0s` answers nothing for events that are certainly there,
and `podman events` exits 0 for a container that never existed. ⛔ An unknown id
is therefore decided against four sources - a transcript, a ledger record, the
journal and a guest directory - and refused with its own exit code rather than
answered with an empty document.

⚠ **Two of the entry's requirements described things that are not there**, and
both are written down rather than quietly dropped: there is no chroot route in
this tool, and the two-engine hazard it names belongs to the HOST engine rather
than to the one jobs run in.

⭐ **It found a defect in a different guard.** `TestManualNamesEveryFlag` claims
a flag added tomorrow is covered without its file being touched. That was true
of a flag on a command already on its hand-written list and false of a whole new
command: `inspect` arrived with two flags and the case stayed green over both.
`main.go` dispatches from a table now and the test walks it, and the first run
after that change refused `--since` for not being in the manual.

Measured against the real machine: **69 of 69 acceptance cases**, including the
two this entry adds.
### 2026-09-10T09:30:00Z: the second host had not looked at four commits, and two cases were waiting

**Record:** [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md) carries `WSL-57`;
[`TODO/tooling.md`](TODO/tooling.md) carries `TOOL-11`.
**Deployed:** ⛔ **no deploy.** The selftest is not published and no consumer
fetches it.

On the first CI run that saw the 2.0.0 commits, `bundle` went red on ubuntu and
green on Windows. ⚠ Why it took that long is in the record rather than here. Two selftest cases built a path from `$env:TEMP`
and `$env:WINDIR`, ⛔ **both null under PowerShell on Linux**, where `Join-Path`
refuses a null path. They resolve `whoami` and ask the runtime for a temp
directory now, which is the rule this tree already states applied to a test.

⭐ **The finding worth more than the fix is that the ubuntu half is reachable
from this Windows host in about ten seconds**, so the same class no longer has to
be found by pushing:

```powershell
podman run --rm -v "${PWD}:/repo:ro" mcr.microsoft.com/powershell:latest pwsh -NoProfile -File /repo/scripts/windows/wsl-toolkit/selftest.ps1
```

Three hosts now report the same `131 case(s) over 36 function(s)`.

Beside it, `TOOL-11` closes: the Windows job runs the selftest under `pwsh` and
under `powershell.exe` and compares the two counts rather than asserting a typed
one. ⛔ **The old job ran 7 alone**, and a construct 5.1 cannot parse reached a
release with every check green. Planted and measured: pwsh 7 exits 0 over 131
cases and 5.1 refuses to parse the file at all.
### 2026-09-10T08:58:08Z: three of the guards were not being proved, and nothing said so

**Record:** [`TODO/tooling.md`](TODO/tooling.md) carries `TOOL-19`.
**Deployed:** ⛔ **no deploy.** It changes what a gate refuses and what CI runs,
not anything a consumer fetches.

The mutation table is the instrument that says a guard is real. Running it in
full, for the first time since the code under it moved, reported 58 of 61:

- two rows matched nothing, because `Extract`'s signature changed and the
  verdict rule moved out of `cmd_run.go`. Both still parsed and had been
  proving nothing since the day the code moved;
- one row was called THEATRE against a case that had never run. It calls
  `t.Skip` on Windows, and ⚠ **a skipped case prints `=== RUN` and leaves
  `go test` exiting 0**, which from outside the process is byte for byte a case
  that stayed green.

⭐ **The second is the harness's own defect class, in the harness.** Its header
says three outcomes and not two because a previous version collapsed three
answers into one. It then collapsed a fourth.

What changed: a nineteenth gate check, `mutations`, asserting every row still
reaches its subject - exactly one match, a tracked file, a real replacement, a
case that exists; a `SKIPPED` outcome in the harness, which is not counted as
proved and does not fail the run; and a CI job on ubuntu running the whole
table, where nothing skips.

⚠ **What the new check costs is measured in the entry, with its conditions**,
rather than summarised here. ⛔ The first draft of this paragraph said the cost
"did not move", which two uncontrolled runs do not support; the claim audit read
it before it shipped.

⭐ **It caught a fourth stale row while it was being written**, when two harness
tests were renamed and three rows still named the old names.

Beside it, the compiled tool's base record now goes through `RemoveInside` like
every other removal of state, and [`TODO/RULES.md`](TODO/RULES.md) section 3
draws the line the code actually holds: state that outlives the call goes
through the one deletion, and the rollback half of a write does not, because the
only root such a call could pass is the file's own directory and a containment
check that cannot refuse anything is theatre.
### 2026-09-10T07:13:31Z: `wsl-toolkit-v2.0.0`, and it is breaking on purpose

**Record:** [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md) carries `WSL-42`
through `WSL-56`; [`TODO/tooling.md`](TODO/tooling.md) carries `TOOL-17` and
`TOOL-18`.
**Deployed:** ⚠ **not yet.** The version in the tree is `2.0.0` and the tag is
the operator's to push:
`pwsh -NoProfile -File scripts/windows/wsl-toolkit/release.ps1 -Publish`.

Thirteen defects a consumer agent filed against the published `v1.3.0`, plus
three things the operator asked for. ⭐ **The finding worth keeping is the same
one as last time**: none of the thirteen was reachable by the 39 acceptance cases
that were green on the day they were filed.

**So the suite came first.** `TOOL-17` gave it a wall-time ceiling, a byte-exact
expectation, an assertion that a `--json` surface emitted exactly one parsable
object, and the ability to change state a running process has already read. Each
was written against the binary that HAD a defect and watched to fail before any
fix existed. Beside it, `consumer.ps1` downloads a published release, verifies
every digest, and drives it from a temp directory with no repository present.

**What is breaking, and each is deliberate:**

| change | what a caller sees |
| --- | --- |
| ownership is a prefix plus a marker the guest carries | a `base.name` outside `wsl-toolkit`/`wsl-toolkit-<instance>` is refused when the configuration is READ |
| `base ensure` no longer relabels what it cannot identify | it exits 1 over a base that disagrees with its configuration, rather than 0 over one it renamed |
| `stderr_bytes` is one smaller | framing no longer writes a newline into the payload's stream |
| `artifacts` means DELIVERED | `artifacts_attempted` carries what it used to mean |
| `duration_ns` is the interval the CALLER waited | it stopped at the container's death before |
| an artifact link out of the tree fails the job | it was silently converted to an inert `.link.txt` beside exit 0 |
| `script -Action List` exits nonzero when it was refused | it printed a partial inventory and exited 0 |
| `base shell` starts in the guest account's home | `--here` is the old behaviour |

**And what is additive:** `ready`, `selfupdate`, `artifacts retry`, `examples`,
`images pull`/`warm`, `config validate`/`--effective`, `resources --job`,
`gc --job`, `--instance`, `--config`, `--tick`, and a helper protocol at
version 3.

⭐ **Two defects were caught by guards this session had just built.** The
`--json` sweep found `artifacts retry` putting nothing on stdout, in a command
written hours after the same defect was fixed in `base ensure`. And the mutation
pass found a new selftest case that stayed green with the guard it covers
deleted, because the message it matched appears in both branches.

⚠ **`TOOL-17`'s consumer harness is red against `v1.3.0` and that is correct.**
It tests a published artifact, and the published artifact has the defects this
release fixes.

### 2026-09-10T06:30:00Z: the commit rule gets an instrument that can say no

**Record:** [`TODO/tooling.md`](TODO/tooling.md) carries `TOOL-15`.
**Deployed:** ⛔ **no deploy.** Nothing here is published; it changes what a
session can commit, not what a consumer fetches. ⚠ **Every existing checkout
will fail the gate until it runs `git config core.hooksPath .githooks`**, and the
finding names that command.

`check commits` is the only rule in the gate whose subject is `git log` rather
than the tracked tree, and a session runs the gate BEFORE it commits. At that
moment the commit being made does not exist, so the rule reads old commits and
reports green. It has never once prevented the defect it names. It has reported
it afterwards, twice, both times from a commit that was already pushed, and both
times the remedy was rewriting published history.

`check commit-msg FILE` applies the same four rules to a message that is not a
commit yet, and `.githooks/commit-msg` calls it with the file git hands it. A
refusal writes nothing. The hook calls the check rather than restating it, so
there is one copy of the rule.

A hook git does not clone is a preference again, so the gate gains an eighteenth
check, `hooks`, which refuses a tree with no tracked hook and a checkout not
running it. CI installs the hooks in one line per gate job, so that rule needs no
exception for CI.

⭐ **`tools/check` had no tests at all before this.** `go test` over the gate
ran zero cases, so its own `go` check was vacuously green about it. Five cases now
cover the new mode and the new check, which is the first five rather than coverage
of eighteen.

### 2026-09-10T05:40:00Z: wsl-toolkit v1.3.0, what two readings found with nothing reported

**Record:** [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md) carries `WSL-40`
and `WSL-41`.
**Deployed:** ⭐ **published as `wsl-toolkit-v1.3.0`**, five assets, digests
recomputed from the downloaded files rather than read from the build.

Nothing was reported broken. `v1.2.0` had just closed eight consumer-filed
issues, passed 37 acceptance cases against a real machine and 29 proved
mutations. These are the eight defects that two deliberate readings found anyway,
and they were all in `v1.2.0` and in `v1.1.0` before it.

The core pass, `WSL-40`:

- `prefixWriter` is written to from TWO goroutines, because `provision` passes one
  instance as both `Stdout` and `Stderr` and `os/exec` runs a copier per stream.
  Its `seen` buffer locked itself; its `partial` line buffer did not. It now takes
  a mutex, bounds the unterminated tail at 64 KiB and FLUSHES it rather than
  dropping it, and releases the lock before calling the caller's logger.
- A catalog id of `..` passed `isImageID`, and `matrix --artifacts out` writes
  each row into `out/<id>`. A leading dot is refused.
- `Ledger.Compact` read the file with no lock and then took the lock to write, so
  a record appended between those two steps was read by neither and discarded by
  the write. It takes the lock once.

The review after it, `WSL-41`, took one defect class, a failure reported as a
benign outcome, and read every discarded error in the module:

- `NewClientSpool` had four silent returns, so a helper-route job ran, succeeded,
  and `logs` found nothing with no line saying why. The direct route had logged
  the same failure all along.
- `ClientSpool.Finish` returned nothing when it could not file a transcript it was
  holding. It returns the path the bytes are actually at.
- `logs` reported every read failure as "no transcripts on this machine yet" at
  exit 0.
- `helper stop` discarded the reason nothing answered, one of which is "something
  else is listening on that address".
- Two quality-of-life lines: `run` now prints the command that reads its output
  back, and the matrix table says once that each row's output is kept. ⛔ The
  job id was nowhere in the human output, so `logs ID` could not be typed without
  first running `logs` bare to go hunting for it, which makes the whole feature
  one nobody could find.

⚠ **Two behaviour changes.** A configuration carrying a dot-named image or
distribution is now refused at the point it is read, and the refusal names the
field; nothing plausible is in that set. `logs` on a machine whose `jobs` path
cannot be READ now exits 2 rather than 0, which is the point of the change.
`WSL-41` owns the detail, including what this does and does not separate on
Windows.


### 2026-09-10T02:10:00Z: five of the last six shell pairs become one Go program

**Record:** [`TODO/tooling.md`](TODO/tooling.md) carries `TOOL-14`.
**Deployed:** ⛔ **no deploy.** Nothing under `scripts/common/` or `tools/repo/`
is published as a release; this changes what a session runs, not what a consumer
fetches. ⚠ `Azathothas/TEMPLATE` ships these scripts by raw URL, so a
consumer keeps a working command and gains a Go toolchain requirement. Each
wrapper says so by name rather than failing at a missing binary.

[`tools/repo/`](tools/repo/) is this repository's tool box: the tools that are
not gate checks. `deslop`, `license`, `binfmt`, `remote-items` and `git-sync`
are subcommands of one binary, and the scripts named after them are wrappers,
so the documented commands and the raw-URL fetch both keep working.

⛔ **It is deliberately not `tools/check`.** That binary holds the rules this
repository enforces over its own tree and `check-gate` runs all of them. A
commit path and a licence writer are not rules; folding them in would make the
gate do things that are not checks, and a gate whose scope drifts is one nobody
can state the meaning of.

Every port was measured against the thing it replaces rather than read against
it, and `TOOL-14` carries the five comparisons.

⛔ **The sixth is not ported, and the entry carries the correction under its
premise rather than in place of it.** `scripts/doctor/` runs BEFORE you know
what is installed, and "is there a Go toolchain" is one of the questions it
answers; a wrapper that had to build one first could not report that it was
missing. `check-twins.sh` stated that rule before `TOOL-14` was written, and the
entry was authored from a count rather than from the rule.

⭐ **Two dependencies go and one workaround with them.** `jq` and `curl` are
replaced by the standard library and by `gh` itself, which matters beyond
tidiness: the shell version fetched `action.yml` with no credential, so a
private action or a rate-limited runner read as "runtime unverified" rather than
as what it declares. And `binfmt` no longer needs `MSYS_NO_PATHCONV`, because
`exec.Command` passes its argument list to `CreateProcess` untouched.

## 2026-09-09

### 2026-09-10T01:48:14Z: the mutation harness joins the tree

**Record:** [`TODO/tooling.md`](TODO/tooling.md) carries `TOOL-16`.
**Deployed:** ⛔ **no deploy.** Nothing here is published.

⚠ **Amended 2026-09-10: this heading said `07:45:00Z`, this entry has MOVED
DOWN to match, and that stamp was typed rather than read.** The commit that added this entry, `d7f7136`, is
`2026-09-10T07:33:14+05:45`, which is `01:48:14Z`: the machine runs at UTC+05:45,
so the original was nearly six hours ahead of the work and rounded to the minute.
It is corrected to the commit's own instant rather than left, because a later
entry carrying a TRUE stamp then sorts below it and the file's newest-first rule
reports the honest entry as the wrong one.

⚠ **The other stamps above it are rounded to the minute and were typed too**,
and they are NOT retro-corrected: they are ordered correctly relative to each
other, nobody recorded their real hour, and inventing one is fabricating
evidence. This one is corrected because its value collided with a measured one.
⛔ Read it from the machine.

The harness that proves this tree's guards are real was a Python script under
`.tmp/`, which is gitignored, and three records had just been written citing it
as the command that closed them. Those commands could not be run by anyone
reading the record afterwards, including the next session here.

It is `repo mutate` now, with its table in `tools/repo/mutations.json`,
generated from the script rather than retyped. It reproduces the script's answer
over both modules with no row changed, and it has five cases of its own.

⚠ **It is not a gate check and that is deliberate.** One pass copies every
module and runs a suite per row; a gate somebody waits minutes for is a gate they
skip. It belongs beside the other tools that are not rules.

### 2026-09-09T21:30:00Z: eight defects a consumer found in the published binary

**Record:** [`TODO/wsl-toolkit-go.md`](TODO/wsl-toolkit-go.md) carries `WSL-32`
through `WSL-39`, one per issue.
**Deployed:** ⭐ **yes, as `wsl-toolkit-v1.2.0`.** Every consumer of the release
is affected; `launcher.ps1` fetches whatever is named, so a caller pinned to
`wsl-toolkit-v1.1.0` keeps the defects until they move.

A consumer agent tested the published `wsl-toolkit-v1.1.0` from outside this tree
and filed eight issues before running out of budget. The tree's own gate was
green, its acceptance runner passed 23 of 23, and it had been through three
review lenses. None of that found any of these.

Five were P1. Automatic helper routing decided on `wsl.exe` RESOLVING rather than
answering, so the one caller the helper exists for never reached it. A failed
artifact transfer left the exit code alone, so a build could lose its deliverables
at exit 0 and then have the recoverable guest copy torn down. Artifact names that
collide on NTFS -- a case pair, or `name:stream` -- overwrote each other and
reported success. `run` cut stdout at 8 MiB and stderr at 2 MiB with no signal,
and replayed everything only after the job ended. And `gc --apply --older-than
24h` force-removed every labelled container BEFORE evaluating any age, which
killed a job that had been running for seconds.

⛔ **Two behaviour changes a caller can see.** A job whose requested artifacts do
not arrive now exits 1 rather than 0, and counts as a failed fleet row; a
pipeline that was passing over a lost transfer will start failing, which is the
point. And the helper's `run` and `matrix` answer with a stream of events rather
than one object, so the protocol version moved to 2 and a client refuses a helper
of a different build before sending it work.

⭐ **What is new rather than fixed.** `wsl-toolkit logs` reads a job's complete
transcript back, `--max-output` sets what the answer keeps, `gc --include-live`
is the only way to remove running work, `gc` now reports what it KEPT and why,
and a fleet announces each row as it finishes on both routes.

### 2026-09-09T16:05:00Z: the gate is one binary, and it runs in 30 seconds

**Record:** [`TODO/tooling.md`](TODO/tooling.md) carries `TOOL-13`.
**Deployed:** ⛔ **no deploy.** Nothing under `scripts/common/` or `tools/check/`
is published; this changes what a session and CI run, not what a consumer
fetches.

The gate took about twelve minutes on this machine: the `--fast` run measured
6m20s and `check-twins`, which `--fast` skips, measured 5m35s on its own. That
check existed because every rule was written twice, in sh and in PowerShell, so
a third check had to run both halves of every pair and compare their answers.
The twin requirement was not wrong -- a POSIX check cannot be assumed to run on
Windows, which is the default host here -- but answering it with a second
implementation is what made the third check necessary.

[`tools/check/`](tools/check/) is one Go program holding all seventeen rules,
copied from `Azathothas/pg-toolkit`'s `cmd/check` and adapted rule by rule to
what this repository already enforced. One tree walk shared by every check, and
about 30 seconds for the lot, including shellcheck, PSScriptAnalyzer, a rebuild
of both generated products and the full Go suite. Every documented command still
works: the per-check scripts are wrappers around one named check now, and
`--fast` is refused by name rather than silently meaning nothing.

⭐ **A rule this repository states about its own prose had nothing enforcing
it.** [`docs/conventions/prose.md`](docs/conventions/prose.md) lists adjectives
that assert quality instead of demonstrating it; two of them were in live pages
and are now gone. `TOOL-13` names which.

⛔ **The copy dropped a scope, and a planted key proved it.** pg-toolkit's tree
loader reads tracked files only; every check here scans tracked plus
untracked-but-not-ignored, because a file that has never been staged is exactly
when a new one is most likely to carry a credential. An AWS key dropped into the
working tree went unreported until the second list was put back.

### 2026-09-09T09:00:00Z: an executable joins the script, and it owns a Linux host

**Record:** [`TODO/issue-6.md`](TODO/issue-6.md) carries `WSL-31`;
[`TODO/PROGRESS.md`](TODO/PROGRESS.md) is the state.
**Deployed:** yes. `wsl-toolkit-v1.1.0`, with `wsl-toolkit.ps1`, `launcher.ps1`,
`wsl-toolkit-windows-amd64.exe`, `wsl-toolkit-windows-arm64.exe` and a
`SHA256SUMS` computed in CI over the bytes that were uploaded.

Resolves [issue 6](https://github.com/Azathothas/ToolKit/issues/6). The five
things it asked for and what each became:

**A better user and root mode.** `-UserEnv` prepares a per-uid
`XDG_RUNTIME_DIR`, which is what rootless podman needs and what `runuser` leaves
unset; without it a build dies at `cannot create state directory for
buildah-...`, which reads as a broken image. It also puts every `PATH` entry
through one function that tests for a duplicate, so a nested call no longer grows
the variable by a dozen entries per layer, and it separates a guest with no
`stat` from a runtime directory another uid owns.

**A single-file executable.** `tools/windows/wsl-toolkit` carries
`wsl-toolkit.ps1` inside itself and forwards to it, so there is one
implementation of the throwaway-distro behaviour rather than two. The embedded
copy is written by the same `build.ps1` that writes the tracked bundle, and
`build.ps1 -Check` compares both against the parts.

**A fleet runner.** `matrix` commissions a container per image and decommissions
all of them, with the workspace sent once and copied per row. Three counts
rather than one, because "the image could not be pulled" and "the subject is
broken" need different next moves, and a run where nothing ran exits 2.

**A default container catalog.** Twelve fully qualified references, and a
dedicated `wsl-toolkit` WSL distribution with a rootless engine in it, so
`podman-machine-default` is never touched. The base is meant to be wrecked:
`base ensure` re-provisions a registered-but-unusable one in place and rebuilds
only what will not provision.

**A host survey that answers.** `doctor` resolves past every shim and reports
what would actually run. It found four wrong answers in its own first version,
each a wrong answer rather than a crash: a scoop junction that `EvalSymlinks`
cannot follow made node, ruby and java read as absent; resolving a multiplexer
and then executing the target reported rustup's version as rustc's; one version
pattern read `v4.35.1` as `35.1` and `go1.27.0` as nothing; and a command script
assembled as an argument list exited 1 where the same script by hand exits 0.

⛔ **What the deep review changed, and it was four things.** The door sweep
asked which surfaces reach each new affordance and found the two job routes
disagreeing: `--user` was accepted on the helper route and dropped, so a caller
asking for uid 1000 got root with nothing said. The same sweep found the launcher
deciding "verification failed" from the wording of an error, and a named release
losing to a stale executable beside the launcher. `-LauncherSha256` was ignored
outright on the three paths that name a file rather than fetching one. The guard
mutation found one of its own: deleting the protected-distribution list left the
suite green, because the exact-name rule refuses those names anyway and nothing
asserted the reason the list exists to give. Each is fixed, and each has a case
that fails without the fix.

⛔ **CI then found three more, and none of them could have been found here.**
A workspace symlink pointing out of the tree was packed rather than refused,
because an absolute target was joined onto the link's own directory and landed
inside the workspace by every containment test there is; the case that covers it
cannot run on Windows. A path converter answered differently on Linux. A runner's
short-form temporary directory failed a comparison against a resolver that was
right. The ubuntu job is the second host every check in this repository earns,
and this is the second session in which it earned it.

⛔ **The incident the isolation answers.** A script running in a guest ended with
a recursive removal of a path that pointed at a Windows drive mount. A removal on
a drive mount does not go through the recycle bin, and 29,339 files went in one
call. No host directory is mounted into a container here: a job gets a copy, and
getting anything back is a second act with per-entry validation. Driven on this
machine: a container ran `rm -rf /work/*` and the host workspace was
byte-identical afterwards.

⛔ **Three breaks, and [`docs/consumers.md`](docs/consumers.md) carries the
rows.** The launcher runs the executable by default, a release carries four
assets rather than two, and a read-only script action no longer creates the state
directory. `-LauncherKind script` restores the launcher's previous behaviour
exactly.

**Measured on one Windows 11 Pro 26200 host, 2026-09-09.** The transport: a
script on `wsl.exe` stdin arrives byte-exact and its exit code propagates, while
the same payload as an argument had its dollar name expanded, its backtick
executed, and still reported exit 0. The base: arch 31s and 940 MiB, alpine 28s
and 204 MiB, debian 37s and 556 MiB, each to a rootless container returning a
marker. The fleet: 12 of 12 catalog images ran, 0 failed, 0 unreached, 52.1s with
cold pulls, artifacts returned from every row.


## 2026-08-30

### 2026-08-30T08:31:12Z: the tool becomes a project, and this repository publishes something

**Record:** [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md) carries `WSL-21`
through `WSL-24`; [`TODO/tooling.md`](TODO/tooling.md) carries `TOOL-09` and
`TOOL-10`; [`TODO/docs.md`](TODO/docs.md) carries `DOC-06` and `DOC-07`;
[`TODO/PROGRESS.md`](TODO/PROGRESS.md) is the state.
**Deployed:** ⭐ **yes, and it is the first time.** `wsl-toolkit-v1.0.0` is
published, with `wsl-toolkit.ps1`, `launcher.ps1` and a `SHA256SUMS` computed in
CI over the bytes that were uploaded. The workflow succeeded on its first run,
and the whole path was then driven from an empty directory holding only the
launcher.

⛔ **This entry amends every "this repository publishes nothing" line above it.**
Each of those was true when it was written and none is now. The live pages,
[`README.md`](README.md), [`docs/AGENTS.md`](docs/AGENTS.md) and
[`TODO/RULES.md`](TODO/RULES.md), carry the current fact; the lines below are
left alone because they are the record of what shipped on the day they name.

**What shipped.**

- ⭐ **The 2,792-line single file became 27 parts and a build.**
  `scripts/powershell-windows/wsl-ephemeral.ps1` is now
  `scripts/windows/wsl-toolkit/wsl-toolkit.ps1`, GENERATED by `build.ps1` from
  `src/`, `core/` and `libs/` in the order `bundle.manifest` names. The first
  split was proved byte-identical to the original before anything else changed.
- ⛔ **The old path is gone rather than redirected.** A raw fetch 404s. A git
  symlink was measured and rejected: `raw.githubusercontent.com` serves a
  symlink's own target string with HTTP 200, which is a successful-looking 34
  bytes that nothing can run.
- **A release pipeline.** `release.ps1` verifies and tags;
  `.github/workflows/release.yml` re-verifies on a clean checkout and publishes.
  The launcher grew `-LauncherRelease TAG|latest`, which verifies the asset
  against the release's own `SHA256SUMS` and says out loud that this proves
  transport rather than authorship.
- **Thirteen parameters and an action, most of them from the mockup the issue
  cited**: `-Action Doctor`, `-DryRun`, `-TimestampColumns`, `-TimestampProfile`,
  `-TimestampSeparator`, `-PrefixOnly`, `-Color`, `-StreamLogPath`,
  `-StreamLogOverwrite`, `-EventLog`, `-Redact`, `-MaxLineBytes`,
  `-TickEscalateSeconds`, `-ScriptArgFile`.
- **The tick escalates rather than repeating**, silence ending is its own line,
  and a non-zero exit gets a reading instead of a number.
- **Two new gate checks**, `wsl-toolkit bundle` in both halves and in CI, and a
  case-shadowed-parameter scan inside `build.ps1 -Test`. The suite went from 63
  cases to 117.

**What was found while doing it**, each by a different pass:

| what | how |
| --- | --- |
| a `$state` local shadowing a `$State` parameter, killing the tick mid-run | ⭐ driving a real distro. The suite could not see it. |
| ⛔ `-ScriptArg` documented as repeatable and never bindable through `-File` | driving the documented example from the issue comment |
| ⛔ `[int[]] -TickEscalateSeconds 5,9` binding the single value `59` | instrumenting an escalation that silently never fired |
| ⛔ `check-docs.ps1` reporting correct three-deep links as broken, for as long as nothing was three deep | the two halves of the twin disagreeing |
| a new check reporting one blank finding over a clean tree | reading the finding instead of the exit code |
| the vhdx write TIME advancing while a guest slept, so it is not a progress signal | sampling it against a guaranteed-idle guest |
| ⛔ `check-no-secrets.ps1` unable to match a Windows home path at all, on the host that produces them | the FULL gate: `check-twins` named the drift, and `--fast` skips it |
| ⛔ `-TimestampProfile raw` reading settings its own branch never built | the door sweep, on "what other door reaches this code" |
| a sink-path refusal that a `-DryRun` returned before ever reaching | the guard-mutation pass, planting `-StreamLogPath nul` |

**And the open question from the last session is now measured rather than
reasoned:** the stream log runs correctly under Windows PowerShell 5.1, driven
against a real Alpine distro, including the carriage-return redraw and the exit
code passthrough.

### 2026-08-30T06:10:00Z: issue 5, and the three things a consumer had to do by hand

**Record:** [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md) carries `WSL-16`
through `WSL-20`; [`TODO/PROGRESS.md`](TODO/PROGRESS.md) is the state and says
what was deferred.
**Deployed:** ⛔ **no deploy.** This repository publishes nothing.

**What shipped.** `wsl-ephemeral.ps1` grew a stream log that is on by default: a
timestamp on every line of a command's output, a heartbeat while there are none,
and a carriage return treated as a line terminator so a progress meter is
visible instead of being twenty minutes of nothing. `-NoTimestamps` turns all of
it off and is byte-exact. `-CommandFile` now repairs the copy it sends rather
than warning about it, `-ScriptArg` passes values in without anybody running
`sed` over a payload, and `-CommandTimeoutSeconds` bounds a command that would
otherwise be killed by hand. The launcher grew `-LauncherRef auto` and `latest`,
`-LauncherSha256 auto` and a lock file, so a consumer never pastes a commit or a
digest again, and three hosts are tried for the bytes rather than one.

**What broke.** Two consumer-visible changes, both recorded in
[`docs/consumers.md`](docs/consumers.md): stdout from `New -Command` and
`Run -Command` now carries a prefix unless `-NoTimestamps` is passed, and an
explicit `-LauncherRef` now wins over a `wsl-ephemeral.ps1` sitting beside the
launcher.

**The story of each defect this batch found and fixed** is in
[`docs/HISTORY/wsl-toolkit.md`](docs/HISTORY/wsl-toolkit.md), which is where
that kind of text goes from now on rather than onto the live page.

---

## 2026-08-29

### 2026-08-29T15:18:27Z: the four open issues, and six defects found doing them

**Record:** [`TODO/docs.md`](TODO/docs.md), [`TODO/tooling.md`](TODO/tooling.md)
and [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md) carry the twelve entries
this session filed and closed; [`TODO/PROGRESS.md`](TODO/PROGRESS.md) is the
state.
**Deployed:** ⛔ **no deploy.** This repository publishes nothing.

⭐ **What was asked for.** The four issues open against this repository: a list
of feature requests for `wsl-ephemeral.ps1`, the template skeletons still
carrying placeholders, a missing `docs/AGENTS.md`, and the scripts copied from
`Azathothas/TEMPLATE` needing to be iterated on and completed.

⭐ **What the tooling half found that nobody had asked about.** Six defects, and
four of them were reporting success:

- ⛔ **A CI step named `yaml parses` had never parsed a file.** Python's `glob`
  does not descend into a dot-directory and every yaml file here is under
  `.github/`, so it iterated nothing and exited 0 for its whole life. `TOOL-08`.
- ⛔ **`check-remote-items` exited 1 for an item that only needed reading**, so
  the weekly workflow was red for as long as any issue was open, and its two
  modes disagreed about the same tree: text 1, json 0. `TOOL-05`.
- ⛔ **`check-gate.ps1` skipped six checks on the host it exists for**, running
  the `sh` half of every twin and reporting green when no shell was found.
  `TOOL-06`.
- ⛔ **Both halves of `deslop` printed the number of files they had planned to
  remove**, never reading the state back. Fixed while importing them. `TOOL-07`.
- ⛔ **The tree broke its own character rule in 164 places**, in 28 files, every
  one a comment banner in a script, while `check-docs.sh` reported it clean
  because it reads markdown alone. `DOC-02`.
- ⛔ **Seventeen sentences had two homes.** `DOC-03`.

**What changed, by issue.**

- **Issue 1.** `wsl-ephemeral.ps1` gains `-Action Resources`, which reports what
  WSL and the container engine are holding and **prints** the cleanup commands
  without running one, and `-Action HostAddress`, which answers what a distro
  reaches this host at without creating a distro to find out. A launcher,
  `wsl-ephemeral-launcher.ps1`, resolves the script, verifies it, clears the
  download mark and forwards the rest. ⛔ `-PortForward` was refused: it needs
  elevation and leaves a rule on the machine. `WSL-13`, `WSL-14`, `WSL-15`.
- **Issue 2.** `docs/templates/` is gone. The one form this repository authors
  from is `TODO/ENTRY.md`, and `TODO/RULES.md` is written for real. Two checks
  now hold the prose rules that produced the issue. `DOC-02`, `DOC-03`,
  `DOC-04`.
- **Issue 3.** `docs/README.md` becomes `docs/AGENTS.md`, the document an agent
  reads in full. `README.md` takes the map and is written for a person, and the
  root `AGENTS.md` is a door. `DOC-05`.
- **Issue 4.** `check-markers`, `check-one-home`, `deslop` and `fill-license`
  moved here with the `LICENSES/` texts, `check-remote-items` and `check-docs`
  were refreshed from the newer upstream copies, and `mine-repo` deliberately
  stayed where it is. `TOOL-04` through `TOOL-08`.

⚠ **One behaviour change a caller could observe**, recorded in
[`docs/consumers.md`](docs/consumers.md): `wsl-ephemeral.ps1` now writes its
final `ERROR:` line to stderr. Nothing was renamed and no exit code changed
meaning, so it is not a break by that file's definition; it is written down
because it is the only thing in this batch that is visible from outside.

⚠ **A consumer nobody had written down was found again.**
`Azathothas/bit-cli` fetches this script by raw URL from its own
`docs/containers.md`. That is the second time reading one unrelated page has
turned up a consumer the register did not have.

## 2026-08-27

### 2026-08-27T16:30:00Z: the BSD work moved out, and this ledger is closed

**Record:** [`TODO/bsd.md`](TODO/bsd.md) carries the closure and says where each
part went; [`TODO/PROGRESS.md`](TODO/PROGRESS.md) is the empty work order.
**Deployed:** ⛔ **no deploy.** This repository publishes nothing.

⭐ **`pkgforge-dev/docker-bsd` is standalone.** It had been borrowing this
tree's checks, conventions and methodology while it was written. Those are now
copies living there, adapted, so a clone of it reproduces every measurement with
nothing else checked out. ⚠ One reference back remains, pinned, and that
repository records it.

- ⛔ **`BSD-01` and `BSD-02` are marked done here and `BSD-01` is not done.**
  It is open in its new home with its acceptance command unchanged. This
  repository has no "moved" status, and leaving an entry open that nobody here
  will work is worse than a closure that says plainly where the work went.
- ⭐ **What was reached before it moved**, recorded in
  [`TODO/bsd.md`](TODO/bsd.md) rather than only in the other repository: a
  FreeBSD userland on this Windows host's own hypervisor with no nesting, and a
  BSD shell reachable on any host with only a container engine.
- ⚠ **This repository has no open entries.** That is a state, not an
  achievement, and `PROGRESS.md` names five things worth doing while filing none
  of them.

### 2026-08-27T14:30:00Z: a FreeBSD userland runs on this Windows host, with no nesting

**Record:** `BSD-01` in [`TODO/bsd.md`](TODO/bsd.md) carries the result, the
five corrections and the evidence; the experiments are in
`pkgforge-dev/docker-bsd` under `experiments/`, each committed with its result.
⚠ **`BSD-01` stays open**, on its own acceptance command rather than on its
purpose. [`TODO/PROGRESS.md`](TODO/PROGRESS.md) says exactly what is left.
**Deployed:** no deploy. ⛔ This repository publishes nothing. The experiments
were pushed to `pkgforge-dev/docker-bsd`.

⭐ **The result, measured on this machine and not derived from anything.**
`qemu-system-x86_64 -accel whpx -M q35 -cpu Icelake-Server-v7` boots FreeBSD
15.1-RELEASE from the published BASIC-CI image and answers commands on its
serial console, unelevated, with the WSL2 podman machine running throughout.
⛔ **One hypervisor, the host's own. No nesting anywhere**, which is what the
operator's ruling asked for and what the previous plan called the fallback.

- ⭐ **The ruling's order of work was right, and the experiment that FAILED is
  why the one that worked, worked.** smolBSD under WHPX boots a NetBSD kernel
  and never finds its disk. The identical command line under `-accel tcg`
  boots to a shell. That contrast located the cause, and FreeBSD then printed
  it: under WHPX the guest sees the **host's** hypervisor signature,
  `Microsoft Hv`, not QEMU's. NetBSD's paravirtual bus is looking for QEMU,
  does not find it, and never enumerates virtio-mmio. FreeBSD has Hyper-V
  support and carries on.
- ⛔ **The WHPX CPU-model prediction for this machine was wrong.** `BSD-01`
  recorded, explicitly as derived and not measured, that this host's Model 154
  CPU would be handed a newer model and wedge QEMU. Measured on QEMU 11.1.0:
  five models including the two the advice forbids, `host` and `max`, all
  behaved identically and none wedged. ⚠ The sources are not falsified; they
  measured QEMU 9.x on other hardware. The prediction about this host is.
- ⛔ **The untried avenue is now tried and it is closed.** `computecore.dll`
  loads unelevated and every HCS v2 entry point resolves, so reaching the API
  WSL is built on needs no patched service. But `HcsEnumerateComputeSystems`,
  a **read**, returns `0x8037011B`: Hyper-V Administrators only. ⚠ That
  inverts the Approach table's ranking on this host, and not on performance:
  the recommended Hyper-V route needs elevation, and the row it called a
  fallback needs none.
- ⭐ **Four BSD boots, where the previous session could claim none.** NetBSD
  under TCG to a shell in 499 ms of kernel time; FreeBSD under Firecracker on
  the WSL2 nested KVM to a login prompt in **1.8 s**; FreeBSD under WHPX with
  no nesting in **117 s**.

- ⭐ **A container runs inside that guest**, `rc=0`, with `podman info`
  reporting `freebsd/amd64 runtime=ocijail`, so the runtime underneath is jails.
  ⛔ **A long-running `podman system service` panics the guest KERNEL**, in
  `_umtx_op`, which is what Go's scheduler parks threads on. That is the whole
  distance between `BSD-01`'s purpose and its acceptance command, and it is why
  the entry stays open.
- ⚠ **One explanation was published and then withdrawn, in the same session.**
  Under WHPX FreeBSD selects a Hyper-V timecounter and Go binaries die of
  `SIGFPE`; switching to `ACPI-fast` moves `podman run` to `rc=0`. ⛔ **The
  clock is not the cause**: with `ACPI-fast` it measurably works, and the daemon
  still takes the kernel down. The correction is written under the claim rather
  than over it.

⛔ **Three defects in this session's own scripts, each a class this repository
already names, and one of them shipped a false success:**

- an experiment printed ⭐ **"a container ran"** over a `podman run` that had
  exited with an error, because it matched its success marker against the
  guest's **echo of the command line** that mentioned the marker. The tty had
  wrapped the echo, so the filter meant to remove it missed it;
- `curl` and `xz` guarded by `cmd; rc=$?` under `set -e`, where the shell has
  already exited before the guard can run;
- a probe reporting `vmcompute.dll did not load` about a library that had
  loaded and exports 36 functions, because a `try`/`catch` around a P/Invoke
  cannot tell a missing library from a missing entry point.

⭐ **Five rows added to
[`docs/conventions/forbidden-patterns.md`](docs/conventions/forbidden-patterns.md)**,
one per class plus the one that made a security claim false: `-display none`
does not mean no network, and QEMU attaches a default NIC unless given
`-nic none`. An experiment printed `network NONE` while its guest was running
`dhclient`.

⚠ **And one claim in this session's own first write-up was wrong and is
corrected in place**: the 117 s boot was attributed to `growfs`. The console
shows no `growfs`, reports the filesystem `CLEAN; SKIPPING CHECKS`, and a
second boot landed within 0.3 s of the first. The experiment now stamps four
boot phases so the next run reports where the time goes instead of attributing
it.

### 2026-08-27T12:52:00Z: the BSD reference sweep, BSD-02 answered, and an unregistered consumer

⚠ **One entry for one session, covering two commits**, `796e40f` and the
commit that carries this line. They are one unit of work: the sweep, what it
corrected, and what it turned up on the way.


**Record:** [`docs/reference-sweeps/findings.md`](docs/reference-sweeps/findings.md)
carries `R6` to `R28` with the ranking;
[`docs/reference-sweeps/usable.md`](docs/reference-sweeps/usable.md) carries the
commands; `BSD-01` and `BSD-02` in [`TODO/bsd.md`](TODO/bsd.md) carry the
corrections.
**Deployed:** no deploy. ⛔ This repository publishes nothing, and no code
changed.

A sweep of the operator's reference list, every repository reached and none
recorded as gone, with issues, pull requests and discussions pulled in both
states. ⭐ The scope, the counts and what was found are in
[`docs/reference-sweeps/findings.md`](docs/reference-sweeps/findings.md), which
is where nearly everything below came from.

- ⭐ **`BSD-02` closed**, and its premise was wrong by conflating two questions.
  "Runnable" as an OCI container and "runnable" as a booted guest have different
  answers per BSD. Three BSDs have no jail-equivalent runtime, which is what the
  entry meant; all four are runnable as guests, and two have been for years.
- ⛔ **A claim this repository published twice is false as stated.**
  [`docs/reference-sweeps/usable.md`](docs/reference-sweeps/usable.md) said there
  is no counterpart presenting FreeBSD syscalls on a Linux kernel.
  `AkihiroSuda/lsf` is one. It is a 2022 proof of concept that crashes, so the
  conclusion holds and the reasoning had to change. ⭐ It also explains the
  exit code 139: the Linux kernel does not validate an ELF binary's OSABI on
  `execve`.
- ⭐ **The Windows Hypervisor Platform is measurable without elevation.**
  `WHvGetCapability` through `WinHvPlatform.dll` returns `HypervisorPresent` as
  `1` on this machine. That closes a caveat `BSD-01` recorded as unreadable
  because `Get-WindowsOptionalFeature` needs elevation.
- ⛔ **Under WHPX, `-cpu host` and `-cpu max` wedge QEMU**, measured
  independently by two projects, and a named CPU model newer than the host wedges
  it too. This machine's CPU falls on the wrong side of the published rule.
  Derived rather than measured here: no QEMU is installed.
- ⭐ **The highest-value unknown in `BSD-01` is answered.** `x86_64`
  GitHub-hosted runners expose `/dev/kvm` and need a `udev` rule to make it
  writable; arm64 runners do not have it at all. Four references agree.
- ⭐ **Three options `BSD-01`'s table did not contain**, ranked underneath it
  rather than edited into it: a BSD microvm through smolBSD or Firecracker, the
  Host Compute System API driven directly, and `libkrun`.
- ⚠ **`gronke/freebsd-ci` and `no-pictures/freebsd-ci` are the same tree**,
  sharing a commit byte for byte. Recorded so the second is not mined again.
- ⚠ **The operator asked whether `XaeroVincent/FreeBSD-Gaming-Kernel` carries a
  performance fix that applies. It does not**, and the whole repository is 690
  bytes of README over 14 blobs.

⚠ **No BSD was booted and nothing was run.** Every boot time and failure quoted
is somebody else's measurement, attributed where it appears. The four local
measurements are the WHPX capability, the absent tooling, the host CPU identity,
and a re-derivation of the nested KVM facts already on record.


**Record:** [`docs/consumers.md`](docs/consumers.md) carries the consumer row and
the evidence; [`TODO/PROGRESS.md`](TODO/PROGRESS.md) and
[`TODO/SUMMARY.md`](TODO/SUMMARY.md) carry the session.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing. The
`pkgforge-dev/docker-bsd` half shipped as `878e286`, CI green on both jobs.

- ⛔ **A second consumer of `wsl-ephemeral.ps1` was found and it was not in the
  register.** `pkgforge-dev/cross-libc-dlopen` carries a **vendored copy**, 536
  lines against this tree's 1,579, with no pin, no digest and no reference back
  here. ⭐ **It carries both P0s this repository has closed**, verified by
  reading its source: its `-Action New` path warns on a non-zero exit instead of
  propagating it, which is `WSL-01`, and its smoke probe is a here-string passed
  as an argument carrying a double quote, which is `WSL-12` on Windows
  PowerShell 5.1. ⚠ Its `-Action Run` path propagates correctly, which is the
  one-gated-door shape `WSL-01` was filed against.
- ⚠ **Not fixed, and not filed as work.** That repository is read-only to this
  one. It is recorded in the register rather than turned into an entry this
  repository cannot close.
- ⭐ **The register's own closing line is now demonstrated rather than asserted.**
  It says to treat the register as a lower bound. One reading of one unrelated
  repository, for an unrelated reason, found a consumer it did not have.
- **`pkgforge-dev/docker-bsd` prepared** for the session that follows: an
  `experiments/` directory with the layout borrowed from
  `pkgforge-dev/cross-libc-dlopen`, two host probes that were actually run, its
  own ignored `.tmp/`, and a `TOOLKIT.md` naming in three tiers what it must
  carry from here to stand alone, including which scripts **not** to copy.
- ⛔ **Driving those probes on all three hosts found two defects in them**, which
  is the whole reason part (b) of the gate exists. `grep -i microsoft
  /proc/version` answers "not WSL" inside a machine running a custom WSL2 kernel
  that reports `7.2.0-WSL2-STABLE`. And `Add-Type` on Windows PowerShell 5.1
  shells out to `csc.exe`, which reads `LIB` and treats a stale directory there
  as a warning, and `Add-Type` compiles warnings-as-errors, so the whole probe
  failed with a message about the C# compiler and nothing about the hypervisor.

### 2026-08-27T12:35:00Z: git-sync.ps1 bound a gate string to the author identity

**Record:** `TOOL-03` in [`TODO/tooling.md`](TODO/tooling.md), filed and closed
in place.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

⛔ **The one script whose job is enforcing the identity rule invented an
identity and printed `identity verified` one line under it.** Found by using it,
not reported by anything.

- ⛔ **`-File` does not take PowerShell expressions.** `-Gate "a","b","c","d"`
  reaches the child as four separate arguments; the first bound to `-Gate` and
  the rest bound **positionally** onto `-Name`, `-Email` and `-Branch`. It
  committed with an author of `sh scripts/common/check-control-bytes.sh` and
  tried to push a branch named after another gate.
- ⚠ **The push failing is what stopped it reaching a remote.** That is luck, not
  a guard. The commit was reset and remade; nothing published was rewritten.
- ⭐ **The fix is `[CmdletBinding(PositionalBinding = $false)]`**, so a stray
  positional argument does not bind at all and nothing runs.
- ⚠ **`git-sync.sh` does not share the defect**, checked rather than assumed: it
  reads `--gate` in a `case`, where a bare argument is an explicit error.
- ⚠ **`Azathothas/TEMPLATE` carries its own copy of this script and its own copy
  of this defect.** Not fixed there in this session, and named in the entry so
  the next session in that tree has it written down.

### 2026-08-27T12:05:00Z: the door sweep, and the two things it found

**Record:** notes written under `WSL-09` and `WSL-04` in
[`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), each below its closure rather
than edited into it.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

⭐ **Part (c) of the gate, asking which paths reach one operation.** Both
findings are the same shape and it is the one this repository keeps meeting: a
guard on one of several doors.

- ⛔ **The file write was the one unbounded question the script asks.**
  `-OciEnv` and `-Systemd` both write through `Write-DistroFile`, which used
  `Invoke-InDistro` and therefore had no time limit, while the two probes either
  side of it did. A distro that wedged after the smoke probe hung there forever.
- ⛔ **The rollback did not `--terminate` before `--unregister`**, while
  `Remove-EphemeralDistro` did. The rollback was the more exposed of the two,
  and it is the path that runs when something has already gone wrong. ⚠ The
  race is still unreproduced, so this is two paths agreeing, not a measured fix.

⚠ **What the sweep did NOT find, said so it is not read as untested:** no leak.
After a session of deliberate failures, timeouts, space refusals and rejected
switches, the base directory held exactly the eight directories matching the
eight registered distros.

### 2026-08-27T11:45:00Z: an Enter action, so the interactive path is first class

**Record:** `WSL-11` in [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), closed
in place with its evidence.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

⭐ **`-Action Enter -Name eph-...` attaches a shell**, honouring `-User` and
returning the shell's exit code. The summary `New` prints now names it instead
of telling the reader to run `wsl -d ...` by hand.

- ⛔ **It sends no command**, which is the whole feature: with one, everything
  typed goes nowhere. Proven by planting the command back and watching stdin be
  ignored.
- ⛔ **`-TimeoutSeconds` does not apply.** A person in a shell is not a wedged
  init.
- ⚠ **It reaches only distros this script created**, because the name is
  prefix-forced: `-Name podman-machine-default` asks for
  `eph-podman-machine-default` and is refused.
- ⚠ **A human TTY session was not driven**, because this harness has no
  terminal to allocate. What was driven is everything under it: stdin belongs
  to the guest shell, `-User` selects the account, and the exit code comes back.

### 2026-08-27T11:30:00Z: a generated name that collides is drawn again

**Record:** `WSL-10` in [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), closed
in place with its evidence.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

⭐ **A name this script drew and that happens to be taken is drawn again**, up
to 8 times, instead of failing a command that was correct. ⛔ **A name the
caller gave is still refused**, because silently substituting a different one
would be worse than saying no.

- ⚠ The space is 36^4, so the retry is expected to run zero times. That is why
  it is worth having: a path that fires once in a million runs is one nobody
  will debug when it does.

### 2026-08-27T11:15:00Z: a hard bound on the script's own questions

**Record:** `WSL-09` in [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), closed
in place with its evidence.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

⭐ **A distro whose init wedges no longer hangs the script forever.** Every
question `wsl-ephemeral.ps1` asks a distro now has a hard time limit, default
120 seconds, changed with `-TimeoutSeconds`.

- ⛔ **`-Command` is not bounded by it.** A build that runs for an hour is a
  legitimate command; what is bounded is the smoke probe and the `-Systemd`
  check, which are the script's own questions.
- ⭐ **"It never answered" is a different message from "it is not installed"**,
  which is what [`docs/conventions/shell.md`](docs/conventions/shell.md) section
  9 asks for.
- ⚠ **New behaviour for a caller:** `New` can exit 1 where it used to hang. On
  the timeout path the distro is terminated and rolled back.
- ⚠ It was proven against a rootfs whose `/bin/sh` is `exec sleep 900`, not
  against a simulation. Unbounded, the same distro left the script running
  after 75 seconds with no output.

### 2026-08-27T10:55:00Z: optional systemd, and a switch that checks its own effect

**Record:** `WSL-07` in [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), closed
in place with its evidence.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

⭐ **`-Systemd` writes `/etc/wsl.conf`, restarts the distro, and refuses if
systemd did not become PID 1.** Units, timers and `systemctl` are testable in a
throwaway distro now.

- ⛔ **Most OCI base images do not ship systemd**, which is the catch and it is
  measured: `alpine:3.22`, `ubuntu:24.04` and `fedora:41` have no
  `/usr/lib/systemd/systemd`; `almalinux:9` has.
- ⛔ **Writing the flag into an image without it does nothing and says
  nothing.** `ubuntu:24.04` carried on with its own init silently. That is a
  flag no code reads, so the switch verifies `/proc/1/comm` and fails loudly
  instead of handing back a distro the caller believes has systemd.
- ⚠ `wsl: Failed to start the systemd user session for 'root'` appears on a
  WORKING systemd distro. It is not the failure signal.
- ⚠ The restart costs about 11 seconds before the first command answers.

### 2026-08-27T10:35:00Z: a disk-space preflight, against a measured floor

**Record:** `WSL-06` in [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), closed
in place with its evidence.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

⭐ **`New` now refuses an import the volume cannot hold**, before `--import`
runs and before anything is registered. Running out midway left a partial VHDX
and a broken registration.

- ⛔ **The entry's own estimate was wrong and the measurement is in the closure.**
  "Roughly twice the rootfs size" is not the rule: an 8.2 MiB alpine rootfs
  costs **76 MiB** of VHDX and a 76.9 MiB ubuntu one costs 172 MiB. The cost is
  a fixed floor plus a small multiple, so the requirement is `256 MiB + 2x the
  tarball`, set above every measured row rather than fitted to them.
- ⚠ **A volume whose free space cannot be read is imported anyway, and says
  so.** That is a third answer and it is treated as one.
- ⚠ **New behaviour for a caller:** `New` can now exit 1 where it used to start
  an import. Nothing renamed, no exit code changed meaning.

### 2026-08-27T10:13:41Z: a command channel that survives both shells

**Record:** `WSL-08` in [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), closed
in place with its evidence.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing. The
`Azathothas/TEMPLATE` pin has NOT moved for this change yet.

⭐ **`wsl-ephemeral.ps1` now carries a command as base64 and sources it inside
the distro**, so a quote, a dollar sign, a backtick or a tab arrives byte-exact.
`echo $PATH` works; it used to die with ``syntax error: unexpected "("``.

- **New parameters, both additive:** `-CommandFile` reads a file on this machine
  verbatim, so a multi-line script works; `-CommandB64` takes the command as
  base64. `-Command` stays and is unchanged in spelling. The three are mutually
  exclusive and passing two is refused.
- ⛔ **All three payloads the script sends now go through one function**, which
  **asserts** the transport stays inside the measured alphabet. Two of the three
  used to be hand-written inside that alphabet with nothing enforcing it, which
  is how `WSL-12` shipped.
- ⚠ **One observable difference for an existing caller**, and it is a fix rather
  than a break: `$VAR` in a `-Command` is now expanded by the guest's login
  shell instead of in transit, so with `-OciEnv` it expands to the image's
  value. Nothing renamed, no exit code changed meaning.
- ⚠ **What is still not possible, and it is not this script:** Windows
  PowerShell 5.1 drops a double quote when it builds a child process's argument
  list, so a `-Command` value loses it before this script runs. `-CommandB64` is
  immune and is the documented answer.
- ⭐ **The smoke probe carries `WSL-12`'s exact line again**, brackets and double
  quotes included, and `-Action New` works on 5.1. It is the first thing that
  fails if the channel ever breaks again.

### 2026-08-27T09:20:00Z: the door sweep found New broken on PowerShell 5.1

**Record:** `WSL-12` in [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md), filed
and closed in place.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing. The
`Azathothas/TEMPLATE` pin DID move, which is the closest thing to one.

⛔ **`-Action New` was failing outright under Windows PowerShell 5.1**, on a
host `.NOTES` claimed to be tested on. The smoke probe carried a bracket inside
a double-quoted `echo`, the quoting did not survive `wsl.exe`, and every run
reported `Distro imported but /bin/sh did not run` and rolled back.

- Nobody reported it. It came out of part (c) of the gate, the door sweep, run
  against the `WSL-01` to `WSL-05` batch.
- ⚠ **It is host-specific**, which is why it survived: under PowerShell 7.6.5
  the probe runs, and every measurement in this repository until now had been
  taken with `pwsh`.
- The same sweep found a fourth deletion path that did not go through the one
  helper, while `wsl-ephemeral.md` claimed there was one and every path reached
  it. The claim was false for one commit and is true again.
- The claim audit corrected two more published sentences: a limits row saying
  the caller owns `-Command` quoting, which measurement disproved, and a
  pin-state cell and an entry closure that pointed at each other and stated
  nothing.

⭐ **The `Azathothas/TEMPLATE` pin moved** to the head of this batch.
[`docs/consumers.md`](docs/consumers.md) says why: leaving a 5.1 caller pinned
to the old commit protects them from the fix rather than from the break.


### 2026-08-27T08:45:00Z: one command for the gate, a record writer, and a binfmt check

**Record:** `TOOL-02`, `TOOL-01` and `DOC-01` in
[`TODO/tooling.md`](TODO/tooling.md), each closed in place with its evidence.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

Three tools, and each removes something a session was doing by hand.

- `scripts/common/check-gate.sh` and its twin run every local gate in one
  command. ⛔ Not a second set of rules: every line delegates to a check that
  already exists. `--fast` skips `check-twins` alone, measured at 41s against
  208s for the full run.
- `scripts/common/check-powershell.ps1` holds the two PowerShell assertions CI
  had inline. ⚠ **PSScriptAnalyzer is optional and saying so is the point**: a
  machine without it reports SKIPPED and exits 0, and CI installs it and then
  asserts it was not skipped.
- `scripts/common/set-record.mjs` moves an entry's status and re-derives every
  count. It does not run the reader and report green.
- `scripts/common/check-binfmt.sh` and its twin read the kernel rather than a
  unit's exit code. ⭐ **No `podman machine ssh`**, so nothing is written into
  the directory it runs from.

⭐ **The gate found three defects, two of them its own**, and they are written
into `TOOL-02`: an infinite recursion with `check-twins` that left twenty stray
shells holding their own files open; a skipped analyzer reported as a passed
check, which is the forbidden pattern its own header cites; and a `.ps1`
shipped with no UTF-8 BOM, caught by the analyzer it had just wired in.


### 2026-08-27T07:32:10Z: `New -Command` propagates the inner exit code

**Record:** `WSL-01` in [`TODO/wsl-ephemeral.md`](TODO/wsl-ephemeral.md),
closed in place with its evidence.
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

⛔ **This is a breaking change and it is the point of the change.**
`wsl-ephemeral.ps1 -Action New -Command` used to print a warning over a failing
command and exit 0. It now exits with the command's own code, the same way
`-Action Run` always has.

- **Who it breaks.** Anything reading `New -Command` as a gate and getting a
  pass because it could not fail. The failure such a caller now sees is real,
  and it was there before: the step was reporting green over it.
- **Who was checked.** `Azathothas/TEMPLATE`, the only consumer in the
  register. Its wrapper forwards arguments and propagates the inner code, so it
  needs no edit beyond its pin.
  [`docs/consumers.md`](docs/consumers.md) carries the pin state.
- ⚠ **A pinned consumer does not get this until its pin moves**, which is a
  separate change in that repository.

The asymmetry is gone structurally rather than by convention: both actions run
the caller's command through one function, so there is no second place for a
code to be dropped.


### 2026-08-27T06:24:28Z: the BSD research, and the record moved into TODO

**Record:** [`TODO/PROGRESS.md`](TODO/PROGRESS.md).
**Deployed:** no deploy from here. ⛔ This repository publishes nothing.

- The record follows the todo model's own shape now: `TODO/` holding
  `PROGRESS.md`, `INDEX.md` and the entries by category, rather than two files
  at the root with every entry inlined into the index.
- `check-record` and its twin assert that the counts agree with the rows, in
  **both** the index and the record. ⚠ The record half was a gap a review found
  after the check had already reported clean once over exactly that drift.
- ⭐ The `BSD-01` research produced a shape different from the one requested,
  and the images half moved out entirely: `pkgforge-dev/docker-bsd` now builds
  FreeBSD, NetBSD, OpenBSD and DragonFly for amd64. Only FreeBSD publishes OCI
  images upstream, so three of the four are new.
- What is left here is one scripted VM guest, ranked against every alternative
  on friction, performance and interop in [`TODO/bsd.md`](TODO/bsd.md).


### 2026-08-27T04:58:19Z: repository created, and `wsl-ephemeral.ps1` decoupled from the template

**Record:** [`TODO/PROGRESS.md`](TODO/PROGRESS.md), the bootstrap entry.
**Deployed:** no deploy. This repository publishes files, not a service.

`Azathothas/ToolKit` was bootstrapped from `Azathothas/TEMPLATE` so that tools
used across many projects have one home, and so the template keeps only what
every project needs.

- `scripts/powershell-windows/wsl-ephemeral.ps1` moved here byte-for-byte from
  the template. ⛔ **No behaviour changed in the move**, so the diff is
  reviewable as a move. Its eleven known findings came with it, unfixed, and are
  tracked as `WSL-01` through `WSL-11` in [`TODO/INDEX.md`](TODO/INDEX.md).
- `scripts/powershell-windows/wsl-ephemeral.md` written, including an explicit
  list of those limits. A limit hidden is a defect filed against the user later.
- ⚠ **`Azathothas/TEMPLATE` now carries a wrapper at the same path**, which
  fetches this copy by pinned commit and verifies a SHA-256 before executing.
  Callers of the old path keep working. [`docs/consumers.md`](docs/consumers.md)
  is the register.
