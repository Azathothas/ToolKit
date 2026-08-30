function Assert-SinkPathIsUsable {
    <#
      Refuse a sink path that cannot mean what the caller thinks it means.

      ⛔ A WINDOWS RESERVED DEVICE NAME IS REFUSED BY NAME. 'nul' is not a file:
      every byte written to it is discarded and every write reports success, so
      a run ends with the caller holding a log they never got. 'con' is the
      console, so a "file" copy of the log would be printed a second time into
      the stream the log exists to keep clean. The set is matched with the
      EXTENSION STRIPPED and case-insensitively, because 'NUL.txt' and 'nul' are
      the same device.

      ⛔ IT TOUCHES NOTHING, and that is why it can be called from Main. It used
      to live inside the sink openers, which are reached only when a command
      actually runs: -DryRun returned before them, so a dry run reported a plan
      that the real run would have refused. A dry run that disagrees with the
      run it describes is worse than no dry run. Found by the guard-mutation
      pass, planting `-StreamLogPath nul`.
    #>
    param(
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Path,
        [Parameter(Mandatory = $true)][string]$Parameter
    )
    if (-not $Path) { return }
    # ⛔ THE SEPARATORS ARE SPLIT HERE, NOT BY [IO.Path]. That method uses the
    # HOST's separators, so on Linux a backslash is an ordinary character and
    # 'logs\CON.jsonl' has a file name of 'logs\CON', which is not on the list.
    # The rule is about Windows semantics whatever host is asking, and the
    # selftest runs on Linux in CI, which is where this was caught: the case
    # passed on Windows and failed on the ubuntu job, on the same commit.
    $leaf = ($Path -split '[\\/]')[-1]
    $dot = $leaf.IndexOf('.')
    if ($dot -ge 0) { $leaf = $leaf.Substring(0, $dot) }
    if ($script:ReservedDeviceNames -contains $leaf.ToUpperInvariant()) {
        throw ("$Parameter '$Path' names the Windows reserved device '$leaf'. Writing to it " +
               'discards everything and reports success, so the run would end with no log and ' +
               'no error. Pick a real path.')
    }
}

function New-SinkDirectory {
    <#
      The parent directory of a sink, created rather than demanded.

      ⭐ A caller naming logs/run.jsonl has said where they want it; failing
      because the directory is absent is a step they would take by hand every
      time. ⛔ Separate from the refusal above because this one WRITES, and the
      refusal has to be callable from Main, before anything is created.
    #>
    param([Parameter(Mandatory = $true)][string]$Path)
    $dir = Split-Path -Parent $Path
    if ($dir -and -not (Test-Path -LiteralPath $dir)) {
        $null = New-Item -ItemType Directory -Path $dir -Force
    }
}

function New-TextSink {
    <#
      A UTF-8 file the rendered log is copied into, appended by default.

      ⛔ NO BYTE ORDER MARK AND NO COLOUR REACHES IT. A log with escape
      sequences in it is a log a later grep answers wrongly about, and a byte
      order mark in the middle of an appended file is a byte nothing expects.
      Colour is decided once, per sink, rather than per line: the console gets
      it when -Color says so and a file never does.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [switch]$Overwrite
    )
    New-SinkDirectory -Path $Path
    $stream = if ($Overwrite) {
        [IO.File]::Open($Path, [IO.FileMode]::Create, [IO.FileAccess]::Write, [IO.FileShare]::Read)
    }
    else {
        [IO.File]::Open($Path, [IO.FileMode]::Append, [IO.FileAccess]::Write, [IO.FileShare]::Read)
    }
    $w = [IO.StreamWriter]::new($stream, [Text.UTF8Encoding]::new($false))
    $w.AutoFlush = $true
    return $w
}

function New-EventSink {
    <#
      One JSON object per line, one per event.

      ⭐ THE RENDERED LOG IS A VIEW OVER THIS. Anything the terminal shows that
      this does not carry would be a renderer that knows something the record
      does not, and a reader who reconstructs the run from the file would be
      reading a shorter run than happened.

      ⛔ THE SCHEMA CARRIES A VERSION. A positional or unversioned record that
      changes shape mis-reads silently, and the reader that mis-reads it is a
      program rather than a person.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$DistroName
    )
    New-SinkDirectory -Path $Path
    $stream = [IO.File]::Open($Path, [IO.FileMode]::Append, [IO.FileAccess]::Write, [IO.FileShare]::Read)
    $w = [IO.StreamWriter]::new($stream, [Text.UTF8Encoding]::new($false))
    $w.AutoFlush = $true
    return [pscustomobject]@{
        Writer = $w
        Seq    = [long]0
        Distro = $DistroName
        Start  = [DateTimeOffset]::Now.ToString('o', [Globalization.CultureInfo]::InvariantCulture)
    }
}

function Write-EventRecord {
    <#
      One event. Fields that are always present are always present, and a field
      that could not be measured is ABSENT rather than zero.

      ⭐ `seq` IS MONOTONIC AND GAPLESS. A gap in it means records were dropped,
      which is itself a finding rather than something for a reader to work out.

      ⭐ `prov` SAYS HOW THE FACT WAS OBTAINED: obs read from an interface, der
      computed from observations, inf a judgement that can be wrong. ⛔ An
      inference carries `because`, naming what it was drawn from. An unexplained
      guess in a machine-readable log is a guess a program will treat as a
      measurement.
    #>
    param(
        [Parameter(Mandatory = $true)]$Sink,
        [Parameter(Mandatory = $true)][string]$Kind,
        [Parameter(Mandatory = $true)][double]$RelativeSeconds,
        [ValidateSet('obs', 'der', 'inf')][string]$Provenance = 'obs',
        [string]$Stream,
        [AllowEmptyString()][string]$Text,
        [switch]$Partial,
        [hashtable]$Data
    )
    if ($null -eq $Sink) { return }
    $inv = [Globalization.CultureInfo]::InvariantCulture
    $Sink.Seq++
    $rec = [ordered]@{
        schema = 'wsl-toolkit-event/1'
        seq    = $Sink.Seq
        t_rel  = [Math]::Round($RelativeSeconds, 3)
        t_wall = [DateTimeOffset]::Now.ToString('o', $inv)
        kind   = $Kind
        prov   = $Provenance
        distro = $Sink.Distro
    }
    if ($PSBoundParameters.ContainsKey('Stream') -and $Stream) { $rec['stream'] = $Stream }
    if ($PSBoundParameters.ContainsKey('Text')) {
        $rec['text'] = $Text
        $rec['partial'] = [bool]$Partial
    }
    if ($Data) { foreach ($k in ($Data.Keys | Sort-Object)) { $rec[$k] = $Data[$k] } }
    # ⛔ -Depth, because ConvertTo-Json defaults to 2 and renders anything deeper
    # as the literal text 'System.Collections.Hashtable'.
    $Sink.Writer.WriteLine(($rec | ConvertTo-Json -Depth 6 -Compress))
}

function Close-Sink {
    <#
      Flush and dispose, and never throw while doing it. A sink that fails to
      close at the end of a run must not turn a command that succeeded into a
      script that reported an error.
    #>
    param($Sink)
    if ($null -eq $Sink) { return }
    $w = if ($Sink -is [IO.StreamWriter]) { $Sink } else { $Sink.Writer }
    try { $w.Flush() } catch { $null = $_ }
    try { $w.Dispose() } catch { $null = $_ }
}
