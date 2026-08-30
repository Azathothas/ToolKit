<#
.SYNOPSIS
    Build wsl-toolkit.ps1 from the parts named in bundle.manifest.

.DESCRIPTION
    The defect this exists to catch is a single file nobody can work in. The
    tool is 2,800 lines and growing, and a reader looking for the stream log
    reads past the safety model, the OCI config and the disk preflight to reach
    it. Splitting it is only possible because this builder puts it back
    together: the consumers of this tool fetch ONE URL and verify ONE digest,
    and that contract is what a directory of modules would break.

    So the parts under src/, core/ and libs/ are the source, and
    wsl-toolkit.ps1 is the product. THE PRODUCT IS TRACKED, because a consumer
    fetching a raw URL cannot run a build step.

    WHAT IT REFUSES, and each refusal is a defect it has to be able to catch:

      * a part listed in the manifest that does not exist on disk;
      * a part on disk under src/, core/ or libs/ that the manifest does not
        list. A builder that silently skipped a file would ship a script
        missing a function, and the caller who reached that function is what
        would report it;
      * a build whose result does not parse;
      * a build whose result has no param() block, or has one that is not the
        first statement. PowerShell requires it there, and a part reordered
        above it turns every parameter into a parse error at the caller;
      * -Check, when the tracked bundle disagrees with what the parts build.

    LINE ENDINGS AND THE BYTE ORDER MARK are decided here rather than inherited.
    Each part is read as bytes, a UTF-8 byte order mark is stripped if it has
    one, and CRLF and lone CR are normalised to LF. The result is written with
    CRLF and exactly one byte order mark at the front, which is what
    .gitattributes resolves for a .ps1 and what Windows PowerShell 5.1 needs to
    decode a file with any non-ASCII byte in it. Normalising on the way IN is
    what makes -Check answer the same on a Windows checkout and a Linux one.

.PARAMETER Check
    Build in memory and compare with the tracked bundle. Writes nothing. Exit 1
    when they differ, naming the first line that does.

.PARAMETER Test
    Build, then prove the result: every part parses, the bundle parses, the
    parameter surface still matches surface.lock, PSScriptAnalyzer is clean
    over the bundle when it is installed, and the selftest passes against it.

.PARAMETER Json
    One object on stdout, for a gate runner.

.EXAMPLE
    pwsh -NoProfile -File scripts/windows/wsl-toolkit/build.ps1

.EXAMPLE
    pwsh -NoProfile -File scripts/windows/wsl-toolkit/build.ps1 -Check

.NOTES
    Exit codes: 0 pass, 1 fail, 2 could not run.
    Read the exit code from this process, unpiped.
#>
# -- PSScriptAnalyzer, suppressed per rule with the reason --------------------
# Each is scoped to ONE rule and carries its justification. ⛔ Not a settings
# file switching the rule off everywhere, which weakens the gate for every
# future script to spare this one.
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseSingularNouns', '',
    Justification = 'Get-ManifestParts returns the whole ordered list, Build-BundleBytes returns the whole byte sequence, Compare-Bytes compares two of them, Invoke-BuildTests runs several, and Test-BundleParses ends in a verb the rule reads as a plural. Renaming any of them to satisfy the rule would make the name describe the thing less accurately.')]
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSReviewUnusedParameter', '',
    Justification = 'UpdateSurface and Json are read by Invoke-BuildTests and Write-Line through script scope rather than as arguments, which the analyzer does not follow.')]
