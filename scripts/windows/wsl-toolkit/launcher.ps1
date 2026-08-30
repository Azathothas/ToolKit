#Requires -Version 5.1

<#
.SYNOPSIS
    Resolve wsl-toolkit.ps1, make it safe to run, and run it with the
    arguments given here.

.DESCRIPTION
    ONE FILE A CALLER FETCHES. Everything wsl-toolkit.ps1 needs before it can
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
      2. -LauncherRelease TAG|latest, or WSL_TOOLKIT_RELEASE. A published
         release, verified against the SHA256SUMS published in it.
      3. -LauncherRef, or WSL_EPHEMERAL_REF. A revision you named.
      4. wsl-toolkit.ps1 BESIDE THIS FILE. The clone case, and no network.

    A RELEASE IS THE ONE TO REACH FOR, and the difference from a commit is not
    convenience. A commit names a TREE, and the file at a path in it is whatever
    happened to be there. A release names an ARTEFACT that was built from its
    sources, tested, and published on purpose, and it carries a SHA256SUMS of
    its own. A tag does not move either, so the shape check that refuses a
    branch has nothing to complain about.

    TRAP: WHAT THE RELEASE DIGEST PROVES. The SHA256SUMS comes from the same release
    as the asset, so checking one against the other proves the bytes arrived
    intact. It does not prove who published them. -LauncherSha256 with a digest
    the CALLER holds is the check that proves that, and it applies on top.

    AN EXPLICIT REF NOW WINS OVER THE SIBLING, and it used to be the other way
    round. A caller passing a commit AND a digest could get the line "Using the
    copy beside this launcher", run a stale file, and verify nothing. A consumer
    hit that, worked around it by deleting the sibling before every call, and
    wrote the workaround into their own documentation. The sibling is what "you
    did not say" resolves to, not something that overrides what you did say.

    THERE IS NO DEFAULT REF, ON PURPOSE. With no sibling and no ref it refuses
    and prints what to pass. Falling back to a branch would be running code
    nobody reviewed, which is the rule this repository states first and
    everywhere. What it offers instead of a default is two ways of resolving one:

      -LauncherRef auto     resolve main to a commit ONCE, record the commit AND
                            its digest in a lock file, and use that record on
                            every later run. One trust decision, written down.
      -LauncherRef latest   resolve main on EVERY run, warning every time. A
                            standing trust decision, and it says so.
      -LauncherSha256 auto  read the digest from api.github.com for whatever ref
                            is in play, and verify the raw download against it.

    NEITHER KEYWORD EVER FETCHES A BRANCH. Both resolve one to a commit first,
    so the URL downloaded always names an immutable object. The pin rule is
    kept; what is removed is the caller having to paste two long strings.

    WHAT IT DOES THAT A curl AND A pwsh DO NOT

      - REFUSES A MOVING REF by shape. A 40-character commit, one of the two
        keywords above, or an explicit -LauncherAllowMovingRef.
      - VERIFIES A DIGEST when one is given, and refuses on a mismatch rather
        than warning.
      - REFUSES A DIGEST THAT IS NOT 64 HEX CHARACTERS by name, rather than
        letting a typo arrive later as a mismatch nobody can explain.
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
    wsl-toolkit.ps1 verbatim EXCEPT the -Launcher* switches listed below,
    which are removed first.

    Restating the inner script's parameter list here is how a wrapper drifts
    from the thing it wraps, so it is not restated. The Launcher prefix is what
    makes that safe: the inner script cannot grow a parameter that collides with
    one of these, whatever it adds.

      -LauncherLocal PATH        run this file. No network.
      -LauncherRelease TAG|latest
                                 fetch a published release's wsl-toolkit.ps1 and
                                 verify it against that release's SHA256SUMS.
                                 'latest' warns, every run, and names the tag it
                                 resolved so the run is reproducible from a log.
      -LauncherRef SHA|auto|latest
                                 fetch this revision from Azathothas/ToolKit,
                                 or resolve main once (auto) or every run
                                 (latest).
      -LauncherSha256 HEX|auto   expect this SHA-256 of the fetched bytes, or
                                 read it from the API for the resolved ref.
      -LauncherLock PATH         where -LauncherRef auto keeps what it resolved.
                                 Default: <install dir>\wsl-toolkit.lock.json.
      -LauncherAllowMovingRef    permit a branch or a tag. Prints a warning.
      -LauncherInstallDir DIR    where a fetched copy is kept.
      -LauncherAddToPath         put the directory on PATH and DO NOT RUN.
      -LauncherHelp              print this and stop.

    The environment equivalents, for a caller that would rather not touch the
    argument list at all:

      WSL_EPHEMERAL_LOCAL, WSL_TOOLKIT_RELEASE, WSL_EPHEMERAL_REF,
      WSL_EPHEMERAL_SHA256, WSL_EPHEMERAL_ALLOW_MOVING_REF=1,
      WSL_EPHEMERAL_CACHE, WSL_EPHEMERAL_LOCK

.EXAMPLE
    .\launcher.ps1 -Action List

.EXAMPLE
    .\launcher.ps1 -LauncherRelease wsl-toolkit-v1.0.0 -Action Doctor

.EXAMPLE
    .\launcher.ps1 -LauncherRef 7127ff7... -Action New -Image alpine:3.22 -Ephemeral -Force

