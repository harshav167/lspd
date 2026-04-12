#!/bin/sh
# lspd installer — downloads pre-built binaries, merges hooks, zero dependencies.
# Idempotent: safe to run multiple times.
set -eu

REPO="harshav167/lspd"
INSTALL_DIR="${LSPD_INSTALL_DIR:-$HOME/.local/bin}"
CONFIG_DIR="$HOME/.factory/hooks/lsp"
SETTINGS_FILE="$HOME/.factory/settings.json"

# Detect platform
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
    *)       echo "Error: unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Resolve version
VERSION="${LSPD_VERSION:-latest}"
if [ "$VERSION" = "latest" ]; then
    DOWNLOAD_BASE="https://github.com/$REPO/releases/latest/download"
else
    DOWNLOAD_BASE="https://github.com/$REPO/releases/download/$VERSION"
fi

echo "==> Installing lspd for $OS/$ARCH..."

# Create directories
mkdir -p "$INSTALL_DIR"
mkdir -p "$CONFIG_DIR"
mkdir -p "$HOME/.factory/run/lspd"
mkdir -p "$HOME/.factory/logs/lspd"
mkdir -p "$HOME/.factory/ide"

# Stop existing daemon — try graceful first, then force kill
if [ -x "$INSTALL_DIR/lspd" ]; then
    "$INSTALL_DIR/lspd" stop --config "$CONFIG_DIR/lspd.yaml" >/dev/null 2>&1 || true
fi
# Force kill any remaining lspd processes and clean stale state
pkill -9 -x lspd 2>/dev/null || true
sleep 1
rm -f "$HOME/.factory/ide/"*.lock "$HOME/.factory/run/lspd/lspd.pid" "$HOME/.factory/run/lspd/lspd.port" 2>/dev/null || true

# Download binaries
echo "==> Downloading binaries..."
curl -fsSL "$DOWNLOAD_BASE/lspd-$OS-$ARCH" -o "$INSTALL_DIR/lspd"
curl -fsSL "$DOWNLOAD_BASE/lsp-read-hook-$OS-$ARCH" -o "$INSTALL_DIR/lsp-read-hook"
curl -fsSL "$DOWNLOAD_BASE/session-start.sh" -o "$INSTALL_DIR/lspd-session-start"
chmod +x "$INSTALL_DIR/lspd" "$INSTALL_DIR/lsp-read-hook" "$INSTALL_DIR/lspd-session-start"
echo "    Installed to $INSTALL_DIR"

# Write config (skip if exists — don't overwrite user customizations)
if [ ! -f "$CONFIG_DIR/lspd.yaml" ]; then
    cat > "$CONFIG_DIR/lspd.yaml" << 'YAML'
run_dir: ~/.factory/run/lspd
log_file: ~/.factory/logs/lspd/lspd.log
socket:
  path: ~/.factory/run/lspd/lspd.sock
YAML
    echo "==> Config written to $CONFIG_DIR/lspd.yaml"
else
    echo "==> Config already exists at $CONFIG_DIR/lspd.yaml (preserved)"
fi

# Merge hooks into settings.json
echo "==> Merging hooks into settings..."

if [ ! -f "$SETTINGS_FILE" ]; then
    echo '{}' > "$SETTINGS_FILE"
fi

# Backup before modifying
cp "$SETTINGS_FILE" "$SETTINGS_FILE.pre-lspd.bak"

python3 << 'PY'
import json, os

settings_path = os.path.expanduser("~/.factory/settings.json")
install_dir = os.environ.get("LSPD_INSTALL_DIR", os.path.expanduser("~/.local/bin"))
home = os.path.expanduser("~")

with open(settings_path) as f:
    settings = json.load(f)

if "hooks" not in settings:
    settings["hooks"] = {}
if "general" not in settings:
    settings["general"] = {}

settings["general"]["ideAutoConnect"] = True

lspd_hooks = {
    "SessionStart": {
        "matcher": "",
        "hooks": [{"type": "command", "command": os.path.join(install_dir, "lspd-session-start"), "timeout": 5}]
    },
    "PostToolUse": {
        "matcher": "Read",
        "hooks": [{"type": "command", "command": "LSPD_SOCKET_PATH=" + os.path.join(home, ".factory/run/lspd/lspd.sock") + " " + os.path.join(install_dir, "lsp-read-hook"), "timeout": 3}]
    },
}

for event, new_hook in lspd_hooks.items():
    existing = settings["hooks"].get(event, [])
    cleaned = [g for g in existing if not any("lspd" in h.get("command", "") or "lsp-read-hook" in h.get("command", "") for h in g.get("hooks", []))]
    cleaned.append(new_hook)
    settings["hooks"][event] = cleaned

with open(settings_path, "w") as f:
    json.dump(settings, f, indent=2)

print("    Hooks merged (ideAutoConnect: true)")
PY

# Start lspd NOW — so the lock file exists before droid ever starts.
# No timing race. The daemon stays running across sessions.
echo "==> Starting lspd..."
"$INSTALL_DIR/lspd" stop --config "$CONFIG_DIR/lspd.yaml" >/dev/null 2>&1 || true
"$INSTALL_DIR/lspd" start --config "$CONFIG_DIR/lspd.yaml" >/dev/null 2>&1

# Verify it's running and the lock file exists
if "$INSTALL_DIR/lspd" ping --config "$CONFIG_DIR/lspd.yaml" >/dev/null 2>&1; then
    LOCK_COUNT=$(ls "$HOME/.factory/ide/"*.lock 2>/dev/null | wc -l)
    PORT=$(cat "$HOME/.factory/run/lspd/lspd.port" 2>/dev/null || echo "unknown")
    echo "    lspd running on port $PORT ($LOCK_COUNT lock file(s))"
else
    echo "    WARNING: lspd failed to start. Run '$INSTALL_DIR/lspd start --config $CONFIG_DIR/lspd.yaml' manually."
fi

echo ""
echo "Done! Run 'droid' normally — lspd is already running."
echo ""
echo "Update:    curl -fsSL https://github.com/$REPO/releases/latest/download/install.sh | sh"
echo "Uninstall: curl -fsSL https://github.com/$REPO/releases/latest/download/uninstall.sh | sh"
