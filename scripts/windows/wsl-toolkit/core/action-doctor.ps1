function Invoke-ActionDoctor {
    <#
      What this host can and cannot do, before anything is created.

      ⭐ THE DEFECT IT EXISTS FOR IS A CRYPTIC FAILURE HALFWAY IN. Without it,
      "the podman machine is not running" arrives as a pull error, "WSL is set
      to mirrored networking" arrives as a guest that cannot reach the host, and
      "this console cannot print UTF-8" arrives as mojibake in somebody's log.
      Each of those is knowable in under a second and none of them was said.

      ⛔ IT IS READ-ONLY AND IT CREATES NOTHING. No distro, no pull, no import,
      no file. Every row is a question already answerable from this host.

      ⭐ EVERY ROW SAYS HOW IT WAS OBTAINED. `obs` was read from an interface,
      `der` was computed from readings, `abs` means this machine cannot answer
      at all. A row that could not be measured says so instead of carrying a
      number nobody took, and an absent answer is the tool refusing to
      fabricate rather than a failure of the tool.
    #>
    $inv = [Globalization.CultureInfo]::InvariantCulture
    $script:DoctorRows = @()
    $add = {
        param([string]$Name, [string]$Prov, [string]$Value)
        $script:DoctorRows += [pscustomobject]@{ Name = $Name; Prov = $Prov; Value = $Value }
    }

    & $add 'tool' 'obs' ("wsl-toolkit " + $script:ToolkitVersion)
    & $add 'powershell' 'obs' ($PSVersionTable.PSVersion.ToString() + ' (' + $PSVersionTable.PSEdition + ')')
    & $add 'os' 'obs' ([Environment]::OSVersion.VersionString)

    # -- wsl.exe, and whether it answers at all ------------------------------
    $wsl = $null
    try { $wsl = Get-WslExe } catch { $null = $_ }
    if (-not $wsl) { & $add 'wsl.exe' 'abs' 'not on PATH. Nothing this tool does can work without it.' }
    else {
        & $add 'wsl.exe' 'obs' $wsl
        $rc = 0; $timedOut = $false
        $ver = ''
        try {
            $ver = Invoke-BoundedProcess -Arguments @('--version') -TimeoutSeconds 15 -ExitCode ([ref]$rc) -TimedOut ([ref]$timedOut)
        }
        catch { $null = $_ }
        if ($timedOut) { & $add 'wsl version' 'abs' 'wsl --version did not answer inside 15s' }
        elseif ($ver) {
            $first = @($ver -split "`r?`n" | Where-Object { $_.Trim() } | Select-Object -First 1)
            & $add 'wsl version' 'obs' ("$first".Trim())
        }
        else { & $add 'wsl version' 'abs' 'wsl --version printed nothing' }

        $all = @()
        try { $all = @(Get-WslDistroNames) } catch { $null = $_ }
        $mine = @($all | Where-Object { $_.StartsWith($script:Prefix, [StringComparison]::Ordinal) })
        & $add 'distros' 'obs' ("$($all.Count) registered, $($mine.Count) with the '$($script:Prefix)' prefix")
        $prot = @($all | Where-Object { Test-ProtectedName -DistroName $_ })
        & $add 'protected' 'obs' $(if ($prot.Count -eq 0) { 'none of the registered distros is on the protected list' } else { ($prot -join ', ') + ' -- this tool refuses to remove these' })
    }

    # -- networking, which is where a guest silently fails to reach the host --
    try {
        $net = Resolve-HostAddress
        & $add 'networkingMode' 'obs' ($net.Mode + '  (' + $net.Source + ')')
        & $add 'host address' 'der' ($net.Address + '  -- what a distro reaches this host at. Read, never recorded: WSL reassigns it.')
    }
    catch { & $add 'host address' 'abs' $_.Exception.Message }

    # -- the container engine, which only -Image needs ------------------------
    $engine = $null
    try { $engine = Get-ContainerEngine } catch { $null = $_ }
    if (-not $engine) {
        & $add 'container engine' 'abs' 'no podman or docker on PATH. -Image cannot work; -Tarball still can.'
    }
    else {
        & $add 'container engine' 'obs' ($engine.Name + '  (' + $engine.Path + ')')
        # ⭐ The SAME function Export-ImageRootfs pins --platform with. Asking a
        # second way here would let this row say one thing while a pull did
        # another, which is the one row a reader would trust and should not.
        try { & $add 'platform' 'obs' ((Get-EnginePlatform -Engine $engine) + '  -- named on every pull, never inherited') }
        catch { & $add 'platform' 'abs' $_.Exception.Message }
    }

    # -- where this tool keeps things, and what is left there -----------------
    & $add 'base directory' 'obs' $script:BaseDir
    $free = Get-VolumeFreeBytes -Path $script:BaseDir
    if ($null -eq $free) { & $add 'free space' 'abs' 'the volume did not report free space' }
    else { & $add 'free space' 'obs' ((($free / 1GB).ToString('N1', $inv)) + ' GiB, against a floor of ' + (($script:ImportSpaceFloor / 1MB).ToString('N0', $inv)) + ' MiB per import') }
    $orphans = @(Get-OrphanTarball)
    if ($orphans.Count -eq 0) { & $add 'orphan tarballs' 'obs' 'none' }
    else {
        $sum = ($orphans | Measure-Object -Property Length -Sum).Sum
        & $add 'orphan tarballs' 'obs' ("$($orphans.Count), " + (($sum / 1MB).ToString('N1', $inv)) + ' MiB. -Action Purge removes them.')
    }

    # -- the console, which decides whether a log is readable -----------------
    & $add 'stdout' 'obs' $(if ([Console]::IsOutputRedirected) { 'redirected, so the stream log will not colour it' } else { 'a console' })
    & $add 'output encoding' 'obs' ([Console]::OutputEncoding.WebName)

    # ⭐ MEASURED, NOT ASSUMED. The stream log renders %9f, and .NET's tick is
    # 100ns while the system clock's own resolution is coarser still. Printing
    # nine digits without saying which of them were measured is nine digits of
    # invented precision, so the figure is taken here, on this host, now.
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $seen = @{}
    $spins = 0
    while ($sw.ElapsedMilliseconds -lt 40 -and $spins -lt 2000000) {
        $seen[[DateTime]::UtcNow.Ticks] = $true
        $spins++
    }
    $distinct = @($seen.Keys | Sort-Object)
    if ($distinct.Count -lt 2) { & $add 'clock resolution' 'abs' 'could not be measured in the sampling window' }
    else {
        $gaps = @()
        for ($i = 1; $i -lt $distinct.Count; $i++) { $gaps += ($distinct[$i] - $distinct[$i - 1]) }
        # ⛔ REPORTED IN NANOSECONDS, because milliseconds rounds the answer to
        # zero on a host whose clock is finer than a millisecond and a zero
        # reads as "not measured". A .NET tick is 100 ns and that is the floor
        # this can ever report; the count of distinct readings is printed beside
        # it so a reader can see the sample the figure came from.
        $ns = [long](($gaps | Measure-Object -Minimum).Minimum) * 100
        & $add 'clock resolution' 'der' ($ns.ToString($inv) + ' ns smallest gap, over ' + $distinct.Count +
            ' distinct readings in 40 ms. ⚠ -TimestampFormat %9f pads below this rather than measuring below it.')
    }

    Write-Step "wsl-toolkit doctor -- read-only, and it created nothing"
    foreach ($r in $script:DoctorRows) {
        Write-Host ("  {0,-18} {1}  {2}" -f $r.Name, $r.Prov, $r.Value)
    }
    $absent = @($script:DoctorRows | Where-Object { $_.Prov -eq 'abs' })
    if ($absent.Count -eq 0) { Write-Ok 'every question this host was asked, it answered' }
    else {
        Write-Warn ("$($absent.Count) question(s) this host cannot answer: " + (($absent | ForEach-Object { $_.Name }) -join ', '))
        Write-Warn 'an absent row is this tool refusing to fabricate, not a failure of the tool.'
    }
}
