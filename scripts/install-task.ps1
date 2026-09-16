# OpenDrop — install as a Windows logon task (equivalent of tmux on Mac).
# Run in PowerShell as your normal user (no admin needed for logon task):
#   powershell -ExecutionPolicy Bypass -File scripts\install-task.ps1
# Optional: -BinaryPath "C:\Tools\opendrop.exe" -Port 53317
param(
  [string]$BinaryPath = "$PSScriptRoot\..\opendrop.exe",
  [int]$Port = 53317
)
$ErrorActionPreference = "Stop"
$bin = [System.IO.Path]::GetFullPath($BinaryPath)
if (-not (Test-Path $bin)) {
  Write-Host "Binary not found at $bin"
  Write-Host "Build it first: go build -o opendrop.exe ./cmd/opendrop"
  exit 1
}
$taskName = "OpenDrop"
$action = New-ScheduledTaskAction -Execute $bin -Argument "daemon --port $Port" -WorkingDirectory (Split-Path $bin)
$trigger = New-ScheduledTaskTrigger -AtLogOn
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -StartWhenAvailable
try { Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue } catch {}
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings -Description "OpenDrop LAN file+clipboard daemon" | Out-Null
Start-ScheduledTask -TaskName $taskName
Write-Host "Installed + started task '$taskName' -> $bin daemon --port $Port"
Write-Host "Verify: Get-ScheduledTask OpenDrop | Select State ; .\opendrop.exe status"
Write-Host "Firewall: run once as admin:"
Write-Host "  New-NetFirewallRule -DisplayName OpenDrop -Direction Inbound -LocalPort 53317,53318 -Protocol TCP -Action Allow"
Write-Host "  New-NetFirewallRule -DisplayName 'OpenDrop UDP' -Direction Inbound -LocalPort 53318 -Protocol UDP -Action Allow"
