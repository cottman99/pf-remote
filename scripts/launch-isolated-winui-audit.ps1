[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$ApplicationPath,
    [Parameter(Mandatory = $true)]
    [string]$PidPath,
    [string]$DesktopName = 'PFRemoteAudit',
    [string]$AutomationPath,
	[string]$LaunchEvidencePath,
    [switch]$NoWindow
)

$ErrorActionPreference = 'Stop'
$application = (Resolve-Path -LiteralPath $ApplicationPath).Path
$workingDirectory = Split-Path -Parent $application
$pidFile = [IO.Path]::GetFullPath($PidPath)
$pidDirectory = Split-Path -Parent $pidFile
New-Item -ItemType Directory -Force -Path $pidDirectory | Out-Null

$source = @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Threading;

public sealed class IsolatedDesktopProcess : IDisposable
{
    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct STARTUPINFO
    {
        public int cb;
        public string lpReserved;
        public string lpDesktop;
        public string lpTitle;
        public int dwX, dwY, dwXSize, dwYSize, dwXCountChars, dwYCountChars;
        public int dwFillAttribute, dwFlags;
        public short wShowWindow, cbReserved2;
        public IntPtr lpReserved2, hStdInput, hStdOutput, hStdError;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct PROCESS_INFORMATION
    {
        public IntPtr hProcess;
        public IntPtr hThread;
        public int dwProcessId;
        public int dwThreadId;
    }

    private delegate bool EnumDesktopWindowsDelegate(IntPtr hwnd, IntPtr lParam);

    [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern IntPtr CreateDesktop(string name, IntPtr device, IntPtr devmode, int flags, uint access, IntPtr attributes);
    [DllImport("user32.dll", SetLastError = true)]
    private static extern bool EnumDesktopWindows(IntPtr desktop, EnumDesktopWindowsDelegate callback, IntPtr lParam);
    [DllImport("user32.dll")]
    private static extern bool IsWindowVisible(IntPtr hwnd);
    [DllImport("user32.dll")]
    private static extern uint GetWindowThreadProcessId(IntPtr hwnd, out int processId);
    [DllImport("user32.dll", SetLastError = true)]
    private static extern IntPtr SendMessageTimeout(IntPtr hwnd, uint message, IntPtr wParam, IntPtr lParam, uint flags, uint timeout, out IntPtr result);
    [DllImport("user32.dll")]
    private static extern bool CloseDesktop(IntPtr desktop);
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern bool CreateProcess(string application, string commandLine, IntPtr processAttributes, IntPtr threadAttributes, bool inheritHandles, uint flags, IntPtr environment, string directory, ref STARTUPINFO startup, out PROCESS_INFORMATION process);
    [DllImport("kernel32.dll")]
    private static extern uint WaitForSingleObject(IntPtr handle, uint milliseconds);
    [DllImport("kernel32.dll")]
    private static extern bool CloseHandle(IntPtr handle);

    private IntPtr desktop;
    private IntPtr process;
    private IntPtr window;
    public int ProcessId { get; private set; }

    public static IsolatedDesktopProcess Start(string application, string directory, string desktopName)
    {
        var result = new IsolatedDesktopProcess();
        result.desktop = CreateDesktop(desktopName, IntPtr.Zero, IntPtr.Zero, 0, 0x10000000, IntPtr.Zero);
        if (result.desktop == IntPtr.Zero) throw new Win32Exception();
        var startup = new STARTUPINFO { cb = Marshal.SizeOf<STARTUPINFO>(), lpDesktop = desktopName };
        PROCESS_INFORMATION created;
        if (!CreateProcess(application, null, IntPtr.Zero, IntPtr.Zero, false, 0, IntPtr.Zero, directory, ref startup, out created))
        {
            result.Dispose();
            throw new Win32Exception();
        }
        result.process = created.hProcess;
        result.ProcessId = created.dwProcessId;
        CloseHandle(created.hThread);
        return result;
    }

    public IntPtr WaitForWindow(int timeoutMilliseconds)
    {
        int elapsed = 0;
        while (elapsed < timeoutMilliseconds)
        {
            IntPtr found = IntPtr.Zero;
            EnumDesktopWindows(desktop, delegate(IntPtr candidate, IntPtr ignored)
            {
                int owner;
                GetWindowThreadProcessId(candidate, out owner);
                if (owner == ProcessId && IsWindowVisible(candidate))
                {
                    found = candidate;
                    return false;
                }
                return true;
            }, IntPtr.Zero);
            if (found != IntPtr.Zero)
            {
                window = found;
                return found;
            }
            if (WaitForSingleObject(process, 0) == 0) throw new InvalidOperationException("PF Remote Center exited before showing its window.");
            Thread.Sleep(100);
            elapsed += 100;
        }
        throw new TimeoutException("PF Remote Center did not show an isolated top-level window.");
    }

    public bool IsResponsive()
    {
        IntPtr ignored;
        return window != IntPtr.Zero && SendMessageTimeout(window, 0, IntPtr.Zero, IntPtr.Zero, 2, 2000, out ignored) != IntPtr.Zero;
    }

    public bool IsRunning() { return WaitForSingleObject(process, 0) != 0; }

    public static int RunOnDesktop(string application, string directory, string desktopName, int timeoutMilliseconds)
    {
        var startup = new STARTUPINFO { cb = Marshal.SizeOf<STARTUPINFO>(), lpDesktop = desktopName };
        PROCESS_INFORMATION created;
        if (!CreateProcess(application, null, IntPtr.Zero, IntPtr.Zero, false, 0, IntPtr.Zero, directory, ref startup, out created))
            throw new Win32Exception();
        CloseHandle(created.hThread);
        uint result = WaitForSingleObject(created.hProcess, (uint)timeoutMilliseconds);
        if (result != 0) { CloseHandle(created.hProcess); throw new TimeoutException("Isolated automation did not finish."); }
        uint exitCode;
        if (!GetExitCodeProcess(created.hProcess, out exitCode)) { CloseHandle(created.hProcess); throw new Win32Exception(); }
        CloseHandle(created.hProcess);
        return (int)exitCode;
    }

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool GetExitCodeProcess(IntPtr process, out uint exitCode);

    public void Wait() { WaitForSingleObject(process, 0xffffffff); }
    public void Dispose()
    {
        if (process != IntPtr.Zero) { CloseHandle(process); process = IntPtr.Zero; }
        if (desktop != IntPtr.Zero) { CloseDesktop(desktop); desktop = IntPtr.Zero; }
    }
}
'@

Add-Type -TypeDefinition $source
$env:PFREMOTE_GOLDEN_LAUNCH_STARTED_UTC = [DateTimeOffset]::UtcNow.ToString('O')
$launchWatch = [Diagnostics.Stopwatch]::StartNew()
$isolated = [IsolatedDesktopProcess]::Start($application, $workingDirectory, $DesktopName)
try {
    if ($NoWindow) {
        Start-Sleep -Milliseconds 750
        if (-not $isolated.IsRunning()) { throw 'The isolated background process exited during startup.' }
    }
    else {
        [void]$isolated.WaitForWindow(20000)
        if (-not $isolated.IsResponsive()) { throw 'PF Remote Center isolated top-level window is not responsive.' }
    }
	$launchWatch.Stop()
	if (-not [string]::IsNullOrWhiteSpace($LaunchEvidencePath)) {
		$launchEvidence = [IO.Path]::GetFullPath($LaunchEvidencePath)
		$launchEvidenceDirectory = Split-Path -Parent $launchEvidence
		New-Item -ItemType Directory -Force -Path $launchEvidenceDirectory | Out-Null
		[ordered]@{
			schema_version = 'pfremote.launch-timing/v1'
			status = 'passed'
			window_ready_seconds = [Math]::Round($launchWatch.Elapsed.TotalSeconds, 3)
			responsive_top_level_window = (-not $NoWindow)
		} | ConvertTo-Json | Set-Content -LiteralPath $launchEvidence -Encoding utf8
	}
    Set-Content -LiteralPath $pidFile -Value $isolated.ProcessId -Encoding ascii
    if (-not [string]::IsNullOrWhiteSpace($AutomationPath)) {
        $env:PFREMOTE_GOLDEN_CENTER_PID = [string]$isolated.ProcessId
        $automation = (Resolve-Path -LiteralPath $AutomationPath).Path
        $automationExitCode = [IsolatedDesktopProcess]::RunOnDesktop($automation, (Split-Path -Parent $automation), $DesktopName, 90000)
        if ($automationExitCode -ne 0) { throw "Isolated UI automation failed with exit code $automationExitCode." }
    }
    $isolated.Wait()
}
finally {
    $isolated.Dispose()
}
