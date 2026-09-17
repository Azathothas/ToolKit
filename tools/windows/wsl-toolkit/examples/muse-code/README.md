# Agents in one named WSL base

⭐ **This is the guide. Follow it from the top and you get a working base.** It builds
one Linux base on Windows, installs herdr and the three agents, signs each one in, and
drives one of them. It is the concrete provider example for the one-checkout profile in
[`../common/access-profiles.md`](../common/access-profiles.md).

Every command below was run on 2026-09-17, on Windows 11 Pro 26200 with WSL 2.7.12.

⛔ **Run every command in PowerShell.** Git Bash rewrites a guest path: `--dir
/workspaces/proj` arrived as `C:/Program Files/Git/workspaces/proj`.

---

## What you get

| part | what it is |
| --- | --- |
| the base | one Arch distribution, `wsl-toolkit-base`, with systemd and rootless Podman |
| the grant | one Windows directory the agents can read and write. Nothing else |
| herdr | the multiplexer. One server in the base, and two ways in |
| muse, pi, omp | three agents, each on `PATH`, each known to herdr |

---

## 1. Install the tool

Download the newest `wsl-toolkit.exe` from this repository's releases. Put it in
`%USERPROFILE%\bin`. Add that directory to `PATH`.

Then update it at any time:

```powershell
wsl-toolkit selfupdate
```

Check the version:

```powershell
wsl-toolkit --version
```

⭐ **Read the manual from the tool, not from a page.** It is generated from the
commands the executable really has:

```powershell
wsl-toolkit man --no-pager
```

---

## 2. Write the configuration

[`wsl-toolkit-base.json`](wsl-toolkit-base.json) is the profile. Copy it to the
instance's own state directory:

```powershell
New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\wsl-toolkit\instances\base" | Out-Null
Copy-Item wsl-toolkit-base.json "$env:LOCALAPPDATA\wsl-toolkit\instances\base\config.json"
```

⛔ **Do not point `--config` at the file in this directory.** `base grant` writes the
configuration in effect. It would edit the example.

Read back the configuration in effect:

```powershell
wsl-toolkit --instance base config
```

---

## 3. Build the base

```powershell
wsl-toolkit --instance base base ensure
```

This builds the distribution, provisions it, and installs the four adapters. A second
run reconciles and does not rebuild.

⚠ **`base ensure` takes about 80 seconds from nothing**, and a few seconds after that.
The agents add to the first run.

The last lines print the state. Every adapter must read `healthy`.

---

## 4. Grant one project

Go to the checkout you want an agent to work in. Grant it:

```powershell
wsl-toolkit --instance base base grant --source . --mode rw
```

The grant is live. No restart is needed. The base sees the directory under
`/workspaces`:

```powershell
wsl-toolkit --instance base base exec -c 'ls /workspaces'
```

⛔ **A directory no grant covers is refused.** The refusal prints the `base grant` line
that would cover it. That is the profile working.

⚠ **Use `--mode ro` for a directory an agent must not change.**

Take a grant away by the path the base sees it at:

```powershell
wsl-toolkit --instance base base revoke --target /workspaces/demoproj
```

⛔ **`base revoke` takes `--target` alone.** A grant is named by where the base sees
it, not by the Windows directory. The mount goes at once. An empty directory is left
behind and reaches nothing.

---

## 5. Sign each agent in

⛔ **Signing in is yours, and this tool never sees a credential.** Do each one once.

### Muse Code

```powershell
wsl-toolkit --instance base base shell
```

Then, in the base:

```bash
muse login
```

⛔ **Do not paste the code or the URL anywhere but the browser.**

### pi

pi reaches a model through a named provider. Declare the provider first, then give it
the key.

⭐ **The key goes in pi's own file.** It does not go in `~/.profile`. An agent herdr
starts inherits the herdr **service's** environment, not a login shell's, so a key
exported in a profile never reaches it.

In the base, write the provider. This example is an Anthropic-style gateway:

```bash
mkdir -p ~/.pi/agent
cat > ~/.pi/agent/models.json <<'JSON'
{
  "providers": {
    "muse-gateway": {
      "baseUrl": "https://YOUR-GATEWAY-HOST",
      "apiKey": "$MUSE_GATEWAY_API_KEY",
      "api": "anthropic-messages",
      "models": [
        {
          "id": "muse-spark-1.3-contributor",
          "name": "Muse Spark 1.3 Contributor",
          "reasoning": true,
          "input": ["text", "image"],
          "contextWindow": 1048403,
          "maxTokens": 128000,
          "thinkingLevelMap": { "xhigh": "xhigh", "max": "max" }
        }
      ]
    }
  }
}
JSON
```

⛔ **Declare every model you will use.** pi resolves its startup model against this
file. A model the gateway serves but this file omits makes pi fall back to a built-in
model on a different provider, **and it says nothing**.

⛔ **And declare `thinkingLevelMap`.** Without it pi treats `xhigh` and `max` as
unsupported and clamps every session to `high`, silently.

Now store the key. Type it at the prompt so it never reaches your shell history:

```bash
mkdir -p ~/.pi/agent && chmod 700 ~/.pi/agent
[ -f ~/.pi/agent/auth.json ] || echo '{}' > ~/.pi/agent/auth.json
read -r -s -p 'gateway key: ' k; echo
jq --arg k "$k" '. + {"muse-gateway":{"type":"api_key","key":$k}}' ~/.pi/agent/auth.json > ~/.pi/agent/auth.new
mv ~/.pi/agent/auth.new ~/.pi/agent/auth.json
chmod 600 ~/.pi/agent/auth.json && unset k
```

Check it:

```bash
pi auth check --provider muse-gateway
```

