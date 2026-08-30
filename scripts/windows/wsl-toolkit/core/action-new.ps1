function Invoke-ActionNew {
    if (-not $Image -and -not $Tarball) { throw "Action New requires -Image (e.g. alpine:3.22) or -Tarball <path>." }
    if ($Image -and $Tarball)           { throw "Pass either -Image or -Tarball, not both." }

    # Resolve-NewDistroName owns the collision: it retries a name this script
    # drew and refuses a name the caller gave. Both used to throw here.
    $distro = Resolve-NewDistroName -Requested $Name -FromImage $Image
    if (Test-ProtectedName -DistroName $distro) { throw "Refusing to create a distro named '$distro' (protected)." }

    $target  = Join-Path $script:BaseDir $distro
    Assert-InsideBaseDir -Path $target               # validate before we ever create it

    # ⛔ THE DRY RUN RETURNS BEFORE THE FIRST New-Item, which is the first thing
    # on this path that changes the machine. The name it prints carries a random
    # suffix drawn just now, so a real run draws a different one; that is said
    # on the line rather than covered up with a fake constant.
    if ($DryRun) {
        $steps = @()
        if ($Tarball) { $steps += "import     $Tarball" }
        else {
            $engine = Get-ContainerEngine
            $steps += ("engine     " + $(if ($engine) { $engine.Name + ' at ' + $engine.Path } else { 'NONE FOUND -- -Image would be refused' }))
            if ($engine) { try { $steps += ("platform   " + (Get-EnginePlatform -Engine $engine)) } catch { $steps += ("platform   unreadable: " + $_.Exception.Message) } }
            $steps += "pull       $Image"
            $steps += ("export     " + (Join-Path $script:BaseDir ("{0}.tar" -f $distro)))
        }
        $steps += "directory  $target"
        $steps += ("wsl.exe    --import " + $distro + ' ' + $target + ' <rootfs.tar> --version 2')
        if ($Systemd)  { $steps += 'systemd    /etc/wsl.conf written, then the distro restarted' }
        if ($OciEnv)   { $steps += 'ocienv     /etc/profile.d/10-oci-env.sh written from the image config' }
        $plan = Get-CommandPlanLine -DistroName $distro -RunAs $User
        if ($plan) { $steps += "command    $plan" }
        if ($Ephemeral) { $steps += 'ephemeral  the distro would then be unregistered and its disk deleted' }
        $steps += '⚠ the name above carries a random suffix drawn for this plan; a real run draws its own'
        Write-DryRunPlan -Action 'New' -DistroName $distro -Steps $steps
        return
    }

    $tarPath = $null
    $tempTar = $false
    # 0 means "nothing ran, nothing failed": with no -Command there is no inner
    # code to carry. Declared here so [ref]$rc has a variable to bind to, and so
    # the exit at the bottom of this function can read it whatever happened.
    $rc      = 0

    try {
        New-Item -ItemType Directory -Path $target -Force | Out-Null

        if ($Tarball) {
            if (-not (Test-Path -LiteralPath $Tarball)) { throw "Tarball not found: $Tarball" }
            $tarPath = (Resolve-Path -LiteralPath $Tarball).Path
        }
        else {
            $tarPath = Join-Path $script:BaseDir ("{0}.tar" -f $distro)
            $tempTar = $true
            Export-ImageRootfs -ImageRef $Image -OutFile $tarPath
        }

        Assert-EnoughDiskSpace -TarballPath $tarPath -TargetDir $target

        Write-Step "Importing as WSL2 distro '$distro'"
        $wsl = Get-WslExe
        Invoke-Native -FilePath $wsl -Arguments @('--import', $distro, $target, $tarPath, '--version', '2') | Out-Null

        # Smoke test: a distro whose /bin/sh does not run is useless. Fail loudly now.
        # The loop also absorbs a first-boot race: drvfs automount of /mnt/<drive> can lag
        # the first shell by a second or two, which otherwise makes the very first user
        # command fail on a path under /mnt/c for no visible reason.
        #
        # ⭐ IT ALSO PROVES THE COMMAND CHANNEL, because it goes through the
        # same transport every -Command does. A guest with no base64, or no
        # /dev/fd, fails HERE, at creation, with a message naming it, instead
        # of at some later Run whose exit code would look like the command's.
        # NOT A HERE-STRING, ON PURPOSE. docs/conventions/shell.md: Windows
        # PowerShell 5.1 mis-parses a here-string whose terminator arrives
        # with a bare LF, and this template's defence is to write none in a
        # .ps1 at all rather than to rely on .gitattributes reaching every
        # checkout. An array joined with a newline carries the same script
        # and no line ending can break it.
        #
        # ⭐ THE BRACKET AND THE DOUBLE QUOTE ON THE NOTE LINE ARE
        # DELIBERATE. That exact line is what WSL-12 was: it reached the
        # guest as syntax under Windows PowerShell 5.1, every -Action New
        # failed, and the fix then was to rewrite the payload inside an
        # alphabet nothing enforced. It is back, unchanged, because the
        # transport now carries it. If it ever breaks again, this line is
        # the one that says so, on the host it broke on.
        $probeScript = @(
            'echo __WSL_OK__',
            'for _ in 1 2 3 4 5 6 7 8 9 10; do',
            '    if [ -d /mnt/c ]; then break; fi',
            '    sleep 1',
            'done',
            'if [ ! -d /mnt/c ]; then echo "note: no /mnt/c (Windows drives not mounted)"; fi',
            'head -2 /etc/os-release 2>/dev/null || echo "os-release: n/a"'
        ) -join "`n"
        $probeRc = 0
        $probeText = Get-DistroOutput -DistroName $distro -RunAs 'root' `
            -ScriptBytes (ConvertTo-Utf8Bytes -Text $probeScript) -ExitCode ([ref]$probeRc) `
            -What 'the smoke probe'
        if ($probeText -notmatch '__WSL_OK__') {
            throw ("Distro imported but /bin/sh did not run, or this rootfs cannot carry a " +
                   "command: the channel needs base64 and /dev/fd inside the guest. " +
                   "The probe exited $probeRc. Output: $($probeText.Trim())")
        }
        Write-Ok "'$distro' is up"
        foreach ($l in ($probeText -split "`r?`n")) {
            $t = $l.Trim()
            if ($t -and $t -ne '__WSL_OK__') { Write-Host "    $t" -ForegroundColor DarkGray }
        }

        # Before -OciEnv and before the command, so both run under systemd when
        # it was asked for. The profile script is on disk and survives the
        # restart either way.
        if ($Systemd) { Enable-DistroSystemd -DistroName $distro }

        if ($OciEnv) {
            if ($Tarball) {
                Write-Warn "-OciEnv ignored: a rootfs tarball carries no OCI configuration."
            }
            else {
                Write-Step "Carrying the image's OCI configuration into the distro"
                $cfgEngine = Get-ContainerEngine
                if (-not $cfgEngine) { throw "-OciEnv needs the container engine that built this rootfs, and none is on PATH now." }
                $cfg = Get-ImageOciConfig -EnginePath $cfgEngine.Path -ImageRef $Image
                $ociBody = New-OciEnvScript -Config $cfg -ImageRef $Image
                Write-DistroFile -DistroName $distro -Path '/etc/profile.d/10-oci-env.sh' -Content $ociBody
                Write-Ok "wrote /etc/profile.d/10-oci-env.sh"
            }
        }

        if ($null -ne $script:CommandBytes) {
            Write-Step "Running command as '$User'"
            Invoke-InDistro -DistroName $distro -RunAs $User -ScriptBytes $script:CommandBytes -ExitCode ([ref]$rc)
            if ($rc -ne 0) { Write-Warn "command exited $rc" }
        }

        if (-not $Ephemeral) {
            Write-Host ""
            Write-Host "  Distro : $distro"        -ForegroundColor White
            Write-Host "  Disk   : $target"        -ForegroundColor White
            Write-Host "  Enter  : -Action Enter -Name $distro" -ForegroundColor White
            Write-Host "  Remove : -Action Remove -Name $distro -Force" -ForegroundColor White
        }
    }
    catch {
        Write-Warn "creation failed; rolling back"
        try {
            if ((Get-WslDistroNames) -contains $distro) {
                Assert-Removable -DistroName $distro
                # ⛔ --terminate FIRST, exactly as Remove-EphemeralDistro does.
                # A door sweep found the two paths disagreeing: this one went
                # straight to --unregister, which releases the disk
                # asynchronously and is the race Remove-PathWithRetry below
                # exists to survive. ⚠ That race has still never been
                # reproduced on this machine, so this is the two paths agreeing
                # rather than a measured bug fix. Two ways of doing one thing is
                # how one of them ends up wrong.
                Invoke-Native -FilePath (Get-WslExe) -Arguments @('--terminate', $distro) -IgnoreExitCode | Out-Null
                Invoke-Native -FilePath (Get-WslExe) -Arguments @('--unregister', $distro) -IgnoreExitCode | Out-Null
            }
            if (Test-Path -LiteralPath $target) {
                # Through the same helper as every other deletion. If it cannot
                # delete, it throws, the catch below reports the rollback as
                # incomplete, and the ORIGINAL error is still what gets rethrown.
                Remove-PathWithRetry -Path $target -What "partial disk for '$distro'"
            }
        }
        catch { Write-Warn "rollback incomplete: $($_.Exception.Message)" }
        throw
    }
    finally {
        if ($tempTar -and $tarPath -and (Test-Path -LiteralPath $tarPath)) {
            # ⛔ THROUGH THE SAME DELETION AS EVERYTHING ELSE. This was the
            # fourth deletion path and it had its own Remove-Item, no
            # containment guard and no read-back, while wsl-toolkit.md
            # claimed there was one deletion and every path reached it. A door
            # sweep found it; the claim was false for one commit.
            #
            # ⚠ Caught, not propagated. This runs in a finally, and a failure
            # here must not replace the real outcome of the action: a tarball
            # left behind is an orphan, which List reports and Purge removes.
            try { Remove-PathWithRetry -Path $tarPath -What 'temporary rootfs tarball' }
            catch { Write-Warn "could not remove the temp tarball: $($_.Exception.Message)" }
        }
    }

    # ⛔ The teardown is OUTSIDE the try on purpose. Inside it, a teardown that
    # could not delete the disk was reported as "creation failed; rolling back",
    # which is false in a way that sends the reader looking in the wrong place:
    # the distro was created, the command ran, and the only thing wrong is that
    # several gigabytes are still on disk. Out here, that failure arrives as
    # itself and the script exits 1 naming the path.
    if ($Ephemeral) {
        Write-Step "-Ephemeral set: tearing down '$distro'"
        Remove-EphemeralDistro -DistroName $distro -SkipConfirm
    }

    # After the teardown above, and after the finally has removed the temp
    # tarball. Both of those are the reason this is at the bottom of the
    # function rather than beside the command that produced the code.
    exit $rc
}

