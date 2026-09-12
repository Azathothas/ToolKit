# Common provider-base setup

⭐ **The scripts this page used to carry now live at the top of the tree**, in
[`../../../../../scripts/common/`](../../../../../scripts/common/):
[`bootstrap.sh`](../../../../../scripts/common/bootstrap.sh) and
[`tmux.conf`](../../../../../scripts/common/tmux.conf).

⚠ **They moved because neither is about this tool.** `bootstrap.sh` brings any
Unix userland up to a named tool set, on twelve package managers including the
three BSD ones, and `tmux.conf` is a multiplexer policy. Keeping them under one
tool's `examples/` directory meant anybody who wanted either had to know that
this provider example existed. [`../../../../../scripts/README.md`](../../../../../scripts/README.md)
is their contract.

This page is what remains: how those two are used from a provider base, and
nothing that belongs to them.

---

## What the base gives you, and what the bootstrap adds

`base.toolset: "developer"` provisions the DISTRIBUTION as root during
`base ensure`: Bash, a C/C++ build chain, curl, git, jq, Node/npm, OpenSSH,
ripgrep, tmux, unzip, and the base's own rootless Podman.

⚠ **The bootstrap is the other half and it is not the same half.** It runs as the
ordinary account, it can add the language toolchains and CodeGraph that no base
preset installs, and it writes into that account's home rather than into the
image. Run it from the granted checkout:

```sh
sh /workspaces/project/.wsl-toolkit/common/bootstrap.sh --toolset agent
```

`--toolset agent` is developer plus `fd`, and plus the languages this operator
uses: Rust and cargo, Go, Nim, Python, PowerShell. It also installs CodeGraph at
the version the npm registry currently calls latest, and prints that version and
its digest so a later run can pin them:

```sh
sh bootstrap.sh --toolset agent --codegraph 1.6.0 --expect-integrity sha512-...
```

⛔ **Do not run it as root.** Provider state, authentication and CodeGraph belong
to the unprivileged account whose home persists with the named base.

---

## The durable session

`bootstrap.sh` installs [`tmux.conf`](../../../../../scripts/common/tmux.conf) as
`~/.tmux.conf` when it can find it beside itself. Start or reattach with:

```sh
tmux new-session -A -s provider
```

⭐ **The keys are on the status line**, so none of them has to be remembered:
detach with `prefix d`, list sessions with `M-s` and no prefix, and kill the
session with `prefix K`, which asks first. Both `M-g` and `C-b` are the prefix.

⚠ **An accidental `exit` leaves the pane in place** rather than closing it, and
the ordinary kill-pane and kill-window keys are unbound. `prefix R` respawns a
pane whose command has ended.

---

[`access-profiles.md`](access-profiles.md) carries the one-checkout and
zero-grant configurations, including what each one does and does not isolate.
