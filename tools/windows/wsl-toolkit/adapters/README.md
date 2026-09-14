# Adapters

An adapter is one piece of software a base runs for its agents. A configuration
names it in `base.adapters`, `base ensure` installs it after provisioning, and
`base status --probe` reads it back. [`../wsl-toolkit.md`](../wsl-toolkit.md) is
where an operator reads what each one does; this page is the contract for writing
one.

| adapter | what it installs | driven on |
| --- | --- | --- |
| [`herdr/`](herdr/) | herdr 0.9.0, its server as a system unit, a tracked configuration, and an SSH door for the Windows herdr client | the `arch` preset, with systemd |
| [`muse/`](muse/) | Muse Code for the base's account, from Meta's own installer while its digest is approved, and `/usr/local/bin/muse` | the `arch` preset |

`pi` and `omp` are the next adapters named, and neither is built.

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
   from another preset is refused rather than guessed at.
3. Run the fix above, then `base ensure` and `base status --probe` on the
   throwaway instance, and read every fact back.