.EXAMPLE
    .\launcher.ps1 -LauncherRef auto -LauncherLock .\toolkit.lock.json -Action List

.EXAMPLE
    . .\launcher.ps1 -LauncherAddToPath

.NOTES
    -LauncherAddToPath AND DOT-SOURCING, WHICH IS THE ONE THING TO READ HERE.

    A child process cannot change its parent's environment. Run this normally
    and it can install the script and PRINT the line that puts it on PATH; it
    cannot put it there. DOT-SOURCE it and the assignment happens in the calling
    session, which is what the caller actually wanted:

        . .\launcher.ps1 -LauncherAddToPath

    A launcher that claimed to have changed PATH from a child process would be
    reporting a result it never read, so it does not claim it.

    DOT-SOURCING IS REFUSED FOR EVERY OTHER USE, and that is not tidiness.
    wsl-toolkit.ps1 calls `exit`, which ends the HOST SESSION when it is
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
$UpstreamPath  = 'scripts/windows/wsl-toolkit/wsl-toolkit.ps1'
$ScriptLeaf    = 'wsl-toolkit.ps1'
# The branch -LauncherRef auto and -LauncherRef latest resolve. It is a constant
# rather than a parameter because it is a property of the repository this
# launcher is written against, not a choice a caller makes: a different branch
# is a different -LauncherRef, spelled as the commit it points at.
$UpstreamBranch = 'main'
# The API hosts, tried in order. The first is the source. The second is
# pkgforge-dev/reverse-proxies' read-only mirror of it, and it is here for the
# two cases that make the first unusable: the anonymous rate limit of 60
# requests an hour per address, and a network that cannot reach api.github.com
# at all. See Get-ApiUri for what was measured across the two.
$ApiHosts = @('api.github.com', 'api.gh.pkgforge.dev')

# EVERY LINE THIS FILE PRINTS GOES TO STDERR, and there is no Write-Host in it
# at all. A wrapper that writes to the wrapped program's stdout corrupts it:
# `wsl-toolkit.ps1 -Action HostAddress` puts one address there and nothing
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

    # HARD RULE: EVERY KEY IS DECLARED HERE EVEN WHEN IT IS EMPTY. Set-StrictMode Latest
    # throws on a property that was never set, so a key added to $takesValue and
    # not to this table turns the first read of it into "the property cannot be
    # found on this object" from a caller who passed nothing unusual at all.
    $opt = @{
        Local = ''; Release = ''; Ref = ''; Sha256 = ''; InstallDir = ''; Lock = ''
        AllowMovingRef = $false; AddToPath = $false; Help = $false
    }
    # An ordinary array with +=, NOT an ArrayList. See the note above: this is
    # the difference between an argument list and a positional mess.
    $rest = @()
    $takesValue = @{
        '-launcherlocal' = 'Local'; '-launcherref' = 'Ref'
        '-launchersha256' = 'Sha256'; '-launcherinstalldir' = 'InstallDir'
        '-launcherlock' = 'Lock'; '-launcherrelease' = 'Release'
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
            throw ("$a is not a launcher option. The set is -LauncherLocal, -LauncherRelease, " +
                   "-LauncherRef, -LauncherSha256, -LauncherAllowMovingRef, -LauncherInstallDir, " +
                   "-LauncherLock, -LauncherAddToPath and -LauncherHelp.")
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
    <#
      Download wsl-toolkit.ps1 at one commit, trying every source in order,
      to a temp file in the destination directory, then rename.

      THREE SOURCES, AND ALL THREE SERVE THE SAME BYTES. Measured on
      2026-08-30 at commit 8efe6e02: raw.githubusercontent.com, api.github.com
      and api.gh.pkgforge.dev each returned 96,170 bytes hashing to
      ab4f6bd6c040bb9d.... So a
      fallback is not a lesser copy; it is the same object over another route,
      and the digest check downstream holds it to that whichever one answered.

      WHY MORE THAN ONE. raw.githubusercontent.com is blocked on some corporate
      networks while api.github.com is not, and api.github.com's anonymous rate
      limit is 60 requests an hour per address while the proxy in front of it
      reports 5000. Each of those makes exactly one of the three unusable, and
      none of them makes the file unavailable.

      THE TEMP FILE IS IN THE DESTINATION DIRECTORY. A rename across volumes
      is a copy, and loses the property that a killed process leaves the old
      file intact rather than a truncated one.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Ref,
        [Parameter(Mandatory = $true)][string]$Destination
    )
    $dir = Split-Path -Parent $Destination
    if (-not (Test-Path -LiteralPath $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    $temp = Join-Path $dir ('.download.' + [Guid]::NewGuid().ToString('N') + '.tmp')

    $sources = @(
        @{ Name = 'raw.githubusercontent.com'
           Uri  = "https://raw.githubusercontent.com/$UpstreamOwner/$UpstreamRepo/$Ref/$UpstreamPath"
           Accept = '*/*' }
    )
    foreach ($apiHost in $ApiHosts) {
        $sources += @{ Name = $apiHost
                       Uri  = (Get-ApiUri -ApiHost $apiHost -Path ("repos/$UpstreamOwner/$UpstreamRepo/contents/$UpstreamPath" + "?ref=$Ref"))
                       Accept = 'application/vnd.github.raw' }
    }

    $problems = @()
    try {
        foreach ($src in $sources) {
            try {
                $null = Invoke-UpstreamRequest -Uri $src.Uri -Accept $src.Accept -OutFile $temp
                if (-not (Test-Path -LiteralPath $temp)) { throw 'the download produced no file' }
                $size = (Get-Item -LiteralPath $temp).Length
                # AN IMPLAUSIBLY SMALL FILE IS A FAILURE OF THIS SOURCE, not
                # of the fetch. A redirect page, a captive portal and an error
                # body all arrive with HTTP 200 and a few hundred bytes, and the
                # next source is exactly what should be tried.
                if ($size -lt 1KB) { throw "the downloaded file is implausibly small ($size bytes)" }
                if ($src.Name -ne $sources[0].Name) { Write-Warn "$($sources[0].Name) did not answer; this came from $($src.Name) instead." }
                Move-Item -LiteralPath $temp -Destination $Destination -Force
                return
            }
            catch {
                $problems += "$($src.Name) : $($_.Exception.Message.Trim())"
                if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Force -ErrorAction SilentlyContinue }
            }
        }
        throw ("no source served $ScriptLeaf at $Ref :`n  " + ($problems -join "`n  "))
    }
    finally {
        if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Force -ErrorAction SilentlyContinue }
    }
}

