#Requires -Version 5.1

<#
.SYNOPSIS
    Run wsl-toolkit.ps1's pure functions against a table of cases, on any
    Windows host, without WSL and without creating anything.

.DESCRIPTION
    THE DEFECT IT EXISTS TO CATCH. Most of wsl-toolkit.ps1 can only be proved
    by driving a real distro, which needs WSL, a container engine, several
    hundred megabytes and a minute. The parts that decide what a caller SEES do
    not: the timestamp renderer, the line splitter that makes a progress bar
    visible, the file channel that repairs a CRLF payload, the argument
    prologue, and the transport alphabet. Each of those is a pure function of
    its input, each has a wrong answer that looks right, and before this file
    none of them had a test at all.

    It is a test rather than a check by scripts/README.md's contract, and it
    still satisfies that contract: exit 0 pass, 1 fail, 2 could not run, a -Json
    switch, no dependence on the working directory, and it writes nothing.

    HOW IT LOADS THE FUNCTIONS. wsl-toolkit.ps1 cannot be dot-sourced: its top
    level dispatches an action and calls exit, which would end this session. So
    this parses the file, takes the function definitions it names, and defines
    those alone. THE NUMBER IT FOUND IS ASSERTED against the number it asked
    for, because a renamed function would otherwise silently drop its cases and
    leave a smaller suite reporting green.

    WHAT IT DOES NOT COVER, said plainly rather than left to be assumed:
    anything that talks to wsl.exe, to a container engine, or to the filesystem.
    Invoke-InDistroLogged, the disk preflight and every destructive path are
    proved by running them, which is part (b) of docs/methodology/gate.md and
    not this file's job.

.PARAMETER Json
    One line of JSON: the schema, the number of cases, and the number that
    failed. For a gate runner rather than for a person.

.EXAMPLE
    pwsh -NoProfile -File scripts\windows\wsl-toolkit\selftest.ps1

.NOTES
    ASCII-ONLY ON PURPOSE, like the launcher beside it.
    docs/conventions/shell.md section 8: a .ps1 holding any non-ASCII byte needs
    a UTF-8 BOM before Windows PowerShell 5.1 decodes it correctly. Staying
    ASCII removes the requirement rather than depending on it.

    Requires : Windows PowerShell 5.1 or PowerShell 7+. No WSL, no engine.
#>

[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseSingularNouns', '',
    Justification = 'Get-TestSettings builds the whole settings object the stream log reads, and the real function it stands in for is Resolve-StreamLogSettings. Renaming it to a singular would make it describe something it does not return.')]
[CmdletBinding(PositionalBinding = $false)]
param([switch]$Json)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Target = Join-Path (Split-Path -Parent $PSCommandPath) 'wsl-toolkit.ps1'
if (-not (Test-Path -LiteralPath $Target -PathType Leaf)) {
    [Console]::Error.WriteLine("selftest: $Target is not there, so there is nothing to test.")
    exit 2
}

# -- load the functions under test -------------------------------------------
$Wanted = @(
    'Format-StrftimeStamp'
    'Get-StampDefaultFormat'
    'Format-ByteCount'
    'Format-Duration'
    'Split-StreamChunk'
    'ConvertTo-ShellSingleQuoted'
    'ConvertTo-DistroScriptCommand'
    'New-GuestScratchPath'
    'ConvertTo-NativeArgumentString'
    'ConvertFrom-CommandFileBytes'
    'ConvertTo-ScriptArgPrologue'
    'ConvertTo-Utf8Bytes'
    'Resolve-CommandBytes'
    'New-StreamLogState'
    'Write-StreamLogLine'
    'Resolve-StampColumns'
    'Format-StampColumn'
    'Get-ColumnFormat'
    'Test-ColumnTakesFormat'
    'Resolve-StreamLogSettings'
    'Format-StreamLogPrefix'
    'New-RedactionSet'
    'Invoke-Redaction'
    'Limit-LineBytes'
    'Get-ExitCodeDiagnosis'
    'Get-ParameterApplicability'
    'Assert-ParametersApplyToAction'
    'Split-DelimitedArgument'
    'Get-ScriptArgPairs'
    'Assert-SinkPathIsUsable'
)

$parseErrors = $null
$tokens = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
    (Resolve-Path -LiteralPath $Target).Path, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors -and $parseErrors.Count -gt 0) {
    [Console]::Error.WriteLine("selftest: $Target does not parse; check-powershell.ps1 owns that verdict.")
    exit 1
}

