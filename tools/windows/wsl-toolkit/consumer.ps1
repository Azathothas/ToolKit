# consumer.ps1 - test a PUBLISHED release the way somebody who does not have
# this repository would.
#
# WHY THIS EXISTS, and it is the whole point of the file. Every case in
# acceptance.ps1 runs a binary built from the working tree, against state the
# case just created, with the tree beside it. Seventeen defects in two sessions
# were found from a different vantage point: an outside agent that downloaded
# the release, verified its digests, and ran it from an empty directory with no
# knowledge of the tree. Nothing in this repository occupied that vantage point
# and this file is it. TOOL-17.
#
# WHAT IT ASSERTS: the MANUAL's claims, and nothing else.
#
# HARD RULE: IT DOES NOT ASSERT INTERNALS. acceptance.ps1 owns those, and a
# second suite that starts checking the same things has become a copy of the
# first that drifts from it. If a claim here is not written in
# tools/windows/wsl-toolkit/wsl-toolkit.md, it does not belong here.
#
# WHAT IT REQUIRES: a network, the gh CLI or a plain HTTPS reach to the release
# assets, and - for the cases marked so - a Windows host with WSL2 and a
# container engine. Without the second it still runs every case that does not
# need a distribution, and says how many it skipped.
#
# HARD RULE: IT NEVER TOUCHES THE OPERATOR'S OWN STATE. Everything it does runs
# under a state directory it creates in a temp folder and removes at the end, so
# a consumer run cannot disturb the base a real session is using.
#
# Usage:
#   pwsh -NoProfile -File tools/windows/wsl-toolkit/consumer.ps1
#   pwsh -NoProfile -File tools/windows/wsl-toolkit/consumer.ps1 -Tag wsl-toolkit-v1.3.0
#   pwsh -NoProfile -File tools/windows/wsl-toolkit/consumer.ps1 -Tag ... -Json
#
# Exit codes: 0 every case passed, 1 a case failed, 2 could not run.
# Read the exit code from this process, unpiped.
#
# ASCII-ONLY ON PURPOSE. docs/conventions/shell.md section 8: a .ps1 holding any
# non-ASCII byte needs a UTF-8 BOM before Windows PowerShell 5.1 decodes it
# correctly. Staying ASCII removes the requirement rather than depending on it.
[CmdletBinding(PositionalBinding = $false)]
param(
    [string]$Tag = '',
    [string]$Repo = 'Azathothas/ToolKit',
    [switch]$Json,
    [switch]$KeepDownload
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:Cases = @()
$script:Failed = 0
$script:Skipped = 0

function Write-Line { param([string]$Text) if (-not $Json) { Write-Output $Text } }

function Exit-Cannot {
    param([string]$Reason)
    if ($Json) {
        Write-Output (@{ schema = 'wsl-toolkit-consumer/1'; ok = $false; reason = $Reason; cases = @() } |
            ConvertTo-Json -Depth 5 -Compress)
    }
    else { [Console]::Error.WriteLine("consumer: $Reason") }
    exit 2
}

# Same shape as acceptance.ps1's, and deliberately a second copy rather than a
# shared module: this file has to run from a directory that does NOT have this
# repository in it, which is the property it exists to test. A dot-source of a
# sibling would make that impossible.
function Test-Case {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Expected,
        [Parameter(Mandatory = $true)][scriptblock]$Body,
        [double]$MaxSeconds = 0
    )
    $actual = ''
    $caught = ''
    $started = Get-Date
    try { $actual = [string](& $Body) }
    catch { $caught = $_.Exception.Message; $actual = "THREW: $($_.Exception.Message)" }
    $seconds = [math]::Round(((Get-Date) - $started).TotalSeconds, 2)
    $pass = ($actual -eq $Expected)
    $overrun = ($MaxSeconds -gt 0 -and $seconds -gt $MaxSeconds)
    if ($overrun) { $pass = $false }
    $entry = [ordered]@{ name = $Name; expected = $Expected; actual = $actual; pass = $pass; seconds = $seconds }
    if ($MaxSeconds -gt 0) { $entry['max_seconds'] = $MaxSeconds }
    $script:Cases += $entry
    if ($pass) { Write-Line ("  ok    {0}" -f $Name) }
    else {
        $script:Failed++
        Write-Line ("  FAIL  {0}" -f $Name)
        if ($overrun) { Write-Line ("        wall time: {0}s, and this case allows {1}s" -f $seconds, $MaxSeconds) }
        if ($actual -ne $Expected) {
            Write-Line ("        expected: {0}" -f $Expected)
            Write-Line ("        actual  : {0}" -f $actual)
        }
        if ($caught) { Write-Line ("        error   : {0}" -f $caught) }
    }
}

