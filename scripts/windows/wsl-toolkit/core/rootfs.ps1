# --------------------------------------------------------------------------------------
# Rootfs acquisition
# --------------------------------------------------------------------------------------
function Get-ContainerEngine {
    foreach ($exe in @('podman', 'docker')) {
        $cmd = Get-Command "$exe.exe" -ErrorAction SilentlyContinue
        if ($cmd) { return [pscustomobject]@{ Name = $exe; Path = $cmd.Source } }
    }
    $candidates = @(
        [pscustomobject]@{ Name = 'podman'; Path = (Join-Path $env:LOCALAPPDATA 'Programs\Podman\podman.exe') },
        [pscustomobject]@{ Name = 'podman'; Path = (Join-Path $env:ProgramFiles 'RedHat\Podman\podman.exe') },
        [pscustomobject]@{ Name = 'docker'; Path = (Join-Path $env:ProgramFiles 'Docker\Docker\resources\bin\docker.exe') }
    )
    foreach ($c in $candidates) {
        if (Test-Path -LiteralPath $c.Path) { return $c }
    }
    return $null
}

function ConvertTo-OciArch {
    <#
      Maps whatever a container engine calls the host architecture onto the name
      --platform wants. The two engines disagree with each other AND with the
      OCI names: podman info says 'amd64', docker info says 'x86_64', and
      --platform accepts only the first.

      An unrecognised value passes through lowercased rather than being rejected
      here. A riscv64 or ppc64le host is a legitimate answer this table has not
      been taught, and the caller validates the shape before using it; guessing
      a substitute would be worse than handing the engine a name it can refuse.
    #>
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Raw)
    $v = $Raw.Trim().ToLowerInvariant()
    switch ($v) {
        'x86_64'  { return 'amd64' }
        'x86-64'  { return 'amd64' }
        'aarch64' { return 'arm64' }
        'armv8l'  { return 'arm64' }
        'armv7l'  { return 'arm' }
        'armhf'   { return 'arm' }
        'armv6l'  { return 'arm' }
        'i386'    { return '386' }
        'i486'    { return '386' }
        'i586'    { return '386' }
        'i686'    { return '386' }
        'x86'     { return '386' }
        default   { return $v }
    }
}

function Get-EnginePlatform {
    <#
      The linux/ARCH string every pull and every create names, read from the
      engine rather than assumed.

      ⭐ ONE READ PATH. Export-ImageRootfs pins --platform with it and
      -Action Doctor reports it, and both go through here: an answer computed in
      two places is an answer that will one day be two answers, and this one
      decides what architecture of image lands on the machine.

      IT DOUBLES AS THE READINESS PROBE, and its answer is USED rather than
      discarded. A stopped podman machine otherwise yields a cryptic pull error
      with no hint that the VM is down.

      The two engines spell the field differently. podman has .Host.Arch and
      docker has .Architecture, and asking docker for .Host.Arch fails the whole
      probe, which reads as "docker is not responding" on a machine where docker
      is fine.
    #>
    param([Parameter(Mandatory = $true)]$Engine)
    $archField = '{{.Host.Arch}}'
    if ($Engine.Name -eq 'docker') { $archField = '{{.Architecture}}' }

    $rawArch = ''
    try {
        $probe = Invoke-Native -FilePath $Engine.Path -Arguments @('info', '--format', $archField)
        if ($probe) { $rawArch = ($probe | Select-Object -Last 1).ToString().Trim() }
    }
    catch {
        throw ("$($Engine.Name) is installed but not responding. If you use podman on Windows, " +
               "start its VM with:  podman machine start`nUnderlying error: $($_.Exception.Message)")
    }

    # Refusing here rather than pulling unqualified. An unqualified pull takes
    # whatever the shared local tag currently points at, and one earlier
    # --platform pull is enough to have repointed it: the rootfs then imports
    # and every binary in it fails to execute, which surfaces only as the smoke
    # test's "/bin/sh did not run" with no hint of the cause.
    $arch = ConvertTo-OciArch -Raw $rawArch
    if ($arch -notmatch '^[a-z0-9_]+$') {
        throw ("Could not read the host architecture from '$($Engine.Name) info --format " +
               "$archField'; it answered '$rawArch'. Refusing to pull without --platform, " +
               "because an unqualified pull can silently export the wrong architecture.")
    }
    return "linux/$arch"
}

function Export-ImageRootfs {
    <# Pull an OCI image and flatten it to a rootfs tarball. #>
    param(
        [Parameter(Mandatory = $true)][string]$ImageRef,
        [Parameter(Mandatory = $true)][string]$OutFile
    )
    $engine = Get-ContainerEngine
    if (-not $engine) {
        throw ("No container engine found. Install podman or docker, or pass -Tarball " +
               "with a rootfs archive instead.")
    }
    Write-Step "Engine: $($engine.Name) ($($engine.Path))"

    $platform = Get-EnginePlatform -Engine $engine
    Write-Step "Platform: $platform"

    Write-Step "Pulling $ImageRef"
    Invoke-Native -FilePath $engine.Path -Arguments @('pull', '--platform', $platform, $ImageRef) | Out-Null

    $cid = $null
    try {
        # 'create' materialises a container without running it; its filesystem is the rootfs.
        # Images with no CMD/ENTRYPOINT reject a bare create, so fall back to naming one.
        try {
            $cid = (Invoke-Native -FilePath $engine.Path -Arguments @('create', '--platform', $platform, $ImageRef) |
                    Select-Object -Last 1).ToString().Trim()
        }
        catch {
            Write-Warn "bare create failed; retrying with an explicit command"
            $cid = (Invoke-Native -FilePath $engine.Path -Arguments @('create', '--platform', $platform, $ImageRef, '/bin/sh') |
                    Select-Object -Last 1).ToString().Trim()
        }
        if ([string]::IsNullOrWhiteSpace($cid)) { throw "Container id was empty." }

        $short = $cid.Substring(0, [Math]::Min(12, $cid.Length))
        Write-Step "Exporting rootfs (container $short)"
        # -o is mandatory: PowerShell redirection corrupts binary streams.
        Invoke-Native -FilePath $engine.Path -Arguments @('export', '-o', $OutFile, $cid) | Out-Null
    }
    finally {
        if ($cid) {
            try { Invoke-Native -FilePath $engine.Path -Arguments @('rm', '-f', $cid) -IgnoreExitCode | Out-Null }
            catch { Write-Warn "could not remove temp container $cid" }
        }
    }

    if (-not (Test-Path -LiteralPath $OutFile)) { throw "Export produced no file at $OutFile" }
    $size = (Get-Item -LiteralPath $OutFile).Length
    if ($size -lt 1KB) { throw "Exported rootfs is implausibly small ($size bytes)." }
    Write-Ok ("rootfs: {0:N1} MiB" -f ($size / 1MB))
}

