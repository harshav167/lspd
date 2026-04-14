# Improve Codebase Architecture — Implementation Plan

## Purpose

This document turns the architecture report into an execution plan that matches the implemented codebase as it exists now.

This is not a greenfield redesign. It is a plan for improving the current repository without losing the product behavior that already works in production:

- Go diagnostics work through the IDE seam
- Read-hook diagnostics work
- install / update / uninstall exist
- plain `droid` startup works with lock-file auto-discovery

The goal is to improve the codebase while **freezing the production route that currently works** and avoiding another round of product-shape churn.

---

## Ground truth about the current codebase

Before planning changes, it is important to state what the repo already is.

### 1. The current production route is `droid` + install script + SessionStart hook

The supported path today is:

1. install binaries with `install.sh`
2. merge hooks into `~/.factory/settings.json`
3. enable `ideAutoConnect`
4. launch `droid`
5. SessionStart hook starts `lspd`
6. `lspd` writes an IDE lock file
7. Droid auto-discovers it and connects through the IDE seam

This path already works. That matters because it means the startup path should now be treated as a **stability boundary**, not as an open design question.

### 2. The codebase has already drifted beyond the report

The architecture report assumed one MCP server. The current codebase has already moved further than that in the worktree:

- daemon owns both `MCP` and `NavMCP`
- config includes both `Port` and `NavPort`
- MCP server logic has already started moving toward a split surface

So the right question is not “should we invent a split?” The right question is:

> do we finish and stabilize the split, or do we consciously back it out and simplify back to one server?

### 3. Some live experiments are intentionally out of scope for this plan

The current worktree and recent discussion touched two topics that are **not part of this plan**:

- TypeScript explicit `getIdeDiagnostics` cold-start behavior
- MCP surface splitting / nav endpoint experiments

Those are parked. They may continue on a separate branch, but they are not part of the scoped implementation plan here.

---

## Planning principles

These principles drive the plan below.

### Freeze the working production route first

Do not keep redesigning startup, discovery, wrappers, or install flow while deeper refactors are happening.

That means:

- plain `droid` is the supported production path
- the install script remains the distribution path
- wrapper usage is explicitly demoted to testing / advanced / isolated debugging
- startup behavior must stay boring while deeper internal refactors happen

### Product correctness before elegance

The first phase should fix the product behavior that is already supposed to be supported:

- one production startup path
- one testing / advanced path
- one documented story across scripts and README

Only after that should the codebase be deepened.

### Deep modules, not helper sprawl

The report’s core thesis is still right:

> the codebase should hide choreography behind smaller public boundaries

That means the implementation work should create service boundaries, not just shared helper functions.

### Preserve already-proven behavior

Any refactor in diagnostics or session lifecycle must preserve:

- router resolution
- document tracking
- restart-and-reregister behavior
- read-hook behavior
- policy filtering behavior

If a new abstraction makes those guarantees harder to see or test, it is the wrong abstraction.

---

## Priority 1 — freeze one supported startup story

### Problem statement

The implemented codebase drifted between multiple startup stories:

- global install + plain `droid`
- wrapper-based `droid-lsp`
- lock-file-only discovery
- install-time startup vs SessionStart startup
- uncertainty around how the Read hook is installed and invoked

That ambiguity is the highest-value issue to remove because it affects every user immediately and poisons later architecture work.

### Supported product contract

Freeze the supported product contract as:

- **Production**: install once, then run plain `droid`
- **Testing / advanced**: use `droid-lsp` only for isolated local build/debug workflows

The wrapper remains, but it is not part of the main product story.

### Why this must be first

This comes before every deeper refactor for one reason:

> if the startup story is unclear, every later change gets judged through a moving target.

Right now the repository still carries traces of multiple product narratives:

- “plain `droid` is the real path”
- “the wrapper is the real path”
- “install starts the daemon”
- “SessionStart starts the daemon”
- “the Read hook is just an implementation detail”

That ambiguity is more damaging than any single internal code smell because it affects every user immediately.

So the first phase is not just cleanup. It is a contract freeze:

- one production path
- one testing path
- one explanation of how startup works
- one explanation of where the Read hook fits

Once that contract is frozen, architecture work underneath it becomes much easier to reason about.

### Proposed implementation

### 1. Make the startup path explicit

The production path must be documented and implemented as one coherent route:

- `install.sh`
- `uninstall.sh`
- `session-start.sh`
- `cmd/lsp-read-hook/main.go`
- `README.md`

The wrapper must stop acting like an equal first-class production path in docs or behavior.

### 2. Keep install/uninstall narrow and boring

Install should only:

