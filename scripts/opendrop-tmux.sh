#!/bin/bash
# OpenDrop — macOS tmux runner.
# Usage: ./scripts/opendrop-tmux.sh [start|stop|logs|status]
set -euo pipefail
SESSION="opendrop"
BIN="${OPENDROP_BIN:-./opendrop-darwin-arm64}"
if [[ ! -x "$BIN" ]]; then
  # fallback: build output or go run
  if [[ -x "./opendrop" ]]; then BIN="./opendrop"; fi
fi

case "${1:-start}" in
  start)
    if tmux has-session -t "$SESSION" 2>/dev/null; then
      echo "tmux session '$SESSION' already running. Attach: tmux attach -t $SESSION"
    else
      tmux new-session -d -s "$SESSION" "$BIN daemon"
      echo "started '$SESSION': $BIN daemon"
      echo "attach: tmux attach -t $SESSION | stop: $0 stop | logs: $0 logs"
    fi
    ;;
  stop)
    tmux kill-session -t "$SESSION" 2>/dev/null && echo "stopped '$SESSION'" || echo "no session '$SESSION'"
    ;;
  logs|attach)
    tmux attach -t "$SESSION" 2>/dev/null || tmux capture-pane -p -t "$SESSION" 2>/dev/null || echo "no session '$SESSION'"
    ;;
  status)
    if tmux has-session -t "$SESSION" 2>/dev/null; then echo "RUNNING ($SESSION)"; else echo "NOT RUNNING"; fi
    "$BIN" status || true
    ;;
esac
