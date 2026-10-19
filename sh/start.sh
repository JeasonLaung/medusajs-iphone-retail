#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_DIR="$PROJECT_DIR/medusa-store"
LOG_DIR="$PROJECT_DIR/log"
LOG_FILE="$LOG_DIR/$(date +%Y%m%d%H%M).log"

if ! command -v docker >/dev/null 2>&1; then
  echo "Missing command: docker" >&2
  exit 1
fi
if ! command -v curl >/dev/null 2>&1; then
  echo "Missing command: curl" >&2
  exit 1
fi

if command -v pnpm >/dev/null 2>&1; then
  pnpm_command=(pnpm)
elif command -v corepack >/dev/null 2>&1; then
  pnpm_command=(corepack pnpm)
else
  echo "Missing command: pnpm or corepack" >&2
  exit 1
fi

if [[ ! -d "$WORKSPACE_DIR/node_modules" ]]; then
  echo "Dependencies are missing. Run 'corepack pnpm install' in $WORKSPACE_DIR." >&2
  exit 1
fi

if [[ ! -f "$PROJECT_DIR/.env" || ! -f "$WORKSPACE_DIR/apps/backend/.env" ]]; then
  echo "Missing .env configuration. See README.md for local setup." >&2
  exit 1
fi

mkdir -p "$LOG_DIR"

start_service() {
  local name="$1" directory="$2" port="$3" url="$4" pid_file="$LOG_DIR/$1.pid"
  local pid attempt

  if [[ -f "$pid_file" ]]; then
    pid="$(cat "$pid_file")"
    if [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null; then
      echo "$name is already running (PID $pid)."
      return
    fi
    rm -f "$pid_file"
  fi

  if command -v lsof >/dev/null 2>&1 && lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "Port $port is already in use; cannot start $name." >&2
    return 1
  fi

  nohup bash -c 'cd "$1" && shift && exec "$@" run dev' _ "$directory" "${pnpm_command[@]}" >>"$LOG_FILE" 2>&1 </dev/null &
  pid=$!
  echo "$pid" >"$pid_file"
  for attempt in {1..60}; do
    if curl -fsS -o /dev/null --max-time 2 "$url" 2>/dev/null; then
      echo "$name is ready (PID $pid)."
      return
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
      break
    fi
    sleep 2
  done

  echo "$name failed to become ready. Recent log output:" >&2
  tail -n 30 "$LOG_FILE" >&2
  return 1
}

echo "Starting Docker services..."
(cd "$PROJECT_DIR" && docker compose up -d --wait) 2>&1 | tee -a "$LOG_FILE"

start_service medusa "$WORKSPACE_DIR/apps/backend" 9001 'http://127.0.0.1:9001/health'
start_service storefront "$WORKSPACE_DIR/apps/storefront" 8000 'http://127.0.0.1:8000/dk/store'

echo "Log: $LOG_FILE"
echo "Admin: http://127.0.0.1:9001/app"
echo "Store: http://127.0.0.1:8000/dk/store"
