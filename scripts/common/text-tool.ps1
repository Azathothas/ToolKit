<#
.SYNOPSIS
    Write and edit a file without the shell touching the payload.

.DESCRIPTION
    A WRAPPER. The tool is `text-tool` in tools/text-tool.

    WHAT IT IS FOR. A payload crossing a shell boundary loses its quoting
    SILENTLY: the file is written, nothing returns non-zero, and the damage is a
    substituted fragment in the middle of a long document.
    docs/conventions/shell.md section 1 carries the measurement.

    USE YOUR HARNESS'S OWN WRITE AND EDIT TOOLS FIRST. They put bytes on disk
    with no shell in the path at all. This is for a harness that has none, and
    for what those tools cannot express: a substitution whose match count you
    want asserted, a line range, or a file that is not valid UTF-8.

    FROM POWERSHELL, USE --b64 OR --from. PowerShell's native-command pipe
    APPENDS a trailing CRLF, measured on a 59-byte fixture that arrived as 61
    bytes. Base64 is [A-Za-z0-9+/=] and needs no quoting in any shell.

.EXAMPLE
    pwsh -NoProfile -File scripts/common/text-tool.ps1 write PATH --b64 BASE64

.EXAMPLE
    pwsh -NoProfile -File scripts/common/text-tool.ps1 edit PATH --replace FIND --text NEW --expect 1

.NOTES
    Exit codes: 0 it did it, 1 it refused, 2 it could not run.
    Read the exit code from this process, unpiped.
#>
[CmdletBinding(PositionalBinding = $false)]
param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Rest)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-Tool {
    param([Parameter(Mandatory = $true)][string]$Name)
    $c = Get-Command $Name -CommandType Application -ErrorAction SilentlyContinue
    if ($c) { return $c[0].Source }
    return ''
}

$git = Get-Tool 'git'
if (-not $git) { [Console]::Error.WriteLine('text: git not found'); exit 2 }
# THE REPOSITORY IS RESOLVED FROM THIS FILE, for the reason findings 3 and 34 state.
$repoRoot = (& $git -C $PSScriptRoot rev-parse --show-toplevel 2>$null)
if ($LASTEXITCODE -ne 0 -or -not $repoRoot) { [Console]::Error.WriteLine('text: not a git repository'); exit 2 }
$repoRoot = $repoRoot.Trim()

$go = Get-Tool 'go'
if (-not $go) {
    [Console]::Error.WriteLine('text: no Go toolchain on PATH, and this tool is a Go program.')
    [Console]::Error.WriteLine('  That is "could not run" rather than a pass: nothing was done.')
    exit 2
}

$tmp = Join-Path $repoRoot '.tmp'
if (-not (Test-Path -LiteralPath $tmp)) { $null = New-Item -ItemType Directory -Path $tmp -Force }
$bin = Join-Path $tmp ('text-tool' + $(if ($IsWindows -or $env:OS -eq 'Windows_NT') { '.exe' } else { '' }))

$toolDir = Join-Path $repoRoot 'tools/text-tool'
# TESTED RATHER THAN THROWN, so "could not run" stays 2 and does not become 1.
if (-not (Test-Path -LiteralPath $toolDir -PathType Container)) {
    [Console]::Error.WriteLine("text: $toolDir does not exist, so the tool cannot be built")
    exit 2
}
Push-Location $toolDir
try { & $go build -o $bin . }
finally { Pop-Location }
if ($LASTEXITCODE -ne 0) { [Console]::Error.WriteLine('text: the tool did not build'); exit 2 }

# THE CALLER'S DIRECTORY IS KEPT, unlike the other wrappers here. A path this
# tool is given is the caller's own.
& $bin @Rest
exit $LASTEXITCODE