function Get-ApiUri {
    <#
      The URL for one API question against one host.

      TWO HOSTS ANSWER THE SAME QUESTIONS. api.github.com is first because it is
      the source; api.gh.pkgforge.dev is a reverse proxy in front of it, and it
      exists here for the two cases that make the first one unusable: the
      anonymous rate limit, which is 60 requests an hour per address, and a
      network where api.github.com is simply not reachable.
      https://github.com/pkgforge-dev/reverse-proxies is what it is.

      MEASURED ON 2026-08-30, and this is the claim the fallback rests on: the
      contents endpoint through the proxy returned 96,170 bytes hashing to
      ab4f6bd6c040bb9d..., which is
      byte-identical to what raw.githubusercontent.com and api.github.com serve
      for the same commit. Its own rate limit reported 5000 core requests
      remaining against the anonymous 60.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$ApiHost,
        [Parameter(Mandatory = $true)][string]$Path
    )
    return "https://$ApiHost/$Path"
}

function Get-UpstreamUserAgent {
    <#
      ONE USER AGENT, AND ITS SHAPE IS LOAD-BEARING.

      api.github.com answers 403 to a request carrying none, and the body says
      so in a sentence that never reaches the caller because the exception only
      names the status.

      THE PROXY IS STRICTER AND IT IS NOT DOCUMENTATION, IT IS ENFORCED.
      Measured on 2026-08-30 against api.gh.pkgforge.dev, same URL and same
      Accept header, varying only this:

        'wsl-toolkit-launcher/1'  -> HTTP 420, refused
        'Mozilla/5.0'             -> HTTP 420, refused
        'curl/8.21.0'             -> HTTP 200
        none at all               -> HTTP 200

      RE-MEASURED ON 2026-08-30 AFTER THE TOOL WAS RENAMED, because a rename is
      exactly the change that would have broken this without saying so: the
      agent below answered HTTP 200 and the same string without its
      compatibility token answered 420.

      Its allowlist is a substring match on curl, wget, pkgforge or soar. So the
      token is here, beside the tool's real name and its home, rather than in
      place of them: this is not a claim to be curl, and a reader of a log
      should be able to tell exactly what made the request.
    #>
    return 'wsl-toolkit-launcher/1 (curl-compatible; +https://github.com/Azathothas/ToolKit)'
}

function Invoke-UpstreamRequest {
    <#
      One GET, to a file or to a string, with the TLS floor and the progress bar
      dealt with once.

      Windows PowerShell 5.1 negotiates TLS 1.0 on some machines and GitHub
      refuses it, which surfaces as "The request was aborted: Could not create
      SSL/TLS secure channel" and reads like an outage. PowerShell 7 already
      defaults correctly and setting it is harmless there.

      The progress bar makes Invoke-WebRequest an order of magnitude slower on
      5.1 and writes nothing a caller needs.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$Accept,
        [string]$OutFile = ''
    )
    try {
        [Net.ServicePointManager]::SecurityProtocol =
            [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    }
    catch { $null = $_ }
    $headers = @{ 'Accept' = $Accept; 'User-Agent' = Get-UpstreamUserAgent }
    $prevProgress = $ProgressPreference
    $ProgressPreference = 'SilentlyContinue'
    try {
        if ($OutFile) {
            Invoke-WebRequest -Uri $Uri -OutFile $OutFile -Headers $headers -UseBasicParsing -TimeoutSec 30
            return ''
        }
        $resp = Invoke-WebRequest -Uri $Uri -Headers $headers -UseBasicParsing -TimeoutSec 30
        # .Content IS NOT ALWAYS A STRING, and casting it as though it were
        # produces something that looks like data. Invoke-WebRequest decides by
        # the response's content type, and 'application/vnd.github.sha' is not
        # one it treats as text, so Content arrives as a byte[]. `[string]` on a
        # byte array joins the DECIMAL BYTE VALUES with spaces: the commit
        # 8efe6e02 came back as "56 101 102 101 54 101 48 50 ...", forty numbers
        # that are the right answer in the wrong alphabet.
        if ($resp.Content -is [byte[]]) { return [Text.Encoding]::UTF8.GetString($resp.Content) }
        return [string]$resp.Content
    }
    finally { $ProgressPreference = $prevProgress }
}

