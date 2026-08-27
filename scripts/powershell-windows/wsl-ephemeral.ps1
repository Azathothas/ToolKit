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
    Shell command to run, via /bin/sh -lc.

.PARAMETER User
    User to run as inside the distro. Default 'root'.

.PARAMETER Ephemeral
    With -Action New: run -Command then immediately destroy the distro.

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

    PLATFORM -- every pull and every create names linux/ARCH explicitly, where
    ARCH is this host's own, read from the engine. Naming it is not politeness:
    the local image store is keyed by tag and NOT by architecture, so a single
    earlier 'pull --platform linux/riscv64 alpine' repoints the shared
    alpine:latest, and the next unqualified pull is a no-op that exports a
    rootfs nothing in it can execute.

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
    Justification = 'Image, Tarball, Command, User, Ephemeral and Force are read by the Invoke-Action* functions through script scope rather than as arguments. The analyzer does not follow that, and threading six parameters through every call to satisfy it would make the code worse.')]
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
    [string]$User = 'root',
    [switch]$Ephemeral,
    [switch]$Force
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# --------------------------------------------------------------------------------------
# Constants
# --------------------------------------------------------------------------------------
$script:Prefix  = 'eph-'
$script:BaseDir = Join-Path $env:LOCALAPPDATA 'wsl-ephemeral'

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

function Invoke-InDistro {
    <#
      The ONE path that runs a caller's command inside a distro. New and Run both
      go through it, so an inner exit code cannot be propagated by one action and
      dropped by the other. It was dropped by New, which is what made -Command
      useless as a gate.

      The code comes back through -ExitCode rather than as the return value, on
      purpose. The command's own stdout flows out of this function's success
      stream so the caller can see it, and `$rc = Invoke-InDistro ...` would
      therefore capture that OUTPUT into $rc instead of the code.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$DistroName,
        [Parameter(Mandatory = $true)][string]$RunAs,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$ShellCommand,
        [Parameter(Mandatory = $true)][ref]$ExitCode
    )
    # Set before the try, and set non-zero. Under Set-StrictMode -Version Latest
    # an unassigned variable throws when it is READ, so every path out of here
    # has to leave a code behind; and "it never answered" is a failure, not a
    # pass, so the value it starts at has to be one that fails.
    $ExitCode.Value = 1
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        & (Get-WslExe) -d $DistroName -u $RunAs -- /bin/sh -lc $ShellCommand
        if ($null -ne $LASTEXITCODE) { $ExitCode.Value = [int]$LASTEXITCODE }
    }
    finally { $ErrorActionPreference = $prev }
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
# Actions
# --------------------------------------------------------------------------------------
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
        Assert-InsideBaseDir -Path $dir               # containment guard
        Remove-Item -LiteralPath $dir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Ok "deleted $dir"
    }
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

        Write-Step "Importing as WSL2 distro '$distro'"
        $wsl = Get-WslExe
        Invoke-Native -FilePath $wsl -Arguments @('--import', $distro, $target, $tarPath, '--version', '2') | Out-Null

        # Smoke test: a distro whose /bin/sh does not run is useless. Fail loudly now.
        # The loop also absorbs a first-boot race: drvfs automount of /mnt/<drive> can lag
        # the first shell by a second or two, which otherwise makes the very first user
        # command fail on a path under /mnt/c for no visible reason.
        $prev = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
        try {
            # NOT A HERE-STRING, ON PURPOSE. docs/conventions/shell.md: Windows
            # PowerShell 5.1 mis-parses a here-string whose terminator arrives
            # with a bare LF, and this template's defence is to write none in a
            # .ps1 at all rather than to rely on .gitattributes reaching every
            # checkout. An array joined with a newline carries the same script
            # and no line ending can break it.
            $probeScript = @(
                'echo __WSL_OK__',
                'for _ in 1 2 3 4 5 6 7 8 9 10; do',
                '    if [ -d /mnt/c ]; then break; fi',
                '    sleep 1',
                'done',
                'if [ ! -d /mnt/c ]; then echo "note: no /mnt/c (Windows drives not mounted)"; fi',
                'head -2 /etc/os-release 2>/dev/null || echo "os-release: n/a"'
            ) -join "`n"
            $probe = & $wsl -d $distro -u root -- /bin/sh -lc $probeScript 2>&1
        }
        finally { $ErrorActionPreference = $prev }

        $probeText = ($probe | Out-String)
        if ($probeText -notmatch '__WSL_OK__') {
            throw "Distro imported but /bin/sh did not run. Output: $($probeText.Trim())"
        }
        Write-Ok "'$distro' is up"
        foreach ($l in ($probeText -split "`r?`n")) {
            $t = $l.Trim()
            if ($t -and $t -ne '__WSL_OK__') { Write-Host "    $t" -ForegroundColor DarkGray }
        }

        if ($Command) {
            Write-Step "Running command as '$User'"
            Invoke-InDistro -DistroName $distro -RunAs $User -ShellCommand $Command -ExitCode ([ref]$rc)
            if ($rc -ne 0) { Write-Warn "command exited $rc" }
        }

        if ($Ephemeral) {
            # The teardown happens HERE, inside the try, so that the exit at the
            # bottom of this function cannot be reached with a distro still
            # registered. Exiting before this point leaks a distro and a VHDX.
            Write-Step "-Ephemeral set: tearing down '$distro'"
            Remove-EphemeralDistro -DistroName $distro -SkipConfirm
        }
        else {
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
                Assert-InsideBaseDir -Path $target
                Remove-Item -LiteralPath $target -Recurse -Force -ErrorAction SilentlyContinue
            }
        }
        catch { Write-Warn "rollback incomplete: $($_.Exception.Message)" }
        throw
    }
    finally {
        if ($tempTar -and $tarPath -and (Test-Path -LiteralPath $tarPath)) {
            Remove-Item -LiteralPath $tarPath -Force -ErrorAction SilentlyContinue
        }
    }

    # After the teardown above, and after the finally has removed the temp
    # tarball. Both of those are the reason this is at the bottom of the
    # function rather than beside the command that produced the code.
    exit $rc
}

function Invoke-ActionRun {
    if (-not $Name)    { throw "Action Run requires -Name." }
    if (-not $Command) { throw "Action Run requires -Command." }
    $distro = Resolve-DistroName -Requested $Name -FromImage ''
    if ((Get-WslDistroNames) -notcontains $distro) {
        throw "Distro '$distro' is not registered. Create it with -Action New."
    }
    $rc = 0
    Invoke-InDistro -DistroName $distro -RunAs $User -ShellCommand $Command -ExitCode ([ref]$rc)
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
}

function Invoke-ActionPurge {
    $mine = @(Get-WslDistroNames | Where-Object { $_.StartsWith($script:Prefix, [StringComparison]::Ordinal) })
    if ($mine.Count -eq 0) { Write-Ok "nothing to purge"; return }
    Write-Step "Purging $($mine.Count) ephemeral distro(s): $($mine -join ', ')"
    if (-not (Confirm-Destructive -Target "$($mine.Count) distro(s)" -Operation 'Purge ephemeral distros')) { return }
    foreach ($d in $mine) {
        try { Remove-EphemeralDistro -DistroName $d -SkipConfirm }
        catch { Write-Warn "skip ${d}: $($_.Exception.Message)" }
    }
}

# --------------------------------------------------------------------------------------
# Main
# --------------------------------------------------------------------------------------
try {
    if ([string]::IsNullOrWhiteSpace($script:BaseDir)) { throw "LOCALAPPDATA is not set; cannot choose a base directory." }
    New-Item -ItemType Directory -Path $script:BaseDir -Force | Out-Null

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
