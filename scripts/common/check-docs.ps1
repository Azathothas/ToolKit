# check-docs.ps1 - do the documents still resolve, and are they written the way
# this repository writes documents?
#
# ⭐ THE TWIN OF check-docs.sh. Same schema, same exit codes, same exemptions.
# check-twins.ps1 is what stops the two drifting.
#
# The defect this exists to catch is a document that was true when it was
# written. Three shapes of it, and every one is invisible to every other check:
#
#   - a link or a path that stopped resolving when something was renamed;
#   - a fenced shell block that does not parse, which is a block nobody can
#     copy and paste;
#   - an angle-bracket placeholder inside a shell block: a human reads it as
#     "fill this in" and bash reads it as a redirect, so the reader gets a
#     cryptic syntax error instead of an obvious instruction.
#
# ⚠ CONTROL BYTES ARE NOT CHECKED HERE. That rule scanned markdown only while
# every .ts, .py, .rs and .sh in the tree went unchecked, so it moved to
# check-control-bytes.ps1, which reads every text file. Run both.
#
# It also enforces the mechanical half of the prose rule: no em dash, and no
# emoji outside the five this repository defines. docs/conventions/prose.md is
# the rule; this is the part a machine can hold.
#
# ⛔ WHAT IT DOES NOT CHECK IS WHETHER A CLAIM IS TRUE. That is a reading, and
# it belongs to the review pass. A guard that tried to verify prose would
# either pass vacuously or refuse legitimate writing, and both are worse than
# an honest scope.
#
# ⚠ THE SHELL-BLOCK PARSE NEEDS A POSIX SHELL, AND THIS HOST MAY NOT HAVE ONE.
# When no `sh` is on PATH the blocks are still COUNTED, so the schema matches
# the sh twin, and the parse rule is reported as SKIPPED on stderr rather than
# silently passing. ⛔ A rule that cannot run must say so: reporting green for
# a check that never executed is the failure this whole repository is built to
# avoid.
#
# Usage:
#   pwsh -NoProfile -File scripts/common/check-docs.ps1
#   pwsh -NoProfile -File scripts/common/check-docs.ps1 -Json
#   pwsh -NoProfile -File scripts/common/check-docs.ps1 -Path docs
#
# Exit codes: 0 clean, 1 something is wrong, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

[CmdletBinding()]
param(
    [switch]$Json,
    [string]$Path = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    [Console]::Error.WriteLine('check-docs: git not found')
    exit 2
}
$root = (& git rev-parse --show-toplevel 2>$null)
if ($LASTEXITCODE -ne 0 -or -not $root) {
    [Console]::Error.WriteLine('check-docs: not a git repository')
    exit 2
}
$root = ($root | Select-Object -First 1).Trim()

Push-Location $root
try {
    $tracked = @(& git ls-files 2>$null)
    $untracked = @(& git ls-files --others --exclude-standard 2>$null)
}
finally { Pop-Location }

