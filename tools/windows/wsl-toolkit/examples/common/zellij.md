# Zellij: one WSL session, two ways in

Zellij runs **inside the named WSL base**. The operator may attach from the
native Windows Zellij client over its authenticated localhost web transport;
the agent controls the same Linux session through `wsl-toolkit base exec`.

Measured on 2026-09-13:

- Linux server: Zellij 0.45.1 in `wsl-toolkit-muse`;
- Windows client: Zellij 0.45.1 at `%LOCALAPPDATA%\Zellij\zellij.exe`, which is
  not on `PATH` on the measured host;
- Windows reached the WSL server at `http://127.0.0.1:8082` and completed an
  authenticated terminal-to-terminal attach;
- JSON pane discovery, stable pane IDs, targeted input and screen dumps all
  worked through `base exec`.

The Windows client and the Linux server share no local IPC endpoint: Windows
uses named pipes and Linux uses a runtime socket. A bare Windows
`zellij attach muse-code` therefore does not reach WSL, and the remote attach
below is the bridge. Zellij documents the
[web client and remote terminal attach](https://zellij.dev/documentation/web-client.html);
Microsoft documents [Windows-to-WSL localhost forwarding](https://learn.microsoft.com/windows/wsl/networking).

## First operator connection

Create the Linux session in the granted checkout and start a localhost-only web
server:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec --dir /workspaces/project -c 'zellij attach --create-background muse-code'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij web --status >/dev/null 2>&1 || zellij web --daemonize --ip 127.0.0.1 --port 8082'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij web --create-token'
```

The last command prints a login token once. Keep it out of the checkout, issue
tracker and chat. Zellij 0.45.1 was measured rejecting the documented
`--create-token --token-name NAME` combination, so let it assign a name such as
`token_1`.

Paste the token at the prompt below. It does not enter PowerShell history;
`--remember` stores the authenticated session in the native client for four
weeks:

```powershell
$token = Read-Host 'Paste the one-time Zellij token'
& "$env:LOCALAPPDATA\Zellij\zellij.exe" attach 'http://127.0.0.1:8082/muse-code' --token $token --remember
Remove-Variable token
```

Later attachments need no token while that remembered credential remains:

```powershell
& "$env:LOCALAPPDATA\Zellij\zellij.exe" attach 'http://127.0.0.1:8082/muse-code'
```

After WSL has stopped, start the localhost server again with the second command
from the first block. The token and session metadata live in the managed
account's persistent home.

## The human quickstart

Start with Zellij's default keybinding preset; the bottom status bar always
shows the keys valid in the current mode.

| What you want | Default keys |
| --- | --- |
| new pane where Zellij chooses | `Alt-n` |
| move between panes | `Alt` + arrow, or `Alt` + `h/j/k/l` |
| split down or right | `Ctrl-p`, then `d` or `r` |
| close the focused pane | `Ctrl-p`, then `x` |
| enter scroll mode | `Ctrl-s`; use arrows or Page Up/Down, then `Esc` |
| open the session manager | `Ctrl-o`, then `w` |
| open configuration | `Ctrl-o`, then `c` |
| detach and leave work running | `Ctrl-o`, then `d` |

If Muse or an editor needs shortcuts Zellij intercepts, open configuration with
`Ctrl-o c` and choose **Unlock-First (non-colliding)**. Under that preset,
Zellij actions begin with `Ctrl-g`; for example a new pane is `Ctrl-g p n`.
The official guide explains both
[keybinding presets](https://zellij.dev/documentation/keybinding-presets).

The terminal alternative, useful before the native connection is configured,
is:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base shell
```

Then run `cd /workspaces/project && zellij attach --create muse-code`.

## Agent control

The agent does not invoke the native Windows Zellij. It uses the audited
host-to-guest seam:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij attach --create-background muse-code'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij --session muse-code action list-panes --all --json'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij --session muse-code run --cwd /workspaces/project --name checks --block-until-exit -- sh -lc "git status --short"'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij --session muse-code action dump-screen --full --pane-id terminal_0'
```

For an interactive pane, read its ID from `new-pane`, send text to that ID, then
send Enter separately:

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij --session muse-code action new-pane --cwd /workspaces/project --name agent -- sh'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij --session muse-code action write-chars --pane-id terminal_1 "git status --short"'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij --session muse-code action send-keys --pane-id terminal_1 ENTER'
```

Discover the pane ID immediately before using it; do not assume `terminal_1`
survived from an earlier run.

## Token and server control

```powershell
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij web --list-tokens'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij web --revoke-token token_1'
wsl-toolkit --instance muse --config C:\path\to\project\wsl-toolkit.json base exec -c 'zellij web --stop'
```

Revoke only the exact token name you intend. Keep the server on
`127.0.0.1`; binding beyond localhost requires TLS and firewall decisions this
example deliberately does not make.