$found = @{}
foreach ($fn in $ast.FindAll({ $args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $true)) {
    if ($Wanted -contains $fn.Name) { $found[$fn.Name] = $fn.Extent.Text }
}

$missing = @($Wanted | Where-Object { -not $found.ContainsKey($_) })
if ($missing.Count -gt 0) {
    [Console]::Error.WriteLine("selftest: these functions are named here and are not in $Target : " + ($missing -join ', '))
    [Console]::Error.WriteLine("  A rename is not a failure of the tool, it is a failure of this file to follow it. Fix the list, do not delete the cases.")
    exit 1
}

foreach ($name in $Wanted) {
    # [ScriptBlock]::Create and a dot-source, rather than Invoke-Expression:
    # the same effect, and it does not reach for the cmdlet whose every other
    # use is a defect.
    . ([ScriptBlock]::Create($found[$name]))
}

# -- the constants those functions read, taken from the same file -------------
# THE DEFECT THIS EXISTS FOR IS A TEST THAT PASSES FOR THE WRONG REASON. The
# reserved-device cases were written before this, so Assert-SinkPathIsUsable
# threw on an unset $script:ReservedDeviceNames rather than on the device name,
# and every case expecting a refusal was green over a function that had never
# reached its own guard. The two cases expecting SUCCESS are what exposed it.
#
# It is loaded rather than copied for the reason a copy is always wrong: a
# second list here is a second place for the set to be right, and one of them
# would go stale.
$WantedConstant = @('ReservedDeviceNames')
foreach ($cname in $WantedConstant) {
    $hit = @($ast.FindAll({
        $args[0] -is [System.Management.Automation.Language.AssignmentStatementAst] -and
        $args[0].Left.Extent.Text -eq ('$script:' + $cname)
    }, $false))
    if ($hit.Count -ne 1) {
        [Console]::Error.WriteLine("selftest: expected exactly one assignment to `$script:$cname in $Target, found $($hit.Count).")
        exit 1
    }
    . ([ScriptBlock]::Create($hit[0].Extent.Text))
}

# -- the doubles, and what each one stands in for ----------------------------
# The functions above call these. They are replaced rather than loaded so a case
# can read what was reported, and so nothing here touches a network interface.
$script:Reported = @()
function Write-Step { param([string]$Message) $script:Reported += "step: $Message" }
function Write-Ok   { param([string]$Message) $script:Reported += "ok: $Message" }
function Write-Warn { param([string]$Message) $script:Reported += "warn: $Message" }
function Write-Note { param([string]$Message) $script:Reported += "note: $Message" }
function Resolve-HostAddress {
    # A fixed answer. The real one reads a network interface, and this file is
    # documented to touch nothing; -Action HostAddress on a real host is what
    # proves that one.
    return [pscustomobject]@{ Address = '172.23.96.1'; Mode = 'nat'; Source = 'a double'; Path = $null; Interface = 'vEthernet (WSL)' }
}

# Script-scoped parameters the loaded functions read. In wsl-toolkit.ps1 these
# are its own param() block; here a case sets them before it runs.
$script:Verbatim = $false
$script:ScriptArg = @()
$script:TimestampMode = 'Relative'
$script:TimestampFormat = ''

# -- the harness -------------------------------------------------------------
$Results = @()
function Test-Case {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Expect,
        [Parameter(Mandatory = $true)][scriptblock]$Body
    )
    $script:Reported = @()
    try { $actual = [string](& $Body) }
    catch { $actual = 'UNEXPECTED THROW: ' + $_.Exception.Message }
    # -cne: case matters here more than anywhere, because the case-sensitivity
    # of the strftime specifiers is one of the things under test.
    $script:Results += [pscustomobject]@{ Name = $Name; Expect = $Expect; Actual = $actual; Pass = ($actual -ceq $Expect) }
}

function Format-ByteDump {
    param([AllowEmptyCollection()][byte[]]$Bytes)
    if ($null -eq $Bytes -or $Bytes.Length -eq 0) { return '(empty)' }
    return (($Bytes | ForEach-Object { $_.ToString('x2') }) -join ' ')
}

function Get-Utf8 {
    param([AllowEmptyString()][string]$Text)
    return , [Text.Encoding]::UTF8.GetBytes($Text)
}

# -- Format-StrftimeStamp ----------------------------------------------------
$fixedWall = [datetimeoffset]::new(2026, 8, 30, 4, 56, 24, 123, [timespan]::FromMinutes(345))
$zero = [timespan]::Zero

Test-Case 'relative renders hours, minutes, seconds and milliseconds' '01:02:03.456' {
    Format-StrftimeStamp -Format '%H:%M:%S.%3f' -Wall $fixedWall -Elapsed ([timespan]::new(0, 1, 2, 3, 456)) -Mode 'Relative'
}
Test-Case 'relative hours are TOTAL hours and do not wrap at a day' '25:00:01.000' {
    Format-StrftimeStamp -Format '%H:%M:%S.%3f' -Wall $fixedWall -Elapsed ([timespan]::new(1, 1, 0, 1, 0)) -Mode 'Relative'
}
Test-Case 'wall renders tss default format' '2026-08-30 04:56:24' {
    Format-StrftimeStamp -Format '%Y-%m-%d %H:%M:%S' -Wall $fixedWall -Elapsed $zero -Mode 'Wall'
}
# The regression this pair exists for: a PowerShell hashtable would fold %m into
# %M, so a month and a minute would render the same value and the format would
# be silently wrong on every line of every log.
Test-Case 'lowercase m is the month' '08' {
    Format-StrftimeStamp -Format '%m' -Wall $fixedWall -Elapsed $zero -Mode 'Wall'
}
Test-Case 'uppercase M is the minute' '56' {
    Format-StrftimeStamp -Format '%M' -Wall $fixedWall -Elapsed $zero -Mode 'Wall'
}
Test-Case 'lowercase z is the offset and uppercase Z is not' 'different' {
    $a = Format-StrftimeStamp -Format '%z' -Wall $fixedWall -Elapsed $zero -Mode 'Wall'
    $b = Format-StrftimeStamp -Format '%Z' -Wall $fixedWall -Elapsed $zero -Mode 'Wall'
    if ($a -ceq '+05:45' -and $b -cne $a) { 'different' } else { "a=$a b=$b" }
}
Test-Case 'a literal percent is written as two' '100%' {
    Format-StrftimeStamp -Format '100%%' -Wall $fixedWall -Elapsed $zero -Mode 'Wall'
}
Test-Case 'microseconds are six digits' '123000' {
    Format-StrftimeStamp -Format '%6f' -Wall $fixedWall -Elapsed $zero -Mode 'Wall'
}
Test-Case 'nanoseconds are nine digits whose last two are padding' '123000000 padded' {
    $v = Format-StrftimeStamp -Format '%9f' -Wall $fixedWall -Elapsed $zero -Mode 'Wall'
    if ($v.Length -eq 9 -and $v.EndsWith('00')) { "$v padded" } else { $v }
}
Test-Case 'an unknown specifier is refused by name' 'threw naming q' {
    try { $null = Format-StrftimeStamp -Format '%q' -Wall $fixedWall -Elapsed $zero -Mode 'Wall'; 'no throw' }
    catch { if ($_.Exception.Message -clike "*'%q'*") { 'threw naming q' } else { 'threw: ' + $_.Exception.Message } }
}
Test-Case 'a year has no meaning in relative mode and is refused' 'threw naming Y' {
    try { $null = Format-StrftimeStamp -Format '%Y' -Wall $fixedWall -Elapsed $zero -Mode 'Relative'; 'no throw' }
    catch { if ($_.Exception.Message -clike "*'%Y'*") { 'threw naming Y' } else { 'threw: ' + $_.Exception.Message } }
}
Test-Case 'a trailing bare percent is refused' 'threw' {
    try { $null = Format-StrftimeStamp -Format 'x%' -Wall $fixedWall -Elapsed $zero -Mode 'Wall'; 'no throw' }
    catch { 'threw' }
}
# THE CULTURE CASE, and it is the one that could not be found by reading. A .NET
# custom format string treats ':' as a culture-dependent placeholder, so the
# obvious implementation renders a different separator on a host whose culture
# names one. This renders under a culture that does exactly that and asserts the
# colon survived.
Test-Case 'a literal colon survives a culture that would replace it' '04:56:24 under fi-FI' {
    $prev = [Threading.Thread]::CurrentThread.CurrentCulture
    try {
        [Threading.Thread]::CurrentThread.CurrentCulture = [Globalization.CultureInfo]::GetCultureInfo('fi-FI')
        $v = Format-StrftimeStamp -Format '%H:%M:%S' -Wall $fixedWall -Elapsed $zero -Mode 'Wall'
        "$v under fi-FI"
    }
    finally { [Threading.Thread]::CurrentThread.CurrentCulture = $prev }
}
Test-Case 'the default format differs by mode' 'wall dated, relative not' {
    $w = Get-StampDefaultFormat -Mode 'Wall'
    $r = Get-StampDefaultFormat -Mode 'Relative'
    if ($w -clike '*%Y*' -and $r -cnotlike '*%Y*') { 'wall dated, relative not' } else { "w=$w r=$r" }
}

