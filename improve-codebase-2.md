# Improve Codebase Architecture — Workflow Output

## Scope

This is a read-only analysis artifact that completes the improve-codebase workflow for the current `deslop` branch state.

It does not create issues, change code, or assume the older report still matches the repository exactly.

## Current architectural candidates

### 1. Startup contract and hook chain

- **Cluster**: `scripts/install.sh`, `scripts/uninstall.sh`, `scripts/session-start.sh`, `scripts/droid-launcher.sh`, `cmd/lsp-read-hook/main.go`
- **Why they are coupled**: they jointly define the product entrypoint, discovery path, and read-hook lifecycle
- **Dependency category**: Local-substitutable
- **Test impact**: replace setup-path seam tests with boundary tests around the production `droid` path versus the wrapper/debug path

### 2. Diagnostics delivery boundary

- **Cluster**: `internal/policy/policy.go`, `internal/lsp/store/store.go`, `internal/socket/handlers.go`, `internal/mcp/tools/compat/diagnostics.go`, `cmd/lsp-read-hook/main.go`, `internal/daemon/daemon.go`
- **Why they are coupled**: freshness, presentation, dedup, and code-action attachment are one domain concept but still appear across transports
- **Dependency category**: Local-substitutable
- **Test impact**: replace transport-specific seam tests with boundary tests on `DiagnosticsService`

### 3. LSP session runtime

- **Cluster**: `internal/lsp/router/router.go`, `internal/lsp/router/rootdetect.go`, `internal/lsp/client/manager.go`, `internal/lsp/client/requests.go`, `internal/lsp/client/docs.go`, `internal/lsp/supervisor/supervisor.go`, `internal/lsp/store/store.go`
- **Why they are coupled**: callers currently depend on path routing, ensure-open, tracked documents, restart/replay, and diagnostics version semantics as one workflow
- **Dependency category**: Local-substitutable
- **Test impact**: replace router/supervisor coordination tests with boundary tests around one session runtime abstraction

### 4. Navigation request handling

- **Cluster**: `internal/mcp/tools/nav/common.go` plus the nav handlers under `internal/mcp/tools/nav/`
- **Why they are coupled**: the handlers all repeat path normalization, document acquisition, position normalization, manager calls, and response shaping
- **Dependency category**: In-process
- **Test impact**: replace repeated handler seam coverage with tests at one nav boundary

### 5. Runtime ownership and reload honesty

- **Cluster**: `internal/daemon/daemon.go`, `internal/mcp/server.go`, `internal/socket/server.go`, `internal/config/reload.go`, `internal/watcher/watcher.go`
- **Why they are coupled**: daemon status, config generation, reload truth, and transport ownership all surface from the same runtime composition root
- **Dependency category**: In-process
- **Test impact**: replace status/reload seam assertions with explicit runtime-boundary tests

## Recommended candidate to deepen first

The strongest next candidate is **LSP session runtime**.

Why:

- it is the core subsystem every higher-level flow leans on
- it currently leaks too much orchestration to callers
- the current interface is nearly as complex as the implementation
- fixing it makes nav, diagnostics, and daemon code shallower at the same time

## Problem framing for the LSP session runtime

Any deeper interface has to satisfy these constraints:

1. resolve a file path to the correct root and language server session
2. guarantee the document is open and current before protocol operations run
3. survive manager death and replay tracked documents after restart
4. preserve diagnostics visibility and version waiting behavior
5. support the dominant MCP/nav caller path without exposing raw `*client.Manager`

The current caller shape is effectively:

```go
manager, doc, _, err := deps.Router.ResolveDocument(ctx, path)
if err != nil { ... }
result, err := manager.Definition(ctx, paramsUsing(doc.URI))
```

The deepening opportunity is to turn that multi-step choreography into one stable boundary.

## Interface options explored

### Option A — Minimal session API

```go
type Runtime interface {
    Open(ctx context.Context, path string) (Session, error)
}

type Session interface {
    Document() client.Document
    LSP() Requester
    Diagnostics() DiagnosticsView
}
```

