$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../internal/executor/desktop/rdp_signer_managed.ps1')
$root = Join-Path (Split-Path $PSScriptRoot) ('.codex_tmp/rdp-signing-tests-' + $PID)
New-Item -ItemType Directory -Force $root | Out-Null
$rsa = [Security.Cryptography.RSA]::Create(2048)
$request = [Security.Cryptography.X509Certificates.CertificateRequest]::new('CN=Example RDP test', $rsa, [Security.Cryptography.HashAlgorithmName]::SHA256, [Security.Cryptography.RSASignaturePadding]::Pkcs1)
$certificate = $request.CreateSelfSigned([DateTimeOffset]::UtcNow.AddMinutes(-1), [DateTimeOffset]::UtcNow.AddDays(1))
try {
    $path = Join-Path $root 'example.rdp'
    $inputLines = @('full address:s:192.0.2.10:3389', 'authentication level:i:2', 'redirectclipboard:i:1', 'drivestoredirect:s:*')
    [IO.File]::WriteAllLines($path, $inputLines, [Text.UTF8Encoding]::new($false))
    Sign-PfRemoteRdpProfile -RdpPath $path -Certificate $certificate
    $lines = [IO.File]::ReadAllLines($path)
    $scope = 'signscope:s:Full Address,Alternate Full Address,Authentication Level,RedirectClipboard,DrivesToRedirect'
    if ($scope -notin $lines) { throw 'Unexpected RDP signature scope.' }
    $canonical = @('full address:s:192.0.2.10:3389', 'alternate full address:s:192.0.2.10:3389', 'authentication level:i:2', 'redirectclipboard:i:1', 'drivestoredirect:s:*', $scope, '')
    $content = [Text.Encoding]::Unicode.GetBytes(($canonical -join [Environment]::NewLine) + [char]0)
    $signature = @($lines | Where-Object { $_ -like 'signature:s:*' })[0].Substring(12)
    $blob = [Convert]::FromBase64String($signature)
    if ([BitConverter]::ToUInt32($blob, 0) -ne 0x00010001 -or [BitConverter]::ToUInt32($blob, 4) -ne 1 -or [BitConverter]::ToUInt32($blob, 8) -ne $blob.Length - 12) { throw 'Invalid native signature envelope.' }
    $cms = [Security.Cryptography.Pkcs.SignedCms]::new([Security.Cryptography.Pkcs.ContentInfo]::new($content), $true)
    $cms.Decode([byte[]]$blob[12..($blob.Length-1)])
    $cms.CheckSignature($true)
    if ($cms.SignerInfos[0].Certificate.Thumbprint -cne $certificate.Thumbprint) { throw 'Signer identity changed.' }
    $bad = [Text.Encoding]::Unicode.GetBytes((($canonical -join [Environment]::NewLine).Replace('192.0.2.10', '192.0.2.11')) + [char]0)
    $verify = [Security.Cryptography.Pkcs.SignedCms]::new([Security.Cryptography.Pkcs.ContentInfo]::new($bad), $true)
    $verify.Decode([byte[]]$blob[12..($blob.Length-1)])
    $rejected = $false
    try { $verify.CheckSignature($true) } catch { $rejected = $true }
    if (-not $rejected) { throw 'Modified RDP target was accepted.' }
    [IO.File]::WriteAllLines($path, @('full address:s:192.0.2.10', 'full address:s:192.0.2.11'), [Text.UTF8Encoding]::new($false))
    $rejected = $false
    try { Sign-PfRemoteRdpProfile -RdpPath $path -Certificate $certificate } catch { $rejected = $true }
    if (-not $rejected) { throw 'Duplicate RDP target was accepted.' }
    'Managed RDP signing passed: native envelope, pinned signer, tampering and duplicate rejection.'
} finally {
    $certificate.Dispose()
    $rsa.Dispose()
    $resolved = [IO.Path]::GetFullPath($root)
    $scopeRoot = [IO.Path]::GetFullPath((Join-Path (Split-Path $PSScriptRoot) '.codex_tmp')) + [IO.Path]::DirectorySeparatorChar
    if (-not $resolved.StartsWith($scopeRoot)) { throw 'Unexpected test cleanup path.' }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
