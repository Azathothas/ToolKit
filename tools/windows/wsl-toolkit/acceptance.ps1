# acceptance.ps1 - drive the wsl-toolkit executable against a real machine.
#
# The defect this exists to catch is a green unit suite over a tool that cannot
# do its job. selftest.ps1 and `go test` prove the pure parts; nothing there
# talks to wsl.exe, to a container engine or to a real distribution, and every
# defect this tool has carried was found by running it. This is part (b) of
# docs/methodology/gate.md, written down so it is a command rather than a memory.
#
# WHAT IT REQUIRES: a Windows host with WSL2, a container engine for the base
# build, and a network for the image pulls. It CREATES the base distribution if
# there is none, and it leaves it registered.
#
# HARD RULE: IT NEVER TOUCHES podman-machine-default OR ANY OTHER DISTRIBUTION.
# Everything it makes is inside the distribution the tool owns, and the last
# case asserts the machine is back where it started.
#
# HARD RULE: THE CASE COUNT IS ASSERTED. A table that stopped early exits 0 over
# a smaller suite, which is the shape a check takes on its way to reporting
# nothing.
#
# Usage:
#   pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1 -Binary PATH
#   pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1 -Binary PATH -Quick
#   pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1 -Binary PATH -Json
#
# Exit codes: 0 every case passed, 1 a case failed, 2 could not run.
# Read the exit code from this process, unpiped.
#
# ASCII-ONLY ON PURPOSE. docs/conventions/shell.md section 8: a .ps1 holding any
# non-ASCII byte needs a UTF-8 BOM before Windows PowerShell 5.1 decodes it
# correctly. Staying ASCII removes the requirement rather than depending on it.
[CmdletBinding(PositionalBinding = $false)]
param(
    [string]$Binary = '',
    [switch]$Quick,
    [switch]$Json
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:Cases = @()
$script:Failed = 0

function Write-Line { param([string]$Text) if (-not $Json) { Write-Output $Text } }

function Exit-Cannot {
    param([string]$Reason)
    if ($Json) { Write-Output (@{ schema = 'wsl-toolkit-acceptance/1'; ok = $false; reason = $Reason; cases = @() } | ConvertTo-Json -Depth 5 -Compress) }
    else { [Console]::Error.WriteLine("acceptance: $Reason") }
    exit 2
}

# Every case is a name, an expectation and a scriptblock returning what actually
# happened. The comparison is here so no case can decide for itself whether it
# passed.
#
# -MaxSeconds is A WALL-TIME CEILING and it is part of the verdict. Every case
# has always been timed and nothing ever compared the number to anything, so a
# twelve second wait for a two second deadline read as a pass for as long as the
# deadline existed. TOOL-17. A case that names no ceiling is unbounded, which is
# the old behaviour and is right for one that pulls twelve images.
function Test-Case {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Expected,
        [Parameter(Mandatory = $true)][scriptblock]$Body,
        [double]$MaxSeconds = 0
    )
    $actual = ''
    # NOTE: not $error. That is an automatic variable, and a local of the same
    # name IS it, because PowerShell ignores case in variable names.
    $caught = ''
    $started = Get-Date
    try { $actual = [string](& $Body) }
    catch { $caught = $_.Exception.Message; $actual = "THREW: $($_.Exception.Message)" }
    $seconds = [math]::Round(((Get-Date) - $started).TotalSeconds, 2)
    $pass = ($actual -eq $Expected)
    $overrun = ($MaxSeconds -gt 0 -and $seconds -gt $MaxSeconds)
    if ($overrun) { $pass = $false }
    $entry = [ordered]@{
        name = $Name; expected = $Expected; actual = $actual
        pass = $pass; seconds = $seconds
    }
    if ($MaxSeconds -gt 0) { $entry['max_seconds'] = $MaxSeconds }
    $script:Cases += $entry
    if ($pass) { Write-Line ("  ok    {0}" -f $Name) }
    else {
        $script:Failed++
        Write-Line ("  FAIL  {0}" -f $Name)
        if ($overrun) {
            Write-Line ("        wall time: {0}s, and this case allows {1}s" -f $seconds, $MaxSeconds)
        }
        if ($actual -ne $Expected) {
            Write-Line ("        expected: {0}" -f $Expected)
            Write-Line ("        actual  : {0}" -f $actual)
        }
        if ($caught) { Write-Line ("        error   : {0}" -f $caught) }
    }
}

# HARD RULE: ProcessStartInfo.ArgumentList, NOT Start-Process -ArgumentList, and
# not a joined string. Start-Process JOINS the list with spaces and applies its
# own quoting, so `-c 'printf "%s" "$(id -u)"'` reached the tool as several
# arguments and the guest answered "usage: printf FORMAT". Four cases here were
# red for that reason and the tool was correct throughout. ArgumentList passes
# each element as one argument and .NET does the escaping.
#
# NOTE: BOTH STREAMS ARE READ BEFORE THE WAIT. Calling WaitForExit first
# deadlocks any child that fills a pipe buffer: the child blocks on write and
# the parent blocks on the wait. docs/conventions/shell.md section 8.
#
# NOTE: THE EXIT CODE IS READ FROM THE PROCESS, NOT THROUGH A PIPE.
function Invoke-Tool {
    param([Parameter(Mandatory = $true)][string[]]$ToolArgs)
    $psi = [Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = $script:Binary
    foreach ($a in $ToolArgs) { $null = $psi.ArgumentList.Add($a) }
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $p = [Diagnostics.Process]::Start($psi)
    $outTask = $p.StandardOutput.ReadToEndAsync()
    $errTask = $p.StandardError.ReadToEndAsync()
    $p.WaitForExit()
    return [pscustomobject]@{ Code = $p.ExitCode; Out = $outTask.Result; Err = $errTask.Result }
}

# -- the four capabilities the thirteen defects needed -----------------------
# TOOL-17. Each of these exists because a shipped defect was unreachable without
# it, and each is named at the entry that measured it.

# Measure-Tool is Invoke-Tool with THE CLOCK AROUND THE PROCESS. A duration the
# tool reports about itself cannot catch a tool that returns late, because both
# numbers come from the same run and the one that lies is the one being read.
# WSL-45: exit 124 after 12 seconds, reported as 4.4.
function Measure-Tool {
    param([Parameter(Mandatory = $true)][string[]]$ToolArgs)
    $started = [Diagnostics.Stopwatch]::StartNew()
    $r = Invoke-Tool $ToolArgs
    $started.Stop()
    return [pscustomobject]@{
        Code = $r.Code; Out = $r.Out; Err = $r.Err
        Seconds = [math]::Round($started.Elapsed.TotalSeconds, 2)
    }
}

# Show-Bytes renders a string so an EXPECTATION CAN BE WRITTEN BYTE FOR BYTE.
# Every case here asks whether output CONTAINS a marker, and a substring test
# cannot see an invented byte: WSL-46's stray newline survived all 39 of them,
# and this repository is what introduced it. The rendering is reversible by eye
# and safe to put in an -Expected string, which a raw control byte is not.
#
# NOTE: it counts CHARACTERS of a .NET string, which for the ASCII this tool
# emits is the byte count. Non-ASCII is rendered as \uXXXX so a case comparing
# it still compares something exact rather than something that looks equal.
# NOTE: if/elseif rather than a switch. PowerShell's switch IS a loop, so
# `break` and `continue` inside one mean something different from what they mean
# inside the foreach around it, and a renderer that silently skipped a branch
# would produce a plausible wrong answer, which is the class this whole file
# exists to catch.
function Show-Bytes {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Text)
    $sb = [Text.StringBuilder]::new()
    foreach ($ch in $Text.ToCharArray()) {
        $code = [int]$ch
        if ($code -eq 10) { $null = $sb.Append('\n') }
        elseif ($code -eq 13) { $null = $sb.Append('\r') }
        elseif ($code -eq 9) { $null = $sb.Append('\t') }
        elseif ($code -eq 92) { $null = $sb.Append('\\') }
        elseif ($code -ge 32 -and $code -le 126) { $null = $sb.Append($ch) }
        else { $null = $sb.Append(('\u{0:x4}' -f $code)) }
    }
    return ("len={0} [{1}]" -f $Text.Length, $sb.ToString())
}

# Read-ToolJson parses stdout and REFUSES anything that is not exactly one
# object. WSL-46: `base ensure --json` accepts the flag, writes progress to
# stderr, exits 0 and puts nothing on stdout, and every case that took a
# ConvertFrom-Json result and read one field off it passed by never asking.
#
# It throws rather than returning $null, so a case that forgets to check is
# still a red case. A silent $null is how this blind spot survives a second time.
function Read-ToolJson {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Stdout, [string]$What = 'the command')
    if ($null -eq $Stdout -or $Stdout.Trim() -eq '') {
        throw "$What advertises --json and put nothing on stdout"
    }
    $obj = $null
    try { $obj = $Stdout | ConvertFrom-Json }
    catch { throw "$What put something on stdout that is not JSON: $(Show-Bytes ($Stdout.Substring(0, [math]::Min(200, $Stdout.Length))))" }
    # ConvertFrom-Json turns a stream of two objects into an ARRAY, so a
    # command emitting two documents is caught here rather than reading as one.
    if ($obj -is [Array]) { throw "$What put $($obj.Count) JSON documents on stdout and an answer is one" }
    if ($null -eq $obj) { throw "$What put a JSON null on stdout" }
    return $obj
}

# New-StateHome and Set-StateConfig are the mid-flight mutation capability.
# WSL-44: a detached helper reads its config once at startup and never again, so
# the defect only appears when something changes AFTER the process is up. Every
# case here built its state before the first invocation, so no case could reach
# it. These make "start it, change it underneath, ask again" a three-line case.
function New-StateHome {
    param([Parameter(Mandatory = $true)][string]$Name)
    $dir = Join-Path $script:Scratch $Name
    $null = New-Item -ItemType Directory -Path $dir -Force
    return $dir
}

