$ErrorActionPreference = 'Stop'
$v1 = (Resolve-Path dist/v1.msi).Path
$v2 = (Resolve-Path dist/v2.msi).Path
$installDir = 'C:\Program Files\Obot\obot-sentry'
$exe = Join-Path $installDir 'obot-sentry.exe'
$launcher = Join-Path $installDir 'obot-sentryw.exe'
$deviceKey = "$env:ProgramData\obot\obot-sentry\device_key.pem"
$scanState = "$env:LOCALAPPDATA\obot\obot-sentry\scan.json"
$requestLog = Join-Path $PWD 'mock-requests.log'
$script:failures = @()

$mock = Start-Process python -ArgumentList '.github/e2e/mock_server.py' -PassThru -RedirectStandardError mock.err -RedirectStandardOutput mock.out
Start-Sleep -Seconds 3

function Check([string]$What, [bool]$Ok) {
    if ($Ok) { Write-Host "PASS: $What" } else { Write-Host "FAIL: $What"; $script:failures += $What }
}

function Install-Sentry([string]$Name, [string]$Msi, [string]$ServerURL) {
    $log = Join-Path $PWD "$Name.log"
    $argList = @('/i', "`"$Msi`"", '/qn', '/l*v', "`"$log`"", "SERVER_URL=`"$ServerURL`"", 'ENROLLMENT_KEY="ode1-1-1-e2e"', 'SCAN_INTERVAL_MINUTES="60"')
    $p = Start-Process msiexec.exe -Wait -PassThru -ArgumentList ($argList -join ' ')
    Write-Host "[$Name] msiexec exit code $($p.ExitCode)"
    Select-String -Path $log -Pattern 'CheckEnrollmentKey|RegisterEventSource' | Select-Object -First 8 | ForEach-Object { Write-Host "  log: $($_.Line)" }
    return $p.ExitCode
}

function Uninstall-Sentry([string]$Name, [string]$Msi) {
    $log = Join-Path $PWD "$Name-uninstall.log"
    $p = Start-Process msiexec.exe -Wait -PassThru -ArgumentList "/x `"$Msi`" /qn /l*v `"$log`""
    Write-Host "[$Name] uninstall exit code $($p.ExitCode)"
}

# The installer starts the scan task right away. Stop it from running again, and wait out the run the installer
# started, so the scans this script runs don't race it.
function Disable-SentryTasks {
    Get-ScheduledTask -TaskName 'Obot Sentry*' -ErrorAction SilentlyContinue | Disable-ScheduledTask | Out-Null
    $deadline = (Get-Date).AddSeconds(120)
    while ((Get-Date) -lt $deadline -and (Get-ScheduledTask -TaskName 'Obot Sentry*' -ErrorAction SilentlyContinue | Where-Object { $_.State -eq 'Running' })) {
        Start-Sleep -Seconds 1
    }
}

function Reset-State {
    Remove-Item -Recurse -Force "$env:ProgramData\obot\obot-sentry" -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force 'C:\Windows\System32\config\systemprofile\AppData\Local\obot' -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force "$env:LOCALAPPDATA\obot" -ErrorAction SilentlyContinue
    Remove-Item -Force $requestLog -ErrorAction SilentlyContinue
}

function Get-SentryEvents([datetime]$Since) {
    $events = Get-WinEvent -FilterHashtable @{ LogName = 'Application'; ProviderName = 'Obot Sentry'; StartTime = $Since } -ErrorAction SilentlyContinue
    foreach ($e in $events) { Write-Host "  event $($e.Id) [$($e.LevelDisplayName)]: $($e.Message)" }
    return @($events)
}

function Get-Requests {
    if (-not (Test-Path $requestLog)) { return @() }
    $lines = @(Get-Content $requestLog)
    foreach ($l in $lines) { Write-Host "  request: $l" }
    return $lines
}

function Installed { return (Test-Path $exe) }
function TasksRegistered { return [bool](Get-ScheduledTask -TaskName 'Obot Sentry Scan' -ErrorAction SilentlyContinue) }

