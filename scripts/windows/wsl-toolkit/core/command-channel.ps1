function ConvertTo-Utf8Bytes {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Text)
    return ,[Text.Encoding]::UTF8.GetBytes($Text)
}

function ConvertFrom-CommandFileBytes {
    <#
      A file's bytes, made into something /bin/sh can read, with every change
      named on the way past.

      ⛔ THE FILE ON DISK IS NEVER WRITTEN TO. The distinction this function
      draws, and it is the whole design, is between THE FILE, which is the
      caller's and is never touched, and THE COPY IN TRANSIT, which is this
      script's to make correct. Rewriting somebody's script is the failure the
      base64 channel exists to remove; refusing to fix a copy of it is not the
      same rule, it is that rule applied where it does not belong.

      WHAT IT DOES, AND WHY EACH ONE IS A DEFECT WITHOUT IT:

        UTF-16          REFUSED by name. Its bytes are NUL-interleaved, so
                        base64 carries them intact to a guest whose /bin/sh
                        reads the first NUL and stops. The failure arrives as a
                        command that did nothing and said nothing.
        UTF-8 BOM       removed. It is three bytes ahead of the first line, so
                        a leading `#!/bin/sh` is not a comment any more and a
                        leading `set -e` is not a command.
        CRLF            turned into LF. /bin/sh reads the carriage return as
                        part of the last word on every line, so every command
                        in the file is not found and no message says why.

      ⚠ -Verbatim TURNS ALL THREE OFF and sends the bytes exactly as they are,
      for a caller who means it. The UTF-16 refusal becomes a warning there,
      because refusing bytes somebody explicitly asked to send verbatim would be
      this function overriding them.
    #>
    param(
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][byte[]]$Bytes,
        [Parameter(Mandatory = $true)][string]$Source
    )
    $utf16 = ($Bytes.Length -ge 2 -and (
        ($Bytes[0] -eq 0xFF -and $Bytes[1] -eq 0xFE) -or ($Bytes[0] -eq 0xFE -and $Bytes[1] -eq 0xFF)))

    if ($Verbatim) {
        if ($utf16) { Write-Warn "$Source starts with a UTF-16 byte order mark and -Verbatim was passed, so it is sent as-is. /bin/sh will stop at the first NUL byte." }
        if ($Bytes -contains 13) { Write-Warn "$Source has carriage returns and -Verbatim was passed, so they are sent as-is. /bin/sh reads a CR as part of the last word on the line." }
        return ,$Bytes
    }

    if ($utf16) {
        throw ("$Source is UTF-16: its bytes carry a NUL after nearly every character, and " +
               "/bin/sh stops at the first one, so the command would do nothing and say nothing. " +
               "Save it as UTF-8, or pass -Verbatim if sending it unchanged is what you meant.")
    }

    $out = $Bytes
    $bom = $false
    if ($out.Length -ge 3 -and $out[0] -eq 0xEF -and $out[1] -eq 0xBB -and $out[2] -eq 0xBF) {
        $out = $out[3..($out.Length - 1)]
        $bom = $true
    }

    # ⛔ CRLF ONLY, never a lone CR. A carriage return that is not followed by a
    # newline is a deliberate one: a progress meter, or a literal in a here-doc
    # the script writes out. Turning that into a newline would edit the payload
    # rather than repair it, which is exactly the line this function does not
    # cross.
    $crlf = 0
    $keep = [byte[]]::new($out.Length)
    $n = 0
    for ($i = 0; $i -lt $out.Length; $i++) {
        if ($out[$i] -eq 13 -and $i + 1 -lt $out.Length -and $out[$i + 1] -eq 10) { $crlf++; continue }
        $keep[$n] = $out[$i]
        $n++
    }
    if ($crlf -gt 0) { $out = $keep[0..($n - 1)] }

    if ($bom)       { Write-Ok "$Source carried a UTF-8 byte order mark; it was left out of the copy being sent." }
    if ($crlf -gt 0) {
        Write-Ok "$Source has $crlf CRLF line ending(s); the copy being sent uses LF."
        Write-Ok "the file on disk was NOT modified. Pass -Verbatim to send its bytes exactly."
    }
    return ,$out
}

