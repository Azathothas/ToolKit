<#
.SYNOPSIS
    Create, use, and destroy ephemeral WSL2 distros on demand.

.DESCRIPTION
    Builds throwaway WSL2 distros from OCI images (any distro, any tag available on a
    container registry) or from a local rootfs tarball, runs commands inside them, and
    removes them cleanly.

    SAFETY MODEL -- this script is destructive by nature, so removal is constrained four ways:
      1. Every distro it creates is named with a fixed prefix (default 'eph-').
      2. It REFUSES to remove any distro whose name lacks that prefix.
      3. It REFUSES to remove any name on an explicit protected list, prefix or not.
         'podman-machine-default' and the Docker Desktop distros are protected, so a
         mistake here cannot destroy your container runtime.
      4. Directory deletion is confined to %LOCALAPPDATA%\wsl-ephemeral\<distro>; the base
         directory itself and anything outside it can never be the target.

    Destructive actions require -Force when running non-interactively.

.PARAMETER Action
    New          Create an ephemeral distro (from -Image or -Tarball).
    Run          Run a command inside an existing ephemeral distro.
    Enter        Attach an interactive shell to an existing ephemeral distro.
    List         List ephemeral distros, and show what else exists (never touched).
    Remove       Unregister one ephemeral distro and delete its disk.
    Purge        Remove ALL ephemeral distros (prefix-matched only).
    Resources    Report what WSL and the container engine are holding on this
                 machine, and PRINT the cleanup commands without running any of
                 them. Read-only. Nothing it reports is this script's to remove.
    HostAddress  Print the address a distro reaches THIS host at, for the
                 current WSL networking mode. Read-only, and it does not create
                 a distro to find out.

.PARAMETER Image
    OCI image reference, e.g. 'alpine:3.22', 'debian:bullseye-slim', 'fedora:44',
    'archlinux:latest', 'opensuse/tumbleweed', 'rockylinux:9', 'voidlinux/voidlinux'.

.PARAMETER Tarball
    Path to a rootfs tarball (.tar / .tar.gz) to import instead of pulling an image.
    Lets the script work with no container engine installed.

.PARAMETER Name
    Distro name. Auto-generated when omitted. The prefix is added if missing.

.PARAMETER Command
    Shell command to run, via /bin/sh -lc. It travels as base64 and is sourced
    inside the distro, so a quote, a dollar sign, a backtick or a tab arrives
    byte-exact instead of being re-parsed in transit.

.PARAMETER CommandFile
    Path to a file ON THIS MACHINE whose bytes are the command. Read verbatim,
    which is what makes a multi-line script possible at all.

.PARAMETER CommandB64
    The command as base64 of its UTF-8 bytes, for a caller that already holds
    the text and wants no shell anywhere near it. Command, CommandFile and
    CommandB64 are mutually exclusive.

.PARAMETER User
    User to run as inside the distro. Default 'root'.

.PARAMETER Ephemeral
    With -Action New: run -Command then immediately destroy the distro.

.PARAMETER OciEnv
    With -Action New -Image: carry the image's OCI environment into the distro,
    as /etc/profile.d/10-oci-env.sh. Off by default, because turning it on
    changes PATH for every caller of a shape they already depend on.

.PARAMETER Systemd
    With -Action New: write /etc/wsl.conf enabling systemd, restart the distro
    so WSL reads it, and REFUSE if systemd did not become PID 1. Most OCI base
    images do not ship systemd at all; see Enable-DistroSystemd.

.PARAMETER Verbatim
    With -CommandFile: send the file's bytes exactly as they are. Off by
    default, which means the COPY IN TRANSIT gets its CRLF turned into LF and a
    UTF-8 byte order mark removed, with a line saying what changed. THE FILE ON
    DISK IS NEVER WRITTEN TO EITHER WAY.

.PARAMETER ScriptArg
    NAME=VALUE, repeatable. Prepended to the command as POSIX-quoted shell
    assignments, so a caller stops running sed over their own script to inject
    a value. The literal token @hostaddress inside a VALUE is replaced with what
    -Action HostAddress would print.

.PARAMETER NoTimestamps
    Turn the whole stream log off: no timestamps, no heartbeat, no bound on the
    command. The child inherits this process's handles and its bytes reach the
    terminal untouched, which is what this script did before the log existed.

.PARAMETER TimestampMode
    Which clock the prefix carries: Relative (the default, elapsed since the
    command started), Delta (since the previous line), Wall, Iso or Epoch.

.PARAMETER TimestampFormat
    A strftime format, with tss's specifier set, for Relative and Wall. Passing
    it with Delta, Iso or Epoch is refused rather than ignored.

.PARAMETER TickSeconds
    Seconds of SILENCE before a heartbeat line. Default 30. 0 turns the
    heartbeat off and leaves the timestamps on.

.PARAMETER CommandTimeoutSeconds
    A bound on the caller's own command, in seconds. No default. On expiry the
    distro is terminated and the exit code is 124, as coreutils' timeout
    reports it.

.PARAMETER Force
    Required for destructive actions when non-interactive. Skips confirmation.

.EXAMPLE
    .\wsl-toolkit.ps1 -Action New -Image alpine:3.22

.EXAMPLE
    .\wsl-toolkit.ps1 -Action New -Image debian:bullseye-slim -Command "ldd --version" -Ephemeral -Force

.EXAMPLE
    .\wsl-toolkit.ps1 -Action Run -Name eph-alpine-3.22-a1b2 -Command "apk add gcc && gcc --version"

.EXAMPLE
    .\wsl-toolkit.ps1 -Action Purge -Force

