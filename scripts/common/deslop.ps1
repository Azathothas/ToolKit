# deslop.ps1 - which files in this tree address a reader as an agent?
#
# ⭐ THE TWIN OF deslop.sh. Same schema, same exit codes, same patterns.
#
# ⭐ AN INVENTORY, NOT A GATE. It exits 0 whether it finds twenty agent-facing
# files or none, exactly as scripts/doctor/ does, because in the repository
# that SHIPS them their presence is correct. Only -Apply changes anything, and
# only then can it fail.
#
# ⚠ IT IS NOT A CHECK, so scripts/README.md's five-point contract does not
# apply in full. What it keeps is the header rule, the exit-code rule, and
# read-only-unless-a-flag-is-passed.
#
# -- WHAT IT IS FOR ----------------------------------------------------------
#
# A project adopts the engineering of `Azathothas/TEMPLATE` and wants none of
# the content written for a machine: a compliance rule forbids it, or the
# maintainer does not want it, which is a complete reason on its own.
#
# ⛔ IT IS AIMED AT ANOTHER TREE, NOT AT THIS ONE. It reports on whichever
# repository it is run from. ⚠ Run with `-Apply` here and it removes THIS
# repository's own router and methodology.
#
# ⭐ THE INTENDED PATH IS TO NEVER INSTALL IT, which is a SELECTION made at
# adoption and cheaper than any removal:
# https://github.com/Azathothas/TEMPLATE/blob/main/docs/methodology/lean-adoption.md
#
# -- ⛔ THE THREE THINGS IT WILL NOT DO --------------------------------------
#
# 1. ⛔ IT NEVER TOUCHES HISTORY. Removing a file going forward is complete and
#    reversible; rewriting published history un-publishes nothing, because
#    every fork, mirror, cache and archive keeps its copy, and it breaks every
#    clone and every open contribution. A red line in remote-ops.md.
# 2. ⛔ IT NEVER DELETES WITHOUT -Apply, and -Apply refuses on a dirty tree.
# 3. ⛔ IT DELETES NOTHING OUTSIDE THE LIST IT PRINTED.
#
# -- ⚠ WHAT IT CANNOT DECIDE -------------------------------------------------
#
# ⚠ Whether a file addresses an agent is a READING. This matches names. It will
# miss a file named something else. ⭐ The list is a starting point for a
# person, never an answer, which is why the default mode only prints.
#
# ⛔ AND IT CANNOT LIFT THE ENGINEERING OUT FIRST. Four practices under
# docs/methodology/ are engineering rather than agent instruction. The
# lean-adoption page linked above names all four. Read them, write them into the
# project's own contributing guide, THEN run this.
#
# Usage:
#   pwsh -NoProfile -File scripts/common/deslop.ps1
#   pwsh -NoProfile -File scripts/common/deslop.ps1 -Json
#   pwsh -NoProfile -File scripts/common/deslop.ps1 -Apply
#
# Exit codes: 0 the inventory ran, or the removal succeeded;
#             1 -Apply was asked for and could not be done safely;
#             2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

# ⛔ PositionalBinding IS OFF. A stray argument reaching a script whose job is
# deleting files must fail to bind rather than land on a parameter. A sibling
# script in this directory committed under a fabricated author because four
# expanded strings bound positionally, and that one only wrote a commit.
[CmdletBinding(PositionalBinding = $false)]
param(
    [switch]$Json,
    [switch]$Apply,
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($DryRun) { $Apply = $false }

if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    [Console]::Error.WriteLine('deslop: git not found')
    exit 2
}
$root = (& git rev-parse --show-toplevel 2>$null)
if ($LASTEXITCODE -ne 0 -or -not $root) {
    [Console]::Error.WriteLine('deslop: not a git repository')
    exit 2
}
$root = ($root | Select-Object -First 1).Trim()

# ⛔ ANCHORED, AND MATCHED ON THE WHOLE PATH. An unanchored match on "agent"
# would take src/agents/ in a project that happens to build one, which is a
# deletion of somebody's source code. Verified against exactly that fixture.
#
# ⚠ -cmatch, NOT -match. PowerShell's default comparison is case-INSENSITIVE,
# so `agents.md` in a project that ships a document about software agents would
# match AGENTS.md and be deleted. This trap has already made an exclusion in a
# sibling check swallow every real finding.
$patterns = @(
    '^AGENTS\.md$',           '/AGENTS\.md$',
    '^CLAUDE\.md$',           '/CLAUDE\.md$',
    '^GEMINI\.md$',           '/GEMINI\.md$',
    '^\.cursorrules$',        '/\.cursorrules$',
    '^\.windsurfrules$',      '/\.windsurfrules$',
    '^ROUTE\.md$', '^ADOPT\.md$', '^MAINTAIN\.md$',
    '^bootstrap/',
    '^docs/methodology/',
    '^docs/templates/',
    '^\.github/copilot-instructions\.md$'
)
function Test-AgentFacing([string]$P) {
    foreach ($pat in $patterns) { if ($P -cmatch $pat) { return $true } }
    return $false
}

Push-Location $root
try {
    $tracked = @(& git ls-files 2>$null)
    $untracked = @(& git ls-files --others --exclude-standard 2>$null)
}
finally { Pop-Location }

