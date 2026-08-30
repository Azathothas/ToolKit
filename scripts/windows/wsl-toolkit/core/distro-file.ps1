# --------------------------------------------------------------------------------------
# Getting bytes into a distro
# --------------------------------------------------------------------------------------
function ConvertTo-ShellSingleQuoted {
    <#
      POSIX single-quoting. The only character a single-quoted string cannot
      contain is a single quote, so it is written by closing, escaping and
      reopening. Used for values that end up INSIDE a file in the guest, never
      for the command that carries them there: see Write-DistroFile for why
      quoting does not survive the trip.
    #>
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Raw)
    return "'" + ($Raw -replace "'", "'\''") + "'"
}

function Write-DistroFile {
    <#
      Put bytes inside a distro without a shell touching them.

      ⭐ THE SCRIPT THIS SENDS IS NOW A PAYLOAD LIKE ANY OTHER. It used to be
      hand-written inside the alphabet that survives wsl.exe:

        mkdir -p DIR && echo B64|base64 -d>PATH && chmod MODE PATH

      which worked, and was a constraint nothing enforced. The next person to
      add a quote to it would have shipped WSL-12 again in a new place. It now
      goes through ConvertTo-DistroScriptCommand like every other payload, so
      the alphabet rule is checked by a machine and the text below is free to
      be quoted properly.

      THE CONTENT STAYS BASE64 inside that payload. Single-quoting it would
      work for text and would need a second escaping rule for anything else;
      base64 needs none and has no failure mode a here-document has.

      THE PATH is single-quoted now that it can be, AND still restricted to an
      absolute path of letters, digits, dot, dash and underscore. ⚠ The
      restriction is no longer load-bearing for the transport; it is kept as a
      sanity guard, because every path this script writes is one it chose, and
      a path arriving with a newline in it is a bug rather than an intention.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$DistroName,
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Content,
        [string]$Mode = '0644'
    )
    if ($Path -notmatch '^/[A-Za-z0-9_.\-]+(/[A-Za-z0-9_.\-]+)*$') {
        throw ("Refusing to write '$Path' inside the distro. Paths written by this script are " +
               "restricted to an absolute path of letters, digits, dot, dash and underscore.")
    }
    if ($Mode -notmatch '^[0-7]{3,4}$') { throw "Mode '$Mode' is not an octal file mode." }

    $b64   = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($Content))
    $slash = $Path.LastIndexOf('/')
    $dir   = if ($slash -gt 0) { $Path.Substring(0, $slash) } else { '/' }
    $qPath = ConvertTo-ShellSingleQuoted -Raw $Path
    $qDir  = ConvertTo-ShellSingleQuoted -Raw $dir

    # Each step reports its own failure. Without the || exit lines a missing
    # base64 leaves an empty file and a chmod that succeeds over it, and the
    # whole thing returns 0 having written nothing.
    $payload = @(
        "mkdir -p $qDir || exit 1",
        "echo $b64 | base64 -d > $qPath || exit 1",
        "chmod $Mode $qPath || exit 1"
    ) -join "`n"

    # ⛔ Get-DistroOutput, NOT Invoke-InDistro, and the difference is the point.
    # This is a question the SCRIPT asks, so it is bounded by -TimeoutSeconds
    # like every other one; Invoke-InDistro is for the CALLER'S command, which
    # is deliberately not. A door sweep found this on the wrong side of that
    # line: a distro that wedged after the smoke probe hung here forever, and
    # -Systemd reaches this before its own bounded check.
    #
    # Capturing rather than streaming also puts the guest's own complaint into
    # the error below instead of leaving it loose on the console above it.
    $rc = 0
    $said = Get-DistroOutput -DistroName $DistroName -RunAs 'root' `
        -ScriptBytes (ConvertTo-Utf8Bytes -Text $payload) -ExitCode ([ref]$rc) `
        -What "the write of $Path"
    if ($rc -ne 0) {
        throw ("Could not write $Path inside '$DistroName' (exit $rc). The likeliest cause is an " +
               "image with no base64: busybox has one and coreutils has one, but a rootfs built " +
               "from scratch may have neither. The guest said: $($said.Trim())")
    }
}

