---
name: wsl-toolkit
description: "Build and operate a named Linux base on Windows with wsl-toolkit: one WSL distribution the tool owns, with systemd, rootless Podman, a toolset, and Windows directories granted to it one at a time. Use when the user asks to make, inspect, repair, grant a directory to, or attack a wsl-toolkit base, or names wsl-toolkit, a wsl-toolkit instance, or wsl-toolkit-base. Do not use for ordinary WSL work that does not involve this tool."
---

# wsl-toolkit

`wsl-toolkit` is a single Windows executable. It owns one WSL distribution per
**instance** and does nothing to any other distribution.

⛔ **Never act on a WSL distribution this tool did not create.** `wsl --shutdown`,
`wsl --unregister` and `wsl --terminate` reach every distribution on the machine,
including the user's own work. Use the tool's own commands.

---

## 1. Get the tool, and keep it current

⛔ **Do not assume a version.** Ask:

```powershell
wsl-toolkit --version
```

If the command is not found, the executable is not on `PATH`. It is a single file;
the user downloads it from the repository's releases and puts it in a directory on
`PATH`, usually `%USERPROFILE%\bin`.

To move it to the newest release:

```powershell
wsl-toolkit selfupdate
```

⚠ **An old executable cannot read a newer configuration.** The refusal says so, and
names the field it does not know. `selfupdate` is the fix.

---

## 2. Read the manual before using a flag

⛔ **Never guess a flag, and never trust a flag list written on a page.** The manual
is generated from the commands the executable really has, so it is always current:

```powershell
wsl-toolkit man --no-pager
```

For one command:

```powershell
wsl-toolkit base --help
wsl-toolkit base exec --help
```

⭐ **Read the manual first when a command refuses something.** Its refusals name the
file, the field and the command that fixes it.

---

## 3. Two rules that cost a whole session when broken

⛔ **RUN IT FROM POWERSHELL, NOT GIT BASH.** MSYS rewrites a guest path: `--dir
/workspaces/proj` arrived inside the tool as `C:/Program Files/Git/workspaces/proj`.
Every example here is PowerShell.

⛔ **READ AN EXIT CODE FROM THE PROCESS, NOT THROUGH A PIPE.** `cmd | Select-Object`
gives you the pipeline's status, not the tool's. Read `$LASTEXITCODE` on the line
after the command.

---

## 4. The shape of a base

One base is one JSON configuration. The instance reads it from its own state
directory, so `--instance NAME` works from any directory:

```powershell
wsl-toolkit --instance base config
```

That prints the file in effect and what it resolves to. Start there whenever
something is unexpected.

| what the configuration says | what it means |
| --- | --- |
| `image` | the OCI image the distribution is built from. Adapters are measured on one preset only |
| `user` | the unprivileged Linux account everything runs as |
| `automount` / `interop` | whether Windows drives are mounted and Windows executables run. `off` for both makes a grant the only door |
| `toolset` | what the base installs as root |
| `mounts` | the grants. One Windows directory each |
| `adapters` | the software `base ensure` installs, in order |

---

## 5. The commands that do the work

```powershell
wsl-toolkit --instance base base ensure
```

Builds, provisions, installs every adapter, and prints the state. A second run
reconciles rather than rebuilding. ⭐ **It is the answer to most problems**: run it
after any configuration change.

```powershell
wsl-toolkit --instance base base status --probe
```

⛔ **`registered` is not `usable`.** Without `--probe` this reads the configuration.
With it, the tool runs a container and reads every adapter back from the machine.
Exit 0 means healthy; exit 1 lists each problem, and each problem names its fix.

```powershell
wsl-toolkit --instance base base grant --source . --mode rw
```

Grants **one** Windows directory, live, under `/workspaces`. Use `--mode ro` when the
work must not change it. ⛔ A directory no grant covers is refused with exit 2 and the
`base grant` line that would cover it. **That refusal is the design working. Do not
widen a grant to make a step pass** - report it instead.

To take one away:

```powershell
wsl-toolkit --instance base base revoke --target /workspaces/NAME
```

⛔ **`revoke` takes `--target` alone**, the path the base sees it at, not the Windows
directory. The mount goes at once and an empty directory is left behind, reaching
nothing.

```powershell
wsl-toolkit --instance base base exec -c 'COMMAND'
wsl-toolkit --instance base base exec --script LOCAL-FILE.sh
wsl-toolkit --instance base base shell
```

`base exec` runs one command as the account and returns its exit code. `--script`
sends a local file's bytes, which avoids every quoting problem. `base shell` opens an
interactive login shell.

⚠ **`base exec` runs with a CLEARED environment.** When the question is what a login
shell or an agent's pane really sees, run `runuser -l ACCOUNT -c '...'` inside it
instead. A guard proved on `base exec`'s curated environment has been wrong before.

```powershell
wsl-toolkit --instance base base doors
```

Attacks the base from the unprivileged account and reports every door it found open.
⛔ **Run this rather than believing any page about what a base cannot reach.**

---

## 6. When it is broken

| what you see | what to do |
| --- | --- |
| a field the tool does not know | the executable is older than the configuration. `wsl-toolkit selfupdate` |
| an adapter reports a problem | read the line. It names the file and the command |
| the base is registered and nothing works | `wsl-toolkit --instance NAME base ensure --repair` |
| Windows restarted WSL under it | the same `--repair`. It clears the stale boot id. ⛔ **`wsl --shutdown` breaks EVERY instance holding an engine**, so the remediation names the others and gives each one its command: repair the one that complained and the next session meets the rest |
| a Windows launcher says it is another build | `base ensure` rewrites it |
| `produced nothing for 4m and was given up` | ⭐ **a STALL, not a slow link.** Every host-engine call carries a total ceiling and a silence deadline, so a pull or an export that has stopped is given up in minutes rather than waited out. The line says where to look, and it differs for a pull and an export |

⭐ **The tool keeps a complete transcript of every job it ran:**

```powershell
wsl-toolkit logs
```

---

## 7. What this tool does not promise

⛔ **A base with no grants is not a sandbox, and the documentation never calls it
one.** `base doors` lists what stays open, and the manual's safety model names each
with the measurement beside it. Read both before telling anyone a base is sealed.

⚠ Two settings narrow it further and both cost something: `base.shared_tmpfs = "off"`
closes the shared `/mnt/wsl`, and `base exec --private-net` gives one command its own
network namespace with the Windows host refused. ⛔ **No container runs inside that
namespace**, which is why it is a flag and not how every command starts.

---

## 8. Where the detail lives

Everything above is the shape. The authority for behaviour is
`wsl-toolkit man --no-pager`, and it ships with the executable you are running.
