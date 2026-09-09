<#
.SYNOPSIS
    only the five allowed characters, and not too many of them

.DESCRIPTION
    A WRAPPER, and the rule lives in tools/check. Every rule this repository
    enforces over its own tree is one Go program now: it runs natively on either
    host, so there is no second implementation to keep in step and no third
    check comparing them. scripts/common/check.ps1 is the entry point and this
    forwards to one named check of it.

.NOTES
    Exit codes: 0 agreed, 1 disagreed, 2 could not run.
    Read the exit code from this process, unpiped.
#>
[CmdletBinding(PositionalBinding = $false)]
param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Rest)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# The binary takes POSIX flags. A caller writing -Json here is writing what
# every other PowerShell entry point in this repository takes, so it is
# translated rather than refused.
$forward = @('markers')
foreach ($a in $Rest) {
    switch -Regex ($a) {
        '^-{1,2}[Jj]son$'   { $forward += '--json'; continue }
        '^-{1,2}[Pp]ublic$' { continue }
        default             { $forward += $a }
    }
}

$entry = Join-Path $PSScriptRoot 'check.ps1'
& $entry @forward
exit $LASTEXITCODE
