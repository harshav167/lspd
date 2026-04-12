#!/usr/bin/env sh
# lspd SessionStart hook — starts daemon if needed, emits additionalContext.
# Installed to ~/.local/bin/lspd-session-start by scripts/install.sh.
set -eu

CONFIG="${LSPD_CONFIG:-$HOME/.factory/hooks/lsp/lspd.yaml}"

# Start lspd if not already running (idempotent).
# lspd writes ~/.factory/ide/<port>.lock on startup, so Droid auto-discovers it.
if ! lspd ping --config "$CONFIG" >/dev/null 2>&1; then
    lspd start --config "$CONFIG" >/dev/null 2>&1 || exit 0
fi

# Emit additionalContext so the model knows LSP tools are available
printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"LSP bridge active. Diagnostics are injected after every Edit, Create, and Read. Semantic tools available: lspDefinition, lspReferences, lspHover, lspWorkspaceSymbol, lspDocumentSymbol, lspCodeActions, lspRename, lspFormat, lspCallHierarchy, lspTypeHierarchy."}}'
