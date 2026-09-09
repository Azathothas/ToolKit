<#
.SYNOPSIS
    Run every check this repository has over its own tree.

.DESCRIPTION
    ONE BINARY, AND THAT IS THE WHOLE CHANGE. tools/check holds every rule, so
    this is a wrapper rather than a runner: there is no list of checks here to
    fall out of step with the list there.

    -Fast IS GONE AND NOTHING WAS LOST. It existed to skip check-twins, which
    ran both halves of every sh/PowerShell pair and compared their answers, and
    which was most of a gate that took 13m20s on this host. The rules are one
    implementation now, so there are no halves to compare and no reason to skip
    anything.

.NOTES
    Exit codes: 0 agreed, 1 disagreed, 2 could not run.
    Read the exit code from this process, unpiped.
#>
[CmdletBinding(PositionalBinding = $false)]
param([switch]$Fast, [Parameter(ValueFromRemainingArguments = $true)][string[]]$Rest)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($Fast) {
    [Console]::Error.WriteLine('check-gate: -Fast no longer means anything.')
    [Console]::Error.WriteLine('  It skipped check-twins, which compared the sh and PowerShell halves of')
    [Console]::Error.WriteLine('  every rule. The rules are one Go program now and the full run is about')
    [Console]::Error.WriteLine('  30 seconds. Drop the flag.')
    exit 2
}

# The binary takes POSIX flags; -Json is what every other PowerShell entry
# point here takes, so it is translated rather than refused.
$forward = @()
foreach ($a in $Rest) {
    if ($a -match '^-{1,2}[Jj]son$') { $forward += '--json' } else { $forward += $a }
}

$entry = Join-Path $PSScriptRoot 'check.ps1'
if ($forward.Count -gt 0) { & $entry @forward } else { & $entry }
exit $LASTEXITCODE