function ConvertTo-ScriptArgPrologue {
    <#
      -ScriptArg NAME=VALUE, as POSIX shell assignments ahead of the command.

      ⭐ VALUES ARE ASSIGNED, NEVER SUBSTITUTED INTO THE SCRIPT. The alternative,
      and the thing this replaces, is a caller running sed over their own file
      to inject a URL. A value carrying a slash, an ampersand or a quote breaks
      that edit, and the corruption lands in the middle of a script nobody reads
      again. Nothing in the caller's bytes is rewritten here: text is added in
      front of them, single-quoted, so a value can contain anything at all.

      ⚠ THE FIRST LINE OF THE FILE IS NO LONGER THE FIRST LINE THE SHELL SEES.
      That costs nothing: the body is SOURCED by /bin/sh, so a leading
      `#!/bin/sh` was already a comment rather than an interpreter line.

      @hostaddress IS EXPANDED, and it is the one substitution that happens.
      A caller wiring a guest at a fixture on this host needs the NAT address,
      which is a second command and a value that changes; -Action HostAddress
      already answers it without creating a distro, so this reuses that answer
      rather than making the caller pass it in.
    #>
    param([Parameter(Mandatory = $true)][AllowEmptyCollection()][string[]]$Pairs)
    $lines = @()
    $addr = $null
    foreach ($pair in $Pairs) {
        $eq = "$pair".IndexOf('=')
        if ($eq -lt 1) {
            throw "-ScriptArg '$pair' is not NAME=VALUE. The name comes first, then one equals sign, then the value, which may be empty."
        }
        $name  = $pair.Substring(0, $eq)
        $value = $pair.Substring($eq + 1)
        if ($name -notmatch '^[A-Za-z_][A-Za-z0-9_]*$') {
            throw ("-ScriptArg name '$name' is not a POSIX shell variable name. It must start with " +
                   "a letter or an underscore and carry only letters, digits and underscores.")
        }
        if ($value.Contains('@hostaddress')) {
            if (-not $addr) { $addr = (Resolve-HostAddress).Address }
            $value = $value.Replace('@hostaddress', $addr)
            Write-Ok "-ScriptArg $name : @hostaddress resolved to $addr. WSL assigns it and it changes; it is read here and not recorded."
        }
        $lines += ($name + '=' + (ConvertTo-ShellSingleQuoted -Raw $value))
        $lines += ('export ' + $name)
    }
    if ($lines.Count -eq 0) { return '' }
    return (($lines -join "`n") + "`n")
}

function Get-ScriptArgPairs {
    <#
      Every NAME=VALUE pair for this run, from the file first and then the flag.

      ⭐ THIS EXISTS BECAUSE -ScriptArg CANNOT BE REPEATED. Measured on
      2026-08-30 under both PowerShell hosts: a script run through -File, which
      is how every consumer runs this one, refuses `-ScriptArg A=1 -ScriptArg
      B=2` with "parameter 'ScriptArg' is specified more than once", directly
      and through the launcher, which splats the same argument list. The help
      called it repeatable for a session and it never was.

      ⛔ A FILE RATHER THAN A DELIMITER, and the reason is that there is no safe
      delimiter. A VALUE is arbitrary: a URL with a query string carries commas
      and semicolons, and splitting on one would corrupt the value it was
      injecting, which is the exact failure -ScriptArg was added to remove.

      THE FILE'S BYTES GO THROUGH ConvertFrom-CommandFileBytes, the same repair
      -CommandFile gets, so a file written on Windows works without the caller
      converting anything. ⛔ The file on disk is never written to.
    #>
    param(
        [AllowEmptyString()][string]$FromFile,
        [AllowEmptyCollection()][string[]]$Pairs
    )
    $out = @()
    if ($FromFile) {
        if (-not (Test-Path -LiteralPath $FromFile -PathType Leaf)) {
            throw "-ScriptArgFile not found: $FromFile"
        }
        $raw  = [IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $FromFile).Path)
        $text = [Text.Encoding]::UTF8.GetString((ConvertFrom-CommandFileBytes -Bytes $raw -Source $FromFile))
        $n = 0
        foreach ($line in ($text -split "`n")) {
            $n++
            $t = "$line".Trim()
            if ($t.Length -eq 0) { continue }
            if ($t.StartsWith('#')) { continue }
            if ($t.IndexOf('=') -lt 1) {
                throw "-ScriptArgFile $FromFile line ${n}: '$t' is not NAME=VALUE."
            }
            $out += $t
        }
    }
    foreach ($p in @($Pairs)) { if ($p) { $out += $p } }
    return $out
}

