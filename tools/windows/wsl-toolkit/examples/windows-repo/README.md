# A GitHub repository on Windows, an agent inside the base

You clone on Windows. An agent runs inside the base and edits the same files. You
commit and push on Windows. ⭐ **Nothing signs in inside WSL, and no credential and
no git identity is ever written there.**

Every command and every reading on this page was driven on Windows 11 Pro 26200,
WSL 2.7.12, against `wsl-toolkit-base`, on 2026-09-17.

---

## Why the usual WSL recipe does not work here

Most pages tell you to point git in WSL at the Windows credential helper:

```text
git config --global credential.helper "/mnt/c/Program Files/Git/.../git-credential-manager.exe"
```

⛔ **That cannot work on a base this tool builds.** The base runs with
`automount off` and `interop off`, so there is no `/mnt/c` and no way to start a
Windows program. Measured on the operator's base: `/mnt` holds `wsl` and `wslg`
and nothing else.

⭐ **The answer is better than the workaround.** git never enters the base at
all. The checkout lives on Windows, where your credentials already are, and the
base reaches it through one grant.

---

## 1. Clone on Windows

```powershell
cd $env:USERPROFILE\src
git clone https://github.com/OWNER/REPO.git
cd REPO
```

Your Windows git already has what this needs. Read it back rather than trusting
this page:

```powershell
git config --show-origin --get credential.helper
git config --get user.name
git config --get user.email
```

⚠ **A fresh Git for Windows sets `credential.helper manager` in
`C:/Program Files/Git/etc/gitconfig`**, which is a system file rather than yours.
If the first command prints nothing, no helper is configured and a push will ask
for a password every time.

---

## 2. Grant the clone to the base

```powershell
wsl-toolkit --instance base base grant --source . --mode rw
```

One Windows directory, mounted live under `/workspaces`. ⭐ **The grant is the
only door.** A directory no grant covers is refused, and the refusal is the design
working.

⛔ **Do not widen a grant to make a step pass.** Grant the one directory the work
needs.

---

## 3. Start the agent from Windows, and it runs inside the base

`base ensure` writes a launcher into `%USERPROFILE%\bin` for each **agent adapter
the configuration names**. The launcher is this tool under the agent's name. You
type the agent's name on Windows and the agent runs in the base, in the granted
directory:

```powershell
muse
```

⭐ **Run it from the clone.** The launcher reads the Windows directory you are in
and starts the agent at the matching guest path. From a directory with no grant it
refuses and prints the command that fixes it:

```text
muse: C:\...\REPO is not granted to wsl-toolkit-base, so an agent there cannot
see it. Grant it, then run the agent again:
  wsl-toolkit --instance base base grant --source C:\...\REPO --mode rw
```

⚠ **Check which program the name reaches before you trust it.** A native Windows
install of the same agent takes the name if its directory comes first on `PATH`:

```powershell
Get-Command muse, pi, omp | Select-Object Name, Source
```

⛔ **Measured on this host on 2026-09-17: `omp` resolved to a native Windows
install and not to the launcher.** `C:\ProgramData\scoop\persist\bun\bin` sat at
`PATH` position 3 and `%USERPROFILE%\bin` at position 66, and both files exist. The
native program answered `omp/18.1.19` while the base holds `omp/18.2.3`. A native
program cannot see the grant and cannot see the base. Run the launcher by its full
path when the name is taken:

```powershell
& "$env:USERPROFILE\bin\omp.exe"
```

---

## ⚠ Can a native Windows agent run its commands in the base instead?

Partly, and the measurement is why this repository does not ship the bridge.

⭐ **The seam exists.** A native Windows omp takes `shellPath`, and its shell is
called as `SHELL -c "COMMAND"`, which is the shape `base exec -c` already takes. A
shim that forwards one to the other, after moving to the guest path that matches
the Windows directory, was written and driven on 2026-09-17:

| driven | result |
| --- | --- |
| a relative command | exit 0. `pwd` answered `/workspaces/proj` and `uname -s` answered `Linux` |
| a command carrying a Windows absolute path | ⛔ exit 1, and the path arrived as `C:UsersAjamX...` with every backslash eaten as a shell escape |

⛔ **The second row is not a bug to fix. It is the shape of the idea.** A shim
cannot rewrite paths inside a command without guessing which strings are paths,
which is the defect that makes Git Bash unusable for this tool.

⛔ **And the shell is only part of what an agent does.** A native agent's file
tools - read, write, edit, grep, glob - are not shell commands. They keep running
on Windows against Windows paths while only the shell reaches the base, so one
agent holds two views of one tree. ⚠ **omp's own settings make this worse rather
than better**: `bashInterceptor.patterns` pushes it away from shell commands and
towards those native tools, so the more native the agent is, the less of its work
the shim reaches.

⭐ **The launcher has none of this**, because the whole agent runs in the base and
there is one view of the tree. That is why section 3 is the answer and this
section is a measurement.

---

## 4. Watch or steer the agent, without opening anything

```powershell
wsl-toolkit --instance base base attach
wsl-toolkit --instance base base herdr -- agent list
```

`base attach` prints the exact line for this machine and starts nothing.
`base herdr` sends one herdr command across the bridge, and everything after `--`
is herdr's own.

---

## 5. Commit and push on Windows

The agent has edited files in the clone. Windows sees every change at once,
because it is the same directory:

```powershell
git status
git add -A
git commit -m "what the agent changed"
git push
```

⭐ **Your identity and your credential are Windows'.** Nothing was configured in
WSL, and nothing needs to be.

---

## ⛔ The one sharp edge: the agent cannot commit

An agent that runs `git commit` **inside** the base fails. Measured on the
operator's base on 2026-09-17:

```text
commit rc=128
Author identity unknown
```

The base has no `user.name`, no `user.email` and no `credential.helper`, which is
the point of this arrangement. ⚠ **Tell the agent not to commit.** Ask it to leave
the work in the tree and do step 5 yourself.

⭐ **If you want the agent to commit**, give that one checkout an identity from
inside the base. It is a name and an address, not a secret:

```powershell
wsl-toolkit --instance base base exec -c 'cd /workspaces/REPO && git config user.name "NAME" && git config user.email "ADDRESS"'
```

⛔ **Do not give the base a credential.** A commit needs an identity and a push
needs a credential, and only the first of those is safe to put there. Push from
Windows.

---

## What the base can and cannot reach

| reading, 2026-09-17 | result |
| --- | --- |
| the granted directory, written by the base's account | writes succeed |
| who owns the checkout, as the base sees it | the base's own account, so git raises no ownership refusal |
| `git --version` in the base | 2.55.0 |
| `git ls-remote https://github.com/...` from the base | exit 0, so the base reaches GitHub |
| `user.name`, `user.email`, `credential.helper` in the base | none of the three |
| `/mnt` in the base | `wsl` and `wslg`, and no Windows drive |

⚠ **The base reaching GitHub is not the base being able to push.** A public read
needs no credential. A push needs one, and the base has none.

---

## Taking the grant away

```powershell
wsl-toolkit --instance base base revoke --target /workspaces/REPO
```

⛔ **`revoke` takes `--target` alone**, which is the path the base sees, and not
the Windows directory. The mount goes at once.
