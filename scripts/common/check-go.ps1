# check-go.ps1 - does the wsl-toolkit executable still build, vet and pass its
# own tests?
#
# THE POWERSHELL TWIN of check-go.sh, and it exists for the reason every twin
# here does: a POSIX sh check cannot be assumed to run on Windows, which is the
# default host for this repository. scripts/README.md carries the measurement.
#
# The defect it exists to catch is a repository that gained a compiled tool and
# left the gate reading only the PowerShell half of it. Before this file, a Go
# change that did not compile reached a commit and the first thing to notice was
# CI, or a release.
#
# WHAT IT CHECKS, and each one separately: gofmt, because an unformatted tree is
# a diff nobody can read; go vet, which catches the class a compiler does not;
# go build; and go test, the suite over the parts that decide what a caller
# sees.
#
# HARD RULE: THE MODULE IS NOT AT THE REPOSITORY ROOT and this resolves it rather
# than assuming. `go build ./...` from the root finds no module and exits 1,
# which reads as a broken build and is a wrong directory.
#
# NOTE: NO go ON PATH IS "COULD NOT RUN", NOT "PASS". Exit 2.
#
# Usage:
#   pwsh -NoProfile -File scripts/common/check-go.ps1
#   pwsh -NoProfile -File scripts/common/check-go.ps1 -Json
#
# Exit codes: 0 clean, 1 a step failed, 2 could not run.
# Read the exit code from this process, unpiped.
[CmdletBinding(PositionalBinding = $false)]
param([switch]$Json)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Write-Line { param([string]$Text) if (-not $Json) { Write-Output $Text } }

$repoRoot  = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$moduleDir = Join-Path $repoRoot ([IO.Path]::Combine('tools', 'windows', 'wsl-toolkit'))

function Exit-Cannot {
    param([string]$Reason)
    if ($Json) {
        Write-Output ([ordered]@{ schema = 'check-go/1'; ok = $false; reason = $Reason; steps = @() } | ConvertTo-Json -Depth 4 -Compress)
    }
    else { [Console]::Error.WriteLine("check-go: $Reason") }
    exit 2
}

if (-not (Test-Path -LiteralPath $moduleDir -PathType Container)) { Exit-Cannot 'no Go module at tools/windows/wsl-toolkit' }
if (-not (Test-Path -LiteralPath (Join-Path $moduleDir 'go.mod'))) { Exit-Cannot 'tools/windows/wsl-toolkit has no go.mod' }

# NOTE: Get-Command finds cmdlets, functions and aliases too. Filtering to
# Application is what makes this ask about an executable rather than about
# anything with that name.
$go = Get-Command 'go' -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $go) { Exit-Cannot 'go is not on PATH' }

# NOTE: A SCRATCH BUILD CACHE INSIDE THE TREE. The default cache is under the
# user's home, which a restricted process may not be able to write, and the
# failure arrives as "failed to initialize build cache" from a command that
# looks like a compiler problem. .tmp/ is gitignored.
$cache = Join-Path $repoRoot '.tmp\go-build'
$null = New-Item -ItemType Directory -Path $cache -Force
$env:GOCACHE = $cache
$env:GOFLAGS = '-mod=mod'

$steps = @()
$problems = 0
function Add-Step {
    param([string]$Name, [string]$Status, [string]$Detail = '')
    $script:steps += [ordered]@{ step = $Name; status = $Status }
    Write-Line ("  {0,-6} {1}{2}" -f $Status, $Name, $Detail)
    if ($Status -eq 'FAIL') { $script:problems++ }
}

Write-Line "check-go: $moduleDir"

Push-Location $moduleDir
try {
    $fmt = & $go.Source 'fmt' '-n' './...' 2>&1
    $null = $fmt
    $unformatted = & $go.Source 'run' 'cmd/gofmt' '-l' '.' 2>$null
    if ($LASTEXITCODE -ne 0) {
        # `go run cmd/gofmt` needs the module cache. gofmt beside the toolchain
        # is the one that is always there, so it is the fallback rather than the
        # first choice: the first choice needs no PATH entry of its own.
        $gofmt = Join-Path (Split-Path -Parent $go.Source) 'gofmt.exe'
        if (-not (Test-Path -LiteralPath $gofmt)) { $gofmt = Join-Path (Split-Path -Parent $go.Source) 'gofmt' }
        if (Test-Path -LiteralPath $gofmt) { $unformatted = & $gofmt '-l' '.' 2>$null }
        else { $unformatted = @() }
    }
    $unformatted = @($unformatted | Where-Object { "$_".Trim().Length -gt 0 })
    if ($unformatted.Count -gt 0) { Add-Step 'gofmt' 'FAIL' (' -- ' + ($unformatted -join ' ')) }
    else { Add-Step 'gofmt' 'ok' }

    $buildOut = & $go.Source 'build' './...' 2>&1
    if ($LASTEXITCODE -eq 0) { Add-Step 'build' 'ok' }
    else { Add-Step 'build' 'FAIL' (' -- ' + (($buildOut | Select-Object -First 3) -join ' ')) }

    $vetOut = & $go.Source 'vet' './...' 2>&1
    if ($LASTEXITCODE -eq 0) { Add-Step 'vet' 'ok' }
    else { Add-Step 'vet' 'FAIL' (' -- ' + (($vetOut | Select-Object -First 3) -join ' ')) }

    $testOut = & $go.Source 'test' './...' 2>&1
    if ($LASTEXITCODE -eq 0) { Add-Step 'test' 'ok' }
    else {
        # NOTE: -cmatch, not -match. PowerShell's -match is case-INSENSITIVE, so
        # 'FAIL' would match "0 failed" in a summary line and the name of the
        # case that actually failed would be lost exactly when it is needed.
        $lines = @($testOut | Where-Object { "$_" -cmatch '^(---|FAIL)' } | Select-Object -First 3)
        if ($lines.Count -eq 0) { $lines = @($testOut | Select-Object -First 3) }
        Add-Step 'test' 'FAIL' (' -- ' + ($lines -join ' '))
    }
}
finally { Pop-Location }

if ($Json) {
    # HARD RULE: [ordered], NOT @{}. A PowerShell hashtable has no order, so
    # ConvertTo-Json emits the keys in whatever order it happens to enumerate
    # them, and check-twins compares the two halves' answers as TEXT. The two
    # said the same thing in a different order and the twin check called it
    # drift, which is the check working: an unordered answer is one that can
    # change between runs on the same tree.
    Write-Output ([ordered]@{
        schema   = 'check-go/1'
        ok       = ($problems -eq 0)
        problems = $problems
        steps    = $steps
    } | ConvertTo-Json -Depth 4 -Compress)
}
elseif ($problems -eq 0) { Write-Output 'check-go: clean.' }
else { [Console]::Error.WriteLine("check-go FAILED: $problems step(s).") }

if ($problems -ne 0) { exit 1 }
exit 0