# ⛔ PositionalBinding OFF. A .ps1 called through -File with positional binding
# left on lets an argument list overflow into whatever parameter is next in
# declaration order, silently. That is the TOOL-03 defect, and the check for it
# is this attribute rather than a rule anybody has to remember.
[CmdletBinding(PositionalBinding = $false)]
param(
    [switch]$Check,
    [switch]$Test,
    # ⛔ DELIBERATE, NEVER AUTOMATIC. surface.lock is the record of what the CLI
    # promised last time, and a build that refreshed it on its own would turn
    # every rename into a silent change to the thing consumers bind to. Writing
    # it is a separate act, and docs/consumers.md gets the row.
    [switch]$UpdateSurface,
    [switch]$Json
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$script:ToolRoot   = $PSScriptRoot
$script:Manifest   = Join-Path $script:ToolRoot 'bundle.manifest'
$script:BundlePath = Join-Path $script:ToolRoot 'wsl-toolkit.ps1'
$script:PartDirs   = @('src', 'core', 'libs')

# ⛔ Deterministic. No date, no machine name, no version read from anywhere that
# moves. -Check compares bytes, so anything here that changed between two runs
# would make the gate red on a tree nobody had touched.
$script:Banner = @(
    '# ⛔ GENERATED FILE. DO NOT EDIT THIS FILE.',
    '#',
    '# It is built by joining the parts named in bundle.manifest, in that order.',
    '# Edit the PART, not this file: an edit here is lost the next time anything',
    '# runs the build, and the gate refuses a bundle that disagrees with its',
    '# sources, so the edit is lost loudly rather than quietly.',
    '#',
    '#   pwsh -NoProfile -File scripts/windows/wsl-toolkit/build.ps1',
    '#   pwsh -NoProfile -File scripts/windows/wsl-toolkit/build.ps1 -Check',
    '#',
    '# ⭐ The single file is the PRODUCT and the parts are the SOURCE. It is',
    '# tracked, and released, because a consumer fetching one raw URL cannot run',
    '# a build step: one URL, one digest, one thing to verify.',
    ''
)

$script:Failures = @()
function Add-Failure { param([string]$Text) $script:Failures += $Text }

function Write-Line {
    param([string]$Text)
    if (-not $Json) { Write-Output $Text }
}

# --------------------------------------------------------------------------------------
# Reading the manifest and the parts
# --------------------------------------------------------------------------------------
function Get-ManifestParts {
    <#
      The manifest's order, with comments and blank lines dropped. Returns the
      relative paths as written, so an error message names what a reader can
      find in the file.
    #>
    if (-not (Test-Path -LiteralPath $script:Manifest)) {
        throw "bundle.manifest not found at $script:Manifest"
    }
    $lines = [IO.File]::ReadAllLines($script:Manifest)
    $out = @()
    foreach ($raw in $lines) {
        $line = $raw.Trim()
        if ($line.Length -eq 0) { continue }
        if ($line.StartsWith('#')) { continue }
        $out += $line
    }
    return , $out
}

function Get-PartsOnDisk {
    <#
      Every .ps1 under src/, core/ and libs/, as forward-slashed relative paths
      so they compare against the manifest's spelling on either platform.
    #>
    $found = @()
    foreach ($d in $script:PartDirs) {
        $dir = Join-Path $script:ToolRoot $d
        if (-not (Test-Path -LiteralPath $dir)) { continue }
        foreach ($f in (Get-ChildItem -LiteralPath $dir -Filter '*.ps1' -File | Sort-Object Name)) {
            $found += ($d + '/' + $f.Name)
        }
    }
    return , $found
}

function Read-PartText {
    <#
      One part, as text with LF endings and no byte order mark.

      ⛔ Read as BYTES and decode explicitly. Get-Content -Raw applies the
      host's default encoding, which is not the same on Windows PowerShell 5.1
      as on PowerShell 7, so the same tree would build two different bundles and
      -Check would be red on one host and green on the other.
    #>
    param([Parameter(Mandatory = $true)][string]$Path)
    $bytes = [IO.File]::ReadAllBytes($Path)
    if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
        $bytes = $bytes[3..($bytes.Length - 1)]
    }
    $text = [Text.Encoding]::UTF8.GetString($bytes)
    return ($text -replace "`r`n", "`n") -replace "`r", "`n"
}

