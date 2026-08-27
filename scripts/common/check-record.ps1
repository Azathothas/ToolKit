# check-record.ps1 - do the record's counts still agree with its rows, and does
# every entry have a row and every row an entry?
#
# ⭐ THE TWIN OF check-record.sh. Same schema, same exit codes, same rules.
# check-twins.sh is what stops the two drifting.
#
# The defect this exists to catch is a record that contradicts itself. Closing
# one entry moves several numbers: the index total line, the priority table row,
# that table's total row, and the status beside the entry itself. Nothing is
# wrong with any single file afterwards. What is missing is anything that
# compares two of them.
#
# ⭐ THIS WAS PAID FOR BEFORE IT WAS WRITTEN. docs/methodology/work-todo.md
# records the incident: a published record said two entries were open, beside
# entries saying done, for the whole of the next session.
#
# ⛔ IT CANNOT CHECK THAT AN ENTRY IS TRUE. That is a reading and it belongs to
# the review pass.
#
# ⚠ THIS TWIN EXISTS BECAUSE THE sh ONE CANNOT BE ASSUMED TO RUN HERE. A native
# PowerShell session may have no awk and no sed at all, and its `sort` is an
# alias for Sort-Object, which succeeds and answers differently.
#
# Usage:
#   pwsh -NoProfile -File scripts/common/check-record.ps1
#   pwsh -NoProfile -File scripts/common/check-record.ps1 -Json
#   pwsh -NoProfile -File scripts/common/check-record.ps1 -Dir TODO
#
# Exit codes: 0 consistent, 1 inconsistent, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

[CmdletBinding()]
param(
    [switch]$Json,
    [string]$Dir = 'TODO'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    [Console]::Error.WriteLine('check-record: git not found')
    exit 2
}
# ⛔ stdout alone. `git rev-parse` in a repository with no commits writes a
# fatal to stderr, and a version of this that merged the streams would put that
# fatal into a path. docs/conventions/shell.md section 3.
$root = (& git rev-parse --show-toplevel 2>$null)
if ($LASTEXITCODE -ne 0 -or -not $root) {
    [Console]::Error.WriteLine('check-record: not a git repository')
    exit 2
}
$root = ($root | Select-Object -First 1).Trim()

$indexRel = "$Dir/INDEX.md"
$indexFull = Join-Path $root $indexRel
if (-not (Test-Path -LiteralPath $indexFull -PathType Leaf)) {
    [Console]::Error.WriteLine("check-record: no index at $indexRel")
    exit 2
}

$problems = New-Object System.Collections.ArrayList
function Add-Problem([string]$Text) { [void]$problems.Add('  ' + $Text) }

# ── the index rows ──────────────────────────────────────────────────────────
# A row is: | ID | PRI | EFF | STATUS | title | [`file`](file) |
# ⚠ Selected on an id shaped like LETTERS-DIGITS, not on position: the header
# and the separator both begin with a pipe.
$rows = New-Object System.Collections.ArrayList
foreach ($line in ([System.IO.File]::ReadAllText($indexFull) -split "`r?`n")) {
    if ($line -notmatch '^\|\s*[A-Z]+-[0-9]+\s*\|') { continue }
    $cells = $line -split '\|'
    if ($cells.Count -lt 7) { continue }
    $file = $cells[6].Trim()
    if ($file -match '\(([^)]+)\)') { $file = $Matches[1] }
    [void]$rows.Add([pscustomobject]@{
        Id     = $cells[1].Trim()
        Pri    = $cells[2].Trim()
        Eff    = $cells[3].Trim()
        Status = $cells[4].Trim()
        File   = $file.Trim()
    })
}
if ($rows.Count -eq 0) {
    [Console]::Error.WriteLine("check-record: no entry rows found in $indexRel")
    exit 2
}

