# check-binfmt.ps1 - are binfmt_misc handlers actually registered in the kernel
# that containers on this machine run against?
#
# ⭐ THE TWIN OF check-binfmt.sh, and the one to prefer on Windows, which is the
# host this check is mostly for: it drives wsl.exe natively rather than through
# an msys layer, and a native PowerShell session may have no POSIX shell at all.
#
# The defect this exists to catch is cross-architecture execution that has never
# once worked while every visible signal says the machine is healthy. Measured
# on the reporting machine on 2026-08-27: systemd-binfmt.service reported
# status=0/SUCCESS having registered ZERO handlers, because a systemd autofs was
# stacked on the binfmt_misc mount and every read returned ELOOP. The unit was
# green, the config was complete, the emulators were installed, and
# `podman run --platform linux/ARCH` failed with Exec format error that reads
# like an unrelated breakage.
#
# ⭐ It reads the KERNEL, not a unit's exit code. The unit is the thing that lied.
#
# ── ⚠ WHY NOT `podman machine ssh` ─────────────────────────────────────────
#
# The issue that asked for this assumed it. Measured on 2026-08-27: on Windows
# that command passes -o UserKnownHostsFile=NUL to its own ssh, and under Git
# Bash NUL is a FILENAME rather than the null device, so it writes a 99-byte
# file called NUL into the directory you ran it from. A diagnostic that litters
# the repository it is diagnosing is a worse defect than the one it detects.
#
# ⭐ It is also unnecessary. Every WSL2 distro on a machine shares ONE kernel, so
# `wsl -d DISTRO` reads the same handlers with no ssh, no key file, and nothing
# written anywhere.
#
# Usage:
#   pwsh -NoProfile -File scripts/common/check-binfmt.ps1
#   pwsh -NoProfile -File scripts/common/check-binfmt.ps1 -Json
#   pwsh -NoProfile -File scripts/common/check-binfmt.ps1 -Distro podman-machine-default
#   pwsh -NoProfile -File scripts/common/check-binfmt.ps1 -Require 1
#
# ⚠ -Require N is what turns this from a report into an assertion. WITHOUT it a
# count of zero is reported and exits 0, because a machine that never wanted
# cross-architecture execution is not broken. scripts/README.md: a check that
# measures an open defect must not fail the build for that defect alone.
#
# Exit codes: 0 read it, 1 the kernel state is broken or below -Require,
#             2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

[CmdletBinding()]
param(
    [switch]$Json,
    [string]$Distro = 'podman-machine-default',
    [int]$Require = 0
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$BinfmtDir = '/proc/sys/fs/binfmt_misc'

function Exit-With {
    param([Parameter(Mandatory = $true)][int]$Code, [Parameter(Mandatory = $true)][string]$Text)
    [Console]::Error.WriteLine("check-binfmt: $Text")
    exit $Code
}

if ($Require -lt 0) { Exit-With 2 '-Require cannot be negative' }

# WSL emits UTF-16LE unless this is set; without it every parsed string is
# NUL-riddled. docs/conventions/shell.md section 7.
$env:WSL_UTF8 = '1'

function Get-WslExe {
    $cmd = Get-Command wsl.exe -CommandType Application -ErrorAction SilentlyContinue |
           Select-Object -First 1
    if ($cmd) { return $cmd.Source }
    $fallback = Join-Path $env:WINDIR 'System32\wsl.exe'
    if (Test-Path -LiteralPath $fallback) { return $fallback }
    return $null
}

function Invoke-InDistro {
    <#
      Run one command in the distro and hand back its merged output and code.
      ⚠ stderr is MERGED on purpose here: ELOOP is the whole diagnosis and it
      arrives on stderr, so a version that read stdout alone would report "no
      handlers" over the one state this check exists to name.
    #>
    param([Parameter(Mandatory = $true)][string]$ShellCommand)
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $out = & $script:Wsl -d $Distro -u root -- /bin/sh -lc $ShellCommand 2>&1
        $code = $LASTEXITCODE
    }
    finally { $ErrorActionPreference = $prev }
    if ($null -eq $code) { $code = 1 }
    return [pscustomobject]@{
        Text = (($out | Out-String) -replace "`r", '').Trim()
        Code = $code
    }
}

