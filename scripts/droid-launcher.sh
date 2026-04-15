#!/usr/bin/env sh
# DROID_LSP_WRAPPER
set -eu

LSPD_BIN="${LSPD_BIN:-$(command -v lspd || printf '%s' "$HOME/.local/bin/lspd")}"
LSPD_CONFIG="${DROID_LSP_CONFIG:-$HOME/.factory/hooks/lsp/lspd.yaml}"
PORT_FILE="${LSPD_PORT_FILE:-${HOME}/.factory/run/lspd/lspd.port}"
TMP_PORT="/tmp/lspd-port.$$"
TMP_ERR="/tmp/lspd.err.$$"
trap 'rm -f "$TMP_PORT" "$TMP_ERR"' EXIT

resolve_real_droid() {
  if [ -n "${REAL_DROID:-}" ] && [ -x "${REAL_DROID}" ]; then
    printf '%s\n' "${REAL_DROID}"
    return 0
  fi

  candidate="$HOME/.local/bin/droid.real"
  if [ -x "$candidate" ]; then
    printf '%s\n' "$candidate"
    return 0
  fi

  echo "Error: unable to locate the real droid binary at $HOME/.local/bin/droid.real. Set REAL_DROID=/path/to/droid." >&2
  exit 1
}

export LSPD_CONFIG
mkdir -p "$(dirname "$PORT_FILE")"

nohup "$LSPD_BIN" start --foreground --config "$LSPD_CONFIG" >"$TMP_PORT" 2>"$TMP_ERR" </dev/null &
i=0
while [ $i -lt 50 ]; do
  if [ -s "$TMP_PORT" ]; then
    cat "$TMP_PORT" >"$PORT_FILE"
    break
  fi
  if [ -s "$PORT_FILE" ]; then
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

export FACTORY_VSCODE_MCP_PORT="$(cat "$PORT_FILE")"
exec "$(resolve_real_droid)" "$@"
