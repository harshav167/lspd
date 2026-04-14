# Improve codebase report addendum (`deslop`)

This branch no longer matches the older "everything is still pending" framing.

## What is already true now

- **Startup is no longer an open architecture question.** The accepted production path is plain `droid` plus the hook chain `SessionStart` → `PostToolUse(Read)` → `SessionEnd`. `scripts/droid-launcher.sh` remains a testing/debug path, not an equal production story.
- **Diagnostics now have an explicit boundary.** `policy.DiagnosticsService` owns freshness/presentation behavior, and MCP/socket callers consume that boundary instead of each transport inventing its own semantics.
- **Lifecycle behavior is surfaced, not implied.** Router state and daemon status now expose open documents, supervisor state, config generation, and reload metadata.
- **Runtime/reload honesty is partially implemented already.** `config.ReloadReport` and daemon status distinguish `applied_now` vs `deferred until restart`, so the report should describe reload truthfulness as in-progress hardening rather than untouched future work.

## What is stale in the older report framing

- Treating startup/discovery/wrapper behavior as still undecided is stale on this branch.
- Treating diagnostics as transport-shaped glue rather than a service boundary is stale on this branch.
- Treating runtime/reload status as opaque is stale on this branch; the code already emits structured reload/status truth, even if deeper cleanup may still remain.

## Test alignment added with this branch state

- `test/e2e/diagnostics_boundary_test.go` now covers surfaced-diagnostic dedup as a **session-scoped** contract.
- `test/integration/go_test.go` now asserts router lifecycle state keeps documents registered across resolve/restart boundaries.
- `test/integration/navigation_test.go` now pins path/column normalization for definition, call hierarchy, and type hierarchy responses.
- `test/e2e/runtime_reload_test.go` now pins reload honesty around `applied_now`, `deferred`, and config generation advancement.
