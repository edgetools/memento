# Change Request: Page Name as Filename

## Motivation

Obsidian resolves `[[Foo Bar]]` by looking for a file named `Foo Bar.md`. The
current implementation maps page names to slug-style filenames (e.g.,
`foo-bar.md`), which means wikilinks don't resolve when browsing content in
Obsidian. Changing the filename convention to use the page name directly makes
Memento content fully compatible with Obsidian's wikilink resolution.

## Behavioral Changes

### 1. Filename format

**Before:** Page name is lowercased and spaces are replaced with dashes.
`"Crowd Control"` → `crowd-control.md`

**After:** Page name is used directly as the filename, preserving original
casing and spaces. `"Crowd Control"` → `Crowd Control.md`

### 2. Page name validation

Page names must not contain characters that are forbidden in filenames across
Windows, macOS, Linux, iOS, and Android. This matches Obsidian's own filename
restrictions.

**Forbidden characters:** `* " [ ] # ^ | < > : ? / \`

**Forbidden patterns:** Page names must not start with `.`

When a tool call provides a page name containing forbidden characters, the MCP
returns an error describing which characters are not allowed. It does not
silently strip or replace them.

### 3. Case-insensitive lookup

Case-insensitive page name resolution continues to work as before. Since the
filename now preserves the original casing, the in-memory index must handle
case-insensitive matching on lookup (this is already the design — just
confirming it applies to the new filename format too). On case-insensitive
filesystems (Windows, macOS) the OS handles this naturally; on case-sensitive
filesystems (Linux) the MCP's in-memory index is authoritative.

### 4. Existing content migration

Any existing content directories using slug-style filenames will need their
files renamed. This is out of scope for this change — it can be handled by a
one-time migration script or manually. The MCP itself does not need to handle
both formats; it only supports the new format.

## Affected Tools

All tools that create or look up files by page name are affected:

- **`write_page`**: Creates files using the page name directly as filename.
- **`patch_page`**: Looks up files by page name; append/prepend create files
  using the page name directly.
- **`rename_page`**: Renames the file on disk to match the new page name.
- **`delete_page`**: Looks up files by page name.
- **`get_page`**: Looks up files by page name.
- **`search`**: Results reference page names (no change to search behavior,
  but the underlying filename lookup changes).

## Affected Internal Components

- **`pages/names.go`**: The page name → filename mapping changes from slug
  generation to direct use. Validation of forbidden characters is added.
  Case-insensitive lookup logic may need adjustment since filenames now
  preserve casing.
- **`pages/store.go`**: Filesystem operations (read, write, delete, scan) use
  the new filename format.
- **Any tests** that assert on generated filenames or create test fixtures
  with slug-style names.

## What Does NOT Change

- Page names are still the sole identifier used in tool calls and wikilinks.
- Case-insensitive resolution with whitespace normalization still works.
- The `# heading` inside the file still matches the page name.
- The in-memory index, search pipeline, and link graph are unaffected.
- Git auto-commit behavior is unaffected.
- All tool input/output schemas are unaffected.