function Invoke-ApiWithFallback {
    <#
      Ask one API question, trying each host in order and stopping at the first
      that answers.

      EVERY FAILURE IS TRIED PAST, not only a rate limit. A 403, a 5xx, a DNS
      failure and a timeout are four different reasons the first host is no use
      right now, and a fallback that only handles the one it was written for is
      a fallback that is absent on the day it is needed. What is NOT tried past
      is a bad answer: the caller checks the shape of what came back, so a proxy
      returning a login page fails the same way the source would.

      THE HOST THAT ANSWERED IS NAMED. A fallback nobody can see is a fallback
      nobody knows fired, and "it worked" through a proxy is a different fact
      from "it worked".
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Accept,
        [string]$OutFile = ''
    )
    $problems = @()
    foreach ($apiHost in $ApiHosts) {
        $uri = Get-ApiUri -ApiHost $apiHost -Path $Path
        try {
            $text = Invoke-UpstreamRequest -Uri $uri -Accept $Accept -OutFile $OutFile
            if ($apiHost -ne $ApiHosts[0]) { Write-Warn "$($ApiHosts[0]) did not answer; this came from $apiHost instead." }
            return $text
        }
        catch {
            $problems += "$apiHost : $($_.Exception.Message.Trim())"
        }
    }
    throw ("No API host answered for $Path :`n  " + ($problems -join "`n  "))
}

function Resolve-UpstreamRef {
    <#
      Turn the branch name into the commit it points at RIGHT NOW.

      THE POINT OF DOING THIS AT ALL. The rule everywhere in this repository is
      to pin a commit and never a branch, and the rule is right: a moved
      reference runs code nobody reviewed. But making every caller run a gh
      command and paste a 40-character string is what the rule cost them, and a
      rule that expensive gets worked around rather than followed.

      NO gh, AND NO EXTERNAL TOOL AT ALL. Invoke-WebRequest ships with every
      PowerShell this script supports. A launcher whose convenience path needed
      the GitHub CLI installed would have replaced one setup step with another.

      SO THE BRANCH IS NEVER FETCHED. It is resolved to a commit here, and the
      URL that is actually downloaded always names that commit. What runs is
      always an immutable object; the only question 'auto' and 'latest' answer
      differently is HOW OFTEN the resolution happens.
    #>
    param([Parameter(Mandatory = $true)][string]$Branch)
    $path = "repos/$UpstreamOwner/$UpstreamRepo/commits/$Branch"
    $text = ''
    try { $text = (Invoke-ApiWithFallback -Path $path -Accept 'application/vnd.github.sha').Trim() }
    catch {
        throw ("Could not resolve $UpstreamOwner/$UpstreamRepo@$Branch to a commit.`n" +
               "  $($_.Exception.Message.Trim())`n" +
               "  Offline? Pass a 40-character -LauncherRef, or point -LauncherLocal at a copy.")
    }
    if ($text -notmatch '^[0-9a-f]{40}$') {
        throw ("The API answered something that is not a commit for $path. First 120 characters: " +
               $text.Substring(0, [Math]::Min(120, $text.Length)))
    }
    return $text
}

