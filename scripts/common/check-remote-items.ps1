<#
.SYNOPSIS
    what is open against this repository, and does it say anything that
    survives being checked?

.DESCRIPTION
    A WRAPPER. The tool is `repo remote-items` in tools/repo.

    The defect it exists to catch is a change accepted on the strength of its
    own description. A bot's pull request title says what it believes it is
    doing; a contributor's issue says what they believe is wrong. Both are
    claims, and both are usually right, which is what makes the wrong one
    expensive: nobody is looking by the hundredth bump.

    IT IS READ ONLY. It never merges, never closes, never comments, never
    approves. Deciding is the operator's.

    IT CANNOT TELL YOU WHETHER A CHANGE IS A GOOD IDEA. It checks the facts an
    item asserts about the world. Whether you want the change is a reading.

    AN UNREAD ITEM IS NOT A FAILED CHECK. Any repository with an open issue
    would otherwise be permanently red, which is how a check stops being read.

    -Json PUTS THE JSON DOCUMENT ON STDOUT AND NOTHING ELSE. The report still
    goes out, on stderr.

.NOTES
    Exit codes: 0 nothing open, or nothing open failed a check;
                1 an item's claim did not survive checking;
                2 could not run.
    Read the exit code from this process, unpiped.
#>
[CmdletBinding()]
param(
    [switch]$Json,
    [string]$Repo = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$forward = @('remote-items')
if ($Json) { $forward += '--json' }
if ($Repo) { $forward += @('--repo', $Repo) }

$entry = Join-Path $PSScriptRoot 'repo.ps1'
& $entry @forward
exit $LASTEXITCODE
