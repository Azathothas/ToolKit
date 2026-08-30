# --------------------------------------------------------------------------------------
# The stream log: a timestamp on every line, and a heartbeat when there are none
# --------------------------------------------------------------------------------------
function Format-StrftimeStamp {
    <#
      Render a strftime format string. It is the surface `tss` has, because a
      caller who already timestamps a pipeline with that tool should not have to
      learn a second spelling to timestamp this one.

      IT DOES NOT BUILD A .NET CUSTOM FORMAT STRING, and that is the trap this
      function exists to avoid rather than a style choice. In a .NET custom
      format, ':' and '/' are CULTURE-DEPENDENT PLACEHOLDERS and not literals,
      so 'HH:mm:ss' renders with whatever separator the host culture names and a
      machine in another locale writes a log nothing greps. Every value here is
      formatted with the invariant culture and substituted directly, so a
      literal character in the format stays that character.

      AN UNKNOWN SPECIFIER IS REFUSED. A format that silently renders '%q' as
      'q' is a caller believing they asked for something.

      A SPECIFIER WITH NO MEANING IN THIS MODE IS REFUSED TOO. Relative mode
      measures a duration, so '%Y' has no value to render; answering 1970 would
      put an invented number on a log line.

      %9f CANNOT BE MEASURED HERE AND IT PADS. .NET's tick is 100ns, so the
      ninth digit is a zero this function wrote; on Windows the clock's own
      resolution is coarser still. The page beside this script says so, rather
      than leaving nine digits to be read as nine digits of measurement.
    #>
    param(
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Format,
        [Parameter(Mandatory = $true)][datetimeoffset]$Wall,
        [Parameter(Mandatory = $true)][timespan]$Elapsed,
        [Parameter(Mandatory = $true)][ValidateSet('Relative', 'Wall')][string]$Mode
    )
    $inv = [Globalization.CultureInfo]::InvariantCulture
    # ⛔ AN ORDINAL DICTIONARY, NOT A HASHTABLE. PowerShell hashtable keys are
    # CASE-INSENSITIVE, so '%m' and '%M' are one key and '%z' and '%Z' are
    # another. A literal @{} here does not merely lose the distinction at
    # lookup time: it refuses to parse, which is the loud version of this
    # mistake and the one that found it. strftime distinguishes month from
    # minute by case alone, so the comparer has to as well.
    $values = [Collections.Generic.Dictionary[string, string]]::new([StringComparer]::Ordinal)
    if ($Mode -eq 'Wall') {
        $sub  = $Wall.Ticks % 10000000
        $off  = $Wall.Offset
        $sign = if ($off.Ticks -lt 0) { '-' } else { '+' }
        $values['Y'] = $Wall.Year.ToString('D4', $inv)
        $values['m'] = $Wall.Month.ToString('D2', $inv)
        $values['d'] = $Wall.Day.ToString('D2', $inv)
        $values['H'] = $Wall.Hour.ToString('D2', $inv)
        $values['M'] = $Wall.Minute.ToString('D2', $inv)
        $values['S'] = $Wall.Second.ToString('D2', $inv)
        $values['z'] = ($sign + [Math]::Abs($off.Hours).ToString('D2', $inv) + ':' + [Math]::Abs($off.Minutes).ToString('D2', $inv))
        $values['Z'] = [TimeZoneInfo]::Local.Id
    }
    else {
        $sub = $Elapsed.Ticks % 10000000
        # TOTAL hours, not hours-within-a-day. A run that passes 24 hours reads
        # 24:00:01 rather than starting again at zero: a relative stamp that
        # wraps is a stamp that lies about a long build.
        $values['H'] = ([long][Math]::Floor($Elapsed.TotalHours)).ToString('00', $inv)
        $values['M'] = $Elapsed.Minutes.ToString('D2', $inv)
        $values['S'] = $Elapsed.Seconds.ToString('D2', $inv)
    }
    $values['3f'] = ([long][Math]::Floor($sub / 10000)).ToString('D3', $inv)
    $values['6f'] = ([long][Math]::Floor($sub / 10)).ToString('D6', $inv)
    $values['9f'] = ([long]$sub * 100).ToString('D9', $inv)
    $values['%']  = '%'

    $out = New-Object Text.StringBuilder
    $i = 0
    while ($i -lt $Format.Length) {
        $c = $Format[$i]
        if ($c -ne '%') { $null = $out.Append($c); $i++; continue }
        if ($i + 1 -ge $Format.Length) { throw "-TimestampFormat ends with a bare '%'." }
        $key   = [string]$Format[$i + 1]
        $width = 2
        if (($key -eq '3' -or $key -eq '6' -or $key -eq '9') -and
            $i + 2 -lt $Format.Length -and $Format[$i + 2] -eq 'f') {
            $key   = $key + 'f'
            $width = 3
        }
        if (-not $values.ContainsKey($key)) {
            throw ("-TimestampFormat carries '%$key', which is not a specifier this script renders " +
                   "in $Mode mode. The set here is: " + (($values.Keys | Sort-Object) -join ' ') +
                   ", each written with a leading percent sign.")
        }
        $null = $out.Append($values[$key])
        $i += $width
    }
    return $out.ToString()
}

function Get-StampDefaultFormat {
    # tss's own default for a wall clock, and milliseconds for a relative one: a
    # distro's lifecycle is interesting at that resolution, and a date repeated
    # on every line of one run is a column nobody reads.
    param([Parameter(Mandatory = $true)][string]$Mode)
    if ($Mode -eq 'Wall') { return '%Y-%m-%d %H:%M:%S' }
    return '%H:%M:%S.%3f'
}

