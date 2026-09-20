[CmdletBinding()]
param(
 [Parameter(Mandatory=$true)][ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$')][string]$Version,
 [Parameter(Mandatory=$true)][string]$PublisherPublicKey
)
$ErrorActionPreference='Stop'
if([Convert]::FromBase64String($PublisherPublicKey).Length -ne 32){throw 'Invalid publisher key'}
$repo=Split-Path -Parent $PSScriptRoot
$output=Join-Path $repo ".codex_tmp/linux-release/$Version"
if(Test-Path $output){throw 'Release output already exists'}
New-Item -ItemType Directory -Path $output|Out-Null
$previousOS=$env:GOOS;$previousArch=$env:GOARCH;$previousCGO=$env:CGO_ENABLED
Push-Location $repo
try {
 $env:GOOS='linux';$env:GOARCH='amd64';$env:CGO_ENABLED='0'
 foreach($name in @('pfremote','pfremoted','pfremote-update')) {
  $flags="-s -w -X github.com/cottman99/pf-remote/internal/autoupdate.PublisherKey=$PublisherPublicKey -X main.releaseVersion=$Version"
  go build -trimpath -buildvcs=false -ldflags $flags -o (Join-Path $output "$name-linux-x64") "./cmd/$name"
  if($LASTEXITCODE -ne 0){throw 'Linux build failed'}
 }
 @{version=$Version}|ConvertTo-Json|Set-Content (Join-Path $output release-manifest.json)
 Write-Output "Linux release built: $Version"
} finally {
 $env:GOOS=$previousOS;$env:GOARCH=$previousArch;$env:CGO_ENABLED=$previousCGO
 Pop-Location
}
