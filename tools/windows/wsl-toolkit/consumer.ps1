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
#   pwsh -NoProfile -File tools/windows/wsl-toolkit/consumer.ps1 -Exe .tmp/wsl-toolkit.exe
#
# -Exe drives a binary from the working tree instead of downloading one. The
# four cases that check a RELEASE - the digests, the bundles, the signature and
# the tag - skip, because a local build has no release to check, and the job
# cases skip unless -WithJobs is also passed, because building a distribution
# costs minutes and the gate that runs this is measured in seconds. That is how
# a refusal added to the tool is caught BEFORE a tag rather than after one.
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
    [string]$Exe = '',
    [switch]$WithJobs,
    [switch]$KeepDownload
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:Cases = @()
$script:Failed = 0
$script:Skipped = 0
$script:JobsWhy = ''

# THE CASES THIS FILE CARRIES, BY NAME, and this is the one home for the list.
# It used to be the number 14 typed at the bottom, which is the shape a stale
# enumeration always takes: nothing about the number said which cases it was
# counting, so a case renamed in place read as a table that stopped early, and
# a case dropped read as nothing at all. The report compares the names REACHED
# against these, both ways, and derives the count from them.
#
# THE THREE GROUPS ARE WHAT CAN BE SKIPPED AS A GROUP, and each group names the
# thing it needs: the release group needs a published release and the job group
# needs a distribution.
$script:ReleaseCaseNames = @(
    'every digest in SHA256SUMS matches the file it names',
    'every published asset carries a signature bundle',
    'the signature verifies against this repository release workflow',
    'the executable reports the version named by the release tag',
    'the two files the manual tells a consumer to take are enough to run it')
$script:HostCaseNames = @(
    'the survey runs from an empty state directory and creates no distribution',
    'the catalog is fully qualified, which is what the manual says it is',
    'the state directory it names is the one it was told to use',
    'the usage text names the commands the manual documents')
$script:JobCaseNames = @(
    'a released binary runs a container job from an empty state directory',
    'a container gets a copy of a workspace and never the host directory',
    'a failing payload returns its own exit code',
    'what a job writes to /out comes back to the directory named',
    'a job past its deadline returns 124 and the caller is not held past it',
    'gc --apply removes what this run made')
$script:CaseNames = $script:ReleaseCaseNames + $script:HostCaseNames + $script:JobCaseNames

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


# THE ONE REASON A LOCAL DRIVE SKIPS A CASE, written once so the four release
# cases and the six job cases cannot describe the same run differently.
$script:LocalWhy = 'a binary from the working tree has no release to check'

