package index_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/edgetools/memento/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Fixtures

const (
	testModelID       = "all-MiniLM-L6-v2"
	testSentexVersion = "v0.1.3"
	testDims          = 4 // small dims for test vectors
)

// makeTestVector returns a small float32 slice of the given length,
// with values 0.1*i so they are deterministic and distinguishable.
func makeTestVector(dims int, seed float32) []float32 {
	v := make([]float32, dims)
	for i := range v {
		v[i] = seed + float32(i)*0.1
	}
	return v
}

// sampleEntries returns a small but realistic set of CacheEntry values for
// round-trip and multi-entry tests.
func sampleEntries() []index.CacheEntry {
	return []index.CacheEntry{
		{
			PageName:    "deployment",
			ContentHash: "abc123def456",
			Chunks: []index.CachedChunk{
				{StartLine: 1, EndLine: 10, Vector: makeTestVector(testDims, 0.1)},
				{StartLine: 11, EndLine: 25, Vector: makeTestVector(testDims, 0.5)},
			},
		},
		{
			PageName:    "ci-cd-pipeline",
			ContentHash: "deadbeef1234",
			Chunks: []index.CachedChunk{
				{StartLine: 1, EndLine: 50, Vector: makeTestVector(testDims, 1.0)},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// SaveCache / LoadCache round-trip

func TestSaveLoadCache_RoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	entries := sampleEntries()

	err := index.SaveCache(path, entries, testModelID, testSentexVersion, testDims)
	require.NoError(t, err, "SaveCache should not return an error")

	loaded, err := index.LoadCache(path, testModelID, testSentexVersion, testDims)
	require.NoError(t, err, "LoadCache should not return an error")

	require.Len(t, loaded, len(entries), "loaded entry count should match saved count")

	for i, want := range entries {
		got := loaded[i]
		assert.Equal(t, want.PageName, got.PageName, "PageName mismatch at index %d", i)
		assert.Equal(t, want.ContentHash, got.ContentHash, "ContentHash mismatch at index %d", i)
		require.Len(t, got.Chunks, len(want.Chunks), "Chunk count mismatch at index %d", i)
		for j, wantChunk := range want.Chunks {
			gotChunk := got.Chunks[j]
			assert.Equal(t, wantChunk.StartLine, gotChunk.StartLine, "StartLine mismatch entry %d chunk %d", i, j)
			assert.Equal(t, wantChunk.EndLine, gotChunk.EndLine, "EndLine mismatch entry %d chunk %d", i, j)
			assert.Equal(t, wantChunk.Vector, gotChunk.Vector, "Vector mismatch entry %d chunk %d", i, j)
		}
	}
}

// ---------------------------------------------------------------------------
// Missing / non-existent cache file

func TestLoadCache_FileNotExist_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	// File does not exist — first run scenario.
	loaded, err := index.LoadCache(path, testModelID, testSentexVersion, testDims)
	require.NoError(t, err, "LoadCache on a missing file should not return an error")
	assert.Empty(t, loaded, "LoadCache on a missing file should return an empty slice")
}

// ---------------------------------------------------------------------------
// Cache invalidation: stale header fields

func TestLoadCache_ModelIDMismatch_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	require.NoError(t, index.SaveCache(path, sampleEntries(), testModelID, testSentexVersion, testDims))

	// Load with a different model ID — vectors are incompatible.
	loaded, err := index.LoadCache(path, "different-model", testSentexVersion, testDims)
	require.NoError(t, err, "model mismatch should not surface as a hard error")
	assert.Empty(t, loaded, "model mismatch should return empty slice (stale cache)")
}

func TestLoadCache_SentexVersionMismatch_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	require.NoError(t, index.SaveCache(path, sampleEntries(), testModelID, testSentexVersion, testDims))

	// Library update may have changed preprocessing — treat cache as stale.
	loaded, err := index.LoadCache(path, testModelID, "v9.9.9", testDims)
	require.NoError(t, err, "sentex version mismatch should not surface as a hard error")
	assert.Empty(t, loaded, "sentex version mismatch should return empty slice (stale cache)")
}

func TestLoadCache_DimensionsMismatch_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	require.NoError(t, index.SaveCache(path, sampleEntries(), testModelID, testSentexVersion, testDims))

	// Different dimensionality — vectors are structurally incompatible.
	loaded, err := index.LoadCache(path, testModelID, testSentexVersion, 768)
	require.NoError(t, err, "dimensions mismatch should not surface as a hard error")
	assert.Empty(t, loaded, "dimensions mismatch should return empty slice (stale cache)")
}

// ---------------------------------------------------------------------------
// Corrupt / unreadable file

func TestLoadCache_CorruptFile_ReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	require.NoError(t, os.WriteFile(path, []byte("this is not valid gob data !!!"), 0o644))

	loaded, err := index.LoadCache(path, testModelID, testSentexVersion, testDims)
	assert.Error(t, err, "corrupt file should return an error")
	assert.Empty(t, loaded, "corrupt file should return nil/empty entries alongside the error")
}

// ---------------------------------------------------------------------------
// Full rewrite on subsequent saves

