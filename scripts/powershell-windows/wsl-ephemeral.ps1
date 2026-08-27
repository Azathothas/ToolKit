<#
.SYNOPSIS
    Create, use, and destroy ephemeral WSL2 distros on demand.

.DESCRIPTION
    Builds throwaway WSL2 distros from OCI images (any distro, any tag available on a
    container registry) or from a local rootfs tarball, runs commands inside them, and
    removes them cleanly.

    SAFETY MODEL -- this script is destructive by nature, so removal is constrained four ways:
      1. Every distro it creates is named with a fixed prefix (default 'eph-').
      2. It REFUSES to remove any distro whose name lacks that prefix.
      3. It REFUSES to remove any name on an explicit protected list, prefix or not.
         'podman-machine-default' and the Docker Desktop distros are protected, so a
         mistake here cannot destroy your container runtime.
      4. Directory deletion is confined to %LOCALAPPDATA%\wsl-ephemeral\<distro>; the base
         directory itself and anything outside it can never be the target.

    Destructive actions require -Force when running non-interactively.

.PARAMETER Action
    New     Create an ephemeral distro (from -Image or -Tarball).
    Run     Run a command inside an existing ephemeral distro.
    List    List ephemeral distros, and show what else exists (never touched).
    Remove  Unregister one ephemeral distro and delete its disk.
    Purge   Remove ALL ephemeral distros (prefix-matched only).

.PARAMETER Image
    OCI image reference, e.g. 'alpine:3.22', 'debian:bullseye-slim', 'fedora:44',
    'archlinux:latest', 'opensuse/tumbleweed', 'rockylinux:9', 'voidlinux/voidlinux'.

.PARAMETER Tarball
    Path to a rootfs tarball (.tar / .tar.gz) to import instead of pulling an image.
    Lets the script work with no container engine installed.

.PARAMETER Name
    Distro name. Auto-generated when omitted. The prefix is added if missing.

.PARAMETER Command
    Shell command to run, via /bin/sh -lc. It travels as base64 and is sourced
    inside the distro, so a quote, a dollar sign, a backtick or a tab arrives
    byte-exact instead of being re-parsed in transit.

.PARAMETER CommandFile
    Path to a file ON THIS MACHINE whose bytes are the command. Read verbatim,
    which is what makes a multi-line script possible at all.

.PARAMETER CommandB64
    The command as base64 of its UTF-8 bytes, for a caller that already holds
    the text and wants no shell anywhere near it. Command, CommandFile and
    CommandB64 are mutually exclusive.

.PARAMETER User
    User to run as inside the distro. Default 'root'.

.PARAMETER Ephemeral
    With -Action New: run -Command then immediately destroy the distro.

.PARAMETER OciEnv
    With -Action New -Image: carry the image's OCI environment into the distro,
    as /etc/profile.d/10-oci-env.sh. Off by default, because turning it on
    changes PATH for every caller of a shape they already depend on.

.PARAMETER Force
    Required for destructive actions when non-interactive. Skips confirmation.

.EXAMPLE
    .\wsl-ephemeral.ps1 -Action New -Image alpine:3.22

.EXAMPLE
    .\wsl-ephemeral.ps1 -Action New -Image debian:bullseye-slim -Command "ldd --version" -Ephemeral -Force

.EXAMPLE
    .\wsl-ephemeral.ps1 -Action Run -Name eph-alpine-3.22-a1b2 -Command "apk add gcc && gcc --version"

.EXAMPLE
    .\wsl-ephemeral.ps1 -Action Purge -Force

.NOTES
    EXIT CODES -- New and Run behave the SAME way. They are written here
    together because they drifted apart once and nothing noticed: New warned
    over a failing command and still exited 0, so every green result downstream
    of it meant nothing.

      -Action Run -Command ...             exits with the inner command's code
      -Action New -Command ...             exits with the inner command's code
      -Action New -Command ... -Ephemeral  tears the distro down FIRST, then
                                           exits with the inner command's code
      New with no -Command                 exits 0 when the distro came up
      any action, script failure           exits 1, with a message naming what

    Both actions run the caller's command through ONE function, Invoke-InDistro,
    so there is no second place for the code to be dropped.

    A caller that relied on New never failing now sees the real code. That is a
    deliberate break: see docs/consumers.md.

    COMMAND CHANNEL -- -Command, -CommandFile and -CommandB64 are three ways of
    handing over the same thing: bytes. They are base64'd here, decoded to a
    file inside the distro, and sourced by the login shell. NOTHING is quoted
    for the guest, because quoting does not survive the trip and no caller can
    make it. Measured on this host on 2026-08-27 under BOTH PowerShell hosts:
    a payload handed to `wsl.exe -- /bin/sh -lc` has its $VAR expanded and the
    RESULT re-parsed, even inside POSIX single quotes, which is why
    `echo $PATH` died on the bracket in "Program Files (x86)" and why a
    backtick opened a command substitution. Base64 is [A-Za-z0-9+/=] and
    arrives intact on both hosts and in both busybox ash and dash.

    ONE function builds that skeleton, ConvertTo-DistroScriptCommand, and it
    ASSERTS the result stays inside the measured alphabet. Every payload this
    script sends goes through it: the caller's command, the smoke probe, and
    the script Write-DistroFile sends. Before this, two of those three were
    hand-written inside that alphabet with nothing enforcing it, which is the
    defect that made WSL-12 possible.

    The transport file is written, opened, UNLINKED, and only then sourced from
    its open descriptor. So a run leaves nothing behind inside the distro, and
    a command carrying a credential does not persist as a file.

    ORPHANS -- List reports any rootfs .tar left loose in the base directory
    and Purge removes them, through the same deletion and the same containment
    guard as a distro disk. An interrupted New is what leaves one: the tarball
    is cleaned in a finally, and a hard interrupt does not always run one.

    OCI CONFIG -- -OciEnv carries the image's ENV and WORKDIR into the distro
    as /etc/profile.d/10-oci-env.sh, which a login shell sources. It is OFF by
    default: turning it on changes PATH, and every existing caller depends on
    the shape they have. USER and ENTRYPOINT are deliberately NOT carried; the
    reasons are in New-OciEnvScript.

    PLATFORM -- every pull and every create names linux/ARCH explicitly, where
    ARCH is this host's own, read from the engine. Naming it is not politeness:
    the local image store is keyed by tag and NOT by architecture, so a single
    earlier 'pull --platform linux/riscv64 alpine' repoints the shared
    alpine:latest, and the next unqualified pull is a no-op that exports a
    rootfs nothing in it can execute.

    SPACE -- New checks the target volume before --import and REFUSES rather
    than warning, because running out midway leaves a partial VHDX and a
    registered distro that does not work. The requirement is a floor plus a
    multiple of the rootfs tarball, and the entry that asked for this was
    wrong about the shape of it: measured here, an 8 MiB rootfs costs 76 MiB
    and a 77 MiB one costs 172 MiB, so the FLOOR dominates and "roughly twice
    the rootfs" is not the rule. The numbers are in Assert-EnoughDiskSpace.
    A volume whose free space cannot be read is SAID SO and imported anyway;
    a preflight that skipped is not a preflight that passed.

    Requires : Windows 10 2004+ / Windows 11 with WSL2.
    Optional : podman or docker (only for -Image).
    Tested on: Windows PowerShell 5.1 and PowerShell 7+.
