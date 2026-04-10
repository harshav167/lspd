#!/usr/bin/env sh
set -eu

LSPD_BIN="${LSPD_BIN:-$(command -v lspd || printf '%s' "$HOME/.local/bin/lspd")}"
PORT_FILE="${LSPD_PORT_FILE:-${HOME}/.factory/run/lspd.port}"
TMP_PORT="/tmp/lspd-port.$$"
TMP_ERR="/tmp/lspd.err.$$"
trap 'rm -f "$TMP_PORT" "$TMP_ERR"' EXIT

if [ -f "$PORT_FILE" ] && "$LSPD_BIN" ping >/dev/null 2>&1; then
  :
else
  nohup "$LSPD_BIN" start --foreground >"$TMP_PORT" 2>"$TMP_ERR" </dev/null &
  i=0
  while [ $i -lt 50 ]; do
    if [ -s "$TMP_PORT" ]; then
      cat "$TMP_PORT" >"$PORT_FILE"
      break
    fi
    i=$((i + 1))
    sleep 0.1
  done
  if [ ! -s "$PORT_FILE" ]; then
    echo "Error: lspd failed to start within timeout" >&2
    cat "$TMP_ERR" >&2 2>/dev/null || true
    exit 1
  fi
fi

export FACTORY_VSCODE_MCP_PORT="$(cat "$PORT_FILE")"
exec droid "$@"
