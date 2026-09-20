[CmdletBinding()]
param(
    [switch]$SkipWindowsApp
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$scratch = Join-Path $repoRoot '.codex_tmp'
$binDir = Join-Path $scratch 'bin'
$tempDir = Join-Path $scratch 'temp'
$nugetDir = Join-Path $scratch 'nuget-packages'
New-Item -ItemType Directory -Force -Path $binDir, $tempDir, $nugetDir | Out-Null
$previousTemp = [Environment]::GetEnvironmentVariable('TEMP', 'Process')
$previousTmp = [Environment]::GetEnvironmentVariable('TMP', 'Process')
$previousNugetPackages = [Environment]::GetEnvironmentVariable('NUGET_PACKAGES', 'Process')
$previousTestingTelemetry = [Environment]::GetEnvironmentVariable('TESTINGPLATFORM_TELEMETRY_OPTOUT', 'Process')
$env:TEMP = $tempDir
$env:TMP = $tempDir
$env:NUGET_PACKAGES = $nugetDir
$env:TESTINGPLATFORM_TELEMETRY_OPTOUT = '1'

Push-Location $repoRoot
try {
    & (Join-Path $PSScriptRoot 'check-private-data.ps1')

    go run ./scripts/governancecheck
    if ($LASTEXITCODE -ne 0) { throw 'Execution-governance consistency check failed.' }

    $unformatted = @(gofmt -l cmd internal pkg scripts/governancecheck)
    if ($unformatted.Count -gt 0) {
        throw "Unformatted Go files: $($unformatted -join ', ')"
    }
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }

    go build -o (Join-Path $binDir 'pfremote.exe') ./cmd/pfremote
    go build -o (Join-Path $binDir 'pfremote-mcp.exe') ./cmd/pfremote-mcp
    go build -o (Join-Path $binDir 'pfremote-migrate.exe') ./cmd/pfremote-migrate
    go build -o (Join-Path $binDir 'pfremote-recovery.exe') ./cmd/pfremote-recovery
    go build -o (Join-Path $binDir 'pfremoted.exe') ./cmd/pfremoted
    go build -o (Join-Path $binDir 'pfremote-gateway.exe') ./cmd/pfremote-gateway

    & (Join-Path $PSScriptRoot 'verify-connection-invitation.ps1') | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Connection invitation live-reload check failed.' }

    $migrationFixture = Join-Path $repoRoot 'fixtures/migration/legacy-inventory-v1.synthetic.json'
    $migrationPlanA = Join-Path $tempDir 'migration-plan-a.json'
    $migrationPlanB = Join-Path $tempDir 'migration-plan-b.json'
    & (Join-Path $binDir 'pfremote-migrate.exe') plan --input $migrationFixture | Set-Content -LiteralPath $migrationPlanA -Encoding utf8
    if ($LASTEXITCODE -ne 0) { throw 'Synthetic migration plan failed.' }
    & (Join-Path $binDir 'pfremote-migrate.exe') plan --input $migrationFixture | Set-Content -LiteralPath $migrationPlanB -Encoding utf8
    if ($LASTEXITCODE -ne 0 -or (Get-FileHash -LiteralPath $migrationPlanA).Hash -ne (Get-FileHash -LiteralPath $migrationPlanB).Hash) {
        throw 'Synthetic migration plan is not deterministic.'
    }
    $legacyObservation = Join-Path $repoRoot 'fixtures/migration/observation-legacy-v1.synthetic.json'
    $candidateObservation = Join-Path $repoRoot 'fixtures/migration/observation-candidate-v1.synthetic.json'
    $observationReport = Join-Path $tempDir 'migration-observation-report.json'
    & (Join-Path $binDir 'pfremote-migrate.exe') observe --legacy $legacyObservation --candidate $candidateObservation | Set-Content -LiteralPath $observationReport -Encoding utf8
    if ($LASTEXITCODE -ne 0 -or (Get-Content -LiteralPath $observationReport -Raw | ConvertFrom-Json).overall -ne 'ready') {
        throw 'Synthetic side-by-side migration observation failed.'
    }
    $observationReview = Join-Path $tempDir 'migration-observation-review.md'
    $reviewProcess = New-Object System.Diagnostics.Process
    $reviewProcess.StartInfo = New-Object System.Diagnostics.ProcessStartInfo
    $reviewProcess.StartInfo.FileName = Join-Path $binDir 'pfremote-migrate.exe'
    $reviewProcess.StartInfo.Arguments = "observe --legacy `"$legacyObservation`" --candidate `"$candidateObservation`" --format review --locale zh-CN"
    $reviewProcess.StartInfo.UseShellExecute = $false
    $reviewProcess.StartInfo.CreateNoWindow = $true
    $reviewProcess.StartInfo.RedirectStandardOutput = $true
    $reviewProcess.StartInfo.RedirectStandardError = $true
    $reviewProcess.StartInfo.StandardOutputEncoding = New-Object System.Text.UTF8Encoding($false)
    if (-not $reviewProcess.Start()) { throw 'Product-manager migration review did not start.' }
    $reviewText = $reviewProcess.StandardOutput.ReadToEnd()
    $reviewError = $reviewProcess.StandardError.ReadToEnd()
    $reviewProcess.WaitForExit()
    $reviewExitCode = $reviewProcess.ExitCode
    $reviewText | Set-Content -LiteralPath $observationReview -Encoding utf8
    # Keep the script parseable by Windows PowerShell 5, which treats a
    # BOM-less .ps1 file as the active ANSI code page rather than UTF-8.
    $readyDecisionText = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('5Y+v5Lul6L+b5YWl5Y+X5o6n5YiH5o2i5YeG5aSH'))
    $noMutationText = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('5rKh5pyJ5L+u5pS544CB5YGc5q2i5oiW5YiH5o2i5Lu75L2V5pyN5Yqh'))
    $reviewHasReadyDecision = $reviewText.Contains($readyDecisionText)
    $reviewHasNoMutationStatement = $reviewText.Contains($noMutationText)
    if ($reviewExitCode -ne 0 -or -not $reviewHasReadyDecision -or -not $reviewHasNoMutationStatement) {
        throw "Product-manager migration review failed (exit=$reviewExitCode, ready=$reviewHasReadyDecision, no-mutation=$reviewHasNoMutationStatement)."
    }

    $smokeRoot = Join-Path $scratch ("daemon-smoke-{0}" -f $PID)
    $smokeConfig = Join-Path $smokeRoot 'config'
    New-Item -ItemType Directory -Force -Path $smokeConfig | Out-Null
    $previousAppData = [Environment]::GetEnvironmentVariable('APPDATA', 'Process')
    $previousLocalEndpoint = [Environment]::GetEnvironmentVariable('PFREMOTE_LOCAL_ENDPOINT', 'Process')
    $env:APPDATA = $smokeConfig
    $env:PFREMOTE_LOCAL_ENDPOINT = "\\.\pipe\pfremote-check-$PID"
    $daemon = $null
    try {
        $daemon = Start-Process -FilePath (Join-Path $binDir 'pfremoted.exe') -PassThru -WindowStyle Hidden `
            -RedirectStandardOutput (Join-Path $smokeRoot 'pfremoted.stdout.log') `
            -RedirectStandardError (Join-Path $smokeRoot 'pfremoted.stderr.log')
        $ready = $false
        for ($attempt = 0; $attempt -lt 50; $attempt++) {
            if (Test-Path -LiteralPath $env:PFREMOTE_LOCAL_ENDPOINT) {
                $ready = $true
                break
            }
            if ($daemon.HasExited) { break }
            Start-Sleep -Milliseconds 100
        }
        if (-not $ready) { throw 'PF Remote daemon did not open its isolated local endpoint.' }
        & (Join-Path $binDir 'pfremote.exe') list --json | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'CLI-to-daemon smoke test failed.' }
        $updateDoctor = & (Join-Path $binDir 'pfremote.exe') doctor --json | ConvertFrom-Json
        if ($LASTEXITCODE -ne 0 -or @($updateDoctor.checks | Where-Object { $_.name -eq 'updates' -and $_.status -eq 'skip' }).Count -ne 1) {
            throw 'Unprovisioned daemon did not report its trusted-update bootstrap requirement.'
        }
    }
    finally {
        if ($null -ne $daemon -and -not $daemon.HasExited) {
            Stop-Process -Id $daemon.Id -Force
            $daemon.WaitForExit()
        }
        [Environment]::SetEnvironmentVariable('APPDATA', $previousAppData, 'Process')
        [Environment]::SetEnvironmentVariable('PFREMOTE_LOCAL_ENDPOINT', $previousLocalEndpoint, 'Process')
        if (Test-Path -LiteralPath $smokeRoot) {
            $resolvedSmoke = (Resolve-Path -LiteralPath $smokeRoot).Path
            $resolvedScratch = (Resolve-Path -LiteralPath $scratch).Path
            if (-not $resolvedSmoke.StartsWith($resolvedScratch + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
                throw "Refusing to clean unexpected smoke path: $resolvedSmoke"
            }
            Remove-Item -LiteralPath $resolvedSmoke -Recurse -Force
        }
    }

    & (Join-Path $PSScriptRoot 'verify-agent-alignment.ps1') | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Center-to-Agent target alignment check failed.' }

    & (Join-Path $PSScriptRoot 'verify-legacy-compatibility.ps1') | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Legacy compatibility target alignment check failed.' }

    $winProject = Join-Path $repoRoot 'apps/windows/PFRemoteCenter/PFRemoteCenter.csproj'
    if (-not $SkipWindowsApp -and (Test-Path -LiteralPath $winProject)) {
        $platform = switch ($env:PROCESSOR_ARCHITECTURE) {
            'ARM64' { 'ARM64' }
            'x86' { 'x86' }
            default { 'x64' }
        }
        $testProject = Join-Path $repoRoot 'apps/windows/PFRemoteCenter.Tests/PFRemoteCenter.Tests.csproj'
        $goldenUiProject = Join-Path $repoRoot 'scripts/goldenjourneyui/GoldenJourneyUI.csproj'
        $centerSource = Get-Content -LiteralPath (Join-Path $repoRoot 'apps/windows/PFRemoteCenter/MainPage.xaml.cs') -Raw
        $resourceKeys = [regex]::Matches($centerSource, '_resources\.GetString\("(?<key>[^"]+)"\)') |
            ForEach-Object { $_.Groups['key'].Value.Replace('/', '.') } |
            Sort-Object -Unique
        foreach ($culture in @('en-US', 'zh-CN')) {
            [xml]$resourceDocument = Get-Content -LiteralPath (Join-Path $repoRoot "apps/windows/PFRemoteCenter/Strings/$culture/Resources.resw") -Raw
            $availableKeys = @($resourceDocument.root.data | ForEach-Object { [string]$_.name })
            $missingKeys = @($resourceKeys | Where-Object { $_ -notin $availableKeys })
            if ($missingKeys.Count -gt 0) {
                throw "Windows Center $culture resources are missing code-referenced keys: $($missingKeys -join ', ')"
            }
        }
        dotnet test --project $testProject -c Debug --minimum-expected-tests 9
        if ($LASTEXITCODE -ne 0) { throw 'Windows Center presentation tests failed.' }
        dotnet run --project (Join-Path $repoRoot 'scripts/singleinstancecheck/SingleInstanceCheck.csproj') -c Release
        if ($LASTEXITCODE -ne 0) { throw 'Windows Center single-instance process checks failed.' }
        dotnet build $goldenUiProject --configuration Release --nologo
        if ($LASTEXITCODE -ne 0) { throw 'Golden-journey UI helper build failed.' }
        dotnet build $winProject --configuration Debug -p:Platform=$platform --nologo
        if ($LASTEXITCODE -ne 0) { throw 'Windows Center build failed.' }
    }

    Write-Host 'PF Remote checks passed.'
}
finally {
    Pop-Location
    [Environment]::SetEnvironmentVariable('TEMP', $previousTemp, 'Process')
    [Environment]::SetEnvironmentVariable('TMP', $previousTmp, 'Process')
    [Environment]::SetEnvironmentVariable('NUGET_PACKAGES', $previousNugetPackages, 'Process')
    [Environment]::SetEnvironmentVariable('TESTINGPLATFORM_TELEMETRY_OPTOUT', $previousTestingTelemetry, 'Process')
}
