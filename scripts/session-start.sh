#!/usr/bin/env sh
# lspd SessionStart hook — starts the daemon if not running.
# lspd writes ~/.factory/ide/<port>.lock on startup.
# Droid auto-discovers it via ideAutoConnect.
set -eu

LSPD_BIN="${LSPD_BIN:-$HOME/.local/bin/lspd}"
CONFIG="${LSPD_CONFIG:-$HOME/.factory/hooks/lsp/lspd.yaml}"
PORT_FILE="${LSPD_PORT_FILE:-$HOME/.factory/run/lspd/lspd.port}"
TMP_PORT="${PORT_FILE}.tmp.$$"
TMP_ERR="${PORT_FILE}.err.$$"
trap 'rm -f "$TMP_PORT" "$TMP_ERR"' EXIT

mkdir -p "$(dirname "$PORT_FILE")"
nohup "$LSPD_BIN" start --foreground --config "$CONFIG" >"$TMP_PORT" 2>"$TMP_ERR" </dev/null &
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
    echo "[lspd] warning: daemon failed to start within timeout" >&2
    exit 0
fi

printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"LSP bridge active. Diagnostics are injected after every Edit, Create, and Read."}}'
