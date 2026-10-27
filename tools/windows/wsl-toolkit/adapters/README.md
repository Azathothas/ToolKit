# Adapters

An adapter is one piece of software a base runs for its agents. A configuration
names it in `base.adapters`, `base ensure` installs it after provisioning, and
`base status --probe` reads it back. [`../wsl-toolkit.md`](../wsl-toolkit.md) is
where an operator reads what each one does; this page is the contract for writing
one.

| adapter | what it installs | driven on |
| --- | --- | --- |
| [`herdr/`](herdr/) | herdr, its server as a system unit, a tracked configuration, and an SSH door for the Windows herdr client. ⭐ With `private_net`, a network namespace for that server and its agents | the `arch` preset, with systemd |
| [`muse/`](muse/) | Muse Code for the base's account, from Meta's own installer while its digest is approved, `/usr/local/bin/muse`, a `muse.exe` launcher on this machine, ⭐ a herdr reporter for its lifecycle, and herdr's rules for its screen | the `arch` preset |
| [`pi/`](pi/) | the Pi coding agent from pi's own release while its digest is pinned, npm with `--ignore-scripts` only when that cannot be fetched, and **herdr's own** pi integration | the `arch` preset |
| [`omp/`](omp/) | Oh My Pi from omp's own release while its digest is pinned, npm only when that cannot be fetched, **herdr's own** omp integration, and a refusal when it and pi resolve to one extension directory | the `arch` preset |

⭐ **All four are driven**, on one base with herdr: `WSL-88` and `WSL-89` carry
the runs for `pi` and `omp`. ⭐ **The muse adapter's herdr reporter is driven**
against herdr 0.9.0 and Muse Code 1.3.0, through Muse's credential-free `--provider
echo`: `SessionStart`, `UserPromptSubmit`, `Stop`, `SessionEnd` with its release, and
the adoption of a resumed session. ⚠ `PreToolUse` and `PermissionRequest` are
registered and not yet driven, because the echo provider calls no tool.

⭐ **The muse adapter also writes herdr's own rules for Muse's screen**, at the local
path herdr reads before its bundled and remote manifests. They match Muse Code
1.4.13's idle screen, which herdr's own rules did not, and the reporter stays as the
fallback. `WSL-158`

⭐ **herdr's `private_net` adds three files and changes none.** A drop-in beside the
server's unit, a unit that links the namespace, and the script it runs. Each is a
file in [`herdr/`](herdr/), and `install.sh` removes all three when the key goes.
`WSL-159`

⭐ **The muse probe drives `SessionStart` itself**, with one echo session per `base
ensure` and none on a read. A hook Muse stops running is a note naming its version. A
Muse that cannot run the session leaves it unmeasured. Measured with herdr 0.9.5 and
Muse Code 1.4.11. `WSL-157`

⭐ **`base ensure` installs every adapter it is given, herdr first and the rest in
order, and one that fails does not stop the rest.** muse, pi and omp each set herdr up
only where herdr is already installed. `WSL-158` The ensure then answers one error naming each adapter that
did not finish. ⛔ **Each agent probe asks first whether its agent is in the base at
all.** An agent that is not there answers one problem, `NAME is not installed in this
base`. The checks that only mean something for an installed agent do not run.
`WSL-107`

⭐ **A probe run with `TK_PROBE_FOOTPRINT=1` answers two facts and stops**:
`installed`, and `footprint`, the bytes its files take in the base. `base status
--probe` asks it of each adapter the configuration no longer names, because the base
keeps that adapter's files until `base recreate`. `WSL-109`

## ⭐ How a version moves without an edit to this tree

An adapter that downloads a release pins it **by version and by digest**, and a
configuration may move it to another without editing a script or rebuilding the
executable:

```json
{ "name": "herdr", "version": "0.10.1",
  "sha256": { "x86_64": "…", "aarch64": "…" } }
