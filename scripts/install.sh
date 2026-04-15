#!/bin/sh
# lspd installer — installs binaries/config and merges read/cleanup hooks without changing the droid entrypoint.
# Idempotent: safe to run multiple times.
set -eu

REPO="harshav167/lspd"
INSTALL_DIR="${LSPD_INSTALL_DIR:-$HOME/.local/bin}"
CONFIG_DIR="$HOME/.factory/hooks/lsp"
SETTINGS_FILE="$HOME/.factory/settings.json"
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
ROOT_DIR="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"

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

# Install binaries/scripts from local repo when available, otherwise fall back to release assets.
echo "==> Installing binaries and scripts..."
if [ -x "$ROOT_DIR/lspd" ]; then
    cp "$ROOT_DIR/lspd" "$INSTALL_DIR/lspd"
else
    curl -fsSL "$DOWNLOAD_BASE/lspd-$OS-$ARCH" -o "$INSTALL_DIR/lspd"
fi
if [ -x "$ROOT_DIR/lsp-read-hook" ]; then
    cp "$ROOT_DIR/lsp-read-hook" "$INSTALL_DIR/lsp-read-hook"
else
    curl -fsSL "$DOWNLOAD_BASE/lsp-read-hook-$OS-$ARCH" -o "$INSTALL_DIR/lsp-read-hook"
fi
if [ -f "$SCRIPT_DIR/droid-launcher.sh" ]; then
    cp "$SCRIPT_DIR/droid-launcher.sh" "$INSTALL_DIR/droid-lsp"
else
    curl -fsSL "$DOWNLOAD_BASE/droid-launcher.sh" -o "$INSTALL_DIR/droid-lsp"
fi
chmod +x "$INSTALL_DIR/lspd" "$INSTALL_DIR/lsp-read-hook" "$INSTALL_DIR/droid-lsp"
echo "    Installed to $INSTALL_DIR"
echo "    Installed optional convenience wrapper as $INSTALL_DIR/droid-lsp"
rm -f "$INSTALL_DIR/lspd-session-start"

# Undo any previous wrapper promotion so regular `droid` remains untouched.
if [ -x "$INSTALL_DIR/droid.real" ] && grep -q "DROID_LSP_WRAPPER" "$INSTALL_DIR/droid" 2>/dev/null; then
    rm -f "$INSTALL_DIR/droid"
    mv "$INSTALL_DIR/droid.real" "$INSTALL_DIR/droid"
    echo "==> Restored original droid binary"
fi

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

events_to_clean = ["SessionStart", "PostToolUse", "SessionEnd"]

for event in events_to_clean:
    existing = settings["hooks"].get(event, [])
    cleaned = [
        g for g in existing
        if not any(
            "lspd" in h.get("command", "") or "lsp-read-hook" in h.get("command", "")
            for h in g.get("hooks", [])
        )
    ]
    if cleaned:
        settings["hooks"][event] = cleaned
    elif event in settings["hooks"]:
        del settings["hooks"][event]

session_scoped_hooks = {
    "PostToolUse": {
        "matcher": "Read",
        "hooks": [{"type": "command", "command": "LSPD_SOCKET_PATH=" + os.path.join(home, ".factory/run/lspd/lspd.sock") + " " + os.path.join(install_dir, "lsp-read-hook"), "timeout": 3}]
    },
    "SessionEnd": {
        "matcher": ".*",
        "hooks": [{"type": "command", "command": os.path.join(install_dir, "lspd") + " forget --session \"$session_id\"", "timeout": 2}]
    },
}

for event, hook_group in session_scoped_hooks.items():
    existing = settings["hooks"].get(event, [])
    existing.append(hook_group)
    settings["hooks"][event] = existing

with open(settings_path, "w") as f:
    json.dump(settings, f, indent=2)

print("    Read/SessionEnd hooks merged")
PY

echo ""
echo "Done! Start lspd explicitly for a coding session:"
echo "    lspd start --config $CONFIG_DIR/lspd.yaml"
echo "    droid"
echo ""
echo "lspd will exit automatically after its idle timeout, or you can stop it manually:"
echo "    lspd stop --config $CONFIG_DIR/lspd.yaml"
echo ""
echo "Update:    curl -fsSL https://github.com/$REPO/releases/latest/download/install.sh | sh"
echo "Uninstall: curl -fsSL https://github.com/$REPO/releases/latest/download/uninstall.sh | sh"
