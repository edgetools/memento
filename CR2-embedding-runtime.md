# Change Request: ONNX Embedding Runtime

## Summary

Add an `embed` package that loads an ONNX embedding model and converts text
strings into float32 vectors. This is the inference foundation — it takes a
string in and returns a vector out. Used downstream by the vector index (CR3)
to embed chunks and queries.

---

## Changes

### New package: `embed/`

#### `Model` type

```go
type Model struct {
    // internal: ONNX session, tokenizer, model dimensions
}
```

#### `LoadModel() (*Model, error)`

Loads the embedded ONNX model from the binary (via `go:embed`). Returns
an initialized model ready for inference. This is called once at startup.

The model file (`all-MiniLM-L6-v2` or equivalent) is embedded in the binary
using `//go:embed`. The specific model and embedding approach should be
determined during implementation based on what Go ONNX runtime libraries are
available and mature. Key requirements:

- Pure Go (no CGo) strongly preferred
- Must produce deterministic output for the same input
- Must support the tokenizer the model expects (typically WordPiece for
  MiniLM-family models)

If a pure-Go ONNX runtime with tokenizer support doesn't exist, the
implementation should document the tradeoff and proceed with the best
available option (CGo-based ONNX runtime is acceptable as a fallback if
pure Go is not viable).

#### `(*Model) Embed(text string) ([]float32, error)`

Tokenizes the input text using the model's tokenizer, runs inference, and
returns the embedding vector. The vector length is determined by the model
(384 for `all-MiniLM-L6-v2`).

The input text may be longer than the model's context window (~256 tokens).
Text beyond the window is truncated — this is standard behavior for
embedding models and is handled at the tokenizer level.

#### `(*Model) EmbedBatch(texts []string) ([][]float32, error)`

Batch variant of `Embed` for efficiency. Embeds multiple texts in a single
inference pass (or multiple passes if the batch is too large). Returns
vectors in the same order as the input texts.

This is used at startup to embed all chunks for new/changed pages. Batching
amortizes the per-inference overhead.

#### `(*Model) Dimensions() int`

Returns the dimensionality of the model's output vectors (e.g. 384). Used
by the vector index to pre-allocate storage.

---

## Behavior Details

### Model selection

The primary candidate is `all-MiniLM-L6-v2`:
- 384-dimensional output
- ~80MB ONNX file
- Well-studied for semantic similarity and retrieval tasks
- Widely used as a baseline in the embedding space

If implementation research reveals a better option (smaller model with
acceptable quality, or a model with better Go runtime support), the
implementer should use their judgment. The key constraint is that the model
must be small enough to `go:embed` without impractical build times.

### Tokenization

The model expects tokenized input (typically WordPiece tokenization for
BERT-family models). The `embed` package must handle tokenization internally
— callers pass plain text strings. If no pure-Go WordPiece tokenizer exists,
a simple whitespace + subword tokenizer that produces compatible token IDs
is acceptable as a starting point.

### Determinism

The same input text must always produce the same output vector. This is
important for cache validity — if embeddings were non-deterministic, content
hash-based caching (CR5) would not work correctly.

---

## Non-Changes

- No changes to existing packages (`index/`, `pages/`, `tools/`).
- No MCP tool schema changes.
- No vector storage or search — that's CR3.
- No model downloading at runtime — the model is baked into the binary.
