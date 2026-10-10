#Requires -Version 7.5
<#
.SYNOPSIS
    The suite for cc_profile.ps1. Every case drives the script as its own process.

.DESCRIPTION
    WHAT GOES WRONG WITHOUT IT. cc_profile edits a file a person also edits by
    hand, and starts an app whose isolation cannot be seen from outside. A guard
    that stopped refusing, or a launch that passes the caller's environment on,
    makes no command fail.

    Each case runs the script through pwsh -File against its own config under
    .tmp/cc_profile-tests in this checkout, never ~/.cc_profiles.json, and reads
    the exit code before the output. A unit case dot-sources the script in a
    child process and stands in for the functions that would start or find the
    app, so the launch logic runs without launching anything.

    -Live adds the cases that start the real Claude desktop app with a throwaway
    profile. They kill the process that launched it, both with taskkill /T and by
    closing a kill-on-close job, close a pseudo-console terminal around it, and
    assert what survives. Then they end the app and assert that nothing restarts it.

    -Mutate plants each guard's defect into a copy of the script, and requires the
    case named for that guard to fail for the reason the row names, after the same
    case passed unmutated.

.EXAMPLE
    pwsh -NoProfile -File scripts/cc_profile/tests/cc_profile.tests.ps1

.EXAMPLE
    pwsh -NoProfile -File scripts/cc_profile/tests/cc_profile.tests.ps1 -Live -Mutate

.NOTES
    Exit codes: 0 every case passed and every mutation run was caught, 1 something
    failed, 2 it could not run. The last line is a fixed summary:
    cases=N passed=N failed=N skipped=N mutations=N proved=N theatre=N misfired=N broken=N unrun=N
    A row is unrun when its case did not pass unmutated in this run, which is how a
    live row reads in a run without -Live.

    SPDX-License-Identifier: 0BSD
#>
[CmdletBinding(PositionalBinding = $false)]
param(
    [string]$Script,
    [string[]]$Case,
    [switch]$Live,
    [switch]$Mutate,
    [switch]$Json,
    [switch]$Keep
)

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

function Write-Say([string]$Text, [string]$Color) {
    if ($Json) { [Console]::Error.WriteLine($Text); return }
    $msg = [Management.Automation.HostInformationMessage]@{ Message = $Text; NoNewLine = $false }
    if ($Color) { $msg.ForegroundColor = [ConsoleColor]$Color }
    Write-Information -MessageData $msg -InformationAction Continue
}

if (-not $IsWindows) {
    Write-Say 'cc_profile and its suite run on Windows only.' 'Yellow'
    exit 2
}

