#!/usr/bin/env sh
# Advanced/testing launcher only.
# Production installs should run plain `droid`; this wrapper is for branch-local
# settings isolation when debugging lspd itself.
set -eu

LSPD_BIN="${LSPD_BIN:-$(command -v lspd || printf '%s' "$HOME/.local/bin/lspd")}"
LSPD_CONFIG="${DROID_LSP_CONFIG:-$HOME/.local/bin/droid-lsp-config.yaml}"
PORT_FILE="${LSPD_PORT_FILE:-${HOME}/.factory/run/droid-lsp/lspd.port}"
DEFAULT_PORT_FILE="${HOME}/.factory/run/droid-lsp/lspd.port"
SETTINGS_FILE="${DROID_LSP_SETTINGS:-$HOME/.local/bin/droid-lsp-settings.json}"
TMP_PORT="/tmp/lspd-port.$$"
TMP_ERR="/tmp/lspd.err.$$"
trap 'rm -f "$TMP_PORT" "$TMP_ERR"' EXIT

ensure_port_file() {
  if [ -s "$PORT_FILE" ]; then
    return 0
  fi
  status_json="$("$LSPD_BIN" status --json --config "$LSPD_CONFIG" 2>/dev/null || true)"
  [ -n "$status_json" ] || return 1
  port="$(python3 -c 'import json,sys; data=json.loads(sys.argv[1]); port=data.get("port"); print(port if port else "", end="")' "$status_json")"
  [ -n "$port" ] || return 1
  mkdir -p "$(dirname "$PORT_FILE")"
  printf '%s' "$port" >"$PORT_FILE"
  return 0
}

resolve_real_droid() {
  self_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
  self_path="${self_dir}/$(basename -- "$0")"

  if [ -n "${REAL_DROID:-}" ] && [ -x "${REAL_DROID}" ]; then
    printf '%s\n' "${REAL_DROID}"
    return 0
  fi

  candidate="$HOME/.local/bin/droid"
  if [ -x "$candidate" ] && [ "$candidate" != "$self_path" ]; then
    printf '%s\n' "$candidate"
    return 0
  fi

  echo "Error: unable to locate the real droid binary at $HOME/.local/bin/droid. Set REAL_DROID=/path/to/droid." >&2
  exit 1
}

export LSPD_CONFIG
mkdir -p "$(dirname "$PORT_FILE")"

if "$LSPD_BIN" ping --config "$LSPD_CONFIG" >/dev/null 2>&1; then
  if [ ! -f "$PORT_FILE" ] && [ -f "$DEFAULT_PORT_FILE" ]; then
    cat "$DEFAULT_PORT_FILE" >"$PORT_FILE"
  fi
  ensure_port_file || true
else
  "$LSPD_BIN" start --config "$LSPD_CONFIG" >"$TMP_PORT" 2>"$TMP_ERR" || true
  i=0
  while [ $i -lt 50 ]; do
    if [ ! -s "$PORT_FILE" ] && [ -s "$TMP_PORT" ]; then
      cat "$TMP_PORT" >"$PORT_FILE"
    fi
    if [ -s "$PORT_FILE" ]; then
      break
    fi
    i=$((i + 1))
    sleep 0.1
  done
  if [ ! -s "$PORT_FILE" ]; then
    ensure_port_file || true
  fi
  if [ ! -s "$PORT_FILE" ]; then
    echo "Error: lspd failed to start within timeout" >&2
    cat "$TMP_ERR" >&2 2>/dev/null || true
    exit 1
  fi
fi

export FACTORY_VSCODE_MCP_PORT="$(cat "$PORT_FILE")"
for arg in "$@"; do
  if [ "$arg" = "--settings" ]; then
    exec "$(resolve_real_droid)" "$@"
  fi
done
exec "$(resolve_real_droid)" --settings "$SETTINGS_FILE" "$@"
