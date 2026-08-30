function Get-ParameterApplicability {
    <#
      Which parameters each action actually READS.

      ⛔ THIS TABLE IS THE ANSWER TO A REAL DEFECT, not tidiness. -Image passed
      to -Action List did nothing and said nothing, so a caller who typed it
      believed something was happening. "A setting or flag that no code reads"
      is a row in docs/conventions/forbidden-patterns.md, and a parameter
      silently ignored by the action it was handed to is the same lie from the
      caller's side.

      ⭐ IT IS DERIVED FROM WHERE THE VARIABLE IS READ, not from what the help
      says. Every entry below was checked against the function that reads it:
      -TimeoutSeconds reaches Get-DistroOutput and Get-EngineAnswer and nothing
      else, so it belongs to New and Resources and is REFUSED on Run, where the
      parameter a caller actually wants is -CommandTimeoutSeconds. The refusal
      says that rather than only saying no.

      ⚠ A parameter added without a row here is refused on every action, which
      is loud, and the selftest asserts that every parameter in the block has
      one. The failure mode of forgetting is a refusal, never a silent gap.
    #>
    $relay = @('New', 'Run')
    return [ordered]@{
        Image                 = @('New')
        Tarball               = @('New')
        Name                  = @('New', 'Run', 'Enter', 'Remove')
        Command               = $relay
        CommandFile           = $relay
        CommandB64            = $relay
        User                  = @('New', 'Run', 'Enter')
        Ephemeral             = @('New')
        OciEnv                = @('New')
        Systemd               = @('New')
        TimeoutSeconds        = @('New', 'Resources')
        Verbatim              = $relay
        ScriptArg             = $relay
        ScriptArgFile         = $relay
        NoTimestamps          = $relay
        TimestampMode         = $relay
        TimestampFormat       = $relay
        TimestampColumns      = $relay
        TimestampSeparator    = $relay
        TimestampProfile      = $relay
        PrefixOnly            = $relay
        Color                 = $relay
        StreamLogPath         = $relay
        StreamLogOverwrite    = $relay
        EventLog              = $relay
        Redact                = $relay
        MaxLineBytes          = $relay
        TickSeconds           = $relay
        TickEscalateSeconds   = $relay
        CommandTimeoutSeconds = $relay
        DryRun                = @('New', 'Run', 'Enter', 'Remove', 'Purge')
        Force                 = @('New', 'Remove', 'Purge')
    }
}

function Assert-ParametersApplyToAction {
    <#
      Refuse a parameter the chosen action does not read.

      ⛔ THIS IS A BREAK AND IT IS MEANT TO BE. A caller who was passing a
      parameter that did nothing now gets a refusal on the first run, which is
      the point: they were not getting what they asked for and nothing told
      them. docs/consumers.md carries the row.

      -Action is not in the table because it IS the choice, and PowerShell's own
      common parameters are never this script's to judge.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Action,
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][string[]]$Passed
    )
    $table  = Get-ParameterApplicability
    $common = @([System.Management.Automation.Cmdlet]::CommonParameters) +
              @([System.Management.Automation.Cmdlet]::OptionalCommonParameters) +
              @('Action')

    $bad = @()
    foreach ($p in $Passed) {
        if ($common -contains $p) { continue }
        if (-not $table.Contains($p)) {
            # A parameter with no row is refused everywhere rather than allowed
            # everywhere. Forgetting to add a row then fails loudly on the first
            # use instead of reinstating exactly the defect this exists to stop.
            $bad += "-$p has no entry in this script's parameter table, so it cannot be checked against -Action $Action"
            continue
        }
        if ($table[$p] -notcontains $Action) {
            $bad += ("-$p is read by -Action " + (($table[$p]) -join ', ') + " and not by -Action $Action")
        }
    }
    if ($bad.Count -eq 0) { return }

    $hint = ''
    if (($Passed -contains 'TimeoutSeconds') -and ($Action -eq 'Run')) {
        $hint = " ⭐ -TimeoutSeconds bounds the questions this script asks a distro for itself. The bound on YOUR command is -CommandTimeoutSeconds."
    }
    throw ("-Action $Action ignores parameters you passed, so it would have done something other " +
           'than what you asked: ' + ($bad -join '; ') + '.' + $hint)
}

function Write-DryRunPlan {
    <#
      What would happen, printed, with nothing done.

      ⭐ IT GOES TO STDOUT THROUGH Write-Output, so a caller can capture and
      audit it. A plan a person can only read on a terminal is a plan nothing
      can check, and the point of a dry run is that the wrapper is auditable
      rather than trusted.

      ⛔ IT CREATES, WRITES AND DELETES NOTHING. Every caller of this returns
      immediately afterwards.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Action,
        [string]$DistroName,
        [string[]]$Steps
    )
    Write-Output "DRY RUN: -Action $Action. Nothing below has been done."
    if ($DistroName) { Write-Output "  distro     $DistroName" }
    foreach ($s in @($Steps)) { Write-Output "  $s" }
    Write-Output '  ⛔ nothing was created, imported, written or removed. Drop -DryRun to run it.'
}

function Get-CommandPlanLine {
    <#
      The exact wsl.exe argument string a run would use, for a dry run to print.

      ⭐ IT IS BUILT BY THE SAME TWO FUNCTIONS THE REAL RUN USES, so the plan
      cannot describe a command line the run would not produce. A dry run that
      renders its own approximation is a dry run that stops matching the day
      either function changes.

      ⚠ The guest scratch path carries a random component, so the plan's path
      and the run's path differ. That is said on the line rather than hidden by
      printing a fake constant.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$DistroName,
        [Parameter(Mandatory = $true)][string]$RunAs
    )
    if ($null -eq $script:CommandBytes) { return $null }
    $line = ConvertTo-DistroScriptCommand -ScriptBytes $script:CommandBytes -GuestPath (New-GuestScratchPath)
    # ⛔ NOT $args. It is an automatic variable inside a function, and PowerShell
    # variable names are case-insensitive, so assigning to it here would collide
    # with the one the engine owns. docs/conventions/shell.md section 8.
    $argLine = ConvertTo-NativeArgumentString -Arguments @('-d', $DistroName, '-u', $RunAs, '--', '/bin/sh', '-lc', $line)
    return ((Get-WslExe) + ' ' + $argLine)
}
