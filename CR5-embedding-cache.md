# Change Request: Embedding Cache

## Summary

Persist chunk embeddings to a sidecar cache file so that startup doesn't
require re-embedding every page. On startup, only pages whose content has
changed since the last run are re-embedded. The cache includes a model
identifier so it auto-invalidates when the embedded model changes.

---

## Changes

### New file: `index/cache.go`

#### Cache file format

The cache file uses Go's `encoding/gob` binary format, stored at
`.memento-vectors` in the content directory. Gob is stdlib, trivial to
implement (`gob.Encode`/`gob.Decode` on the struct), compact, and fast.
Since the cache is derived data (safe to delete, rebuilds on next startup),
human readability is not a concern.

The cache contains:

```
Header:
  model_id       string   // identifies the model (e.g. "all-MiniLM-L6-v2")
  sentex_version string   // go-sentex library version (e.g. "v0.1.3")
  dimensions     int      // vector dimensionality (e.g. 384)
  version        int      // cache format version (for future migrations)

Per-page entries:
  page_name    string
  content_hash string   // SHA-256 hex of the page's raw markdown content
  chunks: []
    start_line int
    end_line   int
    vector     []float32
```

#### `SaveCache(path string, entries []CacheEntry, modelID, sentexVersion string, dims int) error`

Writes the full cache to disk atomically (write to temp file, then rename).
Atomic writes prevent corruption if memento is killed mid-write. Every
save is a full rewrite — the cache is small (even 10K chunks at 384 dims
is ~15MB), so simple wins over incremental bookkeeping.

#### `LoadCache(path string, modelID, sentexVersion string, dims int) ([]CacheEntry, error)`

Reads the cache from disk. Returns an error (or empty result) if:
- The file doesn't exist (first run — not an error, just empty)
- The `model_id` doesn't match (model changed — cache is stale)
- The `sentex_version` doesn't match (library update may have changed
  preprocessing or output — cache is stale)
- The `dimensions` don't match
- The `version` is unsupported
- The file is corrupt

#### `CacheEntry` type

```go
type CacheEntry struct {
    PageName    string
    ContentHash string
    Chunks      []CachedChunk
}

type CachedChunk struct {
    StartLine int
    EndLine   int
    Vector    []float32
}
```

### Modified: startup sequence (integration point for CR7)

The intended startup flow (wired in CR7) is:

1. `store.Scan()` all pages
2. Build BM25 index (existing)
3. Load embedding cache from sidecar file
4. For each page:
   - Compute content hash
   - If hash matches cache entry: load cached vectors into vector index
   - If hash doesn't match (or no cache entry): embed chunks, add to vector index
5. Save updated cache to sidecar file

### Modified: write operations

After a write operation updates the index (`idx.Add`) and the vector index
re-embeds the page, the cache is saved immediately (write-through). The
sidecar file is small (even 10K chunks at 384 dims is ~15MB) and atomic
writes are fast. This ensures the cache survives unclean shutdowns (SIGKILL)
without requiring a full re-embedding on next startup.

---

## Behavior Details

### Content hashing

The hash is computed over the raw markdown file content (the same bytes
that `pages.Parse` receives). This means any edit — even whitespace-only
changes — triggers re-embedding. This is intentional: the chunking
boundaries might shift even for whitespace changes, and re-embedding a
single page is fast.

### Cache location

Default: `.memento-vectors` in the content directory. This keeps the cache
co-located with the content it describes. The file should be added to
`.gitignore` since it's machine-local derived data.

### First run

When no cache file exists, all pages are embedded from scratch. The cache
is written after the initial embedding pass. Subsequent startups are fast.

### Model change

When the embedded model changes (e.g. a memento version bump), the
`model_id` in the cache header won't match. The entire cache is discarded
and rebuilt. This is the correct behavior — vectors from different models
are incompatible.

### Deleted pages

Pages that exist in the cache but not on disk are simply not loaded. The
next cache save will omit them, effectively garbage-collecting stale
entries.

### Multiple memento instances on the same brain

When two memento instances share a content directory, both will read and
write `.memento-vectors`. This is safe:

- Vectors are deterministic for `(content_hash, model_id, sentex_version)`.
  Two instances computing the same entry produce bit-identical output, so
  reading a peer's cache entry is equivalent to recomputing it locally.
- Atomic rename prevents partial-write corruption.
- Concurrent saves race on last-write-wins. The "loser" may temporarily
  overwrite an entry another instance had. But because both instances
  watch the same `.md` files (CR6), they converge on the same entry set
  within one debounce window and one of them will flush again.
- Worst case is a one-time redundant re-embed if an instance's entry was
  clobbered before it re-reads the cache. Not a correctness issue.

The cache is derived data — even complete loss self-heals on next startup.

---

## Non-Changes

- No changes to MCP tool schemas or output format.
- No changes to the search pipeline.
- No changes to BM25 or graph indexes (they don't use the cache).