# NOTE: the parameter is StateHome and not Home. $Home is an automatic variable
# and PowerShell ignores case in variable names, so a parameter called Home IS
# the user's home directory. docs/conventions/forbidden-patterns.md carries the
# same collision in the tool's own source.
function Set-StateConfig {
    param(
        [Parameter(Mandatory = $true)][string]$StateHome,
        [Parameter(Mandatory = $true)][hashtable]$Config
    )
    if (-not $Config.ContainsKey('schema')) { $Config['schema'] = 'wsl-toolkit-config/1' }
    $json = $Config | ConvertTo-Json -Depth 8
    # WriteAllText with an explicit UTF8 encoding that carries no BOM.
    # Set-Content would write the host's default, and the tool decodes bytes.
    [IO.File]::WriteAllText((Join-Path $StateHome 'config.json'), $json, [Text.UTF8Encoding]::new($false))
    return (Join-Path $StateHome 'config.json')
}

# -- setup -------------------------------------------------------------------
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..\..')).Path
if (-not $Binary) { $Binary = Join-Path $repoRoot '.tmp\wsl-toolkit.exe' }
if (-not (Test-Path -LiteralPath $Binary -PathType Leaf)) {
    Exit-Cannot "no executable at $Binary. Build one: cd tools/windows/wsl-toolkit; go build -o ../../../.tmp/wsl-toolkit.exe ."
}
$script:Binary = (Resolve-Path -LiteralPath $Binary).Path
# ProcessStartInfo.ArgumentList is .NET Core only, and passing a joined string
# instead is the defect this file already met. Refusing is the honest answer.
if ($PSVersionTable.PSEdition -ne 'Core') {
    Exit-Cannot 'this runner needs PowerShell 7 or later, because it passes arguments as a list rather than as a joined string'
}
$script:Scratch = Join-Path $repoRoot ('.tmp\acceptance.' + [Guid]::NewGuid().ToString('N').Substring(0, 8))
$null = New-Item -ItemType Directory -Path $script:Scratch -Force

# What this machine had before anything ran. The last case asserts every one of
# them is still there, which is the operator's constraint stated as a reading
# rather than as a promise.
$script:PreExisting = @()
$before = Invoke-Tool @('doctor', '--json', '--fast')
if ($before.Code -eq 0) {
    $d = $before.Out | ConvertFrom-Json
    if ($null -ne $d.wsl -and $null -ne $d.wsl.distros) {
        $script:PreExisting = @($d.wsl.distros | Where-Object { -not $_.owned } | ForEach-Object { $_.name })
    }
}

