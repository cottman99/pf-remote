[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$')]
    [string]$Version,
    [ValidateSet('x64')]
    [string]$Architecture = 'x64',
    [string]$OutputRoot,
    [string]$PublisherPublicKey,
    [string]$SigningCertificateThumbprint,
    [string]$ArtifactSigningDlibPath,
    [string]$ArtifactSigningMetadataPath,
    [string]$ExpectedPublisherSubject,
    [string]$TimestampUrl,
    [string]$TigerVNCViewerPath,
    [string]$TigerVNCLicensePath,
    [string]$TigerVNCVersion,
    [ValidatePattern('^[0-9A-Fa-f]{64}$')]
    [string]$TigerVNCViewerSha256,
    [string]$TigerVNCSignerSubject,
    [switch]$DevelopmentUnsigned
)

$ErrorActionPreference = 'Stop'
$publisherFlags = ''
if ($PublisherPublicKey) {
    if ([Convert]::FromBase64String($PublisherPublicKey).Length -ne 32) { throw 'Publisher public key must be 32 bytes.' }
    $publisherFlags = "-X github.com/cottman99/pf-remote/internal/autoupdate.PublisherKey=$PublisherPublicKey"
}
$repoRoot = Split-Path -Parent $PSScriptRoot
$scratchRoot = Join-Path $repoRoot '.codex_tmp'
if ([string]::IsNullOrWhiteSpace($OutputRoot)) {
    $OutputRoot = Join-Path $scratchRoot 'releases'
}
$outputRootPath = [IO.Path]::GetFullPath($OutputRoot)
$scratchPath = [IO.Path]::GetFullPath($scratchRoot)
if (-not $outputRootPath.StartsWith($scratchPath + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Windows release output must stay under the repository scratch directory.'
}
$localCertificateSigning = -not [string]::IsNullOrWhiteSpace($SigningCertificateThumbprint)
$artifactSigning = -not [string]::IsNullOrWhiteSpace($ArtifactSigningDlibPath) -or -not [string]::IsNullOrWhiteSpace($ArtifactSigningMetadataPath)
if ($DevelopmentUnsigned -and ($localCertificateSigning -or $artifactSigning)) {
    throw 'Choose either an explicitly unsigned development build or one signing provider.'
}
if (-not $DevelopmentUnsigned -and $localCertificateSigning -eq $artifactSigning) {
    throw 'Promoted Windows release signing requires exactly one provider: a local certificate or Microsoft Artifact Signing.'
}
if ($artifactSigning -and ([string]::IsNullOrWhiteSpace($ArtifactSigningDlibPath) -or [string]::IsNullOrWhiteSpace($ArtifactSigningMetadataPath))) {
    throw 'Microsoft Artifact Signing requires both its SignTool dlib and external metadata file.'
}
if (-not $DevelopmentUnsigned -and ([string]::IsNullOrWhiteSpace($ExpectedPublisherSubject) -or [string]::IsNullOrWhiteSpace($TimestampUrl))) {
    throw 'Promoted Windows release signing requires the exact approved publisher subject and an RFC 3161 timestamp service.'
}
if (-not $DevelopmentUnsigned -and $TimestampUrl -notmatch '^https://|^http://timestamp\.acs\.microsoft\.com/?$') {
    throw 'The timestamp service must use HTTPS or the official Microsoft Artifact Signing timestamp endpoint.'
}
$selectedCertificate = $null
if ($artifactSigning) {
    if (-not (Test-Path -LiteralPath $ArtifactSigningDlibPath -PathType Leaf) -or -not (Test-Path -LiteralPath $ArtifactSigningMetadataPath -PathType Leaf)) {
        throw 'Microsoft Artifact Signing dlib or external metadata file is unavailable.'
    }
    $ArtifactSigningDlibPath = (Resolve-Path -LiteralPath $ArtifactSigningDlibPath).Path
    $ArtifactSigningMetadataPath = (Resolve-Path -LiteralPath $ArtifactSigningMetadataPath).Path
}
elseif ($localCertificateSigning) {
    $selectedCertificate = Get-Item -LiteralPath ("Cert:\CurrentUser\My\{0}" -f $SigningCertificateThumbprint) -ErrorAction SilentlyContinue
    if ($null -eq $selectedCertificate -or -not $selectedCertificate.HasPrivateKey -or $selectedCertificate.NotAfter -le (Get-Date)) {
        throw 'The selected release signing certificate is unavailable, expired, or has no private key.'
    }
    if ($selectedCertificate.Subject -cne $ExpectedPublisherSubject) {
        throw 'The selected release signing certificate does not match the approved publisher subject.'
    }
}
$releaseStatus = if ($DevelopmentUnsigned) { 'development-unsigned' } else { 'development-signed' }
$tigerVNCValues = @($TigerVNCViewerPath, $TigerVNCLicensePath, $TigerVNCVersion, $TigerVNCViewerSha256, $TigerVNCSignerSubject)
if (@($tigerVNCValues | Where-Object { [string]::IsNullOrWhiteSpace($_) }).Count -gt 0) {
    throw 'Windows releases require the TigerVNC viewer, license, version, SHA-256, and exact signer subject.'
}
if (-not (Test-Path -LiteralPath $TigerVNCViewerPath -PathType Leaf) -or -not (Test-Path -LiteralPath $TigerVNCLicensePath -PathType Leaf)) {
    throw 'TigerVNC viewer or license evidence is unavailable.'
}
$TigerVNCViewerPath = (Resolve-Path -LiteralPath $TigerVNCViewerPath).Path
$TigerVNCLicensePath = (Resolve-Path -LiteralPath $TigerVNCLicensePath).Path
if ((Get-FileHash -LiteralPath $TigerVNCViewerPath -Algorithm SHA256).Hash -cne $TigerVNCViewerSha256.ToUpperInvariant()) {
    throw 'TigerVNC viewer hash does not match the reviewed binary.'
}
$viewerSignature = Get-AuthenticodeSignature -LiteralPath $TigerVNCViewerPath
if ($viewerSignature.Status -ne 'Valid' -or $null -eq $viewerSignature.SignerCertificate -or $viewerSignature.SignerCertificate.Subject -cne $TigerVNCSignerSubject) {
    throw 'TigerVNC viewer signature does not match the reviewed publisher.'
}
$viewerLicenseText = Get-Content -LiteralPath $TigerVNCLicensePath -Raw
if ($viewerLicenseText -notmatch 'GNU GENERAL PUBLIC LICENSE\s+Version 2') {
    throw 'TigerVNC license evidence is not GPL version 2.'
}

$runRoot = Join-Path $scratchRoot ("windows-release-{0}" -f $PID)
$payloadRoot = Join-Path $runRoot 'payload'
$releaseRoot = Join-Path $outputRootPath $Version
$runtimeIdentifier = "win-$Architecture"
$platform = $Architecture
$project = Join-Path $repoRoot 'apps\windows\PFRemoteCenter\PFRemoteCenter.csproj'
if (Test-Path -LiteralPath $releaseRoot) {
    throw "Release output already exists; use a fresh version instead of mixing artifacts: $releaseRoot"
}
New-Item -ItemType Directory -Force -Path $payloadRoot, $releaseRoot | Out-Null

function Get-RelativeReleasePath([string]$Root, [string]$Path) {
    $rootPrefix = [IO.Path]::GetFullPath($Root).TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    $fullPath = [IO.Path]::GetFullPath($Path)
    if (-not $fullPath.StartsWith($rootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Release file escaped payload root: $Path"
    }
    return $fullPath.Substring($rootPrefix.Length).Replace('\', '/')
}

function Get-SortedReleaseFiles([string]$Root) {
    $byPath = @{}
    $paths = New-Object 'System.Collections.Generic.List[string]'
    foreach ($file in Get-ChildItem -LiteralPath $Root -Recurse -File) {
        $relative = Get-RelativeReleasePath $Root $file.FullName
        if ($byPath.ContainsKey($relative)) { throw "Release contains a duplicate path: $relative" }
        $byPath[$relative] = $file
        $paths.Add($relative)
    }
    $paths.Sort([StringComparer]::OrdinalIgnoreCase)
    foreach ($relative in $paths) {
        [pscustomobject]@{ Relative = $relative; File = $byPath[$relative] }
    }
}

function Write-DeterministicZip([string]$Source, [string]$Destination) {
    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $stream = [IO.File]::Open($Destination, [IO.FileMode]::Create, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    try {
        $archive = New-Object IO.Compression.ZipArchive($stream, [IO.Compression.ZipArchiveMode]::Create, $false)
        try {
            $epoch = [DateTimeOffset]::new(2026, 1, 1, 0, 0, 0, [TimeSpan]::Zero)
            foreach ($item in Get-SortedReleaseFiles $Source) {
                $file = $item.File
                $relative = $item.Relative
                $entry = $archive.CreateEntry($relative, [IO.Compression.CompressionLevel]::Optimal)
                $entry.LastWriteTime = $epoch
                $input = [IO.File]::OpenRead($file.FullName)
                $output = $entry.Open()
                try { $input.CopyTo($output) }
                finally { $output.Dispose(); $input.Dispose() }
            }
        }
        finally { $archive.Dispose() }
    }
    finally { $stream.Dispose() }
}

function Get-StringSha256([string]$Value) {
    $algorithm = [Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($algorithm.ComputeHash([Text.Encoding]::UTF8.GetBytes($Value)))).Replace('-', '') }
    finally { $algorithm.Dispose() }
}

function Write-Utf8NoBom([string]$Path, [string]$Value) {
    [IO.File]::WriteAllText($Path, $Value + [Environment]::NewLine, (New-Object Text.UTF8Encoding($false)))
}

function Find-SignTool {
    $candidate = Get-ChildItem -LiteralPath 'C:\Program Files (x86)\Windows Kits\10\bin' -Recurse -Filter 'signtool.exe' -File -ErrorAction SilentlyContinue |
        Where-Object { $_.FullName -match '\\x64\\signtool\.exe$' } | Sort-Object FullName -Descending | Select-Object -First 1
    if ($null -eq $candidate) { throw 'Windows SDK SignTool is unavailable.' }
    return $candidate.FullName
}

function Sign-And-Verify(
    [string]$SignTool,
    [string]$Path,
    [string]$Provider,
    [string]$Thumbprint,
    [string]$DlibPath,
    [string]$MetadataPath,
    [string]$TimestampService,
    [string]$PublisherSubject
) {
    if ($Provider -eq 'artifact-signing') {
        & $SignTool sign /fd SHA256 /tr $TimestampService /td SHA256 /dlib $DlibPath /dmdf $MetadataPath $Path | Out-Null
    }
    else {
        & $SignTool sign /fd SHA256 /sha1 $Thumbprint /tr $TimestampService /td SHA256 $Path | Out-Null
    }
    if ($LASTEXITCODE -ne 0) { throw "Release signing failed: $Path" }
    & $SignTool verify /pa /all $Path | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Release signature verification failed: $Path" }
    $signature = Get-AuthenticodeSignature -LiteralPath $Path
    if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -cne $PublisherSubject) {
        throw "Release signer does not match the approved publisher: $Path"
    }
}

function Resolve-LicenseId([string]$PackageName, [string]$LicenseText) {
    if ($PackageName -eq 'Microsoft.Windows.SDK.BuildTools' -or $LicenseText -match 'MICROSOFT WINDOWS SOFTWARE DEVELOPMENT KIT') {
        return 'LicenseRef-Microsoft-Windows-SDK'
    }
    if ($LicenseText -match 'Microsoft Windows ML Runtime') { return 'LicenseRef-Microsoft-Windows-ML-Runtime' }
    if ($LicenseText -match 'MICROSOFT WINDOWS APP SDK') { return 'LicenseRef-Microsoft-Windows-App-SDK' }
    if ($LicenseText -match 'Mozilla Public License,? version 2\.0') { return 'MPL-2.0' }
    if ($LicenseText -match 'Apache License\s+Version 2\.0') { return 'Apache-2.0' }
    if ($LicenseText -match 'GNU GENERAL PUBLIC LICENSE\s+Version 2') { return 'GPL-2.0-only' }
    if ($LicenseText -match 'Permission is hereby granted, free of charge') { return 'MIT' }
    if ($LicenseText -match 'Redistribution and use in source and binary forms') {
        if ($LicenseText -match 'Neither (the )?name') { return 'BSD-3-Clause' }
        return 'BSD-2-Clause'
    }
    throw "Dependency license is not recognized: $PackageName"
}

function Find-LicenseFile([string]$Root, [string]$PreferredName) {
    if (-not [string]::IsNullOrWhiteSpace($PreferredName)) {
        $preferred = Join-Path $Root $PreferredName
        if (Test-Path -LiteralPath $preferred) { return Get-Item -LiteralPath $preferred }
        $leaf = Split-Path -Leaf $PreferredName
        $found = Get-ChildItem -LiteralPath $Root -Recurse -File -ErrorAction SilentlyContinue |
            Where-Object Name -eq $leaf | Select-Object -First 1
        if ($null -ne $found) { return $found }
    }
    return Get-ChildItem -LiteralPath $Root -File -ErrorAction SilentlyContinue |
        Where-Object Name -Match '^(LICENSE|COPYING)(\..*)?$' | Select-Object -First 1
}

Push-Location $repoRoot
try {
    dotnet publish $project -c Release -r $runtimeIdentifier --self-contained true `
        -p:Platform=$platform -p:PublishTrimmed=false -p:PublishReadyToRun=false -p:DebugType=None -p:DebugSymbols=false --nologo -o $payloadRoot
    if ($LASTEXITCODE -ne 0) { throw 'Windows Center publish failed.' }

    # The WinUI SDK leaves compiled XAML and PRI assets in TargetDir when
    # dotnet publish uses an explicit output directory. A package without them
    # builds successfully but crashes before its first window appears.
    $compiledResource = Get-ChildItem -LiteralPath (Join-Path $repoRoot "apps\windows\PFRemoteCenter\bin\$platform\Release") `
        -Recurse -Filter 'PFRemoteCenter.pri' -File | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if ($null -eq $compiledResource) { throw 'Compiled WinUI resource index is missing.' }
    $winuiBuildRoot = Split-Path -Parent $compiledResource.FullName
    foreach ($resourceName in @('App.xbf', 'MainPage.xbf', 'MainWindow.xbf', 'PFRemoteCenter.pri')) {
        $resourcePath = Join-Path $winuiBuildRoot $resourceName
        if (-not (Test-Path -LiteralPath $resourcePath)) { throw "Compiled WinUI resource is missing: $resourceName" }
        Copy-Item -LiteralPath $resourcePath -Destination (Join-Path $payloadRoot $resourceName)
    }
    $assetRoot = Join-Path $winuiBuildRoot 'Assets'
    if (-not (Test-Path -LiteralPath (Join-Path $assetRoot 'AppIcon.ico'))) { throw 'Compiled WinUI assets are missing.' }
    Copy-Item -LiteralPath $assetRoot -Destination (Join-Path $payloadRoot 'Assets') -Recurse

    $goCommands = [ordered]@{
        'pfremote.exe' = './cmd/pfremote'
        'pfremote-gateway.exe' = './cmd/pfremote-gateway'
        'pfremote-mcp.exe' = './cmd/pfremote-mcp'
        'pfremote-migrate.exe' = './cmd/pfremote-migrate'
        'pfremote-recovery.exe' = './cmd/pfremote-recovery'
        'pfremoted.exe' = './cmd/pfremoted'
    }
    foreach ($name in $goCommands.Keys) {
        go build -trimpath -ldflags "-s -w -X main.releaseVersion=$Version $publisherFlags" -o (Join-Path $payloadRoot $name) $goCommands[$name]
        if ($LASTEXITCODE -ne 0) { throw "Go release build failed: $name" }
    }

    $viewerDirectory = Join-Path $payloadRoot 'protocol-executors\tigervnc'
    New-Item -ItemType Directory -Force -Path $viewerDirectory | Out-Null
    Copy-Item -LiteralPath $TigerVNCViewerPath -Destination (Join-Path $viewerDirectory 'vncviewer.exe')
    Copy-Item -LiteralPath $TigerVNCLicensePath -Destination (Join-Path $viewerDirectory 'LICENCE.TXT')

    Copy-Item -LiteralPath (Join-Path $repoRoot 'LICENSE') -Destination (Join-Path $payloadRoot 'LICENSE.txt')
    Copy-Item -LiteralPath (Join-Path $repoRoot 'skills\pf-remote') -Destination (Join-Path $payloadRoot 'skills\pf-remote') -Recurse

    $goModules = @(go list -m -f '{{.Path}}|{{.Version}}' all | ForEach-Object {
        $parts = $_ -split '\|', 2
        [pscustomobject]@{ Path = $parts[0]; Version = $parts[1] }
    })
    if ($LASTEXITCODE -ne 0) { throw 'Go dependency inventory failed.' }
    $dotnetInventory = dotnet list $project package --include-transitive --format json | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0) { throw '.NET dependency inventory failed.' }
    $nugetPackages = @()
    foreach ($framework in $dotnetInventory.projects.frameworks) {
        foreach ($package in @($framework.topLevelPackages) + @($framework.transitivePackages)) {
            if ($null -ne $package) {
                $nugetPackages += [pscustomobject][ordered]@{ name = $package.id; version = $package.resolvedVersion; source = 'nuget' }
            }
        }
    }
    $dependencyInputs = @(
        $goModules | Where-Object Path -ne 'github.com/cottman99/pf-remote' | ForEach-Object {
            [pscustomobject][ordered]@{ name = $_.Path; version = $_.Version; source = 'go' }
        }
    ) + $nugetPackages
    $dependencyInputs = @($dependencyInputs | Sort-Object source, name, version -Unique)

    $nugetLocation = dotnet nuget locals global-packages --list
    if ($LASTEXITCODE -ne 0 -or $nugetLocation -notmatch '^global-packages:\s*(.+)$') { throw 'NuGet package cache could not be located.' }
    $nugetRoot = $Matches[1].Trim()
    $dependencies = @()
    $noticeSections = New-Object 'System.Collections.Generic.List[string]'
    foreach ($dependency in $dependencyInputs) {
        $licenseText = ''
        $evidenceLabel = ''
        $distributionScope = 'runtime-or-transitive'
        if ($dependency.source -eq 'go') {
            $download = (go mod download -json ("{0}@{1}" -f $dependency.name, $dependency.version) | ConvertFrom-Json)
            if ($LASTEXITCODE -ne 0 -or $null -eq $download.Dir) { throw "Go dependency source could not be inspected: $($dependency.name)" }
            $licenseFile = Find-LicenseFile $download.Dir ''
            if ($null -eq $licenseFile) { throw "Go dependency license file is missing: $($dependency.name)" }
            $licenseText = Get-Content -LiteralPath $licenseFile.FullName -Raw
            $evidenceLabel = $licenseFile.Name
        }
        else {
            $packageRoot = Join-Path $nugetRoot (([string]$dependency.name).ToLowerInvariant() + '\' + $dependency.version)
            $nuspec = Get-ChildItem -LiteralPath $packageRoot -Filter '*.nuspec' -File -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($null -eq $nuspec) { throw "NuGet package metadata is missing: $($dependency.name)" }
            [xml]$metadata = Get-Content -LiteralPath $nuspec.FullName -Raw
            $licenseNode = $metadata.package.metadata.license
            $preferredLicense = if ($null -ne $licenseNode -and $licenseNode.type -eq 'file') { [string]$licenseNode.InnerText } else { '' }
            $licenseFile = Find-LicenseFile $packageRoot $preferredLicense
            if ($null -ne $licenseFile) {
                $licenseText = Get-Content -LiteralPath $licenseFile.FullName -Raw
                $evidenceLabel = $licenseFile.Name
            }
            elseif ($dependency.name -eq 'Microsoft.Windows.SDK.BuildTools') {
                $licenseText = 'Microsoft Windows SDK license: https://aka.ms/WinSDKLicenseURL'
                $evidenceLabel = 'NuGet metadata license URL'
                $distributionScope = 'build-only'
            }
            else {
                throw "NuGet dependency license file is missing: $($dependency.name)"
            }
            if ($dependency.name -eq 'Microsoft.Windows.SDK.BuildTools.MSIX') { $distributionScope = 'build-only' }
        }
        $licenseId = Resolve-LicenseId $dependency.name $licenseText
        $evidenceHash = Get-StringSha256 $licenseText
        $noticeSections.Add(("{0} {1}`nSource: {2}`nLicense: {3}`nEvidence: {4}`n`n{5}`n" -f $dependency.name, $dependency.version, $dependency.source, $licenseId, $evidenceLabel, $licenseText.Trim()))
        $dependencies += [pscustomobject][ordered]@{
            name = $dependency.name
            version = $dependency.version
            source = $dependency.source
            declared_license = $licenseId
            distribution_scope = $distributionScope
            review_status = 'engineering-reviewed'
            evidence_sha256 = $evidenceHash.ToLowerInvariant()
        }
    }
    $viewerLicenseText = Get-Content -LiteralPath $TigerVNCLicensePath -Raw
    $viewerLicenseHash = Get-StringSha256 $viewerLicenseText
    $noticeSections.Add(("TigerVNC {0}`nSource: external-binary`nLicense: GPL-2.0-only`nEvidence: LICENCE.TXT`n`n{1}`n" -f $TigerVNCVersion, $viewerLicenseText.Trim()))
    $dependencies += [pscustomobject][ordered]@{
        name = 'TigerVNC Viewer'
        version = $TigerVNCVersion
        source = 'external-binary'
        declared_license = 'GPL-2.0-only'
        distribution_scope = 'separate-protocol-executor'
        review_status = 'engineering-reviewed'
        evidence_sha256 = $viewerLicenseHash.ToLowerInvariant()
    }
    $noticePathInPayload = Join-Path $payloadRoot 'THIRD-PARTY-NOTICES.txt'
    Write-Utf8NoBom $noticePathInPayload (($noticeSections | ForEach-Object { $_ }) -join "`n-------------------------------------------------------------------------------`n")

    $signTool = $null
    if (-not $DevelopmentUnsigned) {
        $signTool = Find-SignTool
        $signingProvider = if ($artifactSigning) { 'artifact-signing' } else { 'local-certificate' }
        foreach ($executable in Get-ChildItem -LiteralPath $payloadRoot -Recurse -Filter '*.exe' -File) {
            Sign-And-Verify $signTool $executable.FullName $signingProvider $SigningCertificateThumbprint $ArtifactSigningDlibPath $ArtifactSigningMetadataPath $TimestampUrl $ExpectedPublisherSubject
        }
    }

    $files = @()
    foreach ($item in Get-SortedReleaseFiles $payloadRoot) {
        $file = $item.File
        $files += [ordered]@{
            path = $item.Relative
            size = $file.Length
            sha256 = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
    $manifest = [ordered]@{
        schema_version = 'pfremote.windows-release/v1'
        product = 'PF Remote'
        version = $Version
        architecture = $Architecture
        created_at = '2026-01-01T00:00:00Z'
        files = $files
    }
    $manifestPath = Join-Path $releaseRoot 'release-manifest.json'
    Write-Utf8NoBom $manifestPath ($manifest | ConvertTo-Json -Depth 6)

    $licenseInventory = [ordered]@{
        schema_version = 'pfremote.license-inventory/v1'
        product_license = 'Apache-2.0'
        review_status = 'engineering-review-complete'
        dependencies = @($dependencies | ForEach-Object {
            [ordered]@{
                name = $_.name
                version = $_.version
                source = $_.source
                declared_license = $_.declared_license
                distribution_scope = $_.distribution_scope
                review_status = $_.review_status
                evidence_sha256 = $_.evidence_sha256
            }
        })
    }
    $licensePath = Join-Path $releaseRoot 'licenses.json'
    Write-Utf8NoBom $licensePath ($licenseInventory | ConvertTo-Json -Depth 6)

    $sbom = [ordered]@{
        spdxVersion = 'SPDX-2.3'
        dataLicense = 'CC0-1.0'
        SPDXID = 'SPDXRef-DOCUMENT'
        name = "PF-Remote-Windows-$Architecture-$Version"
        documentNamespace = "https://pfremote.example.invalid/spdx/$Version/$Architecture"
        creationInfo = [ordered]@{ created = '2026-01-01T00:00:00Z'; creators = @('Tool: PF Remote release builder') }
        packages = @($dependencies | ForEach-Object {
            [ordered]@{
                SPDXID = 'SPDXRef-Package-' + (Get-StringSha256 "$($_.source):$($_.name):$($_.version)").Substring(0, 16)
                name = $_.name
                versionInfo = $_.version
                downloadLocation = 'NOASSERTION'
                filesAnalyzed = $false
                licenseConcluded = $_.declared_license
                licenseDeclared = $_.declared_license
            }
        })
        files = @($files | ForEach-Object {
            [ordered]@{ SPDXID = 'SPDXRef-File-' + $_.sha256.Substring(0, 16); fileName = './' + $_.path; checksums = @([ordered]@{ algorithm = 'SHA256'; checksumValue = $_.sha256 }) }
        })
    }
    $sbomPath = Join-Path $releaseRoot 'sbom.spdx.json'
    Write-Utf8NoBom $sbomPath ($sbom | ConvertTo-Json -Depth 8)

    $archivePath = Join-Path $releaseRoot "PFRemote-Windows-$Architecture-$Version.zip"
    Write-DeterministicZip $payloadRoot $archivePath
    $archiveHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    $manifestHash = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $setupPath = Join-Path $releaseRoot 'PFRemoteSetup.exe'
    go build -trimpath -ldflags "-s -w -H=windowsgui -X main.releaseVersion=$Version -X main.releaseArchitecture=$Architecture -X main.payloadSHA256=$archiveHash -X main.manifestSHA256=$manifestHash $publisherFlags" -o $setupPath ./cmd/pfremote-setup
    if ($LASTEXITCODE -ne 0) { throw 'PF Remote Setup build failed.' }
    if (-not $DevelopmentUnsigned) {
        Sign-And-Verify $signTool $setupPath $signingProvider $SigningCertificateThumbprint $ArtifactSigningDlibPath $ArtifactSigningMetadataPath $TimestampUrl $ExpectedPublisherSubject
    }
    $published = @($archivePath, $setupPath, $manifestPath, $sbomPath, $licensePath)
    $hashLines = foreach ($artifact in $published | Sort-Object { Split-Path -Leaf $_ }) {
        '{0}  {1}' -f (Get-FileHash -LiteralPath $artifact -Algorithm SHA256).Hash.ToLowerInvariant(), (Split-Path -Leaf $artifact)
    }
    $hashPath = Join-Path $releaseRoot 'SHA256SUMS.txt'
    $hashLines | Set-Content -LiteralPath $hashPath -Encoding ascii

    [pscustomobject]@{
        schema_version = 'pfremote.windows-release-build/v1'
        status = $releaseStatus
        version = $Version
        architecture = $Architecture
        payload_files = $files.Count
        dependencies = $dependencies.Count
        output = $releaseRoot
    } | ConvertTo-Json
}
finally {
    Pop-Location
    if (Test-Path -LiteralPath $runRoot) {
        $resolvedRun = (Resolve-Path -LiteralPath $runRoot).Path
        if (-not $resolvedRun.StartsWith($scratchPath + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
            throw "Refusing to clean unexpected release scratch path: $resolvedRun"
        }
        Remove-Item -LiteralPath $resolvedRun -Recurse -Force
    }
}
