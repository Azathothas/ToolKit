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
    13m20s. One implementation that runs natively on either host has no halves
    to compare.

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
$repoRoot = (& $git rev-parse --show-toplevel 2>$null)
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

Push-Location (Join-Path $repoRoot 'tools/check')
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
