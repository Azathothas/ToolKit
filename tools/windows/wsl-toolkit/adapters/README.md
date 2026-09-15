# Adapters

An adapter is one piece of software a base runs for its agents. A configuration
names it in `base.adapters`, `base ensure` installs it after provisioning, and
`base status --probe` reads it back. [`../wsl-toolkit.md`](../wsl-toolkit.md) is
where an operator reads what each one does; this page is the contract for writing
one.

| adapter | what it installs | driven on |
| --- | --- | --- |
| [`herdr/`](herdr/) | herdr, its server as a system unit, a tracked configuration, and an SSH door for the Windows herdr client | the `arch` preset, with systemd |
| [`muse/`](muse/) | Muse Code for the base's account, from Meta's own installer while its digest is approved, `/usr/local/bin/muse`, a `muse.exe` launcher on this machine, and ⭐ a herdr reporter for its lifecycle | the `arch` preset |
| [`pi/`](pi/) | the Pi coding agent from npm with `--ignore-scripts`, and **herdr's own** pi integration | ⚠ not yet driven |
| [`omp/`](omp/) | Oh My Pi from npm, **herdr's own** omp integration, and a refusal when it and pi resolve to one extension directory | ⚠ not yet driven |

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

⚠ **An npm adapter pins differently and the difference is the route, not the
standard.** `pi` and `omp` are npm packages, and npm checks a package against the
registry's own integrity value; a digest written here would duplicate that check
and then go stale against it. `version` alone pins one for an operator who wants
that, and `TK_ADAPTER_PACKAGE` names a different package for a fork.

⭐ **No adapter's summary carries a version number.** A number in a Go string is a
second home for a fact the probe reads back from the machine, and it is the one
nobody updates.

⭐ **`pi` and `omp` are the next two, and both now have entries rather than a
sentence.** `WSL-88` and `WSL-89` in
[`../../../../TODO/wsl-toolkit-go.md`](../../../../TODO/wsl-toolkit-go.md) carry
what each installs, costed from a reference sweep on 2026-09-15: both are npm
packages installed with `--ignore-scripts` and **neither needs a piped installer**,
both already have official herdr integrations, and both give herdr **lifecycle
authority** rather than the screen detection Muse gets. ⛔ `WSL-89` also carries
the one trap: herdr **refuses** the omp integration when pi and omp resolve to the
same extension directory, and `PI_CODING_AGENT_DIR` is read by both.

---

## The files

One directory per adapter, named as the configuration names it.

| file | runs as | contract |
| --- | --- | --- |
| `install.sh` | root, in the base | ⛔ looks before it changes anything, so a second run changes nothing. Its last line is `adapter-complete NAME`, and a zero exit without it is a failure |
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

## The pins

⛔ **A download is pinned by version and verified by digest before use.** The digest
sits on a line whose variable name carries `PINNED_SHA256`, which is the shape the
gate's `secrets` rule accepts, and a file that does not match is deleted without
being run.

⛔ **A provider's own installer, which no upstream pins, runs only while its digest
is approved:** the one the adapter pins, which the operator approved, or the
`installer_sha256` its configuration carries. Any other is saved for
the operator to read, never run, and the adapter exits non-zero naming its digest.

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
