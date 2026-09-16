# OpenDrop — LAN file + clipboard sharing (macOS ↔ Windows)

Single Go binary. No cloud, no accounts. Same WiFi only.

## Quick start (Mac)

```bash
make build
./opendrop-darwin-arm64 status        # creates ~/.opendrop/config.yaml + prints pairing token
./scripts/install-mac.sh              # persistent LaunchAgent: runs at login, restarts on crash
./opendrop-darwin-arm64 peers         # find your Windows box
./opendrop-darwin-arm64 send ./photo.jpg --to <peer-name-or-ip>
```

`scripts/opendrop-tmux.sh` remains as a manual/debug runner (`start`/`stop`/`status`); stop it before using the LaunchAgent so they don't fight over :53317.

Copy text on one machine → auto-appears on the other. `opendrop clip pause` before copying passwords.

## Pairing (do once)

Both devices must share the same secret:

```bash
# on Mac, show token:
grep psk_token ~/.opendrop/config.yaml
# on the OTHER device:
opendrop pair --token <SAME_TOKEN>
# restart both daemons
```

## Commands

| cmd | what |
|---|---|
| `opendrop daemon [--port 53317]` | background service (tmux / Task Scheduler runs this) |
| `opendrop send <file> --to <peer>` | send file (`--to` = name, id-prefix, or `host:port`) |
| `opendrop peers` | LAN discovery (UDP beacon :53318 + manual `peers:` in config) |
| `opendrop status` | config + daemon running? |
| `opendrop clip push [--text ...]` | manual clipboard push |
| `opendrop clip pause / resume` | kill-switch for passwords |
| `opendrop pair --token X` | set shared secret |

Files land in `~/Downloads/OpenDrop/` (Windows: `%USERPROFILE%\Downloads\OpenDrop\`) with `(1)` dedup + toast notification on both ends.

## Config (`~/.opendrop/config.yaml`)

```yaml
device_name: MacBook-Air
port: 53317
psk_token: <secret — same on both>
download_dir: ~/Downloads/OpenDrop
clipboard_enabled: true
peers: ["192.168.1.50:53317"]   # fallback when discovery blocked
```

Ports: `53317/TCP` (files+clipboard API), `53318/UDP` (discovery beacons).

## Web UI (drag & drop)

Separate service for sending files without the terminal. Loopback-only (no new LAN exposure) — it forwards through the daemon's PSK API.

```bash
make build-web
./opendrop-web            # http://127.0.0.1:8655 (or http://opendrop.local:8655, see below)
```

Pick a peer, drag & drop 1..N files, per-file progress, plus an inbox view of received files. The daemon must be running (it does the actual receiving).

Optional pretty URL on Mac (one-time, needs admin):

```bash
echo "127.0.0.1 opendrop.local" | sudo tee -a /etc/hosts
dscacheutil -flushcache; sudo killall -HUP mDNSResponder
# then open http://opendrop.local:8655
```

Note: macOS reserves `.local` for Bonjour — if the name ever fails to resolve, `127.0.0.1:8655` always works.

## Windows

See **[WINDOWS.md](WINDOWS.md)** — copy-paste handoff for the Windows agent (build, firewall, Task Scheduler, verify).