# --------------------------------------------------------------------------------------
# The build
# --------------------------------------------------------------------------------------
function Build-BundleBytes {
    <#
      The bundle, as the exact bytes that belong on disk: one byte order mark,
      CRLF endings, UTF-8.

      Returns $null and records a failure when the manifest and the tree
      disagree, rather than building something that is missing a part.
    #>
    $manifest = Get-ManifestParts
    $onDisk   = Get-PartsOnDisk

    $missing = @($manifest | Where-Object { $onDisk -notcontains $_ })
    $unlisted = @($onDisk | Where-Object { $manifest -notcontains $_ })

    foreach ($m in $missing)  { Add-Failure "bundle.manifest lists $m, which is not on disk" }
    foreach ($u in $unlisted) { Add-Failure "$u exists and bundle.manifest does not list it" }

    $dupes = @($manifest | Group-Object | Where-Object { $_.Count -gt 1 })
    foreach ($d in $dupes) { Add-Failure "bundle.manifest lists $($d.Name) $($d.Count) times" }

    if ($script:Failures.Count -gt 0) { return $null }
    if ($manifest.Count -eq 0) {
        Add-Failure 'bundle.manifest names no parts, so this build would write an empty file'
        return $null
    }

    $chunks = @()
    $first = $true
    foreach ($rel in $manifest) {
        $full = Join-Path $script:ToolRoot ($rel -replace '/', [IO.Path]::DirectorySeparatorChar)
        $text = Read-PartText -Path $full
        if ($first) {
            # ⭐ The banner goes ABOVE the comment-based help, which
            # about_Comment_Based_Help permits: script help may be preceded by
            # comments and blank lines and by nothing else. Putting it here is
            # what makes "do not edit" the first thing a reader of the raw URL
            # sees.
            $chunks += (($script:Banner -join "`n") + "`n")
            $first = $false
        }
        $chunks += $text
    }

    $joined = ($chunks -join '')
    $crlf = ($joined -replace "`n", "`r`n")

    $utf8 = [Text.Encoding]::UTF8.GetBytes($crlf)
    $bom  = [byte[]]@(0xEF, 0xBB, 0xBF)
    $out  = [byte[]]::new($bom.Length + $utf8.Length)
    [Array]::Copy($bom, 0, $out, 0, $bom.Length)
    [Array]::Copy($utf8, 0, $out, $bom.Length, $utf8.Length)
    return , $out
}

function Test-BundleParses {
    <#
      Does the built text parse, and is param() the first statement.

      ⛔ The second half is not decoration. PowerShell requires param() to be
      the first statement in a script, so a manifest that put a part above
      src/10-parameters.ps1 would produce a file whose every parameter is a
      parse error at the CALLER, not here.
    #>
    param([Parameter(Mandatory = $true)][string]$Text)
    $errs = $null
    $tokens = $null
    $ast = [System.Management.Automation.Language.Parser]::ParseInput($Text, [ref]$tokens, [ref]$errs)
    if ($errs -and $errs.Count -gt 0) {
        foreach ($e in ($errs | Select-Object -First 5)) {
            Add-Failure ("the built bundle does not parse, line {0}: {1}" -f $e.Extent.StartLineNumber, $e.Message)
        }
        return $false
    }
    if ($null -eq $ast.ParamBlock) {
        Add-Failure 'the built bundle has no param() block'
        return $false
    }
    return $true
}

function Compare-Bytes {
    param(
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][byte[]]$Left,
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][byte[]]$Right
    )
    if ($Left.Length -ne $Right.Length) { return $false }
    for ($i = 0; $i -lt $Left.Length; $i++) {
        if ($Left[$i] -ne $Right[$i]) { return $false }
    }
    return $true
}

function Get-FirstDifferingLine {
    <#
      Which line a reader should open. A byte offset is true and useless.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$Built,
        [Parameter(Mandatory = $true)][string]$OnDisk
    )
    $a = $Built  -split "`n"
    $b = $OnDisk -split "`n"
    $n = [Math]::Min($a.Count, $b.Count)
    for ($i = 0; $i -lt $n; $i++) {
        if ($a[$i] -ne $b[$i]) {
            return [pscustomobject]@{ Line = $i + 1; Built = $a[$i]; OnDisk = $b[$i] }
        }
    }
    return [pscustomobject]@{ Line = $n + 1; Built = "($($a.Count) lines)"; OnDisk = "($($b.Count) lines)" }
}

function Write-BundleAtomically {
    <#
      A temp file in the SAME directory, then a rename. A killed process leaves
      the old bundle intact rather than a truncated one, and same-directory
      matters: a rename across volumes is a copy and loses the guarantee.
    #>
    param([Parameter(Mandatory = $true)][byte[]]$Bytes)
    $tmp = $script:BundlePath + '.tmp'
    [IO.File]::WriteAllBytes($tmp, $Bytes)
    if (Test-Path -LiteralPath $script:BundlePath) { Remove-Item -LiteralPath $script:BundlePath -Force }
    Move-Item -LiteralPath $tmp -Destination $script:BundlePath -Force
}

