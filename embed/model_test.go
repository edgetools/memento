package embed_test

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/edgetools/memento/embed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testModel is loaded once for the entire test binary via TestMain.
// Model loading (~80MB ONNX file) is expensive; doing it once avoids
// multi-second overhead on every test run.
var testModel *embed.Model

func TestMain(m *testing.M) {
	var err error
	testModel, err = embed.LoadModel()
	if err != nil {
		// If the model cannot load, every downstream test is meaningless.
		// Print the error and exit with a non-zero code so CI fails clearly.
		_, _ = os.Stderr.WriteString("embed_test: LoadModel failed: " + err.Error() + "\n")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// cosineSim computes the cosine similarity between two equal-length float32 vectors.
// Returns a value in [-1, 1]; 1 means identical direction, 0 means orthogonal.
func cosineSim(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// ── LoadModel ────────────────────────────────────────────────────────────────

func TestLoadModel(t *testing.T) {
	t.Parallel()

	t.Run("returns_non_nil_model", func(t *testing.T) {
		t.Parallel()
		// LoadModel is called in TestMain; we just assert the result is usable.
		// A second call to LoadModel must also succeed — the model file is embedded,
		// so there are no external dependencies that could fail on a second call.
		m, err := embed.LoadModel()
		require.NoError(t, err, "LoadModel should not return an error")
		require.NotNil(t, m, "LoadModel should return a non-nil *Model")
	})

	t.Run("called_multiple_times_is_safe", func(t *testing.T) {
		t.Parallel()
		// The spec says the model is baked into the binary; calling LoadModel
		// more than once (e.g. from different goroutines at startup) must not panic.
		m1, err1 := embed.LoadModel()
		require.NoError(t, err1)
		m2, err2 := embed.LoadModel()
		require.NoError(t, err2)
		// Both models must be functional.
		v1, err := m1.Embed("hello")
		require.NoError(t, err)
		v2, err := m2.Embed("hello")
		require.NoError(t, err)
		require.Equal(t, len(v1), len(v2), "independently loaded models should produce same-length vectors")
	})
}

// ── Dimensions ───────────────────────────────────────────────────────────────

func TestModel_Dimensions(t *testing.T) {
	t.Parallel()

	t.Run("returns_positive_integer", func(t *testing.T) {
		t.Parallel()
		dims := testModel.Dimensions()
		assert.Greater(t, dims, 0,
			"Dimensions() must return a positive integer")
	})

	t.Run("matches_actual_embed_output_length", func(t *testing.T) {
		t.Parallel()
		// The Dimensions() contract: it must equal the length of any vector
		// returned by Embed.
		vec, err := testModel.Embed("test")
		require.NoError(t, err)
		assert.Equal(t, testModel.Dimensions(), len(vec),
			"Dimensions() must equal len(Embed(text))")
	})
}

// ── Embed ─────────────────────────────────────────────────────────────────────

func TestModel_Embed(t *testing.T) {
	t.Parallel()

	t.Run("returns_vector_of_correct_length", func(t *testing.T) {
		t.Parallel()
		vec, err := testModel.Embed("the quick brown fox")
		require.NoError(t, err)
		require.NotNil(t, vec)
		assert.Equal(t, testModel.Dimensions(), len(vec),
			"Embed output length must equal Dimensions()")
	})

	t.Run("does_not_return_all_zeros", func(t *testing.T) {
		t.Parallel()
		// A zero vector indicates a silent failure in inference; a real embedding
		// model should return a non-zero vector for any non-trivial input.
		vec, err := testModel.Embed("the quick brown fox")
		require.NoError(t, err)
		var sumAbs float32
		for _, v := range vec {
			if v < 0 {
				sumAbs -= v
			} else {
				sumAbs += v
			}
		}
		assert.Greater(t, sumAbs, float32(0),
			"Embed output must not be an all-zeros vector")
	})

	t.Run("empty_string_does_not_error", func(t *testing.T) {
		t.Parallel()
		// The spec does not restrict input to non-empty strings. The model may
		// return a valid (possibly zero) vector for empty input — it must not panic
		// or return an error.
		vec, err := testModel.Embed("")
		require.NoError(t, err)
		assert.Equal(t, testModel.Dimensions(), len(vec),
			"empty string should still produce a correctly-sized vector")
	})

	t.Run("long_text_is_truncated_not_errored", func(t *testing.T) {
		t.Parallel()
		// Text longer than the model's context window (~256 tokens) is silently
		// truncated — the spec says this is standard behaviour. It must not error.
		longText := strings.Repeat("deployment strategy continuous integration testing ", 60)
		vec, err := testModel.Embed(longText)
		require.NoError(t, err,
			"text exceeding the model context window should be truncated, not error")
		assert.Equal(t, testModel.Dimensions(), len(vec),
			"truncated long-text embedding must still have correct dimensionality")
	})

	t.Run("single_word_returns_vector", func(t *testing.T) {
		t.Parallel()
		vec, err := testModel.Embed("kubernetes")
		require.NoError(t, err)
		assert.Equal(t, testModel.Dimensions(), len(vec))
	})
}

// ── Determinism ───────────────────────────────────────────────────────────────

func TestModel_Embed_Deterministic(t *testing.T) {
	t.Parallel()

	t.Run("same_text_produces_identical_vector", func(t *testing.T) {
		t.Parallel()
		// Determinism is critical for content-hash-based cache validity (CR5).
		// The same input text must always yield bit-for-bit identical float32 vectors.
		text := "CI/CD pipeline deployment strategy"
		vec1, err := testModel.Embed(text)
		require.NoError(t, err)
		vec2, err := testModel.Embed(text)
		require.NoError(t, err)
		require.Equal(t, len(vec1), len(vec2))
		for i := range vec1 {
			assert.Equalf(t, vec1[i], vec2[i],
				"component %d differs: first call=%v second call=%v", i, vec1[i], vec2[i])
		}
	})

	t.Run("different_texts_produce_different_vectors", func(t *testing.T) {
		t.Parallel()
		// Two semantically distinct inputs should produce distinct vectors.
		vecA, err := testModel.Embed("database schema migration")
		require.NoError(t, err)
		vecB, err := testModel.Embed("kubernetes pod scheduling")
		require.NoError(t, err)
		// Vectors are not identical — at least one component must differ.
		identical := true
		for i := range vecA {
			if vecA[i] != vecB[i] {
				identical = false
				break
			}
		}
		assert.False(t, identical,
			"distinct inputs must produce distinct output vectors")
	})
}

// ── EmbedBatch ────────────────────────────────────────────────────────────────

func TestModel_EmbedBatch(t *testing.T) {
	t.Parallel()

	t.Run("returns_one_vector_per_input", func(t *testing.T) {
		t.Parallel()
		texts := []string{
			"first document about databases",
			"second document about networking",
			"third document about file systems",
		}
		vecs, err := testModel.EmbedBatch(texts)
		require.NoError(t, err)
		require.Len(t, vecs, len(texts),
			"EmbedBatch must return exactly one vector per input text")
	})

	t.Run("each_vector_has_correct_dimensions", func(t *testing.T) {
		t.Parallel()
		texts := []string{"alpha", "beta", "gamma"}
		vecs, err := testModel.EmbedBatch(texts)
		require.NoError(t, err)
		for i, vec := range vecs {
			assert.Equalf(t, testModel.Dimensions(), len(vec),
				"vector at index %d has wrong length: got %d, want %d",
				i, len(vec), testModel.Dimensions())
		}
	})

	t.Run("empty_input_returns_empty_result", func(t *testing.T) {
		t.Parallel()
		vecs, err := testModel.EmbedBatch([]string{})
		require.NoError(t, err)
		assert.Empty(t, vecs,
			"EmbedBatch with empty input must return an empty (not nil error) result")
	})

	t.Run("nil_input_returns_empty_result", func(t *testing.T) {
		t.Parallel()
		// Defensive: nil slice should behave the same as an empty slice.
		assert.NotPanics(t, func() {
			vecs, err := testModel.EmbedBatch(nil)
			require.NoError(t, err)
			assert.Empty(t, vecs)
		})
	})

	t.Run("order_preserved_matches_individual_embed", func(t *testing.T) {
		t.Parallel()
		// The spec states results are returned in the same order as inputs.
		// Verify by comparing each batch vector to the individually-computed vector.
		texts := []string{
			"deployment strategy",
			"CI/CD pipeline",
			"monitoring and alerting",
		}
		batchVecs, err := testModel.EmbedBatch(texts)
		require.NoError(t, err)
		require.Len(t, batchVecs, len(texts))

		for i, text := range texts {
			singleVec, err := testModel.Embed(text)
			require.NoErrorf(t, err, "Embed(%q) failed", text)
			require.Lenf(t, batchVecs[i], testModel.Dimensions(),
				"batch vector %d has wrong length", i)
			for j := range singleVec {
				assert.Equalf(t, singleVec[j], batchVecs[i][j],
					"text[%d]=%q: component %d differs between Embed and EmbedBatch", i, text, j)
			}
		}
	})

	t.Run("single_element_batch_matches_embed", func(t *testing.T) {
		t.Parallel()
		// A one-element batch must produce the same vector as a single Embed call.
		text := "semantic search retrieval"
		batchVecs, err := testModel.EmbedBatch([]string{text})
		require.NoError(t, err)
		require.Len(t, batchVecs, 1)

		singleVec, err := testModel.Embed(text)
		require.NoError(t, err)

		for i := range singleVec {
			assert.Equalf(t, singleVec[i], batchVecs[0][i],
				"component %d differs between Embed and single-element EmbedBatch", i)
		}
	})

	t.Run("large_batch_returns_all_vectors", func(t *testing.T) {
		t.Parallel()
		// The spec says EmbedBatch may use multiple passes for large batches.
		// Regardless of internal batching strategy, all vectors must be returned.
		texts := make([]string, 50)
		for i := range texts {
			texts[i] = strings.Repeat("word ", i+1) // vary lengths
		}
		vecs, err := testModel.EmbedBatch(texts)
		require.NoError(t, err)
		assert.Len(t, vecs, 50,
			"large batch should return all 50 vectors regardless of internal batch size")
	})
}

// ── Semantic similarity ───────────────────────────────────────────────────────

func TestModel_SemanticSimilarity(t *testing.T) {
	t.Parallel()

	t.Run("similar_texts_score_higher_than_dissimilar", func(t *testing.T) {
		t.Parallel()
		// Core semantic search property: vectors for related concepts should be
		// closer (higher cosine similarity) than vectors for unrelated concepts.
		// This validates that the model produces meaningful embeddings, not random noise.
		query := "deployment strategy"

		// "CI/CD Pipeline" is semantically related to "deployment strategy"
		related := "CI/CD Pipeline continuous integration delivery"
		// "Ancient Roman aqueducts" is semantically unrelated
		unrelated := "ancient Roman aqueducts water supply infrastructure"

		vecQuery, err := testModel.Embed(query)
		require.NoError(t, err)
		vecRelated, err := testModel.Embed(related)
		require.NoError(t, err)
		vecUnrelated, err := testModel.Embed(unrelated)
		require.NoError(t, err)

		simRelated := cosineSim(vecQuery, vecRelated)
		simUnrelated := cosineSim(vecQuery, vecUnrelated)

		assert.Greater(t, simRelated, simUnrelated,
			"cosine similarity between related texts (%.4f) should exceed similarity to unrelated texts (%.4f)",
			simRelated, simUnrelated)
	})

	t.Run("identical_texts_have_cosine_sim_near_one", func(t *testing.T) {
		t.Parallel()
		// A text compared to itself should have cosine similarity ≈ 1.
		text := "the quick brown fox jumps over the lazy dog"
		vec1, err := testModel.Embed(text)
		require.NoError(t, err)
		vec2, err := testModel.Embed(text)
		require.NoError(t, err)

		sim := cosineSim(vec1, vec2)
		assert.InDelta(t, 1.0, sim, 0.0001,
			"identical texts must have cosine similarity ≈ 1.0, got %.6f", sim)
	})
}
