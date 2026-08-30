Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# --------------------------------------------------------------------------------------
# Constants
# --------------------------------------------------------------------------------------
# ⛔ NEITHER OF THESE TWO IS RENAMED WITH THE TOOL, and that is the decision
# rather than an oversight. Both name state that already exists on a machine:
# `eph-` is the prefix of distros that are registered right now, and the base
# directory holds their disks and their rootfs tarballs. Renaming either makes
# List and Purge blind to everything created before the rename, so multi-gigabyte
# VHDX files stop being reported by the only thing that reports them. A tool that
# renames itself and orphans the state it is responsible for has done the one
# thing docs/conventions/forbidden-patterns.md calls out about deletion, from the
# other direction: it has made the leftovers invisible instead of deleting them.
$script:Prefix  = 'eph-'
$script:BaseDir = Join-Path $env:LOCALAPPDATA 'wsl-ephemeral'

# What --import costs on the target volume, as a floor plus a multiple of the
# rootfs tarball. ⛔ Both are set ABOVE every measurement in
# Assert-EnoughDiskSpace rather than fitted to them: a tight preflight refuses
# an import that would have worked, which is a worse failure than the one it
# prevents. The floor is what matters, because an 8 MiB rootfs still costs 76.
$script:ImportSpaceFactor = 2
$script:ImportSpaceFloor  = 256MB

# How long an UNTERMINATED line may sit in the relay before the stream log
# shows it early, marked as unterminated. A constant rather than a flag: a
# prompt reading stdin that will never arrive is the case it exists for, and a
# caller who has hit that case is not in a position to know they should have
# raised a bound. 2000ms is comfortably longer than a progress bar's redraw
# interval, so an ordinary '\r' meter is never split by it.
$script:StreamFlushMs = 2000

# Names that must NEVER be unregistered, even if somebody prefixes them.
$script:Protected = @(
    'podman-machine-default',
    'docker-desktop',
    'docker-desktop-data',
    'rancher-desktop',
    'rancher-desktop-data'
)

# ⭐ ONE HOME FOR THE VERSION, and it is here. -Action Doctor prints it and
# release.ps1 reads it out of the built bundle to form the tag, so a release
# whose tag disagrees with the file inside it cannot be produced by accident.
# ⛔ Nothing else in this tree may carry a copy of it.
$script:ToolkitVersion = '1.0.1'

# ⛔ The Windows reserved device names, in any case and with any extension. A
# path check against this list is why -StreamLogPath nul is refused rather than
# accepted: writing to 'nul' discards everything and reports success, so a
# caller ends the run believing they have a log. 'con' writes to the console
# instead, which corrupts the very output the log was supposed to separate from.
$script:ReservedDeviceNames = @(
    'CON', 'PRN', 'AUX', 'NUL',
    'COM1', 'COM2', 'COM3', 'COM4', 'COM5', 'COM6', 'COM7', 'COM8', 'COM9',
    'LPT1', 'LPT2', 'LPT3', 'LPT4', 'LPT5', 'LPT6', 'LPT7', 'LPT8', 'LPT9'
)

# WSL emits UTF-16LE unless this is set; without it every parsed string is NUL-riddled.
$env:WSL_UTF8 = '1'

