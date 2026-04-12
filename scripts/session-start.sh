#!/usr/bin/env sh
# lspd SessionStart hook — restarts daemon if it died since install.
# Primary startup happens at install time, not here.
set -eu

LSPD_BIN="${LSPD_BIN:-$HOME/.local/bin/lspd}"
CONFIG="${LSPD_CONFIG:-$HOME/.factory/hooks/lsp/lspd.yaml}"

# Restart lspd if not running (e.g., after reboot)
if ! "$LSPD_BIN" ping --config "$CONFIG" >/dev/null 2>&1; then
    "$LSPD_BIN" start --config "$CONFIG" >/dev/null 2>&1 || exit 0
fi

# Write port to CLAUDE_ENV_FILE so Droid's IdeContextManager connects via Priority 1
PORT_FILE="$HOME/.factory/run/lspd/lspd.port"
if [ -s "$PORT_FILE" ] && [ -n "${CLAUDE_ENV_FILE:-}" ]; then
    printf 'export FACTORY_VSCODE_MCP_PORT=%s\n' "$(cat "$PORT_FILE")" >> "$CLAUDE_ENV_FILE"
fi

printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"LSP bridge active. Diagnostics are injected after every Edit, Create, and Read."}}'
