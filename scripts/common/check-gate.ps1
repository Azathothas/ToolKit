# check-gate.ps1 - run every local gate this host can run, in one command.
#
# ⭐ THE TWIN OF check-gate.sh, and the one to prefer on Windows. It earns a
# twin by the rule in check-twins.sh: a native PowerShell session may have no
# POSIX shell at all, and "run the whole gate" is exactly the command somebody
# reaches for on a machine where that is true. Everything else under common/
# runs after the probe has reported and can assume sh; this cannot, because it
# is what a session runs first.
#
# The defect this exists to catch is a gate that was skipped because it was the
# ninth thing to remember. Part (a) of docs/methodology/gate.md is a LIST, and a
# list run by hand is run in the order somebody recalls it, missing whichever
# entry was added last.
#
# ⛔ IT IS NOT A SECOND SET OF RULES. Every line below shells out to a check
# that already exists and reads that check's own exit code. When this file and
# .github/workflows/ci.yml disagree about what runs, CI is the one that gates a
# push and this one is the defect.
#
# ⛔ IT RUNS EACH CHECK'S POWERSHELL TWIN, NOT ITS sh HALF. It used to run the
# sh half of all six twinned checks and skip every one of them when no POSIX
# shell was found, which is the host this file exists for. What still needs sh
# is what has no twin: `sh -n`, `shellcheck`, and check-twins itself.
#
# -- ⚠ A SKIPPED CHECK IS NOT A PASSED CHECK ---------------------------------
#
# Some of these need a tool that is not everywhere: sh, jq, shellcheck,
# PSScriptAnalyzer. A gate that silently dropped one and still printed green
# would be the "step that exits 0 having done nothing it was asked to do" row in
# docs/conventions/forbidden-patterns.md. So a missing tool is SKIP, counted
# separately, named in the summary and carried in -Json as `skipped`.
#
# -- ⚠ -Fast, AND WHY IT IS A FLAG RATHER THAN THE DEFAULT -------------------
#
# Measured on one Windows 11 Pro 26200 machine, 2026-08-29, with 13 twin pairs:
# the full run took 379s and check-twins ALONE took 270s, because that check
# runs both halves of every pair. Without it the sh half is 66s and the
# PowerShell half 41s. That is the right price before a push and the wrong one
# before each of a dozen commits, and a gate too slow to run is a gate that gets
# run once at the end.
#
# ⚠ The three numbers are separate runs on a machine doing other things, so
# they do not add up and are not meant to. Each carries its own conditions,
# which is what makes any of them comparable to a later one.
# ⚠ The figures carried before this were 208s and 171s over TEN pairs, taken
# on 2026-08-27. Two pairs were added on 2026-08-29 and the old numbers stopped
# describing this tree, which is why they were re-taken rather than adjusted.
#
# ⛔ -Fast SKIPS check-twins. It does not weaken anything else, it is reported
# as a SKIP like every other, and the summary says so. The full run is what a
# push is gated on.
#
# Usage:
#   pwsh -NoProfile -File scripts/common/check-gate.ps1
#   pwsh -NoProfile -File scripts/common/check-gate.ps1 -Fast
#   pwsh -NoProfile -File scripts/common/check-gate.ps1 -Json
#
#   pwsh -NoProfile -File scripts/common/git-sync.ps1 -Message "..." -BodyFile msg.txt `
#        -Gate "pwsh -NoProfile -File scripts/common/check-gate.ps1"
#
# Exit codes: 0 everything that ran passed, 1 something failed, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

# -- PSScriptAnalyzer, suppressed per rule with the reason --------------------
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '',
    Justification = 'Not used here. Declared so a future edit that reaches for Write-Host has to delete this line and think about it; every line of output below goes through Write-Output so -Json stays parseable.')]
