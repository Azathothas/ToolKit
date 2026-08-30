# --------------------------------------------------------------------------------------
# Main
# --------------------------------------------------------------------------------------
try {
    if ([string]::IsNullOrWhiteSpace($script:BaseDir)) { throw "LOCALAPPDATA is not set; cannot choose a base directory." }
    New-Item -ItemType Directory -Path $script:BaseDir -Force | Out-Null

    # ⛔ EVERY PARAMETER IS CHECKED AGAINST THE ACTION IT WAS PASSED TO, and one
    # that the action does not read is REFUSED. A caller who passes -Image to
    # -Action List believes something is happening; nothing is, and nothing said
    # so. "A setting or flag that no code reads" is a row in
    # docs/conventions/forbidden-patterns.md and this is the other half of it.
    Assert-ParametersApplyToAction -Action $Action -Passed @($PSBoundParameters.Keys)

    # ⭐ raw IS -NoTimestamps UNDER ANOTHER NAME, resolved here so that exactly
    # one variable decides whether the relay runs. Two switches that mean the
    # same thing and are read in two places is how they come to disagree.
    # ⛔ SCRIPT-SCOPED, because Invoke-InDistro reads it. It was a local for one
    # build, Invoke-InDistro branched on -NoTimestamps instead, and
    # -TimestampProfile raw therefore took the relay path with no settings built
    # for it.
    $script:RelayOff = [bool]$NoTimestamps -or ($TimestampProfile -eq 'raw')
    $relayOff = $script:RelayOff

    # ⛔ REFUSED, NEVER IGNORED. Every combination below is one where a
    # parameter the caller typed would have no effect at all. The refusal names
    # both halves, so the message says which one to drop rather than that
    # something is wrong.
    if ($relayOff) {
        $dead = @('TimestampMode', 'TimestampFormat', 'TimestampColumns', 'TimestampSeparator',
                  'PrefixOnly', 'Color', 'StreamLogPath', 'StreamLogOverwrite', 'EventLog',
                  'Redact', 'MaxLineBytes', 'TickSeconds', 'TickEscalateSeconds',
                  'CommandTimeoutSeconds') |
            Where-Object { $PSBoundParameters.ContainsKey($_) }
        if ($dead) {
            $off = if ($NoTimestamps) { '-NoTimestamps' } else { '-TimestampProfile raw' }
            throw ("$off turns the stream log off, so -" + ($dead -join ' and -') +
                   " would do nothing. Drop $off to use them, or drop them.")
        }
    }

    if (-not $relayOff) {
        # ⭐ RESOLVED ONCE, HERE, AND EVERY SINK READS THE RESULT. A typo in a
        # format or an unusable log path is otherwise found by the first line of
        # output, which on -Action New is after a pull, an export and an import:
        # forty seconds and several hundred megabytes spent to learn that '%q'
        # is not a specifier.
        $script:LogSettings = Resolve-StreamLogSettings `
            -Mode $TimestampMode -Columns $TimestampColumns -Format $TimestampFormat `
            -Separator $TimestampSeparator -PrefixOnly:$PrefixOnly -Color $Color `
            -Preset $TimestampProfile -TextPath $StreamLogPath -TextOverwrite:$StreamLogOverwrite `
            -EventPath $EventLog -RedactPatterns $Redact -MaxBytes $MaxLineBytes `
            -TickSeconds $TickSeconds -Escalate $TickEscalateSeconds `
            -Explicit @($PSBoundParameters.Keys)

        # ⛔ THE SINK PATHS ARE REFUSED HERE, not where they are opened. The
        # openers are reached only when a command actually runs, so -DryRun
        # returned before them and reported a plan the real run would have
        # refused.
        Assert-SinkPathIsUsable -Path $StreamLogPath -Parameter '-StreamLogPath'
        Assert-SinkPathIsUsable -Path $EventLog      -Parameter '-EventLog'

        # Rendered once and thrown away, so a bad specifier is refused before a
        # distro exists to refuse it against.
        foreach ($c in $script:LogSettings.Columns) {
            $null = Format-StampColumn -Column $c -Format $script:LogSettings.Format `
                -Wall ([DateTimeOffset]::Now) -Elapsed ([timespan]::Zero) -Delta ([timespan]::Zero)
        }
    }

    # Resolved ONCE, here, so New and Run cannot disagree about which switch
    # won and a bad -CommandFile is refused before a distro is built for it.
    $pairs = Get-ScriptArgPairs -FromFile $ScriptArgFile -Pairs $ScriptArg
    $script:CommandBytes = Resolve-CommandBytes -Text $Command -FromFile $CommandFile -FromB64 $CommandB64 -Pairs $pairs

    switch ($Action) {
        'New'    { Invoke-ActionNew }
        'Run'    { Invoke-ActionRun }
        'Enter'  { Invoke-ActionEnter }
        'List'   { Invoke-ActionList }
        # ⛔ These three are read-only and none of them creates a distro. They
        # sit beside List rather than under it because a caller asking "what is
        # on this machine", one asking "what do I bind to" and one asking "what
        # can this host even do" want different answers, and folding any of them
        # into List would make the one line a script consumes arrive in the
        # middle of a page of prose.
        'Resources'   { Invoke-ActionResources }
        'HostAddress' { Invoke-ActionHostAddress }
        'Doctor'      { Invoke-ActionDoctor }
        'Remove' {
            if (-not $Name) { throw "Action Remove requires -Name." }
            $target = Resolve-DistroName -Requested $Name -FromImage ''
            if ($DryRun) {
                Write-DryRunPlan -Action 'Remove' -DistroName $target -Steps @(
                    ('unregister ' + $target),
                    ('delete     ' + (Join-Path $script:BaseDir $target)),
                    'the name was prefix-forced first, so a protected distro cannot be reached by asking for it')
            }
            else { Remove-EphemeralDistro -DistroName $target }
        }
        'Purge'  { Invoke-ActionPurge }
    }
}
catch {
    # ⛔ STDERR, NOT STDOUT. An error is not a result, and -Action HostAddress
    # makes that concrete: a caller assigning this script's stdout to a variable
    # would otherwise get the string "ERROR: ..." where an IP address goes, and
    # act on it. Every check under scripts/common/ already reports this way.
    # ⚠ Nothing about the exit code changed, and the exit code is what every
    # existing caller reads. docs/consumers.md records the move.
    [Console]::Error.WriteLine("ERROR: $($_.Exception.Message)")
    exit 1
}
