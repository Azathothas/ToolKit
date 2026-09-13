# Review passes

Where the deep reviews over a change are recorded, one pass per lens, each
pass obliged to FIND something or verify something, never to summarise. The
method and the lenses live in
[`methodology/reviews.md`](methodology/reviews.md); this page is the record.

⛔ A pass with no finding and no verification is not a pass. A pass that only
says "looks fine" is a reader, not a reviewer.

---

## 2026-09-13: wsl-toolkit implements the compatibility interface natively

The change: `tools/windows/wsl-toolkit` no longer embeds or launches
`wsl-toolkit.ps1`; `internal/compat` implements the same command line in Go.
Five passes, each through one lens, fixes folded in per pass.

### Pass 1 - re-derivation

Every guard decision in the port was re-derived against the PowerShell source,
not against the port's own comments. Five real divergences were found and
fixed:

1. **Member checks compared case-sensitively.** The script's `-contains` and
   `-notcontains` are case-insensitive; the port's exact `==` would treat a
   distro WSL reports as `EPH-X` as absent from a list holding `eph-x`, and the
   collision and ownership checks would take the wrong branch.
   `containsString` now compares with `EqualFold`.
2. **The containment guard had lost its boundary.** `GetFullPath` in the script
   KEEPS a trailing separator; Go's `filepath.Clean` DROPS one, so a prefix
   test against the cleaned base directory accepts a sibling whose name merely
   starts with the same characters (`baseDirEVIL`). The guard now appends the
   separator after cleaning and requires it in the prefix.
3. **The `.wslconfig` value did not stop at whitespace.** The script's regex
   captured `[^\s#;]+`; the port took the rest of the line, so
   `networkingMode=nat mirrored` answered with a mode WSL never configured.
   The value now ends at the first space, tab, `#` or `;`.
4. **The rollback path skipped the removal guard.** The script's rollback calls
   `Assert-Removable` before unregistering; the port went straight to
   `--unregister`. A second removal path without the guard is exactly how the
   prefix rule and the protected list stop being applied.
5. **The OCI-env script dropped malformed entries silently.** The script warns
   on a malformed `Env` entry and on a name that is not a shell identifier;
   the port `continue`d without a word. Silence reads as carried.

Also re-derived and fixed: the dry-run plan line is now built by
`nativeArgumentString`, the port of the script's argument join, instead of a
hand concatenation, so the plan cannot describe a command line the run would
not produce.

### Pass 2 - claim-scope

Every claim written about the change was checked against what the code does.

1. **The release could publish two versions under one tag.** The tag is formed
   from the script's `$script:ToolkitVersion` and the executable now declares
   its own `Version`; the workflow refused a mismatch only AFTER the
   irreversible tag push. `release.ps1` now reads the executable's version
   file (a text read, so the no-Go-toolchain property holds) and refuses the
   tag while the two disagree.
2. **A dead type carried a false claim.** `orderedRecord` maintained a field
   order the JSON encoder never used, with a comment admitting it was
   advisory. Deleted; the schema-version comment is the honest one.
3. **A dead flag carried a false claim.** `console.color` existed but was
   never set, so the action report's colour rule existed only as a field.
   Wired at the one construction path, or gone.
4. **The `-Name=value` claim was an overclaim.** The page said the binder
   accepts what the script's host accepted and named `-Name=value`; the host's
   documented forms are `-Name value` and `-Name:value`. The claim now names
   what is provable and says the `=` form is the binder's own superset.

Verified in scope: the exit-code table in the package doc, the count claims in
the acceptance runner's page (72 full / 70 quick, matching the asserted
numbers), and the CHANGELOG's no-deploy line.

### Pass 3 - consistency-router

Every path that reaches a decision was walked to see whether it goes through
one router.

1. **The session had two construction paths.** `run` built the console from
   the same two writers the session holds; the tests built structs by hand.
   `newSession` is now the one construction path, and the console derives
   from the session's writers, so a second spelling cannot disagree.
2. **The version had a second router.** `toolkit.ScriptVersion` was a hook
   that existed so toolkit could read a version out of an embedded script
   without an import cycle. The version lives IN toolkit now; the hook is
   deleted and the readers read the constant.
3. **The snapshots directory had two spellings.** `snapshots()` joined its own
   path; `snapshotDir()` is the one resolver and every reader goes through it.
4. **Existence checks had two spellings.** `removeEphemeralDistro` used a raw
   `os.Lstat` beside the `pathExists` helper every other caller uses.

Verified in scope: every wsl.exe call resolves through `resolveWsl`, every
bounded wait through `boundedCapture`, every refusal through the one
`ERROR:` line, and every version read through `toolkit.Version`.

### Pass 4 - security-adversarial

1. **An unknown stdin was treated as interactive.** `interactive()` fell
   through to `true` for any reader that was not an `*os.File`, so a piped
   caller was PRINTED A DELETION PROMPT instead of being told to pass
   `-Force`. Found by the new adversarial case, which drives Purge with a
   string reader. The default is now NO: only this process's own
   character-device stdin counts as somebody to ask.
2. **Hostile names were traced end to end.** `../..`, `.`, `/etc/passwd`,
   `podman-machine-default`, `CON`, and traversal behind a plausible prefix,
   through `Remove`, `Run` and `New`: each either refuses or resolves to a
   strict child of the state directory, and the dry-run plan carries no
   traversal bytes.
3. **Snapshot tags joined to paths are validated first**, including the
   reserved device names by whole tag, and the validated tag lands under the
   snapshots subdirectory and nowhere else.
4. **A redaction pattern that does not compile is refused at the settings**,
   and the replacement marker carries no `$`-expansion, so the guest's text
   cannot paste itself back through the substitution.

### Pass 5 - tests-CI

1. **One test answered differently depending on how full the host volume
   was.** The smoke-probe case drove `New` far enough to reach the space
   preflight, and on a small volume the preflight's refusal fired before the
   probe. The free-space measurement is now an injected hook and the test
   holds the volume still; found by running the repository's own `check go`
   gate, which failed while the test passed on a bigger volume.
2. **Three guards were mutation-proved:** the containment separator boundary,
   the binder's duplicate-scalar refusal, and the transport alphabet check.
   The alphabet check's honest mutation is planting a character in the
   skeleton (the WSL-12 shape), which several tests catch; neutering the call
   site is not input-reachable and was recorded as such rather than claimed
   as killed.
3. **Both Windows release targets cross-compile** (`windows/amd64`,
   `windows/arm64`, CGO off, `-trimpath`), and the repository gates run clean:
   `go`, `docs`, `changelog`, `markers`, `control-bytes`, `placeholders`,
   `one-home`.
