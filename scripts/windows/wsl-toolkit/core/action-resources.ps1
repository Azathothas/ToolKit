function Get-DirectorySizeBytes {
    <#
      Bytes under a directory, or $null when it cannot be measured.

      $null is a THIRD answer and every caller treats it as one, the same way
      Get-VolumeFreeBytes does. A VHDX that is open, a path that vanished
      between the listing and the walk, and a permission refusal all land here,
      and none of them means "zero".
    #>
    param([Parameter(Mandatory = $true)][string]$Path)
    try {
        # ⛔ THE CONTAINMENT GUARD, ON A READ. It is not here to protect a
        # deletion; nothing here deletes. It is here because the path is built
        # from a distro name `wsl.exe` reported, and a name carrying a traversal
        # would send a recursive walk somewhere it has no business being. A door
        # sweep found this path had no guard while every writing path had two.
        Assert-InsideBaseDir -Path $Path
        if (-not (Test-Path -LiteralPath $Path)) { return $null }
        $sum = (Get-ChildItem -LiteralPath $Path -Recurse -File -Force -ErrorAction SilentlyContinue |
                Measure-Object -Property Length -Sum).Sum
        if ($null -eq $sum) { return [int64]0 }
        return [int64]$sum
    }
    catch { $null = $_; return $null }
}

