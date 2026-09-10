function Invoke-ActionPurge {
    $mine    = @(Get-WslDistroNames | Where-Object { $_.StartsWith($script:Prefix, [StringComparison]::Ordinal) })
    $orphans = @(Get-OrphanTarball)
    # ⛔ SNAPSHOTS ARE REPORTED AND NEVER REMOVED, and that is WSL-26's one
    # design question answered. Purge exists to collect what was LEFT BEHIND: an
    # interrupted New's rootfs, a distro nobody unregistered. A snapshot is the
    # one durable thing this tool makes on purpose, and a caller who believed it
    # was durable losing it to a routine cleanup is the worse failure by far.
    # They are named here, with the directory, so removing one is deliberate.
    $snaps = @(Get-Snapshot)
    $snapLine = {
        if ($snaps.Count -eq 0) { return }
        $ssum = ($snaps | Measure-Object -Property Length -Sum).Sum
        Write-Note ("  {0} snapshot(s), {1:N1} MiB, KEPT in {2}" -f $snaps.Count, ($ssum / 1MB), (Get-SnapshotDir))
        Write-Note '  Purge never removes those. Delete the file to remove one.'
    }

    if ($mine.Count -eq 0 -and $orphans.Count -eq 0) {
        Write-Ok "nothing to purge"
        & $snapLine
        return
    }

    $what = @()
    if ($mine.Count -gt 0) {
        Write-Step "Ephemeral distro(s): $($mine -join ', ')"
        $what += "$($mine.Count) distro(s)"
    }
    if ($orphans.Count -gt 0) {
        $sum = ($orphans | Measure-Object -Property Length -Sum).Sum
        Write-Step ("Orphaned rootfs tarball(s), {0:N1} MiB: {1}" -f ($sum / 1MB), (($orphans | ForEach-Object { $_.Name }) -join ', '))
        $what += ("{0} tarball(s)" -f $orphans.Count)
    }

    # ⛔ BEFORE THE CONFIRMATION AND BEFORE THE FIRST DELETION. A dry run that
    # asked for confirmation would be teaching a caller to answer yes to a
    # prompt that sometimes deletes and sometimes does not.
    if ($DryRun) {
        $steps = @()
        foreach ($d in $mine)    { $steps += ("unregister " + $d + ', and delete ' + (Join-Path $script:BaseDir $d)) }
        foreach ($t in $orphans) { $steps += ("delete     " + $t.FullName) }
        Write-DryRunPlan -Action 'Purge' -Steps $steps
        return
    }

    & $snapLine

    # ONE confirmation covering both classes. Two prompts over one -Force is how
    # somebody learns to pass -Force without reading either of them.
    if (-not (Confirm-Destructive -Target ($what -join ' and ') -Operation 'Purge')) { return }

    # Counted, not thrown on at the first failure. One stuck item must not hide
    # the state of the rest, so both loops finish and the tally decides the exit
    # code.
    $failed = 0
    foreach ($d in $mine) {
        try { Remove-EphemeralDistro -DistroName $d -SkipConfirm }
        catch { Write-Warn "skip ${d}: $($_.Exception.Message)"; $failed++ }
    }
    foreach ($t in $orphans) {
        # Through the SAME deletion as a distro disk, so the containment guard
        # covers both. A second removal path with its own checks is how one of
        # them ends up without any.
        try { Remove-PathWithRetry -Path $t.FullName -What 'orphaned rootfs tarball' }
        catch { Write-Warn "skip $($t.Name): $($_.Exception.Message)"; $failed++ }
    }

    # A Purge that could not remove something must NOT exit 0. Warning and
    # returning success is the defect WSL-04 took out of the single delete,
    # reappearing one level up in the loop that calls it.
    if ($failed -gt 0) {
        throw ("$failed of " + ($mine.Count + $orphans.Count) +
               " item(s) were NOT removed. Each is named in a warning above.")
    }
}