#>
# ── PSScriptAnalyzer, suppressed per rule with the reason ────────────────────
# CI runs Invoke-ScriptAnalyzer over scripts/ at Error and Warning, so a
# suppression here is the difference between a red gate and a green one. Each
# is scoped to ONE rule and carries its justification. ⛔ Do not replace these
# with a settings file that switches the rule off everywhere: that weakens the
# gate for every future script to spare this one, which is how a check stops
# checking.
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '',
    Justification = 'This is an interactive console tool. Its entire output is progress and a summary for a human at a terminal, which is the documented case for Write-Host. Nothing here is a value another script consumes: Run exits with the inner command''s code and callers read that.')]
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSReviewUnusedParameter', '',
    Justification = 'Image, Tarball, Command, CommandFile, CommandB64, User, Ephemeral and Force are read by the Invoke-Action* functions through script scope rather than as arguments. The analyzer does not follow that, and threading eight parameters through every call to satisfy it would make the code worse.')]
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseSingularNouns', '',
    Justification = 'Get-WslDistroNames returns the whole list and Export-ImageRootfs writes one rootfs whose name simply ends in s. Renaming either to satisfy the rule would make the name describe the thing less accurately.')]
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '',
    Justification = 'Removal already goes through Confirm-Destructive, which refuses non-interactively unless -Force was passed. Adding ShouldProcess would give a second, differently spelled confirmation path over the same guard, and two confirmation mechanisms is how one of them gets bypassed.')]
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('New', 'Run', 'List', 'Remove', 'Purge')]
    [string]$Action,

    [string]$Image,
    [string]$Tarball,
    [string]$Name,
    [string]$Command,
    [string]$CommandFile,
    [string]$CommandB64,
    [string]$User = 'root',
    [switch]$Ephemeral,
    [switch]$OciEnv,
    [switch]$Force
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# --------------------------------------------------------------------------------------
# Constants
# --------------------------------------------------------------------------------------
$script:Prefix  = 'eph-'
$script:BaseDir = Join-Path $env:LOCALAPPDATA 'wsl-ephemeral'

# What --import costs on the target volume, as a floor plus a multiple of the
# rootfs tarball. ⛔ Both are set ABOVE every measurement in
# Assert-EnoughDiskSpace rather than fitted to them: a tight preflight refuses
# an import that would have worked, which is a worse failure than the one it
# prevents. The floor is what matters, because an 8 MiB rootfs still costs 76.
$script:ImportSpaceFactor = 2
$script:ImportSpaceFloor  = 256MB

# Names that must NEVER be unregistered, even if somebody prefixes them.
$script:Protected = @(
    'podman-machine-default',
    'docker-desktop',
    'docker-desktop-data',
    'rancher-desktop',
    'rancher-desktop-data'
)

# WSL emits UTF-16LE unless this is set; without it every parsed string is NUL-riddled.
$env:WSL_UTF8 = '1'

# --------------------------------------------------------------------------------------
# Output helpers
# --------------------------------------------------------------------------------------
function Write-Step { param([string]$Message) Write-Host "==> $Message" -ForegroundColor Cyan }
function Write-Ok   { param([string]$Message) Write-Host "  * $Message" -ForegroundColor Green }
function Write-Warn { param([string]$Message) Write-Host "  ! $Message" -ForegroundColor Yellow }

# --------------------------------------------------------------------------------------
# Process helpers
# --------------------------------------------------------------------------------------
function Invoke-Native {
    <#
      Run a native exe, capture merged stdout+stderr, throw on non-zero exit.
      $ErrorActionPreference is deliberately relaxed for the duration: with it set to
      'Stop', PowerShell 7.3+ turns native stderr captured via 2>&1 into a terminating
      NativeCommandError, which would misreport success as failure.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [string[]]$Arguments = @(),
        [switch]$IgnoreExitCode
    )
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $out  = & $FilePath @Arguments 2>&1
        $code = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $prev
    }
    if (-not $IgnoreExitCode -and $code -ne 0) {
        $joined = ($out | Out-String).Trim()
        throw "$([IO.Path]::GetFileName($FilePath)) $($Arguments -join ' ') failed (exit $code): $joined"
    }
    return $out
}

function Test-Interactive {
    # Read-Host blocks or throws when stdin is not a console; detect that up front.
    if (-not [Environment]::UserInteractive) { return $false }
    try { return -not [Console]::IsInputRedirected } catch { return $false }
}

function Confirm-Destructive {
    param(
        [Parameter(Mandatory = $true)][string]$Target,
        [Parameter(Mandatory = $true)][string]$Operation
    )
    if ($Force) { return $true }
    if (-not (Test-Interactive)) {
        Write-Warn "$Operation on '$Target' needs confirmation, but this session is non-interactive."
        Write-Warn "Re-run with -Force to proceed."
        return $false
    }
    $answer = Read-Host "$Operation on '$Target'? [y/N]"
    return ($answer -match '^(y|yes)$')
}

# --------------------------------------------------------------------------------------
# WSL helpers
# --------------------------------------------------------------------------------------
function Get-WslExe {
    $cmd = Get-Command wsl.exe -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $fallback = Join-Path $env:WINDIR 'System32\wsl.exe'
    if (Test-Path -LiteralPath $fallback) { return $fallback }
    throw "wsl.exe not found. WSL2 is required."
}

function Get-WslDistroNames {
    $wsl  = Get-WslExe
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try { $raw = & $wsl --list --quiet 2>$null } finally { $ErrorActionPreference = $prev }
    if (-not $raw) { return @() }
    $names = @()
    foreach ($line in $raw) {
        # Belt and braces: strip NULs in case WSL_UTF8 is unsupported on this build.
        $clean = ($line -replace "`0", '').Trim()
        if ($clean) { $names += $clean }
    }
    return $names
}

function New-GuestScratchPath {
    <#
      A path for the transport file inside the guest. It is interpolated into
      the skeleton RAW, so it is drawn from the cleared alphabet and nothing
      else, and ConvertTo-DistroScriptCommand re-checks it rather than trusting
      this function to have been the one that produced it.

      It is random per call because two concurrent Run commands against one
      distro must not write each other's file. The window is microseconds wide
      and it costs four characters to close it.
    #>
    $suffix = -join ((48..57) + (97..122) | Get-Random -Count 8 | ForEach-Object { [char]$_ })
    return "/tmp/.wsl-eph-$suffix"
}

