# --------------------------------------------------------------------------------------
# Process helpers
# --------------------------------------------------------------------------------------
function Invoke-Native {
    <#
      Run a native exe, capture merged stdout+stderr, throw on non-zero exit.
      $ErrorActionPreference is deliberately relaxed for the duration: with it set to
      'Stop', PowerShell 7.3+ turns native stderr captured via 2>&1 into a terminating
      NativeCommandError, which would misreport success as failure.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [string[]]$Arguments = @(),
        [switch]$IgnoreExitCode
    )
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $out  = & $FilePath @Arguments 2>&1
        $code = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $prev
    }
    if (-not $IgnoreExitCode -and $code -ne 0) {
        $joined = ($out | Out-String).Trim()
        throw "$([IO.Path]::GetFileName($FilePath)) $($Arguments -join ' ') failed (exit $code): $joined"
    }
    return $out
}

function Test-Interactive {
    # Read-Host blocks or throws when stdin is not a console; detect that up front.
    if (-not [Environment]::UserInteractive) { return $false }
    try { return -not [Console]::IsInputRedirected } catch { return $false }
}

function Confirm-Destructive {
    param(
        [Parameter(Mandatory = $true)][string]$Target,
        [Parameter(Mandatory = $true)][string]$Operation
    )
    if ($Force) { return $true }
    if (-not (Test-Interactive)) {
        Write-Warn "$Operation on '$Target' needs confirmation, but this session is non-interactive."
        Write-Warn "Re-run with -Force to proceed."
        return $false
    }
    $answer = Read-Host "$Operation on '$Target'? [y/N]"
    return ($answer -match '^(y|yes)$')
}

