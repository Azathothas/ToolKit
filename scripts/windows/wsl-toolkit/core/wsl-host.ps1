# --------------------------------------------------------------------------------------
# WSL helpers
# --------------------------------------------------------------------------------------
function Get-WslExe {
    $cmd = Get-Command wsl.exe -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $fallback = Join-Path $env:WINDIR 'System32\wsl.exe'
    if (Test-Path -LiteralPath $fallback) { return $fallback }
    throw "wsl.exe not found. WSL2 is required."
}

function Get-WslDistroNames {
    <#
      Every distribution registered on this machine.

      ⛔ AN ENUMERATION THAT WAS REFUSED THROWS, and it used to return an empty
      list. `2>$null` discarded the reason and `-not $raw` folded "WSL said no
      distributions" together with "WSL would not answer me", so a process that
      cannot reach WSL was told the machine was empty and the command exited 0.
      An agent reading that answer concludes there is nothing here. WSL-48,
      issue 25, and it is the same defect class as issue 10: a refusal rendered
      as a successful empty result.

      ⚠ A machine with genuinely no distributions is a real answer and stays
      an empty list. The two are separated by the EXIT CODE and what came back
      on stderr, not by the emptiness of the output.
    #>
    $wsl  = Get-WslExe
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $err = $null
        $raw = & $wsl --list --quiet 2>&1 |
            ForEach-Object {
                if ($_ -is [Management.Automation.ErrorRecord]) { $err = "$($err)$($_.ToString())"; }
                else { $_ }
            }
        $code = $LASTEXITCODE
    }
    finally { $ErrorActionPreference = $prev }

    return (Resolve-DistroListing -ExitCode $code -Lines @($raw) -ErrorText $err)
}

function Resolve-DistroListing {
    <#
      The DECISION half of Get-WslDistroNames, separated so it can be proved
      without WSL.

      ⭐ A PURE FUNCTION OF THREE FACTS: the child's exit code, what it wrote,
      and what it wrote to stderr. It is split out because the defect lives
      entirely in the decision and not in the process call, and a case for it
      would otherwise need a fake wsl.exe on disk.

      ⛔ AN EMPTY LIST AND A REFUSAL ARE DIFFERENT ANSWERS. `2>$null` discarded
      the reason and `-not $raw` folded them together, so a process WSL refuses
      was told the machine had no distributions and the caller exited 0.
      WSL-48, issue 25.
    #>
    param(
        [Parameter(Mandatory = $true)][int]$ExitCode,
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][AllowNull()][object[]]$Lines,
        [AllowEmptyString()][AllowNull()][string]$ErrorText = ''
    )
    if ($ExitCode -ne 0) {
        $why = ("$ErrorText" -replace "`0", '').Trim()
        if (-not $why) { $why = "it exited $ExitCode and said nothing" }
        throw "could not list the distributions on this machine: $why"
    }
    $names = @()
    foreach ($line in @($Lines)) {
        # Belt and braces: strip NULs in case WSL_UTF8 is unsupported on this build.
        $clean = ("$line" -replace "`0", '').Trim()
        if ($clean) { $names += $clean }
    }
    # ⚠ A MACHINE WITH NO DISTRIBUTIONS IS A REAL ANSWER and stays an empty
    # list. Only the exit code separates it from a refusal.
    return $names
}

function Get-WslNetworkingMode {
    <#
      Which networking mode WSL is configured for, read from
      %USERPROFILE%\.wslconfig without starting anything.

      ⛔ A COMMENTED SETTING IS NOT A SETTING. Real .wslconfig files carry the
      alternatives commented out above the live one, which is how they are
      written and how Microsoft's own example is written. A parser that grepped
      for the key would find `#networkingMode=mirrored` and answer mirrored on a
      host running NAT, which is the wrong answer in the direction that costs an
      hour: 127.0.0.1 is a plausible address that never connects.

      ⚠ THE SECTION MATTERS. `[experimental]` carries keys with related names,
      and only `[wsl2]` sets this one.

      ⚠ LAST ONE WINS, because that is what an ini parser does and what WSL
      does. A file that sets the key twice has one live value and it is the
      second.

      With no file, or no key in it, the answer is `nat`: that is WSL's
      documented default, and `Source` says which of the two this was so a
      caller can tell a measured answer from an assumed one.
    #>
    $path = $null
    if (-not [string]::IsNullOrWhiteSpace($env:USERPROFILE)) {
        $candidate = Join-Path $env:USERPROFILE '.wslconfig'
        if (Test-Path -LiteralPath $candidate -PathType Leaf) { $path = $candidate }
    }
    if (-not $path) {
        return [pscustomobject]@{ Mode = 'nat'; Source = 'the WSL default, no .wslconfig'; Path = $null }
    }

    $section = ''
    $mode = ''
    foreach ($raw in @(Get-Content -LiteralPath $path -ErrorAction SilentlyContinue)) {
        $line = "$raw".Trim()
        if (-not $line) { continue }
        if ($line.StartsWith('#') -or $line.StartsWith(';')) { continue }
        if ($line -match '^\[(.+)\]$') { $section = $Matches[1].Trim().ToLowerInvariant(); continue }
        if ($section -ne 'wsl2') { continue }
        if ($line -match '^networkingMode\s*=\s*([^\s#;]+)') { $mode = $Matches[1].Trim().ToLowerInvariant() }
    }

    if (-not $mode) {
        return [pscustomobject]@{ Mode = 'nat'; Source = 'the WSL default, no key in .wslconfig'; Path = $path }
    }
    return [pscustomobject]@{ Mode = $mode; Source = '.wslconfig'; Path = $path }
}

