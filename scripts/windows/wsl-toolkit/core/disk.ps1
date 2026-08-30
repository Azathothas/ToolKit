# --------------------------------------------------------------------------------------
# Actions
# --------------------------------------------------------------------------------------
function Get-VolumeFreeBytes {
    <#
      Free bytes on the volume a path lives on, or $null when that cannot be
      read. AvailableFreeSpace rather than TotalFreeSpace: a quota'd volume can
      have plenty of the second and none of the first, and the import fails on
      the first.

      $null is a THIRD answer and the caller treats it as one. "I could not
      measure" is not "there is room", and it is not a reason to refuse either.
    #>
    param([Parameter(Mandatory = $true)][string]$Path)
    try {
        $root = [IO.Path]::GetPathRoot([IO.Path]::GetFullPath($Path))
        if ([string]::IsNullOrWhiteSpace($root)) { return $null }
        $drive = New-Object IO.DriveInfo $root
        if (-not $drive.IsReady) { return $null }
        return [int64]$drive.AvailableFreeSpace
    }
    catch { $null = $_; return $null }
}

function Assert-EnoughDiskSpace {
    <#
      Running out of space midway through --import leaves a partial VHDX and a
      registered distro that does not work, which the user then has to unpick.
      Refusing before the import is recoverable; that is the whole trade.

      ⚠ THE FACTOR IS MEASURED, NOT ASSUMED. The entry that asked for this said
      an import needs "roughly twice the rootfs size" and flagged the two as an
      estimate. It is not a multiple at all. Measured on this machine on
      2026-08-27, VHDX size on disk against the rootfs tarball that produced it:

        alpine:3.22          8.2 MiB tar ->  76 MiB vhdx   9.27x
        python:3.13-alpine  45.4 MiB tar -> 140 MiB vhdx   3.08x
        debian:bookworm     74.3 MiB tar -> 172 MiB vhdx   2.31x
        ubuntu:24.04        76.9 MiB tar -> 172 MiB vhdx   2.24x

      The cost is dominated by a FIXED FLOOR, not by a multiple: an 8 MiB
      rootfs still costs 76 MiB. So the requirement is a floor plus a multiple,
      and both are set above every measurement rather than fitted to them. A
      preflight that is tight is a preflight that refuses a working import.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$TarballPath,
        [Parameter(Mandatory = $true)][string]$TargetDir
    )
    $tar  = (Get-Item -LiteralPath $TarballPath).Length
    $need = ($tar * $script:ImportSpaceFactor) + $script:ImportSpaceFloor
    $free = Get-VolumeFreeBytes -Path $TargetDir

    if ($null -eq $free) {
        # ⛔ Named, not silent. A preflight that skipped is not a preflight
        # that passed, and the import is still worth attempting.
        Write-Warn ("could not read free space for '$TargetDir'; importing without the preflight. " +
                    "If the volume is full, the import will leave a partial disk.")
        return
    }

    Write-Ok ("space: {0:N0} MiB needed, {1:N0} MiB free" -f ($need / 1MB), ($free / 1MB))
    if ($free -ge $need) { return }

    throw ("NOT ENOUGH DISK SPACE to import '$TargetDir'. " +
           ("Need about {0:N0} MiB and {1:N0} MiB is free" -f ($need / 1MB), ($free / 1MB)) +
           (" on the volume holding {0}." -f [IO.Path]::GetPathRoot([IO.Path]::GetFullPath($TargetDir))) +
           " Nothing has been imported and nothing is registered. Free some space, or point" +
           " LOCALAPPDATA at a volume that has it, and run this again.")
}

function Remove-EphemeralDistro {
    param(
        [Parameter(Mandatory = $true)][string]$DistroName,
        [switch]$SkipConfirm
    )
    Assert-Removable -DistroName $DistroName          # hard guard, always first

    if (-not $SkipConfirm) {
        if (-not (Confirm-Destructive -Target $DistroName -Operation 'Unregister WSL distro and DELETE its disk')) {
            Write-Warn "skipped $DistroName"
            return
        }
    }

    $wsl = Get-WslExe
    if ((Get-WslDistroNames) -contains $DistroName) {
        Invoke-Native -FilePath $wsl -Arguments @('--terminate', $DistroName) -IgnoreExitCode | Out-Null
        Invoke-Native -FilePath $wsl -Arguments @('--unregister', $DistroName) | Out-Null
        Write-Ok "unregistered $DistroName"
    }
    else {
        Write-Warn "$DistroName was not registered"
    }

    $dir = Join-Path $script:BaseDir $DistroName
    if (Test-Path -LiteralPath $dir) {
        $remedy = "Close it and re-run: -Action Remove -Name $DistroName -Force."
        Remove-PathWithRetry -Path $dir -What "disk for '$DistroName'" -Remedy $remedy
    }
}

function Get-OrphanTarball {
    <#
      Rootfs tarballs sitting loose in the base directory. New writes one there
      and removes it in a finally, and a finally does not run on every hard
      interrupt, so an interrupted run can leave several hundred MiB that
      nothing reported.

      IT CANNOT TELL AN ORPHAN FROM A RUNNING NEW, and it does not pretend to:
      a New that is executing right now has its tarball in exactly this place.
      That is why the report carries the last-written time instead of a verdict.
    #>
    if (-not (Test-Path -LiteralPath $script:BaseDir)) { return @() }
    return @(Get-ChildItem -LiteralPath $script:BaseDir -Filter '*.tar' -File -ErrorAction SilentlyContinue |
             Sort-Object -Property Name)
}

