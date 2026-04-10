# Solo Implementation Contract

**If you are an agent building `lspd` from `PLAN.md`, read this document in full before writing any code or doing any planning. This is your scope lock. It overrides any assumption you might otherwise make about scope.**

---

## The contract

You are building the `lspd` daemon end-to-end in this repo (`/Users/harsha/.factory/droid-lsp/`). Your deliverable is the full Go daemon described in `PLAN.md`, not a subset of it. No MVP. No "solid foundation". No "phase 1 only". No placeholder implementations of any tool. No "I'll do the rest later". No "I'll ship a skeleton and someone else can fill it in." You are the someone else.

The plan has roughly 6,800 lines of Go code across 39 work units listed in `PLAN.md` §17. **All 39 ship.** That is the job.

You have one session to do it. Do not stop at intermediate checkpoints to report progress and wait for the user to tell you to continue. The user has already told you to continue by handing you this document.

---

## What "done" means (definition of done)

All of these must be true before you report the work complete:

1. Every directory and file listed in `PLAN.md` §15 exists and contains real code (not `TODO`, not `// placeholder`, not empty stubs, not `panic("not implemented")`).
2. Every work unit listed in `PLAN.md` §17 has been implemented. All 39 of them.
3. Every MCP tool listed in `PLAN.md` §6.2 and §6.3 has a real handler wired into the MCP server. Tier 1 = 4 tools (3 are intentionally stub no-ops per the plan; `getIdeDiagnostics` is a real handler). Tier 2 = 10 tools, all real handlers.
4. Every success criterion in `PLAN.md` §1.3 is satisfied.
5. `go build ./...` succeeds with no errors.
6. `go vet ./...` is clean.
7. `go test -race ./...` passes.
8. The MCP contract test at `test/e2e/mcp_contract_test.go` passes: spin up `lspd` in-process, connect an MCP client, call every registered tool, assert responses match the shapes in `PLAN.md` Appendix B.
9. Policy golden tests at `test/golden/policy/` pass.
10. Integration tests for at least TypeScript, Python, and Go pass against the real language servers (`typescript-language-server`, `pyright-langserver`, `gopls`). Rust and C/C++ tests should exist but may be gated behind a build tag if the language servers are not installed.
11. `lspd start --foreground` runs the daemon cleanly. `lspd status` reports per-language state. `lspd stop` shuts down cleanly.
12. The final commit on `main` names which work units shipped in its message body.

---

## What "NOT done" means

These states are all `not done`, no matter how productive they feel:

- "Foundation sub-pass complete." → Keep going. Foundation is step zero.
- "Milestone 1 complete, reporting progress." → Keep going to M2, M3, M4 in the same session.
- "The daemon compiles." → Compilation is necessary, not sufficient. Verify behavior.
- "Unit tests pass." → Also need golden tests, integration tests, MCP contract test, e2e.
- "I used placeholders for the tier-2 tools." → Implement them. All ten.
- "I skipped rust-analyzer because it's heavy." → Implement it. Keep `warmup: false` per the config.
- "I wrote the structure and left TODOs inside the functions." → Implement the functions.
- "Here's a simplified version." → The plan is already the simplified version. Implement the plan.
- "I shipped the parts that matter most." → All 39 matter equally. Ship them all.

If you stop for any reason other than completing the full scope, your report must start with the literal phrase: **"I did not finish. Here is exactly what's missing:"** followed by a list of every unimplemented work unit by number and title. Do not frame partial work as "a good starting point", "foundation in place", "ready for the next phase", or "phase 1 complete." Those framings are forbidden — they are dishonest reporting.

---

## Forbidden rationalizations

If you catch yourself thinking or writing any of these phrases, STOP. Re-read `PLAN.md` §1 Scope Lock and this document. Then resume full-scope execution.

- "implement [the plan] likely doesn't mean the entire project in one go"
- "I could aim for a solid foundation that's partially functional"
- "minimal but substantial architecture"
- "placeholders for tools"
- "I'll skip [LSP client integration / rust-analyzer / the MCP contract test / the golden tests] for now"
- "this is phase 1, the rest can come later"
- "10 to 12 test files is probably enough"
- "monolithic approach with fewer files to keep things manageable"
- "I'll keep things simple and summarize what's still needed"
- "I'll scaffold the structure and TODO the bodies"
- "the user probably didn't mean the entire thing"
- "I'll do Milestone 1 now and Milestone 2–4 in the next session"

These phrases are scope drift. None of them are allowed unless the user explicitly used words like "simple", "minimal", "subset", "MVP", "prototype", "sketch", or "skeleton" in the task they gave you. "Implement PLAN.md" means implement PLAN.md in full.

---

## The anti-rat-hole rule

Do not spend more than 5–10 tool calls on API archaeology, dependency research, or abstract analysis before writing code. When you encounter an unfamiliar library like `mcp-go` or `go.lsp.dev/protocol`:

1. Look for an `examples/` directory. Skim one relevant example.
2. Copy the pattern.
3. Move on.

Rat-holing is when you read every file in a library's source tree trying to build a perfect mental model before writing any code. It feels productive but produces no deliverable. Ship first, refine later. If you notice yourself reading `mcp-go/server/*.go` looking for the "right" API to use and you've already spent 10 tool calls doing it, you are rat-holing. Stop, open `mcp-go/examples/typed_tools/main.go`, copy the structure, and build.

The second-most-common failure mode after shortcut-taking is rat-holing. Do not do either.

---

## The honesty rule

- Never say "I implemented X" unless you actually wrote the code for X and it's in the repo.
- Never say "the foundation is complete" as if it were a milestone worth stopping at.
- Never say "a partial functional version" as an acceptable delivery state unless the user literally authorized it.
- If you stop before full scope ships, your message must start with: **"I did not finish. Here is exactly what's missing:"** followed by a numbered list of unimplemented work units from §17.

Distinguish clearly between:
- **Planned**: you intend to do it.
- **In progress**: you are currently doing it.
- **Done**: the code exists in the repo, commits are made, tests pass.
- **Verified**: you have run the relevant verification commands and seen them succeed.

Do not claim "done" when the actual state is "planned" or "in progress". Do not claim "verified" when you only claimed "done" without running verification.

---

## Reference material

- **`/Users/harsha/.factory/droid-lsp/PLAN.md`** — the full plan. Read it in full before starting any implementation. All ~2,800 lines.
- **`/Users/harsha/.factory/droid-lsp/mcp-go/`** — a cloned reference of the `github.com/mark3labs/mcp-go` repo. Skim `examples/typed_tools/main.go`, `examples/structured_input_and_output/`, `server/streamable_http.go`, and `mcp/tools.go` once each, then use the APIs. Do not edit any file in this folder — it is read-only reference.
- **`/Users/harsha/.factory/droid-lsp/serena/`** — **IGNORE.** This folder is out of scope per the user's explicit instruction. Do not read files under it. Do not reference it in your code.
- **`/Users/harsha/.factory/droid-source-code/src/`** — Droid's source code. Specifically:
  - `services/VSCodeIdeClient.ts` — the MCP client that `lspd` impersonates. Read this to verify your MCP tool shapes match what Droid expects.
  - `services/IdeContextManager.ts` — where Droid reads `FACTORY_VSCODE_MCP_PORT`.
  - `tools/executors/client/file-tools/diagnostics-utils.ts` — `fetchDiagnostics` / `compareDiagnostics` / `formatDiagnosticsForSystemReminder`, the pipeline `lspd` feeds.
  - `tools/executors/client/edit-cli.ts`, `create-cli.ts`, `apply-patch-cli.ts` — the call sites for the diagnostic pipeline.
  - `services/HookService.ts` — the hook execution path for the Read hook.
  - `core/ToolExecutor.ts` — the PostToolUse dispatch path where `additionalContext` gets injected into the conversation.

---

## Architecture decisions already made (do NOT re-litigate)

- Language: Go. Not Python, not Rust, not Node.
- MCP library: `github.com/mark3labs/mcp-go` via a `replace` directive in `go.mod` pointing at the local `../mcp-go` clone.
- LSP library: `go.lsp.dev/jsonrpc2` + `go.lsp.dev/protocol`.
- Transport: StreamableHTTP (matches Droid's `VSCodeIdeClient`).
- Integration seam: `FACTORY_VSCODE_MCP_PORT` environment variable. `lspd` writes the port to `~/.factory/run/lspd.port` at startup; the launcher wrapper exports it before `exec droid`.
- Read-gap closure: PostToolUse hook → `lsp-read-hook` binary → Unix socket → daemon `peek` op → `hookSpecificOutput.additionalContext`.
- Daemonization: Go re-exec sentinel pattern (not raw `os.Fork` — the Go runtime breaks under fork-in-multithreaded-process).
- Serena is NOT part of this build.
- No new MCP tools beyond the 14 listed in `PLAN.md` (4 tier-1 + 10 tier-2). Do not invent `lspApplyCodeAction` or any other tool.

---

## The resume rule

If your context window fills up before the build is complete:

1. Commit your current work with a clear message listing which work units are done and which are not.
2. Write a one-paragraph summary of exactly where you left off to a file at `/Users/harsha/.factory/droid-lsp/.resume-state.md`.
3. End your session.

Do NOT declare the project done just because you're running low on context. Do NOT invent shortcuts to finish faster. Do NOT skip the verification steps because you're tired. Either ship the full scope, or ship the partial scope honestly with a `"I did not finish"` preamble.

---

## TL;DR

Read `PLAN.md`. Implement all 39 work units. Ship real code for every file in §15. Pass every test. Run every verification. Commit. Do not stop early. Do not take shortcuts. Do not rat-hole. Do not substitute "simplified" for "full." The plan is the pruned spec. Your job is to implement the pruned spec, not to prune it again.