$script:RepoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\..'))
$script:OrigTool = if ($Script) { [IO.Path]::GetFullPath($Script) } else { [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\cc_profile.ps1')) }
if (-not [IO.File]::Exists($script:OrigTool)) { Write-Say "No script at $($script:OrigTool)." 'Red'; exit 2 }
$script:ToolPath = $script:OrigTool
$script:Pwsh = (Get-Process -Id $PID).Path
$script:TestsRoot = Join-Path $script:RepoRoot '.tmp\cc_profile-tests'
$script:RunRoot = Join-Path $script:TestsRoot ('{0}-{1}' -f [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ'), [guid]::NewGuid().ToString('N').Substring(0, 6))
$script:Ctx = $null
$script:Counter = 0
[void][IO.Directory]::CreateDirectory($script:RunRoot)

#region ---------------------------------------------------------------- harness

function Format-Snip([string]$Text) {
    $t = ($Text -replace '\s+', ' ').Trim()
    if ($t.Length -gt 400) { $t = $t.Substring(0, 400) + '...' }
    return "[$t]"
}

function Assert-That([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}

function Assert-Exit($Result, [int]$Want) {
    if ($Result.Code -ne $Want) {
        throw "exit $($Result.Code), wanted $Want. stdout $(Format-Snip $Result.Out) stderr $(Format-Snip $Result.Err)"
    }
}

# Addresses are built when a case runs, so the tree holds no address for the secrets
# check to flag. example.com and example.org are reserved for examples (RFC 2606).
function Get-TestMail([string]$User, [string]$Domain = 'example.com') { return $User + '@' + $Domain }

# Starts pwsh -File with an exact argument list and both streams captured. Both are
# read asynchronously before the wait, so a child that fills a pipe cannot deadlock it.
function Start-Pwsh([string]$File, [string[]]$Arguments, [hashtable]$Env) {
    $psi = [Diagnostics.ProcessStartInfo]::new($script:Pwsh)
    foreach ($a in @('-NoLogo', '-NoProfile', '-NonInteractive', '-File', $File) + @($Arguments)) { $psi.ArgumentList.Add([string]$a) }
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.RedirectStandardInput = $true
    $psi.WorkingDirectory = $script:RunRoot
    $psi.Environment['NO_COLOR'] = '1'
    foreach ($k in @($Env.Keys)) {
        if ($null -eq $Env[$k]) { [void]$psi.Environment.Remove($k) } else { $psi.Environment[$k] = [string]$Env[$k] }
    }
    $p = [Diagnostics.Process]::Start($psi)
    $p.StandardInput.Close()
    return [pscustomobject]@{ Process = $p; Out = $p.StandardOutput.ReadToEndAsync(); Err = $p.StandardError.ReadToEndAsync() }
}

function Wait-Pwsh($Started, [int]$TimeoutSec = 180) {
    if (-not $Started.Process.WaitForExit($TimeoutSec * 1000)) {
        $Started.Process.Kill($true)
        throw "a child did not finish within ${TimeoutSec}s"
    }
    $Started.Process.WaitForExit()
    return [pscustomobject]@{ Code = $Started.Process.ExitCode; Out = $Started.Out.Result; Err = $Started.Err.Result }
}

function Get-CaseEnv {
    $e = @{ CC_PROFILES_CONFIG = $script:Ctx.Config }
    foreach ($k in @($script:Ctx.ExtraEnv.Keys)) { $e[$k] = $script:Ctx.ExtraEnv[$k] }
    return $e
}

# Runs the tool against the current case's config. Arguments are passed exactly as
# given, so "-Alias", "p,q" reaches the tool as the two strings it gets from a shell.
function Invoke-ToolRun {
    $toolArgs = @($args | ForEach-Object { [string]$_ }) + @('-ConfigPath', $script:Ctx.Config)
    return Wait-Pwsh (Start-Pwsh $script:ToolPath $toolArgs (Get-CaseEnv))
}
Set-Alias -Name Run -Value Invoke-ToolRun

# Runs a block in a fresh process with the tool dot-sourced, so a unit case reaches
# the functions without the script's command dispatch, and a mutation reaches it too.
# A function the block defines after the dot-source stands in for the tool's own.
function Invoke-Unit([scriptblock]$Code) {
    $script:Counter++
    $file = Join-Path $script:Ctx.Dir ("unit-{0}.ps1" -f $script:Counter)
    $header = @(
        'param([string]$ToolPath)'
        '$ErrorActionPreference = ''Stop'''
        '. $ToolPath'
        'Initialize-Locations'
    ) -join "`n"
    [IO.File]::WriteAllText($file, $header + "`n" + $Code.ToString() + "`n", [Text.UTF8Encoding]::new($false))
    return Wait-Pwsh (Start-Pwsh $file @('-ToolPath', $script:ToolPath) (Get-CaseEnv))
}

function New-CaseContext([string]$Name) {
    $dir = Join-Path $script:RunRoot ($Name + '-' + [guid]::NewGuid().ToString('N').Substring(0, 4))
    [void][IO.Directory]::CreateDirectory($dir)
    $cfg = Join-Path $dir 'cfg.json'
    $root = Join-Path $dir 'profiles'
    [IO.File]::WriteAllText($cfg, ('{ "version": 1, "root": "' + $root.Replace('\', '/') + '", "profiles": {} }'), [Text.UTF8Encoding]::new($false))
    return [pscustomobject]@{ Dir = $dir; Config = $cfg; Root = $root; DataDir = Join-Path $dir 'cfg.d'; ExtraEnv = @{} }
}

function Read-CaseConfig { return ([IO.File]::ReadAllText($script:Ctx.Config) | ConvertFrom-Json -AsHashtable) }
function Set-CaseConfig($Config) { [IO.File]::WriteAllText($script:Ctx.Config, ($Config | ConvertTo-Json -Depth 10)) }

function Get-ProfileProcesses([string]$UserDataDir) {
    $needle = '--user-data-dir=' + $UserDataDir
    return @(Get-CimInstance Win32_Process -Filter "Name='Claude.exe'" -Verbose:$false |
            Where-Object { $_.CommandLine -and $_.CommandLine.Contains($needle) })
}

function Get-MainProcess([string]$UserDataDir) {
    return @(Get-ProfileProcesses $UserDataDir | Where-Object { $_.CommandLine -notmatch '--type=' }) | Select-Object -First 1
}

function Wait-Until([scriptblock]$Condition, [int]$Seconds) {
    $deadline = [DateTime]::UtcNow.AddSeconds($Seconds)
    do {
        if (& $Condition) { return $true }
        Start-Sleep -Milliseconds 500
    } while ([DateTime]::UtcNow -lt $deadline)
    return [bool](& $Condition)
}

# Ends every process of a test profile and waits until none is left: a child can
# outlive its main process by a few seconds, and a case must not leave one behind.
function Stop-ProfileApp([string]$UserDataDir) {
    $deadline = [DateTime]::UtcNow.AddSeconds(20)
    do {
        $left = @(Get-ProfileProcesses $UserDataDir)
        foreach ($p in $left) { Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue }
        if (-not $left.Count) { return }
        Start-Sleep -Milliseconds 500
    } while ([DateTime]::UtcNow -lt $deadline)
}

#endregion
#region ---------------------------------------------------------------- the job object the live cases use

$script:JobSource = @(
    'using System;'
    'using System.Runtime.InteropServices;'
    'public static class CcProfileTestJob {'
    '    [StructLayout(LayoutKind.Sequential)] struct BASIC {'
    '        public long PerProcessUserTimeLimit, PerJobUserTimeLimit; public uint LimitFlags;'
    '        public UIntPtr MinimumWorkingSetSize, MaximumWorkingSetSize; public uint ActiveProcessLimit;'
    '        public UIntPtr Affinity; public uint PriorityClass, SchedulingClass; }'
    '    [StructLayout(LayoutKind.Sequential)] struct IOC { public ulong a, b, c, d, e, f; }'
    '    [StructLayout(LayoutKind.Sequential)] struct EXTENDED {'
    '        public BASIC Basic; public IOC Io; public UIntPtr ProcessMemoryLimit, JobMemoryLimit,'
    '        PeakProcessMemoryUsed, PeakJobMemoryUsed; }'
    '    [DllImport("kernel32.dll", SetLastError = true)] static extern IntPtr CreateJobObject(IntPtr a, string n);'
    '    [DllImport("kernel32.dll", SetLastError = true)] static extern bool SetInformationJobObject(IntPtr j, int c, ref EXTENDED i, int l);'
    '    [DllImport("kernel32.dll", SetLastError = true)] static extern bool AssignProcessToJobObject(IntPtr j, IntPtr p);'
    '    [DllImport("kernel32.dll", SetLastError = true)] public static extern bool CloseHandle(IntPtr h);'
    '    // KILL_ON_JOB_CLOSE 0x2000, and BREAKAWAY_OK 0x800 when allowBreakaway.'
    '    public static IntPtr Create(bool allowBreakaway) {'
    '        IntPtr j = CreateJobObject(IntPtr.Zero, null);'
    '        if (j == IntPtr.Zero) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());'
    '        var info = new EXTENDED(); info.Basic.LimitFlags = 0x2000u | (allowBreakaway ? 0x800u : 0u);'
    '        if (!SetInformationJobObject(j, 9, ref info, Marshal.SizeOf(typeof(EXTENDED))))'
    '            throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());'
    '        return j; }'
    '    public static void Assign(IntPtr job, IntPtr process) {'
    '        if (!AssignProcessToJobObject(job, process)) throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error()); }'
    '}'
) -join "`n"

# A real terminal tab, built the way Windows Terminal builds one: a pseudo-console
# (ConPTY) with a shell on it. Close() is what closing the tab does, ClosePseudoConsole,
# which sends CTRL_CLOSE_EVENT to every process on that console. Screen is everything
# the console rendered, so a log line the app printed into the terminal can be found.
$script:TerminalSource = @(
    'using System;'
    'using System.ComponentModel;'
    'using System.IO;'
    'using System.Runtime.InteropServices;'
    'using System.Text;'
    'using System.Threading;'
    'using Microsoft.Win32.SafeHandles;'
    'public sealed class CcProfileTestTerminal : IDisposable {'
    '    [StructLayout(LayoutKind.Sequential)] struct COORD { public short X; public short Y; }'
    '    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)] struct STARTUPINFO {'
    '        public int cb; public string lpReserved; public string lpDesktop; public string lpTitle;'
    '        public int dwX, dwY, dwXSize, dwYSize, dwXCountChars, dwYCountChars, dwFillAttribute, dwFlags;'
    '        public short wShowWindow, cbReserved2; public IntPtr lpReserved2, hStdInput, hStdOutput, hStdError; }'
    '    [StructLayout(LayoutKind.Sequential)] struct STARTUPINFOEX { public STARTUPINFO StartupInfo; public IntPtr lpAttributeList; }'
    '    [StructLayout(LayoutKind.Sequential)] struct PROCESS_INFORMATION { public IntPtr hProcess, hThread; public int dwProcessId, dwThreadId; }'
    '    [DllImport("kernel32.dll", SetLastError = true)] static extern bool CreatePipe(out SafeFileHandle r, out SafeFileHandle w, IntPtr sa, int size);'
    '    [DllImport("kernel32.dll")] static extern int CreatePseudoConsole(COORD size, SafeFileHandle input, SafeFileHandle output, uint flags, out IntPtr pc);'
    '    [DllImport("kernel32.dll")] static extern void ClosePseudoConsole(IntPtr pc);'
    '    [DllImport("kernel32.dll", SetLastError = true)] static extern bool InitializeProcThreadAttributeList(IntPtr list, int count, int flags, ref IntPtr size);'
    '    [DllImport("kernel32.dll", SetLastError = true)] static extern bool UpdateProcThreadAttribute(IntPtr list, uint flags, IntPtr attr, IntPtr value, IntPtr size, IntPtr prev, IntPtr ret);'
    '    [DllImport("kernel32.dll")] static extern void DeleteProcThreadAttributeList(IntPtr list);'
    '    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)] static extern bool CreateProcessW(string app, StringBuilder cmd,'
    '        IntPtr pa, IntPtr ta, bool inherit, uint flags, IntPtr env, string cwd, ref STARTUPINFOEX si, out PROCESS_INFORMATION pi);'
    '    [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr h);'
    '    IntPtr pc, attrs;'
    '    SafeFileHandle toTerminal;'
    '    readonly StringBuilder screen = new StringBuilder();'
    '    public int ProcessId;'
    '    public CcProfileTestTerminal(string commandLine, string cwd) {'
    '        SafeFileHandle inRead, outWrite, fromTerminal;'
    '        if (!CreatePipe(out inRead, out toTerminal, IntPtr.Zero, 0)) throw new Win32Exception(Marshal.GetLastWin32Error());'
    '        if (!CreatePipe(out fromTerminal, out outWrite, IntPtr.Zero, 0)) throw new Win32Exception(Marshal.GetLastWin32Error());'
    '        var size = new COORD(); size.X = 220; size.Y = 60;'
    '        int hr = CreatePseudoConsole(size, inRead, outWrite, 0, out pc);'
    '        if (hr != 0) throw new Win32Exception(hr);'
    '        inRead.Dispose(); outWrite.Dispose();'
    '        var reader = new Thread(() => {'
    '            var buf = new byte[8192];'
    '            try {'
    '                using (var fs = new FileStream(fromTerminal, FileAccess.Read)) {'
    '                    int n;'
    '                    while ((n = fs.Read(buf, 0, buf.Length)) > 0) { lock (screen) screen.Append(Encoding.UTF8.GetString(buf, 0, n)); }'
    '                }'
    '            } catch (IOException) { /* the pseudo-console closed its end */ }'
    '        });'
    '        reader.IsBackground = true;'
    '        reader.Start();'
    '        IntPtr bytes = IntPtr.Zero;'
    '        InitializeProcThreadAttributeList(IntPtr.Zero, 1, 0, ref bytes);'
    '        attrs = Marshal.AllocHGlobal(bytes);'
    '        if (!InitializeProcThreadAttributeList(attrs, 1, 0, ref bytes)) throw new Win32Exception(Marshal.GetLastWin32Error());'
    '        // PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE'
    '        if (!UpdateProcThreadAttribute(attrs, 0, (IntPtr)0x00020016, pc, (IntPtr)IntPtr.Size, IntPtr.Zero, IntPtr.Zero))'
    '            throw new Win32Exception(Marshal.GetLastWin32Error());'
    '        var si = new STARTUPINFOEX();'
    '        si.StartupInfo.cb = Marshal.SizeOf(typeof(STARTUPINFOEX));'
    '        // STARTF_USESTDHANDLES with null handles, as Windows Terminal does: otherwise the'
    '        // shell inherits THIS process''s redirected streams instead of the pseudo-console.'
    '        si.StartupInfo.dwFlags = 0x00000100;'
    '        si.lpAttributeList = attrs;'
    '        PROCESS_INFORMATION pi;'
    '        // EXTENDED_STARTUPINFO_PRESENT'
    '        if (!CreateProcessW(null, new StringBuilder(commandLine), IntPtr.Zero, IntPtr.Zero, false, 0x00080000, IntPtr.Zero, cwd, ref si, out pi))'
    '            throw new Win32Exception(Marshal.GetLastWin32Error());'
    '        CloseHandle(pi.hThread); CloseHandle(pi.hProcess);'
    '        ProcessId = pi.dwProcessId;'
    '    }'
    '    public string Screen { get { lock (screen) return screen.ToString(); } }'
    '    public void Close() { if (pc != IntPtr.Zero) { ClosePseudoConsole(pc); pc = IntPtr.Zero; } }'
    '    public void Dispose() {'
    '        Close();'
    '        if (attrs != IntPtr.Zero) { DeleteProcThreadAttributeList(attrs); Marshal.FreeHGlobal(attrs); attrs = IntPtr.Zero; }'
    '        if (toTerminal != null) toTerminal.Dispose();'
    '    }'
    '}'
) -join "`n"

# The suite's OWN console probe, independent of the one under test: attach a throwaway
# process to the target's console and list who is on it.
$script:ProbeSource = @(
    'using System;'
    'using System.Runtime.InteropServices;'
    'public static class CcProfileTestProbe {'
    '    [DllImport("kernel32.dll", SetLastError = true)] static extern bool FreeConsole();'
    '    [DllImport("kernel32.dll", SetLastError = true)] static extern bool AttachConsole(uint pid);'
    '    [DllImport("kernel32.dll", SetLastError = true)] static extern uint GetConsoleProcessList(uint[] list, uint count);'
    '    public static string Probe(uint pid) {'
    '        FreeConsole();'
    '        if (!AttachConsole(pid)) { int e = Marshal.GetLastWin32Error(); return e == 6 ? "none" : "unknown " + e; }'
    '        try {'
    '            uint[] buf = new uint[1024];'
    '            uint n = GetConsoleProcessList(buf, (uint)buf.Length);'
    '            var sb = new System.Text.StringBuilder("attached");'
    '            for (uint i = 0; i < n && i < buf.Length; i++) { sb.Append(" "); sb.Append(buf[i]); }'
    '            return sb.ToString();'
    '        } finally { FreeConsole(); }'
    '    }'
    '}'
) -join "`n"

function Get-TestConsoleState([int]$TargetPid) {
    $file = Join-Path $script:RunRoot 'probe.ps1'
    if (-not [IO.File]::Exists($file)) {
        $lines = @(
            'param([int]$Target)'
            'Add-Type -TypeDefinition $env:CC_TEST_PROBE_SOURCE'
            '[Console]::Out.WriteLine([CcProfileTestProbe]::Probe([uint32]$Target))'
        )
        [IO.File]::WriteAllText($file, ($lines -join "`n") + "`n", [Text.UTF8Encoding]::new($false))
    }
    $r = Wait-Pwsh (Start-Pwsh $file @('-Target', $TargetPid) @{ CC_TEST_PROBE_SOURCE = $script:ProbeSource })
    return $r.Out.Trim()
}

function ConvertTo-PsLiteral([string]$Text) { return "'" + $Text.Replace("'", "''") + "'" }

function Start-ConsoleTerminal([string[]]$Lines) {
    $file = Join-Path $script:Ctx.Dir ('console-{0}.ps1' -f [guid]::NewGuid().ToString('N').Substring(0, 6))
    [IO.File]::WriteAllText($file, ($Lines -join "`n") + "`n", [Text.UTF8Encoding]::new($false))
    if (-not ('CcProfileTestTerminal' -as [type])) { Add-Type -TypeDefinition $script:TerminalSource -Language CSharp }
    return [CcProfileTestTerminal]::new("`"$($script:Pwsh)`" -NoLogo -NoProfile -File `"$file`"", $script:Ctx.Dir)
}

# Lines Electron writes when it is attached to a console; the user's report had three of them.
$script:AppLogPattern = 'Error occurred in handler|:ERROR:|:INFO:|:WARNING:|CompositorAnimationObserver|MaxListenersExceededWarning'

# A stand-in for a terminal: a shell that runs the tool IN-PROCESS, as an alias does,
# and then stays open. The launch log is the tool's own, so nothing here is guessed.
# -Verify keeps the launch's own read-back, which the other live cases skip for speed.
function Start-Terminal([string]$ProfileName, [switch]$Verify) {
    $file = Join-Path $script:Ctx.Dir 'terminal.ps1'
    $launch = if ($Verify) { '& $ToolPath launch $ProfileName -ConfigPath $ConfigPath *> $LogPath' } else { '& $ToolPath launch $ProfileName -ConfigPath $ConfigPath -NoVerify *> $LogPath' }
    $lines = @(
        'param([string]$ToolPath, [string]$ConfigPath, [string]$ProfileName, [string]$LogPath)'
        'Start-Sleep -Seconds 2'
        $launch
        'Start-Sleep -Seconds 600'
    )
    [IO.File]::WriteAllText($file, ($lines -join "`n") + "`n", [Text.UTF8Encoding]::new($false))
    $log = Join-Path $script:Ctx.Dir 'terminal.log'
    $started = Start-Pwsh $file @('-ToolPath', $script:ToolPath, '-ConfigPath', $script:Ctx.Config, '-ProfileName', $ProfileName, '-LogPath', $log) (Get-CaseEnv)
    return [pscustomobject]@{ Process = $started.Process; Log = $log }
}

# Reads a log another process is still writing, which File.ReadAllText refuses.
function Read-TextIfAny([string]$Path) {
    try {
        if (-not [IO.File]::Exists($Path)) { return '' }
        $fs = [IO.FileStream]::new($Path, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete)
        try { return [IO.StreamReader]::new($fs).ReadToEnd() } finally { $fs.Dispose() }
    }
    catch { return '' }
}

#endregion
#region ---------------------------------------------------------------- cases

$script:Cases = [ordered]@{}
function Add-Case([string]$Name, [scriptblock]$Body, [switch]$LiveOnly) {
    $script:Cases[$Name] = [pscustomobject]@{ Name = $Name; Body = $Body; Live = $LiveOnly.IsPresent }
}

Add-Case 'version-answers-with-the-tool-name' {
    $r = Run 'version'
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'cc_profile \d+\.\d+\.\d+') "no version line in $(Format-Snip $r.Out)"
}

Add-Case 'add-creates-a-profile-with-isolated-folders' {
    $mail = Get-TestMail 'a'
    Assert-Exit (Run 'add' 't-work' '-Email' $mail '-Alias' 'w') 0
    $p = (Read-CaseConfig)['profiles']['t-work']
    Assert-That ($p['email'] -eq $mail) 'email not stored'
    Assert-That (@($p['aliases']) -contains 'w') 'alias not stored'
    Assert-That ($p['userDataDir'] -like '*/profiles/t-work') "userDataDir is $($p['userDataDir'])"
    Assert-That ($p['claudeConfigDir'] -like '*/profiles/t-work/.claude') "claudeConfigDir is $($p['claudeConfigDir'])"
    Assert-That ([IO.Directory]::Exists((Join-Path $script:Ctx.Root 't-work\.claude'))) 'folders were not created'
}

Add-Case 'add-again-writes-nothing' {
    $mail = Get-TestMail 'a'
    Assert-Exit (Run 'add' 't-work' '-Email' $mail '-Alias' 'w') 0
    $before = [IO.FileInfo]::new($script:Ctx.Config)
    $stamp = $before.LastWriteTimeUtc
    $hash = (Get-FileHash $script:Ctx.Config).Hash
    $backups = @(Get-ChildItem (Join-Path $script:Ctx.DataDir 'backups') -File).Count
    Start-Sleep -Milliseconds 1100
    $r = Run 'add' 't-work' '-Email' $mail '-Alias' 'w'
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'nothing to do') "no idempotency message in $(Format-Snip $r.Out)"
    Assert-That ([IO.FileInfo]::new($script:Ctx.Config).LastWriteTimeUtc -eq $stamp) 'the config was rewritten'
    Assert-That ((Get-FileHash $script:Ctx.Config).Hash -eq $hash) 'the config changed'
    Assert-That (@(Get-ChildItem (Join-Path $script:Ctx.DataDir 'backups') -File).Count -eq $backups) 'a backup was taken for a no-op'
}

Add-Case 'add-with-another-email-is-refused' {
    Assert-Exit (Run 'add' 't-work' '-Email' (Get-TestMail 'a')) 0
    $hash = (Get-FileHash $script:Ctx.Config).Hash
    $r = Run 'add' 't-work' '-Email' (Get-TestMail 'b')
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'already exists and differs') "wrong refusal $(Format-Snip $r.Err)"
    Assert-That ((Get-FileHash $script:Ctx.Config).Hash -eq $hash) 'the config changed on a refusal'
}

Add-Case 'add-derives-the-name-from-the-email' {
    Assert-Exit (Run 'add' '-Email' (Get-TestMail 'Bob.Smith' 'example.org')) 0
    Assert-That ((Read-CaseConfig)['profiles'].Contains('bob.smith')) 'no profile named bob.smith'
    $r = Run 'add' '-Email' 'not-an-address'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'is not a valid email address') "a name was derived from a bad address: $(Format-Snip $r.Err)"
}

# AppData is redirected into the case's own folder, so neither the refusal nor a
# mutant that lets the add through writes into the real AppData.
Add-Case 'a-profile-under-appdata-is-refused' {
    $fake = Join-Path $script:Ctx.Dir 'fake-localappdata'
    [void][IO.Directory]::CreateDirectory($fake)
    $script:Ctx.ExtraEnv = @{ LOCALAPPDATA = $fake }
    $bad = Join-Path $fake 'cc-profile-test'
    $r = Run 'add' 't-bad' '-UserDataDir' $bad
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'AppData') "wrong refusal $(Format-Snip $r.Err)"
    Assert-That (-not (Read-CaseConfig)['profiles'].Contains('t-bad')) 'the profile was saved'
    Assert-That (-not [IO.Directory]::Exists($bad)) 'the folder was created'
}

Add-Case 'overlapping-profile-folders-are-refused' {
    Assert-Exit (Run 'add' 't-a') 0
    $r = Run 'add' 't-b' '-UserDataDir' (Join-Path $script:Ctx.Root 't-a\inner')
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'overlap') "wrong refusal $(Format-Snip $r.Err)"
}

# One rule, three doors: a name given to add, a name already in a hand-edited file,
# and an alias. A mutation of the one rule turns every door red.
Add-Case 'a-command-name-is-refused-as-a-profile-name' {
    $r = Run 'add' 'list'
    Assert-Exit $r 1
    Assert-That ($r.Err -match "'list' is reserved") "add: wrong refusal $(Format-Snip $r.Err)"
    Assert-Exit (Run 'add' 't-a') 0
    $r = Run 'alias' 't-a' 'ls'
    Assert-Exit $r 1
    Assert-That ($r.Err -match "'ls' is reserved") "alias: wrong refusal $(Format-Snip $r.Err)"
    $c = Read-CaseConfig
    $c['profiles']['status'] = @{ userDataDir = (Join-Path $script:Ctx.Root 'st').Replace('\', '/') }
    Set-CaseConfig $c
    $r = Run 'validate'
    Assert-Exit $r 1
    Assert-That ($r.Out -match "'status' is reserved") "validate: wrong refusal $(Format-Snip $r.Out)"
}

Add-Case 'an-alias-two-profiles-claim-is-refused' {
    Assert-Exit (Run 'add' 't-a' '-Alias' 'x') 0
    $r = Run 'add' 't-b' '-Alias' 'x'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'already used') "wrong refusal $(Format-Snip $r.Err)"
}

# Through pwsh -File, a list arrives as one word. Options and positional words both
# split it on commas.
Add-Case 'a-comma-list-through-file-is-split' {
    Assert-Exit (Run 'add' 't-a' '-Alias' 'p,q') 0
    $al = @((Read-CaseConfig)['profiles']['t-a']['aliases'])
    Assert-That ($al.Count -eq 2 -and $al -contains 'p' -and $al -contains 'q') "-Alias: aliases are [$($al -join '|')]"
    Assert-Exit (Run 'alias' 't-a' 'r,s') 0
    $al = @((Read-CaseConfig)['profiles']['t-a']['aliases'])
    Assert-That ($al.Count -eq 4 -and $al -contains 'r' -and $al -contains 's') "alias: aliases are [$($al -join '|')]"
    Assert-Exit (Run 'unalias' 'r,s') 0
    $al = @((Read-CaseConfig)['profiles']['t-a']['aliases'])
    Assert-That ($al.Count -eq 2 -and $al -notcontains 'r') "unalias: aliases are [$($al -join '|')]"
    Assert-Exit (Run 'set' 't-a' '-RemoveAlias' 'p,q') 0
    $al = @((Read-CaseConfig)['profiles']['t-a']['aliases'])
    Assert-That ($al.Count -eq 0) "-RemoveAlias: aliases are [$($al -join '|')]"
    Assert-Exit (Run 'set' 't-a' '-SetEnv' 'ONE=1') 0
    Assert-Exit (Run 'set' 't-a' '-SetEnv' 'TWO=2') 0
    Assert-Exit (Run 'set' 't-a' '-UnsetEnv' 'ONE,TWO') 0
    $envd = (Read-CaseConfig)['profiles']['t-a']['env']
    Assert-That (-not @($envd.Keys).Count) "-UnsetEnv: env is [$(@($envd.Keys) -join '|')]"
}

Add-Case 'setenv-takes-name-equals-value-and-show-redacts-secrets' {
    Assert-Exit (Run 'add' 't-a' '-SetEnv' 'FOO=bar=baz') 0
    Assert-Exit (Run 'set' 't-a' '-SetEnv' 'MY_TOKEN=s3cret') 0
    $raw = (Read-CaseConfig)['profiles']['t-a']['env']
    Assert-That ($raw['FOO'] -eq 'bar=baz' -and $raw['MY_TOKEN'] -eq 's3cret') 'the values were not stored as given'
    $r = Run 'show' 't-a' '-Json'
    Assert-Exit $r 0
    $shown = ($r.Out | ConvertFrom-Json -AsHashtable)['env']
    Assert-That ($shown['FOO'] -eq 'bar=baz') 'FOO is not shown'
    Assert-That ($shown['MY_TOKEN'] -eq '<redacted>') "MY_TOKEN is shown as $($shown['MY_TOKEN'])"
}

# Windows reads "http_proxy" and "HTTP_PROXY" as one variable. A new spelling replaces
# the old entry instead of sitting beside it with the other value.
Add-Case 'setenv-replaces-a-variable-whatever-its-case' {
    Assert-Exit (Run 'add' 't-a' '-SetEnv' 'http_proxy=http://old:1') 0
    $r = Run 'set' 't-a' '-SetEnv' 'HTTP_PROXY=http://new:2'
    Assert-Exit $r 0
    $envd = (Read-CaseConfig)['profiles']['t-a']['env']
    $keys = @($envd.Keys)
    Assert-That ($keys.Count -eq 1 -and $keys[0] -ceq 'HTTP_PROXY' -and $envd['HTTP_PROXY'] -eq 'http://new:2') "env is [$(@($keys | ForEach-Object { "$_=$($envd[$_])" }) -join '|')]"
}

Add-Case 'the-default-profile-takes-details-but-not-folders' {
    Assert-Exit (Run 'set' 'default' '-Email' (Get-TestMail 'me' 'example.net') '-Alias' 'main') 0
    $d = (Read-CaseConfig)['profiles']['default']
    Assert-That ($d['email'] -eq (Get-TestMail 'me' 'example.net') -and @($d['aliases']) -contains 'main') 'details not stored'
    $r = Run 'set' 'default' '-UserDataDir' (Join-Path $script:Ctx.Root 'x')
    Assert-Exit $r 1
    Assert-That ($r.Err -match '-UserDataDir is not allowed on the built-in default profile') "set: wrong refusal $(Format-Snip $r.Err)"
    $c = Read-CaseConfig
    $c['profiles']['default']['userDataDir'] = (Join-Path $script:Ctx.Root 'x').Replace('\', '/')
    Set-CaseConfig $c
    $r = Run 'validate'
    Assert-Exit $r 1
    Assert-That ($r.Out -match 'profiles\.default\.userDataDir: not allowed on the built-in default profile') "validate: wrong refusal $(Format-Snip $r.Out)"
}

Add-Case 'a-profile-resolves-by-name-alias-and-email' {
    $mail = Get-TestMail 'a'
    Assert-Exit (Run 'add' 't-a' '-Email' $mail '-Alias' 'aa') 0
    $dirs = foreach ($id in 't-a', 'aa', $mail, $mail.ToUpperInvariant()) {
        $r = Run 'path' $id '-Json'
        Assert-Exit $r 0
        ($r.Out | ConvertFrom-Json).UserDataDir
    }
    Assert-That (@($dirs | Select-Object -Unique).Count -eq 1) "the identifiers resolved to [$($dirs -join '|')]"
}

Add-Case 'an-unknown-profile-is-refused-with-a-hint' {
    Assert-Exit (Run 'add' 't-work') 0
    $r = Run 't-wor'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'Did you mean' -and $r.Err -match 't-work') "no hint in $(Format-Snip $r.Err)"
}

Add-Case 'a-dry-run-carries-isolation-and-none-of-the-callers-variables' {
    Assert-Exit (Run 'add' 't-a') 0
    $script:Ctx.ExtraEnv = @{ ANTHROPIC_API_KEY = 'leak'; CLAUDECODE = '1'; ELECTRON_RUN_AS_NODE = '1'; CLAUDE_CONFIG_DIR = 'C:\wrong'; ELECTRON_NO_ATTACH_CONSOLE = '0' }
    $r = Run 't-a' '-DryRun'
    Assert-Exit $r 0
    $udd = (Join-Path $script:Ctx.Root 't-a')
    Assert-That ($r.Out.Contains("--user-data-dir=$udd")) "no --user-data-dir in $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match [regex]::Escape("CLAUDE_CONFIG_DIR  : $udd\.claude")) 'CLAUDE_CONFIG_DIR is not the profile one'
    Assert-That ($r.Out -match '(?m)^\s+[+~] ELECTRON_NO_ATTACH_CONSOLE=1\s*$') 'the app is not told to stay off the console'
    foreach ($v in 'ANTHROPIC_API_KEY', 'CLAUDECODE', 'ELECTRON_RUN_AS_NODE') {
        Assert-That ($r.Out -match "(?m)^\s+- $v\s*$") "$v is not reported as dropped"
        Assert-That ($r.Out -notmatch "(?m)^\s+[+~] $v=") "$v reaches the app"
    }
    Assert-That ($r.Out -notmatch 'USERNAME=SYSTEM') 'the clean environment carries USERNAME=SYSTEM'
    # In "inherit" mode the base IS the caller's environment, so only the scrub stands
    # between these variables and the app. An empty scrubEnv must not open that door:
    # the built-in patterns apply whatever the config says.
    $c = Read-CaseConfig
    $c['envMode'] = 'inherit'
    $c['scrubEnv'] = @()
    Set-CaseConfig $c
    $r = Run 't-a' '-DryRun'
    Assert-Exit $r 0
    foreach ($v in 'ANTHROPIC_API_KEY', 'CLAUDECODE', 'ELECTRON_RUN_AS_NODE') {
        Assert-That ($r.Out -match "(?m)^\s+- $v\s*$") "inherit mode with an empty scrubEnv: $v is not reported as dropped"
    }
    Assert-That ($r.Out -match [regex]::Escape("CLAUDE_CONFIG_DIR  : $udd\.claude")) 'inherit mode: the caller CLAUDE_CONFIG_DIR won'
    Assert-That ($r.Out -match '(?m)^\s+[+~] ELECTRON_NO_ATTACH_CONSOLE=1\s*$') 'inherit mode: the caller value of ELECTRON_NO_ATTACH_CONSOLE won'
    $r = Run 't-a' '-DryRun' '-AppArgs:--user-data-dir=C:\elsewhere'
    Assert-Exit $r 1
    Assert-That ($r.Err -match '-AppArgs must not contain --user-data-dir') "an -AppArgs folder was not refused: $(Format-Snip $r.Err)"
}

Add-Case 'a-profile-cannot-set-a-variable-the-launch-owns' {
    Assert-Exit (Run 'add' 't-a') 0
    $r = Run 'set' 't-a' '-SetEnv' 'ELECTRON_NO_ATTACH_CONSOLE=0'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'managed') "wrong refusal $(Format-Snip $r.Err)"
}

Add-Case 'the-console-probe-tells-attached-from-none' {
    $r = Invoke-Unit {
        $ping = [IO.Path]::Combine([Environment]::SystemDirectory, 'PING.EXE')
        $envList = [string[]]@("SystemRoot=$env:SystemRoot", "PATH=$([Environment]::SystemDirectory)")
        $none = (Invoke-NativeStart $ping 'ping -n 30 127.0.0.1' $envList $HOME $false 0).Pid
        $hidden = (Invoke-NativeStart $ping 'ping -n 30 127.0.0.1' $envList $HOME $true 0).Pid
        $gone = (Invoke-NativeStart $ping 'ping -n 1 127.0.0.1' $envList $HOME $false 10000).Pid
        try {
            Start-Sleep -Milliseconds 500
            $states = Get-ConsoleAttachment @($none, $hidden, $gone)
            foreach ($k in @(@('DETACHED', $none), @('HIDDEN', $hidden), @('GONE', $gone))) {
                $s = $states[$k[1]]
                "$($k[0])=$(if ($s) { $s.State } else { 'missing' })"
            }
        }
        finally { Stop-Process -Id $none, $hidden -Force -ErrorAction SilentlyContinue }
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'DETACHED=none') "a process with no console was not read as none: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match 'HIDDEN=attached') "a process on a console was not read as attached: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match 'GONE=unknown') "a process that is gone was not read as unknown: $(Format-Snip $r.Out)"
}

Add-Case 'an-in-process-call-takes-arrays-and-hashtables' {
    $file = Join-Path $script:Ctx.Dir 'inproc.ps1'
    $lines = @(
        'param([string]$ToolPath, [string]$ConfigPath)'
        '& $ToolPath add t-a -Alias x, y -SetEnv @{ FOO = ''1'' } -ConfigPath $ConfigPath 6>$null'
        '"ADD=$LASTEXITCODE"'
        '& $ToolPath t-a -DryRun -ConfigPath $ConfigPath 6>$null'
        '"DRY=$LASTEXITCODE"'
        '& $ToolPath list -ConfigPath $ConfigPath 6>$null'
        '"LIST=$LASTEXITCODE"'
    )
    [IO.File]::WriteAllText($file, ($lines -join "`n") + "`n")
    $r = Wait-Pwsh (Start-Pwsh $file @('-ToolPath', $script:ToolPath, '-ConfigPath', $script:Ctx.Config) (Get-CaseEnv))
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'ADD=0' -and $r.Out -match 'DRY=0' -and $r.Out -match 'LIST=0') "in-process calls: $(Format-Snip ($r.Out + $r.Err))"
    $p = (Read-CaseConfig)['profiles']['t-a']
    Assert-That (@($p['aliases']).Count -eq 2 -and $p['env']['FOO'] -eq '1') 'the array or the hashtable did not arrive'
}

# The "What if" line about the launch is the point of -WhatIf, so it must not be one
# of dozens: finding the app runs with WhatIf off, and only this tool's own
# operations may answer.
Add-Case 'whatif-launches-nothing' {
    Assert-Exit (Run 'add' 't-a') 0
    $udd = Join-Path $script:Ctx.Root 't-a'
    Remove-Item -LiteralPath $udd -Recurse -Force
    $r = Run 'launch' 't-a' '-WhatIf'
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'What if: Performing the operation "Launch" on target "Claude profile ''t-a''"') "no What if for the launch in $(Format-Snip $r.Out)"
    $other = @(($r.Out -split '\r?\n') | Where-Object { $_ -match 'What if:' -and $_ -notmatch 'operation "(Launch|Create directory)"' })
    Assert-That (-not $other.Count) "$($other.Count) other What if line(s), the first: $(Format-Snip ($other | Select-Object -First 1))"
    Start-Sleep -Seconds 2
    Assert-That (-not (Get-MainProcess $udd)) 'an app instance started'
    Assert-That (-not [IO.Directory]::Exists($udd)) 'the profile folder was created'
}

