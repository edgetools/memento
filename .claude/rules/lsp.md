### Code Intelligence

**IMPORTANT** prefer LSP over Grep/Read/Search for code navigation — it's faster, precise, and avoids reading entire files.

LSP commands:
- `workspaceSymbol` to find where something is defined
- `findReferences` to see all usages across the codebase
- `goToDefinition` / `goToImplementation` to jump to source
- `hover` for type info without reading the file

Use Grep/Read/Search only when LSP isn't available or for text/pattern searches (comments, strings, config).

After writing or editing code, check LSP diagnostics and fix errors before proceeding.
