#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$PROJECT_DIR/log"

collect_children() {
  local child
  while read -r child; do
    [[ -n "$child" ]] || continue
    collect_children "$child"
    descendants+=("$child")
  done < <(pgrep -P "$1" 2>/dev/null || true)
}

stop_service() {
  local name="$1" pid_file="$LOG_DIR/$1.pid" pid process attempt
  local -a descendants=()

  if [[ ! -f "$pid_file" ]]; then
    echo "$name is not managed by these scripts."
    return
  fi

  pid="$(cat "$pid_file")"
  if [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null; then
    descendants=("$pid")
    collect_children "$pid"
    kill -TERM "${descendants[@]}" 2>/dev/null || true
    for attempt in {1..10}; do
      if ! kill -0 "$pid" 2>/dev/null; then
        break
      fi
      sleep 0.5
    done
    for process in "${descendants[@]}"; do
      if kill -0 "$process" 2>/dev/null; then
        kill -KILL "$process" 2>/dev/null || true
      fi
    done
    echo "Stopped $name (PID $pid)."
  fi
  rm -f "$pid_file"
}

stop_service storefront
stop_service medusa

if command -v docker >/dev/null 2>&1; then
  (cd "$PROJECT_DIR" && docker compose down)
else
  echo "Docker is unavailable; Docker services were not stopped." >&2
  exit 1
fi