Add-Case 'a-corrupt-config-is-refused-and-restore-recovers-it' {
    Assert-Exit (Run 'add' 't-a') 0
    Assert-Exit (Run 'add' 't-b') 0
    [IO.File]::WriteAllText($script:Ctx.Config, '{ "version": 1, "profiles": { "x": ')
    $r = Run 'list'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'Invalid JSON') "wrong refusal $(Format-Snip $r.Err)"
    Assert-Exit (Run 'restore' '1') 0
    Assert-Exit (Run 'validate') 0
    Assert-That ((Read-CaseConfig)['profiles'].Contains('t-a')) 'restore did not bring t-a back'
}

Add-Case 'restore-works-with-a-single-backup' {
    # The first write backs up the file it replaces, so there is exactly one backup.
    Assert-Exit (Run 'add' 't-a') 0
    $list = Run 'restore'
    Assert-Exit $list 0
    Assert-That ($list.Out -match '\[ 1\] cc_profiles\.') "the one backup is not listed: $(Format-Snip $list.Out)"
    Assert-Exit (Run 'restore' '1') 0
    Assert-That (-not (Read-CaseConfig)['profiles'].Contains('t-a')) 'restore 1 did not bring back the file from before the add'
}

# t-a.b is t-a's neighbour by name; an error in it must not block t-a. A profile on a
# drive that is not there is not an error, and must not stop the check either.
Add-Case 'a-bad-entry-blocks-only-its-own-profile' {
    Assert-Exit (Run 'add' 't-a') 0
    Assert-Exit (Run 'add' 't-a.b') 0
    Assert-Exit (Run 'add' 't-c') 0
    $free = @([char[]](90..68) | Where-Object { -not [IO.Directory]::Exists("$($_):\") }) | Select-Object -First 1
    Assert-That ($null -ne $free) 'no free drive letter to stand for a missing drive'
    $c = Read-CaseConfig
    $c['profiles']['t-a.b']['email'] = 'not-an-email'
    $c['profiles']['t-c']['userDataDir'] = "$($free):/missing/t-c"
    $c['profiles']['t-c']['claudeConfigDir'] = "$($free):/missing/t-c/.claude"
    Set-CaseConfig $c
    Assert-Exit (Run 't-a' '-DryRun') 0
    $r = Run 't-a.b' '-DryRun'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'not a valid email address') "wrong refusal $(Format-Snip $r.Err)"
    Assert-Exit (Run 'validate') 1
}

Add-Case 'a-newer-schema-is-read-only' {
    Assert-Exit (Run 'add' 't-a') 0
    $c = Read-CaseConfig
    $c['version'] = 99
    Set-CaseConfig $c
    Assert-Exit (Run 'list') 0
    $r = Run 'set' 't-a' '-Notes' 'x'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'newer') "set: wrong refusal $(Format-Snip $r.Err)"
    # restore and edit write through the same door.
    $c['version'] = 1
    Set-CaseConfig $c
    $hash = (Get-FileHash $script:Ctx.Config).Hash
    $c['version'] = 99
    $backup = Join-Path $script:Ctx.DataDir 'backups\cc_profiles.99991231-235959-999.manual.json'
    [IO.File]::WriteAllText($backup, ($c | ConvertTo-Json -Depth 10))
    $r = Run 'restore' '1'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'newer') "restore: wrong refusal $(Format-Snip $r.Err)"
    $editor = Join-Path $script:Ctx.Dir 'editor.ps1'
    $lines = @(
        'param([string]$Path)'
        'Start-Sleep -Milliseconds 3200'
        '$j = Get-Content -Raw $Path | ConvertFrom-Json -AsHashtable'
        '$j.version = 99'
        '[IO.File]::WriteAllText($Path, ($j | ConvertTo-Json -Depth 10))'
    )
    [IO.File]::WriteAllText($editor, ($lines -join "`n") + "`n")
    $script:Ctx.ExtraEnv = @{ EDITOR = "`"$($script:Pwsh)`" -NoProfile -File `"$editor`""; VISUAL = $null }
    $r = Run 'edit'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'newer' -and $r.Err -match 'kept at') "edit: wrong refusal $(Format-Snip $r.Err)"
    Assert-That ((Get-FileHash $script:Ctx.Config).Hash -eq $hash) 'a schema-99 file was written'
}

Add-Case 'parallel-writers-all-land' {
    Assert-Exit (Run 'add' 't-a') 0
    $started = foreach ($n in 1..6) {
        Start-Pwsh $script:ToolPath @('alias', 't-a', "p$n", '-ConfigPath', $script:Ctx.Config) (Get-CaseEnv)
    }
    $results = foreach ($s in $started) { Wait-Pwsh $s }
    $codes = @($results | ForEach-Object { $_.Code })
    Assert-That (-not @($codes | Where-Object { $_ -ne 0 }).Count) "exit codes $($codes -join ',')"
    $al = @((Read-CaseConfig)['profiles']['t-a']['aliases'])
    foreach ($n in 1..6) { Assert-That ($al -contains "p$n") "p$n was lost; aliases are [$($al -join '|')]" }
}

Add-Case 'a-write-refuses-when-the-file-changed-under-it' {
    $r = Invoke-Unit {
        $rr = Read-Config
        [IO.File]::AppendAllText($script:ConfigFile, ' ')
        try { Save-ConfigText (ConvertTo-ConfigJson $rr.Config) $rr.Hash; 'SAVED' }
        catch { 'REFUSED: ' + $_.Exception.Message }
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'REFUSED: .*changed on disk') "a stale write was not refused: $(Format-Snip $r.Out)"
}

# A canonical rewrite would drop a comment, so a write refuses a file that has one
# unless -Force says to rewrite it.
Add-Case 'a-config-with-comments-is-not-rewritten-without-force' {
    Assert-Exit (Run 'add' 't-a') 0
    $text = [IO.File]::ReadAllText($script:Ctx.Config)
    $text = $text.Replace('"profiles": {', ('"profiles": {' + "`n    // keep this remark"))
    [IO.File]::WriteAllText($script:Ctx.Config, $text)
    $v = Run 'validate'
    Assert-Exit $v 0
    Assert-That ($v.Out -match 'not strict JSON') "validate did not mention the comment: $(Format-Snip $v.Out)"
    $r = Run 'set' 't-a' '-Notes' 'x'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'comments') "wrong refusal $(Format-Snip $r.Err)"
    Assert-That ([IO.File]::ReadAllText($script:Ctx.Config).Contains('keep this remark')) 'the remark was lost on a refusal'
    Assert-Exit (Run 'set' 't-a' '-Notes' 'x' '-Force') 0
    Assert-That ((Read-CaseConfig)['profiles']['t-a']['notes'] -eq 'x') '-Force did not write the change'
}

# A key the tool does not know is kept, even one that differs from another only in
# case: JSON keys are case-sensitive, and a write must not merge two into one.
Add-Case 'unknown-keys-survive-a-write' {
    Assert-Exit (Run 'add' 't-a') 0
    $text = [IO.File]::ReadAllText($script:Ctx.Config)
    $text = '{' + "`n  `"owner`": `"lower`",`n  `"Owner`": `"upper`"," + $text.TrimStart().Substring(1)
    [IO.File]::WriteAllText($script:Ctx.Config, $text)
    Assert-Exit (Run 'set' 't-a' '-Notes' 'x') 0
    $after = [IO.File]::ReadAllText($script:Ctx.Config)
    Assert-That ($after -cmatch '"owner": "lower"' -and $after -cmatch '"Owner": "upper"') "a key was dropped: $(Format-Snip $after)"
}

Add-Case 'a-utf16-config-is-read' {
    Assert-Exit (Run 'add' 't-a') 0
    $text = [IO.File]::ReadAllText($script:Ctx.Config)
    [IO.File]::WriteAllText($script:Ctx.Config, $text, [Text.UnicodeEncoding]::new($false, $true))
    Assert-Exit (Run 'validate') 0
    Assert-Exit (Run 't-a' '-DryRun') 0
}

Add-Case 'a-name-with-a-line-feed-is-refused' {
    $r = Run 'add' "t-n`n"
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'not a valid name') "wrong refusal $(Format-Snip $r.Err)"
    Assert-That (-not @((Read-CaseConfig)['profiles'].Keys).Count) 'a profile was saved'
}