function Get-EngineAnswer {
    <#
      Ask the container engine one read-only question, bounded, and return what
      it printed. An empty string means it had nothing to say OR that it failed;
      -ExitCode is how the caller tells those apart.

      ⛔ NOTHING HERE WRITES. Every argument list this is called with is a
      report: `system df`, `images`, `ps -a`, `volume ls`. -Action Resources
      prints removal commands and runs none of them, and this function is why
      that is a property of the code rather than a promise in a comment.

      ⚠ podman on Windows talks to a VM. A machine that is starting, stopping or
      wedged leaves the client waiting with nothing on either stream, so the
      call is bounded like every other question this script asks.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$EnginePath,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][ref]$ExitCode
    )
    $ExitCode.Value = 1
    $timedOut = $false
    $text = Invoke-BoundedProcess -FilePath $EnginePath -Arguments $Arguments `
        -TimeoutSeconds $script:TimeoutSeconds -ExitCode $ExitCode -TimedOut ([ref]$timedOut)
    if ($timedOut) {
        # ⛔ Reported, not thrown. A wedged engine must not stop the WSL half of
        # the report, which is the half this script actually owns.
        $ExitCode.Value = 124
        return ''
    }
    return $text
}

function Get-EngineUsage {
    <#
      What the container engine is holding, read only.

      ⚠ THE FIELD SEPARATOR IS A PIPE AND NOT A TAB, and that is not a style
      choice. ConvertTo-NativeArgumentString REFUSES an argument carrying a
      backslash, so a Go template written as `{{.Type}}\t{{.Size}}` cannot be
      passed at all. A pipe needs no escape and no shell sees it: this builds a
      ProcessStartInfo argument string, not a command line for a shell.
    #>
    param([Parameter(Mandatory = $true)]$Engine)

    $rc = 0
    $df = Get-EngineAnswer -EnginePath $Engine.Path -ExitCode ([ref]$rc) -Arguments @(
        'system', 'df', '--format', '{{.Type}}|{{.Total}}|{{.Active}}|{{.Size}}|{{.Reclaimable}}')
    if ($rc -ne 0) {
        return [pscustomobject]@{ Reachable = $false; Reason = $df.Trim(); Rows = @(); Dangling = -1; Unused = -1 }
    }

    $rows = @()
    foreach ($line in ($df -split "`r?`n")) {
        $t = $line.Trim()
        if (-not $t) { continue }
        $p = $t -split '\|'
        if ($p.Count -lt 5) { continue }      # a warning on stderr is not a row
        $rows += [pscustomobject]@{
            Type = $p[0]; Total = $p[1]; Active = $p[2]; Size = $p[3]; Reclaimable = $p[4]
        }
    }

    # Counted separately, because `system df` reports a size and not a count of
    # the things nothing is using.
    $dRc = 0
    $dang = Get-EngineAnswer -EnginePath $Engine.Path -ExitCode ([ref]$dRc) -Arguments @(
        'images', '--filter', 'dangling=true', '--format', '{{.ID}}')
    $dangling = if ($dRc -eq 0) { @($dang -split "`r?`n" | Where-Object { $_.Trim() }).Count } else { -1 }

    $vRc = 0
    $vol = Get-EngineAnswer -EnginePath $Engine.Path -ExitCode ([ref]$vRc) -Arguments @(
        'volume', 'ls', '--filter', 'dangling=true', '--format', '{{.Name}}')
    $unused = if ($vRc -eq 0) { @($vol -split "`r?`n" | Where-Object { $_.Trim() }).Count } else { -1 }

    return [pscustomobject]@{ Reachable = $true; Reason = ''; Rows = $rows; Dangling = $dangling; Unused = $unused }
}

function Invoke-ActionResources {
    <#
      What this machine is holding, and the commands that would free it.

      ⛔ IT OFFERS AND IT DOES NOT DO. Nothing here removes anything, and the
      cleanup commands are PRINTED rather than run, including the ones for
      distros this script created. The report is for an agent to hand to a
      person, and deciding to reclaim somebody's 30 GB of images is that
      person's call rather than a tool's.

      ⚠ MOST OF WHAT IT REPORTS IS NOT THIS SCRIPT'S. The story that asked for
      it is an agent finding hundreds of images and several orphaned volumes on
      a machine where this script had never been run: the engine is shared with
      everything else on the host. So the report separates what this script made
      from what it merely found, and says which is which on every line.

      ⚠ It cannot tell a leftover from something in use. A named volume with no
      container attached is not garbage: it is how somebody keeps data between
      runs. That is why the verdict is a count and a size rather than a
      recommendation.
    #>
    $all  = @(Get-WslDistroNames)
    $mine = @($all | Where-Object { $_.StartsWith($script:Prefix, [StringComparison]::Ordinal) })

    Write-Step "What this script made, under $($script:BaseDir)"
    $mineBytes = 0
    $unmeasured = 0
    if ($mine.Count -eq 0) {
        Write-Host "  (no ephemeral distros)" -ForegroundColor DarkGray
    }
    foreach ($d in $mine) {
        $size = Get-DirectorySizeBytes -Path (Join-Path $script:BaseDir $d)
        if ($null -eq $size) {
            $unmeasured++
            Write-Host ("  {0,-40} size could not be read" -f $d) -ForegroundColor Yellow
        }
        else {
            $mineBytes += $size
            Write-Host ("  {0,-40} {1,10:N1} MiB" -f $d, ($size / 1MB))
        }
    }

    $orphans = @(Get-OrphanTarball)
    $orphanBytes = 0
    if ($orphans.Count -gt 0) {
        $orphanBytes = ($orphans | Measure-Object -Property Length -Sum).Sum
        foreach ($t in $orphans) {
            Write-Host ("  {0,-40} {1,10:N1} MiB   rootfs tarball, written {2:yyyy-MM-dd HH:mm:ss}Z" -f `
                $t.Name, ($t.Length / 1MB), $t.LastWriteTimeUtc)
        }
        Write-Warn "a New running right now also has a .tar here: check the time before purging."
    }

    # ⛔ A dash rather than a number when something could not be measured. A
    # total that silently counts an unreadable directory as zero is a number
    # somebody acts on.
    if ($unmeasured -gt 0) {
        Write-Warn ("total not stated: $unmeasured director(y/ies) could not be measured. " +
                    "Measured so far: {0:N1} MiB" -f (($mineBytes + $orphanBytes) / 1MB))
    }
    else {
        Write-Ok ("{0:N1} MiB held by this script, across {1} distro(s) and {2} tarball(s)" -f `
            (($mineBytes + $orphanBytes) / 1MB), $mine.Count, $orphans.Count)
    }

    Write-Step "What else is registered with WSL, which this script never touches"
    $others = @($all | Where-Object { -not $_.StartsWith($script:Prefix, [StringComparison]::Ordinal) })
    if ($others.Count -eq 0) { Write-Host "  (none)" -ForegroundColor DarkGray }
    foreach ($o in $others) {
        $tag = if (Test-ProtectedName -DistroName $o) { '   [PROTECTED]' } else { '' }
        Write-Host ("  {0}{1}" -f $o, $tag) -ForegroundColor DarkGray
    }
    Write-Host "  their disks are wherever they were imported to, which this script does not know." -ForegroundColor DarkGray

    Write-Step "What the container engine is holding, which this script never made"
    $engine = Get-ContainerEngine
    if (-not $engine) {
        Write-Host "  (no podman or docker on this host)" -ForegroundColor DarkGray
    }
    else {
        Write-Host "  engine: $($engine.Name) ($($engine.Path))" -ForegroundColor DarkGray
        $usage = Get-EngineUsage -Engine $engine
        if (-not $usage.Reachable) {
            Write-Warn "the engine did not answer, so nothing about it is reported."
            Write-Warn "on Windows podman runs in its own VM: try  podman machine start"
            if ($usage.Reason) { Write-Host "    $($usage.Reason)" -ForegroundColor DarkGray }
        }
        else {
            Write-Host ("  {0,-15} {1,6} {2,7} {3,12}  {4}" -f 'TYPE', 'TOTAL', 'ACTIVE', 'SIZE', 'RECLAIMABLE')
            foreach ($r in $usage.Rows) {
                Write-Host ("  {0,-15} {1,6} {2,7} {3,12}  {4}" -f $r.Type, $r.Total, $r.Active, $r.Size, $r.Reclaimable)
            }
            if ($usage.Dangling -ge 0) { Write-Host ("  dangling images: {0}" -f $usage.Dangling) }
            if ($usage.Unused -ge 0)   { Write-Host ("  unused volumes:  {0}" -f $usage.Unused) }
            Write-Warn ("RECLAIMABLE is not the whole prize. This engine reports three rows " +
                        "from system df, Images, Containers and Local Volumes, and no build cache " +
                        "row, so a prune can free considerably more than the figure above.")
        }
    }

    Write-Step "The commands that would free it. NONE of them was run."
    Write-Host ""
    Write-Host "  # this script's own, and the only ones it will ever remove:" -ForegroundColor DarkGray
    Write-Host "  pwsh -NoProfile -File wsl-toolkit.ps1 -Action Purge -Force"
    Write-Host ""
    Write-Host "  # the engine's, and NOT this script's to run. Read them before pasting one:" -ForegroundColor DarkGray
    Write-Host "  podman system df                      # the numbers above, again"
    Write-Host "  podman image prune --force            # dangling images only"
    Write-Host "  podman volume prune --force           # volumes nothing references"
    Write-Host "  podman system prune -a --volumes --force"
    Write-Host ""
    Write-Warn "the last one removes every image no RUNNING container uses, which is not the"
    Write-Warn "same as unused: an image you pulled this morning goes too, and so does every"
    Write-Warn "named volume holding data somebody kept on purpose."
}

