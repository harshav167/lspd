#!/usr/bin/env sh
set -eu

LSPD_BIN="${LSPD_BIN:-$(command -v lspd || printf '%s' "$HOME/.local/bin/lspd")}"
PORT_FILE="${LSPD_PORT_FILE:-${HOME}/.factory/run/lspd.port}"
TMP_PORT="${PORT_FILE}.tmp.$$"
TMP_ERR="${PORT_FILE}.err.$$"
trap 'rm -f "$TMP_PORT" "$TMP_ERR"' EXIT

if [ -f "$PORT_FILE" ] && "$LSPD_BIN" ping >/dev/null 2>&1; then
  PORT="$(cat "$PORT_FILE")"
else
  nohup "$LSPD_BIN" start --foreground >"$TMP_PORT" 2>"$TMP_ERR" </dev/null &
  i=0
  while [ $i -lt 50 ]; do
    if [ -s "$TMP_PORT" ]; then
      PORT="$(cat "$TMP_PORT")"
      printf '%s' "$PORT" >"$PORT_FILE"
      break
    fi
    i=$((i + 1))
    sleep 0.1
  done
  if [ -z "${PORT:-}" ]; then
    echo "[lspd] warning: daemon failed to start within timeout" >&2
    exit 0
  fi
fi

if [ -n "${CLAUDE_ENV_FILE:-}" ]; then
  printf 'export FACTORY_VSCODE_MCP_PORT=%s\n' "$PORT" >>"$CLAUDE_ENV_FILE"
fi

printf '{"hookSpecificOutput":{"additionalContext":"LSP bridge active: semantic code navigation tools (lspDefinition, lspReferences, lspHover, lspWorkspaceSymbol, lspDocumentSymbol, lspCodeActions, lspRename, lspFormat, lspCallHierarchy, lspTypeHierarchy) are available as IDE-native tools. Diagnostics are automatically injected after Read, Edit, Create, and Write."}}'
