# lspd

`lspd` is a local Go daemon that exposes LSP diagnostics and semantic navigation to Droid over the same MCP seam used by the VS Code integration.

## Commands

- `lspd start`
- `lspd stop`
- `lspd status`
- `lspd reload`
- `lspd ping`

## Install

Run:

```sh
./scripts/install.sh
```

Then point Droid hooks at `examples/settings.json`.