# -- Split-StreamChunk -------------------------------------------------------
function Show-Split {
    param([string]$Pending)
    $rest = ''
    $lines = Split-StreamChunk -Pending $Pending -Remainder ([ref]$rest)
    $parts = @()
    foreach ($l in $lines) { $parts += (($(if ($l.Partial) { '~' } else { '=' })) + $l.Text) }
    return '[' + ($parts -join '|') + '] rest=<' + $rest + '>'
}

Test-Case 'two newline-terminated lines' '[=a|=b] rest=<>' { Show-Split "a`nb`n" }
Test-Case 'an unterminated tail is held back' '[=a] rest=<b>' { Show-Split "a`nb" }
Test-Case 'CRLF terminates once, not twice' '[=a|=b] rest=<>' { Show-Split "a`r`nb`r`n" }
# The progress-bar case. A line-oriented reader shows nothing at all here, which
# is the silence the whole stream log exists to remove.
Test-Case 'a carriage return ends a line and marks it unterminated' ('[~50%] rest=<75%' + [char]13 + '>') {
    Show-Split ("50%" + [char]13 + "75%" + [char]13)
}
Test-Case 'a lone trailing CR is held, never emitted' ('[] rest=<x' + [char]13 + '>') {
    Show-Split ("x" + [char]13)
}
# THE REGRESSION THE HELD CR EXISTS FOR: a CRLF split across two reads must not
# become two lines.
Test-Case 'a CRLF split across two reads stays one line' '[=abc|=def] rest=<>' {
    $rest = ''
    $null = Split-StreamChunk -Pending ("abc" + [char]13) -Remainder ([ref]$rest)
    Show-Split ($rest + "`ndef`n")
}
Test-Case 'an empty chunk produces nothing' '[] rest=<>' { Show-Split '' }
Test-Case 'an empty line is a line' '[=|=a] rest=<>' { Show-Split "`na`n" }

# -- Format-Duration and Format-ByteCount -----------------------------------
Test-Case 'seconds under a minute' '0s 59s' {
    (Format-Duration -Span ([timespan]::FromSeconds(0))) + ' ' + (Format-Duration -Span ([timespan]::FromSeconds(59)))
}
Test-Case 'minutes carry zero-padded seconds' '1m00s 59m59s' {
    (Format-Duration -Span ([timespan]::FromSeconds(60))) + ' ' + (Format-Duration -Span ([timespan]::FromSeconds(3599)))
}
Test-Case 'hours carry zero-padded minutes' '1h00m 1h30m' {
    (Format-Duration -Span ([timespan]::FromSeconds(3600))) + ' ' + (Format-Duration -Span ([timespan]::FromSeconds(5445)))
}
Test-Case 'bytes below a kibibyte are bytes' '0 B 1023 B' {
    (Format-ByteCount -Bytes 0) + ' ' + (Format-ByteCount -Bytes 1023)
}
Test-Case 'binary units, labelled as binary' '1.0 KiB 1.0 MiB' {
    (Format-ByteCount -Bytes 1024) + ' ' + (Format-ByteCount -Bytes 1048576)
}

# -- the transport alphabet --------------------------------------------------
Test-Case 'a payload full of hazards still yields an alphabet-clean line' 'clean' {
    $payload = 'echo "$PATH" `date` ' + [char]39 + 'quoted' + [char]39 + " tab`there"
    $line = ConvertTo-DistroScriptCommand -ScriptBytes (ConvertTo-Utf8Bytes -Text $payload) -GuestPath '/tmp/.wsl-eph-abc12345'
    if ($line -cmatch '[^A-Za-z0-9+/=|<>&;. _-]') { 'DIRTY: ' + $Matches[0] } else { 'clean' }
}
# TWO GUARDS SIT HERE AND THIS CASE REACHES ONLY ONE OF THEM, which is worth
# saying because the case used to be named as though it reached both. The
# GuestPath pattern check is what refuses the line below. The second guard, the
# assert over the finished skeleton, exists for an EDIT to that skeleton and
# cannot be reached from outside the function: nothing a caller passes can put a
# character into it. It was proved by mutation instead, on 2026-08-30 - a dollar
# sign planted in the skeleton string turned the alphabet case above red - and
# that is recorded here rather than dressed up as a case.
Test-Case 'a guest path outside the pattern is refused' 'threw' {
    try { $null = ConvertTo-DistroScriptCommand -ScriptBytes (ConvertTo-Utf8Bytes -Text 'true') -GuestPath '/tmp/$(id)'; 'no throw' }
    catch { 'threw' }
}
Test-Case 'a generated guest path is inside the alphabet' 'inside' {
    $p = New-GuestScratchPath
    if ($p -cmatch '^/tmp/\.wsl-eph-[a-z0-9]{8}$') { 'inside' } else { "outside: $p" }
}
Test-Case 'an argument carrying a quote is refused, never escaped' 'threw' {
    try { $null = ConvertTo-NativeArgumentString -Arguments @('a"b'); 'no throw' } catch { 'threw' }
}
Test-Case 'an argument carrying a backslash is refused' 'threw' {
    try { $null = ConvertTo-NativeArgumentString -Arguments @('a\b'); 'no throw' } catch { 'threw' }
}
Test-Case 'an argument with a space is quoted' '-d "eph a" -u root' {
    ConvertTo-NativeArgumentString -Arguments @('-d', 'eph a', '-u', 'root')
}
Test-Case 'a single quote is closed, escaped and reopened' ("'a'\''b'") {
    ConvertTo-ShellSingleQuoted -Raw "a'b"
}

