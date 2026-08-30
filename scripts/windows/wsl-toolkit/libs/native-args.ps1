function ConvertTo-NativeArgumentString {
    <#
      Joins arguments into the one command-line string ProcessStartInfo takes.
      ⚠ ProcessStartInfo.ArgumentList, which would do this properly, is .NET
      Core only: Windows PowerShell 5.1 runs on .NET Framework and has only the
      string. So the join is done here, and it is SAFE ONLY BECAUSE OF WHAT IT
      REFUSES.

      ⛔ An argument carrying a double quote or a backslash is refused rather
      than escaped. Every argument this script passes is one it built: a distro
      name from ConvertTo-SafeName, a user name, and a transport skeleton
      ConvertTo-DistroScriptCommand has already checked against an alphabet
      with no quote in it. Hand-rolling an escape for a case that cannot occur
      is how a quoting bug gets written and never exercised.
    #>
    param([Parameter(Mandatory = $true)][string[]]$Arguments)
    $parts = @()
    foreach ($a in $Arguments) {
        if ($a -match '["\\]') {
            throw ("Refusing to build a command line containing a quote or a backslash: '$a'. " +
                   "This script passes only arguments it built itself.")
        }
        if ($a -match '\s') { $parts += ('"' + $a + '"') } else { $parts += $a }
    }
    return ($parts -join ' ')
}

