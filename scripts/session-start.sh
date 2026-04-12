#!/usr/bin/env sh
# lspd SessionStart hook — starts daemon if needed, emits additionalContext.
# Installed to ~/.local/bin/lspd-session-start by scripts/install.sh.
set -eu

LSPD_BIN="${LSPD_BIN:-$HOME/.local/bin/lspd}"
CONFIG="${LSPD_CONFIG:-$HOME/.factory/hooks/lsp/lspd.yaml}"

# Start lspd if not already running (idempotent).
# Uses absolute path — ~/.local/bin may not be in PATH on all systems.
# lspd writes ~/.factory/ide/<port>.lock on startup, so Droid auto-discovers it.
if ! "$LSPD_BIN" ping --config "$CONFIG" >/dev/null 2>&1; then
    "$LSPD_BIN" start --config "$CONFIG" >/dev/null 2>&1 || exit 0
fi

# Emit additionalContext so the model knows LSP tools are available
printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"LSP bridge active. Diagnostics are injected after every Edit, Create, and Read. Semantic tools available: lspDefinition, lspReferences, lspHover, lspWorkspaceSymbol, lspDocumentSymbol, lspCodeActions, lspRename, lspFormat, lspCallHierarchy, lspTypeHierarchy."}}'
