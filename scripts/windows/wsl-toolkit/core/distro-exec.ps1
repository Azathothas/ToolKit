function New-GuestScratchPath {
    <#
      A path for the transport file inside the guest. It is interpolated into
      the skeleton RAW, so it is drawn from the cleared alphabet and nothing
      else, and ConvertTo-DistroScriptCommand re-checks it rather than trusting
      this function to have been the one that produced it.

      It is random per call because two concurrent Run commands against one
      distro must not write each other's file. The window is microseconds wide
      and it costs four characters to close it.
    #>
    $suffix = -join ((48..57) + (97..122) | Get-Random -Count 8 | ForEach-Object { [char]$_ })
    return "/tmp/.wsl-eph-$suffix"
}

function Get-GuestUserEnvironmentPrelude {
    <#
      The shell prologue -UserEnv prepends to a command, as ONE home for both
      of the defects it answers.

      1. ⛔ ROOTLESS PODMAN NEEDS XDG_RUNTIME_DIR AND runuser LEAVES IT UNSET.
         A build then dies at `cannot create state directory for buildah-...`,
         which reads as a broken image and is a missing directory. The reported
         case is in the issue this was written for.
      2. ⚠ A GUEST'S PATH IS NOT THE PATH ITS TOOLS WERE INSTALLED ONTO. A login
         shell in an imported distro carries WSL's PATH, not the image's, so an
         agent that installed something under $HOME/.local/bin does not find it,
         installs a second copy somewhere else, and the next session finds two.

      ⭐ THE DEDUPLICATION IS THE HALF THAT IS EASY TO DROP AND SHOULD NOT BE.
      Without it a nested invocation prepends the same directories again, so a
      PATH grows by a dozen entries every layer and eventually stops fitting in
      whatever reads it. Every entry is put through one function, which is also
      what keeps the inherited PATH's own duplicates out.

      ⛔ It installs nothing, sources no account rc file and creates exactly one
      directory. Preparing an environment is not the same act as provisioning a
      machine, and a command that quietly installed a package would make the
      switch impossible to reason about.
    #>
    # ⚠ THE DIRECTORY ORDER IS THE CONTRACT. A tool a user installed for
    # themselves wins over a system copy of the same name, which is the case an
    # agent hits: it installs into $HOME/.local/bin and then runs the distro's
    # older one. sbin comes after bin so an unprivileged caller reaches the
    # ordinary tool first.
    $dirs = @(
        '$HOME/.local/bin', '$HOME/bin', '$HOME/.cargo/bin', '$HOME/go/bin',
        '$HOME/.bun/bin', '$HOME/.deno/bin', '$HOME/.nix-profile/bin',
        '/nix/var/nix/profiles/default/bin',
        '/usr/local/go/bin', '/usr/local/cargo/bin',
        '/usr/local/bin', '/usr/bin', '/bin',
        '/usr/local/sbin', '/usr/sbin', '/sbin'
    )
    return @(
        '_wtk_uid=$(id -u) 2>/dev/null || { echo "wsl-toolkit: this guest has no id command" >&2; exit 2; }',
        '_wtk_run=/tmp/wsl-toolkit-run-$_wtk_uid',
        # umask rather than a chmod afterwards: a directory created 0755 and
        # narrowed a moment later is world-readable for that moment.
        '(umask 077; mkdir "$_wtk_run") 2>/dev/null || :',
        'if [ -L "$_wtk_run" ]; then echo "wsl-toolkit: $_wtk_run is a symlink and will not be used" >&2; exit 2; fi',
        'if [ ! -d "$_wtk_run" ]; then echo "wsl-toolkit: could not create $_wtk_run" >&2; exit 2; fi',
        # ⛔ THREE OUTCOMES, NOT TWO. A guest with no `stat` and a directory
        # owned by somebody else are different facts, and reporting the first as
        # the second sends a reader after an attacker who is not there.
        'if command -v stat >/dev/null 2>&1; then',
        '    _wtk_own=$(stat -c %u "$_wtk_run" 2>/dev/null) || _wtk_own=',
        '    if [ -z "$_wtk_own" ]; then echo "wsl-toolkit: stat cannot read $_wtk_run" >&2; exit 2; fi',
        '    if [ "$_wtk_own" != "$_wtk_uid" ]; then echo "wsl-toolkit: $_wtk_run belongs to uid $_wtk_own, not $_wtk_uid" >&2; exit 2; fi',
        'fi',
        'chmod 700 "$_wtk_run" || exit 2',
        'XDG_RUNTIME_DIR=$_wtk_run; export XDG_RUNTIME_DIR',
        # A per-uid TMPDIR under the runtime directory, so what a job leaves
        # behind is one directory the host can measure and remove rather than
        # loose files mixed into /tmp with everybody else's.
        '(umask 077; mkdir "$_wtk_run/tmp") 2>/dev/null || :',
        'if [ -d "$_wtk_run/tmp" ] && [ ! -L "$_wtk_run/tmp" ]; then TMPDIR=$_wtk_run/tmp; export TMPDIR; fi',
        '_wtk_path=',
        '_wtk_add() {',
        '    case ":$_wtk_path:" in *":$1:"*) return 0 ;; esac',
        '    [ -d "$1" ] || return 0',
        '    _wtk_path=${_wtk_path:+$_wtk_path:}$1',
        '}',
        ('for _wtk_dir in ' + ($dirs -join ' ') + '; do _wtk_add "$_wtk_dir"; done'),
        # The inherited PATH goes through the same function, which both keeps
        # its entries and removes the copies this prologue has already added.
        '_wtk_old=$PATH',
        'while [ -n "$_wtk_old" ]; do',
        '    case "$_wtk_old" in *:*) _wtk_one=${_wtk_old%%:*}; _wtk_old=${_wtk_old#*:} ;; *) _wtk_one=$_wtk_old; _wtk_old= ;; esac',
        '    [ -n "$_wtk_one" ] && _wtk_add "$_wtk_one"',
        'done',
        'PATH=$_wtk_path; export PATH',
        'unset -f _wtk_add 2>/dev/null || :',
        'unset _wtk_uid _wtk_run _wtk_own _wtk_path _wtk_dir _wtk_old _wtk_one'
    ) -join "`n"
}