try {
    Write-Line "acceptance: $script:Binary"
    Write-Line "scratch:    $script:Scratch"
    Write-Line ("untouched:  {0}" -f (($script:PreExisting -join ', ')))
    Write-Line ''

    # -- the tool answers at all ---------------------------------------------
    Test-Case 'the executable reports the version its embedded script declares' 'True' {
        $v = Invoke-Tool @('version')
        $j = (Invoke-Tool @('version', '--json')).Out | ConvertFrom-Json
        (($v.Code -eq 0) -and ($v.Out.Trim() -eq $j.version) -and $j.script_reversible).ToString()
    }

    Test-Case 'the survey runs, creates nothing, and separates installed from callable' 'True' {
        $r = Invoke-Tool @('doctor', '--json', '--fast')
        $d = $r.Out | ConvertFrom-Json
        (($r.Code -eq 0) -and ($d.schema -eq 'agent-doctor/1') -and ($null -ne $d.wsl) -and ($d.wsl.PSObject.Properties.Name -contains 'callable')).ToString()
    }

    Test-Case 'the catalog is twelve fully qualified references' '12'  {
        $r = Invoke-Tool @('images', '--json')
        $d = $r.Out | ConvertFrom-Json
        $bad = @($d.images | Where-Object { $_.ref -notmatch '^[a-z0-9.-]+\.[a-z]{2,}/' -and $_.ref -notmatch '^localhost/' })
        if ($bad.Count -gt 0) { return "unqualified: $($bad[0].ref)" }
        ([string]$d.images.Count)
    }

    # -- the base ------------------------------------------------------------
    Test-Case 'the base is registered and can actually run a container' 'True' {
        $r = Invoke-Tool @('base', 'ensure')
        if ($r.Code -ne 0) { return "ensure exited $($r.Code): $($r.Err)" }
        $s = Invoke-Tool @('base', 'status', '--probe', '--json')
        $d = $s.Out | ConvertFrom-Json
        (($d.registered -eq $true) -and ($d.healthy -eq $true)).ToString()
    }

    Test-Case 'the presets each carry a measurement or say they carry none' 'True' {
        $r = Invoke-Tool @('base', 'presets', '--json')
        $d = $r.Out | ConvertFrom-Json
        $blank = @($d.presets | Where-Object { -not $_.build })
        (($r.Code -eq 0) -and ($blank.Count -eq 0) -and ($d.presets.Count -ge 3)).ToString()
    }

    # -- a job, as the container's default account and as another one ---------
    Test-Case 'a container job runs as the image default and returns its output' 'uid=0' {
        $r = Invoke-Tool @('run', '--image', 'alpine', '-c', 'printf "uid=%s" "$(id -u)"')
        if ($r.Code -ne 0) { return "exited $($r.Code): $($r.Err)" }
        $r.Out.Trim()
    }

    Test-Case 'a container job runs as another account and can still write /out' 'uid=1000 nonroot' {
        $art = Join-Path $script:Scratch 'art-user'
        $r = Invoke-Tool @('run', '--image', 'alpine', '--user', '1000:1000', '--artifacts', $art,
            '-c', 'printf "uid=%s " "$(id -u)"; printf nonroot > /out/who.txt')
        if ($r.Code -ne 0) { return "exited $($r.Code): $($r.Err)" }
        $written = if (Test-Path -LiteralPath (Join-Path $art 'who.txt')) { [IO.File]::ReadAllText((Join-Path $art 'who.txt')) } else { 'NOT WRITTEN' }
        ($r.Out.Trim() + ' ' + $written)
    }

    Test-Case 'a failing command returns its own exit code, not a flattened one' '37' {
        $r = Invoke-Tool @('run', '--image', 'alpine', '-c', 'exit 37')
        ([string]$r.Code)
    }

    Test-Case 'a command past its deadline is killed and reports 124' '124' {
        $r = Invoke-Tool @('run', '--image', 'alpine', '--timeout', '10s', '-c', 'sleep 600')
        ([string]$r.Code)
    }

    # -- the isolation, which is the whole point -----------------------------
    Test-Case 'a container that destroys its workspace leaves the host copy intact' 'True' {
        $ws = Join-Path $script:Scratch 'ws'
        $null = New-Item -ItemType Directory -Path (Join-Path $ws 'sub') -Force
        [IO.File]::WriteAllText((Join-Path $ws 'keep.txt'), 'PRECIOUS')
        [IO.File]::WriteAllText((Join-Path $ws 'sub\nested.txt'), 'NESTED')
        $before = (Get-ChildItem -LiteralPath $ws -Recurse -File | Sort-Object FullName | ForEach-Object { $_.Name }) -join ','
        $r = Invoke-Tool @('run', '--image', 'alpine', '--workspace', $ws,
            '-c', 'rm -rf /work/* /work/.[!.]* 2>/dev/null; ls -A /work | wc -l')
        $after = (Get-ChildItem -LiteralPath $ws -Recurse -File | Sort-Object FullName | ForEach-Object { $_.Name }) -join ','
        $kept = [IO.File]::ReadAllText((Join-Path $ws 'keep.txt'))
        (($r.Code -eq 0) -and ($before -eq $after) -and ($kept -eq 'PRECIOUS') -and ($r.Out.Trim() -eq '0')).ToString()
    }

    # WSL-47, issue 21. The container writes a real file AND a link out of the
    # tree. It used to be converted to an inert escape.link.txt beside exit 0,
    # so a caller could not tell its deliverables were incomplete; the manual
    # promised a refusal the whole time.
    Test-Case 'an artifact link that leaves the tree fails the job' 'True' {
        $art = Join-Path $script:Scratch 'art-hostile'
        $sentinel = Join-Path $script:Scratch 'ESCAPED.txt'
        if (Test-Path -LiteralPath $sentinel) { Remove-Item -LiteralPath $sentinel -Force }
        $r = Invoke-Tool @('run', '--image', 'alpine', '--artifacts', $art, '--json',
            '-c', 'printf ok > /out/fine.txt; ln -s /etc/hostname /out/escape')
        $d = Read-ToolJson -Stdout $r.Out -What 'run --json'
        $escaped = Test-Path -LiteralPath $sentinel
        $isLink = Test-Path -LiteralPath (Join-Path $art 'escape')
        $converted = Test-Path -LiteralPath (Join-Path $art 'escape.link.txt')
        $said = ($d.artifact_error -match 'absolute path')
        # v1.3.0 exited 0 here and wrote escape.link.txt.
        (($r.Code -eq 1) -and $said -and (-not $escaped) -and (-not $isLink) -and (-not $converted) -and
         ($d.effective_exit -eq 1) -and ($d.retained_kind -eq 'guest')).ToString()
    }

    # AND A LINK THAT STAYS INSIDE IS STILL DELIVERED AS A NOTE. A rule that
    # widened to refuse every link would break the case the sidecar exists for,
    # and a case that only drove the refusal would not notice.
    Test-Case 'an artifact link that stays inside the tree is recorded, not refused' 'True' {
        $art = Join-Path $script:Scratch 'art-internal-link'
        $r = Invoke-Tool @('run', '--image', 'alpine', '--artifacts', $art,
            '-c', 'printf real > /out/real.txt; ln -s real.txt /out/alias')
        if ($r.Code -ne 0) { return "the job exited $($r.Code): $($r.Err)" }
        $note = Join-Path $art 'alias.link.txt'
        $recorded = Test-Path -LiteralPath $note
        $isLink = Test-Path -LiteralPath (Join-Path $art 'alias')
        $names = $false
        if ($recorded) { $names = ([IO.File]::ReadAllText($note) -match 'real\.txt') }
        ($recorded -and $names -and (-not $isLink)).ToString()
    }

    Test-Case 'a workspace over its ceiling is refused rather than truncated' 'True' {
        $ws = Join-Path $script:Scratch 'ws-big'
        $null = New-Item -ItemType Directory -Path $ws -Force
        [IO.File]::WriteAllBytes((Join-Path $ws 'big.bin'), (New-Object byte[] 200000))
        $r = Invoke-Tool @('run', '--image', 'alpine', '--workspace', $ws, '--max-bytes', '1024', '-c', 'true')
        (($r.Code -ne 0) -and (($r.Err + $r.Out) -match 'workspace refused')).ToString()
    }

    Test-Case 'an unqualified image reference is refused by name' 'True' {
        $r = Invoke-Tool @('run', '--image', 'alpine:latest', '-c', 'true')
        (($r.Code -eq 2) -and (($r.Err + $r.Out) -match 'not fully qualified')).ToString()
    }

    Test-Case 'this tool refuses to act on a distribution it does not own' 'True' {
        # The name is passed as the configured base through the environment, so
        # the refusal comes from the guard rather than from a missing flag.
        $home2 = Join-Path $script:Scratch 'home-protected'
        $null = New-Item -ItemType Directory -Path $home2 -Force
        [IO.File]::WriteAllText((Join-Path $home2 'config.json'),
            '{"schema":"wsl-toolkit-config/1","base":{"name":"podman-machine-default","image":"docker.io/library/alpine:latest","user":"toolkit"}}')
        $r = Invoke-Tool @('--home', $home2, 'base', 'status', '--json')
        (($r.Code -eq 2) -and (($r.Err + $r.Out) -match 'container runtime')).ToString()
    }

    # -- the fleet -----------------------------------------------------------
    if (-not $Quick) {
        Test-Case 'every catalog image runs the same command and returns an artifact' 'True' {
            $art = Join-Path $script:Scratch 'art-all'
            $tr = Join-Path $script:Scratch 'transcripts'
            $r = Invoke-Tool @('matrix', '--images', 'all', '--timeout', '20m', '--artifacts', $art,
                '--transcripts', $tr, '--json', '-c', '. /etc/os-release 2>/dev/null || true; printf "%s" "${ID:-unknown}" > /out/id.txt')
            $d = $r.Out | ConvertFrom-Json
            $withArtifact = @(Get-ChildItem -LiteralPath $art -Directory -ErrorAction SilentlyContinue |
                Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName 'id.txt') })
            (($r.Code -eq 0) -and ($d.ran -eq 12) -and ($d.failed -eq 0) -and ($d.unreached -eq 0) -and
             ($withArtifact.Count -eq 12)).ToString()
        }
    }

    Test-Case 'a selector that matches nothing is a refusal rather than an empty run' 'True' {
        $r = Invoke-Tool @('matrix', '--images', 'nosuchimage', '-c', 'true')
        (($r.Code -eq 2) -and (($r.Err + $r.Out) -match 'not a catalog image')).ToString()
    }

    # -- the second permission path ------------------------------------------
    Test-Case 'a job runs through the local helper and its artifacts come back' 'helper-ok' {
        $start = Invoke-Tool @('helper', 'serve', '--detach', '--json')
        if ($start.Code -ne 0) { return "helper would not start: $($start.Err)" }
        try {
            $art = Join-Path $script:Scratch 'art-helper'
            $ws = Join-Path $script:Scratch 'ws-helper'
            $null = New-Item -ItemType Directory -Path $ws -Force
            [IO.File]::WriteAllText((Join-Path $ws 'marker.txt'), 'from-the-host')
            $r = Invoke-Tool @('run', '--via-helper', '--image', 'alpine', '--workspace', $ws,
                '--artifacts', $art, '-c', 'cat /work/marker.txt > /out/echoed.txt; printf helper-ok')
            if ($r.Code -ne 0) { return "the helper job exited $($r.Code): $($r.Err)" }
            $echoed = if (Test-Path -LiteralPath (Join-Path $art 'echoed.txt')) { [IO.File]::ReadAllText((Join-Path $art 'echoed.txt')) } else { 'NOT RETURNED' }
            if ($echoed -ne 'from-the-host') { return "the workspace did not survive the round trip: $echoed" }
            $r.Out.Trim()
        }
        finally { $null = Invoke-Tool @('helper', 'stop') }
    }

    # A FLAG THE SECOND PATH IGNORES IS A JOB THAT RAN AS SOMEBODY ELSE. This
    # case asks both paths the same question and compares their answers, which
    # is the only shape that catches a field dropped between them.
    Test-Case 'both paths agree about which account a job runs as' 'direct=1000 helper=1000' {
        $start = Invoke-Tool @('helper', 'serve', '--detach', '--json')
        if ($start.Code -ne 0) { return "helper would not start: $($start.Err)" }
        try {
            $ask = @('--image', 'alpine', '--user', '1000:1000', '-c', 'printf "%s" "$(id -u)"')
            $direct = Invoke-Tool (@('run') + $ask)
            $viaHelper = Invoke-Tool (@('run', '--via-helper') + $ask)
            if ($direct.Code -ne 0) { return "the direct job exited $($direct.Code): $($direct.Err)" }
            if ($viaHelper.Code -ne 0) { return "the helper job exited $($viaHelper.Code): $($viaHelper.Err)" }
            "direct=$($direct.Out.Trim()) helper=$($viaHelper.Out.Trim())"
        }
        finally { $null = Invoke-Tool @('helper', 'stop') }
    }

    Test-Case 'the helper is gone once it is stopped' 'True' {
        $r = Invoke-Tool @('helper', 'status', '--json')
        $d = $r.Out | ConvertFrom-Json
        (($r.Code -eq 1) -and ($d.listening -eq $false)).ToString()
    }

    # -- the embedded script -------------------------------------------------
    Test-Case 'the embedded script runs and reports the distributions it may touch' 'True' {
        $r = Invoke-Tool @('script', '-Action', 'List')
        (($r.Code -eq 0) -and (($r.Out + $r.Err) -match 'PROTECTED')).ToString()
    }

    Test-Case 'the embedded script puts one address on stdout and nothing else' 'True' {
        $r = Invoke-Tool @('script', '-Action', 'HostAddress')
        if ($r.Code -ne 0) { return "exited $($r.Code): $($r.Err)" }
        ($r.Out.Trim() -match '^(\d{1,3}\.){3}\d{1,3}$').ToString()
    }

    # -- the eight defects a consumer found in wsl-toolkit-v1.1.0 ------------
    # Each of these fails against v1.1.0 and passes here. They are grouped so a
    # reader can see which issue a red case belongs to.

    # issue 9: two names that are one file on the destination.
    Test-Case 'artifact names that collide on this host are refused, not merged' 'True' {
        $art = Join-Path $script:Scratch 'art-case'
        $r = Invoke-Tool @('run', '--image', 'alpine', '--artifacts', $art,
            '-c', 'printf upper > /out/Result; printf lower > /out/result')
        $said = (($r.Err + $r.Out) -match 'name one file')
        # v1.1.0 exited 0 here and delivered a single five-byte file.
        (($r.Code -ne 0) -and $said).ToString()
    }

    # issue 9: an NTFS alternate data stream is not a file name.
    Test-Case 'an artifact naming an alternate data stream is refused' 'True' {
        $art = Join-Path $script:Scratch 'art-ads'
        $r = Invoke-Tool @('run', '--image', 'alpine', '--artifacts', $art,
            '-c', 'printf hidden > /out/normal.txt:stream')
        $said = (($r.Err + $r.Out) -match 'alternate data stream')
        $empty = -not (Test-Path -LiteralPath (Join-Path $art 'normal.txt'))
        (($r.Code -ne 0) -and $said -and $empty).ToString()
    }

    # issue 8: a transfer that failed is not a job that passed, and its output
    # survives.
    Test-Case 'a job whose artifacts are refused exits nonzero and keeps the guest copy' 'True' {
        $art = Join-Path $script:Scratch 'art-refused'
        $r = Invoke-Tool @('run', '--image', 'alpine', '--artifacts', $art, '--json',
            '-c', 'printf real > /out/good.txt; printf forbidden > /out/NUL.txt')
        $d = $r.Out | ConvertFrom-Json
        $kept = ($null -ne $d.guest_dir) -and ($d.guest_dir -ne '')
        $named = ($null -ne $d.artifact_error) -and ($d.artifact_error -ne '')
        # v1.1.0 exited 0 with artifacts: 0 and then removed the guest copy.
        (($r.Code -eq 1) -and $named -and $kept).ToString()
    }

    # issue 8: the container's own code still wins.
    Test-Case 'a container that failed AND lost its artifacts reports the container code' '37' {
        $art = Join-Path $script:Scratch 'art-both'
        $r = Invoke-Tool @('run', '--image', 'alpine', '--artifacts', $art,
            '-c', 'printf forbidden > /out/NUL.txt; exit 37')
        ([string]$r.Code)
    }

    # issue 14: an image that was never pulled did not run.
    Test-Case 'an image the registry does not have is unreached, not a failed job' 'True' {
        $r = Invoke-Tool @('run', '--json', '--image',
            'docker.io/library/alpine:wsl-toolkit-no-such-tag-20260909', '-c', 'echo must-not-run')
        $d = $r.Out | ConvertFrom-Json
        $ranAnyway = ($r.Out -match 'must-not-run')
        # v1.1.0 answered exit 125 with unreached false.
        (($r.Code -eq 2) -and ($d.unreached -eq $true) -and (-not $ranAnyway)).ToString()
    }

    # issue 14: and a payload that itself exits 125 is NOT unreached.
    Test-Case 'a payload that exits 125 is a job that ran' 'True' {
        $r = Invoke-Tool @('run', '--json', '--image', 'alpine', '-c', 'exit 125')
        $d = $r.Out | ConvertFrom-Json
        (($r.Code -eq 125) -and ($d.unreached -eq $false)).ToString()
    }

    # issue 10: output is complete, and the answer says when its copy is not.
    Test-Case 'output past the capture limit keeps its last byte in the transcript' 'True' {
        $r = Invoke-Tool @('run', '--json', '--image', 'alpine', '--max-output', '4096',
            '-c', 'head -c 20000 /dev/zero | tr "\000" x; printf END-MARKER')
        $d = $r.Out | ConvertFrom-Json
        $cut = ($d.stdout_truncated -eq $true)
        $counted = ($d.stdout_bytes -ge 20010)
        $whole = $false
        if ($null -ne $d.transcript -and $d.transcript -ne '') {
            $log = Join-Path $d.transcript 'stdout.log'
            if (Test-Path -LiteralPath $log) {
                $whole = ([IO.File]::ReadAllText($log)).EndsWith('END-MARKER')
            }
        }
        # v1.1.0 cut at 8 MiB with no field and no transcript.
        (($r.Code -eq 0) -and $cut -and $counted -and $whole).ToString()
    }

    # issue 10: and logs reads it back.
    Test-Case 'logs writes a job transcript back, complete' 'True' {
        $r = Invoke-Tool @('run', '--json', '--image', 'alpine', '-c', 'printf READ-ME-BACK')
        $d = $r.Out | ConvertFrom-Json
        $l = Invoke-Tool @('logs', $d.id)
        (($l.Code -eq 0) -and ($l.Out -match 'READ-ME-BACK')).ToString()
    }

    # THE CLIENT'S OWN COPY, which the case above does not reach. A job run
    # through the helper has its transcript written by the HELPER, under the
    # helper's state directory, so where the two do not share WSL_TOOLKIT_HOME the
    # result named a path that did not exist on the machine the caller was sitting
    # at. This asks for a job through the helper and reads it back with the CLIENT,
    # which is what a restricted caller actually does.
    Test-Case 'a helper job leaves its transcript on the machine that asked for it' 'True' {
        $start = Invoke-Tool @('helper', 'serve', '--detach', '--json')
        if ($start.Code -ne 0) { return "helper would not start: $($start.Err)" }
        try {
            $r = Invoke-Tool @('run', '--via-helper', '--json', '--image', 'alpine', '-c', 'printf HELPER-TRANSCRIPT')
            if ($r.Code -ne 0) { return "the helper job exited $($r.Code): $($r.Err)" }
            $d = $r.Out | ConvertFrom-Json
            $l = Invoke-Tool @('logs', $d.id)
            (($l.Code -eq 0) -and ($l.Out -match 'HELPER-TRANSCRIPT')).ToString()
        }
        finally { $null = Invoke-Tool @('helper', 'stop') }
    }

    # A FEATURE NOBODY CAN FIND IS ONE THAT WAS NOT SHIPPED. v1.2.0 printed
    # the image label and the exit code and nothing else, so the id `logs` takes
    # could not be typed without first running `logs` bare to go hunting for it.
    Test-Case 'a run says the command that reads its output back' 'True' {
        $r = Invoke-Tool @('run', '--image', 'alpine', '-c', 'printf FIND-ME')
        if ($r.Code -ne 0) { return "the job exited $($r.Code): $($r.Err)" }
        (($r.Err + $r.Out) -match 'wsl-toolkit logs [0-9a-f]{16}').ToString()
    }

    # issue 13: the refusals the CLI was not making.
    Test-Case 'a stray word and the options after it are refused, not ignored' 'True' {
        $r = Invoke-Tool @('run', '--image', 'alpine', '-c', 'echo SHOULD-NOT-RUN', 'stray', '--no-such-option')
        $ran = ($r.Out -match 'SHOULD-NOT-RUN')
        $said = (($r.Err + $r.Out) -match 'positional')
        # v1.1.0 printed SHOULD-NOT-RUN and exited 0.
        (($r.Code -eq 2) -and (-not $ran) -and $said).ToString()
    }

    Test-Case 'an unusable environment name is refused where it was typed' 'True' {
        $r = Invoke-Tool @('run', '--image', 'alpine', '--env', 'BAD-NAME=must-not-drop', '-c', 'env')
        (($r.Code -eq 2) -and (($r.Err + $r.Out) -match 'BAD-NAME')).ToString()
    }

    Test-Case 'a negative timeout is refused rather than meaning unlimited' 'True' {
        $r = Invoke-Tool @('run', '--image', 'alpine', '--timeout', '-1s', '-c', 'echo ran-unbounded')
        $ran = ($r.Out -match 'ran-unbounded')
        (($r.Code -eq 2) -and (-not $ran) -and (($r.Err + $r.Out) -match 'negative')).ToString()
    }

    # issue 11: cleanup does not kill work that is running.
    Test-Case 'gc --apply leaves a job that is running right now alone' 'True' {
        # A real job, started in the background, and a real gc beside it. The
        # wait is on the CONDITION - the container appearing in resources - and
        # never on a duration, because a sleep long enough here is a race there.
        $psi = [Diagnostics.ProcessStartInfo]::new()
        $psi.FileName = $script:Binary
        foreach ($a in @('run', '--image', 'alpine', '--json', '-c', 'echo live-job-started; sleep 45; echo live-job-finished')) {
            $null = $psi.ArgumentList.Add($a)
        }
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $psi.UseShellExecute = $false
        $psi.CreateNoWindow = $true
        $live = [Diagnostics.Process]::Start($psi)
        $liveOut = $live.StandardOutput.ReadToEndAsync()
        $liveErr = $live.StandardError.ReadToEndAsync()
        try {
            $seen = $false
            $deadline = (Get-Date).AddMinutes(3)
            while ((Get-Date) -lt $deadline) {
                $res = Invoke-Tool @('resources', '--json')
                if ($res.Code -eq 0) {
                    $rd = $res.Out | ConvertFrom-Json
                    if ($null -ne $rd.owned.containers -and @($rd.owned.containers).Count -gt 0) { $seen = $true; break }
                }
                if ($live.HasExited) { break }
                Start-Sleep -Milliseconds 500
            }
            if (-not $seen) { return 'the live job never showed a container, so this case proved nothing' }
            $plan = Invoke-Tool @('gc', '--json', '--older-than', '24h')
            $apply = Invoke-Tool @('gc', '--apply', '--json', '--older-than', '24h')
            $live.WaitForExit()
            $out = $liveOut.Result
            $finished = ($out -match 'live-job-finished')
            $pd = $plan.Out | ConvertFrom-Json
            $spared = $false
            if ($null -ne $pd.kept) { $spared = (@($pd.kept) | Where-Object { $_ -match 'in use' }).Count -gt 0 }
            # v1.1.0 killed it: exit 137, no live-job-finished, gc exit 0.
            (($live.ExitCode -eq 0) -and $finished -and ($apply.Code -eq 0) -and $spared).ToString()
        }
        finally {
            if (-not $live.HasExited) { $live.Kill() }
            $null = $liveErr.Result
        }
    }

    # issue 12: the helper releases what it staged.
    Test-Case 'a helper job leaves no uploaded workspace or artifact set behind' 'True' {
        $home3 = Join-Path $script:Scratch 'home-helper-life'
        $null = New-Item -ItemType Directory -Path $home3 -Force
        $null = Invoke-Tool @('--home', $home3, 'helper', 'serve', '--detach')
        try {
            $ws = Join-Path $script:Scratch 'ws-life'
            $null = New-Item -ItemType Directory -Path $ws -Force
            [IO.File]::WriteAllText((Join-Path $ws 'input.txt'), 'LIFECYCLE')
            $art = Join-Path $script:Scratch 'art-life'
            $r = Invoke-Tool @('--home', $home3, 'run', '--via-helper', '--image', 'alpine',
                '--workspace', $ws, '--artifacts', $art, '-c', 'cp /work/input.txt /out/result.txt')
            $delivered = $false
            if (Test-Path -LiteralPath (Join-Path $art 'result.txt')) {
                $delivered = ([IO.File]::ReadAllText((Join-Path $art 'result.txt')) -eq 'LIFECYCLE')
            }
            $uploads = @()
            $artifacts = @()
            if (Test-Path -LiteralPath (Join-Path $home3 'uploads')) {
                $uploads = @(Get-ChildItem -LiteralPath (Join-Path $home3 'uploads') -Directory -ErrorAction SilentlyContinue)
            }
            if (Test-Path -LiteralPath (Join-Path $home3 'artifacts')) {
                $artifacts = @(Get-ChildItem -LiteralPath (Join-Path $home3 'artifacts') -Directory -ErrorAction SilentlyContinue)
            }
            # v1.1.0 left both behind and gc could not see either.
            (($r.Code -eq 0) -and $delivered -and ($uploads.Count -eq 0) -and ($artifacts.Count -eq 0)).ToString()
        }
        finally {
            $null = Invoke-Tool @('--home', $home3, 'helper', 'stop')
        }
    }

    # issue 7: the route is decided by asking WSL, not by finding wsl.exe.
    Test-Case 'a process that can reach wsl.exe keeps the direct route' 'True' {
        # The positive half of the routing rule, which is the half a machine
        # with working WSL can prove. The denial half needs a restricted
        # process and is covered by TestRouting in the Go suite.
        $r = Invoke-Tool @('run', '--image', 'alpine', '-c', 'true')
        $usedHelper = (($r.Err + $r.Out) -match 'using the helper')
        (($r.Code -eq 0) -and (-not $usedHelper)).ToString()
    }

    # -- the four capabilities, each proving a defect the suite could not see -
    # TOOL-17. A capability that cannot reproduce a known defect is not a
    # capability, so each of these was written against the binary that HAS the
    # defect and watched to fail before the fix existed. The entry carries that
    # output.

    # WSL-46, issue 22. Every surface that advertises --json, asked the same
    # question by one assertion rather than by fifteen cases. The old shape read
    # one field off a ConvertFrom-Json result, so a command that printed NOTHING
    # passed by never being asked.
    Test-Case 'every surface that advertises --json puts exactly one object on stdout' 'True' {
        # Each row is a name and the arguments. Read-only where it can be:
        # `base ensure` is here because it is idempotent and the base is already
        # up by this point, and `gc` without --apply only plans.
        $surfaces = @(
            @{ n = 'version';       a = @('version', '--json') }
            @{ n = 'doctor';        a = @('doctor', '--json', '--fast') }
            @{ n = 'images';        a = @('images', '--json') }
            @{ n = 'config';        a = @('config', '--json') }
            @{ n = 'base status';   a = @('base', 'status', '--json') }
            @{ n = 'base presets';  a = @('base', 'presets', '--json') }
            @{ n = 'base ensure';   a = @('base', 'ensure', '--json') }
            @{ n = 'resources';     a = @('resources', '--json') }
            @{ n = 'gc';            a = @('gc', '--json') }
            @{ n = 'logs';          a = @('logs', '--json') }
            @{ n = 'helper status'; a = @('helper', 'status', '--json') }
            @{ n = 'run';           a = @('run', '--json', '--image', 'alpine', '-c', 'true') }
        )
        $bad = @()
        foreach ($s in $surfaces) {
            $r = Invoke-Tool $s.a
            try { $null = Read-ToolJson -Stdout $r.Out -What $s.n }
            catch { $bad += ("{0}: {1}" -f $s.n, $_.Exception.Message) }
        }
        if ($bad.Count -gt 0) { return ($bad -join ' | ') }
        'True'
    }

    # WSL-46, issue 23. THE BYTE THIS REPOSITORY INVENTED. The container wrapper
    # prints its completion token as printf '\n%s\n' and the stripper writes the
    # leading newline back, so a payload that writes no stderr is reported as
    # having written one byte. Every case above asks whether output CONTAINS
    # something, and none of them can see an extra byte.
    Test-Case 'a payload that writes no error output is reported as writing none' 'stderr=len=0 [] bytes=0' {
        $r = Invoke-Tool @('run', '--json', '--image', 'alpine', '-c', 'true')
        $d = Read-ToolJson -Stdout $r.Out -What 'run --json'
        $stderr = ''
        if ($d.PSObject.Properties.Name -contains 'stderr') { $stderr = [string]$d.stderr }
        ("stderr={0} bytes={1}" -f (Show-Bytes $stderr), $d.stderr_bytes)
    }

    # WSL-45, issue 20. A DEADLINE THAT DOES NOT BOUND WAITING IS NOT A DEADLINE.
    # Measured on this machine on 2026-09-10 against the binary that has the
    # defect: a 2s deadline over sleep 8 returned after 10.6s, over sleep 20
    # after 16.0s and over sleep 30 after 15.8s, each reporting about 4.4s.
    # The ceiling is the deadline plus the grace the manual states, and the
    # clock is around the process because the number inside it is the one that
    # was wrong.
    Test-Case 'a deadline bounds the caller and the reported duration is the one it waited' 'True' -MaxSeconds 10 {
        $m = Measure-Tool @('run', '--json', '--image', 'alpine', '--timeout', '2s', '-c', 'sleep 60')
        $d = Read-ToolJson -Stdout $m.Out -What 'run --json'
        $reported = [math]::Round($d.duration_ns / 1e9, 2)
        if ($m.Code -ne 124) { return "exited $($m.Code) rather than 124" }
        # The two clocks must agree to within a second. A duration measured from
        # a different clock than the caller's is two numbers that disagree,
        # which is worse than one that is approximate.
        if ([math]::Abs($reported - $m.Seconds) -gt 1.0) {
            return ("it waited {0}s and reported {1}s" -f $m.Seconds, $reported)
        }
        'True'
    }

    # WSL-44, issue 17. A HELPER FREEZES ITS CONFIG AT STARTUP. Nothing here has
    # ever changed state a running process had already read: every case builds,
    # acts and asserts. This one starts a helper against a catalog, replaces the
    # catalog underneath it, and asks the helper to resolve an id only the new
    # one has.
    Test-Case 'a helper resolves a catalog id against the config as it is now' 'True' {
        $h = New-StateHome 'home-midflight'
        $null = Invoke-Tool @('--home', $h, 'helper', 'serve', '--detach')
        try {
            $null = Set-StateConfig -StateHome $h -Config @{
                images = @(
                    @{ id = 'midflight'; ref = 'docker.io/library/alpine:latest'
                       libc = 'musl'; family = 'apk'; kind = 'musl' }
                )
            }
            # The CLIENT resolves this against the file just written, so it is a
            # valid selection; the helper resolves the id it is sent against the
            # catalog it read at startup, which has no such row.
            $r = Invoke-Tool @('--home', $h, 'matrix', '--via-helper', '--json',
                '--images', 'midflight', '--timeout', '2m', '-c', 'printf MIDFLIGHT')
            if ($r.Code -ne 0) {
                $why = @(($r.Err + $r.Out).Split("`n") | Where-Object { $_.Trim() -ne '' })
                $last = if ($why.Count -gt 0) { $why[-1].Trim() } else { 'it said nothing' }
                return "the fleet exited $($r.Code): $last"
            }
            $d = Read-ToolJson -Stdout $r.Out -What 'matrix --json'
            (($d.ran -eq 1) -and ($d.failed -eq 0) -and ($d.unreached -eq 0)).ToString()
        }
        finally { $null = Invoke-Tool @('--home', $h, 'helper', 'stop') }
    }

    # WSL-47, issue 26. A Windows junction pointing outside a workspace was
    # skipped by the walker and the job exited 0 having never seen it, so a
    # build ran against a tree missing something and reported on it as the real
    # one. It is COUNTED and NAMED rather than refused: a junction somewhere in
    # a large tree is a normal thing to have.
    Test-Case 'a junction in a workspace is left out, counted and named' 'True' {
        $ws = Join-Path $script:Scratch 'ws-junction'
        $outside = Join-Path $script:Scratch 'outside-target'
        $null = New-Item -ItemType Directory -Path $ws -Force
        $null = New-Item -ItemType Directory -Path $outside -Force
        [IO.File]::WriteAllText((Join-Path $outside 'secret.txt'), 'ELSEWHERE')
        [IO.File]::WriteAllText((Join-Path $ws 'real.txt'), 'CARRIED')
        # mklink /J needs no privilege, unlike a symlink, which is why the
        # reporter could make one and could not make the other.
        $mk = Start-Process -FilePath $env:ComSpec -ArgumentList @('/d', '/s', '/c',
            ('mklink /J "{0}" "{1}"' -f (Join-Path $ws 'link'), $outside)) `
            -Wait -PassThru -WindowStyle Hidden
        if ($mk.ExitCode -ne 0) { return 'this host would not create a junction, so this case proved nothing' }
        $r = Invoke-Tool @('run', '--json', '--image', 'alpine', '--workspace', $ws,
            '-c', 'ls -A /work | while read -r n; do printf "[%s]" "$n"; done')
        if ($r.Code -ne 0) { return "the job exited $($r.Code): $($r.Err)" }
        $d = Read-ToolJson -Stdout $r.Out -What 'run --json'
        $named = $false
        if ($d.PSObject.Properties.Name -contains 'workspace_omission') {
            $named = (@($d.workspace_omission | Where-Object { $_.path -eq 'link' }).Count -eq 1)
        }
        $carried = ($d.stdout -match 'real\.txt')
        $didNotTravel = ($d.stdout -notmatch 'link')
        # v1.3.0 answered omitted 0 and said nothing at all.
        (($d.workspace_omitted -eq 1) -and $named -and $carried -and $didNotTravel).ToString()
    }

    # -- what this tool owns, and how it proves it ---------------------------
    # WSL-42, issues 16 and 18. WSL-43 and WSL-51 are the same ruling read from
    # the other two sides.

    # issue 16: base.name was editable and the guard compared it against itself.
    Test-Case 'a configured name outside the prefix is refused by name' 'True' {
        $h = New-StateHome 'home-foreign'
        $null = Set-StateConfig -StateHome $h -Config @{
            base = @{ name = 'my-distro'; image = 'docker.io/library/alpine:latest'; user = 'toolkit' }
        }
        $r = Invoke-Tool @('--home', $h, 'base', 'status', '--json')
        # v1.3.0 accepted this, would have CREATED it on `base ensure`, and
        # would have unregistered it on `base remove --yes`.
        (($r.Code -eq 2) -and (($r.Err + $r.Out) -match 'base\.name')).ToString()
    }

    # WSL-43: and an instance name INSIDE the prefix is accepted, which is the
    # half the reporter did not ask for and a fixed single name would forbid.
    Test-Case 'an instance name inside the prefix is accepted' 'True' {
        $h = New-StateHome 'home-instance-name'
        $null = Set-StateConfig -StateHome $h -Config @{
            base = @{ name = 'wsl-toolkit-two'; image = 'docker.io/library/alpine:latest'; user = 'toolkit' }
        }
        $r = Invoke-Tool @('--home', $h, 'base', 'status', '--json')
        $d = Read-ToolJson -Stdout $r.Out -What 'base status --json'
        # Not registered, which is correct: nothing built it. What matters is
        # that the NAME was not refused.
        (($d.name -eq 'wsl-toolkit-two') -and ($d.registered -eq $false)).ToString()
    }

    # issue 18: a health probe identifies the ENGINE and not the rootfs, so
    # `ensure` relabelled an Arch base as Alpine because an Alpine CONTAINER ran.
    Test-Case 'the guest own marker decides what the base was built from' 'True' {
        $probe = Invoke-Tool @('base', 'status', '--probe', '--json')
        $before = Read-ToolJson -Stdout $probe.Out -What 'base status --probe --json'
        if (-not $before.identity) { return 'the base carries no identity marker' }
        $real = [string]$before.identity.image
        $cfgPath = (Invoke-Tool @('config')).Out.Trim()
        $saved = if (Test-Path -LiteralPath $cfgPath) { [IO.File]::ReadAllText($cfgPath) } else { $null }
        try {
            # Change the CONFIGURED image without rebuilding anything.
            $lie = if ($real -match 'alpine') { 'ghcr.io/pkgforge-dev/archlinux:latest' } else { 'docker.io/library/alpine:latest' }
            $null = New-Item -ItemType Directory -Path (Split-Path -Parent $cfgPath) -Force
            [IO.File]::WriteAllText($cfgPath, (@{
                schema = 'wsl-toolkit-config/1'
                base   = @{ name = 'wsl-toolkit'; image = $lie; user = 'toolkit' }
            } | ConvertTo-Json -Depth 6), [Text.UTF8Encoding]::new($false))
            $e = Invoke-Tool @('base', 'ensure')
            # EXIT 1, AND THAT IS THE POINT. `ensure` was asked to bring the
            # machine to the state the configuration describes and it could not,
            # short of a rebuild it must not perform on its own. 1 means "it ran
            # and it disagreed". v1.3.0 exited 0 here, because relabelling the
            # record was what made the disagreement disappear.
            if ($e.Code -ne 1) { return "base ensure exited $($e.Code) rather than 1 over a drifted base" }
            $after = Read-ToolJson -Stdout (Invoke-Tool @('base', 'status', '--probe', '--json')).Out -What 'base status --probe --json'
            $said = (($e.Err + $e.Out) -match 'was built from')
            $rebuild = (($e.Err + $e.Out) -match 'base recreate')
            # v1.3.0 rewrote the record to $lie here and reported no drift.
            (($after.identity.image -eq $real) -and ($after.built_from -eq $real) -and $said -and $rebuild).ToString()
        }
        finally {
            if ($null -ne $saved) { [IO.File]::WriteAllText($cfgPath, $saved) }
            elseif (Test-Path -LiteralPath $cfgPath) { Remove-Item -LiteralPath $cfgPath -Force }
        }
    }

    # WSL-51: the nearest configuration wins WHOLE, and `config` says which file
    # it resolved and everything it looked at on the way.
    Test-Case 'the nearer configuration wins whole and config names the order' 'True' {
        $outer = Join-Path $script:Scratch 'cfg-outer'
        $inner = Join-Path $outer 'inner'
        $null = New-Item -ItemType Directory -Path $inner -Force
        $write = {
            param($dir, $image)
            [IO.File]::WriteAllText((Join-Path $dir 'wsl-toolkit.json'), (@{
                schema = 'wsl-toolkit-config/1'
                base   = @{ name = 'wsl-toolkit'; image = $image; user = 'toolkit' }
            } | ConvertTo-Json -Depth 6), [Text.UTF8Encoding]::new($false))
        }
        & $write $outer 'docker.io/library/debian:latest'
        & $write $inner 'docker.io/library/alpine:latest'
        $psi = [Diagnostics.ProcessStartInfo]::new()
        $psi.FileName = $script:Binary
        foreach ($a in @('config', '--json')) { $null = $psi.ArgumentList.Add($a) }
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $psi.UseShellExecute = $false
        $psi.CreateNoWindow = $true
        $psi.WorkingDirectory = $inner
        $p = [Diagnostics.Process]::Start($psi)
        $o = $p.StandardOutput.ReadToEndAsync(); $e = $p.StandardError.ReadToEndAsync()
        $p.WaitForExit()
        $null = $e.Result
        $d = Read-ToolJson -Stdout $o.Result -What 'config --json'
        $nearest = ($d.base.image -eq 'docker.io/library/alpine:latest')
        $named = ($d.path -eq (Join-Path $inner 'wsl-toolkit.json'))
        $ordered = (@($d.searched)[0] -eq (Join-Path $inner 'wsl-toolkit.json'))
        (($p.ExitCode -eq 0) -and $nearest -and $named -and $ordered).ToString()
    }

    # WSL-51: a spelling this tool does not read is REFUSED rather than skipped.
    Test-Case 'a configuration this tool does not read is refused by name' 'True' {
        $dir = Join-Path $script:Scratch 'cfg-toml'
        $null = New-Item -ItemType Directory -Path $dir -Force
        [IO.File]::WriteAllText((Join-Path $dir 'wsl-toolkit.toml'), "[base]`nname = 'wsl-toolkit'`n")
        $psi = [Diagnostics.ProcessStartInfo]::new()
        $psi.FileName = $script:Binary
        foreach ($a in @('config', '--json')) { $null = $psi.ArgumentList.Add($a) }
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $psi.UseShellExecute = $false
        $psi.CreateNoWindow = $true
        $psi.WorkingDirectory = $dir
        $p = [Diagnostics.Process]::Start($psi)
        $o = $p.StandardOutput.ReadToEndAsync(); $e = $p.StandardError.ReadToEndAsync()
        $p.WaitForExit()
        $said = (($e.Result + $o.Result) -match 'wsl-toolkit\.toml')
        (($p.ExitCode -eq 2) -and $said).ToString()
    }

    # WSL-43: TWO INSTANCES, BUILT IN ONE RUN. It is behind -Quick because it
    # builds a second distribution, which is minutes of pulling and importing.
    if (-not $Quick) {
        Test-Case 'two instances share no distribution, state, transcript or artifact' 'True' {
            $art = Join-Path $script:Scratch 'art-instances'
            try {
                $e = Invoke-Tool @('--instance', 'acc', 'base', 'ensure')
                if ($e.Code -ne 0) { return "the second instance would not build: $($e.Err)" }

                # Each runs a job that writes its own instance name.
                $one = Invoke-Tool @('run', '--json', '--image', 'alpine', '--artifacts', (Join-Path $art 'default'),
                    '-c', 'printf default > /out/who.txt; printf default')
                $two = Invoke-Tool @('--instance', 'acc', 'run', '--json', '--image', 'alpine', '--artifacts', (Join-Path $art 'acc'),
                    '-c', 'printf acc > /out/who.txt; printf acc')
                if ($one.Code -ne 0 -or $two.Code -ne 0) { return "a job failed: $($one.Code)/$($two.Code)" }
                $d1 = Read-ToolJson -Stdout $one.Out -What 'run --json'
                $d2 = Read-ToolJson -Stdout $two.Out -What 'run --json'

                # The artifacts came back to the right place.
                $w1 = [IO.File]::ReadAllText((Join-Path $art 'default\who.txt'))
                $w2 = [IO.File]::ReadAllText((Join-Path $art 'acc\who.txt'))
                if ($w1 -ne 'default' -or $w2 -ne 'acc') { return "artifacts crossed: $w1 / $w2" }

                # Neither transcript is in the other's state directory.
                if ($d1.transcript -eq $d2.transcript) { return 'both jobs wrote one transcript directory' }
                if ($d2.transcript -notmatch 'instances') { return "the second instance wrote to $($d2.transcript)" }

                # Each sees exactly its own distribution.
                $s1 = Read-ToolJson -Stdout (Invoke-Tool @('base', 'status', '--json')).Out -What 'base status'
                $s2 = Read-ToolJson -Stdout (Invoke-Tool @('--instance', 'acc', 'base', 'status', '--json')).Out -What 'base status'
                if ($s1.name -ne 'wsl-toolkit' -or $s2.name -ne 'wsl-toolkit-acc') { return "names: $($s1.name) / $($s2.name)" }

                # And gc on one leaves the other whole.
                $g = Invoke-Tool @('--instance', 'acc', 'gc', '--apply', '--json')
                if ($g.Code -ne 0) { return "gc on the second instance exited $($g.Code)" }
                $stillThere = Read-ToolJson -Stdout (Invoke-Tool @('logs', '--json')).Out -What 'logs --json'
                $kept = @($stillThere.transcripts | Where-Object { $_.id -eq $d1.id })
                ($kept.Count -eq 1).ToString()
            }
            finally {
                # TORN DOWN WHATEVER HAPPENED. A second distribution left
                # registered is exactly what the last case in this file exists
                # to catch, and leaving one would make this case fail that one
                # rather than itself.
                $null = Invoke-Tool @('--instance', 'acc', 'gc', '--apply')
                $null = Invoke-Tool @('--instance', 'acc', 'base', 'remove', '--yes')
            }
        }
    }

    # -- one command to readiness, and the one that moves off this version ---
    # WSL-49 and WSL-53.

    Test-Case 'ready proves the whole path with one command and one object' 'True' {
        $r = Invoke-Tool @('ready', '--smoke', '--json')
        $d = Read-ToolJson -Stdout $r.Out -What 'ready --json'
        if ($r.Code -ne 0) { return "ready exited $($r.Code): verdict $($d.verdict), problems $((@($d.problems) -join '; '))" }
        # Six facts, and the case asserts each rather than the verdict alone: a
        # verdict computed from six booleans is satisfied by a bug in the
        # computation as easily as by a machine that works.
        $s = $d.smoke
        $facts = @(
            $s.ran, ($s.uid -ne ''), ($s.kernel -ne ''),
            $s.work_writable, $s.artifact_returned, $s.transcript_readable, $s.no_host_mount)
        if (@($facts | Where-Object { -not $_ }).Count -gt 0) { return "smoke: $($s | ConvertTo-Json -Compress)" }
        (($d.ready -eq $true) -and ($d.verdict -eq 'ready') -and
         ($d.route.selected -eq 'direct') -and ($d.base.healthy -eq $true) -and
         ($d.base.identified -eq $true) -and ($d.catalog -ge 3)).ToString()
    }

    # A READINESS CHECK THAT BUILDS A DISTRIBUTION HAS CHANGED THE THING IT
    # WAS ASKED TO MEASURE. Without --ensure it reports and creates nothing, and
    # a second run is a fast no-op.
    Test-Case 'ready reports without building, and rerunning it changes nothing' 'True' -MaxSeconds 90 {
        $before = Read-ToolJson -Stdout (Invoke-Tool @('resources', '--json')).Out -What 'resources'
        $a = Read-ToolJson -Stdout (Invoke-Tool @('ready', '--json')).Out -What 'ready --json'
        $b = Read-ToolJson -Stdout (Invoke-Tool @('ready', '--json')).Out -What 'ready --json'
        $after = Read-ToolJson -Stdout (Invoke-Tool @('resources', '--json')).Out -What 'resources'
        $jobsBefore = @($before.owned.guest_jobs | Where-Object { $_ }).Count
        $jobsAfter = @($after.owned.guest_jobs | Where-Object { $_ }).Count
        (($a.verdict -eq $b.verdict) -and ($a.config.fingerprint -eq $b.config.fingerprint) -and
         ($jobsBefore -eq $jobsAfter)).ToString()
    }

    # An instance with no base is the first-run case, and the one an agent
    # actually meets.
    #
    # NOTE: an instance and NOT a bare --home. A fresh --home moves the STATE and
    # leaves the distribution at the default, which is registered here, so that
    # spelling answers `ready` and proves nothing. The first draft of this case
    # did exactly that and passed for the wrong reason until it was run.
    Test-Case 'ready answers for an instance whose base does not exist yet' 'True' {
        $r = Invoke-Tool @('--instance', 'nobase', 'ready', '--json')
        $d = Read-ToolJson -Stdout $r.Out -What 'ready --json'
        # What matters is that it ANSWERED, and named one exact command rather
        # than a subsystem.
        (($r.Code -eq 1) -and ($d.ready -eq $false) -and ($d.verdict -eq 'no-base') -and
         ($d.base.name -eq 'wsl-toolkit-nobase') -and
         (@($d.remediation).Count -ge 1) -and (@($d.remediation)[0] -match 'base ensure')).ToString()
    }

    Test-Case 'selfupdate --check names the running version and changes nothing' 'True' {
        $before = (Get-FileHash -LiteralPath $script:Binary -Algorithm SHA256).Hash
        $r = Invoke-Tool @('selfupdate', '--check', '--json')
        $d = Read-ToolJson -Stdout $r.Out -What 'selfupdate --check --json'
        $after = (Get-FileHash -LiteralPath $script:Binary -Algorithm SHA256).Hash
        $version = (Invoke-Tool @('version')).Out.Trim()
        # 0 current or ahead, 1 a newer release exists, 2 the question could
        # not be asked. All three are correct answers here; what is asserted is
        # that it said which, named the running version, and did not touch the
        # executable.
        #
        # ⛔ AND THAT A BUILD FROM THE WORKING TREE IS NOT OFFERED AN UPDATE.
        # The first version compared the two version strings for inequality, so
        # a tree bumped past the newest release was told to downgrade itself.
        # A suite that runs against a build from this tree would otherwise be
        # the one place that defect is invisible.
        $ahead = $false
        if ($d.PSObject.Properties.Name -contains 'latest' -and $d.latest) {
            $ahead = ([version]$version -gt [version]$d.latest)
        }
        if ($ahead -and $r.Code -ne 0) { return "a build ahead of $($d.latest) was told to update, exit $($r.Code)" }
        (($r.Code -in @(0, 1, 2)) -and ($d.checked_only -eq $true) -and
         ($d.running -eq $version) -and ($before -eq $after)).ToString()
    }

    # -- a heartbeat, and the six commands behind an answer -------------------
    # WSL-50 and WSL-52.

    Test-Case 'a job that outlives the tick interval says so, and stops saying it' 'True' {
        # A payload that writes something every few seconds, so the byte counts
        # RISE across ticks. A tick whose numbers never move is what a stall
        # looks like, and a case that did not check that would pass over one.
        $r = Invoke-Tool @('run', '--tick', '2s', '--timeout', '2m', '--image', 'alpine',
            '-c', 'i=0; while [ $i -lt 5 ]; do printf "chunk%s" $i; sleep 2; i=$((i+1)); done')
        if ($r.Code -ne 0) { return "the job exited $($r.Code): $($r.Err)" }
        $ticks = @(($r.Err -split "`n") | Where-Object { $_ -match '^\s+~ ' })
        if ($ticks.Count -lt 3) { return "only $($ticks.Count) tick(s) over a ten second job at 2s" }
        # The byte counts have to move, or this is a timer rather than a
        # heartbeat.
        $counts = @($ticks | ForEach-Object {
            if ($_ -match '(\d+)/\d+ bytes') { [int]$Matches[1] } else { -1 } })
        $rose = ($counts[-1] -gt $counts[0])
        # And nothing may tick after the answer: the last line of stderr is the
        # job's own summary, never a tick.
        $lastReal = @(($r.Err -split "`n") | Where-Object { $_.Trim() -ne '' })[-1]
        $tickLast = ($lastReal -match '^\s+~ ')
        ($rose -and (-not $tickLast)).ToString()
    }

    Test-Case 'nothing ticks unless it is asked to' 'True' {
        $r = Invoke-Tool @('run', '--image', 'alpine', '-c', 'sleep 3')
        if ($r.Code -ne 0) { return "the job exited $($r.Code): $($r.Err)" }
        (($r.Err -notmatch '^\s+~ ')).ToString()
    }

    Test-Case 'config validate refuses what the loader refuses and writes nothing' 'True' {
        $dir = Join-Path $script:Scratch 'cfg-validate'
        $null = New-Item -ItemType Directory -Path $dir -Force
        $bad = Join-Path $dir 'bad.json'
        [IO.File]::WriteAllText($bad, (@{
            schema = 'wsl-toolkit-config/1'
            base   = @{ name = 'Ubuntu'; image = 'docker.io/library/alpine:latest'; user = 'toolkit' }
        } | ConvertTo-Json -Depth 6), [Text.UTF8Encoding]::new($false))
        $good = Join-Path $dir 'good.json'
        [IO.File]::WriteAllText($good, (@{
            schema = 'wsl-toolkit-config/1'
            base   = @{ name = 'wsl-toolkit'; image = 'docker.io/library/alpine:latest'; user = 'toolkit' }
        } | ConvertTo-Json -Depth 6), [Text.UTF8Encoding]::new($false))
        $h = New-StateHome 'home-validate'
        $before = @(Get-ChildItem -LiteralPath $h -Force -ErrorAction SilentlyContinue).Count
        $r1 = Invoke-Tool @('--home', $h, 'config', 'validate', '--path', $bad, '--json')
        $d1 = Read-ToolJson -Stdout $r1.Out -What 'config validate --json'
        $r2 = Invoke-Tool @('--home', $h, 'config', 'validate', '--path', $good, '--json')
        $d2 = Read-ToolJson -Stdout $r2.Out -What 'config validate --json'
        $after = @(Get-ChildItem -LiteralPath $h -Force -ErrorAction SilentlyContinue).Count
        # It WRITES NOTHING, which is the whole point of it existing beside
        # config --write.
        (($r1.Code -eq 1) -and ($d1.valid -eq $false) -and ($d1.reason -match 'base\.name') -and
         ($r2.Code -eq 0) -and ($d2.valid -eq $true) -and ($before -eq $after)).ToString()
    }

    Test-Case 'config --effective prints a configuration that can be handed back' 'True' {
        $r = Invoke-Tool @('config', '--effective')
        if ($r.Code -ne 0) { return "config --effective exited $($r.Code): $($r.Err)" }
        $d = Read-ToolJson -Stdout $r.Out -What 'config --effective'
        # The round trip is the assertion: what it printed has to be a file this
        # tool accepts, or "can be handed back" is a claim rather than a fact.
        $round = Join-Path $script:Scratch 'effective.json'
        [IO.File]::WriteAllText($round, $r.Out, [Text.UTF8Encoding]::new($false))
        $v = Invoke-Tool @('config', 'validate', '--path', $round, '--json')
        $vd = Read-ToolJson -Stdout $v.Out -What 'config validate'
        (($d.schema -eq 'wsl-toolkit-config/1') -and (@($d.images).Count -ge 3) -and
         ($v.Code -eq 0) -and ($vd.valid -eq $true)).ToString()
    }

    Test-Case 'artifacts retry says so when nothing was retained, and never re-runs' 'True' {
        $r = Invoke-Tool @('run', '--json', '--image', 'alpine', '-c', 'printf NOT-RETAINED')
        $d = Read-ToolJson -Stdout $r.Out -What 'run --json'
        $to = Join-Path $script:Scratch 'retry-nothing'
        $a = Invoke-Tool @('artifacts', 'retry', $d.id, '--to', $to, '--json')
        $ad = Read-ToolJson -Stdout $a.Out -What 'artifacts retry --json'
        $ranAgain = ($a.Out + $a.Err) -match 'NOT-RETAINED'
        # A clean job keeps nothing, so this is the "nothing was retained" path
        # and it must not offer to produce the output again.
        (($a.Code -eq 1) -and ($ad.retained_kind -eq '') -and ($ad.reason -ne '') -and
         (-not $ranAgain)).ToString()
    }

    Test-Case 'artifacts retry fetches the copy a failed transfer kept' 'True' {
        $art = Join-Path $script:Scratch 'art-retry-fail'
        $r = Invoke-Tool @('run', '--json', '--image', 'alpine', '--artifacts', $art,
            '-c', 'printf recovered > /out/kept.txt; printf forbidden > /out/NUL.txt')
        $d = Read-ToolJson -Stdout $r.Out -What 'run --json'
        if ($d.retained_kind -ne 'guest') { return "the job retained nothing: $($d.artifact_error)" }
        $to = Join-Path $script:Scratch 'retry-recovered'
        $a = Invoke-Tool @('artifacts', 'retry', $d.id, '--to', $to, '--json')
        $ad = Read-ToolJson -Stdout $a.Out -What 'artifacts retry --json'
        # NUL.txt is still refused on the way out, so the retry fails the same
        # way and delivers nothing. What is asserted is that it FOUND the copy
        # and said so, rather than reporting there was none -- and that it put
        # an object on stdout while doing it, which the first version did not.
        (($ad.retained_kind -eq 'guest') -and ($ad.reason -ne '') -and
         ($ad.id -eq $d.id)).ToString()
    }

    Test-Case 'gc --job leaves every other job alone' 'True' {
        $one = Read-ToolJson -Stdout (Invoke-Tool @('run', '--json', '--image', 'alpine', '-c', 'true')).Out -What 'run'
        $two = Read-ToolJson -Stdout (Invoke-Tool @('run', '--json', '--image', 'alpine', '-c', 'true')).Out -What 'run'
        $g = Invoke-Tool @('gc', '--job', $one.id, '--apply', '--json')
        if ($g.Code -ne 0) { return "gc --job exited $($g.Code): $($g.Err)" }
        $plan = Read-ToolJson -Stdout $g.Out -What 'gc --json'
        $named = @($plan.removed | Where-Object { "$_" -match $one.id })
        $other = @($plan.removed | Where-Object { "$_" -match $two.id })
        # Both transcripts survive either way: gc removes guest state, and the
        # host transcript is what `logs` reads. What is asserted is that the
        # plan touched one job and not the other.
        ($other.Count -eq 0).ToString()
    }

    Test-Case 'images warm says what is here without going to a registry' 'True' -MaxSeconds 120 {
        $r = Invoke-Tool @('images', 'warm', '--select', 'alpine', '--json')
        $d = Read-ToolJson -Stdout $r.Out -What 'images warm --json'
        $row = @($d.images)[0]
        (($r.Code -eq 0) -and ($row.id -eq 'alpine') -and ($row.cached -eq $true) -and
         ($row.pulled -eq $false) -and ($d.unreachable -eq 0)).ToString()
    }

    Test-Case 'images pull reports an unreachable reference differently' 'True' {
        $h = New-StateHome 'home-unreachable'
        $null = Set-StateConfig -StateHome $h -Config @{
            images = @(
                @{ id = 'nosuch'; ref = 'docker.io/library/alpine:wsl-toolkit-no-such-tag-20260910'
                   libc = 'musl'; family = 'apk'; kind = 'musl' }
            )
        }
        $r = Invoke-Tool @('--home', $h, 'images', 'pull', '--json')
        $d = Read-ToolJson -Stdout $r.Out -What 'images pull --json'
        $row = @($d.images)[0]
        (($r.Code -eq 1) -and ($d.unreachable -eq 1) -and ($row.reachable -eq $false) -and
         ($row.reason -ne '')).ToString()
    }

    Test-Case 'examples names every command it teaches, and each parses as one' 'True' {
        $r = Invoke-Tool @('examples', '--json')
        $d = Read-ToolJson -Stdout $r.Out -What 'examples --json'
        $rows = @($d.examples)
        if ($rows.Count -lt 8) { return "only $($rows.Count) example(s)" }
        # Every example has to START with this tool's name, or it is teaching
        # something other than a call to it.
        $bad = @($rows | Where-Object { $_.command -notmatch '^wsl-toolkit ' })
        (($r.Code -eq 0) -and ($bad.Count -eq 0)).ToString()
    }

    Test-Case 'helper status says whose configuration the helper is running' 'True' {
        $start = Invoke-Tool @('helper', 'serve', '--detach', '--json')
        if ($start.Code -ne 0) { return "helper would not start: $($start.Err)" }
        try {
            $r = Invoke-Tool @('helper', 'status', '--json')
            $d = Read-ToolJson -Stdout $r.Out -What 'helper status --json'
            (($r.Code -eq 0) -and ($d.config_fingerprint -ne '') -and
             ($d.client_config_fingerprint -eq $d.config_fingerprint) -and
             ($d.config_matches_client -eq $true)).ToString()
        }
        finally { $null = Invoke-Tool @('helper', 'stop') }
    }

    # WSL-55. paths.go states the rule in the function it is about: a report that
    # creates a directory on a machine it is only describing has changed the
    # thing it was asked to measure. Every read-only command reached EnsureHome
    # through NewBase, NewRunner or OpenLedger, so every one of them broke it.
    Test-Case 'no read-only command creates the state directory it describes' 'True' {
        $created = @()
        foreach ($call in @(
                @('base', 'status', '--json'),
                @('ready', '--json'),
                @('resources', '--json'),
                @('logs', '--json'),
                @('config', '--json'),
                @('images', '--json'))) {
            $h = Join-Path $script:Scratch ('ro-' + ($call -join '-'))
            if (Test-Path -LiteralPath $h) { Remove-Item -LiteralPath $h -Recurse -Force }
            $null = Invoke-Tool (@('--home', $h) + $call)
            if (Test-Path -LiteralPath $h) { $created += ($call -join ' ') }
        }
        if ($created.Count -gt 0) { return "created a state directory: $($created -join '; ')" }
        # BOTH HALVES. A tool that stopped answering would pass the half above,
        # so a command that WRITES must still bring the directory into existence.
        $w = Join-Path $script:Scratch 'ro-writes'
        if (Test-Path -LiteralPath $w) { Remove-Item -LiteralPath $w -Recurse -Force }
        $r = Invoke-Tool @('--home', $w, 'config', '--write')
        (($r.Code -eq 0) -and (Test-Path -LiteralPath (Join-Path $w 'config.json'))).ToString()
    }

    # -- cleanup, counted rather than remembered -----------------------------
    Test-Case 'cleanup removes what this tool made and the counts return to zero' 'True' {
        $g = Invoke-Tool @('gc', '--apply', '--json')
        if ($g.Code -ne 0) { return "gc exited $($g.Code): $($g.Err)" }
        $r = Invoke-Tool @('resources', '--json')
        $d = $r.Out | ConvertFrom-Json
        $jobs = @($d.owned.guest_jobs | Where-Object { $_ } | ForEach-Object { $_.path })
        $open = @($d.owned.open_records | Where-Object { $_ } | ForEach-Object { $_.id })
        $containers = @($d.owned.containers | Where-Object { $_ } | ForEach-Object { $_.name })
        if ($jobs.Count -ne 0 -or $open.Count -ne 0 -or $containers.Count -ne 0) {
            # NAMES, not counts. A count says cleanup missed something; a name
            # says which command made the thing it missed.
            # NOTE: the plan omits an empty list, and Set-StrictMode makes
            # reading an absent property THROW. A diagnostic that throws
            # reports its own bug instead of the failure it was written for,
            # which is what happened the first time this case went red.
            $plan = $g.Out | ConvertFrom-Json
            $gcRemoved = 0
            $gcFailed = ''
            $gcKept = ''
            if ($plan.PSObject.Properties.Name -contains 'removed') { $gcRemoved = @($plan.removed).Count }
            if ($plan.PSObject.Properties.Name -contains 'failed') { $gcFailed = (@($plan.failed) -join ',') }
            if ($plan.PSObject.Properties.Name -contains 'kept') { $gcKept = (@($plan.kept) -join ',') }
            return ("left behind -- jobs: [{0}] records: [{1}] containers: [{2}] gc removed {3}, gc failed [{4}], gc kept [{5}]" -f `
                ($jobs -join ','), ($open -join ','), ($containers -join ','), `
                $gcRemoved, $gcFailed, $gcKept)
        }
        'True'
    }

    Test-Case 'the two pre-existing distributions are still registered and untouched' 'True' {
        $r = Invoke-Tool @('doctor', '--json', '--fast')
        $d = $r.Out | ConvertFrom-Json
        $names = @($d.wsl.distros | ForEach-Object { $_.name })
        # NOTE: A machine without them is not a failure of this tool. The case
        # asserts that whatever was there before is there now, which is what the
        # operator's constraint actually says.
        $missing = @($script:PreExisting | Where-Object { $names -notcontains $_ })
        if ($missing.Count -gt 0) { return "gone: $($missing -join ',')" }
        'True'
    }
}
finally {
    if (Test-Path -LiteralPath $script:Scratch) {
        Remove-Item -LiteralPath $script:Scratch -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# -- the report --------------------------------------------------------------
# HARD RULE: THE COUNT IS ASSERTED. A table that stopped early exits 0 over a
# smaller suite, and this is what makes that impossible.
$expected = if ($Quick) { 65 } else { 67 }
$ran = $script:Cases.Count
if ($ran -ne $expected) {
    $script:Failed++
    Write-Line ''
    Write-Line ("  FAIL  {0} case(s) ran and this file carries {1}" -f $ran, $expected)
}

if ($Json) {
    Write-Output (@{
        schema = 'wsl-toolkit-acceptance/1'
        ok     = ($script:Failed -eq 0)
        cases  = $ran
        failed = $script:Failed
        quick  = [bool]$Quick
        detail = $script:Cases
    } | ConvertTo-Json -Depth 5 -Compress)
}
else {
    Write-Line ''
    if ($script:Failed -eq 0) { Write-Line ("acceptance: {0} case(s) passed against a real machine." -f $ran) }
    else { [Console]::Error.WriteLine("acceptance FAILED: $script:Failed of $ran case(s).") }
}

if ($script:Failed -ne 0) { exit 1 }
exit 0
