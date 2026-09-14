# herdr: one server in the base, two ways in

herdr runs inside the named base as its account, and its server starts with the
base. The operator watches and steers the agents from herdr's Windows client; an
agent on Windows drives the same server through `wsl-toolkit base exec`. The base's
`herdr` adapter installs all of it, and
[`../../wsl-toolkit.md`](../../wsl-toolkit.md) says what that adapter writes.

## Attach from Windows

herdr's Windows client must be on `PATH`. Ask the tool for the line, then run it:

```powershell
wsl-toolkit --instance base base attach
herdr --remote wsl-toolkit-base --remote-keybindings server
```

⭐ **Nothing needs starting first.** The first connection starts the base if it has
stopped, the base starts herdr's server, and herdr puts the workspaces back.

| to | press |
| --- | --- |
| leave, with every pane and agent still running | prefix then `q` |
| enter a key for herdr rather than for the pane | the prefix, `ctrl+b` |

⛔ **Closing a pane or a tab has no key.** The base's configuration unbinds both,
because herdr closes the focused pane at once with no question. Close one on
purpose with `herdr pane close`.

⚠ **Keep `--remote-keybindings server` on the line.** Without it the Windows client
uses the keys configured on Windows, which are herdr's defaults when nothing is
configured there.

## From a shell inside the base

```powershell
wsl-toolkit --instance base base shell
```

Then run `herdr`. Detach the same way.

## An agent on Windows

Every herdr command answers in JSON. Read a pane's id from the command that made
it rather than guessing one, because the base may already hold workspaces:

```powershell
$made = wsl-toolkit --instance base base exec -c 'herdr workspace create --cwd ~ --label work --no-focus' | ConvertFrom-Json
$pane = $made.result.root_pane.pane_id
wsl-toolkit --instance base base exec -c "herdr pane run $pane 'git status --short'"
wsl-toolkit --instance base base exec -c "herdr pane read $pane --source recent --lines 40"
```

⚠ **The pane runs the command and returns at once.** `herdr pane wait-output PANE
--regex TEXT --timeout MS` waits for what the command prints.
