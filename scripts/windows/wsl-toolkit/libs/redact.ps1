function Split-DelimitedArgument {
    <#
      One list parameter, however a caller was able to spell it.

      ⛔ THE DEFECT THIS EXISTS FOR IS MEASURED AND SILENT. A .ps1 run through
      `pwsh -File`, which is how every consumer runs this one, cannot be given a
      real array and cannot have a parameter repeated:

        -X 5,9        arrives as ONE string, "5,9"
        -X a -X b     is refused: "parameter 'X' is specified more than once"
        -X 5 9        is refused: "a positional parameter cannot be found"

      Measured under PowerShell 7.6.5 and Windows PowerShell 5.1 on 2026-08-30,
      directly and through the launcher, which splats the same argument list.
      ⭐ So a list parameter splits its own value, and an in-process caller
      passing a real array still gets every element: both paths land here.

      ⚠ IT IS NOT SAFE FOR EVERY LIST, which is why it is a helper rather than
      something applied to all of them. It is correct where a comma cannot occur
      inside a value (a number, a column name) and where a documented escape
      exists (a regex writes a literal comma as the class [,]). It is WRONG for
      a value that is arbitrary text, and -ScriptArg is exactly that: a URL
      query string carries commas, so that parameter takes a FILE instead.
    #>
    param([AllowEmptyCollection()][string[]]$Values)
    $out = @()
    foreach ($v in @($Values)) {
        foreach ($piece in ("$v" -split ',')) {
            $t = $piece.Trim()
            if ($t.Length -gt 0) { $out += $t }
        }
    }
    return $out
}

function New-RedactionSet {
    <#
      Compile -Redact into regexes once, ahead of the run.

      COMPILING HERE RATHER THAN PER LINE is not an optimisation, it is where a
      bad pattern is REPORTED. A regex that does not compile fails on the first
      line of guest output otherwise, which is halfway into a run that has
      already created a distro, and the message names a line of somebody else's
      script rather than the pattern the caller passed.

      ⛔ THE LIMIT IS STATED IN THE HELP RATHER THAN IMPLIED. A secret split
      across a carriage-return redraw arrives as two partial lines and matches
      neither, so this reduces accidental disclosure in a log that was going to
      be kept. It is not a control to rely on for a secret that matters, and the
      honest place to keep one of those out of a log is to not print it.
    #>
    param([string[]]$Patterns)
    $out = @()
    foreach ($p in @($Patterns)) {
        if (-not $p) { continue }
        try {
            $out += [regex]::new($p, [Text.RegularExpressions.RegexOptions]::Compiled)
        }
        catch {
            throw "-Redact '$p' is not a regular expression this host can compile: $($_.Exception.Message)"
        }
    }
    return , $out
}

function Invoke-Redaction {
    <#
      Every match replaced by three asterisks, before any sink sees the text.

      ⭐ IT RUNS ONCE, ON THE WAY IN. The rendered line, the file copy and the
      JSON record are all built from the result, so there is no sink that could
      be reached by a path that skipped this. A redaction applied per sink is a
      redaction that will one day be applied to two of three.
    #>
    param(
        [Parameter(Mandatory = $true)][AllowEmptyCollection()][object[]]$Set,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Text
    )
    if ($Set.Count -eq 0) { return $Text }
    $t = $Text
    foreach ($re in $Set) {
        # ⛔ A REPLACEMENT DELEGATE, NEVER A REPLACEMENT STRING. In a .NET
        # replacement string '$&', '$`' and "$'" are expanded, and "$'" means
        # "everything after the match", so a literal replacement carrying one
        # would paste the rest of the line back in. Three asterisks contain none
        # of them today; the delegate is what keeps that true if the marker ever
        # changes. Same class as the row in docs/conventions/forbidden-patterns.md
        # about String.replace in JavaScript.
        $t = $re.Replace($t, [Text.RegularExpressions.MatchEvaluator] { param($m) $null = $m; '***' })
    }
    return $t
}

function Limit-LineBytes {
    <#
      Cut the DETAIL at -MaxLineBytes and say how much went, or return it
      unchanged when the bound is off or not reached.

      ⛔ IT COUNTS BYTES AND CUTS CHARACTERS, and the difference is the defect
      this comment exists for. A UTF-8 character is up to four bytes, so cutting
      a byte array at an arbitrary index splits one and produces a replacement
      character that was never in the guest's output. The loop below adds
      characters until the next one would cross the bound, so the result is
      always a whole number of characters and always inside it.
    #>
    param(
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Text,
        [Parameter(Mandatory = $true)][int]$MaxBytes
    )
    if ($MaxBytes -le 0) { return $Text }
    $enc = [Text.Encoding]::UTF8
    $total = $enc.GetByteCount($Text)
    if ($total -le $MaxBytes) { return $Text }

    $kept = 0
    $take = 0
    foreach ($ch in $Text.ToCharArray()) {
        $w = $enc.GetByteCount([string]$ch)
        if ($kept + $w -gt $MaxBytes) { break }
        $kept += $w
        $take++
    }
    $inv = [Globalization.CultureInfo]::InvariantCulture
    return ($Text.Substring(0, $take) + '...(+' + ($total - $kept).ToString($inv) + ' bytes cut)')
}