# ── the entry headings, from every category file in the directory ───────────
$entries = New-Object System.Collections.ArrayList
$dirFull = Join-Path $root $Dir
foreach ($f in (Get-ChildItem -LiteralPath $dirFull -Filter '*.md' -File)) {
    if ($f.Name -in 'INDEX.md', 'PROGRESS.md', 'RULES.md') { continue }
    $text = [System.IO.File]::ReadAllText($f.FullName)
    $lines = $text -split "`r?`n"
    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -notmatch '^##\s+([A-Z]+-[0-9]+)\.') { continue }
        $id = $Matches[1]
        # the status stated under the heading, before the next heading
        $status = ''
        for ($j = $i + 1; $j -lt $lines.Count; $j++) {
            if ($lines[$j] -match '^##\s') { break }
            if ($lines[$j] -match '\*\*Status\*\*[^A-Za-z]*([A-Za-z]+)') { $status = $Matches[1]; break }
        }
        [void]$entries.Add([pscustomobject]@{
            Id = $id; File = "$Dir/$($f.Name)"; Status = $status
        })
    }
}

# ── every row has an entry, in the file the row names ───────────────────────
foreach ($r in $rows) {
    $hit = @($entries | Where-Object { $_.Id -ceq $r.Id })
    if ($hit.Count -eq 0) {
        Add-Problem "${indexRel}: row '$($r.Id)' has no entry heading '## $($r.Id).' in any file under $Dir/"
        continue
    }
    $e = $hit[0]
    if ($e.File -ne "$Dir/$($r.File)" -and $e.File -ne $r.File) {
        Add-Problem "${indexRel}: row '$($r.Id)' names $($r.File) but the entry is in $($e.File)"
    }
    if ($e.Status -and $e.Status -cne $r.Status) {
        Add-Problem "$($r.Id): index says '$($r.Status)', $($e.File) says '$($e.Status)'"
    }
}

# ── every entry has a row ───────────────────────────────────────────────────
foreach ($e in $entries) {
    if (-not @($rows | Where-Object { $_.Id -ceq $e.Id })) {
        Add-Problem "$($e.File): entry '$($e.Id)' has no row in $indexRel"
    }
}

# ── the declared counts agree with the rows ─────────────────────────────────
$aTotal   = $rows.Count
$aOpen    = @($rows | Where-Object { $_.Status -ceq 'open' }).Count
$aBlocked = @($rows | Where-Object { $_.Status -ceq 'blocked' }).Count
$aDone    = @($rows | Where-Object { $_.Status -ceq 'done' }).Count

$indexText = [System.IO.File]::ReadAllText($indexFull)
if ($indexText -match '(?m)^total\s+(\d+)\s+open\s+(\d+)\s+blocked\s+(\d+)\s+done\s+(\d+)') {
    $dTotal = [int]$Matches[1]; $dOpen = [int]$Matches[2]
    $dBlocked = [int]$Matches[3]; $dDone = [int]$Matches[4]
    if ($dTotal   -ne $aTotal)   { Add-Problem "${indexRel}: declares total $dTotal, rows say $aTotal" }
    if ($dOpen    -ne $aOpen)    { Add-Problem "${indexRel}: declares open $dOpen, rows say $aOpen" }
    if ($dBlocked -ne $aBlocked) { Add-Problem "${indexRel}: declares blocked $dBlocked, rows say $aBlocked" }
    if ($dDone    -ne $aDone)    { Add-Problem "${indexRel}: declares done $dDone, rows say $aDone" }
}
else {
    Add-Problem "${indexRel}: no 'total N open N blocked N done N' line. Nothing can be checked against the rows."
}

