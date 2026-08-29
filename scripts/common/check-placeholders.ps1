# check-placeholders.ps1 - did a template placeholder survive into a real file?
#
# ⭐ THE TWIN OF check-placeholders.sh. Same schema, same exit codes, same
# exemptions. check-twins.sh is what stops the two drifting.
#
# The defect this exists to catch is a document that reads as finished and is
# not. A leftover double-brace marker in a router, a record or a licence is a
# sentence that looks authoritative and says nothing, and the next session acts
# on it. The failure is quiet: nothing errors, and the file is the right shape.
#
# It also catches the other half, which is easier to miss: a template GUIDANCE
# comment left in a real file. Those read as instructions and are addressed to
# whoever was filling the file in, not to whoever is reading it now.
#
# Usage:
#   pwsh -NoProfile -File scripts/common/check-placeholders.ps1
#   pwsh -NoProfile -File scripts/common/check-placeholders.ps1 -Json
#
# Exit codes: 0 clean, 1 something survived, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

[CmdletBinding()]
param(
    [switch]$Json
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    [Console]::Error.WriteLine('check-placeholders: git not found')
    exit 2
}
$root = (& git rev-parse --show-toplevel 2>$null)
if ($LASTEXITCODE -ne 0 -or -not $root) {
    [Console]::Error.WriteLine('check-placeholders: not a git repository')
    exit 2
}
$root = ($root | Select-Object -First 1).Trim()

# ⚠ THE ONE FILL-IN FORM IS EXEMPT AND MUST BE. TODO/ENTRY.md is the shape an
# entry is written from, so holding placeholders is its whole job, and a check
# that failed on it would fail on a correct tree.
# ⛔ IT NAMES ONE FILE, NOT A DIRECTORY. It used to exempt three directories
# inherited from a template repository, none of which exist here now.
# ⛔ BOTH implementations of this check are exempt, because each one contains
# the patterns it looks for. Exempting only one is how the twins disagree.
$exempt = '^(TODO/ENTRY\.md$|scripts/common/check-placeholders\.(sh|ps1))'

Push-Location $root
try {
    $tracked = @(& git ls-files 2>$null)
    $untracked = @(& git ls-files --others --exclude-standard 2>$null)
}
finally { Pop-Location }

$files = @($tracked + $untracked |
    ForEach-Object { $_.Trim() } |
    Where-Object { $_ -and $_ -notmatch $exempt } |
    Sort-Object -Unique)

if ($files.Count -eq 0) {
    [Console]::Error.WriteLine('check-placeholders: no files in scope')
    exit 2
}

# ⚠ A binary file is skipped, matching `grep -I` in the sh twin. Reading one as
# text would either throw or produce replacement characters, and neither is a
# finding about placeholders.
function Read-TextOrNull([string]$Path) {
    try {
        $bytes = [System.IO.File]::ReadAllBytes($Path)
    } catch { return $null }
    $limit = [Math]::Min($bytes.Length, 8000)
    for ($i = 0; $i -lt $limit; $i++) { if ($bytes[$i] -eq 0) { return $null } }
    return [System.Text.Encoding]::UTF8.GetString($bytes)
}

$categories = 0
$report = New-Object System.Collections.ArrayList

function Add-Category([string]$Title, $Hits) {
    if ($Hits.Count -eq 0) { return $false }
    [void]$report.Add('')
    [void]$report.Add("== $Title ==")
    $Hits | ForEach-Object { [void]$report.Add($_) }
    return $true
}

$braceHits = New-Object System.Collections.ArrayList
$guideHits = New-Object System.Collections.ArrayList
$standHits = New-Object System.Collections.ArrayList
$ownerHits = New-Object System.Collections.ArrayList

