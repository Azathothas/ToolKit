function Invoke-ActionRun {
    if (-not $Name) { throw "Action Run requires -Name." }
    if ($null -eq $script:CommandBytes) {
        throw "Action Run requires -Command, -CommandFile or -CommandB64."
    }
    $distro = Resolve-DistroName -Requested $Name -FromImage ''
    if ((Get-WslDistroNames) -notcontains $distro) {
        throw "Distro '$distro' is not registered. Create it with -Action New."
    }
    if ($DryRun) {
        Write-DryRunPlan -Action 'Run' -DistroName $distro -Steps @(
            ("command    " + (Get-CommandPlanLine -DistroName $distro -RunAs $User)))
        return
    }
    $rc = 0
    Invoke-InDistro -DistroName $distro -RunAs $User -ScriptBytes $script:CommandBytes -ExitCode ([ref]$rc)
    exit $rc
}

function Invoke-ActionEnter {
    <#
      An interactive shell in an existing ephemeral distro.

      ⛔ IT SENDS NO COMMAND, and that is the whole difference from Run. No
      '--', no '/bin/sh -lc', and no base64 transport: wsl.exe is handed the
      distro and the user and nothing else, so the guest's login shell owns the
      terminal. Routing this through Invoke-InDistro would produce a shell
      reading a script, which is not an interactive session.

      ⛔ IT IS NOT BOUNDED BY -TimeoutSeconds either. A person sitting in a
      shell is not a wedged init, and a tool that kills their session after two
      minutes is broken.

      ⚠ The name is prefix-forced like every other action, so -Name
      podman-machine-default asks for 'eph-podman-machine-default' and is
      refused as unregistered. This action cannot reach a distro the script did
      not create.
    #>
    if (-not $Name) { throw "Action Enter requires -Name." }
    $distro = Resolve-DistroName -Requested $Name -FromImage ''
    if ((Get-WslDistroNames) -notcontains $distro) {
        throw ("Distro '$distro' is not registered. '-Action List' shows the ones that are, " +
               "and '-Action New' creates one.")
    }
    if ($DryRun) {
        Write-DryRunPlan -Action 'Enter' -DistroName $distro -Steps @(
            ("wsl.exe    " + (Get-WslExe) + " -d $distro -u $User"),
            'no command, no transport, no bound: the guest login shell would own the terminal')
        return
    }

    Write-Step "Attaching to '$distro' as '$User'. Leave it with exit, or Ctrl-D."
    $rc = 1
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        & (Get-WslExe) -d $distro -u $User
        if ($null -ne $LASTEXITCODE) { $rc = [int]$LASTEXITCODE }
    }
    finally { $ErrorActionPreference = $prev }
    exit $rc
}

function Invoke-ActionList {
    $all  = @(Get-WslDistroNames)
    $mine = @($all | Where-Object { $_.StartsWith($script:Prefix, [StringComparison]::Ordinal) })
    Write-Step "Ephemeral distros (prefix '$($script:Prefix)')"
    if ($mine.Count -eq 0) { Write-Host "  (none)" -ForegroundColor DarkGray }
    else { foreach ($m in $mine) { Write-Host "  $m" } }

    Write-Step "Other distros on this system -- never touched by this script"
    $others = @($all | Where-Object { -not $_.StartsWith($script:Prefix, [StringComparison]::Ordinal) })
    if ($others.Count -eq 0) { Write-Host "  (none)" -ForegroundColor DarkGray }
    else {
        foreach ($o in $others) {
            $tag = ''
            if (Test-ProtectedName -DistroName $o) { $tag = '   [PROTECTED]' }
            Write-Host ("  {0}{1}" -f $o, $tag) -ForegroundColor DarkGray
        }
    }

    Write-Step "Orphaned rootfs tarballs in $($script:BaseDir)"
    $orphans = @(Get-OrphanTarball)
    if ($orphans.Count -eq 0) { Write-Host "  (none)" -ForegroundColor DarkGray }
    else {
        foreach ($t in $orphans) {
            $line = "  {0}   {1:N1} MiB   written {2:yyyy-MM-dd HH:mm:ss}Z"
            Write-Host ($line -f $t.Name, ($t.Length / 1MB), $t.LastWriteTimeUtc)
        }
        $sum = ($orphans | Measure-Object -Property Length -Sum).Sum
        Write-Warn ("{0:N1} MiB total. Remove them with -Action Purge." -f ($sum / 1MB))
        Write-Warn "a New running right now also has a .tar here: check the time before purging."
    }
}

