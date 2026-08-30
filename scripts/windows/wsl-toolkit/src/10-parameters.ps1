# -- PSScriptAnalyzer, suppressed per rule with the reason --------------------
# CI runs Invoke-ScriptAnalyzer over scripts/ at Error and Warning, so a
# suppression here is the difference between a red gate and a green one. Each
# is scoped to ONE rule and carries its justification. ⛔ Do not replace these
# with a settings file that switches the rule off everywhere: that weakens the
# gate for every future script to spare this one, which is how a check stops
# checking.
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingWriteHost', '',
    Justification = 'This is an interactive console tool and its progress output is for a human at a terminal, which is the documented case for Write-Host. The ONE value another script consumes is the address -Action HostAddress prints, and that goes through Write-Output precisely so it is the only thing on stdout; Write-Host would put it on the information stream, where a caller assigning the result gets nothing. Every other action reports through the exit code.')]
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSReviewUnusedParameter', '',
    Justification = 'Image, Tarball, Command, CommandFile, CommandB64, User, Ephemeral, OciEnv, Systemd, Verbatim, ScriptArg, NoTimestamps, TimestampMode, TimestampFormat, TimestampColumns, TimestampSeparator, TimestampProfile, PrefixOnly, Color, StreamLogPath, StreamLogOverwrite, EventLog, Redact, MaxLineBytes, TickSeconds, TickEscalateSeconds, CommandTimeoutSeconds, DryRun and Force are read by the Invoke-Action* and stream-log functions through script scope rather than as arguments. The analyzer does not follow that, and threading thirty parameters through every call to satisfy it would make the code worse.')]
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseSingularNouns', '',
    Justification = 'Get-WslDistroNames returns the whole list and Export-ImageRootfs writes one rootfs whose name simply ends in s. Renaming either to satisfy the rule would make the name describe the thing less accurately.')]
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '',
    Justification = 'Removal already goes through Confirm-Destructive, which refuses non-interactively unless -Force was passed. Adding ShouldProcess would give a second, differently spelled confirmation path over the same guard, and two confirmation mechanisms is how one of them gets bypassed.')]
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('New', 'Run', 'Enter', 'List', 'Remove', 'Purge', 'Resources', 'HostAddress', 'Doctor')]
    [string]$Action,

    [string]$Image,
    [string]$Tarball,
    [string]$Name,
    [string]$Command,
    [string]$CommandFile,
    [string]$CommandB64,
    [string]$User = 'root',
    [switch]$Ephemeral,
    [switch]$OciEnv,
    [switch]$Systemd,
    # How long the SCRIPT'S OWN questions to a distro may take before it gives
    # up. ⛔ The caller's -Command is deliberately NOT bounded by it: a build
    # that runs for an hour is a legitimate command and a tool that kills it at
    # two minutes is broken. What is bounded is every question this script asks
    # for itself, which is where a wedged init hangs it with no output.
    #
    # ⛔ READ AS $script:TimeoutSeconds BY THE FUNCTIONS BELOW, and there must
    # be NO CONSTANT OF THAT NAME. A script parameter IS a script-scoped
    # variable, so a `$script:TimeoutSeconds = 120` in the constants block
    # further down silently overwrote whatever the caller passed. It did: this
    # shipped for one run, -TimeoutSeconds 15 timed out at 120, and the
    # acceptance caught it because it asserted the number and not just the
    # refusal.
    [ValidateRange(5, 3600)]
    [int]$TimeoutSeconds = 120,

    # THE STREAM LOG, WHICH IS ON BY DEFAULT. A command that prints nothing for
    # twenty minutes and a command that has died look identical to a caller
    # reading a pipe, and there is no way to tell them apart after the fact.
    # With the log on, silence is itself a line: the tick says the distro is
    # still there, how long it has been quiet, and how much has come out so far.
    #
    # -NoTimestamps IS THE ONE SWITCH THAT TURNS ALL OF IT OFF, and it restores
    # the previous behaviour exactly: the child inherits this process's handles
    # and its bytes reach the terminal untouched. Nothing is relayed, so nothing
    # can be re-encoded, re-ordered or buffered by this script.
    [switch]$NoTimestamps,
    [ValidateSet('Relative', 'Delta', 'Wall', 'Iso', 'Epoch')]
    [string]$TimestampMode = 'Relative',
    # strftime, with tss's specifier set. It applies to Relative and Wall, which
    # are the two modes that have a format; passing it with Delta, Iso or Epoch
    # is REFUSED rather than ignored, because a parameter silently doing nothing
    # is a caller believing they asked for something.
    [string]$TimestampFormat,
    # Seconds of SILENCE before a heartbeat line. 0 turns the heartbeat off and
    # leaves the timestamps on.
    [ValidateRange(0, 86400)]
    [int]$TickSeconds = 30,
    # A bound on the caller's own command. NO DEFAULT, and that is deliberate:
    # -TimeoutSeconds above bounds the questions this script asks for itself,
    # and a build that runs for an hour is a legitimate command. This exists
    # because the caller sometimes knows a bound the script cannot, and without
    # one the only way out of a wedged command is killing the session. Exit 124,
    # as coreutils' timeout reports it.
    [ValidateRange(0, 604800)]
    [int]$CommandTimeoutSeconds = 0,

    # -CommandFile carries a file's bytes. By default the COPY IN TRANSIT has
    # its CRLF line endings turned into LF and a UTF-8 byte order mark removed,
    # and it says what it changed. -Verbatim sends the bytes exactly as they
    # are. THE FILE ON DISK IS NEVER WRITTEN TO IN EITHER CASE.
    [switch]$Verbatim,
    # NAME=VALUE, prepended to the command as POSIX-quoted shell assignments. It
    # exists so a caller stops running sed over their own script to inject a
    # value, which is a string edit that can corrupt the payload it is editing.
    # The literal token @hostaddress in a VALUE is replaced with what
    # -Action HostAddress would print.
    #
    # ⛔ ONE PAIR PER INVOCATION WHEN THIS SCRIPT IS RUN THROUGH -File, WHICH IS
    # HOW EVERY CONSUMER RUNS IT. This was documented as repeatable and it is
    # not: measured on 2026-08-30 under PowerShell 7.6.5 and Windows PowerShell
    # 5.1, `-ScriptArg A=1 -ScriptArg B=2` is refused with "parameter
    # 'ScriptArg' is specified more than once", directly and through the
    # launcher, which splats the same argument list. ⭐ For more than one pair
    # use -ScriptArgFile, which has no delimiter to be ambiguous about. An
    # in-process caller passing a real array still gets every element.
    [string[]]$ScriptArg,
    # A file of NAME=VALUE lines, one per line, applied before -ScriptArg.
    # Blank lines and lines starting with # are skipped. Its bytes go through
    # the same repair as -CommandFile: CRLF to LF, a byte order mark dropped,
    # UTF-16 refused by name. ⛔ The file on disk is never written to.
    [string]$ScriptArgFile,

    # -- how the stream log RENDERS, which is a different question from what it
    #    measures. Everything below changes the prefix or where a copy of the
    #    log goes, and none of it changes the guest's own bytes.
    #
    # ⭐ COMPOSED COLUMNS, which the single-valued -TimestampMode cannot express.
    # 'rel,delta' is the pair that finds a stall: every line carries how far into
    # the run it is AND how long since the previous one, so a 50,000-line log has
    # one obviously interesting row and `awk` can find it. Passing this together
    # with -TimestampMode is refused, because they are two spellings of one
    # decision and a precedence between them is a rule nobody would remember.
    [string[]]$TimestampColumns,
    # What goes between the stamp and the tag. tss spells it -s.
    [string]$TimestampSeparator = ' ',
    # The prefix and nothing else. For a command whose output is enormous, or
    # whose content must not reach a log, where the CADENCE is still the thing
    # worth watching.
    [switch]$PrefixOnly,
    # ⚠ 'auto' means colour only when stdout is a real console AND NO_COLOR is
    # unset. A file sink NEVER gets colour whatever this says: escape sequences
    # in a log file are what makes a later grep answer wrong.
    [ValidateSet('auto', 'always', 'never')]
    [string]$Color = 'auto',
    # Presets. Anything passed explicitly beside one wins over it, so a profile
    # is a starting point rather than a mode.
    #   human    the default. Relative milliseconds, colour if this is a console.
    #   ci       relative and delta, no colour. For a log a person reads later.
    #   forensic wall, relative and delta, microseconds. For correlating with
    #            another system's log.
    #   wall     tss's own defaults, byte for byte.
    #   raw      the same as -NoTimestamps.
    [ValidateSet('human', 'ci', 'forensic', 'wall', 'raw')]
    [string]$TimestampProfile,
    # A copy of the rendered log, appended. tss spells these -o and
    # --force-overwrite. ⛔ A Windows reserved device name is refused BY NAME:
    # 'nul' silently discards everything written to it and 'con' writes to the
    # console, and either one is a caller believing they have a log file.
    [string]$StreamLogPath,
    [switch]$StreamLogOverwrite,
    # ⭐ One JSON object per line, one per event, for a caller that is a program.
    # The rendered log is a view over this: a field the terminal shows and this
    # does not would be a renderer that knows something the record does not.
    # ⛔ Refused with -NoTimestamps, which turns the relay off entirely: with no
    # relay there are no events to record, and writing an empty file would be
    # the quietest possible lie.
    [string]$EventLog,
    # A .NET regex, or several separated by commas, replaced with '***' before
    # ANY sink.
    # ⭐ IT SPLITS ON COMMAS ITSELF because a script run through -File cannot be
    # given a real array and cannot have a parameter repeated. A pattern that
    # needs a literal comma writes it as the character class [,], which matches
    # exactly the same thing and contains no delimiter.
    # ⚠ Best-effort and it says so on the page: a secret split across a
    # carriage-return redraw is two partial lines and matches neither. It
    # reduces accidental disclosure in a log you were keeping anyway. It is not
    # a control to rely on for a secret that matters.
    [string[]]$Redact,
    # Truncate the DETAIL at this many bytes, appending how many were dropped.
    # 0 never truncates. The prefix is never counted and never cut.
    [ValidateRange(0, 1048576)]
    [int]$MaxLineBytes = 0,
    # ⭐ Silence thresholds, in seconds, comma-separated, at which the tick says
    # MORE rather than the same thing again. At the first step it says what the
    # relay costs; at the second it says what it can and cannot conclude and
    # names the evidence. An empty value turns escalation off and leaves the
    # plain tick.
    #
    # ⛔ [string[]] AND NOT [int[]], AND THAT IS THE FIX FOR A MEASURED SILENT
    # WRONG ANSWER. Through -File, `-TickEscalateSeconds 5,9` arrives as the one
    # string "5,9", and PowerShell converts a string to an int with the current
    # culture's number style, where a comma is the THOUSANDS separator. So an
    # [int[]] parameter bound it to the single value 59 and the escalation never
    # fired, with nothing said. Measured under both PowerShell hosts on
    # 2026-08-30. Taking strings and parsing them here turns that into two
    # values, and turns anything that is not an integer into a refusal.
    [string[]]$TickEscalateSeconds = @('120', '300', '900'),
    # Print the wsl.exe command line that would run, and the state that would be
    # changed, then stop. ⛔ Nothing is created, imported, written or removed.
    [switch]$DryRun,

    [switch]$Force
)

