function Get-DistroRunState {
    <#
      What WSL says about one distro, for the heartbeat line.

      WHY IT IS ON THE TICK AT ALL. A command that prints nothing and a distro
      that has gone away look identical to a caller reading a pipe, and telling
      those two apart is the reason the tick exists. wsl.exe exiting covers the
      case where the utility VM went down underneath everything; this covers the
      case where the distro stopped and the relay has not noticed yet.

      IT NEVER THROWS. A heartbeat that can fail is one that stops beating at
      the moment it matters, so an unreadable answer is the word 'unknown' and
      the tick still prints.
    #>
    param([Parameter(Mandatory = $true)][string]$DistroName)
    try {
        $rc = 0
        $timedOut = $false
        $text = Invoke-BoundedProcess -Arguments @('--list', '--verbose') -TimeoutSeconds 10 `
            -ExitCode ([ref]$rc) -TimedOut ([ref]$timedOut)
        if ($timedOut) { return 'unknown (wsl --list did not answer)' }
        foreach ($raw in ($text -split "`r?`n")) {
            $line = "$raw".Trim()
            if (-not $line) { continue }
            if ($line.StartsWith('*')) { $line = $line.Substring(1).Trim() }
            $fields = @($line -split '\s+' | Where-Object { $_ })
            if ($fields.Count -lt 2) { continue }
            if ($fields[0] -cne $DistroName) { continue }
            return $fields[1]
        }
        return 'not registered'
    }
    catch { $null = $_; return 'unknown' }
}

function Get-DistroDiskFact {
    <#
      The distro's own virtual disk: how big it is, and when the utility VM last
      wrote to it.

      ⭐ THE SIZE IS THE SIGNAL AND THE WRITE TIME IS NOT, and that is a
      measurement rather than a preference. Four candidates were sampled on this
      machine on 2026-08-30 and three of them failed:

        vmmemWSL TotalProcessorTime   0.00 throughout, while a guest wrote
                                      480 MiB. ⛔ Not a signal at all.
        vmmemWSL WorkingSet64         1228 -> 1822 -> 1582 MiB. Machine-wide,
                                      shared by every distro in one utility VM,
                                      and it tracks page cache rather than work.
                                      ⛔ Not attributable to a distro.
        ext4.vhdx LastWriteTimeUtc    ⛔ USELESS, and it looked useful. It
                                      advanced every 1-3s while the guest wrote
                                      AND while the guest sat in `sleep 14` doing
                                      nothing: three consecutive ticks over a
                                      guaranteed-idle guest read "written 2s
                                      ago", "1s ago", "0s ago". WSL touches the
                                      disk on its own, so recency says nothing
                                      about the command.
        ext4.vhdx Length              ⭐ 79,691,776 -> 583,008,256 while the
                                      guest allocated, and flat while it did
                                      not. ⚠ COARSE AND LAGGY: it moves in large
                                      steps, and a second run writing 120 MiB
                                      showed no change six seconds later. So it
                                      answers "yes, something allocated"
                                      sometimes, and "I did not see anything"
                                      the rest of the time.

      ⚠ SO GROWTH IS EVIDENCE AND FLATNESS IS NOT. A disk that grew means
      something inside allocated. A disk that did not grow rules nothing out: a
      computation that writes no files looks exactly like a deadlock from here.
      The tick reports the reading; anything concluded from it is marked as an
      inference and carries what it was drawn from.

      IT NEVER THROWS, for the same reason Get-DistroRunState does not.
    #>
    param([Parameter(Mandatory = $true)][string]$DistroName)
    try {
        $dir = Join-Path $script:BaseDir $DistroName
        if (-not (Test-Path -LiteralPath $dir)) { return $null }
        $vhd = @(Get-ChildItem -LiteralPath $dir -Filter '*.vhdx' -File -ErrorAction SilentlyContinue |
                 Sort-Object Length -Descending | Select-Object -First 1)
        if ($vhd.Count -eq 0) { return $null }
        return [pscustomobject]@{ Bytes = [long]$vhd[0].Length }
    }
    catch { $null = $_; return $null }
}

function Resolve-StreamLogSettings {
    <#
      Every decision about how the log RENDERS, resolved once, before anything
      runs.

      ⭐ ONE PLACE, AND IT IS A PURE FUNCTION. The renderer, the file copy and
      the event record all read this object, so there is no path that could
      reach a sink having skipped a rule. It touches no file and opens no
      handle, which is what lets the selftest exercise the real resolution
      rather than a second copy of it.

      A PROFILE IS A STARTING POINT, NOT A MODE. -Explicit names the parameters
      the caller actually passed, so anything passed beside a profile wins over
      it. Without that list a default is indistinguishable from a choice, and a
      profile would silently overrule a flag the caller typed.
    #>
    param(
        [string]$Mode = 'Relative',
        [string[]]$Columns,
        [AllowEmptyString()][string]$Format,
        [AllowEmptyString()][string]$Separator = ' ',
        [bool]$PrefixOnly = $false,
        [string]$Color = 'auto',
        [AllowEmptyString()][string]$Preset,
        [AllowEmptyString()][string]$TextPath,
        [bool]$TextOverwrite = $false,
        [AllowEmptyString()][string]$EventPath,
        [string[]]$RedactPatterns,
        [int]$MaxBytes = 0,
        [int]$TickSeconds = 30,
        [string[]]$Escalate,
        [string[]]$Explicit = @()
    )
    $was = { param($n) return ($Explicit -contains $n) }

    # The presets, expanded before anything explicit is applied over them.
    if ($Preset) {
        switch ($Preset) {
            'ci' {
                if (-not (& $was 'TimestampColumns')) { $Columns = @('rel', 'delta') }
                if (-not (& $was 'Color'))            { $Color   = 'never' }
            }
            'forensic' {
                if (-not (& $was 'TimestampColumns')) { $Columns = @('wall', 'rel', 'delta') }
                if (-not (& $was 'TimestampFormat'))  { $Format  = '%Y-%m-%d %H:%M:%S.%6f' }
                if (-not (& $was 'Color'))            { $Color   = 'never' }
            }
            'wall' {
                # ⭐ tss's own defaults, so a caller who already timestamps a
                # pipeline with that tool gets the same bytes from this one.
                if (-not (& $was 'TimestampColumns')) { $Columns = @('wall') }
                if (-not (& $was 'TimestampFormat'))  { $Format  = '%Y-%m-%d %H:%M:%S' }
            }
            default { $null = $Preset }   # 'human' is the built-in default; 'raw' is handled in Main
        }
    }

    $modeWasPassed = ((& $was 'TimestampMode') -and -not $Preset)
    $cols = Resolve-StampColumns -Columns $Columns -Mode $Mode -ModeWasPassed $modeWasPassed

    if ($Format -and -not (@($cols | Where-Object { Test-ColumnTakesFormat -Column $_ }).Count -gt 0)) {
        throw ("-TimestampFormat renders a date or an elapsed time, and none of the column(s) you " +
               "asked for takes one: $($cols -join ', '). It applies to 'rel' and 'wall'.")
    }

    # ⛔ 'auto' is decided ONCE, here, and not per line. A run whose output is
    # redirected halfway through does not exist, and asking the console every
    # line would make the answer depend on the line.
    $useColor = switch ($Color) {
        'always' { $true }
        'never'  { $false }
        default  { (-not [Console]::IsOutputRedirected) -and (-not [Console]::IsErrorRedirected) -and (-not $env:NO_COLOR) }
    }

    # ⛔ SPLIT AND PARSED HERE, NEVER BY A [int[]] PARAMETER. A script run
    # through -File receives every argument as a string, so `-TickEscalateSeconds
    # 5,9` arrives as the single string "5,9". PowerShell converts a string to
    # an int with the current culture's number style, where a comma is the
    # THOUSANDS separator, so an [int[]] parameter bound that to 59: one
    # threshold, five hundred and ninety times too late, and nothing said so.
    # Measured under PowerShell 7.6.5 and Windows PowerShell 5.1 on 2026-08-30.
    $esc = @()
    foreach ($e in (Split-DelimitedArgument -Values $Escalate)) {
        $n = 0
        if (-not [int]::TryParse($e, [ref]$n)) {
            throw "-TickEscalateSeconds '$e' is not a whole number of seconds. Give them comma-separated, as 120,300,900."
        }
        if ($n -gt 0) { $esc += $n }
    }
    $esc = @($esc | Sort-Object -Unique)

    return [pscustomobject]@{
        Columns     = $cols
        Format      = $Format
        Separator   = $Separator
        PrefixOnly  = $PrefixOnly
        Color       = $useColor
        TextPath    = $TextPath
        TextOverwrite = $TextOverwrite
        EventPath   = $EventPath
        Redact      = (New-RedactionSet -Patterns (Split-DelimitedArgument -Values $RedactPatterns))
        MaxBytes    = $MaxBytes
        TickSeconds = $TickSeconds
        Escalate    = $esc
    }
}

function New-StreamLogState {
    <#
      Everything the log line and the heartbeat need, in one object, so the
      emitter is a function of state rather than of nine script-scoped
      variables.

      ELAPSED IS A STOPWATCH AND NOT A DIFFERENCE OF TWO CLOCK READINGS. A
      stopwatch is monotonic, so a host that steps its clock mid-run, which a
      laptop resuming from sleep does, cannot produce a relative stamp that goes
      backwards.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$DistroName,
        [Parameter(Mandatory = $true)]$Settings
    )
    return [pscustomobject]@{
        Distro    = $DistroName
        Settings  = $Settings
        Clock     = [Diagnostics.Stopwatch]::StartNew()
        LastStamp = [timespan]::Zero
        LastLine  = [timespan]::Zero
        LastTick  = [timespan]::Zero
        Counts    = @{ out = @{ Lines = 0; Bytes = [long]0 }; err = @{ Lines = 0; Bytes = [long]0 } }
        Out       = $null
        Err       = $null
        Text      = $null
        Events    = $null
        Fired     = @()
        Quiet     = $false
        LastDisk  = $null
    }
}