- **Strength**: smallest public surface, strongest hiding of orchestration
- **Hidden complexity**: routing, ensure-open, restart/replay, diagnostics waiting
- **Risk**: pure workspace operations may not fit as naturally

### Option B — Flexible runtime API

```go
type Runtime interface {
    DoDocument(ctx context.Context, req DocumentRequest, op DocumentOperation) error
    DoWorkspace(ctx context.Context, req WorkspaceRequest, op WorkspaceOperation) error
    Inspect(ctx context.Context, req InspectRequest) (RuntimeSnapshot, error)
}
```

- **Strength**: handles more use cases, including status/debug flows
- **Hidden complexity**: same runtime internals, but with broader request modeling
- **Risk**: easiest option to drift into a shallow wrapper around current pieces

### Option C — Caller-optimized API

```go
type Runtime interface {
    WithPosition[T any](ctx context.Context, path string, line, character int, fn func(context.Context, PositionScope) (T, error)) (T, error)
    WithDocument[T any](ctx context.Context, path string, fn func(context.Context, DocumentScope) (T, error)) (T, error)
    WithWorkspace[T any](ctx context.Context, path string, fn func(context.Context, WorkspaceScope) (T, error)) (T, error)
}
```

- **Strength**: best fit for `internal/mcp/tools/nav/*`
- **Hidden complexity**: same runtime internals plus 1-based to 0-based normalization
- **Risk**: more optimized for nav than for the full runtime domain

### Option D — Ports/adapters runtime

```go
type Runtime interface {
    Open(ctx context.Context, path string) (Handle, error)
}

type Handle struct {
    Doc client.Document
    RPC RPCPort
}
```

- **Strength**: strongest explicit adapter seam around the subprocess/protocol side
- **Hidden complexity**: runtime orchestration plus process/protocol adapter ownership
- **Risk**: heavier than the repo likely needs if applied too broadly

## Comparison

- **Best for depth**: Option A
- **Best for current caller ergonomics**: Option C
- **Best for breadth**: Option B
- **Best for explicit adapter isolation**: Option D

## Recommendation

Use a **hybrid of Option A and Option C**.

That means:

- keep the public runtime small
- expose caller-friendly entry points like `WithDocument` and `WithPosition`
- do not return raw `*client.Manager`
- keep restart, replay, root resolution, ensure-open, and diagnostic waiting fully inside the boundary

A good target shape is:

```go
type Runtime interface {
    WithDocument[T any](ctx context.Context, path string, fn func(context.Context, DocumentScope) (T, error)) (T, error)
    WithPosition[T any](ctx context.Context, path string, line, character int, fn func(context.Context, PositionScope) (T, error)) (T, error)
    States() []State
    Close(ctx context.Context) error
}
```

This preserves the strongest property of the minimal design while matching the dominant MCP/nav access pattern.

## Testing impact of the recommendation

If this runtime boundary is introduced, the most obvious test replacements are:

- replace `TestSupervisorRestartsAndReregistersDocuments` with a runtime-boundary restart/replay test
- replace `TestRouterResolveDocumentGuaranteesOpenAndChange` with a runtime-boundary open/change test

Tests likely to remain:

- `TestGoplsDiagnostics` as a real-server integration smoke test
- `TestNavigationContracts` as an end-to-end MCP contract test
- `internal/mcp/tools/nav/common_test.go` for pure utility logic

## Suggested implementation order after this analysis

1. finish the startup contract cleanup
2. keep diagnostics behind `DiagnosticsService`
3. introduce the hybrid runtime boundary
4. move nav handlers onto that boundary
5. continue daemon/runtime ownership cleanup after callers stop touching raw router/manager state

## Bottom line

The repo’s biggest remaining architectural friction is not missing helpers. It is that the LSP runtime is still exposed as cooperating parts instead of one deep module.

The best next move is to deepen that runtime behind a small, caller-friendly boundary and let nav, diagnostics, and daemon code depend on it instead of reproducing its choreography.
