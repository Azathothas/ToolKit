# herdr-remote-probe.ps1 - drive `herdr --remote` in a Windows pseudo console.
#
# The defect this exists to catch is a herdr client that connects and then does
# nothing a person would call working. herdr 0.9.0's Windows --remote client draws
# nothing until the window is focused, runs no typed text and no prefix command;
# a build of herdr's development branch does all three. Neither fact is visible to
# `go test`, to the suite, or to `--version`, because both builds answer
# `herdr 0.9.0`. WSL-90, and herdrdev/herdr#4176 and #4038.
#
# WHAT IT REQUIRES: a Windows host, a running wsl-toolkit base whose herdr server
# answers, and a herdr client binary to drive. `wsl-toolkit --instance NAME base
# attach` prints the client matching the server the base runs.
#
# HARD RULE: IT TYPES ONLY INTO A WORKSPACE IT MADE. The probe creates its own
# herdr workspace, sends every keystroke there, reads every result back from the
# SERVER rather than off the screen, and closes that workspace at the end. The
# operator's own workspaces are never focused, typed into or closed.
#
# HARD RULE: THE SIGNAL COUNT IS ASSERTED. A table that stopped early exits 0 over
# a smaller suite, which is the shape a check takes on its way to reporting nothing.
#
# WHAT IT CANNOT MEASURE: a real focus event. A pseudo console has no window, so
# signal 6 is reported `operator` and never `pass`. It is the one thing in this
# file that needs a person at a real Windows Terminal.
#
# Usage:
#   pwsh -NoProfile -File tools/windows/wsl-toolkit/herdr-remote-probe.ps1 -Herdr PATH -Label NAME
#   pwsh -NoProfile -File tools/windows/wsl-toolkit/herdr-remote-probe.ps1 -Herdr PATH -Label NAME -Json
#
# Exit codes: 0 every measurable signal passed, 1 one did not, 2 could not run.
# Read the exit code from this process, unpiped.
#
# ASCII-ONLY ON PURPOSE. docs/conventions/shell.md section 8: a .ps1 holding any
# non-ASCII byte needs a UTF-8 BOM before Windows PowerShell 5.1 decodes it as
# UTF-8, and this file is read by pwsh 7 and by a reader with neither.