$script:Wsl = Get-WslExe
if (-not $script:Wsl) {
    Exit-With 2 ("no wsl.exe on this host, so there is no Linux kernel to read $BinfmtDir from. " +
                 'Not applicable here, which is not a failure.')
}

$probe = Invoke-InDistro -ShellCommand 'exit 0'
if ($probe.Code -ne 0) {
    Exit-With 2 ("distro '$Distro' is not registered or would not start. Start it, or name " +
                 'another with -Distro. Could not run.')
}

$kernel = (Invoke-InDistro -ShellCommand 'uname -r').Text
$listing = Invoke-InDistro -ShellCommand "ls -1 $BinfmtDir"

$readErr = ''
$stacked = $false
if ($listing.Code -ne 0 -or
    $listing.Text -match 'Too many levels of symbolic links|No such file|Not a directory') {
    $readErr = $listing.Text
    if ($readErr -match 'Too many levels of symbolic links|ELOOP') { $stacked = $true }
}

$count = 0
$statusFile = 'unknown'
if (-not $readErr) {
    $names = @($listing.Text -split "`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ })
    $count = @($names | Where-Object { $_ -like 'qemu-*' }).Count
    $statusFile = if ($names -contains 'status') { 'present' } else { 'absent' }
}

$problem = ''
if ($readErr) { $problem = if ($stacked) { 'stacked-mount' } else { 'unreadable' } }
elseif ($count -lt $Require) { $problem = 'below-require' }

if ($Json) {
    # ⛔ Assembled as a string rather than through ConvertTo-Json, so the key
    # ORDER and the types match check-binfmt.sh byte for byte. check-twins.sh
    # compares the two answers and a reordered object is a false disagreement.
    $s = if ($stacked) { 1 } else { 0 }
    Write-Output ('{"schema":"check-binfmt/1","source":"wsl:' + $Distro + '","kernel":"' + $kernel +
                  '","handlers":' + $count + ',"status_file":"' + $statusFile +
                  '","stacked":' + $s + ',"problem":"' + $problem + '"}')
    if ($problem) { exit 1 }
    exit 0
}

Write-Output 'check-binfmt'
Write-Output "  read from      wsl:$Distro"
Write-Output "  kernel         $kernel"

if ($stacked) {
    Write-Output '  handlers       UNREADABLE'
    Write-Output ''
    Write-Output "⛔ $BinfmtDir exists and CANNOT BE READ: ELOOP."
    Write-Output '   That is a second filesystem stacked on the same path, and it is the'
    Write-Output '   state this check exists to name. systemd-binfmt.service writes into'
    Write-Output '   the path underneath and reports status=0/SUCCESS while registering'
    Write-Output '   nothing, so the unit is green and cross-architecture execution has'
    Write-Output '   never once worked.'
    Write-Output ''
    Write-Output '   The reading, verbatim:'
    foreach ($l in ($readErr -split "`n")) { Write-Output "     $l" }
    exit 1
}

if ($readErr) {
    Write-Output '  handlers       UNREADABLE'
    Write-Output ''
    Write-Output "⛔ $BinfmtDir could not be read, and NOT with the ELOOP this check knows:"
    foreach ($l in ($readErr -split "`n")) { Write-Output "     $l" }
    exit 1
}

Write-Output "  qemu handlers  $count"
Write-Output "  status file    $statusFile"

if ($count -eq 0) {
    Write-Output ''
    Write-Output '⚠ ZERO handlers are registered. Nothing is broken if this machine never'
    Write-Output '  wanted cross-architecture execution. If it did, this is why'
    Write-Output '  "podman run --platform linux/ARCH" fails with Exec format error, and'
    Write-Output '  a green systemd-binfmt.service does not contradict it.'
}

if ($problem -eq 'below-require') {
    Write-Output ''
    Write-Output "⛔ $count handler(s) registered, -Require asked for $Require."
    exit 1
}

Write-Output ''
Write-Output '⚠ Registered is not the same as reaching a container. A handler registered'
Write-Output '  WITHOUT the F flag needs its interpreter to exist inside the mount'
Write-Output '  namespace that runs; with F the kernel holds the interpreter open and it'
Write-Output '  does not. Read one to see which:'
Write-Output "    cat $BinfmtDir/qemu-aarch64"
exit 0