function Resolve-CommandBytes {
    <#
      The three ways of naming a command collapse to one thing here, so every
      action downstream sees bytes and no action has to know which switch the
      caller used.

      $null means NO command was given, which is different from an empty one:
      New with no -Command is a distro that gets created and kept.

      -ScriptArg IS PREPENDED HERE, in this one place, so a value reaches a
      command written as text, as a file and as base64 identically. Three
      spellings that behave differently under a fourth parameter is three
      behaviours.
    #>
    param(
        [AllowEmptyString()][string]$Text,
        [AllowEmptyString()][string]$FromFile,
        [AllowEmptyString()][string]$FromB64,
        [AllowEmptyCollection()][string[]]$Pairs = @()
    )
    $given = @()
    if ($Text)     { $given += '-Command' }
    if ($FromFile) { $given += '-CommandFile' }
    if ($FromB64)  { $given += '-CommandB64' }
    if ($given.Count -gt 1) {
        throw "Pass only one of $($given -join ', '). They are three spellings of the same argument."
    }
    if ($given.Count -eq 0) {
        # ⛔ REFUSED RATHER THAN IGNORED. A caller who passed -ScriptArg and no
        # command has made a mistake this script can see, and a value silently
        # going nowhere is the shape of a flag nothing reads.
        if ($Pairs -and $Pairs.Count -gt 0) {
            throw "-ScriptArg or -ScriptArgFile was given with no command to pass it to. Add -Command, -CommandFile or -CommandB64."
        }
        return $null
    }
    if ($Verbatim -and -not $FromFile) {
        throw "-Verbatim applies to -CommandFile, which is the only spelling whose bytes come off disk. $($given[0]) is already exactly what you passed."
    }

    if ($Text)   { $body = ConvertTo-Utf8Bytes -Text $Text }
    elseif ($FromB64) {
        try { $body = [Convert]::FromBase64String($FromB64) }
        catch { throw "-CommandB64 is not valid base64: $($_.Exception.Message)" }
    }
    else {
        if (-not (Test-Path -LiteralPath $FromFile -PathType Leaf)) {
            throw "-CommandFile not found: $FromFile"
        }
        $raw  = [IO.File]::ReadAllBytes((Resolve-Path -LiteralPath $FromFile).Path)
        $body = ConvertFrom-CommandFileBytes -Bytes $raw -Source $FromFile
    }

    if ($Pairs -and $Pairs.Count -gt 0) {
        # ⛔ Array::Copy, NOT `@($a) + @($b)`. PowerShell's + on two byte arrays
        # UNROLLS them into an [object[]] of mixed byte[] and byte, and the cast
        # back to [byte[]] then fails with a message about System.Byte[] that
        # names neither array. The selftest beside this script is what found it,
        # on the first run, before any distro existed to hit it.
        $pre    = ConvertTo-Utf8Bytes -Text (ConvertTo-ScriptArgPrologue -Pairs $Pairs)
        $merged = [byte[]]::new($pre.Length + $body.Length)
        [Array]::Copy($pre, 0, $merged, 0, $pre.Length)
        [Array]::Copy($body, 0, $merged, $pre.Length, $body.Length)
        $body = $merged
    }
    return ,([byte[]]$body)
}

