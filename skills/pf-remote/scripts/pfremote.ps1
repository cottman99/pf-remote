[CmdletBinding()]
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$PfRemoteArguments
)

$ErrorActionPreference = 'Stop'

$candidates = [System.Collections.Generic.List[string]]::new()
if (-not [string]::IsNullOrWhiteSpace($env:PFREMOTE_CLI)) {
    $candidates.Add($env:PFREMOTE_CLI)
}

$installRoot = Join-Path $env:LOCALAPPDATA 'Programs\PFRemote'
$statePath = Join-Path $installRoot 'current.json'
try {
    $state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
    if ($state.schema_version -eq 'pfremote.windows-installation-state/v1' -and
        $state.current -match '^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$') {
        $candidates.Add((Join-Path $installRoot ("versions\{0}\pfremote.exe" -f $state.current)))
    }
}
catch {
    # Keep trying the explicit development and legacy compatibility paths.
}

$installed = Join-Path $env:ProgramFiles 'PF Remote Center\app\pfremote.exe'
$candidates.Add($installed)

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot '..\..\..') -ErrorAction SilentlyContinue
if ($null -ne $repoRoot) {
    $candidates.Add((Join-Path $repoRoot '.codex_tmp\bin\pfremote.exe'))
    $candidates.Add((Join-Path $repoRoot 'apps\windows\PFRemoteCenter\bin\x64\Debug\net10.0-windows10.0.26100.0\win-x64\pfremote.exe'))
}

$pathCommand = Get-Command pfremote.exe -ErrorAction SilentlyContinue
if ($null -ne $pathCommand) {
    $candidates.Add($pathCommand.Source)
}

$cli = $candidates | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf } | Select-Object -First 1
if ([string]::IsNullOrWhiteSpace($cli)) {
    Write-Error 'PF Remote is not installed or its CLI cannot be found. Install or repair PF Remote Center, then retry.'
    exit 2
}

& $cli @PfRemoteArguments
exit $LASTEXITCODE