[CmdletBinding()]
param(
    [switch]$Json,
    [switch]$Fast
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Exit-With {
    param([Parameter(Mandatory = $true)][int]$Code, [Parameter(Mandatory = $true)][string]$Text)
    [Console]::Error.WriteLine("check-gate: $Text")
    exit $Code
}

if (-not (Get-Command git -ErrorAction SilentlyContinue)) { Exit-With 2 'git not found' }
$root = (& git rev-parse --show-toplevel 2>$null)
if ($LASTEXITCODE -ne 0 -or -not $root) { Exit-With 2 'not a git repository' }
$root = ($root | Select-Object -First 1).Trim()
Set-Location -LiteralPath $root

$script:Passed = 0
$script:Failed = 0
$script:Skipped = 0
$script:FailedNames = @()
$script:SkippedNames = @()

function Write-Line { param([string]$Text) if (-not $Json) { Write-Output $Text } }

function Add-Pass { param([string]$Name) $script:Passed++; Write-Line "  ok    $Name" }
function Add-Fail {
    param([string]$Name, [int]$Code)
    $script:Failed++
    $script:FailedNames += $Name
    Write-Line "  FAIL  $Name (exit $Code)"
}
function Add-Skip {
    param([string]$Name, [string]$Reason)
    $script:Skipped++
    $script:SkippedNames += $Name
    Write-Line "  SKIP  $Name -- $Reason"
}

function Invoke-Check {
    <#
      Run one check, read its exit code from the process that produced it, and
      show its output only when it failed.

      -PassCodes exists for check-changelog, whose 2 means "could not run" and
      is the honest answer in a project with no CHANGELOG.md. Collapsing that
      into 0 with a blanket ignore would hide a genuine 1 as well.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string]$FilePath,
        [string[]]$Arguments = @(),
        [int[]]$PassCodes = @(0)
    )
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $out = & $FilePath @Arguments 2>&1
        $code = $LASTEXITCODE
    }
    finally { $ErrorActionPreference = $prev }

    if ($null -eq $code) { $code = 1 }
    if ($PassCodes -contains $code) { Add-Pass $Name; return }

    Add-Fail $Name $code
    if (-not $Json) {
        foreach ($l in ($out | Out-String) -split "`r?`n") {
            if ($l.Trim()) { Write-Output "  | $l" }
        }
    }
}