function ConvertTo-DistroScriptCommand {
    <#
      THE ONE PLACE THE TRANSPORT SKELETON IS BUILT, and the reason there is
      only one is that a payload hand-written inside the safe alphabet is a
      constraint nothing enforces. That is exactly how WSL-12 shipped: the
      smoke probe carried a bracket inside a double-quoted echo, and every
      -Action New failed on Windows PowerShell 5.1.

      WHAT DOES NOT SURVIVE, measured on this host on 2026-08-27 against real
      Alpine and Debian distros, under BOTH PowerShell 7.6.5 and Windows
      PowerShell 5.1, with every hazard ALREADY correctly single-quoted for sh
      before it was passed:

        $VAR       expanded in transit, and the RESULT is then re-parsed. That
                   is why `echo $PATH` dies: the value carries the bracket in
                   "Program Files (x86)".
        backtick   opens a command substitution. Both hosts.
        "          survives on 7.6.5 and does NOT on 5.1, where it gives
                   "syntax error: unterminated quoted string".

      The single quotes never reach the guest, so a caller cannot fix this by
      quoting harder. WHAT DOES SURVIVE is base64: [A-Za-z0-9+/=], plus the
      operators this skeleton needs. So the payload goes as base64 and every
      character outside it is REFUSED here rather than mangled in transit.

      THE SKELETON, and each link earns its place:

        mkdir -p /tmp        a rootfs exported from a scratch image may have no
                             /tmp at all, and the failure is otherwise a
                             redirect error naming nothing.
        base64 -d>PATH       the decode. && means a guest with no base64 STOPS
                             here instead of sourcing an empty file and
                             reporting success over a command that never ran.
        exec 8>PATH          create it for writing...
        exec 9<PATH          ...open a reader on it...
        rm -f PATH           ...and UNLINK IT BEFORE ANY CONTENT EXISTS. Both
                             descriptors keep the inode alive, so the command's
                             text is never a file anybody can read, and no
                             later failure can leave one behind.
        base64 -d>&8         the decode, written through the descriptor. && is
                             what makes a guest with no base64 STOP here
                             instead of sourcing an empty file and reporting
                             success over a command that never ran.
        . /dev/fd/9          source it from the open descriptor. The login
                             shell runs it, so /etc/profile and -OciEnv still
                             apply, and its exit code becomes the shell's.

      ⚠ THE ORDER OF THOSE FIVE IS THE POINT, and it was got wrong first.
      Writing the file and then unlinking it reads the same and is not: a
      redirect CREATES the file before the decode runs, so a guest with no
      base64 was left holding an empty /tmp file that nothing removed. The
      mutation that planted a missing decoder is what found it.
    #>
    param(
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][byte[]]$ScriptBytes,
        [Parameter(Mandatory = $true)][string]$GuestPath
    )
    if ($GuestPath -notmatch '^/[A-Za-z0-9_.\-]+(/[A-Za-z0-9_.\-]+)*$') {
        throw "Transport path '$GuestPath' is outside the alphabet that survives the trip."
    }
    $b64  = [Convert]::ToBase64String($ScriptBytes)
    $line = "mkdir -p /tmp&&exec 8>$GuestPath&&exec 9<$GuestPath&&rm -f $GuestPath&&echo $b64|base64 -d>&8&&. /dev/fd/9"

    # ⛔ THE CHECK THAT DID NOT EXIST. Every payload now reaches the guest as
    # base64, so nothing a caller writes can break the alphabet; this catches
    # the OTHER direction, an edit to the skeleton above that adds a character
    # the measurement never cleared. Plant a $ in that string and this fires.
    $bad = [regex]::Match($line, '[^A-Za-z0-9+/=|<>&;. _-]')
    if ($bad.Success) {
        throw ("Transport skeleton carries '$($bad.Value)', which is outside the alphabet measured " +
               "to survive PowerShell to wsl.exe to /bin/sh. Re-measure before widening it.")
    }
    return $line
}

function Invoke-InDistro {
    <#
      The ONE path that runs a caller's command inside a distro. New and Run both
      go through it, so an inner exit code cannot be propagated by one action and
      dropped by the other. It was dropped by New, which is what made -Command
      useless as a gate.

      It takes BYTES rather than a string because -CommandFile is read verbatim
      from disk: a file that is not UTF-8 would otherwise be re-encoded on its
      way through a parameter typed as text, which is a mangling of exactly the
      kind this whole entry exists to remove.

      The code comes back through -ExitCode rather than as the return value, on
      purpose. The command's own stdout flows out of this function's success
      stream so the caller can see it, and `$rc = Invoke-InDistro ...` would
      therefore capture that OUTPUT into $rc instead of the code.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$DistroName,
        [Parameter(Mandatory = $true)][string]$RunAs,
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][byte[]]$ScriptBytes,
        [Parameter(Mandatory = $true)][ref]$ExitCode
    )
    # Set before the try, and set non-zero. Under Set-StrictMode -Version Latest
    # an unassigned variable throws when it is READ, so every path out of here
    # has to leave a code behind; and "it never answered" is a failure, not a
    # pass, so the value it starts at has to be one that fails.
    $ExitCode.Value = 1
    $line = ConvertTo-DistroScriptCommand -ScriptBytes $ScriptBytes -GuestPath (New-GuestScratchPath)
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        & (Get-WslExe) -d $DistroName -u $RunAs -- /bin/sh -lc $line
        if ($null -ne $LASTEXITCODE) { $ExitCode.Value = [int]$LASTEXITCODE }
    }
    finally { $ErrorActionPreference = $prev }
}

function ConvertTo-Utf8Bytes {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Text)
    return ,[Text.Encoding]::UTF8.GetBytes($Text)
}

function Resolve-CommandBytes {
    <#
      The three ways of naming a command collapse to one thing here, so every
      action downstream sees bytes and no action has to know which switch the
      caller used.

      $null means NO command was given, which is different from an empty one:
      New with no -Command is a distro that gets created and kept.
    #>
    param(
        [AllowEmptyString()][string]$Text,
        [AllowEmptyString()][string]$FromFile,
        [AllowEmptyString()][string]$FromB64
    )
    $given = @()
    if ($Text)     { $given += '-Command' }
    if ($FromFile) { $given += '-CommandFile' }
    if ($FromB64)  { $given += '-CommandB64' }
    if ($given.Count -gt 1) {
        throw "Pass only one of $($given -join ', '). They are three spellings of the same argument."
    }
    if ($given.Count -eq 0) { return $null }

    if ($Text) { return (ConvertTo-Utf8Bytes -Text $Text) }

    if ($FromFile) {
        if (-not (Test-Path -LiteralPath $FromFile -PathType Leaf)) {
            throw "-CommandFile not found: $FromFile"
        }
        $bytes = [IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $FromFile).Path)
        # ⚠ NOT rewritten. The file's bytes are the command, and silently
        # editing somebody's payload is the failure this entry is about. A
        # carriage return is legal in a POSIX script and means something, so
        # this says what is about to happen rather than deciding for them.
        if ($bytes -contains 13) {
            Write-Warn "$FromFile has CRLF line endings. /bin/sh reads the CR as part of the last word on each line, so expect 'not found' errors. Convert it to LF if that is not what you meant."
        }
        return ,$bytes
    }

    try { $decoded = [Convert]::FromBase64String($FromB64) }
    catch { throw "-CommandB64 is not valid base64: $($_.Exception.Message)" }
    return ,$decoded
}

# --------------------------------------------------------------------------------------
# Naming and safety guards
# --------------------------------------------------------------------------------------
function Test-ProtectedName {
    param([Parameter(Mandatory = $true)][string]$DistroName)
    foreach ($p in $script:Protected) { if ($DistroName -ieq $p) { return $true } }
    return $false
}

function Assert-Removable {
    <# The single choke point for every destructive path. #>
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$DistroName)

    if ([string]::IsNullOrWhiteSpace($DistroName)) {
        throw "Refusing to remove: empty distro name."
    }
    if (Test-ProtectedName -DistroName $DistroName) {
        throw "REFUSING to remove protected distro '$DistroName'. This is a hard guard."
    }
    if (-not $DistroName.StartsWith($script:Prefix, [StringComparison]::Ordinal)) {
        throw ("REFUSING to remove '$DistroName': it does not start with '$($script:Prefix)'. " +
               "This script only removes distros it created.")
    }
}

