[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$scratch = Join-Path $repoRoot '.codex_tmp'
$binRoot = Join-Path $scratch 'bin'
$runRoot = Join-Path $scratch ("connection-invitation-{0}" -f $PID)
$configRoot = Join-Path $runRoot 'config'
New-Item -ItemType Directory -Force -Path $configRoot | Out-Null

$previousAppData = $env:APPDATA
$previousEndpoint = $env:PFREMOTE_LOCAL_ENDPOINT
$env:APPDATA = $configRoot
$env:PFREMOTE_LOCAL_ENDPOINT = "\\.\pipe\pfremote-connection-invitation-$PID"
$daemon = $null
try {
    $daemon = Start-Process -FilePath (Join-Path $binRoot 'pfremoted.exe') -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput (Join-Path $runRoot 'daemon.stdout.log') `
        -RedirectStandardError (Join-Path $runRoot 'daemon.stderr.log')
    $ready = $false
    for ($attempt = 0; $attempt -lt 50; $attempt++) {
        try {
            & (Join-Path $binRoot 'pfremote.exe') list --json 2>$null | Out-Null
            $listExitCode = $LASTEXITCODE
        }
        catch {
            # A structured daemon-unavailable response is expected while the
            # isolated process is still entering its protected IPC loop.
            $listExitCode = if ($null -eq $LASTEXITCODE) { 1 } else { $LASTEXITCODE }
        }
        if ($listExitCode -eq 0) { $ready = $true; break }
        if ($daemon.HasExited) { break }
        Start-Sleep -Milliseconds 100
    }
    if (-not $ready) { throw 'The isolated daemon did not become ready.' }

    $invitationPath = Join-Path $runRoot 'join.pfremote-link'
    go run ./scripts/invitationfixture $invitationPath
    if ($LASTEXITCODE -ne 0) { throw 'The isolated invitation could not be created.' }
    $configure = & (Join-Path $binRoot 'pfremote-migrate.exe') configure-connection-service --invitation $invitationPath 2>&1
    if ($LASTEXITCODE -ne 0) { throw "The invitation was rejected: $configure" }
    if (($configure | Out-String) -match '127\.0\.0\.1') { throw 'Connection status exposed the private endpoint.' }

    $observed = 'skip'
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        $doctor = & (Join-Path $binRoot 'pfremote.exe') doctor --json | ConvertFrom-Json
        $observed = ($doctor.checks | Where-Object name -eq 'connection-service').status
        if ($observed -ne 'skip') { break }
        Start-Sleep -Milliseconds 250
    }
    if ($observed -ne 'pending' -and $observed -ne 'pass') {
        throw "The running daemon did not discover the invitation profile: $observed"
    }
    Write-Host "Connection invitation live reload passed ($observed)."
}
finally {
    if ($null -ne $daemon -and -not $daemon.HasExited) {
        Stop-Process -Id $daemon.Id
        $daemon.WaitForExit()
    }
    $env:APPDATA = $previousAppData
    $env:PFREMOTE_LOCAL_ENDPOINT = $previousEndpoint
    $resolvedRun = (Resolve-Path -LiteralPath $runRoot -ErrorAction SilentlyContinue).Path
    $resolvedScratch = (Resolve-Path -LiteralPath $scratch).Path
    if ($resolvedRun -and $resolvedRun.StartsWith($resolvedScratch + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        Remove-Item -LiteralPath $resolvedRun -Recurse -Force
    }
}
