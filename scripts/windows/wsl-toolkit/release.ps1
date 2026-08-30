<#
.SYNOPSIS
    Verify wsl-toolkit.ps1 is releasable, then tag it so CI publishes it.

.DESCRIPTION
    ⭐ THIS SCRIPT DOES NOT PUBLISH. It verifies, and it pushes an annotated tag.
    `.github/workflows/release.yml` is what creates the release, from a clean
    checkout of that tag, after re-running the same verification. That split is
    deliberate: a release cut from this machine could be cut from a dirty tree,
    from an unpushed commit, or from a bundle that disagrees with its sources,
    and none of those is visible in the artefact afterwards.

    WHAT IT REFUSES, and each one is a release that would have been wrong:

      * a dirty working tree. The asset would not match any commit.
      * a bundle that disagrees with its sources. The parts are the source and
        the file is the product; publishing a stale product publishes something
        nobody wrote.
      * a failing -Test. The suite, the surface lock and the analyzer all run.
      * a tag that already exists, locally or on the remote. A moved tag runs
        code nobody reviewed, which is the rule the launcher is built around.
      * HEAD not pushed. CI cannot check out a commit the remote does not have.

    THE VERSION HAS ONE HOME and it is `$script:ToolkitVersion` in
    src/20-prelude.ps1. This reads it out of the BUILT BUNDLE rather than out of
    the source, so a tag can never name a version the published file does not
    carry.

.PARAMETER Publish
    Actually create and push the tag. Without it this verifies and prints what
    it would do, which is the default because a tag push is not undoable in any
    way a consumer would notice.

.PARAMETER Remote
    The git remote to push the tag to. Default 'origin'.

.EXAMPLE
    pwsh -NoProfile -File scripts/windows/wsl-toolkit/release.ps1

.EXAMPLE
    pwsh -NoProfile -File scripts/windows/wsl-toolkit/release.ps1 -Publish

.NOTES
    Exit codes: 0 ready or published, 1 refused, 2 could not run.
#>
[CmdletBinding(PositionalBinding = $false)]
param(
    [switch]$Publish,
    [string]$Remote = 'origin'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:ToolRoot = $PSScriptRoot
$script:Bundle   = Join-Path $script:ToolRoot 'wsl-toolkit.ps1'
$script:Problems = @()

function Add-Problem { param([string]$Text) $script:Problems += $Text }

function Invoke-Git {
    <#
      git, with its exit code read from the process that produced it.
      ⛔ Never through a pipe: a pipeline reports the LAST command's status, so
      a git call that failed reads as green.
    #>
    param([Parameter(Mandatory = $true)][string[]]$Arguments)
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $out = & git @Arguments 2>&1
        return [pscustomobject]@{ Code = $LASTEXITCODE; Text = (@($out) -join "`n").Trim() }
    }
    finally { $ErrorActionPreference = $prev }
}

function Get-BundleVersion {
    <#
      The version the PRODUCT carries, read from the built file.

      ⛔ FROM THE BUNDLE, NOT FROM THE SOURCE. A tag formed from the source could
      name a version the published asset does not contain, and the asset is the
      thing a consumer runs. The match count is asserted, so a second assignment
      appearing anywhere is a refusal rather than a coin toss about which one won.
    #>
    if (-not (Test-Path -LiteralPath $script:Bundle)) { throw "wsl-toolkit.ps1 is not built. Run build.ps1 first." }
    $text = [IO.File]::ReadAllText($script:Bundle)
    $m = [regex]::Matches($text, "(?m)^\s*\`$script:ToolkitVersion\s*=\s*'([0-9]+\.[0-9]+\.[0-9]+)'\s*$")
    if ($m.Count -ne 1) {
        throw ("the bundle carries $($m.Count) assignments to `$script:ToolkitVersion and exactly one is expected. " +
               'The version has one home, in src/20-prelude.ps1.')
    }
    return $m[0].Groups[1].Value
}