function Assert-InsideBaseDir {
    <#
      Guarantees a directory slated for recursive deletion is a *strict* child of BaseDir.
      Without this, an empty or crafted distro name could resolve the target to BaseDir
      itself (or, with traversal, somewhere else entirely).
    #>
    param([Parameter(Mandatory = $true)][string]$Path)

    $baseFull = [IO.Path]::GetFullPath($script:BaseDir.TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar)
    $full     = [IO.Path]::GetFullPath($Path)

    if ($full.TrimEnd('\', '/') -ieq $baseFull.TrimEnd('\', '/')) {
        throw "REFUSING to delete the base directory itself ($full)."
    }
    if (-not $full.StartsWith($baseFull, [StringComparison]::OrdinalIgnoreCase)) {
        throw "REFUSING to delete '$full': outside $($script:BaseDir)."
    }
}

function Remove-PathWithRetry {
    <#
      THE one deletion in this script. Every path that removes something on disk
      goes through here, so the containment guard cannot be applied to one of
      them and forgotten on another.

      It deletes, then READS THE STATE BACK, and reports what is true rather
      than what was attempted. The predecessor printed "deleted DIR" beside a
      Remove-Item -ErrorAction SilentlyContinue, so a multi-gigabyte VHDX left
      behind read as a disk that had gone.

      The retry is not decoration. 'wsl --unregister' releases the VHDX
      asynchronously, so the delete immediately after it can lose the race
      against a handle that is about to be closed anyway.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$What,
        [string]$Remedy = '',
        [int]$Attempts = 5
    )
    Assert-InsideBaseDir -Path $Path          # ⛔ inside the helper, not beside it

    for ($i = 1; $i -le $Attempts; $i++) {
        # The read-back below is the verdict. This catch exists so a failed
        # attempt does not abort the retry loop; it is not where success is
        # decided, which is why it discards rather than reports.
        try { Remove-Item -LiteralPath $Path -Recurse -Force -ErrorAction Stop }
        catch { $null = $_ }

        if (-not (Test-Path -LiteralPath $Path)) {
            Write-Ok "deleted $Path"
            return
        }
        if ($i -lt $Attempts) { Start-Sleep -Milliseconds (200 * $i) }
    }

    $msg = "FAILED to delete the $What at '$Path'. It is STILL THERE after $Attempts attempts."
    if ($Remedy) { $msg = "$msg $Remedy" }
    throw ($msg + " Something is holding it open: WSL releases the disk asynchronously, and an " +
           "explorer window, a shell whose working directory is inside it, or an indexer will " +
           "each do it too.")
}

function ConvertTo-SafeName {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Raw)
    $s = $Raw.ToLowerInvariant() -replace '[^a-z0-9._-]', '-'
    $s = $s -replace '-{2,}', '-'
    $s = $s -replace '\.{2,}', '.'      # kill any ".." traversal component
    return $s.Trim('-', '.')
}

function New-DistroName {
    param([AllowEmptyString()][string]$FromImage)
    $stem = 'rootfs'
    if (-not [string]::IsNullOrWhiteSpace($FromImage)) { $stem = ConvertTo-SafeName -Raw $FromImage }
    if ([string]::IsNullOrWhiteSpace($stem)) { $stem = 'rootfs' }
    if ($stem.Length -gt 32) { $stem = $stem.Substring(0, 32).Trim('-', '.') }
    $suffix = -join ((48..57) + (97..122) | Get-Random -Count 4 | ForEach-Object { [char]$_ })
    return "$($script:Prefix)$stem-$suffix"
}

function Resolve-DistroName {
    param([AllowEmptyString()][string]$Requested, [AllowEmptyString()][string]$FromImage)
    if ([string]::IsNullOrWhiteSpace($Requested)) { return (New-DistroName -FromImage $FromImage) }
    $n = ConvertTo-SafeName -Raw $Requested
    if ([string]::IsNullOrWhiteSpace($n)) { throw "Name '$Requested' sanitises to nothing." }
    if (-not $n.StartsWith($script:Prefix, [StringComparison]::Ordinal)) { $n = "$($script:Prefix)$n" }
    return $n
}

# --------------------------------------------------------------------------------------
# Rootfs acquisition
# --------------------------------------------------------------------------------------
function Get-ContainerEngine {
    foreach ($exe in @('podman', 'docker')) {
        $cmd = Get-Command "$exe.exe" -ErrorAction SilentlyContinue
        if ($cmd) { return [pscustomobject]@{ Name = $exe; Path = $cmd.Source } }
    }
    $candidates = @(
        [pscustomobject]@{ Name = 'podman'; Path = (Join-Path $env:LOCALAPPDATA 'Programs\Podman\podman.exe') },
        [pscustomobject]@{ Name = 'podman'; Path = (Join-Path $env:ProgramFiles 'RedHat\Podman\podman.exe') },
        [pscustomobject]@{ Name = 'docker'; Path = (Join-Path $env:ProgramFiles 'Docker\Docker\resources\bin\docker.exe') }
    )
    foreach ($c in $candidates) {
        if (Test-Path -LiteralPath $c.Path) { return $c }
    }
    return $null
}

function ConvertTo-OciArch {
    <#
      Maps whatever a container engine calls the host architecture onto the name
      --platform wants. The two engines disagree with each other AND with the
      OCI names: podman info says 'amd64', docker info says 'x86_64', and
      --platform accepts only the first.

      An unrecognised value passes through lowercased rather than being rejected
      here. A riscv64 or ppc64le host is a legitimate answer this table has not
      been taught, and the caller validates the shape before using it; guessing
      a substitute would be worse than handing the engine a name it can refuse.
    #>
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Raw)
    $v = $Raw.Trim().ToLowerInvariant()
    switch ($v) {
        'x86_64'  { return 'amd64' }
        'x86-64'  { return 'amd64' }
        'aarch64' { return 'arm64' }
        'armv8l'  { return 'arm64' }
        'armv7l'  { return 'arm' }
        'armhf'   { return 'arm' }
        'armv6l'  { return 'arm' }
        'i386'    { return '386' }
        'i486'    { return '386' }
        'i586'    { return '386' }
        'i686'    { return '386' }
        'x86'     { return '386' }
        default   { return $v }
    }
}