function Invoke-PsCheck {
    <#
      Run a check's POWERSHELL TWIN, through this same host.

      ⛔ THE TWIN IS PREFERRED HERE, NOT THE sh HALF, and getting that wrong is
      what this function exists to stop. This file used to shell out to the .sh
      half of every check and SKIP six of them outright when no POSIX shell was
      found, on precisely the machine the twins were written for. Its own header
      says it earns a twin because a native PowerShell session may have no sh;
      scripts/README.md says to run the .ps1 half on Windows. The runner was the
      one place not doing it.

      ⚠ THE HOST IS RE-ENTERED BY PATH, never by the name `pwsh`. A Windows
      PowerShell 5.1 session may have no `pwsh` on PATH at all, and a 5.1 caller
      has to get 5.1 back rather than whichever host happens to answer.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string]$Script,
        [string[]]$Arguments = @(),
        [int[]]$PassCodes = @(0)
    )
    $full = Join-Path $root $Script
    if (-not (Test-Path -LiteralPath $full)) {
        # ⛔ Named, not dropped. A check whose file is gone is a finding.
        Add-Skip $Name "$Script is missing"
        return
    }
    Invoke-Check -Name $Name -FilePath (Get-Process -Id $PID).Path `
        -Arguments (@('-NoProfile', '-File', $full) + $Arguments) -PassCodes $PassCodes
}

function Get-PosixShell {
    # ⚠ Get-Command finds cmdlets, functions and aliases too, so it is filtered
    # to a real executable. docs/conventions/shell.md section 8.
    foreach ($n in @('sh', 'sh.exe', 'bash', 'bash.exe')) {
        $c = Get-Command $n -CommandType Application -ErrorAction SilentlyContinue |
             Select-Object -First 1
        if ($c) { return $c.Source }
    }
    # Git for Windows ships one and does not always put it on PATH.
    foreach ($p in @("$env:ProgramFiles\Git\bin\sh.exe", "$env:ProgramFiles\Git\usr\bin\sh.exe")) {
        if ($p -and (Test-Path -LiteralPath $p)) { return $p }
    }
    return $null
}

Write-Line "check-gate: $root"
Write-Line ''

$sh = Get-PosixShell
$common = 'scripts/common'

# -- the PowerShell half runs first, because it needs no sh -----------------
# ⛔ SCORED AS TWO ENTRIES, because they can have different answers. The parse
# either ran or it did not; the analyzer is a module that may be absent, and
# check-powershell exits 0 either way. One verdict for both is how a skipped
# analyzer reads as a passed check, which it did once before the fixed status
# line existed.
$psCheck = Join-Path $root 'scripts/common/check-powershell.ps1'
if (-not (Test-Path -LiteralPath $psCheck)) {
    Add-Skip 'powershell parse' 'scripts/common/check-powershell.ps1 is missing'
    Add-Skip 'PSScriptAnalyzer'  'scripts/common/check-powershell.ps1 is missing'
}
else {
    $self = (Get-Process -Id $PID).Path
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $psOut = & $self -NoProfile -File $psCheck 2>&1
        $psRc = $LASTEXITCODE
    }
    finally { $ErrorActionPreference = $prev }
    if ($null -eq $psRc) { $psRc = 1 }

    $psText = ($psOut | Out-String)
    if ($psRc -eq 0) { Add-Pass 'powershell parse' }
    elseif ($psRc -eq 2) { Add-Skip 'powershell parse' 'the host reported it could not run' }
    else {
        Add-Fail 'powershell parse' $psRc
        if (-not $Json) {
            foreach ($l in $psText -split "`r?`n") { if ($l.Trim()) { Write-Output "  | $l" } }
        }
    }

    # The fixed last line, not the prose. check-powershell.ps1 documents it.
    if ($psText -match 'analyzer=skipped') { Add-Skip 'PSScriptAnalyzer' 'not installed on this host' }
    elseif ($psText -match 'analyzer=clean') { Add-Pass 'PSScriptAnalyzer' }
    elseif ($psText -match 'analyzer=issues') { Add-Fail 'PSScriptAnalyzer' 1 }
    else { Add-Skip 'PSScriptAnalyzer' 'check-powershell printed no analyzer status line' }
}

# -- every check that has a twin, run through the twin -----------------------
# ⛔ BOTH check-docs AND check-markers. The first reads markdown; the second
# owns the character rule over every tracked text file. check-docs was green on
# this tree while check-markers had 164 findings, all of them in scripts.
Invoke-PsCheck -Name 'check-docs'          -Script 'scripts/common/check-docs.ps1'
Invoke-PsCheck -Name 'check-markers'       -Script 'scripts/common/check-markers.ps1'
Invoke-PsCheck -Name 'check-one-home'      -Script 'scripts/common/check-one-home.ps1'
Invoke-PsCheck -Name 'check-placeholders'  -Script 'scripts/common/check-placeholders.ps1'
Invoke-PsCheck -Name 'check-control-bytes' -Script 'scripts/common/check-control-bytes.ps1'
Invoke-PsCheck -Name 'check-record'        -Script 'scripts/common/check-record.ps1'
Invoke-PsCheck -Name 'check-no-secrets'    -Script 'scripts/common/check-no-secrets.ps1' -Arguments @('-Public')
Invoke-PsCheck -Name 'check-changelog'     -Script 'scripts/common/check-changelog.ps1' -PassCodes @(0, 2)

# -- line endings, from git's own answer rather than a second table ----------
# ⛔ IT USED TO LIVE INSIDE THE sh BRANCH AND NEEDS NO SHELL. On a host without
# one it was neither run nor skipped, so it left the report entirely: the counts
# still added up, the name was simply absent, and nothing said so. That is the
# quietest way a gate loses a check.
$eol = @(& git ls-files --eol | Where-Object { $_ -notmatch 'i/lf' -and $_ -notmatch 'i/-text' })
if ($eol.Count -eq 0) { Add-Pass 'line-endings' }
else {
    Add-Fail 'line-endings' 1
    if (-not $Json) { foreach ($l in $eol) { Write-Output "  | $l" } }
}

# -- the probe, through its own twin -----------------------------------------
Invoke-PsCheck -Name 'doctor probe' -Script 'scripts/doctor/doctor.ps1' -Arguments @('-Fast')

# ⭐ THE ONE TEST IN THIS TREE, and it is in the gate because part (a) of
# docs/methodology/gate.md is the suite as well as the checks. It needs no WSL
# and no container engine: it runs wsl-toolkit.ps1's pure functions against a
# table of cases, which is where the timestamp renderer, the line splitter and
# the file channel decide what a caller sees.
Invoke-PsCheck -Name 'wsl-toolkit selftest' -Script 'scripts/windows/wsl-toolkit/selftest.ps1'

# ⛔ THE PRODUCT AGREES WITH ITS SOURCES. wsl-toolkit.ps1 is BUILT from the parts
# under scripts/windows/wsl-toolkit/{src,core,libs}, and it is tracked because a
# consumer fetching one raw URL cannot run a build step. That makes it the one
# file here that can silently stop matching what anybody wrote: an edit to a part
# that was never rebuilt, or an edit to the product that no part carries.
Invoke-PsCheck -Name 'wsl-toolkit bundle' -Script 'scripts/windows/wsl-toolkit/build.ps1' -Arguments @('-Check')

if (-not $sh) {
    # ⛔ Not a silent degrade. What is left below genuinely needs a POSIX shell,
    # and saying which ones did not run is the difference between a gate and a
    # green badge.
    Add-Skip 'sh -n'               'no POSIX shell on this host'
    Add-Skip 'shellcheck'          'no POSIX shell on this host'
    Add-Skip 'check-twins'         'no POSIX shell on this host; it runs both halves of every pair'
}
else {
    # Every tracked .sh parses.
    $shFiles = @(& git ls-files '*.sh' | Where-Object { $_ })
    $bad = @()
    foreach ($f in $shFiles) {
        $prev = $ErrorActionPreference
        $ErrorActionPreference = 'Continue'
        try { & $sh -n $f 2>&1 | Out-Null; $code = $LASTEXITCODE } finally { $ErrorActionPreference = $prev }
        if ($code -ne 0) { $bad += $f }
    }
    if ($bad.Count -eq 0) { Add-Pass 'sh -n' }
    else {
        Add-Fail 'sh -n' 1
        if (-not $Json) { foreach ($f in $bad) { Write-Output "  | parse FAIL $f" } }
    }

    $sc = Get-Command shellcheck -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $sc) { Add-Skip 'shellcheck' 'shellcheck is not on PATH' }
    else {
        $bad = @()
        foreach ($f in $shFiles) {
            $prev = $ErrorActionPreference
            $ErrorActionPreference = 'Continue'
            try { & $sc.Source -s sh $f 2>&1 | Out-Null; $code = $LASTEXITCODE } finally { $ErrorActionPreference = $prev }
            if ($code -ne 0) { $bad += $f }
        }
        if ($bad.Count -eq 0) { Add-Pass 'shellcheck' }
        else {
            Add-Fail 'shellcheck' 1
            if (-not $Json) { foreach ($f in $bad) { Write-Output "  | shellcheck $f" } }
        }
    }

    # ⛔ THIS PAIR RUNS THIS SCRIPT. check-twins.sh compares both halves of
    # every twin and check-gate is one of them, so an unguarded call here is an
    # infinite recursion: gate runs twins runs gate runs twins. It hung for ten
    # minutes before this guard existed, which is how the guard came to exist.
    # check-twins.sh exports the same variable, so a session that starts from
    # there gets a gate one level deep rather than three.
    $jq = Get-Command jq -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($Fast) {
        Add-Skip 'check-twins' '-Fast was passed; check-twins alone is 270s, measured 2026-08-29'
    }
    elseif ($env:CHECK_GATE_INNER -eq '1') {
        Add-Skip 'check-twins' 'already running inside check-twins; calling it here would recurse'
    }
    elseif (-not $jq) { Add-Skip 'check-twins' 'jq is not on PATH; it compares json' }
    else {
        $env:CHECK_GATE_INNER = '1'
        try { Invoke-Check -Name 'check-twins' -FilePath $sh -Arguments @("$common/check-twins.sh") }
        finally { $env:CHECK_GATE_INNER = $null }
    }
}

# -- report ----------------------------------------------------------------
$total = $script:Passed + $script:Failed + $script:Skipped

if ($Json) {
    $payload = [ordered]@{
        schema  = 'check-gate/1'
        total   = $total
        passed  = $script:Passed
        failed  = $script:Failed
        skipped = $script:Skipped
    }
    Write-Output ($payload | ConvertTo-Json -Compress -Depth 4)
    if ($script:Failed -gt 0) { exit 1 }
    exit 0
}

Write-Output ''
if ($script:Failed -gt 0) {
    Write-Output "GATE FAILED: $($script:Failed) of $total. Failed: $($script:FailedNames -join ' ')"
    if ($script:Skipped -gt 0) { Write-Output "Also skipped $($script:Skipped): $($script:SkippedNames -join ' ')" }
    exit 1
}

if ($script:Skipped -gt 0) {
    Write-Output "gate ok: $($script:Passed) passed, but $($script:Skipped) SKIPPED on this host: $($script:SkippedNames -join ' ')"
    Write-Output 'A skipped check is not a passed check. CI runs on two hosts that between'
    Write-Output 'them have every tool; that is where the coverage for these comes from.'
    exit 0
}

Write-Output "gate ok: all $($script:Passed) checks passed"
exit 0