# Every shape rule of Test-Config, each in a config that is valid but for it.
Add-Case 'validation-refuses-each-broken-shape' {
    $r = Invoke-Unit {
        $root = [IO.Path]::GetDirectoryName($script:ConfigFile)
        $p = $root.Replace('\', '/') + '/profiles'
        $base = '{ "version": 1, "root": "' + $p + '", "profiles": { "t-a": { "userDataDir": "' + $p + '/t-a" } } }'
        $mail = 'd' + '@' + 'example.com'
        $J = { param($text) ConvertFrom-JsonText $text 'row' }
        $rows = [ordered]@{
            'version'          = @({ param($c) $c['version'] = 'x' }, 'version: must be a positive integer')
            'root-empty'       = @({ param($c) $c['root'] = '' }, 'root: must be a non-empty path')
            'root-relative'    = @({ param($c) $c['root'] = 'relative' }, 'root: .*must be absolute')
            'envMode'          = @({ param($c) $c['envMode'] = 'weird' }, "envMode: must be 'clean' or 'inherit'")
            'scrubEnv'         = @({ param($c) $c['scrubEnv'] = 'CLAUDE*' }, 'scrubEnv: must be an array')
            'env-name'         = @({ param($c) $c['env'] = @{ '1BAD' = 'x' } }, "env: '1BAD' is not a valid variable name")
            'env-value'        = @({ param($c) $c['env'] = @{ 'A' = 1 } }, 'env\.A: value must be a string')
            'env-case'         = @({ param($c) $c['env'] = & $J '{ "Path": "a", "PATH": "b" }' }, 'name the same variable')
            'env-managed'      = @({ param($c) $c['env'] = @{ 'CLAUDE_CONFIG_DIR' = 'x' } }, 'CLAUDE_CONFIG_DIR is managed')
            'app-shape'        = @({ param($c) $c['app'] = 'x' }, 'app: must be an object')
            'app-exe'          = @({ param($c) $c['app'] = @{ exePath = 1; extraArgs = @() } }, 'app\.exePath: must be a path string')
            'app-args'         = @({ param($c) $c['app'] = @{ exePath = $null; extraArgs = @('--user-data-dir=x') } }, 'app\.extraArgs: must not contain --user-data-dir')
            'launch-seconds'   = @({ param($c) $c['launch'] = @{ verifySeconds = 999; via = 'cmd' } }, 'launch\.verifySeconds: must be an integer')
            'launch-via'       = @({ param($c) $c['launch'] = @{ verifySeconds = 1; via = 'x' } }, "launch\.via: must be 'cmd' or 'direct'")
            'key-case-top'     = @({ param($c) $c['Root'] = 'x' }, 'Root: keys are case-sensitive')
            'key-case-app'     = @({ param($c) $c['app'] = & $J '{ "ExePath": null, "extraArgs": [] }' }, 'app\.ExePath: keys are case-sensitive')
            'key-case-profile' = @({ param($c) $c['profiles']['t-a']['Email'] = 'x' }, 'profiles\.t-a\.Email: keys are case-sensitive')
            'name-invalid'     = @({ param($c) $c['profiles']['bad name'] = @{ userDataDir = "$p/bn" } }, "'bad name' is not a valid name")
            'name-dot'         = @({ param($c) $c['profiles']['t-b.'] = @{ userDataDir = "$p/tb" } }, "'t-b\.' is not a valid name")
            'name-line-feed'   = @({ param($c) $c['profiles']["t-b`n"] = @{ userDataDir = "$p/tb" } }, 'is not a valid name')
            'name-reserved'    = @({ param($c) $c['profiles']['list'] = @{ userDataDir = "$p/l" } }, "'list' is reserved")
            'alias-reserved'   = @({ param($c) $c['profiles']['t-a']['aliases'] = @('ls') }, "'ls' is reserved")
            'alias-invalid'    = @({ param($c) $c['profiles']['t-a']['aliases'] = @('a b') }, "'a b' is not a valid alias")
            'not-object'       = @({ param($c) $c['profiles']['t-b'] = 'x' }, 'profiles\.t-b: must be an object')
            'notes'            = @({ param($c) $c['profiles']['t-a']['notes'] = 1 }, 'notes: must be a string')
            'args-shape'       = @({ param($c) $c['profiles']['t-a']['args'] = 'x' }, 'args: must be an array of strings')
            'args-folder'      = @({ param($c) $c['profiles']['t-a']['args'] = @('--user-data-dir=x') }, 'args: must not contain --user-data-dir')
            'share-bool'       = @({ param($c) $c['profiles']['t-a']['shareClaudeConfig'] = 'yes' }, 'shareClaudeConfig: must be true or false')
            'udd-required'     = @({ param($c) $c['profiles']['t-a'].Remove('userDataDir') }, 'userDataDir: required')
            'udd-relative'     = @({ param($c) $c['profiles']['t-a']['userDataDir'] = 'relative' }, 'must be absolute')
            'share-and-dir'    = @({ param($c) $c['profiles']['t-a']['shareClaudeConfig'] = $true; $c['profiles']['t-a']['claudeConfigDir'] = "$p/x" }, 'not both')
            'udd-in-ccd'       = @({ param($c) $c['profiles']['t-a']['userDataDir'] = "$p/x/in"; $c['profiles']['t-a']['claudeConfigDir'] = "$p/x" }, 'must not be the same as, or inside, claudeConfigDir')
            'drive-root'       = @({ param($c) $c['profiles']['t-a']['userDataDir'] = 'Z:\' }, 'cannot be a drive root')
            'network'          = @({ param($c) $c['profiles']['t-a']['userDataDir'] = '\\server\share\x' }, 'not on a drive letter')
            'device-prefix'    = @({ param($c) $c['profiles']['t-a']['userDataDir'] = '\\?\' + [IO.Path]::Combine($env:LOCALAPPDATA, 'zz') }, 'overlaps AppData\\Local')
            'roaming'          = @({ param($c) $c['profiles']['t-a']['userDataDir'] = [IO.Path]::Combine($env:APPDATA, 'zz') }, 'overlaps AppData\\Roaming')
            'claude-dir'       = @({ param($c) $c['profiles']['t-a']['claudeConfigDir'] = [IO.Path]::Combine($HOME, '.claude') }, 'overlaps the default Claude Code config')
            'home'             = @({ param($c) $c['profiles']['t-a']['userDataDir'] = $HOME }, 'overlaps the default Claude Code config')
            'tool-data'        = @({ param($c) $c['profiles']['t-a']['userDataDir'] = $script:DataDir }, "this tool's own data folder")
            'windows'          = @({ param($c) $c['profiles']['t-a']['userDataDir'] = [IO.Path]::Combine($env:SystemRoot, 'zz') }, 'the Windows folder')
            'install-folder'   = @({ param($c) $c['profiles']['t-a']['userDataDir'] = [IO.Path]::Combine($env:ProgramFiles, 'WindowsApps', 'zz') }, "the app's install folder")
            'email-invalid'    = @({ param($c) $c['profiles']['t-a']['email'] = 'nope' }, 'is not a valid email address')
            'email-twice'      = @({ param($c) $c['profiles']['t-a']['email'] = $mail; $c['profiles']['t-b'] = @{ userDataDir = "$p/t-b"; email = $mail } }, "email '.+' is already used by profile 't-a'")
            'alias-is-a-name'  = @({ param($c) $c['profiles']['t-b'] = @{ userDataDir = "$p/t-b"; aliases = @('t-a') } }, "alias 't-a' is already used by profile 't-a'")
            'alias-default'    = @({ param($c) $c['profiles']['t-a']['aliases'] = @('default') }, "already used by the built-in 'default' profile")
            'name-case'        = @({ param($c) $c['profiles']['T-A'] = @{ userDataDir = "$p/T-A2" } }, "name 'T-A' is already used by profile 't-a'")
            'default-folders'  = @({ param($c) $c['profiles']['default'] = @{ userDataDir = "$p/d" } }, 'profiles\.default\.userDataDir: not allowed')
            'overlap'          = @({ param($c) $c['profiles']['t-b'] = @{ userDataDir = "$p/t-a/in" } }, 'overlap')
        }
        $baseErrors = @((Test-Config (Add-ConfigDefaults (& $J $base))).Errors)
        if ($baseErrors.Count) { "BASE-ERRORS: $($baseErrors -join ' | ')" }
        foreach ($k in $rows.Keys) {
            $c = & $J $base
            & $rows[$k][0] $c
            $errs = @((Test-Config (Add-ConfigDefaults $c)).Errors)
            if (-not @($errs | Where-Object { $_ -match $rows[$k][1] }).Count) { "MISS: $k -> [$($errs -join ' | ')]" }
        }
        "ROWS=$($rows.Count)"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -notmatch 'BASE-ERRORS') "the base config is not valid: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -notmatch 'MISS:') "rules that did not refuse: $(Format-Snip (($r.Out -split '\r?\n' | Where-Object { $_ -match 'MISS:' }) -join ' '))"
    Assert-That ($r.Out -match 'ROWS=\d+') "the table did not run: $(Format-Snip $r.Out)"
}

Add-Case 'seed-copies-settings-and-never-credentials' {
    Assert-Exit (Run 'add' 't-a') 0
    $src = Join-Path $script:Ctx.Dir 'src'
    [void][IO.Directory]::CreateDirectory((Join-Path $src 'skills\demo'))
    [void][IO.Directory]::CreateDirectory((Join-Path $src 'projects\p1'))
    [IO.File]::WriteAllText((Join-Path $src 'CLAUDE.md'), '# test')
    [IO.File]::WriteAllText((Join-Path $src 'skills\demo\SKILL.md'), 'x')
    [IO.File]::WriteAllText((Join-Path $src '.credentials.json'), '{}')
    [IO.File]::WriteAllText((Join-Path $src 'projects\p1\t.jsonl'), '{}')
    Assert-Exit (Run 'seed' 't-a' '-SeedFrom' $src) 0
    $dest = Join-Path $script:Ctx.Root 't-a\.claude'
    Assert-That ([IO.File]::Exists((Join-Path $dest 'skills\demo\SKILL.md'))) 'skills were not copied'
    Assert-That (-not [IO.File]::Exists((Join-Path $dest '.credentials.json'))) 'credentials were copied'
    $again = Run 'seed' 't-a' '-SeedFrom' $src
    Assert-Exit $again 0
    Assert-That ($again.Out -match 'keep\s+CLAUDE.md') 'a second seed did not keep what was there'
    $r = Run 'seed' 't-a' '-SeedFrom' $src '-Include' '.credentials.json'
    Assert-Exit $r 1
    # Windows opens ".credentials.json." as .credentials.json, so a spelling that only
    # differs by a trailing dot or space must be refused as well.
    foreach ($bad in '.credentials.json.', '.credentials.json ', 'projects.', '...', 'skills\demo') {
        $r = Run 'seed' 't-a' '-SeedFrom' $src '-Include' $bad
        Assert-Exit $r 1
    }
    Assert-That (-not [IO.File]::Exists((Join-Path $dest '.credentials.json'))) 'credentials were copied on request'
    Assert-That (-not [IO.Directory]::Exists((Join-Path $dest 'projects'))) 'a session history was copied on request'
}

Add-Case 'seed-refuses-a-folder-the-config-may-not-name' {
    Assert-Exit (Run 'add' 't-a') 0
    $src = Join-Path $script:Ctx.Dir 'src'
    [void][IO.Directory]::CreateDirectory($src)
    [IO.File]::WriteAllText((Join-Path $src 'CLAUDE.md'), '# test')
    $c = Read-CaseConfig
    $c['profiles']['t-a']['claudeConfigDir'] = $script:Ctx.DataDir.Replace('\', '/')
    Set-CaseConfig $c
    $r = Run 'seed' 't-a' '-SeedFrom' $src
    Assert-Exit $r 1
    Assert-That ($r.Err -match "this tool's own data folder") "wrong refusal $(Format-Snip $r.Err)"
    Assert-That (-not [IO.File]::Exists((Join-Path $script:Ctx.DataDir 'CLAUDE.md'))) 'seed wrote into the tool''s data folder'
}

Add-Case 'discover-finds-and-registers-an-unregistered-folder' {
    $stray = Join-Path $script:Ctx.Root 't-stray'
    [void][IO.Directory]::CreateDirectory($stray)
    [IO.File]::WriteAllText((Join-Path $stray 'Local State'), '{}')
    $r = Run 'discover'
    Assert-Exit $r 0
    Assert-That ($r.Out -match 't-stray') "discover did not list it: $(Format-Snip $r.Out)"
    Assert-Exit (Run 'discover' '-Register') 0
    Assert-That ((Read-CaseConfig)['profiles'].Contains('t-stray')) 'it was not registered'
}

Add-Case 'discover-registers-a-folder-with-a-long-name' {
    $long = 'p' + ('x' * 72)
    $stray = Join-Path $script:Ctx.Root $long
    [void][IO.Directory]::CreateDirectory($stray)
    [IO.File]::WriteAllText((Join-Path $stray 'Local State'), '{}')
    $r = Run 'discover' '-Register'
    Assert-Exit $r 0
    # Discover also registers whatever instance runs on this machine, so only the
    # folder made here is looked for.
    $all = @((Read-CaseConfig)['profiles'].Keys)
    $names = @($all | Where-Object { $_.Length -le 64 -and $long.StartsWith($_) })
    Assert-That ($names.Count -eq 1) "registered as [$($all -join '|')]: $(Format-Snip ($r.Out + $r.Err))"
}

Add-Case 'remove-keeps-data-and-purge-moves-it-to-trash' {
    Assert-Exit (Run 'add' 't-a') 0
    $udd = Join-Path $script:Ctx.Root 't-a'
    [IO.File]::WriteAllText((Join-Path $udd 'Local State'), '{}')
    Assert-Exit (Run 'remove' 't-a') 0
    Assert-That ([IO.File]::Exists((Join-Path $udd 'Local State'))) 'remove without -PurgeData deleted data'
    Assert-Exit (Run 'add' 't-a') 0
    Assert-Exit (Run 'remove' 't-a' '-PurgeData' '-Force') 0
    Assert-That (-not [IO.Directory]::Exists($udd)) 'the folder is still there'
    $trashed = @(Get-ChildItem (Join-Path $script:Ctx.Root '.cc-trash') -Directory -Filter 't-a-*')
    Assert-That ($trashed.Count -eq 1 -and [IO.File]::Exists((Join-Path $trashed[0].FullName 'Local State'))) 'the data is not in .cc-trash'
}

# Two folders of one profile side by side are moved in the same second. Each keeps
# its own name in .cc-trash, so the second move does not collide with the first.
Add-Case 'purge-moves-both-folders-side-by-side' {
    $udd = Join-Path $script:Ctx.Root 't-a'
    $ccd = Join-Path $script:Ctx.Root 't-a-cc'
    Assert-Exit (Run 'add' 't-a' '-ClaudeConfigDir' $ccd) 0
    Assert-That ((Read-CaseConfig)['profiles']['t-a']['claudeConfigDir'] -like '*/t-a-cc') '-ClaudeConfigDir was not stored'
    [IO.File]::WriteAllText((Join-Path $udd 'Local State'), '{}')
    [IO.File]::WriteAllText((Join-Path $ccd 'settings.json'), '{}')
    $r = Run 'remove' 't-a' '-PurgeData' '-Force'
    Assert-Exit $r 0
    Assert-That (-not [IO.Directory]::Exists($udd) -and -not [IO.Directory]::Exists($ccd)) "a folder was left behind: $(Format-Snip $r.Out)"
    $trashed = @(Get-ChildItem (Join-Path $script:Ctx.Root '.cc-trash') -Directory)
    Assert-That ($trashed.Count -eq 2) "the trash holds [$(@($trashed | ForEach-Object Name) -join '|')]"
    Assert-That (-not (Read-CaseConfig)['profiles'].Contains('t-a')) 'the entry is still there'
}

# An earlier purge of the same profile already holds the name this one would take,
# for this second and the next few, so the move has to find another.
Add-Case 'a-purge-does-not-collide-with-an-earlier-one' {
    $r = Invoke-Unit {
        $parent = [IO.Path]::Combine([IO.Path]::GetDirectoryName($script:ConfigFile), 'p')
        $trash = [IO.Path]::Combine($parent, '.cc-trash')
        $now = [DateTime]::Now
        foreach ($s in 0..3) {
            $stamp = $now.AddSeconds($s).ToString('yyyyMMdd-HHmmss', [Globalization.CultureInfo]::InvariantCulture)
            [void][IO.Directory]::CreateDirectory([IO.Path]::Combine($trash, "t-a-$stamp"))
        }
        $dir = [IO.Path]::Combine($parent, 't-a')
        [void][IO.Directory]::CreateDirectory($dir)
        [IO.File]::WriteAllText([IO.Path]::Combine($dir, 'f.txt'), 'x')
        Move-ToTrash $dir 't-a' 6>$null
        "MOVED=$(-not [IO.Directory]::Exists($dir))"
        "TRASHED=$(@([IO.Directory]::GetDirectories($trash)).Count)"
    }
    Assert-That ($r.Out -match 'MOVED=True' -and $r.Out -match 'TRASHED=5') "a purge collided with an earlier one: $(Format-Snip ($r.Out + $r.Err))"
    Assert-Exit $r 0
}

# A hand edit can name the tool's own data folder as a profile folder. Purging that
# profile must refuse before anything moves, and keep its entry.
Add-Case 'purge-refuses-a-folder-it-must-not-move' {
    Assert-Exit (Run 'add' 't-a') 0
    $marker = Join-Path $script:Ctx.DataDir 'marker.txt'
    [IO.File]::WriteAllText($marker, 'keep')
    $c = Read-CaseConfig
    $c['profiles']['t-a']['userDataDir'] = $script:Ctx.DataDir.Replace('\', '/')
    $c['profiles']['t-a']['claudeConfigDir'] = (Join-Path $script:Ctx.Root 't-a\.claude').Replace('\', '/')
    Set-CaseConfig $c
    $r = Run 'remove' 't-a' '-PurgeData' '-Force'
    Assert-Exit $r 1
    Assert-That ($r.Err -match "this tool's own data folder") "wrong refusal $(Format-Snip $r.Err)"
    Assert-That ([IO.File]::Exists($marker)) 'the marker in the data folder was moved'
    Assert-That ((Read-CaseConfig)['profiles'].Contains('t-a')) 'the entry was removed although nothing moved'
}

Add-Case 'rename-moves-the-entry-and-refuses-bad-names' {
    Assert-Exit (Run 'add' 't-a' '-Alias' 'x' '-Notes' 'kept') 0
    Assert-Exit (Run 'rename' 't-a' 't-b') 0
    $c = Read-CaseConfig
    Assert-That (-not $c['profiles'].Contains('t-a')) 't-a is still there'
    Assert-That ($c['profiles'].Contains('t-b') -and $c['profiles']['t-b']['notes'] -eq 'kept' -and @($c['profiles']['t-b']['aliases']) -contains 'x') 't-b did not take over the entry'
    $r = Run 'rename' 't-b'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'Usage: rename') "no usage line: $(Format-Snip $r.Err)"
    $r = Run 'rename' 't-b' 'list'
    Assert-Exit $r 1
    Assert-That ($r.Err -match "'list' is reserved") "wrong refusal $(Format-Snip $r.Err)"
    $r = Run 'rename' 't-b' 'default'
    Assert-Exit $r 1
    Assert-That ((Read-CaseConfig)['profiles'].Contains('t-b')) 'a refused rename changed the entry'
}

# Refusals that change nothing, each with the words that say why.
Add-Case 'refusals-say-what-they-refuse' {
    $foreign = Join-Path $script:Ctx.Dir 'not-a-profile'
    [void][IO.Directory]::CreateDirectory($foreign)
    [IO.File]::WriteAllText((Join-Path $foreign 'mine.txt'), 'x')
    $r = Run 'add' 't-x' '-UserDataDir' $foreign
    Assert-Exit $r 1
    Assert-That ($r.Err -match "doesn't look like a Claude profile folder") "add over a foreign folder: $(Format-Snip $r.Err)"
    Assert-Exit (Run 'add' 't-a') 0
    [IO.File]::WriteAllText((Join-Path $script:Ctx.Root 't-a\Local State'), '{}')
    $r = Run 'set' 't-a' '-UserDataDir' (Join-Path $script:Ctx.Root 't-moved')
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'does not move data') "set over data: $(Format-Snip $r.Err)"
    $r = Run 'show' 't-a' 'extra'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'Unexpected argument') "an extra argument: $(Format-Snip $r.Err)"
    $r = Run 'restore' '99'
    Assert-Exit $r 1
    Assert-That ($r.Err -match 'No backup number 99') "restore 99: $(Format-Snip $r.Err)"
    Assert-Exit (Run 'set' 't-a' '-SetEnv' 'API_TOKEN=s3cret') 0
    $r = Run 't-a' '-DryRun'
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'API_TOKEN=<redacted>' -and $r.Out -notmatch 's3cret') "the dry run shows a secret: $(Format-Snip $r.Out)"
    # A list where the profiles object belongs is refused by name, not by a crash.
    [IO.File]::WriteAllText($script:Ctx.Config, '{ "version": 1, "profiles": [] }')
    $r = Run 'list'
    Assert-Exit $r 1
    Assert-That ($r.Err -match "'profiles' must be an object") "a profiles list: $(Format-Snip $r.Err)"
}

Add-Case 'backup-takes-a-copy-on-request' {
    Assert-Exit (Run 'add' 't-a') 0
    $r = Run 'backup'
    Assert-Exit $r 0
    $manual = @(Get-ChildItem (Join-Path $script:Ctx.DataDir 'backups') -File -Filter '*.manual.json')
    Assert-That ($manual.Count -eq 1) "manual backups: $($manual.Count)"
    Assert-That ((Get-FileHash $manual[0].FullName).Hash -eq (Get-FileHash $script:Ctx.Config).Hash) 'the backup differs from the config'
}

Add-Case 'edit-saves-a-valid-change' {
    Assert-Exit (Run 'add' 't-a') 0
    $editor = Join-Path $script:Ctx.Dir 'editor.ps1'
    $lines = @(
        'param([string]$Path)'
        'Start-Sleep -Milliseconds 3200'
        '$j = Get-Content -Raw $Path | ConvertFrom-Json -AsHashtable'
        '$j.profiles[''t-a''].notes = ''edited'''
        '[IO.File]::WriteAllText($Path, ($j | ConvertTo-Json -Depth 10))'
    )
    [IO.File]::WriteAllText($editor, ($lines -join "`n") + "`n")
    $script:Ctx.ExtraEnv = @{ EDITOR = "`"$($script:Pwsh)`" -NoProfile -File `"$editor`""; VISUAL = $null }
    Assert-Exit (Run 'edit') 0
    Assert-That ((Read-CaseConfig)['profiles']['t-a']['notes'] -eq 'edited') 'the edit was not saved'
}

Add-Case 'json-output-stays-pure-when-messages-are-printed' {
    $c = Read-CaseConfig
    $c['profiles']['t-broken'] = @{ userDataDir = 'relative\path' }
    Set-CaseConfig $c
    $r = Run 'list' '-Json'
    Assert-Exit $r 0
    $parsed = $null
    try { $parsed = $r.Out | ConvertFrom-Json } catch { throw "stdout is not JSON: $(Format-Snip $r.Out)" }
    Assert-That (@($parsed).Count -ge 1) 'the JSON is empty'
    Assert-That ($r.Err -match 'Skipping profile') "the warning did not go to stderr: $(Format-Snip $r.Err)"
    # -Verbose detail goes to stderr too; an unreadable state file produces some.
    [void][IO.Directory]::CreateDirectory($script:Ctx.DataDir)
    [IO.File]::WriteAllText((Join-Path $script:Ctx.DataDir 'state.json'), '{ not json')
    $v = Run 'list' '-Json' '-Verbose'
    Assert-Exit $v 0
    try { $null = $v.Out | ConvertFrom-Json } catch { throw "with -Verbose, stdout is not JSON: $(Format-Snip $v.Out)" }
    Assert-That ($v.Err -match 'VERBOSE: Ignoring unreadable state file') "the verbose line did not go to stderr: $(Format-Snip $v.Err)"
}

Add-Case 'times-print-the-same-in-every-culture' {
    $r = Invoke-Unit {
        & {
            [Globalization.CultureInfo]::CurrentCulture = [Globalization.CultureInfo]::new('fi-FI')
            "WHEN=$(Format-When '2026-01-02T03:04:05')"
        }
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'WHEN=2026-01-02 03:04') "a time printed under fi-FI: $(Format-Snip $r.Out)"
}

Add-Case 'command-lines-round-trip' {
    $r = Invoke-Unit {
        $samples = @('plain', 'with space', 'trail\', 'sp trail\', 'q"uote', 'a\"b', '', '--user-data-dir=C:\a b\c\')
        $line = ($samples | ForEach-Object { Format-CommandLineArg $_ }) -join ' '
        $back = @(Split-CommandLine $line)
        $bad = 0
        for ($i = 0; $i -lt $samples.Count; $i++) { if ($samples[$i] -cne $back[$i]) { $bad++ } }
        if ($back.Count -ne $samples.Count) { $bad++ }
        if ((Get-UserDataDirArg @(Split-CommandLine '"C:\x\Claude.exe" --user-data-dir="C:\a b\c" --y')) -ne 'C:\a b\c') { $bad++ }
        if ((Get-UserDataDirArg @(Split-CommandLine 'C:\x\Claude.exe --user-data-dir C:\plain')) -ne 'C:\plain') { $bad++ }
        if (@(Split-CommandLine '').Count -ne 0) { $bad++ }
        "BAD=$bad"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'BAD=0') "round trip failed: $(Format-Snip $r.Out)"
}

# Outside the root by "..", in a sibling whose name starts with the root's, and the
# root itself spelled three ways: every one refused, and only the file inside removed.
Add-Case 'removal-outside-its-root-is-refused' {
    $r = Invoke-Unit {
        $base = [IO.Path]::GetDirectoryName($script:ConfigFile)
        $root = [IO.Path]::Combine($base, 'inside')
        $sibling = [IO.Path]::Combine($base, 'inside-sibling')
        [void][IO.Directory]::CreateDirectory($root)
        [void][IO.Directory]::CreateDirectory($sibling)
        $outside = [IO.Path]::Combine($base, 'outside.txt')
        $near = [IO.Path]::Combine($sibling, 'near.txt')
        $kept = [IO.Path]::Combine($root, 'kept.txt')
        foreach ($f in $outside, $near, $kept) { [IO.File]::WriteAllText($f, 'keep') }
        foreach ($p in @($outside, "$root\..\outside.txt", $near, "$root\...", "$root\.", $root)) {
            try { Remove-Contained $p $root; "REMOVED: $p" } catch { "REFUSED: $p" }
        }
        "OUTSIDE=$([IO.File]::Exists($outside)) NEAR=$([IO.File]::Exists($near)) ROOT=$([IO.File]::Exists($kept))"
        [void][IO.Directory]::CreateDirectory($root)
        $inner = [IO.Path]::Combine($root, 'gone.txt')
        [IO.File]::WriteAllText($inner, 'x')
        Remove-Contained $inner $root
        "INNER-GONE=$(-not [IO.File]::Exists($inner))"
    }
    # Before the exit code: a mutant that removed the root also breaks what follows.
    Assert-That ($r.Out -notmatch 'REMOVED') "a removal outside its root, or of the root, went ahead: $(Format-Snip $r.Out)"
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'INNER-GONE=True') 'a removal inside its root did not happen'
    Assert-That ($r.Out -match 'OUTSIDE=True NEAR=True ROOT=True') "something outside, or the root, was removed: $(Format-Snip $r.Out)"
}

Add-Case 'the-clean-environment-is-the-users-not-systems' {
    $r = Invoke-Unit {
        $e = Get-BaseEnvironment 'clean'
        "USER=$($e['USERNAME'])|$env:USERNAME"
        "HOME=$($e['USERPROFILE'])|$env:USERPROFILE"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match '(?m)^USER=([^|\r\n]+)\|\1\s*$') "USERNAME differs: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match '(?m)^HOME=([^|\r\n]+)\|\1\s*$') "USERPROFILE differs: $(Format-Snip $r.Out)"
}

Add-Case 'install-writes-an-identical-copy-and-a-cmd-entry' {
    $bin = Join-Path $script:Ctx.Dir 'bin'
    $r = Run 'install' '-Destination' $bin
    Assert-Exit $r 0
    $copy = Join-Path $bin 'cc_profile.ps1'
    Assert-That ((Get-FileHash $copy).Hash -eq (Get-FileHash $script:ToolPath).Hash) 'the installed copy differs from the source'
    $shim = Join-Path $bin 'cc_profile.cmd'
    Assert-That ([IO.File]::ReadAllText($shim).Contains('%~dp0cc_profile.ps1')) 'the cmd entry does not point beside itself'
    Assert-That ($r.Out -match 'not on your PATH') 'a folder off PATH was not reported'
    $again = Run 'install' '-Destination' $bin
    Assert-Exit $again 0
    Assert-That ($again.Out -match 'identical: .*cc_profile\.ps1') "a second install rewrote the copy: $(Format-Snip $again.Out)"
    $psi = [Diagnostics.ProcessStartInfo]::new((Join-Path ([Environment]::SystemDirectory) 'cmd.exe'))
    $psi.Arguments = "/d /s /c `"`"$shim`" version`""
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.Environment['CC_PROFILES_CONFIG'] = $script:Ctx.Config
    $p = [Diagnostics.Process]::Start($psi)
    $out = $p.StandardOutput.ReadToEnd()
    $p.WaitForExit()
    Assert-That ($p.ExitCode -eq 0 -and $out -match 'cc_profile \d') "the cmd entry answered $($p.ExitCode): $(Format-Snip $out)"
}

Add-Case 'the-profile-block-is-replaced-not-duplicated' {
    $r = Invoke-Unit {
        $pf = [IO.Path]::Combine([IO.Path]::GetDirectoryName($script:ConfigFile), 'profile.ps1')
        [IO.File]::WriteAllText($pf, "# mine`nSet-Alias ll Get-ChildItem`n")
        Update-PowerShellProfile 'C:\tools\cc_profile.ps1' -ProfilePath $pf -AliasName 'ccp' 6>$null
        Update-PowerShellProfile 'C:\tools\cc_profile.ps1' -ProfilePath $pf -AliasName 'ccp' 6>$null
        Update-PowerShellProfile 'C:\tools\cc_profile.ps1' -ProfilePath $pf -AliasName 'ccx' 6>$null
        $text = [IO.File]::ReadAllText($pf)
        "BLOCKS=$(([regex]::Matches($text, '# >>> cc_profile >>>')).Count)"
        "MINE=$($text.Contains('Set-Alias ll Get-ChildItem'))"
        "LATEST=$($text.Contains('Set-Alias -Name ccx'))"
        "OLD=$($text.Contains('Set-Alias -Name ccp'))"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'BLOCKS=1' -and $r.Out -match 'MINE=True' -and $r.Out -match 'LATEST=True' -and $r.Out -match 'OLD=False') "profile block: $(Format-Snip $r.Out)"
}

# The block runs in every new shell: it must parse, leave the user's own lines
# running, and complete. The script path holds a typographic quote, which PowerShell
# also reads as a quote.
Add-Case 'the-profile-block-parses-runs-and-completes' {
    $r = Invoke-Unit {
        $dir = [IO.Path]::GetDirectoryName($script:ConfigFile)
        $pf = [IO.Path]::Combine($dir, 'profile.ps1')
        [IO.File]::WriteAllText($pf, "# mine`n`$env:CC_TEST_MINE = 'yes'`n")
        $toolDir = [IO.Path]::Combine($dir, ('tools' + [char]0x2019 + 's'))
        [void][IO.Directory]::CreateDirectory($toolDir)
        $tool = [IO.Path]::Combine($toolDir, 'cc_profile.ps1')
        [IO.File]::WriteAllText($tool, "param([Parameter(Position = 0)][string]`$Command, [Parameter(Position = 1)][string]`$Target)`n")
        Update-PowerShellProfile $tool -ProfilePath $pf -AliasName 'ccp' 6>$null
        $text = [IO.File]::ReadAllText($pf)
        $tk = $null; $pe = $null
        [void][Management.Automation.Language.Parser]::ParseInput($text, [ref]$tk, [ref]$pe)
        "PARSE=$(@($pe).Count)"
        . $pf
        "MINE=$env:CC_TEST_MINE"
        "ALIAS=$((Get-Alias ccp).Definition -eq $tool)"
        $c = TabExpansion2 -inputScript 'ccp li' -cursorColumn 6
        "COMPLETES=$(@($c.CompletionMatches | ForEach-Object CompletionText) -contains 'list')"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'PARSE=0') "the profile does not parse: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match 'MINE=yes' -and $r.Out -match 'ALIAS=True') "the profile did not run whole: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match 'COMPLETES=True') "the command name does not complete: $(Format-Snip $r.Out)"
}

Add-Case 'a-profile-block-that-would-not-parse-is-not-written' {
    $r = Invoke-Unit {
        $pf = [IO.Path]::Combine([IO.Path]::GetDirectoryName($script:ConfigFile), 'profile.ps1')
        [IO.File]::WriteAllText($pf, "# mine`n")
        function Get-ProfileBlock { "# >>> cc_profile >>>`nSet-Alias -Name ccp -Value 'unterminated`n# <<< cc_profile <<<" }
        try { Update-PowerShellProfile 'C:\tools\cc_profile.ps1' -ProfilePath $pf -AliasName 'ccp' 6>$null; 'WRITTEN' }
        catch { 'REFUSED: ' + $_.Exception.Message }
        "UNCHANGED=$([IO.File]::ReadAllText($pf) -ceq "# mine`n")"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'REFUSED: .*would not parse') "a broken block was not refused: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match 'UNCHANGED=True') 'the profile changed'
}

Add-Case 'a-shortcut-runs-the-script-it-was-made-from' {
    Assert-Exit (Run 'add' 't-a') 0
    $r = Invoke-Unit {
        $dir = [IO.Path]::Combine([IO.Path]::GetDirectoryName($script:ConfigFile), 'shortcuts')
        $script:ScriptPath = 'C:\tools\cc_profile.ps1'
        function Find-ClaudeApp { [pscustomobject]@{ Exe = (Get-Process -Id $PID).Path; Version = '0'; Source = 'stub'; InstallDir = $null; LogoDirs = @() } }
        $cfg = (Read-Config).Config
        Set-ProfileShortcut (Get-ProfileView $cfg 't-a') (Find-ClaudeApp $cfg) @($dir) 6>$null
        $lnk = (New-Object -ComObject WScript.Shell).CreateShortcut([IO.Path]::Combine($dir, 'Claude (t-a).lnk'))
        "ARGS=$($lnk.Arguments)"
        "OWNED=$(Test-ShortcutOwned ([IO.Path]::Combine($dir, 'Claude (t-a).lnk')))"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match [regex]::Escape('-File "C:\tools\cc_profile.ps1" launch "t-a"')) "shortcut arguments: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match [regex]::Escape("-ConfigPath `"$($script:Ctx.Config)`"")) 'the config is not carried'
    Assert-That ($r.Out -match 'OWNED=True') 'the tool does not recognise its own shortcut'
}

# Shortcuts are named for the profile only, so a profile of the same name in another
# config has a shortcut of the same name. That one is never rewritten or removed.
Add-Case 'shortcuts-of-another-config-are-left-alone' {
    Assert-Exit (Run 'add' 't-a') 0
    $r = Invoke-Unit {
        $base = [IO.Path]::GetDirectoryName($script:ConfigFile)
        $mine = [IO.Path]::Combine($base, 'mine')
        $theirs = [IO.Path]::Combine($base, 'theirs')
        function Get-ShortcutDirs([string]$Where) { $null = $Where; return @($mine, $theirs) }
        function Find-ClaudeApp { [pscustomobject]@{ Exe = (Get-Process -Id $PID).Path; Version = '0'; Source = 'stub'; InstallDir = $null; LogoDirs = @() } }
        $script:ScriptPath = 'C:\tools\cc_profile.ps1'
        $cfg = (Read-Config).Config
        Set-ProfileShortcut (Get-ProfileView $cfg 't-a') (Find-ClaudeApp $cfg) @($mine) 6>$null
        [void][IO.Directory]::CreateDirectory($theirs)
        $foreign = [IO.Path]::Combine($theirs, 'Claude (t-a).lnk')
        $s = (New-Object -ComObject WScript.Shell).CreateShortcut($foreign)
        $s.TargetPath = (Get-Process -Id $PID).Path
        $s.Arguments = '-File "C:\tools\cc_profile.ps1" launch "t-a" -FromShortcut -ConfigPath "C:\another\cfg.json"'
        $s.Save()
        try { Set-ProfileShortcut (Get-ProfileView $cfg 't-a') (Find-ClaudeApp $cfg) @($theirs) 6>$null; 'OVERWROTE' } catch { 'REFUSED-OVERWRITE' }
        $script:Tgt = 't-a'
        Invoke-Remove $cfg 6>$null
        "MINE-GONE=$(-not [IO.File]::Exists([IO.Path]::Combine($mine, 'Claude (t-a).lnk')))"
        "THEIRS-KEPT=$([IO.File]::Exists($foreign))"
        "THEIRS-ARGS=$((New-Object -ComObject WScript.Shell).CreateShortcut($foreign).Arguments)"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'REFUSED-OVERWRITE') "another config's shortcut was overwritten: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match 'MINE-GONE=True') "remove left this config's shortcut: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match 'THEIRS-KEPT=True' -and $r.Out -match [regex]::Escape('C:\another\cfg.json')) "remove touched another config's shortcut: $(Format-Snip $r.Out)"
}

# Rename refuses before it changes anything when the shortcuts cannot follow, and a
# rename that goes ahead writes the new shortcut before it removes the old one.
Add-Case 'rename-takes-its-shortcuts-or-changes-nothing' {
    Assert-Exit (Run 'add' 't-a') 0
    $r = Invoke-Unit {
        $dir = [IO.Path]::Combine([IO.Path]::GetDirectoryName($script:ConfigFile), 'shortcuts')
        function Get-ShortcutDirs([string]$Where) { $null = $Where; return @($dir) }
        function Find-ClaudeApp { [pscustomobject]@{ Exe = (Get-Process -Id $PID).Path; Version = '0'; Source = 'stub'; InstallDir = $null; LogoDirs = @() } }
        $script:ScriptPath = 'C:\tools\cc_profile.ps1'
        Set-ProfileShortcut (Get-ProfileView (Read-Config).Config 't-a') (Find-ClaudeApp $null) @($dir) 6>$null
        $script:ScriptPath = [IO.Path]::Combine($env:TEMP, 'cc-profile-test', 'cc_profile.ps1')
        $script:Tgt = 't-a'; $script:RestArgs = @('t-b')
        try { Invoke-Rename 6>$null; 'RENAMED-FROM-TEMP' } catch { 'REFUSED: ' + $_.Exception.Message }
        "KEPT-ENTRY=$((Read-Config).Config['profiles'].Contains('t-a'))"
        "KEPT-LINK=$([IO.File]::Exists([IO.Path]::Combine($dir, 'Claude (t-a).lnk')))"
        $script:ScriptPath = 'C:\tools\cc_profile.ps1'
        $script:Tgt = 't-a'; $script:RestArgs = @('t-b')
        Invoke-Rename 6>$null
        "NEW-ENTRY=$((Read-Config).Config['profiles'].Contains('t-b'))"
        "OLD-LINK-GONE=$(-not [IO.File]::Exists([IO.Path]::Combine($dir, 'Claude (t-a).lnk')))"
        "NEW-LINK=$(Test-ShortcutOwned ([IO.Path]::Combine($dir, 'Claude (t-b).lnk')))"
        "NEW-ARGS=$((New-Object -ComObject WScript.Shell).CreateShortcut([IO.Path]::Combine($dir, 'Claude (t-b).lnk')).Arguments)"
    }
    # Before the exit code: a rename that went ahead leaves the second one nothing to rename.
    Assert-That ($r.Out -match 'REFUSED: .*temporary folder') "a rename from a temporary copy was not refused: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match 'KEPT-ENTRY=True' -and $r.Out -match 'KEPT-LINK=True') "a refused rename changed something: $(Format-Snip $r.Out)"
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'NEW-ENTRY=True' -and $r.Out -match 'OLD-LINK-GONE=True' -and $r.Out -match 'NEW-LINK=True') "the rename did not take its shortcut: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match 'NEW-ARGS=.*launch "t-b"') "the new shortcut launches the wrong profile: $(Format-Snip $r.Out)"
}

Add-Case 'doctor-is-clean-on-a-healthy-config' {
    Assert-Exit (Run 'add' 't-a' '-Email' (Get-TestMail 'a')) 0
    $r = Run 'doctor'
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'No problems found') "doctor: $(Format-Snip $r.Out)"
}

# The launch logic, with the functions that find or start the app stood in for: no
# app starts here, and every start the launch asks for is counted.
Add-Case 'a-launch-starts-the-app-once-whatever-cmd-answers' {
    $r = Invoke-Unit {
        $script:Calls = [Collections.Generic.List[string]]::new()
        $script:Answer = $null
        function Invoke-NativeStart([string]$Exe, [string]$CommandLine, [string[]]$Environment, [string]$Directory, [bool]$HiddenConsole, [int]$WaitMs) {
            $null = $CommandLine, $Environment, $Directory, $HiddenConsole, $WaitMs
            $script:Calls.Add([IO.Path]::GetFileName($Exe))
            if ([IO.Path]::GetFileName($Exe) -eq 'cmd.exe') { return $script:Answer }
            return [pscustomobject]@{ Pid = 4343; BrokeAway = $true; Exited = $false; ExitCode = -1 }
        }
        $spec = [pscustomobject]@{ Exe = 'C:\x\Claude.exe'; Args = @('--user-data-dir=C:\p'); Env = @{ A = '1' }; WorkingDirectory = 'C:\' }
        $scenarios = [ordered]@{
            'unfinished' = [pscustomobject]@{ Pid = 4242; BrokeAway = $true; Exited = $false; ExitCode = -1 }
            'failed'     = [pscustomobject]@{ Pid = 4242; BrokeAway = $true; Exited = $true; ExitCode = 1 }
            'started'    = [pscustomobject]@{ Pid = 4242; BrokeAway = $false; Exited = $true; ExitCode = 0 }
        }
        foreach ($k in $scenarios.Keys) {
            $script:Calls.Clear(); $script:Answer = $scenarios[$k]
            $res = Start-DetachedProcess $spec 'cmd' 6>$null
            "$k=$($script:Calls -join '+')|$($res.Method)"
        }
        $script:Calls.Clear()
        $pct = [pscustomobject]@{ Exe = 'C:\x\Claude.exe'; Args = @('--user-data-dir=C:\100%'); Env = @{ A = '1' }; WorkingDirectory = 'C:\' }
        $res = Start-DetachedProcess $pct 'cmd' 6>$null
        "percent=$($script:Calls -join '+')|$($res.Method)"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match '(?m)^unfinished=cmd\.exe\|cmd start, unfinished') "an unfinished cmd start led to a second start: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match '(?m)^failed=cmd\.exe\+Claude\.exe\|CreateProcess') "a failed cmd start did not fall back to a direct start: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match '(?m)^started=cmd\.exe\|cmd start, orphaned and detached; job breakaway not allowed') "a started cmd start: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match '(?m)^percent=Claude\.exe\|CreateProcess') "an argument cmd cannot carry went through cmd: $(Format-Snip $r.Out)"
}

Add-Case 'an-elevated-shell-launches-only-with-allowelevated' {
    Assert-Exit (Run 'add' 't-a') 0
    $r = Invoke-Unit {
        $script:Starts = 0
        function Test-IsElevated { return $true }
        function Find-ClaudeApp { [pscustomobject]@{ Exe = 'C:\x\Claude.exe'; Version = '0'; Source = 'stub'; InstallDir = $null; LogoDirs = @() } }
        function Get-ClaudeInstances { return @() }
        function Start-DetachedProcess { $script:Starts++; [pscustomobject]@{ Pid = 1; Via = $null; Method = 'stub' } }
        $cfg = (Read-Config).Config
        # The script's own switches, which the tool reads from this scope.
        Set-Variable -Name NoVerify -Value $true -Scope Script
        # -Force answers "end a running instance"; it must not also allow elevation.
        Set-Variable -Name Force -Value $true -Scope Script
        try { Invoke-Launch $cfg 't-a' 6>$null; 'LAUNCHED-WITH-FORCE' } catch { 'REFUSED: ' + $_.Exception.Message }
        "STARTS=$($script:Starts)"
        Set-Variable -Name AllowElevated -Value $true -Scope Script
        Invoke-Launch $cfg 't-a' 6>$null
        "STARTS=$($script:Starts)"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'REFUSED: .*elevated') "an elevated launch with -Force was not refused: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match '(?s)STARTS=0.*STARTS=1') "starts: $(Format-Snip $r.Out)"
}

# What the launch reads back from the running app, each answer stood in for.
Add-Case 'a-launch-reads-back-what-it-claims' {
    Assert-Exit (Run 'add' 't-a') 0
    $r = Invoke-Unit {
        $cfg = (Read-Config).Config
        $cfg['launch']['verifySeconds'] = 2
        $view = Get-ProfileView $cfg 't-a'
        # 999999 is no process: Windows process ids are multiples of 4.
        $script:Instance = [pscustomobject]@{ Pid = 777; ParentPid = 999999; Exe = 'C:\x\Claude.exe'; UserDataDir = $view.UserDataDir; Started = $null }
        $script:Seen = $true; $script:Wrote = $true; $script:Console = $null
        function Get-ClaudeInstances { if ($script:Seen) { @($script:Instance) } else { @() } }
        function Get-DirActivityUtc { if ($script:Wrote) { [DateTime]::UtcNow } else { [DateTime]::MinValue } }
        function Get-ConsoleAttachment { $t = @{}; if ($script:Console) { $t[777] = $script:Console }; $t }
        $runs = [ordered]@{
            'detached' = @($true, $true, [pscustomobject]@{ State = 'none'; Sharing = @(); Detail = '' })
            'attached' = @($true, $true, [pscustomobject]@{ State = 'attached'; Sharing = @(4, 5); Detail = '' })
            'unread'   = @($true, $true, $null)
            'nowrite'  = @($true, $false, $null)
            'noproc'   = @($false, $true, $null)
        }
        foreach ($k in $runs.Keys) {
            $script:Seen, $script:Wrote, $script:Console = $runs[$k]
            $text = (Confirm-Isolation $cfg $view ([DateTime]::UtcNow) 6>&1 | Out-String) -replace '\s+', ' '
            "$k=$text"
        }
        $default = Get-ProfileView $cfg 'default'
        $script:Instance = [pscustomobject]@{ Pid = 777; ParentPid = 999999; Exe = 'C:\x\Claude.exe'; UserDataDir = $null; Started = $null }
        $script:Seen, $script:Wrote, $script:Console = $true, $false, [pscustomobject]@{ State = 'attached'; Sharing = @(4); Detail = '' }
        $text = (Confirm-Isolation $cfg $default ([DateTime]::UtcNow) 6>&1 | Out-String) -replace '\s+', ' '
        "default=$text"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match '(?m)^detached=.*Verified: pid 777 .*Detached: it has no console') "detached: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match '(?m)^attached=.*Verified: pid 777 .*attached to a console') "attached: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match '(?m)^unread=.*could not be read') "unread: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match '(?m)^nowrite=.*wrote nothing') "nowrite: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match '(?m)^noproc=.*No instance carrying') "noproc: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match '(?m)^default=.*Verified: pid 777 is running\. .*attached to a console') "the default profile is not read back: $(Format-Snip $r.Out)"
}