```

⛔ **A version with no digest is refused, and digests with no version are refused
too.** Taking the version from a configuration and the digest from the script
would download one release and check it against another's; and a digest nothing
reads is how an operator believes they pinned a thing they did not. They move
together or neither moves. An architecture the configuration does not name is
refused rather than falling back.

⭐ **`pi` and `omp` pin their own releases the same way.** Each pins a version and a
digest per architecture, and the same two fields move it. Their npm fallback takes
that version too, so the two routes install one release. `WSL-138`

⭐ **No adapter's summary carries a version number.** A number in a Go string is a
second home for a fact the probe reads back from the machine, and it is the one
nobody updates.

⭐ **`pi` and `omp` come from each provider's own release, and neither needs a piped
installer.** Each saves the release's file, prints its length and SHA-256, and
installs it only while the digest is pinned. npm, with `--ignore-scripts`, is taken
only when the release cannot be fetched, and never over a changed digest. Both have
official herdr integrations, and both give herdr **lifecycle authority** rather than
the screen detection Muse gets.
`WSL-88` and `WSL-89` in
[`../../../../TODO/wsl-toolkit-go.md`](../../../../TODO/wsl-toolkit-go.md) carry
what each installs. ⛔ `WSL-89` also carries the one trap: herdr **refuses** the
omp integration when pi and omp resolve to the same extension directory, and both
read `PI_CODING_AGENT_DIR`.

---

## ⛔ An agent has to be on the PATH a PANE has

⭐ **This is the one an adapter gets wrong.**

herdr starts an agent by running its canonical name in a pane, and that pane is a
**login shell**. On a base this tool builds, that PATH is:

```text
/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:...
```

⛔ **`$HOME/.local/bin` is NOT on it**, and `/etc/profile` appends `/usr/local/bin`
and nothing else. So an adapter that installs through `npm -g` into the account's
prefix produces an agent that works from `base exec`, works from the adapter's own
script, and **cannot be started by herdr at all**: the pane answers `command not
found`, and `herdr agent start` waits for an agent that cannot appear. `WSL-88`

**So an adapter that names an `Agent` owes two things:**

1. ⭐ **A root-owned wrapper at `/usr/local/bin/NAME`** that refuses any account but
   the configured one and execs the real binary.
2. ⛔ **A probe line that reads the name back on a LOGIN shell.** It asks through
   `runuser -l`, not on the PATH the adapter set up. It is the line the three agent
   probes run:

   ```sh
   pane_path=$(runuser -l "$TK_USER" -c "command -v NAME" 2>/dev/null | tr -d '\r' | head -1)
   ```

   ⛔ **Not `as_account sh -lc`, which passes an npm adapter with no wrapper.** In
   an npm adapter, `as_account` hands the shell a PATH with the account's npm prefix
   first. On the `arch` preset, `/etc/profile` appends to that PATH, so the prefix
   stays first. `command -v` then finds npm's own binary before the wrapper. The
   muse adapter's `as_account` has no prefix, and its PATH is still not a pane's.
   `runuser -l` starts from the account's own login environment, which is the PATH a
   pane has. `WSL-145`

   ⚠ A probe that reads the name on the adapter's own curated PATH passes over an
   agent herdr cannot start. The probe asks the question herdr asks.

⛔ **AND IT HAS TO RESOLVE TO THE WRAPPER, NOT MERELY RESOLVE.** An earlier
`bootstrap.sh` wrote `export PATH="$HOME/.local/bin:$PATH"` into the account's
profile, so after `base bootstrap` a login shell found the vendor's own launcher
first, and the wrapper on the system path was not reached. It now puts the prefix
last and rewrites that line. ⭐ **So an agent adapter's probe reads the resolved file
and refuses one that does not carry this tool's marker**, and it reads it through
`runuser -l`. Its problem names `base bootstrap` as the remedy. ⚠ `as_account` has a
curated PATH, so a probe that asks through it cannot see this.

## The files


One directory per adapter, named as the configuration names it.

| file | runs as | contract |
| --- | --- | --- |
| `install.sh` | root, in the base | ⛔ looks before it changes anything, so a second run changes nothing. Its last line is `adapter-complete NAME`, and a zero exit without it is a failure. ⛔ It writes in the account's home only as the account, as the next section says |
| `probe.sh` | root, in the base | prints one fact per line as `name value`, a `version` line, and `problem TEXT` for each thing wrong, and exits 0 whatever it finds |
| any other file | nobody | reaches `install.sh` base64-encoded in `TK_FILE_<NAME>_B64`, where NAME is the file name upper-cased with each other character run turned into `_` |

Both scripts receive `TK_USER`, the base's account, and `TK_DISTRO`, its
distribution, as exported variables. An adapter with a half on this machine
receives what that half prepares, which for herdr is `TK_SSH_CLIENT_KEY`. An adapter
that runs a provider's installer receives `TK_INSTALLER_SHA256` when its
configuration entry carries `installer_sha256`, and every other adapter refuses that
key.

⛔ **Nothing is substituted into a script.** A value that becomes shell source is a
value whose quote becomes code.

## ⛔ Root writes nothing in the account's home

`install.sh` runs as root, and the account can place a link at any path in its home.
`chown`, `install -d`, `cp` and a redirect each follow that link. With
`passwordless_sudo` off, or in a herdr pane under `private_net`, the account cannot act
as root. So root's write through the link was one the account could not make. `WSL-201`

⭐ **So every write in the home is the account's**, through the three helpers each
installer carries between its `the account's own writes` markers:

