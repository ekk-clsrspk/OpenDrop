#!/bin/bash
# OpenDrop — install macOS LaunchAgent (primary runner; replaces tmux).
# Usage: ./scripts/install-mac.sh [--binary ./opendrop-darwin-arm64] [--unload]
set -euo pipefail
cd "$(dirname "$0")/.."

BIN="opendrop-darwin-arm64"
UNLOAD_ONLY=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BIN="$2"; shift 2 ;;
    --unload) UNLOAD_ONLY=1; shift ;;
    *) echo "unknown arg: $1"; exit 2 ;;
  esac
done

LABEL="com.opendrop.daemon"
AGENT_DIR="$HOME/Library/LaunchAgents"
PLIST="$AGENT_DIR/$LABEL.plist"

if launchctl list "$LABEL" >/dev/null 2>&1; then
  echo "unloading existing $LABEL …"
  launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || launchctl unload "$PLIST" 2>/dev/null || true
fi
if [[ "$UNLOAD_ONLY" == "1" ]]; then echo "unloaded."; exit 0; fi

if [[ ! -x "$BIN" ]]; then
  echo "building $BIN …"
  go build -o "$BIN" ./cmd/opendrop
fi
ABS_BIN="$(cd "$(dirname "$BIN")" && pwd)/$(basename "$BIN")"
WORKDIR="$(pwd)"
mkdir -p "$AGENT_DIR" "$HOME/.opendrop"

sed -e "s#__OPENDROP_BIN__#$ABS_BIN#" \
    -e "s#__OPENDROP_WORKDIR__#$WORKDIR#" \
    -e "s#__HOME__#$HOME#" \
    scripts/com.opendrop.daemon.plist.template > "$PLIST"
chmod 644 "$PLIST"
echo "wrote $PLIST"

# Stop the legacy tmux runner if it's holding :53317
if tmux has-session -t opendrop 2>/dev/null; then
  echo "stopping legacy tmux session 'opendrop' (launchd takes over; script kept for debugging)…"
  tmux kill-session -t opendrop 2>/dev/null || true
  sleep 1
fi

launchctl bootstrap "gui/$(id -u)" "$PLIST" 2>&1 || launchctl load "$PLIST" 2>&1 || true
sleep 2
if launchctl list "$LABEL" 2>/dev/null | head -5; then
  echo "---"
  ./$BIN status || "$ABS_BIN" status || true
fi
echo "Logs: tail -f ~/.opendrop/daemon.out.log ~/.opendrop/daemon.err.log"
echo "Stop: ./scripts/install-mac.sh --unload"
