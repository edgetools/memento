# Change Request: Markdown Chunking

## Summary

Add a chunking function that splits a page's content into semantically
meaningful chunks with line-range tracking. This is a pure logic layer with
no embedding dependency — it takes page content in and returns chunks out.
Used downstream by the vector index (CR3) to produce per-chunk embeddings.

---

## Changes

### New file: `index/chunk.go`

#### `Chunk` type

```go
type Chunk struct {
    Text      string // the chunk content (title-prefixed)
    StartLine int    // 1-indexed start line in the original page
    EndLine   int    // 1-indexed end line (inclusive)
}
```

#### `ChunkPage(page pages.Page) []Chunk`

Splits a page into chunks using the following strategy:

1. **Primary split: section headings.** Split on lines matching `^#{2,} `
   (i.e. `##`, `###`, etc. — not the `# Title` which is the page heading).
   Each section heading starts a new chunk. The content before the first
   section heading is its own chunk (the "intro" chunk).

2. **Fallback split: paragraph breaks.** If the page has no section headings
   (common for snapshot-accumulated pages), split on double-newline (`\n\n`)
   paragraph boundaries instead.

3. **Title anchoring.** Every chunk is prefixed with the page's `# Title`
   line followed by a newline. This ensures the embedding for any chunk
   carries the page's identity, even for chunks deep in a long page. The
   title prefix does NOT affect `StartLine`/`EndLine` — those refer to the
   original page content.

4. **Minimum size merging.** After splitting, any chunk whose text content
   (excluding the title prefix) is shorter than 50 whitespace-delimited
   tokens is merged with the following chunk. If it's the last chunk, merge
   it with the preceding chunk. A page that is entirely below the minimum
   produces a single chunk.

5. **Single-chunk pages.** Pages whose entire content (after title) falls
   below the minimum or has no split points produce a single chunk covering
   the whole page.

#### Line numbering

`StartLine` and `EndLine` use 1-indexed line numbers matching the page's
raw markdown content (where line 1 is the `# Title` heading). These values
are used by the search pipeline to generate `line` in search results — the
same line-numbering scheme used by `get_page` line ranges.

---

## Behavior Details

### Section heading detection

A section heading is a line that starts with two or more `#` characters
followed by a space: `## Foo`, `### Bar`, etc. The `# Title` (single `#`)
is the page heading and is NOT treated as a split point — it becomes the
title prefix for all chunks.

### Paragraph break detection

A paragraph break is two or more consecutive newlines (`\n\n` or `\n\n\n`,
etc.), matching standard markdown paragraph semantics. Paragraph splitting
only applies when no section headings are found in the page.

### Edge cases

- **Empty body:** a page with only a title and no body produces a single
  chunk containing just the title.
- **Title-only prefix for small pages:** a 20-word page produces one chunk
  with title + body. No splitting occurs.
- **Mixed heading levels:** `##` and `###` are both split points. A `###`
  subsection under a `##` section becomes its own chunk (not merged into
  the parent section).
- **Code blocks:** heading-like lines inside fenced code blocks (`` ``` ``)
  are NOT treated as split points. The chunker must skip fenced code block
  contents when scanning for headings.

---

## Non-Changes

- No changes to `pages.Page` or `pages.Parser`.
- No changes to the BM25 index or search pipeline.
- No embedding or vector logic — that's CR2 and CR3.
- The chunking function is internal to the `index` package. No new MCP
  tools or tool schema changes.
