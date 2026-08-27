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
# ── ⚠ A SKIPPED CHECK IS NOT A PASSED CHECK ─────────────────────────────────
#
# Some of these need a tool that is not everywhere: sh, jq, shellcheck,
# PSScriptAnalyzer. A gate that silently dropped one and still printed green
# would be the "step that exits 0 having done nothing it was asked to do" row in
# docs/conventions/forbidden-patterns.md. So a missing tool is SKIP, counted
# separately, named in the summary and carried in -Json as `skipped`.
#
# ── ⚠ -Fast, AND WHY IT IS A FLAG RATHER THAN THE DEFAULT ───────────────────
#
# Measured on one Windows 11 machine, 2026-08-27: the full run took 208s and
# check-twins was 171s of it, because that check runs both halves of every pair
# and there are ten pairs. That is the right price before a push and the wrong
# one before each of eleven commits, and a gate too slow to run is a gate that
# gets run once at the end.
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

# ── PSScriptAnalyzer, suppressed per rule with the reason ────────────────────
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

# ── the PowerShell half runs first, because it needs no sh ─────────────────
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

if (-not $sh) {
    # ⛔ Not a silent degrade. Everything below is a POSIX sh check, and saying
    # which ones did not run is the difference between a gate and a green badge.
    Add-Skip 'check-docs'          'no POSIX shell on this host'
    Add-Skip 'check-placeholders'  'no POSIX shell on this host'
    Add-Skip 'check-control-bytes' 'no POSIX shell on this host'
    Add-Skip 'check-record'        'no POSIX shell on this host'
    Add-Skip 'check-changelog'     'no POSIX shell on this host'
    Add-Skip 'check-no-secrets'    'no POSIX shell on this host'
    Add-Skip 'sh -n'               'no POSIX shell on this host'
    Add-Skip 'shellcheck'          'no POSIX shell on this host'
    Add-Skip 'doctor probe'        'no POSIX shell on this host'
    Add-Skip 'check-twins'         'no POSIX shell on this host; it runs both halves of every pair'
}
else {
    Invoke-Check -Name 'check-docs'          -FilePath $sh -Arguments @("$common/check-docs.sh")
    Invoke-Check -Name 'check-placeholders'  -FilePath $sh -Arguments @("$common/check-placeholders.sh")
    Invoke-Check -Name 'check-control-bytes' -FilePath $sh -Arguments @("$common/check-control-bytes.sh")
    Invoke-Check -Name 'check-record'        -FilePath $sh -Arguments @("$common/check-record.sh")
    Invoke-Check -Name 'check-no-secrets'    -FilePath $sh -Arguments @("$common/check-no-secrets.sh", '--public')
    Invoke-Check -Name 'check-changelog'     -FilePath $sh -Arguments @("$common/check-changelog.sh") -PassCodes @(0, 2)

    # Line endings, read from git's own answer rather than a second table.
    $eol = @(& git ls-files --eol | Where-Object { $_ -notmatch 'i/lf' -and $_ -notmatch 'i/-text' })
    if ($eol.Count -eq 0) { Add-Pass 'line-endings' }
    else {
        Add-Fail 'line-endings' 1
        if (-not $Json) { foreach ($l in $eol) { Write-Output "  | $l" } }
    }

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

    Invoke-Check -Name 'doctor probe' -FilePath $sh -Arguments @('scripts/doctor/doctor.sh', '--fast')

    # ⛔ THIS PAIR RUNS THIS SCRIPT. check-twins.sh compares both halves of
    # every twin and check-gate is one of them, so an unguarded call here is an
    # infinite recursion: gate runs twins runs gate runs twins. It hung for ten
    # minutes before this guard existed, which is how the guard came to exist.
    # check-twins.sh exports the same variable, so a session that starts from
    # there gets a gate one level deep rather than three.
    $jq = Get-Command jq -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($Fast) {
        Add-Skip 'check-twins' '-Fast was passed; it is 171s of a 208s run'
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

# ── report ────────────────────────────────────────────────────────────────
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