Add-Case 'a-default-launch-is-read-back-too' {
    $r = Invoke-Unit {
        $script:Verified = @()
        function Test-IsElevated { return $false }
        function Find-ClaudeApp { [pscustomobject]@{ Exe = 'C:\x\Claude.exe'; Version = '0'; Source = 'stub'; InstallDir = $null; LogoDirs = @() } }
        function Get-ClaudeInstances { return @() }
        function Start-DetachedProcess { [pscustomobject]@{ Pid = 1; Via = $null; Method = 'stub' } }
        function Confirm-Isolation($Cfg, $View) { $null = $Cfg; $script:Verified += $View.Name }
        Invoke-Launch (Read-Config).Config 'default' 6>$null
        "VERIFIED=$($script:Verified -join ',')"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'VERIFIED=default') "a default launch was not read back: $(Format-Snip $r.Out)"
}

# An instance whose console could not be read is named, not passed as fine.
Add-Case 'an-unread-console-is-reported-not-passed' {
    $r = Invoke-Unit {
        $view = Get-ProfileView (Read-Config).Config 'default'
        $inst = @([pscustomobject]@{ Pid = 777 })
        $none = @{ 777 = [pscustomobject]@{ State = 'none'; Sharing = @(); Detail = '' } }
        $att = @{ 777 = [pscustomobject]@{ State = 'attached'; Sharing = @(4); Detail = '' } }
        $unk = @{ 777 = [pscustomobject]@{ State = 'unknown'; Sharing = @(); Detail = 'error 5' } }
        "NONE=$(Write-AttachedInstances $view $inst -Probe $none 6>$null)"
        "ATTACHED=$(Write-AttachedInstances $view $inst -Probe $att 6>$null)"
        "UNKNOWN=$(Write-AttachedInstances $view $inst -Probe $unk 6>$null)"
        "MISSING=$(Write-AttachedInstances $view $inst -Probe @{} 6>$null)"
    }
    Assert-Exit $r 0
    Assert-That ($r.Out -match 'NONE=0' -and $r.Out -match 'ATTACHED=1') "attached and none: $(Format-Snip $r.Out)"
    Assert-That ($r.Out -match 'UNKNOWN=1' -and $r.Out -match 'MISSING=1') "an unread console passed as fine: $(Format-Snip $r.Out)"
}