# 1. Rejected key: the install fails, rolls back, and logs why, without enrolling anything.
Reset-State
$t = Get-Date
$code = Install-Sentry 's1-reject' $v1 'http://127.0.0.1:18080/reject'
Check 'rejected key fails the install with 1603' ($code -eq 1603)
Check 'rejected key leaves no binary' (-not (Installed))
Check 'rejected key leaves no scan task' (-not (TasksRegistered))
Check 'rejected key leaves no config' (-not (Test-Path 'HKLM:\SOFTWARE\Obot\obot-sentry'))
Check 'rejected key leaves no device key' (-not (Test-Path $deviceKey))
$ev = Get-SentryEvents $t
Check 'rejected key logs event 101 with a formatted message' ([bool]($ev | Where-Object { $_.Id -eq 101 -and $_.Message -match 'invalid, expired, or deleted' }))
Check 'MSI log records exit code 3 from CheckEnrollmentKey' ([bool](Select-String -Path 's1-reject.log' -Pattern 'CheckEnrollmentKey returned actual error code 3' -Quiet))
$req = Get-Requests
Check 'install only checks the key' ((@($req | Where-Object { $_ -eq 'GET /reject/api/mdm/enroll/check' }).Count -eq 1) -and -not ($req | Where-Object { $_ -like 'POST *' }))

# 2. Device limit: the key is fine, so the install succeeds; the scan's enrollment is refused and logged.
Reset-State
$t = Get-Date
$code = Install-Sentry 's2-limit' $v1 'http://127.0.0.1:18080/limit'
Disable-SentryTasks
Check 'device limit installs with 0' ($code -eq 0)
$ev = Get-SentryEvents $t
Check 'device limit install logs event 100' ([bool]($ev | Where-Object { $_.Id -eq 100 }))
& $exe scan --submit --force 2>&1 | ForEach-Object { Write-Host "  scan: $_" }
Check 'scan refused by the device limit exits 3' ($LASTEXITCODE -eq 3)
# Either this scan or the one the installer started reports it first, so look back to the install.
$ev = Get-SentryEvents $t
Check 'device limit scan logs event 110' ([bool]($ev | Where-Object { $_.Id -eq 110 -and $_.Message -match 'device limit' }))
Uninstall-Sentry 's2-limit' $v1

# 3. Unreachable server: the install succeeds and warns.
Reset-State
$t = Get-Date
$code = Install-Sentry 's3-unreachable' $v1 'http://127.0.0.1:1'
Check 'unreachable server installs with 0' ($code -eq 0)
Check 'unreachable server installs the binary' (Installed)
Check 'unreachable server registers the scan task' (TasksRegistered)
$ev = Get-SentryEvents $t
Check 'unreachable server logs warning event 102' ([bool]($ev | Where-Object { $_.Id -eq 102 }))
Uninstall-Sentry 's3-unreachable' $v1
Check 'uninstall removes the binary' (-not (Installed))
Check 'uninstall unregisters the event source' (-not [System.Diagnostics.EventLog]::SourceExists('Obot Sentry'))

# 4. Server error: the install succeeds and warns.
Reset-State
$t = Get-Date
$code = Install-Sentry 's4-down' $v1 'http://127.0.0.1:18080/down'
Check 'server error installs with 0' ($code -eq 0)
$ev = Get-SentryEvents $t
Check 'server error logs warning event 102' ([bool]($ev | Where-Object { $_.Id -eq 102 }))
Uninstall-Sentry 's4-down' $v1

# 5. A server without the key check answers a valid key with 403: the install succeeds and warns.
Reset-State
$t = Get-Date
$code = Install-Sentry 's5-old' $v1 'http://127.0.0.1:18080/old'
Check 'server without the check installs with 0' ($code -eq 0)
$ev = Get-SentryEvents $t
Check 'server without the check logs warning event 102' ([bool]($ev | Where-Object { $_.Id -eq 102 -and $_.Message -match '403' }))
Uninstall-Sentry 's5-old' $v1