function Get-UpstreamDigest {
    <#
      The SHA-256 of the file at one commit, read from the API rather than from
      the raw endpoint the download comes from.

      WHAT THIS IS AND IS NOT, because the difference matters and is easy to
      overstate. It is a TRANSPORT check: these bytes come from an API host and
      the download comes from raw.githubusercontent.com, so a truncated
      transfer, a captive portal or a proxy that rewrote one of the two is
      caught. It is NOT a provenance check. A digest a person obtained out of
      band and reviewed is a different and stronger thing, and passing one as
      -LauncherSha256 remains the strongest option here.

      IT DOWNLOADS TO A FILE AND HASHES THE FILE. The obvious version reads the
      response as a STRING and hashes its UTF-8 bytes, and that is wrong in a
      way that would arrive as a digest mismatch nobody could explain:
      wsl-toolkit.ps1 begins with a UTF-8 byte order mark, and a response
      decoded to text and re-encoded does not reliably carry one back. Bytes to
      disk, then Get-FileHash, has no encoding step in it at all.
    #>
    param([Parameter(Mandatory = $true)][string]$Ref, [Parameter(Mandatory = $true)][string]$WorkDir)
    if (-not (Test-Path -LiteralPath $WorkDir)) { New-Item -ItemType Directory -Path $WorkDir -Force | Out-Null }
    $temp = Join-Path $WorkDir ('.digest.' + [Guid]::NewGuid().ToString('N') + '.tmp')
    try {
        try {
            $null = Invoke-ApiWithFallback -Path ("repos/$UpstreamOwner/$UpstreamRepo/contents/$UpstreamPath" + "?ref=$Ref") `
                -Accept 'application/vnd.github.raw' -OutFile $temp
        }
        catch { throw "Could not read the digest for $UpstreamPath at $Ref : $($_.Exception.Message.Trim())" }
        if (-not (Test-Path -LiteralPath $temp)) { throw "the digest request produced no file for $UpstreamPath at $Ref" }
        if ((Get-Item -LiteralPath $temp).Length -lt 1KB) {
            throw ("the digest request returned $((Get-Item -LiteralPath $temp).Length) bytes, which is too " +
                   "small to be $ScriptLeaf. A file over a megabyte comes back empty from that endpoint, and so " +
                   "does an error page.")
        }
        return (Get-Sha256 -LiteralFile $temp)
    }
    finally {
        if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Force -ErrorAction SilentlyContinue }
    }
}
function Get-LockPath {
    param([Parameter(Mandatory = $true)]$Options, [Parameter(Mandatory = $true)][string]$CacheDir)
    $p = $Options.Lock
    if (-not $p) { $p = Get-EnvOrDefault 'WSL_EPHEMERAL_LOCK' }
    if ($p) { return $p }
    # NOT THE WORKING DIRECTORY. A launcher that writes a file into whatever
    # directory it was run from has written into somebody's repository without
    # being asked. The cache directory is this tool's own, and a caller who
    # wants the lock committed beside their project names it with -LauncherLock.
    return (Join-Path $CacheDir 'wsl-toolkit.lock.json')
}

function Read-LauncherLock {
    <#
      The recorded resolution, or nothing.

      IT VALIDATES WHAT IT IS FOR. A lock naming another repository or another
      path is not this launcher's, and using its commit would fetch a file whose
      digest could never match. Refusing by name is a one-line message; the
      alternative is a digest mismatch that reads like an attack.
    #>
    param([Parameter(Mandatory = $true)][string]$Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $null }
    try { $lock = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json }
    catch { Write-Warn "the lock at $Path is not JSON, so it is being resolved again: $($_.Exception.Message.Trim())"; return $null }
    foreach ($field in @('schema', 'repository', 'path', 'ref', 'sha256')) {
        if (-not ($lock.PSObject.Properties.Name -contains $field)) {
            Write-Warn "the lock at $Path carries no '$field', so it is being resolved again."
            return $null
        }
    }
    if ($lock.schema -ne 'wsl-toolkit-lock/1') {
        throw "The lock at $Path says schema '$($lock.schema)' and this launcher writes 'wsl-toolkit-lock/1'. Delete it, or point -LauncherLock somewhere else."
    }
    if ($lock.repository -ne "$UpstreamOwner/$UpstreamRepo" -or $lock.path -ne $UpstreamPath) {
        throw ("The lock at $Path is for $($lock.repository) at $($lock.path), and this launcher " +
               "fetches $UpstreamOwner/$UpstreamRepo at $UpstreamPath. It is somebody else's lock.")
    }
    if ($lock.ref -notmatch '^[0-9a-f]{40}$' -or $lock.sha256 -notmatch '^[0-9a-f]{64}$') {
        throw "The lock at $Path holds a ref or a digest that is not the right shape. Delete it to resolve again."
    }
    return $lock
}

function Write-LauncherLock {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Ref,
        [Parameter(Mandatory = $true)][string]$Sha256,
        [Parameter(Mandatory = $true)][string]$Branch
    )
    $dir = Split-Path -Parent $Path
    if ($dir -and -not (Test-Path -LiteralPath $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    $lock = [ordered]@{
        schema     = 'wsl-toolkit-lock/1'
        repository = "$UpstreamOwner/$UpstreamRepo"
        path       = $UpstreamPath
        branch     = $Branch
        ref        = $Ref
        sha256     = $Sha256
        resolved   = [DateTimeOffset]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ', [Globalization.CultureInfo]::InvariantCulture)
    }
    # Written to a temp file in the SAME directory and renamed, so a killed run
    # leaves the old lock rather than a truncated one.
    $temp = "$Path.$([Guid]::NewGuid().ToString('N')).tmp"
    Set-Content -LiteralPath $temp -Value ($lock | ConvertTo-Json -Depth 5) -Encoding UTF8
    Move-Item -LiteralPath $temp -Destination $Path -Force
}

function Get-ReleaseJson {
    <#
      One release, by tag or 'latest', from the first API host that answers.

      NOTE: THE API RATHER THAN THE browser_download_url. An asset's browser URL is
      on github.com, which neither of the fallback hosts proxies, so a network
      that can only reach the proxy could resolve a release and then fail to
      download it. The assets endpoint answers on every host in $ApiHosts.
    #>
    param([Parameter(Mandatory = $true)][string]$Tag)
    $path = if ($Tag -ieq 'latest') {
        "repos/$UpstreamOwner/$UpstreamRepo/releases/latest"
    }
    else {
        "repos/$UpstreamOwner/$UpstreamRepo/releases/tags/$Tag"
    }
    $problems = @()
    foreach ($apiHost in $ApiHosts) {
        try {
            $body = Invoke-UpstreamRequest -Uri (Get-ApiUri -ApiHost $apiHost -Path $path) -Accept 'application/vnd.github+json'
            $json = $body | ConvertFrom-Json
            # HARD RULE: THE SHAPE IS CHECKED BEFORE IT IS TRUSTED. A proxy returning a
            # login page, an error document or an empty object all arrive with
            # HTTP 200, and reading .assets off one of those throws a message
            # about a property rather than about the fetch.
            if (-not $json.tag_name) { throw 'the response carries no tag_name, so it is not a release' }
            if (-not $json.assets)   { throw "release $($json.tag_name) has no assets" }
            if ($apiHost -ne $ApiHosts[0]) { Write-Warn "$($ApiHosts[0]) did not answer; this release came from $apiHost instead." }
            return $json
        }
        catch { $problems += "$apiHost : $($_.Exception.Message.Trim())" }
    }
    throw ("no host served release '$Tag':`n  " + ($problems -join "`n  "))
}

function Save-ReleaseAsset {
    <#
      One asset of one release, to a file, from the first host that answers.
    #>
    param(
        [Parameter(Mandatory = $true)]$Release,
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string]$Destination
    )
    $asset = @($Release.assets | Where-Object { $_.name -eq $Name })
    if ($asset.Count -eq 0) {
        $have = (@($Release.assets | ForEach-Object { $_.name }) -join ', ')
        throw "release $($Release.tag_name) has no asset named '$Name'. It has: $have"
    }
    $id = $asset[0].id
    $problems = @()
    foreach ($apiHost in $ApiHosts) {
        try {
            $null = Invoke-UpstreamRequest -Uri (Get-ApiUri -ApiHost $apiHost -Path "repos/$UpstreamOwner/$UpstreamRepo/releases/assets/$id") `
                -Accept 'application/octet-stream' -OutFile $Destination
            if (-not (Test-Path -LiteralPath $Destination)) { throw 'the download produced no file' }
            return
        }
        catch { $problems += "$apiHost : $($_.Exception.Message.Trim())" }
    }
    throw ("no host served asset '$Name' of release $($Release.tag_name):`n  " + ($problems -join "`n  "))
}

