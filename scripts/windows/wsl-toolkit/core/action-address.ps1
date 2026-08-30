function Resolve-HostAddress {
    <#
      The address a distro reaches THIS host at, as a value, with nothing
      printed.

      ⭐ ONE RESOLUTION, TWO CALLERS. -Action HostAddress prints it and
      -ScriptArg expands @hostaddress with it. Splitting the lookup from the
      report is what keeps those two from drifting: a second copy of the mode
      table would be a second place for the mirrored branch to be wrong, and
      the wrong answer there is 127.0.0.1, which is a plausible address that
      never connects.

      ⛔ IT REFUSES RATHER THAN GUESSING. A mode it cannot resolve to one
      address throws with the candidates named. An address invented here is one
      a caller binds a fixture to and then debugs for an hour.
    #>
    $net = Get-WslNetworkingMode
    if ($net.Mode -eq 'mirrored') {
        return [pscustomobject]@{
            Address = '127.0.0.1'; Mode = $net.Mode; Source = $net.Source; Path = $net.Path; Interface = 'loopback'
        }
    }
    if ($net.Mode -ne 'nat') {
        # bridged, or something a later WSL adds. Both have more than one right
        # answer and this script has no way to choose between them.
        throw ("Networking mode is '$($net.Mode)', and this script can only answer for 'nat' " +
               "and 'mirrored'. In bridged mode the distro is on the LAN and reaches this host " +
               "at whichever host address is on that switch, which is a choice rather than a " +
               "lookup. Read it from inside a distro instead: " +
               "awk '`$2 == 00000000 { print `$3 }' /proc/net/route, little-endian hex.")
    }

    $addr = $null
    $ifname = ''
    foreach ($n in [Net.NetworkInformation.NetworkInterface]::GetAllNetworkInterfaces()) {
        # ⚠ MATCHED ON A PREFIX, NOT ON AN EXACT NAME. Windows has called this
        # adapter both 'vEthernet (WSL)' and 'vEthernet (WSL (Hyper-V
        # firewall))'; measured on 2026-08-29 this host has the second.
        if ($n.Name -notlike 'vEthernet (WSL*') { continue }
        if ($n.OperationalStatus -ne [Net.NetworkInformation.OperationalStatus]::Up) { continue }
        foreach ($u in $n.GetIPProperties().UnicastAddresses) {
            if ($u.Address.AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetwork) { continue }
            $addr = $u.Address.IPAddressToString
            $ifname = $n.Name
            break
        }
        if ($addr) { break }
    }

    if (-not $addr) {
        throw ("NAT mode, and no WSL network adapter is up on this host. It is created when the " +
               "WSL utility VM first starts, so this is what an answer looks like before anything " +
               "has run: start a distro and ask again. Nothing was created to find that out.")
    }

    return [pscustomobject]@{
        Address = $addr; Mode = $net.Mode; Source = $net.Source; Path = $net.Path; Interface = $ifname
    }
}

function Invoke-ActionHostAddress {
    <#
      The address a distro reaches THIS host at, printed without creating a
      distro to find out.

      ⭐ THE VALUE IS THE ONLY THING ON STDOUT. Every explanatory line goes
      through Write-Note, which is stderr, so both of these assign one address:

          $addr = & .\wsl-toolkit.ps1 -Action HostAddress
          $addr = pwsh -NoProfile -File .\wsl-toolkit.ps1 -Action HostAddress 2>$null

      ⚠ The two differ in what the OPERATOR sees, not in what the caller gets.
      In-process the notes reach the console directly and PowerShell's `2>`
      cannot suppress them; out of process they are ordinary stderr and it can.
      Measured both ways on 2026-08-29, and both gave `172.23.96.1`.

      ⛔ Write-Host WOULD NOT DO. See Write-Note: it is invisible in-process and
      lands on stdout out of process, so the two calls above would disagree.
      ⛔ Do not add a second Write-Output to this path.

      WHY IT EXISTS. A caller that wanted this had to create a distro, read
      /proc/net/route inside it and decode little-endian hex, which is a
      throwaway VM built to answer a question the host already knows. In
      mirrored mode the answer is 127.0.0.1 and the caller's branch disappears.

      ⛔ IT REFUSES RATHER THAN GUESSING. A mode it cannot resolve to one
      address exits 1 with the candidates named. An address invented here is
      one a caller binds a fixture to and then debugs for an hour.
    #>
    $net = Resolve-HostAddress
    Write-Note "==> WSL networking mode: $($net.Mode) (from $($net.Source))"
    if ($net.Path) { Write-Note "    $($net.Path)" }

    if ($net.Mode -eq 'mirrored') {
        Write-Note "  * mirrored mode: the distro and this host share the loopback address."
        Write-Note "    A host service on 127.0.0.1 is reachable from inside the distro."
        Write-Output $net.Address
        return
    }

    Write-Note "  * NAT mode: the distro reaches this host at $($net.Address), on '$($net.Interface)'."
    Write-Note "  ! A HOST SERVICE ON 127.0.0.1 IS NOT REACHABLE FROM THE DISTRO IN THIS MODE."
    Write-Note "    Bind it to $($net.Address), or to 0.0.0.0 if you accept the LAN as well."
    Write-Note "    The failure is silent: a fixture on loopback simply never receives a"
    Write-Note "    connection, and nothing on either side says why."
    Write-Note "    This address is assigned by WSL and changes. Read it, never record it."
    Write-Output $net.Address
}

