# OpenDrop — Windows Setup (for agent)

> You are on the Windows machine. The Mac side is already built and pushed here.
> Goal: build `opendrop.exe`, pair it with the Mac, verify file + clipboard + notifications, persist via Task Scheduler.

## 0. Confirm LAN

```powershell
ipconfig | Select-String "IPv4"
# Must be same subnet as Mac (e.g. 192.168.1.x). Same WiFi required.
```

## 1. Prereqs

```powershell
go version    # need Go 1.24+. If missing: winget install Go.Go
git --version # if missing: winget install Git.Git
```

## 2. Clone + build

```powershell
git clone https://github.com/ekk-clsrspk/OpenDrop.git
cd OpenDrop
go build -o opendrop.exe ./cmd/opendrop
.\opendrop.exe status
# First run creates %USERPROFILE%\.opendrop\config.yaml and prints a pairing token.
```

No Go on this box? Alternative: copy `opendrop.exe` built on the Mac (`GOOS=windows go build`) — then skip to step 3.

## 3. Pairing (MUST match Mac)

Ask the human for the Mac's token (`grep psk_token ~/.opendrop/config.yaml` on Mac), then:

```powershell
.\opendrop.exe pair --token <SAME_TOKEN_AS_MAC>
```

## 4. Firewall (run once as ADMIN)

```powershell
New-NetFirewallRule -DisplayName OpenDrop -Direction Inbound -LocalPort 53317 -Protocol TCP -Action Allow
New-NetFirewallRule -DisplayName "OpenDrop UDP" -Direction Inbound -LocalPort 53318 -Protocol UDP -Action Allow
```

No admin? At minimum allow the popup when `daemon` first listens.

## 5. Test (before persistence)

```powershell
# terminal 1: run daemon in foreground
.\opendrop.exe daemon

# terminal 2:
.\opendrop.exe peers
# expect: Mac device listed (name + 192.168.1.x:53317 + ok). If empty, add manual peer:
# edit %USERPROFILE%\.opendrop\config.yaml -> peers: ["<MAC_IP>:53317"], restart daemon.

echo "hello from windows" > test.txt
.\opendrop.exe send --to <MAC_NAME_OR_IP> test.txt
# expect: OK + toast on BOTH machines, file in Mac ~/Downloads/OpenDrop/

.\opendrop.exe clip push --text "hello clipboard from windows"
# expect: text appears in Mac clipboard (paste to verify)
# then copy text on Mac -> should auto-appear here (run Get-Clipboard to check)
```

Copy the exact output of `peers` + `send` back to the human if anything fails.

## 6. Persist (Task Scheduler = Windows equivalent of tmux)

```powershell
powershell -ExecutionPolicy Bypass -File scripts\install-task.ps1
Get-ScheduledTask OpenDrop | Select TaskName, State
.\opendrop.exe status   # Daemon: RUNNING
```

This creates logon task `OpenDrop` → `opendrop.exe daemon`, restart-on-failure, works on battery.

## 7. Reboot verify

1. Reboot.
2. `Get-ScheduledTask OpenDrop` → Ready/Running.
3. `.\opendrop.exe peers` → Mac visible.
4. Send a file Mac → Windows, confirm `%USERPROFILE%\Downloads\OpenDrop\` + toast.

## Troubleshoot

| symptom | fix |
|---|---|
| `peers` empty | same WiFi? firewall rules above? try manual `peers: ["<MAC_IP>:53317"]` in config, restart daemon |
| `send` 401 | PSK mismatch — re-run `pair --token` with Mac's token on BOTH, restart daemons |
| `send` connection refused | daemon not running on target (`status`), wrong IP/port, Windows firewall |
| clipboard loops/spam | `.\opendrop.exe clip pause` (password-safe), `resume` when done |
| toast missing | Windows Settings → Notifications → allow; daemon still logs to console |
| logs | daemon stdout (foreground) or Task Scheduler history; config at `%USERPROFILE%\.opendrop\config.yaml` |

Report back: `status` output, `peers` output, one `send` transcript, clipboard both-directions result.

## Optional: Web UI (drag & drop)

Same repo, same Prereqs. Loopback-only page for sending files without the terminal:

```powershell
go build -o opendrop-web.exe ./cmd/opendrop-web
.\opendrop-web.exe
# open http://127.0.0.1:8655 — pick the Mac, drag & drop files
```

The daemon must be running (it receives). No firewall change needed — the page never leaves this machine; files still travel over the existing PSK-protected :53317 path.