function Add-GuestUserEnvironment {
    <#
      The prologue above, then the caller's bytes, unchanged.

      ⛔ THE CALLER'S BYTES ARE THE SUFFIX AND NOTHING IS SUBSTITUTED INTO THEM.
      A prologue that edited the payload would be the sed-over-a-script defect
      -ScriptArg exists to remove, one layer down.
    #>
    param([Parameter(Mandatory = $true)][AllowEmptyCollection()][byte[]]$ScriptBytes)
    $prefixBytes = [Text.Encoding]::UTF8.GetBytes((Get-GuestUserEnvironmentPrelude) + "`n")
    $combined = New-Object byte[] ($prefixBytes.Length + $ScriptBytes.Length)
    [Array]::Copy($prefixBytes, 0, $combined, 0, $prefixBytes.Length)
    [Array]::Copy($ScriptBytes, 0, $combined, $prefixBytes.Length, $ScriptBytes.Length)
    return , $combined
}

function ConvertTo-DistroScriptCommand {
    <#
      THE ONE PLACE THE TRANSPORT SKELETON IS BUILT, and the reason there is
      only one is that a payload hand-written inside the safe alphabet is a
      constraint nothing enforces. That is exactly how WSL-12 shipped: the
      smoke probe carried a bracket inside a double-quoted echo, and every
      -Action New failed on Windows PowerShell 5.1.

      WHAT DOES NOT SURVIVE, measured on this host on 2026-08-27 against real
      Alpine and Debian distros, under BOTH PowerShell 7.6.5 and Windows
      PowerShell 5.1, with every hazard ALREADY correctly single-quoted for sh
      before it was passed:

        $VAR       expanded in transit, and the RESULT is then re-parsed. That
                   is why `echo $PATH` dies: the value carries the bracket in
                   "Program Files (x86)".
        backtick   opens a command substitution. Both hosts.
        "          survives on 7.6.5 and does NOT on 5.1, where it gives
                   "syntax error: unterminated quoted string".

      The single quotes never reach the guest, so a caller cannot fix this by
      quoting harder. WHAT DOES SURVIVE is base64: [A-Za-z0-9+/=], plus the
      operators this skeleton needs. So the payload goes as base64 and every
      character outside it is REFUSED here rather than mangled in transit.

      THE SKELETON, and each link earns its place:

        mkdir -p /tmp        a rootfs exported from a scratch image may have no
                             /tmp at all, and the failure is otherwise a
                             redirect error naming nothing.
        base64 -d>PATH       the decode. && means a guest with no base64 STOPS
                             here instead of sourcing an empty file and
                             reporting success over a command that never ran.
        exec 8>PATH          create it for writing...
        exec 9<PATH          ...open a reader on it...
        rm -f PATH           ...and UNLINK IT BEFORE ANY CONTENT EXISTS. Both
                             descriptors keep the inode alive, so the command's
                             text is never a file anybody can read, and no
                             later failure can leave one behind.
        base64 -d>&8         the decode, written through the descriptor. && is
                             what makes a guest with no base64 STOP here
                             instead of sourcing an empty file and reporting
                             success over a command that never ran.
        . /dev/fd/9          source it from the open descriptor. The login
                             shell runs it, so /etc/profile and -OciEnv still
                             apply, and its exit code becomes the shell's.

      ⚠ THE ORDER OF THOSE FIVE IS THE POINT, and it was got wrong first.
      Writing the file and then unlinking it reads the same and is not: a
      redirect CREATES the file before the decode runs, so a guest with no
      base64 was left holding an empty /tmp file that nothing removed. The
      mutation that planted a missing decoder is what found it.
    #>
    param(
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][byte[]]$ScriptBytes,
        [Parameter(Mandatory = $true)][string]$GuestPath
    )
    if ($GuestPath -notmatch '^/[A-Za-z0-9_.\-]+(/[A-Za-z0-9_.\-]+)*$') {
        throw "Transport path '$GuestPath' is outside the alphabet that survives the trip."
    }
    $b64  = [Convert]::ToBase64String($ScriptBytes)
    $line = "mkdir -p /tmp&&exec 8>$GuestPath&&exec 9<$GuestPath&&rm -f $GuestPath&&echo $b64|base64 -d>&8&&. /dev/fd/9"

    # ⛔ THE CHECK THAT DID NOT EXIST. Every payload now reaches the guest as
    # base64, so nothing a caller writes can break the alphabet; this catches
    # the OTHER direction, an edit to the skeleton above that adds a character
    # the measurement never cleared. Plant a $ in that string and this fires.
    $bad = [regex]::Match($line, '[^A-Za-z0-9+/=|<>&;. _-]')
    if ($bad.Success) {
        throw ("Transport skeleton carries '$($bad.Value)', which is outside the alphabet measured " +
               "to survive PowerShell to wsl.exe to /bin/sh. Re-measure before widening it.")
    }
    return $line
}