function Open-StreamLogWriter {
    <#
      A BOM-less UTF-8 writer straight onto a standard handle.

      IT DOES NOT SET [Console]::OutputEncoding, which is the obvious
      alternative and is a change to the MACHINE rather than to this process:
      that property calls SetConsoleOutputCP, which is per-console and outlives
      the script that set it. Writing through our own encoder leaves the
      console's code page exactly as it was found, and a redirected caller gets
      UTF-8 either way, which is the case that matters.
    #>
    param([Parameter(Mandatory = $true)][ValidateSet('Out', 'Err')][string]$Which)
    $handle = if ($Which -eq 'Out') { [Console]::OpenStandardOutput() } else { [Console]::OpenStandardError() }
    $writer = [IO.StreamWriter]::new($handle, [Text.UTF8Encoding]::new($false))
    $writer.AutoFlush = $true
    return $writer
}

function Format-StreamLogPrefix {
    <#
      The columns and the tag, as one string, with nothing of the guest's in it.

      THE TAG IS A FIXED FOUR-CHARACTER FIELD after the separator, which is the
      property a downstream awk or grep depends on. A trailing '~' inside those
      four says the line had not ended when it was printed: a carriage return
      redrew it, or it sat unterminated past the flush bound and was shown
      early. Both are the same fact to a reader, and without the marker a log
      would carry lines that never existed.
    #>
    param(
        [Parameter(Mandatory = $true)]$State,
        [Parameter(Mandatory = $true)][string]$Tag,
        [Parameter(Mandatory = $true)][timespan]$Now,
        [Parameter(Mandatory = $true)][timespan]$Delta,
        [switch]$Partial
    )
    $cfg  = $State.Settings
    $wall = [DateTimeOffset]::Now
    $parts = @()
    foreach ($c in $cfg.Columns) {
        $parts += (Format-StampColumn -Column $c -Format $cfg.Format -Wall $wall -Elapsed $Now -Delta $Delta)
    }
    $field = if ($Partial) { $Tag.PadRight(3) + '~' } else { $Tag.PadRight(4) }
    return (($parts -join ' ') + $cfg.Separator + $field)
}

