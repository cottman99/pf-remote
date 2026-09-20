[CmdletBinding()]
param(
    [switch]$WithCodex
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$scratchRoot = Join-Path $repoRoot '.codex_tmp'
$binRoot = Join-Path $scratchRoot 'bin'
$runRoot = Join-Path $scratchRoot ("agent-alignment-{0}" -f $PID)
$configRoot = Join-Path $runRoot 'config'
$endpoint = "\\.\pipe\pfremote-agent-alignment-$PID"
$skillCommand = Join-Path $repoRoot 'skills\pf-remote\scripts\pfremote.ps1'

New-Item -ItemType Directory -Force -Path $binRoot, $configRoot | Out-Null

$previousAppData = $env:APPDATA
$previousEndpoint = $env:PFREMOTE_LOCAL_ENDPOINT
$previousCli = $env:PFREMOTE_CLI
$previousPath = $env:PATH
$daemon = $null
$controlRecordPath = Join-Path $runRoot 'agent-control-record.json'

try {
    Push-Location $repoRoot
    try {
        go build -o (Join-Path $binRoot 'pfremote.exe') ./cmd/pfremote
        if ($LASTEXITCODE -ne 0) { throw 'PF Remote CLI build failed.' }
        go build -o (Join-Path $binRoot 'pfremoted.exe') ./cmd/pfremoted
        if ($LASTEXITCODE -ne 0) { throw 'PF Remote daemon build failed.' }
        go build -o (Join-Path $binRoot 'pfremote-mcp.exe') ./cmd/pfremote-mcp
        if ($LASTEXITCODE -ne 0) { throw 'PF Remote MCP build failed.' }
        go build -o (Join-Path $binRoot 'pfremote-alignment-fixture.exe') ./scripts/alignmentfixture
        if ($LASTEXITCODE -ne 0) { throw 'PF Remote Agent control fixture build failed.' }
    }
    finally {
        Pop-Location
    }

    $env:APPDATA = $configRoot
    $env:PFREMOTE_LOCAL_ENDPOINT = $endpoint
    $env:PFREMOTE_CLI = Join-Path $binRoot 'pfremote.exe'
    $env:PATH = $binRoot + [IO.Path]::PathSeparator + $previousPath
    # The production daemon intentionally discovers an enabled legacy catalog
    # outside APPDATA. Use the clean-room fixture for this deterministic check
    # so an installed private deployment cannot replace compute-node/shell.
    $daemonPath = Join-Path $binRoot 'pfremote-alignment-fixture.exe'
    $daemonStart = @{
        FilePath = $daemonPath
        PassThru = $true
        WindowStyle = 'Hidden'
        RedirectStandardOutput = (Join-Path $runRoot 'pfremoted.stdout.log')
        RedirectStandardError = (Join-Path $runRoot 'pfremoted.stderr.log')
    }
    $daemonStart.ArgumentList = @('--record', $controlRecordPath)
    $daemon = Start-Process @daemonStart

    $ready = $false
    for ($attempt = 0; $attempt -lt 50; $attempt++) {
        if (Test-Path -LiteralPath $endpoint) {
            $ready = $true
            break
        }
        if ($daemon.HasExited) { break }
        Start-Sleep -Milliseconds 100
    }
    if (-not $ready) { throw 'The isolated PF Remote daemon did not become ready.' }

    $contextJson = & $skillCommand context compute-node/shell --json
    if ($LASTEXITCODE -ne 0) { throw 'The Skill could not create target context.' }
    $context = $contextJson | ConvertFrom-Json
    if ($context.envelope -notmatch '(?m)^target: (?<target>pfremote://[^\r\n]+)$') {
        throw 'The target envelope does not contain a canonical PF Remote target.'
    }
    $handedTarget = $Matches.target

    $inspectJson = & $skillCommand inspect $handedTarget --json
    if ($LASTEXITCODE -ne 0) { throw 'The Skill could not inspect the handed target.' }
    $inspect = $inspectJson | ConvertFrom-Json
    if ($inspect.target.canonical -ne $handedTarget) {
        throw 'Agent target alignment failed: inspected target differs from handed target.'
    }
    if ($context.target.canonical -ne $inspect.target.canonical) {
        throw 'Center context and Agent inspection resolved different targets.'
    }

    $agentVerified = $false
    if ($WithCodex) {
        $codex = Get-Command codex -ErrorAction SilentlyContinue
        if ($null -eq $codex) { throw 'Codex is not installed for the Agent alignment check.' }

        $schemaPath = Join-Path $runRoot 'agent-result.schema.json'
        $resultPath = Join-Path $runRoot 'agent-result.json'
        @'
{
  "type": "object",
  "additionalProperties": false,
  "required": ["status", "target", "alias", "kind", "authorization_valid_until", "action_status", "action_output"],
  "properties": {
    "status": { "type": "string", "enum": ["verified"] },
    "target": { "type": "string" },
    "alias": { "type": "string" },
    "kind": { "type": "string" },
    "authorization_valid_until": { "type": "string" },
    "action_status": { "type": "string", "enum": ["completed"] },
    "action_output": { "type": "string" }
  }
}
'@ | Set-Content -LiteralPath $schemaPath -Encoding utf8

        $prompt = @"
Use the PF Remote Skill at $repoRoot\skills\pf-remote\SKILL.md and the
configured PF Remote MCP tools.
The user supplied this exact context envelope:

$($context.envelope)

Do not infer or select another target. First call the PF Remote MCP inspect
tool for the exact supplied target. Then call the PF Remote MCP exec tool for
that same canonical target with the exact argument vector
["fixture-task", "--exact-target"]. Do not use a shell command. Return the
verified target, alias, capability kind, exact live authorization valid-until
value, action status, and action output using the required JSON output schema.
The valid-until value is intentionally absent from this prompt; do not report
verified unless both MCP calls succeed.
"@
        $mcpCommand = Join-Path $binRoot 'pfremote-mcp.exe'
        $mcpCommandConfig = "mcp_servers.pf_remote.command='$mcpCommand'"
        $mcpEndpointConfig = "mcp_servers.pf_remote.env.PFREMOTE_LOCAL_ENDPOINT='$endpoint'"
        & $codex.Source exec --approve-for-me --skip-git-repo-check --ephemeral --ignore-user-config `
            --config $mcpCommandConfig --config $mcpEndpointConfig `
            --output-schema $schemaPath --output-last-message $resultPath `
            --cd $repoRoot $prompt | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Codex could not complete the isolated PF Remote Skill check.' }

        $agentResult = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json
        $agentValidUntil = [DateTimeOffset]::MinValue
        if (-not [DateTimeOffset]::TryParse($agentResult.authorization_valid_until, [ref]$agentValidUntil)) {
            throw 'Codex did not return a live PF Remote authorization deadline.'
        }
        $expectedValidUntil = [DateTimeOffset]$inspect.target.authorization.valid_until
        if ($agentResult.status -ne 'verified' -or $agentResult.target -ne $handedTarget -or
            $agentResult.alias -ne $inspect.target.alias -or $agentResult.kind -ne $inspect.target.capability.kind -or
            $agentResult.action_status -ne 'completed' -or $agentResult.action_output -ne "isolated target action completed`n" -or
            $agentValidUntil.ToUniversalTime() -ne $expectedValidUntil.ToUniversalTime()) {
            throw 'Codex did not align with the target handed off by PF Remote.'
        }
        if (-not (Test-Path -LiteralPath $controlRecordPath)) {
            throw 'The exact target action did not reach the isolated action runner.'
        }
        $controlRecord = Get-Content -LiteralPath $controlRecordPath -Raw | ConvertFrom-Json
        if ($controlRecord.schema_version -ne 'pfremote.agent-control-record/v1' -or
            $controlRecord.target -ne $handedTarget -or $controlRecord.command.Count -ne 2 -or
            $controlRecord.command[0] -ne 'fixture-task' -or $controlRecord.command[1] -ne '--exact-target') {
            throw 'Codex control reached a different target or changed the action arguments.'
        }
        $agentVerified = $true
    }

    [pscustomobject]@{
        schema_version = 'pfremote.agent-alignment-check/v1'
        status = 'passed'
        envelope_version = ($context.envelope -split "`n")[0]
        target_match = $true
        capability_kind = $inspect.target.capability.kind
        agent_verified = $agentVerified
        agent_control_verified = $agentVerified
    } | ConvertTo-Json
}
finally {
    if ($null -ne $daemon -and -not $daemon.HasExited) {
        Stop-Process -Id $daemon.Id -Force
        $daemon.WaitForExit()
    }
    $env:APPDATA = $previousAppData
    $env:PFREMOTE_LOCAL_ENDPOINT = $previousEndpoint
    $env:PFREMOTE_CLI = $previousCli
    $env:PATH = $previousPath
    if (Test-Path -LiteralPath $runRoot) {
        $resolvedRun = (Resolve-Path -LiteralPath $runRoot).Path
        $resolvedScratch = (Resolve-Path -LiteralPath $scratchRoot).Path
        if (-not $resolvedRun.StartsWith($resolvedScratch + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
            throw "Refusing to clean unexpected alignment path: $resolvedRun"
        }
        Remove-Item -LiteralPath $resolvedRun -Recurse -Force
    }
}
