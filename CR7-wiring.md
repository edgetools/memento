# Change Request: Wiring

## Summary

Connect all new components in `main.go` and the tool registration layer.
This is the integration CR that brings vector search, embedding cache, and
filesystem watching together into the running server.

---

## Changes

### Modified: `main.go`

#### Startup sequence

```
1. Parse flags (existing)
2. Resolve content directory (existing)
3. Verify git repo if --auto-commit (existing)
4. Create store (existing)
5. Load embedding model (NEW)
6. Create composite index with model (MODIFIED — pass model to NewIndex)
7. Load embedding cache from sidecar file (NEW)
8. Scan pages and build index (MODIFIED):
   a. For each page from store.Scan():
      - idx.Add(page) — builds BM25 + graph (existing)
      - If page has cached vectors (hash matches): load vectors into vector index
      - If page needs embedding (new or changed): queue for embedding
   b. Batch-embed all queued pages
   c. Save updated cache
9. Start filesystem watcher (NEW)
10. Register MCP tools (existing)
11. Serve stdio (existing)
12. On shutdown: close watcher (NEW)
```

#### Model loading

```go
model, err := embed.LoadModel()
if err != nil {
    log.Fatalf("failed to load embedding model: %v", err)
}
```

The model load is fatal — if the embedded model can't be loaded, the binary
is broken. There's no "graceful degradation" for a corrupted binary.

#### Watcher lifecycle

```go
w, err := watcher.NewWatcher(absDir, store, idx)
if err != nil {
    log.Printf("warning: filesystem watching unavailable: %v", err)
} else {
    w.Start()
    defer w.Close()
}
```

Watcher failure is a warning, not fatal. The server works fine without it —
the index is just stale for external changes until restart.

#### Cache persistence on writes

When a write operation triggers `idx.Add` and the vector index re-embeds
a page, the cache should be updated (write-through). This can be wired
either by:
- Having `Index.Add` return a signal that embeddings changed, and the
  tool handler saves the cache
- Having the cache save happen inside `Index.Add` (if the cache path is
  known to the index)

The implementer should choose whichever approach keeps the dependencies
clean. The important thing is that the cache stays current after writes.

### Modified: `tools/tools.go`

#### `Register` and `RegisterAutoCommit`

Both functions may need to accept additional parameters (the watcher, the
cache path) if cache write-through is handled at the tool layer. The
implementer should determine the cleanest integration point.

---

## Behavior Details

### First-run experience

On first run with no cache file:
1. Model loads from the embedded binary (instant)
2. All pages are scanned and BM25-indexed (fast, existing behavior)
3. All pages are chunked and embedded (takes a few seconds for ~50-100 pages)
4. Cache is written to `.memento-vectors`
5. Subsequent startups only re-embed changed pages

### Graceful degradation

- **Watcher fails:** log warning, continue. Index is stale for external
  changes until restart.
- **Cache file missing or corrupt:** full re-embedding on startup.
  Slightly slower first start, but self-healing.
- **Cache file in `.gitignore`:** implementer should note in output or
  docs that `.memento-vectors` should be gitignored. The cache is
  machine-local derived data.

### Shutdown

`defer w.Close()` ensures the watcher goroutine is stopped cleanly on
normal shutdown. On SIGKILL, the goroutine dies with the process — no
cleanup needed. The cache was already saved write-through on each mutation.

---

## Non-Changes

- No changes to MCP tool schemas or output format.
- No new CLI flags (the model is embedded, the cache path is derived,
  the watcher is always-on).
- No changes to search ranking or pipeline logic — that was all CR4.

---

## Testing

This CR is primarily integration wiring. Tests should cover:
- Startup with no cache file (first run)
- Startup with a valid cache file (cache hit)
- Startup with a stale cache file (model ID mismatch)
- Watcher detects external file creation and index updates
- Watcher detects external file deletion and index updates
- End-to-end: write a page via MCP, verify search finds it, modify the
  file externally, verify search reflects the change
