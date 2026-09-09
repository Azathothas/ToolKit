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
function Test-Case {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Expected,
        [Parameter(Mandatory = $true)][scriptblock]$Body
    )
    $actual = ''
    # NOTE: not $error. That is an automatic variable, and a local of the same
    # name IS it, because PowerShell ignores case in variable names.
    $caught = ''
    $started = Get-Date
    try { $actual = [string](& $Body) }
    catch { $caught = $_.Exception.Message; $actual = "THREW: $($_.Exception.Message)" }
    $pass = ($actual -eq $Expected)
    $script:Cases += [ordered]@{
        name = $Name; expected = $Expected; actual = $actual
        pass = $pass; seconds = [math]::Round(((Get-Date) - $started).TotalSeconds, 2)
    }
    if ($pass) { Write-Line ("  ok    {0}" -f $Name) }
    else {
        $script:Failed++
        Write-Line ("  FAIL  {0}" -f $Name)
        Write-Line ("        expected: {0}" -f $Expected)
        Write-Line ("        actual  : {0}" -f $actual)
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

    Test-Case 'a hostile artifact name is refused rather than written outside' 'True' {
        $art = Join-Path $script:Scratch 'art-hostile'
        $sentinel = Join-Path $script:Scratch 'ESCAPED.txt'
        if (Test-Path -LiteralPath $sentinel) { Remove-Item -LiteralPath $sentinel -Force }
        # The container writes a real file AND asks tar to carry a name that
        # climbs out. The tool packs /out itself, so the escape is attempted
        # through a link, which the extractor records rather than follows.
        $r = Invoke-Tool @('run', '--image', 'alpine', '--artifacts', $art,
            '-c', 'printf ok > /out/fine.txt; ln -s /etc/hostname /out/escape')
        $escaped = Test-Path -LiteralPath $sentinel
        $recorded = Test-Path -LiteralPath (Join-Path $art 'escape.link.txt')
        $isLink = $false
        if (Test-Path -LiteralPath (Join-Path $art 'escape')) { $isLink = $true }
        (($r.Code -eq 0) -and (-not $escaped) -and $recorded -and (-not $isLink)).ToString()
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
            return ("left behind -- jobs: [{0}] records: [{1}] containers: [{2}] gc removed {3}, gc failed [{4}]" -f `
                ($jobs -join ','), ($open -join ','), ($containers -join ','), `
                @($g.Out | ConvertFrom-Json | ForEach-Object { $_.removed }).Count, `
                (@($g.Out | ConvertFrom-Json | ForEach-Object { $_.failed }) -join ','))
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
$expected = if ($Quick) { 22 } else { 23 }
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