# --------------------------------------------------------------------------------------
# -Test: the proof that the product still behaves
# --------------------------------------------------------------------------------------
function Invoke-BuildTests {
    param([Parameter(Mandatory = $true)][string]$BundleText)
    $results = @()

    # Every part parses on its own. A broken part names ITSELF here; in the
    # bundle it names a line number 2,000 lines from where the reader edits.
    $badParts = @()
    foreach ($rel in (Get-ManifestParts)) {
        $full = Join-Path $script:ToolRoot ($rel -replace '/', [IO.Path]::DirectorySeparatorChar)
        $e = $null; $t = $null
        $null = [System.Management.Automation.Language.Parser]::ParseFile($full, [ref]$t, [ref]$e)
        if ($e -and $e.Count -gt 0) { $badParts += ("{0} line {1}: {2}" -f $rel, $e[0].Extent.StartLineNumber, $e[0].Message) }
    }
    if ($badParts.Count -eq 0) { $results += 'parts parse: ok' }
    else { foreach ($b in $badParts) { Add-Failure "a part does not parse: $b" }; $results += 'parts parse: FAIL' }

    $shadow = @(Get-CaseShadowedParameter -Text $BundleText)
    if ($shadow.Count -eq 0) { $results += 'no case-shadowed parameter: ok' }
    else {
        foreach ($s in $shadow) { Add-Failure "case-shadowed parameter: $s" }
        $results += 'no case-shadowed parameter: FAIL'
    }

    # The parameter surface, against the lock. A renamed or removed parameter is
    # a break by docs/consumers.md's definition, and this is what makes it fail
    # a gate instead of failing a caller nobody can reach.
    $lock = Join-Path $script:ToolRoot 'surface.lock'
    $surface = Get-ParameterSurface -Text $BundleText
    if ($UpdateSurface) {
        $header = @(
            '# surface.lock - the CLI surface wsl-toolkit.ps1 promises, one line per parameter.',
            '#',
            '# docs/consumers.md calls a renamed parameter, a changed type and a changed exit',
            '# code the three things that break a caller who did nothing wrong. This file is',
            '# what turns the first two into a failed gate instead of a failed caller.',
            '#',
            '# It is derived from the built bundle AST, so a parameter cannot be added',
            '# without appearing here. Refresh it deliberately, and only when the change is',
            '# intended, with:',
            '#',
            '#   pwsh -NoProfile -File scripts/windows/wsl-toolkit/build.ps1 -Test -UpdateSurface',
            '#',
            '# A refresh in the same commit as a rename is the record that the rename was a',
            '# decision. A refresh on its own is the record that nobody looked.',
            ''
        )
        [IO.File]::WriteAllText($lock, (($header + $surface) -join "`n") + "`n", [Text.UTF8Encoding]::new($false))
        $results += "surface: written ($($surface.Count) entries)"
    }
    elseif (-not (Test-Path -LiteralPath $lock)) {
        Add-Failure 'surface.lock is missing; write it with: build.ps1 -Test -UpdateSurface'
        $results += 'surface: FAIL (no lock)'
    }
    else {
        $want = @([IO.File]::ReadAllLines($lock) | Where-Object { $_.Trim().Length -gt 0 -and -not $_.Trim().StartsWith('#') })
        $added   = @($surface | Where-Object { $want -notcontains $_ })
        $removed = @($want    | Where-Object { $surface -notcontains $_ })
        foreach ($a in $added)   { Add-Failure "the CLI surface GAINED a line the lock does not have: $a" }
        foreach ($r in $removed) { Add-Failure "the CLI surface LOST a line the lock has: $r" }
        if ($added.Count -eq 0 -and $removed.Count -eq 0) { $results += "surface: ok ($($surface.Count) entries)" }
        else { $results += 'surface: FAIL' }
    }

    # The selftest, against the bundle this build produced.
    $selftest = Join-Path $script:ToolRoot 'selftest.ps1'
    if (-not (Test-Path -LiteralPath $selftest)) {
        Add-Failure 'selftest.ps1 is missing, so the build proved nothing about behaviour'
        $results += 'selftest: FAIL (missing)'
    }
    else {
        $ps = (Get-Process -Id $PID).Path
        & $ps -NoProfile -File $selftest | Out-Null
        if ($LASTEXITCODE -ne 0) {
            Add-Failure "selftest.ps1 exited $LASTEXITCODE against the built bundle"
            $results += 'selftest: FAIL'
        }
        else { $results += 'selftest: ok' }
    }

    # PSScriptAnalyzer over the PRODUCT. The parts are excluded from the
    # analyzer on purpose, because a script-scoped SuppressMessage attribute
    # only covers the file it is in and the suppressions all live in
    # src/10-parameters.ps1. Analysing the bundle covers every line of every
    # part, and the -Check gate is what makes "the bundle is current" true.
    $mod = Get-Module -ListAvailable -Name PSScriptAnalyzer | Select-Object -First 1
    if (-not $mod) { $results += 'analyzer: skipped (PSScriptAnalyzer not installed)' }
    else {
        Import-Module PSScriptAnalyzer -ErrorAction Stop
        $issues = @(Invoke-ScriptAnalyzer -Path $script:BundlePath -Severity Error, Warning)
        if ($issues.Count -eq 0) { $results += 'analyzer: clean' }
        else {
            foreach ($i in ($issues | Select-Object -First 10)) {
                Add-Failure ("analyzer {0} line {1}: {2}" -f $i.RuleName, $i.Line, $i.Message)
            }
            $results += "analyzer: $($issues.Count) issue(s)"
        }
    }

    return , $results
}