# -- the file channel --------------------------------------------------------
Test-Case 'an LF file is passed through unchanged' '61 0a 62 0a' {
    Format-ByteDump (ConvertFrom-CommandFileBytes -Bytes (Get-Utf8 "a`nb`n") -Source 'f')
}
Test-Case 'CRLF becomes LF in the copy being sent' '61 0a 62 0a' {
    Format-ByteDump (ConvertFrom-CommandFileBytes -Bytes (Get-Utf8 "a`r`nb`r`n") -Source 'f')
}
Test-Case 'the CRLF repair says how many it found and that the file was not touched' 'reported 2 and NOT modified' {
    $null = ConvertFrom-CommandFileBytes -Bytes (Get-Utf8 "a`r`nb`r`n") -Source 'f'
    $said = ($script:Reported -join ' ')
    if ($said -clike '*2 CRLF*' -and $said -clike '*NOT modified*') { 'reported 2 and NOT modified' } else { $said }
}
Test-Case 'a UTF-8 byte order mark is left out of the copy' '61 0a' {
    Format-ByteDump (ConvertFrom-CommandFileBytes -Bytes ([byte[]]@(0xEF, 0xBB, 0xBF, 0x61, 0x0A)) -Source 'f')
}
# A lone CR is a deliberate byte, not damage. Turning it into a newline would
# edit the payload rather than repair it.
Test-Case 'a lone carriage return is preserved' '61 0d 62 0a' {
    Format-ByteDump (ConvertFrom-CommandFileBytes -Bytes (Get-Utf8 ("a" + [char]13 + "b`n")) -Source 'f')
}
Test-Case 'UTF-16 is refused by name rather than sent to die on a NUL' 'threw naming UTF-16' {
    try { $null = ConvertFrom-CommandFileBytes -Bytes ([byte[]]@(0xFF, 0xFE, 0x61, 0x00)) -Source 'f'; 'no throw' }
    catch { if ($_.Exception.Message -clike '*UTF-16*') { 'threw naming UTF-16' } else { 'threw: ' + $_.Exception.Message } }
}
Test-Case 'an empty file is an empty command, not an error' '(empty)' {
    Format-ByteDump (ConvertFrom-CommandFileBytes -Bytes ([byte[]]@()) -Source 'f')
}
Test-Case '-Verbatim sends the bytes exactly, mark and carriage returns included' 'ef bb bf 61 0d 0a' {
    $script:Verbatim = $true
    try { Format-ByteDump (ConvertFrom-CommandFileBytes -Bytes ([byte[]]@(0xEF, 0xBB, 0xBF, 0x61, 0x0D, 0x0A)) -Source 'f') }
    finally { $script:Verbatim = $false }
}
Test-Case '-Verbatim still warns about what it is about to send' 'warned' {
    $script:Verbatim = $true
    try {
        $null = ConvertFrom-CommandFileBytes -Bytes (Get-Utf8 "a`r`n") -Source 'f'
        if (($script:Reported -join ' ') -clike '*carriage returns*') { 'warned' } else { ($script:Reported -join ' ') }
    }
    finally { $script:Verbatim = $false }
}

# -- the argument prologue ---------------------------------------------------
Test-Case 'a value is assigned and exported, never substituted' "URL='https://x/a&b';export URL;" {
    (ConvertTo-ScriptArgPrologue -Pairs @('URL=https://x/a&b')) -replace "`n", ';'
}
Test-Case 'a value carrying a single quote is escaped, not broken' ("A='it'\''s';export A;") {
    (ConvertTo-ScriptArgPrologue -Pairs @("A=it's")) -replace "`n", ';'
}
Test-Case 'a value carrying shell metacharacters is left alone inside the quotes' ('B=' + "'" + '$X `id` "q" $(x)' + "'" + ';export B;') {
    (ConvertTo-ScriptArgPrologue -Pairs @('B=$X `id` "q" $(x)')) -replace "`n", ';'
}
Test-Case 'an empty value is an empty string, not a missing one' "A='';export A;" {
    (ConvertTo-ScriptArgPrologue -Pairs @('A=')) -replace "`n", ';'
}
Test-Case 'a name that is not a shell variable name is refused' 'threw' {
    try { $null = ConvertTo-ScriptArgPrologue -Pairs @('1BAD=x'); 'no throw' } catch { 'threw' }
}
Test-Case 'a pair with no equals sign is refused' 'threw' {
    try { $null = ConvertTo-ScriptArgPrologue -Pairs @('NOEQUALS'); 'no throw' } catch { 'threw' }
}
Test-Case 'a pair beginning with an equals sign is refused' 'threw' {
    try { $null = ConvertTo-ScriptArgPrologue -Pairs @('=x'); 'no throw' } catch { 'threw' }
}
Test-Case 'the host address token is expanded inside a value' "U='https://172.23.96.1:443/';export U;" {
    (ConvertTo-ScriptArgPrologue -Pairs @('U=https://@hostaddress:443/')) -replace "`n", ';'
}

