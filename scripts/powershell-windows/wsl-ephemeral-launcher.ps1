#Requires -Version 5.1

<#
.SYNOPSIS
    Resolve wsl-ephemeral.ps1, make it safe to run, and run it with the
    arguments given here.

.DESCRIPTION
    ONE FILE A CALLER FETCHES. Everything wsl-ephemeral.ps1 needs before it can
    run on a Windows host is here: finding a copy, verifying it, clearing the
    download mark Windows puts on it, and putting it somewhere a session can
    reach by name. Then it runs it and propagates its exit code.

    WHY IT IS NOT A PINNED WRAPPER

    Azathothas/TEMPLATE carries a wrapper that fetches this repository's script
    at a hardcoded commit and digest. That shape is right THERE and wrong HERE:
    a pin inside the repository that owns the file can only ever name one of its
    own ancestors, so it is stale the moment the file it points at changes, and
    the file sitting next to it is the newer one.

    So this resolves in order, and the first hit wins:

      1. -LauncherLocal PATH, or WSL_EPHEMERAL_LOCAL. An explicit file.
      2. wsl-ephemeral.ps1 BESIDE THIS FILE. The clone case, and no network.
      3. -LauncherRef SHA, or WSL_EPHEMERAL_REF. Fetched from this repository
         at that exact revision.

    THERE IS NO DEFAULT REF, ON PURPOSE. With no sibling and no ref it refuses
    and prints the command that resolves one. Falling back to a branch would be
    running code nobody reviewed, which is the rule this repository states first
    and everywhere.

    WHAT IT DOES THAT A curl AND A pwsh DO NOT

      - REFUSES A MOVING REF by shape. A 40-character commit, or an explicit
        -LauncherAllowMovingRef, and nothing in between.
      - VERIFIES A DIGEST when one is given, and refuses on a mismatch rather
        than warning.
      - PARSES THE FILE AS POWERSHELL BEFORE RUNNING IT, so a captive portal or
        a 404 body arriving with HTTP 200 cannot reach the execution path.
      - CLEARS THE MARK OF THE WEB. A file downloaded on Windows carries a
        Zone.Identifier alternate data stream, and an execution policy that
        would run a local script refuses the same bytes with that stream on
        them. The error names the policy and not the stream, which is what makes
        it worth doing rather than explaining.
      - CACHES BY REF, so changing the ref cannot serve the old copy.

.PARAMETER LauncherHelp
    Not declared, and neither is anything else. Every argument is forwarded to
    wsl-ephemeral.ps1 verbatim EXCEPT the -Launcher* switches listed below,
    which are removed first.

    Restating the inner script's parameter list here is how a wrapper drifts
    from the thing it wraps, so it is not restated. The Launcher prefix is what
    makes that safe: the inner script cannot grow a parameter that collides with
    one of these, whatever it adds.

      -LauncherLocal PATH        run this file. No network.
      -LauncherRef SHA           fetch this revision from Azathothas/ToolKit.
      -LauncherSha256 HEX        expect this SHA-256 of the fetched bytes.
      -LauncherAllowMovingRef    permit a branch or a tag. Prints a warning.
      -LauncherInstallDir DIR    where a fetched copy is kept.
      -LauncherAddToPath         put the directory on PATH and DO NOT RUN.
      -LauncherHelp              print this and stop.

    The environment equivalents, for a caller that would rather not touch the
    argument list at all:

      WSL_EPHEMERAL_LOCAL, WSL_EPHEMERAL_REF, WSL_EPHEMERAL_SHA256,
      WSL_EPHEMERAL_ALLOW_MOVING_REF=1, WSL_EPHEMERAL_CACHE

.EXAMPLE
    .\wsl-ephemeral-launcher.ps1 -Action List

.EXAMPLE
    .\wsl-ephemeral-launcher.ps1 -LauncherRef 7127ff7... -Action New -Image alpine:3.22 -Ephemeral -Force

.EXAMPLE
    . .\wsl-ephemeral-launcher.ps1 -LauncherAddToPath

