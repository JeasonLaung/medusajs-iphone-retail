#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$PROJECT_DIR/log"

if [[ $# -gt 1 ]]; then
  echo "Usage: $0 [yyyymmddHHMM]" >&2
  exit 1
fi

if [[ $# -eq 1 ]]; then
  if [[ ! "$1" =~ ^[0-9]{12}$ ]]; then
    echo "Expected a timestamp in yyyymmddHHMM format." >&2
    exit 1
  fi
  log_file="$LOG_DIR/$1.log"
else
  shopt -s nullglob
  logs=("$LOG_DIR"/[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9].log)
  if [[ ${#logs[@]} -eq 0 ]]; then
    echo "No logs found in $LOG_DIR." >&2
    exit 1
  fi
  log_file="${logs[${#logs[@]}-1]}"
fi

if [[ ! -f "$log_file" ]]; then
  echo "Log not found: $log_file" >&2
  exit 1
fi

echo "Following $log_file (Ctrl+C to exit)"
tail -n 100 -f "$log_file"
