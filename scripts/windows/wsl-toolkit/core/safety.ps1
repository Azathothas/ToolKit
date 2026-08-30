# --------------------------------------------------------------------------------------
# Naming and safety guards
# --------------------------------------------------------------------------------------
function Test-ProtectedName {
    param([Parameter(Mandatory = $true)][string]$DistroName)
    foreach ($p in $script:Protected) { if ($DistroName -ieq $p) { return $true } }
    return $false
}

function Assert-Removable {
    <# The single choke point for every destructive path. #>
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$DistroName)

    if ([string]::IsNullOrWhiteSpace($DistroName)) {
        throw "Refusing to remove: empty distro name."
    }
    if (Test-ProtectedName -DistroName $DistroName) {
        throw "REFUSING to remove protected distro '$DistroName'. This is a hard guard."
    }
    if (-not $DistroName.StartsWith($script:Prefix, [StringComparison]::Ordinal)) {
        throw ("REFUSING to remove '$DistroName': it does not start with '$($script:Prefix)'. " +
               "This script only removes distros it created.")
    }
}

function Assert-InsideBaseDir {
    <#
      Guarantees a directory slated for recursive deletion is a *strict* child of BaseDir.
      Without this, an empty or crafted distro name could resolve the target to BaseDir
      itself (or, with traversal, somewhere else entirely).
    #>
    param([Parameter(Mandatory = $true)][string]$Path)

    $baseFull = [IO.Path]::GetFullPath($script:BaseDir.TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar)
    $full     = [IO.Path]::GetFullPath($Path)

    if ($full.TrimEnd('\', '/') -ieq $baseFull.TrimEnd('\', '/')) {
        throw "REFUSING to delete the base directory itself ($full)."
    }
    if (-not $full.StartsWith($baseFull, [StringComparison]::OrdinalIgnoreCase)) {
        throw "REFUSING to delete '$full': outside $($script:BaseDir)."
    }
}

function Remove-PathWithRetry {
    <#
      THE one deletion in this script. Every path that removes something on disk
      goes through here, so the containment guard cannot be applied to one of
      them and forgotten on another.

      It deletes, then READS THE STATE BACK, and reports what is true rather
      than what was attempted. The predecessor printed "deleted DIR" beside a
      Remove-Item -ErrorAction SilentlyContinue, so a multi-gigabyte VHDX left
      behind read as a disk that had gone.

      The retry is not decoration. 'wsl --unregister' releases the VHDX
      asynchronously, so the delete immediately after it can lose the race
      against a handle that is about to be closed anyway.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$What,
        [string]$Remedy = '',
        [int]$Attempts = 5
    )
    Assert-InsideBaseDir -Path $Path          # ⛔ inside the helper, not beside it

    for ($i = 1; $i -le $Attempts; $i++) {
        # The read-back below is the verdict. This catch exists so a failed
        # attempt does not abort the retry loop; it is not where success is
        # decided, which is why it discards rather than reports.
        try { Remove-Item -LiteralPath $Path -Recurse -Force -ErrorAction Stop }
        catch { $null = $_ }

        if (-not (Test-Path -LiteralPath $Path)) {
            Write-Ok "deleted $Path"
            return
        }
        if ($i -lt $Attempts) { Start-Sleep -Milliseconds (200 * $i) }
    }

    $msg = "FAILED to delete the $What at '$Path'. It is STILL THERE after $Attempts attempts."
    if ($Remedy) { $msg = "$msg $Remedy" }
    throw ($msg + " Something is holding it open: WSL releases the disk asynchronously, and an " +
           "explorer window, a shell whose working directory is inside it, or an indexer will " +
           "each do it too.")
}

function ConvertTo-SafeName {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Raw)
    $s = $Raw.ToLowerInvariant() -replace '[^a-z0-9._-]', '-'
    $s = $s -replace '-{2,}', '-'
    $s = $s -replace '\.{2,}', '.'      # kill any ".." traversal component
    return $s.Trim('-', '.')
}

function New-DistroName {
    param([AllowEmptyString()][string]$FromImage)
    $stem = 'rootfs'
    if (-not [string]::IsNullOrWhiteSpace($FromImage)) { $stem = ConvertTo-SafeName -Raw $FromImage }
    if ([string]::IsNullOrWhiteSpace($stem)) { $stem = 'rootfs' }
    if ($stem.Length -gt 32) { $stem = $stem.Substring(0, 32).Trim('-', '.') }
    $suffix = -join ((48..57) + (97..122) | Get-Random -Count 4 | ForEach-Object { [char]$_ })
    return "$($script:Prefix)$stem-$suffix"
}

function Resolve-NewDistroName {
    <#
      The name for a distro about to be created, and the ONE place that decides
      whether a collision is an error or a retry. The two cases are genuinely
      different and conflating them was the defect:

        a name the CALLER gave      a collision is their answer being wrong,
                                    and silently using a different name would
                                    be worse than refusing. It throws, as before.
        a name this script DREW     a collision is a coin landing badly. Drawing
                                    again is the whole answer, and throwing made
                                    the user re-run a command that was correct.

      The space is 36^4 = 1,679,616, so this loop is expected to run once. That
      is exactly why it is worth having: a path that fires once in a million
      runs is a path nobody will debug when it does.

      ⚠ The registered list is read ONCE. Re-reading per attempt would cost a
      wsl.exe call per draw to defend against a distro appearing in the
      microseconds between two draws, and if that happens --import refuses
      anyway.
    #>
    param(
        [AllowEmptyString()][string]$Requested,
        [AllowEmptyString()][string]$FromImage,
        [int]$Attempts = 8
    )
    $existing = @(Get-WslDistroNames)

    if (-not [string]::IsNullOrWhiteSpace($Requested)) {
        $named = Resolve-DistroName -Requested $Requested -FromImage ''
        if ($existing -contains $named) {
            throw "Distro '$named' already exists. Choose another -Name or remove it first."
        }
        return $named
    }

    for ($i = 1; $i -le $Attempts; $i++) {
        $drawn = New-DistroName -FromImage $FromImage
        if ($existing -notcontains $drawn) { return $drawn }
        Write-Warn "generated name '$drawn' is already taken; drawing again ($i of $Attempts)"
    }

    throw ("Could not draw an unused distro name in $Attempts attempts. The suffix is four " +
           "characters from a 36-symbol alphabet, so this should not happen with fewer than " +
           "a few hundred thousand 'eph-' distros registered. Check 'wsl --list --quiet', or " +
           "pass -Name yourself.")
}

function Resolve-DistroName {
    param([AllowEmptyString()][string]$Requested, [AllowEmptyString()][string]$FromImage)
    if ([string]::IsNullOrWhiteSpace($Requested)) { return (New-DistroName -FromImage $FromImage) }
    $n = ConvertTo-SafeName -Raw $Requested
    if ([string]::IsNullOrWhiteSpace($n)) { throw "Name '$Requested' sanitises to nothing." }
    if (-not $n.StartsWith($script:Prefix, [StringComparison]::Ordinal)) { $n = "$($script:Prefix)$n" }
    return $n
}

