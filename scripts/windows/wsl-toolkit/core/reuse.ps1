function Get-OriginPath {
    <# Where a distro records the image it was built from. #>
    param([Parameter(Mandatory = $true)][string]$DistroName)
    return (Join-Path (Join-Path $script:BaseDir $DistroName) 'origin.json')
}

function Write-DistroOrigin {
    <#
      Record what a distro was made from, beside its disk.

      ⛔ A FILE, NOT THE NAME. The generated distro name carries a sanitised
      fragment of the image reference, and reading the image back out of it
      would be a value re-parsed out of a mutable name, which
      docs/conventions/code.md names as the wrong answer: a stored thing's
      identity is a stable opaque token, never something recovered from a label
      somebody can change. `alpine:3.22` and `alpine:3.21` sanitise to names that
      differ by one character, and `-Name` lets a caller pick a name with no
      relation to the image at all.

      ⛔ VERSIONED AND SELF-DESCRIBING. A positional record that changes shape
      mis-reads silently, and this one decides whether a caller's command runs in
      a distribution built from a different image.
    #>
    param(
        [Parameter(Mandatory = $true)][string]$DistroName,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$ImageRef
    )
    if (-not $ImageRef) { return }
    $path = Get-OriginPath -DistroName $DistroName
    Assert-InsideBaseDir -Path $path
    $inv = [Globalization.CultureInfo]::InvariantCulture
    $body = [ordered]@{
        schema  = 'wsl-toolkit-origin/1'
        image   = $ImageRef
        created = [DateTimeOffset]::Now.ToString('o', $inv)
    } | ConvertTo-Json -Depth 4
    # ⚠ Written atomically, as a sibling then renamed. A killed write otherwise
    # leaves a truncated JSON that -Reuse would refuse to parse, which reads as
    # a broken tool rather than as an interrupted run.
    $temp = $path + '.' + [Guid]::NewGuid().ToString('N') + '.tmp'
    try {
        [IO.File]::WriteAllText($temp, $body, [Text.UTF8Encoding]::new($false))
        Move-Item -LiteralPath $temp -Destination $path -Force
    }
    finally {
        if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Force -ErrorAction SilentlyContinue }
    }
}

function Read-DistroOrigin {
    <#
      What one distro was built from, or $null.

      ⚠ AN UNREADABLE OR UNKNOWN RECORD IS $null AND NEVER A GUESS. A distro
      created before this file existed has none, and treating "no record" as
      "matches whatever you asked for" would run a caller's command in a
      distribution built from something else.
    #>
    param([Parameter(Mandatory = $true)][string]$DistroName)
    $path = Get-OriginPath -DistroName $DistroName
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { return $null }
    try { $rec = [IO.File]::ReadAllText($path) | ConvertFrom-Json }
    catch { $null = $_; return $null }
    if (-not $rec.PSObject.Properties['schema'] -or $rec.schema -ne 'wsl-toolkit-origin/1') { return $null }
    if (-not $rec.PSObject.Properties['image'] -or -not $rec.image) { return $null }
    return $rec
}

function Find-ReusableDistro {
    <#
      A registered ephemeral distro built from this exact image reference, or
      $null.

      ⛔ AN EXACT MATCH ON THE REFERENCE, not a resolved digest and not a tag
      prefix. `alpine:latest` yesterday and `alpine:latest` today can be two
      different images, and this cannot tell them apart; what it promises is
      that the caller ASKED for the same thing, which is the claim the record
      actually supports. A caller who needs the image itself re-pulled does not
      pass -Reuse.

      ⚠ THE NEWEST ONE, so a caller who has several gets the one their last run
      prepared rather than an arbitrary member of the set.
    #>
    param([Parameter(Mandatory = $true)][string]$ImageRef)
    if (-not $script:BaseDir -or -not (Test-Path -LiteralPath $script:BaseDir)) { return $null }
    $registered = @(Get-WslDistroNames)
    $best = $null
    foreach ($name in $registered) {
        if (-not $name.StartsWith($script:Prefix, [StringComparison]::Ordinal)) { continue }
        $rec = Read-DistroOrigin -DistroName $name
        if ($null -eq $rec) { continue }
        if ([string]$rec.image -cne $ImageRef) { continue }
        $when = [DateTimeOffset]::MinValue
        try { $when = [DateTimeOffset]::Parse([string]$rec.created, [Globalization.CultureInfo]::InvariantCulture) }
        catch { $null = $_ }
        if ($null -eq $best -or $when -gt $best.When) {
            $best = [pscustomobject]@{ Name = $name; When = $when; Image = [string]$rec.image }
        }
    }
    return $best
}

function Format-DistroAge {
    <# How long ago a distro was prepared, for the line -Reuse must print. #>
    param([Parameter(Mandatory = $true)]$Found)
    if ($Found.When -eq [DateTimeOffset]::MinValue) { return 'age unknown' }
    return (Format-Duration -Span ([DateTimeOffset]::Now - $Found.When)) + ' old'
}
