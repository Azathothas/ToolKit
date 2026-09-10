function Read-EventLogFile {
    <#
      One recorded run, as records, with the shape checked before it is trusted.

      ⛔ A GAP IN `seq` IS REPORTED, NEVER SMOOTHED OVER. The field is documented
      as monotonic and gapless, so a gap means records were dropped. That is a
      finding about the RECORDING rather than about the run, and a reader handed
      a quietly-renumbered log would draw conclusions about a run they were not
      shown. It is a refusal here and it names the two sequence numbers.

      ⛔ THE SCHEMA IS CHECKED, NOT ASSUMED. A positional or unversioned record
      that changes shape mis-reads silently, and the reader is a program.
    #>
    param([Parameter(Mandatory = $true)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "No event log at '$Path'. -EventLog writes one; this reads it back."
    }
    $records = @()
    $line = 0
    $prev = $null
    foreach ($text in [IO.File]::ReadLines($Path)) {
        $line++
        if ([string]::IsNullOrWhiteSpace($text)) { continue }
        try { $rec = $text | ConvertFrom-Json }
        catch { throw "Line $line of '$Path' is not JSON: $($_.Exception.Message)" }
        if (-not $rec.PSObject.Properties['schema'] -or $rec.schema -ne 'wsl-toolkit-event/1') {
            $saw = if ($rec.PSObject.Properties['schema']) { $rec.schema } else { '(none)' }
            throw ("Line $line of '$Path' declares schema '$saw' and this build reads " +
                   "'wsl-toolkit-event/1'.")
        }
        if ($null -ne $prev -and $rec.seq -ne ($prev + 1)) {
            throw ("Line $line of '$Path': seq jumps from $prev to $($rec.seq). That field is " +
                   'gapless by construction, so records were dropped and this log is not the ' +
                   'whole run. Nothing was rendered.')
        }
        $prev = [long]$rec.seq
        $records += $rec
    }
    if ($records.Count -eq 0) { throw "'$Path' holds no records." }
    return $records
}

function Get-EventLogSummary {
    <#
      The figures a comparison is made of, derived once so Replay and Compare
      cannot disagree about them.

      ⭐ THE LONGEST SILENCE IS THE FIGURE THAT EARNS THIS. A run whose result
      stayed green while its longest gap grew fifteen times has a regression no
      exit code reports, and nothing else in the record surfaces it.
    #>
    param([Parameter(Mandatory = $true)][AllowEmptyCollection()][object[]]$Records)

    # ⚠ THE KINDS AND STREAM NAMES ARE THE WRITER'S, read from it rather than
    # imagined. Write-StreamLogLine writes kind LOG with stream stdout, stderr
    # or watcher; the first version of this reader looked for LINE with out and
    # err, matched nothing, and reported a real run as having produced no
    # output at all. A reader written against a schema nobody checked is the
    # silent mis-read the version field exists to prevent, from the other side.
    $out = @{ Lines = 0; Bytes = [long]0 }
    $err = @{ Lines = 0; Bytes = [long]0 }
    $firstOutput = $null
    $longest = 0.0
    $longestAt = 0.0
    $lastOutput = 0.0
    $exit = $null
    $timedOut = $false
    $duration = 0.0
    $enc = [Text.Encoding]::UTF8

    foreach ($r in $Records) {
        $t = [double]$r.t_rel
        if ($t -gt $duration) { $duration = $t }
        if ($r.kind -eq 'LOG') {
            $bucket = if ($r.PSObject.Properties['stream'] -and $r.stream -eq 'stderr') { $err } else { $out }
            $bucket.Lines++
            if ($r.PSObject.Properties['text']) { $bucket.Bytes += $enc.GetByteCount([string]$r.text) }
            if ($null -eq $firstOutput) { $firstOutput = $t }
            $gap = $t - $lastOutput
            if ($gap -gt $longest) { $longest = $gap; $longestAt = $t }
            $lastOutput = $t
        }
        elseif ($r.kind -eq 'EXIT') {
            if ($r.PSObject.Properties['exit_code']) { $exit = [int]$r.exit_code }
            if ($r.PSObject.Properties['timed_out']) { $timedOut = [bool]$r.timed_out }
        }
    }
    # ⚠ THE TAIL COUNTS. A run whose last line arrived at four seconds and which
    # ended at four minutes was silent for the rest, and a summary that measured
    # only the gaps BETWEEN lines would report the quietest part of the run as
    # not having happened at all.
    $tail = $duration - $lastOutput
    if ($tail -gt $longest) { $longest = $tail; $longestAt = $duration }

    return [pscustomobject]@{
        Records      = $Records.Count
        Duration     = $duration
        OutLines     = $out.Lines
        OutBytes     = $out.Bytes
        ErrLines     = $err.Lines
        ErrBytes     = $err.Bytes
        FirstOutput  = $firstOutput
        LongestGap   = $longest
        LongestGapAt = $longestAt
        ExitCode     = $exit
        TimedOut     = $timedOut
    }
}

function Format-RunSummary {
    <# The one-line reading of a recorded run, shared by Replay and Compare. #>
    param([Parameter(Mandatory = $true)]$Summary)
    $first = if ($null -eq $Summary.FirstOutput) { 'no output' }
             else { (Format-Duration -Span ([timespan]::FromSeconds($Summary.FirstOutput))) + ' to first output' }
    $code  = if ($null -eq $Summary.ExitCode) { 'no exit recorded' } else { 'exit ' + $Summary.ExitCode }
    return ('elapsed ' + (Format-Duration -Span ([timespan]::FromSeconds($Summary.Duration))) +
            ' | ' + $first +
            ' | longest silence ' + (Format-Duration -Span ([timespan]::FromSeconds($Summary.LongestGap))) +
            ' | out ' + $Summary.OutLines + ' lines ' + (Format-ByteCount -Bytes $Summary.OutBytes) +
            ' | err ' + $Summary.ErrLines + ' lines ' + (Format-ByteCount -Bytes $Summary.ErrBytes) +
            ' | ' + $code)
}

function Invoke-ActionReplay {
    <#
      Render a recorded run again, in whatever timestamp shape is asked for.

      ⭐ IT IS POSSIBLE BECAUSE THE RENDERER IS ALREADY A PURE FUNCTION OF THE
      RECORD. Format-StreamLogPrefix needs a clock reading and a tag, and both
      are in every record, so this reuses the renderer rather than growing a
      second one. A second renderer is how a log file and a terminal come to
      show different runs.

      ⛔ IT RUNS NOTHING AND CREATES NOTHING. It reads a file and writes to the
      two streams. A report that made the state directory it was about to
      describe is the defect WSL-55 closed in the other product.
    #>
    if (-not $From) { throw 'Action Replay requires -From <event log>.' }
    $records = Read-EventLogFile -Path $From

    # ⭐ THE SETTINGS MAIN ALREADY RESOLVED, not a second resolution. Resolving
    # them again here would be a second place where a profile is expanded and an
    # explicit flag is applied over it, and the two would drift: a Replay would
    # render a line one way and a live run the other, from one set of flags.
    # ⛔ -NoTimestamps and -TimestampProfile raw turn the relay off entirely, so
    # main builds no settings for them; on a Replay that means the plain text.
    $settings = if ($script:RelayOff) { $null } else { $script:LogSettings }
    if ($null -eq $settings) {
        foreach ($r in $records) {
            if (@('LOG', 'TICK', 'NOTE') -notcontains $r.kind) { continue }
            $text = if ($r.PSObject.Properties['text']) { [string]$r.text } else { '' }
            $isErr = ($r.kind -ne 'LOG') -or ($r.PSObject.Properties['stream'] -and $r.stream -eq 'stderr')
            if ($isErr) { [Console]::Error.WriteLine($text) } else { [Console]::Out.WriteLine($text) }
        }
        $plain = Get-EventLogSummary -Records $records
        Write-Step ("replayed {0} record(s) from {1}, with no prefix" -f $plain.Records, $From)
        Write-Ok (Format-RunSummary -Summary $plain)
        return
    }

    $distro = if ($records[0].PSObject.Properties['distro']) { [string]$records[0].distro } else { '(unknown)' }
    $state = New-StreamLogState -DistroName $distro -Settings $settings
    $state.Out = Open-StreamLogWriter -Which 'Out'
    $state.Err = Open-StreamLogWriter -Which 'Err'
    try {
        $last = [timespan]::Zero
        foreach ($r in $records) {
            if (@('LOG', 'TICK', 'NOTE') -notcontains $r.kind) { continue }
            $now = [timespan]::FromSeconds([double]$r.t_rel)
            $tag = switch ($r.kind) {
                'LOG'   { if ($r.PSObject.Properties['stream'] -and $r.stream -eq 'stderr') { 'err' } else { 'out' } }
                'TICK'  { 'tick' }
                default { 'note' }
            }
            $text = if ($r.PSObject.Properties['text']) { [string]$r.text } else { '' }
            $partial = [bool]($r.PSObject.Properties['partial'] -and $r.partial)
            # ⚠ THE RECORD'S OWN WALL READING, not this machine's clock. A
            # replay of last week's run stamped with today's date is a document
            # that says something false about when the work happened.
            $wall = $null
            if ($r.PSObject.Properties['t_wall']) {
                try {
                    $wall = [DateTimeOffset]::Parse([string]$r.t_wall,
                        [Globalization.CultureInfo]::InvariantCulture)
                }
                catch { $null = $_ }
            }
            $prefix = Format-StreamLogPrefix -State $state -Tag $tag -Now $now -Delta ($now - $last) `
                -Partial:$partial -Wall $wall
            $body = if ($settings.PrefixOnly) { '' } else { ' ' + $text }
            $sink = if ($tag -eq 'out') { $state.Out } else { $state.Err }
            $sink.WriteLine($prefix + $body)
            $last = $now
        }
    }
    finally {
        try { $state.Out.Flush() } catch { $null = $_ }
        try { $state.Err.Flush() } catch { $null = $_ }
    }

    $s = Get-EventLogSummary -Records $records
    Write-Step ("replayed {0} record(s) from {1}" -f $s.Records, $From)
    Write-Ok (Format-RunSummary -Summary $s)
}

function Invoke-ActionCompare {
    <#
      Two recorded runs, and what moved between them.

      ⭐ THE LONGEST SILENCE IS WHY THIS EXISTS. Two runs that both exited 0 are
      the same result and can be very different runs, and the figure that says so
      is not in the exit code.

      ⛔ IT REPORTS AND DOES NOT JUDGE. There is no threshold at which it calls a
      difference a regression: a ratio that is a regression for one workload is
      noise for another, and a tool that ruled on it would be inventing a
      standard nobody set.
    #>
    if (-not $From)    { throw 'Action Compare requires -From <event log>.' }
    if (-not $Against) { throw 'Action Compare requires -Against <event log>.' }

    $a = Get-EventLogSummary -Records (Read-EventLogFile -Path $From)
    $b = Get-EventLogSummary -Records (Read-EventLogFile -Path $Against)

    $inv = [Globalization.CultureInfo]::InvariantCulture
    $span = { param($v) if ($null -eq $v) { '-' } else { Format-Duration -Span ([timespan]::FromSeconds([double]$v)) } }
    $num  = { param($v) if ($null -eq $v) { '-' } else { ([string]$v) } }
    # ⛔ A DASH WHERE THE VALUE IS UNKNOWN, never a zero. A fabricated number on
    # a report is worse than a blank, because a blank gets checked and a number
    # gets used. docs/conventions/prose.md.
    $delta = {
        param($x, $y)
        if ($null -eq $x -or $null -eq $y) { return '-' }
        $d = [double]$y - [double]$x
        $sign = if ($d -gt 0) { '+' } else { '' }
        return $sign + $d.ToString('0.###', $inv)
    }

    Write-Step "A: $From"
    Write-Step "B: $Against"
    $rows = @(
        @('elapsed',         (& $span $a.Duration),    (& $span $b.Duration),    (& $delta $a.Duration $b.Duration)),
        @('to first output', (& $span $a.FirstOutput), (& $span $b.FirstOutput), (& $delta $a.FirstOutput $b.FirstOutput)),
        @('longest silence', (& $span $a.LongestGap),  (& $span $b.LongestGap),  (& $delta $a.LongestGap $b.LongestGap)),
        @('out lines',       (& $num $a.OutLines),     (& $num $b.OutLines),     (& $delta $a.OutLines $b.OutLines)),
        @('out bytes',       (& $num $a.OutBytes),     (& $num $b.OutBytes),     (& $delta $a.OutBytes $b.OutBytes)),
        @('err lines',       (& $num $a.ErrLines),     (& $num $b.ErrLines),     (& $delta $a.ErrLines $b.ErrLines)),
        @('err bytes',       (& $num $a.ErrBytes),     (& $num $b.ErrBytes),     (& $delta $a.ErrBytes $b.ErrBytes)),
        @('exit code',       (& $num $a.ExitCode),     (& $num $b.ExitCode),     (& $delta $a.ExitCode $b.ExitCode))
    )
    Write-Note ("  {0,-16} {1,-14} {2,-14} {3}" -f 'figure', 'A', 'B', 'B - A')
    foreach ($r in $rows) {
        Write-Note ("  {0,-16} {1,-14} {2,-14} {3}" -f $r[0], $r[1], $r[2], $r[3])
    }
    if ($a.LongestGap -ne $b.LongestGap) {
        $longer = if ($b.LongestGap -gt $a.LongestGap) { 'B' } else { 'A' }
        $at = if ($longer -eq 'B') { $b.LongestGapAt } else { $a.LongestGapAt }
        Write-Warn ("$longer has the longer silence, " +
                    (Format-Duration -Span ([timespan]::FromSeconds([Math]::Max($a.LongestGap, $b.LongestGap)))) +
                    ', ending at ' + (Format-Duration -Span ([timespan]::FromSeconds($at))) + ' into the run.')
    }
    if ($a.ExitCode -ne $b.ExitCode) {
        Write-Warn ('the two runs did not end the same way: A ' + (& $num $a.ExitCode) +
                    ', B ' + (& $num $b.ExitCode))
    }
}