function Write-StreamLogLine {
    <#
      One line out, with the stamp and the tag ahead of it.

      THE GUEST'S BYTES ARE NEVER TOUCHED unless the caller asked. Everything
      added is to the LEFT of the text. -Redact and -MaxLineBytes are the two
      exceptions and both were asked for by name; each announces itself in the
      line it changed, so a reader never sees an edit they cannot see the mark of.

      FOUR TAGS, TWO STREAMS. Guest stdout is written to stdout and guest stderr
      to stderr, because merging them would destroy the fact the tag reports.
      The tick and the note are the WATCHER'S lines rather than the command's,
      so they go to stderr: a caller capturing stdout to read a result must not
      find a heartbeat in it.

      COLOUR NEVER REACHES A FILE OR A RECORD. The plain line is built once and
      the escape sequences are added only on the way to a console, so a log file
      greps the same as the terminal read.
    #>
    param(
        [Parameter(Mandatory = $true)]$State,
        [Parameter(Mandatory = $true)][ValidateSet('out', 'err', 'tick', 'note')][string]$Tag,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Text,
        [switch]$Partial,
        [ValidateSet('obs', 'der', 'inf')][string]$Provenance = 'obs'
    )
    $cfg = $State.Settings
    $now = $State.Clock.Elapsed
    $delta = $now - $State.LastStamp

    # ⛔ A TICK DOES NOT ADVANCE THE DELTA CLOCK, and this is a defect that was
    # shipped for one run. Delta means "since the previous line", and a tick is
    # the ABSENCE of a line rather than one. With ticks advancing it, a command
    # that went quiet for five seconds and then printed showed '+0.619' against
    # the last tick, on stdout, where the ticks the reader would need to add up
    # are not even present: they go to stderr. A number that small over a gap
    # that large is worse than no number.
    if ($Tag -ne 'tick' -and $Tag -ne 'note') { $State.LastStamp = $now }

    $body = Invoke-Redaction -Set $cfg.Redact -Text $Text
    $body = Limit-LineBytes -Text $body -MaxBytes $cfg.MaxBytes

    $prefix = Format-StreamLogPrefix -State $State -Tag $Tag -Now $now -Delta $delta -Partial:$Partial
    $line = if ($cfg.PrefixOnly) { $prefix } else { $prefix + ' ' + $body }

    $writer = if ($Tag -eq 'out') { $State.Out } else { $State.Err }
    if ($cfg.Color) {
        # ⛔ The escape is written as a codepoint, never as the byte. A literal
        # control byte makes a tracked file invisible to review: grep calls it
        # binary and skips it, and a diff says only that the files differ.
        $e = [string][char]0x1B
        $dim = $e + '[90m'
        $off = $e + '[0m'
        $tagColor = switch ($Tag) {
            'err'  { $e + '[31m' }
            'tick' { $e + '[36m' }
            'note' { $e + '[36m' }
            default { '' }
        }
        $coloured = $dim + $prefix.Substring(0, $prefix.Length - 4) + $off + $tagColor + $prefix.Substring($prefix.Length - 4) + $off
        $writer.WriteLine($(if ($cfg.PrefixOnly) { $coloured } else { $coloured + ' ' + $body }))
    }
    else {
        $writer.WriteLine($line)
    }

    if ($State.Text) { $State.Text.WriteLine($line) }
    if ($State.Events) {
        $stream = switch ($Tag) { 'out' { 'stdout' } 'err' { 'stderr' } default { 'watcher' } }
        $kind   = switch ($Tag) { 'tick' { 'TICK' } 'note' { 'NOTE' } default { 'LOG' } }
        Write-EventRecord -Sink $State.Events -Kind $kind -RelativeSeconds $now.TotalSeconds `
            -Provenance $Provenance -Stream $stream -Text $body -Partial:$Partial
    }
}

function Write-StreamLogTick {
    <#
      The heartbeat, and the reason this layer exists at all.

      NOTHING IS INJECTED INTO THE GUEST TO PRODUCE IT. Every figure on the line
      is one this process already holds: its own stopwatch, the counts it kept
      while relaying, one read-only question to wsl.exe, and the length and
      write time of a file on this host's own disk. An image with no shell, no
      coreutils and no clock ticks exactly as well as a full userspace.

      IT FIRES ON SILENCE, NOT ON A TIMER. A command printing a line a second
      produces no ticks at all. A heartbeat that beats through a conversation is
      one people filter out, and a filtered heartbeat is not a heartbeat.

      ⭐ IT SAYS MORE AS THE SILENCE GROWS, rather than the same thing again. A
      line repeated forty times is one a reader stops reading, and the questions
      worth answering are different at two minutes and at fifteen.
    #>
    param([Parameter(Mandatory = $true)]$State)
    $now    = $State.Clock.Elapsed
    $silent = $now - $State.LastLine
    $disk   = Get-DistroDiskFact -DistroName $State.Distro
    # ⭐ THE DELTA SINCE THE PREVIOUS TICK, not the absolute size. The size on
    # its own is the same number every tick and tells a reader nothing; what
    # moved between two readings is the only part that is evidence.
    $grew = $null
    if ($null -ne $disk) {
        if ($null -ne $State.LastDisk) { $grew = $disk.Bytes - $State.LastDisk }
        $State.LastDisk = $disk.Bytes
    }
    $diskText = if ($null -eq $disk) { 'disk unreadable' }
                elseif ($null -eq $grew) { 'disk ' + (Format-ByteCount -Bytes $disk.Bytes) }
                elseif ($grew -gt 0) { 'disk ' + (Format-ByteCount -Bytes $disk.Bytes) + ' (+' + (Format-ByteCount -Bytes $grew) + ' since last tick)' }
                else { 'disk ' + (Format-ByteCount -Bytes $disk.Bytes) + ' (unchanged)' }
    # ⛔ NOT $state. PowerShell variable names are case-insensitive, so a local
    # spelled that way IS the $State parameter, and assigning a string to it
    # replaced the state object with the word 'Running'. The tick then died on
    # the next property read, mid-run, on the one code path whose whole job is
    # to keep reporting when everything else has gone quiet. Found by driving a
    # real distro; the suite could not see it. Same class as the $args rule in
    # docs/conventions/shell.md section 8.
    $runState = Get-DistroRunState -DistroName $State.Distro
    $text = ((Format-Duration -Span $silent) + ' silent | elapsed ' + (Format-Duration -Span $now) +
             ' | out ' + $State.Counts.out.Lines + ' lines ' + (Format-ByteCount -Bytes $State.Counts.out.Bytes) +
             ' | err ' + $State.Counts.err.Lines + ' lines ' + (Format-ByteCount -Bytes $State.Counts.err.Bytes) +
             ' | distro ' + $runState + ' | ' + $diskText)
    Write-StreamLogLine -State $State -Tag 'tick' -Text $text
    if ($State.Events) {
        $data = @{ silence_s = [Math]::Round($silent.TotalSeconds, 1); distro_state = $runState
                   out_lines = $State.Counts.out.Lines; err_lines = $State.Counts.err.Lines }
        if ($null -ne $disk) { $data['disk_bytes'] = $disk.Bytes }
        if ($null -ne $grew) { $data['disk_grew_bytes'] = $grew }
        Write-EventRecord -Sink $State.Events -Kind 'TICK_FACTS' -RelativeSeconds $now.TotalSeconds -Data $data
    }
    Write-StreamLogEscalation -State $State -Silent $silent -DiskGrew $grew -DistroState $runState
    $State.LastTick = $now
}

function Write-StreamLogEscalation {
    <#
      What the tick adds once the silence passes a threshold, and each threshold
      fires ONCE per silence rather than on every tick after it.

      ⛔ NOTHING HERE IS STATED AS A FACT ABOUT THE COMMAND. The strongest thing
      it says is what the readings are consistent with, marked as an inference,
      with the readings it was drawn from named on the same line. Proving a hang
      needs the program's intent, which a watcher does not have, and a watcher
      that says "hung" will one day say it about a working build.
    #>
    param(
        [Parameter(Mandatory = $true)]$State,
        [Parameter(Mandatory = $true)][timespan]$Silent,
        $DiskGrew,
        [Parameter(Mandatory = $true)][string]$DistroState
    )
    foreach ($step in $State.Settings.Escalate) {
        if ($Silent.TotalSeconds -lt $step) { continue }
        if ($State.Fired -contains $step) { continue }
        $State.Fired += $step

        if ($State.Fired.Count -eq 1) {
            Write-StreamLogLine -State $State -Tag 'note' -Provenance 'obs' -Text (
                'this process relays the guest through a pipe rather than a terminal, so an ' +
                'application that block-buffers off a tty is buffering, and a line will arrive ' +
                'late carrying the time it was RECEIVED. -NoTimestamps hands the handles through ' +
                'untouched and has none of that.')
        }

        $because = @()
        $verdict = 'nothing can be concluded from what is measurable here'
        if ($DistroState -ne 'Running') {
            $because += "wsl says the distro is '$DistroState'"
            $verdict = 'the distro is not running, so this is not a quiet command'
        }
        elseif ($null -eq $DiskGrew) {
            $because += 'the distro disk could not be read, so there is no second signal'
        }
        elseif ($DiskGrew -gt 0) {
            $because += ('the distro disk grew ' + (Format-ByteCount -Bytes $DiskGrew) + ' between the last two ticks')
            $verdict = ('something inside allocated while it was quiet. Consistent with a download, an ' +
                        'unpack or a build that reports nothing')
        }
        else {
            # ⛔ THIS BRANCH RULES NOTHING OUT AND SAYS SO. A flat disk is not
            # evidence of a stall: a computation that writes no files produces
            # exactly this reading, and so does a deadlock. Saying "looks hung"
            # here would be the tool inventing the one fact it does not have.
            $because += ('the distro disk did not grow between the last two ticks, and that reading ' +
                         'is coarse: it moves in large steps and has been measured not to change ' +
                         'six seconds after a guest wrote 120 MiB')
            $verdict = ('NOTHING is ruled out. A prompt waiting on stdin that was never attached, a ' +
                        'lock, a network call inside a long connect timeout, a computation that writes ' +
                        'no files, and a guest writing hard whose disk has not been extended yet all ' +
                        'read exactly like this')
        }
        Write-StreamLogLine -State $State -Tag 'note' -Provenance 'inf' -Text (
            "after $(Format-Duration -Span $Silent) of silence: $verdict. because: " + ($because -join '; '))

        if ($State.Fired.Count -ge 2) {
            Write-StreamLogLine -State $State -Tag 'note' -Provenance 'obs' -Text (
                'to bound a run like this, pass -CommandTimeoutSeconds N: the distro is terminated and ' +
                "the exit code is 124. To see what is registered right now, from another shell: " +
                'wsl-toolkit.ps1 -Action List')
        }
    }
}

function Write-StreamLogSilenceEnd {
    <#
      Output came back.

      ⭐ IT IS A LINE BECAUSE THE ABSENCE OF ONE IS AMBIGUOUS. A run that
      recovered at four minutes and a run that never recovered look identical in
      a log that reports only the alarm, and the reader who scrolls to the last
      tick cannot tell which they are looking at.
    #>
    param(
        [Parameter(Mandatory = $true)]$State,
        [Parameter(Mandatory = $true)][timespan]$Silent
    )
    Write-StreamLogLine -State $State -Tag 'note' -Provenance 'obs' -Text (
        'output resumed after ' + (Format-Duration -Span $Silent) + ' of silence')
    $State.Fired = @()
}

function Split-StreamChunk {
    <#
      Cut a stream's pending text into the lines that are ready to print, and
      hand back what is not.

      A CARRIAGE RETURN TERMINATES A LINE HERE, and that is the difference
      between this and every line-oriented timestamper. curl, apt and every
      layer-progress bar redraw one line with a carriage return and emit no
      newline for minutes, so a reader that waits for a newline shows NOTHING
      while a 200 MB download is visibly working. That silence is
      indistinguishable from a deadlock, which is the failure this layer was
      asked for.

      A TRAILING CARRIAGE RETURN IS HELD, NEVER EMITTED. It may be the first
      half of a CRLF split across two reads, and printing it would turn one line
      into two. The next read resolves it, and the flush bound covers the case
      where there is no next read.
    #>
    param(
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Pending,
        [Parameter(Mandatory = $true)][ref]$Remainder
    )
    $lines = @()
    $start = 0
    $i = 0
    while ($i -lt $Pending.Length) {
        $c = $Pending[$i]
        if ($c -eq "`n") {
            $lines += [pscustomobject]@{ Text = $Pending.Substring($start, $i - $start); Partial = $false }
            $i++
            $start = $i
            continue
        }
        if ($c -eq "`r") {
            if ($i + 1 -ge $Pending.Length) { break }
            if ($Pending[$i + 1] -eq "`n") {
                $lines += [pscustomobject]@{ Text = $Pending.Substring($start, $i - $start); Partial = $false }
                $i += 2
                $start = $i
                continue
            }
            $lines += [pscustomobject]@{ Text = $Pending.Substring($start, $i - $start); Partial = $true }
            $i++
            $start = $i
            continue
        }
        $i++
    }
    $Remainder.Value = $Pending.Substring($start)
    return , $lines
}

