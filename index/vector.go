package index

import (
	"math"
	"sort"

	"github.com/edgetools/memento/embed"
	"github.com/edgetools/memento/pages"
)

// VectorResult is a single result from a vector similarity search.
type VectorResult struct {
	Page  string
	Score float64 // cosine similarity, range [-1, 1] but typically [0, 1] for text
	Line  int     // 1-indexed start line of the best-matching chunk
}

// chunkEntry holds a stored chunk's embedding and metadata.
type chunkEntry struct {
	page     string    // canonical page name (as provided to Add)
	normPage string    // normalized page name (via pages.Normalize)
	line     int       // 1-indexed start line of this chunk
	vector   []float32 // L2-normalized embedding vector
}

// VectorIndex stores per-chunk embeddings and supports Add/Remove/Search
// with case-insensitive page-name matching.
type VectorIndex struct {
	model  *embed.Model
	chunks []chunkEntry
}

// NewVectorIndex creates an empty vector index backed by the given embedding model.
func NewVectorIndex(model *embed.Model) *VectorIndex {
	return &VectorIndex{model: model}
}

// Add chunks the page, embeds each chunk, and stores the resulting vectors.
// If the page was previously indexed its old chunks are replaced.
func (vi *VectorIndex) Add(page pages.Page) error {
	normName := pages.Normalize(page.Name)

	// Replace: remove any existing chunks for this page first.
	vi.removeByNorm(normName)

	// Split page into semantic chunks.
	chunks := ChunkPage(page)
	if len(chunks) == 0 {
		return nil
	}

	// Embed all chunks in a single batch call.
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}
	vectors, err := vi.model.EmbedBatch(texts)
	if err != nil {
		return err
	}

	// Append new entries, normalizing each vector for safe cosine computation.
	for i, c := range chunks {
		vi.chunks = append(vi.chunks, chunkEntry{
			page:     page.Name,
			normPage: normName,
			line:     c.StartLine,
			vector:   vecNormalize(vectors[i]),
		})
	}
	return nil
}

// Remove removes all stored chunks for the named page (case-insensitive).
func (vi *VectorIndex) Remove(name string) {
	vi.removeByNorm(pages.Normalize(name))
}

// removeByNorm filters out all chunks whose normPage equals normName,
// reusing the existing backing array to avoid an allocation.
func (vi *VectorIndex) removeByNorm(normName string) {
	kept := vi.chunks[:0]
	for _, c := range vi.chunks {
		if c.normPage != normName {
			kept = append(kept, c)
		}
	}
	vi.chunks = kept
}

// Search embeds the query, scores all stored chunk vectors via cosine similarity,
// deduplicates to one result per page (best-scoring chunk wins), then returns
// up to limit results sorted by score descending.
// Returns nil when the index is empty.
func (vi *VectorIndex) Search(query string, limit int) []VectorResult {
	if len(vi.chunks) == 0 {
		return nil
	}

	qVec, err := vi.model.Embed(query)
	if err != nil {
		return nil
	}
	qVec = vecNormalize(qVec)

	// Compute scores and deduplicate by normalized page name, keeping the best chunk.
	type pageScore struct {
		page  string
		score float64
		line  int
	}
	best := make(map[string]pageScore, len(vi.chunks))
	for _, c := range vi.chunks {
		score := vecDot(qVec, c.vector)
		if ex, ok := best[c.normPage]; !ok || score > ex.score {
			best[c.normPage] = pageScore{page: c.page, score: score, line: c.line}
		}
	}

	// Collect into a slice.
	results := make([]VectorResult, 0, len(best))
	for _, ps := range best {
		results = append(results, VectorResult{
			Page:  ps.page,
			Score: ps.score,
			Line:  ps.line,
		})
	}

	// Sort by score descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Honour the limit.
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// vecNormalize returns a unit-length copy of v.
// If v is the zero vector it is returned unchanged.
func vecNormalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	inv := 1.0 / math.Sqrt(sum)
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) * inv)
	}
	return out
}

// vecDot computes the dot product of two equal-length float32 vectors.
func vecDot(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}
