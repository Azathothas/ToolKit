# Common provider-base setup

Use this after a project profile selects `base.toolset: "developer"`. That
toolset installs Bash, a C/C++ build chain, Git, curl, jq, Node/npm, SSH,
ripgrep, tmux, unzip, and the base's existing rootless Podman.

Run the bootstrap as the ordinary base account from the granted checkout:

```sh
sh .wsl-toolkit/common/bootstrap.sh
```

It installs CodeGraph 1.5.0 below `~/.local`, after verifying the npm tarballs
for the main package and the selected Linux architecture against SHA-512 values
recorded in the script. It fetches no provider CLI. It also installs
[`tmux.conf`](tmux.conf) as `~/.tmux.conf`.

Start or reattach a durable session with:

```sh
tmux new-session -A -s provider
```

Detach with `Ctrl-b d`. An accidental `exit` leaves the pane in place. The
ordinary kill-pane and kill-window keys are unbound; `Ctrl-b K` intentionally
kills the session after confirmation.

⛔ Do not run the bootstrap as root. Provider state, authentication and
CodeGraph belong to the unprivileged account whose home persists with the named
base.

[`access-profiles.md`](access-profiles.md) carries the one-checkout and
zero-grant configurations, including what each one does and does not isolate.
