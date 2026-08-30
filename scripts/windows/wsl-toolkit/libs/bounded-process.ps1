function Invoke-BoundedProcess {
    <#
      Runs a native program with a HARD TIME LIMIT and returns what it printed.
      With no -FilePath it runs wsl.exe, which is what every caller wanted when
      it was written.

      The defect: a distro whose init wedges hangs the script forever with no
      output. There is a bounded sleep loop INSIDE the guest for the drvfs
      race, but the outer call had no limit at all, so anything that never
      answers never returns.

      ⭐ IT TAKES A -FilePath BECAUSE A SECOND PROGRAM CAN HANG THE SAME WAY.
      `podman` on Windows talks to a VM over ssh, and a machine that is starting,
      stopping or wedged leaves the client waiting with nothing on either
      stream. -Action Resources asks it several questions and none of them is
      worth hanging a session for. ⛔ One bounded runner rather than two: a
      second implementation of the read-before-wait ordering below is a second
      place to get it wrong.

      ⛔ "IT NEVER ANSWERED" IS A DIFFERENT FACT FROM "IT IS NOT INSTALLED",
      and docs/conventions/shell.md section 9 says so in as many words. They
      come back in different fields here: -TimedOut for the first, and
      Get-WslExe's own throw for the second, which fires before this runs.

      ⚠ The streams are read BEFORE the wait, not after. Calling WaitForExit
      first deadlocks any child that fills the pipe buffer: the child blocks
      writing, the parent blocks waiting, and neither moves until the timeout.
      docs/conventions/shell.md section 8.
    #>
    param(
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][int]$TimeoutSeconds,
        [Parameter(Mandatory = $true)][ref]$ExitCode,
        [Parameter(Mandatory = $true)][ref]$TimedOut,
        [string]$FilePath = ''
    )
    $ExitCode.Value = 1
    $TimedOut.Value = $false

    $psi = New-Object Diagnostics.ProcessStartInfo
    $psi.FileName               = if ($FilePath) { $FilePath } else { Get-WslExe }
    $psi.Arguments              = ConvertTo-NativeArgumentString -Arguments $Arguments
    $psi.UseShellExecute        = $false
    $psi.CreateNoWindow         = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError  = $true
    # WSL_UTF8 is set at the top of this script; without this the decode on
    # this side would use the console code page and mangle anything non-ASCII.
    $psi.StandardOutputEncoding = [Text.Encoding]::UTF8
    $psi.StandardErrorEncoding  = [Text.Encoding]::UTF8

    $proc = [Diagnostics.Process]::Start($psi)
    $outTask = $proc.StandardOutput.ReadToEndAsync()
    $errTask = $proc.StandardError.ReadToEndAsync()

    if ($proc.WaitForExit($TimeoutSeconds * 1000)) {
        $ExitCode.Value = $proc.ExitCode
    }
    else {
        $TimedOut.Value = $true
        try { $proc.Kill() } catch { $null = $_ }
        # Without an argument this waits for the streams to close as well, so
        # the two tasks below are complete rather than merely started.
        try { $proc.WaitForExit() } catch { $null = $_ }
    }

    $text = ''
    try { $text = [string]$outTask.Result + [string]$errTask.Result } catch { $null = $_ }
    $proc.Dispose()
    return $text
}

