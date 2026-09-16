#!/bin/bash
# OpenDrop — install macOS LaunchAgents (daemon :53317 + web UI :8655).
# These are the primary runners; they start at login and restart on crash.
# Usage: ./scripts/install-mac.sh [--unload] [--daemon-only | --web-only]
set -euo pipefail
cd "$(dirname "$0")/.."

UNLOAD_ONLY=0
ONLY=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --unload) UNLOAD_ONLY=1; shift ;;
    --daemon-only) ONLY="daemon"; shift ;;
    --web-only) ONLY="web"; shift ;;
    *) echo "unknown arg: $1"; exit 2 ;;
  esac
done

AGENT_DIR="$HOME/Library/LaunchAgents"
mkdir -p "$AGENT_DIR" "$HOME/.opendrop"

unload() {
  local label="$1"
  if launchctl list "$label" >/dev/null 2>&1; then
    echo "unloading $label …"
    launchctl bootout "gui/$(id -u)/$label" 2>/dev/null \
      || launchctl unload "$AGENT_DIR/$label.plist" 2>/dev/null || true
  fi
}

install_agent() {
  local label="$1" template="$2" bin="$3" build_pkg="$4"
  local plist="$AGENT_DIR/$label.plist"
  unload "$label"
  if [[ "$UNLOAD_ONLY" == "1" ]]; then return 0; fi
  if [[ ! -x "$bin" ]]; then
    echo "building $bin …"
    go build -o "$bin" "$build_pkg"
  fi
  local abs_bin="$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")"
  sed -e "s#__OPENDROP_BIN__#$abs_bin#" \
      -e "s#__OPENDROP_WORKDIR__#$(pwd)#" \
      -e "s#__HOME__#$HOME#" \
      "scripts/$template" > "$plist"
  chmod 644 "$plist"
  echo "wrote $plist"
  launchctl bootstrap "gui/$(id -u)" "$plist" 2>&1 || launchctl load "$plist" 2>&1 || true
}

if [[ "$ONLY" != "web" ]]; then
  install_agent "com.opendrop.daemon" "com.opendrop.daemon.plist.template" \
    "opendrop-darwin-arm64" "./cmd/opendrop"
fi
if [[ "$ONLY" != "daemon" ]]; then
  install_agent "com.opendrop.web" "com.opendrop.web.plist.template" \
    "opendrop-web" "./cmd/opendrop-web"
fi

if [[ "$UNLOAD_ONLY" == "1" ]]; then echo "unloaded."; exit 0; fi

# Stop the legacy tmux runner if it's holding :53317
if tmux has-session -t opendrop 2>/dev/null; then
  echo "stopping legacy tmux session 'opendrop' (launchd takes over; script kept for debugging)…"
  tmux kill-session -t opendrop 2>/dev/null || true
  sleep 1
fi

sleep 2
echo "--- daemon:"; ./opendrop-darwin-arm64 status 2>/dev/null || true
echo "--- web:"; curl -s --max-time 3 http://127.0.0.1:8655/api/info 2>&1 | head -c 200; echo
echo "Logs: tail -f ~/.opendrop/{daemon,web}.{out,err}.log"
echo "Stop: ./scripts/install-mac.sh --unload"
