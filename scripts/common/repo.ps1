<#
.SYNOPSIS
    Build this repository's tool box and run one of its tools.

.DESCRIPTION
    THE TOOLS ARE ONE BINARY. tools/repo holds the ones that are not gate
    checks: a host probe, a commit path, a licence writer and two remote
    readers. This is the shortest path to one of them on a PowerShell host.

    WHY NOT tools/check. That binary holds the rules this repository enforces
    over its own tree, and check-gate runs all of them. Folding a commit path
    and a licence writer in would make the gate do things that are not checks.

    WHY THEY ARE NOT SHELL ANY MORE. Each was written twice, in sh and in
    PowerShell, because the default host here is Windows and a POSIX script
    cannot be assumed to run on it. Keeping the two in step needed a third check
    that ran both halves of every pair and compared their answers, and that
    check was most of a gate taking about twelve minutes. One implementation
    that runs natively on either host has no halves to compare.

    THIS FILE AND ITS sh TWIN ARE A WRAPPER, not an implementation. They are
    three lines of the same errand, and there is nothing left for them to
    disagree about.

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
if (-not $git) { [Console]::Error.WriteLine('repo: git not found'); exit 2 }
$repoRoot = (& $git rev-parse --show-toplevel 2>$null)
if ($LASTEXITCODE -ne 0 -or -not $repoRoot) { [Console]::Error.WriteLine('repo: not a git repository'); exit 2 }
$repoRoot = $repoRoot.Trim()

$go = Get-Tool 'go'
if (-not $go) {
    [Console]::Error.WriteLine('repo: no Go toolchain on PATH, and these tools are a Go program.')
    [Console]::Error.WriteLine('  That is "could not run" rather than a pass: nothing was done.')
    exit 2
}

$tmp = Join-Path $repoRoot '.tmp'
if (-not (Test-Path -LiteralPath $tmp)) { $null = New-Item -ItemType Directory -Path $tmp -Force }
$bin = Join-Path $tmp ('repo' + $(if ($IsWindows -or $env:OS -eq 'Windows_NT') { '.exe' } else { '' }))

Push-Location (Join-Path $repoRoot 'tools/repo')
try { & $go build -o $bin . }
finally { Pop-Location }
if ($LASTEXITCODE -ne 0) { [Console]::Error.WriteLine('repo: the tool box did not build'); exit 2 }

Push-Location $repoRoot
try {
    if ($Rest) { & $bin @Rest } else { & $bin }
    $code = $LASTEXITCODE
}
finally { Pop-Location }
exit $code
