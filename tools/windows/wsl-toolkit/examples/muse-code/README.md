# Muse Code in one named WSL base

This is the concrete provider example for the one-checkout profile in
[`../common/access-profiles.md`](../common/access-profiles.md). It gives Muse a
persistent Linux home, systemd, rootless Podman, developer tools, CodeGraph and
tmux, with exactly one Windows checkout writable by the ordinary base account.

## Prepare the checkout on Windows

1. Copy [`../../../../../scripts/common/bootstrap.sh`](../../../../../scripts/common/bootstrap.sh)
   and [`../../../../../scripts/common/tmux.conf`](../../../../../scripts/common/tmux.conf)
   into the target checkout as `.wsl-toolkit/common/`. ⚠ Both, and in the same
   directory: the bootstrap installs the tmux configuration it finds BESIDE
   itself, and reports that it found none rather than reaching the network for
   one.
2. Save the **One read/write checkout** JSON from
   [`access-profiles.md`](../common/access-profiles.md) as
   `wsl-toolkit.json` at that checkout's root.
3. Validate before creating anything:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json config validate
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json config
```

The second command must print one `grant` row whose Windows source is the exact
target checkout and whose guest target is `/workspaces/project`.

Create or reconcile the named base, then make it prove its live state:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base ensure
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base status --probe --json
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base shell
```

Do not add `--root`. The ordinary account is the access boundary.

## Bootstrap the durable session

Inside the base:

```sh
cd /workspaces/project
sh .wsl-toolkit/common/bootstrap.sh --toolset agent
tmux new-session -A -s muse-code
```

Detach without stopping the session with `Ctrl-b d`, and run the same
`tmux new-session -A -s muse-code` command after reconnecting. The status line
carries the other two keys that matter.

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
sed -n '1,240p' /tmp/muse-code-install.sh
sh /tmp/muse-code-install.sh
rm /tmp/muse-code-install.sh
muse --version
muse login
```

Start Muse from the tmux session and the granted checkout:

```sh
cd /workspaces/project
muse
```

Windows interop is off. If authentication prints a URL, open it yourself in a
Windows browser rather than trying to launch one from the guest. Muse's auth,
configuration and sessions stay in the persistent Linux home; the checkout is
the only configured Windows directory it can edit.