function Get-ExitCodeDiagnosis {
    <#
      What a non-zero exit code could mean, when the number alone is ambiguous.

      ⭐ 137 IS THE CODE FROM THE INCIDENT THAT PRODUCED THIS WHOLE LAYER, and
      it means "killed by signal 9" and nothing more. Printing 'FAILED, exit
      137' leaves a reader exactly where the report started. Naming the four
      things that produce it, and which of them this process can rule out, is a
      thing a watcher can do that a grep cannot.

      ⛔ IT NEVER CLAIMS TO KNOW WHICH. Where the evidence does not separate
      them, it says so and lists them.
    #>
    param(
        [Parameter(Mandatory = $true)][int]$ExitCode,
        [Parameter(Mandatory = $true)][string]$DistroState
    )
    if ($ExitCode -eq 0) { return $null }
    if ($ExitCode -eq 124) {
        return 'exit 124 is this script''s own -CommandTimeoutSeconds. The distro was terminated by it.'
    }
    if ($ExitCode -gt 128 -and $ExitCode -lt 160) {
        $sig = $ExitCode - 128
        $named = switch ($sig) {
            9  { 'SIGKILL' }
            15 { 'SIGTERM' }
            2  { 'SIGINT' }
            11 { 'SIGSEGV' }
            default { "signal $sig" }
        }
        $causes = @(
            'the kernel out-of-memory killer inside the utility VM, which every WSL distro shares',
            'something outside this run sending a signal',
            'wsl --shutdown, or the utility VM going away, which takes every distro at once'
        )
        $ruled = if ($DistroState -eq 'Running') {
            'the distro is still Running, so the whole utility VM did not go away'
        }
        else {
            "wsl reports the distro as '$DistroState', which is consistent with the VM having gone"
        }
        return ("exit $ExitCode is 128+$sig, which is $named and nothing more. It is produced by: " +
                ($causes -join '; ') + ". What can be ruled on here: $ruled. " +
                '⛔ This script did not send it: its own timeout reports 124.')
    }
    return "exit $ExitCode is the command's own, passed through unchanged."
}
