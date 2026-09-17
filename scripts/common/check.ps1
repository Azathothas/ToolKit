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
# exit 2 below was never reached. TODO/PROGRESS.md findings 3 and 34.
$repoRoot = (& $git -C $PSScriptRoot rev-parse --show-toplevel 2>$null)
if ($LASTEXITCODE -ne 0 -or -not $repoRoot) { [Console]::Error.WriteLine('check: not a git repository'); exit 2 }
$repoRoot = $repoRoot.Trim()

$go = Get-Tool 'go'
if (-not $go) {
    [Console]::Error.WriteLine('check: no Go toolchain on PATH, and the gate is a Go program.')
    [Console]::Error.WriteLine('  That is "could not run" rather than a pass: nothing was checked.')
    exit 2
}

$tmp = Join-Path $repoRoot '.tmp'
if (-not (Test-Path -LiteralPath $tmp)) { $null = New-Item -ItemType Directory -Path $tmp -Force }
$bin = Join-Path $tmp ('check' + $(if ($IsWindows -or $env:OS -eq 'Windows_NT') { '.exe' } else { '' }))

$checkDir = Join-Path $repoRoot 'tools/check'
# TESTED RATHER THAN THROWN. Under $ErrorActionPreference = 'Stop' a
# Push-Location onto a missing directory is a TERMINATING error, so pwsh ends 1
# and the exit 2 this file documents is never reached - which merges "could not
# run" into "a check failed", and is what this half did where its sh twin
# answered 2. TODO/PROGRESS.md finding 34.
if (-not (Test-Path -LiteralPath $checkDir -PathType Container)) {
    [Console]::Error.WriteLine("check: $checkDir does not exist, so the gate cannot be built")
    exit 2
}
Push-Location $checkDir
try { & $go build -o $bin . }
finally { Pop-Location }
if ($LASTEXITCODE -ne 0) { [Console]::Error.WriteLine('check: the gate did not build'); exit 2 }

Push-Location $repoRoot
try {
    if ($Rest) { & $bin @Rest } else { & $bin }
    $code = $LASTEXITCODE
}
finally { Pop-Location }
exit $code
