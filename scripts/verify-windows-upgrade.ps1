[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$PreviousReleaseRoot,
    [Parameter(Mandatory = $true)][string]$ReleaseRoot
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$scratch = [IO.Path]::GetFullPath((Join-Path $repoRoot '.codex_tmp'))
$testRoot = Join-Path $scratch ("upgrade-verification-{0}" -f $PID)
if (Test-Path -LiteralPath $testRoot) { throw 'Fresh upgrade verification directory already exists.' }
$previousSetup = Join-Path (Resolve-Path -LiteralPath $PreviousReleaseRoot).Path 'PFRemoteSetup.exe'
$nextSetup = Join-Path (Resolve-Path -LiteralPath $ReleaseRoot).Path 'PFRemoteSetup.exe'
$oldVersion = (Get-Content -LiteralPath (Join-Path $PreviousReleaseRoot 'release-manifest.json') -Raw | ConvertFrom-Json).version
$newVersion = (Get-Content -LiteralPath (Join-Path $ReleaseRoot 'release-manifest.json') -Raw | ConvertFrom-Json).version
if ($oldVersion -eq $newVersion) { throw 'Upgrade verification requires distinct versions.' }
$installation = Join-Path $testRoot 'program'
$runtime = Join-Path $testRoot 'synthetic-runtime'
New-Item -ItemType Directory -Path $runtime -Force | Out-Null
$savedTesting = [Environment]::GetEnvironmentVariable('PFREMOTE_SETUP_TESTING', 'Process')
$env:PFREMOTE_SETUP_TESTING = 'true'
$savedProfile = @{}
foreach ($name in @('APPDATA', 'LOCALAPPDATA', 'ProgramData', 'PFREMOTE_LOCAL_ENDPOINT', 'PFREMOTE_GATEWAY_URL', 'PFREMOTE_GATEWAY_OWNER_SYNC', 'PFREMOTE_LEGACY_CENTER_CATALOG')) {
    $savedProfile[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
$env:APPDATA = Join-Path $testRoot 'Roaming'
$env:LOCALAPPDATA = Join-Path $testRoot 'Local'
$env:ProgramData = Join-Path $testRoot 'ProgramData'
$env:PFREMOTE_LOCAL_ENDPOINT = "\\.\pipe\pfremote-upgrade-verification-$PID"
$env:PFREMOTE_GATEWAY_URL = $null
$env:PFREMOTE_GATEWAY_OWNER_SYNC = $null
$env:PFREMOTE_LEGACY_CENTER_CATALOG = $null
New-Item -ItemType Directory -Force -Path $env:APPDATA, $env:LOCALAPPDATA, $env:ProgramData | Out-Null

function Invoke-IsolatedSetup([string]$Executable, [string]$Action) {
    $process = Start-Process -FilePath $Executable -ArgumentList @($Action, '--root', ('"' + $installation + '"'), '--no-launch') -WindowStyle Hidden -PassThru
    if (-not $process.WaitForExit(60000)) {
        Stop-Process -Id $process.Id -Force
        throw 'Isolated setup timed out.'
    }
    if ($process.ExitCode -ne 0) { throw "Isolated setup failed: $Action" }
}

function Read-IsolatedProfile([string]$Version) {
    $versionRoot = Join-Path $installation ("versions\{0}" -f $Version)
    $daemon = Start-Process -FilePath (Join-Path $versionRoot 'pfremoted.exe') -WindowStyle Hidden -PassThru `
        -RedirectStandardOutput (Join-Path $testRoot 'daemon.out') -RedirectStandardError (Join-Path $testRoot 'daemon.err')
    try {
        $ready = $false
        for ($attempt = 0; $attempt -lt 100; $attempt++) {
            if (Test-Path -LiteralPath $env:PFREMOTE_LOCAL_ENDPOINT) { $ready = $true; break }
            if ($daemon.HasExited) { break }
            Start-Sleep -Milliseconds 100
        }
        if (-not $ready) { throw 'Isolated packaged daemon did not become ready.' }
        $catalogText = & (Join-Path $versionRoot 'pfremote.exe') list --json
        if ($LASTEXITCODE -ne 0) { throw 'Packaged client could not load retained profile.' }
        $catalog = $catalogText | ConvertFrom-Json
        $targets = @($catalog.targets | ForEach-Object { $_.canonical } | Sort-Object)
        if ($targets.Count -eq 0) { throw 'Isolated profile has no retained targets.' }
        $identityHash = (Get-FileHash -LiteralPath (Join-Path $env:APPDATA 'PF Remote\identity\device-identity-v1.json')).Hash
        return [pscustomobject]@{ identity = $identityHash; targets = ($targets -join "`n") }
    }
    finally {
        if (-not $daemon.HasExited) { Stop-Process -Id $daemon.Id -Force; $null = $daemon.WaitForExit(5000) }
    }
}

try {
    $hashes = @{}
    foreach ($name in @('identity.json', 'credentials.json', 'history.db', 'gateway.json', 'tls.key', 'legacy.json')) {
        $path = Join-Path $runtime $name
        Set-Content -LiteralPath $path -Value ("synthetic retained {0}" -f $name) -Encoding ascii
        $hashes[$name] = (Get-FileHash -LiteralPath $path).Hash
    }
    Invoke-IsolatedSetup $previousSetup 'install'
    $beforeProfile = Read-IsolatedProfile $oldVersion
    Invoke-IsolatedSetup $nextSetup 'install'
    $afterProfile = Read-IsolatedProfile $newVersion
    if ($beforeProfile.identity -ne $afterProfile.identity -or $beforeProfile.targets -cne $afterProfile.targets) { throw 'Upgrade did not load the original identity and targets.' }
    $state = Get-Content -LiteralPath (Join-Path $installation 'current.json') -Raw | ConvertFrom-Json
    if ($state.current -ne $newVersion -or $state.previous -ne $oldVersion) { throw 'Upgrade selected unexpected versions.' }
    if (Test-Path -LiteralPath (Join-Path $installation 'pending-upgrade.json')) { throw 'Successful upgrade left pending recovery.' }
    Invoke-IsolatedSetup $nextSetup 'rollback'
    $state = Get-Content -LiteralPath (Join-Path $installation 'current.json') -Raw | ConvertFrom-Json
    if ($state.current -ne $oldVersion) { throw 'Rollback did not restore the previous package.' }
    $rollbackProfile = Read-IsolatedProfile $oldVersion
    if ($beforeProfile.identity -ne $rollbackProfile.identity -or $beforeProfile.targets -cne $rollbackProfile.targets) { throw 'Rollback did not load the original identity and targets.' }
    foreach ($name in $hashes.Keys) {
        if ((Get-FileHash -LiteralPath (Join-Path $runtime $name)).Hash -ne $hashes[$name]) { throw 'Synthetic user data changed.' }
    }
    [pscustomobject]@{ status = 'passed'; previous = $oldVersion; candidate = $newVersion; retained_files = $hashes.Count; profile_reload = 'identity-and-targets-preserved'; automatic_activation_failure = 'unit-tested' } | ConvertTo-Json
}
finally {
    [Environment]::SetEnvironmentVariable('PFREMOTE_SETUP_TESTING', $savedTesting, 'Process')
    foreach ($name in $savedProfile.Keys) { [Environment]::SetEnvironmentVariable($name, $savedProfile[$name], 'Process') }
    $resolved = (Resolve-Path -LiteralPath $testRoot).Path
    if (-not $resolved.StartsWith($scratch + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Refusing unexpected cleanup path.' }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