# --------------------------------------------------------------------------------------
# Main
# --------------------------------------------------------------------------------------
$exit = 0
try {
    $repoRoot = (Invoke-Git -Arguments @('rev-parse', '--show-toplevel'))
    if ($repoRoot.Code -ne 0) { throw 'not a git repository' }
    $root = $repoRoot.Text
    Set-Location -LiteralPath $root

    # -- the bundle agrees with its sources, and the suite passes -------------
    $ps = (Get-Process -Id $PID).Path
    & $ps -NoProfile -File (Join-Path $script:ToolRoot 'build.ps1') -Check | Out-Null
    if ($LASTEXITCODE -ne 0) { Add-Problem 'the tracked bundle disagrees with its parts; run build.ps1' }
    & $ps -NoProfile -File (Join-Path $script:ToolRoot 'build.ps1') -Test | Out-Null
    if ($LASTEXITCODE -ne 0) { Add-Problem 'build.ps1 -Test did not pass' }

    $version = Get-BundleVersion
    $tag = "wsl-toolkit-v$version"
    Write-Output "wsl-toolkit $version  ->  tag $tag"

    # -- the tree is clean ----------------------------------------------------
    $status = Invoke-Git -Arguments @('status', '--porcelain')
    if ($status.Code -ne 0) { Add-Problem 'git status failed' }
    elseif ($status.Text) {
        Add-Problem ("the working tree is not clean, so the asset would match no commit:`n    " +
                     (($status.Text -split "`n") -join "`n    "))
    }

    # -- HEAD is on the remote ------------------------------------------------
    $head = (Invoke-Git -Arguments @('rev-parse', 'HEAD')).Text
    $onRemote = Invoke-Git -Arguments @('branch', '-r', '--contains', $head)
    if ($onRemote.Code -ne 0 -or -not $onRemote.Text) {
        Add-Problem "HEAD ($($head.Substring(0, 12))) is not on any remote branch. CI cannot check out a commit the remote does not have."
    }

    # -- the tag is new, here and there ---------------------------------------
    $localTag = Invoke-Git -Arguments @('tag', '--list', $tag)
    if ($localTag.Text) { Add-Problem "tag $tag already exists locally. Bump `$script:ToolkitVersion in src/20-prelude.ps1 and rebuild." }
    $remoteTag = Invoke-Git -Arguments @('ls-remote', '--tags', $Remote, "refs/tags/$tag")
    if ($remoteTag.Code -eq 0 -and $remoteTag.Text) {
        Add-Problem "tag $tag already exists on '$Remote'. A moved tag runs code nobody reviewed; bump the version instead."
    }

    # -- what the release will carry, and its digests -------------------------
    $assets = @($script:Bundle, (Join-Path $script:ToolRoot 'launcher.ps1'))
    foreach ($a in $assets) {
        if (-not (Test-Path -LiteralPath $a)) { Add-Problem "asset missing: $a"; continue }
        $h = (Get-FileHash -LiteralPath $a -Algorithm SHA256).Hash.ToLowerInvariant()
        Write-Output ("  {0}  {1} ({2:N0} bytes)" -f $h, (Split-Path -Leaf $a), (Get-Item -LiteralPath $a).Length)
    }
    Write-Output '  ⚠ those are the WORKING TREE digests, which are CRLF here. CI publishes what it'
    Write-Output '    checks out and computes SHA256SUMS there, so the published digests are the ones'
    Write-Output '    a consumer can verify. Do not copy these into anything.'

    if ($script:Problems.Count -gt 0) {
        foreach ($p in $script:Problems) { [Console]::Error.WriteLine("  ! $p") }
        [Console]::Error.WriteLine('  ! nothing was tagged.')
        exit 1
    }

    if (-not $Publish) {
        Write-Output ''
        Write-Output 'ready. Nothing has been tagged or pushed. To do it:'
        Write-Output '  pwsh -NoProfile -File scripts/windows/wsl-toolkit/release.ps1 -Publish'
        exit 0
    }

    # ⛔ AN ANNOTATED TAG, and its message says what it is rather than repeating
    # the changelog. The release notes are generated by GitHub from the commits,
    # which is the one place that cannot go stale.
    $msg = "wsl-toolkit $version"
    $mk = Invoke-Git -Arguments @('tag', '-a', $tag, '-m', $msg)
    if ($mk.Code -ne 0) { throw "git tag failed: $($mk.Text)" }
    $push = Invoke-Git -Arguments @('push', $Remote, "refs/tags/$tag")
    if ($push.Code -ne 0) {
        # Leave no local tag behind that the remote does not have: the next run
        # would refuse on "already exists locally" for a release that never
        # happened.
        $null = Invoke-Git -Arguments @('tag', '-d', $tag)
        throw "pushing $tag to $Remote failed, and the local tag was removed again: $($push.Text)"
    }
    Write-Output "pushed $tag to $Remote."
    Write-Output 'release.yml takes it from here: it re-verifies on a clean checkout, writes SHA256SUMS and publishes.'
}
catch {
    [Console]::Error.WriteLine("ERROR: $($_.Exception.Message)")
    $exit = 2
}
exit $exit