func TestSaveCache_FullRewrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	first := sampleEntries()
	require.NoError(t, index.SaveCache(path, first, testModelID, testSentexVersion, testDims))

	// Overwrite with a single entry that has a different page name.
	second := []index.CacheEntry{
		{
			PageName:    "new-page-only",
			ContentHash: "fresh0000",
			Chunks: []index.CachedChunk{
				{StartLine: 1, EndLine: 5, Vector: makeTestVector(testDims, 2.0)},
			},
		},
	}
	require.NoError(t, index.SaveCache(path, second, testModelID, testSentexVersion, testDims))

	loaded, err := index.LoadCache(path, testModelID, testSentexVersion, testDims)
	require.NoError(t, err)
	require.Len(t, loaded, 1, "second save should fully replace the first")
	assert.Equal(t, "new-page-only", loaded[0].PageName)
}

// ---------------------------------------------------------------------------
// Empty entries slice

func TestSaveLoadCache_EmptyEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	require.NoError(t, index.SaveCache(path, []index.CacheEntry{}, testModelID, testSentexVersion, testDims))

	loaded, err := index.LoadCache(path, testModelID, testSentexVersion, testDims)
	require.NoError(t, err)
	assert.Empty(t, loaded, "saving empty entries then loading should yield empty slice")
}

// ---------------------------------------------------------------------------
// Entry with no chunks

func TestSaveLoadCache_EntryWithNoChunks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	entries := []index.CacheEntry{
		{PageName: "empty-page", ContentHash: "hash0", Chunks: []index.CachedChunk{}},
	}
	require.NoError(t, index.SaveCache(path, entries, testModelID, testSentexVersion, testDims))

	loaded, err := index.LoadCache(path, testModelID, testSentexVersion, testDims)
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, "empty-page", loaded[0].PageName)
	assert.Empty(t, loaded[0].Chunks)
}

// ---------------------------------------------------------------------------
// Atomic write: file must exist and be valid immediately after SaveCache returns.
// (A partial write would appear as corrupt or missing — either way, invalid.)

func TestSaveCache_FileExistsAfterSave(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	require.NoError(t, index.SaveCache(path, sampleEntries(), testModelID, testSentexVersion, testDims))

	info, err := os.Stat(path)
	require.NoError(t, err, "cache file should exist at the specified path after save")
	assert.Greater(t, info.Size(), int64(0), "cache file should be non-empty")
}

// TestSaveCache_NoTempFileLeftBehind verifies that atomic rename-based writing
// does not leave a temporary file alongside the cache file.
func TestSaveCache_NoTempFileLeftBehind(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	require.NoError(t, index.SaveCache(path, sampleEntries(), testModelID, testSentexVersion, testDims))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the final cache file should exist; no temp files left behind")
	assert.Equal(t, ".memento-vectors", entries[0].Name())
}

// ---------------------------------------------------------------------------
// Vector precision: float32 values should survive the gob round-trip exactly.

func TestSaveLoadCache_VectorPrecision(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	// Use values that are not exactly representable in fewer bits to exercise
	// full float32 fidelity.
	vec := []float32{0.123456789, -0.987654321, 1.0 / 3.0, 3.14159265}
	entries := []index.CacheEntry{
		{
			PageName:    "precision-page",
			ContentHash: "hash1",
			Chunks: []index.CachedChunk{
				{StartLine: 1, EndLine: 1, Vector: vec},
			},
		},
	}

	require.NoError(t, index.SaveCache(path, entries, testModelID, testSentexVersion, len(vec)))

	loaded, err := index.LoadCache(path, testModelID, testSentexVersion, len(vec))
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Chunks, 1)
	assert.Equal(t, vec, loaded[0].Chunks[0].Vector, "float32 vector should survive gob round-trip bit-for-bit")
}

// ---------------------------------------------------------------------------
// Content hash is preserved (used by startup to detect stale pages)

func TestSaveLoadCache_ContentHashPreserved(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	hash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	entries := []index.CacheEntry{
		{
			PageName:    "hash-page",
			ContentHash: hash,
			Chunks: []index.CachedChunk{
				{StartLine: 1, EndLine: 10, Vector: makeTestVector(testDims, 0.0)},
			},
		},
	}

	require.NoError(t, index.SaveCache(path, entries, testModelID, testSentexVersion, testDims))

	loaded, err := index.LoadCache(path, testModelID, testSentexVersion, testDims)
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Equal(t, hash, loaded[0].ContentHash, "ContentHash must survive round-trip unchanged")
}

// ---------------------------------------------------------------------------
// Large number of entries (performance / correctness at scale)

func TestSaveLoadCache_ManyEntries(t *testing.T) {
	t.Parallel()

	const pageCount = 200
	const chunksPerPage = 5

	dir := t.TempDir()
	path := filepath.Join(dir, ".memento-vectors")

	entries := make([]index.CacheEntry, pageCount)
	for i := range entries {
		chunks := make([]index.CachedChunk, chunksPerPage)
		for j := range chunks {
			chunks[j] = index.CachedChunk{
				StartLine: j * 10,
				EndLine:   j*10 + 9,
				Vector:    makeTestVector(testDims, float32(i*chunksPerPage+j)*0.01),
			}
		}
		entries[i] = index.CacheEntry{
			PageName:    fmt.Sprintf("page-%d", i),
			ContentHash: fmt.Sprintf("hash%064d", i), // unique 64-char hex-like string per page
			Chunks:      chunks,
		}
	}

	require.NoError(t, index.SaveCache(path, entries, testModelID, testSentexVersion, testDims))

	loaded, err := index.LoadCache(path, testModelID, testSentexVersion, testDims)
	require.NoError(t, err)
	assert.Len(t, loaded, pageCount, "all %d entries should round-trip correctly", pageCount)
}
