# Change Request: Embedding Runtime (go-sentex wrapper)

## Summary

Add an `embed` package that wraps
[`github.com/edgetools/go-sentex`](https://github.com/edgetools/go-sentex)
and exposes a small, memento-owned API for loading a model and embedding
text. This is the inference foundation — it takes a string in and returns
a vector out. Used downstream by the vector index (CR3) to embed chunks
and queries.

The wrapper exists so that memento's internals depend on `embed.Model`
(not directly on `sentex.Model`), which keeps the option open to swap the
backend later without touching callers.

---

## Changes

### New package: `embed/`

#### `Model` type

```go
type Model struct {
    // internal: *sentex.Model
}
```

#### `LoadModel() (*Model, error)`

Calls `sentex.LoadModel()` and wraps the result in an `embed.Model`.
go-sentex handles acquisition (HuggingFace Hub cache under `HF_HOME`) and
ONNX session setup. On first run with no cached model, go-sentex downloads
~87MB once; subsequent runs are offline.

#### `(*Model) Embed(text string) ([]float32, error)`

Delegates to `sentex.Model.Embed`. Returns a 384-dimensional L2-normalized
vector. Inputs longer than the model's context window (~256 tokens) are
truncated by go-sentex — no error.

#### `(*Model) EmbedBatch(texts []string) ([][]float32, error)`

Delegates to `sentex.Model.EmbedBatch`. Returns vectors in the same order
as the input texts. Used at startup to embed all chunks for new/changed
pages.

#### `(*Model) Dimensions() int`

Returns `sentex.Model.Dimensions()` — 384 for `all-MiniLM-L6-v2`. Used by
the vector index to pre-allocate storage.

---

## Behavior Details

### Backend: go-sentex

- Pure Go, no CGo. Builds with `CGO_ENABLED=0`, cross-compiles cleanly.
- Model: `all-MiniLM-L6-v2`, 384-dim L2-normalized output.
- Max input: 256 tokens (truncation handled internally).
- Model file cached at the standard HuggingFace Hub location; honors the
  `HF_HOME` environment variable. Compatible with existing caches created
  by Python HuggingFace tooling.

### Determinism

go-sentex produces bit-identical output for identical input on the same
machine. This is required for content-hash-based cache validity (CR5):
if embeddings were non-deterministic, cache hits would silently drift
from re-embeds. The test suite enforces this with an exact-equality
assertion.

### Model identity for cache invalidation

The cache (CR5) records both the model name and the go-sentex library
version in its header. A go-sentex update that changes preprocessing or
the underlying model invalidates the cache on next startup.

---

## Non-Changes

- No changes to existing packages (`index/`, `pages/`, `tools/`).
- No MCP tool schema changes.
- No vector storage or search — that's CR3.
- No tokenizer code in memento — go-sentex owns tokenization.
- No `go:embed` of the model — go-sentex handles acquisition.
