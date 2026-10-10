#Requires -Version 7.5
<#
.SYNOPSIS
    Run the Claude desktop app as several isolated profiles, one per account.

.DESCRIPTION
    WHAT GOES WRONG WITHOUT IT. A second desktop instance started with only
    --user-data-dir still shares ~/.claude and ~/.claude.json with the first, so
    Claude Code settings, memory and session transcripts of two accounts mix. It
    also inherits whatever the launching shell carries: started from a Claude Code
    terminal, the new app receives that session's CLAUDE_* and ANTHROPIC_*
    variables and its crash-reporter pipe, and it can die with the terminal.

    WHAT IT DOES. Each profile has its own Electron user-data directory and its
    own CLAUDE_CONFIG_DIR. A launch builds the environment from the registry, the
    way Explorer does, strips the scrub patterns, and starts the app so that it
    has no console, no live parent and no job of the caller's. It then reads back
    that the app wrote into the profile's own directory and has no console.

    WINDOWS ONLY, POWERSHELL 7.5 OR LATER. On any other host it exits 2. 7.5 is
    the first with ConvertFrom-Json -DateKind; before it, a value shaped like a
    date in the config is read back as a date and the next write corrupts it.

    Profiles live in ~/.cc_profiles.json, which is hand-editable and validated on
    every write. State, backups, icons and the launch log live in ~/.cc_profiles.d.
    scripts/cc_profile/README.md is the reference; "cc_profile help" lists commands.

.EXAMPLE
    cc_profile add work -Email WORK-EMAIL -Alias w

.EXAMPLE
    cc_profile w

.EXAMPLE
    cc_profile doctor

.NOTES
    Exit codes: 0 it did it, 1 it refused or found a problem, 2 it could not run.
    Through pwsh -File a list parameter arrives as one string, so -Alias,
    -RemoveAlias, -UnsetEnv and -Include split their own value on commas.

    SPDX-License-Identifier: 0BSD
#>
[CmdletBinding(SupportsShouldProcess, ConfirmImpact = 'Medium', PositionalBinding = $false)]
param(
    [Parameter(Position = 0)][string]$Command,
    [Parameter(Position = 1)][string]$Target,
    [Parameter(Position = 2, ValueFromRemainingArguments)][string[]]$Rest,

    [string]$Email,
    [string[]]$Alias,
    [string[]]$RemoveAlias,
    [string]$UserDataDir,
    [string]$ClaudeConfigDir,
    [switch]$ShareClaudeConfig,
    [switch]$IsolateClaudeConfig,
    [object[]]$SetEnv,
    [string[]]$UnsetEnv,
    [string[]]$ProfileArgs,
    [string]$Notes,
    [string]$SeedFrom,
    [string[]]$Include,
    [string[]]$AppArgs,
    [ValidateSet('Desktop', 'StartMenu', 'Both')][string]$Location = 'Both',
    [string]$Destination,
    [string]$FunctionName = 'ccp',
    [string]$AppExe,
    [string]$ConfigPath,

    [switch]$Restart,
    [switch]$Force,
    [switch]$DryRun,
    [switch]$NoVerify,
    [switch]$Json,
    [switch]$PurgeData,
    [switch]$Remove,
    [switch]$AddToProfile,
    [switch]$All,
    [switch]$Register,
    [switch]$AllowElevated,
    [switch]$FromShortcut
)

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$script:ToolName = 'cc_profile'
$script:ToolVersion = '1.0.0'
$script:SchemaVersion = 1
$script:Bound = $PSBoundParameters
$script:MachineOutput = $Json.IsPresent
$script:TraceOn = $VerbosePreference -ne 'SilentlyContinue'
$script:AppCache = $null
$script:ConfigFile = $null
$script:DataDir = $null
$script:ScriptPath = $PSCommandPath
$script:Tgt = $Target
$script:RestArgs = @()
$script:EnvSettings = [ordered]@{}
$script:ExitCode = 0

$script:CommandMap = @{
    'list' = 'list'; 'ls' = 'list'
    'show' = 'show'; 'info' = 'show'
    'add' = 'add'; 'new' = 'add'; 'create' = 'add'
    'set' = 'set'; 'update' = 'set'
    'alias' = 'alias'; 'unalias' = 'unalias'
    'rename' = 'rename'; 'mv' = 'rename'
    'remove' = 'remove'; 'rm' = 'remove'; 'delete' = 'remove'
    'launch' = 'launch'; 'run' = 'launch'; 'open' = 'launch'; 'start' = 'launch'
    'status' = 'status'; 'ps' = 'status'
    'doctor' = 'doctor'; 'check' = 'doctor'
    'validate' = 'validate'
    'edit' = 'edit'
    'backup' = 'backup'; 'restore' = 'restore'
    'seed' = 'seed'
    'shortcut' = 'shortcut'
    'discover' = 'discover'; 'adopt' = 'discover'
    'install' = 'install'
    'path' = 'path'
    'help' = 'help'; '?' = 'help'
    'version' = 'version'
}
$script:ReservedNames = @($script:CommandMap.Keys) + @('all', 'none')

# \z, not $: in .NET, $ also matches before a final line feed, so "name`n" passed.
# A name does not end in a dot, which Windows drops from a folder name.
$script:NameRx = '^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,62}[A-Za-z0-9_-])?\z'
$script:EmailRx = '^[^@\s"<>]+@[^@\s"<>]+\.[^@\s"<>]+\z'
$script:EnvKeyRx = '^[A-Za-z_][A-Za-z0-9_]*\z'
# Set by every launch, never by a config. ELECTRON_NO_ATTACH_CONSOLE stops Electron
# from attaching to its parent's console, where it would print its log and be ended
# when that terminal closes.
$script:ManagedEnv = @('CLAUDE_CONFIG_DIR', 'CLAUDE_USER_DATA_DIR', 'ELECTRON_NO_ATTACH_CONSOLE')
# Removed from every launch, whatever the config says; a config's scrubEnv adds to it.
$script:DefaultScrub = @('CLAUDE*', 'ANTHROPIC*', 'ELECTRON_*', 'NODE_OPTIONS')

# The keys this version knows, in the order a write puts them. Other keys are kept.
$script:TopKeys = @('version', 'root', 'envMode', 'scrubEnv', 'env', 'app', 'launch', 'profiles')
$script:AppKeys = @('exePath', 'extraArgs')
$script:LaunchKeys = @('verifySeconds', 'via')
$script:ProfileKeys = @('email', 'aliases', 'userDataDir', 'claudeConfigDir', 'shareClaudeConfig', 'env', 'args', 'notes')

# Variables Windows synthesizes per logon session; they are not in the registry
# environment, so a "clean" launch copies them from the current process.
$script:SessionEnvVars = @(
    'ALLUSERSPROFILE', 'APPDATA', 'CommonProgramFiles', 'CommonProgramFiles(x86)', 'CommonProgramW6432',
    'COMPUTERNAME', 'DriverData', 'HOMEDRIVE', 'HOMEPATH', 'LOCALAPPDATA', 'LOGONSERVER', 'ProgramData',
    'ProgramFiles', 'ProgramFiles(x86)', 'ProgramW6432', 'PUBLIC', 'SESSIONNAME', 'CLIENTNAME', 'SystemDrive',
    'SystemRoot', 'USERDNSDOMAIN', 'USERDOMAIN', 'USERDOMAIN_ROAMINGPROFILE', 'USERNAME', 'USERPROFILE', 'windir'
)

# Claude Code config items that are safe to copy between profiles, and ones that never are.
$script:SeedDefault = @('CLAUDE.md', 'settings.json', 'keybindings.json', 'agents', 'commands', 'skills', 'output-styles')
$script:SeedBlocked = @('.credentials.json', '.claude.json', 'projects', 'sessions', 'session-env', 'shell-snapshots',
    'todos', 'statsig', 'file-history', 'history.jsonl', 'ide', 'debug', 'telemetry', 'plans', 'paste-cache', 'cache')

#region ---------------------------------------------------------------- output

# Every line a person reads goes through Write-Display; only a VALUE a caller
# consumes (-Json, path) goes to the output stream.
#
# The information stream rather than the output stream, so a caller that
# captures a command's result does not capture its messages. UNDER -Json the
# messages go to stderr instead: run through pwsh -File, the information stream
# is written to stdout, and a status line ahead of the JSON corrupts the value.
function Write-Display([string]$Text = '', [string]$Color, [switch]$NoNewLine) {
    if ($script:MachineOutput) {
        if ($NoNewLine) { [Console]::Error.Write($Text) } else { [Console]::Error.WriteLine($Text) }
        return
    }
    $msg = [Management.Automation.HostInformationMessage]@{ Message = $Text; NoNewLine = $NoNewLine.IsPresent }
    if ($Color) { $msg.ForegroundColor = [ConsoleColor]$Color }
    Write-Information -MessageData $msg -InformationAction Continue
}
function Write-Line([string]$Message = '', [string]$Color) { Write-Display $Message $Color }
function Write-Ok([string]$Message) { Write-Display '[ ok ] ' 'Green' -NoNewLine; Write-Display $Message }
function Write-Note([string]$Message) { Write-Display '[info] ' 'Cyan' -NoNewLine; Write-Display $Message }
function Write-Warn([string]$Message) { Write-Display '[warn] ' 'Yellow' -NoNewLine; Write-Display $Message }
function Write-Bad([string]$Message) { Write-Display '[FAIL] ' 'Red' -NoNewLine; Write-Display $Message }

# Verbose detail. Under -Json it goes to stderr, for the reason Write-Display does,
# and Invoke-Main silences every cmdlet's own verbose output there too.
function Write-Trace([string]$Message) {
    if (-not $script:TraceOn) { return }
    if ($script:MachineOutput) { [Console]::Error.WriteLine("VERBOSE: $Message") } else { Write-Verbose $Message -Verbose }
}

# Exit 2 is "this host cannot run it", which is a different fact from a refusal.
function Stop-CannotRun([string]$Message) {
    $script:ExitCode = 2
    throw $Message
}

# Through pwsh -File, "-Alias a,b" arrives as the one string "a,b". Only lists
# whose items cannot contain a comma are split; -AppArgs and -ProfileArgs are not.
function Split-ListValue([string[]]$Values) {
    return @(foreach ($v in @($Values)) { foreach ($part in ([string]$v).Split(',')) { if ($part.Trim()) { $part.Trim() } } })
}

# -SetEnv takes NAME=VALUE strings, which survive pwsh -File, or a hashtable in-process.
function ConvertTo-EnvSettings([object[]]$Values) {
    $out = [ordered]@{}
    foreach ($v in @($Values)) {
        if ($null -eq $v) { continue }
        if ($v -is [Collections.IDictionary]) {
            foreach ($k in $v.Keys) { $out[[string]$k] = if ($null -eq $v[$k]) { $null } else { [string]$v[$k] } }
            continue
        }
        $s = [string]$v
        $eq = $s.IndexOf('=')
        if ($eq -lt 1) { throw "-SetEnv takes NAME=VALUE (or a hashtable in-process); got '$s'." }
        $out[$s.Substring(0, $eq)] = $s.Substring($eq + 1)
    }
    return $out
}

function Format-Secret([string]$Key, [string]$Value) {
    if ($Key -match '(TOKEN|SECRET|PASSWORD|PASSWD|API_?KEY|CREDENTIAL|AUTH)') { return '<redacted>' }
    return $Value
}

#endregion
#region ---------------------------------------------------------------- paths

function Expand-UserPath([string]$Path) {
    if ([string]::IsNullOrWhiteSpace($Path)) { return $null }
    $p = $Path.Trim()
    if ($p -eq '~') { $p = $HOME }
    elseif ($p -match '^~[\\/]') { $p = [IO.Path]::Combine($HOME, $p.Substring(2)) }
    return [Environment]::ExpandEnvironmentVariables($p)
}

