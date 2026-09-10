function Get-SnapshotDir {
    <#
      Where a snapshot lives.

      ⭐ A SUBDIRECTORY, AND THAT IS THE ANSWER TO WSL-26'S ONE DESIGN QUESTION.
      Get-OrphanTarball enumerates `*.tar` in the base directory and nowhere
      else, so a snapshot kept beside a distro's own rootfs would be reported as
      an orphan and removed by the next Purge. A caller who thought a snapshot
      was durable would lose it, and the loss would look like the tool working.

      ⛔ DISTINGUISHED BY STRUCTURE, NOT BY A NAME PATTERN. A convention like
      `snap-*.tar` is one rename away from making every existing snapshot an
      orphan again, which is the failure the prelude's own comment describes for
      the base directory itself.
    #>
    if (-not $script:BaseDir) { throw 'No state directory: LOCALAPPDATA is unset and no -StateDir was given.' }
    return (Join-Path $script:BaseDir 'snapshots')
}

function Get-SnapshotTag {
    <#
      A tag that can be a file name, or a refusal saying why not.

      ⛔ VALIDATED BEFORE IT IS JOINED TO A PATH. A caller-supplied path
      component is how a tag becomes a write anywhere on the disk, and the
      reserved device names are refused by name because 'nul' silently discards
      everything written to it: a caller would believe they had a snapshot.
      docs/conventions/shell.md section 7.
    #>
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Tag)
    if ([string]::IsNullOrWhiteSpace($Tag)) { throw 'A snapshot tag is required. Pass -As <tag>.' }
    $t = $Tag.Trim()
    if ($t -notmatch '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$') {
        throw ("'$Tag' is not a usable snapshot tag. Use 1 to 64 characters of letters, digits, " +
               'dot, dash or underscore, starting with a letter or a digit.')
    }
    $reserved = @('con', 'prn', 'aux', 'nul') + (1..9 | ForEach-Object { "com$_" }) + (1..9 | ForEach-Object { "lpt$_" })
    if ($reserved -contains $t.ToLowerInvariant()) {
        throw ("'$t' is a Windows reserved device name, so a file of that name is not a file. " +
               'Pick another tag.')
    }
    return $t
}

function Get-SnapshotPath {
    param([Parameter(Mandatory = $true)][string]$Tag)
    return (Join-Path (Get-SnapshotDir) ((Get-SnapshotTag -Tag $Tag) + '.tar'))
}

function Get-Snapshot {
    <#
      Every snapshot on this machine, oldest name first. Reports rather than
      judges: List, Doctor and Purge all read this one function.
    #>
    $dir = if ($script:BaseDir) { Join-Path $script:BaseDir 'snapshots' } else { '' }
    if (-not $dir -or -not (Test-Path -LiteralPath $dir)) { return @() }
    return @(Get-ChildItem -LiteralPath $dir -Filter '*.tar' -File -ErrorAction SilentlyContinue |
             Sort-Object -Property Name)
}

function Invoke-ActionSnapshot {
    <#
      Export a registered distro back to a rootfs tarball that -Action New can
      import.

      ⭐ WHY THIS EXISTS. New always pulls, exports and imports, so a workload
      that spent fourteen minutes in an `apt` install pays for it again on the
      second run and on the third. Between "throw everything away" and "manage a
      long-lived distro by hand" there was nothing.

      ⭐ THE THIRD CALLER OF ONE PATH. Export-ImageRootfs writes a tarball and
      Invoke-ActionNew imports one; this writes one from a distro rather than
      from an image, and New reads it back through the -Tarball it already has.
      Nothing about the import path is duplicated here.

      ⚠ A SNAPSHOT CARRIES WHATEVER THE LAST COMMAND LEFT IN IT, including a
      credential a caller passed with -ScriptArg or wrote to a file. It is a
      plain tarball on this machine's disk and nothing in it is encrypted. Said
      here as well as on the page, because this is where the tag is named.
    #>
    if (-not $Name) { throw 'Action Snapshot requires -Name <distro>.' }
    $tag    = Get-SnapshotTag -Tag $As
    $distro = Resolve-DistroName -Requested $Name -FromImage ''

    # ⛔ THROUGH THE SAME OWNERSHIP CHECKS AS EVERY DESTRUCTIVE PATH, even though
    # this removes nothing. An export READS a distribution whole and writes it to
    # a file the caller keeps, so exporting one this tool did not create would be
    # this tool copying somebody else's disk out. Assert-Removable is where the
    # prefix rule and the protected-name list already live, and a second copy of
    # either is how one of them stops being applied.
    Assert-Removable -DistroName $distro

    $known = @(Get-WslDistroNames)
    if ($known -notcontains $distro) {
        throw ("'$distro' is not a registered distribution. -Action List names the ones this tool made.")
    }

    $dir  = Get-SnapshotDir
    $out  = Join-Path $dir ($tag + '.tar')
    $wsl  = Get-WslExe

    if ($DryRun) {
        Write-DryRunPlan -Action 'Snapshot' -DistroName $distro -Steps @(
            ("export     " + $distro + ' -> ' + $out)
        )
        return
    }

    if ((Test-Path -LiteralPath $out) -and -not $Force) {
        throw ("A snapshot tagged '$tag' already exists at $out. Pass -Force to replace it.")
    }

    New-Item -ItemType Directory -Path $dir -Force | Out-Null

    # ⚠ WRITTEN TO A TEMPORARY AND RENAMED, in the same directory. A killed
    # export otherwise leaves a truncated tarball under the tag, and the next
    # New -Tarball would import it: a rename across volumes is a copy and loses
    # the guarantee, which is why the temporary is a sibling.
    $temp = Join-Path $dir ('.' + [Guid]::NewGuid().ToString('N') + '.tmp')
    try {
        Write-Step "Exporting '$distro' as snapshot '$tag'"
        Invoke-Native -FilePath $wsl -Arguments @('--export', $distro, $temp) | Out-Null
        if (-not (Test-Path -LiteralPath $temp)) { throw "The export produced no file at $temp" }
        $size = (Get-Item -LiteralPath $temp).Length
        # ⛔ THE EFFECT IS READ BACK. wsl.exe --export has been seen to exit 0
        # over a file nobody could import; a size floor turns that into a
        # refusal here rather than into a failed import days later.
        if ($size -lt 1KB) { throw "The exported snapshot is implausibly small ($size bytes)." }
        Move-Item -LiteralPath $temp -Destination $out -Force
        Write-Ok ("snapshot '{0}': {1:N1} MiB at {2}" -f $tag, ($size / 1MB), $out)
        Write-Warn ('it carries whatever that distribution held, including anything a previous ' +
                    '-Command or -ScriptArg left in it.')
        Write-Note ("  reuse it with: -Action New -Tarball $tag")
    }
    finally {
        if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Force -ErrorAction SilentlyContinue }
    }
}
