# herdr: one server in the base, three ways in

herdr runs inside the named base as its account, and its server starts with the
base. The operator watches and steers the agents from herdr's Windows client; an
agent on Windows drives the same server through `wsl-toolkit base exec`. The base's
`herdr` adapter installs all of it, and
[`../../wsl-toolkit.md`](../../wsl-toolkit.md) says what that adapter writes.

⚠ **Everything below about herdr's own behaviour was READ from herdr's
documentation and tracker on 2026-09-15, not measured here.**
[`../../../../../docs/reference-sweeps/usable.md`](../../../../../docs/reference-sweeps/usable.md)
carries the sweep and its commits; this page carries only what an operator does.

---

## ⛔ Two things to settle before the first attach

1. ⛔ **herdr 0.9.0's Windows `--remote` client repaints only on
   window-activation events, and prefix commands never take effect.** That is
   `herdrdev/herdr#4176`, closed, so a later release carries the fix. The adapter
   pins 0.9.0. Check `herdr --version` on Windows against what the adapter
   installed in the base before concluding anything about SSH.
2. ⛔ **The base must be a glibc preset.** `herdrdev/herdr#4174`, open: a 0.9.0
   Linux server aborts in a musl malloc integrity check and every pane child
   dies. `arch` is the default and is glibc; `alpine`, `void-musl` and `chimera`
   are not.

---

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

⚠ **Authentication is Windows OpenSSH's, not herdr's.** herdr does not reuse a
Unix control socket on Windows, so a key with a passphrase is loaded with
`ssh-add` once rather than typed per connection.

---

## ⭐ Watch the agents without opening anything

`herdr --machine` routes one command to a saved SSH machine over its JSON API,
with **no terminal UI open at all**. This is the surface to script against:

```powershell
herdr machine add wsl-toolkit-base --label base
herdr --machine base agent list
herdr --machine base pane list
herdr --machine base agent prompt w1:p1 "summarise what you just changed"
```

| ⛔ | |
| --- | --- |
| the selector is a saved profile **id or a unique, case-sensitive label** | not an SSH hostname |
| `--machine` with `--session` or `--remote` | is an error |
| local pane ids are not inherited, and `--current` cannot mean a local pane | name the remote id |
| `agent attach` and plugin **installation** are not forwarded | run those in the base |

⚠ **The multi-machine sidebar is a different feature and herdr's own pages
disagree about it on Windows**: its capability table calls saved machines
supported, and its connecting-machines page says multi-machine connections are
not yet verified or supported on a Windows client. `--remote` and `--machine`
are not in dispute. Measure before planning on the sidebar.

---

## From a shell inside the base

```powershell
wsl-toolkit --instance base base shell
```

Then run `herdr`. Detach the same way.

⛔ **Do not run bare `herdr` to find out what it can do** - it launches or
attaches the terminal UI. Print a group instead, `herdr agent`, `herdr pane`, or
take the machine-readable contract with `herdr api schema --json`.

---

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

⛔ **A wait has no default timeout and can wait for ever.** Always pass
`--timeout`. A server error prints JSON on stderr and exits 1; bad CLI syntax
exits 2.

---

## ⛔ Four traps this tool's shape walks into

1. ⛔ **tmux inside a herdr pane hides the agent.** herdr's agents page: detection
   does not inspect a tmux session launched inside a pane, so herdr sees `tmux`
   as the pane process instead of the agent behind it. ⭐ **tmux is this tree's
   generic fallback for a shell that is not inside herdr**, and a shell framework
   that auto-enters tmux breaks agent detection completely.
   [`README.md`](README.md) says where each one belongs.
2. ⛔ **A launcher is a wrapper, and a wrapper hides the agent.** herdr reads the
   pane's foreground process; set `HERDR_AGENT=<agent>` **on the wrapper command
   on the herdr side**, never only inside the guest - herdr cannot see a variable
   set inside a container or VM.
3. ⚠ **WSL may not expose a foreground process group.** herdr offers
   `HERDR_PROCESS_DETECTION=child-groups` for restricted Linux runtimes. It is
   read by the **server**, needs a restart, and is best effort: a newer background
   job can be mistaken for the foreground one. Set it in the base, not on the
   Windows client.
4. ⛔ **herdr copies nothing onto an SSH host.** No plugin, configuration,
   executable or secret crosses; missing remote commands fail visibly. Whatever
   the base needs is installed **in the base**, which is what the adapter is for.

---

## The two halves that do not share an endpoint

⛔ **The Windows client and the Linux server have no common local IPC.** herdr's
socket is a Unix domain socket on Linux and a **named pipe** on Windows, so a
bare Windows `herdr` reaches only Windows. SSH is the bridge, and it is the only
one. That is why the `herdr` adapter installs an SSH door in the base rather than
forwarding a port.

⭐ **A payload never becomes part of a shell command.** herdr states that
`--machine` requests travel through its JSON API over non-interactive SSH and are
not interpolated into the SSH shell command - the same rule this tool holds for
every guest payload, reached independently.