$all = @($tracked + $untracked | ForEach-Object { $_.Trim() } | Where-Object { $_ } | Sort-Object -Unique)
$files = @($all | Where-Object { $_ -match '\.md$' })
if ($Path) {
    $prefix = $Path.TrimEnd('/', '\').Replace('\', '/')
    $files = @($files | Where-Object { $_ -like "$prefix/*" -or $_ -eq $prefix })
}
if ($files.Count -eq 0) {
    [Console]::Error.WriteLine('check-docs: no markdown files in scope')
    exit 2
}

$problems = New-Object System.Collections.ArrayList
$count = 0
$nlinks = 0
$nblocks = 0
function Add-Problem([string]$Text) {
    [void]$script:problems.Add('  ' + $Text)
    $script:count++
}
$script:problems = $problems
$script:count = 0

# A POSIX shell, if this host has one. See the header: absence is reported,
# never silently treated as a pass.
$shell = $null
foreach ($c in 'sh', 'bash') {
    $g = Get-Command $c -ErrorAction SilentlyContinue
    if ($g -and $g.CommandType -in 'Application', 'ExternalScript') { $shell = $g.Source; break }
}
$skippedParse = 0

# ⚠ THE TEMPLATE DIRECTORIES ARE EXEMPT FROM THE LINK CHECK, AND MUST BE.
# A template's links are written relative to where the file will live in the
# PROJECT, not where it lives here: docs/templates/AGENTS.md links to
# docs/methodology/gate.md because in a project that file sits at the root.
# Checking those here reports thirty-odd failures on a correct tree, and a
# check that fails on a correct tree gets switched off within a week.
# ⭐ The PROSE rules still apply to templates. Only link resolution is exempt,
# because only that one is position-dependent.
function Test-LinkExempt([string]$Rel) {
    return ($Rel -like 'docs/templates/*' -or $Rel -like 'bootstrap/prompts/*')
}

$linked = New-Object System.Collections.Generic.HashSet[string]

function Get-LinkTarget([string]$Text) {
    # Strip fenced blocks, then code spans, then take every ](...) target.
    # ⚠ Stripping code spans is why a backticked expression is not reported as
    # a broken link. Markdown does not linkify a code span, and an earlier
    # version of this check reported exactly that as broken.
    $out = New-Object System.Collections.ArrayList
    $fence = $false
    $n = 0
    foreach ($line in ($Text -split "`r?`n")) {
        $n++
        if ($line -match '^[ \t]*```') { $fence = -not $fence; continue }
        if ($fence) { continue }
        $clean = [regex]::Replace($line, '`[^`]*`', '')
        foreach ($m in [regex]::Matches($clean, '\]\(([^)\s]+)')) {
            [void]$out.Add([pscustomobject]@{ Line = $n; Target = $m.Groups[1].Value })
        }
    }
    return $out
}

foreach ($rel in $files) {
    $full = Join-Path $root $rel
    if (-not (Test-Path -LiteralPath $full -PathType Leaf)) { continue }
    $text = [System.IO.File]::ReadAllText($full)
    # ⛔ FORWARD SLASHES, ALWAYS. Split-Path returns a WINDOWS separator, so
    # `docs\conventions` has no `/` in it, and the `..` collapse below then
    # treats the whole thing as ONE segment: `docs\conventions/../../x`
    # collapsed to `../x`, which resolves outside the repository and reported
    # thirty-one correct links as broken. git speaks forward slashes and so
    # does every link in a markdown file; the only thing that did not was this
    # one call.
    $dir = (Split-Path -Parent $rel).Replace([char]92, '/')
    if (-not $dir) { $dir = '.' }
    $linkCheck = -not (Test-LinkExempt $rel)

    # ── em dash, outside fences and code spans ──────────────────────────────
    $fence = $false
    $n = 0
    foreach ($line in ($text -split "`r?`n")) {
        $n++
        if ($line -match '^[ \t]*```') { $fence = -not $fence; continue }
        if ($fence) { continue }
        $clean = [regex]::Replace($line, '`[^`]*`', '')
        if ($clean.Contains([char]0x2014)) {
            Add-Problem ($rel + ':' + $n + ' em dash. docs/conventions/prose.md')
        }
    }

    # ── links ───────────────────────────────────────────────────────────────
    foreach ($t in (Get-LinkTarget $text)) {
        $target = $t.Target
        if ($target -match '^(https?:|mailto:)' -or -not $target) { continue }
        # ⚠ COUNTED BEFORE THE EMPTY TEST, to match the sh twin exactly. A
        # pure-anchor link like the section links in this repository's own
        # documents has no path part, so it is counted as examined and then
        # skipped. Counting it after instead put the two implementations one
        # apart on a clean tree, which check-twins reports as drift and which
        # is a real disagreement about what the number means.
        $bare = ($target -split '#')[0]
        if (-not $linkCheck) { continue }
        $script:nlinks++
        if (-not $bare) { continue }
        # Normalise to a repo-relative path so a link from a subdirectory and
        # one from the root name the same file.
        $joined = if ($dir -eq '.') { $bare } else { $dir + '/' + $bare }
        $norm = $joined -replace '/\./', '/'
        while ($norm -match '[^/]+/\.\./') { $norm = $norm -replace '[^/]+/\.\./', '' }
        $norm = $norm -replace '^\./', ''
        [void]$linked.Add($norm)
        if (-not (Test-Path -LiteralPath (Join-Path $root $norm))) {
            Add-Problem ($rel + ':' + $t.Line + ' broken link -> ' + $target)
        }
    }
}

# ── fenced shell blocks ─────────────────────────────────────────────────────
foreach ($rel in $files) {
    $full = Join-Path $root $rel
    if (-not (Test-Path -LiteralPath $full -PathType Leaf)) { continue }
    $lines = [System.IO.File]::ReadAllText($full) -split "`r?`n"
    $inBlock = $false
    $start = 0
    $buf = New-Object System.Collections.ArrayList
    for ($i = 0; $i -lt $lines.Count; $i++) {
        $line = $lines[$i]
        if (-not $inBlock -and $line -match '^[ \t]*```(bash|sh)[ \t]*$') {
            $inBlock = $true; $start = $i + 1; [void]$buf.Clear(); continue
        }
        if ($inBlock -and $line -match '^[ \t]*```') {
            $inBlock = $false
            $nblocks++
            $body = ($buf -join "`n")

            if ($body -match '<[a-z][a-z0-9-]*>') {
                Add-Problem ($rel + ':' + $start + ' shell-unsafe placeholder. bash reads it as a redirect; use UPPER_SNAKE')
            }

            if ($shell) {
                # ⛔ A TEMP FILE, NOT stdin. docs/conventions/shell.md: from
                # PowerShell a native command's stdin is not byte-exact, and a
                # trailing CRLF gets appended. For a syntax check that is the
                # difference between a real answer and a fabricated one.
                $tmp = Join-Path ([System.IO.Path]::GetTempPath()) ('checkdocs-' + [guid]::NewGuid().ToString('N') + '.sh')
                try {
                    [System.IO.File]::WriteAllText($tmp, ($body -replace "`r", '') + "`n")
                    $prev = $ErrorActionPreference
                    $ErrorActionPreference = 'Continue'
                    try { & $shell -n $tmp 2>$null | Out-Null } finally { $ErrorActionPreference = $prev }
                    if ($LASTEXITCODE -ne 0) {
                        Add-Problem ($rel + ':' + $start + ' shell block does not parse')
                    }
                }
                finally { Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue }
            }
            else { $skippedParse++ }
            continue
        }
        if ($inBlock) { [void]$buf.Add($line) }
    }
}

# ── a page nothing links to ─────────────────────────────────────────────────
# ⛔ AN UNLINKED PAGE IS NOT READ, SO IT IS NOT CORRECTED, and that is the state
# every stale document passes through on the way to being wrong.
# Roots are exempt: a README is an entry point, and the files at the repository
# root are what a reader or a raw URL arrives at directly.
foreach ($rel in $files) {
    if ($rel -match '(^|/)README\.md$') { continue }
    if ($rel -notmatch '/') { continue }
    if (-not $linked.Contains($rel)) {
        Add-Problem ($rel + ' is linked from nowhere. An unlinked page is not read, so it is not corrected.')
    }
}

# ── only the five defined characters ──────────────────────────────────────────
# ⭐ .NET regex speaks unicode natively, so unlike the sh twin this rule never
# has to degrade. The sh side needs a grep with -P in UTF-8 mode and says so
# when it does not have one.
# ⚠ CODEPOINTS, NOT A REGEX CLASS. .NET does not understand `\u{1F300}`: that
# is PCRE and JavaScript syntax, and .NET reads it as `\u` plus a brace and
# throws "Insufficient or invalid hexadecimal digits". Astral characters would
# otherwise need surrogate pairs spelled out by hand. Comparing integers is
# both correct and readable, and it keeps these ranges identical to the sh
# twin's, which is what check-twins compares.
$ranges = @(
    @(0x1F300, 0x1FAFF),
    @(0x2190, 0x21FF),
    @(0x2300, 0x23FF),
    @(0x2500, 0x27BF),
    @(0x2B00, 0x2BFF),
    @(0xFE0F, 0xFE0F)
)
# The five this repository defines, and nothing else. prose.md is the rule:
# three prose markers, then two status glyphs for machine output and result
# tables. ⛔ Adding a sixth is a change to prose.md first, and to both twins.
#
# ⚠ This twin tests one CHARACTER at a time, so it never had the line-level
# false negative the sh twin was carrying. Keep it that way: a per-line test
# here would hide a banned emoji sitting beside an allowed marker.
$allowed = @(0x26D4, 0x2B50, 0x26A0, 0x2705, 0x274C)
foreach ($rel in $files) {
    $full = Join-Path $root $rel
    if (-not (Test-Path -LiteralPath $full -PathType Leaf)) { continue }
    $n = 0
    foreach ($line in ([System.IO.File]::ReadAllText($full) -split "`r?`n")) {
        $n++
        for ($i = 0; $i -lt $line.Length; $i++) {
            $cp = [int]$line[$i]
            # ⚠ A surrogate pair is ONE character. Reading the halves as two
            # separate code units puts every emoji above U+FFFF in the D800
            # range, which is in none of the ranges above, so every one of them
            # would be missed.
            if ([char]::IsHighSurrogate($line[$i]) -and ($i + 1) -lt $line.Length) {
                $cp = [char]::ConvertToUtf32($line[$i], $line[$i + 1])
                $i++
            }
            if ($allowed -contains $cp) { continue }
            foreach ($r in $ranges) {
                if ($cp -ge $r[0] -and $cp -le $r[1]) {
                    $shown = $line
                    if ($shown.Length -gt 90) { $shown = $shown.Substring(0, 90) }
                    Add-Problem ('an emoji outside the five defined characters: ' + $rel + ':' + $n + ':' + $shown)
                    break
                }
            }
        }
    }
}

$count = $script:count

if ($Json) {
    Write-Output ('{"schema":"check-docs/1","problems":' + $count + ',"files":' + $files.Count + ',"links":' + $nlinks + ',"shell_blocks":' + $nblocks + '}')
    if ($skippedParse -gt 0) {
        [Console]::Error.WriteLine('⚠ no POSIX shell on PATH: ' + $skippedParse + ' shell block(s) counted but NOT parsed')
    }
    if ($count -gt 0) { exit 1 }
    exit 0
}

if ($count -gt 0) {
    Write-Output ('documentation check failed, ' + $count + ' problem(s):')
    Write-Output ''
    $problems | ForEach-Object { Write-Output $_ }
    Write-Output ''
    if ($skippedParse -gt 0) {
        Write-Output ('⚠ no POSIX shell on PATH: ' + $skippedParse + ' shell block(s) counted but NOT parsed')
    }
    exit 1
}

Write-Output ('docs ok: ' + $files.Count + ' files, ' + $nlinks + ' relative links, ' + $nblocks + ' shell blocks. Links and prose clean.')
if ($skippedParse -gt 0) {
    Write-Output ('⚠ no POSIX shell on PATH: ' + $skippedParse + ' shell block(s) counted but NOT parsed')
}
exit 0
