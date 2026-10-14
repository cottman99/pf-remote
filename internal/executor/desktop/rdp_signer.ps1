$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$identity = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$mutex = [Threading.Mutex]::new($false, ('Local\PFRemoteRdpSigner-' + $identity))
$locked = $false
try {
    try { $locked = $mutex.WaitOne(20000) } catch [Threading.AbandonedMutexException] { $locked = $true }
    if (-not $locked) { throw 'Local RDP signing is busy.' }
    $state = Join-Path $env:APPDATA 'PF Remote\state'
    New-Item -ItemType Directory -Force $state | Out-Null
    $bindingPath = Join-Path $state 'rdp-signer-v1.json'
    $subject = 'CN=PF Remote local RDP profiles'
    $cert = $null
    if (Test-Path -LiteralPath $bindingPath) {
        $binding = Get-Content -LiteralPath $bindingPath -Raw | ConvertFrom-Json
        if ($binding.schema_version -ne 'pfremote.rdp-signer/v1' -or $binding.thumbprint -notmatch '^[A-Fa-f0-9]{40}$') {
            throw 'Local RDP signing identity is invalid.'
        }
        $cert = Get-Item -LiteralPath ('Cert:\CurrentUser\My\' + $binding.thumbprint) -ErrorAction SilentlyContinue
        if ($null -eq $cert -or -not $cert.HasPrivateKey -or $cert.Subject -cne $subject) {
            throw 'Local RDP signing identity needs repair.'
        }
        if ($cert.NotAfter -le (Get-Date).AddDays(7)) { $cert = $null }
    }
    if ($null -eq $cert) {
        $cert = New-SelfSignedCertificate -Subject $subject -FriendlyName 'PF Remote local RDP profiles' -Type CodeSigningCert -KeyAlgorithm RSA -KeyLength 2048 -HashAlgorithm SHA256 -KeyExportPolicy NonExportable -CertStoreLocation 'Cert:\CurrentUser\My' -NotAfter (Get-Date).AddYears(2)
        $binding = @{ schema_version = 'pfremote.rdp-signer/v1'; thumbprint = $cert.Thumbprint }
        $temp = Join-Path $state ('rdp-signer-' + [Guid]::NewGuid().ToString('N') + '.tmp')
        try {
            [IO.File]::WriteAllText($temp, ($binding | ConvertTo-Json -Compress), [Text.UTF8Encoding]::new($false))
            Move-Item -LiteralPath $temp -Destination $bindingPath -Force
        } finally { if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp } }
    }
    foreach ($name in @('TrustedPublisher')) {
        $store = [Security.Cryptography.X509Certificates.X509Store]::new($name, 'CurrentUser')
        try {
            $store.Open('ReadWrite')
            $public = [Security.Cryptography.X509Certificates.X509Certificate2]::new($cert.RawData)
            $store.Add($public)
        } finally { $store.Close() }
    }
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $sha256 = ([BitConverter]::ToString($sha.ComputeHash($cert.RawData))).Replace('-', '') } finally { $sha.Dispose() }
    $policy = 'HKCU:\SOFTWARE\Policies\Microsoft\Windows NT\Terminal Services'
    $existing = (Get-ItemProperty -Path $policy -ErrorAction SilentlyContinue).TrustedCertThumbprints
    $thumbprints = @($existing -split ',' | Where-Object { $_ })
    $changed = $false
    foreach ($hash in @($cert.Thumbprint, $sha256)) {
        if ($hash -notin $thumbprints) { $thumbprints += $hash; $changed = $true }
    }
    if ($changed) {
        if (-not (Test-Path $policy)) { New-Item -Path $policy -Force | Out-Null }
        New-ItemProperty -Path $policy -Name 'TrustedCertThumbprints' -Value ($thumbprints -join ',') -PropertyType String -Force | Out-Null
    }
    $tool = Join-Path $env:WINDIR 'System32\rdpsign.exe'
    & $tool /sha256 $cert.Thumbprint /q $rdpFile | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Native RDP signing failed.' }
    if (-not (Select-String -LiteralPath $rdpFile -Pattern '^signature:s:' -Quiet)) { throw 'RDP signature is missing.' }
} finally {
    if ($locked) { $mutex.ReleaseMutex() }
    $mutex.Dispose()
}
