# Change Request: Vector Index

## Summary

Add a vector index that stores per-chunk embeddings and supports
Add/Remove/Search operations. This connects the chunking logic (CR1) and
embedding runtime (CR2) into a searchable index that mirrors the BM25
index's interface pattern.

---

## Changes

### New file: `index/vector.go`

#### `VectorIndex` type

```go
type VectorIndex struct {
    // internal: model, chunk embeddings, page-to-chunks mapping
}
```

#### `NewVectorIndex(model *embed.Model) *VectorIndex`

Creates an empty vector index backed by the given embedding model.

#### `(*VectorIndex) Add(page pages.Page) error`

1. Chunks the page using `ChunkPage` (CR1)
2. Embeds each chunk using the model (CR2)
3. Stores the chunk vectors, replacing any existing vectors for this page

If the page was previously indexed, its old chunks are removed before the
new ones are stored. This mirrors `BM25.Add` behavior.

#### `(*VectorIndex) Remove(name string)`

Removes all stored chunks and vectors for the named page. Case-insensitive
name matching (same as BM25).

#### `(*VectorIndex) Search(query string, limit int) []VectorResult`

1. Embeds the query string using the model
2. Computes cosine similarity against all stored chunk vectors
3. Deduplicates by page name — if multiple chunks from the same page match,
   keeps the highest-scoring chunk
4. Returns up to `limit` results sorted by score descending

#### `VectorResult` type

```go
type VectorResult struct {
    Page  string
    Score float64 // cosine similarity, range [-1, 1] but typically [0, 1] for text
    Line  int     // start line of the matching chunk
}
```

---

## Behavior Details

### Cosine similarity

Score is computed as the dot product of two L2-normalized vectors. The
embedding model typically produces normalized vectors, but the implementation
should normalize regardless to be safe.

### Page deduplication

A page with 5 chunks might have 3 chunks that match a query. The search
returns the page once, with the score and line number from the
highest-scoring chunk. This matches the BM25 behavior where a page appears
at most once in results.

### Score range

Cosine similarity produces scores in [-1, 1]. For text embeddings, scores
are typically in [0, 1] (negative similarity is rare for natural language).
The vector index returns raw cosine scores. Score normalization for merging
with BM25 is handled in CR4.

### Empty index

Searching an empty vector index returns nil (no results). Adding and
removing from an empty index does not panic.

### Case-insensitive names

Page names in the vector index are stored and matched case-insensitively,
consistent with the BM25 index and the rest of memento.

---

## Non-Changes

- No changes to the existing `Index` composite type — that's CR4.
- No changes to MCP tools or search output format.
- No cache persistence — that's CR5.
- No filesystem watching — that's CR6.
