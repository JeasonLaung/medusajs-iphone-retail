#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
FRPC_BIN="${SCRIPT_DIR}/frp/frpc"
FRPC_CONFIG="${SCRIPT_DIR}/frp/frpc.toml"
LOG_DIR="${SCRIPT_DIR}/logs"
GATEWAY_PID=""
FRPC_PID=""

if [[ ! -x "${FRPC_BIN}" ]]; then
  FRPC_BIN="$(command -v frpc || true)"
fi
if [[ -z "${FRPC_BIN}" ]]; then
  echo "FRP client executable not found. Install frpc or place it in frp/frpc." >&2
  exit 1
fi
if [[ ! -f "${FRPC_CONFIG}" ]]; then
  echo "FRP client configuration not found: ${FRPC_CONFIG}" >&2
  exit 1
fi
if grep -q 'YOUR_FRP_' "${FRPC_CONFIG}"; then
  echo "Fill in the credentials in frp/frpc.toml before starting FRP." >&2
  exit 1
fi
"${FRPC_BIN}" verify -c "${FRPC_CONFIG}"
command -v node >/dev/null
command -v lsof >/dev/null
mkdir -p "${LOG_DIR}"
if [[ -f "${LOG_DIR}/frpc.pid" ]]; then
  existing_pid="$(cat "${LOG_DIR}/frpc.pid")"
  if [[ "${existing_pid}" =~ ^[0-9]+$ ]] && kill -0 "${existing_pid}" 2>/dev/null; then
    echo "FRP PID ${existing_pid} is still running. Check it before starting another tunnel." >&2
    exit 1
  fi
fi

cleanup() {
  if [[ -n "${FRPC_PID}" ]]; then
    kill "${FRPC_PID}" 2>/dev/null || true
    wait "${FRPC_PID}" 2>/dev/null || true
    rm -f "${LOG_DIR}/frpc.pid"
  fi
  if [[ -n "${GATEWAY_PID}" ]]; then
    kill "${GATEWAY_PID}" 2>/dev/null || true
    wait "${GATEWAY_PID}" 2>/dev/null || true
    rm -f "${LOG_DIR}/frp-gateway.pid"
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

listeners="$(lsof -nP -tiTCP:18080 -sTCP:LISTEN || true)"
if [[ -n "${listeners}" ]]; then
  gateway_owner="$(cat "${LOG_DIR}/frp-gateway.pid" 2>/dev/null || true)"
  if [[ "${listeners}" != "${gateway_owner}" ]] || ! ps -p "${gateway_owner}" -o command= | grep -Fq "${SCRIPT_DIR}/scripts/frp-gateway.cjs"; then
    echo "Port 18080 is occupied by another process." >&2
    exit 1
  fi
else
  node "${SCRIPT_DIR}/scripts/frp-gateway.cjs" >>"${LOG_DIR}/frp-gateway.out.log" 2>>"${LOG_DIR}/frp-gateway.err.log" &
  GATEWAY_PID=$!
  echo "${GATEWAY_PID}" >"${LOG_DIR}/frp-gateway.pid"
  for attempt in {1..20}; do
    if lsof -nP -a -p "${GATEWAY_PID}" -iTCP:18080 -sTCP:LISTEN >/dev/null 2>&1; then
      break
    fi
    if ! kill -0 "${GATEWAY_PID}" 2>/dev/null || [[ "${attempt}" == 20 ]]; then
      echo "FRP gateway failed to start. Check logs/frp-gateway.err.log." >&2
      exit 1
    fi
    sleep 0.2
  done
fi

echo "FRP gateway: 127.0.0.1:18080 -> backend 9001 / storefront 8000"
echo "Store: https://frp.xeappliance.com/"
echo "Admin: https://frp.xeappliance.com/app"
"${FRPC_BIN}" -c "${FRPC_CONFIG}" &
FRPC_PID=$!
echo "${FRPC_PID}" >"${LOG_DIR}/frpc.pid"
wait "${FRPC_PID}"