function Format-ByteCount {
    param([Parameter(Mandatory = $true)][long]$Bytes)
    $inv = [Globalization.CultureInfo]::InvariantCulture
    if ($Bytes -lt 1024)    { return ($Bytes.ToString($inv) + ' B') }
    if ($Bytes -lt 1048576) { return (($Bytes / 1024.0).ToString('0.0', $inv) + ' KiB') }
    return (($Bytes / 1048576.0).ToString('0.0', $inv) + ' MiB')
}

function Resolve-StampColumns {
    <#
      Turn -TimestampMode or -TimestampColumns into the ordered list the
      renderer walks.

      ⛔ PASSING BOTH IS REFUSED. They are two spellings of one decision, and a
      precedence between them would be a rule a caller has to remember to
      predict their own output. The refusal names both parameters.

      ⭐ The composed form is what the single value cannot express. 'rel,delta'
      puts how far into the run a line is NEXT TO how long since the previous
      one, which is what makes a stall findable: every delta is sub-second and
      then one is not.
    #>
    param(
        [string[]]$Columns,
        [Parameter(Mandatory = $true)][string]$Mode,
        [Parameter(Mandatory = $true)][bool]$ModeWasPassed
    )
    $known = @('rel', 'delta', 'wall', 'iso', 'epoch')
    if ($Columns -and @($Columns).Count -gt 0) {
        if ($ModeWasPassed) {
            throw ('-TimestampColumns and -TimestampMode say the same thing two ways. Pass one. ' +
                   '-TimestampColumns is the one that can carry more than a single value.')
        }
        $out = @()
        foreach ($raw in $Columns) {
            foreach ($c in ($raw -split ',')) {
                $t = "$c".Trim().ToLowerInvariant()
                if (-not $t) { continue }
                if ($known -notcontains $t) {
                    throw "-TimestampColumns carries '$t'. The set is: $($known -join ', ')."
                }
                if ($out -contains $t) { throw "-TimestampColumns names '$t' twice." }
                $out += $t
            }
        }
        if ($out.Count -eq 0) { throw '-TimestampColumns was passed with nothing in it.' }
        return , $out
    }
    switch ($Mode) {
        'Delta' { return , @('delta') }
        'Wall'  { return , @('wall') }
        'Iso'   { return , @('iso') }
        'Epoch' { return , @('epoch') }
        default { return , @('rel') }
    }
}

function Format-StampColumn {
    <#
      One column's text. Every column is a pure function of the two clock
      readings it is given, so a line's whole prefix can be rebuilt from a
      record without re-running anything.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Column,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Format,
        [Parameter(Mandatory = $true)][datetimeoffset]$Wall,
        [Parameter(Mandatory = $true)][timespan]$Elapsed,
        [Parameter(Mandatory = $true)][timespan]$Delta
    )
    $inv = [Globalization.CultureInfo]::InvariantCulture
    switch ($Column) {
        'delta' {
            # ⛔ The spelling here is the one -TimestampMode Delta already
            # produced, character for character. A second spelling for the same
            # number would mean a caller who moved from the mode to the column
            # got different bytes for the same request.
            return ('+' + ([long][Math]::Floor($Delta.TotalSeconds)).ToString($inv) + '.' +
                    ([long][Math]::Floor(($Delta.Ticks % 10000000) / 10000)).ToString('D3', $inv))
        }
        'epoch' { return $Wall.ToUnixTimeSeconds().ToString($inv) }
        'iso'   { return (Format-StrftimeStamp -Format '%Y-%m-%dT%H:%M:%S.%3f%z' -Wall $Wall -Elapsed $Elapsed -Mode 'Wall') }
        'wall'  { return (Format-StrftimeStamp -Format (Get-ColumnFormat -Column 'wall' -Format $Format) -Wall $Wall -Elapsed $Elapsed -Mode 'Wall') }
        default { return (Format-StrftimeStamp -Format (Get-ColumnFormat -Column 'rel' -Format $Format) -Wall $Wall -Elapsed $Elapsed -Mode 'Relative') }
    }
}

function Get-ColumnFormat {
    <#
      -TimestampFormat applies to the two columns that HAVE a format, and to
      nothing else.

      ⛔ Passing it with a column that has none is refused in Main rather than
      ignored here, because a parameter that silently does nothing is a caller
      believing they asked for something. This function is what decides which
      columns those are, so the refusal and the renderer cannot disagree.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Column,
        [AllowEmptyString()][string]$Format
    )
    if ($Format) { return $Format }
    if ($Column -eq 'wall') { return (Get-StampDefaultFormat -Mode 'Wall') }
    return (Get-StampDefaultFormat -Mode 'Relative')
}

function Test-ColumnTakesFormat {
    param([Parameter(Mandatory = $true)][string]$Column)
    return ($Column -eq 'rel' -or $Column -eq 'wall')
}

function Format-Duration {
    param([Parameter(Mandatory = $true)][timespan]$Span)
    $inv   = [Globalization.CultureInfo]::InvariantCulture
    $total = [long][Math]::Floor($Span.TotalSeconds)
    if ($total -lt 60)   { return ($total.ToString($inv) + 's') }
    if ($total -lt 3600) { return (([long][Math]::Floor($total / 60)).ToString($inv) + 'm' + ($total % 60).ToString('D2', $inv) + 's') }
    return (([long][Math]::Floor($total / 3600)).ToString($inv) + 'h' +
            ([long][Math]::Floor(($total % 3600) / 60)).ToString('D2', $inv) + 'm')
}