# ── the priority table agrees with the rows ─────────────────────────────────
foreach ($p in 'P0', 'P1', 'P2', 'P3') {
    $aP = @($rows | Where-Object { $_.Pri -ceq $p })
    $pOpen = @($aP | Where-Object { $_.Status -ceq 'open' }).Count
    $pBlk  = @($aP | Where-Object { $_.Status -ceq 'blocked' }).Count
    $pDone = @($aP | Where-Object { $_.Status -ceq 'done' }).Count
    $found = $false
    foreach ($line in ($indexText -split "`r?`n")) {
        if ($line -notmatch '^\|') { continue }
        $cells = $line -split '\|'
        if ($cells.Count -lt 6) { continue }
        if ($cells[1].Trim() -cne $p) { continue }
        $found = $true
        $dOpenP = $cells[2].Trim(); $dBlkP = $cells[3].Trim()
        $dDoneP = $cells[4].Trim(); $dTotP = $cells[5].Trim()
        if ($dOpenP -ne "$pOpen")      { Add-Problem "${indexRel}: $p declares open $dOpenP, rows say $pOpen" }
        if ($dBlkP  -ne "$pBlk")       { Add-Problem "${indexRel}: $p declares blocked $dBlkP, rows say $pBlk" }
        if ($dDoneP -ne "$pDone")      { Add-Problem "${indexRel}: $p declares done $dDoneP, rows say $pDone" }
        if ($dTotP  -ne "$($aP.Count)") { Add-Problem "${indexRel}: $p declares total $dTotP, rows say $($aP.Count)" }
        break
    }
    if (-not $found) { Add-Problem "${indexRel}: priority table has no row for $p" }
}

# ── the RECORD's own count line agrees too ─────────────────────────────────
# ⛔ THIS WAS THE GAP AND A REVIEW FOUND IT, NOT THE CHECK. The first version
# compared the index against the entries and stopped, so PROGRESS.md sat
# declaring "open 14 blocked 1" beside an index saying "open 15 blocked 0" and
# this reported clean. work-todo.md says closing an entry moves the record's own
# counts too. ⛔ Keep identical to the sh twin.
$recordRel = "$Dir/PROGRESS.md"
$recordFull = Join-Path $root $recordRel
if (Test-Path -LiteralPath $recordFull -PathType Leaf) {
    $recordText = [System.IO.File]::ReadAllText($recordFull)
    if ($recordText -match '(?m)total\s+(\d+)\s+open\s+(\d+)\s+blocked\s+(\d+)\s+done\s+(\d+)') {
        $rTotal = [int]$Matches[1]; $rOpen = [int]$Matches[2]
        $rBlocked = [int]$Matches[3]; $rDone = [int]$Matches[4]
        if ($rTotal   -ne $aTotal)   { Add-Problem "${recordRel}: declares total $rTotal, rows say $aTotal" }
        if ($rOpen    -ne $aOpen)    { Add-Problem "${recordRel}: declares open $rOpen, rows say $aOpen" }
        if ($rBlocked -ne $aBlocked) { Add-Problem "${recordRel}: declares blocked $rBlocked, rows say $aBlocked" }
        if ($rDone    -ne $aDone)    { Add-Problem "${recordRel}: declares done $rDone, rows say $aDone" }
    }
    # ⚠ No count line is not an error. Not every project states one.
}

# ── report ──────────────────────────────────────────────────────────────────
if ($Json) {
    Write-Output ('{"schema":"check-record/1","problems":' + $problems.Count +
                  ',"entries":' + $aTotal + ',"open":' + $aOpen +
                  ',"blocked":' + $aBlocked + ',"done":' + $aDone + '}')
    if ($problems.Count -gt 0) { exit 1 }
    exit 0
}

if ($problems.Count -gt 0) {
    Write-Output ("record check failed, " + $problems.Count + " problem(s):")
    Write-Output ''
    $problems | ForEach-Object { Write-Output $_ }
    Write-Output ''
    Write-Output 'The rules are in docs/methodology/work-todo.md. Fix the file that is'
    Write-Output 'wrong, not the count that reports it.'
    exit 1
}

Write-Output ("record ok: $aTotal entries ($aOpen open, $aBlocked blocked, $aDone done), counts agree with rows")
exit 0
