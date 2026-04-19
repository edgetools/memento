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

The model load is fatal. Vector search is a core part of the offering;
the server should not silently degrade to BM25-only. go-sentex acquires
the model from the HuggingFace Hub cache on first run (~87MB download,
honors `HF_HOME`), so a first-run failure usually means no network — in
which case the operator should retry with network access or pre-populate
the cache.

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

`Index.Add` owns cache write-through. The cache path is stored as a field
on `Index` (set during `NewIndex`), and every `Add`/`Remove` that mutates
the vector index writes the cache file at the end of the call.

This keeps the tool layer and the filesystem watcher symmetric — both go
through `Index.Add`, so both trigger the same write-through. If the cache
save lived in the tool handler, the watcher path would need to duplicate
it.

The save is a full rewrite (see CR5). For the expected scale (hundreds of
pages, thousands of chunks) this is sub-second.

### Modified: `tools/tools.go`

No changes required. Cache write-through lives in `Index.Add`, so the
tool layer is unaffected — `Register` and `RegisterAutoCommit` keep their
existing signatures.

---

## Behavior Details

### First-run experience

On first run with no HuggingFace cache and no `.memento-vectors`:
1. go-sentex downloads `all-MiniLM-L6-v2` (~87MB) to the HF Hub cache
   (honors `HF_HOME`). One-time cost.
2. Model loads into memory.
3. All pages are scanned and BM25-indexed (fast, existing behavior).
4. All pages are chunked and embedded (takes a few seconds for ~50-100
   pages).
5. Cache is written to `.memento-vectors`.
6. Subsequent startups skip the download and only re-embed changed pages.

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
- No new CLI flags. The model is loaded by go-sentex (HF Hub cache,
  `HF_HOME`), the `.memento-vectors` path is derived from the content
  directory, and the watcher is always-on with a warn-on-failure policy.
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