.NOTES
    -LauncherAddToPath AND DOT-SOURCING, WHICH IS THE ONE THING TO READ HERE.

    A child process cannot change its parent's environment. Run this normally
    and it can install the script and PRINT the line that puts it on PATH; it
    cannot put it there. DOT-SOURCE it and the assignment happens in the calling
    session, which is what the caller actually wanted:

        . .\wsl-ephemeral-launcher.ps1 -LauncherAddToPath

    A launcher that claimed to have changed PATH from a child process would be
    reporting a result it never read, so it does not claim it.

    DOT-SOURCING IS REFUSED FOR EVERY OTHER USE, and that is not tidiness.
    wsl-ephemeral.ps1 calls `exit`, which ends the HOST SESSION when it is
    reached through a dot-source rather than an invocation.

    Requires : Windows PowerShell 5.1 or PowerShell 7+.
    ASCII-ONLY ON PURPOSE. docs/conventions/shell.md section 8: a .ps1 holding
    any non-ASCII byte needs a UTF-8 BOM before 5.1 will decode it correctly.
    Staying ASCII removes the requirement rather than depending on it.
#>

param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$UpstreamOwner = 'Azathothas'
$UpstreamRepo  = 'ToolKit'
$UpstreamPath  = 'scripts/powershell-windows/wsl-ephemeral.ps1'
$ScriptLeaf    = 'wsl-ephemeral.ps1'

# EVERY LINE THIS FILE PRINTS GOES TO STDERR, and there is no Write-Host in it
# at all. A wrapper that writes to the wrapped program's stdout corrupts it:
# `wsl-ephemeral.ps1 -Action HostAddress` puts one address there and nothing
# else, and a progress line from out here arrives in the same stream. Measured
# on 2026-08-29: with Write-Host, a caller capturing this launcher's stdout got
# "==> Using the copy beside this launcher" ahead of the address.
function Write-Step { param([string]$Message) [Console]::Error.WriteLine("==> $Message") }
function Write-Ok   { param([string]$Message) [Console]::Error.WriteLine("  * $Message") }
function Write-Warn { param([string]$Message) [Console]::Error.WriteLine("  ! $Message") }
function Write-Note { param([string]$Message) [Console]::Error.WriteLine($Message) }

function Get-EnvOrDefault {
    param([Parameter(Mandatory = $true)][string]$Name, [AllowEmptyString()][string]$Fallback = '')
    $v = [Environment]::GetEnvironmentVariable($Name)
    if ([string]::IsNullOrWhiteSpace($v)) { return $Fallback }
    return $v.Trim()
}

function Split-LauncherArgument {
    <#
      Take the -Launcher* switches out of the argument list and hand back both
      halves: the options this file understands, and everything else, in the
      order it arrived.

      A switch with no value is a flag; one with a value consumes the next
      argument. An unknown -Launcher* argument is REFUSED rather than forwarded:
      forwarding it would reach the inner script, which would refuse it with a
      message about a parameter it does not have, and the caller would go
      looking in the wrong file.

      HOW THE FORWARD LIST IS BUILT IS LOAD-BEARING AND IT LOOKS LIKE STYLE.
      A splatted array is re-parsed as a command line, so `-Action` in it binds
      as a PARAMETER NAME. That property does not survive every way of building
      an array. Measured on this host on 2026-08-29, forwarding -Action a to a
      script with a ValidateSet on -Action:

        $a = @(); for (...) { $a += $args[$i] }   -> Action=[a]
        @($args | Where-Object { $true })         -> Action=[a]
        $args[0..($args.Count - 1)]               -> Action=[a]
        through an [object[]] parameter           -> Action=[a]
        (New-Object ArrayList).ToArray()          -> REFUSED, -Action bound
                                                     POSITIONALLY as the VALUE
                                                     of -Action

      Every element is a System.String in all five cases, so nothing about the
      values explains it. The first version of this file used the last row and
      no argument reached the inner script correctly.
    #>
    param([Parameter(Mandatory = $true)][AllowEmptyCollection()][object[]]$Argument)

    $opt = @{
        Local = ''; Ref = ''; Sha256 = ''; InstallDir = ''
        AllowMovingRef = $false; AddToPath = $false; Help = $false
    }
    # An ordinary array with +=, NOT an ArrayList. See the note above: this is
    # the difference between an argument list and a positional mess.
    $rest = @()
    $takesValue = @{
        '-launcherlocal' = 'Local'; '-launcherref' = 'Ref'
        '-launchersha256' = 'Sha256'; '-launcherinstalldir' = 'InstallDir'
    }
    $flags = @{
        '-launcherallowmovingref' = 'AllowMovingRef'
        '-launcheraddtopath' = 'AddToPath'
        '-launcherhelp' = 'Help'
    }

    for ($i = 0; $i -lt $Argument.Count; $i++) {
        $a = [string]$Argument[$i]
        $k = $a.ToLowerInvariant()
        if ($takesValue.ContainsKey($k)) {
            if ($i + 1 -ge $Argument.Count) { throw "$a needs a value." }
            $opt[$takesValue[$k]] = [string]$Argument[$i + 1]
            $i++
            continue
        }
        if ($flags.ContainsKey($k)) { $opt[$flags[$k]] = $true; continue }
        if ($k.StartsWith('-launcher')) {
            throw ("$a is not a launcher option. The set is -LauncherLocal, -LauncherRef, " +
                   "-LauncherSha256, -LauncherAllowMovingRef, -LauncherInstallDir, " +
                   "-LauncherAddToPath and -LauncherHelp.")
        }
        $rest += $Argument[$i]
    }
    return [pscustomobject]@{ Options = $opt; Forward = $rest }
}

