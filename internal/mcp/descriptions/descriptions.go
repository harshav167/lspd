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

Use this instead of Grep when you need the language server's actual target rather than a textual match. It is
the right tool for "jump to definition", alias chasing, import/re-export resolution, interface method lookup,
and any cross-file navigation where text search can lie.

The response is LLM-facing: every definition includes an absolute path, 1-indexed line/column coordinates, end
positions, and a preview line so you can reason about the target without immediately issuing a follow-up Read.`
	LspReferences = `Find every semantic reference to the symbol at a 1-indexed file position.

Prefer this over text search for impact analysis, rename preparation, and "who calls or uses this?" questions.
The language server filters out comments, strings, and unrelated same-name identifiers, so the result set is
much more accurate than Grep for real code references.

The response includes the total count, a per-file usage summary, and the first 100 concrete reference locations
with preview lines. If the result is truncated, omitted tells you how many references were left out.`
	LspHover = `Fetch the language server's hover view for the symbol at a 1-indexed file position.

Use this when you want the compiler-grade understanding of a symbol before editing it: inferred type, callable
signature, and attached documentation. This is especially useful when names are ambiguous or when the project
leans on inference instead of explicit type annotations.

The response is normalized for models: type_signature and documentation are split into separate fields, markdown
fences are cleaned up, and any returned range is expressed with 1-indexed line/column coordinates.`
	LspWorkspaceSymbol = `Search the workspace-wide symbol index using the language server.

Use this when you know the symbol name or a fuzzy fragment like "fetchUser" but do not know the file yet. It is
the semantic replacement for broad filename or Grep sweeps when you are looking for definitions, not arbitrary
text.

Results are returned as ranked symbol hits with kind, container, path, and 1-indexed line/column coordinates.
The list is capped at 100 symbols; if truncated, omitted tells you how many additional hits exist.`
	LspDocumentSymbol = `Return the hierarchical semantic outline of a single file.

Use this before editing a large file when you need to understand its real symbol tree: classes, methods,
functions, constants, nested members, and their ranges. This is more reliable than scanning raw text because it
comes from the language server's parsed structure.

Each node includes kind, path, 1-indexed line/column coordinates, end positions, and recursive children so the
model can navigate the file structure without flattening important nesting information.`
	LspCodeActions = `List the quick fixes and refactor actions the language server offers for a file range.

Use this when the compiler is already telling you something is wrong or when you suspect the language server can
produce a canonical edit plan faster than hand-authoring one. Typical actions include adding missing imports,
creating missing declarations, renaming typoed symbols, and applying refactors.

The response preserves only the LLM-useful parts: title, kind, preferred status, optional disabled reason,
workspace edit changes expressed as path/range/new_text, and any follow-up command metadata. The daemon never
applies these edits itself.`
	LspRename = `Prepare a semantic cross-file rename at a 1-indexed file position.

Use this for real symbol renames instead of manual search-and-replace. The language server understands imports,
method implementations, interface links, and workspace-wide symbol identity, so it can produce a safer rename
plan than text substitution.

The response includes the inferred old name, requested new name, dry_run flag, file/edit counts, and a
Droid-friendly edit plan expressed as concrete file/range/new_text changes. The daemon does not apply the
changes; Droid should review and apply them through normal editing tools.`
	LspFormat = `Format an entire document using the language server's formatter.

Use this when you want server-accurate formatting rather than guessing or manually rewriting whitespace. This is
especially helpful after multi-line edits, generated code, or cross-language workspaces with project-specific
formatting rules.

The response is intentionally simple for models: path, range (null for whole-document formatting), changed, and
the fully formatted new_text so you can diff or apply it with normal Droid file-editing tools.`
	LspCallHierarchy = `Inspect incoming or outgoing semantic call-hierarchy edges for a callable symbol.

Use this for "who calls this?" or "what does this call?" questions when simple references are too noisy. Call
hierarchy is aware of callable structure, so it is often a better fit than raw references for understanding flow
between functions or methods.

The response includes the prepared root item, the requested direction, and call edges with precise 1-indexed
call-site locations. Incoming edges use from, outgoing edges use to, and call_sites identify where the call
appears in source.`
	LspTypeHierarchy = `Inspect semantic supertypes or subtypes for a type-oriented symbol.

Use this for inheritance, interface implementation, and subtype/supertype exploration when the language server
supports type hierarchy. It is the right tool for "what implements this?" and "what does this extend?" questions.

The response includes the prepared root item, the requested direction, and the resulting related types with
1-indexed line/column coordinates so the model can reason about relationships without raw protocol payloads.`
)