1. download binaries
2. write config if absent
3. merge hooks idempotently
4. install and wire the hook chain cleanly:
   - `SessionStart` starts `lspd`
   - `PostToolUse(Read)` runs `lsp-read-hook`
   - `SessionEnd` stops or cleans up `lspd`
5. enable the minimum settings needed for discovery

Uninstall should only:

1. stop/remove lspd assets
2. remove lspd-specific hooks
3. leave unrelated Droid state alone

### 3. Clarify wrapper semantics

The wrapper should be explicitly documented as:

- branch-local testing
- advanced / isolated debugging
- not the main production story

### 4. Remove contradictory product language

README and scripts must stop implying two equally supported ways to run the product.

### Concrete proposed fixes

#### `scripts/install.sh`

- keep it focused on distribution + config merge
- make sure it installs both `lspd` and `lsp-read-hook`
- make sure it preserves existing config where appropriate
- make sure it does not silently become a second startup strategy

#### `scripts/uninstall.sh`

- keep it focused on lspd-owned cleanup only
- make sure it cleanly removes lspd-specific hooks
- do not let it remove unrelated Droid setup

#### `scripts/session-start.sh`

- make its job explicit: ensure the daemon is available for production `droid`
- do not let it become an installer, a wrapper, or a second control plane
- make its contract obvious in code comments

#### hook chain

- treat the full hook chain as part of the startup contract, not just one helper binary
- make `SessionStart`, `PostToolUse(Read)`, and `SessionEnd` explicit in docs and script behavior
- ensure install/docs/wiring all reflect the real command paths, socket usage, and cleanup responsibilities
- make it obvious that reads use the Read hook path while writes use the IDE seam

#### `scripts/droid-launcher.sh`

- keep it available for branch-local testing
- explicitly mark it as non-production
- remove any language that makes it sound like the normal user path

#### `README.md`

- move all startup language to one coherent production story
- explicitly describe the wrapper as testing/advanced only
- explicitly explain the read-hook chain

### Why this plan is better

Because it stabilizes the user-facing contract first. Once users have one clear story, deeper refactors can happen without constantly dragging setup confusion back into every design discussion.

### Files

- `scripts/install.sh`
- `scripts/uninstall.sh`
- `scripts/session-start.sh`
- `scripts/droid-launcher.sh`
- `cmd/lsp-read-hook/main.go`
- `README.md`

### Definition of done

- one production path
- one testing / advanced path
- no contradictory startup behavior across docs, scripts, and config
- wrapper is clearly secondary, not ambiguous
- the full hook chain (`SessionStart`, `PostToolUse(Read)`, `SessionEnd`) is explicitly part of the supported startup model

---

## Priority 3 — deepen the diagnostics pipeline

## Problem statement

This remains the best deep-module opportunity in the repo.

The same diagnostics-domain workflow still appears through:

- IDE compat handler
- socket drain/peek path
- daemon helpers
- policy engine
- read-hook bridge

That is too much choreography leaking upward.

## Proposed design

Introduce a diagnostics service boundary with a request model that captures the real semantics the repo already has.

### Service shape

Something in the shape of:

- `Fetch(ctx, req) (result, error)`
- `ResetSession(sessionID)`

With explicit request fields for:

- path / URI target
- freshness mode:
  - peek
  - drain
  - best-effort-now
- presentation mode:
  - raw
  - surfaced
- session ID

### What moves into the service

- path normalization
- URI conversion
- manager resolution
- ensure-open
- wait/fallback logic
- policy application
- dedup
- code-action attachment

### What stays outside

- MCP request parsing
- socket protocol parsing
- hook output formatting

### Why this plan is better

Because it lets each transport become thin and stable, and it makes diagnostics behavior testable as one domain concept instead of several seam tests.

### Files

- `internal/mcp/tools/compat/diagnostics.go`
- `internal/socket/handlers.go`
- `internal/socket/protocol.go`
- `internal/daemon/daemon.go`
- `internal/policy/policy.go`
- `internal/policy/dedup.go`
- `internal/lsp/store/store.go`
- `cmd/lsp-read-hook/main.go`

### Definition of done

- diagnostics-domain behavior lives behind one boundary
- transports stop rebuilding it independently

---

## Priority 4 — unify LSP session lifecycle

## Problem statement

The core LSP subsystem is still split across:

- router
- manager
- supervisor
- store

and callers still know too much about the sequence needed to use them.

## Proposed design

Introduce a deeper session boundary.

### Proposed shape

A service that acquires a healthy session for a path and owns:

- root detection
- manager creation/reuse
- health/restart
- document-open guarantees
- replay after restart

Handlers and diagnostics code then consume that boundary instead of coordinating the pieces themselves.

### Why this plan is better

Because it turns the actual core subsystem into a true abstraction rather than a set of cooperating parts every caller must understand.

