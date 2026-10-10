# cc_profile

Runs the Claude desktop app on Windows as several isolated profiles, one per
account, from one PowerShell script.

⚠ **Windows only, and PowerShell 7.5 or later.** On any other host it exits 2.
Version 7.5 added `ConvertFrom-Json -DateKind`. Without it, a note shaped like a
date is read back as a date, and the next write changes it.

---

## What a profile keeps apart

| what | the default app | a profile |
| --- | --- | --- |
| the app's own data: sign-in, settings, `claude_desktop_config.json` and its MCP servers | the app's default folder | its own folder, passed as `--user-data-dir` |
| Claude Code's settings, memory, sign-in and transcripts | `~/.claude` and `~/.claude.json` | its own folder, passed as `CLAUDE_CONFIG_DIR` |
| the environment the app starts with | whatever started it | rebuilt from the registry the way Explorer builds it, with the scrub patterns removed |
| the terminal it was started from | a console it may attach to | none, as [Launching](#launching) describes |

## What every profile still shares

These belong to the installed app, not to a profile, and no flag separates them.
`cc_profile doctor` prints the same list.

- The quick-entry hotkey, Ctrl+Alt+Space. The first instance to start owns it.
- `claude://` links, including the hand-off at the end of a browser sign-in.
  Windows gives them to the default profile, so sign in to another profile with
  the email code.
- Claude in Chrome, which pairs with one instance at a time.
- The Cowork service, `CoworkVMService`, which every profile uses.
- "Run at startup" and the pinned taskbar icon, which start the default profile.
  `cc_profile shortcut NAME` makes an icon for a profile.

---

## Install

```bash
pwsh -NoProfile -File scripts/cc_profile/cc_profile.ps1 install
```

It copies the script to `~/bin`, with a `.cmd` file beside it for `cmd.exe` and
the Run dialog. It does not edit `PATH`; it says whether `~/bin` is on it.
`-Destination DIR` installs somewhere else. It also points this configuration's
existing shortcuts at the installed copy.

`install -AddToProfile` also writes a `ccp` alias with tab completion into the
PowerShell profile, inside a marked block. A second run replaces that block
rather than adding one, and the old profile file is backed up first. The profile
is not written when the new text would not parse, because a profile that does not
parse stops every new shell from loading it.

## Use

```bash
cc_profile add work -Email WORK-EMAIL -Alias w
```

```bash
cc_profile w
```

| command | what it does |
| --- | --- |
| `cc_profile` or `cc_profile list` | every profile, whether it runs, and its two folders |
| `cc_profile NAME` | launch by name, alias or email. The same as `launch NAME` |
| `launch NAME -DryRun` | print the exact command line and environment, and start nothing |
| `launch NAME -Restart -Force` | end a running instance, even one in the tray, and start it again |
| `launch NAME -AllowElevated` | launch from an elevated shell, which is refused otherwise |
| `add [NAME] -Email E -Alias A` | create a profile. Without NAME, the name comes from the email |
| `set NAME ...` | change a profile: `-Email`, `-Alias`, `-RemoveAlias`, `-SetEnv NAME=VALUE`, `-UnsetEnv`, `-Notes`, `-ProfileArgs`, a folder |
| `alias NAME ALIAS`, `unalias ALIAS` | add or remove aliases |
| `rename NAME NEW`, `remove NAME [-PurgeData]` | rename a profile with its shortcuts, or unregister it |
| `seed NAME [-SeedFrom default]` | copy Claude Code settings into a profile |
| `discover [-Register]` | find profile folders and running instances that are not registered |
| `show NAME`, `status`, `path NAME` | details, running instances, and folders. `-Json` for a value a script reads |
| `shortcut NAME` or `shortcut -All` | Desktop and Start Menu icons that launch a profile |
| `doctor`, `validate` | a full health check, and a check of the config file alone |
| `edit`, `backup`, `restore [N]` | change the config safely, or put a backup back |
| `help`, `version` | the command reference, and the version |

`default` is the app as Windows starts it, with no folder of its own.
`set default -Email E -Alias A` names it, so `cc_profile A` reaches it too.

`remove` keeps the profile's folders. `-PurgeData` moves them into a `.cc-trash`
folder beside them rather than deleting them. It checks everything before it moves
anything: the instance is closed, each folder is the profile's alone, and the
change can be saved. The folders move before the entry goes, so a move that fails
leaves the entry that names what is left.

`seed` copies `CLAUDE.md`, `settings.json`, `keybindings.json`, `agents`,
`commands`, `skills` and `output-styles` by default. It refuses
`.credentials.json`, `.claude.json`, `projects` and the other names on its
refusal list, which hold a sign-in or a history. A name is taken only as it is
spelled on disk: Windows opens `.credentials.json.` as `.credentials.json`, so a
trailing dot or space is refused. `-Force` replaces what is there, after copying
the old version into `~/.cc_profiles.d/backups`.

**Shortcuts belong to one configuration.** Each one names its configuration file
with `-ConfigPath`. A command changes or removes only its own configuration's
shortcuts, and refuses to overwrite one it did not make. A shortcut to a copy of
the script in a temporary folder is refused, because it breaks when that folder
is cleaned.

---

## The configuration file

`~/.cc_profiles.json` is yours to edit. `CC_PROFILES_CONFIG` or `-ConfigPath`
moves it, and `~/.cc_profiles.d` moves with it.

```json
{
  "version": 1,
  "root": "~/claude-profiles",
  "envMode": "clean",
  "scrubEnv": ["CLAUDE*", "ANTHROPIC*", "ELECTRON_*", "NODE_OPTIONS"],
  "env": {},
  "app": { "exePath": null, "extraArgs": [] },
  "launch": { "verifySeconds": 20, "via": "cmd" },
  "profiles": {
    "default": { "email": "YOUR-EMAIL", "aliases": ["main"] },
    "work": {
      "email": "WORK-EMAIL",
      "aliases": ["w"],
      "userDataDir": "~/claude-profiles/work",
      "claudeConfigDir": "~/claude-profiles/work/.claude",
      "env": { "HTTPS_PROXY": "http://proxy.example.com:8080" },
      "args": []
    }
  }
}
```

| key | means |
| --- | --- |
| `root` | where `add` puts a new profile's folders |
| `envMode` | `clean` rebuilds the environment from the registry. `inherit` starts from the shell's own |
| `scrubEnv` | more variable names, wildcards allowed, that never reach the app. `CLAUDE*`, `ANTHROPIC*`, `ELECTRON_*` and `NODE_OPTIONS` always apply, whatever this list says |
| `env`, and a profile's `env` | variables to set for every launch, or for one profile. `null` removes one |
| `app.exePath` | the app to start, when discovery must be skipped |
| `launch.via` | `cmd` starts the app through `cmd /c start`, and `direct` without that hop |
| a profile's `claudeConfigDir` | its Claude Code folder. Without it, `.claude` inside `userDataDir` |
| a profile's `shareClaudeConfig: true` | use `~/.claude`, which the default profile also uses |

Every write is checked against these rules, and an invalid configuration is never saved:

- A name or alias is letters, digits, `.`, `_` and `-`, does not end with a dot,
  and is not a command name.
- No name, alias or email belongs to two profiles.
- No profile folder overlaps another profile's folders.
- No profile folder is under `AppData`: the app is MSIX-packaged, so Windows
  redirects its writes there and the app and this script would see different
  folders.
- No profile folder is a drive root or a network path, or overlaps `~/.claude`,
  the Windows folder, the app's install folder or `~/.cc_profiles.d`. The home
  folder holds `~/.claude`, so it is refused too.
- A key spelled like a known key in another case, such as `Root`, is refused.
  Keys are case-sensitive, and that one would be ignored.
- Two variable names that differ only in case are refused, because Windows reads
  them as one variable.
- `CLAUDE_CONFIG_DIR`, `CLAUDE_USER_DATA_DIR` and `ELECTRON_NO_ATTACH_CONSOLE`
  belong to the launch, and a configuration cannot set them.

An error in one profile stops only that profile's launch and `seed`. An error in the
shared settings stops every launch. A write needs the whole file valid.

⭐ **A write cannot lose an edit or half-write a file.** It takes a lock that is
named for the configuration file. It reads, changes and validates, and refuses when the
file changed on disk since the read. It backs up the old file, replaces it
atomically, and reads it back. A change that changes nothing writes nothing.
`edit` validates a copy before it puts the copy in place. A symlinked configuration
stays a symlink.

A rewrite is canonical JSON, so a comment would be lost. A command refuses to
change a file that holds one until the comment is taken out, or `-Force` says to
rewrite it. A key this version does not know is kept. A configuration of a newer
schema is read, and never written.

---

## Launching

**Finding the app.** Each launch finds the app again. It takes `app.exePath` or
`-AppExe` when one is set. Then it tries the installed package, the package
registry, the `claude-desktop.exe` alias, a Squirrel, per-user or machine-wide
install, and last a running instance. An update that moves the install folder
needs nothing.

**The environment.** `clean` mode copies the machine variables, the logon
session's variables and the user's variables, in that order. The user's `PATH`
goes after the machine's. It then removes the scrub patterns and adds the
configuration's `env` and then the profile's. Last, it sets
`ELECTRON_NO_ATTACH_CONSOLE=1` and the profile's `CLAUDE_CONFIG_DIR`.

⭐ **Detached, which has three parts:**

| part | how | what it stops |
| --- | --- | --- |
| no console | `ELECTRON_NO_ATTACH_CONSOLE=1`, and the process is created without one | the app printing its log into the terminal, and the app ending when the terminal closes |
| no parent | `cmd /c start`, and that `cmd.exe` exits at once | a tree kill of the terminal, such as `taskkill /T` or "End process tree" |
| no job | creation with breakaway, where the caller's job permits it | a terminal that closes a kill-on-close job |

Through `cmd`, every argument is quoted. An argument holding `%`, a double quote or
a line break cannot pass through `cmd`, so that launch goes direct: no console,
but a child of the launching process. `launch.via = direct` does the same for
every launch. When `cmd` has not finished after 15 seconds, the launch does not
start the app a second time. It says so, and the check below shows whether the
app came up.

**Verification.** After a launch it waits for a main process of the profile. For
a profile with a folder of its own, it also waits for the app to write into that
folder. Then it reads the instance's console attachment back from the running
process. One it cannot read is named, not passed as fine. `verifySeconds = 0`
turns this off.

**An instance that is already running** is brought to the front, and the new
environment does not apply until `-Restart`. ⚠ **An instance attached to a
terminal is named, with the command that replaces it.** Something other than
this tool started it, and it ends when that terminal closes. `status` shows it in
its Console column. `doctor` fails on it, as it does on an instance whose console
it cannot read.

**An elevated shell is refused** unless `-AllowElevated` is given, so Claude does
not run as administrator by accident. `-Force` does not allow it: `-Force` only
ends an instance that went to the tray.

---

## What it does not do

- It cannot detach an instance that something else started. It finds one and
  names it, and `-Restart -Force` replaces it.
- Some jobs forbid breakaway. A launch inside one says so in `launch.log`. The
  suite does not assert whether the app outlives such a job.
- The suite ends the app by ending its process. It never drives the app's own
  Quit command.
- `ELECTRON_NO_ATTACH_CONSOLE=1` reaches every process the app starts. An
  Electron app started from a terminal inside it does not attach to that
  terminal either.
- `clean` mode drops a variable that only the launching shell set. Put such a
  variable in `env`.
- Through `pwsh -File` or the `.cmd`, a list is one string. `-Alias`,
  `-RemoveAlias`, `-UnsetEnv`, `-Include`, and the words after `alias` and
  `unalias`, are split on commas. `-AppArgs` and `-ProfileArgs` take one argument
  that way, because an argument can hold a comma.
- `status`, `doctor`, the check after a launch, and a launch that finds an
  instance running each start a short-lived PowerShell process to read console
  attachment. That process attaches to the console it reads, and writes nothing
  to it.
- `show` and `doctor` read `oauthAccount` from a profile's `.claude.json` to say
  which account Claude Code uses. Nothing reads a credential.

## Files

| path | holds |
| --- | --- |
| `~/.cc_profiles.json` | the profiles |
| `~/.cc_profiles.d/backups/` | the newest 30 configuration backups, and a PowerShell profile from before `-AddToProfile` changed it |
| `~/.cc_profiles.d/state.json` | the last launch of each profile. Losing it loses nothing else |
| `~/.cc_profiles.d/launch.log` | one line per launch, per warning from a launch, and per error |
| `~/.cc_profiles.d/icons/claude.ico` | the shortcut icon, copied out of the install folder because that folder moves on every update |

## Exit codes

0 it did it, 1 it refused or found a problem, 2 it could not run on this host.
Read the exit code from the process, unpiped.

---

## Tests

```bash
pwsh -NoProfile -File scripts/cc_profile/tests/cc_profile.tests.ps1
```

Each case runs the script as its own process, against its own configuration under
`.tmp/cc_profile-tests`. No case reads `~/.cc_profiles.json`. A unit case loads
the script into a child process and stands in for the functions that find or
start the app. So the launch logic runs offline, and the app is not started.
Several cases need the Claude app installed, which is why the suite is not part
of the gate.

```bash
pwsh -NoProfile -File scripts/cc_profile/tests/cc_profile.tests.ps1 -Live -Mutate
```

⚠ **`-Live` starts the real app with throwaway profiles**, so windows open and
close on the desktop for several minutes. Those cases close a terminal tab built
on a pseudo-console, the way Windows Terminal builds one. They kill a terminal's
process tree, and close a kill-on-close job around it. They also start the app
straight from a shell, and require every command to name it before closing the
tab ends it.

`-Mutate` puts each guard's defect into a copy of the script. The case named for
that guard must then fail, after it passed unmutated, and for the reason the row
names. A failure for another reason is counted as misfired. A row whose case did
not run is counted as unrun, which is how a live row reads without `-Live`. The
last line is a fixed summary that a script can read.