# [IO.Path] throughout, never Join-Path: Join-Path asks the provider, and a path on
# a drive that is not mounted throws "Cannot find drive" instead of being judged.
#
# The \\?\ and \\.\ prefixes are stripped, so "\\?\C:\Windows\x" is compared as
# the C:\ path it names. Without that, every rule about a folder could be passed
# by spelling the folder another way.
function Get-FullPath([string]$Path, [string]$What = 'Path') {
    $p = Expand-UserPath $Path
    if (-not $p) { return $null }
    if ($p -match '^[\\/]{2}[?.][\\/]UNC[\\/](.+)$') { $p = '\\' + $Matches[1] }
    elseif ($p -match '^[\\/]{2}[?.][\\/](.+)$') { $p = $Matches[1] }
    if (-not [IO.Path]::IsPathFullyQualified($p)) { throw "$What must be absolute or start with '~': '$Path'" }
    $full = [IO.Path]::GetFullPath($p)
    if ($full.Length -gt 3) { $full = $full.TrimEnd('\', '/') }
    return $full
}

# A profile folder is on a local drive letter: an Electron profile on a network
# share is not something this tool can promise anything about.
function Test-LocalDrivePath([string]$FullPath) { return [bool]($FullPath -match '^[A-Za-z]:\\') }

# Store paths under the home folder as "~/..." so the config stays portable and readable.
function Compress-UserPath([string]$Path) {
    if (-not $Path) { return $Path }
    $h = [IO.Path]::GetFullPath($HOME).TrimEnd('\')
    if ($Path.Equals($h, [StringComparison]::OrdinalIgnoreCase)) { return '~' }
    if ($Path.StartsWith($h + '\', [StringComparison]::OrdinalIgnoreCase)) {
        return '~/' + $Path.Substring($h.Length + 1).Replace('\', '/')
    }
    return $Path
}

function Test-PathEqual([string]$A, [string]$B) {
    if (-not $A -or -not $B) { return $false }
    return [string]::Equals($A.TrimEnd('\'), $B.TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase)
}
function Test-PathWithin([string]$Child, [string]$Parent) {
    if (-not $Child -or -not $Parent) { return $false }
    return $Child.StartsWith($Parent.TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase)
}
function Test-PathOverlap([string]$A, [string]$B) {
    return (Test-PathEqual $A $B) -or (Test-PathWithin $A $B) -or (Test-PathWithin $B $A)
}

# Places a profile directory must never be, or contain.
function Get-ProtectedDirs {
    $out = [Collections.Generic.List[object]]::new()
    $add = {
        param($p, $why)
        if ($p) { $out.Add([pscustomobject]@{ Path = [IO.Path]::GetFullPath($p).TrimEnd('\'); Why = $why }) }
    }
    & $add ([IO.Path]::Combine($HOME, '.claude')) "the default Claude Code config (~/.claude)"
    & $add $env:APPDATA "AppData\Roaming (MSIX virtualizes writes there, so the app and this script would see different folders)"
    & $add $env:LOCALAPPDATA "AppData\Local (MSIX virtualizes writes there, so the app and this script would see different folders)"
    if ($env:ProgramFiles) { & $add ([IO.Path]::Combine($env:ProgramFiles, 'WindowsApps')) "the app's install folder" }
    if ($env:SystemRoot) { & $add $env:SystemRoot "the Windows folder" }
    if ($script:DataDir) { & $add $script:DataDir "this tool's own data folder" }
    return $out
}

# Every read of a file another process may replace goes through here. It shares
# read, write AND delete: File.ReadAllBytes shares read only, and a reader holding
# the file that way makes a writer's File.Replace fail with "Unable to remove the
# file to be replaced".
function Read-SharedBytes([string]$Path) {
    $fs = [IO.FileStream]::new($Path, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete)
    try {
        $ms = [IO.MemoryStream]::new()
        $fs.CopyTo($ms)
        # The comma keeps the byte[] whole; without it every byte is a pipeline item.
        # Callers use the result as it is and never wrap it in @().
        return , $ms.ToArray()
    }
    finally { $fs.Dispose() }
}

function Get-FileSha256([string]$Path) {
    return [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData((Read-SharedBytes $Path)))
}

# Retries an IO action that a scanner or an indexer holding the file can make fail
# for a moment. Anything still failing after about three seconds is real.
function Invoke-IoRetry([scriptblock]$Action) {
    for ($attempt = 1; ; $attempt++) {
        try { & $Action; return }
        catch [IO.IOException], [UnauthorizedAccessException] {
            if ($attempt -ge 15) { throw }
            Start-Sleep -Milliseconds (50 * $attempt)
        }
    }
}

function Write-FileAtomic([string]$Path, [string]$Text) {
    $dir = [IO.Path]::GetDirectoryName($Path)
    [void][IO.Directory]::CreateDirectory($dir)
    $tmp = [IO.Path]::Combine($dir, ('.{0}.{1}.tmp' -f [IO.Path]::GetFileName($Path), [guid]::NewGuid().ToString('N')))
    try {
        $bytes = [Text.UTF8Encoding]::new($false).GetBytes($Text)
        $fs = [IO.FileStream]::new($tmp, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None, 4096, [IO.FileOptions]::WriteThrough)
        try { $fs.Write($bytes, 0, $bytes.Length); $fs.Flush($true) } finally { $fs.Dispose() }
        # [NullString]: PowerShell would otherwise pass $null as "" and Replace rejects an empty backup path.
        Invoke-IoRetry {
            if ([IO.File]::Exists($Path)) { [IO.File]::Replace($tmp, $Path, [NullString]::Value, $true) }
            else { [IO.File]::Move($tmp, $Path) }
        }
    }
    finally {
        # The rollback half of this write: the temporary it created above, nothing else.
        if ([IO.File]::Exists($tmp)) { Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue }
    }
}

#endregion
#region ---------------------------------------------------------------- command lines

# Splits a Windows command line the way the MSVC runtime / CommandLineToArgvW does.
function Split-CommandLine([string]$Line) {
    $argv = [Collections.Generic.List[string]]::new()
    if (-not $Line) { return @() }
    $sb = [Text.StringBuilder]::new()
    $inQuotes = $false; $hasToken = $false; $i = 0
    while ($i -lt $Line.Length) {
        $c = $Line[$i]
        if ($c -eq [char]'\') {
            $n = 0
            while ($i -lt $Line.Length -and $Line[$i] -eq [char]'\') { $n++; $i++ }
            if ($i -lt $Line.Length -and $Line[$i] -eq [char]'"') {
                [void]$sb.Append([char]'\', [int][Math]::Floor($n / 2))
                if ($n % 2 -eq 1) { [void]$sb.Append([char]'"'); $i++ }
            }
            else { [void]$sb.Append([char]'\', $n) }
            $hasToken = $true
            continue
        }
        if ($c -eq [char]'"') {
            if ($inQuotes -and $i + 1 -lt $Line.Length -and $Line[$i + 1] -eq [char]'"') { [void]$sb.Append([char]'"'); $i += 2; continue }
            $inQuotes = -not $inQuotes; $hasToken = $true; $i++
            continue
        }
        if (-not $inQuotes -and ($c -eq [char]' ' -or $c -eq [char]"`t")) {
            if ($hasToken) { $argv.Add($sb.ToString()); [void]$sb.Clear(); $hasToken = $false }
            $i++
            continue
        }
        [void]$sb.Append($c); $hasToken = $true; $i++
    }
    if ($hasToken) { $argv.Add($sb.ToString()) }
    # A plain return: the caller wraps it in @(). docs/conventions/forbidden-patterns.md
    # has the row for "return , $list" read through @( ... ).
    return $argv.ToArray()
}

function Get-UserDataDirArg([string[]]$Argv) {
    for ($i = 1; $i -lt $Argv.Count; $i++) {
        $a = $Argv[$i]
        if ($a -match '^(?:--|-|/)user-data-dir=(.*)$') { return $Matches[1] }
        if ($a -match '^(?:--|-|/)user-data-dir$' -and $i + 1 -lt $Argv.Count) { return $Argv[$i + 1] }
    }
    return $null
}

function Test-UserDataDirArg([string[]]$Values) {
    foreach ($v in @($Values)) { if ($v -match '^(?:--|-|/)user-data-dir(=|$)') { return $true } }
    return $false
}

#endregion

#region ---------------------------------------------------------------- config file I/O

function Initialize-Locations {
    $cfgPath = if ($ConfigPath) { $ConfigPath } elseif ($env:CC_PROFILES_CONFIG) { $env:CC_PROFILES_CONFIG } else { '~/.cc_profiles.json' }
    $script:ConfigFile = Get-FullPath $cfgPath 'Config path'
    $script:DataDir = [IO.Path]::ChangeExtension($script:ConfigFile, '.d')
    $script:BackupDir = [IO.Path]::Combine($script:DataDir, 'backups')
    $script:StateFile = [IO.Path]::Combine($script:DataDir, 'state.json')
    $script:LogFile = [IO.Path]::Combine($script:DataDir, 'launch.log')
}

# Writes go to the link target so a symlinked config (e.g. from a dotfiles repo) stays a symlink.
function Get-ConfigTargetPath {
    $fi = [IO.FileInfo]::new($script:ConfigFile)
    if ($fi.Exists -and $fi.LinkTarget) {
        $t = $fi.ResolveLinkTarget($true)
        if ($t) { return $t.FullName }
    }
    return $script:ConfigFile
}

# The text of a file, whatever its byte order mark says: UTF-8, or the UTF-16 that
# Windows PowerShell 5.1's Out-File writes. A file with no mark is read as UTF-8.
function ConvertFrom-ConfigBytes([byte[]]$Bytes) {
    $reader = [IO.StreamReader]::new([IO.MemoryStream]::new($Bytes), [Text.UTF8Encoding]::new($false), $true)
    try { return $reader.ReadToEnd() } finally { $reader.Dispose() }
}

function ConvertFrom-JsonText([string]$Text, [string]$Source) {
    # -DateKind String: without it a value shaped like a date comes back as a DateTime.
    try { return ($Text | ConvertFrom-Json -AsHashtable -DateKind String) }
    catch { throw "Invalid JSON in ${Source}: $($_.Exception.Message)" }
}

# ConvertFrom-Json accepts comments and trailing commas, and the canonical rewrite
# drops both. System.Text.Json refuses them in its default mode, so a file it refuses
# holds something that a rewrite would lose.
function Test-StrictJson([string]$Text) {
    try {
        $doc = [Text.Json.JsonDocument]::Parse($Text)
        $doc.Dispose()
        return $true
    }
    catch { return $false }
}

function New-DefaultConfig {
    return [ordered]@{
        version  = $script:SchemaVersion
        root     = '~/claude-profiles'
        envMode  = 'clean'
        scrubEnv = @($script:DefaultScrub)
        env      = [ordered]@{}
        app      = [ordered]@{ exePath = $null; extraArgs = @() }
        launch   = [ordered]@{ verifySeconds = 20; via = 'cmd' }
        profiles = [ordered]@{}
    }
}

# Fills in missing top-level settings, so a hand-written config may name only its profiles.
function Add-ConfigDefaults($Cfg) {
    $d = New-DefaultConfig
    foreach ($k in @($d.Keys)) { if (-not $Cfg.Contains($k) -or $null -eq $Cfg[$k]) { $Cfg[$k] = $d[$k] } }
    foreach ($section in 'app', 'launch') {
        if ($Cfg[$section] -is [Collections.IDictionary]) {
            foreach ($k in @($d[$section].Keys)) { if (-not $Cfg[$section].Contains($k)) { $Cfg[$section][$k] = $d[$section][$k] } }
        }
    }
    return $Cfg
}

function Read-Config {
    $path = Get-ConfigTargetPath
    if (-not [IO.File]::Exists($path)) {
        return [pscustomobject]@{ Config = (New-DefaultConfig); Hash = $null; Exists = $false; Path = $path; Strict = $true }
    }
    $bytes = Read-SharedBytes $path
    $hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes))
    $text = ConvertFrom-ConfigBytes $bytes
    if ([string]::IsNullOrWhiteSpace($text)) { throw "Config '$path' is empty. Fix it, or run 'restore' to pick a backup." }
    $raw = ConvertFrom-JsonText $text $path
    if ($raw -isnot [Collections.IDictionary]) { throw "Config '$path' must contain a JSON object at the top level." }
    # Every command walks the profiles, so a wrong shape is refused here, once, by name.
    if ($null -ne $raw['profiles'] -and $raw['profiles'] -isnot [Collections.IDictionary]) {
        throw "Config '$path': 'profiles' must be an object keyed by profile name. Fix it with 'edit', or 'restore' a backup."
    }
    return [pscustomobject]@{ Config = (Add-ConfigDefaults $raw); Hash = $hash; Exists = $true; Path = $path; Strict = (Test-StrictJson $text) }
}

# Ordinal, because JSON keys are case-sensitive: a case-insensitive dictionary here
# merged "Path" and "PATH" into one key and silently dropped one of the values.
function New-OrdinalDictionary { return [Collections.Specialized.OrderedDictionary]::new([StringComparer]::Ordinal) }

function Select-Ordered($Dict, [string[]]$Order) {
    $o = New-OrdinalDictionary
    foreach ($k in $Order) { if ($Dict.Contains($k)) { $o[$k] = $Dict[$k] } }
    foreach ($k in @($Dict.Keys | Sort-Object -CaseSensitive)) { if (-not $o.Contains($k)) { $o[$k] = $Dict[$k] } }
    return $o
}

# Deterministic serialization: same config -> same bytes, unknown keys preserved.
function ConvertTo-ConfigJson($Cfg) {
    $c = Select-Ordered $Cfg $script:TopKeys
    if ($c['env'] -is [Collections.IDictionary]) { $c['env'] = Select-Ordered $c['env'] @() }
    if ($c['app'] -is [Collections.IDictionary]) { $c['app'] = Select-Ordered $c['app'] $script:AppKeys }
    if ($c['launch'] -is [Collections.IDictionary]) { $c['launch'] = Select-Ordered $c['launch'] $script:LaunchKeys }
    if ($c['profiles'] -is [Collections.IDictionary]) {
        $ps = New-OrdinalDictionary
        $names = @($c['profiles'].Keys | Sort-Object { if ($_ -eq 'default') { '' } else { $_.ToLowerInvariant() } }, { $_ } -CaseSensitive)
        foreach ($n in $names) {
            $p = $c['profiles'][$n]
            if ($p -is [Collections.IDictionary]) {
                $p = Select-Ordered $p $script:ProfileKeys
                if ($p['env'] -is [Collections.IDictionary]) { $p['env'] = Select-Ordered $p['env'] @() }
            }
            $ps[$n] = $p
        }
        $c['profiles'] = $ps
    }
    return ($c | ConvertTo-Json -Depth 32) + [Environment]::NewLine
}

# Named for the file a write lands on, so two paths to one file take one lock.
function Get-ConfigLockName {
    $target = [IO.Path]::GetFullPath((Get-ConfigTargetPath)).ToLowerInvariant()
    $key = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($target))).Substring(0, 24)
    return "Local\cc_profile-$key"
}

# One named mutex per config file serialises every writer across processes. It is
# taken and released on the same thread, which a mutex requires.
function Enter-ConfigLock {
    $mutex = [Threading.Mutex]::new($false, (Get-ConfigLockName))
    $held = $false
    # An abandoned mutex means its holder died mid-write; Write-FileAtomic left the
    # old file whole in that case, so taking the lock over is safe.
    try { $held = $mutex.WaitOne([TimeSpan]::FromSeconds(30)) }
    catch [Threading.AbandonedMutexException] { $held = $true }
    if (-not $held) {
        $mutex.Dispose()
        throw "Timed out waiting for another $($script:ToolName) process to finish with the config."
    }
    return $mutex
}

function Exit-ConfigLock($Mutex) {
    $Mutex.ReleaseMutex()
    $Mutex.Dispose()
}

function Invoke-WithConfigLock([scriptblock]$__Body) {
    $__mutex = Enter-ConfigLock
    try { & $__Body }
    finally { Exit-ConfigLock $__mutex }
}

function Backup-ConfigFile([string]$Path, [string]$Reason = 'auto') {
    if (-not [IO.File]::Exists($Path)) { return $null }
    [void][IO.Directory]::CreateDirectory($script:BackupDir)
    $newest = Get-ChildItem -LiteralPath $script:BackupDir -Filter 'cc_profiles.*.json' -File | Sort-Object Name -Descending | Select-Object -First 1
    if ($Reason -eq 'auto' -and $newest -and (Get-FileSha256 $newest.FullName) -eq (Get-FileSha256 $Path)) { return $newest.FullName }
    $stamp = [DateTime]::Now.ToString('yyyyMMdd-HHmmss-fff', [Globalization.CultureInfo]::InvariantCulture)
    $dest = [IO.Path]::Combine($script:BackupDir, "cc_profiles.$stamp.$Reason.json")
    [IO.File]::WriteAllBytes($dest, (Read-SharedBytes $Path))
    # Keep the newest 30 backups.
    Get-ChildItem -LiteralPath $script:BackupDir -Filter 'cc_profiles.*.json' -File |
        Sort-Object Name -Descending | Select-Object -Skip 30 |
        ForEach-Object { Remove-Contained $_.FullName $script:BackupDir }
    return $dest
}

# The one door every write of the config goes through: Update-Config, restore and
# edit all end here. It refuses a config of a newer schema and an invalid config, and
# refuses when the file changed on disk since it was read. Then it backs up the
# current file, replaces it atomically, and reads it back.
function Save-ConfigText([string]$Text, $ExpectedHash, [string]$Reason = 'auto') {
    $path = Get-ConfigTargetPath
    $parsed = ConvertFrom-JsonText $Text 'the new config'
    if ($parsed -isnot [Collections.IDictionary]) { throw 'Refusing to save: the top level of the config must be a JSON object.' }
    $ver = $parsed['version']
    if ((Test-IsInteger $ver) -and $ver -gt $script:SchemaVersion) {
        throw "Refusing to save: the config is schema $ver, newer than the $($script:SchemaVersion) this $($script:ToolName) understands. Update the script first."
    }
    $check = Test-Config (Add-ConfigDefaults $parsed)
    if ($check.Errors.Count) { throw ("Refusing to save an invalid config:`n  - " + ($check.Errors -join "`n  - ")) }
    $current = if ([IO.File]::Exists($path)) { Get-FileSha256 $path } else { $null }
    if ($current -ne $ExpectedHash) {
        throw "The config file changed on disk while this command ran (edited elsewhere?). Nothing was written; run the command again."
    }
    [void](Backup-ConfigFile $path $Reason)
    Write-FileAtomic $path $Text
    if ((ConvertFrom-ConfigBytes (Read-SharedBytes $path)) -cne $Text) { throw "Post-write verification of '$path' failed. Restore with 'restore'." }
}

# Read-modify-write under a cross-process lock. $Mutate receives the config and may change it.
#
# The locals carry a "__" prefix because $Mutate runs in a child of this scope and
# reads its caller's variables through dynamic scoping: a local here named $name
# would silently replace the caller's $name inside the block.
function Update-Config {
    [CmdletBinding(SupportsShouldProcess)]
    param([string]$__Action, [scriptblock]$__Mutate)
    $__lock = Enter-ConfigLock
    try {
        $__state = Read-Config
        $__before = if ($__state.Exists) { ConvertTo-ConfigJson $__state.Config } else { $null }
        $__result = & $__Mutate $__state.Config
        $__after = ConvertTo-ConfigJson $__state.Config
        if ($null -ne $__before -and $__before -ceq $__after) {
            return [pscustomobject]@{ Changed = $false; Result = $__result; Config = $__state.Config }
        }
        # The rewrite is canonical JSON. A comment in a hand-edited file would be lost.
        if ($__state.Exists -and -not $__state.Strict -and -not $Force) {
            throw "The config holds comments or other text that is not strict JSON, and this change would rewrite the file without them. Take them out with 'edit' (a profile's 'notes' keeps a remark), or add -Force to rewrite it."
        }
        if (-not $PSCmdlet.ShouldProcess($script:ConfigFile, $__Action)) {
            return [pscustomobject]@{ Changed = $false; Result = $__result; Config = $__state.Config }
        }
        Save-ConfigText $__after $__state.Hash
        return [pscustomobject]@{ Changed = $true; Result = $__result; Config = $__state.Config }
    }
    finally { Exit-ConfigLock $__lock }
}

#endregion
#region ---------------------------------------------------------------- state (tool-owned, disposable)

function Read-State {
    try {
        if ([IO.File]::Exists($script:StateFile)) {
            $s = ConvertFrom-JsonText (ConvertFrom-ConfigBytes (Read-SharedBytes $script:StateFile)) $script:StateFile
            if ($s -is [Collections.IDictionary]) {
                if ($s['profiles'] -isnot [Collections.IDictionary]) { $s['profiles'] = @{} }
                return $s
            }
        }
    }
    catch { Write-Trace "Ignoring unreadable state file: $($_.Exception.Message)" }
    return @{ profiles = @{} }
}

function Update-ProfileState([string]$Name, [hashtable]$Values) {
    if ($WhatIfPreference) { return }
    try {
        Invoke-WithConfigLock {
            $s = Read-State
            if ($s['profiles'][$Name] -isnot [Collections.IDictionary]) { $s['profiles'][$Name] = @{} }
            foreach ($k in $Values.Keys) { $s['profiles'][$Name][$k] = $Values[$k] }
            Write-FileAtomic $script:StateFile (($s | ConvertTo-Json -Depth 8) + [Environment]::NewLine)
        }
    }
    catch { Write-Trace "Could not update state: $($_.Exception.Message)" }
}

function Rename-ProfileState([string]$Old, [string]$New) {
    if ($WhatIfPreference) { return }
    try {
        Invoke-WithConfigLock {
            $s = Read-State
            if ($s['profiles'].Contains($Old)) {
                $s['profiles'][$New] = $s['profiles'][$Old]; $s['profiles'].Remove($Old)
                Write-FileAtomic $script:StateFile (($s | ConvertTo-Json -Depth 8) + [Environment]::NewLine)
            }
        }
    }
    catch { Write-Trace "Could not update state: $($_.Exception.Message)" }
}

function Write-LaunchLog([string]$Message) {
    if ($WhatIfPreference) { return }
    try {
        [void][IO.Directory]::CreateDirectory($script:DataDir)
        $line = '{0}  {1}' -f [DateTime]::Now.ToString('yyyy-MM-dd HH:mm:ss', [Globalization.CultureInfo]::InvariantCulture), $Message
        [IO.File]::AppendAllText($script:LogFile, $line + [Environment]::NewLine)
        if ([IO.FileInfo]::new($script:LogFile).Length -gt 1MB) {
            $keep = [IO.File]::ReadAllLines($script:LogFile) | Select-Object -Last 2000
            Write-FileAtomic $script:LogFile (($keep -join [Environment]::NewLine) + [Environment]::NewLine)
        }
    }
    catch { Write-Trace "Could not write launch log: $($_.Exception.Message)" }
}

#endregion

#region ---------------------------------------------------------------- validation

function Test-IsInteger($Value) { return ($Value -is [long] -or $Value -is [int]) }

function Test-StringArray($Value) {
    if ($null -eq $Value -or $Value -is [string] -or $Value -isnot [Collections.IEnumerable]) { return $false }
    foreach ($v in $Value) { if ($v -isnot [string]) { return $false } }
    return $true
}

# The rule for a profile name or alias, in one place. Test-Config applies it to every
# saved config, and add, rename and discover apply it before they build a folder path
# from a name. Returns $null for a usable name.
function Get-NameProblem([string]$Name, [string]$Kind = 'name') {
    if ($Name -notmatch $script:NameRx) { return "'$Name' is not a valid $Kind (letters, digits, '.', '_', '-', up to 64 characters, starting with a letter or digit and not ending with a dot)" }
    if ($script:ReservedNames -contains $Name) { return "'$Name' is reserved (it is a command name)" }
    return $null
}

# Writes one message per problem to the pipeline; nothing for a valid value.
function Test-EnvDict($Value, [string]$Where) {
    if ($null -eq $Value) { return }
    if ($Value -isnot [Collections.IDictionary]) { "${Where}: must be an object of NAME: value"; return }
    $seen = @{}
    foreach ($k in @($Value.Keys)) {
        if ($k -notmatch $script:EnvKeyRx) { "${Where}: '$k' is not a valid variable name" }
        elseif ($script:ManagedEnv -contains $k) { "${Where}: $k is managed by $($script:ToolName) and cannot be set" }
        # Windows ignores case in a variable name, so "Path" and "PATH" are one variable,
        # and which value won would depend on the order the keys were read in.
        elseif ($seen.ContainsKey($k)) { "${Where}: '$($seen[$k])' and '$k' name the same variable (Windows ignores case); keep one" }
        else { $seen[$k] = $k }
        $v = $Value[$k]
        if ($null -ne $v -and $v -isnot [string]) { "${Where}.${k}: value must be a string (or null to remove the variable)" }
    }
}

# A key spelled like a known key in another case is a typo the reader cannot see:
# JSON keys are case-sensitive, so "Root" is not "root" and would be ignored.
function Test-KeyCase($Dict, [string[]]$Known, [string]$Where) {
    foreach ($k in @($Dict.Keys)) {
        $hit = @($Known | Where-Object { $_ -ieq $k -and $_ -cne $k }) | Select-Object -First 1
        if ($hit) { "$Where${k}: keys are case-sensitive; this one is ignored. Did you mean '$hit'?" }
    }
}

# Returns Errors, Warnings and Issues. Each issue names the profiles it concerns,
# and one that names none concerns every profile; Assert-ConfigUsable reads that.
# Never throws for bad input; callers decide.
function Test-Config($Cfg) {
    $issues = [Collections.Generic.List[object]]::new()
    $E = { param([string]$m, [string[]]$on = @()) $issues.Add([pscustomobject]@{ Level = 'error'; Message = $m; Profiles = $on }) }
    $W = { param([string]$m, [string[]]$on = @()) $issues.Add([pscustomobject]@{ Level = 'warning'; Message = $m; Profiles = $on }) }
    $result = {
        [pscustomobject]@{
            Errors   = @($issues | Where-Object Level -EQ 'error' | ForEach-Object Message)
            Warnings = @($issues | Where-Object Level -EQ 'warning' | ForEach-Object Message)
            Issues   = $issues.ToArray()
        }
    }

    foreach ($m in @(Test-KeyCase $Cfg $script:TopKeys '')) { & $E $m }
    $ver = $Cfg['version']
    if (-not (Test-IsInteger $ver) -or $ver -lt 1) { & $E 'version: must be a positive integer' }
    elseif ($ver -gt $script:SchemaVersion) { & $W "version: $ver is newer than this script understands ($($script:SchemaVersion)); treat as read-only" }

    if ($Cfg['root'] -isnot [string] -or -not $Cfg['root']) { & $E 'root: must be a non-empty path' }
    else { try { [void](Get-FullPath $Cfg['root'] 'root') } catch { & $E "root: $($_.Exception.Message)" } }

    if ($Cfg['envMode'] -isnot [string] -or $Cfg['envMode'] -notin 'clean', 'inherit') { & $E "envMode: must be 'clean' or 'inherit'" }
    if (-not (Test-StringArray $Cfg['scrubEnv'])) { & $E 'scrubEnv: must be an array of variable names (wildcards allowed)' }
    foreach ($m in @(Test-EnvDict $Cfg['env'] 'env')) { & $E $m }

    if ($Cfg['app'] -isnot [Collections.IDictionary]) { & $E 'app: must be an object' }
    else {
        foreach ($m in @(Test-KeyCase $Cfg['app'] $script:AppKeys 'app.')) { & $E $m }
        $x = $Cfg['app']['exePath']
        if ($null -ne $x -and $x -isnot [string]) { & $E 'app.exePath: must be a path string or null' }
        $xa = $Cfg['app']['extraArgs']
        if (-not (Test-StringArray $xa)) { & $E 'app.extraArgs: must be an array of strings' }
        elseif (Test-UserDataDirArg $xa) { & $E 'app.extraArgs: must not contain --user-data-dir (it is managed per profile)' }
    }
    if ($Cfg['launch'] -isnot [Collections.IDictionary]) { & $E 'launch: must be an object' }
    else {
        foreach ($m in @(Test-KeyCase $Cfg['launch'] $script:LaunchKeys 'launch.')) { & $E $m }
        $v = $Cfg['launch']['verifySeconds']
        if (-not (Test-IsInteger $v) -or $v -lt 0 -or $v -gt 300) { & $E 'launch.verifySeconds: must be an integer from 0 to 300' }
        if ($Cfg['launch']['via'] -isnot [string] -or $Cfg['launch']['via'] -notin 'cmd', 'direct') { & $E "launch.via: must be 'cmd' or 'direct'" }
    }
    if ($Cfg['profiles'] -isnot [Collections.IDictionary]) { & $E 'profiles: must be an object keyed by profile name'; return (& $result) }

    $protected = Get-ProtectedDirs
    # Lowercased name, alias or email -> who holds it. The built-in default holds its
    # name without an entry, so a clash with it concerns only the other profile.
    $claimed = @{ 'default' = [pscustomobject]@{ Profile = $null; Text = "the built-in 'default' profile" } }
    $dirs = [Collections.Generic.List[object]]::new()

    foreach ($name in @($Cfg['profiles'].Keys)) {
        $p = $Cfg['profiles'][$name]
        $where = "profiles.$name"
        $isDefault = $name -ieq 'default'
        $problem = Get-NameProblem $name
        if ($problem) { & $E "${where}: $problem" $name; continue }
        if ($p -isnot [Collections.IDictionary]) { & $E "${where}: must be an object" $name; continue }
        foreach ($m in @(Test-KeyCase $p $script:ProfileKeys "$where.")) { & $E $m $name }

        $claim = {
            param($id, $kind)
            $k = $id.ToLowerInvariant()
            $owner = $claimed[$k]
            if ($null -eq $owner) { $claimed[$k] = [pscustomobject]@{ Profile = $name; Text = "profile '$name' ($kind)" } }
            elseif (-not ($isDefault -and $kind -eq 'name')) {
                & $E "${where}: $kind '$id' is already used by $($owner.Text)" (@($name) + @($owner.Profile | Where-Object { $_ }))
            }
        }
        & $claim $name 'name'

        $em = $p['email']
        if ($null -ne $em) {
            if ($em -isnot [string] -or $em -notmatch $script:EmailRx) { & $E "${where}.email: '$em' is not a valid email address" $name }
            else { & $claim $em 'email' }
        }
        $al = $p['aliases']
        if ($null -ne $al) {
            if (-not (Test-StringArray $al)) { & $E "${where}.aliases: must be an array of strings" $name }
            else {
                foreach ($a in $al) {
                    $problem = Get-NameProblem $a 'alias'
                    if ($problem) { & $E "${where}.aliases: $problem" $name }
                    else { & $claim $a 'alias' }
                }
            }
        }
        foreach ($m in @(Test-EnvDict $p['env'] "$where.env")) { & $E $m $name }
        if ($null -ne $p['notes'] -and $p['notes'] -isnot [string]) { & $E "${where}.notes: must be a string" $name }
        if ($null -ne $p['args']) {
            if (-not (Test-StringArray $p['args'])) { & $E "${where}.args: must be an array of strings" $name }
            elseif (Test-UserDataDirArg $p['args']) { & $E "${where}.args: must not contain --user-data-dir (use userDataDir)" $name }
        }
        if ($null -ne $p['shareClaudeConfig'] -and $p['shareClaudeConfig'] -isnot [bool]) { & $E "${where}.shareClaudeConfig: must be true or false" $name }

        if ($isDefault) {
            foreach ($k in 'userDataDir', 'claudeConfigDir', 'shareClaudeConfig', 'args') {
                if ($p.Contains($k) -and $null -ne $p[$k]) { & $E "${where}.${k}: not allowed on the built-in default profile (it always uses the app's own folders and ~/.claude)" $name }
            }
            continue
        }

        $udd = $null; $ccd = $null
        if ($p['userDataDir'] -isnot [string] -or -not $p['userDataDir']) { & $E "${where}.userDataDir: required" $name }
        else { try { $udd = Get-FullPath $p['userDataDir'] "$where.userDataDir" } catch { & $E $_.Exception.Message $name } }
        if ($p['shareClaudeConfig'] -eq $true) {
            if ($null -ne $p['claudeConfigDir']) { & $E "${where}: set claudeConfigDir or shareClaudeConfig, not both" $name }
            & $W "${where}: shares ~/.claude with the default profile (settings, memory and transcripts mix)" $name
        }
        elseif ($null -ne $p['claudeConfigDir']) {
            if ($p['claudeConfigDir'] -isnot [string]) { & $E "${where}.claudeConfigDir: must be a path string" $name }
            else { try { $ccd = Get-FullPath $p['claudeConfigDir'] "$where.claudeConfigDir" } catch { & $E $_.Exception.Message $name } }
        }
        elseif ($udd) { $ccd = [IO.Path]::Combine($udd, '.claude') }

        # The home folder needs no rule of its own: it holds ~/.claude, so the overlap
        # rule below refuses it and every parent of it.
        foreach ($pair in @(, @('userDataDir', $udd)) + @(, @('claudeConfigDir', $ccd))) {
            $kind = $pair[0]; $path = $pair[1]
            if (-not $path) { continue }
            # A network path can name a protected folder by another route, such as
            # \\localhost\C$\Windows, which no rule here would recognise.
            if (-not (Test-LocalDrivePath $path)) { & $E "${where}.${kind}: '$path' is not on a drive letter; a profile folder cannot be a network path" $name; continue }
            if ([IO.Path]::GetPathRoot($path).TrimEnd('\') -ieq $path.TrimEnd('\')) { & $E "${where}.${kind}: cannot be a drive root" $name; continue }
            foreach ($pd in $protected) {
                if (Test-PathOverlap $path $pd.Path) { & $E "${where}.${kind}: '$path' overlaps $($pd.Why)" $name }
            }
            $dirs.Add([pscustomobject]@{ Profile = $name; Kind = $kind; Path = $path })
        }
        if ($udd -and $ccd -and ((Test-PathEqual $udd $ccd) -or (Test-PathWithin $udd $ccd))) {
            & $E "${where}: userDataDir must not be the same as, or inside, claudeConfigDir" $name
        }
    }

    for ($i = 0; $i -lt $dirs.Count; $i++) {
        for ($j = $i + 1; $j -lt $dirs.Count; $j++) {
            $a = $dirs[$i]; $b = $dirs[$j]
            if ($a.Profile -ine $b.Profile -and (Test-PathOverlap $a.Path $b.Path)) {
                & $E "profiles.$($a.Profile).$($a.Kind) and profiles.$($b.Profile).$($b.Kind) overlap ('$($a.Path)' / '$($b.Path)'); every profile needs its own folders" @($a.Profile, $b.Profile)
            }
        }
    }
    return (& $result)
}

#endregion
#region ---------------------------------------------------------------- profiles

function Get-ProfileView($Cfg, [string]$Name) {
    $p = $null
    foreach ($k in @($Cfg['profiles'].Keys)) { if ($k -ieq $Name) { $Name = $k; $p = $Cfg['profiles'][$k]; break } }
    $isDefault = $Name -ieq 'default'
    if ($null -eq $p) {
        if ($isDefault) { $p = @{} } else { throw "No profile named '$Name'." }
    }
    $udd = $null; $ccd = $null; $share = $isDefault
    if (-not $isDefault) {
        $udd = Get-FullPath $p['userDataDir'] "profiles.$Name.userDataDir"
        $share = $p['shareClaudeConfig'] -eq $true
        if (-not $share) {
            $ccd = if ($p['claudeConfigDir']) { Get-FullPath $p['claudeConfigDir'] "profiles.$Name.claudeConfigDir" } else { [IO.Path]::Combine($udd, '.claude') }
        }
    }
    $envd = if ($p['env'] -is [Collections.IDictionary]) { $p['env'] } else { @{} }
    return [pscustomobject]@{
        Name               = if ($isDefault) { 'default' } else { $Name }
        IsDefault          = $isDefault
        Email              = if ($p['email'] -is [string]) { $p['email'] } else { $null }
        Aliases            = @(if (Test-StringArray $p['aliases']) { $p['aliases'] })
        UserDataDir        = $udd
        ClaudeConfigDir    = $ccd
        SharesClaudeConfig = $share
        EffectiveClaudeDir = if ($ccd) { $ccd } else { [IO.Path]::Combine($HOME, '.claude') }
        Env                = $envd
        Args               = @(if (Test-StringArray $p['args']) { $p['args'] })
        Notes              = if ($p['notes'] -is [string]) { $p['notes'] } else { $null }
    }
}

function Get-AllProfileViews($Cfg) {
    $names = @($Cfg['profiles'].Keys | Where-Object { $_ -ine 'default' } | Sort-Object { $_.ToLowerInvariant() })
    $views = [Collections.Generic.List[object]]::new()
    $views.Add((Get-ProfileView $Cfg 'default'))
    foreach ($n in $names) {
        try { $views.Add((Get-ProfileView $Cfg $n)) } catch { Write-Warn "Skipping profile '$n': $($_.Exception.Message)" }
    }
    return $views
}

# Map of lowercased name/alias/email -> profile name.
function Get-IdentifierMap($Cfg) {
    $map = @{ 'default' = 'default' }
    foreach ($n in @($Cfg['profiles'].Keys)) {
        $p = $Cfg['profiles'][$n]
        $map[$n.ToLowerInvariant()] = $n
        if ($p -isnot [Collections.IDictionary]) { continue }
        if ($p['email'] -is [string]) { $map[$p['email'].ToLowerInvariant()] = $n }
        if (Test-StringArray $p['aliases']) { foreach ($a in $p['aliases']) { $map[$a.ToLowerInvariant()] = $n } }
    }
    return $map
}

function Resolve-ProfileName($Cfg, [string]$Id) {
    if ([string]::IsNullOrWhiteSpace($Id)) { throw 'Specify a profile by name, alias or email.' }
    $Id = $Id.Trim()
    foreach ($n in @('default') + @($Cfg['profiles'].Keys)) { if ($n -ieq $Id) { return $(if ($n -ieq 'default') { 'default' } else { $n }) } }
    $hits = [Collections.Generic.List[string]]::new()
    foreach ($n in @($Cfg['profiles'].Keys)) {
        $p = $Cfg['profiles'][$n]
        if ($p -isnot [Collections.IDictionary]) { continue }
        $aliasHit = (Test-StringArray $p['aliases']) -and (@($p['aliases']) -icontains $Id)
        $emailHit = $p['email'] -is [string] -and $p['email'] -ieq $Id
        if (($aliasHit -or $emailHit) -and -not $hits.Contains($n)) { $hits.Add($n) }
    }
    if ($hits.Count -eq 1) { return $hits[0] }
    if ($hits.Count -gt 1) { throw "'$Id' matches several profiles ($($hits -join ', ')). Use the profile name." }
    $known = @((Get-IdentifierMap $Cfg).Keys | Sort-Object)
    $pattern = '*' + [WildcardPattern]::Escape($Id.ToLowerInvariant()) + '*'
    $similar = @($known | Where-Object { $_ -like $pattern })
    $hint = if ($similar.Count) { " Did you mean: $($similar -join ', ')?" } else { " Known: $($known -join ', ')." }
    throw "No profile or command matches '$Id'.$hint"
}

function ConvertTo-ProfileName([string]$EmailAddress, $Cfg) {
    $parts = $EmailAddress.Split('@', 2)
    $base = ($parts[0].ToLowerInvariant() -replace '[^a-z0-9._-]', '-').Trim('.', '_', '-')
    if (-not $base) { $base = 'profile' }
    if ($base.Length -gt 48) { $base = $base.Substring(0, 48) }
    $domain = (($parts[1].Split('.')[0]).ToLowerInvariant() -replace '[^a-z0-9-]', '-').Trim('-')
    $taken = Get-IdentifierMap $Cfg
    $candidates = @($base) + @(if ($domain) { "$base-$domain" }) + @(2..99 | ForEach-Object { "$base-$_" })
    foreach ($c in $candidates) {
        if (-not (Get-NameProblem $c) -and -not $taken.ContainsKey($c)) { return $c }
    }
    throw "Could not derive a free profile name from '$EmailAddress'; pass one explicitly."
}

# Claude Code keeps its account in $CLAUDE_CONFIG_DIR/.claude.json, or ~/.claude.json when unset.
function Get-DetectedEmail($View) {
    $file = if ($View.ClaudeConfigDir) { [IO.Path]::Combine($View.ClaudeConfigDir, '.claude.json') } else { [IO.Path]::Combine($HOME, '.claude.json') }
    try {
        if (-not [IO.File]::Exists($file)) { return $null }
        $j = ConvertFrom-JsonText (ConvertFrom-ConfigBytes (Read-SharedBytes $file)) $file
        if ($j -is [Collections.IDictionary] -and $j['oauthAccount'] -is [Collections.IDictionary]) {
            $e = $j['oauthAccount']['emailAddress']
            if ($e -is [string] -and $e) { return $e }
        }
    }
    catch { Write-Trace "Could not read ${file}: $($_.Exception.Message)" }
    return $null
}

#endregion

#region ---------------------------------------------------------------- app discovery

# An Electron app ships resources\app.asar (or an unpacked resources\app) next to its exe.
# This is what separates the desktop app from the Claude Code CLI, which is also claude.exe.
function Test-ElectronAppExe([string]$Exe) {
    if (-not $Exe) { return $false }
    try {
        if (-not [IO.File]::Exists($Exe)) { return $false }
        $dir = [IO.Path]::GetDirectoryName($Exe)
        return [IO.File]::Exists([IO.Path]::Combine($dir, 'resources', 'app.asar')) -or [IO.Directory]::Exists([IO.Path]::Combine($dir, 'resources', 'app'))
    }
    catch { return $false }
}

function Get-ExeVersion([string]$Exe) {
    try {
        $vi = [Diagnostics.FileVersionInfo]::GetVersionInfo($Exe)
        if ($vi.ProductVersion) { return $vi.ProductVersion.Trim() }
    }
    catch {
        # An unreadable version resource means "version unknown", and callers print a dash.
        $null = $_
    }
    return $null
}

function Get-ManifestApp([string]$InstallDir) {
    try {
        $manifest = [IO.Path]::Combine($InstallDir, 'AppxManifest.xml')
        if (-not [IO.File]::Exists($manifest)) { return $null }
        $x = [xml][IO.File]::ReadAllText($manifest)
        $apps = @($x.DocumentElement.GetElementsByTagName('Application'))
        $ordered = @($apps | Where-Object { $_.GetAttribute('Id') -eq 'Claude' }) + @($apps | Where-Object { $_.GetAttribute('Id') -ne 'Claude' })
        foreach ($a in $ordered) {
            $rel = $a.GetAttribute('Executable')
            if (-not $rel) { continue }
            $exe = [IO.Path]::GetFullPath([IO.Path]::Combine($InstallDir, $rel))
            if (-not (Test-ElectronAppExe $exe)) { continue }
            $logoDirs = [Collections.Generic.List[string]]::new()
            foreach ($ve in @($a.GetElementsByTagName('VisualElements', '*'))) {
                foreach ($attr in @($ve.Attributes)) {
                    if ($attr.Value -match '\.png$') {
                        $d = [IO.Path]::GetDirectoryName([IO.Path]::GetFullPath([IO.Path]::Combine($InstallDir, $attr.Value)))
                        if (-not $logoDirs.Contains($d)) { $logoDirs.Add($d) }
                    }
                }
            }
            return [pscustomobject]@{ Exe = $exe; AppId = $a.GetAttribute('Id'); LogoDirs = $logoDirs.ToArray() }
        }
    }
    catch { Write-Trace "Could not read manifest in ${InstallDir}: $($_.Exception.Message)" }
    return $null
}

function New-AppCandidate([string]$Exe, [string]$Source, $Version, [string]$InstallDir, [string]$Family, [string[]]$LogoDirs) {
    return [pscustomobject]@{
        Exe           = $Exe
        Source        = $Source
        Version       = if ($Version) { [string]$Version } else { Get-ExeVersion $Exe }
        InstallDir    = $InstallDir
        PackageFamily = $Family
        LogoDirs      = @($LogoDirs)
        IsElectron    = Test-ElectronAppExe $Exe
    }
}

# The installed packages that are the app, read in a runspace of their own. Finding
# the app changes nothing, but under -WhatIf the Appx module's first import printed
# 45 "What if" lines that hid the one about the launch. Turning the preference off
# here, in the script and globally, left those lines; a separate runspace has none of
# this one's preferences. It also leaves no module in a caller's session.
function Get-ClaudePackages {
    $ps = [PowerShell]::Create()
    try {
        $pkgs = @($ps.AddCommand('Get-AppxPackage').AddParameter('Name', 'Claude').Invoke())
        if (-not $pkgs.Count) {
            $ps.Commands.Clear()
            $pkgs = @($ps.AddCommand('Get-AppxPackage').Invoke() | Where-Object { -not $_.IsFramework -and $_.Publisher -match 'Anthropic' })
        }
        if (-not $pkgs.Count -and $ps.HadErrors) { throw @($ps.Streams.Error)[0].Exception }
        return $pkgs
    }
    finally { $ps.Dispose() }
}

# Every way we know to locate the app, best first. Nothing here depends on a version
# number or install path, so MSIX updates (which move the install folder) are picked up.
function Find-ClaudeAppCandidates($Cfg) {
    $out = [Collections.Generic.List[object]]::new()

    $override = if ($AppExe) { $AppExe } elseif ($Cfg -and $Cfg['app'] -is [Collections.IDictionary] -and $Cfg['app']['exePath']) { $Cfg['app']['exePath'] } else { $null }
    if ($override) {
        $o = Get-FullPath $override 'app.exePath'
        if ([IO.File]::Exists($o)) { $out.Add((New-AppCandidate $o 'override')) }
        else { Write-Warn "Configured app path '$o' does not exist; falling back to discovery." }
    }

    try {
        foreach ($p in @(Get-ClaudePackages | Sort-Object { [version]$_.Version } -Descending)) {
            if (-not $p.InstallLocation) { continue }
            $m = Get-ManifestApp $p.InstallLocation
            if ($m) { $out.Add((New-AppCandidate $m.Exe 'msix' $p.Version $p.InstallLocation $p.PackageFamilyName $m.LogoDirs)) }
        }
    }
    catch { Write-Trace "Get-AppxPackage unavailable: $($_.Exception.Message)" }

    $repo = 'HKCU:\Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages'
    try {
        $keys = @(Get-ChildItem -LiteralPath $repo -ErrorAction Stop | Where-Object { $_.PSChildName -match '^Claude_[0-9.]+_[^_]*__\w+$' })
        foreach ($k in @($keys | Sort-Object { [version]($_.PSChildName.Split('_')[1]) } -Descending)) {
            $parts = $k.PSChildName.Split('_')
            $root = $null
            # A key without the value is a stale registration; the next key is tried.
            try { $root = Get-ItemPropertyValue -LiteralPath $k.PSPath -Name 'PackageRootFolder' -ErrorAction Stop } catch { $null = $_ }
            if (-not $root -or -not [IO.Directory]::Exists($root)) { continue }
            $m = Get-ManifestApp $root
            if ($m) { $out.Add((New-AppCandidate $m.Exe 'msix-registry' $parts[1] $root "Claude_$($parts[-1])" $m.LogoDirs)) }
        }
    }
    catch { Write-Trace "Package repository not readable: $($_.Exception.Message)" }

    if ($env:LOCALAPPDATA) {
        $alias = [IO.Path]::Combine($env:LOCALAPPDATA, 'Microsoft', 'WindowsApps', 'claude-desktop.exe')
        if ([IO.File]::Exists($alias)) { $out.Add((New-AppCandidate $alias 'execution-alias')) }

        $sq = [IO.Path]::Combine($env:LOCALAPPDATA, 'AnthropicClaude')
        if ([IO.Directory]::Exists($sq)) {
            $appDirs = @(Get-ChildItem -LiteralPath $sq -Directory -Filter 'app-*' -ErrorAction SilentlyContinue |
                    Where-Object { $_.Name -match '^app-(\d+(\.\d+){1,3})$' } |
                    Sort-Object { [version]($_.Name.Substring(4)) } -Descending)
            foreach ($d in $appDirs) {
                $e = [IO.Path]::Combine($d.FullName, 'Claude.exe')
                if (Test-ElectronAppExe $e) { $out.Add((New-AppCandidate $e 'squirrel' $d.Name.Substring(4))) }
            }
        }
        $e = [IO.Path]::Combine($env:LOCALAPPDATA, 'Programs', 'Claude', 'Claude.exe')
        if (Test-ElectronAppExe $e) { $out.Add((New-AppCandidate $e 'per-user-install')) }
    }
    if ($env:ProgramFiles) {
        $e = [IO.Path]::Combine($env:ProgramFiles, 'Claude', 'Claude.exe')
        if (Test-ElectronAppExe $e) { $out.Add((New-AppCandidate $e 'machine-install')) }
    }
    foreach ($i in @(Get-ClaudeInstances)) { $out.Add((New-AppCandidate $i.Exe 'running-process')) }

    $seen = @{}
    return @($out | Where-Object { $k = $_.Exe.ToLowerInvariant(); if ($seen.ContainsKey($k)) { $false } else { $seen[$k] = 1; $true } })
}

function Find-ClaudeApp($Cfg) {
    if ($script:AppCache) { return $script:AppCache }
    $pick = @(Find-ClaudeAppCandidates $Cfg) | Select-Object -First 1
    if (-not $pick) {
        Stop-CannotRun "Could not find the Claude desktop app. Install it, or set app.exePath in $($script:ConfigFile) (or pass -AppExe)."
    }
    $script:AppCache = $pick
    return $pick
}

#endregion
#region ---------------------------------------------------------------- icon

function Read-PngSize([string]$Path) {
    try {
        $fs = [IO.File]::OpenRead($Path)
        try {
            $b = [byte[]]::new(24)
            if ($fs.Read($b, 0, 24) -lt 24) { return $null }
        }
        finally { $fs.Dispose() }
        if ($b[0] -ne 0x89 -or $b[1] -ne 0x50 -or $b[2] -ne 0x4E -or $b[3] -ne 0x47) { return $null }
        $w = ([int]$b[16] -shl 24) -bor ([int]$b[17] -shl 16) -bor ([int]$b[18] -shl 8) -bor [int]$b[19]
        $h = ([int]$b[20] -shl 24) -bor ([int]$b[21] -shl 16) -bor ([int]$b[22] -shl 8) -bor [int]$b[23]
        return [pscustomobject]@{ Width = $w; Height = $h }
    }
    catch { return $null }
}

# A Vista+ .ico may embed a PNG directly: 6-byte header + one 16-byte entry + the PNG.
function Write-IcoFromPng([byte[]]$Png, [int]$Size, [string]$Path) {
    $ms = [IO.MemoryStream]::new()
    $bw = [IO.BinaryWriter]::new($ms)
    $dim = if ($Size -ge 256) { 0 } else { $Size }
    $bw.Write([uint16]0); $bw.Write([uint16]1); $bw.Write([uint16]1)
    $bw.Write([byte]$dim); $bw.Write([byte]$dim); $bw.Write([byte]0); $bw.Write([byte]0)
    $bw.Write([uint16]1); $bw.Write([uint16]32); $bw.Write([uint32]$Png.Length); $bw.Write([uint32]22)
    $bw.Write($Png); $bw.Flush()
    $tmp = "$Path.$([guid]::NewGuid().ToString('N')).tmp"
    [IO.File]::WriteAllBytes($tmp, $ms.ToArray())
    [IO.File]::Move($tmp, $Path, $true)
}

# Shortcut icons must not point into the versioned install folder (it moves on every
# MSIX update), so the icon is copied to ~/.cc_profiles.d/icons once.
function Get-StableIconPath($App, [switch]$Refresh) {
    $iconDir = [IO.Path]::Combine($script:DataDir, 'icons')
    $ico = [IO.Path]::Combine($iconDir, 'claude.ico')
    if (-not $Refresh -and [IO.File]::Exists($ico)) { return $ico }
    [void][IO.Directory]::CreateDirectory($iconDir)
    $dirs = @($App.LogoDirs | Where-Object { $_ })
    if (-not $dirs.Count -and $App.InstallDir) { $dirs = @([IO.Path]::Combine($App.InstallDir, 'Assets')) }
    $best = $null; $bestScore = -1
    foreach ($d in $dirs) {
        if (-not [IO.Directory]::Exists($d)) { continue }
        foreach ($f in [IO.Directory]::EnumerateFiles($d, '*.png')) {
            $s = Read-PngSize $f
            if (-not $s -or $s.Width -ne $s.Height -or $s.Width -gt 256 -or $s.Width -lt 16) { continue }
            $leaf = [IO.Path]::GetFileName($f)
            $variant = if ($leaf -match 'lightunplated') { 0 } elseif ($leaf -match 'unplated') { 2 } else { 1 }
            $score = $s.Width * 10 + $variant
            if ($score -gt $bestScore) { $bestScore = $score; $best = [pscustomobject]@{ Path = $f; Size = $s.Width } }
        }
    }
    try {
        if ($best) { Write-IcoFromPng ([IO.File]::ReadAllBytes($best.Path)) $best.Size $ico; return $ico }
        Add-Type -AssemblyName System.Drawing
        $icon = [Drawing.Icon]::ExtractAssociatedIcon($App.Exe)
        if ($icon) {
            $fs = [IO.File]::Create($ico)
            try { $icon.Save($fs) } finally { $fs.Dispose(); $icon.Dispose() }
            return $ico
        }
    }
    catch { Write-Trace "Icon extraction failed: $($_.Exception.Message)" }
    return $App.Exe
}

#endregion
#region ---------------------------------------------------------------- processes

# Main (browser) processes of the desktop app, with the --user-data-dir each was started with.
function Get-ClaudeInstances {
    $list = [Collections.Generic.List[object]]::new()
    $procs = @()
    try { $procs = @(Get-CimInstance -ClassName Win32_Process -Filter "Name='Claude.exe'" -ErrorAction Stop -Verbose:$false) }
    catch { Write-Trace "Process query failed: $($_.Exception.Message)"; return $list }
    foreach ($p in $procs) {
        $cl = [string]$p.CommandLine
        if (-not $cl) { continue }
        $argv = Split-CommandLine $cl
        if (@($argv | Where-Object { $_ -like '--type=*' }).Count) { continue }
        $exe = if ($p.ExecutablePath) { [string]$p.ExecutablePath } elseif ($argv.Count) { $argv[0] } else { $null }
        if (-not (Test-ElectronAppExe $exe)) { continue }
        $udd = Get-UserDataDirArg $argv
        $full = $null
        if ($udd) { try { $full = Get-FullPath $udd } catch { $full = $udd } }
        $list.Add([pscustomobject]@{ Pid = [int]$p.ProcessId; ParentPid = [int]$p.ParentProcessId; Exe = $exe; UserDataDir = $full; Started = $p.CreationDate })
    }
    return $list
}

function Test-InstanceMatch($Instance, $View) {
    if ($View.IsDefault) { return -not $Instance.UserDataDir }
    return [bool]($Instance.UserDataDir -and (Test-PathEqual $Instance.UserDataDir $View.UserDataDir))
}

function Get-DirActivityUtc([string]$Dir) {
    $latest = [datetime]::MinValue
    foreach ($d in @($Dir, [IO.Path]::Combine($Dir, 'logs'))) {
        if (-not [IO.Directory]::Exists($d)) { continue }
        foreach ($e in [IO.DirectoryInfo]::new($d).EnumerateFileSystemInfos()) {
            if ($e.LastWriteTimeUtc -gt $latest) { $latest = $e.LastWriteTimeUtc }
        }
    }
    return $latest
}

function Wait-ProcessesExit([int[]]$Ids, [int]$Seconds) {
    $deadline = [DateTime]::UtcNow.AddSeconds($Seconds)
    do {
        $alive = @($Ids | Where-Object { Get-Process -Id $_ -ErrorAction SilentlyContinue })
        if (-not $alive.Count) { return $true }
        Start-Sleep -Milliseconds 300
    } while ([DateTime]::UtcNow -lt $deadline)
    return $false
}

# CloseMainWindow first. The app usually goes to the tray on a close, so a stop
# that has to END the process needs -Force, and says it terminated rather than closed.
function Stop-ProfileInstances {
    [CmdletBinding(SupportsShouldProcess)]
    param($View, [object[]]$Instances)
    $ids = @($Instances | ForEach-Object { $_.Pid })
    if (-not $PSCmdlet.ShouldProcess("Claude profile '$($View.Name)' (pid $($ids -join ', '))", 'Close')) { throw 'Cancelled.' }
    foreach ($id in $ids) {
        $pr = Get-Process -Id $id -ErrorAction SilentlyContinue
        if ($pr) { [void]$pr.CloseMainWindow() }
    }
    if (Wait-ProcessesExit $ids 15) { Write-Ok "Closed '$($View.Name)'."; return }
    if (-not $Force) {
        throw "'$($View.Name)' is still running (it probably went to the tray). Quit it from the tray icon, or add -Force to terminate it."
    }
    Stop-Process -Id $ids -Force -ErrorAction SilentlyContinue
    if (-not (Wait-ProcessesExit $ids 10)) { throw "Could not stop pid $($ids -join ', ')." }
    Write-Warn "Terminated '$($View.Name)'."
}

#endregion

#region ---------------------------------------------------------------- environment

function Test-WildcardAny([string]$Name, [string[]]$Patterns) {
    foreach ($p in @($Patterns)) { if ($p -and $Name -like $p) { return $true } }
    return $false
}

# "clean": what a fresh Start-menu launch gets (machine + user registry environment,
# user PATH appended to machine PATH, plus per-session variables), regardless of the
# shell this script runs in. "inherit": the current process environment.
function Get-BaseEnvironment([string]$Mode) {
    $envd = [Collections.Generic.Dictionary[string, string]]::new([StringComparer]::OrdinalIgnoreCase)
    if ($Mode -eq 'inherit') {
        foreach ($de in [Environment]::GetEnvironmentVariables().GetEnumerator()) { $envd[[string]$de.Key] = [string]$de.Value }
        return $envd
    }
    # Same layering as Windows' CreateEnvironmentBlock: machine, then the logon session's
    # own variables (the machine block carries USERNAME=SYSTEM), then the user's.
    $machine = [Environment]::GetEnvironmentVariables('Machine')
    foreach ($k in @($machine.Keys)) { $envd[[string]$k] = [Environment]::ExpandEnvironmentVariables([string]$machine[$k]) }
    foreach ($n in $script:SessionEnvVars) {
        $v = [Environment]::GetEnvironmentVariable($n)
        if ($v) { $envd[$n] = $v }
    }
    $user = [Environment]::GetEnvironmentVariables('User')
    foreach ($k in @($user.Keys)) {
        $v = [Environment]::ExpandEnvironmentVariables([string]$user[$k])
        if ($k -ieq 'Path' -and $envd.ContainsKey('Path')) { $envd['Path'] = $envd['Path'].TrimEnd(';') + ';' + $v }
        else { $envd[[string]$k] = $v }
    }
    # A plain return: PowerShell does not unroll an IDictionary.
    return $envd
}

function New-LaunchSpec($Cfg, $View, $App) {
    $envd = Get-BaseEnvironment $Cfg['envMode']
    # The built-in patterns always apply: a config can add to them, not empty them.
    # A variable the app should get goes in env, which is applied after the scrub.
    $patterns = $script:DefaultScrub + @($Cfg['scrubEnv']) + $script:ManagedEnv
    $scrubbed = [Collections.Generic.List[string]]::new()
    foreach ($k in @($envd.Keys)) {
        if (Test-WildcardAny $k $patterns) { [void]$envd.Remove($k); $scrubbed.Add($k) }
    }
    $applied = [Collections.Generic.List[string]]::new()
    foreach ($src in @($Cfg['env'], $View.Env)) {
        if ($src -isnot [Collections.IDictionary]) { continue }
        foreach ($k in @($src.Keys)) {
            if ($null -eq $src[$k]) { [void]$envd.Remove([string]$k) } else { $envd[[string]$k] = [string]$src[$k] }
            $applied.Add([string]$k)
        }
    }
    if ($View.ClaudeConfigDir) { $envd['CLAUDE_CONFIG_DIR'] = $View.ClaudeConfigDir }
    # Without it Electron calls AttachConsole(ATTACH_PARENT_PROCESS): started from a
    # terminal, it prints its log there and dies when that terminal closes.
    $envd['ELECTRON_NO_ATTACH_CONSOLE'] = '1'

    if (Test-UserDataDirArg $AppArgs) { throw '-AppArgs must not contain --user-data-dir (set userDataDir on the profile instead).' }
    $argv = [Collections.Generic.List[string]]::new()
    if (-not $View.IsDefault) { $argv.Add("--user-data-dir=$($View.UserDataDir)") }
    foreach ($a in @($Cfg['app']['extraArgs']) + @($View.Args) + @($AppArgs)) { if ($a) { $argv.Add([string]$a) } }

    return [pscustomobject]@{
        Exe              = $App.Exe
        Args             = $argv.ToArray()
        Env              = $envd
        Scrubbed         = $scrubbed.ToArray()
        Applied          = $applied.ToArray()
        WorkingDirectory = [IO.Path]::GetFullPath($HOME)
    }
}

function Show-LaunchSpec($Spec, $View, $App, $Cfg) {
    $quoted = @($Spec.Args | ForEach-Object { if ($_ -match '\s') { '"' + $_ + '"' } else { $_ } })
    Write-Line "Profile            : $($View.Name)$(if ($View.Email) { " <$($View.Email)>" })"
    Write-Line "Executable         : $($Spec.Exe)  (v$($App.Version), via $($App.Source))"
    Write-Line "Arguments          : $($quoted -join ' ')"
    Write-Line "CLAUDE_CONFIG_DIR  : $(if ($Spec.Env.ContainsKey('CLAUDE_CONFIG_DIR')) { $Spec.Env['CLAUDE_CONFIG_DIR'] } else { '(unset -> ~/.claude)' })"
    Write-Line "Working directory  : $($Spec.WorkingDirectory)"
    Write-Line "Environment        : $($Cfg['envMode']) mode, $($Spec.Env.Count) variables"
    $current = [Collections.Generic.Dictionary[string, string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($de in [Environment]::GetEnvironmentVariables().GetEnumerator()) { $current[[string]$de.Key] = [string]$de.Value }
    $added = @($Spec.Env.Keys | Where-Object { -not $current.ContainsKey($_) } | Sort-Object)
    $changed = @($Spec.Env.Keys | Where-Object { $current.ContainsKey($_) -and $current[$_] -cne $Spec.Env[$_] } | Sort-Object)
    $removed = @($current.Keys | Where-Object { -not $Spec.Env.ContainsKey($_) } | Sort-Object)
    foreach ($k in $added) { Write-Line "  + $k=$(Format-Secret $k $Spec.Env[$k])" -Color Green }
    foreach ($k in $changed) {
        $v = Format-Secret $k $Spec.Env[$k]
        if ($v.Length -gt 120) { $v = $v.Substring(0, 117) + '...' }
        Write-Line "  ~ $k=$v" -Color Yellow
    }
    foreach ($k in $removed) { Write-Line "  - $k" -Color DarkGray }
    Write-Line '(+ added, ~ changed, - not passed on; compared with this shell)' -Color DarkGray
}

#endregion
#region ---------------------------------------------------------------- process creation

# Quotes one argument so CommandLineToArgvW / the MSVC runtime read it back unchanged.
# -Always quotes even a plain one, which a line cmd.exe parses needs: unquoted, cmd
# reads & | < > ( ) ^ as its own syntax.
function Format-CommandLineArg([string]$Arg, [switch]$Always) {
    if (-not $Always -and $Arg.Length -gt 0 -and $Arg -notmatch '[\s"]') { return $Arg }
    $sb = [Text.StringBuilder]::new('"')
    $slashes = 0
    foreach ($ch in $Arg.ToCharArray()) {
        if ($ch -eq [char]'\') { $slashes++; continue }
        if ($ch -eq [char]'"') { [void]$sb.Append([char]'\', $slashes * 2 + 1).Append('"'); $slashes = 0; continue }
        if ($slashes) { [void]$sb.Append([char]'\', $slashes); $slashes = 0 }
        [void]$sb.Append($ch)
    }
    if ($slashes) { [void]$sb.Append([char]'\', $slashes * 2) }
    return $sb.Append('"').ToString()
}

# Lines rather than a here-string: Windows PowerShell 5.1 mis-parses a here-string
# whose terminator arrives with a bare LF (docs/conventions/shell.md section 5).
# The type name carries a version, because a type cannot be redefined in a session.
$script:NativeSource = @(
    'using System;'
    'using System.ComponentModel;'
    'using System.Runtime.InteropServices;'
    'using System.Text;'
    ''
    'public static class CcProfileNative2 {'
    '    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]'
    '    struct STARTUPINFO {'
    '        public int cb; public string lpReserved; public string lpDesktop; public string lpTitle;'
    '        public int dwX, dwY, dwXSize, dwYSize, dwXCountChars, dwYCountChars, dwFillAttribute, dwFlags;'
    '        public short wShowWindow, cbReserved2;'
    '        public IntPtr lpReserved2, hStdInput, hStdOutput, hStdError;'
    '    }'
    '    [StructLayout(LayoutKind.Sequential)]'
    '    struct PROCESS_INFORMATION { public IntPtr hProcess, hThread; public int dwProcessId, dwThreadId; }'
    ''
    '    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]'
    '    static extern bool CreateProcessW(string app, StringBuilder cmd, IntPtr pa, IntPtr ta, bool inherit,'
    '        uint flags, IntPtr env, string cwd, ref STARTUPINFO si, out PROCESS_INFORMATION pi);'
    '    [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr h);'
    '    [DllImport("kernel32.dll")] static extern uint WaitForSingleObject(IntPtr h, uint ms);'
    '    [DllImport("kernel32.dll")] static extern bool GetExitCodeProcess(IntPtr h, out uint code);'
    ''
    '    const uint DETACHED_PROCESS = 0x8, CREATE_NEW_PROCESS_GROUP = 0x200, CREATE_UNICODE_ENVIRONMENT = 0x400,'
    '               CREATE_BREAKAWAY_FROM_JOB = 0x01000000, CREATE_DEFAULT_ERROR_MODE = 0x04000000,'
    '               CREATE_NO_WINDOW = 0x08000000;'
    ''
    '    // No inherited handles, its own process group, and out of the job of the caller when'
    '    // that job allows it. hiddenConsole gives a console program a console nobody sees;'
    '    // otherwise the process has none. waitMs > 0 waits that long for it to exit, and'
    '    // exited says whether it did.'
    '    public static int Start(string exe, string commandLine, string[] env, string cwd, bool hiddenConsole,'
    '                            int waitMs, out bool brokeAway, out bool exited, out int exitCode) {'
    '        var sb = new StringBuilder();'
    '        foreach (var e in env) sb.Append(e).Append((char)0);'
    '        sb.Append((char)0);'
    '        IntPtr block = Marshal.StringToHGlobalUni(sb.ToString());'
    '        try {'
    '            uint flags = (hiddenConsole ? CREATE_NO_WINDOW : DETACHED_PROCESS) | CREATE_NEW_PROCESS_GROUP'
    '                         | CREATE_UNICODE_ENVIRONMENT | CREATE_DEFAULT_ERROR_MODE;'
    '            var si = new STARTUPINFO(); si.cb = Marshal.SizeOf(typeof(STARTUPINFO));'
    '            PROCESS_INFORMATION pi;'
    '            brokeAway = true;'
    '            bool ok = CreateProcessW(exe, new StringBuilder(commandLine), IntPtr.Zero, IntPtr.Zero, false,'
    '                flags | CREATE_BREAKAWAY_FROM_JOB, block, cwd, ref si, out pi);'
    '            if (!ok && Marshal.GetLastWin32Error() == 5) {'
    '                brokeAway = false;'
    '                ok = CreateProcessW(exe, new StringBuilder(commandLine), IntPtr.Zero, IntPtr.Zero, false,'
    '                    flags, block, cwd, ref si, out pi);'
    '            }'
    '            if (!ok) throw new Win32Exception(Marshal.GetLastWin32Error());'
    '            exited = false; exitCode = -1;'
    '            if (waitMs > 0 && WaitForSingleObject(pi.hProcess, (uint)waitMs) == 0) {'
    '                exited = true;'
    '                uint code;'
    '                if (GetExitCodeProcess(pi.hProcess, out code)) exitCode = (int)code;'
    '            }'
    '            CloseHandle(pi.hThread); CloseHandle(pi.hProcess);'
    '            return pi.dwProcessId;'
    '        } finally { Marshal.FreeHGlobal(block); }'
    '    }'
    '}'
) -join "`n"

# One start through CreateProcessW. It is a function of its own so that a test can
# stand in for it and see every start a launch asks for, without starting anything.
function Invoke-NativeStart([string]$Exe, [string]$CommandLine, [string[]]$Environment, [string]$Directory, [bool]$HiddenConsole, [int]$WaitMs) {
    try { if (-not ('CcProfileNative2' -as [type])) { Add-Type -TypeDefinition $script:NativeSource -Language CSharp } }
    catch {
        # No fallback to Process.Start: it hands the app this terminal's console and
        # handles, which is the failure this tool exists to prevent.
        Stop-CannotRun "Cannot start Claude detached here: compiling the launcher failed ($($_.Exception.Message)). Nothing was launched."
    }
    $brokeAway = $false; $exited = $false; $exitCode = -1
    try { $id = [CcProfileNative2]::Start($Exe, $CommandLine, $Environment, $Directory, $HiddenConsole, $WaitMs, [ref]$brokeAway, [ref]$exited, [ref]$exitCode) }
    catch {
        $inner = if ($_.Exception.InnerException) { $_.Exception.InnerException.Message } else { $_.Exception.Message }
        throw "Could not start ${Exe}: $inner"
    }
    return [pscustomobject]@{ Pid = $id; BrokeAway = $brokeAway; Exited = $exited; ExitCode = $exitCode }
}

# Starts the app so nothing the launching terminal does can end it.
#
# NO CONSOLE. ELECTRON_NO_ATTACH_CONSOLE, which New-LaunchSpec sets on every launch,
# stops the app attaching to its parent's console, and the process is created
# without one. So the app prints nothing into the terminal, and closing the terminal
# sends it nothing. Confirm-Isolation reads this back from the running app.
# NOT IN THE TREE. Through cmd's start, the app's parent is a cmd.exe that exits at
# once, so a tree kill of the terminal (taskkill /T, "End process tree") cannot reach
# it. launch.via = "direct" skips this hop, and the app is then a child of this process.
# OUT OF THE JOB. Created with breakaway, where the caller's job permits it.
#
# Through cmd every argument is quoted, which makes & | < > ( ) ^ literal. cmd still
# expands % inside quotes and cannot carry a double quote or a line break, so an
# argument holding one goes direct. /v:off keeps ! literal whatever the registry says.
function Start-DetachedProcess($Spec, [string]$Via = 'cmd') {
    $argv = @($Spec.Exe) + @($Spec.Args)
    $direct = ($argv | ForEach-Object { Format-CommandLineArg $_ }) -join ' '
    $quoted = ($argv | ForEach-Object { Format-CommandLineArg $_ -Always }) -join ' '
    $envList = [string[]]@($Spec.Env.Keys | Sort-Object { $_.ToUpperInvariant() } | ForEach-Object { "$_=$($Spec.Env[$_])" })
    $viaCmd = $Via -eq 'cmd' -and -not @($argv | Where-Object { $_ -match '[%"\r\n]' }).Count
    if ($viaCmd) {
        $cmdExe = [IO.Path]::Combine([Environment]::SystemDirectory, 'cmd.exe')
        $cmdLine = "`"$cmdExe`" /d /v:off /s /c `"start `"`" $quoted`""
        $r = Invoke-NativeStart $cmdExe $cmdLine $envList $Spec.WorkingDirectory $true 15000
        $jobNote = if ($r.BrokeAway) { '' } else { '; job breakaway not allowed' }
        if (-not $r.Exited) {
            # Still running, so start may be waiting on a prompt and may yet start the
            # app. A direct start now could start it twice.
            Write-Warn "cmd start (pid $($r.Pid)) had not finished after 15 s; not starting the app a second time."
            return [pscustomobject]@{ Pid = $null; Via = $r.Pid; Method = "cmd start, unfinished after 15 s$jobNote" }
        }
        if ($r.ExitCode -eq 0) { return [pscustomobject]@{ Pid = $null; Via = $r.Pid; Method = "cmd start, orphaned and detached$jobNote" } }
        Write-Warn "cmd start answered exit code $($r.ExitCode); launching directly instead."
    }
    $r = Invoke-NativeStart $Spec.Exe $direct $envList $Spec.WorkingDirectory $false 0
    $jobNote = if ($r.BrokeAway) { '' } else { '; job breakaway not allowed' }
    return [pscustomobject]@{ Pid = $r.Pid; Via = $null; Method = "CreateProcess, detached, a child of this process$jobNote" }
}

#endregion
#region ---------------------------------------------------------------- consoles

$script:ConsoleProbeSource = @(
    'using System;'
    'using System.Runtime.InteropServices;'
    'public static class CcProfileConsoleProbe1 {'
    '    [DllImport("kernel32.dll", SetLastError = true)] static extern bool FreeConsole();'
    '    [DllImport("kernel32.dll", SetLastError = true)] static extern bool AttachConsole(uint pid);'
    '    [DllImport("kernel32.dll", SetLastError = true)] static extern uint GetConsoleProcessList(uint[] list, uint count);'
    '    // "attached|PID,PID" lists the other processes on that console; "none|" means the'
    '    // process has no console (ERROR_INVALID_HANDLE); "unknown|CODE" is anything else.'
    '    public static string Probe(uint pid) {'
    '        FreeConsole();'
    '        if (!AttachConsole(pid)) {'
    '            int e = Marshal.GetLastWin32Error();'
    '            return e == 6 ? "none|" : "unknown|" + e;'
    '        }'
    '        try {'
    '            uint[] buf = new uint[1024];'
    '            uint n = GetConsoleProcessList(buf, (uint)buf.Length);'
    '            uint self = (uint)System.Diagnostics.Process.GetCurrentProcess().Id;'
    '            var sb = new System.Text.StringBuilder();'
    '            for (uint i = 0; i < n && i < buf.Length; i++) {'
    '                if (buf[i] == self) continue;'
    '                if (sb.Length > 0) sb.Append(",");'
    '                sb.Append(buf[i]);'
    '            }'
    '            return "attached|" + sb.ToString();'
    '        } finally { FreeConsole(); }'
    '    }'
    '}'
) -join "`n"

$script:ConsoleProbeScript = @(
    'Add-Type -TypeDefinition $env:CC_PROFILE_PROBE_SOURCE'
    'foreach ($t in $env:CC_PROFILE_PROBE_PIDS.Split('','')) {'
    '    [Console]::Out.WriteLine(''PROBE|'' + $t + ''|'' + [CcProfileConsoleProbe1]::Probe([uint32]$t))'
    '}'
) -join "`n"

# Whether each process is attached to a console, and which processes share it.
#
# Electron attaches to its parent's console unless told not to. Attached, the app
# prints its log into that terminal, and closing the terminal sends CTRL_CLOSE_EVENT to
# every process on the console, which ends the app.
#
# Asked from a CHILD process, because the only way to ask is to attach: FreeConsole
# in this process would cut the caller's own shell off its terminal. Read-only: the
# child writes nothing to the console it visits.
function Get-ConsoleAttachment([int[]]$Ids) {
    $result = @{}
    $wanted = @($Ids | Where-Object { $_ -gt 0 } | Select-Object -Unique)
    if (-not $wanted.Count) { return $result }
    $psi = [Diagnostics.ProcessStartInfo]::new((Get-Process -Id $PID).Path)
    $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($script:ConsoleProbeScript))
    foreach ($a in '-NoLogo', '-NoProfile', '-NonInteractive', '-EncodedCommand', $encoded) { $psi.ArgumentList.Add($a) }
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.Environment['CC_PROFILE_PROBE_SOURCE'] = $script:ConsoleProbeSource
    $psi.Environment['CC_PROFILE_PROBE_PIDS'] = $wanted -join ','
    $p = [Diagnostics.Process]::Start($psi)
    $outTask = $p.StandardOutput.ReadToEndAsync()
    $errTask = $p.StandardError.ReadToEndAsync()
    if (-not $p.WaitForExit(30000)) {
        $p.Kill($true)
        Write-Trace 'The console probe did not answer within 30s.'
        return $result
    }
    foreach ($line in ($outTask.Result -split '\r?\n')) {
        if ($line -match '^PROBE\|(\d+)\|(attached|none|unknown)\|(.*)$') {
            $sharing = @(if ($Matches[2] -eq 'attached') { $Matches[3].Split(',', [StringSplitOptions]::RemoveEmptyEntries) | ForEach-Object { [int]$_ } })
            $detail = if ($Matches[2] -eq 'unknown') { "error $($Matches[3])" } else { '' }
            $result[[int]$Matches[1]] = [pscustomobject]@{ State = $Matches[2]; Sharing = $sharing; Detail = $detail }
        }
    }
    if ($errTask.Result) { Write-Trace "Console probe stderr: $($errTask.Result.Trim())" }
    return $result
}

function Format-ProcessList([int[]]$Ids, [int]$Exclude) {
    $names = @(foreach ($id in @($Ids | Where-Object { $_ -ne $Exclude })) {
            $p = Get-Process -Id $id -ErrorAction SilentlyContinue
            if ($p) { "$($p.ProcessName) $id" } else { "$id" }
        })
    if (-not $names.Count) { return 'no other process' }
    if ($names.Count -gt 4) { return (($names | Select-Object -First 4) -join ', ') + ", and $($names.Count - 4) more" }
    return $names -join ', '
}

# Names each instance that is attached to a console, and each whose console could
# not be read. An attached instance was started outside this tool, or by a version
# that did not set ELECTRON_NO_ATTACH_CONSOLE. An unread one is not reported as fine:
# that is the claim this tool cannot make without reading it.
# Returns how many it named. $Probe is a probe result already taken, if any.
function Write-AttachedInstances($View, [object[]]$Instances, [switch]$AsFailure, [hashtable]$Probe) {
    $states = if ($null -ne $Probe) { $Probe } else { Get-ConsoleAttachment @($Instances | ForEach-Object { $_.Pid }) }
    $fix = "Quit it and run '$($script:ToolName) $($View.Name)' again, or restart it now with '$($script:ToolName) $($View.Name) -Restart -Force'."
    $found = 0
    foreach ($i in $Instances) {
        $a = $states[$i.Pid]
        if ($a -and $a.State -eq 'none') { continue }
        $found++
        if ($a -and $a.State -eq 'attached') {
            $msg = "pid $($i.Pid) of '$($View.Name)' is attached to a terminal (shared with $(Format-ProcessList $a.Sharing $i.Pid)). " +
            "It prints its log there, and closing that terminal ends it. $fix"
        }
        else {
            $why = if ($a) { $a.Detail } else { 'no answer from the probe' }
            $msg = "Could not read whether pid $($i.Pid) of '$($View.Name)' is attached to a terminal ($why). " +
            "If it was started from one, closing that terminal ends it. $fix"
        }
        if ($AsFailure) { Write-Bad $msg } else { Write-Warn $msg }
    }
    return $found
}

#endregion
#region ---------------------------------------------------------------- launch

function Test-IsElevated {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    return ([Security.Principal.WindowsPrincipal]::new($id)).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# An error blocks a profile when it names that profile, or names none. So a mistake
# in one profile does not stop the others, and a mistake in the shared settings
# stops them all. Test-Config says which profiles each error concerns.
function Assert-ConfigUsable($Cfg, [string]$Name) {
    $t = Test-Config $Cfg
    if (-not $t.Errors.Count) { return }
    $mine = @($t.Issues | Where-Object { $_.Level -eq 'error' -and (-not $_.Profiles.Count -or $_.Profiles -contains $Name) } | ForEach-Object { $_.Message })
    if ($mine.Count) { throw ("The config has problems that affect '$Name' (fix with 'edit' or 'restore'):`n  - " + ($mine -join "`n  - ")) }
    Write-Warn "The config has problems in other profiles; run 'validate'."
}

function Invoke-Launch {
    [CmdletBinding(SupportsShouldProcess)]
    param($Cfg, [string]$Name)
    $view = Get-ProfileView $Cfg $Name
    $app = Find-ClaudeApp $Cfg
    $spec = New-LaunchSpec $Cfg $view $app
    $running = @(Get-ClaudeInstances | Where-Object { Test-InstanceMatch $_ $view })

    if ($DryRun) {
        Show-LaunchSpec $spec $view $app $Cfg
        if ($running.Count) { Write-Note "Running now: pid $(@($running | ForEach-Object { $_.Pid }) -join ', ')" }
        return
    }
    # A switch of its own: -Force answers "end the running instance", and advice that
    # says -Restart -Force must not also start Claude as administrator.
    if ((Test-IsElevated) -and -not $AllowElevated) {
        throw 'This shell is elevated, so Claude would run as administrator. Launch from a normal shell, or add -AllowElevated.'
    }
    if (-not $view.IsDefault) {
        foreach ($d in @($view.UserDataDir, $view.ClaudeConfigDir)) {
            if ($d -and -not [IO.Directory]::Exists($d) -and $PSCmdlet.ShouldProcess($d, 'Create directory')) {
                [void][IO.Directory]::CreateDirectory($d)
            }
        }
    }
    if ($running.Count) {
        if ($Restart) { Stop-ProfileInstances $view $running; $running = @() }
        else {
            Write-Note "'$($view.Name)' is already running (pid $(@($running | ForEach-Object { $_.Pid }) -join ', ')); bringing it to the front."
            Write-Note 'Environment and argument changes only apply after a restart (-Restart).'
            # Focusing is not "ok" when the instance is tied to a terminal.
            [void](Write-AttachedInstances $view $running)
        }
    }
    elseif ($Restart) { Write-Note "'$($view.Name)' was not running." }

    if (-not $PSCmdlet.ShouldProcess("Claude profile '$($view.Name)'", 'Launch')) { return }
    $t0 = [DateTime]::UtcNow
    $started = Start-DetachedProcess $spec $Cfg['launch']['via']
    Write-Trace "Started: $($started.Method) (pid $($started.Pid), through pid $($started.Via))"
    Write-LaunchLog "launch profile=$($view.Name) app=$($app.Version) via=$($app.Source) method=$($started.Method) exe=$($spec.Exe)"
    Update-ProfileState $view.Name @{ lastLaunched = [DateTime]::Now.ToString('o', [Globalization.CultureInfo]::InvariantCulture); lastAppVersion = $app.Version; lastExe = $app.Exe }
    $verb = if ($running.Count) { 'Focused' } else { 'Launched' }
    Write-Ok "$verb '$($view.Name)'$(if ($view.Email) { " <$($view.Email)>" }) with Claude $($app.Version)."
    if (-not $running.Count -and -not $NoVerify) { Confirm-Isolation $Cfg $view $t0 }
}

# Reads back what the launch claims, from the running app: a main process for the
# profile appears, it writes into the profile's own folder (the default profile has
# none to check), and it has no console. Catches a future app that ignores a flag.
function Confirm-Isolation($Cfg, $View, [datetime]$StartUtc) {
    $seconds = [int]$Cfg['launch']['verifySeconds']
    if ($seconds -le 0) { return }
    $deadline = $StartUtc.AddSeconds($seconds)
    $instance = $null
    $active = $View.IsDefault
    do {
        Start-Sleep -Milliseconds 500
        if (-not $instance) { $instance = @(Get-ClaudeInstances | Where-Object { Test-InstanceMatch $_ $View }) | Select-Object -First 1 }
        if (-not $active) { $active = (Get-DirActivityUtc $View.UserDataDir) -ge $StartUtc.AddSeconds(-2) }
    } while (-not ($instance -and $active) -and [DateTime]::UtcNow -lt $deadline)

    if (-not $instance -or -not $active) {
        $msg = if (-not $instance) {
            if ($View.IsDefault) { "No default instance appeared within ${seconds}s. If no window opened, run 'doctor'." }
            else { "No instance carrying this profile's --user-data-dir appeared within ${seconds}s. If no window opened, run 'doctor'." }
        }
        else { "The app started but wrote nothing to $(Compress-UserPath $View.UserDataDir) within ${seconds}s. A newer app version may be ignoring --user-data-dir; run 'doctor' before signing in." }
        Write-Warn $msg
        Write-LaunchLog "verify-warning profile=$($View.Name): $msg"
        return
    }
    if ($View.IsDefault) { Write-Ok "Verified: pid $($instance.Pid) is running." }
    else { Write-Ok "Verified: pid $($instance.Pid) is running with its data in $(Compress-UserPath $View.UserDataDir)." }
    # Both facts are read from the running app, not assumed from how it was started.
    $console = (Get-ConsoleAttachment @($instance.Pid))[$instance.Pid]
    $parentAlive = [bool](Get-Process -Id $instance.ParentPid -ErrorAction SilentlyContinue)
    if ($console -and $console.State -eq 'none' -and -not $parentAlive) {
        Write-Ok 'Detached: it has no console and its parent has exited, so closing or killing this terminal does not end it. Quitting the app does.'
        return
    }
    if ($console -and $console.State -eq 'none') {
        Write-Ok 'Detached from the console: closing this terminal does not end it. It is a child of a running process, so a tree kill of that process would.'
        return
    }
    $msg = if ($console -and $console.State -eq 'attached') {
        "It is attached to a console (shared with $(Format-ProcessList $console.Sharing $instance.Pid)), so closing that terminal ends it. This app version may ignore ELECTRON_NO_ATTACH_CONSOLE; run 'doctor'."
    }
    else { "Whether it is attached to a console could not be read ($(if ($console) { $console.Detail } else { 'no answer from the probe' })); run 'doctor'." }
    Write-Warn $msg
    Write-LaunchLog "verify-warning profile=$($View.Name): $msg"
}

#endregion

#region ---------------------------------------------------------------- commands: profiles

function Assert-NoExtraArgs {
    if ($script:RestArgs.Count) { throw "Unexpected argument(s): $($script:RestArgs -join ' '). Run 'help' for usage." }
}

function Find-ProfileKey($Cfg, [string]$Name) {
    foreach ($k in @($Cfg['profiles'].Keys)) { if ($k -ieq $Name) { return $k } }
    return $null
}

# Invariant culture throughout: a custom format's ':' is the culture's time separator,
# so under fi-FI "HH:mm" printed "12.34", and -Json output changed with the locale.
function Format-When($Value) {
    if (-not $Value) { return '' }
    $inv = [Globalization.CultureInfo]::InvariantCulture
    try { return ([datetime]::Parse([string]$Value, $inv)).ToString('yyyy-MM-dd HH:mm', $inv) } catch { return [string]$Value }
}

function Invoke-List($Cfg, $ReadResult) {
    Assert-NoExtraArgs
    $state = Read-State
    $instances = @(Get-ClaudeInstances)
    $views = @(Get-AllProfileViews $Cfg)
    $rows = foreach ($v in $views) {
        $pids = @($instances | Where-Object { Test-InstanceMatch $_ $v } | ForEach-Object { $_.Pid })
        $st = $state['profiles'][$v.Name]
        [pscustomobject]@{
            Name       = $v.Name
            Email      = $v.Email
            Aliases    = $v.Aliases -join ', '
            Running    = if ($pids.Count) { "pid $($pids -join ',')" } else { '' }
            ClaudeCode = if ($v.IsDefault) { '~/.claude' } elseif ($v.SharesClaudeConfig) { '~/.claude (shared!)' } else { Compress-UserPath $v.ClaudeConfigDir }
            AppData    = if ($v.IsDefault) { '(app default)' } else { Compress-UserPath $v.UserDataDir }
            LastLaunch = if ($st -is [Collections.IDictionary]) { Format-When $st['lastLaunched'] } else { '' }
        }
    }
    if ($Json) { ConvertTo-Json -InputObject @($rows) -Depth 4; return }
    Write-Display ($rows | Format-Table -AutoSize | Out-String -Width 4096).TrimEnd()
    foreach ($i in $instances) {
        if (-not @($views | Where-Object { Test-InstanceMatch $i $_ }).Count) {
            Write-Warn "pid $($i.Pid) runs with an unregistered folder: $($i.UserDataDir)  (see 'discover')"
        }
    }
    if (-not $ReadResult.Exists) { Write-Note "No config yet; 'add NAME -Email EMAIL' creates $($script:ConfigFile)." }
}

function Invoke-Show($Cfg) {
    Assert-NoExtraArgs
    $name = Resolve-ProfileName $Cfg $script:Tgt
    $v = Get-ProfileView $Cfg $name
    $st = (Read-State)['profiles'][$v.Name]
    $pids = @(Get-ClaudeInstances | Where-Object { Test-InstanceMatch $_ $v } | ForEach-Object { $_.Pid })
    $envShown = [ordered]@{}
    foreach ($k in @($v.Env.Keys | Sort-Object)) { $envShown[$k] = if ($null -eq $v.Env[$k]) { '(removed)' } else { Format-Secret $k $v.Env[$k] } }
    $info = [ordered]@{
        name                 = $v.Name
        email                = $v.Email
        aliases              = @($v.Aliases)
        claudeCodeSignedInAs = Get-DetectedEmail $v
        userDataDir          = if ($v.IsDefault) { '(app default)' } else { $v.UserDataDir }
        claudeConfigDir      = $v.EffectiveClaudeDir
        sharesClaudeConfig   = $v.SharesClaudeConfig
        env                  = $envShown
        args                 = @($v.Args)
        notes                = $v.Notes
        running              = @($pids)
        lastLaunched         = if ($st -is [Collections.IDictionary]) { Format-When $st['lastLaunched'] } else { '' }
        lastAppVersion       = if ($st -is [Collections.IDictionary]) { $st['lastAppVersion'] } else { $null }
    }
    if ($Json) { $info | ConvertTo-Json -Depth 5; return }
    foreach ($k in $info.Keys) {
        $val = $info[$k]
        $text = if ($val -is [Collections.IDictionary]) { (@($val.Keys | ForEach-Object { "$_=$($val[$_])" }) -join '; ') }
        elseif ($val -is [array]) { $val -join ', ' } else { [string]$val }
        Write-Line ('{0,-21}: {1}' -f $k, $text)
    }
    if ($v.Email -and $info.claudeCodeSignedInAs -and $v.Email -ine $info.claudeCodeSignedInAs) {
        Write-Warn "Configured for $($v.Email), but Claude Code in this profile is signed in as $($info.claudeCodeSignedInAs)."
    }
}

function Invoke-Add {
    Assert-NoExtraArgs
    $name = $script:Tgt
    # A name is made from an address only once the address is known to be one.
    if ($Email -and $Email -notmatch $script:EmailRx) { throw "'$Email' is not a valid email address." }
    if (-not $name) {
        if (-not $Email) { throw 'Usage: add NAME [-Email EMAIL] [-Alias a,b] [-UserDataDir DIR] [-ClaudeConfigDir DIR | -ShareClaudeConfig] [-SetEnv NAME=VALUE] [-SeedFrom default]' }
        $name = ConvertTo-ProfileName $Email (Read-Config).Config
    }
    if ($name -ieq 'default') { throw "'default' is built in. Use: set default -Email EMAIL -Alias main" }
    # Before a folder path is built from it. The save applies the same rule again.
    $problem = Get-NameProblem $name
    if ($problem) { throw "Cannot add that profile: $problem." }
    if ($ShareClaudeConfig -and $ClaudeConfigDir) { throw 'Use either -ShareClaudeConfig or -ClaudeConfigDir, not both.' }

    $res = Update-Config "add profile '$name'" {
        param($cfg)
        $root = Get-FullPath $cfg['root'] 'root'
        $udd = if ($UserDataDir) { Get-FullPath $UserDataDir '-UserDataDir' } else { [IO.Path]::Combine($root, $name) }
        $ccd = if ($ShareClaudeConfig) { $null } elseif ($ClaudeConfigDir) { Get-FullPath $ClaudeConfigDir '-ClaudeConfigDir' } else { [IO.Path]::Combine($udd, '.claude') }
        $existing = Find-ProfileKey $cfg $name
        if ($existing) {
            $v = Get-ProfileView $cfg $existing
            $diff = [Collections.Generic.List[string]]::new()
            if ($Email -and $v.Email -ine $Email) { $diff.Add("email is '$($v.Email)'") }
            if ($script:Bound.ContainsKey('UserDataDir') -and -not (Test-PathEqual $v.UserDataDir $udd)) { $diff.Add("userDataDir is '$(Compress-UserPath $v.UserDataDir)'") }
            if ($script:Bound.ContainsKey('ClaudeConfigDir') -and -not (Test-PathEqual $v.ClaudeConfigDir $ccd)) { $diff.Add("claudeConfigDir is '$(Compress-UserPath $v.EffectiveClaudeDir)'") }
            if ($ShareClaudeConfig -and -not $v.SharesClaudeConfig) { $diff.Add('it has its own Claude Code config') }
            foreach ($a in @($Alias | Where-Object { $_ })) { if (@($v.Aliases) -inotcontains $a) { $diff.Add("alias '$a' is not set") } }
            if ($diff.Count) { throw "Profile '$existing' already exists and differs ($($diff -join '; ')). Use 'set' to change it." }
            return 'unchanged'
        }
        if ([IO.Directory]::Exists($udd) -and -not $Force) {
            # A folder holding only the .claude dir this script creates counts as ours.
            $entries = @([IO.Directory]::EnumerateFileSystemEntries($udd) | Where-Object { [IO.Path]::GetFileName($_) -ne '.claude' } | Select-Object -First 1)
            if ($entries.Count -and -not [IO.File]::Exists([IO.Path]::Combine($udd, 'Local State'))) {
                throw "'$udd' exists and doesn't look like a Claude profile folder. Pick another -UserDataDir, or add -Force to use it anyway."
            }
        }
        $entry = [ordered]@{}
        if ($Email) { $entry['email'] = $Email }
        $entry['aliases'] = @($Alias | Where-Object { $_ } | Select-Object -Unique)
        $entry['userDataDir'] = Compress-UserPath $udd
        if ($ShareClaudeConfig) { $entry['shareClaudeConfig'] = $true } else { $entry['claudeConfigDir'] = Compress-UserPath $ccd }
        $entry['env'] = [ordered]@{}
        foreach ($k in $script:EnvSettings.Keys) { $entry['env'][$k] = $script:EnvSettings[$k] }
        $entry['args'] = @($ProfileArgs | Where-Object { $_ })
        if ($Notes) { $entry['notes'] = $Notes }
        $cfg['profiles'][$name] = $entry
        return $(if ([IO.Directory]::Exists($udd)) { 'adopted' } else { 'added' })
    }

    if ($res.Result -eq 'unchanged') { Write-Ok "Profile '$name' already exists with these settings; nothing to do." }
    elseif (-not $res.Changed) { return }
    else {
        $v = Get-ProfileView $res.Config $name
        foreach ($d in @($v.UserDataDir, $v.ClaudeConfigDir)) { if ($d) { [void][IO.Directory]::CreateDirectory($d) } }
        if ($res.Result -eq 'adopted') { Write-Ok "Added '$name', adopting the existing data in $(Compress-UserPath $v.UserDataDir)." }
        else { Write-Ok "Added '$name'." }
        Write-Line "  app data    : $(Compress-UserPath $v.UserDataDir)"
        Write-Line "  claude code : $(Compress-UserPath $v.EffectiveClaudeDir)$(if ($v.SharesClaudeConfig) { ' (shared with default)' })"
    }
    if ($SeedFrom) { Invoke-Seed (Read-Config).Config $name $SeedFrom }
    if ($res.Changed) {
        $handle = if ($Alias) { @($Alias)[0] } else { $name }
        Write-Line ''
        Write-Line "Launch it   : $($script:ToolName) $handle      (desktop and start-menu icon: $($script:ToolName) shortcut $name)"
        Write-Line 'Signing in  : use the email code inside the window. Browser sign-in returns through a claude:// link,'
        Write-Line '              which Windows always hands to the default profile.'
    }
}

function Invoke-Set {
    Assert-NoExtraArgs
    if (-not $script:Tgt) { throw 'Usage: set PROFILE [-Email ..] [-Alias ..] [-RemoveAlias ..] [-SetEnv NAME=VALUE] [-UnsetEnv ..] [-Notes ..] [-ProfileArgs ..] [-UserDataDir ..] [-ClaudeConfigDir .. | -ShareClaudeConfig | -IsolateClaudeConfig]' }
    $knobs = @('Email', 'Alias', 'RemoveAlias', 'SetEnv', 'UnsetEnv', 'Notes', 'ProfileArgs', 'UserDataDir', 'ClaudeConfigDir', 'ShareClaudeConfig', 'IsolateClaudeConfig')
    if (-not @($knobs | Where-Object { $script:Bound.ContainsKey($_) }).Count) { throw "Nothing to change. Options: -$($knobs -join ', -')" }
    if (@('ClaudeConfigDir', 'ShareClaudeConfig', 'IsolateClaudeConfig' | Where-Object { $script:Bound.ContainsKey($_) }).Count -gt 1) {
        throw 'Use only one of -ClaudeConfigDir, -ShareClaudeConfig, -IsolateClaudeConfig.'
    }
    $res = Update-Config "update profile '$($script:Tgt)'" {
        param($cfg)
        $name = Resolve-ProfileName $cfg $script:Tgt
        $key = Find-ProfileKey $cfg $name
        if (-not $key) { $key = 'default'; $cfg['profiles'][$key] = [ordered]@{} }
        $p = $cfg['profiles'][$key]
        if ($name -eq 'default') {
            foreach ($k in 'UserDataDir', 'ClaudeConfigDir', 'ShareClaudeConfig', 'IsolateClaudeConfig', 'ProfileArgs') {
                if ($script:Bound.ContainsKey($k)) { throw "-$k is not allowed on the built-in default profile." }
            }
        }
        if ($script:Bound.ContainsKey('Email')) {
            if ($Email) { if ($Email -notmatch $script:EmailRx) { throw "'$Email' is not a valid email address." }; $p['email'] = $Email }
            else { $p.Remove('email') }
        }
        if ($script:Bound.ContainsKey('Alias') -or $script:Bound.ContainsKey('RemoveAlias')) {
            $cur = @(if (Test-StringArray $p['aliases']) { $p['aliases'] })
            foreach ($a in @($Alias | Where-Object { $_ })) { if ($cur -inotcontains $a) { $cur += $a } }
            foreach ($a in @($RemoveAlias | Where-Object { $_ })) {
                if ($cur -icontains $a) { $cur = @($cur | Where-Object { $_ -ine $a }) } else { Write-Warn "'$name' has no alias '$a'." }
            }
            $p['aliases'] = @($cur)
        }
        if ($SetEnv -or $UnsetEnv) {
            if ($p['env'] -isnot [Collections.IDictionary]) { $p['env'] = [ordered]@{} }
            $envd = $p['env']
            foreach ($k in $script:EnvSettings.Keys) {
                # One variable whatever its case: Windows reads "http_proxy" and
                # "HTTP_PROXY" as one, so the new spelling replaces the old entry.
                foreach ($old in @($envd.Keys | Where-Object { $_ -ieq $k -and $_ -cne $k })) { $envd.Remove($old) }
                $envd[$k] = $script:EnvSettings[$k]
            }
            foreach ($k in @($UnsetEnv | Where-Object { $_ })) {
                $hit = @($envd.Keys | Where-Object { $_ -ieq $k })
                if ($hit.Count) { foreach ($h in $hit) { $envd.Remove($h) } } else { Write-Warn "'$name' does not set $k." }
            }
        }
        if ($script:Bound.ContainsKey('Notes')) { if ($Notes) { $p['notes'] = $Notes } else { $p.Remove('notes') } }
        if ($script:Bound.ContainsKey('ProfileArgs')) { $p['args'] = @($ProfileArgs | Where-Object { $_ }) }
        if ($script:Bound.ContainsKey('UserDataDir')) {
            $new = Get-FullPath $UserDataDir '-UserDataDir'
            $old = Get-FullPath $p['userDataDir'] 'userDataDir'
            if (-not (Test-PathEqual $new $old)) {
                $hasData = [IO.Directory]::Exists($old) -and @([IO.Directory]::EnumerateFileSystemEntries($old) | Select-Object -First 1).Count
                if ($hasData -and -not $Force) {
                    throw "Changing userDataDir does not move data: '$old' would be left behind and '$name' would start signed out. Move the folder first, or add -Force."
                }
                if (-not $p['claudeConfigDir'] -and $p['shareClaudeConfig'] -ne $true) {
                    $keep = [IO.Path]::Combine($old, '.claude')
                    $p['claudeConfigDir'] = Compress-UserPath $keep
                    Write-Note "claudeConfigDir stays at $(Compress-UserPath $keep); change it with -ClaudeConfigDir."
                }
                $p['userDataDir'] = Compress-UserPath $new
            }
        }
        if ($script:Bound.ContainsKey('ClaudeConfigDir')) { $p.Remove('shareClaudeConfig'); $p['claudeConfigDir'] = Compress-UserPath (Get-FullPath $ClaudeConfigDir '-ClaudeConfigDir') }
        if ($ShareClaudeConfig) { $p.Remove('claudeConfigDir'); $p['shareClaudeConfig'] = $true }
        if ($IsolateClaudeConfig) {
            $p.Remove('shareClaudeConfig')
            if (-not $p['claudeConfigDir']) { $p['claudeConfigDir'] = Compress-UserPath ([IO.Path]::Combine((Get-FullPath $p['userDataDir'] 'userDataDir'), '.claude')) }
        }
        return $name
    }
    if (-not $res.Changed) { if (-not $WhatIfPreference) { Write-Ok 'No change.' }; return }
    $name = $res.Result
    Write-Ok "Updated '$name'."
    $needsRestart = @('SetEnv', 'UnsetEnv', 'ProfileArgs', 'UserDataDir', 'ClaudeConfigDir', 'ShareClaudeConfig', 'IsolateClaudeConfig' | Where-Object { $script:Bound.ContainsKey($_) }).Count
    $v = Get-ProfileView $res.Config $name
    if ($needsRestart -and @(Get-ClaudeInstances | Where-Object { Test-InstanceMatch $_ $v }).Count) { Write-Note "'$name' is running; restart it to apply: $($script:ToolName) $name -Restart" }
}

# Positional words are split on commas too: through pwsh -File and the .cmd entry,
# "alias work a,b" arrives as the one word "a,b", as -Alias a,b does.
function Invoke-Alias {
    $aliases = @(Split-ListValue (@($script:RestArgs) + @($Alias)))
    $script:RestArgs = @()
    if (-not $script:Tgt -or -not $aliases.Count) { throw 'Usage: alias PROFILE ALIAS [ALIAS ...]' }
    $res = Update-Config "add alias(es) to '$($script:Tgt)'" {
        param($cfg)
        $name = Resolve-ProfileName $cfg $script:Tgt
        $key = Find-ProfileKey $cfg $name
        if (-not $key) { $key = 'default'; $cfg['profiles'][$key] = [ordered]@{} }
        $p = $cfg['profiles'][$key]
        $cur = [Collections.Generic.List[string]]::new()
        if (Test-StringArray $p['aliases']) { foreach ($a in $p['aliases']) { $cur.Add($a) } }
        foreach ($a in $aliases) { if (-not ($cur -icontains $a)) { $cur.Add($a) } }
        $p['aliases'] = @($cur)
        return $name
    }
    if ($res.Changed) { Write-Ok "'$($res.Result)' now answers to: $((Get-ProfileView $res.Config $res.Result).Aliases -join ', ')" }
    elseif (-not $WhatIfPreference) { Write-Ok 'Already set; no change.' }
}

function Invoke-Unalias {
    $aliases = @(Split-ListValue (@($script:Tgt) + @($script:RestArgs)))
    $script:RestArgs = @()
    if (-not $aliases.Count) { throw 'Usage: unalias ALIAS [ALIAS ...]' }
    $res = Update-Config 'remove alias(es)' {
        param($cfg)
        $removed = [Collections.Generic.List[string]]::new()
        foreach ($k in @($cfg['profiles'].Keys)) {
            $p = $cfg['profiles'][$k]
            if ($p -isnot [Collections.IDictionary] -or -not (Test-StringArray $p['aliases'])) { continue }
            $keep = @($p['aliases'] | Where-Object { $aliases -inotcontains $_ })
            if ($keep.Count -ne @($p['aliases']).Count) { $p['aliases'] = $keep; $removed.Add($k) }
        }
        return $removed
    }
    if ($res.Changed) { Write-Ok "Removed from: $(@($res.Result) -join ', ')" } elseif (-not $WhatIfPreference) { Write-Warn 'No such alias.' }
}

function Invoke-Rename {
    $newName = if ($script:RestArgs.Count) { [string]$script:RestArgs[0] } else { $null }
    $script:RestArgs = @($script:RestArgs | Select-Object -Skip 1)
    Assert-NoExtraArgs
    if (-not $script:Tgt -or -not $newName) { throw 'Usage: rename PROFILE NEW-NAME' }
    if ($newName -ieq 'default') { throw "'default' is built in; pick another name." }
    $problem = Get-NameProblem $newName
    if ($problem) { throw "Cannot rename to that: $problem." }
    $before = (Read-Config).Config
    $oldName = Resolve-ProfileName $before $script:Tgt
    if ($oldName -eq 'default') { throw 'The built-in default profile cannot be renamed.' }
    # The profile's shortcuts are renamed with it. Whatever could stop that is checked
    # before the config changes, so a refusal leaves the config and the shortcuts as
    # they were, and a new shortcut exists before the old one is removed.
    $links = @(foreach ($dir in Get-ShortcutDirs 'Both') {
            $old = Get-ShortcutPath $dir $oldName
            if (Test-ShortcutOwned $old) { [pscustomobject]@{ Dir = $dir; Old = $old; New = Get-ShortcutPath $dir $newName } }
        })
    $app = $null
    if ($links.Count) {
        foreach ($l in $links) {
            if (-not (Test-PathEqual $l.Old $l.New) -and [IO.File]::Exists($l.New) -and -not (Test-ShortcutOwned $l.New)) {
                throw "$($l.New) exists and is not this configuration's shortcut. Nothing was renamed."
            }
        }
        Assert-ShortcutScript
        $app = Find-ClaudeApp $before
    }
    $res = Update-Config "rename profile '$oldName' to '$newName'" {
        param($cfg)
        $old = Resolve-ProfileName $cfg $script:Tgt
        if ($old -cne $oldName) { throw 'The config changed while this command ran. Nothing was renamed; run it again.' }
        $clash = Find-ProfileKey $cfg $newName
        if ($clash -and $clash -cne $old) { throw "A profile named '$clash' already exists." }
        $rebuilt = [ordered]@{}
        foreach ($k in @($cfg['profiles'].Keys)) { if ($k -ceq $old) { $rebuilt[$newName] = $cfg['profiles'][$k] } else { $rebuilt[$k] = $cfg['profiles'][$k] } }
        $cfg['profiles'] = $rebuilt
        return $old
    }
    if (-not $res.Changed) { return }
    Rename-ProfileState $oldName $newName
    Write-Ok "Renamed '$oldName' to '$newName'. Its folders did not move."
    if (-not $links.Count) { return }
    Set-ProfileShortcut (Get-ProfileView $res.Config $newName) $app @($links | ForEach-Object { $_.Dir })
    foreach ($l in $links) {
        # A rename that only changes case names the same file twice.
        if (-not (Test-PathEqual $l.Old $l.New) -and [IO.File]::Exists($l.New)) { Remove-Contained $l.Old $l.Dir }
    }
}

# Refuses to move a folder that is not one profile's alone: one off a drive letter, a
# drive root, a protected folder or one holding it, or a folder another profile uses.
# Test-Config refuses a config that names one, but a hand edit can still hold one,
# and this is the last check before data moves.
function Assert-SafeToTrash([string]$Path, [object[]]$Others) {
    if (-not (Test-LocalDrivePath $Path)) { throw "Refusing to move '$Path': it is not on a drive letter. Nothing was moved." }
    if ([IO.Path]::GetPathRoot($Path).TrimEnd('\') -ieq $Path.TrimEnd('\')) { throw "Refusing to move '$Path': it is a drive root. Nothing was moved." }
    foreach ($pd in Get-ProtectedDirs) {
        if (Test-PathOverlap $Path $pd.Path) { throw "Refusing to move '$Path': it overlaps $($pd.Why). Nothing was moved." }
    }
    foreach ($o in $Others) {
        foreach ($d in @($o.UserDataDir, $o.EffectiveClaudeDir)) {
            if ($d -and (Test-PathOverlap $Path $d)) { throw "Refusing to move '$Path': profile '$($o.Name)' uses '$d'. Nothing was moved." }
        }
    }
}

# A profile's data is never deleted: it is moved beside itself, into .cc-trash, on
# the same volume so the move is a rename. The state is read back afterwards.
function Move-ToTrash {
    [CmdletBinding(SupportsShouldProcess)]
    param([string]$Path, [string]$Label)
    if (-not [IO.Directory]::Exists($Path)) { return }
    $trash = [IO.Path]::Combine([IO.Path]::GetDirectoryName($Path), '.cc-trash')
    $stamp = [DateTime]::Now.ToString('yyyyMMdd-HHmmss', [Globalization.CultureInfo]::InvariantCulture)
    # The folder's own name goes into the destination, then a counter when that is
    # taken, so two folders of one profile moved in the same second never collide.
    $leaf = [IO.Path]::GetFileName($Path)
    $base = if ($leaf -ieq $Label) { "$Label-$stamp" } else { "$Label-$leaf-$stamp" }
    $dest = [IO.Path]::Combine($trash, $base)
    for ($n = 2; [IO.Directory]::Exists($dest) -or [IO.File]::Exists($dest); $n++) { $dest = [IO.Path]::Combine($trash, "$base-$n") }
    if (-not $PSCmdlet.ShouldProcess($Path, "Move to $dest")) { return }
    [void][IO.Directory]::CreateDirectory($trash)
    [IO.Directory]::Move($Path, $dest)
    if ([IO.Directory]::Exists($Path) -or -not [IO.Directory]::Exists($dest)) {
        throw "Moving '$Path' to '$dest' did not take effect; check both folders."
    }
    Write-Ok "Moved $Path -> $dest (delete it yourself once you are sure)."
}

# The one deletion in this tool. It refuses a path outside $Root, and $Root itself:
# "root\..." and "root\." both normalise to the root. It reads the state back, so a
# removal that failed is reported as failed. TODO/RULES.md section 3.
function Remove-Contained([string]$Path, [string]$Root) {
    $full = [IO.Path]::GetFullPath($Path).TrimEnd('\')
    $rootFull = [IO.Path]::GetFullPath($Root).TrimEnd('\')
    if (-not (Test-PathWithin $full $rootFull)) { throw "Refusing to remove '$full': it is not inside '$rootFull'." }
    if (-not (Test-Path -LiteralPath $full)) { return }
    Remove-Item -LiteralPath $full -Recurse -Force
    if (Test-Path -LiteralPath $full) { throw "Removing '$full' did not take effect; it is still there." }
}

function Invoke-Remove {
    [CmdletBinding(SupportsShouldProcess)]
    param($Cfg, [switch]$Force)
    Assert-NoExtraArgs
    $name = Resolve-ProfileName $Cfg $script:Tgt
    if ($name -eq 'default') { throw "The built-in default profile can't be removed (clear its details with: set default -Email '')." }
    $v = Get-ProfileView $Cfg $name
    $dirs = @($v.UserDataDir) + @(if ($v.ClaudeConfigDir -and -not (Test-PathWithin $v.ClaudeConfigDir $v.UserDataDir)) { $v.ClaudeConfigDir })
    if ($PurgeData) {
        # Everything that could refuse is asked before anything moves, and the folders
        # move before the entry goes. So a refusal leaves both, and a failed move
        # leaves the entry that names what is left.
        $running = @(Get-ClaudeInstances | Where-Object { Test-InstanceMatch $_ $v })
        if ($running.Count) { throw "'$name' is running (pid $(@($running | ForEach-Object { $_.Pid }) -join ', ')). Quit it first." }
        $others = @(Get-AllProfileViews $Cfg | Where-Object { $_.Name -ne $name })
        foreach ($d in $dirs) { Assert-SafeToTrash $d $others }
        $left = @((Test-Config $Cfg).Issues | Where-Object { $_.Level -eq 'error' -and $_.Profiles -notcontains $name } | ForEach-Object { $_.Message })
        if ($left.Count) { throw ("Removing '$name' could not be saved, because the config has other problems. Nothing was moved. Fix them first:`n  - " + ($left -join "`n  - ")) }
        if (-not (Read-Config).Strict -and -not $Force) {
            throw "The config holds comments or other text that is not strict JSON, and removing '$name' would rewrite it without them. Nothing was moved. Take them out with 'edit', or add -Force."
        }
        if (-not $Force -and -not $WhatIfPreference) {
            if (-not $PSCmdlet.ShouldContinue("Move these folders of '$name' to a .cc-trash folder next to them?`n  $($dirs -join "`n  ")", 'Purge profile data')) { throw 'Cancelled.' }
        }
        foreach ($d in $dirs) { Move-ToTrash $d $name }
    }
    $res = Update-Config "remove profile '$name'" {
        param($cfg)
        $k = Find-ProfileKey $cfg $name
        if ($k) { $cfg['profiles'].Remove($k) }
        return $k
    }
    if (-not $res.Changed) { return }
    Write-Ok "Removed '$name' from the config (previous version backed up in $(Compress-UserPath $script:BackupDir))."
    foreach ($dir in Get-ShortcutDirs 'Both') {
        $lnk = Get-ShortcutPath $dir $name
        if (-not [IO.File]::Exists($lnk)) { continue }
        if (Test-ShortcutOwned $lnk) { Remove-Contained $lnk $dir; Write-Ok "Removed shortcut $lnk" }
        else { Write-Note "Left $lnk as it is: another configuration launches it, or $($script:ToolName) did not make it." }
    }
    if (-not $PurgeData) { Write-Note "Data kept: $(@($dirs | ForEach-Object { Compress-UserPath $_ }) -join ', '). Re-adding '$name' adopts it again." }
}

function Invoke-Status($Cfg) {
    Assert-NoExtraArgs
    $views = @(Get-AllProfileViews $Cfg)
    $instances = @(Get-ClaudeInstances)
    $states = Get-ConsoleAttachment @($instances | ForEach-Object { $_.Pid })
    $rows = foreach ($i in $instances) {
        $owner = @($views | Where-Object { Test-InstanceMatch $i $_ } | ForEach-Object { $_.Name }) | Select-Object -First 1
        $c = $states[$i.Pid]
        [pscustomobject]@{
            Pid         = $i.Pid
            Profile     = if ($owner) { $owner } else { '(unregistered)' }
            Console     = if (-not $c) { 'unknown (no answer)' } elseif ($c.State -eq 'attached') { "ATTACHED: $(Format-ProcessList $c.Sharing $i.Pid)" } elseif ($c.State -eq 'unknown') { "unknown ($($c.Detail))" } else { $c.State }
            Started     = if ($i.Started) { ([datetime]$i.Started).ToString('yyyy-MM-dd HH:mm:ss', [Globalization.CultureInfo]::InvariantCulture) } else { '' }
            UserDataDir = if ($i.UserDataDir) { Compress-UserPath $i.UserDataDir } else { '(app default)' }
        }
    }
    if ($Json) { ConvertTo-Json -InputObject @($rows) -Depth 3; return }
    if (-not @($rows).Count) { Write-Note 'No Claude desktop instances are running.'; return }
    Write-Display ($rows | Format-Table -AutoSize | Out-String -Width 4096).TrimEnd()
    if (@($rows | Where-Object { $_.Console -like 'ATTACHED*' }).Count) {
        Write-Warn "An ATTACHED instance prints its log into that terminal and ends when it closes; 'doctor' says how to restart it detached."
    }
    if (@($rows | Where-Object { $_.Console -like 'unknown*' }).Count) {
        Write-Warn "The console of an instance marked unknown could not be read, so whether a terminal can end it is not known."
    }
}

#endregion

#region ---------------------------------------------------------------- commands: seed, shortcuts, discover

function Invoke-Seed {
    [CmdletBinding(SupportsShouldProcess)]
    param($Cfg, [string]$ProfileId, [string]$From)
    $name = Resolve-ProfileName $Cfg $ProfileId
    # Seed writes into the profile's folder, so a folder the config may not name is
    # refused here, as a launch refuses it.
    Assert-ConfigUsable $Cfg $name
    $v = Get-ProfileView $Cfg $name
    if ($v.SharesClaudeConfig) { throw "'$name' uses ~/.claude directly; there is nothing to seed." }
    $src = if (-not $From -or $From -ieq 'default') { [IO.Path]::Combine($HOME, '.claude') }
    elseif ($From -match '[\\/:~]') { Get-FullPath $From '-SeedFrom' }
    else { (Get-ProfileView $Cfg (Resolve-ProfileName $Cfg $From)).EffectiveClaudeDir }
    if (Test-PathOverlap $src $v.ClaudeConfigDir) { throw "The source and the destination overlap ('$src' and '$($v.ClaudeConfigDir)')." }
    if (-not [IO.Directory]::Exists($src)) { throw "Source folder '$src' does not exist." }
    $items = if ($Include) { @($Include) } else { $script:SeedDefault }
    foreach ($item in $items) {
        # Windows drops a trailing dot or space, so ".credentials.json." opens the
        # credentials file. A name is taken only as it is spelled on disk.
        if ($item -match '[\\/:*?"<>|]' -or $item -in '.', '..' -or $item -cne $item.Trim() -or $item.EndsWith('.')) {
            throw "'$item': name a top-level file or folder of the Claude Code config, exactly as it is spelled."
        }
        if ($script:SeedBlocked -contains $item) { throw "'$item' holds account or session data and is never copied between profiles." }
    }
    if ($items -contains 'plugins') { Write-Warn "plugins/ stores absolute paths into '$src'; plugins may need reinstalling in '$name'." }
    # Items are found by their long names only, so a short 8.3 alias of a refused
    # name, such as CREDEN~1.JSO, matches nothing.
    $onDisk = @{}
    foreach ($e in [IO.DirectoryInfo]::new($src).EnumerateFileSystemInfos()) { $onDisk[$e.Name] = $e }
    Write-Line "Seeding '$name' from $(Compress-UserPath $src):"
    if ($PSCmdlet.ShouldProcess($v.ClaudeConfigDir, 'Create directory')) { [void][IO.Directory]::CreateDirectory($v.ClaudeConfigDir) }
    $backupRoot = [IO.Path]::Combine($script:BackupDir, 'seed', ('{0}-{1}' -f $name, [DateTime]::Now.ToString('yyyyMMdd-HHmmss', [Globalization.CultureInfo]::InvariantCulture)))
    foreach ($item in $items) {
        $entry = $onDisk[$item]
        if (-not $entry) { Write-Line "  skip  $item (not in source)" -Color DarkGray; continue }
        $s = $entry.FullName
        $d = [IO.Path]::Combine($v.ClaudeConfigDir, $entry.Name)
        if ([IO.File]::Exists($d) -or [IO.Directory]::Exists($d)) {
            if (-not $Force) { Write-Line "  keep  $($entry.Name) (already present; -Force replaces it)" -Color DarkGray; continue }
            if (-not $PSCmdlet.ShouldProcess($d, 'Replace (old copy is backed up)')) { continue }
            [void][IO.Directory]::CreateDirectory($backupRoot)
            $saved = [IO.Path]::Combine($backupRoot, $entry.Name)
            Copy-Item -LiteralPath $d -Destination $saved -Recurse -Force
            if (-not ([IO.File]::Exists($saved) -or [IO.Directory]::Exists($saved))) { throw "Backing up '$d' failed; nothing was replaced." }
            Remove-Contained $d $v.ClaudeConfigDir
        }
        elseif (-not $PSCmdlet.ShouldProcess($d, "Copy from $s")) { continue }
        Copy-Item -LiteralPath $s -Destination $d -Recurse -Force
        Write-Line "  copy  $($entry.Name)" -Color Green
    }
}

function Get-ShortcutDirs([string]$Where) {
    $out = @()
    if ($Where -in 'Desktop', 'Both') { $out += [Environment]::GetFolderPath('Desktop') }
    if ($Where -in 'StartMenu', 'Both') { $out += [IO.Path]::Combine([Environment]::GetFolderPath('Programs'), 'Claude Profiles') }
    return $out
}

function Get-ShortcutPath([string]$Dir, [string]$Name) { return [IO.Path]::Combine($Dir, "Claude ($Name).lnk") }

# The config a shortcut launches with: its -ConfigPath, or ~/.cc_profiles.json when it
# names none. $null for a shortcut this tool did not write; nothing here touches one.
function Get-ShortcutConfig([string]$Lnk) {
    if (-not [IO.File]::Exists($Lnk)) { return $null }
    try {
        $argv = @(Split-CommandLine (New-Object -ComObject WScript.Shell).CreateShortcut($Lnk).Arguments)
        if ($argv -notcontains '-FromShortcut') { return $null }
        $i = [array]::IndexOf($argv, '-ConfigPath')
        return Get-FullPath $(if ($i -ge 0 -and $i + 1 -lt $argv.Count) { $argv[$i + 1] } else { '~/.cc_profiles.json' })
    }
    catch { return $null }
}

# Shortcuts are named for the profile alone, so a profile of the same name in another
# config has a shortcut of the same name. Only this config's are changed or removed.
function Test-ShortcutOwned([string]$Lnk) {
    $c = Get-ShortcutConfig $Lnk
    return [bool]($c -and (Test-PathEqual $c $script:ConfigFile))
}

# pwsh from the Store lives in a versioned folder; its execution alias survives updates.
function Get-StablePwshPath {
    $p = (Get-Process -Id $PID).Path
    if ($p -match '\\WindowsApps\\' -and $env:LOCALAPPDATA) {
        $alias = [IO.Path]::Combine($env:LOCALAPPDATA, 'Microsoft', 'WindowsApps', 'pwsh.exe')
        if ([IO.File]::Exists($alias)) { return $alias }
    }
    return $p
}

function Test-TransientScriptPath {
    $sp = $script:ScriptPath
    $places = @($env:TEMP, $env:TMP) + @(if ($env:APPDATA) { [IO.Path]::Combine($env:APPDATA, 'Claude') }) + @(if ($env:LOCALAPPDATA) { [IO.Path]::Combine($env:LOCALAPPDATA, 'Packages') })
    foreach ($t in $places) {
        if ($t -and (Test-PathWithin $sp ([IO.Path]::GetFullPath($t)))) { return $true }
    }
    return $false
}

# A shortcut runs this script by its path, so a shortcut to a copy in a temporary
# folder breaks when that folder is cleaned.
function Assert-ShortcutScript {
    if ((Test-TransientScriptPath) -and -not $Force) {
        throw "This script runs from a temporary folder ($($script:ScriptPath)), and a shortcut to it would break when that folder is cleaned. Run 'install' and use the installed copy, or add -Force."
    }
}

# Writes the profile's shortcut into each of $Dirs. Every door that writes one comes
# through here (shortcut, rename and install), so the checks are here too.
function Set-ProfileShortcut {
    [CmdletBinding(SupportsShouldProcess)]
    param($View, $App, [string[]]$Dirs)
    Assert-ShortcutScript
    $links = @($Dirs | ForEach-Object { Get-ShortcutPath $_ $View.Name })
    foreach ($lnk in $links) {
        if ([IO.File]::Exists($lnk) -and -not (Test-ShortcutOwned $lnk)) {
            throw "$lnk is not this configuration's shortcut (another configuration launches it, or $($script:ToolName) did not make it), so nothing was written. Remove it yourself, or rename this profile."
        }
    }
    $pwsh = Get-StablePwshPath
    $icon = if ($WhatIfPreference) { $App.Exe } else { Get-StableIconPath $App }
    # -ConfigPath always: a shortcut that named no config would follow whatever
    # CC_PROFILES_CONFIG holds when it is clicked.
    $argText = '-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -WindowStyle Hidden -File {0} launch {1} -FromShortcut -ConfigPath {2}' -f (Format-CommandLineArg $script:ScriptPath -Always), (Format-CommandLineArg $View.Name -Always), (Format-CommandLineArg $script:ConfigFile -Always)
    $shell = New-Object -ComObject WScript.Shell
    foreach ($lnk in $links) {
        if (-not $PSCmdlet.ShouldProcess($lnk, 'Create or update shortcut')) { continue }
        [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($lnk))
        $s = $shell.CreateShortcut($lnk)
        $s.TargetPath = $pwsh
        $s.Arguments = $argText
        $s.WorkingDirectory = [IO.Path]::GetFullPath($HOME)
        $s.IconLocation = "$icon,0"
        $s.WindowStyle = 7
        $s.Description = "Claude desktop - profile $($View.Name)$(if ($View.Email) { " ($($View.Email))" })"
        $s.Save()
        Write-Ok "Shortcut: $lnk"
    }
}

function Invoke-Shortcut {
    [CmdletBinding(SupportsShouldProcess)]
    param($Cfg)
    Assert-NoExtraArgs
    if (-not $All -and -not $script:Tgt) { throw 'Usage: shortcut PROFILE [-Location Desktop|StartMenu|Both] [-Remove]   or   shortcut -All' }
    $views = if ($All) { @(Get-AllProfileViews $Cfg | Where-Object { -not $_.IsDefault }) } else { @(Get-ProfileView $Cfg (Resolve-ProfileName $Cfg $script:Tgt)) }
    if ($Remove) {
        foreach ($v in $views) {
            foreach ($dir in Get-ShortcutDirs $Location) {
                $lnk = Get-ShortcutPath $dir $v.Name
                if (-not [IO.File]::Exists($lnk)) { continue }
                if (-not (Test-ShortcutOwned $lnk)) { Write-Note "Left $lnk as it is: another configuration launches it, or $($script:ToolName) did not make it."; continue }
                if ($PSCmdlet.ShouldProcess($lnk, 'Remove shortcut')) { Remove-Contained $lnk $dir; Write-Ok "Removed $lnk" }
            }
        }
        return
    }
    # Every refusal before the first shortcut is written, so -All writes all or none.
    Assert-ShortcutScript
    $foreign = @(foreach ($v in $views) {
            foreach ($dir in Get-ShortcutDirs $Location) {
                $lnk = Get-ShortcutPath $dir $v.Name
                if ([IO.File]::Exists($lnk) -and -not (Test-ShortcutOwned $lnk)) { $lnk }
            }
        })
    if ($foreign.Count) { throw "Not this configuration's shortcuts, so nothing was written: $($foreign -join ', '). Remove them yourself, or rename those profiles." }
    $app = Find-ClaudeApp $Cfg
    if (-not $WhatIfPreference) { [void](Get-StableIconPath $app -Refresh) }
    foreach ($v in $views) { Set-ProfileShortcut $v $app (Get-ShortcutDirs $Location) }
}

function Invoke-Discover($Cfg) {
    Assert-NoExtraArgs
    $root = Get-FullPath $Cfg['root'] 'root'
    $views = @(Get-AllProfileViews $Cfg)
    $known = @($views | Where-Object { -not $_.IsDefault } | ForEach-Object { $_.UserDataDir })
    $isKnown = { param($d) [bool]@($known | Where-Object { Test-PathEqual $_ $d }).Count }
    $found = [Collections.Generic.List[object]]::new()
    if ([IO.Directory]::Exists($root)) {
        foreach ($d in Get-ChildItem -LiteralPath $root -Directory -Force | Where-Object { $_.Name -notlike '.*' }) {
            if ([IO.File]::Exists([IO.Path]::Combine($d.FullName, 'Local State')) -and -not (& $isKnown $d.FullName)) {
                $found.Add([pscustomobject]@{ Dir = $d.FullName; Source = 'folder' })
            }
        }
    }
    foreach ($i in @(Get-ClaudeInstances)) {
        if ($i.UserDataDir -and -not (& $isKnown $i.UserDataDir) -and -not @($found | Where-Object { Test-PathEqual $_.Dir $i.UserDataDir }).Count) {
            $found.Add([pscustomobject]@{ Dir = $i.UserDataDir; Source = "running (pid $($i.Pid))" })
        }
    }
    if (-not $found.Count) { Write-Ok "Nothing unregistered under $(Compress-UserPath $root) or among running instances."; return }
    foreach ($f in $found) { Write-Line "  $(Compress-UserPath $f.Dir)   [$($f.Source)]" }
    if (-not $Register) { Write-Line ''; Write-Line "Register them with: $($script:ToolName) discover -Register   (or add NAME -UserDataDir DIR -Email EMAIL)"; return }
    $res = Update-Config 'register discovered profiles' {
        param($cfg)
        $added = [Collections.Generic.List[string]]::new()
        foreach ($f in $found) {
            $taken = Get-IdentifierMap $cfg
            $base = ([IO.Path]::GetFileName($f.Dir).ToLowerInvariant() -replace '[^a-z0-9._-]', '-').Trim('.', '_', '-')
            # 48 leaves room for the "-99" a taken name gets, inside the 64 a name may have.
            if ($base.Length -gt 48) { $base = $base.Substring(0, 48).Trim('.', '_', '-') }
            if (-not $base) { $base = 'profile' }
            $name = @(@($base) + @(2..99 | ForEach-Object { "$base-$_" }) | Where-Object { -not (Get-NameProblem $_) -and -not $taken.ContainsKey($_) }) | Select-Object -First 1
            if (-not $name) { Write-Warn "Skipping $(Compress-UserPath $f.Dir): no free profile name could be made from its folder name. Add it with: add NAME -UserDataDir DIR"; continue }
            $cfg['profiles'][$name] = [ordered]@{
                aliases = @(); userDataDir = Compress-UserPath $f.Dir
                claudeConfigDir = Compress-UserPath ([IO.Path]::Combine($f.Dir, '.claude')); env = [ordered]@{}; args = @()
            }
            # A folder the config may not name, such as one under AppData that an
            # instance started by hand uses, is left out so it does not block the rest.
            $bad = @((Test-Config $cfg).Issues | Where-Object { $_.Level -eq 'error' -and $_.Profiles -contains $name } | ForEach-Object { $_.Message })
            if ($bad.Count) { $cfg['profiles'].Remove($name); Write-Warn "Skipping $(Compress-UserPath $f.Dir): $($bad[0])"; continue }
            $added.Add($name)
        }
        return $added
    }
    if ($res.Changed) {
        Write-Ok "Registered: $(@($res.Result) -join ', '). Add details with: set NAME -Email EMAIL -Alias X"
        Write-Note 'Claude Code in these profiles now gets its own config dir; copy settings over with: seed NAME'
    }
}

#endregion
#region ---------------------------------------------------------------- commands: doctor & config maintenance

function Invoke-Doctor($Cfg, $ReadResult) {
    Assert-NoExtraArgs
    $fails = 0
    Write-Line "$($script:ToolName) $($script:ToolVersion) | PowerShell $($PSVersionTable.PSVersion) | $([Environment]::OSVersion.VersionString)" -Color Cyan
    if (-not $ReadResult.Exists) { Write-Note "No config yet at $($script:ConfigFile) (the first 'add' creates it)." }
    else {
        $check = Test-Config $Cfg
        if ($check.Errors.Count) { $fails += $check.Errors.Count; foreach ($e in $check.Errors) { Write-Bad "config: $e" } }
        else { Write-Ok "Config valid: $($script:ConfigFile)" }
        foreach ($w in $check.Warnings) { Write-Warn "config: $w" }
        if (-not $ReadResult.Strict) { Write-Warn "config: it holds comments or other text that is not strict JSON; a change made by a command refuses until they are taken out, or -Force rewrites it without them." }
        $nb = if ([IO.Directory]::Exists($script:BackupDir)) { @(Get-ChildItem -LiteralPath $script:BackupDir -Filter 'cc_profiles.*.json' -File).Count } else { 0 }
        Write-Note "$nb config backup(s) in $(Compress-UserPath $script:BackupDir)"
    }

    $cands = @(Find-ClaudeAppCandidates $Cfg)
    if (-not $cands.Count) { $fails++; Write-Bad 'Claude desktop app not found (install it, or set app.exePath).' }
    else {
        $a = $cands[0]
        Write-Ok "App: v$($a.Version) via $($a.Source): $($a.Exe)"
        if (-not $a.IsElectron) { Write-Warn "Launching through $($a.Source); the install itself can't be inspected. Launch verification still checks isolation." }
        foreach ($c in @($cands | Select-Object -Skip 1)) { Write-Line "       fallback: $($c.Source): $($c.Exe)" -Color DarkGray }
    }

    if (Test-IsElevated) { Write-Warn 'This shell is elevated; a launch from it is refused without -AllowElevated, so Claude does not run as administrator.' }
    $leaky = @([Environment]::GetEnvironmentVariables().Keys | ForEach-Object { [string]$_ } | Where-Object { Test-WildcardAny $_ ($script:DefaultScrub + @($Cfg['scrubEnv']) + $script:ManagedEnv) })
    if ($leaky.Count) { Write-Note "This shell has $($leaky.Count) Claude/Electron variable(s) ($((@($leaky | Sort-Object) | Select-Object -First 4) -join ', ')...); launches never pass them on." }

    $views = @(Get-AllProfileViews $Cfg)
    $instances = @(Get-ClaudeInstances)
    $states = Get-ConsoleAttachment @($instances | ForEach-Object { $_.Pid })
    foreach ($v in $views) {
        $tag = "profile '$($v.Name)'"
        $run = @($instances | Where-Object { Test-InstanceMatch $_ $v })
        # An instance whose console is attached, or could not be read, is a problem:
        # whether a terminal can end it is the question this tool exists to answer.
        if ($run.Count) { $fails += Write-AttachedInstances $v $run -AsFailure -Probe $states }
        if (-not $v.IsDefault) {
            if (-not [IO.Directory]::Exists($v.UserDataDir)) { Write-Note "${tag}: not launched yet ($(Compress-UserPath $v.UserDataDir) will be created)." }
            if ($v.SharesClaudeConfig) { Write-Warn "${tag}: shares ~/.claude with the default profile." }
            if ($run.Count) {
                $started = ([datetime]$run[0].Started).ToUniversalTime()
                if ((Get-DirActivityUtc $v.UserDataDir) -ge $started.AddSeconds(-5)) { Write-Ok "${tag}: running (pid $($run[0].Pid)) and writing to its own folder." }
                else { $fails++; Write-Bad "${tag}: running (pid $($run[0].Pid)) but its folder is untouched since start; the app may be ignoring --user-data-dir." }
            }
        }
        elseif ($run.Count) { Write-Ok "${tag}: running (pid $($run[0].Pid))." }
        $det = Get-DetectedEmail $v
        if ($v.Email -and $det -and $v.Email -ine $det) { Write-Warn "${tag}: configured for $($v.Email), but Claude Code there is signed in as $det." }
        elseif ($det -and -not ($v.SharesClaudeConfig -and -not $v.IsDefault)) { Write-Ok "${tag}: Claude Code signed in as $det." }
    }
    foreach ($i in $instances) {
        if (-not @($views | Where-Object { Test-InstanceMatch $i $_ }).Count) {
            Write-Warn "pid $($i.Pid) runs with an unregistered folder: $($i.UserDataDir) (see 'discover')."
            $c = $states[$i.Pid]
            if ($c -and $c.State -eq 'attached') { Write-Warn "pid $($i.Pid) is also attached to a terminal (shared with $(Format-ProcessList $c.Sharing $i.Pid)), and closing it ends that instance." }
            elseif (-not $c -or $c.State -ne 'none') { Write-Warn "pid $($i.Pid): whether it is attached to a terminal could not be read." }
        }
    }

    # This config's shortcuts only; another config's are that config's business.
    foreach ($dir in Get-ShortcutDirs 'Both') {
        if (-not [IO.Directory]::Exists($dir)) { continue }
        foreach ($lnk in Get-ChildItem -LiteralPath $dir -Filter 'Claude (*).lnk' -File) {
            if (-not (Test-ShortcutOwned $lnk.FullName)) { continue }
            $argv = @(Split-CommandLine (New-Object -ComObject WScript.Shell).CreateShortcut($lnk.FullName).Arguments)
            $f = [array]::IndexOf($argv, '-File')
            $target = if ($f -ge 0 -and $f + 1 -lt $argv.Count) { $argv[$f + 1] } else { $null }
            if (-not $target -or -not [IO.File]::Exists($target)) {
                $fails++; Write-Bad "Shortcut $($lnk.Name) points to a missing script ($target); run 'shortcut -All' from the installed copy."
            }
            $l = [array]::IndexOf($argv, 'launch')
            $pn = if ($l -ge 0 -and $l + 1 -lt $argv.Count) { $argv[$l + 1] } else { $null }
            if ($pn -and $pn -ine 'default' -and -not (Find-ProfileKey $Cfg $pn)) {
                Write-Warn "Shortcut $($lnk.Name) launches profile '$pn', which is not in the config. Delete it, or add the profile back."
            }
        }
    }

    Write-Line ''
    Write-Line 'Shared by every profile (flags cannot separate these):' -Color Cyan
    Write-Line '  - Quick-entry hotkey (Ctrl+Alt+Space): the first instance to start owns it; rebind it in the other profiles.'
    Write-Line '  - claude:// links (including browser sign-in hand-offs) open the default profile; sign in to others with the email code.'
    Write-Line '  - Claude in Chrome pairs with one instance at a time (one native-messaging host per machine).'
    $svc = Get-Service -Name 'CoworkVMService' -ErrorAction SilentlyContinue
    Write-Line "  - Cowork uses one Windows service for all profiles$(if ($svc) { " (CoworkVMService: $($svc.Status))" }); avoid running Cowork in two profiles at once."
    Write-Line "  - 'Run at startup' and the pinned taskbar icon start the default profile; use 'shortcut PROFILE' for the others."
    if ($fails) { Write-Line ''; Write-Bad "$fails problem(s) found."; $script:ExitCode = 1 } else { Write-Line ''; Write-Ok 'No problems found.' }
}

function Invoke-Validate {
    Assert-NoExtraArgs
    $rr = Read-Config
    if (-not $rr.Exists) { Write-Note "No config at $($script:ConfigFile); defaults apply."; return }
    $check = Test-Config $rr.Config
    foreach ($w in $check.Warnings) { Write-Warn $w }
    if (-not $rr.Strict) { Write-Warn 'The config holds comments or other text that is not strict JSON; a change made by a command refuses until they are taken out.' }
    if ($check.Errors.Count) { foreach ($e in $check.Errors) { Write-Bad $e }; $script:ExitCode = 1; return }
    Write-Ok "Valid: $($rr.Path) ($(@($rr.Config['profiles'].Keys).Count) profile(s))"
}

# Newest first. Callers wrap the result in @(): PowerShell unrolls an array a function
# returns, and one backup would otherwise arrive as a file with no Count.
function Get-ConfigBackups {
    if (-not [IO.Directory]::Exists($script:BackupDir)) { return @() }
    return @(Get-ChildItem -LiteralPath $script:BackupDir -Filter 'cc_profiles.*.json' -File | Sort-Object Name -Descending)
}

function Invoke-Backup {
    Assert-NoExtraArgs
    $path = Get-ConfigTargetPath
    if (-not [IO.File]::Exists($path)) { throw "No config at $path to back up." }
    $dest = Invoke-WithConfigLock { Backup-ConfigFile $path 'manual' }
    Write-Ok "Backed up to $dest"
}

function Invoke-Restore {
    [CmdletBinding(SupportsShouldProcess)]
    param()
    Assert-NoExtraArgs
    $files = @(Get-ConfigBackups)
    if (-not $script:Tgt) {
        if (-not $files.Count) { Write-Note 'No backups yet.'; return }
        $i = 0
        foreach ($f in $files) {
            $i++
            $summary = try { "$(@((ConvertFrom-JsonText (ConvertFrom-ConfigBytes (Read-SharedBytes $f.FullName)) $f.Name)['profiles'].Keys) -join ', ')" } catch { '(unreadable)' }
            Write-Line ('[{0,2}] {1}  {2}' -f $i, $f.Name, $summary)
        }
        Write-Line ''
        Write-Line "Restore one with: $($script:ToolName) restore NUMBER   (the current config is backed up first)"
        return
    }
    $file = if ($script:Tgt -match '^\d+$') {
        $n = [int]$script:Tgt
        if ($n -lt 1 -or $n -gt $files.Count) { throw "No backup number $n (there are $($files.Count))." }
        $files[$n - 1].FullName
    }
    else { Get-FullPath $script:Tgt 'backup file' }
    if (-not [IO.File]::Exists($file)) { throw "No backup at $file." }
    $text = ConvertFrom-ConfigBytes (Read-SharedBytes $file)
    if (-not $PSCmdlet.ShouldProcess($script:ConfigFile, "Restore from $file")) { return }
    # Save-ConfigText is the gate: it refuses an invalid backup, and one of a newer schema.
    Invoke-WithConfigLock {
        $path = Get-ConfigTargetPath
        $h = if ([IO.File]::Exists($path)) { Get-FileSha256 $path } else { $null }
        Save-ConfigText $text $h 'pre-restore'
    }
    Write-Ok "Restored $($script:ConfigFile) from $([IO.Path]::GetFileName($file))."
}

function Open-Editor([string]$File) {
    $cmd = if ($env:VISUAL) { $env:VISUAL } elseif ($env:EDITOR) { $env:EDITOR } elseif (Get-Command code -CommandType Application -ErrorAction SilentlyContinue) { 'code --wait' } else { 'notepad.exe' }
    $argv = @(Split-CommandLine $cmd)
    if (-not $argv.Count) { throw "Could not parse the editor command '$cmd'." }
    $exe = $argv[0]
    $editorArgs = @($argv | Select-Object -Skip 1) + @($File)
    $sw = [Diagnostics.Stopwatch]::StartNew()
    & $exe @editorArgs
    # An editor that returns at once (notepad on Windows 11 does) has not been edited in yet.
    if ($sw.Elapsed.TotalSeconds -lt 3) { [void](Read-Host "Edit $File in the editor, save it, then press Enter here") }
}

# visudo-style: edit a copy, validate it, and only then swap it in atomically.
function Invoke-Edit {
    [CmdletBinding(SupportsShouldProcess)]
    param()
    Assert-NoExtraArgs
    $path = Get-ConfigTargetPath
    $exists = [IO.File]::Exists($path)
    # One read gives both the text and the hash the save is checked against.
    $origHash = $null
    $orig = ConvertTo-ConfigJson (New-DefaultConfig)
    if ($exists) {
        $bytes = Read-SharedBytes $path
        $origHash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes))
        $orig = ConvertFrom-ConfigBytes $bytes
    }
    [void][IO.Directory]::CreateDirectory($script:DataDir)
    $tmp = [IO.Path]::Combine($script:DataDir, ('edit-{0}.json' -f [DateTime]::Now.ToString('yyyyMMdd-HHmmss', [Globalization.CultureInfo]::InvariantCulture)))
    [IO.File]::WriteAllText($tmp, $orig, [Text.UTF8Encoding]::new($false))
    while ($true) {
        Open-Editor $tmp
        $new = ConvertFrom-ConfigBytes (Read-SharedBytes $tmp)
        if ($exists -and $new -ceq $orig) { Remove-Contained $tmp $script:DataDir; Write-Ok 'No changes.'; return }
        $errs = @()
        try {
            $c = ConvertFrom-JsonText $new 'your edit'
            if ($c -isnot [Collections.IDictionary]) { $errs = @('The top level must be a JSON object.') }
            else { $errs = @((Test-Config (Add-ConfigDefaults $c)).Errors) }
        }
        catch { $errs = @($_.Exception.Message) }
        if (-not $errs.Count) { break }
        foreach ($e in $errs) { Write-Bad $e }
        $ans = Read-Host 'Re-open the editor to fix it? [Y/n]'
        if ($ans -match '^(n|no)$') { Write-Warn "Not saved. Your edit is kept at $tmp"; return }
    }
    if (-not $PSCmdlet.ShouldProcess($path, 'Save edited config')) { return }
    try { Invoke-WithConfigLock { Save-ConfigText $new $origHash 'pre-edit' } }
    catch { throw "$($_.Exception.Message) Your edit is kept at $tmp." }
    Remove-Contained $tmp $script:DataDir
    Write-Ok "Saved $path"
}

#endregion
#region ---------------------------------------------------------------- install

# Copies bytes, not text, so the installed copy hashes the same as its source.
function Copy-FileAtomic([string]$Source, [string]$Dest) {
    $dir = [IO.Path]::GetDirectoryName($Dest)
    [void][IO.Directory]::CreateDirectory($dir)
    $tmp = [IO.Path]::Combine($dir, ('.{0}.{1}.tmp' -f [IO.Path]::GetFileName($Dest), [guid]::NewGuid().ToString('N')))
    try {
        [IO.File]::Copy($Source, $tmp, $false)
        Invoke-IoRetry {
            if ([IO.File]::Exists($Dest)) { [IO.File]::Replace($tmp, $Dest, [NullString]::Value, $true) }
            else { [IO.File]::Move($tmp, $Dest) }
        }
    }
    finally {
        # The rollback half of this copy: the temporary it created above, nothing else.
        if ([IO.File]::Exists($tmp)) { Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue }
    }
    if ((Get-FileSha256 $Dest) -ne (Get-FileSha256 $Source)) { throw "The copy at $Dest does not match $Source." }
}

# PowerShell text for a single-quoted string holding $Text. EscapeSingleQuotedStringContent
# doubles every quote PowerShell reads as one, including the typographic ones; doubling
# the ASCII quote alone left a path with a typographic quote unterminated.
function ConvertTo-SingleQuoted([string]$Text) {
    return "'" + [Management.Automation.Language.CodeGeneration]::EscapeSingleQuotedStringContent($Text) + "'"
}

function Get-ProfileBlock([string]$ScriptFile, [string]$AliasName) {
    $quotedPath = ConvertTo-SingleQuoted $ScriptFile
    $fn = $AliasName
    # The command list is read from the dispatch table, so a new command completes too.
    $commandList = (@($script:CommandMap.Values | Sort-Object -Unique -CaseSensitive) | ForEach-Object { ConvertTo-SingleQuoted $_ }) -join ','
    $completer = @(
        '{'
        '    param($commandName, $parameterName, $wordToComplete)'
        '    $ids = [System.Collections.Generic.List[string]]::new()'
        ('    if ($parameterName -eq ''Command'') { foreach ($c in @(' + $commandList + ')) { $ids.Add($c) } }')
        '    try {'
        '        $f = if ($env:CC_PROFILES_CONFIG) { $env:CC_PROFILES_CONFIG } else { Join-Path $HOME ''.cc_profiles.json'' }'
        '        $j = [System.IO.File]::ReadAllText($f) | ConvertFrom-Json -AsHashtable'
        '        $ids.Add(''default'')'
        '        foreach ($n in $j[''profiles''].Keys) {'
        '            if ($n -ne ''default'') { $ids.Add($n) }'
        '            $p = $j[''profiles''][$n]'
        '            foreach ($a in @($p[''aliases''])) { if ($a) { $ids.Add($a) } }'
        '            if ($p[''email'']) { $ids.Add($p[''email'']) }'
        '        }'
        '    }'
        '    catch { $null = $_ }'
        '    $prefix = [System.Management.Automation.WildcardPattern]::Escape($wordToComplete) + ''*'''
        '    $ids | Select-Object -Unique | Where-Object { $_ -like $prefix } | ForEach-Object {'
        '        [System.Management.Automation.CompletionResult]::new($_, $_, ''ParameterValue'', $_)'
        '    }'
        '}'
    ) -join [Environment]::NewLine
    return @(
        "# >>> $($script:ToolName) >>>  (managed by `"$($script:ToolName) install -AddToProfile`"; edits inside are replaced)"
        "Set-Alias -Name $fn -Value $quotedPath -Scope Global"
        "Register-ArgumentCompleter -CommandName $fn, $($script:ToolName), $($script:ToolName).ps1 -ParameterName Command -ScriptBlock $completer"
        "Register-ArgumentCompleter -CommandName $fn, $($script:ToolName), $($script:ToolName).ps1 -ParameterName Target -ScriptBlock $completer"
        "# <<< $($script:ToolName) <<<"
    ) -join [Environment]::NewLine
}

function Get-ParseErrorCount([string]$Text) {
    $tokens = $null; $errors = $null
    [void][Management.Automation.Language.Parser]::ParseInput($Text, [ref]$tokens, [ref]$errors)
    return @($errors).Count
}

# The profile runs in every new shell, so a block that does not parse stops the user's
# whole profile loading. The profile is not written when the new text has more parse
# errors than the old; a profile that already had some keeps working as it did.
function Update-PowerShellProfile {
    [CmdletBinding(SupportsShouldProcess)]
    param([string]$ScriptFile, [string]$ProfilePath = $PROFILE.CurrentUserAllHosts, [string]$AliasName = $FunctionName)
    if ($AliasName -notmatch '^[A-Za-z_][A-Za-z0-9_-]{0,63}\z') { throw "'$AliasName' cannot be an alias name: use letters, digits, '_' and '-'." }
    $pf = $ProfilePath
    $block = Get-ProfileBlock $ScriptFile $AliasName
    $text = if ([IO.File]::Exists($pf)) { ConvertFrom-ConfigBytes (Read-SharedBytes $pf) } else { '' }
    $marker = [regex]::Escape($script:ToolName)
    $m = [regex]::Match($text, "(?ms)^# >>> $marker >>>.*?^# <<< $marker <<<[^\r\n]*")
    $new = if ($m.Success) { $text.Substring(0, $m.Index) + $block + $text.Substring($m.Index + $m.Length) }
    elseif ($text.Trim()) { $text.TrimEnd() + [Environment]::NewLine + [Environment]::NewLine + $block + [Environment]::NewLine }
    else { $block + [Environment]::NewLine }
    if ($new -ceq $text) { Write-Ok "PowerShell profile already up to date ($pf)."; return }
    if ((Get-ParseErrorCount $new) -gt (Get-ParseErrorCount $text)) { throw "$pf would not parse after the change; it was not changed." }
    if (-not $PSCmdlet.ShouldProcess($pf, "Add '$AliasName' alias and tab completion")) { return }
    if ($text) {
        [void][IO.Directory]::CreateDirectory($script:BackupDir)
        $saved = [IO.Path]::Combine($script:BackupDir, ('powershell-profile.{0}.ps1' -f [DateTime]::Now.ToString('yyyyMMdd-HHmmss', [Globalization.CultureInfo]::InvariantCulture)))
        [IO.File]::WriteAllBytes($saved, (Read-SharedBytes $pf))
    }
    Write-FileAtomic $pf $new
    Write-Ok "Updated $pf. Open a new terminal, then try: $AliasName list"
}

function Test-DirOnPath([string]$Dir) {
    $all = @([Environment]::GetEnvironmentVariable('Path', 'User'), [Environment]::GetEnvironmentVariable('Path', 'Machine')) -join ';'
    foreach ($d in $all.Split(';')) {
        if (-not $d.Trim()) { continue }
        try { $full = [IO.Path]::GetFullPath([Environment]::ExpandEnvironmentVariables($d.Trim())) }
        catch {
            # A malformed PATH entry names no directory; it cannot be the one asked about.
            $null = $_
            continue
        }
        if (Test-PathEqual $full $Dir) { return $true }
    }
    return $false
}

# Installs into ~/bin by default: the .ps1, which PowerShell runs by name from PATH,
# and a .cmd beside it for cmd.exe and the Run dialog. PATH itself is never edited.
function Invoke-Install {
    [CmdletBinding(SupportsShouldProcess)]
    param()
    Assert-NoExtraArgs
    $destDir = if ($Destination) { Get-FullPath $Destination '-Destination' } else { [IO.Path]::Combine($HOME, 'bin') }
    $dest = [IO.Path]::Combine($destDir, "$($script:ToolName).ps1")
    $shim = [IO.Path]::Combine($destDir, "$($script:ToolName).cmd")
    $src = $script:ScriptPath
    if (-not $src) { Stop-CannotRun 'Cannot determine where this script is running from.' }
    if (Test-PathEqual $src $dest) { Write-Ok "Running the installed copy ($dest)." }
    elseif ([IO.File]::Exists($dest) -and (Get-FileSha256 $dest) -eq (Get-FileSha256 $src)) { Write-Ok "Already installed and identical: $dest" }
    elseif ($PSCmdlet.ShouldProcess($dest, "Install from $src")) {
        if ([IO.File]::Exists($dest)) { Copy-FileAtomic $dest "$dest.previous" }
        Copy-FileAtomic $src $dest
        Write-Ok "Installed $dest"
    }
    $shimText = "@echo off`r`npwsh -NoLogo -NoProfile -ExecutionPolicy Bypass -File `"%~dp0$($script:ToolName).ps1`" %*`r`nexit /b %ERRORLEVEL%`r`n"
    $shimCurrent = if ([IO.File]::Exists($shim)) { [IO.File]::ReadAllText($shim) } else { $null }
    if ($shimCurrent -ceq $shimText) { Write-Ok "Already installed and identical: $shim" }
    elseif ($PSCmdlet.ShouldProcess($shim, 'Write the cmd.exe entry point')) { Write-FileAtomic $shim $shimText; Write-Ok "Installed $shim" }

    if (Test-DirOnPath $destDir) { Write-Ok "$destDir is on PATH: '$($script:ToolName)' runs from any new PowerShell or cmd window." }
    else { Write-Warn "$destDir is not on your PATH, so '$($script:ToolName)' will not resolve by name. Add it in Settings, or use -AddToProfile." }

    $script:ScriptPath = $dest
    if ($AddToProfile) { Update-PowerShellProfile $dest }
    else { Write-Note "'$($script:ToolName) install -AddToProfile' also adds a '$FunctionName' alias with tab completion to your PowerShell profile." }
    # This config's existing shortcuts are pointed at the installed copy. Shortcuts of
    # another config, and ones this tool did not make, are left alone.
    $rr = Read-Config
    $owned = @(foreach ($v in @(Get-AllProfileViews $rr.Config)) {
            $dirs = @(Get-ShortcutDirs 'Both' | Where-Object { Test-ShortcutOwned (Get-ShortcutPath $_ $v.Name) })
            if ($dirs.Count) { [pscustomobject]@{ View = $v; Dirs = $dirs } }
        })
    if ($owned.Count) {
        $app = Find-ClaudeApp $rr.Config
        foreach ($o in $owned) { Set-ProfileShortcut $o.View $app $o.Dirs }
    }
}

#endregion
#region ---------------------------------------------------------------- help & entry point

function Show-Help {
    $n = $script:ToolName
    $lines = @(
        "$n - isolated profiles for the Claude desktop app (Windows, PowerShell 7.5+)"
        ''
        'USAGE'
        "  $n [COMMAND] [PROFILE] [options]"
        "  $n PROFILE [options]                      launch by name, alias or email"
        ''
        'PROFILES'
        '  list                                     all profiles, running state, folders  (the default command)'
        '  show PROFILE [-Json]                     details, and which account Claude Code is signed in as'
        '  add [NAME] [-Email E] [-Alias A,B]       create a profile (NAME comes from -Email when omitted)'
        '      [-UserDataDir D] [-ClaudeConfigDir D | -ShareClaudeConfig]'
        '      [-SetEnv NAME=VALUE] [-ProfileArgs ..] [-Notes ..] [-SeedFrom default|PROFILE|DIR]'
        "  set PROFILE [-Email E|''] [-Alias A] [-RemoveAlias A] [-SetEnv NAME=VALUE] [-UnsetEnv NAME]"
        '      [-Notes ..] [-ProfileArgs ..] [-UserDataDir D -Force] [-ClaudeConfigDir D | -ShareClaudeConfig | -IsolateClaudeConfig]'
        '  alias PROFILE ALIAS...                   add aliases          unalias ALIAS...   remove aliases'
        '  rename PROFILE NEW-NAME                  rename (folders stay where they are)'
        '  remove PROFILE [-PurgeData [-Force]]     unregister; -PurgeData moves its folders into .cc-trash beside them'
        '  discover [-Register]                     find profile folders and running instances not in the config'
        '  seed PROFILE [-SeedFrom default] [-Include CLAUDE.md,skills] [-Force]'
        '                                           copy Claude Code settings (never credentials or history)'
        ''
        'RUNNING'
        '  launch PROFILE [-Restart [-Force]] [-DryRun] [-NoVerify] [-AppArgs ..] [-AllowElevated]'
        '                                           -DryRun prints the exact command line and environment;'
        '                                           -Force ends an instance that went to the tray, and'
        '                                           -AllowElevated lets an elevated shell launch at all'
        '  status [-Json]                           running desktop instances and the profile each belongs to'
        '  shortcut PROFILE|-All [-Location Desktop|StartMenu|Both] [-Remove]'
        '  path PROFILE [-Json]                     the two folders of a profile'
        ''
        'MAINTENANCE'
        '  doctor                                   health check, including the isolation of running instances'
        '  validate                                 check the config file'
        '  edit                                     edit the config; it is validated before it is saved'
        '  backup | restore [N|FILE]                config backups (one is also taken before every change)'
        '  install [-Destination DIR] [-AddToProfile [-FunctionName ccp]]'
        '                                           copy to ~/bin with a .cmd beside it; PATH is not edited'
        '  help | version'
        ''
        'COMMON OPTIONS'
        '  -WhatIf, -Confirm    preview or confirm changes     -ConfigPath FILE   use another config file'
        '  -AppExe PATH         skip app discovery             -Json              machine-readable output'
        ''
        'FILES'
        '  ~/.cc_profiles.json  your profiles: hand-editable, written atomically, validated, locked'
        '  ~/.cc_profiles.d/    state, backups, icons, launch.log   (CC_PROFILES_CONFIG moves both)'
        ''
        'EXIT CODES'
        '  0 it did it, 1 it refused or found a problem, 2 it could not run on this host'
    )
    Write-Display ($lines -join [Environment]::NewLine)
}

function Invoke-Path($Cfg) {
    Assert-NoExtraArgs
    $v = Get-ProfileView $Cfg (Resolve-ProfileName $Cfg $script:Tgt)
    $o = [pscustomobject]@{ Name = $v.Name; UserDataDir = $v.UserDataDir; ClaudeConfigDir = $v.EffectiveClaudeDir }
    if ($Json) { $o | ConvertTo-Json } else { $o }
}

function Invoke-Main {
    if (-not $IsWindows) { Stop-CannotRun "$($script:ToolName) runs on Windows only; this host is $([Runtime.InteropServices.RuntimeInformation]::OSDescription)." }
    Initialize-Locations
    # The parameters are typed [string[]]; an empty pipeline result cannot be cast to
    # one when the script runs in-process, so the result is wrapped and cast first.
    # Under -Json a cmdlet's own verbose line would reach stdout ahead of the value;
    # Write-Trace still sends this script's detail to stderr.
    if ($Json) { $VerbosePreference = 'SilentlyContinue' }
    $script:Alias = [string[]]@(Split-ListValue $Alias)
    $script:RemoveAlias = [string[]]@(Split-ListValue $RemoveAlias)
    $script:UnsetEnv = [string[]]@(Split-ListValue $UnsetEnv)
    $script:Include = [string[]]@(Split-ListValue $Include)
    $script:EnvSettings = ConvertTo-EnvSettings $SetEnv
    $script:RestArgs = @($Rest | Where-Object { $null -ne $_ -and $_ -ne '' })
    $cmd = if ($Command) { $Command.Trim() } else { 'list' }
    $verb = $script:CommandMap[$cmd.ToLowerInvariant()]
    if (-not $verb) {
        $verb = 'launch'
        if ($Target) { $script:RestArgs = @($Target) + $script:RestArgs }
        $script:Tgt = $cmd
    }
    switch ($verb) {
        'help' { Show-Help; return }
        'version' { Write-Line "$($script:ToolName) $($script:ToolVersion) (config schema $($script:SchemaVersion))"; return }
        'install' { Invoke-Install; return }
        'edit' { Invoke-Edit; return }
        'restore' { Invoke-Restore; return }
        'backup' { Invoke-Backup; return }
        'validate' { Invoke-Validate; return }
        'add' { Invoke-Add; return }
        'set' { Invoke-Set; return }
        'alias' { Invoke-Alias; return }
        'unalias' { Invoke-Unalias; return }
        'rename' { Invoke-Rename; return }
    }
    $rr = Read-Config
    $cfg = $rr.Config
    switch ($verb) {
        'list' { Invoke-List $cfg $rr }
        'show' { Invoke-Show $cfg }
        'remove' { Invoke-Remove $cfg -Force:$Force }
        'status' { Invoke-Status $cfg }
        'doctor' { Invoke-Doctor $cfg $rr }
        'seed' { Assert-NoExtraArgs; Invoke-Seed $cfg $script:Tgt $SeedFrom }
        'shortcut' { Invoke-Shortcut $cfg }
        'discover' { Invoke-Discover $cfg }
        'path' { Invoke-Path $cfg }
        'launch' {
            Assert-NoExtraArgs
            $name = Resolve-ProfileName $cfg $script:Tgt
            Assert-ConfigUsable $cfg $name
            Invoke-Launch $cfg $name
        }
    }
}

# Dot-sourcing loads the functions without running a command; the tests use that.
if ($MyInvocation.InvocationName -ne '.') {
    try { Invoke-Main }
    catch {
        $msg = $_.Exception.Message
        $line = "$($script:ToolName): $msg"
        if (-not [Console]::IsErrorRedirected -and -not $env:NO_COLOR) { $line = "$([char]27)[31m$line$([char]27)[0m" }
        [Console]::Error.WriteLine($line)
        if ($VerbosePreference -eq 'Continue') { [Console]::Error.WriteLine($_.ScriptStackTrace) }
        if ($script:DataDir) { Write-LaunchLog "error ($Command $($script:Tgt)): $msg" }
        if ($FromShortcut) {
            try { [void](New-Object -ComObject WScript.Shell).Popup($msg, 0, 'Claude profile launcher', 16) }
            catch {
                # No desktop to show it on; the launch log above already holds the message.
                $null = $_
            }
        }
        if ($script:ExitCode -ne 2) { $script:ExitCode = 1 }
    }
    exit $script:ExitCode
}

#endregion
