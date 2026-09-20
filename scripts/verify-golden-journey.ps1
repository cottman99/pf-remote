[CmdletBinding()]
param(
    [switch]$WithCodex,
    [switch]$SkipDotnetBuild,
    [string]$EvidenceRoot
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$scratchRoot = Join-Path $repoRoot '.codex_tmp'
if ([string]::IsNullOrWhiteSpace($EvidenceRoot)) {
    $EvidenceRoot = Join-Path $repoRoot 'docs\status\evidence'
}
$evidencePath = [IO.Path]::GetFullPath($EvidenceRoot)
$allowedEvidence = [IO.Path]::GetFullPath((Join-Path $repoRoot 'docs\status\evidence'))
if (-not $evidencePath.StartsWith($allowedEvidence, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Golden-journey evidence must stay under docs/status/evidence.'
}

$runRoot = Join-Path $scratchRoot ("golden-journey-{0}" -f $PID)
$configRoot = Join-Path $runRoot 'config'
$localAppData = Join-Path $runRoot 'local-app-data'
$binRoot = Join-Path $runRoot 'bin'
$endpoint = "\\.\pipe\pfremote-golden-journey-$PID"
$desktopRecordPath = Join-Path $runRoot 'desktop-record.json'
$agentRecordPath = Join-Path $runRoot 'agent-record.json'
$contextPath = Join-Path $runRoot 'handed-context.txt'
$uiResultPath = Join-Path $runRoot 'ui-automation.json'
$screenshotPath = Join-Path $evidencePath 'M5_4_GOLDEN.png'
$reportPath = Join-Path $evidencePath 'M5_4_GOLDEN.json'
$desktopName = "PFRemoteGolden$PID"
$project = Join-Path $repoRoot 'apps\windows\PFRemoteCenter\PFRemoteCenter.csproj'
$appRoot = Join-Path $repoRoot 'apps\windows\PFRemoteCenter\bin\x64\Debug\net10.0-windows10.0.26100.0\win-x64'
$appPath = Join-Path $appRoot 'PFRemoteCenter.exe'
$fixturePath = Join-Path $binRoot 'pfremote-golden-fixture.exe'
$mcpPath = Join-Path $appRoot 'pfremote-mcp.exe'
$uiProject = Join-Path $repoRoot 'scripts\goldenjourneyui\GoldenJourneyUI.csproj'
$uiRoot = Join-Path $repoRoot 'scripts\goldenjourneyui\bin\Release\net10.0-windows'
$uiPath = Join-Path $uiRoot 'GoldenJourneyUI.exe'

New-Item -ItemType Directory -Force -Path $runRoot, $configRoot, $localAppData, $binRoot, $evidencePath | Out-Null

$source = @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Threading;

public sealed class IsolatedGoldenApp : IDisposable
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
    [DllImport("user32.dll", SetLastError = true)]
    private static extern bool PostMessage(IntPtr hwnd, uint message, IntPtr wParam, IntPtr lParam);
    [DllImport("user32.dll")]
    private static extern bool CloseDesktop(IntPtr desktop);
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern bool CreateProcess(string application, string commandLine, IntPtr processAttributes, IntPtr threadAttributes, bool inheritHandles, uint flags, IntPtr environment, string directory, ref STARTUPINFO startup, out PROCESS_INFORMATION process);
    [DllImport("kernel32.dll")]
    private static extern uint WaitForSingleObject(IntPtr handle, uint milliseconds);
    [DllImport("kernel32.dll")]
    private static extern bool GetExitCodeProcess(IntPtr handle, out uint exitCode);
    [DllImport("kernel32.dll")]
    private static extern bool TerminateProcess(IntPtr handle, uint exitCode);
    [DllImport("kernel32.dll")]
    private static extern bool CloseHandle(IntPtr handle);

    private IntPtr desktop;
    private IntPtr process;
    private IntPtr window;
    public int ProcessId { get; private set; }

    public static IsolatedGoldenApp Start(string application, string directory, string desktopName)
    {
        var result = new IsolatedGoldenApp();
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

    public static uint RunOnDesktop(string application, string directory, string desktopName, int timeoutMilliseconds)
    {
        var startup = new STARTUPINFO { cb = Marshal.SizeOf<STARTUPINFO>(), lpDesktop = desktopName };
        PROCESS_INFORMATION created;
        if (!CreateProcess(application, null, IntPtr.Zero, IntPtr.Zero, false, 0, IntPtr.Zero, directory, ref startup, out created))
            throw new Win32Exception();
        CloseHandle(created.hThread);
        try
        {
            uint wait = WaitForSingleObject(created.hProcess, (uint)timeoutMilliseconds);
            if (wait == 258)
            {
                TerminateProcess(created.hProcess, 1);
                throw new TimeoutException("Golden-journey UI helper did not finish.");
            }
            uint exitCode;
            if (wait != 0 || !GetExitCodeProcess(created.hProcess, out exitCode)) throw new Win32Exception();
            return exitCode;
        }
        finally { CloseHandle(created.hProcess); }
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

    public void Dispose()
    {
        if (process != IntPtr.Zero)
        {
            if (WaitForSingleObject(process, 0) != 0)
            {
                if (window != IntPtr.Zero) PostMessage(window, 0x0010, IntPtr.Zero, IntPtr.Zero);
                if (WaitForSingleObject(process, 5000) != 0) TerminateProcess(process, 1);
            }
            CloseHandle(process);
            process = IntPtr.Zero;
        }
        if (desktop != IntPtr.Zero) { CloseDesktop(desktop); desktop = IntPtr.Zero; }
    }
}
'@

Add-Type -TypeDefinition $source

function Wait-NonEmptyFile([string]$Path, [int]$TimeoutSeconds = 10) {
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    do {
        if ((Test-Path -LiteralPath $Path) -and (Get-Item -LiteralPath $Path).Length -gt 0) { return }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "Golden-journey evidence was not produced: $Path"
}

function Invoke-McpCall([System.Diagnostics.Process]$Process, [int]$Id, [string]$Name, [hashtable]$Arguments) {
    $message = [ordered]@{
        jsonrpc = '2.0'
        id = $Id
        method = 'tools/call'
        params = [ordered]@{ name = $Name; arguments = $Arguments }
    } | ConvertTo-Json -Depth 8 -Compress
    $Process.StandardInput.WriteLine($message)
    $Process.StandardInput.Flush()
    $line = $Process.StandardOutput.ReadLine()
    if ([string]::IsNullOrWhiteSpace($line)) { throw "PF Remote MCP returned no response for $Name." }
    $response = $line | ConvertFrom-Json
    if ($null -ne $response.error -or $response.result.isError) { throw "PF Remote MCP failed: $Name" }
    return $response.result.structuredContent
}

$previousAppData = $env:APPDATA
$previousLocalAppData = $env:LOCALAPPDATA
$previousEndpoint = $env:PFREMOTE_LOCAL_ENDPOINT
$previousLegacyCatalog = $env:PFREMOTE_LEGACY_CENTER_CATALOG
$previousContextExport = $env:PFREMOTE_CONTEXT_EXPORT_PATH
$previousVisualAudit = $env:PFREMOTE_VISUAL_AUDIT_PATH
$previousGoldenCenterPid = $env:PFREMOTE_GOLDEN_CENTER_PID
$previousGoldenUiResult = $env:PFREMOTE_GOLDEN_UI_RESULT_PATH
$previousGoldenDesktopId = $env:PFREMOTE_GOLDEN_DESKTOP_AUTOMATION_ID
$previousGoldenDesktopAlias = $env:PFREMOTE_GOLDEN_DESKTOP_DEVICE_ALIAS
$previousGoldenAgentId = $env:PFREMOTE_GOLDEN_AGENT_AUTOMATION_ID
$previousGoldenDesktopRecord = $env:PFREMOTE_GOLDEN_DESKTOP_RECORD_PATH
$previousGoldenContext = $env:PFREMOTE_GOLDEN_CONTEXT_PATH
$previousGoldenRepeatSession = $env:PFREMOTE_GOLDEN_REPEAT_SESSION_AUTOMATION_ID
$fixture = $null
$app = $null
$mcp = $null

try {
    Push-Location $repoRoot
    try {
        if (-not $SkipDotnetBuild) {
            dotnet build $project -c Debug -p:Platform=x64 --nologo | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'PF Remote Center build failed.' }
            dotnet build $uiProject -c Release --nologo | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'Golden-journey UI helper build failed.' }
        }
        go build -o $fixturePath ./scripts/alignmentfixture
        if ($LASTEXITCODE -ne 0) { throw 'Golden-journey target fixture build failed.' }
    }
    finally { Pop-Location }

    $env:APPDATA = $configRoot
    $env:LOCALAPPDATA = $localAppData
    $env:PFREMOTE_LOCAL_ENDPOINT = $endpoint
    $env:PFREMOTE_LEGACY_CENTER_CATALOG = Join-Path $runRoot 'no-existing-setup.json'
    $env:PFREMOTE_CONTEXT_EXPORT_PATH = $contextPath
    $env:PFREMOTE_VISUAL_AUDIT_PATH = $screenshotPath

    $fixture = Start-Process -FilePath $fixturePath -ArgumentList @('--record', $agentRecordPath, '--desktop-record', $desktopRecordPath) `
        -PassThru -WindowStyle Hidden -RedirectStandardOutput (Join-Path $runRoot 'fixture.stdout.log') `
        -RedirectStandardError (Join-Path $runRoot 'fixture.stderr.log')
    for ($attempt = 0; $attempt -lt 50 -and -not (Test-Path -LiteralPath $endpoint); $attempt++) {
        if ($fixture.HasExited) { break }
        Start-Sleep -Milliseconds 100
    }
    if (-not (Test-Path -LiteralPath $endpoint)) { throw 'Golden-journey target fixture did not become ready.' }

    $journey = [Diagnostics.Stopwatch]::StartNew()
    $app = [IsolatedGoldenApp]::Start($appPath, $appRoot, $desktopName)
    $windowHandle = $app.WaitForWindow(20000)
    if (-not $app.IsResponsive()) { throw 'PF Remote Center isolated top-level window is not responsive.' }

    $env:PFREMOTE_GOLDEN_CENTER_PID = $app.ProcessId
    $env:PFREMOTE_GOLDEN_UI_RESULT_PATH = $uiResultPath
    $env:PFREMOTE_GOLDEN_DESKTOP_AUTOMATION_ID = 'primary-device-compute'
    $env:PFREMOTE_GOLDEN_DESKTOP_DEVICE_ALIAS = 'compute-node'
    $env:PFREMOTE_GOLDEN_AGENT_AUTOMATION_ID = 'agent-device-compute'
    $env:PFREMOTE_GOLDEN_DESKTOP_RECORD_PATH = $desktopRecordPath
    $env:PFREMOTE_GOLDEN_CONTEXT_PATH = $contextPath
    $env:PFREMOTE_GOLDEN_REPEAT_SESSION_AUTOMATION_ID = 'repeat-session-isolated-desktop-open'
    $uiExitCode = [IsolatedGoldenApp]::RunOnDesktop($uiPath, $uiRoot, $desktopName, 60000)
    Wait-NonEmptyFile $uiResultPath
    $uiResult = Get-Content -LiteralPath $uiResultPath -Raw | ConvertFrom-Json
    if ($uiExitCode -ne 0 -or $uiResult.status -ne 'passed' -or $uiResult.input_injection_used -or
        -not $uiResult.desktop_keyboard_focusable -or -not $uiResult.agent_keyboard_focusable -or
        -not $uiResult.repeat_session_visible) {
        throw "Golden-journey native UI actions failed: $($uiResult.error)"
    }

    $desktopRecord = Get-Content -LiteralPath $desktopRecordPath -Raw | ConvertFrom-Json
    Wait-NonEmptyFile $screenshotPath
    $contextEnvelope = Get-Content -LiteralPath $contextPath -Raw
    if ($contextEnvelope -notmatch '(?m)^target: (?<target>pfremote://[^\r\n]+)$') {
        throw 'The native Center did not export an exact canonical PF Remote target.'
    }
    $handedTarget = $Matches.target
    if ($handedTarget -notmatch '/devices/(?<device>[^/]+)/capabilities/') { throw 'Handed target has no immutable Device identity.' }
    $handedDevice = $Matches.device
    if ($desktopRecord.target -notmatch '/devices/(?<device>[^/]+)/capabilities/') { throw 'Desktop action has no immutable Device identity.' }
    $desktopDevice = $Matches.device
    if ($handedDevice -ne $desktopDevice -or $desktopRecord.action -ne 'open') {
        throw 'The native Desktop action and Agent handoff selected different computers.'
    }

    $agentMode = if ($WithCodex) { 'codex-mcp' } else { 'direct-mcp' }
    if ($WithCodex) {
        $codex = Get-Command codex -ErrorAction SilentlyContinue
        if ($null -eq $codex) { throw 'Codex is unavailable for the golden journey.' }
        $schemaPath = Join-Path $runRoot 'codex-result.schema.json'
        $resultPath = Join-Path $runRoot 'codex-result.json'
        @'
{
  "type": "object",
  "additionalProperties": false,
  "required": ["status", "target", "action_status", "action_output"],
  "properties": {
    "status": { "type": "string", "enum": ["verified"] },
    "target": { "type": "string" },
    "action_status": { "type": "string", "enum": ["completed"] },
    "action_output": { "type": "string" }
  }
}
'@ | Set-Content -LiteralPath $schemaPath -Encoding utf8
        $prompt = @"
Use the PF Remote Skill at $repoRoot\skills\pf-remote\SKILL.md and the configured PF Remote MCP tools.
The user supplied this exact context envelope from the PF Remote Center:

$contextEnvelope

Do not infer another target. Inspect this exact canonical target, then use PF Remote MCP exec on the same target with the exact argument vector ["fixture-task", "--golden-journey"]. Return only the required structured result. Report verified only when both calls succeed.
"@
        $mcpConfig = "mcp_servers.pf_remote.command='$mcpPath'"
        $endpointConfig = "mcp_servers.pf_remote.env.PFREMOTE_LOCAL_ENDPOINT='$endpoint'"
        & $codex.Source exec --approve-for-me --skip-git-repo-check --ephemeral --ignore-user-config `
            --config $mcpConfig --config $endpointConfig --output-schema $schemaPath `
            --output-last-message $resultPath --cd $repoRoot $prompt | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Codex did not complete the PF Remote golden journey.' }
        $agentResult = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json
        if ($agentResult.status -ne 'verified' -or $agentResult.target -ne $handedTarget -or
            $agentResult.action_status -ne 'completed' -or $agentResult.action_output -ne "isolated target action completed`n") {
            throw 'Codex did not act on the exact target handed over by PF Remote Center.'
        }
    }
    else {
        $start = New-Object Diagnostics.ProcessStartInfo
        $start.FileName = $mcpPath
        $start.UseShellExecute = $false
        $start.CreateNoWindow = $true
        $start.RedirectStandardInput = $true
        $start.RedirectStandardOutput = $true
        $start.RedirectStandardError = $true
        $mcp = [Diagnostics.Process]::Start($start)
        $inspect = Invoke-McpCall $mcp 1 'pfremote_inspect' @{ target = $handedTarget }
        if ($inspect.target.canonical -ne $handedTarget) { throw 'MCP inspection resolved a different target.' }
        $execution = Invoke-McpCall $mcp 2 'pfremote_exec' @{ target = $handedTarget; command = @('fixture-task', '--golden-journey') }
        if ($execution.target.canonical -ne $handedTarget -or $execution.status -ne 'completed') {
            throw 'MCP did not act on the exact target handed over by PF Remote Center.'
        }
        $mcp.StandardInput.Close()
        $mcp.WaitForExit(5000) | Out-Null
    }

    Wait-NonEmptyFile $agentRecordPath
    $agentRecord = Get-Content -LiteralPath $agentRecordPath -Raw | ConvertFrom-Json
    if ($agentRecord.action -ne 'exec' -or $agentRecord.target -ne $handedTarget -or
        $agentRecord.command.Count -ne 2 -or $agentRecord.command[0] -ne 'fixture-task' -or
        $agentRecord.command[1] -ne '--golden-journey') {
        throw 'The Agent action runner received a different target or argument vector.'
    }
    $journey.Stop()
    if ($journey.Elapsed -gt [TimeSpan]::FromMinutes(15)) { throw 'Golden journey exceeded fifteen minutes.' }

    $result = [ordered]@{
        schema_version = 'pfremote.golden-journey/v1'
        status = 'passed'
        elapsed_seconds = [Math]::Round($journey.Elapsed.TotalSeconds, 3)
        user_steps = 2
        visible_computer = 'compute-node'
        desktop_target = $desktopRecord.target
        handed_target = $handedTarget
        immutable_device_match = $true
        agent_mode = $agentMode
        agent_action = 'completed'
        responsive_top_level_window = $true
        desktop_button = $uiResult.desktop_button_name
        agent_button = $uiResult.agent_button_name
        keyboard_accessible = $true
        active_desktop_input_used = $false
        private_target_used = $false
        legacy_changed = $false
        public_binary_published = $false
        screenshot = 'M5_4_GOLDEN.png'
    }
    [IO.File]::WriteAllText($reportPath, (($result | ConvertTo-Json -Depth 5) + [Environment]::NewLine), (New-Object Text.UTF8Encoding($false)))
    $result | ConvertTo-Json -Depth 5
}
finally {
    if ($null -ne $mcp -and -not $mcp.HasExited) { $mcp.Kill(); $mcp.WaitForExit() }
    if ($null -ne $app) { $app.Dispose() }
    if ($null -ne $fixture -and -not $fixture.HasExited) { Stop-Process -Id $fixture.Id -Force; $fixture.WaitForExit() }
    $env:APPDATA = $previousAppData
    $env:LOCALAPPDATA = $previousLocalAppData
    $env:PFREMOTE_LOCAL_ENDPOINT = $previousEndpoint
    $env:PFREMOTE_LEGACY_CENTER_CATALOG = $previousLegacyCatalog
    $env:PFREMOTE_CONTEXT_EXPORT_PATH = $previousContextExport
    $env:PFREMOTE_VISUAL_AUDIT_PATH = $previousVisualAudit
    $env:PFREMOTE_GOLDEN_CENTER_PID = $previousGoldenCenterPid
    $env:PFREMOTE_GOLDEN_UI_RESULT_PATH = $previousGoldenUiResult
    $env:PFREMOTE_GOLDEN_DESKTOP_AUTOMATION_ID = $previousGoldenDesktopId
    $env:PFREMOTE_GOLDEN_DESKTOP_DEVICE_ALIAS = $previousGoldenDesktopAlias
    $env:PFREMOTE_GOLDEN_AGENT_AUTOMATION_ID = $previousGoldenAgentId
    $env:PFREMOTE_GOLDEN_DESKTOP_RECORD_PATH = $previousGoldenDesktopRecord
    $env:PFREMOTE_GOLDEN_CONTEXT_PATH = $previousGoldenContext
    $env:PFREMOTE_GOLDEN_REPEAT_SESSION_AUTOMATION_ID = $previousGoldenRepeatSession
    if (Test-Path -LiteralPath $runRoot) {
        $resolvedRun = (Resolve-Path -LiteralPath $runRoot).Path
        $resolvedScratch = (Resolve-Path -LiteralPath $scratchRoot).Path
        if (-not $resolvedRun.StartsWith($resolvedScratch + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
            throw "Refusing to clean unexpected golden-journey path: $resolvedRun"
        }
        Remove-Item -LiteralPath $resolvedRun -Recurse -Force
    }
}
