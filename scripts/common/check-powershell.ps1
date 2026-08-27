# check-powershell.ps1 - does every tracked .ps1 parse, and is PSScriptAnalyzer
# clean over scripts/ at Error and Warning?
#
# The defect this exists to catch is a PowerShell script that only fails when
# somebody runs it. A .ps1 with a syntax error is a file git will happily carry
# and no other check in this repository looks at: check-docs reads markdown,
# shellcheck reads sh, and a parse error in a .ps1 surfaces on the machine of
# whoever ran it next. CI has had both halves of this since the beginning; this
# is the same two assertions in one command a session can run locally.
#
# ── ⚠ THE ANALYZER IS OPTIONAL AND SAYING SO IS THE POINT ───────────────────
#
# PSScriptAnalyzer is a module, not part of PowerShell. On a machine without it
# this reports SKIPPED and exits 0, because "this host cannot run that one" is
# not a failure of the tree. ⛔ What it does NOT do is print a clean verdict it
# did not earn: the skip is named in the output and carried in -Json as
# `analyzerSkipped`, so a caller can tell a real pass from an absent tool.
#
# ⛔ It is never installed from here. A check that installs software is a check
# that changes the machine it is measuring, and this one runs before a commit.
# CI installs it explicitly, which is where the guaranteed coverage lives.
#
# Usage:
#   pwsh -NoProfile -File scripts/common/check-powershell.ps1
#   pwsh -NoProfile -File scripts/common/check-powershell.ps1 -Json
#
# Exit codes: 0 pass, 1 fail, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

[CmdletBinding()]
param(
    [switch]$Json
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Exit-With {
    param([Parameter(Mandatory = $true)][int]$Code, [Parameter(Mandatory = $true)][string]$Text)
    [Console]::Error.WriteLine("check-powershell: $Text")
    exit $Code
}

if (-not (Get-Command git -ErrorAction SilentlyContinue)) { Exit-With 2 'git not found' }

$root = (& git rev-parse --show-toplevel 2>$null)
if ($LASTEXITCODE -ne 0 -or -not $root) { Exit-With 2 'not a git repository' }
$root = ($root | Select-Object -First 1).Trim()

# The scope of a guard must not depend on who called it. check-record.sh carries
# the same rule and the same reason.
Set-Location -LiteralPath $root

$files = @(& git ls-files '*.ps1' | Where-Object { $_ })
if ($files.Count -eq 0) { Exit-With 2 'no tracked .ps1 files; nothing to check' }

# ── every tracked .ps1 parses ──────────────────────────────────────────────
# ⛔ Every tracked file, not a hardcoded list. A list is a thing somebody
# forgets to extend, and the script they forgot is the one that breaks.
$parseErrors = @()
foreach ($f in $files) {
    $full = (Resolve-Path -LiteralPath $f).Path
    $errs = $null
    $tokens = $null
    $null = [System.Management.Automation.Language.Parser]::ParseFile($full, [ref]$tokens, [ref]$errs)
    if ($errs -and $errs.Count -gt 0) {
        foreach ($e in $errs) {
            $parseErrors += ("{0}:{1}: {2}" -f $f, $e.Extent.StartLineNumber, $e.Message)
        }
    }
}

# ── PSScriptAnalyzer, if this machine has it ───────────────────────────────
$analyzerSkipped = $true
$analyzerReason = 'PSScriptAnalyzer is not installed on this host'
$findings = @()

$module = Get-Module -ListAvailable -Name PSScriptAnalyzer -ErrorAction SilentlyContinue
if ($module) {
    try {
        Import-Module PSScriptAnalyzer -ErrorAction Stop
        $findings = @(Invoke-ScriptAnalyzer -Path (Join-Path $root 'scripts') -Recurse -Severity Error, Warning)
        $analyzerSkipped = $false
        $analyzerReason = ''
    }
    catch {
        # ⚠ Present but unusable is a DIFFERENT fact from absent, and it is the
        # one somebody has to act on. It is reported as a skip with its reason
        # rather than as a clean run, which is the whole point of this block.
        $analyzerSkipped = $true
        $analyzerReason = "PSScriptAnalyzer is installed but did not load: $($_.Exception.Message)"
    }
}

$problems = $parseErrors.Count + $findings.Count

if ($Json) {
    $payload = [ordered]@{
        schema          = 'check-powershell/1'
        problems        = $problems
        files           = $files.Count
        parseErrors     = $parseErrors.Count
        analyzerSkipped = [bool]$analyzerSkipped
        analyzerIssues  = $findings.Count
    }
    Write-Output ($payload | ConvertTo-Json -Compress -Depth 4)
    if ($problems -gt 0) { exit 1 }
    exit 0
}

Write-Output "check-powershell: $($files.Count) tracked .ps1 file(s)"

if ($parseErrors.Count -gt 0) {
    Write-Output ''
    Write-Output "PARSE ERRORS ($($parseErrors.Count)):"
    foreach ($p in $parseErrors) { Write-Output "  $p" }
}
else {
    Write-Output '  ok    every tracked .ps1 parses'
}

if ($analyzerSkipped) {
    Write-Output "  SKIP  PSScriptAnalyzer -- $analyzerReason"
    Write-Output '        Install it with:  Install-Module PSScriptAnalyzer -Scope CurrentUser'
    Write-Output '        A skipped check is not a passed check. CI installs it and runs it.'
}
elseif ($findings.Count -gt 0) {
    Write-Output ''
    Write-Output "PSScriptAnalyzer, Error and Warning ($($findings.Count)):"
    $findings | Format-Table -AutoSize RuleName, Severity, ScriptName, Line, Message | Out-String | Write-Output
}
else {
    Write-Output '  ok    PSScriptAnalyzer clean over scripts/ at Error and Warning'
}

# ⭐ A FIXED LINE, LAST, FOR A CALLER TO READ. check-gate has to be able to
# tell "the analyzer ran and was clean" from "the analyzer was not here", and
# both of those exit 0. Without this line the gate reports a skipped analyzer as
# a passed check, which is the exact defect this file's header warns about and
# which it did, once, before the line existed.
#
# ⛔ Parse this, never the prose above it. docs/methodology/work-todo.md: a file
# that quotes a value another file measures has to be checkable, written as a
# fixed line rather than as a sentence that reads better.
$analyzerState = if ($analyzerSkipped) { 'skipped' }
                 elseif ($findings.Count -gt 0) { "issues:$($findings.Count)" }
                 else { 'clean' }
Write-Output ''
Write-Output "check-powershell: files=$($files.Count) parseErrors=$($parseErrors.Count) analyzer=$analyzerState"

if ($problems -gt 0) {
    Write-Output "check-powershell FAILED: $problems problem(s)."
    exit 1
}

exit 0
