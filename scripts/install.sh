#!/usr/bin/env sh
set -eu

mkdir -p "${HOME}/.local/bin"

go build -o "${HOME}/.local/bin/lspd" ./cmd/lspd
go build -o "${HOME}/.local/bin/lsp-read-hook" ./cmd/lsp-read-hook
install -m 0755 ./scripts/droid-launcher.sh "${HOME}/.local/bin/droid-lsp"
install -m 0755 ./scripts/session-start.sh "${HOME}/.local/bin/lspd-session-start"
