# --------------------------------------------------------------------------------------
# Output helpers
# --------------------------------------------------------------------------------------
function Write-Step { param([string]$Message) Write-Host "==> $Message" -ForegroundColor Cyan }
function Write-Ok   { param([string]$Message) Write-Host "  * $Message" -ForegroundColor Green }
function Write-Warn { param([string]$Message) Write-Host "  ! $Message" -ForegroundColor Yellow }

function Write-Note {
    <#
      A line for a person, on STDERR, so stdout can carry a value.

      ⛔ Write-Host IS NOT ENOUGH FOR THAT AND THE REASON IS NOT OBVIOUS.
      In-process it writes to the information stream and a caller assigning the
      result sees only Write-Output, which is what makes it look correct. Run as
      a CHILD PROCESS, which is how this script is documented to be called, the
      host writes it to the real stdout and it merges with the value. Measured
      on 2026-08-29: `-Action HostAddress` captured in-process gave one address
      and the same call through `pwsh -File` gave nine lines with the address
      last. stderr is the only channel that behaves the same both ways.

      ⚠ Used by -Action HostAddress alone. Every other action's output IS the
      report, so moving it would only make it harder to read.
    #>
    param([string]$Message)
    [Console]::Error.WriteLine($Message)
}

