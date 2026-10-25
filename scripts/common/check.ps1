<#
.SYNOPSIS
    Build the gate and run it.

.DESCRIPTION
    THE GATE IS ONE BINARY. tools/check holds every rule this repository
    enforces over its own tree, and this is the shortest path from a session to
    an answer on a PowerShell host.

    WHY THE RULES ARE NOT SHELL ANY MORE. Each one used to be written twice, in
    sh and in PowerShell, because the default host here is Windows and a POSIX
    check cannot be assumed to run on it. Keeping the two in step needed a third
    check that ran both halves of every pair, and that was most of a gate taking
    about twelve minutes. One implementation that runs natively on either host
    has no halves to compare.

    THIS FILE AND ITS sh TWIN ARE THE ONE REMAINING PAIR, and they are three
    lines of the same errand rather than two implementations of a rule. There
    is nothing left for them to disagree about.

.NOTES
    Exit codes: 0 agreed, 1 disagreed, 2 could not run.
    Read the exit code from this process, unpiped.
#>
[CmdletBinding(PositionalBinding = $false)]
param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Rest)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-Tool {
    param([Parameter(Mandatory = $true)][string]$Name)
    $c = Get-Command $Name -CommandType Application, ExternalScript -ErrorAction SilentlyContinue
    if ($c) { return $c[0].Source }
    return ''
}

$git = Get-Tool 'git'
if (-not $git) { [Console]::Error.WriteLine('check: git not found'); exit 2 }
# THE REPOSITORY IS RESOLVED FROM THIS FILE, NOT FROM THE CALLER'S DIRECTORY,
# for the reason its sh twin states. Measured on 2026-09-17 from a git repository
# with no tools/check: this half ended 1 where the twin ended 2, because
# $ErrorActionPreference = 'Stop' made Push-Location's failure terminating and the
# exit 2 below was never reached. docs/HISTORY/record.md findings 3 and 34.
$repoRoot = (& $git -C $PSScriptRoot rev-parse --show-toplevel 2>$null)
if ($LASTEXITCODE -ne 0 -or -not $repoRoot) { [Console]::Error.WriteLine('check: not a git repository'); exit 2 }
$repoRoot = $repoRoot.Trim()

$go = Get-Tool 'go'
if (-not $go) {
    [Console]::Error.WriteLine('check: no Go toolchain on PATH, and the gate is a Go program.')
    [Console]::Error.WriteLine('  That is "could not run" rather than a pass: nothing was checked.')
    exit 2
}

# A CALLER THAT BUILT THE GATE ONCE HANDS IT DOWN, for the reason its sh twin
# states: check-twins.sh runs both halves of a pair at once.
$own = ''
if ($env:TOOLKIT_CHECK_BIN) {
    $bin = $env:TOOLKIT_CHECK_BIN
    if (-not (Test-Path -LiteralPath $bin -PathType Leaf)) {
        [Console]::Error.WriteLine("check: TOOLKIT_CHECK_BIN names $bin, which is not a file")
        exit 2
    }
}
else {
    $tmp = Join-Path $repoRoot '.tmp'
    if (-not (Test-Path -LiteralPath $tmp)) { $null = New-Item -ItemType Directory -Path $tmp -Force }
    $exe = $(if ($IsWindows -or $env:OS -eq 'Windows_NT') { '.exe' } else { '' })
    $bin = Join-Path $tmp ('check' + $exe)
    $own = Join-Path $tmp ('check.' + $PID + $exe)

    $checkDir = Join-Path $repoRoot 'tools/check'
    # TESTED RATHER THAN THROWN. Under $ErrorActionPreference = 'Stop' a
    # Push-Location onto a missing directory is a TERMINATING error, so pwsh ends 1
    # and the exit 2 this file documents is never reached - which merges "could not
    # run" into "a check failed", and is what this half did where its sh twin
    # answered 2. docs/HISTORY/record.md finding 34.
    if (-not (Test-Path -LiteralPath $checkDir -PathType Container)) {
        [Console]::Error.WriteLine("check: $checkDir does not exist, so the gate cannot be built")
        exit 2
    }
    # BUILT INTO A COPY OF ITS OWN, THEN MOVED OVER THE BINARY WHOLE, for the
    # reason its sh twin states.
    if (Test-Path -LiteralPath $bin -PathType Leaf) { Copy-Item -LiteralPath $bin -Destination $own -Force -ErrorAction SilentlyContinue }
    Push-Location $checkDir
    try { & $go build -o $own . }
    finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) {
        Remove-Item -LiteralPath $own -Force -ErrorAction SilentlyContinue
        [Console]::Error.WriteLine('check: the gate did not build')
        exit 2
    }
    # WINDOWS REFUSES TO REPLACE A BINARY THAT IS RUNNING, so a copy that could
    # not be moved runs where it is and goes when it ends.
    try { Move-Item -LiteralPath $own -Destination $bin -Force; $own = '' }
    catch { $bin = $own }
}

Push-Location $repoRoot
try {
    if ($Rest) { & $bin @Rest } else { & $bin }
    $code = $LASTEXITCODE
}
finally {
    Pop-Location
    if ($own) { Remove-Item -LiteralPath $own -Force -ErrorAction SilentlyContinue }
}
exit $code