function Assert-PowerShellSyntax {
    <#
      A downloaded file that is not PowerShell must never reach the execution
      path. The realistic case is not a hostile payload, it is a captive portal
      or a 404 body arriving with HTTP 200.
    #>
    param([Parameter(Mandatory = $true)][string]$LiteralFile)
    $errs = $null
    $tokens = $null
    $null = [System.Management.Automation.Language.Parser]::ParseFile($LiteralFile, [ref]$tokens, [ref]$errs)
    if ($errs -and $errs.Count -gt 0) {
        throw ("The file at $LiteralFile is not valid PowerShell (first error: line " +
               $errs[0].Extent.StartLineNumber + ": " + $errs[0].Message + "). " +
               "For a fetched file this usually means a proxy or a captive portal answered " +
               "instead of GitHub.")
    }
}

function Get-Sha256 {
    param([Parameter(Mandatory = $true)][string]$LiteralFile)
    return (Get-FileHash -LiteralPath $LiteralFile -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Clear-DownloadMark {
    <#
      Remove the Zone.Identifier stream Windows attaches to a downloaded file,
      and SAY WHETHER THERE WAS ONE. A caller that hits the execution-policy
      refusal gets an error naming the policy rather than the stream, so knowing
      the mark was there is most of the diagnosis.

      Returns $true when a mark was found and cleared.

      This is not a change to the machine or to any policy: it clears one
      attribute on one file, which is what Unblock-File exists for.
    #>
    param([Parameter(Mandatory = $true)][string]$LiteralFile)
    $had = $false
    try {
        $s = Get-Item -LiteralPath $LiteralFile -Stream 'Zone.Identifier' -ErrorAction Stop
        if ($s) { $had = $true }
    }
    catch { $null = $_ }     # no stream, or a filesystem with no stream support
    if (-not $had) { return $false }
    try { Unblock-File -LiteralPath $LiteralFile -ErrorAction Stop; return $true }
    catch {
        Write-Warn "could not clear the download mark: $($_.Exception.Message)"
        return $false
    }
}

function Save-Upstream {
    <# Download to a temp file in the destination directory, then rename. #>
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$Destination
    )
    $dir = Split-Path -Parent $Destination
    if (-not (Test-Path -LiteralPath $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    $temp = Join-Path $dir ('.download.' + [Guid]::NewGuid().ToString('N') + '.tmp')

    # Windows PowerShell 5.1 negotiates TLS 1.0 on some machines and GitHub
    # refuses it, which surfaces as "The request was aborted: Could not create
    # SSL/TLS secure channel" and reads like an outage. PowerShell 7 already
    # defaults correctly; setting it is harmless there.
    try {
        [Net.ServicePointManager]::SecurityProtocol =
            [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    }
    catch { $null = $_ }

    # The progress bar makes Invoke-WebRequest an order of magnitude slower on
    # 5.1 and writes nothing a caller needs.
    $prevProgress = $ProgressPreference
    $ProgressPreference = 'SilentlyContinue'
    try {
        Invoke-WebRequest -Uri $Uri -OutFile $temp -UseBasicParsing -TimeoutSec 30
        if (-not (Test-Path -LiteralPath $temp)) { throw "download produced no file" }
        if ((Get-Item -LiteralPath $temp).Length -lt 1KB) {
            throw "downloaded file is implausibly small ($((Get-Item -LiteralPath $temp).Length) bytes)"
        }
        Move-Item -LiteralPath $temp -Destination $Destination -Force
    }
    finally {
        $ProgressPreference = $prevProgress
        if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Force -ErrorAction SilentlyContinue }
    }
}

function Resolve-Upstream {
    <#
      Find the script, in the documented order, and return the path to a copy
      that has been verified as far as the caller allowed.
    #>
    param([Parameter(Mandatory = $true)]$Options)

    # 1. an explicit local file
    $local = $Options.Local
    if (-not $local) { $local = Get-EnvOrDefault 'WSL_EPHEMERAL_LOCAL' }
    if ($local) {
        if (-not (Test-Path -LiteralPath $local -PathType Leaf)) {
            throw "-LauncherLocal points at '$local', which is not a file."
        }
        return (Resolve-Path -LiteralPath $local).Path
    }

    # 2. the sibling, which is the clone case and needs no network at all
    $here = Split-Path -Parent $PSCommandPath
    if ($here) {
        $sibling = Join-Path $here $ScriptLeaf
        if (Test-Path -LiteralPath $sibling -PathType Leaf) {
            Write-Step "Using the copy beside this launcher"
            return (Resolve-Path -LiteralPath $sibling).Path
        }
    }

    # 3. fetch, at a revision the caller named
    $ref = $Options.Ref
    if (-not $ref) { $ref = Get-EnvOrDefault 'WSL_EPHEMERAL_REF' }
    if (-not $ref) {
        throw ("No copy of $ScriptLeaf beside this launcher, and no revision to fetch. " +
               "There is no default: a branch moves, and a moved reference runs code nobody " +
               "reviewed. Resolve one and pass it as -LauncherRef:`n" +
               "    gh api repos/$UpstreamOwner/$UpstreamRepo/commits/main --jq .sha")
    }

    $allowMoving = $Options.AllowMovingRef -or ((Get-EnvOrDefault 'WSL_EPHEMERAL_ALLOW_MOVING_REF') -eq '1')
    if ($ref -notmatch '^[0-9a-fA-F]{40}$') {
        if (-not $allowMoving) {
            throw ("'$ref' is not a 40-character commit. A branch or a tag moves and is refused. " +
                   "Pass -LauncherAllowMovingRef to accept one anyway.")
        }
        Write-Warn "'$ref' is not a commit. It can move, and what runs may change under you."
    }

    $expected = $Options.Sha256
    if (-not $expected) { $expected = Get-EnvOrDefault 'WSL_EPHEMERAL_SHA256' }
    $expected = $expected.ToLowerInvariant()

    $cacheDir = $Options.InstallDir
    if (-not $cacheDir) { $cacheDir = Get-EnvOrDefault 'WSL_EPHEMERAL_CACHE' }
    if (-not $cacheDir) {
        if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
            throw "LOCALAPPDATA is not set and no -LauncherInstallDir was given; nowhere to keep it."
        }
        $cacheDir = Join-Path $env:LOCALAPPDATA 'wsl-ephemeral\bin'
    }

    # KEYED BY REF. A cache keyed without the thing that varies hands back the
    # previous answer, which is the shape this repository's forbidden-patterns
    # table records under a fetched variant landing on a shared tag.
    $safeRef = ($ref -replace '[^A-Za-z0-9._-]', '-')
    $cached  = Join-Path $cacheDir ("wsl-ephemeral-$safeRef.ps1")
    $uri = "https://raw.githubusercontent.com/$UpstreamOwner/$UpstreamRepo/$ref/$UpstreamPath"

    $useCache = $false
    if ($expected -and (Test-Path -LiteralPath $cached -PathType Leaf)) {
        if ((Get-Sha256 -LiteralFile $cached) -eq $expected) { $useCache = $true }
        else { Write-Warn "the cached copy failed its digest check; fetching again." }
    }

    if (-not $useCache) {
        Write-Step "Fetching $UpstreamOwner/$UpstreamRepo@$($ref.Substring(0, [Math]::Min(12, $ref.Length)))"
        try { Save-Upstream -Uri $uri -Destination $cached }
        catch {
            if ($expected -and (Test-Path -LiteralPath $cached -PathType Leaf) -and
                (Get-Sha256 -LiteralFile $cached) -eq $expected) {
                Write-Warn "fetch failed ($($_.Exception.Message.Trim())); using the verified cached copy."
            }
            else {
                throw ("Could not fetch $uri and no verified cached copy exists.`n" +
                       "  Underlying error: $($_.Exception.Message.Trim())`n" +
                       "  Offline? Point -LauncherLocal at a copy on this machine.")
            }
        }
    }

    if ($expected) {
        $actual = Get-Sha256 -LiteralFile $cached
        if ($actual -ne $expected) {
            Remove-Item -LiteralPath $cached -Force -ErrorAction SilentlyContinue
            throw ("DIGEST MISMATCH for $uri`n" +
                   "  expected $expected`n" +
                   "  actual   $actual`n" +
                   "  Refusing to run it. The fetched copy has been deleted.")
        }
        Write-Ok "digest matches"
    }
    else {
        # SAID OUT LOUD. A commit cannot be pushed over, so a pinned ref with no
        # digest is far from nothing; it is still not the same as verified bytes,
        # and a caller who thinks it is will not notice a proxy.
        Write-Warn "no -LauncherSha256 given, so the bytes were not verified. Read one with:"
        Write-Warn ("  gh api repos/$UpstreamOwner/$UpstreamRepo/contents/$UpstreamPath" +
                    "?ref=REF --jq .content | base64 -d | sha256sum")
    }

    return $cached
}

try {
    $split = Split-LauncherArgument -Argument @($args)
    $opt = $split.Options

    if ($opt.Help) {
        Get-Help -Full $PSCommandPath
        exit 0
    }

    $dotSourced = ($MyInvocation.InvocationName -eq '.')

    $resolved = Resolve-Upstream -Options $opt
    Assert-PowerShellSyntax -LiteralFile $resolved
    if (Clear-DownloadMark -LiteralFile $resolved) {
        Write-Ok "cleared the download mark Windows put on it"
    }

    if ($opt.AddToPath) {
        if ($split.Forward.Count -gt 0) {
            throw ("-LauncherAddToPath does not run anything, so it takes no other arguments. " +
                   "Got: $($split.Forward -join ' ')")
        }
        $dir = Split-Path -Parent $resolved
        Write-Ok "wsl-ephemeral.ps1 is at $resolved"
        if ($dotSourced) {
            if (($env:PATH -split ';') -notcontains $dir) { $env:PATH = "$dir;$env:PATH" }
            Write-Ok "added '$dir' to PATH for THIS session only"
            Write-Note "  Nothing outside this session changed, and nothing was written to the"
            Write-Note "  machine's environment."
        }
        else {
            # A child process cannot change its parent's environment, so this
            # prints the line rather than claiming to have done it.
            Write-Warn "PATH was NOT changed: a child process cannot change the session that ran it."
            Write-Note "  Dot-source this launcher to have it done in your session:"
            Write-Note "    . $PSCommandPath -LauncherAddToPath"
            Write-Note "  Or set it yourself:"
            Write-Note "    `$env:PATH = '$dir;' + `$env:PATH"
        }
        return
    }

    if ($dotSourced) {
        throw ("Refusing to run wsl-ephemeral.ps1 from a dot-source. It calls exit, which " +
               "would end this session rather than the script. Invoke this launcher instead, " +
               "and dot-source it only for -LauncherAddToPath.")
    }

    # SPLATTED FROM A VARIABLE. `& $x @($a)` passes the ARRAY as one argument;
    # only `@name` splats.
    $forward = $split.Forward
    & $resolved @forward
    $innerCode = if (Test-Path variable:LASTEXITCODE) { $LASTEXITCODE } else { 0 }
    exit ([int]$innerCode)
}
catch {
    # stderr, for the same reason wsl-ephemeral.ps1 reports there: an error is
    # not a result, and -Action HostAddress puts a value on stdout.
    [Console]::Error.WriteLine("ERROR: $($_.Exception.Message)")
    exit 1
}
