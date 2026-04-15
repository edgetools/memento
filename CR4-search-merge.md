# Change Request: Search Pipeline Merge

## Summary

Wire the vector index (CR3) into the existing composite `Index` so that
every search query runs BM25 and vector search in parallel, merges the
results, and feeds them through the existing graph boost and relevance
threshold pipeline. The search tool's output format is unchanged.

---

## Changes

### Modified: `index/index.go`

#### `Index` struct gains a `vector` field

```go
type Index struct {
    bm25   *BM25
    tri    *Trigram
    graph  *Graph
    vector *VectorIndex // nil when no embedding model is available
    pages  map[string]pages.Page
}
```

#### `NewIndex` gains an optional model parameter

```go
func NewIndex(model *embed.Model) *Index
```

When `model` is non-nil, a `VectorIndex` is created and wired in. When
`model` is nil, the index behaves exactly as it does today (BM25 + trigram
+ graph only). This preserves backward compatibility for tests and for
configurations without an embedding model.

Note: changing `NewIndex`'s signature will require updating all existing
callers (tests and `main.go`). Existing callers that don't use vector
search should pass `nil`.

#### `Index.Add` and `Index.Remove` update the vector index

When `vector` is non-nil, `Add` also calls `vector.Add(page)` and `Remove`
also calls `vector.Remove(name)`. This ensures the vector index stays in
sync with the BM25 index on every mutation — whether the mutation comes
from an MCP tool call, a filesystem watcher, or the startup scan.

#### `Index.Search` runs both pipelines and merges

Updated pipeline:

```
1. BM25 keyword search (existing)
2. Trigram fallback if <3 BM25 results (existing)
3. Vector cosine search (new, only when vector != nil)
4. Merge BM25 and vector results (new)
5. Graph boost (existing)
6. Relevance threshold (existing)
7. Build results with snippets (existing)
```

**Merge logic (step 4):**

The BM25 and vector scores are on different scales (BM25 is unbounded
positive, cosine similarity is [0, 1]). Before merging, both score sets
are normalized to [0, 1] relative to their respective top scores.

For each page that appears in either result set:
- If it appears in both: take the higher normalized score
- If it appears in only one: use that normalized score

The merged scores feed into the existing graph boost step unchanged.

**Snippet generation for vector-only matches:**

Pages that matched via vector search but not BM25 need snippets. The
matching chunk's line range is known (from `VectorResult.Line`). Use the
existing `densitySnippet` approach but centered on the chunk's line range
rather than searching the whole page for term density.

---

## Behavior Details

### Backward compatibility

When `model` is nil (no embedding model), `Index.Search` behaves exactly
as it does today. All existing tests must continue to pass by passing `nil`
for the model parameter.

### Score normalization

Both BM25 and vector scores are normalized to [0, 1] before merging:
- BM25: divide by the top BM25 score (already done for the existing
  relevance ratio)
- Vector: divide by the top vector score (cosine similarity is already
  bounded but normalizing ensures consistent scaling)

If either pipeline returns no results, the other pipeline's results pass
through unnormalized (they're already the full result set).

### Vector-only results and IsDirect

Pages that appear only via vector search (not BM25) should be marked as
`IsDirect: true` since they are a direct semantic match — they matched the
query's meaning, just not its keywords. This distinguishes them from
graph-boosted pages (which are `IsDirect: false`).

---

## Non-Changes

- No changes to MCP tool schemas or output format.
- No changes to graph boost or relevance threshold logic.
- No cache persistence — that's CR5.
- No filesystem watching — that's CR6.
