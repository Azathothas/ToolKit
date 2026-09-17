---
name: wsl-toolkit-agents
description: "Drive coding agents through herdr in a wsl-toolkit base: start Muse Code, pi or omp in a pane, send a turn, read the answer back, and set the model and the reasoning effort each one begins on. Works from Windows across the wsl-toolkit bridge and from a shell inside the base. Use when the user asks to run, steer, watch or configure an agent in a base, or names herdr, muse, pi or omp. Do not use for an agent that is not running under herdr."
---

# Driving agents under herdr

herdr is a terminal multiplexer that knows what a coding agent is. One herdr server
runs inside a wsl-toolkit base. Agents run in its panes, and you steer them from
Windows without opening anything.

⛔ **The Windows client and the Linux server share no local IPC.** herdr's socket is a
Unix socket on Linux and a named pipe on Windows. SSH is the only bridge, and the
base's `herdr` adapter installs it.

---

## 1. Work out where you are

**Inside a herdr pane**, the `herdr` command talks to the session you are in:

```bash
test "${HERDR_ENV:-}" = 1
```

**On Windows, outside herdr**, every herdr command goes through the bridge:

```powershell
wsl-toolkit --instance base base herdr -- agent list
```

⭐ **Everything after `--` is herdr's, argument for argument**, so a prompt carrying
quotes or a dollar sign is never read by a shell. Every answer is JSON.

⚠ Replace `base` with the instance the user actually has. `wsl-toolkit --instance NAME
config` prints what is configured.

---

## 2. Learn the CLI from the CLI

⛔ **Do not guess a herdr flag.** herdr moves quickly and a pasted flag list rots:

```powershell
wsl-toolkit --instance base base herdr -- agent --help
wsl-toolkit --instance base base herdr -- agent start --help
wsl-toolkit --instance base base herdr -- api schema --json
```

⛔ **Never run bare `herdr` to find out what it does.** It launches or attaches the
terminal UI and blocks. Print a group instead.

---

## 3. Start an agent

Make a pane, read its id back from the command that made it, and start an agent in it:

```powershell
$made = wsl-toolkit --instance base base herdr -- workspace create --cwd '~' --label work --no-focus | ConvertFrom-Json
$pane = $made.result.root_pane.pane_id
wsl-toolkit --instance base base herdr -- agent start work --kind muse --pane $pane --timeout 120000
```

| part | what it is |
| --- | --- |
| the first argument | the NAME you address the agent by afterwards |
| `--kind` | which agent: `muse`, `pi`, `omp`, and others herdr knows |
| `--pane` | a pane already at its interactive shell prompt |
| `--timeout` | milliseconds to wait for readiness. Default 30000, maximum 300000 |

⚠ **Read the pane id from the command output.** The base may already hold workspaces,
so a guessed id points at someone else's work.

⛔ **The name must be free.** A second start under a name in use answers
`agent_name_taken` and names the pane already holding it. Pick another name.

---

## 4. Send a turn, and read the answer

```powershell
wsl-toolkit --instance base base herdr -- agent prompt work "Answer with only the number of files git tracks here."
wsl-toolkit --instance base base herdr -- agent read work --source recent --lines 40
```

⛔ **Submit with `agent prompt`, not with `pane send-text` plus `pane send-keys
enter`.** Measured against Muse Code 1.3.0: the two pane commands left the text
sitting in the input with a newline after it, and only `agent prompt` submitted it.

⛔ **Every wait needs a `--timeout`.** A wait has no default and can wait for ever.

```powershell
wsl-toolkit --instance base base herdr -- agent wait work --until idle --timeout 300000
```

---

## 5. ⭐ Read back what the agent is really on

⛔ **A configuration file saying a model is set is not the model the agent started
on.** Read the agent's own status line:

```powershell
wsl-toolkit --instance base base herdr -- agent read work --lines 5
```

| agent | what its status line looks like |
| --- | --- |
| muse | `<model> · <effort> · <cwd>` |
| pi | `(<provider>) <model> • <effort>` |
| omp | `◕ <model name>` in its status bar |

Two silent failures found this way on 2026-09-17, both of which every file said were
fine:

- ⛔ **pi resolves its startup model against its own catalogue.** A model the provider
  serves but `~/.pi/agent/models.json` does not declare makes pi fall back to a
  built-in model **on another provider**, with no message.
- ⛔ **pi clamps `max` to `high`** unless that model declares a `thinkingLevelMap`
  exposing it. Its own documentation says so.

---

## 6. Set the model and the effort a session begins on

⛔ **AN ENVIRONMENT VARIABLE CANNOT DO THIS.** An agent herdr starts inherits the
herdr **service's** environment, not a login shell's. A value exported in `~/.profile`
or `~/.bashrc` never reaches it. This was measured by reading `/proc/PID/environ` for
every pane process.

⭐ **Set it in the base's configuration and let the tool write each agent's own file:**

```json
"adapters": [
  { "name": "herdr", "channel": "nightly" },
  { "name": "muse", "model": "MODEL", "effort": "max" },
  { "name": "pi",   "model": "PROVIDER/MODEL", "effort": "max" },
  { "name": "omp",  "model": "MODEL", "effort": "max" }
]
```

Then:

```powershell
wsl-toolkit --instance base base ensure
```

| field | what it does |
| --- | --- |
| `model` | the model a session nobody passed a flag to begins on. Empty leaves the agent's own default alone. A value with a slash is `provider/id`, for an agent that reaches a model through a named provider |
| `effort` | the reasoning effort. **`max` when a configuration names none** |

⚠ **Each agent's effort vocabulary is its own**, and a word an agent does not take is
refused rather than sent to it. `wsl-toolkit man --no-pager` carries the three lists
and where each value is written.

**To change either by hand**, per agent:

| agent | the door |
| --- | --- |
| pi | `~/.pi/agent/settings.json`: `defaultProvider`, `defaultModel`, `defaultThinkingLevel` |
| omp | `omp config set modelRoles` for the `default` role, and `omp config set defaultThinkingLevel` |
| muse | ⛔ no settings key exists. `/usr/local/bin/muse` passes `--model` and `--reasoning-effort` |

⛔ **Muse refuses a root option in front of a subcommand**, and refuses a repeated
`--model`. So `muse --model M config status` exits 2, and the wrapper adds nothing at
all to `muse login`.

---

## 7. Attach, when a person wants to watch

```powershell
wsl-toolkit --instance base base attach
```

It prints the exact line for this machine and runs nothing. Nothing needs starting
first. Leave with the prefix, `ctrl+b`, then `q`; every pane and agent keeps running.

From inside the base, `wsl-toolkit --instance base base shell`, then `herdr`.

---

## 8. Traps

⛔ **An agent must be on the PATH a PANE has**, which is a login shell's and does not
include `~/.local/bin`. The adapters write a wrapper on the system path for this. If
`agent start` times out with `command not found` in the pane, run `base ensure`.

⛔ **tmux inside a herdr pane hides the agent.** herdr then sees `tmux` as the pane
process. Never start a multiplexer inside a pane.

⛔ **A launcher that does not `exec` hides the agent** too, because herdr reads the
pane's foreground process.

⛔ **Signing an agent in is the user's.** Never ask for, print, copy or store a
credential, a code or a sign-in URL.

⚠ **`herdr agent prompt` answers `agent_prompt_stalled` for an agent with no turn to
run**, which is not a failure of the prompt.
