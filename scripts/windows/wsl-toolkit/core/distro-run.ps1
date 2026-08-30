function Invoke-InDistroLogged {
    <#
      Run the caller's command with the stream log on: a stamp on every line, a
      heartbeat while there are none, and an optional bound on the whole thing.

      THE COST OF DOING THIS AT ALL, stated here because it is the one thing a
      caller can be surprised by. Relaying means the guest's stdout is a PIPE
      rather than whatever this process inherited, so an application that
      block-buffers when it is not writing to a terminal will buffer. Its lines
      then arrive late and carry the time THIS process received them.
      -NoTimestamps hands the handles straight through and is byte-exact.

      THE EXIT CODE IS THE COMMAND'S, unchanged. Nothing here decides it except
      -CommandTimeoutSeconds, which is opt-in, has no default, and reports 124
      the way coreutils' timeout does.

      THE STREAMS ARE READ BEFORE THE PROCESS IS WAITED ON. Waiting first
      deadlocks any child that fills a pipe buffer: the child blocks writing and
      the parent blocks waiting. Same ordering as Invoke-BoundedProcess, for the
      same reason.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$DistroName,
        [Parameter(Mandatory = $true)][string]$RunAs,
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][byte[]]$ScriptBytes,
        [Parameter(Mandatory = $true)][ref]$ExitCode
    )
    $ExitCode.Value = 1
    $line = ConvertTo-DistroScriptCommand -ScriptBytes $ScriptBytes -GuestPath (New-GuestScratchPath)

    $psi = New-Object Diagnostics.ProcessStartInfo
    $psi.FileName               = Get-WslExe
    $psi.Arguments              = ConvertTo-NativeArgumentString -Arguments @('-d', $DistroName, '-u', $RunAs, '--', '/bin/sh', '-lc', $line)
    $psi.UseShellExecute        = $false
    $psi.CreateNoWindow         = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError  = $true
    $psi.StandardOutputEncoding = [Text.Encoding]::UTF8
    $psi.StandardErrorEncoding  = [Text.Encoding]::UTF8

    $st = New-StreamLogState -DistroName $DistroName -Settings $script:LogSettings
    $st.Out = Open-StreamLogWriter -Which 'Out'
    $st.Err = Open-StreamLogWriter -Which 'Err'
    # ⛔ THE SINKS ARE OPENED BEFORE THE PROCESS STARTS. A reserved device name
    # or an unwritable directory has to be refused while nothing has happened
    # yet; discovering it after the guest has run for ten minutes means the run
    # is over and the log the caller asked for does not exist.
    if ($script:LogSettings.TextPath) {
        $st.Text = New-TextSink -Path $script:LogSettings.TextPath -Overwrite:$script:LogSettings.TextOverwrite
    }
    if ($script:LogSettings.EventPath) {
        $st.Events = New-EventSink -Path $script:LogSettings.EventPath -DistroName $DistroName
    }

    $proc = [Diagnostics.Process]::Start($psi)
    $hitDeadline = $false
    try {
        $streams = @(
            [pscustomobject]@{ Tag = 'out'; Reader = $proc.StandardOutput; Buffer = [char[]]::new(8192); Task = $null; Pending = ''; Since = [timespan]::Zero; Done = $false },
            [pscustomobject]@{ Tag = 'err'; Reader = $proc.StandardError;  Buffer = [char[]]::new(8192); Task = $null; Pending = ''; Since = [timespan]::Zero; Done = $false }
        )
        $pollMs   = 250
        $flush    = [timespan]::FromMilliseconds($script:StreamFlushMs)
        $tick     = if ($script:TickSeconds -gt 0) { [timespan]::FromSeconds($script:TickSeconds) } else { [timespan]::Zero }
        $deadline = if ($script:CommandTimeoutSeconds -gt 0) { [timespan]::FromSeconds($script:CommandTimeoutSeconds) } else { [timespan]::Zero }

        while ($true) {
            $now = $st.Clock.Elapsed

            if ($deadline -ne [timespan]::Zero -and $now -ge $deadline) { $hitDeadline = $true; break }

            # An unterminated line that has sat this long is shown early, marked
            # as unterminated. A prompt waiting on stdin that will never arrive
            # is exactly this shape, and it is the one case where showing
            # nothing means waiting forever.
            foreach ($s in $streams) {
                if ($s.Done -or -not $s.Pending) { continue }
                if (($now - $s.Since) -lt $flush) { continue }
                Write-StreamLogLine -State $st -Tag $s.Tag -Text $s.Pending -Partial
                $st.Counts[$s.Tag].Lines++
                $s.Pending  = ''
                $st.LastLine = $now
                $st.LastTick = $now
            }

            if ($tick -ne [timespan]::Zero -and ($now - $st.LastLine) -ge $tick -and ($now - $st.LastTick) -ge $tick) {
                Write-StreamLogTick -State $st
                $st.Quiet = $true
            }

            $waiting = @()
            $map     = @()
            foreach ($s in $streams) {
                if ($s.Done) { continue }
                if ($null -eq $s.Task) { $s.Task = $s.Reader.ReadAsync($s.Buffer, 0, $s.Buffer.Length) }
                $waiting += $s.Task
                $map     += $s
            }
            if ($waiting.Count -eq 0) { break }

            $idx = [Threading.Tasks.Task]::WaitAny([Threading.Tasks.Task[]]$waiting, $pollMs)
            if ($idx -lt 0) { continue }

            $s = $map[$idx]
            $read = 0
            try { $read = $s.Task.Result } catch { $null = $_; $read = 0 }
            $s.Task = $null
            if ($read -le 0) {
                $s.Done = $true
                if ($s.Pending) {
                    Write-StreamLogLine -State $st -Tag $s.Tag -Text $s.Pending -Partial
                    $st.Counts[$s.Tag].Lines++
                    $s.Pending = ''
                }
                continue
            }

            $chunk = [string]::new($s.Buffer, 0, $read)
            # ⭐ SILENCE ENDING IS ITSELF A LINE. A run that recovered at four
            # minutes and a run that never recovered are the same picture in a
            # log that only ever reports the alarm.
            if ($st.Quiet) {
                Write-StreamLogSilenceEnd -State $st -Silent ($st.Clock.Elapsed - $st.LastLine)
                $st.Quiet = $false
            }
            $st.Counts[$s.Tag].Bytes += [Text.Encoding]::UTF8.GetByteCount($chunk)
            $had   = [bool]$s.Pending
            $rest  = ''
            $ready = Split-StreamChunk -Pending ($s.Pending + $chunk) -Remainder ([ref]$rest)
            foreach ($l in $ready) {
                Write-StreamLogLine -State $st -Tag $s.Tag -Text $l.Text -Partial:$l.Partial
                $st.Counts[$s.Tag].Lines++
            }
            $s.Pending = $rest
            if ($s.Pending -and (-not $had -or $ready.Count -gt 0)) { $s.Since = $st.Clock.Elapsed }
            $st.LastLine = $st.Clock.Elapsed
            $st.LastTick = $st.LastLine
        }

        if ($hitDeadline) {
            # THE DISTRO IS TERMINATED, NOT LEFT RUNNING. Killing wsl.exe on
            # this side ends the wait and not the process in the guest, which
            # would carry on holding the disk and the CPU with nobody reading
            # it. Get-DistroOutput does the same thing for the same reason.
            Write-StreamLogLine -State $st -Tag 'tick' -Text (
                'TIMED OUT after ' + (Format-Duration -Span $st.Clock.Elapsed) +
                ': -CommandTimeoutSeconds ' + $script:CommandTimeoutSeconds + ' was reached. ' +
                "Terminating '$DistroName'. The exit code is 124, which is what coreutils' " +
                'timeout reports.')
            try { $proc.Kill() } catch { $null = $_ }
            try { $null = $proc.WaitForExit(5000) } catch { $null = $_ }
            try { Invoke-Native -FilePath (Get-WslExe) -Arguments @('--terminate', $DistroName) -IgnoreExitCode | Out-Null }
            catch { $null = $_ }
            $ExitCode.Value = 124
        }
        else {
            $proc.WaitForExit()
            $ExitCode.Value = [int]$proc.ExitCode
        }

        # ⭐ A NON-ZERO CODE GETS A READING, not just a number. 137 is the code
        # from the report this layer was built for, and on its own it says
        # "killed by signal 9" and stops. The distro's state is read HERE, once,
        # while the answer still describes the moment the command ended.
        $why = Get-ExitCodeDiagnosis -ExitCode $ExitCode.Value -DistroState (Get-DistroRunState -DistroName $DistroName)
        if ($why) {
            Write-StreamLogLine -State $st -Tag 'note' -Provenance 'inf' -Text $why
        }
        if ($st.Events) {
            Write-EventRecord -Sink $st.Events -Kind 'EXIT' -RelativeSeconds $st.Clock.Elapsed.TotalSeconds `
                -Data @{ exit_code = $ExitCode.Value; timed_out = $hitDeadline }
        }
    }
    finally {
        try { $st.Out.Flush() } catch { $null = $_ }
        try { $st.Err.Flush() } catch { $null = $_ }
        Close-Sink -Sink $st.Text
        Close-Sink -Sink $st.Events
        $proc.Dispose()
    }
}

function Get-DistroOutput {
    <#
      Runs a payload inside a distro and RETURNS what it printed. Invoke-InDistro
      is the other half of the same pair: it STREAMS, because a caller's command
      has to be visible while it runs, and this one captures, because the script
      itself needs to read an answer.

      Two shapes, ONE transport: both build the command through
      ConvertTo-DistroScriptCommand, so a payload the script asks a question
      with cannot drift onto a channel the caller's payload is not using.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$DistroName,
        [Parameter(Mandatory = $true)][string]$RunAs,
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][byte[]]$ScriptBytes,
        [Parameter(Mandatory = $true)][ref]$ExitCode,
        [string]$What = 'the distro'
    )
    $ExitCode.Value = 1
    $line = ConvertTo-DistroScriptCommand -ScriptBytes $ScriptBytes -GuestPath (New-GuestScratchPath)

    $timedOut = $false
    $text = Invoke-BoundedProcess -Arguments @('-d', $DistroName, '-u', $RunAs, '--', '/bin/sh', '-lc', $line) `
        -TimeoutSeconds $script:TimeoutSeconds -ExitCode $ExitCode -TimedOut ([ref]$timedOut)

    if ($timedOut) {
        # ⛔ The wedged userspace is stopped rather than left running. Killing
        # wsl.exe on this side ends the wait, not the process in the guest.
        try { Invoke-Native -FilePath (Get-WslExe) -Arguments @('--terminate', $DistroName) -IgnoreExitCode | Out-Null }
        catch { $null = $_ }
        throw ("TIMED OUT after $($script:TimeoutSeconds)s waiting for $What in '$DistroName'. " +
               "It never answered, which is not the same as it not being installed: the distro " +
               "is registered and wsl.exe ran, and nothing came back. Its init is most likely " +
               "wedged. The distro has been terminated. Raise the bound with -TimeoutSeconds if " +
               "this machine is simply slow. Partial output: $($text.Trim())")
    }
    return $text
}

function Enable-DistroSystemd {
    <#
      Writes /etc/wsl.conf, restarts the distro so WSL reads it, and then
      CHECKS THAT SYSTEMD IS ACTUALLY PID 1.

      ⛔ THE CHECK IS THE POINT, not the write. A switch that writes a file
      nothing acts on is the forbidden pattern in
      docs/conventions/forbidden-patterns.md twice over: a flag no code reads,
      and a step that exits 0 having done nothing it was asked to do. The
      caller would come away believing they had systemd.

      ⚠ Measured on 2026-08-27, and this is why the check is not optional: the
      OCI base images of alpine:3.22, ubuntu:24.04 and fedora:41 do NOT ship
      systemd. Written into ubuntu:24.04 the flag did nothing at all and said
      nothing; written into alpine:3.22 it cost 20 seconds and then did
      nothing. almalinux:9 does ship it, and there PID 1 became 'systemd' and
      'systemctl is-system-running' answered 'running'.

      --terminate is what makes wsl.conf take effect. A distro already running
      keeps the init it started with, so without this the switch appears to
      work and only the NEXT session gets it.
    #>
    param([Parameter(Mandatory = $true)][string]$DistroName)

    Write-Step "Enabling systemd via /etc/wsl.conf"
    Write-DistroFile -DistroName $DistroName -Path '/etc/wsl.conf' -Content "[boot]`nsystemd=true`n"

    Invoke-Native -FilePath (Get-WslExe) -Arguments @('--terminate', $DistroName) -IgnoreExitCode | Out-Null

    # Delimited on purpose. Windows PowerShell 5.1 wraps captured native stderr
    # in an error record, so the answer is matched inside a marker rather than
    # by comparing the whole captured text.
    $rc = 0
    $probe = 'printf "WSLEPH_PID1[%s]" "$(cat /proc/1/comm 2>/dev/null)"'
    $text = Get-DistroOutput -DistroName $DistroName -RunAs 'root' `
        -ScriptBytes (ConvertTo-Utf8Bytes -Text $probe) -ExitCode ([ref]$rc) -What 'systemd to come up'

    $match = [regex]::Match($text, 'WSLEPH_PID1\[([^\]]*)\]')
    $pid1 = if ($match.Success) { $match.Groups[1].Value } else { '' }

    if ($pid1 -eq 'systemd') {
        Write-Ok "systemd is PID 1"
        return
    }
    throw ("-Systemd was asked for and this distro is NOT running systemd: PID 1 is " +
           "'$pid1'. The likeliest cause is an image that does not ship systemd, and most " +
           "do not: measured on 2026-08-27, alpine:3.22, ubuntu:24.04 and fedora:41 have no " +
           "/usr/lib/systemd/systemd, and almalinux:9 has. Nothing is left registered. " +
           "Use an image that ships systemd, or drop -Systemd.")
}

function Get-ImageOciConfig {
    <#
      The image's OCI configuration. 'podman export' writes a FILESYSTEM and no
      configuration by definition, so ENV, WORKDIR, USER and ENTRYPOINT are not
      in the rootfs and have to be read from the image separately.

      '{{json .Config}}' is the one spelling both engines share. Their arch
      fields are not: see ConvertTo-OciArch.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$EnginePath,
        [Parameter(Mandatory = $true)][string]$ImageRef
    )
    $raw = Invoke-Native -FilePath $EnginePath -Arguments @('image', 'inspect', $ImageRef, '--format', '{{json .Config}}')
    $text = ($raw | Out-String).Trim()
    if ([string]::IsNullOrWhiteSpace($text) -or $text -eq 'null') {
        throw "Could not read the OCI configuration of '$ImageRef'; the engine answered '$text'."
    }
    return ($text | ConvertFrom-Json)
}

function New-OciEnvScript {
    <#
      Turns an image config into a /etc/profile.d snippet.

      ENV and WORKDIR are carried. USER and ENTRYPOINT ARE NOT, and that is a
      decision rather than an omission: WSL fixes the login user at import time
      and -User selects it per call, and a login shell has no entrypoint to run.
      Writing either into profile.d would be a setting that looks like it works.
    #>
    param(
        [Parameter(Mandatory = $true)]$Config,
        [Parameter(Mandatory = $true)][string]$ImageRef
    )
    $lines = @(
        '# Written by wsl-toolkit.ps1 -OciEnv, from the OCI config of:',
        "#   $ImageRef",
        '# The rootfs came from a filesystem export, which carries no config, so',
        '# without this file the environment here is WSL default and not the',
        '# image environment.'
    )
    $names = @($Config.PSObject.Properties.Name)

    if ($names -contains 'Env' -and $Config.Env) {
        foreach ($e in @($Config.Env)) {
            $i = "$e".IndexOf('=')
            if ($i -lt 1) { Write-Warn "skipping malformed image env entry: $e"; continue }
            $k = "$e".Substring(0, $i)
            $v = "$e".Substring($i + 1)
            if ($k -notmatch '^[A-Za-z_][A-Za-z0-9_]*$') {
                Write-Warn "skipping image env name that is not a shell identifier: $k"
                continue
            }
            $lines += ('export {0}={1}' -f $k, (ConvertTo-ShellSingleQuoted -Raw $v))
        }
    }

    if ($names -contains 'WorkingDir' -and $Config.WorkingDir -and $Config.WorkingDir -ne '/') {
        # ':' rather than 'true': it is a shell built-in everywhere, and the
        # guard is there so a WORKDIR the image creates at runtime does not make
        # every login shell fail.
        $lines += ('cd {0} 2>/dev/null || :' -f (ConvertTo-ShellSingleQuoted -Raw $Config.WorkingDir))
    }

    return (($lines -join "`n") + "`n")
}

