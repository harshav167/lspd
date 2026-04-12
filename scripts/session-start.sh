#!/usr/bin/env sh
# lspd SessionStart hook — starts the daemon if not running.
# lspd writes ~/.factory/ide/<port>.lock on startup.
# Droid auto-discovers it via ideAutoConnect.
set -eu

LSPD_BIN="${LSPD_BIN:-$HOME/.local/bin/lspd}"
CONFIG="${LSPD_CONFIG:-$HOME/.factory/hooks/lsp/lspd.yaml}"

if ! "$LSPD_BIN" ping --config "$CONFIG" >/dev/null 2>&1; then
    "$LSPD_BIN" start --config "$CONFIG" >/dev/null 2>&1 || exit 0
fi

printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"LSP bridge active. Diagnostics are injected after every Edit, Create, and Read."}}'