function Get-Sha256SumsEntry {
    <#
      The digest one SHA256SUMS file records for one name.

      TRAP: WHAT THIS PROVES AND WHAT IT DOES NOT, said here rather than left to be
      assumed. The sums file comes from the SAME release as the asset, so
      checking one against the other proves the bytes arrived intact. It does
      NOT prove who published them: anyone who could replace the asset could
      replace the sums beside it. NOTE: -LauncherSha256 with a digest the CALLER
      holds is the check that proves that, and it still applies on top of this.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Text,
        [Parameter(Mandatory = $true)][string]$Name
    )
    foreach ($line in ($Text -split "`r?`n")) {
        $t = "$line".Trim()
        if (-not $t) { continue }
        # 'HEX  name' or 'HEX *name', which is what sha256sum writes in text and
        # in binary mode. Both are accepted; neither is guessed at.
        if ($t -match '^([0-9a-fA-F]{64})\s+\*?(.+)$') {
            if ($Matches[2].Trim() -eq $Name) { return $Matches[1].ToLowerInvariant() }
        }
    }
    throw "SHA256SUMS carries no line for '$Name'."
}

function Resolve-FromRelease {
    <#
      Download one release's wsl-toolkit.ps1, verify it against that release's
      own SHA256SUMS, and against -LauncherSha256 when the caller holds one.

      HARD RULE: THE CACHE IS KEYED BY TAG. Serving a previously downloaded copy for a
      different tag is the "fetching a variant into a cache keyed without the
      variant" row in docs/conventions/forbidden-patterns.md, and it would mean
      changing the tag ran the old file.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Tag,
        [Parameter(Mandatory = $true)]$Options
    )
    $cacheDir = $Options.InstallDir
    if (-not $cacheDir) { $cacheDir = Get-EnvOrDefault 'WSL_EPHEMERAL_CACHE' }
    if (-not $cacheDir) {
        if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
            throw 'LOCALAPPDATA is not set and no -LauncherInstallDir was given; nowhere to keep it.'
        }
        $cacheDir = Join-Path $env:LOCALAPPDATA 'wsl-ephemeral\bin'
    }
    if (-not (Test-Path -LiteralPath $cacheDir)) { $null = New-Item -ItemType Directory -Path $cacheDir -Force }

    if ($Tag -ieq 'latest') {
        Write-Warn "-LauncherRelease latest asks GitHub for the newest release on EVERY run."
        Write-Warn "  What executes can change between one call and the next. The tag it resolved is named below; pass that tag to pin it."
    }
    $rel = Get-ReleaseJson -Tag $Tag
    $realTag = [string]$rel.tag_name
    Write-Ok "release $realTag"

    $safeTag = ($realTag -replace '[^0-9A-Za-z._-]', '_')
    $cached  = Join-Path $cacheDir ("wsl-toolkit-$safeTag.ps1")

    $expected = $Options.Sha256
    if (-not $expected) { $expected = Get-EnvOrDefault 'WSL_EPHEMERAL_SHA256' }
    $expected = $expected.ToLowerInvariant()
    if ($expected -eq 'auto') {
        throw ('-LauncherSha256 auto reads a digest from the contents API for a REF. A release ' +
               'already publishes SHA256SUMS and this verifies against it, so drop the switch, ' +
               'or pass a digest you hold yourself.')
    }

    if (Test-Path -LiteralPath $cached) {
        Write-Ok "already downloaded: $cached"
    }
    else {
        $sums = Join-Path $cacheDir (".sums." + [Guid]::NewGuid().ToString('N') + '.tmp')
        $temp = Join-Path $cacheDir (".download." + [Guid]::NewGuid().ToString('N') + '.tmp')
        try {
            Save-ReleaseAsset -Release $rel -Name 'SHA256SUMS' -Destination $sums
            Save-ReleaseAsset -Release $rel -Name $ScriptLeaf -Destination $temp
            $want = Get-Sha256SumsEntry -Text ([IO.File]::ReadAllText($sums)) -Name $ScriptLeaf
            $got  = Get-Sha256 -LiteralFile $temp
            if ($got -ne $want) {
                throw ("the release asset does not match the SHA256SUMS published beside it.`n" +
                       "  expected $want`n  got      $got`n" +
                       '  Nothing was installed. This is a transport failure or a tampered asset; either way it is not runnable.')
            }
            Write-Ok "digest matches the SHA256SUMS in release $realTag"
            Write-Warn "that proves the bytes arrived intact, not who published them. -LauncherSha256 with a digest you hold yourself is the check that proves that."
            Move-Item -LiteralPath $temp -Destination $cached -Force
        }
        finally {
            foreach ($f in @($sums, $temp)) {
                if (Test-Path -LiteralPath $f) { Remove-Item -LiteralPath $f -Force -ErrorAction SilentlyContinue }
            }
        }
    }

    if ($expected) {
        $got = Get-Sha256 -LiteralFile $cached
        if ($got -ne $expected) {
            throw ("-LauncherSha256 does not match what release $realTag serves.`n" +
                   "  expected $expected`n  got      $got")
        }
        Write-Ok '-LauncherSha256 matches too'
    }

    $null = Clear-DownloadMark -LiteralFile $cached
    Assert-PowerShellSyntax -LiteralFile $cached
    return $cached
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

    $ref = $Options.Ref
    if (-not $ref) { $ref = Get-EnvOrDefault 'WSL_EPHEMERAL_REF' }

    # 2. a published release, which is the shape a consumer should reach for.
    #
    # NOTE: WHY A RELEASE IS BETTER THAN A COMMIT FOR A CONSUMER. A commit names a
    # tree, and the file at that path in that tree is whatever was there; a
    # release names an ARTEFACT that was built, tested and published on purpose,
    # and it carries its own SHA256SUMS. A tag also does not move, so the shape
    # check that refuses a branch has nothing to complain about.
    #
    # HARD RULE: 'latest' HERE IS NOT 'latest' ON -LauncherRef. This one resolves to a
    # specific published tag and then names it in the lock and in the output, so
    # what ran is recoverable from the log. It is still a standing trust
    # decision, and it says so once.
    $release = $Options.Release
    if (-not $release) { $release = Get-EnvOrDefault 'WSL_TOOLKIT_RELEASE' }
    if ($release) {
        if ($ref) {
            throw ('-LauncherRelease and -LauncherRef are two answers to the same question. ' +
                   'Pass one: a release tag names a published artefact, a commit names a tree.')
        }
        return (Resolve-FromRelease -Tag $release -Options $Options)
    }

    # 2. the sibling, which is the clone case and needs no network at all.
    #
    # AN EXPLICIT REF NOW WINS OVER IT, and that is a change. The sibling used
    # to win over everything short of -LauncherLocal, so a run passing both a
    # commit and a digest could print "Using the copy beside this launcher", run
    # a stale file and verify nothing. A consumer hit exactly that, worked
    # around it by deleting the sibling before every call, and wrote the
    # workaround into their own documentation. A caller who names a revision
    # means that revision; the sibling is what "you did not say" resolves to.
    if (-not $ref) {
        $here = Split-Path -Parent $PSCommandPath
        if ($here) {
            $sibling = Join-Path $here $ScriptLeaf
            if (Test-Path -LiteralPath $sibling -PathType Leaf) {
                Write-Step "Using the copy beside this launcher"
                return (Resolve-Path -LiteralPath $sibling).Path
            }
        }
        throw ("No copy of $ScriptLeaf beside this launcher, and no revision to fetch. " +
               "There is no default: a branch moves, and a moved reference runs code nobody " +
               "reviewed. Name one of these as -LauncherRef:`n" +
               "    auto     resolve $UpstreamBranch to a commit ONCE, record it, and use the record after that`n" +
               "    latest   resolve $UpstreamBranch on every run, which is a standing trust decision`n" +
               "    a 40-character commit, which 'auto' would have read for you")
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

    # -- auto and latest, which is where the pin rule meets the caller -------
    #
    # THE COMPLAINT THIS ANSWERS: a consumer had to run a gh command, paste a
    # 40-character commit, run a second command, paste a 64-character digest,
    # and do it again every time either moved. Every one of those steps is a
    # place to paste the wrong string, and a wrong digest fails closed in a way
    # that takes an hour to work out.
    #
    # WHAT IS NOT GIVEN UP. Neither keyword ever fetches a branch. Both resolve
    # one to a commit FIRST, so the URL downloaded below always names an
    # immutable object and what runs is always a reviewable revision. The
    # difference between them is how often that resolution happens, which is the
    # difference between a one-time trust decision and a standing one.
    $lockPath = Get-LockPath -Options $Options -CacheDir $cacheDir
    $keyword  = $ref.ToLowerInvariant()
    if ($keyword -eq 'auto' -or $keyword -eq 'latest') {
        if ($keyword -eq 'auto' -and $expected -and $expected -ne 'auto') {
            throw ("-LauncherRef auto records the digest it resolved, in $lockPath, so a " +
                   "-LauncherSha256 given here can only ever agree with it or contradict it. " +
                   "Pass a commit as -LauncherRef if you want to pin the digest yourself.")
        }
        $lock = $null
        if ($keyword -eq 'auto') { $lock = Read-LauncherLock -Path $lockPath }
        if ($lock) {
            $ref = $lock.ref
            $expected = $lock.sha256
            Write-Ok "lock: $lockPath resolved to $($ref.Substring(0, 12)) with a recorded digest"
        }
        else {
            if ($keyword -eq 'latest') {
                Write-Warn "-LauncherRef latest re-resolves '$UpstreamBranch' on EVERY run."
                Write-Warn "  What executes can change between one call and the next, and nobody reviews it on your behalf."
                Write-Warn "  Use -LauncherRef auto once and keep the lock it writes."
            }
            else {
                Write-Warn "-LauncherRef auto: no usable lock at $lockPath, so '$UpstreamBranch' is being resolved now."
            }
            $ref = Resolve-UpstreamRef -Branch $UpstreamBranch
            Write-Ok "$UpstreamOwner/$UpstreamRepo@$UpstreamBranch is $ref"
            if (-not $expected -or $expected -eq 'auto') { $expected = Get-UpstreamDigest -Ref $ref -WorkDir $cacheDir }
            if ($keyword -eq 'auto') {
                Write-LauncherLock -Path $lockPath -Ref $ref -Sha256 $expected -Branch $UpstreamBranch
                Write-Ok "recorded in $lockPath. Later runs read it and ask GitHub nothing."
                Write-Warn "you have just trusted whatever '$UpstreamBranch' pointed at. Nobody reviewed it on your behalf."
                Write-Warn "  Keep that file, and put it under review with the rest of your project."
            }
        }
    }
    elseif ($expected -eq 'auto') {
        $expected = Get-UpstreamDigest -Ref $ref -WorkDir $cacheDir
        Write-Ok "-LauncherSha256 auto: the API answers $expected for this ref"
    }

    # THE SHAPE CHECK RUNS AFTER THE KEYWORDS, not before. A branch or a tag
    # is still refused; 'auto' and 'latest' reach this line already resolved to
    # a commit, which is the property that lets them exist at all.
    $allowMoving = $Options.AllowMovingRef -or ((Get-EnvOrDefault 'WSL_EPHEMERAL_ALLOW_MOVING_REF') -eq '1')
    if ($ref -notmatch '^[0-9a-fA-F]{40}$') {
        if (-not $allowMoving) {
            throw ("'$ref' is not a 40-character commit. A branch or a tag moves and is refused. " +
                   "Pass -LauncherRef auto to resolve one ONCE and record it, -LauncherRef latest " +
                   "to resolve it on every run, or -LauncherAllowMovingRef to fetch this ref as it is.")
        }
        Write-Warn "'$ref' is not a commit. It can move, and what runs may change under you."
    }
    if ($expected -and $expected -notmatch '^[0-9a-f]{64}$') {
        throw ("-LauncherSha256 '$expected' is not 64 hexadecimal characters, so it can never match " +
               "anything. A digest that is merely wrong fails closed later with a mismatch nobody " +
               "can explain; this says which string is the problem.")
    }

    # KEYED BY REF. A cache keyed without the thing that varies hands back the
    # previous answer, which is the shape this repository's forbidden-patterns
    # table records under a fetched variant landing on a shared tag.
    $safeRef = ($ref -replace '[^A-Za-z0-9._-]', '-')
    $cached  = Join-Path $cacheDir ("wsl-toolkit-$safeRef.ps1")

    $useCache = $false
    if ($expected -and (Test-Path -LiteralPath $cached -PathType Leaf)) {
        if ((Get-Sha256 -LiteralFile $cached) -eq $expected) { $useCache = $true }
        else { Write-Warn "the cached copy failed its digest check; fetching again." }
    }

    if (-not $useCache) {
        Write-Step "Fetching $UpstreamOwner/$UpstreamRepo@$($ref.Substring(0, [Math]::Min(12, $ref.Length)))"
        try { Save-Upstream -Ref $ref -Destination $cached }
        catch {
            if ($expected -and (Test-Path -LiteralPath $cached -PathType Leaf) -and
                (Get-Sha256 -LiteralFile $cached) -eq $expected) {
                Write-Warn "fetch failed ($($_.Exception.Message.Trim())); using the verified cached copy."
            }
            else {
                throw ("Could not fetch $ScriptLeaf at $ref and no verified cached copy exists.`n" +
                       "  $($_.Exception.Message.Trim())`n" +
                       "  Offline? Point -LauncherLocal at a copy on this machine.")
            }
        }
    }

    if ($expected) {
        $actual = Get-Sha256 -LiteralFile $cached
        if ($actual -ne $expected) {
            Remove-Item -LiteralPath $cached -Force -ErrorAction SilentlyContinue
            throw ("DIGEST MISMATCH for $ScriptLeaf at $ref`n" +
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
        Write-Warn "no -LauncherSha256 given, so the bytes were not verified."
        Write-Warn "  -LauncherSha256 auto reads one from the API and checks against it, with no other tool installed."
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
        Write-Ok "wsl-toolkit.ps1 is at $resolved"
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
        throw ("Refusing to run wsl-toolkit.ps1 from a dot-source. It calls exit, which " +
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
    # stderr, for the same reason wsl-toolkit.ps1 reports there: an error is
    # not a result, and -Action HostAddress puts a value on stdout.
    [Console]::Error.WriteLine("ERROR: $($_.Exception.Message)")
    exit 1
}