function Skip-Case {
    param([string]$Name, [string]$Why)
    $script:Skipped++
    $script:Cases += [ordered]@{ name = $Name; skipped = $true; why = $Why }
    Write-Line ("  skip  {0} ({1})" -f $Name, $Why)
}

# NOTE: ProcessStartInfo.ArgumentList, not Start-Process. Start-Process JOINS
# the list and re-quotes it, which turns -c 'exit 37' into two arguments.
# NOTE: both streams are read before the wait, or a child that fills a pipe
# buffer deadlocks against the parent.
# NOTE: the exit code is read from the process, never through a pipe.
function Invoke-Released {
    param([Parameter(Mandatory = $true)][string[]]$ToolArgs)
    $psi = [Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = $script:Exe
    foreach ($a in $ToolArgs) { $null = $psi.ArgumentList.Add($a) }
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    # THE STATE DIRECTORY IS AN ENVIRONMENT VARIABLE, not a flag, because a
    # consumer that forgets the flag once would write into the operator's own
    # state and this file must not be able to.
    $psi.Environment['WSL_TOOLKIT_HOME'] = $script:StateHome
    # The consumer's working directory is NOT this repository. That is the
    # property being tested: a released binary must not need the tree.
    $psi.WorkingDirectory = $script:Elsewhere
    $p = [Diagnostics.Process]::Start($psi)
    $outTask = $p.StandardOutput.ReadToEndAsync()
    $errTask = $p.StandardError.ReadToEndAsync()
    $p.WaitForExit()
    return [pscustomobject]@{ Code = $p.ExitCode; Out = $outTask.Result; Err = $errTask.Result }
}

function Get-Field {
    <#
      One property read that a MISSING field does not turn into an exception.

      ⛔ Set-StrictMode -Version Latest makes `$obj.absent` THROW, including
      inside a `$null -ne $obj.absent` test, so the guard written to tolerate a
      missing field is the line that dies on it. This file reads documents
      produced on a machine it knows nothing about: a host with no WSL answers
      `doctor --json` without the field a host with WSL carries, and the case
      that allowed for that threw on the ubuntu-side runner instead of passing.
      Found by putting this suite in CI, which is what TOOL-12 was for.
    #>
    param($Object, [Parameter(Mandatory = $true)][string]$Name)
    if ($null -eq $Object) { return $null }
    $p = $Object.PSObject.Properties[$Name]
    if ($null -eq $p) { return $null }
    return $p.Value
}

function Read-ToolJson {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Stdout, [string]$What = 'the command')
    if ($null -eq $Stdout -or $Stdout.Trim() -eq '') { throw "$What advertises --json and put nothing on stdout" }
    $obj = $null
    try { $obj = $Stdout | ConvertFrom-Json }
    catch { throw "$What put something on stdout that is not JSON" }
    if ($obj -is [Array]) { throw "$What put $($obj.Count) JSON documents on stdout and an answer is one" }
    if ($null -eq $obj) { throw "$What put a JSON null on stdout" }
    return $obj
}

# -- fetch -------------------------------------------------------------------

function Get-ReleaseAssets {
    <#
      Downloads the release's assets into a directory and returns it.

      It uses gh where gh is present, because gh resolves the tag, follows the
      asset redirects and reports a missing release as an error rather than as
      an HTML page saved to disk. Where gh is absent it falls back to the public
      download URLs, which need no credential.
    #>
    param([Parameter(Mandatory = $true)][string]$Into)
    $gh = Get-Command gh -CommandType Application -ErrorAction SilentlyContinue
    if ($gh) {
        $ghArgs = @('release', 'download', $script:Tag, '--repo', $Repo, '--dir', $Into, '--clobber')
        $prev = $ErrorActionPreference
        $ErrorActionPreference = 'Continue'
        try { $out = & $gh.Source @ghArgs 2>&1; $code = $LASTEXITCODE }
        finally { $ErrorActionPreference = $prev }
        if ($code -ne 0) { throw "gh release download exited $code`: $((@($out) -join ' ').Trim())" }
        return
    }
    $base = "https://github.com/$Repo/releases/download/$($script:Tag)"
    foreach ($name in @('SHA256SUMS', 'wsl-toolkit.ps1', 'launcher.ps1',
            'wsl-toolkit-windows-amd64.exe', 'wsl-toolkit-windows-arm64.exe')) {
        Invoke-WebRequest -Uri "$base/$name" -OutFile (Join-Path $Into $name) -UseBasicParsing
    }
}