function Export-ImageRootfs {
    <# Pull an OCI image and flatten it to a rootfs tarball. #>
    param(
        [Parameter(Mandatory = $true)][string]$ImageRef,
        [Parameter(Mandatory = $true)][string]$OutFile
    )
    $engine = Get-ContainerEngine
    if (-not $engine) {
        throw ("No container engine found. Install podman or docker, or pass -Tarball " +
               "with a rootfs archive instead.")
    }
    Write-Step "Engine: $($engine.Name) ($($engine.Path))"

    # Readiness probe, and ITS ANSWER IS USED: it is what pins --platform below.
    # A stopped podman machine otherwise yields a cryptic pull error.
    #
    # The two engines spell the field differently. podman has .Host.Arch and
    # docker has .Architecture, and asking docker for .Host.Arch fails the whole
    # probe, which reads as "docker is not responding" on a machine where docker
    # is fine.
    $archField = '{{.Host.Arch}}'
    if ($engine.Name -eq 'docker') { $archField = '{{.Architecture}}' }

    $rawArch = ''
    try {
        $probe = Invoke-Native -FilePath $engine.Path -Arguments @('info', '--format', $archField)
        if ($probe) { $rawArch = ($probe | Select-Object -Last 1).ToString().Trim() }
    }
    catch {
        throw ("$($engine.Name) is installed but not responding. If you use podman on Windows, " +
               "start its VM with:  podman machine start`nUnderlying error: $($_.Exception.Message)")
    }

    # Refusing here rather than pulling unqualified. An unqualified pull takes
    # whatever the shared local tag currently points at, and one earlier
    # --platform pull is enough to have repointed it: the rootfs then imports
    # and every binary in it fails to execute, which surfaces only as the smoke
    # test's "/bin/sh did not run" with no hint of the cause.
    $arch = ConvertTo-OciArch -Raw $rawArch
    if ($arch -notmatch '^[a-z0-9_]+$') {
        throw ("Could not read the host architecture from '$($engine.Name) info --format " +
               "$archField'; it answered '$rawArch'. Refusing to pull without --platform, " +
               "because an unqualified pull can silently export the wrong architecture.")
    }
    $platform = "linux/$arch"
    Write-Step "Platform: $platform"

    Write-Step "Pulling $ImageRef"
    Invoke-Native -FilePath $engine.Path -Arguments @('pull', '--platform', $platform, $ImageRef) | Out-Null

    $cid = $null
    try {
        # 'create' materialises a container without running it; its filesystem is the rootfs.
        # Images with no CMD/ENTRYPOINT reject a bare create, so fall back to naming one.
        try {
            $cid = (Invoke-Native -FilePath $engine.Path -Arguments @('create', '--platform', $platform, $ImageRef) |
                    Select-Object -Last 1).ToString().Trim()
        }
        catch {
            Write-Warn "bare create failed; retrying with an explicit command"
            $cid = (Invoke-Native -FilePath $engine.Path -Arguments @('create', '--platform', $platform, $ImageRef, '/bin/sh') |
                    Select-Object -Last 1).ToString().Trim()
        }
        if ([string]::IsNullOrWhiteSpace($cid)) { throw "Container id was empty." }

        $short = $cid.Substring(0, [Math]::Min(12, $cid.Length))
        Write-Step "Exporting rootfs (container $short)"
        # -o is mandatory: PowerShell redirection corrupts binary streams.
        Invoke-Native -FilePath $engine.Path -Arguments @('export', '-o', $OutFile, $cid) | Out-Null
    }
    finally {
        if ($cid) {
            try { Invoke-Native -FilePath $engine.Path -Arguments @('rm', '-f', $cid) -IgnoreExitCode | Out-Null }
            catch { Write-Warn "could not remove temp container $cid" }
        }
    }

    if (-not (Test-Path -LiteralPath $OutFile)) { throw "Export produced no file at $OutFile" }
    $size = (Get-Item -LiteralPath $OutFile).Length
    if ($size -lt 1KB) { throw "Exported rootfs is implausibly small ($size bytes)." }
    Write-Ok ("rootfs: {0:N1} MiB" -f ($size / 1MB))
}

# --------------------------------------------------------------------------------------
# Getting bytes into a distro
# --------------------------------------------------------------------------------------
function ConvertTo-ShellSingleQuoted {
    <#
      POSIX single-quoting. The only character a single-quoted string cannot
      contain is a single quote, so it is written by closing, escaping and
      reopening. Used for values that end up INSIDE a file in the guest, never
      for the command that carries them there: see Write-DistroFile for why
      quoting does not survive the trip.
    #>
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Raw)
    return "'" + ($Raw -replace "'", "'\''") + "'"
}

