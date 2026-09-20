[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$OutputPath,
    [string]$PrivateTermsPath,
    [ValidatePattern('^[0-9a-f]{7,40}$')][string]$PublicBaseCommit
)

# Export the reviewed working source, not Git metadata, release payloads or state.
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$output = [IO.Path]::GetFullPath($OutputPath)
if (Test-Path -LiteralPath $output) { throw 'Choose a new output path; exports are never overwritten.' }
$privateTerms = @()
if ($PrivateTermsPath) {
    $privateTerms = @(Get-Content -LiteralPath $PrivateTermsPath | Where-Object { $_.Trim().Length -ge 4 })
}
Push-Location $repoRoot
try {
    $files = @(git -c core.quotepath=false ls-files --cached --others --exclude-standard | Sort-Object -Unique)
    if ($LASTEXITCODE -ne 0) { throw 'Cannot enumerate source files.' }
    $selected = @()
    $violations = @()
    foreach ($relative in $files) {
        # Evidence captures may contain personal names even when source is clean.
        if ($relative -match '^docs/status/evidence/' -or $relative -eq 'design-qa.md') { continue }
        if ($relative -match '(^|/)(\.git|\.codex_tmp|bin|obj)/' -or
            $relative -match '^(data|backups|secrets|identity|dist|artifacts)/' -or
            $relative -match '(?i)\.(exe|dll|pdb|zip|db|sqlite[0-9]?|log|key|pem|pfx|p12|bak)$') {
            $violations += $relative
            continue
        }
        $path = Join-Path $repoRoot $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { continue }
        if ((Get-Item -LiteralPath $path).Attributes -band [IO.FileAttributes]::ReparsePoint) {
            $violations += $relative
            continue
        }
        $isAsset = $relative -match '^apps/windows/PFRemoteCenter/Assets/[^/]+\.(png|ico)$'
        if (-not $isAsset) {
            if ($relative -match '(?i)\.(png|jpg|jpeg|pdf|gif|vsdx)$') { $violations += $relative; continue }
            $content = [IO.File]::ReadAllText($path)
            foreach ($term in $privateTerms) {
                if ($content.IndexOf($term, [StringComparison]::OrdinalIgnoreCase) -ge 0) {
                    $violations += $relative
                    break
                }
            }
            $privateAddress = $false
            foreach ($match in [regex]::Matches($content, '\b(?:10\.[0-9]+\.[0-9]+\.[0-9]+|192\.168\.[0-9]+\.[0-9]+|172\.(?:1[6-9]|2[0-9]|3[01])\.[0-9]+\.[0-9]+)\b')) {
                $address = $null
                if ([Net.IPAddress]::TryParse($match.Value, [ref]$address)) { $privateAddress = $true }
            }
            if ($content -match '-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----' -or $privateAddress) {
                $violations += $relative
            }
        }
        $selected += $relative
    }
    if ($violations.Count) {
        throw ('Public source review failed in: ' + (($violations | Sort-Object -Unique) -join ', '))
    }
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $output) | Out-Null
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [IO.Compression.ZipFile]::Open($output, [IO.Compression.ZipArchiveMode]::Create)
    try {
        foreach ($relative in $selected) {
            if ($PublicBaseCommit -and $relative -eq 'docs/plans/ACTIVE_WORK.md') {
                $content = [IO.File]::ReadAllText((Join-Path $repoRoot $relative))
                $content = [regex]::Replace($content, '(?m)^base_commit: [^\r\n]+', "base_commit: $PublicBaseCommit")
                $entry = $archive.CreateEntry($relative)
                $writer = [IO.StreamWriter]::new($entry.Open(), [Text.UTF8Encoding]::new($false))
                try { $writer.Write($content) } finally { $writer.Dispose() }
                continue
            }
            [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, (Join-Path $repoRoot $relative), $relative) | Out-Null
        }
    } finally { $archive.Dispose() }
    [pscustomobject]@{ status = 'exported'; files = $selected.Count; sha256 = (Get-FileHash -LiteralPath $output).Hash } | ConvertTo-Json
} finally { Pop-Location }
