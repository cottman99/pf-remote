[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$trackedCandidates = Get-ChildItem -LiteralPath $repoRoot -Recurse -File | Where-Object {
    $_.FullName -notmatch '[\\/](\.git|\.codex_tmp|artifacts|bin|obj)[\\/]' -and
    $_.Length -lt 2MB
}

$forbiddenExtensions = @('.key', '.pfx', '.p12', '.pem', '.sqlite', '.sqlite3', '.db', '.db-wal', '.db-shm', '.sqlite-wal', '.sqlite-shm', '.sqlite3-wal', '.sqlite3-shm', '.bak')
$badFiles = $trackedCandidates | Where-Object { $forbiddenExtensions -contains $_.Extension.ToLowerInvariant() }
if ($badFiles) {
    $badFiles | ForEach-Object { Write-Error "Forbidden private-data file: $($_.FullName)" }
}

$privateKeyMarker = 'BEGIN ' + 'PRIVATE KEY'
$credentialAssignments = '(?i)(password|passwd|token|secret|api[_-]?key)\s*[:=]\s*["''][^"'']{8,}'
$violations = @()
foreach ($file in $trackedCandidates) {
    $content = Get-Content -LiteralPath $file.FullName -Raw -ErrorAction SilentlyContinue
    if ($null -eq $content) { continue }
    if ($content.Contains($privateKeyMarker) -or $content -match $credentialAssignments) {
        $violations += $file.FullName
    }
}
if ($violations) {
    $violations | Sort-Object -Unique | ForEach-Object { Write-Error "Potential embedded secret: $_" }
}

Write-Host 'Private-data hygiene check passed.'
