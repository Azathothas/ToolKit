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
    $wsl  = Get-WslExe
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try { $raw = & $wsl --list --quiet 2>$null } finally { $ErrorActionPreference = $prev }
    if (-not $raw) { return @() }
    $names = @()
    foreach ($line in $raw) {
        # Belt and braces: strip NULs in case WSL_UTF8 is unsupported on this build.
        $clean = ($line -replace "`0", '').Trim()
        if ($clean) { $names += $clean }
    }
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