It answers `ready`.

⚠ **The type is `api_key`.** Another word is accepted by `jq` and read by pi as no
credential at all. ⚠ **The file is merged**, so a provider already in it is kept.

### omp

Start omp once and complete its own onboarding:

```bash
omp
```

Then check which account is in force:

```bash
omp usage
```

It names the account and the quota left.

---

## 6. Prove the base

```powershell
wsl-toolkit --instance base base status --probe --json
```

⛔ **`registered` is not `usable`.** `--probe` runs a container and reads the adapters
back: each version, where each agent resolves on a **login** shell's `PATH`, the
startup model and effort, and whether a credential file is present.

Exit 0 means every adapter is healthy. Exit 1 lists each problem.

---

## 7. Attach

### From Windows

```powershell
wsl-toolkit --instance base base attach
```

`base attach` prints the exact line for this machine. Run that line. Nothing needs
starting first: the first connection starts the base, the base starts herdr's server,
and herdr puts the workspaces back.

⚠ **Use the client `base attach` prints.** It matches the server the base runs.

To leave, press the prefix, `ctrl+b`, then `q`. Every pane and agent keeps running.

### From inside the base

```powershell
wsl-toolkit --instance base base shell
```

Then run `herdr`.

⭐ **`base shell` gives you bash as a login shell** when the base has bash.

[`../common/herdr.md`](../common/herdr.md) is the full guide to both doors.

---

## 8. Drive an agent

Nothing has to be open. Make a workspace, read the pane id back, and start an agent in
it:

```powershell
$made = wsl-toolkit --instance base base herdr -- workspace create --cwd '~' --label work --no-focus | ConvertFrom-Json
$pane = $made.result.root_pane.pane_id
wsl-toolkit --instance base base herdr -- agent start muse --kind muse --pane $pane --timeout 120000
```

⚠ **Read the pane id from the command that made it.** The base may already hold
workspaces.

⚠ **`agent start` takes a NAME and a `--kind`.** The name must be free. A second agent
of the same kind needs another name.

Send it a turn, and read the answer:

```powershell
wsl-toolkit --instance base base herdr -- agent prompt work "Answer with only the number of files git tracks here."
wsl-toolkit --instance base base herdr -- agent read work --lines 40
```

Use `--kind pi` or `--kind omp` for the other two.

---

## 9. Run one prompt from Windows

Each agent adapter writes a launcher into `%USERPROFILE%\bin`:

```powershell
Set-Location C:\path\to\the\granted\project
muse exec --json "Answer with only the number of files git tracks here."
```

The launcher runs the agent in the base, at the guest path this directory is granted
at. Every argument is the agent's. The exit code is the agent's.

⛔ **An agent's own screen needs a terminal, so `muse` with no argument answers exit 2
from Windows.** Use a herdr pane for the screen.

---

## The model and the effort

The profile sets both for all three agents:

```json
{ "name": "muse", "model": "muse-spark-1.3-contributor", "effort": "max" }
```

**To change either, edit the configuration and run `base ensure` again.**

```powershell
wsl-toolkit --instance base base ensure
```

⭐ `effort` is `max` when a configuration names none.
[`../../wsl-toolkit.md`](../../wsl-toolkit.md) carries what each agent accepts, where
each value is written, and how to change it by hand.

⚠ **pi reaches the model through a named provider**, so its `model` is
`provider/id`. The other two name the model alone.

---

## What the profile chose, and why

| choice | reason |
| --- | --- |
| `arch` | the preset every adapter is measured on. A configuration naming one on another preset is refused |
| `automount: off`, `interop: off` | no Windows drive is reachable and no Windows executable runs. A grant is then the only door, and it is explicit |
| `passwordless_sudo: true` | a package install does not stop for a password nobody is there to type. ⚠ It is a **trust** decision about the agent, not a containment one |
| `systemd: true` | herdr's server is a system unit, so it starts with the base |
| `toolset: developer` | the commands the base installs as root, `jq` among them, which the adapters read their configuration with |
| no standing grant | the base is built once and each project is granted when it is worked on. A base left running reaches no checkout at all |
| `herdr` on `nightly` | `herdr --machine` needs a development build on both sides |

---

## The account's own tools

The base's toolset is installed as root into the image. ⭐ **The bootstrap is the other
half.** It adds the languages and CodeGraph into the account's home, which is what
persists with the base:

```powershell
wsl-toolkit --instance base base bootstrap -- --toolset agent
```

⭐ **Nothing is copied and nothing is fetched.** The executable carries the bootstrap.
`wsl-toolkit shipped list` prints its length and SHA-256, and `base bootstrap` sends
those exact bytes into the base and runs them as the account.

⛔ **Not as root.** Provider state, authentication and CodeGraph belong to the
unprivileged account. [`../common/README.md`](../common/README.md) carries what each
toolset name resolves to.

---

## When something is wrong

| what you see | what to do |
| --- | --- |
| an adapter reads a problem | read the line. It names the file and the command |
| `herdr agent start` times out | the agent is not on a pane's `PATH`. Run `base ensure` |
| pi starts on another provider's model | declare that model in `~/.pi/agent/models.json` |
| the effort reads `high` and you asked for `max` | add `thinkingLevelMap` to that model |
| a Windows launcher warns it is another build | run `base ensure` |
| the base was running when Windows restarted WSL | run `base ensure --repair` |

⭐ **Attack the base rather than trusting this page:**

```powershell
wsl-toolkit --instance base base doors
```

It runs 30 attacks as the unprivileged account and reports every door it found open.
[`../../wsl-toolkit.md`](../../wsl-toolkit.md) says what a zero-grant base does **not**
seal.