Add-Case 'live-launch-survives-a-tree-kill-and-ends-with-the-app' -LiveOnly {
    Assert-Exit (Run 'add' 't-live') 0
    $udd = Join-Path $script:Ctx.Root 't-live'
    $term = Start-Terminal 't-live' -Verify
    try {
        Assert-That (Wait-Until { $null -ne (Get-MainProcess $udd) } 60) 'the app did not start'
        $main = Get-MainProcess $udd
        Assert-That ($main.ParentProcessId -ne $term.Process.Id) 'the app is a child of the terminal'
        Assert-That (-not (Get-Process -Id $main.ParentProcessId -ErrorAction SilentlyContinue)) 'the app has a live parent'
        Assert-That (Wait-Until { [IO.File]::Exists((Join-Path $udd 'Local State')) } 30) 'the app wrote nothing into the profile folder'
        # The launch's own read-back, from the real app.
        Assert-That (Wait-Until { (Read-TextIfAny $term.Log) -match 'Detached: it has no console and its parent has exited' } 60) "the launch did not read back a detached app: $(Format-Snip (Read-TextIfAny $term.Log))"
        Assert-That ((Read-TextIfAny $term.Log) -match 'Verified: pid \d+ is running with its data in') "the launch did not verify the folder: $(Format-Snip (Read-TextIfAny $term.Log))"
        $again = Run 'launch' 't-live' '-NoVerify'
        Assert-Exit $again 0
        Assert-That ($again.Out -match 'already running') 'a second launch did not focus the first'
        Start-Sleep -Seconds 3
        Assert-That (@(Get-ProfileProcesses $udd | Where-Object { $_.CommandLine -notmatch '--type=' }).Count -eq 1) 'a second main process started'
        $kill = Start-Process -FilePath (Join-Path ([Environment]::SystemDirectory) 'taskkill.exe') -ArgumentList '/T', '/F', '/PID', $term.Process.Id -Wait -PassThru -WindowStyle Hidden
        Assert-That ($kill.ExitCode -eq 0) "taskkill answered $($kill.ExitCode)"
        Assert-That (Wait-Until { $term.Process.HasExited } 10) 'the terminal did not die'
        Start-Sleep -Seconds 3
        Assert-That ($null -ne (Get-Process -Id $main.ProcessId -ErrorAction SilentlyContinue)) 'the app died with the terminal'
        Stop-Process -Id $main.ProcessId -Force
        Assert-That (Wait-Until { -not @(Get-ProfileProcesses $udd).Count } 30) 'processes of the app outlived it'
        Start-Sleep -Seconds 5
        Assert-That (-not @(Get-ProfileProcesses $udd).Count) 'something started the app again'
    }
    finally {
        if (-not $term.Process.HasExited) { $term.Process.Kill($true) }
        Stop-ProfileApp $udd
    }
}