#Requires -Version 7.0
[CmdletBinding(PositionalBinding = $false)]
param(
    [Parameter(Mandatory = $true)][string]$Herdr,
    [Parameter(Mandatory = $true)][string]$Label,
    [string]$Binary = '',
    [string]$Instance = 'base',
    [string]$Distribution = 'wsl-toolkit-base',
    [string]$OutDir = '',
    [switch]$Json
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Exit-Cannot {
    param([string]$Why)
    # NOT Write-Error. This file sets $ErrorActionPreference = 'Stop', under which
    # Write-Error throws a terminating error, the `exit 2` below is never reached,
    # and pwsh ends with 1. That made "could not run" indistinguishable from "a
    # signal failed" - the exact defect this probe exists to refuse in herdr.
    [Console]::Error.WriteLine("cannot run: $Why")
    exit 2
}

if ($Label -notmatch '^[A-Za-z0-9][A-Za-z0-9._-]*$') {
    Exit-Cannot "the label '$Label' is not a name: letters, digits, dot, dash and underscore"
}
if (-not (Test-Path -LiteralPath $Herdr -PathType Leaf)) {
    Exit-Cannot "no herdr client at $Herdr"
}

# The repository root is three directories above this file, and every default path
# below is built from it. No path in this file names anybody's home.
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..' '..' '..')).Path
if (-not $Binary) { $Binary = Join-Path $repoRoot '.tmp' 'wsl-toolkit.exe' }
if (-not $OutDir) { $OutDir = Join-Path $repoRoot '.tmp' 'herdr-remote-probe' }
if (-not (Test-Path -LiteralPath $Binary -PathType Leaf)) {
    Exit-Cannot "no wsl-toolkit executable at $Binary; build it first"
}

$source = @'
using System;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using Microsoft.Win32.SafeHandles;

public sealed class HerdrProbePty : IDisposable {
    [StructLayout(LayoutKind.Sequential)] struct COORD { public short X; public short Y; }
    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    struct STARTUPINFO { public int cb; public string lpReserved; public string lpDesktop; public string lpTitle;
        public int dwX, dwY, dwXSize, dwYSize, dwXCountChars, dwYCountChars, dwFillAttribute, dwFlags;
        public short wShowWindow, cbReserved2; public IntPtr lpReserved2, hStdInput, hStdOutput, hStdError; }
    [StructLayout(LayoutKind.Sequential)] struct STARTUPINFOEX { public STARTUPINFO StartupInfo; public IntPtr lpAttributeList; }
    [StructLayout(LayoutKind.Sequential)] struct PROCESS_INFORMATION { public IntPtr hProcess, hThread; public int dwProcessId, dwThreadId; }

    [DllImport("kernel32.dll", SetLastError = true)] static extern int CreatePseudoConsole(COORD size, SafeFileHandle hInput, SafeFileHandle hOutput, uint flags, out IntPtr hPC);
    [DllImport("kernel32.dll", SetLastError = true)] static extern int ResizePseudoConsole(IntPtr hPC, COORD size);
    [DllImport("kernel32.dll")] static extern void ClosePseudoConsole(IntPtr hPC);
    [DllImport("kernel32.dll", SetLastError = true)] static extern bool CreatePipe(out SafeFileHandle r, out SafeFileHandle w, IntPtr sa, int size);
    [DllImport("kernel32.dll", SetLastError = true)] static extern bool InitializeProcThreadAttributeList(IntPtr list, int count, int flags, ref IntPtr size);
    [DllImport("kernel32.dll", SetLastError = true)] static extern bool UpdateProcThreadAttribute(IntPtr list, uint flags, IntPtr attr, IntPtr value, IntPtr cb, IntPtr prev, IntPtr ret);
    [DllImport("kernel32.dll")] static extern void DeleteProcThreadAttributeList(IntPtr list);
    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    static extern bool CreateProcessW(string app, StringBuilder cmd, IntPtr pa, IntPtr ta, bool inherit, uint flags, IntPtr env, string cwd, ref STARTUPINFOEX si, out PROCESS_INFORMATION pi);
    [DllImport("kernel32.dll")] static extern uint WaitForSingleObject(IntPtr h, uint ms);
    [DllImport("kernel32.dll")] static extern bool GetExitCodeProcess(IntPtr h, out uint code);
    [DllImport("kernel32.dll")] static extern bool TerminateProcess(IntPtr h, uint code);
    [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr h);

    IntPtr hPC, attrList, hProcess, hThread;
    FileStream input, output;
    readonly MemoryStream seen = new MemoryStream();
    readonly object gate = new object();
    Thread reader;
    public DateTime Started;
    public DateTime FirstByte = DateTime.MinValue;

    public HerdrProbePty(string commandLine, short cols, short rows, string cwd) {
        SafeFileHandle inRead, inWrite, outRead, outWrite;
        if (!CreatePipe(out inRead, out inWrite, IntPtr.Zero, 0)) throw new InvalidOperationException("CreatePipe in " + Marshal.GetLastWin32Error());
        if (!CreatePipe(out outRead, out outWrite, IntPtr.Zero, 0)) throw new InvalidOperationException("CreatePipe out " + Marshal.GetLastWin32Error());
        int hr = CreatePseudoConsole(new COORD { X = cols, Y = rows }, inRead, outWrite, 0, out hPC);
        if (hr != 0) throw new InvalidOperationException("CreatePseudoConsole hr=0x" + hr.ToString("X8"));
        inRead.Dispose(); outWrite.Dispose();
        IntPtr size = IntPtr.Zero;
        InitializeProcThreadAttributeList(IntPtr.Zero, 1, 0, ref size);
        attrList = Marshal.AllocHGlobal(size);
        if (!InitializeProcThreadAttributeList(attrList, 1, 0, ref size)) throw new InvalidOperationException("InitializeProcThreadAttributeList " + Marshal.GetLastWin32Error());
        if (!UpdateProcThreadAttribute(attrList, 0, (IntPtr)0x00020016, hPC, (IntPtr)IntPtr.Size, IntPtr.Zero, IntPtr.Zero)) throw new InvalidOperationException("UpdateProcThreadAttribute " + Marshal.GetLastWin32Error());
        var si = new STARTUPINFOEX();
        si.StartupInfo.cb = Marshal.SizeOf(typeof(STARTUPINFOEX));
        si.StartupInfo.dwFlags = 0x00000100;
        si.lpAttributeList = attrList;
        PROCESS_INFORMATION pi;
        if (!CreateProcessW(null, new StringBuilder(commandLine), IntPtr.Zero, IntPtr.Zero, false, 0x00080000, IntPtr.Zero, cwd, ref si, out pi))
            throw new InvalidOperationException("CreateProcessW " + Marshal.GetLastWin32Error());
        hProcess = pi.hProcess; hThread = pi.hThread;
        Started = DateTime.UtcNow;
        input = new FileStream(inWrite, FileAccess.Write, 1);
        output = new FileStream(outRead, FileAccess.Read, 1);
        reader = new Thread(Pump) { IsBackground = true };
        reader.Start();
    }

    void Pump() {
        var buf = new byte[8192];
        try {
            int n;
            while ((n = output.Read(buf, 0, buf.Length)) > 0) {
                lock (gate) {
                    if (FirstByte == DateTime.MinValue) FirstByte = DateTime.UtcNow;
                    seen.Write(buf, 0, n);
                }
            }
        } catch (Exception) { }
    }

    public long Bytes { get { lock (gate) return seen.Length; } }
    public byte[] Snapshot() { lock (gate) return seen.ToArray(); }

    public void Send(string text) {
        var b = Encoding.UTF8.GetBytes(text);
        input.Write(b, 0, b.Length);
        input.Flush();
    }

    // A real window resize reaches the child as a console size change, which is what
    // ResizePseudoConsole delivers. It is the one signal a pseudo console can stage
    // honestly, because the child reads it through the same path either way.
    public void Resize(short cols, short rows) {
        int hr = ResizePseudoConsole(hPC, new COORD { X = cols, Y = rows });
        if (hr != 0) throw new InvalidOperationException("ResizePseudoConsole hr=0x" + hr.ToString("X8"));
    }

    public bool WaitExit(uint ms, out uint code) {
        code = 0;
        if (WaitForSingleObject(hProcess, ms) != 0) return false;
        GetExitCodeProcess(hProcess, out code);
        return true;
    }

    public void Kill() { TerminateProcess(hProcess, 137); }

    public void Dispose() {
        try { input.Dispose(); } catch (Exception) { }
        if (hPC != IntPtr.Zero) { ClosePseudoConsole(hPC); hPC = IntPtr.Zero; }
        try { output.Dispose(); } catch (Exception) { }
        if (attrList != IntPtr.Zero) { DeleteProcThreadAttributeList(attrList); Marshal.FreeHGlobal(attrList); attrList = IntPtr.Zero; }
        if (hThread != IntPtr.Zero) { CloseHandle(hThread); hThread = IntPtr.Zero; }
        if (hProcess != IntPtr.Zero) { CloseHandle(hProcess); hProcess = IntPtr.Zero; }
    }
}
'@
if (-not ('HerdrProbePty' -as [type])) { Add-Type -TypeDefinition $source -Language CSharp }

# -- the server side, which is where every result is read from -----------------

function Invoke-Herdr {
    param([string[]]$HerdrArgs)
    $out = & $Binary --instance $Instance base herdr -- @HerdrArgs 2>$null
    return [pscustomobject]@{ Exit = $LASTEXITCODE; Text = (@($out) -join "`n") }
}

function Get-HerdrJson {
    param([string[]]$HerdrArgs)
    $r = Invoke-Herdr -HerdrArgs $HerdrArgs
    if ($r.Exit -ne 0) { throw "herdr $($HerdrArgs -join ' ') exited $($r.Exit)" }
    # The tool prints an instance banner before the JSON, so the object is the
    # first line that parses rather than the whole stream.
    foreach ($line in ($r.Text -split "`n")) {
        $t = $line.Trim()
        if ($t.StartsWith('{')) { return $t | ConvertFrom-Json }
    }
    throw "herdr $($HerdrArgs -join ' ') printed no JSON"
}

$signals = [System.Collections.Generic.List[object]]::new()
function Add-Signal {
    param([int]$N, [string]$Name, [string]$Verdict, [string]$Detail)
    $signals.Add([ordered]@{ n = $N; signal = $Name; verdict = $Verdict; detail = $Detail })
}

New-Item -ItemType Directory -Force $OutDir | Out-Null
$stamp = [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ')
$result = [ordered]@{
    label     = $Label
    herdr     = $Herdr
    started   = $stamp
    instance  = $Instance
}

try { $result.version = (& $Herdr --version 2>&1 | Out-String).Trim() }
catch { Exit-Cannot "the client at $Herdr does not answer --version" }

# The probe's own workspace. Everything typed below goes here and nowhere else.
# The id is at result.workspace.workspace_id; result.root_pane names the pane the
# keystrokes land in, which is what the typed-text signal reads back.
#
# ANY EXIT AFTER THE CREATE CLOSES IT. Measured on 2026-09-16: an early version read
# the id from the wrong property, called Exit-Cannot, and left the workspace it had
# just made running on the operator's server - the probe's own "types only into a
# workspace it made" rule broken on its own could-not-run path, where no `finally`
# had been entered yet. Exit-Created is the only exit between here and the try below.
$ws = $null
$rootPane = $null

function Exit-Created {
    param([string]$Why)
    if ($ws) {
        $null = & $Binary --instance $Instance base herdr -- workspace close $ws --group 2>$null
        [Console]::Error.WriteLine("closed the probe's workspace $ws")
    }
    Exit-Cannot $Why
}

function Read-Field {
    # StrictMode makes a missing property THROW, which is how the leak above
    # happened: the throw jumped past the assignment, so the id this probe needed
    # in order to clean up was the very thing it had failed to read.
    param($Object, [string]$Outer, [string]$Inner)
    if ($null -eq $Object) { return $null }
    if ($Object.PSObject.Properties.Name -notcontains $Outer) { return $null }
    $o = $Object.$Outer
    if ($null -eq $o -or $o.PSObject.Properties.Name -notcontains $Inner) { return $null }
    return $o.$Inner
}

$created = $null
try { $created = (Get-HerdrJson -HerdrArgs @('workspace', 'create', '--label', "probe-$Label", '--focus')).result }
catch { Exit-Cannot "could not create a workspace on the base's herdr server: $_" }

$ws = Read-Field $created 'workspace' 'workspace_id'
$rootPane = Read-Field $created 'root_pane' 'pane_id'
if (-not $ws) {
    Exit-Cannot ("the server answered workspace create with no workspace id. A workspace MAY have been made: " +
        "list with `"$Binary --instance $Instance base herdr -- workspace list`" and close any labelled probe-$Label")
}
if (-not $rootPane) { Exit-Created "the server created workspace $ws with no root pane" }
$result.workspace = $ws
$result.root_pane = $rootPane

# The client's own configuration decides onboarding, and a first-run overlay
# swallows every key. A file of the probe's own keeps the operator's untouched.
$clientConfig = Join-Path $OutDir "client-$Label.toml"
Set-Content -LiteralPath $clientConfig -Value 'onboarding = false' -Encoding ascii
$env:HERDR_CONFIG_PATH = $clientConfig
$result.client_config = $clientConfig

$child = $null
$failed = 0
try {
    $tabsBefore = @((Get-HerdrJson -HerdrArgs @('tab', 'list')).result.tabs).Count
    $cmd = '"' + $Herdr + '" --remote ' + $Distribution + ' --remote-keybindings server'
    $child = [HerdrProbePty]::new($cmd, 140, 42, $OutDir)

    # 1 and 2 are measured together, because BYTES ALONE DO NOT SEPARATE THEM.
    # Measured on 2026-09-16: a 0.9.0 client wrote 331 bytes cold and 3,813 after a
    # focus-in, and a nightly client wrote 4,925 cold and 5,002 after. So "drew
    # something" passes BOTH, and the first version of this probe did exactly that -
    # a signal whose name claimed more than it checked. What separates them is what
    # SHARE of the paint arrived before any focus: 98 per cent against 9.
    Start-Sleep -Seconds 6
    $drewCold = $child.Bytes
    $firstMs = if ($child.FirstByte -eq [DateTime]::MinValue) { $null } else { [int]($child.FirstByte - $child.Started).TotalMilliseconds }
    $result.bytes_before_any_input = $drewCold

    # A focus-in reaches a program that enabled focus reporting as CSI I.
    $child.Send([string][char]27 + '[I')
    Start-Sleep -Seconds 3
    $afterFocus = $child.Bytes
    $result.first_byte_ms = $firstMs
    $result.bytes_after_focus_in = $afterFocus
    $share = if ($afterFocus -gt 0) { [math]::Round(100.0 * $drewCold / $afterFocus, 1) } else { 0.0 }
    $result.cold_share_percent = $share

    if ($drewCold -gt 0 -and $share -ge 50.0) {
        Add-Signal 1 'repaint' 'pass' "$drewCold bytes in $firstMs ms with no input, $share pct of the $afterFocus drawn by focus-in"
    }
    elseif ($drewCold -gt 0) {
        Add-Signal 1 'repaint' 'fail' "only $share pct of the paint arrived before focus: $drewCold bytes cold against $afterFocus after focus-in"
        $failed++
    }
    else {
        Add-Signal 1 'repaint' 'fail' 'nothing drawn in 6 s with no input'
        $failed++
    }
    Add-Signal 2 'focus-in (CSI I)' 'info' "$drewCold bytes before, $afterFocus after, cold share $share pct"

    # 3. typed text. The marker cannot be read off the typed line: the shell
    #    computes it, so a client that echoes without running cannot pass.
    $n = Get-Random -Minimum 1000 -Maximum 9999
    $want = '{0}probe{1}' -f ($n * 7), $Label
    $child.Send(('echo $(( {0} * 7 ))probe{1}' -f $n, $Label) + "`r")
    Start-Sleep -Seconds 5
    $ran = $false
    $read = Invoke-Herdr -HerdrArgs @('pane', 'read', $rootPane, '--source', 'recent', '--lines', '80')
    if ($read.Exit -eq 0 -and $read.Text -match [regex]::Escape($want)) {
        $ran = $true
        $result.typed_in_pane = $rootPane
    }
    $result.bytes_after_typing = $child.Bytes
    if ($ran) { Add-Signal 3 'typed text' 'pass' "the pane printed $want" }
    else { Add-Signal 3 'typed text' 'fail' "no pane of $ws printed $want"; $failed++ }

    # 4. a prefix command: ctrl+b then c, then Enter for the default tab name.
    $child.Send([string][char]2)
    Start-Sleep -Milliseconds 400
    $child.Send('c')
    Start-Sleep -Seconds 2
    $child.Send("`r")
    Start-Sleep -Seconds 4
    $tabsAfter = @((Get-HerdrJson -HerdrArgs @('tab', 'list')).result.tabs).Count
    $result.tabs_before = $tabsBefore
    $result.tabs_after_prefix_c = $tabsAfter
    if ($tabsAfter -gt $tabsBefore) { Add-Signal 4 'prefix command' 'pass' "tabs $tabsBefore to $tabsAfter" }
    else { Add-Signal 4 'prefix command' 'fail' "tabs stayed at $tabsBefore"; $failed++ }

    # 5. resize. The server is asked what it thinks the pane is, not the screen.
    $beforeResize = $child.Bytes
    $child.Resize(100, 30)
    Start-Sleep -Seconds 3
    $afterResize = $child.Bytes
    $result.bytes_after_resize = $afterResize
    if ($afterResize -gt $beforeResize) { Add-Signal 5 'resize' 'pass' "$beforeResize to $afterResize bytes after 140x42 to 100x30" }
    else { Add-Signal 5 'resize' 'fail' "nothing redrawn after 140x42 to 100x30"; $failed++ }

    # 6. detach: ctrl+b then q, and the process must END, leaving panes running.
    $child.Send([string][char]2)
    Start-Sleep -Milliseconds 400
    $child.Send('q')
    $code = [uint32]0
    $exited = $child.WaitExit(15000, [ref]$code)
    $result.detach_exit = if ($exited) { [int]$code } else { $null }
    if ($exited -and $code -eq 0) { Add-Signal 6 'detach' 'pass' 'exited 0' }
    elseif ($exited) { Add-Signal 6 'detach' 'fail' "exited $code"; $failed++ }
    else { Add-Signal 6 'detach' 'fail' 'still running after 15 s'; $child.Kill(); $result.killed = $true; $failed++ }

    # 7. the one a pseudo console cannot stage.
    Add-Signal 7 'real window focus' 'operator' 'a pseudo console has no window; needs a real Windows Terminal'
}
finally {
    if ($child) {
        $bytes = $child.Snapshot()
        [IO.File]::WriteAllBytes((Join-Path $OutDir "$Label-$stamp.vt"), $bytes)
        $result.bytes_total = $bytes.Length
        $child.Dispose()
    }
    # The probe's workspace goes, whatever happened above. The operator's do not.
    if ($ws) {
        $closed = Invoke-Herdr -HerdrArgs @('workspace', 'close', $ws, '--group')
        $result.workspace_closed = ($closed.Exit -eq 0)
    }
}

# THE COUNT IS ASSERTED. Seven signals, or this probe reported on a smaller suite
# than it claims and its exit code means nothing.
$expected = 7
if ($signals.Count -ne $expected) {
    Write-Error "the probe recorded $($signals.Count) signals and must record $expected"
    exit 2
}
$result.signals = $signals
$result.measurable = ($expected - 1)
$result.failed = $failed

$path = Join-Path $OutDir "$Label-$stamp.json"
$result | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $path -Encoding ascii
if ($Json) {
    $result | ConvertTo-Json -Depth 6
}
else {
    Write-Output "herdr --remote probe: $Label"
    Write-Output "  client   $Herdr"
    Write-Output "  version  $($result.version)"
    Write-Output "  wrote    $path"
    foreach ($s in $signals) {
        Write-Output ("  {0}  {1,-18} {2}" -f $s.verdict.PadRight(8), $s.signal, $s.detail)
    }
    Write-Output "  $($expected - 1) measurable, $failed failed"
}
if ($failed -gt 0) { exit 1 }
exit 0