# -- Resolve-CommandBytes, where the three spellings meet --------------------
Test-Case 'the prologue is prepended to a command given as text' "X='1'`nexport X`ntrue" {
    # -Pairs is an ARGUMENT now, not a script-scoped read: -ScriptArg cannot be
    # repeated through -File, so Main resolves the file and the flag into one list.
    [Text.Encoding]::UTF8.GetString((Resolve-CommandBytes -Text 'true' -FromFile '' -FromB64 '' -Pairs @('X=1')))

}
Test-Case 'the prologue is prepended to a command given as base64, identically' "X='1'`nexport X`ntrue" {
    # -Pairs is an ARGUMENT now, not a script-scoped read: -ScriptArg cannot be
    # repeated through -File, so Main resolves the file and the flag into one list.
    $b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes('true'))
    [Text.Encoding]::UTF8.GetString((Resolve-CommandBytes -Text '' -FromFile '' -FromB64 $b64 -Pairs @('X=1')))
}
Test-Case 'no command at all is null, not an empty command' 'null' {
    $r = Resolve-CommandBytes -Text '' -FromFile '' -FromB64 ''
    if ($null -eq $r) { 'null' } else { 'not null' }
}
Test-Case '-ScriptArg with no command to carry is refused, never ignored' 'threw' {
    # -Pairs is an ARGUMENT now, not a script-scoped read: -ScriptArg cannot be
    # repeated through -File, so Main resolves the file and the flag into one list.
    try { $null = Resolve-CommandBytes -Text '' -FromFile '' -FromB64 '' -Pairs @('X=1'); 'no throw' }
    catch { 'threw' }

}
Test-Case 'two spellings of the command are refused' 'threw' {
    try { $null = Resolve-CommandBytes -Text 'a' -FromFile '' -FromB64 'YQ=='; 'no throw' } catch { 'threw' }
}
Test-Case '-Verbatim on a spelling whose bytes did not come off disk is refused' 'threw' {
    $script:Verbatim = $true
    try { $null = Resolve-CommandBytes -Text 'a' -FromFile '' -FromB64 ''; 'no throw' }
    catch { 'threw' }
    finally { $script:Verbatim = $false }
}
Test-Case 'base64 that is not base64 is refused' 'threw' {
    try { $null = Resolve-CommandBytes -Text '' -FromFile '' -FromB64 'not base64 at all!'; 'no throw' } catch { 'threw' }
}