function Resolve-LatestTag {
    $gh = Get-Command gh -CommandType Application -ErrorAction SilentlyContinue
    if (-not $gh) { return '' }
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $out = & $gh.Source 'release' 'list' '--repo' $Repo '--limit' '30' '--json' 'tagName' 2>&1
        $code = $LASTEXITCODE
    }
    finally { $ErrorActionPreference = $prev }
    if ($code -ne 0) { return '' }
    $tags = @((@($out) -join '') | ConvertFrom-Json | ForEach-Object { $_.tagName } |
        Where-Object { $_ -like 'wsl-toolkit-v*' })
    if ($tags.Count -eq 0) { return '' }
    return $tags[0]
}

# -- setup -------------------------------------------------------------------

if ($PSVersionTable.PSEdition -ne 'Core') {
    Exit-Cannot 'this runner needs PowerShell 7 or later, because it passes arguments as a list rather than as a joined string'
}
if (-not $Tag) { $Tag = Resolve-LatestTag }
if (-not $Tag) { Exit-Cannot 'no tag given and the latest could not be resolved. Pass -Tag wsl-toolkit-vX.Y.Z' }
$script:Tag = $Tag

$stamp = [Guid]::NewGuid().ToString('N').Substring(0, 8)
$script:Root = Join-Path ([IO.Path]::GetTempPath()) ("wsl-toolkit-consumer." + $stamp)
$script:Download = Join-Path $script:Root 'release'
$script:StateHome = Join-Path $script:Root 'state'
# THE WORKING DIRECTORY A CONSUMER RUNS FROM, and it holds nothing. A released
# binary that needs a file from the repository fails here and passes everywhere
# this repository is checked out, which is the shape that shipped seventeen
# defects.
$script:Elsewhere = Join-Path $script:Root 'elsewhere'
foreach ($d in @($script:Download, $script:Elsewhere, $script:StateHome)) {
    $null = New-Item -ItemType Directory -Path $d -Force
}

# THE DISTRIBUTION THIS RUN BUILDS IS NOT THE OPERATOR'S.
#
# A separate state directory is NOT enough on its own, and assuming it was is
# how this file nearly removed a real base. The distribution name comes from the
# configuration and defaults to `wsl-toolkit` whatever WSL_TOOLKIT_HOME says, so
# a consumer run under a temp home would have found the operator's registered
# base, adopted it, and unregistered it in its own teardown. WSL-43 is the entry
# that makes an instance a first-class thing; until it lands, the name is set
# here, BEFORE the first invocation, so no case in this file can reach the real
# one.
$script:BaseName = 'wsl-toolkit-consumer'
[IO.File]::WriteAllText(
    (Join-Path $script:StateHome 'config.json'),
    (@{ schema = 'wsl-toolkit-config/1'
        base   = @{ name = $script:BaseName; image = 'ghcr.io/pkgforge-dev/archlinux:latest'; user = 'toolkit' }
    } | ConvertTo-Json -Depth 6),
    [Text.UTF8Encoding]::new($false))

Write-Line "consumer: $Repo $($script:Tag)"
Write-Line "download: $script:Download"
Write-Line "state:    $script:StateHome"
Write-Line "base:     $script:BaseName (this run's own, never the operator's)"
Write-Line ''

try {
    Get-ReleaseAssets -Into $script:Download
}
catch {
    Exit-Cannot "could not download $($script:Tag): $($_.Exception.Message)"
}

