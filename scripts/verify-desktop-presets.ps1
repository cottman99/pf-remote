param([switch]$KeepRunning)
$ErrorActionPreference='Stop'
$repo=Split-Path $PSScriptRoot
$run=Join-Path $repo ('.codex_tmp/preset-acceptance-'+$PID)
$bin=Join-Path $repo '.codex_tmp/bin'
New-Item -ItemType Directory -Force $run | Out-Null
$oldApp=$env:APPDATA;$oldPipe=$env:PFREMOTE_LOCAL_ENDPOINT;$oldCatalog=$env:PFREMOTE_LEGACY_CENTER_CATALOG
$daemon=$null
try {
    $env:APPDATA=Join-Path $run 'config'
    $env:PFREMOTE_LOCAL_ENDPOINT='\\.\pipe\pfremote-presets-'+$PID
    $env:PFREMOTE_LEGACY_CENTER_CATALOG=Join-Path $repo 'fixtures/migration/legacy-center-catalog-v1.synthetic.json'
    function Start-TestDaemon {
        $script:daemon=Start-Process (Join-Path $bin 'pfremoted.exe') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $run 'daemon.out') -RedirectStandardError (Join-Path $run 'daemon.err')
        for($i=0;$i -lt 50;$i++){if(Test-Path $env:PFREMOTE_LOCAL_ENDPOINT){return};Start-Sleep -Milliseconds 100}
        throw 'Preset test daemon did not start.'
    }
    function Save-TestPreset([bool]$clear) {
        $u=@{username='example-user';password=$(if($clear){''}else{'example-'+ 'secret'});clear_password=$clear;full_screen=$false;default_desktop=$true}|ConvertTo-Json -Compress
        $result=$u | & (Join-Path $bin 'pfremote.exe') open $target --save-preset-stdin --json
        if($LASTEXITCODE -ne 0){throw 'Preset save failed.'}
        if($result -match ('example-'+ 'secret')){throw 'Secret echoed in preset response.'}
        return ($result|ConvertFrom-Json)
    }
    Start-TestDaemon
    & (Join-Path $bin 'pfremote-migrate.exe') enable-legacy-center | Out-Null
    $catalog=& (Join-Path $bin 'pfremote.exe') list --json|ConvertFrom-Json
    $target=($catalog.targets|Where-Object {$_.capability.desktop_profile.protocol -eq 'rdp'}|Select-Object -First 1).canonical
    if(!$target){throw 'Synthetic RDP target missing.'}
    $saved=Save-TestPreset $false
    if(!$saved.password_saved -or !$saved.default_desktop -or $saved.full_screen){throw 'Preset save result mismatch.'}
    Stop-Process -Id $daemon.Id
    $daemon.WaitForExit()
    Start-TestDaemon
    $loaded=& (Join-Path $bin 'pfremote.exe') inspect $target --desktop-preset --json|ConvertFrom-Json
    if(!$loaded.password_saved -or $loaded.username -ne 'example-user' -or $loaded.full_screen){throw 'Preset did not survive daemon restart.'}
    foreach($mode in @('window','single','all','selected')) {
        $display=@{mode=$mode}
        if($mode -eq 'selected') {
            if(!$loaded.displays -or !$loaded.displays.monitors.Count){throw 'Native local screen discovery failed.'}
            $display.monitor_ids=@($loaded.displays.monitors[0].id)
            $display.layout_id=$loaded.displays.id
        }
        $update=@{username='example-user';full_screen=($mode -ne 'window');default_desktop=$true;display=$display}|ConvertTo-Json -Depth 5 -Compress
        $result=$update | & (Join-Path $bin 'pfremote.exe') open $target --save-preset-stdin --json | ConvertFrom-Json
        if($LASTEXITCODE -ne 0 -or $result.display.mode -ne $mode -or !$result.password_saved){throw 'Display mode did not retain account.'}
    }
    Stop-Process -Id $daemon.Id
    $daemon.WaitForExit()
    Start-TestDaemon
    $loaded=& (Join-Path $bin 'pfremote.exe') inspect $target --desktop-preset --json|ConvertFrom-Json
    if($loaded.display.mode -ne 'selected' -or !$loaded.password_saved){throw 'Selected display preset did not survive restart.'}
    $catalog=& (Join-Path $bin 'pfremote.exe') list --json|ConvertFrom-Json
    if(!($catalog.targets|Where-Object canonical -eq $target).local_default_desktop){throw 'Default desktop is missing from UI catalog.'}
    $cleared=Save-TestPreset $true
    if($cleared.password_saved){throw 'Password was not cleared.'}
    if($KeepRunning){@{appdata=$env:APPDATA;endpoint=$env:PFREMOTE_LOCAL_ENDPOINT;pid=$daemon.Id;target=$target}|ConvertTo-Json|Set-Content (Join-Path $repo '.codex_tmp/preset-acceptance.json') -Encoding utf8}
    'Desktop preset integration passed: protected IPC, four display modes, native monitor IDs, restart, account retention and clear.'
} finally {
    if(!$KeepRunning -and $daemon -and !$daemon.HasExited){Stop-Process -Id $daemon.Id}
    $env:APPDATA=$oldApp;$env:PFREMOTE_LOCAL_ENDPOINT=$oldPipe;$env:PFREMOTE_LEGACY_CENTER_CATALOG=$oldCatalog
}