Add-Case 'live-a-kill-on-close-job-that-allows-breakaway-does-not-end-the-app' -LiveOnly {
    if (-not ('CcProfileTestJob' -as [type])) { Add-Type -TypeDefinition $script:JobSource -Language CSharp }
    Assert-Exit (Run 'add' 't-job') 0
    $udd = Join-Path $script:Ctx.Root 't-job'
    $job = [CcProfileTestJob]::Create($true)
    $term = Start-Terminal 't-job'
    try {
        [CcProfileTestJob]::Assign($job, $term.Process.Handle)
        Assert-That (Wait-Until { $null -ne (Get-MainProcess $udd) } 60) 'the app did not start'
        $main = Get-MainProcess $udd
        [void][CcProfileTestJob]::CloseHandle($job)
        $job = [IntPtr]::Zero
        Assert-That (Wait-Until { $term.Process.HasExited } 10) 'closing the job did not end the terminal'
        Start-Sleep -Seconds 3
        Assert-That ($null -ne (Get-Process -Id $main.ProcessId -ErrorAction SilentlyContinue)) 'the app died with the job'
    }
    finally {
        if ($job -ne [IntPtr]::Zero) { [void][CcProfileTestJob]::CloseHandle($job) }
        if (-not $term.Process.HasExited) { $term.Process.Kill($true) }
        Stop-ProfileApp $udd
    }
}

Add-Case 'live-a-job-that-forbids-breakaway-is-reported' -LiveOnly {
    if (-not ('CcProfileTestJob' -as [type])) { Add-Type -TypeDefinition $script:JobSource -Language CSharp }
    Assert-Exit (Run 'add' 't-held') 0
    $udd = Join-Path $script:Ctx.Root 't-held'
    $job = [CcProfileTestJob]::Create($false)
    $term = Start-Terminal 't-held'
    try {
        [CcProfileTestJob]::Assign($job, $term.Process.Handle)
        Assert-That (Wait-Until { $null -ne (Get-MainProcess $udd) } 60) 'the app did not start'
        $log = [IO.File]::ReadAllText((Join-Path $script:Ctx.DataDir 'launch.log'))
        Assert-That ($log -match 'job breakaway not allowed') 'the launch did not report the job'
    }
    finally {
        if ($job -ne [IntPtr]::Zero) { [void][CcProfileTestJob]::CloseHandle($job) }
        if (-not $term.Process.HasExited) { $term.Process.Kill($true) }
        Stop-ProfileApp $udd
    }
}

# The user's report, as a test: launch, focus, then close the terminal tab.
function Invoke-ConsoleCloseCase([string]$ProfileName) {
    $udd = Join-Path $script:Ctx.Root $ProfileName
    $tool = ConvertTo-PsLiteral $script:ToolPath
    $cfg = ConvertTo-PsLiteral $script:Ctx.Config
    $term = Start-ConsoleTerminal @(
        "& $tool launch $ProfileName -ConfigPath $cfg -NoVerify"
        "& $tool launch $ProfileName -ConfigPath $cfg -NoVerify"
        'Start-Sleep -Seconds 600'
    )
    try {
        Assert-That (Wait-Until { $null -ne (Get-MainProcess $udd) } 60) "the app did not start. Terminal: $(Format-Snip $term.Screen)"
        $main = Get-MainProcess $udd
        Assert-That (Wait-Until { $term.Screen -match 'Focused' } 60) "the second launch did not finish. Terminal: $(Format-Snip $term.Screen)"
        Start-Sleep -Seconds 8
        $state = Get-TestConsoleState $main.ProcessId
        Assert-That ($state -eq 'none') "the app is on a console: $state"
        Assert-That ($term.Screen -notmatch $script:AppLogPattern) "the app printed into the terminal: $(Format-Snip $term.Screen)"
        $term.Close()
        Assert-That (Wait-Until { -not (Get-Process -Id $term.ProcessId -ErrorAction SilentlyContinue) } 20) 'closing the terminal did not end its shell'
        Start-Sleep -Seconds 5
        Assert-That ($null -ne (Get-Process -Id $main.ProcessId -ErrorAction SilentlyContinue)) 'the app died with the terminal'
    }
    finally {
        $term.Dispose()
        Stop-ProfileApp $udd
    }
}

Add-Case 'live-closing-the-terminal-leaves-the-app-running-and-silent' -LiveOnly {
    Assert-Exit (Run 'add' 't-con') 0
    Invoke-ConsoleCloseCase 't-con'
}

Add-Case 'live-a-direct-launch-also-survives-closing-the-terminal' -LiveOnly {
    Assert-Exit (Run 'add' 't-dir') 0
    $c = Read-CaseConfig
    $c['launch'] = @{ verifySeconds = 20; via = 'direct' }
    Set-CaseConfig $c
    Invoke-ConsoleCloseCase 't-dir'
}

# How the instance in the report was started: the app run straight from a shell. It
# attaches to that terminal, and every command that meets it has to say so.
Add-Case 'live-an-app-started-from-a-terminal-is-named-and-dies-with-it' -LiveOnly {
    Assert-Exit (Run 'add' 't-raw') 0
    $udd = Join-Path $script:Ctx.Root 't-raw'
    $exe = (Invoke-Unit { (Find-ClaudeApp (Read-Config).Config).Exe }).Out.Trim()
    Assert-That ([IO.File]::Exists($exe)) "no app at '$exe'"
    $term = Start-ConsoleTerminal @(
        'Remove-Item Env:ELECTRON_NO_ATTACH_CONSOLE -ErrorAction SilentlyContinue'
        "& $(ConvertTo-PsLiteral $exe) $(ConvertTo-PsLiteral "--user-data-dir=$udd")"
        'Start-Sleep -Seconds 600'
    )
    try {
        Assert-That (Wait-Until { $null -ne (Get-MainProcess $udd) } 60) 'the app did not start'
        $main = Get-MainProcess $udd
        Assert-That (Wait-Until { (Get-TestConsoleState $main.ProcessId) -like 'attached*' } 30) 'the app run from a shell did not attach to its console'
        Assert-That ((Get-TestConsoleState $main.ProcessId) -match "\b$($term.ProcessId)\b") 'the app is not on the terminal console'
        $doc = Run 'doctor'
        Assert-Exit $doc 1
        Assert-That ($doc.Out -match 'attached to a terminal') "doctor did not name it: $(Format-Snip $doc.Out)"
        $st = Run 'status'
        Assert-Exit $st 0
        Assert-That ($st.Out -match 'ATTACHED') "status did not name it: $(Format-Snip $st.Out)"
        $foc = Run 'launch' 't-raw' '-NoVerify'
        Assert-Exit $foc 0
        Assert-That ($foc.Out -match 'attached to a terminal') "focusing it did not warn: $(Format-Snip $foc.Out)"
        $term.Close()
        Assert-That (Wait-Until { -not (Get-Process -Id $main.ProcessId -ErrorAction SilentlyContinue) } 30) 'an app attached to the closed terminal survived it'
    }
    finally {
        $term.Dispose()
        Stop-ProfileApp $udd
    }
}

#endregion
#region ---------------------------------------------------------------- the guard mutation table