function Test-ReleaseCase {
    <#
      A case only a PUBLISHED release can answer. On a -Exe run it skips with
      that as the reason.

      IT IS A WRAPPER AND NOT A COPY. Every case in this file, local or not,
      reaches Test-Case or Skip-Case, so the report accounts for all of them
      the same way and a local drive cannot become a quieter suite by leaving
      one out. The name is checked against $script:ReleaseCaseNames here,
      because a case that skips under a name the list does not carry would be
      invisible to the completeness assertion at the bottom.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Expected,
        [Parameter(Mandatory = $true)][scriptblock]$Body)
    if ($script:ReleaseCaseNames -notcontains $Name) {
        throw "Test-ReleaseCase was given a name that is not in `$script:ReleaseCaseNames: $Name"
    }
    if ($script:Local) {
        Skip-Case -Name $Name -Why $script:LocalWhy
        return
    }
    Test-Case -Name $Name -Expected $Expected -Body $Body
}
# NOTE: ProcessStartInfo.ArgumentList, not Start-Process. Start-Process JOINS
# the list and re-quotes it, which turns -c 'exit 37' into two arguments.
# NOTE: both streams are read before the wait, or a child that fills a pipe
# buffer deadlocks against the parent.
# NOTE: the exit code is read from the process, never through a pipe.
function Test-VersionAtLeast {
    <#
      Is the version under test at least the given one?

      ONE PLACE THAT PARSES THE VERSION, and it reads $script:Version rather
      than the tag, because an -Exe run has a version and no tag. THREE cases
      needed this and each wrote its own -split expression inline, which is how
      one of them ends up comparing a string and answering that 10 is less than
      9. The third was still inline when -Exe was added: this docstring claimed
      to be the one place while the usage-text case parsed the tag again four
      hundred lines below it.
    #>
    param([int]$Major, [int]$Minor = 0)
    $v = $script:Version -split '\.'
    if ($v.Count -lt 2) { return $false }
    $haveMajor = 0; $haveMinor = 0
    if (-not [int]::TryParse($v[0], [ref]$haveMajor)) { return $false }
    if (-not [int]::TryParse($v[1], [ref]$haveMinor)) { return $false }
    if ($haveMajor -ne $Major) { return ($haveMajor -gt $Major) }
    return ($haveMinor -ge $Minor)
}

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
    # THE INSTANCE IS AN ENVIRONMENT VARIABLE FOR THE SAME REASON THE HOME IS.
    # A flag this file forgot on one call would act on the DEFAULT distribution,
    # which is the operator's. Set here, no case can be written that misses it.
    $psi.Environment['WSL_TOOLKIT_INSTANCE'] = $script:Instance
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

      Set-StrictMode -Version Latest makes `$obj.absent` THROW, including
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

      IT TAKES EVERY ASSET, AND THAT IS DELIBERATE RATHER THAN AN OVERSIGHT.
      From wsl-toolkit-v3.0.0 a release also carries herdr for four targets,
      about 65 MiB, which this file never runs.
      ../../../docs/consumers.md tells a CONSUMER to take only the executable
      for its architecture and SHA256SUMS, so the two read as disagreeing.

      THEY ARE ANSWERING DIFFERENT QUESTIONS. This file is the only thing
      anywhere that verifies herdr's four builds are signed and match their
      digests; dropping them to save the bandwidth would delete that check and
      nothing would replace it. What the page describes is what a consumer
      NEEDS, and the case named below is what proves that smaller set is
      actually enough. TODO/PROGRESS.md finding 31.
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
    # SHA256SUMS first, because it names what else the release published.
    Invoke-WebRequest -Uri "$base/SHA256SUMS" -OutFile (Join-Path $Into 'SHA256SUMS') -UseBasicParsing
    $names = @('SHA256SUMS')
    foreach ($line in [IO.File]::ReadAllLines((Join-Path $Into 'SHA256SUMS'))) {
        $parts = $line.Trim() -split '\s+', 2
        if ($parts.Count -eq 2) { $names += $parts[1].TrimStart('*') }
    }
    foreach ($name in $names) {
        if ($name -ne 'SHA256SUMS') {
            Invoke-WebRequest -Uri "$base/$name" -OutFile (Join-Path $Into $name) -UseBasicParsing
        }
        # A release before wsl-toolkit-v2.0.1 carries no signature bundles, so
        # their absence is a fact the signature cases report rather than a fetch
        # failure here. `gh release download` with no --pattern already takes
        # whatever is there.
        try {
            Invoke-WebRequest -Uri "$base/$name$script:SignatureSuffix" -OutFile (Join-Path $Into "$name$script:SignatureSuffix") -UseBasicParsing
        }
        catch { $null = $_ }
    }
}

