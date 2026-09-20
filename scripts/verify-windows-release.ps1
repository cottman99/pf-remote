[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$ReleaseRoot,
    [string]$ExpectedPublisherSubject,
    [switch]$AllowDevelopmentUnsigned
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$scratchRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.codex_tmp'))
$release = (Resolve-Path -LiteralPath $ReleaseRoot).Path
$manifestPath = Join-Path $release 'release-manifest.json'
$licensePath = Join-Path $release 'licenses.json'
$sbomPath = Join-Path $release 'sbom.spdx.json'
$hashPath = Join-Path $release 'SHA256SUMS.txt'
$setupPath = Join-Path $release 'PFRemoteSetup.exe'
foreach ($required in @($manifestPath, $licensePath, $sbomPath, $hashPath, $setupPath)) {
    if (-not (Test-Path -LiteralPath $required)) { throw "Release artifact is missing: $(Split-Path -Leaf $required)" }
}

$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
if ($manifest.schema_version -ne 'pfremote.windows-release/v1') { throw 'Release manifest schema is invalid.' }
$archivePath = Join-Path $release ("PFRemote-Windows-{0}-{1}.zip" -f $manifest.architecture, $manifest.version)
if (-not (Test-Path -LiteralPath $archivePath)) { throw 'Release payload archive is missing.' }

$publishedHashes = @{}
foreach ($line in Get-Content -LiteralPath $hashPath) {
    if ($line -notmatch '^([0-9a-fA-F]{64})\s{2}([^\\/]+)$') { throw 'Published hash list contains an invalid line.' }
    $publishedHashes[$Matches[2]] = $Matches[1].ToLowerInvariant()
}
foreach ($artifact in @($archivePath, $setupPath, $manifestPath, $sbomPath, $licensePath)) {
    $name = Split-Path -Leaf $artifact
    $actual = (Get-FileHash -LiteralPath $artifact -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($publishedHashes[$name] -ne $actual) { throw "Published hash does not match: $name" }
}

Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [IO.Compression.ZipFile]::OpenRead($archivePath)
try {
    $entries = @($zip.Entries | Where-Object { -not [string]::IsNullOrEmpty($_.Name) })
    $manifestFiles = @($manifest.files)
    if ($entries.Count -ne $manifestFiles.Count) { throw 'Release archive and manifest have different file counts.' }
    $entryByPath = @{}
    foreach ($entry in $entries) {
        if ($entryByPath.ContainsKey($entry.FullName)) { throw "Release archive contains a duplicate path: $($entry.FullName)" }
        $entryByPath[$entry.FullName] = $entry
    }
    $previous = $null
    foreach ($file in $manifestFiles) {
        if ($null -ne $previous -and [StringComparer]::OrdinalIgnoreCase.Compare($previous, [string]$file.path) -ge 0) {
            throw 'Release manifest paths are not strictly sorted.'
        }
        $previous = [string]$file.path
        $entry = $entryByPath[[string]$file.path]
        if ($null -eq $entry -or $entry.Length -ne [int64]$file.size) { throw "Release payload entry is missing or has the wrong size: $($file.path)" }
        $stream = $entry.Open()
        $algorithm = [Security.Cryptography.SHA256]::Create()
        try { $actual = ([BitConverter]::ToString($algorithm.ComputeHash($stream))).Replace('-', '').ToLowerInvariant() }
        finally { $algorithm.Dispose(); $stream.Dispose() }
        if ($actual -ne $file.sha256) { throw "Release payload entry hash does not match: $($file.path)" }
    }
    foreach ($requiredPayload in @('PFRemoteCenter.exe', 'pfremoted.exe', 'pfremote-gateway.exe', 'PFRemoteCenter.pri', 'App.xbf', 'MainPage.xbf', 'MainWindow.xbf', 'THIRD-PARTY-NOTICES.txt', 'skills/pf-remote/SKILL.md', 'skills/pf-remote/agents/openai.yaml', 'skills/pf-remote/scripts/pfremote.ps1', 'skills/pf-remote/scripts/pfremote-mcp.ps1')) {
        if (-not $entryByPath.ContainsKey($requiredPayload)) { throw "Required release payload is missing: $requiredPayload" }
    }
}
finally { $zip.Dispose() }

$licenses = Get-Content -LiteralPath $licensePath -Raw | ConvertFrom-Json
if ($licenses.schema_version -ne 'pfremote.license-inventory/v1' -or $licenses.review_status -ne 'engineering-review-complete') {
    throw 'Dependency license review is incomplete.'
}
foreach ($dependency in $licenses.dependencies) {
    if ([string]::IsNullOrWhiteSpace($dependency.declared_license) -or $dependency.declared_license -eq 'NOASSERTION' -or $dependency.review_status -ne 'engineering-reviewed') {
        throw "Dependency license is unresolved: $($dependency.name)"
    }
}
$sbom = Get-Content -LiteralPath $sbomPath -Raw | ConvertFrom-Json
if ($sbom.spdxVersion -ne 'SPDX-2.3' -or @($sbom.files).Count -ne @($manifest.files).Count) { throw 'Release SBOM is incomplete.' }

$setupSignature = Get-AuthenticodeSignature -LiteralPath $setupPath
if ($AllowDevelopmentUnsigned) {
    if ($setupSignature.Status -eq 'Valid') { throw 'Development-unsigned verification was requested for a signed installer.' }
}
elseif ($setupSignature.Status -ne 'Valid') {
    throw 'Promoted installer signature is not trusted.'
}
elseif ([string]::IsNullOrWhiteSpace($ExpectedPublisherSubject) -or $setupSignature.SignerCertificate.Subject -cne $ExpectedPublisherSubject) {
    throw 'Promoted installer publisher does not match the approved subject.'
}

$verificationRoot = Join-Path $scratchRoot ("release-verification-{0}" -f $PID)
$installRoot = Join-Path $verificationRoot 'installation'
$sentinel = Join-Path $verificationRoot 'outside-installation.txt'
if (Test-Path -LiteralPath $verificationRoot) { throw "Fresh release verification path already exists: $verificationRoot" }
New-Item -ItemType Directory -Path $verificationRoot | Out-Null
Set-Content -LiteralPath $sentinel -Value 'preserve' -Encoding ascii
$previousTesting = [Environment]::GetEnvironmentVariable('PFREMOTE_SETUP_TESTING', 'Process')
$previousLocalAppData = [Environment]::GetEnvironmentVariable('LOCALAPPDATA', 'Process')
$previousAppData = [Environment]::GetEnvironmentVariable('APPDATA', 'Process')
$previousRegistryPath = [Environment]::GetEnvironmentVariable('PFREMOTE_SETUP_REGISTRY_PATH', 'Process')
$previousRunRegistryPath = [Environment]::GetEnvironmentVariable('PFREMOTE_SETUP_RUN_REGISTRY_PATH', 'Process')
$env:PFREMOTE_SETUP_TESTING = 'true'
try {
    $install = Start-Process -FilePath $setupPath -ArgumentList @('install', '--root', $installRoot, '--no-launch') -Wait -PassThru
    if ($install.ExitCode -ne 0) { throw 'Isolated release installation failed.' }
    $state = Get-Content -LiteralPath (Join-Path $installRoot 'current.json') -Raw | ConvertFrom-Json
    if ($state.current -ne $manifest.version) { throw 'Installed version does not match the verified release.' }
    $appPath = Join-Path $installRoot ("versions\{0}\PFRemoteCenter.exe" -f $manifest.version)
    if (-not (Test-Path -LiteralPath $appPath)) { throw 'Installed PF Remote Center is missing.' }
    if (-not $AllowDevelopmentUnsigned) {
        $appSignature = Get-AuthenticodeSignature -LiteralPath $appPath
        if ($appSignature.Status -ne 'Valid' -or $appSignature.SignerCertificate.Subject -cne $ExpectedPublisherSubject) {
            throw 'Installed PF Remote Center signature or publisher is not trusted.'
        }
    }
    $uninstall = Start-Process -FilePath $setupPath -ArgumentList @('uninstall', '--root', $installRoot, '--no-launch') -Wait -PassThru
    if ($uninstall.ExitCode -ne 0 -or (Test-Path -LiteralPath $installRoot)) { throw 'Isolated release uninstall failed.' }
    if (-not (Test-Path -LiteralPath $sentinel)) { throw 'Release uninstall removed data outside its owned installation root.' }

    $localAppData = Join-Path $verificationRoot 'LocalAppData'
    $appData = Join-Path $verificationRoot 'Roaming'
    New-Item -ItemType Directory -Path $localAppData, $appData | Out-Null
    $env:LOCALAPPDATA = $localAppData
    $env:APPDATA = $appData
    $env:PFREMOTE_SETUP_REGISTRY_PATH = "Software\PFRemoteTests\ReleaseVerification-$PID"
    $env:PFREMOTE_SETUP_RUN_REGISTRY_PATH = "Software\PFRemoteTests\ReleaseVerification-$PID-Run"
    $defaultInstallRoot = Join-Path $localAppData 'Programs\PFRemote'
    $shortcutPath = Join-Path $appData 'Microsoft\Windows\Start Menu\Programs\PF Remote.lnk'
    $helperPath = Join-Path $localAppData 'PFRemote\Installer\PFRemoteSetup.exe'
    $registryProviderPath = "Registry::HKEY_CURRENT_USER\$($env:PFREMOTE_SETUP_REGISTRY_PATH)"
    $runRegistryProviderPath = "Registry::HKEY_CURRENT_USER\$($env:PFREMOTE_SETUP_RUN_REGISTRY_PATH)"
    $defaultInstall = Start-Process -FilePath $setupPath -ArgumentList @('install', '--no-launch') -Wait -PassThru
    if ($defaultInstall.ExitCode -ne 0 -or -not (Test-Path -LiteralPath $shortcutPath) -or -not (Test-Path -LiteralPath $helperPath) -or
        -not (Test-Path -LiteralPath $runRegistryProviderPath)) {
        throw 'Ordinary-user installation did not create its Windows entry points.'
    }
    $startupCommand = (Get-ItemProperty -LiteralPath $runRegistryProviderPath).PFRemote
    if ($startupCommand -notmatch 'PFRemoteSetup\.exe" startup --no-launch$') { throw 'Ordinary-user background startup is not version-independent.' }
    $installedApps = Get-ItemProperty -LiteralPath $registryProviderPath
    if ($installedApps.DisplayVersion -ne $manifest.version -or $installedApps.DisplayName -ne 'PF Remote') {
        throw 'Windows Installed apps registration does not match the verified release.'
    }
    $settingsUninstall = Start-Process -FilePath $helperPath -ArgumentList @('uninstall', '--no-launch') -Wait -PassThru
    if ($settingsUninstall.ExitCode -ne 0) { throw 'Windows Installed apps uninstall path failed.' }
    for ($attempt = 0; $attempt -lt 50 -and (Test-Path -LiteralPath $helperPath); $attempt++) { Start-Sleep -Milliseconds 100 }
    if ((Test-Path -LiteralPath $defaultInstallRoot) -or (Test-Path -LiteralPath $shortcutPath) -or
        (Test-Path -LiteralPath $helperPath) -or (Test-Path -LiteralPath $registryProviderPath) -or
        ((Get-ItemProperty -LiteralPath $runRegistryProviderPath -ErrorAction SilentlyContinue).PFRemote)) {
        throw 'Windows Installed apps uninstall left an owned application entry or executable.'
    }
}
finally {
    [Environment]::SetEnvironmentVariable('PFREMOTE_SETUP_TESTING', $previousTesting, 'Process')
    [Environment]::SetEnvironmentVariable('LOCALAPPDATA', $previousLocalAppData, 'Process')
    [Environment]::SetEnvironmentVariable('APPDATA', $previousAppData, 'Process')
    [Environment]::SetEnvironmentVariable('PFREMOTE_SETUP_REGISTRY_PATH', $previousRegistryPath, 'Process')
    [Environment]::SetEnvironmentVariable('PFREMOTE_SETUP_RUN_REGISTRY_PATH', $previousRunRegistryPath, 'Process')
    if (Test-Path -LiteralPath $verificationRoot) {
        $resolvedVerification = (Resolve-Path -LiteralPath $verificationRoot).Path
        if (-not $resolvedVerification.StartsWith($scratchRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
            throw "Refusing to clean unexpected verification path: $resolvedVerification"
        }
        Remove-Item -LiteralPath $resolvedVerification -Recurse -Force
    }
}

[pscustomobject]@{
    schema_version = 'pfremote.windows-release-verification/v1'
    status = 'passed'
    version = $manifest.version
    architecture = $manifest.architecture
    payload_files = @($manifest.files).Count
    dependencies = @($licenses.dependencies).Count
    signature = [string]$setupSignature.Status
} | ConvertTo-Json
