---
name: wsl-toolkit-agents
description: "Drive coding agents through herdr in a wsl-toolkit base. Start Muse Code, pi or omp in a pane, send a turn, and read the answer back. Set the model and the reasoning effort each one begins on. Works from Windows across the wsl-toolkit bridge and from a shell inside the base. Use when the user asks to run, steer, watch or configure an agent in a base, or names herdr, muse, pi or omp. Do not use for an agent that is not running under herdr."
---

# Driving agents under herdr

herdr is a terminal multiplexer that knows what a coding agent is. One herdr server
runs inside a wsl-toolkit base. Agents run in its panes, and you steer them from
Windows without opening anything.

⛔ **The Windows client and the Linux server share no local IPC.** herdr's socket is a
Unix socket on Linux and a named pipe on Windows. `wsl-toolkit base herdr` reaches the
server through `wsl.exe`. herdr's own Windows client, `herdr --remote`, reaches it
over SSH, through the door the base's `herdr` adapter installs.

---

## 1. Get the tool, and two rules that cost a session when broken

`wsl-toolkit` is a single Windows executable. Ask it what it is:

```powershell
wsl-toolkit --version
```

If the command is not found, the executable is not on `PATH`. The user downloads
`wsl-toolkit-windows-amd64.exe`, or `wsl-toolkit-windows-arm64.exe` on ARM, from the
newest `wsl-toolkit-v*` release of `Azathothas/ToolKit`. The user renames it to
`wsl-toolkit.exe` and puts it in a `PATH` directory such as `%USERPROFILE%\bin`.

To move it to the newest release:

```powershell
wsl-toolkit selfupdate
```

⚠ **A configuration field the tool does not know is refused**, as
`FILE is not readable as wsl-toolkit-config/1: json: unknown field "X"`. A field in
the wrong place gives the same message. Compare the field with `config --effective`.
If its name and place are right, run `selfupdate`.

⛔ **FROM GIT BASH, EXPORT TWO VARIABLES FIRST.** MSYS rewrites a guest path such as
`--cwd /workspaces/proj` into `C:/Program Files/Git/workspaces/proj` before the tool
starts, and the tool refuses the rewrite.
`export MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*'` stops it. Every example here is
PowerShell, and one example pipes into `ConvertFrom-Json`, which is PowerShell only.

⛔ **READ AN EXIT CODE FROM THE PROCESS, NOT THROUGH A PIPE.** Piped into another
program, such as `findstr`, `$LASTEXITCODE` is that program's code, not the tool's.
Read it on the line after the command, with nothing piped.

⚠ **Asking for help is not a failure.** `wsl-toolkit base --help` and every
`wsl-toolkit base SUBCOMMAND --help` answer 0. `wsl-toolkit base` with no
subcommand answers 2, because nothing was asked.

⛔ **Never guess a `wsl-toolkit` flag either.** Its manual is generated from the
commands the executable really has, so it is current for the binary you are
running:

```powershell
wsl-toolkit man --no-pager
```

---

## 2. Work out where you are

**Inside a herdr pane**, the `herdr` command talks to the session you are in:

```bash
test "${HERDR_ENV:-}" = 1
```

**On Windows, outside herdr**, every herdr command goes through the bridge:

```powershell
wsl-toolkit base herdr -- agent list
```

⭐ **Everything after `--` is herdr's, argument for argument.** Each one is quoted
once, so no shell in the base reads it as syntax. Your own PowerShell still does, so
single-quote a prompt in PowerShell. Most answers are JSON.

⛔ **herdr's own 2 is herdr's.** `wsl-toolkit base herdr` answers herdr's exit code: 1
for a server error, and 2 for a command line herdr refuses. It answers 125 where the
tool could not run herdr, such as a base with no `herdr` adapter.

⭐ **The agents run in the default base, so no example here passes `--instance`.** Its
canonical name is `wsl-toolkit-jobs`. ⚠ An older default base may still be named
`wsl-toolkit`, and it keeps working. `wsl-toolkit base status` says which name it has.
For another base, `wsl-toolkit-NAME` is `--instance NAME`.

⛔ **`base` and `jobs` are reserved instance names.** A new instance under either is
refused, and an existing one stays reachable. ⚠ A new instance named `wsl-NAME` is
made as `NAME`, with a warning. `--literal-instance` keeps the name as given.

---

## 3. Learn the CLI from the CLI

⛔ **Do not guess a herdr flag.** herdr moves quickly and a pasted flag list rots:

```powershell
wsl-toolkit base herdr -- agent --help
wsl-toolkit base herdr -- agent start --help
wsl-toolkit base herdr -- api schema --json
```

⛔ **Never run bare `herdr` to find out what it does.** It launches or attaches the
terminal UI and blocks. Print a group instead.

---

## 4. Start an agent