function Write-DistroFile {
    <#
      Put bytes inside a distro without a shell touching them.

      ⭐ THE SCRIPT THIS SENDS IS NOW A PAYLOAD LIKE ANY OTHER. It used to be
      hand-written inside the alphabet that survives wsl.exe:

        mkdir -p DIR && echo B64|base64 -d>PATH && chmod MODE PATH

      which worked, and was a constraint nothing enforced. The next person to
      add a quote to it would have shipped WSL-12 again in a new place. It now
      goes through ConvertTo-DistroScriptCommand like every other payload, so
      the alphabet rule is checked by a machine and the text below is free to
      be quoted properly.

      THE CONTENT STAYS BASE64 inside that payload. Single-quoting it would
      work for text and would need a second escaping rule for anything else;
      base64 needs none and has no failure mode a here-document has.

      THE PATH is single-quoted now that it can be, AND still restricted to an
      absolute path of letters, digits, dot, dash and underscore. ⚠ The
      restriction is no longer load-bearing for the transport; it is kept as a
      sanity guard, because every path this script writes is one it chose, and
      a path arriving with a newline in it is a bug rather than an intention.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$DistroName,
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Content,
        [string]$Mode = '0644'
    )
    if ($Path -notmatch '^/[A-Za-z0-9_.\-]+(/[A-Za-z0-9_.\-]+)*$') {
        throw ("Refusing to write '$Path' inside the distro. Paths written by this script are " +
               "restricted to an absolute path of letters, digits, dot, dash and underscore.")
    }
    if ($Mode -notmatch '^[0-7]{3,4}$') { throw "Mode '$Mode' is not an octal file mode." }

    $b64   = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($Content))
    $slash = $Path.LastIndexOf('/')
    $dir   = if ($slash -gt 0) { $Path.Substring(0, $slash) } else { '/' }
    $qPath = ConvertTo-ShellSingleQuoted -Raw $Path
    $qDir  = ConvertTo-ShellSingleQuoted -Raw $dir

    # Each step reports its own failure. Without the || exit lines a missing
    # base64 leaves an empty file and a chmod that succeeds over it, and the
    # whole thing returns 0 having written nothing.
    $payload = @(
        "mkdir -p $qDir || exit 1",
        "echo $b64 | base64 -d > $qPath || exit 1",
        "chmod $Mode $qPath || exit 1"
    ) -join "`n"

    $rc = 0
    Invoke-InDistro -DistroName $DistroName -RunAs 'root' `
        -ScriptBytes (ConvertTo-Utf8Bytes -Text $payload) -ExitCode ([ref]$rc)
    if ($rc -ne 0) {
        throw ("Could not write $Path inside '$DistroName' (exit $rc). The likeliest cause is an " +
               "image with no base64: busybox has one and coreutils has one, but a rootfs built " +
               "from scratch may have neither.")
    }
}

function Get-ImageOciConfig {
    <#
      The image's OCI configuration. 'podman export' writes a FILESYSTEM and no
      configuration by definition, so ENV, WORKDIR, USER and ENTRYPOINT are not
      in the rootfs and have to be read from the image separately.

      '{{json .Config}}' is the one spelling both engines share. Their arch
      fields are not: see ConvertTo-OciArch.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$EnginePath,
        [Parameter(Mandatory = $true)][string]$ImageRef
    )
    $raw = Invoke-Native -FilePath $EnginePath -Arguments @('image', 'inspect', $ImageRef, '--format', '{{json .Config}}')
    $text = ($raw | Out-String).Trim()
    if ([string]::IsNullOrWhiteSpace($text) -or $text -eq 'null') {
        throw "Could not read the OCI configuration of '$ImageRef'; the engine answered '$text'."
    }
    return ($text | ConvertFrom-Json)
}

function New-OciEnvScript {
    <#
      Turns an image config into a /etc/profile.d snippet.

      ENV and WORKDIR are carried. USER and ENTRYPOINT ARE NOT, and that is a
      decision rather than an omission: WSL fixes the login user at import time
      and -User selects it per call, and a login shell has no entrypoint to run.
      Writing either into profile.d would be a setting that looks like it works.
    #>
    param(
        [Parameter(Mandatory = $true)]$Config,
        [Parameter(Mandatory = $true)][string]$ImageRef
    )
    $lines = @(
        '# Written by wsl-ephemeral.ps1 -OciEnv, from the OCI config of:',
        "#   $ImageRef",
        '# The rootfs came from a filesystem export, which carries no config, so',
        '# without this file the environment here is WSL default and not the',
        '# image environment.'
    )
    $names = @($Config.PSObject.Properties.Name)

    if ($names -contains 'Env' -and $Config.Env) {
        foreach ($e in @($Config.Env)) {
            $i = "$e".IndexOf('=')
            if ($i -lt 1) { Write-Warn "skipping malformed image env entry: $e"; continue }
            $k = "$e".Substring(0, $i)
            $v = "$e".Substring($i + 1)
            if ($k -notmatch '^[A-Za-z_][A-Za-z0-9_]*$') {
                Write-Warn "skipping image env name that is not a shell identifier: $k"
                continue
            }
            $lines += ('export {0}={1}' -f $k, (ConvertTo-ShellSingleQuoted -Raw $v))
        }
    }

    if ($names -contains 'WorkingDir' -and $Config.WorkingDir -and $Config.WorkingDir -ne '/') {
        # ':' rather than 'true': it is a shell built-in everywhere, and the
        # guard is there so a WORKDIR the image creates at runtime does not make
        # every login shell fail.
        $lines += ('cd {0} 2>/dev/null || :' -f (ConvertTo-ShellSingleQuoted -Raw $Config.WorkingDir))
    }

    return (($lines -join "`n") + "`n")
}

# --------------------------------------------------------------------------------------
# Actions
# --------------------------------------------------------------------------------------
function Get-VolumeFreeBytes {
    <#
      Free bytes on the volume a path lives on, or $null when that cannot be
      read. AvailableFreeSpace rather than TotalFreeSpace: a quota'd volume can
      have plenty of the second and none of the first, and the import fails on
      the first.

      $null is a THIRD answer and the caller treats it as one. "I could not
      measure" is not "there is room", and it is not a reason to refuse either.
    #>
    param([Parameter(Mandatory = $true)][string]$Path)
    try {
        $root = [IO.Path]::GetPathRoot([IO.Path]::GetFullPath($Path))
        if ([string]::IsNullOrWhiteSpace($root)) { return $null }
        $drive = New-Object IO.DriveInfo $root
        if (-not $drive.IsReady) { return $null }
        return [int64]$drive.AvailableFreeSpace
    }
    catch { $null = $_; return $null }
}

function Assert-EnoughDiskSpace {
    <#
      Running out of space midway through --import leaves a partial VHDX and a
      registered distro that does not work, which the user then has to unpick.
      Refusing before the import is recoverable; that is the whole trade.

      ⚠ THE FACTOR IS MEASURED, NOT ASSUMED. The entry that asked for this said
      an import needs "roughly twice the rootfs size" and flagged the two as an
      estimate. It is not a multiple at all. Measured on this machine on
      2026-08-27, VHDX size on disk against the rootfs tarball that produced it:

        alpine:3.22          8.2 MiB tar ->  76 MiB vhdx   9.27x
        python:3.13-alpine  45.4 MiB tar -> 140 MiB vhdx   3.08x
        debian:bookworm     74.3 MiB tar -> 172 MiB vhdx   2.31x
        ubuntu:24.04        76.9 MiB tar -> 172 MiB vhdx   2.24x

      The cost is dominated by a FIXED FLOOR, not by a multiple: an 8 MiB
      rootfs still costs 76 MiB. So the requirement is a floor plus a multiple,
      and both are set above every measurement rather than fitted to them. A
      preflight that is tight is a preflight that refuses a working import.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$TarballPath,
        [Parameter(Mandatory = $true)][string]$TargetDir
    )
    $tar  = (Get-Item -LiteralPath $TarballPath).Length
    $need = ($tar * $script:ImportSpaceFactor) + $script:ImportSpaceFloor
    $free = Get-VolumeFreeBytes -Path $TargetDir

    if ($null -eq $free) {
        # ⛔ Named, not silent. A preflight that skipped is not a preflight
        # that passed, and the import is still worth attempting.
        Write-Warn ("could not read free space for '$TargetDir'; importing without the preflight. " +
                    "If the volume is full, the import will leave a partial disk.")
        return
    }

    Write-Ok ("space: {0:N0} MiB needed, {1:N0} MiB free" -f ($need / 1MB), ($free / 1MB))
    if ($free -ge $need) { return }

    throw ("NOT ENOUGH DISK SPACE to import '$TargetDir'. " +
           ("Need about {0:N0} MiB and {1:N0} MiB is free" -f ($need / 1MB), ($free / 1MB)) +
           (" on the volume holding {0}." -f [IO.Path]::GetPathRoot([IO.Path]::GetFullPath($TargetDir))) +
           " Nothing has been imported and nothing is registered. Free some space, or point" +
           " LOCALAPPDATA at a volume that has it, and run this again.")
}

function Remove-EphemeralDistro {
    param(
        [Parameter(Mandatory = $true)][string]$DistroName,
        [switch]$SkipConfirm
    )
    Assert-Removable -DistroName $DistroName          # hard guard, always first

    if (-not $SkipConfirm) {
        if (-not (Confirm-Destructive -Target $DistroName -Operation 'Unregister WSL distro and DELETE its disk')) {
            Write-Warn "skipped $DistroName"
            return
        }
    }

    $wsl = Get-WslExe
    if ((Get-WslDistroNames) -contains $DistroName) {
        Invoke-Native -FilePath $wsl -Arguments @('--terminate', $DistroName) -IgnoreExitCode | Out-Null
        Invoke-Native -FilePath $wsl -Arguments @('--unregister', $DistroName) | Out-Null
        Write-Ok "unregistered $DistroName"
    }
    else {
        Write-Warn "$DistroName was not registered"
    }

    $dir = Join-Path $script:BaseDir $DistroName
    if (Test-Path -LiteralPath $dir) {
        $remedy = "Close it and re-run: -Action Remove -Name $DistroName -Force."
        Remove-PathWithRetry -Path $dir -What "disk for '$DistroName'" -Remedy $remedy
    }
}

function Get-OrphanTarball {
    <#
      Rootfs tarballs sitting loose in the base directory. New writes one there
      and removes it in a finally, and a finally does not run on every hard
      interrupt, so an interrupted run can leave several hundred MiB that
      nothing reported.

      IT CANNOT TELL AN ORPHAN FROM A RUNNING NEW, and it does not pretend to:
      a New that is executing right now has its tarball in exactly this place.
      That is why the report carries the last-written time instead of a verdict.
    #>
    if (-not (Test-Path -LiteralPath $script:BaseDir)) { return @() }
    return @(Get-ChildItem -LiteralPath $script:BaseDir -Filter '*.tar' -File -ErrorAction SilentlyContinue |
             Sort-Object -Property Name)
}

function Invoke-ActionNew {
    if (-not $Image -and -not $Tarball) { throw "Action New requires -Image (e.g. alpine:3.22) or -Tarball <path>." }
    if ($Image -and $Tarball)           { throw "Pass either -Image or -Tarball, not both." }

    $distro = Resolve-DistroName -Requested $Name -FromImage $Image
    if (Test-ProtectedName -DistroName $distro) { throw "Refusing to create a distro named '$distro' (protected)." }
    if ((Get-WslDistroNames) -contains $distro) {
        throw "Distro '$distro' already exists. Choose another -Name or remove it first."
    }

    $target  = Join-Path $script:BaseDir $distro
    Assert-InsideBaseDir -Path $target               # validate before we ever create it
    $tarPath = $null
    $tempTar = $false
    # 0 means "nothing ran, nothing failed": with no -Command there is no inner
    # code to carry. Declared here so [ref]$rc has a variable to bind to, and so
    # the exit at the bottom of this function can read it whatever happened.
    $rc      = 0

    try {
        New-Item -ItemType Directory -Path $target -Force | Out-Null

        if ($Tarball) {
            if (-not (Test-Path -LiteralPath $Tarball)) { throw "Tarball not found: $Tarball" }
            $tarPath = (Resolve-Path -LiteralPath $Tarball).Path
        }
        else {
            $tarPath = Join-Path $script:BaseDir ("{0}.tar" -f $distro)
            $tempTar = $true
            Export-ImageRootfs -ImageRef $Image -OutFile $tarPath
        }

        Assert-EnoughDiskSpace -TarballPath $tarPath -TargetDir $target

        Write-Step "Importing as WSL2 distro '$distro'"
        $wsl = Get-WslExe
        Invoke-Native -FilePath $wsl -Arguments @('--import', $distro, $target, $tarPath, '--version', '2') | Out-Null

        # Smoke test: a distro whose /bin/sh does not run is useless. Fail loudly now.
        # The loop also absorbs a first-boot race: drvfs automount of /mnt/<drive> can lag
        # the first shell by a second or two, which otherwise makes the very first user
        # command fail on a path under /mnt/c for no visible reason.
        #
        # ⭐ IT ALSO PROVES THE COMMAND CHANNEL, because it goes through the
        # same transport every -Command does. A guest with no base64, or no
        # /dev/fd, fails HERE, at creation, with a message naming it, instead
        # of at some later Run whose exit code would look like the command's.
        $prev = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
        try {
            # NOT A HERE-STRING, ON PURPOSE. docs/conventions/shell.md: Windows
            # PowerShell 5.1 mis-parses a here-string whose terminator arrives
            # with a bare LF, and this template's defence is to write none in a
            # .ps1 at all rather than to rely on .gitattributes reaching every
            # checkout. An array joined with a newline carries the same script
            # and no line ending can break it.
            #
            # ⭐ THE BRACKET AND THE DOUBLE QUOTE ON THE NOTE LINE ARE
            # DELIBERATE. That exact line is what WSL-12 was: it reached the
            # guest as syntax under Windows PowerShell 5.1, every -Action New
            # failed, and the fix then was to rewrite the payload inside an
            # alphabet nothing enforced. It is back, unchanged, because the
            # transport now carries it. If it ever breaks again, this line is
            # the one that says so, on the host it broke on.
            $probeScript = @(
                'echo __WSL_OK__',
                'for _ in 1 2 3 4 5 6 7 8 9 10; do',
                '    if [ -d /mnt/c ]; then break; fi',
                '    sleep 1',
                'done',
                'if [ ! -d /mnt/c ]; then echo "note: no /mnt/c (Windows drives not mounted)"; fi',
                'head -2 /etc/os-release 2>/dev/null || echo "os-release: n/a"'
            ) -join "`n"
            $probeLine = ConvertTo-DistroScriptCommand `
                -ScriptBytes (ConvertTo-Utf8Bytes -Text $probeScript) `
                -GuestPath (New-GuestScratchPath)
            $probe = & $wsl -d $distro -u root -- /bin/sh -lc $probeLine 2>&1
        }
        finally { $ErrorActionPreference = $prev }

        $probeText = ($probe | Out-String)
        if ($probeText -notmatch '__WSL_OK__') {
            throw ("Distro imported but /bin/sh did not run, or this rootfs cannot carry a " +
                   "command: the channel needs base64 and /dev/fd inside the guest. " +
                   "Output: $($probeText.Trim())")
        }
        Write-Ok "'$distro' is up"
        foreach ($l in ($probeText -split "`r?`n")) {
            $t = $l.Trim()
            if ($t -and $t -ne '__WSL_OK__') { Write-Host "    $t" -ForegroundColor DarkGray }
        }

        if ($OciEnv) {
            if ($Tarball) {
                Write-Warn "-OciEnv ignored: a rootfs tarball carries no OCI configuration."
            }
            else {
                Write-Step "Carrying the image's OCI configuration into the distro"
                $cfgEngine = Get-ContainerEngine
                if (-not $cfgEngine) { throw "-OciEnv needs the container engine that built this rootfs, and none is on PATH now." }
                $cfg = Get-ImageOciConfig -EnginePath $cfgEngine.Path -ImageRef $Image
                $ociBody = New-OciEnvScript -Config $cfg -ImageRef $Image
                Write-DistroFile -DistroName $distro -Path '/etc/profile.d/10-oci-env.sh' -Content $ociBody
                Write-Ok "wrote /etc/profile.d/10-oci-env.sh"
            }
        }

        if ($null -ne $script:CommandBytes) {
            Write-Step "Running command as '$User'"
            Invoke-InDistro -DistroName $distro -RunAs $User -ScriptBytes $script:CommandBytes -ExitCode ([ref]$rc)
            if ($rc -ne 0) { Write-Warn "command exited $rc" }
        }

        if (-not $Ephemeral) {
            Write-Host ""
            Write-Host "  Distro : $distro"        -ForegroundColor White
            Write-Host "  Disk   : $target"        -ForegroundColor White
            Write-Host "  Enter  : wsl -d $distro" -ForegroundColor White
            Write-Host "  Remove : -Action Remove -Name $distro -Force" -ForegroundColor White
        }
    }
    catch {
        Write-Warn "creation failed; rolling back"
        try {
            if ((Get-WslDistroNames) -contains $distro) {
                Assert-Removable -DistroName $distro
                Invoke-Native -FilePath (Get-WslExe) -Arguments @('--unregister', $distro) -IgnoreExitCode | Out-Null
            }
            if (Test-Path -LiteralPath $target) {
                # Through the same helper as every other deletion. If it cannot
                # delete, it throws, the catch below reports the rollback as
                # incomplete, and the ORIGINAL error is still what gets rethrown.
                Remove-PathWithRetry -Path $target -What "partial disk for '$distro'"
            }
        }
        catch { Write-Warn "rollback incomplete: $($_.Exception.Message)" }
        throw
    }
    finally {
        if ($tempTar -and $tarPath -and (Test-Path -LiteralPath $tarPath)) {
            # ⛔ THROUGH THE SAME DELETION AS EVERYTHING ELSE. This was the
            # fourth deletion path and it had its own Remove-Item, no
            # containment guard and no read-back, while wsl-ephemeral.md
            # claimed there was one deletion and every path reached it. A door
            # sweep found it; the claim was false for one commit.
            #
            # ⚠ Caught, not propagated. This runs in a finally, and a failure
            # here must not replace the real outcome of the action: a tarball
            # left behind is an orphan, which List reports and Purge removes.
            try { Remove-PathWithRetry -Path $tarPath -What 'temporary rootfs tarball' }
            catch { Write-Warn "could not remove the temp tarball: $($_.Exception.Message)" }
        }
    }

    # ⛔ The teardown is OUTSIDE the try on purpose. Inside it, a teardown that
    # could not delete the disk was reported as "creation failed; rolling back",
    # which is false in a way that sends the reader looking in the wrong place:
    # the distro was created, the command ran, and the only thing wrong is that
    # several gigabytes are still on disk. Out here, that failure arrives as
    # itself and the script exits 1 naming the path.
    if ($Ephemeral) {
        Write-Step "-Ephemeral set: tearing down '$distro'"
        Remove-EphemeralDistro -DistroName $distro -SkipConfirm
    }

    # After the teardown above, and after the finally has removed the temp
    # tarball. Both of those are the reason this is at the bottom of the
    # function rather than beside the command that produced the code.
    exit $rc
}