function Get-CaseShadowedParameter {
    <#
      Find a local that a reader would take for a new variable and PowerShell
      takes for a parameter.

      ⛔ THE DEFECT THIS EXISTS FOR SHIPPED, AND ONLY DRIVING A REAL DISTRO
      FOUND IT. Write-StreamLogTick took a $State object and then wrote
      `$state = Get-DistroRunState ...`. PowerShell variable names are
      case-insensitive, so that IS the parameter: the state object became the
      string 'Running' and the next property read died, mid-run, on the one code
      path whose whole job is to keep reporting when everything else has gone
      quiet. PSScriptAnalyzer does not flag it and the suite could not see it.

      ⭐ IT FLAGS ONLY A DIFFERENT SPELLING, which is what makes it precise.
      Reassigning a parameter under its own exact name is ordinary and often
      right, and it reads as what it is. An assignment under a different case is
      an author who believed they were creating something.
    #>
    param([Parameter(Mandatory = $true)][string]$Text)
    $errs = $null; $tokens = $null
    $ast = [System.Management.Automation.Language.Parser]::ParseInput($Text, [ref]$tokens, [ref]$errs)
    if ($errs -and $errs.Count -gt 0) { return , @() }
    $out = @()
    foreach ($fn in $ast.FindAll({ $args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $true)) {
        $names = @()
        if ($fn.Parameters) { $names += @($fn.Parameters | ForEach-Object { $_.Name.VariablePath.UserPath }) }
        if ($fn.Body -and $fn.Body.ParamBlock) { $names += @($fn.Body.ParamBlock.Parameters | ForEach-Object { $_.Name.VariablePath.UserPath }) }
        if ($names.Count -eq 0) { continue }
        foreach ($asgn in $fn.FindAll({ $args[0] -is [System.Management.Automation.Language.AssignmentStatementAst] }, $true)) {
            $lhs = $asgn.Left
            if ($lhs -isnot [System.Management.Automation.Language.VariableExpressionAst]) { continue }
            $used = $lhs.VariablePath.UserPath
            foreach ($p in $names) {
                # ⛔ -ceq AND -ieq, NEVER -eq. PowerShell's -eq is
                # case-INSENSITIVE, so `$used -eq $p` was true for 'state'
                # against 'State' and the guard skipped exactly the thing it was
                # written to catch. It reported clean over a planted defect: a
                # guard that has never been seen to refuse is a guard nobody
                # knows works, and this one did not.
                if ($used -ceq $p) { continue }        # the same spelling: a deliberate reassignment
                if ($used -ieq $p) {
                    $out += ("{0}() assigns to `${1}, which IS the parameter `${2} because PowerShell " -f $fn.Name, $used, $p) +
                            ("ignores case. Line {0}." -f $asgn.Extent.StartLineNumber)
                }
            }
        }
    }
    # ⛔ NOT `return , $out`. Measured on PowerShell 7.6.5 on 2026-08-30: with an
    # empty list, `@(f)` at the call site reports Count 1 and the single element
    # is the empty array, while `$x = f` reports 0. The comma wraps the list in
    # a one-element array, and an array subexpression at the call site keeps
    # that element instead of unrolling it a second time. This check reported
    # one finding with a blank message over a tree that had none, which is a
    # gate inventing a defect: worse than missing one, because it sends a reader
    # after nothing.
    return $out
}

function Get-ParameterSurface {
    <#
      The CLI surface as a sorted list of stable lines: one per parameter, with
      its type, whether it is mandatory, its default, and its validate set; plus
      one per action.

      ⭐ This is the shape docs/consumers.md calls a break: a renamed parameter,
      a changed type, a removed action. Deriving it from the AST rather than
      from a regex means a parameter cannot be added without appearing here.
    #>
    param([Parameter(Mandatory = $true)][string]$Text)
    $errs = $null; $tokens = $null
    $ast = [System.Management.Automation.Language.Parser]::ParseInput($Text, [ref]$tokens, [ref]$errs)
    if ($errs -and $errs.Count -gt 0) { return , @() }
    $out = @()
    foreach ($p in $ast.ParamBlock.Parameters) {
        $name = $p.Name.VariablePath.UserPath
        $type = if ($p.StaticType) { $p.StaticType.Name } else { 'Object' }
        $dflt = if ($p.DefaultValue) { $p.DefaultValue.Extent.Text } else { '' }
        $set = ''
        foreach ($a in $p.Attributes) {
            if ($a.TypeName.Name -eq 'ValidateSet') {
                $vals = @($a.PositionalArguments | ForEach-Object { $_.Extent.Text.Trim("'", '"') })
                $set = ($vals -join '|')
            }
            elseif ($a.TypeName.Name -eq 'ValidateRange') {
                $vals = @($a.PositionalArguments | ForEach-Object { $_.Extent.Text })
                $set = 'range:' + ($vals -join '..')
            }
        }
        $out += ("param {0} type={1} default={2} valid={3}" -f $name, $type, $dflt, $set)
    }
    return , (@($out) | Sort-Object)
}

# --------------------------------------------------------------------------------------
# Main
# --------------------------------------------------------------------------------------
$exit = 0
$testResults = @()
try {
    Write-Line "wsl-toolkit build: $script:ToolRoot"

    $bytes = Build-BundleBytes
    if ($null -eq $bytes) {
        $exit = 1
    }
    else {
        $builtText = [Text.Encoding]::UTF8.GetString($bytes[3..($bytes.Length - 1)])
        if (-not (Test-BundleParses -Text $builtText)) {
            $exit = 1
        }
        elseif ($Check) {
            if (-not (Test-Path -LiteralPath $script:BundlePath)) {
                Add-Failure 'wsl-toolkit.ps1 is not on disk; run the build without -Check'
                $exit = 1
            }
            else {
                $disk = [IO.File]::ReadAllBytes($script:BundlePath)
                if (Compare-Bytes -Left $bytes -Right $disk) {
                    Write-Line "  ok    the tracked bundle matches its $((Get-ManifestParts).Count) parts"
                }
                else {
                    $diskText = [Text.Encoding]::UTF8.GetString($disk[3..($disk.Length - 1)])
                    $d = Get-FirstDifferingLine -Built $builtText -OnDisk $diskText
                    Add-Failure ("the tracked bundle disagrees with its parts at line {0}" -f $d.Line)
                    Add-Failure ("  parts say : {0}" -f $d.Built)
                    Add-Failure ("  bundle has: {0}" -f $d.OnDisk)
                    Add-Failure '  rebuild it: pwsh -NoProfile -File scripts/windows/wsl-toolkit/build.ps1'
                    $exit = 1
                }
            }
        }
        else {
            Write-BundleAtomically -Bytes $bytes
            Write-Line ("  ok    wrote {0} ({1:N0} bytes, {2} parts)" -f (Split-Path -Leaf $script:BundlePath), $bytes.Length, (Get-ManifestParts).Count)
        }

        if ($Test -and $script:Failures.Count -eq 0) {
            $testResults = Invoke-BuildTests -BundleText $builtText
            foreach ($r in $testResults) { Write-Line "  $r" }
        }
    }
}
catch {
    Add-Failure $_.Exception.Message
    $exit = 2
}

if ($script:Failures.Count -gt 0 -and $exit -eq 0) { $exit = 1 }

if ($Json) {
    $obj = [ordered]@{
        schema   = 'wsl-toolkit-build/1'
        mode     = if ($Check) { 'check' } elseif ($Test) { 'test' } else { 'build' }
        failures = @($script:Failures)
        tests    = @($testResults)
        exit     = $exit
    }
    Write-Output ($obj | ConvertTo-Json -Depth 4 -Compress)
}
else {
    foreach ($f in $script:Failures) { [Console]::Error.WriteLine("  ! $f") }
    if ($exit -eq 0) { Write-Output 'build ok' }
}

exit $exit