# 6. Good key, then an upgrade whose command carries a rejected key.
Reset-State
$t = Get-Date
$code = Install-Sentry 's6-ok' $v1 'http://127.0.0.1:18080/ok'
Disable-SentryTasks
Check 'good key installs with 0' ($code -eq 0)
$ev = Get-SentryEvents $t
Check 'good key logs event 100' ([bool]($ev | Where-Object { $_.Id -eq 100 }))
# The scan task the installer starts enrolls as the signed-in user, so the installer's own footprint is SYSTEM's profile.
Check 'good key install writes no SYSTEM enrollment state' (-not (Test-Path 'C:\Windows\System32\config\systemprofile\AppData\Local\obot'))
Start-Sleep -Seconds 1
$t = Get-Date
$tUpgrade = $t
$code = Install-Sentry 's6-upgrade' $v2 'http://127.0.0.1:18080/reject'
Disable-SentryTasks
Check 'upgrade with a rejected key still succeeds' ($code -eq 0)
Check 'upgrade keeps the binary' (Installed)
Check 'upgrade keeps the scan task' (TasksRegistered)
Check 'upgrade keeps the event source' ([System.Diagnostics.EventLog]::SourceExists('Obot Sentry'))
$ev = Get-SentryEvents $t
Check 'upgrade runs no key check' (-not ($ev | Where-Object { $_.Id -in 100, 101, 102 }))
Check 'upgrade skips CheckEnrollmentKey in the MSI log' ([bool](Select-String -Path 's6-upgrade.log' -Pattern 'Skipping action: CheckEnrollmentKey \(condition is false\)' -Quiet))

# 7. Scans against the rejected key (config now points at /reject).
& $exe scan --submit --force 2>&1 | ForEach-Object { Write-Host "  scan: $_" }
Check 'scan with a rejected key exits 3' ($LASTEXITCODE -eq 3)
# Either this scan or the one the upgrade started reports it first, so look back to the upgrade.
$ev = Get-SentryEvents $tUpgrade
Check 'scan failure logs event 110' ([bool]($ev | Where-Object { $_.Id -eq 110 -and $_.Message -match 'invalid, expired, or deleted' }))
Start-Sleep -Seconds 1
$t = Get-Date
$p = Start-Process $launcher -ArgumentList 'scan --submit --force' -Wait -PassThru
Check 'launcher propagates exit code 3' ($p.ExitCode -eq 3)
$ev = Get-SentryEvents $t
Check 'repeated identical scan failure is not logged again' (-not ($ev | Where-Object { $_.Id -eq 110 }))

# 7b. A failure recorded by a version without event logging is still reported after the upgrade.
$state = Get-Content $scanState -Raw | ConvertFrom-Json
Check 'scan state records the reported failure' ([bool]$state.reportedError)
$state.PSObject.Properties.Remove('reportedError')
$state | ConvertTo-Json | Set-Content $scanState
Start-Sleep -Seconds 1
$t = Get-Date
& $exe scan --submit --force 2>&1 | ForEach-Object { Write-Host "  scan: $_" }
$ev = Get-SentryEvents $t
Check 'failure recorded before event logging is reported' ([bool]($ev | Where-Object { $_.Id -eq 110 }))

# 8. Scans recover once the server accepts the device.
Set-ItemProperty -Path 'HKLM:\SOFTWARE\Obot\obot-sentry' -Name ServerURL -Value 'http://127.0.0.1:18080/ok'
Start-Sleep -Seconds 1
$t = Get-Date
& $exe scan --submit --force 2>&1 | ForEach-Object { Write-Host "  scan: $_" }
Check 'scan against a good server exits 0' ($LASTEXITCODE -eq 0)
$ev = Get-SentryEvents $t
Check 'scan recovery logs event 111' ([bool]($ev | Where-Object { $_.Id -eq 111 }))
Uninstall-Sentry 's6-upgrade' $v2

Stop-Process -Id $mock.Id -ErrorAction SilentlyContinue
Get-Content mock.err -ErrorAction SilentlyContinue | Select-Object -Last 40 | ForEach-Object { Write-Host "  $_" }

if ($script:failures.Count -gt 0) {
    Write-Host "`n$($script:failures.Count) check(s) failed:"
    $script:failures | ForEach-Object { Write-Host "  - $_" }
    exit 1
}
Write-Host "`nAll checks passed."