# -- the line grammar, and the clock the delta column reads ------------------
# The state carries a Stopwatch, which cannot be made to say a chosen time. A
# stand-in with an Elapsed property is all Write-StreamLogLine reads, so these
# cases assert an exact rendering rather than a plausible-looking one.
function Get-TestSettings {
    # NOTE: THE REAL RESOLVER, not a hand-built object. A settings shape assembled
    # here would drift from the one Main builds, and the drift would be
    # invisible: every case would still pass against the wrong object.
    param([string]$Mode = 'Relative', [string[]]$Columns, [string]$Format = '',
          [string]$Separator = ' ', [switch]$PrefixOnly, [int]$MaxBytes = 0,
          [string[]]$Redact, [string]$Preset = '', [string[]]$Explicit = @())
    return Resolve-StreamLogSettings -Mode $Mode -Columns $Columns -Format $Format `
        -Separator $Separator -PrefixOnly:$PrefixOnly -Color 'never' -Preset $Preset `
        -RedactPatterns $Redact -MaxBytes $MaxBytes -TickSeconds 30 -Escalate @() `
        -Explicit $Explicit
}
function Get-TestLogState {
    param([string]$Mode = 'Relative', $Settings)
    if (-not $Settings) { $Settings = Get-TestSettings -Mode $Mode }
    $st = New-StreamLogState -DistroName 'eph-test' -Settings $Settings
    $st.Clock = [pscustomobject]@{ Elapsed = [timespan]::Zero }
    $st.Out = [IO.StringWriter]::new()
    $st.Err = [IO.StringWriter]::new()
    return $st
}

Test-Case 'a complete line pads the tag to four and an unterminated one marks it' 'out |out~' {
    $st = Get-TestLogState -Mode 'Epoch'
    Write-StreamLogLine -State $st -Tag 'out' -Text 'a'
    Write-StreamLogLine -State $st -Tag 'out' -Text 'b' -Partial
    # The tag is a FIXED FOUR-character field after the first space, which is
    # the property a downstream awk or grep depends on. Splitting on whitespace
    # would not notice it stopped being fixed.
    $tags = @()
    foreach ($l in ($st.Out.ToString() -split "`r?`n")) { if ($l) { $tags += $l.Substring($l.IndexOf(' ') + 1, 4) } }
    ($tags -join '|')
}
Test-Case 'guest stdout goes to stdout and guest stderr and the tick go to stderr' 'out=1 err=2' {
    $st = Get-TestLogState -Mode 'Epoch'
    Write-StreamLogLine -State $st -Tag 'out'  -Text 'a'
    Write-StreamLogLine -State $st -Tag 'err'  -Text 'b'
    Write-StreamLogLine -State $st -Tag 'tick' -Text 'c'
    $o = @($st.Out.ToString() -split "`r?`n" | Where-Object { $_ })
    $e = @($st.Err.ToString() -split "`r?`n" | Where-Object { $_ })
    "out=$($o.Count) err=$($e.Count)"
}
# THE REGRESSION. A tick advancing the delta clock made a five-second gap render
# as '+0.619' against the last tick, on a stream where the ticks are not even
# present. Delta means "since the previous LINE", and a tick is the absence of
# one.
Test-Case 'a tick does not advance the delta clock' '+1.000|+5.000' {
    $st = Get-TestLogState -Mode 'Delta'
    $st.Clock.Elapsed = [timespan]::FromSeconds(1)
    Write-StreamLogLine -State $st -Tag 'out' -Text 'one'
    $st.Clock.Elapsed = [timespan]::FromSeconds(3)
    Write-StreamLogLine -State $st -Tag 'tick' -Text 'quiet'
    $st.Clock.Elapsed = [timespan]::FromSeconds(6)
    Write-StreamLogLine -State $st -Tag 'out' -Text 'two'
    $stamps = @()
    foreach ($l in ($st.Out.ToString() -split "`r?`n")) { if ($l) { $stamps += $l.Split(' ')[0] } }
    ($stamps -join '|')
}
Test-Case 'the tick itself reads the same gap the delta column would' '+2.000' {
    $st = Get-TestLogState -Mode 'Delta'
    $st.Clock.Elapsed = [timespan]::FromSeconds(1)
    Write-StreamLogLine -State $st -Tag 'out' -Text 'one'
    $st.Clock.Elapsed = [timespan]::FromSeconds(3)
    Write-StreamLogLine -State $st -Tag 'tick' -Text 'quiet'
    ($st.Err.ToString() -split "`r?`n")[0].Split(' ')[0]
}
Test-Case 'relative mode stamps a line from the elapsed clock' '00:01:05.500 out  x' {
    $st = Get-TestLogState -Mode 'Relative'
    $st.Clock.Elapsed = [timespan]::FromSeconds(65.5)
    Write-StreamLogLine -State $st -Tag 'out' -Text 'x'
    ($st.Out.ToString() -split "`r?`n")[0]
}
$script:TimestampMode = 'Relative'

# -- composed columns, the separator, and prefix-only ------------------------
# The single-valued -TimestampMode cannot express 'rel,delta', and that pair is
# what makes a stall findable: every delta is sub-second and then one is not.
Test-Case 'rel and delta compose into one prefix' '00:00:06.000 +5.000 out  two' {
    $st = Get-TestLogState -Settings (Get-TestSettings -Columns @('rel', 'delta'))
    $st.Clock.Elapsed = [timespan]::FromSeconds(1)
    Write-StreamLogLine -State $st -Tag 'out' -Text 'one'
    $st.Clock.Elapsed = [timespan]::FromSeconds(6)
    Write-StreamLogLine -State $st -Tag 'out' -Text 'two'
    @($st.Out.ToString() -split "`r?`n" | Where-Object { $_ })[1]
}
Test-Case 'a comma-separated single argument is the same as several' 'rel|delta' {
    (Resolve-StampColumns -Columns @('rel,delta') -Mode 'Relative' -ModeWasPassed $false) -join '|'
}
Test-Case 'an unknown column is refused rather than dropped' 'threw' {
    try { $null = Resolve-StampColumns -Columns @('rel', 'moon') -Mode 'Relative' -ModeWasPassed $false; 'no throw' } catch { 'threw' }
}
Test-Case 'a column named twice is refused' 'threw' {
    try { $null = Resolve-StampColumns -Columns @('rel', 'rel') -Mode 'Relative' -ModeWasPassed $false; 'no throw' } catch { 'threw' }
}
# Two spellings of one decision. A precedence between them would be a rule a
# caller has to remember in order to predict their own output.
Test-Case 'TimestampColumns with TimestampMode is refused' 'threw' {
    try { $null = Resolve-StampColumns -Columns @('rel') -Mode 'Delta' -ModeWasPassed $true; 'no throw' } catch { 'threw' }
}
Test-Case 'no columns and no mode falls back to relative' 'rel' {
    (Resolve-StampColumns -Columns @() -Mode 'Relative' -ModeWasPassed $false) -join '|'
}
Test-Case 'the separator sits between the stamp and the tag' '00:00:01.000|out ' {
    $st = Get-TestLogState -Settings (Get-TestSettings -Separator '|')
    $st.Clock.Elapsed = [timespan]::FromSeconds(1)
    Write-StreamLogLine -State $st -Tag 'out' -Text 'x'
    @($st.Out.ToString() -split "`r?`n")[0].Substring(0, 17)
}
Test-Case 'prefix-only drops the guest text and keeps the prefix' '00:00:01.000 out ' {
    $st = Get-TestLogState -Settings (Get-TestSettings -PrefixOnly)
    $st.Clock.Elapsed = [timespan]::FromSeconds(1)
    Write-StreamLogLine -State $st -Tag 'out' -Text 'a secret nobody should see'
    @($st.Out.ToString() -split "`r?`n")[0]
}
# The tick and the note are the watcher's lines, so neither may advance the
# clock the delta column reads. The tick case above covers 'tick'; this covers
# the tag added for one-off watcher lines.
Test-Case 'a note does not advance the delta clock either' '+5.000' {
    $st = Get-TestLogState -Settings (Get-TestSettings -Columns @('delta'))
    $st.Clock.Elapsed = [timespan]::FromSeconds(1)
    Write-StreamLogLine -State $st -Tag 'out' -Text 'one'
    $st.Clock.Elapsed = [timespan]::FromSeconds(3)
    Write-StreamLogLine -State $st -Tag 'note' -Text 'watcher'
    $st.Clock.Elapsed = [timespan]::FromSeconds(6)
    Write-StreamLogLine -State $st -Tag 'out' -Text 'two'
    @($st.Out.ToString() -split "`r?`n" | Where-Object { $_ })[1].Split(' ')[0]
}
Test-Case 'the note tag pads to the same four characters as the others' 'note' {
    $st = Get-TestLogState
    Write-StreamLogLine -State $st -Tag 'note' -Text 'x'
    @($st.Err.ToString() -split "`r?`n")[0].Substring(13, 4)
}

# -- the profiles ------------------------------------------------------------
Test-Case 'the ci profile composes rel and delta' 'rel|delta' {
    (Get-TestSettings -Preset 'ci').Columns -join '|'
}
Test-Case 'the wall profile is tss defaults' 'wall|%Y-%m-%d %H:%M:%S' {
    $s = Get-TestSettings -Preset 'wall'
    ($s.Columns -join '|') + '|' + $s.Format
}
# A profile is a starting point, not a mode: what the caller actually typed wins
# over it. Without the -Explicit list a default is indistinguishable from a
# choice, and the profile would silently overrule a flag they passed.
Test-Case 'an explicit column list beats the profile it sits beside' 'epoch' {
    (Get-TestSettings -Preset 'ci' -Columns @('epoch') -Explicit @('TimestampColumns')).Columns -join '|'
}
Test-Case 'a format with no column that takes one is refused' 'threw' {
    try { $null = Get-TestSettings -Columns @('epoch') -Format '%H'; 'no throw' } catch { 'threw' }
}
Test-Case 'rel and wall are the two columns a format applies to' 'True|True|False|False|False' {
    (@('rel', 'wall', 'delta', 'iso', 'epoch') | ForEach-Object { Test-ColumnTakesFormat -Column $_ }) -join '|'
}

# -- redaction and the byte bound --------------------------------------------
Test-Case 'a matching pattern is replaced before any sink sees it' 'token=***' {
    Invoke-Redaction -Set (New-RedactionSet -Patterns @('ghp_[A-Za-z0-9]+')) -Text 'token=ghp_abc123'
}
Test-Case 'no patterns leaves the text exactly as it was' 'token=ghp_abc123' {
    Invoke-Redaction -Set (New-RedactionSet -Patterns @()) -Text 'token=ghp_abc123'
}
# In a .NET replacement STRING, a dollar sign followed by an apostrophe means
# everything after the match. A literal replacement carrying one would paste the
# rest of the line back in. The delegate is what makes that impossible.
Test-Case 'text around the match is not pasted back in by the replacement' 'a***b' {
    Invoke-Redaction -Set (New-RedactionSet -Patterns @('SECRET')) -Text 'aSECRETb'
}
Test-Case 'a pattern that cannot compile is refused when it is passed' 'threw' {
    try { $null = New-RedactionSet -Patterns @('(unclosed'); 'no throw' } catch { 'threw' }
}
Test-Case 'redaction runs on the line the sink is given' '***' {
    $st = Get-TestLogState -Settings (Get-TestSettings -Redact @('hunter2'))
    Write-StreamLogLine -State $st -Tag 'out' -Text 'hunter2'
    # 12 stamp + 1 separator + 4 tag + 1 = the first byte of the guest's text.
    @($st.Out.ToString() -split "`r?`n")[0].Substring(18)
}
Test-Case 'a line inside the bound is untouched' 'abcde' {
    Limit-LineBytes -Text 'abcde' -MaxBytes 10
}
Test-Case 'a bound of zero never truncates' 'abcde' {
    Limit-LineBytes -Text 'abcde' -MaxBytes 0
}
Test-Case 'a line past the bound says how much went' 'abc...(+2 bytes cut)' {
    Limit-LineBytes -Text 'abcde' -MaxBytes 3
}
# Counting bytes and cutting characters are different jobs. Cutting a byte array
# at an index splits a multi-byte character and produces a replacement character
# that was never in the guest's output.
Test-Case 'a multi-byte character is never cut in half' 'ab...(+2 bytes cut)' {
    Limit-LineBytes -Text ('ab' + [char]0x00E9) -MaxBytes 3
}

# -- the exit-code reading ---------------------------------------------------
Test-Case 'a zero exit gets no diagnosis at all' '' {
    [string](Get-ExitCodeDiagnosis -ExitCode 0 -DistroState 'Running')
}
Test-Case '137 is named as 128 plus nine rather than left as a number' 'True' {
    ((Get-ExitCodeDiagnosis -ExitCode 137 -DistroState 'Running') -like '*128+9*SIGKILL*').ToString()
}
# The one thing this process can actually rule on: its own timeout reports 124,
# so a 137 is never it.
Test-Case 'the diagnosis rules out this script as the sender of a signal' 'True' {
    ((Get-ExitCodeDiagnosis -ExitCode 137 -DistroState 'Running') -like '*did not send it*').ToString()
}
Test-Case '124 is named as this script own timeout' 'True' {
    ((Get-ExitCodeDiagnosis -ExitCode 124 -DistroState 'Stopped') -like '*CommandTimeoutSeconds*').ToString()
}
Test-Case 'an ordinary code is passed through without invention' 'True' {
    ((Get-ExitCodeDiagnosis -ExitCode 3 -DistroState 'Running') -like '*passed through unchanged*').ToString()
}

# -- which parameter applies to which action ---------------------------------
# THE TABLE IS ASSERTED AGAINST THE PARAMETER BLOCK, both ways. A parameter added
# without a row would be refused on every action, and a row naming a parameter
# that no longer exists would refuse nothing while looking correct.
Test-Case 'every parameter in the block has a row, and every row a parameter' 'in sync' {
    $errs = $null; $toks = $null
    $a = [System.Management.Automation.Language.Parser]::ParseFile(
        (Resolve-Path -LiteralPath $Target).Path, [ref]$toks, [ref]$errs)
    $params = @($a.ParamBlock.Parameters | ForEach-Object { $_.Name.VariablePath.UserPath } |
                Where-Object { $_ -ne 'Action' })
    $table = Get-ParameterApplicability
    $noRow = @($params | Where-Object { -not $table.Contains($_) })
    $noParam = @($table.Keys | Where-Object { $params -notcontains $_ })
    if ($noRow.Count -eq 0 -and $noParam.Count -eq 0) { 'in sync' }
    else { 'no row: ' + ($noRow -join ',') + ' | no parameter: ' + ($noParam -join ',') }
}
Test-Case 'a parameter the action does not read is refused' 'threw' {
    try { Assert-ParametersApplyToAction -Action 'List' -Passed @('Image'); 'no throw' } catch { 'threw' }
}
Test-Case 'the refusal names the actions that DO read it' 'True' {
    try { Assert-ParametersApplyToAction -Action 'List' -Passed @('Image'); 'no throw' }
    catch { ($_.Exception.Message -like '*read by -Action New*').ToString() }
}
# -TimeoutSeconds bounds the questions this script asks for itself, and on Run it
# asks none. The refusal points at the parameter the caller actually wanted.
Test-Case 'TimeoutSeconds on Run points at CommandTimeoutSeconds' 'True' {
    try { Assert-ParametersApplyToAction -Action 'Run' -Passed @('TimeoutSeconds'); 'no throw' }
    catch { ($_.Exception.Message -like '*-CommandTimeoutSeconds*').ToString() }
}
Test-Case 'a parameter the action does read is accepted' 'ok' {
    Assert-ParametersApplyToAction -Action 'New' -Passed @('Image', 'Ephemeral', 'Force'); 'ok'
}
Test-Case 'the common parameters PowerShell adds are never judged' 'ok' {
    Assert-ParametersApplyToAction -Action 'List' -Passed @('Verbose', 'ErrorAction'); 'ok'
}

# -- a list parameter a -File caller can actually pass -----------------------
# HARD RULE: THE MEASUREMENT BEHIND THESE CASES. Through `pwsh -File`, which is how every
# consumer runs this script: `-X 5,9` arrives as the ONE string "5,9"; `-X a -X b`
# is refused as "specified more than once"; `-X 5 9` is refused as positional.
# So a list parameter splits its own value, and an [int[]] parameter is worse
# than useless: PowerShell converts "5,9" to an int with the culture's number
# style, where a comma is the thousands separator, and binds 59.
Test-Case 'one comma-separated value becomes several' '5|9' {
    (Split-DelimitedArgument -Values @('5,9')) -join '|'
}
Test-Case 'a real array from an in-process caller still gets every element' 'a|b' {
    (Split-DelimitedArgument -Values @('a', 'b')) -join '|'
}
Test-Case 'whitespace around a piece is dropped and an empty piece is skipped' 'a|b' {
    (Split-DelimitedArgument -Values @(' a , , b ')) -join '|'
}
Test-Case 'escalation thresholds arrive as numbers, not as one fused number' '5|9' {
    (Get-TestSettings).PSObject.Properties | Out-Null
    (Resolve-StreamLogSettings -Escalate @('5,9') -Color 'never' -TickSeconds 3).Escalate -join '|'
}
Test-Case 'a threshold that is not a whole number is refused, never coerced' 'threw' {
    try { $null = Resolve-StreamLogSettings -Escalate @('5,soon') -Color 'never'; 'no throw' } catch { 'threw' }
}
Test-Case 'thresholds are sorted and de-duplicated' '5|9|12' {
    (Resolve-StreamLogSettings -Escalate @('12,5,9,5') -Color 'never').Escalate -join '|'
}

# -- -ScriptArgFile, which exists because -ScriptArg cannot be repeated -------
Test-Case 'a file of pairs and the flag land in one list, file first' 'A=1|B=2|C=3' {
    $f = Join-Path ([IO.Path]::GetTempPath()) ('wt-args-' + [guid]::NewGuid().ToString('N') + '.env')
    try {
        # CRLF on purpose: the file gets the same repair -CommandFile gets.
        [IO.File]::WriteAllText($f, "# a comment`r`n`r`nA=1`r`nB=2`r`n")
        (Get-ScriptArgPairs -FromFile $f -Pairs @('C=3')) -join '|'
    }
    finally { Remove-Item -LiteralPath $f -Force -ErrorAction SilentlyContinue }
}
Test-Case 'a line in the file that is not NAME=VALUE is refused with its number' 'threw' {
    $f = Join-Path ([IO.Path]::GetTempPath()) ('wt-args-' + [guid]::NewGuid().ToString('N') + '.env')
    try {
        [IO.File]::WriteAllText($f, "A=1`nNOTAPAIR`n")
        $null = Get-ScriptArgPairs -FromFile $f -Pairs @(); 'no throw'
    }
    catch { 'threw' }
    finally { Remove-Item -LiteralPath $f -Force -ErrorAction SilentlyContinue }
}
Test-Case 'a missing pairs file is refused rather than treated as empty' 'threw' {
    try { $null = Get-ScriptArgPairs -FromFile 'C:\no\such\pairs.env' -Pairs @(); 'no throw' } catch { 'threw' }
}
Test-Case 'no file and no flag is an empty list, not a failure' '0' {
    [string](@(Get-ScriptArgPairs -FromFile '' -Pairs @()).Count)
}

# -- the sink paths, refused before anything is created ----------------------
# THE DEFECT THESE EXIST FOR. The refusal used to live inside the sink openers,
# which a run reaches only after it has pulled, exported and imported. -DryRun
# returned before them, so a dry run reported a plan that the real run would
# have refused. Found by planting -StreamLogPath nul.
# EACH REFUSAL ASSERTS THE REASON, not merely that something threw. Written the
# other way first, and all three were green because the function was dying on an
# unset constant before it reached the guard they are named for.
Test-Case 'a reserved device name is refused, and the message names the device' 'named nul' {
    try { Assert-SinkPathIsUsable -Path 'nul' -Parameter '-StreamLogPath'; 'no throw' }
    catch { if ($_.Exception.Message -like "*reserved device 'nul'*") { 'named nul' } else { 'threw for another reason: ' + $_.Exception.Message } }
}
Test-Case 'the extension is stripped before the name is compared' 'named CON' {
    try { Assert-SinkPathIsUsable -Path 'logs\CON.jsonl' -Parameter '-EventLog'; 'no throw' }
    catch { if ($_.Exception.Message -like "*reserved device 'CON'*") { 'named CON' } else { 'threw for another reason: ' + $_.Exception.Message } }
}
Test-Case 'the comparison ignores case, because the device does' 'named Lpt3' {
    try { Assert-SinkPathIsUsable -Path 'Lpt3.log' -Parameter '-StreamLogPath'; 'no throw' }
    catch { if ($_.Exception.Message -like "*reserved device 'Lpt3'*") { 'named Lpt3' } else { 'threw for another reason: ' + $_.Exception.Message } }
}
Test-Case 'the device list comes from the bundle, not from a copy here' 'True' {
    ([bool]($script:ReservedDeviceNames -contains 'NUL' -and $script:ReservedDeviceNames.Count -ge 22)).ToString()
}
Test-Case 'an ordinary path is accepted' 'ok' {
    Assert-SinkPathIsUsable -Path 'logs\run.jsonl' -Parameter '-EventLog'; 'ok'
}
Test-Case 'a name that merely starts with a device name is not one' 'ok' {
    Assert-SinkPathIsUsable -Path 'console.log' -Parameter '-StreamLogPath'; 'ok'
}
Test-Case 'an empty path is not a path and is not judged' 'ok' {
    Assert-SinkPathIsUsable -Path '' -Parameter '-StreamLogPath'; 'ok'
}

# -- report ------------------------------------------------------------------
$failed = @($Results | Where-Object { -not $_.Pass })

# A suite that examined nothing reports the same green as one that examined
# everything, so the count is asserted rather than printed.
if ($Results.Count -lt 40) {
    [Console]::Error.WriteLine("selftest: only $($Results.Count) case(s) ran, and this file carries more than that. Something stopped the table early.")
    exit 1
}

if ($Json) {
    Write-Output ('{"schema":"wsl-toolkit-selftest/1","cases":' + $Results.Count +
                  ',"failed":' + $failed.Count + ',"functions":' + $Wanted.Count + '}')
    exit ([int]($failed.Count -gt 0))
}

foreach ($r in $Results) {
    if ($r.Pass) { Write-Output ("  ok    " + $r.Name) }
    else {
        Write-Output ("  FAIL  " + $r.Name)
        Write-Output ("        expected <" + $r.Expect + ">")
        Write-Output ("        actual   <" + $r.Actual + ">")
    }
}
Write-Output ''
if ($failed.Count -gt 0) {
    Write-Output ("selftest: " + $failed.Count + " of " + $Results.Count + " case(s) FAILED over " + $Wanted.Count + " function(s).")
    exit 1
}
Write-Output ("selftest: " + $Results.Count + " case(s) passed over " + $Wanted.Count + " function(s) loaded from wsl-toolkit.ps1.")
exit 0