| helper | what it does |
| --- | --- |
| `as_account CMD...` | runs one command as the account, with a fresh environment |
| `account_dirs MODE DIR...` | makes each directory as the account and gives it MODE. A directory that is a link keeps its own mode |
| `account_put DEST MODE` | writes its standard input to a file of the account's own beside DEST, and renames it over DEST. A link at DEST is replaced, never written through |

⭐ Root builds what goes into a file outside the home, in its own temporary file, and
hands it to `account_put` on standard input. Root may still compare a file in the home
with one of its own, because a comparison writes nothing.

⛔ **The suite refuses a new root write in the home.**
`TestNoRootWriteLandsInTheAccountsHome` reads every script the executable carries. It
refuses a `chown`, `install`, `mkdir`, `mv` or `cp` onto a path in the home, and a
redirect onto one, outside `as_account`. A case per adapter plants a link at each path
it writes and finds the link's target untouched.

## The pins

⛔ **A download is pinned by version and verified by digest before use.** The digest
sits on a line whose variable name carries `PINNED_SHA256`, which is the shape the
gate's `secrets` rule accepts, and a file that does not match is deleted without
being run.

⛔ **A provider's own installer, which no upstream pins, runs only while its digest
is approved:** the one the adapter pins, which the operator approved, or the
`installer_sha256` its configuration carries. Any other is saved for
the operator to read, never run, and the adapter exits non-zero naming its digest.

⛔ **A provider's own release asset is kept the same way when its digest is not the
pinned one.** `pi` and `omp` save it, exit 2, and name the release's own checksum
list. Neither falls back to npm over a changed digest. `WSL-138`

⭐ **A release asset is pinned by whoever reads the refusal.** Where the
saved file agrees with the release's own list, the `version` and `sha256` the refusal
prints pin it. Where they disagree, nothing is pinned, and the file is not the
provider's. ⚠ Muse's installer has no such list, so its approval stays as above.

## The copy the executable carries

⛔ **These files have one home, here, and a generated copy** under
[`../internal/toolkit/adapters/`](../internal/toolkit/adapters/), because go:embed
cannot reach out of the package that runs them. The gate's `adapters` rule refuses
a copy that differs, one that is missing and one with no definition. After an
edit here:

```sh
sh scripts/common/check.sh adapters --fix
```

[`../../../../TODO/RULES.md`](../../../../TODO/RULES.md) section 4 lists every
generated file in this tree.

## Adding one

1. **Drive it by hand on a throwaway instance first**, and record what the
   upstream documentation said that did not hold.
2. Write the three kinds of file above, and the entry in `adapterSpecs` in
   [`../internal/toolkit/adapters.go`](../internal/toolkit/adapters.go): its
   summary, whether it needs systemd, and the presets it was driven on. ⛔ A base
   from another preset is refused rather than guessed at. An agent adapter also
   names its command as `Agent`, which gives it `base agent` and a launcher on this
   machine.
3. Run the fix above, then `base ensure` and `base status --probe` on the
   throwaway instance, and read every fact back.
