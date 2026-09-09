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
foreach ($p in $Path) { $forward += @('--path', $p) }
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
