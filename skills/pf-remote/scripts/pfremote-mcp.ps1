[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$installRoot = Join-Path $env:LOCALAPPDATA 'Programs\PFRemote'
$statePath = Join-Path $installRoot 'current.json'

try {
    $state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
    if ($state.schema_version -ne 'pfremote.windows-installation-state/v1' -or
        [string]::IsNullOrWhiteSpace($state.current) -or
        $state.current -notmatch '^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$') {
        throw 'invalid state'
    }
    $server = Join-Path $installRoot ("versions\{0}\pfremote-mcp.exe" -f $state.current)
    if (-not (Test-Path -LiteralPath $server -PathType Leaf)) {
        throw 'server missing'
    }
}
catch {
    [Console]::Error.WriteLine('PF Remote is not ready. Open PF Remote once, or repair the installation, then reopen Codex.')
    exit 2
}

& $server
exit $LASTEXITCODE