function Invoke-ActionRun {
    if (-not $Name) { throw "Action Run requires -Name." }
    if ($null -eq $script:CommandBytes) {
        throw "Action Run requires -Command, -CommandFile or -CommandB64."
    }
    $distro = Resolve-DistroName -Requested $Name -FromImage ''
    if ((Get-WslDistroNames) -notcontains $distro) {
        throw "Distro '$distro' is not registered. Create it with -Action New."
    }
    $rc = 0
    Invoke-InDistro -DistroName $distro -RunAs $User -ScriptBytes $script:CommandBytes -ExitCode ([ref]$rc)
    exit $rc
}

function Invoke-ActionList {
    $all  = @(Get-WslDistroNames)
    $mine = @($all | Where-Object { $_.StartsWith($script:Prefix, [StringComparison]::Ordinal) })
    Write-Step "Ephemeral distros (prefix '$($script:Prefix)')"
    if ($mine.Count -eq 0) { Write-Host "  (none)" -ForegroundColor DarkGray }
    else { foreach ($m in $mine) { Write-Host "  $m" } }

    Write-Step "Other distros on this system -- never touched by this script"
    $others = @($all | Where-Object { -not $_.StartsWith($script:Prefix, [StringComparison]::Ordinal) })
    if ($others.Count -eq 0) { Write-Host "  (none)" -ForegroundColor DarkGray }
    else {
        foreach ($o in $others) {
            $tag = ''
            if (Test-ProtectedName -DistroName $o) { $tag = '   [PROTECTED]' }
            Write-Host ("  {0}{1}" -f $o, $tag) -ForegroundColor DarkGray
        }
    }

    Write-Step "Orphaned rootfs tarballs in $($script:BaseDir)"
    $orphans = @(Get-OrphanTarball)
    if ($orphans.Count -eq 0) { Write-Host "  (none)" -ForegroundColor DarkGray }
    else {
        foreach ($t in $orphans) {
            $line = "  {0}   {1:N1} MiB   written {2:yyyy-MM-dd HH:mm:ss}Z"
            Write-Host ($line -f $t.Name, ($t.Length / 1MB), $t.LastWriteTimeUtc)
        }
        $sum = ($orphans | Measure-Object -Property Length -Sum).Sum
        Write-Warn ("{0:N1} MiB total. Remove them with -Action Purge." -f ($sum / 1MB))
        Write-Warn "a New running right now also has a .tar here: check the time before purging."
    }
}

