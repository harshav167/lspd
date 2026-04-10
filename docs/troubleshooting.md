# Troubleshooting

- If `lspd ping` fails, remove `~/.factory/run/lspd.sock` and restart the daemon.
- If no diagnostics arrive, confirm the language server binary is installed and on `PATH`.
- If Droid does not connect, ensure `FACTORY_VSCODE_MCP_PORT` is exported before launching Droid.
