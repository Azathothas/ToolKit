# herdr: one server in the base, two ways in

herdr runs inside the named base as its account, and its server starts with the
base. The operator watches and steers the agents from herdr's Windows client; an
agent on Windows drives the same server through `wsl-toolkit base herdr`. The base's
`herdr` adapter installs all of it, and
[`../../wsl-toolkit.md`](../../wsl-toolkit.md) says what that adapter writes.

⚠ **What herdr does on this host is measured where a line says so, on 2026-09-15,
with herdr 0.9.0 on both sides.** The rest is read from herdr's documentation for
0.9.0. [`../../../../../docs/reference-sweeps/usable.md`](../../../../../docs/reference-sweeps/usable.md)
carries the sweep and its commits; this page carries only what an operator does.

---

## ⛔ Two things to know before the first attach

1. ⛔ **herdr 0.9.0's Windows `--remote` client is reported to repaint only on
   window activation and to apply no prefix command.** That is
   `herdrdev/herdr#4176`, closed `not_planned` as a duplicate of `#4038`, which herdr
   closed as fixed on its development branch. ⛔ **No published herdr carries that
   fix**: 0.9.0 is the newest stable release and the newest preview predates it. ⭐
   **Measure the repaint before concluding anything about SSH**: a client that
   repaints only on window activation looks exactly like a connection that is not
   working.
2. ⚠ **herdr's Linux release binary is static and carries its own musl allocator,
   whatever the base's libc is.** `herdrdev/herdr#4174`, open, is a heap corruption
   that allocator detected inside a 0.9.0 server, so a glibc base does not avoid it.
   The base is `arch` because the adapters are measured on that preset alone.

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

```powershell
wsl-toolkit --instance base base herdr -- agent list
wsl-toolkit --instance base base herdr -- pane list
wsl-toolkit --instance base base herdr -- agent prompt w1:p1 "summarise what you just changed"
```

Every herdr command answers in JSON, and each argument reaches herdr as an argument,
so a prompt carrying quotes or a dollar sign is not read by any shell.

| measured on 2026-09-15, herdr 0.9.0 on Windows and in the base | result |
| --- | --- |
| `herdr --machine base agent list` | ⛔ exit 2, `unknown option: --machine`. 0.9.0 has no such prefix |
| a build of herdr's development branch, `--machine base agent list`, against the 0.9.0 server | exit 1, `remote Herdr does not support machine API forwarding` |
| `herdr machine add wsl-toolkit-base --label base` | exit 0 in 2.7 s. It saved the profile in `%LOCALAPPDATA%\herdr\client\endpoints.json`, installed nothing in the base, started no second server, and created a workspace on a server that had none |
| a development build on both sides, `--machine base agent list` | exit 0 in 1.3 s, with no terminal UI open |

⭐ **`--machine` needs a development build on both sides.** Set the base's herdr adapter
to `"channel": "nightly"`, as the manual's herdr adapter section says, and `base attach`
prints the Windows client of the build the base runs. ⭐ **Driven on 2026-09-16**: with
that channel set, `base ensure` installed `herdr-nightly-20260916-18061191fdc0` in 8.78 s,
`base attach` printed the nightly's own client, and that client answered `--machine base
agent list` with exit 0 in 2.1 s.

⚠ **A saved machine is for herdr's own multi-machine sidebar**, which herdr's 0.9.0
pages call not yet verified or supported on a Windows client. Remove one with
`herdr machine list`, then `herdr machine remove ID`.

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

Read a pane's id from the command that made it rather than guessing one, because
the base may already hold workspaces:

```powershell
$made = wsl-toolkit --instance base base herdr -- workspace create --cwd '~' --label work --no-focus | ConvertFrom-Json
$pane = $made.result.root_pane.pane_id
wsl-toolkit --instance base base herdr -- pane run $pane 'git status --short'
wsl-toolkit --instance base base herdr -- pane read $pane --source recent --lines 40
```

⚠ **The pane runs the command and returns at once.** `herdr pane wait-output PANE
--regex TEXT --timeout MS` waits for what the command prints.

⛔ **A wait has no default timeout and can wait for ever.** Always pass
`--timeout`. A server error prints JSON on stderr and exits 1; bad CLI syntax
exits 2.

⚠ **Submit a prompt with `herdr agent prompt`, not with `pane send-text` and
`pane send-keys enter`.** Measured against Muse Code 1.3.0: the two pane commands
left the text in Muse's input with a new line after it, and `agent prompt` submitted
it.

---

## ⭐ Starting an agent, and reading what it says

```powershell
$made = wsl-toolkit --instance base base herdr -- workspace create --cwd '~' --label work --no-focus | ConvertFrom-Json
$pane = $made.result.root_pane.pane_id
wsl-toolkit --instance base base herdr -- agent start work --kind muse --pane $pane --timeout 120000
```

| part | what it is |
| --- | --- |
| the first argument | the NAME you will address the agent by. ⛔ It must be free: a second start under a name in use answers `agent_name_taken` and names the pane holding it |
| `--kind` | which agent this is. herdr's list includes `muse`, `pi` and `omp` |
| `--pane` | a pane already at its interactive shell prompt |
| `--timeout` | how long to wait for readiness. Default 30000 ms, maximum 300000 |

⭐ **The name is also the target of every later command**, so `agent prompt`,
`agent read` and `agent wait` all take it:

```powershell
wsl-toolkit --instance base base herdr -- agent prompt work "summarise what you just changed"
wsl-toolkit --instance base base herdr -- agent read work --source recent --lines 40
```

⭐ **`agent read` is how you check what an agent is really doing**, including which
model and effort it started on. Each agent prints that in its own status line:

| agent | what its status line reads |
| --- | --- |
| muse | `muse-spark-1.3-contributor · max · ~` |
| pi | `(muse-gateway) muse-spark-1.3-contributor • max` |
| omp | `◕ Muse Spark 1.3 Contributor` |

⛔ **Read it back rather than trusting the configuration.** Both pi failures found on
2026-09-17 - a model it could not resolve, and an effort it clamped - looked correct in
every file and wrong in that one line.

⭐ **herdr has full lifecycle authority over pi and omp**, so their state comes from the
agent rather than from a guess about its screen:

```powershell
wsl-toolkit --instance base base herdr -- agent explain work
```

It answers `screen_detection_skip_reason: full_lifecycle_hook_authority` for those two.
Muse reports through the hook this tool's `muse` adapter installs.

---

## ⛔ Four traps this tool's shape walks into

1. ⛔ **tmux inside a herdr pane hides the agent.** herdr's agents page: detection
   does not inspect a tmux session launched inside a pane, so herdr sees `tmux`
   as the pane process instead of the agent behind it. ⭐ **tmux is this tree's
   generic fallback for a shell that is not inside herdr**, and a shell framework
   that auto-enters tmux breaks agent detection completely.
   [`README.md`](README.md) says where each one belongs.
2. ⚠ **A launcher that does not `exec` hides the agent.** herdr reads the pane's
   foreground process. Measured: Muse started in a base pane as `muse` passes
   through `/usr/local/bin/muse` and Muse's own launcher, every hop `exec`s, and
   herdr names the process `muse-bin-1.3.0-R3057.1` and the agent `muse`. ⚠
   `muse.exe` and `base agent` never put Muse in a pane at all: they run a command
   with no terminal.
3. ⭐ **WSL exposes a terminal's foreground process group.** Measured: herdr's
   `pane process-info` and the kernel's `tpgid` named the same process group, so
   `HERDR_PROCESS_DETECTION=child-groups` is not needed.
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
