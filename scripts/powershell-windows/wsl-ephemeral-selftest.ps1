#Requires -Version 5.1

<#
.SYNOPSIS
    Run wsl-ephemeral.ps1's pure functions against a table of cases, on any
    Windows host, without WSL and without creating anything.

.DESCRIPTION
    THE DEFECT IT EXISTS TO CATCH. Most of wsl-ephemeral.ps1 can only be proved
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

    HOW IT LOADS THE FUNCTIONS. wsl-ephemeral.ps1 cannot be dot-sourced: its top
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
    pwsh -NoProfile -File scripts\powershell-windows\wsl-ephemeral-selftest.ps1

.NOTES
    ASCII-ONLY ON PURPOSE, like the launcher beside it.
    docs/conventions/shell.md section 8: a .ps1 holding any non-ASCII byte needs
    a UTF-8 BOM before Windows PowerShell 5.1 decodes it correctly. Staying
    ASCII removes the requirement rather than depending on it.

    Requires : Windows PowerShell 5.1 or PowerShell 7+. No WSL, no engine.
#>

[CmdletBinding(PositionalBinding = $false)]
param([switch]$Json)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Target = Join-Path (Split-Path -Parent $PSCommandPath) 'wsl-ephemeral.ps1'
if (-not (Test-Path -LiteralPath $Target -PathType Leaf)) {
    [Console]::Error.WriteLine("wsl-ephemeral-selftest: $Target is not there, so there is nothing to test.")
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
)

$parseErrors = $null
$tokens = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
    (Resolve-Path -LiteralPath $Target).Path, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors -and $parseErrors.Count -gt 0) {
    [Console]::Error.WriteLine("wsl-ephemeral-selftest: $Target does not parse; check-powershell.ps1 owns that verdict.")
    exit 1
}

$found = @{}
foreach ($fn in $ast.FindAll({ $args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $true)) {
    if ($Wanted -contains $fn.Name) { $found[$fn.Name] = $fn.Extent.Text }
}

$missing = @($Wanted | Where-Object { -not $found.ContainsKey($_) })
if ($missing.Count -gt 0) {
    [Console]::Error.WriteLine("wsl-ephemeral-selftest: these functions are named here and are not in $Target : " + ($missing -join ', '))
    [Console]::Error.WriteLine("  A rename is not a failure of the tool, it is a failure of this file to follow it. Fix the list, do not delete the cases.")
    exit 1
}

foreach ($name in $Wanted) {
    # [ScriptBlock]::Create and a dot-source, rather than Invoke-Expression:
    # the same effect, and it does not reach for the cmdlet whose every other
    # use is a defect.
    . ([ScriptBlock]::Create($found[$name]))
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

# Script-scoped parameters the loaded functions read. In wsl-ephemeral.ps1 these
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
    $script:ScriptArg = @('X=1')
    try { [Text.Encoding]::UTF8.GetString((Resolve-CommandBytes -Text 'true' -FromFile '' -FromB64 '')) }
    finally { $script:ScriptArg = @() }
}
Test-Case 'the prologue is prepended to a command given as base64, identically' "X='1'`nexport X`ntrue" {
    $script:ScriptArg = @('X=1')
    try {
        $b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes('true'))
        [Text.Encoding]::UTF8.GetString((Resolve-CommandBytes -Text '' -FromFile '' -FromB64 $b64))
    }
    finally { $script:ScriptArg = @() }
}
Test-Case 'no command at all is null, not an empty command' 'null' {
    $r = Resolve-CommandBytes -Text '' -FromFile '' -FromB64 ''
    if ($null -eq $r) { 'null' } else { 'not null' }
}
Test-Case '-ScriptArg with no command to carry is refused, never ignored' 'threw' {
    $script:ScriptArg = @('X=1')
    try { $null = Resolve-CommandBytes -Text '' -FromFile '' -FromB64 ''; 'no throw' }
    catch { 'threw' }
    finally { $script:ScriptArg = @() }
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
function Get-TestLogState {
    param([string]$Mode)
    $script:TimestampMode = $Mode
    $script:TimestampFormat = ''
    $st = New-StreamLogState -DistroName 'eph-test'
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

# -- report ------------------------------------------------------------------
$failed = @($Results | Where-Object { -not $_.Pass })

# A suite that examined nothing reports the same green as one that examined
# everything, so the count is asserted rather than printed.
if ($Results.Count -lt 40) {
    [Console]::Error.WriteLine("wsl-ephemeral-selftest: only $($Results.Count) case(s) ran, and this file carries more than that. Something stopped the table early.")
    exit 1
}

if ($Json) {
    Write-Output ('{"schema":"wsl-ephemeral-selftest/1","cases":' + $Results.Count +
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
    Write-Output ("wsl-ephemeral-selftest: " + $failed.Count + " of " + $Results.Count + " case(s) FAILED over " + $Wanted.Count + " function(s).")
    exit 1
}
Write-Output ("wsl-ephemeral-selftest: " + $Results.Count + " case(s) passed over " + $Wanted.Count + " function(s) loaded from wsl-ephemeral.ps1.")
exit 0