# Each row deletes, or neuters, one guard. FIND must match exactly once in the script,
# CASE must pass unmutated in this run before its row counts, and the mutated case
# must fail with a detail matching EXPECT: a red that has another cause is misfired.
$script:Mutations = @(
    @{ Name = 'appdata-guard'; Case = 'a-profile-under-appdata-is-refused'; Expect = 'exit 0, wanted 1'
        Find = '    & $add $env:LOCALAPPDATA "AppData\Local (MSIX'; Replace = '    $null = $add; $null = "AppData\Local (MSIX' }
    @{ Name = 'overlap-guard'; Case = 'overlapping-profile-folders-are-refused'; Expect = 'exit 0, wanted 1'
        Find = 'if ($a.Profile -ine $b.Profile -and (Test-PathOverlap $a.Path $b.Path)) {'; Replace = 'if ($a.Profile -ine $b.Profile -and $false) {' }
    @{ Name = 'reserved-names'; Case = 'a-command-name-is-refused-as-a-profile-name'; Expect = 'exit 0, wanted 1'
        Find = 'if ($script:ReservedNames -contains $Name) { return'; Replace = 'if ($false) { return' }
    @{ Name = 'name-ends-at-the-end'; Case = 'a-name-with-a-line-feed-is-refused'; Expect = 'wrong refusal|a profile was saved'
        Find = '[A-Za-z0-9_-])?\z'''; Replace = '[A-Za-z0-9_-])?$''' }
    @{ Name = 'drive-root-rule'; Case = 'validation-refuses-each-broken-shape'; Expect = 'MISS: drive-root'
        Find = "if ([IO.Path]::GetPathRoot(`$path).TrimEnd('\') -ieq `$path.TrimEnd('\')) { & `$E"; Replace = 'if ($false) { & $E' }
    @{ Name = 'network-path-rule'; Case = 'validation-refuses-each-broken-shape'; Expect = 'MISS: network'
        Find = 'if (-not (Test-LocalDrivePath $path)) { & $E'; Replace = 'if ($false) { & $E' }
    @{ Name = 'device-prefix-stripped'; Case = 'validation-refuses-each-broken-shape'; Expect = 'MISS: device-prefix'
        Find = 'elseif ($p -match ''^[\\/]{2}[?.][\\/](.+)$'') { $p = $Matches[1] }'; Replace = 'elseif ($false) { }' }
    @{ Name = 'key-case-rule'; Case = 'validation-refuses-each-broken-shape'; Expect = 'MISS: key-case'
        Find = 'if ($hit) { "$Where${k}: keys are case-sensitive'; Replace = 'if ($false) { "$Where${k}: keys are case-sensitive' }
    @{ Name = 'env-case-collision'; Case = 'validation-refuses-each-broken-shape'; Expect = 'MISS: env-case'
        Find = 'elseif ($seen.ContainsKey($k)) {'; Replace = 'elseif ($false) {' }
    @{ Name = 'issues-name-their-profiles'; Case = 'a-bad-entry-blocks-only-its-own-profile'; Expect = 'exit 1, wanted 0'
        Find = '(-not $_.Profiles.Count -or $_.Profiles -contains $Name)'; Replace = '(-not $_.Profiles.Count -or @($_.Profiles | Where-Object { $_.StartsWith($Name) }).Count)' }
    @{ Name = 'environment-scrub'; Case = 'a-dry-run-carries-isolation-and-none-of-the-callers-variables'; Expect = 'is not reported as dropped'
        Find = '$patterns = $script:DefaultScrub + @($Cfg[''scrubEnv'']) + $script:ManagedEnv'; Replace = '$patterns = @($Cfg[''scrubEnv'']) + $script:ManagedEnv' }
    @{ Name = 'stale-write-guard'; Case = 'a-write-refuses-when-the-file-changed-under-it'; Expect = 'a stale write was not refused'
        Find = 'if ($current -ne $ExpectedHash) {'; Replace = 'if ($false) {' }
    @{ Name = 'schema-refusal'; Case = 'a-newer-schema-is-read-only'; Expect = 'exit 0, wanted 1'
        Find = 'if ((Test-IsInteger $ver) -and $ver -gt $script:SchemaVersion) {'; Replace = 'if ($false) {' }
    @{ Name = 'comments-need-force'; Case = 'a-config-with-comments-is-not-rewritten-without-force'; Expect = 'exit 0, wanted 1'
        Find = 'if ($__state.Exists -and -not $__state.Strict -and -not $Force) {'; Replace = 'if ($false) {' }
    @{ Name = 'ordinal-keys'; Case = 'unknown-keys-survive-a-write'; Expect = 'a key was dropped'
        Find = '    $o = New-OrdinalDictionary'; Replace = '    $o = [ordered]@{}' }
    @{ Name = 'byte-order-marks'; Case = 'a-utf16-config-is-read'; Expect = 'exit 1, wanted 0'
        Find = '[IO.StreamReader]::new([IO.MemoryStream]::new($Bytes), [Text.UTF8Encoding]::new($false), $true)'; Replace = '[IO.StreamReader]::new([IO.MemoryStream]::new($Bytes), [Text.UTF8Encoding]::new($false), $false)' }
    @{ Name = 'cross-process-lock'; Case = 'parallel-writers-all-land'; Expect = 'was lost|exit codes'
        Find = 'return "Local\cc_profile-$key"'; Replace = 'return "Local\cc_profile-$key-$PID"' }
    @{ Name = 'removal-containment'; Case = 'removal-outside-its-root-is-refused'; Expect = 'went ahead'
        Find = 'if (-not (Test-PathWithin $full $rootFull)) { throw'; Replace = 'if ($false) { throw' }
    @{ Name = 'removal-normalised-path'; Case = 'removal-outside-its-root-is-refused'; Expect = 'went ahead'
        Find = '$full = [IO.Path]::GetFullPath($Path).TrimEnd(''\'')'; Replace = '$full = $Path' }
    @{ Name = 'removal-never-the-root'; Case = 'removal-outside-its-root-is-refused'; Expect = 'went ahead'
        Find = '$full = [IO.Path]::GetFullPath($Path).TrimEnd(''\'')'; Replace = '$full = [IO.Path]::GetFullPath($Path)' }
    @{ Name = 'purge-safety'; Case = 'purge-refuses-a-folder-it-must-not-move'; Expect = 'exit 0, wanted 1'
        Find = 'if (Test-PathOverlap $Path $pd.Path) { throw "Refusing to move'; Replace = 'if ($false) { throw "Refusing to move' }
    @{ Name = 'trash-names-never-collide'; Case = 'a-purge-does-not-collide-with-an-earlier-one'; Expect = 'collided with an earlier one'
        Find = 'for ($n = 2; [IO.Directory]::Exists($dest) -or [IO.File]::Exists($dest); $n++) {'; Replace = 'for ($n = 2; $false; $n++) {' }
    @{ Name = 'rename-keeps-the-entry'; Case = 'rename-moves-the-entry-and-refuses-bad-names'; Expect = 'did not take over the entry'
        Find = 'if ($k -ceq $old) { $rebuilt[$newName] = $cfg[''profiles''][$k] }'; Replace = 'if ($k -ceq $old) { $null = $k }' }
    @{ Name = 'list-splitting'; Case = 'a-comma-list-through-file-is-split'; Expect = '-Alias: aliases are|exit 1, wanted 0'
        Find = 'foreach ($part in ([string]$v).Split('',''))'; Replace = 'foreach ($part in @([string]$v))' }
    @{ Name = 'positional-splitting'; Case = 'a-comma-list-through-file-is-split'; Expect = 'exit 1, wanted 0'
        Find = '$aliases = @(Split-ListValue (@($script:RestArgs) + @($Alias)))'; Replace = '$aliases = @(@($script:RestArgs) + @($Alias) | Where-Object { $_ })' }
    @{ Name = 'setenv-one-spelling'; Case = 'setenv-replaces-a-variable-whatever-its-case'; Expect = 'exit 1, wanted 0'
        Find = 'foreach ($old in @($envd.Keys | Where-Object { $_ -ieq $k -and $_ -cne $k })) { $envd.Remove($old) }'; Replace = '$null = $k' }
    @{ Name = 'machine-output-to-stderr'; Case = 'json-output-stays-pure-when-messages-are-printed'; Expect = 'stdout is not JSON'
        Find = '        if ($NoNewLine) { [Console]::Error.Write($Text) } else { [Console]::Error.WriteLine($Text) }'; Replace = '        if ($NoNewLine) { [Console]::Out.Write($Text) } else { [Console]::Out.WriteLine($Text) }' }
    @{ Name = 'trace-to-stderr'; Case = 'json-output-stays-pure-when-messages-are-printed'; Expect = 'with -Verbose, stdout is not JSON'
        Find = 'if ($script:MachineOutput) { [Console]::Error.WriteLine("VERBOSE: $Message") }'; Replace = 'if ($false) { [Console]::Error.WriteLine("VERBOSE: $Message") }' }
    @{ Name = 'invariant-times'; Case = 'times-print-the-same-in-every-culture'; Expect = 'a time printed under fi-FI'
        Find = 'try { return ([datetime]::Parse([string]$Value, $inv)).ToString(''yyyy-MM-dd HH:mm'', $inv) }'; Replace = 'try { return ([datetime]::Parse([string]$Value, $inv)).ToString(''yyyy-MM-dd HH:mm'') }' }
    @{ Name = 'no-op-writes-nothing'; Case = 'add-again-writes-nothing'; Expect = 'rewritten|changed|backup was taken'
        Find = 'if ($null -ne $__before -and $__before -ceq $__after) {'; Replace = 'if ($false) {' }
    @{ Name = 'session-variables-over-machine'; Case = 'the-clean-environment-is-the-users-not-systems'; Expect = 'USERNAME differs'
        Find = '        if ($v) { $envd[$n] = $v }'; Replace = '        if ($v -and -not $envd.ContainsKey($n)) { $envd[$n] = $v }' }
    @{ Name = 'secret-redaction'; Case = 'setenv-takes-name-equals-value-and-show-redacts-secrets'; Expect = 'MY_TOKEN is shown'
        Find = "if (`$Key -match '(TOKEN|SECRET|PASSWORD|PASSWD|API_?KEY|CREDENTIAL|AUTH)') { return '<redacted>' }"; Replace = "if (`$false) { return '<redacted>' }" }
    @{ Name = 'seed-blocklist'; Case = 'seed-copies-settings-and-never-credentials'; Expect = 'exit 0, wanted 1'
        Find = 'if ($script:SeedBlocked -contains $item) { throw'; Replace = 'if ($false) { throw' }
    @{ Name = 'seed-exact-spelling'; Case = 'seed-copies-settings-and-never-credentials'; Expect = 'exit 0, wanted 1'
        Find = '-or $item -cne $item.Trim() -or $item.EndsWith(''.'')) {'; Replace = ') {' }
    @{ Name = 'seed-checks-the-profile'; Case = 'seed-refuses-a-folder-the-config-may-not-name'; Expect = 'exit 0, wanted 1'
        Find = '    Assert-ConfigUsable $Cfg $name'; Replace = '    $null = $name' }
    @{ Name = 'discover-long-names'; Case = 'discover-registers-a-folder-with-a-long-name'; Expect = 'registered as'
        Find = 'if ($base.Length -gt 48) { $base = $base.Substring(0, 48).Trim(''.'', ''_'', ''-'') }'; Replace = '$null = $base.Length' }
    @{ Name = 'restore-counts-one-backup'; Case = 'restore-works-with-a-single-backup'; Expect = 'exit 1, wanted 0'
        Find = '$files = @(Get-ConfigBackups)'; Replace = '$files = Get-ConfigBackups' }
    @{ Name = 'whatif-quiet-discovery'; Case = 'whatif-launches-nothing'; Expect = 'other What if line'
        Find = '$pkgs = @($ps.AddCommand(''Get-AppxPackage'').AddParameter(''Name'', ''Claude'').Invoke())'; Replace = '$pkgs = @(Get-AppxPackage -Name ''Claude'')' }
    @{ Name = 'profile-block-quoting'; Case = 'the-profile-block-parses-runs-and-completes'; Expect = 'exit 1, wanted 0|does not parse|did not run|does not complete'
        Find = 'ForEach-Object { ConvertTo-SingleQuoted $_ }) -join '','''; Replace = 'ForEach-Object { "''''$_''''" }) -join '','''  }
    @{ Name = 'profile-parse-check'; Case = 'a-profile-block-that-would-not-parse-is-not-written'; Expect = 'a broken block was not refused'
        Find = 'if ((Get-ParseErrorCount $new) -gt (Get-ParseErrorCount $text)) { throw'; Replace = 'if ($false) { throw' }
    @{ Name = 'shortcut-ownership'; Case = 'shortcuts-of-another-config-are-left-alone'; Expect = "another config's shortcut"
        Find = 'return [bool]($c -and (Test-PathEqual $c $script:ConfigFile))'; Replace = 'return [IO.File]::Exists($Lnk)' }
    @{ Name = 'shortcut-script-check'; Case = 'rename-takes-its-shortcuts-or-changes-nothing'; Expect = 'a rename from a temporary copy was not refused'
        Find = 'if ((Test-TransientScriptPath) -and -not $Force) {'; Replace = 'if ($false) {' }
    @{ Name = 'cmd-starts-once'; Case = 'a-launch-starts-the-app-once-whatever-cmd-answers'; Expect = 'led to a second start'
        Find = 'if (-not $r.Exited) {'; Replace = 'if ($false) {' }
    @{ Name = 'orphaned-launch-offline'; Case = 'a-launch-starts-the-app-once-whatever-cmd-answers'; Expect = 'did not fall back|a started cmd start|led to a second start'
        Find = '$viaCmd = $Via -eq ''cmd'' -and -not @('; Replace = '$viaCmd = $false -and -not @(' }
    @{ Name = 'elevation-has-its-own-switch'; Case = 'an-elevated-shell-launches-only-with-allowelevated'; Expect = 'was not refused'
        Find = 'if ((Test-IsElevated) -and -not $AllowElevated) {'; Replace = 'if ((Test-IsElevated) -and -not $Force) {' }
    @{ Name = 'verify-reads-the-console'; Case = 'a-launch-reads-back-what-it-claims'; Expect = '^attached:|attached: '
        Find = '$console = (Get-ConsoleAttachment @($instance.Pid))[$instance.Pid]'; Replace = '$console = [pscustomobject]@{ State = ''none''; Sharing = @(); Detail = '''' }' }
    @{ Name = 'default-is-verified'; Case = 'a-default-launch-is-read-back-too'; Expect = 'was not read back'
        Find = 'if (-not $running.Count -and -not $NoVerify) { Confirm-Isolation'; Replace = 'if (-not $running.Count -and -not $NoVerify -and -not $view.IsDefault) { Confirm-Isolation' }
    @{ Name = 'unread-console-is-not-fine'; Case = 'an-unread-console-is-reported-not-passed'; Expect = 'passed as fine'
        Find = '        if ($a -and $a.State -eq ''none'') { continue }'; Replace = '        if (-not $a -or $a.State -ne ''attached'') { continue }' }
    @{ Name = 'console-probe'; Case = 'the-console-probe-tells-attached-from-none'; Expect = 'was not read as'
        Find = 'if ($line -match ''^PROBE\|(\d+)\|(attached|none|unknown)\|(.*)$'') {'; Replace = 'if ($false) {' }
    @{ Name = 'no-console-attach'; Case = 'a-dry-run-carries-isolation-and-none-of-the-callers-variables'; Expect = 'stay off the console'
        Find = '    $envd[''ELECTRON_NO_ATTACH_CONSOLE''] = ''1'''; Replace = '    $null = ''1''' }
    @{ Name = 'orphaned-launch'; Case = 'live-launch-survives-a-tree-kill-and-ends-with-the-app'; Expect = 'child of the terminal|live parent|died with the terminal|did not read back a detached app'
        Find = '$viaCmd = $Via -eq ''cmd'' -and -not @('; Replace = '$viaCmd = $false -and -not @(' }
    @{ Name = 'no-console-attach-live'; Case = 'live-a-direct-launch-also-survives-closing-the-terminal'; Expect = 'on a console|printed into the terminal|died with the terminal'
        Find = '    $envd[''ELECTRON_NO_ATTACH_CONSOLE''] = ''1'''; Replace = '    $null = ''1''' }
    @{ Name = 'attached-instance-warnings'; Case = 'live-an-app-started-from-a-terminal-is-named-and-dies-with-it'; Expect = 'did not name it|did not warn|exit 0, wanted 1'
        Find = '        if ($a -and $a.State -eq ''none'') { continue }'; Replace = '        continue' }
)

#endregion
#region ---------------------------------------------------------------- run

function Invoke-Case($C) {
    $script:Ctx = New-CaseContext $C.Name
    $sw = [Diagnostics.Stopwatch]::StartNew()
    try {
        & $C.Body
        return [pscustomobject]@{ Name = $C.Name; Status = 'passed'; Seconds = [math]::Round($sw.Elapsed.TotalSeconds, 1); Detail = '' }
    }
    catch {
        return [pscustomobject]@{ Name = $C.Name; Status = 'failed'; Seconds = [math]::Round($sw.Elapsed.TotalSeconds, 1); Detail = $_.Exception.Message }
    }
}

# Through pwsh -File, "-Case a,b" arrives as the one string "a,b".
$wantedCases = @($Case | ForEach-Object { ([string]$_).Split(',') } | ForEach-Object { $_.Trim() } | Where-Object { $_ })
$unknown = @($wantedCases | Where-Object { -not $script:Cases.Contains($_) })
if ($unknown.Count) { Write-Say "No case named: $($unknown -join ', ')" 'Red'; exit 2 }
$badRows = @($script:Mutations | Where-Object { -not $script:Cases.Contains($_.Case) -or -not $_.Expect })
if ($badRows.Count) { Write-Say "Mutation rows naming no case, or no Expect: $(@($badRows | ForEach-Object { $_.Name }) -join ', ')" 'Red'; exit 2 }
$selected = @($script:Cases.Values | Where-Object { (-not $wantedCases.Count -or $wantedCases -contains $_.Name) })
$results = [Collections.Generic.List[object]]::new()
foreach ($c in $selected) {
    if ($c.Live -and -not $Live) {
        $results.Add([pscustomobject]@{ Name = $c.Name; Status = 'skipped'; Seconds = 0; Detail = 'needs -Live' })
        continue
    }
    $res = Invoke-Case $c
    $results.Add($res)
    if ($res.Status -eq 'passed') { Write-Say ('  ok    {0} ({1}s)' -f $res.Name, $res.Seconds) 'Green' }
    else { Write-Say ('  FAIL  {0}: {1}' -f $res.Name, $res.Detail) 'Red' }
}

$mutResults = [Collections.Generic.List[object]]::new()
if ($Mutate) {
    $source = [IO.File]::ReadAllText($script:OrigTool)
    foreach ($m in $script:Mutations) {
        $row = [pscustomobject]@{ Name = $m.Name; Case = $m.Case; Status = ''; Detail = '' }
        $base = @($results | Where-Object { $_.Name -eq $m.Case }) | Select-Object -First 1
        if (-not $base -or $base.Status -ne 'passed') {
            $row.Status = 'unrun'; $row.Detail = 'its case did not pass unmutated in this run'
            $mutResults.Add($row); Write-Say ('  unrun mutation {0}: {1}' -f $m.Name, $row.Detail) 'DarkGray'
            continue
        }
        $count = ([regex]::Matches($source, [regex]::Escape($m.Find))).Count
        if ($count -ne 1) {
            $row.Status = 'broken'; $row.Detail = "FIND matches $count times, not once"
            $mutResults.Add($row); Write-Say ('  BROKEN mutation {0}: {1}' -f $m.Name, $row.Detail) 'Red'
            continue
        }
        $mutDir = Join-Path $script:RunRoot "mutant-$($m.Name)"
        [void][IO.Directory]::CreateDirectory($mutDir)
        $mutant = Join-Path $mutDir 'cc_profile.ps1'
        [IO.File]::WriteAllText($mutant, $source.Replace($m.Find, $m.Replace), [Text.UTF8Encoding]::new($false))
        $tk = $null; $pe = $null
        [void][Management.Automation.Language.Parser]::ParseFile($mutant, [ref]$tk, [ref]$pe)
        if ($pe.Count) {
            $row.Status = 'broken'; $row.Detail = "the mutant does not parse: $($pe[0].Message)"
            $mutResults.Add($row); Write-Say ('  BROKEN mutation {0}: {1}' -f $m.Name, $row.Detail) 'Red'
            continue
        }
        $script:ToolPath = $mutant
        $res = Invoke-Case $script:Cases[$m.Case]
        $script:ToolPath = $script:OrigTool
        $row.Detail = $res.Detail
        if ($res.Status -eq 'failed' -and $res.Detail -match $m.Expect) {
            $row.Status = 'proved'; Write-Say ('  ok    mutation {0} turns {1} red' -f $m.Name, $m.Case) 'Green'
        }
        elseif ($res.Status -eq 'failed') {
            $row.Status = 'misfired'; Write-Say ('  MISFIRED mutation {0}: {1} failed, but not as /{2}/ expects: {3}' -f $m.Name, $m.Case, $m.Expect, $res.Detail) 'Red'
        }
        else { $row.Status = 'theatre'; $row.Detail = 'the case still passed'; Write-Say ('  THEATRE mutation {0}: {1} still passes' -f $m.Name, $m.Case) 'Red' }
        $mutResults.Add($row)
    }
}

Stop-ProfileApp $script:RunRoot
$leftover = @(Get-ProfileProcesses $script:RunRoot).Count
if ($leftover) {
    $results.Add([pscustomobject]@{ Name = 'no-test-app-is-left-running'; Status = 'failed'; Seconds = 0; Detail = "$leftover process(es) of a test profile still run" })
    Write-Say "  FAIL  $leftover process(es) of a test profile are still running" 'Red'
}

$passed = @($results | Where-Object Status -EQ 'passed').Count
$failed = @($results | Where-Object Status -EQ 'failed').Count
$skipped = @($results | Where-Object Status -EQ 'skipped').Count
$proved = @($mutResults | Where-Object Status -EQ 'proved').Count
$theatre = @($mutResults | Where-Object Status -EQ 'theatre').Count
$misfired = @($mutResults | Where-Object Status -EQ 'misfired').Count
$broken = @($mutResults | Where-Object Status -EQ 'broken').Count
$unrun = @($mutResults | Where-Object Status -EQ 'unrun').Count
$summary = 'cases={0} passed={1} failed={2} skipped={3} mutations={4} proved={5} theatre={6} misfired={7} broken={8} unrun={9}' -f $results.Count, $passed, $failed, $skipped, $mutResults.Count, $proved, $theatre, $misfired, $broken, $unrun

if (-not $Keep) {
    # A process of an app a live case ended can hold a log in the case's folder for a
    # few seconds after it is gone from the process list, so the removal is retried.
    $full = [IO.Path]::GetFullPath($script:RunRoot)
    if ($full.StartsWith([IO.Path]::GetFullPath($script:TestsRoot) + '\', [StringComparison]::OrdinalIgnoreCase)) {
        for ($attempt = 1; ; $attempt++) {
            try { Remove-Item -LiteralPath $full -Recurse -Force; break }
            catch {
                if ($attempt -ge 10) { Write-Say "Could not remove $($script:RunRoot): $($_.Exception.Message)" 'Yellow'; break }
                Start-Sleep -Seconds 2
            }
        }
    }
}
else { Write-Say "Kept $($script:RunRoot)" }

if ($Json) {
    [pscustomobject]@{ cases = $results; mutations = $mutResults; summary = $summary } | ConvertTo-Json -Depth 4
}
Write-Say $summary
if ($failed -or $theatre -or $misfired -or $broken) { exit 1 }
exit 0

#endregion
