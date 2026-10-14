function Sign-PfRemoteRdpProfile {
    param([string]$RdpPath, [Security.Cryptography.X509Certificates.X509Certificate2]$Certificate)
    Add-Type -AssemblyName System.Security
    $fields = [ordered]@{
        'full address' = 'Full Address'
        'alternate full address' = 'Alternate Full Address'
        'prompt for credentials' = 'Prompt For Credentials'
        'authentication level' = 'Authentication Level'
        'audiomode' = 'AudioMode'
        'redirectprinters' = 'RedirectPrinters'
        'redirectcomports' = 'RedirectCOMPorts'
        'redirectsmartcards' = 'RedirectSmartCards'
        'redirectposdevices' = 'RedirectPOSDevices'
        'redirectclipboard' = 'RedirectClipboard'
        'devicestoredirect' = 'DevicesToRedirect'
        'drivestoredirect' = 'DrivesToRedirect'
        'redirectwebauthn' = 'RedirectWebAuthn'
    }
    $settings = [Collections.Generic.Dictionary[string,string]]::new([StringComparer]::OrdinalIgnoreCase)
    $lines = [Collections.Generic.List[string]]::new()
    foreach ($line in [IO.File]::ReadAllLines($RdpPath)) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line.Split([char[]]@(':'), 3)
        if ($parts.Length -ne 3) { throw 'RDP profile has an invalid setting.' }
        if ($parts[0] -in @('signscope', 'signature')) { continue }
        if ($settings.ContainsKey($parts[0])) { throw 'RDP profile has duplicate settings.' }
        $settings.Add($parts[0], $line)
        $lines.Add($line)
    }
    if (-not $settings.ContainsKey('full address')) { throw 'RDP profile has no target.' }
    if (-not $settings.ContainsKey('alternate full address')) {
        $alternate = 'alternate full address:' + $settings['full address'].Substring('full address:'.Length)
        $settings.Add('alternate full address', $alternate)
        $lines.Add($alternate)
    }
    $secure = [Collections.Generic.List[string]]::new()
    $scope = [Collections.Generic.List[string]]::new()
    foreach ($key in $fields.Keys) {
        if ($settings.ContainsKey($key)) { $secure.Add($settings[$key]); $scope.Add($fields[$key]) }
    }
    $scopeLine = 'signscope:s:' + ($scope -join ',')
    $content = (($secure.ToArray() + @($scopeLine, '')) -join [Environment]::NewLine) + [char]0
    $payload = [Text.Encoding]::Unicode.GetBytes($content)
    $cms = [Security.Cryptography.Pkcs.SignedCms]::new([Security.Cryptography.Pkcs.ContentInfo]::new($payload), $true)
    $signer = [Security.Cryptography.Pkcs.CmsSigner]::new($Certificate)
    $signer.DigestAlgorithm = [Security.Cryptography.Oid]::new('2.16.840.1.101.3.4.2.1')
    $signer.IncludeOption = [Security.Cryptography.X509Certificates.X509IncludeOption]::EndCertOnly
    $cms.ComputeSignature($signer, $true)
    $encoded = $cms.Encode()
    $blob = [byte[]]::new(12 + $encoded.Length)
    [Array]::Copy([BitConverter]::GetBytes([uint32]0x00010001), 0, $blob, 0, 4)
    [Array]::Copy([BitConverter]::GetBytes([uint32]1), 0, $blob, 4, 4)
    [Array]::Copy([BitConverter]::GetBytes([uint32]$encoded.Length), 0, $blob, 8, 4)
    [Array]::Copy($encoded, 0, $blob, 12, $encoded.Length)
    $verify = [Security.Cryptography.Pkcs.SignedCms]::new([Security.Cryptography.Pkcs.ContentInfo]::new($payload), $true)
    $verify.Decode($encoded)
    $verify.CheckSignature($true)
    if ($verify.SignerInfos[0].Certificate.Thumbprint -cne $Certificate.Thumbprint) { throw 'RDP signer identity mismatch.' }
    $lines.Add($scopeLine)
    $lines.Add('signature:s:' + [Convert]::ToBase64String($blob))
    [IO.File]::WriteAllLines($RdpPath, $lines.ToArray(), [Text.Encoding]::Unicode)
}
