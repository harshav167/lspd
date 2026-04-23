#!/usr/bin/env sh
# lspd uninstaller — removes the plain-`droid` integration installed by install.sh.
set -eu

BIN_DIR="${HOME}/.local/bin"
CONFIG_DIR="${HOME}/.factory/hooks/lsp"
SETTINGS_FILE="${HOME}/.factory/settings.json"
RUN_DIR="${HOME}/.factory/run/lspd"
LOG_DIR="${HOME}/.factory/logs/lspd"

info() { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
ok()   { printf '\033[1;32m OK\033[0m %s\n' "$1"; }

# ── 1. Stop lspd if running ──────────────────────────────────────────
info "Stopping lspd..."
if "${BIN_DIR}/lspd" ping --config "${CONFIG_DIR}/lspd.yaml" >/dev/null 2>&1; then
  "${BIN_DIR}/lspd" stop --config "${CONFIG_DIR}/lspd.yaml" 2>/dev/null || true
  ok "lspd stopped"
else
  ok "lspd not running"
fi

# ── 2. Remove binaries ───────────────────────────────────────────────
info "Removing binaries..."
rm -f "${BIN_DIR}/lspd"
rm -f "${BIN_DIR}/lsp-read-hook"
rm -f "${BIN_DIR}/lspd-session-start"
rm -f "${BIN_DIR}/droid-lsp"
if [ -x "${BIN_DIR}/droid.real" ] && grep -q "DROID_LSP_WRAPPER" "${BIN_DIR}/droid" 2>/dev/null; then
  rm -f "${BIN_DIR}/droid"
  mv "${BIN_DIR}/droid.real" "${BIN_DIR}/droid"
  ok "Restored original droid binary"
fi
rm -f "${BIN_DIR}/droid-lsp"
if [ -x "${BIN_DIR}/droid.real" ] && grep -q "DROID_LSP_WRAPPER" "${BIN_DIR}/droid" 2>/dev/null; then
  rm -f "${BIN_DIR}/droid"
  mv "${BIN_DIR}/droid.real" "${BIN_DIR}/droid"
  ok "Restored original droid binary"
fi
ok "Binaries removed"

# ── 3. Remove config ─────────────────────────────────────────────────
info "Removing config..."
rm -rf "$CONFIG_DIR"
ok "Config directory removed"

# ── 4. Remove lspd hooks from settings.json ──────────────────────────
info "Removing hooks from settings.json..."
if [ -f "$SETTINGS_FILE" ]; then
  cp "$SETTINGS_FILE" "${SETTINGS_FILE}.bak"
  python3 - "$SETTINGS_FILE" << 'PYEOF'
import json, sys, os

settings_path = sys.argv[1]

if not os.path.isfile(settings_path):
    sys.exit(0)

with open(settings_path, "r") as f:
    settings = json.load(f)

hooks = settings.get("hooks", {})

our_command_markers = [
    "lspd-session-start",
    "lsp-read-hook",
    "lspd stop",
]

def is_our_hook_entry(entry):
    """Check if a hook array entry belongs to lspd."""
    for h in entry.get("hooks", []):
        cmd = h.get("command", "")
        for marker in our_command_markers:
            if marker in cmd:
                return True
    return False

for event_name in list(hooks.keys()):
    entries = hooks[event_name]
    if isinstance(entries, list):
        filtered = [e for e in entries if not is_our_hook_entry(e)]
        if filtered:
            hooks[event_name] = filtered
        else:
            del hooks[event_name]

with open(settings_path, "w") as f:
    json.dump(settings, f, indent=2)
    f.write("\n")

print("lspd hooks removed from settings.json.")
PYEOF
  ok "Hooks removed"
else
  ok "No settings.json found"
fi

# ── 5. Remove runtime directories ────────────────────────────────────
info "Cleaning up runtime directories..."
rm -rf "$RUN_DIR"
rm -rf "$LOG_DIR"
ok "Runtime directories removed"

echo ""
info "Uninstall complete. Plain 'droid' no longer starts lspd automatically."
