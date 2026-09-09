<#
.SYNOPSIS
    are binfmt_misc handlers actually registered in the kernel that containers
    on this machine run against?

.DESCRIPTION
    A WRAPPER. The tool is `repo binfmt` in tools/repo.

    The defect it exists to catch is cross-architecture execution that has
    never once worked while every visible signal says the machine is healthy.
    Measured on 2026-08-27: systemd-binfmt.service reported status=0/SUCCESS
    having registered ZERO handlers, because the path it writes to had a
    systemd autofs stacked on the binfmt_misc mount and every read returned
    ELOOP. The unit was green and podman run --platform linux/arm64 failed with
    an Exec format error that reads like an unrelated breakage.

    It reads the KERNEL, not a unit's exit code. That is the whole point: the
    unit is the thing that lied.

    ON A WINDOWS HOST THIS STARTS A WSL DISTRIBUTION to read its kernel, and
    the default one is podman-machine-default because that is where the engine
    usually lives. It only READS; it registers nothing and stops nothing. On a
    machine where that distribution belongs to somebody else, name another with
    -Distro.

    -Require N is what turns this from a report into an assertion. Without it a
    count of zero is reported and exits 0, because a machine that never wanted
    cross-architecture execution is not broken.

.NOTES
    Exit codes: 0 read it, 1 the kernel state is broken or below -Require,
                2 could not run.
    Read the exit code from this process, unpiped.
#>
[CmdletBinding()]
param(
    [switch]$Json,
    [string]$Distro = 'podman-machine-default',
    [int]$Require = 0
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$forward = @('binfmt')
if ($Json)    { $forward += '--json' }
if ($Distro)  { $forward += @('--distro', $Distro) }
if ($Require) { $forward += @('--require', [string]$Require) }

$entry = Join-Path $PSScriptRoot 'repo.ps1'
& $entry @forward
exit $LASTEXITCODE
