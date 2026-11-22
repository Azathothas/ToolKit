# ag_profile

Runs the Antigravity desktop app on Windows as separate profiles, one per Google
account, from one PowerShell script.

⚠ **Windows only, and PowerShell 7.5 or later.** Any other host exits 2.
PowerShell 7.5 introduced `ConvertFrom-Json -DateKind`, preventing date-shaped
strings in profile notes from mutating across rewrites.

---

## What each profile isolates

| scope | default app | isolated profile |
| --- | --- | --- |
| Electron user data: `app_storage.json`, single-instance lock, cookies and logs | `%APPDATA%\Antigravity` | its own folder via `--user-data-dir` |
| profile home: `USERPROFILE`, `HOMEDRIVE` and `HOMEPATH` | the Windows logon home | `<userDataDir>\home`, with NTFS junctions to your real shell folders (`Desktop`, `Downloads`, etc.) and `.ssh` |
| agent data and global configuration: `antigravity/` and `config/` | `~/.gemini` | `<homeDir>\.gemini`, also exported as `ANTIGRAVITY_GEMINI_DIR` |
| global skills (`~/.agents/skills`, `~/.skills-manager/skills`, `~/.gemini/config/skills`) | `~/.agents` and `~/.gemini/config/skills` | shared live via NTFS junctions (and `skills.json`) unless `-IsolateSkills` is set |
| Google OAuth tokens | Windows Credential Manager (`gemini:antigravity`) | file-based token storage inside the profile via `SSH_CONNECTION` |
| process environment | the caller's environment | rebuilt from the registry like Explorer, with scrub patterns stripped |
| terminal console | inherited parent console | none, as [Launching](#launching) describes |

Each isolation layer has a per-profile opt-out: `-KeepUserProfile`,
`-ShareGeminiConfig`, `-UseSystemKeyring` and `-IsolateSkills`. Toggle them back
with `-IsolateUserProfile`, `-IsolateGeminiConfig`, `-IsolateKeyring` and
`-ShareSkills`.

## What all profiles share

These belong to the host or installed binary rather than to one profile.
`ag_profile doctor` reports the same items.

- `antigravity://` protocol links open whichever executable Windows registers,
  which targets the default profile. Browser OAuth sign-in returns over a local
  loopback callback, so isolated profiles still sign in through the browser.
- The installed binary and `%LOCALAPPDATA%\antigravity-updater` belong to the
  machine installation.
- `%APPDATA%\Antigravity\credential-user-scope` remains under real `APPDATA` so
  DPAPI, Windows crypto and spawned developer tools keep working.
- Windows Credential Manager (`gemini:antigravity`) is shared by `default` and
  any profile with `useSystemKeyring: true`. Concurrent accounts require the
  default file-based keyring isolation.
- Startup entries and pinned taskbar icons start the default profile. Create
  dedicated icons with `ag_profile shortcut NAME`.

---

## Install

```bash
pwsh -NoProfile -File scripts/ag_profile/ag_profile.ps1 install
```

This copies `ag_profile.ps1` into `~/bin` beside an `ag_profile.cmd` entry for
`cmd.exe` and the Run dialog. It never alters `PATH`, and reports whether
`~/bin` is already on `PATH`. Pass `-Destination DIR` to install elsewhere. It
also updates shortcuts owned by this configuration to point at the installed
copy.

Pass `install -AddToProfile` to register an `agp` alias and tab completion inside
a marked block of your PowerShell profile. Repeated runs replace that block in
place after saving a backup. A block that introduces parse errors is refused
before writing.

## Usage

```bash
ag_profile add work -Email WORK-EMAIL -Alias w
```

```bash
ag_profile w
```

| command | action |
| --- | --- |
| `ag_profile` or `ag_profile list` | show every profile, running PID, `.gemini` folder, keyring mode and app folder |
| `ag_profile NAME` | launch by profile name, alias or email (same as `launch NAME`) |
| `launch NAME -DryRun` | print the resolved command line, isolation settings and environment diff without starting |
| `launch NAME -Restart -Force` | stop a running instance (including tray and child `language_server.exe`) and relaunch |
| `launch NAME -AllowElevated` | permit launching from an administrator shell, which is blocked otherwise |
| `add [NAME] -Email E -Alias A` | register a profile; when `NAME` is omitted, derive one from `-Email` |
| `set NAME ...` | update `-Email`, `-Alias`, `-RemoveAlias`, `-SetEnv`, `-UnsetEnv`, `-Notes`, `-ProfileArgs`, folders or isolation flags |
| `alias NAME ALIAS`, `unalias ALIAS` | attach or remove profile aliases |
| `rename NAME NEW`, `remove NAME [-PurgeData]` | rename a profile and its shortcuts, or unregister it |
| `seed NAME [-SeedFrom default]` | copy global Antigravity settings from `config/` into a profile |
| `discover [-Register]` | locate unregistered Antigravity profile folders and running instances |
| `show NAME`, `status`, `path NAME` | inspect one profile, running instances with language servers, or folders (`-Json` supported) |
| `shortcut NAME` or `shortcut -All` | create Desktop and Start Menu launchers |
| `doctor`, `validate` | full diagnostics (including live isolation checks), or config validation alone |
| `edit`, `backup`, `restore [N]` | edit with pre-save validation, take a snapshot, or restore a backup |
| `help`, `version` | display command reference or script version |

`default` represents the standard Antigravity installation with no extra flags.
Assign metadata to it with `set default -Email E -Alias main`.

`remove` leaves profile directories intact. Adding `-PurgeData` unlinks every
junction inside the profile first, then moves each directory into `.ag-trash`
beside it instead of deleting bytes. Unlinking junctions before moving guarantees
`.ag-trash` never holds a link back to `Desktop`, `.agents` or shared skills.
Before moving anything, it checks that the profile is stopped, every directory
is safe to move, and the remaining configuration is valid. Folders move before
the configuration entry is removed so a partial failure never orphans data.

`seed` copies `config.json`, `mcp_config.json`, `skills.json`, `skills`,
`rules`, `GEMINI.md` and `AGENTS.md` from `<source>\config` by default. It
refuses `antigravity`, `projects`, `auth.json`, `credentials.json`,
`mcp_oauth_tokens.json` and other account or history entries. Exact on-disk
names are required, rejecting trailing dots, spaces or 8.3 aliases. Adding
`-Force` backs up existing items into `~/.ag_profiles.d/backups/seed` before
replacing them.

**Each shortcut is bound to one configuration file.** Every launcher passes
`-ConfigPath` explicitly. Commands modify or delete only shortcuts owned by the
active configuration file, and refuse to create shortcuts pointing into a
temporary directory.

---

## Configuration file

`~/.ag_profiles.json` is plain JSON designed for hand editing. Setting
`AG_PROFILES_CONFIG` or passing `-ConfigPath` relocates both the configuration
file and its companion `~/.ag_profiles.d` folder.

```json
{
  "version": 1,
  "root": "~/ag-profiles",
  "envMode": "clean",
  "scrubEnv": [
    "ANTIGRAVITY_*",
    "AGY_*",
    "GEMINI_*",
    "JETSKI_*",
    "ELECTRON_*",
    "NODE_OPTIONS",
    "DEV_URL",
    "CHROME_DEVTOOLS_MCP_*",
    "SSH_CONNECTION",
    "SSH_CLIENT",
    "SSH_TTY"
  ],
  "env": {},
  "app": { "exePath": null, "extraArgs": [] },
  "launch": { "verifySeconds": 25, "via": "cmd" },
  "profiles": {
    "default": { "email": "YOUR-EMAIL", "aliases": ["main"] },
    "work": {
      "email": "WORK-EMAIL",
      "aliases": ["w"],
      "userDataDir": "~/ag-profiles/work",
      "homeDir": "~/ag-profiles/work/home",
      "geminiDir": "~/ag-profiles/work/home/.gemini",
      "env": { "HTTPS_PROXY": "http://proxy.example.com:8080" },
      "args": []
    }
  }
}
```

| key | purpose |
| --- | --- |
| `root` | parent folder where `add` places new profiles |
| `envMode` | `clean` rebuilds variables from the Windows registry; `inherit` starts from the current shell |
| `scrubEnv` | additional wildcard patterns stripped before launch; built-in Antigravity, Electron and SSH patterns always apply |
| `env` (top-level and per-profile) | variables injected after scrubbing; `null` removes a variable |
| `app.exePath` | explicit `Antigravity.exe` path when bypassing automatic discovery |
| `launch.via` | `cmd` orphans the app through `cmd /c start`; `direct` skips that hop |
| `homeDir` / `keepUserProfile` | isolated `USERPROFILE` folder, or `keepUserProfile: true` to keep the real Windows home |
| `geminiDir` / `shareGeminiConfig` | isolated `.gemini` folder exported as `ANTIGRAVITY_GEMINI_DIR`, or `shareGeminiConfig: true` to share `~/.gemini` |
| `useSystemKeyring` | when `true`, skips `SSH_CONNECTION` keyring bypass and uses Windows Credential Manager |
| `isolateSkills` | when `true`, keeps the profile's skills separate instead of linking `~/.agents`, `~/.skills-manager` and `~/.gemini/config/skills` |

Validation runs before every write and blocks invalid saves:

- Names and aliases allow letters, digits, `.`, `_` and `-`, cannot end with a
  dot, and cannot collide with command names.
- Names, aliases and emails must be unique across profiles.
- Profile folders (`userDataDir`, `homeDir`, `geminiDir`) cannot overlap across
  profiles or nest backward (`userDataDir` inside `homeDir` or `geminiDir`).
- Folders under `AppData`, `LocalAppData`, `~/.gemini`, `~/.agents`,
  `WindowsApps`, `SystemRoot`, `~/.ag_profiles.d`, drive roots or UNC shares
  are refused, with `\\?\` and `\\.\` prefixes stripped first.
- Case-mismatched keys (such as `Root`) and case-colliding environment variable
  names are refused.
- Launch-owned variables (`ANTIGRAVITY_GEMINI_DIR`, `ANTIGRAVITY_APP_DATA_DIR`,
  `ANTIGRAVITY_CONFIG_REL_DIR`, `ELECTRON_NO_ATTACH_CONSOLE`,
  `ELECTRON_RUN_AS_NODE`, `USERPROFILE`, `HOMEDRIVE`, `HOMEPATH`,
  `SSH_CONNECTION`) cannot be set in `env`.

Errors scoped to one profile block only that profile's `launch` and `seed`.
Top-level errors block every profile. Writes require the entire file to be
valid.

⭐ **Configuration writes are locked, backed up and atomic.** Every writer
acquires a named mutex derived from the canonical configuration path, verifies
the file's SHA-256 hash has not changed since read, saves a rolling backup,
replaces the file with `WriteThrough` atomic replacement, and reads the bytes
back. Unchanged writes touch nothing. Symlinked configuration files stay
symlinks.

Canonical JSON rewrites strip comments, so commands refuse to modify a file with
comments unless `-Force` is passed. Unknown keys survive rewrites.
Configuration files declaring a newer schema version are read-only.

---

## Launching and future-proofing

**App discovery.** Every launch discovers `Antigravity.exe` dynamically in order:
explicit override (`-AppExe` or `app.exePath`), per-user install
(`%LOCALAPPDATA%\Programs\antigravity`), Uninstall registry (`HKCU`/`HKLM`),
`antigravity://` protocol handler, MSIX packages and AppModel registry,
`WindowsApps` execution aliases, Squirrel layouts, machine-wide installs, and
running processes. Every candidate must sit beside `resources\app.asar` (or
`resources\app`) so future CLI binaries are never mistaken for the desktop app.

**Process identification.** Antigravity runs main browser windows, Chromium
child processes (`--type=*`), Node sidecar workers (`--import=... main.mjs`),
and `language_server.exe`. `ag_profile` filters out both Chromium children and
Node sidecars when matching main profile instances, and links each main window
to its child `language_server.exe` PIDs.

⭐ **Detached launch in three layers:**

| layer | mechanism | protection |
| --- | --- | --- |
| no console | `ELECTRON_NO_ATTACH_CONSOLE=1` and `DETACHED_PROCESS` | prevents terminal log spam and exit on terminal close |
| no live parent | `cmd /c start` exits immediately | blocks tree kills (`taskkill /T`) of the terminal |
| no caller job | `CREATE_BREAKAWAY_FROM_JOB` where permitted | survives closing a kill-on-close job |

Arguments containing `%`, double quotes or line breaks bypass `cmd` and launch
directly without a console. If `cmd` has not exited after 15 seconds, the tool
warns without starting a second copy.

**Live isolation read-back.** After launch, `ag_profile` waits for the profile's
main process, confirms fresh writes inside `UserDataDir` and `GeminiDir`, checks
`<userDataDir>\logs\language_server.log` for `Using file-based token storage`
(catching a future Antigravity release that drops `SSH_CONNECTION` keyring
isolation before you sign into a second account), and probes console attachment
from a read-only child process. Set `launch.verifySeconds` to `0` or pass
`-NoVerify` to skip read-back.

**Already running instances** are focused without applying new environment
variables until `-Restart`. ⚠ **Instances attached to a terminal are flagged**
in `status`, `doctor` and focus output with the exact `-Restart -Force` command
to replace them.

**Elevated shells are refused** unless `-AllowElevated` is passed. `-Force` only
stops tray instances and never grants elevation.

---

## Limits

- Running instances started outside this tool cannot be detached in place; use
  `-Restart -Force` to relaunch them detached.
- Job objects that disallow breakaway are logged in `launch.log`.
- `clean` mode omits variables defined only in the calling shell; put needed
  variables into `env`.
- Because `pwsh -File` passes list options as one string, `-Alias`,
  `-RemoveAlias`, `-UnsetEnv`, `-Include` and positional alias lists split on
  commas. `-AppArgs` and `-ProfileArgs` are not split.
- `show` and `doctor` read `agy.onboarding.lastLoginUsername` from
  `app_storage.json` to display the signed-in account. No token or credential is
  ever read.
- When `homeDir` is isolated, terminals spawned inside that Antigravity profile
  inherit the profile's `USERPROFILE`. Pass `-KeepUserProfile` if you want
  agent terminals to keep your real home folder (noting Electron's `paths.js`
  then still reads `~/.gemini` while `language_server.exe` uses
  `ANTIGRAVITY_GEMINI_DIR`).

## Files

| path | purpose |
| --- | --- |
| `~/.ag_profiles.json` | profile definitions |
| `~/.ag_profiles.d/backups/` | newest 30 config snapshots, seed backups and PowerShell profile backups |
| `~/.ag_profiles.d/state.json` | disposable last-launch metadata |
| `~/.ag_profiles.d/launch.log` | launch, verification warning and error log (rotated at 1 MB) |
| `~/.ag_profiles.d/icons/antigravity.ico` | extracted shortcut icon independent of install path updates |

## Exit codes

0 succeeded, 1 refused or found an issue, 2 unsupported host or missing runtime.
Read `$LASTEXITCODE` directly without piping.

---

## Tests

```bash
pwsh -NoProfile -File scripts/ag_profile/tests/ag_profile.tests.ps1
```

Each case runs in a fresh process against an isolated configuration under
`.tmp/ag_profile-tests` and never touches `~/.ag_profiles.json`. Unit cases
dot-source the script in a child process and stub app discovery or process
creation so launch logic runs offline.

```bash
pwsh -NoProfile -File scripts/ag_profile/tests/ag_profile.tests.ps1 -Live -Mutate
```

⚠ **`-Live` launches the real Antigravity app with throwaway profiles** under
`.tmp/ag_profile-tests`, opening and closing windows on the desktop without
touching your default profile or Windows Credential Manager. Live cases verify
`UserDataDir`, `GeminiDir` and file-based keyring isolation, tree kills,
kill-on-close jobs, ConPTY terminal closure, and attached-console warnings.

`-Mutate` injects each guard's defect into a temporary copy of `ag_profile.ps1`
and requires the corresponding case to fail with the expected reason after
passing unmutated. The final line emits a machine-readable summary.
