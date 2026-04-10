package descriptions

const (
	GetIdeDiagnostics = `Fetch IDE diagnostics for a file URI using the active language server.

Use this when Droid needs the exact IDE-style diagnostics payload for a file after a read or edit.
The response shape is load-bearing and must remain { diagnostics: [...] } with LSP-style ranges.`
	OpenDiff = `Compatibility stub for Droid's openDiff IDE action.

This daemon has no GUI diff surface, so the tool intentionally returns success without opening anything.`
	CloseDiff = `Compatibility stub for Droid's closeDiff IDE action.

This daemon has no GUI diff surface, so the tool intentionally returns success without closing anything.`
	OpenFile = `Compatibility stub for Droid's openFile IDE action.

This daemon has no GUI file surface, so the tool intentionally returns success without opening anything.`
	LspDefinition = `Resolve the semantic definition of the symbol at a 1-indexed file position.

Prefer this over grep when you need the language server's real symbol target, especially across imports,
methods, interfaces, aliases, or generated workspace state. Returns concrete source locations with previews.`
	LspReferences = `Find semantic references of the symbol at a 1-indexed file position.

Prefer this over text search when you want true code references rather than string matches. Use it for impact
analysis, rename safety checks, and understanding callers/usages across the workspace.`
	LspHover = `Fetch hover information, inferred types, signatures, and documentation for the symbol at a 1-indexed file position.

Use this when you want the language server's understanding of a symbol before editing or refactoring it.`
	LspWorkspaceSymbol = `Search the workspace symbol index using the language server.

Use this to discover likely definitions by symbol name when you do not know the exact file yet.`
	LspDocumentSymbol = `Return the hierarchical outline of a single file from the language server.

Use this to inspect the semantic structure of a file before making edits.`
	LspCodeActions = `Return available quick fixes and refactors for a file range and diagnostics.

Prefer this over inventing imports or fixups manually when the language server can suggest a precise edit plan.`
	LspRename = `Prepare a semantic cross-file rename at a 1-indexed file position.

This returns a WorkspaceEdit describing the rename plan. The daemon never applies edits itself; Droid should
review and apply the returned changes through normal file-editing tools.`
	LspFormat = `Format a document and return the resulting text edits from the language server.

Use this when you want server-accurate formatting rather than guessing style manually.`
	LspCallHierarchy = `Inspect incoming or outgoing call hierarchy edges for a symbol.

Use this to understand who calls a function or what it calls without relying on textual search.`
	LspTypeHierarchy = `Inspect supertypes or subtypes for a type-oriented symbol.

Use this for inheritance and interface relationship analysis when the language server supports type hierarchy.`
)
