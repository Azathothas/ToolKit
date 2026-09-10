<#
.SYNOPSIS
    the sanctioned way to commit and push.

.DESCRIPTION
    A WRAPPER. The tool is `repo git-sync` in tools/repo.

    The defect it exists to catch is a rule that everybody agreed to and nobody
    enforces. docs/conventions/git.md states the identity rule and the
    attribution rule; before this existed the template documented both and
    enforced neither.

    WHAT IT MAKES MECHANICAL, and each one has cost a real session:

      1. Author AND committer are pinned per invocation. git commit --author
         sets only the author, and a commit can carry two different identities;
         the one shown in a log is not the one a checker reads.
      2. An AI-attribution line is REFUSED, never stripped. Silently rewriting
         somebody's commit message is worse than declining to commit it.
      3. A CI-skip marker is refused unless -NoCi was passed. A message that
         merely mentions one skips CI, because GitHub does not read the
         sentence around it.
      4. The body is read from a FILE, never from a shell string.

    NOTHING ABOUT THIS KNOWS WHO YOU ARE. The identity comes from -Name/-Email
    or from git config, and if neither has one it refuses rather than guessing.

    IT IS A HELPER, NOT A CHECK. It writes: that is its job. -Check is the
    read-only half.

    -Path TAKES A COMMA-SEPARATED LIST, and that is not a style choice. A .ps1
    reached through -File cannot have a parameter repeated, and its comma form
    binds one string rather than a list, so this splits its own value. An empty
    element is refused by name. -Gate does NOT split, because its values are
    commands and a command may contain a comma; through -File it takes one, and
    several need `repo.ps1 git-sync --gate A --gate B`.

.NOTES
    Exit codes: 0 done, 1 a rule was broken or a gate failed, 2 could not run.
    Read the exit code from this process, unpiped.
#>
[CmdletBinding()]
param(
    [string]$Message = '',
    [string]$BodyFile = '',
    [string]$Name = '',
    [string]$Email = '',
    [string]$Branch = '',
    [string[]]$Path = @(),
    [string[]]$Gate = @(),
    [switch]$NoPush,
    [switch]$PushOnly,
    [switch]$Check,
    [switch]$SkipGates,
    [switch]$NoCi,
    [switch]$Json
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$forward = @('git-sync')
if ($Message)  { $forward += @('--message', $Message) }
if ($BodyFile) { $forward += @('--body-file', $BodyFile) }
if ($Name)     { $forward += @('--name', $Name) }
if ($Email)    { $forward += @('--email', $Email) }
if ($Branch)   { $forward += @('--branch', $Branch) }
# ⛔ A LIST PARAMETER ON A .ps1 REACHED THROUGH -File CANNOT BE REPEATED, and
# a comma form binds ONE string. Measured on 2026-09-10 under PowerShell 7.6.5:
# `-Path a,b` arrives as the single value 'a,b', and `-Path a -Path b` is
# refused with "specified more than once". This wrapper is the sanctioned way to
# commit on Windows, and until this it could not name two files: the comma form
# was forwarded verbatim as one pathspec and `git add` refused it, which reads as
# a bad path rather than as a wrapper that cannot take a list.
# docs/conventions/forbidden-patterns.md states the remedy: a [string[]] on such
# a script SPLITS ITS OWN VALUE. TOOL-21.
foreach ($p in $Path) {
    foreach ($piece in ($p -split ',')) {
        $piece = $piece.Trim()
        # An empty element is a typo (a trailing comma, or two together), and
        # forwarding it would stage the whole repository, because `git add --`
        # with an empty pathspec means everything.
        # ⚠ Reported and exited, never thrown. A throw from a script body
        # renders as a PowerShell exception with a source extract, and this
        # file's own contract says 2 means "could not run".
        if (-not $piece) {
            [Console]::Error.WriteLine("git-sync.ps1: -Path has an empty element. It splits on commas, so '$p' is not a path.")
            exit 2
        }
        $forward += @('--path', $piece)
    }
}
# ⛔ -Gate IS DELIBERATELY NOT SPLIT. Its values are COMMANDS, and a command can
# contain a comma, so splitting one would corrupt it silently. That is the case
# forbidden-patterns.md calls "arbitrary text has no safe delimiter either".
# Through -File this therefore takes ONE gate; for several, call the tool
# directly: pwsh -File scripts/common/repo.ps1 git-sync --gate A --gate B
foreach ($g in $Gate) { $forward += @('--gate', $g) }
if ($NoPush)    { $forward += '--no-push' }
if ($PushOnly)  { $forward += '--push-only' }
if ($Check)     { $forward += '--check' }
if ($SkipGates) { $forward += '--skip-gates' }
if ($NoCi)      { $forward += '--no-ci' }
if ($Json)      { $forward += '--json' }

$entry = Join-Path $PSScriptRoot 'repo.ps1'
& $entry @forward
exit $LASTEXITCODE