function Invoke-ActionPurge {
    $mine    = @(Get-WslDistroNames | Where-Object { $_.StartsWith($script:Prefix, [StringComparison]::Ordinal) })
    $orphans = @(Get-OrphanTarball)

    if ($mine.Count -eq 0 -and $orphans.Count -eq 0) { Write-Ok "nothing to purge"; return }

    $what = @()
    if ($mine.Count -gt 0) {
        Write-Step "Ephemeral distro(s): $($mine -join ', ')"
        $what += "$($mine.Count) distro(s)"
    }
    if ($orphans.Count -gt 0) {
        $sum = ($orphans | Measure-Object -Property Length -Sum).Sum
        Write-Step ("Orphaned rootfs tarball(s), {0:N1} MiB: {1}" -f ($sum / 1MB), (($orphans | ForEach-Object { $_.Name }) -join ', '))
        $what += ("{0} tarball(s)" -f $orphans.Count)
    }

    # ONE confirmation covering both classes. Two prompts over one -Force is how
    # somebody learns to pass -Force without reading either of them.
    if (-not (Confirm-Destructive -Target ($what -join ' and ') -Operation 'Purge')) { return }

    # Counted, not thrown on at the first failure. One stuck item must not hide
    # the state of the rest, so both loops finish and the tally decides the exit
    # code.
    $failed = 0
    foreach ($d in $mine) {
        try { Remove-EphemeralDistro -DistroName $d -SkipConfirm }
        catch { Write-Warn "skip ${d}: $($_.Exception.Message)"; $failed++ }
    }
    foreach ($t in $orphans) {
        # Through the SAME deletion as a distro disk, so the containment guard
        # covers both. A second removal path with its own checks is how one of
        # them ends up without any.
        try { Remove-PathWithRetry -Path $t.FullName -What 'orphaned rootfs tarball' }
        catch { Write-Warn "skip $($t.Name): $($_.Exception.Message)"; $failed++ }
    }

    # A Purge that could not remove something must NOT exit 0. Warning and
    # returning success is the defect WSL-04 took out of the single delete,
    # reappearing one level up in the loop that calls it.
    if ($failed -gt 0) {
        throw ("$failed of " + ($mine.Count + $orphans.Count) +
               " item(s) were NOT removed. Each is named in a warning above.")
    }
}

# --------------------------------------------------------------------------------------
# Main
# --------------------------------------------------------------------------------------
try {
    if ([string]::IsNullOrWhiteSpace($script:BaseDir)) { throw "LOCALAPPDATA is not set; cannot choose a base directory." }
    New-Item -ItemType Directory -Path $script:BaseDir -Force | Out-Null

    # Resolved ONCE, here, so New and Run cannot disagree about which switch
    # won and a bad -CommandFile is refused before a distro is built for it.
    $script:CommandBytes = Resolve-CommandBytes -Text $Command -FromFile $CommandFile -FromB64 $CommandB64

    switch ($Action) {
        'New'    { Invoke-ActionNew }
        'Run'    { Invoke-ActionRun }
        'List'   { Invoke-ActionList }
        'Remove' {
            if (-not $Name) { throw "Action Remove requires -Name." }
            Remove-EphemeralDistro -DistroName (Resolve-DistroName -Requested $Name -FromImage '')
        }
        'Purge'  { Invoke-ActionPurge }
    }
}
catch {
    Write-Host "ERROR: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
