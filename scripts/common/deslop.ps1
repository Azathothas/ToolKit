<#
.SYNOPSIS
    which files in this tree address a reader as an agent?

.DESCRIPTION
    A WRAPPER. The tool is `repo deslop` in tools/repo, and this exists so the
    documented command keeps working on a PowerShell host. Its own header
    carries what the tool refuses and why.

    AN INVENTORY, NOT A GATE. It exits 0 whether it finds twenty agent-facing
    files or none, because in the repository that SHIPS them their presence is
    correct. Only -Apply changes anything, and only then can it fail.

    IT IS AIMED AT ANOTHER TREE, NOT AT THIS ONE. Every path it matches is one
    that Azathothas/TEMPLATE ships. Run with -Apply here and it removes THIS
    repository's own router and methodology, which are content it wants rather
    than content it regrets.

    IT NEVER TOUCHES HISTORY. No rebase, no amend, no filter, no force push.
    Rewriting published history un-publishes nothing, because every fork,
    mirror, cache and archive keeps its copy.

.NOTES
    Exit codes: 0 the inventory ran, or the removal succeeded;
                1 -Apply was asked for and could not be done safely;
                2 could not run.
    Read the exit code from this process, unpiped.
#>
# The parameters are DECLARED rather than swept up as remaining arguments, so a
# name that collides with one of CmdletBinding's common parameters binds here
# rather than being refused as ambiguous before this script sees it.
[CmdletBinding()]
param(
    [switch]$Json,
    [switch]$Apply,
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$forward = @('deslop')
if ($Json)   { $forward += '--json' }
if ($Apply)  { $forward += '--apply' }
if ($DryRun) { $forward += '--dry-run' }

$entry = Join-Path $PSScriptRoot 'repo.ps1'
& $entry @forward
exit $LASTEXITCODE
