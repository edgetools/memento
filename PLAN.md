# Plan: Vector Search + Filesystem Watching

## Goal

Two independent features that together make memento significantly more
capable as a cross-session knowledge bridge:

1. **Vector search** — add semantic/embedding-based search as a parallel layer
   alongside BM25. A search for "deployment strategy" should find a page
   titled "CI/CD Pipeline" even when those exact words don't appear.

2. **Filesystem watching** — detect when markdown files change outside of
   memento (e.g. another Claude session writing through a different memento
   instance) and update the in-memory index without requiring a restart.

Both features share the same index mutation interface (`idx.Add` /
`idx.Remove`) and are independently implementable. They compose naturally:
when the filesystem watcher detects a change, it calls `idx.Add`, which
updates BM25 and vector indexes together.

---

## Architecture Decisions

### Vector search: parallel layer, merge results

Run BM25 and vector search in parallel on every query. Merge scored results,
deduplicate by page name, then feed into the existing graph boost and
relevance threshold pipeline.

```
Query → [BM25 + trigram fallback] + [Vector cosine search]
      → merge/deduplicate
      → graph boost
      → relevance threshold
      → return
```

The search tool's output format is unchanged. Consumers see the same
`{page, relevance, snippet, line, linked_pages}` structure.

### Embedding model: Go ONNX runtime, baked into the binary

Use a Go ONNX inference library to run `all-MiniLM-L6-v2` (384-dim, ~80MB)
in-process. The model file is embedded in the binary via `go:embed`. The
binary just works — no flags, no downloads, no external processes.

Embedding models are frozen artifacts. They don't receive updates. When a
better model becomes available, that's a memento version bump with cache
auto-invalidation.

### Vector storage: in-memory flat scan, sidecar cache

Store embedding vectors in memory. Search is brute-force cosine similarity.
At the expected scale (hundreds of pages, thousands of chunks), this is
sub-millisecond.

Embeddings are persisted to a sidecar cache file (`.memento-vectors`) keyed
by content hash. On startup, only pages whose content has changed are
re-embedded. The cache includes model identity for auto-invalidation across
model changes. The sidecar is derived data — safe to delete, rebuilds on
next startup, belongs in `.gitignore`.

### Chunked embeddings

Pages are split into chunks before embedding. Each chunk gets its own vector
with a reference to the page name and line range.

**Chunking strategy:**

1. Primary split on markdown section headings (`##`, `###`, etc.)
2. Fallback split on double-newline paragraph breaks (for pages without
   headings, e.g. snapshot-accumulated content)
3. Each chunk is prefixed with the page's `# Title` line so the embedding
   carries page identity
4. Chunks smaller than ~50 tokens are merged with the adjacent chunk
5. Maximum size bounded by the model's context window (~256 tokens)

### Filesystem watcher: `fsnotify` with debouncing

Watch the content directory with `fsnotify`. On file create/modify/delete,
debounce (100-200ms), then re-parse and call `idx.Add` or `idx.Remove`.
Always-on — no opt-in flag needed. Falls back gracefully if `fsnotify` fails
to initialize (e.g. OS inotify limits).

Self-triggered events (from memento's own writes) are allowed to re-index.
The re-parse is fast and the simplicity is worth the negligible cost.

---

## Change Requests

The implementation is broken into seven change requests, each scoped to be
implementable and testable independently. They are ordered by dependency —
later CRs build on earlier ones, but each CR is a self-contained unit of
work.

1. **CR1: Markdown Chunking** — pure chunking logic, no embedding
2. **CR2: ONNX Embedding Runtime** — load model, embed text, return vectors
3. **CR3: Vector Index** — store/search chunk embeddings with Add/Remove
4. **CR4: Search Pipeline Merge** — wire vector into composite Index.Search
5. **CR5: Embedding Cache** — sidecar file persistence with content hashing
6. **CR6: Filesystem Watcher** — fsnotify, debounce, trigger index updates
7. **CR7: Wiring** — main.go startup, model init, connect everything
