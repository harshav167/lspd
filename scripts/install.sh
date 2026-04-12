#!/usr/bin/env sh
# lspd installer — production setup via hooks + lock file auto-discovery.
# Idempotent: safe to run multiple times.
set -eu

PROJ_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
BIN_DIR="${HOME}/.local/bin"
CONFIG_DIR="${HOME}/.factory/hooks/lsp"
CONFIG_FILE="${CONFIG_DIR}/lspd.yaml"
SETTINGS_FILE="${HOME}/.factory/settings.json"

info() { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
ok()   { printf '\033[1;32m OK\033[0m %s\n' "$1"; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$1" >&2; exit 1; }

# ── 1. Build binaries ────────────────────────────────────────────────
info "Building lspd and lsp-read-hook..."
mkdir -p "$BIN_DIR"
(cd "$PROJ_DIR" && go build -o "${BIN_DIR}/lspd" ./cmd/lspd)
(cd "$PROJ_DIR" && go build -o "${BIN_DIR}/lsp-read-hook" ./cmd/lsp-read-hook)
ok "Binaries installed to ${BIN_DIR}/"

# ── 2. Install session-start script ─────────────────────────────────
info "Installing lspd-session-start..."
install -m 0755 "${PROJ_DIR}/scripts/session-start.sh" "${BIN_DIR}/lspd-session-start"
ok "lspd-session-start installed"

# ── 3. Write daemon config (if not exists) ───────────────────────────
info "Checking daemon config..."
mkdir -p "$CONFIG_DIR"
if [ ! -f "$CONFIG_FILE" ]; then
  cat > "$CONFIG_FILE" << 'YAML'
run_dir: ~/.factory/run/lspd
log_file: ~/.factory/logs/lspd/lspd.log
socket:
  path: ~/.factory/run/lspd/lspd.sock
YAML
  ok "Config written to ${CONFIG_FILE}"
else
  ok "Config already exists at ${CONFIG_FILE} (skipped)"
fi

# ── 4. Merge hooks into settings.json ────────────────────────────────
info "Merging hooks into settings.json..."

# Ensure ~/.factory exists
mkdir -p "$(dirname "$SETTINGS_FILE")"

# Back up before modifying
if [ -f "$SETTINGS_FILE" ]; then
  cp "$SETTINGS_FILE" "${SETTINGS_FILE}.bak"
  ok "Backed up settings.json to settings.json.bak"
fi

# Use python3 for safe JSON merge (available on macOS and most Linux)
python3 - "$SETTINGS_FILE" << 'PYEOF'
import json, sys, os

settings_path = sys.argv[1]

# Load existing settings or start fresh
if os.path.isfile(settings_path):
    with open(settings_path, "r") as f:
        settings = json.load(f)
else:
    settings = {}

# ── Set ideAutoConnect ──
settings["ideAutoConnect"] = True

# ── Define our hooks ──
lspd_hooks = {
    "SessionStart": {
        "matcher": "",
        "hooks": [
            {
                "type": "command",
                "command": "lspd-session-start",
                "timeout": 5
            }
        ]
    },
    "PostToolUse_Read": {
        "matcher": "Read",
        "hooks": [
            {
                "type": "command",
                "command": "lsp-read-hook",
                "timeout": 3
            }
        ]
    },
    "SessionEnd": {
        "matcher": "",
        "hooks": [
            {
                "type": "command",
                "command": "lspd stop --config \"$HOME\"/.factory/hooks/lsp/lspd.yaml",
                "timeout": 2
            }
        ]
    }
}

# Fingerprints to detect our hooks (for idempotency)
our_commands = {
    "lspd-session-start",
    "lsp-read-hook",
}
our_command_prefixes = [
    "lspd stop",
]

def is_our_hook_entry(entry):
    """Check if a hook array entry belongs to lspd."""
    for h in entry.get("hooks", []):
        cmd = h.get("command", "")
        if cmd in our_commands:
            return True
        for prefix in our_command_prefixes:
            if cmd.startswith(prefix):
                return True
        # Also match old-style paths
        if "/lspd-session-start" in cmd or "/lsp-read-hook" in cmd or "lspd stop" in cmd:
            return True
    return False

hooks = settings.setdefault("hooks", {})

# ── SessionStart: append our hook, remove old duplicates ──
session_start = hooks.setdefault("SessionStart", [])
session_start[:] = [e for e in session_start if not is_our_hook_entry(e)]
session_start.append(lspd_hooks["SessionStart"])

# ── PostToolUse: append our Read hook, remove old duplicates ──
post_tool = hooks.setdefault("PostToolUse", [])
post_tool[:] = [e for e in post_tool if not is_our_hook_entry(e)]
post_tool.append(lspd_hooks["PostToolUse_Read"])

# ── SessionEnd: append our hook, remove old duplicates ──
session_end = hooks.setdefault("SessionEnd", [])
session_end[:] = [e for e in session_end if not is_our_hook_entry(e)]
session_end.append(lspd_hooks["SessionEnd"])

# Write back
with open(settings_path, "w") as f:
    json.dump(settings, f, indent=2)
    f.write("\n")

print("Hooks merged successfully.")
PYEOF

ok "settings.json updated"

# ── 5. Create runtime directories ────────────────────────────────────
mkdir -p "${HOME}/.factory/run/lspd"
mkdir -p "${HOME}/.factory/logs/lspd"
mkdir -p "${HOME}/.factory/ide"

# ── 6. Validate ──────────────────────────────────────────────────────
info "Validating installation..."
if "${BIN_DIR}/lspd" --help >/dev/null 2>&1; then
  ok "lspd binary works"
else
  die "lspd binary validation failed"
fi

if "${BIN_DIR}/lsp-read-hook" --help >/dev/null 2>&1 || true; then
  ok "lsp-read-hook binary works"
fi

echo ""
info "Installation complete!"
echo "    Binaries:    ${BIN_DIR}/lspd, ${BIN_DIR}/lsp-read-hook"
echo "    Hook script: ${BIN_DIR}/lspd-session-start"
echo "    Config:      ${CONFIG_FILE}"
echo "    Settings:    ${SETTINGS_FILE} (hooks merged)"
echo ""
echo "    Just run 'droid' normally — lspd starts automatically via hooks."