$arch = if ([Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$script:Exe = Join-Path $script:Download "wsl-toolkit-windows-$arch.exe"
if (-not (Test-Path -LiteralPath $script:Exe -PathType Leaf)) {
    Exit-Cannot "the release carries no wsl-toolkit-windows-$arch.exe"
}

# A distribution takes minutes to build and needs a network and an engine. Cases
# that need one say so, and their absence is a SKIP that is counted rather than
# a pass.
$script:CanRunJobs = $false

try {
    # -- what the release itself claims --------------------------------------

    # The manual's install section says the digests in SHA256SUMS cover the
    # bytes that were uploaded. That is checkable in one pass and it is the
    # first thing a consumer should do.
    Test-Case 'every digest in SHA256SUMS matches the file it names' 'True' {
        $sums = Join-Path $script:Download 'SHA256SUMS'
        if (-not (Test-Path -LiteralPath $sums)) { return 'the release carries no SHA256SUMS' }
        $bad = @()
        $seen = 0
        foreach ($line in [IO.File]::ReadAllLines($sums)) {
            $t = $line.Trim()
            if ($t -eq '') { continue }
            # `sha256sum` writes "<hex>  <name>", with the name possibly marked
            # binary by a leading asterisk.
            $parts = $t -split '\s+', 2
            if ($parts.Count -ne 2) { $bad += "unparsable line: $t"; continue }
            $want = $parts[0].ToLowerInvariant()
            $name = $parts[1].TrimStart('*')
            $path = Join-Path $script:Download $name
            if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { $bad += "$name is named and was not downloaded"; continue }
            $seen++
            $got = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
            if ($got -ne $want) { $bad += "$name is $got and SHA256SUMS says $want" }
        }
        # A file that checked nothing and exited 0 is the failure this case is
        # written against, so the count is asserted before the verdict.
        if ($seen -lt 4) { return "SHA256SUMS covered $seen file(s), and the release carries four assets and the sums file" }
        if ($bad.Count -gt 0) { return ($bad -join ' | ') }
        'True'
    }

    # The manual says the executable CARRIES the script and reads its version
    # out of it, so the two cannot be different products.
    Test-Case 'the executable and the published script are the same product' 'True' {
        $v = Invoke-Released @('version')
        if ($v.Code -ne 0) { return "version exited $($v.Code): $($v.Err)" }
        $reported = $v.Out.Trim()
        if ($script:Tag -ne "wsl-toolkit-v$reported") {
            return "the tag is $($script:Tag) and the binary reports $reported"
        }
        $j = Read-ToolJson -Stdout (Invoke-Released @('version', '--json')).Out -What 'version --json'
        if (-not $j.script_reversible) { return 'the embedded script does not reconstruct the tracked file' }
        # The digest the binary reports for its embedded copy has to be the
        # digest of the .ps1 the same release published, or the release is two
        # products under one tag.
        $published = Join-Path $script:Download 'wsl-toolkit.ps1'
        $onDisk = (Get-FileHash -LiteralPath $published -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($j.script_sha256 -ne $onDisk) {
            return "the binary embeds $($j.script_sha256) and the release published $onDisk"
        }
        'True'
    }

    # -- what the manual promises a first-run agent ---------------------------

    Test-Case 'the survey runs from an empty state directory and creates no distribution' 'True' {
        $before = @()
        $r = Invoke-Released @('doctor', '--json', '--fast')
        if ($r.Code -ne 0) { return "doctor exited $($r.Code): $($r.Err)" }
        $d = Read-ToolJson -Stdout $r.Out -What 'doctor --json'
        if ($d.schema -ne 'agent-doctor/1') { return "schema is $($d.schema)" }
        # Every hop through Get-Field, because a host with no WSL answers this
        # document without the fields a host with WSL carries.
        $list = Get-Field (Get-Field $d 'wsl') 'distros'
        if ($null -ne $list) {
            $before = @(@($list) | Where-Object { Get-Field $_ 'owned' } | ForEach-Object { Get-Field $_ 'name' })
        }
        # doctor is a report. A report that registers a distribution has changed
        # the thing it was asked to measure.
        if ($before.Count -ne 0) { return "doctor reported an owned distribution before anything built one: $($before -join ',')" }
        'True'
    }

    Test-Case 'the catalog is fully qualified, which is what the manual says it is' 'True' {
        $r = Invoke-Released @('images', '--json')
        if ($r.Code -ne 0) { return "images exited $($r.Code): $($r.Err)" }
        $d = Read-ToolJson -Stdout $r.Out -What 'images --json'
        $bad = @($d.images | Where-Object { $_.ref -notmatch '^[a-z0-9.-]+\.[a-z]{2,}/' -and $_.ref -notmatch '^localhost/' })
        if ($bad.Count -gt 0) { return "unqualified: $($bad[0].ref)" }
        if (@($d.images).Count -lt 3) { return "the catalog has $(@($d.images).Count) entries" }
        'True'
    }

    Test-Case 'the state directory it names is the one it was told to use' 'True' {
        $r = Invoke-Released @('config', '--json')
        if ($r.Code -ne 0) { return "config exited $($r.Code): $($r.Err)" }
        $d = Read-ToolJson -Stdout $r.Out -What 'config --json'
        $named = [string]$d.path
        if ($named -eq '') { return 'config named no configuration file' }
        if (-not $named.StartsWith($script:StateHome, [StringComparison]::OrdinalIgnoreCase)) {
            return "WSL_TOOLKIT_HOME was $($script:StateHome) and config says $named"
        }
        'True'
    }

    # The manual documents every flag the binary has. A released binary that
    # grew one the manual does not carry is a capability nobody can find.
    Test-Case 'the usage text names the commands the manual documents' 'True' {
        $r = Invoke-Released @('help')
        $text = $r.Out + $r.Err
        $missing = @()
        foreach ($c in @('doctor', 'script', 'base', 'images', 'run', 'matrix',
                'resources', 'gc', 'logs', 'inspect', 'helper', 'config', 'version')) {
            if ($text -notmatch ("(?m)^\s+" + [regex]::Escape($c) + "\s")) { $missing += $c }
        }
        if ($missing.Count -gt 0) { return "not in the usage text: $($missing -join ',')" }
        'True'
    }

    # -- can this machine go further? -----------------------------------------
    #
    # ⛔ THE TOOL HAS A COMMAND FOR THIS QUESTION AND THIS FILE USED TO HAND-ROLL
    # ONE. It read `base status --json` and treated anything but exit 2 as "jobs
    # can run here", which is true on a machine with WSL2 and wrong on one
    # without: a GitHub windows runner has docker and no WSL, so `base status`
    # answered, five job cases ran, and every one of them FAILED over a host
    # that was never going to be able to run them. A suite that fails where it
    # should skip is a suite whose red means nothing.
    #
    # ⛔ AND THE PROBE IS THE THING ITSELF, not a signal that correlates with it.
    # The second attempt read `ready --json` and gated on `route.wsl_callable`,
    # which is a better question than the first one asked and still the wrong
    # one: wsl.exe IS callable on a GitHub windows runner, and a distribution
    # still cannot be built there. Every proxy for "can this host run a job"
    # eventually meets a host where the proxy and the answer disagree.
    #
    # ⭐ `base ensure` is the answer, and it costs nothing extra: the first job
    # case had to run it anyway. A host that cannot build a base skips the job
    # cases with the engine's own words attached, which is a SKIP that carries
    # its reason rather than a red over a machine that was never going to work.
    Write-Line '  probing: base ensure, which is the only honest answer to whether jobs can run here'
    $probe = Invoke-Released @('base', 'ensure')
    if ($probe.Code -ne 0) {
        $why = (@($probe.Err -split "`n") | Where-Object { $_.Trim() } | Select-Object -Last 1)
        Write-Line ''
        Write-Line ("  no distribution can be built on this host, so the job cases are skipped: " + $why)
        Write-Line ''
    }
    else {
        $script:CanRunJobs = $true
    }

    if ($script:CanRunJobs) {
        # The manual's central claim: one command runs one command in one
        # container and returns its output.
        # The base is already up: the probe above built it, which is what made
        # skipping possible on a host that cannot.
        Test-Case 'a released binary runs a container job from an empty state directory' 'uid=0' {
            $r = Invoke-Released @('run', '--image', 'alpine', '-c', 'printf "uid=%s" "$(id -u)"')
            if ($r.Code -ne 0) { return "run exited $($r.Code): $($r.Err)" }
            $r.Out.Trim()
        }

        # The manual says a container gets a COPY of a workspace and no host
        # mount, so destroying it leaves the host copy whole.
        Test-Case 'a container gets a copy of a workspace and never the host directory' 'True' {
            $ws = Join-Path $script:Elsewhere 'ws'
            $null = New-Item -ItemType Directory -Path $ws -Force
            [IO.File]::WriteAllText((Join-Path $ws 'keep.txt'), 'PRECIOUS')
            $r = Invoke-Released @('run', '--image', 'alpine', '--workspace', $ws,
                '-c', 'rm -rf /work/* 2>/dev/null; ls -A /work | wc -l')
            if ($r.Code -ne 0) { return "run exited $($r.Code): $($r.Err)" }
            $kept = [IO.File]::ReadAllText((Join-Path $ws 'keep.txt'))
            (($kept -eq 'PRECIOUS') -and ($r.Out.Trim() -eq '0')).ToString()
        }

        # The manual says the payload's own exit code is what a caller gets.
        Test-Case 'a failing payload returns its own exit code' '37' {
            ([string](Invoke-Released @('run', '--image', 'alpine', '-c', 'exit 37')).Code)
        }

        # The manual says an artifact written to /out comes back.
        Test-Case 'what a job writes to /out comes back to the directory named' 'DELIVERED' {
            $art = Join-Path $script:Elsewhere 'art'
            $r = Invoke-Released @('run', '--image', 'alpine', '--artifacts', $art,
                '-c', 'printf DELIVERED > /out/result.txt')
            if ($r.Code -ne 0) { return "run exited $($r.Code): $($r.Err)" }
            $p = Join-Path $art 'result.txt'
            if (-not (Test-Path -LiteralPath $p)) { return 'nothing was delivered' }
            [IO.File]::ReadAllText($p)
        }

        # The manual documents --timeout as a deadline and 124 as what a job
        # past one returns. A deadline that does not bound the caller's waiting
        # is the defect WSL-45 is about, and a consumer is who notices.
        Test-Case 'a job past its deadline returns 124 and the caller is not held past it' '124' -MaxSeconds 12 {
            ([string](Invoke-Released @('run', '--image', 'alpine', '--timeout', '2s', '-c', 'sleep 60')).Code)
        }

        # The manual says gc removes what this tool made, and the consumer's own
        # teardown depends on it: this run must leave the machine as it found it.
        Test-Case 'gc --apply removes what this run made' 'True' {
            $g = Invoke-Released @('gc', '--apply', '--json')
            if ($g.Code -ne 0) { return "gc exited $($g.Code): $($g.Err)" }
            $null = Read-ToolJson -Stdout $g.Out -What 'gc --json'
            $r = Invoke-Released @('resources', '--json')
            $d = Read-ToolJson -Stdout $r.Out -What 'resources --json'
            $left = @()
            foreach ($k in @('guest_jobs', 'open_records', 'containers')) {
                if ($d.owned.PSObject.Properties.Name -contains $k) {
                    $left += @($d.owned.$k | Where-Object { $_ })
                }
            }
            if ($left.Count -ne 0) { return "gc left $($left.Count) thing(s) behind" }
            'True'
        }
    }
    else {
        foreach ($n in @(
                'a released binary runs a container job from an empty state directory',
                'a container gets a copy of a workspace and never the host directory',
                'a failing payload returns its own exit code',
                'what a job writes to /out comes back to the directory named',
                'a job past its deadline returns 124 and the caller is not held past it',
                'gc --apply removes what this run made')) {
            Skip-Case -Name $n -Why 'no reachable distribution on this host'
        }
    }
}
finally {
    # TEARDOWN, and it is not conditional on the run having gone well. The base
    # this run may have built is REMOVED: it was created under a temporary state
    # home, so leaving it registered would leave a distribution nothing owns.
    #
    # It removes $script:BaseName and the tool refuses any other name, so the
    # operator's own base is out of reach of this line by construction rather
    # than by this script being careful.
    if ($script:CanRunJobs) {
        $null = Invoke-Released @('base', 'remove', '--yes')
    }
    if (-not $KeepDownload -and (Test-Path -LiteralPath $script:Root)) {
        Remove-Item -LiteralPath $script:Root -Recurse -Force -ErrorAction SilentlyContinue
    }
    elseif ($KeepDownload) {
        Write-Line ''
        Write-Line "kept: $script:Root"
    }
}

# -- the report --------------------------------------------------------------
# THE COUNT IS ASSERTED. A table that stopped early exits 0 over a smaller
# suite, which is the shape a check takes on its way to reporting nothing.
$expected = 12
$ran = $script:Cases.Count
if ($ran -ne $expected) {
    $script:Failed++
    Write-Line ''
    Write-Line ("  FAIL  {0} case(s) ran and this file carries {1}" -f $ran, $expected)
}

if ($Json) {
    Write-Output (@{
            schema  = 'wsl-toolkit-consumer/1'
            ok      = ($script:Failed -eq 0)
            tag     = $script:Tag
            cases   = $ran
            failed  = $script:Failed
            skipped = $script:Skipped
            detail  = $script:Cases
        } | ConvertTo-Json -Depth 5 -Compress)
}
else {
    Write-Line ''
    if ($script:Failed -eq 0) {
        Write-Line ("consumer: {0} case(s) passed against {1}, {2} skipped." -f ($ran - $script:Skipped), $script:Tag, $script:Skipped)
    }
    else { [Console]::Error.WriteLine("consumer FAILED: $script:Failed of $ran case(s) against $script:Tag.") }
}

if ($script:Failed -ne 0) { exit 1 }
exit 0