Make a pane, read its id back from the command that made it, and start an agent in it:

```powershell
$made = wsl-toolkit base herdr -- workspace create --cwd /workspaces/proj --label work --no-focus | ConvertFrom-Json
$pane = $made.result.root_pane.pane_id
wsl-toolkit base herdr -- agent start work --kind muse --pane $pane --timeout 120000
```

⚠ **`--cwd` takes an absolute guest path.** herdr does not expand `~`. Use a granted
directory under `/workspaces`, or the account's home by its full path.

The first argument to `agent start` is the NAME you address the agent by afterwards.
`agent start --help` names every other flag and the agents herdr knows.

⚠ **Read the pane id from the command output.** The base may already hold workspaces,
so a guessed id points at someone else's work.

⛔ **The name must be free.** A second start under a name in use answers
`agent_name_taken` and names the pane already holding it. Pick another name.

---

## 5. Send a turn, and read the answer

```powershell
wsl-toolkit base herdr -- agent prompt work 'Answer with only the number of files git tracks here.' --wait --timeout 300000
wsl-toolkit base herdr -- agent read work --source recent --lines 40
```

`--wait` returns when the agent reaches `idle`, `done` or `blocked` after the
submission, so the read comes after the turn. ⚠ herdr does not track turns: if the
agent was already working, the end of that turn can match instead.

⛔ **Submit with `agent prompt`, not with `pane send-text` plus `pane send-keys
enter`.** With Muse Code 1.3.0 the two pane commands leave the text in the input
with a newline after it, and only `agent prompt` submits it. Section 9 names the
one exception.

⛔ **Every wait needs a `--timeout`.** A wait has no default and can wait for ever.

---

## 6. ⭐ Read back what the agent is really on

⛔ **A configuration file saying a model is set is not the model the agent started
on.** Read the agent's own status line:

```powershell
wsl-toolkit base herdr -- agent read work --lines 5
```

| agent | what its status line looks like |
| --- | --- |
| muse | `<model> · <effort> · <cwd>` |
| pi | `(<provider>) <model> • <effort>` |
| omp | `◕ <model name>` in its status bar |

Two silent failures that only the status line shows, while every file says the
setting is right:

- ⛔ **pi resolves its startup model against its own catalogue.** A model the provider
  serves but `~/.pi/agent/models.json` does not declare makes pi fall back to a
  built-in model **on another provider**, with no message.
- ⛔ **pi clamps `max` to `high`** unless that model declares a `thinkingLevelMap`
  exposing it. Its own documentation says so.

---

## 7. Set the model and the effort a session begins on

⛔ **AN ENVIRONMENT VARIABLE CANNOT DO THIS.** An agent herdr starts inherits the
herdr **service's** environment. A value exported in `~/.profile` never reaches it.

⭐ **Set it in the base's configuration and let the tool write each agent's own file.**
The fields go under `base`, beside the file's `schema` and its other fields.
`wsl-toolkit config --write` puts the whole file on disk to edit. ⚠ Where
`wsl-toolkit config` names a `wsl-toolkit.json` instead, that file is in effect, so
edit that one:

```json
{
  "schema": "wsl-toolkit-config/1",
  "base": {
    "systemd": true,
    "adapters": [
      { "name": "herdr" },
      { "name": "muse", "model": "MODEL", "effort": "max" },
      { "name": "pi",   "model": "PROVIDER/MODEL", "effort": "max" },
      { "name": "omp",  "model": "MODEL", "effort": "max" }
    ]
  }
}
```

⚠ `herdr` needs `"systemd": true`, and every adapter needs the `arch` image preset,
which is the default image.

Then:

```powershell
wsl-toolkit base ensure
```

⛔ **`base ensure` can restart the base, which ends every pane and agent in it.** Ask
the user before you run it on a base that holds work. ⚠ Each adapter installs with
`pacman -Syu --needed`, which upgrades every package in the base.

⛔ **A Muse installer whose digest nobody approved stops it with exit 2.** The ensure
keeps the installer and prints how to read it. Approving it is the user's decision,
after reading it. Never add `installer_sha256` yourself.

⛔ **So does a pi or omp release whose digest is not the adapter's pin.** Each comes
from its provider's own release, and npm only when that cannot be fetched. A changed
digest is not a fetch failure, so npm is not tried.

⭐ **Pin that release yourself where it agrees with its own checksum list.** The
refusal names the list and the saved file. Compare the file's SHA-256 with the list's
line for it. Where they agree, set the `version` and `sha256` the refusal prints, run
`base ensure` again, and tell the user the digest and the list's URL. ⛔ Where they
disagree, stop and report both. The file is not the one the provider published.

⚠ **A pi model declared before its key is stored is a note.** `base status --probe`
reads `credential absent` for pi, and stops nothing. So is a model while
`~/.pi/agent/models.json` does not exist yet. A `models.json` that lacks the model
is a problem.

