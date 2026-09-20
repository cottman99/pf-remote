[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$scratchRoot = Join-Path $repoRoot '.codex_tmp'
$binRoot = Join-Path $scratchRoot 'bin'
$runRoot = Join-Path $scratchRoot ("legacy-compatibility-{0}" -f $PID)
$configRoot = Join-Path $runRoot 'config'
$endpoint = "\\.\pipe\pfremote-legacy-compatibility-$PID"
$fixture = Join-Path $repoRoot 'fixtures\migration\legacy-center-catalog-v1.synthetic.json'
$programDataRoot = Join-Path $runRoot 'program-data'
$runtimeCatalogDirectory = Join-Path $programDataRoot 'PFRemoteCenter'
$runtimeCatalog = Join-Path $runtimeCatalogDirectory 'catalog.json'

New-Item -ItemType Directory -Force -Path $binRoot, $configRoot, $runtimeCatalogDirectory | Out-Null
Copy-Item -LiteralPath $fixture -Destination $runtimeCatalog
$fixtureHash = (Get-FileHash -LiteralPath $fixture -Algorithm SHA256).Hash
$previousAppData = $env:APPDATA
$previousEndpoint = $env:PFREMOTE_LOCAL_ENDPOINT
$previousLegacyCatalog = $env:PFREMOTE_LEGACY_CENTER_CATALOG
$previousProgramData = $env:ProgramData
$daemon = $null

try {
    $env:APPDATA = $configRoot
    $env:PFREMOTE_LOCAL_ENDPOINT = $endpoint
    $env:PFREMOTE_LEGACY_CENTER_CATALOG = $null
    $env:ProgramData = $programDataRoot
    $daemon = Start-Process -FilePath (Join-Path $binRoot 'pfremoted.exe') -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput (Join-Path $runRoot 'pfremoted.stdout.log') `
        -RedirectStandardError (Join-Path $runRoot 'pfremoted.stderr.log')

    $ready = $false
    for ($attempt = 0; $attempt -lt 50; $attempt++) {
        if (Test-Path -LiteralPath $endpoint) {
            $ready = $true
            break
        }
        if ($daemon.HasExited) { break }
        Start-Sleep -Milliseconds 100
    }
    if (-not $ready) { throw 'The legacy compatibility daemon did not become ready.' }

    $beforeJson = & (Join-Path $binRoot 'pfremote.exe') list --json
    if ($LASTEXITCODE -ne 0) { throw 'The pre-activation catalog could not be listed.' }
    $before = $beforeJson | ConvertFrom-Json
    if (@($before.targets | Where-Object { $_.device.display_name -eq 'Lab computer' }).Count -ne 0) {
        throw 'The existing setup became active before the user action.'
    }

    & (Join-Path $binRoot 'pfremote-migrate.exe') enable-legacy-center | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'The existing setup could not be enabled.' }

    $catalogJson = & (Join-Path $binRoot 'pfremote.exe') list --json
    if ($LASTEXITCODE -ne 0) { throw 'The legacy compatibility catalog could not be listed.' }
    $catalog = $catalogJson | ConvertFrom-Json
    if ($catalog.targets.Count -ne 3) { throw 'The legacy compatibility catalog did not retain every action.' }

    $desktop = $catalog.targets | Where-Object { $_.capability.display_name -eq 'Engineering desktop' }
    $shell = $catalog.targets | Where-Object { $_.capability.display_name -eq 'Automation' }
    $vnc = $catalog.targets | Where-Object { $_.capability.display_name -eq 'Current screen' }
    if ($null -eq $desktop -or $desktop.capability.state -ne 'available' -or $desktop.capability.desktop_profile.protocol -ne 'rdp') {
        throw 'The identity-proven legacy Desktop was not made available.'
    }
    if ($null -eq $shell -or $shell.capability.state -ne 'setup-required' -or
        $null -eq $vnc -or $vnc.capability.state -ne 'setup-required') {
        throw 'An identity-incomplete legacy action was presented as ready.'
    }

    $contextJson = & (Join-Path $binRoot 'pfremote.exe') context $desktop.canonical --json
    if ($LASTEXITCODE -ne 0) { throw 'The legacy compatibility target context could not be created.' }
    $context = $contextJson | ConvertFrom-Json
    $inspectJson = & (Join-Path $binRoot 'pfremote.exe') inspect $context.target.canonical --json
    if ($LASTEXITCODE -ne 0) { throw 'The handed legacy compatibility target could not be inspected.' }
    $inspect = $inspectJson | ConvertFrom-Json
    if ($inspect.target.canonical -ne $desktop.canonical) {
        throw 'Legacy UI and Agent context resolved different targets.'
    }
    if ((Get-FileHash -LiteralPath $fixture -Algorithm SHA256).Hash -ne $fixtureHash -or
        (Get-FileHash -LiteralPath $runtimeCatalog -Algorithm SHA256).Hash -ne $fixtureHash) {
        throw 'The legacy compatibility source changed during read-only loading.'
    }

    & (Join-Path $binRoot 'pfremote-migrate.exe') disable-legacy-center | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'The existing setup could not be disabled in the new list.' }
    $rolledBackJson = & (Join-Path $binRoot 'pfremote.exe') list --json
    if ($LASTEXITCODE -ne 0) { throw 'The post-rollback local catalog could not be listed.' }
    $rolledBack = $rolledBackJson | ConvertFrom-Json
    $beforeTargets = @($before.targets.canonical | Sort-Object)
    $rolledBackTargets = @($rolledBack.targets.canonical | Sort-Object)
    if ($before.fabric_id -ne $rolledBack.fabric_id -or
        (Compare-Object -ReferenceObject $beforeTargets -DifferenceObject $rolledBackTargets).Count -ne 0) {
        throw 'Rollback did not restore the exact local computer list.'
    }
    if ((Get-FileHash -LiteralPath $runtimeCatalog -Algorithm SHA256).Hash -ne $fixtureHash) {
        throw 'Rollback changed the legacy compatibility source.'
    }

    [pscustomobject]@{
        schema_version = 'pfremote.legacy-compatibility-check/v1'
        status = 'passed'
        computers = @($catalog.targets.device.id | Sort-Object -Unique).Count
        actions = $catalog.targets.Count
        ready_actions = @($catalog.targets | Where-Object { $_.capability.state -eq 'available' }).Count
        setup_required_actions = @($catalog.targets | Where-Object { $_.capability.state -eq 'setup-required' }).Count
        target_match = $true
        rollback_match = $true
        source_unchanged = $true
    } | ConvertTo-Json
}
finally {
    if ($null -ne $daemon -and -not $daemon.HasExited) {
        Stop-Process -Id $daemon.Id -Force
        $daemon.WaitForExit()
    }
    $env:APPDATA = $previousAppData
    $env:PFREMOTE_LOCAL_ENDPOINT = $previousEndpoint
    $env:PFREMOTE_LEGACY_CENTER_CATALOG = $previousLegacyCatalog
    $env:ProgramData = $previousProgramData
    if (Test-Path -LiteralPath $runRoot) {
        $resolvedRun = (Resolve-Path -LiteralPath $runRoot).Path
        $resolvedScratch = (Resolve-Path -LiteralPath $scratchRoot).Path
        if (-not $resolvedRun.StartsWith($resolvedScratch + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
            throw "Refusing to clean unexpected compatibility path: $resolvedRun"
        }
        Remove-Item -LiteralPath $resolvedRun -Recurse -Force
    }
}
