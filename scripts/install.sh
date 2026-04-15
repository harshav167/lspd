#!/bin/sh
# lspd installer — downloads pre-built binaries, merges hooks, zero dependencies.
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
if [ -f "$SCRIPT_DIR/session-start.sh" ]; then
    cp "$SCRIPT_DIR/session-start.sh" "$INSTALL_DIR/lspd-session-start"
else
    curl -fsSL "$DOWNLOAD_BASE/session-start.sh" -o "$INSTALL_DIR/lspd-session-start"
fi
if [ -f "$SCRIPT_DIR/droid-launcher.sh" ]; then
    cp "$SCRIPT_DIR/droid-launcher.sh" "$INSTALL_DIR/droid-lsp"
else
    curl -fsSL "$DOWNLOAD_BASE/droid-launcher.sh" -o "$INSTALL_DIR/droid-lsp"
fi
chmod +x "$INSTALL_DIR/lspd" "$INSTALL_DIR/lsp-read-hook" "$INSTALL_DIR/lspd-session-start" "$INSTALL_DIR/droid-lsp"
echo "    Installed to $INSTALL_DIR"

# Promote the wrapper to the regular `droid` command while preserving the real binary.
if [ -x "$INSTALL_DIR/droid" ] && ! grep -q "DROID_LSP_WRAPPER" "$INSTALL_DIR/droid" 2>/dev/null; then
    mv "$INSTALL_DIR/droid" "$INSTALL_DIR/droid.real"
    echo "==> Backed up real droid to $INSTALL_DIR/droid.real"
fi
cp "$INSTALL_DIR/droid-lsp" "$INSTALL_DIR/droid"
chmod +x "$INSTALL_DIR/droid"
echo "==> Installed lspd bootstrap wrapper as $INSTALL_DIR/droid"

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

# Start or discover lspd so the lock file exists before droid starts.
echo "==> Ensuring lspd is ready..."
PORT_FILE="$HOME/.factory/run/lspd/lspd.port"
TMP_PORT="$PORT_FILE.tmp.$$"
TMP_ERR="$PORT_FILE.err.$$"
trap 'rm -f "$TMP_PORT" "$TMP_ERR"' EXIT
mkdir -p "$(dirname "$PORT_FILE")"
nohup "$INSTALL_DIR/lspd" start --foreground --config "$CONFIG_DIR/lspd.yaml" >"$TMP_PORT" 2>"$TMP_ERR" </dev/null &
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
if [ -s "$PORT_FILE" ]; then
    PORT=$(cat "$PORT_FILE")
    echo "    lspd ready on port $PORT"
else
    echo "    WARNING: lspd failed to become ready during install."
    cat "$TMP_ERR" 2>/dev/null || true
fi

echo ""
echo "Done! Run 'droid' normally — lspd starts automatically."
echo ""
echo "Update:    curl -fsSL https://github.com/$REPO/releases/latest/download/install.sh | sh"
echo "Uninstall: curl -fsSL https://github.com/$REPO/releases/latest/download/uninstall.sh | sh"