⚠ **Only the state directory that owns the base installs an adapter.** From another
one, or from a shell whose `%LOCALAPPDATA%` Windows redirects into an app's copy,
`base ensure` says so in one line and exits 0. Run it from PowerShell or a terminal
outside the app. `wsl-toolkit base status` names the owner.

| field | what it does |
| --- | --- |
| `model` | the model a session nobody passed a flag to begins on. Empty leaves the agent's own default alone. A value with a slash is `provider/id`, for an agent that reaches a model through a named provider |
| `effort` | the reasoning effort. **`max` when a configuration names none** |

⚠ **Each agent's effort vocabulary is its own.** A word an agent does not take is
refused rather than sent to it, and the refusal names the words that agent takes.

**To change either by hand**, per agent:

| agent | the door |
| --- | --- |
| pi | `~/.pi/agent/settings.json`: `defaultProvider`, `defaultModel`, `defaultThinkingLevel` |
| omp | `omp config set modelRoles` for the `default` role, and `omp config set defaultThinkingLevel` |
| muse | ⛔ Muse Code 1.3.0 has no settings key for either. `/usr/local/bin/muse` passes `--model` and `--reasoning-effort` |

⛔ **Muse Code 1.3.0 refuses a root option in front of a subcommand**, and refuses a
repeated `--model`. So `muse --model M config status` exits 2, and the wrapper adds
nothing at all to `muse login`.

---

## 8. Attach, when a person wants to watch

```powershell
wsl-toolkit base attach
```

It prints three commands for this machine and runs none of them: one from Windows,
one from inside the base, and one for an agent. Nothing needs starting first. Leave
with the prefix, `ctrl+b`, then `q`; every pane and agent keeps running.

From inside the base, `wsl-toolkit base shell`, then `herdr`.

---

## 9. Traps

⛔ **An agent must be on the PATH a pane has, and that PATH does not include
`~/.local/bin`.** The adapters write a wrapper on the system path for this, and check
it on a login shell. If `agent start` times out with `command not found` in the pane,
`base status --probe` names the agent. `base ensure` writes its wrapper again, after
the question to the user that section 7 asks. ⚠ This holds for PATH only: an
exported variable still never reaches the agent, as section 7 says.

⛔ **A prefix ahead of that wrapper starts an agent without its defaults.** A login
shell then finds the agent's vendor launcher first. `base bootstrap` puts the
account's prefix after the system path and rewrites the line an earlier run put
first, so run `base bootstrap` when `base status --probe` refuses the base over it.
A line the account wrote itself is the user's to move: report it rather than
editing the account's profile.

⛔ **tmux inside a herdr pane hides the agent.** herdr then sees `tmux` as the pane
process. Never start a multiplexer inside a pane.

⛔ **A launcher that does not `exec` hides the agent** too, because herdr reads the
pane's foreground process.

⛔ **Under the `herdr` adapter's `private_net`, a process in a herdr pane reaches no
Windows host, runs no rootless container and gets no `sudo`.** It holds
only after herdr's server restarts, which ends every pane, so never turn the key on or
off without asking. `base status` names the key on its `herdr net` row, and `base
status --probe` says whether the server is in its namespace yet.

⚠ **It does not confine the rest.** `wsl-toolkit base agent NAME` and `NAME.exe` run
through `base exec`, in the shared namespace. A pane can still run a Windows program
through interop, `wsl.exe -u root` among them, which `base doors` reports as
`interop.exec-pe`. Run a container or a root step through `wsl-toolkit base exec`
from Windows instead.

⛔ **Signing an agent in is the user's.** Never ask for, print, copy or store a
credential, a code or a sign-in URL.

⚠ **`agent prompt --wait` answers `agent_prompt_stalled`** when no `working` or
`blocked` state appears within 5 seconds. herdr says neither that nor a timeout proves
nothing was sent. Read the agent before you send the prompt again.

⚠ **`agent wait --until idle` can time out on an agent that is already idle**, because
the turn ended before the wait began. Read the agent's state first, with `agent get`.

⛔ **A cancelled approval in Muse Code 1.3.0 leaves the agent `blocked`**, and
`agent prompt` then refuses every prompt with `agent_blocked`. This is the one place
for the pane commands. Send one turn with `pane send-text PANE 'TEXT'`, then
`pane send-keys PANE enter`.

⚠ **An idle Muse keeps such text unsent, as section 5 says, but a `blocked` one was
measured once to start the turn.** Nobody has measured why. Confirm it before the
next prompt: `agent get` no longer reads `blocked`, and `pane read` shows the turn,
not the text in the input. If the text still waits there, clear it in Muse's own
pane, through `base attach`, before any `agent prompt`.