$files = @($tracked + $untracked | ForEach-Object { $_.Trim() } |
    Where-Object { $_ } | Sort-Object -Unique)

$hits = @($files | Where-Object { Test-AgentFacing $_ })
$others = @($files | Where-Object { -not (Test-AgentFacing $_) })

# ⭐ THE REFERENCES MATTER MORE THAN THE FILES. Deleting a document is easy;
# the expensive part is the links elsewhere that now resolve to nothing.
# Counting them here means the size of the real job is visible BEFORE anything
# is removed.
$nrefs = 0
if ($hits.Count -gt 0) {
    $escaped = ($hits | ForEach-Object { [regex]::Escape($_) }) -join '|'
    foreach ($f in $others) {
        $full = Join-Path $root $f
        if (-not (Test-Path -LiteralPath $full -PathType Leaf)) { continue }
        # ⚠ A FILE THIS CANNOT READ IS COUNTED AS A REFERENCE, NOT SKIPPED.
        # The count exists to tell somebody how much link-fixing a removal
        # implies, so an unreadable file has to round UP: reporting a smaller
        # number than the truth is the one direction that makes the estimate
        # dangerous. Swallowing it silently would have been worse still, which
        # is what the analyzer refuses an empty catch block for.
        try {
            $t = [System.IO.File]::ReadAllText($full)
            if ($t -cmatch $escaped) { $nrefs++ }
        }
        catch {
            $nrefs++
            [Console]::Error.WriteLine("deslop: could not read $f, counted as a reference: $($_.Exception.Message)")
        }
    }
}

if (-not $Apply) {
    if ($Json) {
        Write-Output ('{"schema":"deslop/1","agent_facing":' + $hits.Count + ',"referencing_files":' + $nrefs + ',"applied":false}')
        exit 0
    }
    if ($hits.Count -eq 0) {
        Write-Output 'no agent-facing files in this tree.'
        exit 0
    }
    Write-Output ("agent-facing files, {0}:" -f $hits.Count)
    Write-Output ''
    $hits | ForEach-Object { Write-Output $_ }
    Write-Output ''
    Write-Output ("{0} other file(s) reference one of them, and every such link breaks on removal." -f $nrefs)
    Write-Output ''
    Write-Output '⛔ Read the lean-adoption page in Azathothas/TEMPLATE before removing any'
    Write-Output 'of this. Four practices under docs/methodology/ are engineering rather than'
    Write-Output "agent instruction. Lift them into the project's own contributing guide first."
    Write-Output ''
    Write-Output 'Nothing was changed. Pass -Apply to remove the list above.'
    exit 0
}

# -- -Apply ------------------------------------------------------------------
# ⛔ REFUSES ON A DIRTY TREE. A deletion of this size mixed into uncommitted
# work cannot be reviewed, and cannot be undone with one command.
Push-Location $root
try { $dirty = @(& git status --porcelain 2>$null | Where-Object { $_ }) }
finally { Pop-Location }
if ($dirty.Count -gt 0) {
    [Console]::Error.WriteLine('deslop: the tree is dirty. Commit or stash first.')
    [Console]::Error.WriteLine('deslop: a removal this size has to be reviewable on its own.')
    exit 1
}

if ($hits.Count -eq 0) {
    Write-Output 'nothing to remove.'
    exit 0
}

# ⛔ THE STATE IS READ BACK, AND THE REPORT IS WHAT IS TRUE rather than what was
# attempted. The predecessor printed the number it had PLANNED to remove beside
# a `Remove-Item -ErrorAction SilentlyContinue`, so a file something held open
# read as a file that had gone. docs/conventions/forbidden-patterns.md carries
# that exact shape.
$removed = 0
$survived = New-Object System.Collections.ArrayList
Push-Location $root
try {
    foreach ($f in $hits) {
        & git rm -q -- $f 2>$null
        if ($LASTEXITCODE -ne 0) {
            Remove-Item -Force -LiteralPath (Join-Path $root $f) -ErrorAction SilentlyContinue
        }
        if (Test-Path -LiteralPath (Join-Path $root $f)) { [void]$survived.Add($f) }
        else { $removed++ }
    }
}
finally { Pop-Location }

Write-Output ("removed {0} of {1} agent-facing file(s)." -f $removed, $hits.Count)
if ($survived.Count -gt 0) {
    Write-Output ''
    Write-Output '⛔ STILL PRESENT after the removal:'
    $survived | ForEach-Object { Write-Output ('  ' + $_) }
    Write-Output ''
    Write-Output 'Something is holding them open, or the path is not writable. Nothing was'
    Write-Output 'reported as removed that is still there.'
    exit 1
}
Write-Output ''
Write-Output ("⛔ NOT DONE YET. {0} file(s) referenced them and those links now resolve" -f $nrefs)
Write-Output 'to nothing. Run the documentation check and fix every one:'
Write-Output ''
Write-Output '    pwsh -NoProfile -File scripts/common/check-docs.ps1'
Write-Output ''
Write-Output '⛔ History was NOT touched, and rewriting it would not un-publish anything.'
Write-Output 'Every fork, mirror and archive keeps its copy. docs/security/remote-ops.md.'
exit 0
