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
    # Windows validates the publisher chain before applying the exact RDP
    # publisher allow-list. TrustedPublisher alone cannot anchor a self-signed
    # leaf. Trust only this local, non-CA, code-signing-only certificate.
    $eku = @($cert.Extensions | Where-Object { $_.Oid.Value -eq '2.5.29.37' } | ForEach-Object { $_.EnhancedKeyUsages } | ForEach-Object { $_.Value })
    $constraints = @($cert.Extensions | Where-Object { $_.Oid.Value -eq '2.5.29.19' })
    if ($eku.Count -ne 1 -or $eku[0] -ne '1.3.6.1.5.5.7.3.3' -or
        ($constraints.Count -gt 0 -and $constraints[0].CertificateAuthority)) {
        throw 'Local RDP certificate has an unexpected purpose.'
    }
    foreach ($name in @('TrustedPublisher')) {
        $store = [Security.Cryptography.X509Certificates.X509Store]::new($name, 'CurrentUser')
        try {
            $store.Open('ReadWrite')
            $public = [Security.Cryptography.X509Certificates.X509Certificate2]::new($cert.RawData)
            $store.Add($public)
        } finally { $store.Close() }
    }
    $chain = [Security.Cryptography.X509Certificates.X509Chain]::new()
    $chain.ChainPolicy.RevocationMode = 'NoCheck'
    [void]$chain.ChainPolicy.ApplicationPolicy.Add([Security.Cryptography.Oid]::new('1.3.6.1.5.5.7.3.3'))
    try {
        if (-not $chain.Build($cert)) {
            $principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
            if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Local RDP publisher requires one-time administrator setup.' }
            # Machine-store installation is noninteractive. CurrentUser Root
            # would show a Windows consent window on the active desktop.
            $roots = [Security.Cryptography.X509Certificates.X509Store]::new('Root', 'LocalMachine')
            try { $roots.Open('ReadWrite'); $roots.Add([Security.Cryptography.X509Certificates.X509Certificate2]::new($cert.RawData)) } finally { $roots.Close() }
            if (-not $chain.Build($cert)) { throw 'Local RDP publisher trust failed validation.' }
        }
    } finally { $chain.Dispose() }
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $sha256 = ([BitConverter]::ToString($sha.ComputeHash($cert.RawData))).Replace('-', '') } finally { $sha.Dispose() }
    $policy = 'HKCU:\SOFTWARE\Policies\Microsoft\Windows NT\Terminal Services'
    $existing = (Get-ItemProperty -Path $policy -ErrorAction SilentlyContinue).TrustedCertThumbprints
    $thumbprints = @($existing -split ',' | Where-Object { $_ })
    $changed = $false
    foreach ($hash in @($cert.Thumbprint, ('sha256:' + $sha256))) {
        if ($hash -notin $thumbprints) { $thumbprints += $hash; $changed = $true }
    }
    if ($changed) {
        if (-not (Test-Path $policy)) { New-Item -Path $policy -Force | Out-Null }
        New-ItemProperty -Path $policy -Name 'TrustedCertThumbprints' -Value ($thumbprints -join ',') -PropertyType String -Force | Out-Null
    }
    $tool = Join-Path $env:WINDIR 'System32\rdpsign.exe'
    if (Test-Path -LiteralPath $tool) {
        & $tool /sha256 $cert.Thumbprint /q $rdpFile | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Native RDP signing failed.' }
    } else {
        Sign-PfRemoteRdpProfile -RdpPath $rdpFile -Certificate $cert
    }
    if (-not (Select-String -LiteralPath $rdpFile -Pattern '^signature:s:' -Quiet)) { throw 'RDP signature is missing.' }
} finally {
    if ($locked) { $mutex.ReleaseMutex() }
    $mutex.Dispose()
}
