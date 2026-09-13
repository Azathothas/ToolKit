# Muse Code in one named WSL base

This is the concrete provider example for the one-checkout profile in
[`../common/access-profiles.md`](../common/access-profiles.md). It gives Muse a
persistent Linux home, systemd, rootless Podman, developer tools, CodeGraph and
Zellij, with exactly one Windows checkout mounted for the base account. The
trusted Muse agent has passwordless sudo, so a package install or a system
change does not stop and wait for a password nobody is there to type.

## Prepare the checkout on Windows

1. Copy [`../../../../../scripts/common/bootstrap.sh`](../../../../../scripts/common/bootstrap.sh)
   into the target checkout as `.wsl-toolkit/common/bootstrap.sh`.
2. Save the **One read/write checkout** JSON from
   [`access-profiles.md`](../common/access-profiles.md) as
   `wsl-toolkit.json` at that checkout's root.
3. Validate before creating anything:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json config validate
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json config
```

The second command must print `passwordless sudo true` and one `grant` row whose
Windows source is the exact target checkout and whose guest target is
`/workspaces/project`.

Create or reconcile the named base, then make it prove its live state:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base ensure
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base status --probe --json
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base shell
```

`base shell` starts as `muse`; `sudo` does not prompt. `base shell --root`
remains the recovery path if that configured account is ever unhealthy.

## Bootstrap the agent and durable session

Install the measured Zellij package explicitly, then run the shared agent
bootstrap as the configured account. The bootstrap installs CodeGraph and skips
the tmux-specific configuration because Zellij owns this provider workflow:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec --root -c 'pacman -Syu --noconfirm --needed zellij'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec --dir /workspaces/project -c 'sh .wsl-toolkit/common/bootstrap.sh --toolset agent --without nim,powershell,tmux --no-tmux-config'
```

The operator starts or rejoins the durable terminal from `base shell`:

```sh
cd /workspaces/project
zellij attach --create muse-code
```

Detach without stopping it by pressing `Ctrl-o`, then `d`, and run the same
attach command after reconnecting.

For native Windows attachment, the first-run, token and key guide is
[`../common/zellij.md`](../common/zellij.md). A native Windows Zellij 0.45.1
client completed an authenticated attach to the WSL 0.45.1 server over localhost
on 2026-09-13, and `WSL-69` in
[`../../../../../TODO/wsl-toolkit-go.md`](../../../../../TODO/wsl-toolkit-go.md)
records that run.

The agent reaches the same session through `base exec`. Zellij 0.45.1 exposes
stable pane IDs, JSON discovery, targeted input and screen capture:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij attach --create-background muse-code'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij --session muse-code action list-panes --all --json'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij --session muse-code action dump-screen --full --pane-id terminal_0'
```

For ordinary unattended commands, use `base exec -c` directly and trust its
forwarded exit status. Use a Zellij pane when the process must remain visible,
interactive, or durable across terminal disconnects.

⚠ **`--toolset agent` is a long install on a fresh base**, because it carries
Rust, Go, Nim, Python and PowerShell as well as the developer set. Drop what this
provider does not need with `--without`, for example
`--without nim,powershell`.

## Install and run Muse Code

Muse's official documentation currently requires a Meta login. Its published
Linux installer is mutable, so it is shown here as an operator step and is not
executed by this repository's bootstrap script. Download it, inspect the saved
file, then run the file you inspected:

```sh
curl --proto '=https' --tlsv1.2 --fail --location \
  https://dev.meta.ai/install.sh --output /tmp/muse-code-install.sh
wc -l /tmp/muse-code-install.sh
sha256sum /tmp/muse-code-install.sh
less /tmp/muse-code-install.sh
sh /tmp/muse-code-install.sh
rm -f /tmp/muse-code-install.sh
muse --version
muse login
```

Start Muse from the Zellij session and the granted checkout:

```sh
cd /workspaces/project
muse
```

Windows interop is off. If authentication prints a URL, open it yourself in a
Windows browser rather than trying to launch one from the guest. Muse's auth,
configuration and sessions stay in the persistent Linux home; the checkout is
the only configured Windows directory it can edit.