.NOTES
    EXIT CODES -- New and Run behave the SAME way. They are written here
    together because they drifted apart once and nothing noticed: New warned
    over a failing command and still exited 0, so every green result downstream
    of it meant nothing.

      -Action Run -Command ...             exits with the inner command's code
      -Action New -Command ...             exits with the inner command's code
      -Action New -Command ... -Ephemeral  tears the distro down FIRST, then
                                           exits with the inner command's code
      New with no -Command                 exits 0 when the distro came up
      -CommandTimeoutSeconds N reached     exits 124, as coreutils' timeout does
      any action, script failure           exits 1, with a message naming what

    THE STREAM LOG -- on by default, and -NoTimestamps turns all of it off.

    The failure it exists for: a command prints nothing for twenty minutes and
    a caller reading a pipe cannot tell that from a command that has died. An
    agent waits on a matcher that never fires, a person eventually kills it, and
    the only evidence left is exit 137. Silence has to be a line, or it says
    nothing at all.

    So every line the command produces gets a stamp and a stream tag, and
    -TickSeconds of SILENCE produces a heartbeat carrying how long it has been
    quiet, how much has come out, and what WSL says about the distro. NOTHING IS
    INJECTED INTO THE GUEST to make that work: every figure is one this process
    already holds, so an image with no shell ticks as well as a full userspace.

    A CARRIAGE RETURN ENDS A LINE, which is what makes a curl or apt progress
    meter visible at all: they redraw one line and emit no newline for minutes,
    and a line-oriented reader shows nothing the whole time. An unterminated
    line that has sat still is shown early. Both are marked with a trailing '~'
    on the tag, meaning the line had not ended when it was printed.

    WHAT IT COSTS, and why the off switch is not decoration. Relaying means the
    guest's stdout is a pipe rather than an inherited handle, so an application
    that block-buffers when it is not on a terminal will buffer, and its lines
    then carry the time THIS process received them. Lines are also terminated
    with the host's newline rather than the guest's. -NoTimestamps is byte-exact
    and has none of that.

    GUEST STDOUT GOES TO STDOUT AND GUEST STDERR TO STDERR, tagged. The tick is
    the watcher's line rather than the command's, so it goes to stderr: a caller
    capturing stdout to read a result must not find a heartbeat in it.

    Both actions run the caller's command through ONE function, Invoke-InDistro,
    so there is no second place for the code to be dropped.

    A caller that relied on New never failing now sees the real code. That is a
    deliberate break: see docs/consumers.md.

    COMMAND CHANNEL -- -Command, -CommandFile and -CommandB64 are three ways of
    handing over the same thing: bytes. They are base64'd here, decoded to a
    file inside the distro, and sourced by the login shell. NOTHING is quoted
    for the guest, because quoting does not survive the trip and no caller can
    make it. Measured on this host on 2026-08-27 under BOTH PowerShell hosts:
    a payload handed to `wsl.exe -- /bin/sh -lc` has its $VAR expanded and the
    RESULT re-parsed, even inside POSIX single quotes, which is why
    `echo $PATH` died on the bracket in "Program Files (x86)" and why a
    backtick opened a command substitution. Base64 is [A-Za-z0-9+/=] and
    arrives intact on both hosts and in both busybox ash and dash.

    ONE function builds that skeleton, ConvertTo-DistroScriptCommand, and it
    ASSERTS the result stays inside the measured alphabet. Every payload this
    script sends goes through it: the caller's command, the smoke probe, and
    the script Write-DistroFile sends. Before this, two of those three were
    hand-written inside that alphabet with nothing enforcing it, which is the
    defect that made WSL-12 possible.

    The transport file is written, opened, UNLINKED, and only then sourced from
    its open descriptor. So a run leaves nothing behind inside the distro, and
    a command carrying a credential does not persist as a file.

    ORPHANS -- List reports any rootfs .tar left loose in the base directory
    and Purge removes them, through the same deletion and the same containment
    guard as a distro disk. An interrupted New is what leaves one: the tarball
    is cleaned in a finally, and a hard interrupt does not always run one.

    OCI CONFIG -- -OciEnv carries the image's ENV and WORKDIR into the distro
    as /etc/profile.d/10-oci-env.sh, which a login shell sources. It is OFF by
    default: turning it on changes PATH, and every existing caller depends on
    the shape they have. USER and ENTRYPOINT are deliberately NOT carried; the
    reasons are in New-OciEnvScript.

    PLATFORM -- every pull and every create names linux/ARCH explicitly, where
    ARCH is this host's own, read from the engine. Naming it is not politeness:
    the local image store is keyed by tag and NOT by architecture, so a single
    earlier 'pull --platform linux/riscv64 alpine' repoints the shared
    alpine:latest, and the next unqualified pull is a no-op that exports a
    rootfs nothing in it can execute.

    SPACE -- New checks the target volume before --import and REFUSES rather
    than warning, because running out midway leaves a partial VHDX and a
    registered distro that does not work. The requirement is a floor plus a
    multiple of the rootfs tarball, and the entry that asked for this was
    wrong about the shape of it: measured here, an 8 MiB rootfs costs 76 MiB
    and a 77 MiB one costs 172 MiB, so the FLOOR dominates and "roughly twice
    the rootfs" is not the rule. The numbers are in Assert-EnoughDiskSpace.
    A volume whose free space cannot be read is SAID SO and imported anyway;
    a preflight that skipped is not a preflight that passed.

    Requires : Windows 10 2004+ / Windows 11 with WSL2.
    Optional : podman or docker (only for -Image).
    Tested on: Windows PowerShell 5.1 and PowerShell 7+.
#>
