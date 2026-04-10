package descriptions

const (
	GetIdeDiagnostics  = "Fetch IDE diagnostics for a file URI. Returns the exact JSON shape Droid expects: { diagnostics: [...] }."
	OpenDiff           = "No-op compatibility stub for Droid's openDiff call. Always returns ok."
	CloseDiff          = "No-op compatibility stub for Droid's closeDiff call. Always returns ok."
	OpenFile           = "No-op compatibility stub for Droid's openFile call. Always returns ok."
	LspDefinition      = "Resolve the semantic definition of the symbol at a 1-indexed file position."
	LspReferences      = "Find semantic references of the symbol at a 1-indexed file position."
	LspHover           = "Fetch hover information, types, and docs for the symbol at a 1-indexed file position."
	LspWorkspaceSymbol = "Search the workspace symbol index using the language server."
	LspDocumentSymbol  = "Return the hierarchical outline of a single file."
	LspCodeActions     = "Return available quick fixes and refactors for a file range and diagnostics."
	LspRename          = "Prepare a semantic cross-file rename. Returns a WorkspaceEdit and never applies edits itself."
	LspFormat          = "Format a document and return the resulting text edits."
	LspCallHierarchy   = "Inspect incoming or outgoing call hierarchy edges for a symbol."
	LspTypeHierarchy   = "Inspect supertypes or subtypes for a symbol."
)
