<#
  shell-matrix.ps1 - prove text-tool writes the SAME BYTES from every Windows shell.

  THE HALF shell-matrix.sh CANNOT DO. That file covers the POSIX shells; this one
  covers PowerShell 7, Windows PowerShell 5.1 and cmd.exe, which quote nothing
  like them and are where a Windows agent actually runs.

  THE DIGEST IS COMPARED ACROSS HOSTS, not merely checked non-empty. The claim is
  that the payload survives, and a payload that is mangled the SAME way in three
  shells would pass any weaker test.

  Usage: pwsh -NoProfile -File shell-matrix.ps1 -Tool PATH [-Work DIR]

  Exit 0 when every host present produced one digest, 1 when any disagreed, and
  2 when fewer than two hosts were found, because one host has compared nothing.

  SPDX-License-Identifier: 0BSD
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Tool,
    [string]$Work = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not $Work) { $Work = Join-Path ([IO.Path]::GetTempPath()) 'text-tool-matrix-win' }
if (Test-Path -LiteralPath $Work) { Remove-Item -LiteralPath $Work -Recurse -Force }
$null = New-Item -ItemType Directory -Path $Work -Force

$tool = (Resolve-Path -LiteralPath $Tool).Path

# base64 OF THE PAYLOAD THAT BROKE A QUOTED HEREDOC: backticks, a dollar sign,
# double quotes, apostrophes and a backslash escape. Putting the raw bytes in
# this file would make THIS file the thing that has to survive quoting.
$b64 = 'YSBsaW5lIHdpdGggYGJhY2t0aWNrc2AsICRET0xMQVJTLCAicXVvdGVzIiwgJ2Fwb3N0cm9waGVzJyBhbmQgRDpcdG9vbHNceFxkKwo='

$script:First = ''
$script:FirstHost = ''
$script:Ran = 0
$script:Skipped = 0
$script:Failed = 0

function Test-Host {
    param([string]$Name, [string]$Exe, [string[]]$Prefix)

    $found = Get-Command $Exe -ErrorAction SilentlyContinue
    if (-not $found) {
        Write-Output ("  skip  {0,-12} not installed" -f $Name)
        $script:Skipped++
        return
    }
    $out = Join-Path $Work ($Name + '.txt')
    # A QUOTED PATH AT THE START OF A POWERSHELL COMMAND IS AN EXPRESSION, not
    # an invocation, and answers 'Unexpected token'. The call operator is what
    # makes it run. cmd needs no such thing, which is why this matrix found it:
    # cmd passed and both PowerShells failed on the SAME line.
    $amp = if ($Name -eq 'cmd') { '' } else { '& ' }
    $line = '{0}"{1}" write "{2}" --b64 {3}' -f $amp, $tool, $out, $b64
    # NOT $args. It is an automatic variable inside a function, which
    # docs/conventions/shell.md section 8 forbids for the reason it gives: it
    # silently swallows a parameter of that name, and names are
    # case-insensitive so $Args collides too. PSScriptAnalyzer says so, and
    # until 2026-09-17 the gate ran the analyzer over scripts/ alone and never
    # looked at this directory.
    $argv = @()
    foreach ($p in $Prefix) { $argv += $p }
    $argv += $line

    & $found.Source @argv | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Output ("  FAIL  {0,-12} the tool exited {1}" -f $Name, $LASTEXITCODE)
        $script:Failed++
        return
    }
    if (-not (Test-Path -LiteralPath $out)) {
        Write-Output ("  FAIL  {0,-12} wrote no file" -f $Name)
        $script:Failed++
        return
    }
    $d = (Get-FileHash -LiteralPath $out -Algorithm SHA256).Hash.ToLowerInvariant()
    $script:Ran++
    if (-not $script:First) {
        $script:First = $d
        $script:FirstHost = $Name
        Write-Output ("  ok    {0,-12} {1}" -f $Name, $d)
    }
    elseif ($d -eq $script:First) {
        Write-Output ("  ok    {0,-12} same bytes" -f $Name)
    }
    else {
        Write-Output ("  FAIL  {0,-12} {1}, and {2} gave {3}" -f $Name, $d, $script:FirstHost, $script:First)
        $script:Failed++
    }
}

Write-Output "text-tool shell matrix (Windows hosts): $tool"
Write-Output ''

# -NoProfile ON BOTH POWERSHELLS. Somebody else's profile runs before the
# command and can write to its output, and a matrix that measured a profile
# would be measuring the wrong machine.
Test-Host -Name 'pwsh' -Exe 'pwsh' -Prefix @('-NoProfile', '-Command')
Test-Host -Name 'powershell' -Exe 'powershell' -Prefix @('-NoProfile', '-Command')
Test-Host -Name 'cmd' -Exe 'cmd' -Prefix @('/c')

Write-Output ''
if ($script:Failed -gt 0) {
    Write-Output ("{0} host(s) ran, {1} skipped, {2} FAILED" -f $script:Ran, $script:Skipped, $script:Failed)
    exit 1
}
if ($script:Ran -lt 2) {
    Write-Output ("{0} host(s) ran and at least 2 are needed to compare anything" -f $script:Ran)
    exit 2
}
Write-Output ("{0} host(s) ran and agreed, {1} skipped, 0 failed" -f $script:Ran, $script:Skipped)