function Invoke-InDistro {
    <#
      The ONE path that runs a caller's command inside a distro. New and Run both
      go through it, so an inner exit code cannot be propagated by one action and
      dropped by the other. It was dropped by New, which is what made -Command
      useless as a gate.

      It takes BYTES rather than a string because -CommandFile is read verbatim
      from disk: a file that is not UTF-8 would otherwise be re-encoded on its
      way through a parameter typed as text, which is a mangling of exactly the
      kind this whole entry exists to remove.

      The code comes back through -ExitCode rather than as the return value, on
      purpose. The command's own stdout flows out of this function's success
      stream so the caller can see it, and `$rc = Invoke-InDistro ...` would
      therefore capture that OUTPUT into $rc instead of the code.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$DistroName,
        [Parameter(Mandatory = $true)][string]$RunAs,
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][byte[]]$ScriptBytes,
        [Parameter(Mandatory = $true)][ref]$ExitCode
    )
    # Set before the try, and set non-zero. Under Set-StrictMode -Version Latest
    # an unassigned variable throws when it is READ, so every path out of here
    # has to leave a code behind; and "it never answered" is a failure, not a
    # pass, so the value it starts at has to be one that fails.
    $ExitCode.Value = 1

    # ⭐ ONE BRANCH, AND IT IS THE ONLY PLACE THE STREAM LOG IS TURNED ON OR
    # OFF. New and Run both arrive here, so the log cannot be on for one action
    # and off for the other, which is the shape WSL-01 was.
    #
    # ⛔ IT READS $script:RelayOff AND NOT $NoTimestamps, and the difference is a
    # defect this shipped for one build. There are TWO spellings of "no relay",
    # -NoTimestamps and -TimestampProfile raw, and Main folds them into one
    # variable. Branching on the parameter here read only the first, so
    # -TimestampProfile raw took the relay path while Main, having taken the
    # other, had never built $script:LogSettings: under Set-StrictMode the next
    # line died on a variable that was never set. Found by the door sweep, on
    # the question "what other door reaches this code".
    if (-not $script:RelayOff) {
        Invoke-InDistroLogged -DistroName $DistroName -RunAs $RunAs -ScriptBytes $ScriptBytes -ExitCode $ExitCode
        return
    }

    # -NoTimestamps: the child inherits this process's handles, so the guest's
    # bytes reach the terminal without passing through this script at all.
    # Nothing here can re-encode, re-order or buffer them, which is the point of
    # having the switch rather than a relay that promises not to.
    $line = ConvertTo-DistroScriptCommand -ScriptBytes $ScriptBytes -GuestPath (New-GuestScratchPath)
    $prev = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        & (Get-WslExe) -d $DistroName -u $RunAs -- /bin/sh -lc $line
        if ($null -ne $LASTEXITCODE) { $ExitCode.Value = [int]$LASTEXITCODE }
    }
    finally { $ErrorActionPreference = $prev }
}

