# OpenDrop — LAN file + clipboard sharing (macOS ↔ Windows)

Single Go binary. No cloud, no accounts. Same WiFi only.

## Quick start (Mac)

```bash
make build
./opendrop-darwin-arm64 status        # creates ~/.opendrop/config.yaml + prints pairing token
./scripts/opendrop-tmux.sh start      # runs daemon in tmux session `opendrop`
./opendrop-darwin-arm64 peers         # find your Windows box
./opendrop-darwin-arm64 send ./photo.jpg --to <peer-name-or-ip>
```

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

## Windows

See **[WINDOWS.md](WINDOWS.md)** — copy-paste handoff for the Windows agent (build, firewall, Task Scheduler, verify).
