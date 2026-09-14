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

The base's `muse` adapter installs it. Name it in `base.adapters` and run `base
ensure`; [`wsl-toolkit-base.json`](wsl-toolkit-base.json) beside this page is the
profile for the one base every agent shares, with the `herdr` and `muse` adapters,
the account `herdr` and no standing grant. From a clone of this repository, put it
where `--instance base` reads it from any directory, then build:

```powershell
New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\wsl-toolkit\instances\base" | Out-Null
Copy-Item tools\windows\wsl-toolkit\examples\muse-code\wsl-toolkit-base.json "$env:LOCALAPPDATA\wsl-toolkit\instances\base\config.json"
wsl-toolkit --instance base base ensure
wsl-toolkit --instance base base status --probe --json
```

⛔ **Meta's installer runs only while its digest is approved.** The adapter saves
it, prints its length and SHA-256, and runs it as the account when the digest is the
one the adapter pins or the `installer_sha256` the profile's `muse` entry carries.
Any other stops the ensure with exit 2 and prints how to read the saved file; add its
digest as `installer_sha256` only after reading it.

⛔ **Signing in is the operator's.** `muse login` is a device-code sign-in: it
prints an `auth.meta.com` URL and a code, and offers to open a browser, which it
cannot do with interop off. Open the URL in a Windows browser. Measured on
2026-09-13, the credential lands in `~/.config/muse/auth.json`, mode 0600, in the
persistent home.

```powershell
wsl-toolkit --instance base base shell
```

Then run `muse login` in that shell.

⭐ **`muse` is on `PATH` in `base exec`**, through `/usr/local/bin/muse`, which the
adapter writes and which runs Muse only as the base's account.

⚠ **Muse says your content may be used for product improvement.** Its first
screen named the model `muse-spark-1.3-contributor` and printed that notice. This
repository makes no claim about Meta's terms; read them before giving it a
checkout that matters.

### An agent on Windows, driving Muse headless

`muse exec` runs one prompt with no terminal and reports JSON events. Put the
prompt in a file inside the base, then:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec --dir /workspaces/project -c '. ~/.profile; muse exec --json --workspace /workspaces/project --approval-mode never --prompt-file /tmp/muse-task.txt'
```

`--approval-mode never` keeps an unattended run from waiting on a prompt nobody
answers, and Muse's own sandbox stays on. ⭐ **Driven through the WSL-69
smoke**: Muse read the checkout, ran a program in it, wrote a file that appeared
on Windows, committed, and pushed to the checkout's remote, in 52 seconds, with
exit 0.

### An agent on Windows, driving Muse's interactive screen

Start Muse in a named pane of the durable session, then type into it and read the
screen back:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c '. ~/.profile; zellij attach --create-background muse-code; zellij --session muse-code action new-pane --cwd /workspaces/project --name muse -- bash -lc "muse"'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij --session muse-code action write-chars --pane-id terminal_1 "Run python3 src/inventory.py and answer with only the number it prints."'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij --session muse-code action send-keys --pane-id terminal_1 ENTER'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij --session muse-code action dump-screen --full --pane-id terminal_1'
```

⚠ **The first start in a checkout asks `Do you trust this workspace?`**, and a
question typed before that is answered is lost rather than queued. Answer it with
`ENTER` for the default, `1 Trust and continue`, then type the question. Read the
pane id from `new-pane` or `list-panes --json` rather than assuming one.
⭐ Driven on 2026-09-13: the answer was on screen 10 seconds after `ENTER`.

Windows interop is off, so nothing in the guest can start a Windows program.
Muse's auth, configuration and sessions stay in the persistent Linux home, and
the checkout is the only configured Windows directory it can edit. ⚠ Muse has
passwordless sudo in this profile, so that is a configuration, not a boundary.