function Resolve-LatestTag {
    $gh = Get-Command gh -CommandType Application -ErrorAction SilentlyContinue
    if (-not $gh) { return '' }
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        # Prereleases are excluded before the limit applies: this repository publishes herdr's
        # nightly builds as prereleases, and thirty of them would hide every wsl-toolkit release.
        $out = & $gh.Source 'release' 'list' '--repo' $Repo '--exclude-pre-releases' '--limit' '30' '--json' 'tagName' 2>&1
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
# -Exe AND -Tag NAME TWO DIFFERENT THINGS TO TEST, so asking for both is a
# refusal rather than a resolution. -Tag says which published release to fetch
# and check; -Exe says there is no release here at all. Picking one over the
# other would hand a caller who typed both a suite they did not ask for.
if ($Exe -and $Tag) { Exit-Cannot '-Exe and -Tag cannot be given together: -Exe drives a binary from the working tree and -Tag fetches a published release' }
if ($WithJobs -and -not $Exe) { Exit-Cannot '-WithJobs only means anything beside -Exe: a release run probes for a distribution on its own' }
$script:Local = [bool]$Exe
if (-not $script:Local) {
    if (-not $Tag) { $Tag = Resolve-LatestTag }
    if (-not $Tag) { Exit-Cannot 'no tag given and the latest could not be resolved. Pass -Tag wsl-toolkit-vX.Y.Z' }
}
$script:Tag = $Tag
$script:SignatureSuffix = '.cosign.bundle'

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
# base, adopted it, and unregistered it in its own teardown.
#
# WSL-43 LANDED, so the selection is an INSTANCE now rather than a name typed
# into a file. Invoke-Released sets WSL_TOOLKIT_INSTANCE, which moves the
# distribution and the state directory together, and WSL-74 REFUSES the shape
# this file used to write: a base.name of `wsl-toolkit-consumer` with no
# instance selected named one distribution while recording into another's
# state. The 3.0.0 smoke run is where that refusal first reached a consumer.
#
# THE FILE IS WRITTEN AT BOTH PATHS, AND THAT IS DELIBERATE. -Tag accepts any
# published tag and nine of them exist; the ones from before instances landed
# ignore WSL_TOOLKIT_INSTANCE and read <home>/config.json. Writing only the
# instance's copy would leave those binaries with no configuration at all,
# defaulting to `wsl-toolkit`, which is the operator's own base, which this
# file's teardown then removes. One document, two files, so NO tag this script
# accepts can reach a real distribution.
$script:Instance = 'consumer'
$script:BaseName = "wsl-toolkit-$script:Instance"
$script:InstanceHome = Join-Path (Join-Path $script:StateHome 'instances') $script:Instance
$null = New-Item -ItemType Directory -Path $script:InstanceHome -Force
$script:ConfigJson = @{ schema = 'wsl-toolkit-config/1'
    base   = @{ name = $script:BaseName; image = 'ghcr.io/pkgforge-dev/archlinux:latest'; user = 'toolkit' }
} | ConvertTo-Json -Depth 6
foreach ($d in @($script:StateHome, $script:InstanceHome)) {
    [IO.File]::WriteAllText(
        (Join-Path $d 'config.json'), $script:ConfigJson, [Text.UTF8Encoding]::new($false))
}

if ($script:Local) { Write-Line "consumer: the working-tree binary $Exe" }
else {
    Write-Line "consumer: $Repo $($script:Tag)"
    Write-Line "download: $script:Download"
}
Write-Line "state:    $script:StateHome"
Write-Line "base:     $script:BaseName (this run's own, never the operator's)"
Write-Line ''

if ($script:Local) {
    # RESOLVED BEFORE ANYTHING RUNS. Invoke-Released sets a working directory
    # of its own, and a relative -Exe left unresolved would be looked for in
    # THAT directory, which this file deliberately leaves empty.
    $resolved = $null
    try { $resolved = (Resolve-Path -LiteralPath $Exe -ErrorAction Stop).Path } catch { $null = $_ }
    if (-not $resolved -or -not (Test-Path -LiteralPath $resolved -PathType Leaf)) {
        Exit-Cannot "-Exe names no file: $Exe"
    }
    $script:Exe = $resolved
}
else {
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
}

# THE VERSION UNDER TEST HAS ONE HOME, read from the tag on a release run and
# from the binary itself on a local one. Every assertion about what a release
# carries reads it through Test-VersionAtLeast, so neither path can grow its
# own idea of which version it is looking at.
if ($script:Local) {
    $probeVersion = Invoke-Released @('version')
    if ($probeVersion.Code -ne 0) {
        Exit-Cannot "-Exe $($script:Exe) could not answer version: exit $($probeVersion.Code) $($probeVersion.Err)"
    }
    $script:Version = $probeVersion.Out.Trim()
}
else { $script:Version = $script:Tag -replace '^wsl-toolkit-v', '' }

# A distribution takes minutes to build and needs a network and an engine. Cases
# that need one say so, and their absence is a SKIP that is counted rather than
# a pass.
$script:CanRunJobs = $false

# What release.yml names each asset's signature bundle. The files it signs are
# SHA256SUMS and every file SHA256SUMS names, read from the release itself, so
# the weekly run verifies whatever the latest release published rather than a
# list typed here for one version of it.
$script:Executables = @('wsl-toolkit-windows-amd64.exe', 'wsl-toolkit-windows-arm64.exe')
# text-tool IS PUBLISHED FROM 3.1.0 AND NOT BEFORE, so the list is gated on the
# tag rather than typed once. -Tag accepts any of the published tags and the
# older ones carry no text-tool at all; asserting it for them would report a
# correct release as broken.
$script:HasTextTool = (Test-VersionAtLeast -Major 3 -Minor 1)
if ($script:HasTextTool) {
    $script:Executables += @(
        'text-tool-windows-amd64.exe', 'text-tool-windows-arm64.exe',
        'text-tool-linux-amd64', 'text-tool-linux-arm64')
}
$script:SignedAssets = @('SHA256SUMS')
$sumsFile = Join-Path $script:Download 'SHA256SUMS'
if (Test-Path -LiteralPath $sumsFile) {
    foreach ($line in [IO.File]::ReadAllLines($sumsFile)) {
        $parts = $line.Trim() -split '\s+', 2
        if ($parts.Count -eq 2) { $script:SignedAssets += $parts[1].TrimStart('*') }
    }
}

try {
    # -- what the release itself claims --------------------------------------

    # The manual's install section says the digests in SHA256SUMS cover the
    # bytes that were uploaded. That is checkable in one pass and it is the
    # first thing a consumer should do.
    Test-ReleaseCase 'every digest in SHA256SUMS matches the file it names' 'True' {
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
        # written against, so what it covered is asserted before the verdict:
        # both executables, whatever else the release also published.
        $named = @($script:SignedAssets | Select-Object -Skip 1)
        $absent = @($script:Executables | Where-Object { $named -notcontains $_ })
        if ($absent.Count -gt 0) { return "SHA256SUMS covers $seen file(s) and does not name $($absent -join ', ')" }
        if ($bad.Count -gt 0) { return ($bad -join ' | ') }
        'True'
    }

    # WHAT THE DIGESTS ABOVE CANNOT SAY. SHA256SUMS ships in the same release as
    # the assets it covers, so anyone who could replace one could replace the
    # other. The signature is what speaks to authorship. WSL-25.
    $bundles = @($script:SignedAssets | Where-Object {
        Test-Path -LiteralPath (Join-Path $script:Download ($_ + $script:SignatureSuffix))
    })
    if ($script:Local -or $bundles.Count -eq 0) {
        # NOT A FAILURE AND NOT A PASS. A release cut before signing existed
        # genuinely has none, and the weekly run points at whatever is latest.
        # Counting it as a pass would be the case answering itself. A local
        # binary has no bundle either, for a different reason, so the two
        # reasons are told apart rather than sharing one wording.
        $why = if ($script:Local) { $script:LocalWhy } else { 'this release predates asset signing' }
        Skip-Case 'every published asset carries a signature bundle' $why
        Skip-Case 'the signature verifies against this repository release workflow' $why
    }
    else {
        Test-Case 'every published asset carries a signature bundle' 'True' {
            $missing = @($script:SignedAssets | Where-Object {
                -not (Test-Path -LiteralPath (Join-Path $script:Download ($_ + $script:SignatureSuffix)))
            })
            if ($missing.Count -gt 0) {
                return ('signed ' + $bundles.Count + ' of ' + $script:SignedAssets.Count + ', missing: ' + ($missing -join ', '))
            }
            'True'
        }

        $cosign = Get-Command cosign -CommandType Application -ErrorAction SilentlyContinue
        if (-not $cosign) {
            Skip-Case 'the signature verifies against this repository release workflow' 'cosign is not installed on this machine'
        }
        else {
            # The claim the bundle makes, checked with the same command and the
            # same identity a consumer uses, from outside the repository that
            # made it. The identity is anchored on the repository and the
            # workflow and NOT on the ref, so the same identity covers every release tag.
            Test-Case 'the signature verifies against this repository release workflow' 'True' {
                $identity = '^https://github\.com/' + [regex]::Escape($Repo) + '/\.github/workflows/release\.yml@'
                $bad = @()
                foreach ($name in $script:SignedAssets) {
                    $file = Join-Path $script:Download $name
                    $prev = $ErrorActionPreference
                    $ErrorActionPreference = 'Continue'
                    try {
                        $out = & $cosign.Source verify-blob --bundle ($file + $script:SignatureSuffix) `
                            --certificate-identity-regexp $identity `
                            --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' `
                            $file 2>&1
                        $code = $LASTEXITCODE
                    }
                    finally { $ErrorActionPreference = $prev }
                    if ($code -ne 0) { $bad += ($name + ': cosign exited ' + $code + ' ' + ((@($out) -join ' ').Trim())) }
                }
                if ($bad.Count -gt 0) { return ($bad -join ' | ') }
                'True'
            }
        }
    }

    # The text and structured surfaces read the same native version, and both
    # must agree with the immutable release tag.
    Test-ReleaseCase 'the executable reports the version named by the release tag' 'True' {
        $v = Invoke-Released @('version')
        if ($v.Code -ne 0) { return "version exited $($v.Code): $($v.Err)" }
        $reported = $v.Out.Trim()
        if ($script:Tag -ne "wsl-toolkit-v$reported") {
            return "the tag is $($script:Tag) and the binary reports $reported"
        }
        $j = Read-ToolJson -Stdout (Invoke-Released @('version', '--json')).Out -What 'version --json'
        if ($j.schema -ne 'wsl-toolkit-version/1') { return "version schema is $($j.schema)" }
        if ($j.version -ne $reported) { return "text reports $reported and JSON reports $($j.version)" }
        'True'
    }


    # WHAT THE PAGE TELLS A CONSUMER TO TAKE, AND WHETHER THAT IS ENOUGH.
    #
    # docs/consumers.md says to download "the executable matching the host
    # architecture and SHA256SUMS" and nothing else. This file downloads every
    # asset, because it is also the only thing that verifies herdr's builds are
    # signed, so nothing here had ever tested the SMALLER set the page
    # describes. Finding 31 read that as a page disagreeing with a script; the
    # disagreement is real and the untested claim is the half that could bite.
    #
    # It copies rather than downloading again: the bytes are already here and a
    # second fetch would test GitHub rather than the release.
    Test-ReleaseCase 'the two files the manual tells a consumer to take are enough to run it' 'True' {
        $only = Join-Path $script:Elsewhere 'minimal'
        $null = New-Item -ItemType Directory -Path $only -Force
        $exeName = Split-Path -Leaf $script:Exe
        Copy-Item -LiteralPath $script:Exe -Destination (Join-Path $only $exeName) -Force
        Copy-Item -LiteralPath (Join-Path $script:Download 'SHA256SUMS') -Destination $only -Force
        $left = @(Get-ChildItem -LiteralPath $only -File | ForEach-Object { $_.Name })
        if ($left.Count -ne 2) { return "the minimal set holds $($left.Count) file(s): $($left -join ', ')" }

        # The digest a consumer is told to check, checked the way they would.
        $want = $null
        foreach ($line in [IO.File]::ReadAllLines((Join-Path $only 'SHA256SUMS'))) {
            $parts = $line.Trim() -split '\s+', 2
            if ($parts.Count -eq 2 -and $parts[1].TrimStart('*') -eq $exeName) { $want = $parts[0].ToLowerInvariant() }
        }
        if (-not $want) { return "SHA256SUMS does not name $exeName" }
        $got = (Get-FileHash -LiteralPath (Join-Path $only $exeName) -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($got -ne $want) { return "the copy is $got and SHA256SUMS says $want" }

        # And it RUNS from there, with nothing else beside it.
        $psi = [Diagnostics.ProcessStartInfo]::new()
        $psi.FileName = Join-Path $only $exeName
        foreach ($a in @('version')) { $null = $psi.ArgumentList.Add($a) }
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $psi.UseShellExecute = $false
        $psi.Environment['WSL_TOOLKIT_HOME'] = $script:StateHome
        $psi.Environment['WSL_TOOLKIT_INSTANCE'] = $script:Instance
        $psi.WorkingDirectory = $only
        $p = [Diagnostics.Process]::Start($psi)
        $o = $p.StandardOutput.ReadToEndAsync()
        $e = $p.StandardError.ReadToEndAsync()
        $p.WaitForExit()
        if ($p.ExitCode -ne 0) { return "the minimal set could not run: exit $($p.ExitCode) $($e.Result)" }
        if ($o.Result.Trim() -ne $script:Version) { return "it reports $($o.Result.Trim()) and the release is $($script:Version)" }
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
        # The commands every release documents, and from 3.0.0 the throwaway
        # distribution commands that replaced the PowerShell product.
        $commandsNamed = @('doctor', 'base', 'images', 'run', 'matrix', 'resources', 'gc', 'logs', 'inspect', 'helper', 'config', 'version')
        if (Test-VersionAtLeast -Major 3) { $commandsNamed += @('distro', 'hostaddress') }
        foreach ($c in $commandsNamed) {
            if ($text -notmatch ("(?m)^\s+" + [regex]::Escape($c) + "\s")) { $missing += $c }
        }
        if ($missing.Count -gt 0) { return "not in the usage text: $($missing -join ',')" }
        'True'
    }

    # -- can this machine go further? -----------------------------------------
    #
    # THE TOOL HAS A COMMAND FOR THIS QUESTION AND THIS FILE USED TO HAND-ROLL
    # ONE. It read `base status --json` and treated anything but exit 2 as "jobs
    # can run here", which is true on a machine with WSL2 and wrong on one
    # without: a GitHub windows runner has docker and no WSL, so `base status`
    # answered, five job cases ran, and every one of them FAILED over a host
    # that was never going to be able to run them. A suite that fails where it
    # should skip is a suite whose red means nothing.
    #
    # AND THE PROBE IS THE THING ITSELF, not a signal that correlates with it.
    # The second attempt read `ready --json` and gated on `route.wsl_callable`,
    # which is a better question than the first one asked and still the wrong
    # one: wsl.exe IS callable on a GitHub windows runner, and a distribution
    # still cannot be built there. Every proxy for "can this host run a job"
    # eventually meets a host where the proxy and the answer disagree.
    #
    # `base ensure` is the answer, and it costs nothing extra: the first job
    # case had to run it anyway. A host that cannot build a base skips the job
    # cases with the engine's own words attached, which is a SKIP that carries
    # its reason rather than a red over a machine that was never going to work.
    #
    # A LOCAL DRIVE DOES NOT PROBE AT ALL UNLESS IT IS ASKED TO, and the reason
    # is the caller -Exe exists for. The gate runs this file over the
    # working-tree binary on every commit, and on a host that CAN build a
    # distribution `base ensure` costs minutes and about a gigabyte, which is
    # not a gate. -WithJobs asks for them anyway, which is what a session
    # driving the whole suite locally passes. The skip says which of the two it
    # was, because "no distribution here" and "you did not ask" are different
    # facts and a run that confused them would hide a broken host.
    if ($script:Local -and -not $WithJobs) {
        $script:JobsWhy = 'a local drive does not build a distribution: pass -WithJobs to run these'
        Write-Line ("  " + $script:JobsWhy)
        Write-Line ''
    }
    else {
        Write-Line '  probing: base ensure, which is the only honest answer to whether jobs can run here'
        $probe = Invoke-Released @('base', 'ensure')
        if ($probe.Code -ne 0) {
            $why = (@($probe.Err -split "`n") | Where-Object { $_.Trim() } | Select-Object -Last 1)
            $script:JobsWhy = "no reachable distribution on this host: $why"
            Write-Line ''
            Write-Line ("  no distribution can be built on this host, so the job cases are skipped: " + $why)
            Write-Line ''
        }
        else {
            $script:CanRunJobs = $true
        }
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
        # THE NAMES COME FROM THE LIST, not from a second copy of them typed
        # here. The six were written out twice until 2026-09-17, which is a
        # value in two places with no check that they agree.
        foreach ($n in $script:JobCaseNames) {
            Skip-Case -Name $n -Why $script:JobsWhy
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
# THE SET IS ASSERTED, NOT A COUNT. A table that stopped early exits 0 over a
# smaller suite, which is the shape a check takes on its way to reporting
# nothing, and the guard against that used to be the literal number 14 typed
# here. A number says nothing about WHICH case went missing, it cannot tell a
# case renamed in place from a case dropped, and it has to be edited by hand
# every time the table grows. The names REACHED are compared with the names
# DECLARED, both ways: one direction catches a case that never ran, the other
# catches a case running under a name nothing accounts for.
#
# IT IS ALSO WHAT KEEPS -Exe HONEST. A local drive reaches every case in the
# list - the ones it cannot answer arrive as skips carrying their reason - so
# a case quietly left out of the local path fails this assertion rather than
# making the run smaller.
$reached = @($script:Cases | ForEach-Object { $_.name })
$missed = @($script:CaseNames | Where-Object { $reached -notcontains $_ })
$extra = @($reached | Where-Object { $script:CaseNames -notcontains $_ })
$script:Complete = (($missed.Count -eq 0) -and ($extra.Count -eq 0))
$ran = $script:Cases.Count
if (-not $script:Complete) {
    $script:Failed++
    Write-Line ''
    Write-Line ("  FAIL  {0} case(s) ran and this file declares {1}" -f $ran, $script:CaseNames.Count)
    if ($missed.Count -gt 0) { Write-Line ("        never reached: " + ($missed -join " | ")) }
    if ($extra.Count -gt 0) { Write-Line ("        not declared : " + ($extra -join " | ")) }
}

$against = if ($script:Local) { "$($script:Exe) ($($script:Version))" } else { $script:Tag }
if ($Json) {
    Write-Output (@{
            schema   = 'wsl-toolkit-consumer/1'
            ok       = ($script:Failed -eq 0)
            # THE SUBJECT, and exactly one of the two is ever set. A reader of
            # this document must be able to tell a release run from a local one
            # without inferring it from which cases were skipped.
            tag      = $script:Tag
            exe      = $(if ($script:Local) { $script:Exe } else { '' })
            version  = $script:Version
            cases    = $ran
            # complete says every case this file DECLARES was reached. A run
            # that is not complete has already failed above; the field is here
            # so a caller reading the document does not have to re-derive it.
            complete = $script:Complete
            failed   = $script:Failed
            skipped  = $script:Skipped
            detail   = $script:Cases
        } | ConvertTo-Json -Depth 5 -Compress)
}
else {
    Write-Line ''
    if ($script:Failed -eq 0) {
        Write-Line ("consumer: {0} case(s) passed against {1}, {2} skipped." -f ($ran - $script:Skipped), $against, $script:Skipped)
    }
    else { [Console]::Error.WriteLine("consumer FAILED: $script:Failed of $ran case(s) against $against.") }
}

if ($script:Failed -ne 0) { exit 1 }
exit 0