This is the most strategically important internal cleanup after diagnostics.

### Files

- `internal/lsp/router/router.go`
- `internal/lsp/router/rootdetect.go`
- `internal/lsp/client/manager.go`
- `internal/lsp/client/handshake.go`
- `internal/lsp/client/docs.go`
- `internal/lsp/client/requests.go`
- `internal/lsp/client/notify.go`
- `internal/lsp/supervisor/supervisor.go`
- `internal/lsp/store/store.go`

### Definition of done

- handlers and diagnostics flows consume a stable session abstraction
- restart-and-reregister is guaranteed inside the boundary

---

## Priority 5 — nav service extraction

## Problem statement

The nav handlers still duplicate:

- resolve manager
- ensure open
- convert indices
- call protocol
- normalize result

## Proposed design

Extract a navigation service with three families:

- cursor / position
- document
- workspace

Handlers become request adapters only.

### Why this plan is better

It removes duplicate handler glue without flattening everything into generic helpers.

It also composes naturally with the deeper session boundary from Priority 4.

### Files

- `internal/mcp/tools/nav/common.go`
- `internal/mcp/tools/nav/definition.go`
- `internal/mcp/tools/nav/references.go`
- `internal/mcp/tools/nav/hover.go`
- `internal/mcp/tools/nav/document_symbol.go`
- `internal/mcp/tools/nav/workspace_symbol.go`
- `internal/mcp/tools/nav/code_actions.go`
- `internal/mcp/tools/nav/rename.go`
- `internal/mcp/tools/nav/format.go`
- `internal/mcp/tools/nav/call_hierarchy.go`
- `internal/mcp/tools/nav/type_hierarchy.go`

### Definition of done

- nav handlers are thin
- normalization logic lives in one place

---

## Priority 6 — runtime and reload honesty

## Problem statement

The report is right that runtime and config truth are still split.

Now that the repo also has setup/startup semantics and dual transport concerns, this matters even more.

## Proposed work

### Runtime boundary cleanup

Reduce `daemon.App` so it is no longer a giant composition root plus orchestration spillover surface.

### Reload honesty

Make reload result explicit:

- applied now
- accepted but deferred

### Why this plan is better

Because runtime behavior becomes understandable from one place instead of several partial truths.

### Files

- `internal/daemon/daemon.go`
- `internal/daemon/signals.go`
- `internal/socket/server.go`
- `internal/mcp/server.go`
- `internal/watcher/watcher.go`
- `internal/config/reload.go`

### Definition of done

- status and reload behavior tell the truth about what changed and what did not

---

## Phase 5 — testing and documentation

## Tests

### Priorities

1. diagnostics service boundary tests
2. runtime package unit tests
3. nav service tests

### Files

- `test/e2e/mcp_contract_test.go`
- `test/integration/*`
- runtime and MCP packages with weak current unit coverage

## Docs

### Report addendum

Add a short addendum to `improve-codebase-report.md` explaining:

- latest `main` now has a dual-MCP-server direction in local work
- the report’s one-server assumption is stale
- the deeper recommendations still hold

### README alignment

Align README to:

- frozen production startup path
- explicit testing/wrapper path
- explicit TS known-issue section

---

## Recommended execution order

1. Freeze production startup route
2. Deepen diagnostics pipeline
3. Unify LSP session lifecycle
4. Extract nav service
5. Clean up runtime ownership
6. Make reload honest
7. Expand tests
8. Update docs + report addendum

---

## Why this sequence is better

This order is better than starting with deep refactors immediately because:

- it fixes the user-visible uncertainty first
- it locks down the product route before internal abstractions move
- it gives later refactors a stable behavioral target
- it prevents architecture work from happening on top of startup confusion

If we refactor internals before freezing product behavior, we risk improving the code while making the product less understandable.

If we freeze product behavior first, the deeper refactors can proceed under stable expectations.

---

## Concrete proposed branch / commit plan

### Branch

- `deslop`

### Commit sequence

1. `plan: freeze production startup route and doc contract`
2. `refactor: extract diagnostics service boundary`
3. `refactor: introduce LSP session boundary`
4. `refactor: extract navigation service`
5. `refactor: clean up daemon runtime ownership`
6. `refactor: make reload semantics explicit`
7. `test: expand diagnostics, runtime, and nav coverage`
8. `docs: update README and architecture report addendum`

---

## Approval request

This plan intentionally does **not** start by editing a dozen files at once.

It first freezes the product contract, then fixes the biggest visible bug, then deepens the architecture.

If you approve this direction, the first implementation pass should start with:

1. production startup route freeze
2. diagnostics pipeline boundary

Everything else becomes cleaner once those two are stable.
