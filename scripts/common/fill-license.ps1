<#
.SYNOPSIS
    write LICENSE from a template, with the holder filled in.

.DESCRIPTION
    A WRAPPER. The tool is `repo license` in tools/repo, and this exists so the
    documented command keeps working on a PowerShell host.

    The defect it exists to catch is a licence file with a placeholder still in
    it, or worse, one whose copyright line was rewritten when it should not
    have been.

    A NAIVE "REPLACE THE COPYRIGHT LINE" SCRIPT CORRUPTS FIVE OF THESE TWELVE,
    and that is why the tool carries a table instead of a regex. The GPL, AGPL
    and LGPL texts open with the Free Software Foundation's copyright on the
    licence document itself; SPDX's ISC text is a licence instance carrying
    Internet Systems Consortium's own copyright; MPL-2.0, CC0-1.0 and Unlicense
    have no copyright line to fill at all.

    With no -Holder, it is read from git config. Nothing is invented: if git
    has no user.name either, it refuses rather than guessing.

.NOTES
    Exit codes: 0 written, 1 refused or a placeholder survived, 2 could not run.
    Read the exit code from this process, unpiped.
#>
# The parameters are DECLARED rather than swept up as remaining arguments, and
# that is not a style choice: CmdletBinding adds the common parameters, so an
# undeclared -Out binds to -OutVariable and PowerShell refuses the call as
# ambiguous before this script sees it. Declaring it wins over the prefix match.
[CmdletBinding()]
param(
    [string]$Id = '',
    [string]$Holder = '',
    [string]$Year = '',
    [string]$Out = 'LICENSE',
    [switch]$Force,
    [switch]$List,
    [switch]$Json
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$forward = @('license')
if ($Id)     { $forward += @('--id', $Id) }
if ($Holder) { $forward += @('--holder', $Holder) }
if ($Year)   { $forward += @('--year', $Year) }
if ($Out)    { $forward += @('--out', $Out) }
if ($Force)  { $forward += '--force' }
if ($List)   { $forward += '--list' }
if ($Json)   { $forward += '--json' }

$entry = Join-Path $PSScriptRoot 'repo.ps1'
& $entry @forward
exit $LASTEXITCODE