foreach ($rel in $files) {
    $full = Join-Path $root $rel
    if (-not (Test-Path -LiteralPath $full -PathType Leaf)) { continue }
    $text = Read-TextOrNull $full
    if ($null -eq $text) { continue }

    $n = 0
    foreach ($line in ($text -split "`r?`n")) {
        $n++

        # 1. A double-brace placeholder.
        # ⚠ `${{ }}` is GitHub Actions expression syntax and `{{.Field}}` is a
        #    Go template: `podman info --format '{{.Host.Arch}}'` has that
        #    shape, and this rule fired on one the day such a script arrived.
        #    ⭐ Narrowed rather than switched off, on a shape that cannot
        #    collide: every placeholder this template ships is a word or a
        #    sentence and every one begins with an UPPERCASE letter.
        # ⚠ EXCLUDING ONLY `{{.` WAS TOO NARROW. It fired on
        #    `podman image inspect --format '{{json .Config.Env}}'`. A Go
        #    template calls functions as well as reading fields, so `{{json`,
        #    `{{range`, `{{printf`, `{{if` and `{{end}}` begin with a lowercase
        #    letter. Excluding "a dot or a lowercase letter" covers every
        #    docker, podman, helm and kubectl format string and still cannot
        #    collide with an uppercase placeholder.
        # ⛔ Keep this identical to the sh twin. check-twins is what notices.
        # ⛔ `-cnotmatch`, NOT `-notmatch`. PowerShell's `-match` family is
        #    CASE-INSENSITIVE, so `[a-z]` matches the `O` in `{{OPERATOR}}` and
        #    the Go-template exclusion silently swallowed every real
        #    placeholder. The check reported "no placeholders survived" over a
        #    file containing one. Caught by planting a placeholder and reading
        #    the exit code, which is the only reason it was caught at all.
        #    docs/conventions/shell.md section 8.
        if ($line -match '\{\{' -and $line -notmatch '\$\{\{' -and $line -cnotmatch '\{\{ *[a-z.]') {
            [void]$braceHits.Add("${rel}:${n}:$line")
        }

        # 2. A template guidance comment, addressed to whoever was filling it in.
        if ($line -match '<!-- *TEMPLATE' -or $line -match 'delete this comment' -or $line -match 'Fill every') {
            [void]$guideHits.Add("${rel}:${n}:$line")
        }

        # 3. The obvious stand-ins. ⚠ Deliberately narrow: these mean "somebody
        #    meant to change this", not every occurrence of the word example.
        #    A rule that fires on example.com is a rule nobody keeps, and
        #    example.com is the CORRECT thing to write in a public document.
        if ($line -cmatch 'YOUR_(NAME|EMAIL|PROJECT|TOKEN)' -or $line -cmatch 'CHANGEME' -or
            $line -match '<your-' -or $line -match 'TODO: fill') {
            [void]$standHits.Add("${rel}:${n}:$line")
        }

        # 4. OWNER/REPO, but only where it is configuration rather than prose.
        # ⚠ Deliberately NOT in the list above. OWNER/REPO is the RECOMMENDED
        #    generic for a public document, so a rule against it everywhere
        #    would fire on correct writing.
        if ($rel -notmatch '\.md$' -and $line -cmatch 'OWNER/REPO') {
            [void]$ownerHits.Add("${rel}:${n}:$line")
        }
    }
}

if (Add-Category 'a placeholder survived' $braceHits) { $categories++ }
if (Add-Category 'a template guidance comment survived' $guideHits) { $categories++ }
if (Add-Category 'a stand-in value survived' $standHits) { $categories++ }
if (Add-Category 'OWNER/REPO survived in a configuration file' $ownerHits) { $categories++ }

if ($Json) {
    Write-Output ('{"schema":"check-placeholders/1","categories":' + $categories + ',"files_scanned":' + $files.Count + '}')
    if ($categories -gt 0) { exit 1 }
    exit 0
}

if ($categories -gt 0) {
    $report | ForEach-Object { Write-Output $_ }
    Write-Output ''
    Write-Output ("⛔ {0} category/categories survived into real files." -f $categories)
    Write-Output ''
    Write-Output 'Each one is a sentence that looks authoritative and says nothing.'
    Write-Output 'Fill it in, or delete the section it is in. ⚠ Do not delete the'
    Write-Output 'placeholder alone and leave the sentence around it: that produces a'
    Write-Output 'claim nobody wrote.'
    exit 1
}

Write-Output ("no placeholders survived in {0} files (TODO/ENTRY.md is exempt)" -f $files.Count)
exit 0
