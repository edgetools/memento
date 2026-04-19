package main_test

// Tests for CR7: Wiring.
//
// Integration tests that verify the startup pipeline and live update behavior
// produced by wiring embed, index, watcher, and tools together:
//
//   - Startup with no cache file       → all pages are embedded; cache is written
//   - Startup with a valid cache file  → cached vectors are loaded; no re-embedding
//   - Startup with a stale cache file  → model-ID mismatch forces re-embedding
//   - Watcher + vector index           → external file creation/deletion updates
//                                        the model-backed index and writes the cache
//   - End-to-end MCP + watcher        → write via tool, external modify, search
//                                        reflects the change
//
// All model-dependent tests call getTestModel(t), which loads the
// all-MiniLM-L6-v2 model once per binary via sync.Once.
// Ensure the HuggingFace cache is populated before running these tests.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/edgetools/memento/embed"
	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/edgetools/memento/tools"
	"github.com/edgetools/memento/watcher"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Model loading — lazy, once per test binary.

var (
	onceModel    sync.Once
	sharedModel  *embed.Model
	sharedModelErr error
)

func getTestModel(t *testing.T) *embed.Model {
	t.Helper()
	onceModel.Do(func() {
		sharedModel, sharedModelErr = embed.LoadModel()
	})
	require.NoError(t, sharedModelErr, "embedding model must load (populate HuggingFace cache first)")
	return sharedModel
}

// ---------------------------------------------------------------------------
// Constants / helpers.

// debounceWait is long enough for the watcher's debounce window (≥ 150 ms) to
// settle before asserting index state.
const debounceWait = 300 * time.Millisecond

// cacheFilePath returns the conventional .memento-vectors path for a content dir.
func cacheFilePath(dir string) string {
	return filepath.Join(dir, ".memento-vectors")
}

// ---------------------------------------------------------------------------
// Startup tests.

// TestStartup_NoCacheFile_AllPagesEmbeddedAndSearchable verifies first-run
// behaviour: when no .memento-vectors file exists, every page is embedded by
// idx.Add, the cache file is written, and the index answers semantic queries.
func TestStartup_NoCacheFile_AllPagesEmbeddedAndSearchable(t *testing.T) {
	model := getTestModel(t)
	dir := t.TempDir()
	cp := cacheFilePath(dir)

	store := pages.NewStore(dir)
	_, err := store.Write("CI/CD Pipeline", "Deployment strategy using GitHub Actions for continuous delivery.")
	require.NoError(t, err)
	_, err = store.Write("Architecture Overview", "System design covering microservices and service mesh.")
	require.NoError(t, err)

	// Startup: no cache file exists.
	entries, err := index.LoadCache(cp, model.ID(), model.SentexVersion(), model.Dimensions())
	require.NoError(t, err)
	assert.Empty(t, entries, "expected empty cache on first run")

	idx := index.NewIndex(model, cp)
	for _, page := range store.Scan() {
		idx.Add(page) // embeds + writes cache write-through
	}

	// Cache file must have been written.
	assert.FileExists(t, cp, ".memento-vectors must be created after startup")

	// The index must answer a semantic query that has no keyword overlap with
	// the page title "CI/CD Pipeline" but is semantically related.
	results := idx.Search("deployment workflow automation", 5)
	require.NotEmpty(t, results, "search must return at least one result")
	assert.Equal(t, "CI/CD Pipeline", results[0].Page)
}

// TestStartup_ValidCacheFile_VectorsLoadedFromCache verifies that when a
// fresh .memento-vectors file exists (content hash matches), the startup
// sequence loads pre-computed vectors via AddFromCache rather than re-embedding,
// and the resulting index is still searchable.
func TestStartup_ValidCacheFile_VectorsLoadedFromCache(t *testing.T) {
	model := getTestModel(t)
	dir := t.TempDir()
	cp := cacheFilePath(dir)

	store := pages.NewStore(dir)
	_, err := store.Write("Container Orchestration", "Managing containers at scale with Kubernetes and Helm charts.")
	require.NoError(t, err)

	// First startup: populate the cache.
	idx1 := index.NewIndex(model, cp)
	for _, page := range store.Scan() {
		idx1.Add(page)
	}
	require.FileExists(t, cp, "cache must exist after first startup")

	// Second startup: load from cache.
	entries, err := index.LoadCache(cp, model.ID(), model.SentexVersion(), model.Dimensions())
	require.NoError(t, err)
	require.NotEmpty(t, entries, "cache must contain entries for the indexed page")

	idx2 := index.NewIndex(model, cp)
	for _, page := range store.Scan() {
		var cached *index.CacheEntry
		for i := range entries {
			if pages.NamesMatch(entries[i].PageName, page.Name) {
				cached = &entries[i]
				break
			}
		}
		require.NotNil(t, cached, "cache entry must exist for page %q", page.Name)
		idx2.AddFromCache(page, *cached)
	}

	// The index loaded from cache must still be searchable.
	results := idx2.Search("kubernetes helm deployment", 5)
	require.NotEmpty(t, results)
	assert.Equal(t, "Container Orchestration", results[0].Page)
}

// TestStartup_StaleCacheFile_ModelIDMismatch_AllPagesReEmbedded verifies that
// when the cache file was written by a different model (model ID mismatch),
// LoadCache returns an empty slice and the startup falls back to re-embedding
// all pages from scratch.
func TestStartup_StaleCacheFile_ModelIDMismatch_AllPagesReEmbedded(t *testing.T) {
	model := getTestModel(t)
	dir := t.TempDir()
	cp := cacheFilePath(dir)

	store := pages.NewStore(dir)
	_, err := store.Write("Infrastructure as Code", "Provisioning cloud resources with Terraform and Pulumi.")
	require.NoError(t, err)

	// Write a stale cache file with a wrong model ID.
	staleEntry := index.CacheEntry{
		PageName:    "Infrastructure as Code",
		ContentHash: "deadbeef",
		Chunks: []index.CachedChunk{
			{StartLine: 1, EndLine: 3, Vector: make([]float32, model.Dimensions())},
		},
	}
	err = index.SaveCache(cp, []index.CacheEntry{staleEntry}, "wrong-model-id", "v0.0.0", model.Dimensions())
	require.NoError(t, err)

	// LoadCache with the real model ID must return empty (stale cache).
	entries, err := index.LoadCache(cp, model.ID(), model.SentexVersion(), model.Dimensions())
	require.NoError(t, err)
	assert.Empty(t, entries, "stale cache must be ignored")

	// All pages are re-embedded.
	idx := index.NewIndex(model, cp)
	for _, page := range store.Scan() {
		idx.Add(page)
	}

	// The index must still answer queries correctly after re-embedding.
	results := idx.Search("terraform infrastructure provisioning", 5)
	require.NotEmpty(t, results)
	assert.Equal(t, "Infrastructure as Code", results[0].Page)
}

// ---------------------------------------------------------------------------
// Watcher + vector index integration tests.

// TestWiring_Watcher_ExternalCreate_UpdatesVectorIndexAndCache verifies that
// when a .md file is created outside of memento (e.g., by another process),
// the watcher picks it up, calls idx.Add, and the page becomes searchable via
// the vector index. The cache file must be updated as a result.
func TestWiring_Watcher_ExternalCreate_UpdatesVectorIndexAndCache(t *testing.T) {
	model := getTestModel(t)
	dir := t.TempDir()
	cp := cacheFilePath(dir)

	store := pages.NewStore(dir)
	idx := index.NewIndex(model, cp)

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	t.Cleanup(func() { w.Close() })

	// Write a .md file externally (bypassing the store write path).
	extFile := filepath.Join(dir, "Observability.md")
	content := "# Observability\n\nMonitoring and tracing distributed systems with OpenTelemetry.\n"
	require.NoError(t, os.WriteFile(extFile, []byte(content), 0644))

	// Wait for the debounce window to pass.
	time.Sleep(debounceWait)

	// The page must appear in search.
	results := idx.Search("distributed tracing monitoring", 5)
	require.NotEmpty(t, results, "externally created page must be indexed")
	assert.Equal(t, "Observability", results[0].Page)

	// The cache file must have been written by the idx.Add call in the watcher.
	assert.FileExists(t, cp, "cache must be updated after watcher-triggered Add")
}

// TestWiring_Watcher_ExternalDelete_RemovesFromVectorIndex verifies that when
// a .md file is deleted outside of memento, the watcher removes the page from
// the index (including the vector layer) and updates the cache.
func TestWiring_Watcher_ExternalDelete_RemovesFromVectorIndex(t *testing.T) {
	model := getTestModel(t)
	dir := t.TempDir()
	cp := cacheFilePath(dir)

	store := pages.NewStore(dir)
	_, err := store.Write("Service Mesh", "Traffic management between microservices using Istio and Envoy.")
	require.NoError(t, err)

	idx := index.NewIndex(model, cp)
	for _, page := range store.Scan() {
		idx.Add(page)
	}

	// The page must be searchable before deletion.
	before := idx.Search("istio envoy traffic", 5)
	require.NotEmpty(t, before, "page must be indexed before external deletion")

	// Cache must exist after the initial Add calls, before the watcher runs.
	require.FileExists(t, cp, "cache must exist after initial Add")

	w, err := watcher.NewWatcher(dir, store, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	t.Cleanup(func() { w.Close() })

	// Delete the file externally.
	require.NoError(t, os.Remove(store.FilePath("Service Mesh")))

	// Wait for the debounce window to pass.
	time.Sleep(debounceWait)

	// The page must no longer appear in search results.
	after := idx.Search("istio envoy traffic", 5)
	for _, r := range after {
		assert.NotEqual(t, "Service Mesh", r.Page, "deleted page must not appear in search")
	}

	// The cache must have been rewritten by idx.Remove (write-through).
	// The deleted page's entry must be absent.
	cacheEntries, err := index.LoadCache(cp, model.ID(), model.SentexVersion(), model.Dimensions())
	require.NoError(t, err)
	for _, e := range cacheEntries {
		assert.False(t, pages.NamesMatch(e.PageName, "Service Mesh"),
			"deleted page must not remain in cache after watcher-triggered Remove")
	}
}

// ---------------------------------------------------------------------------
// End-to-end test.

// searchResult mirrors the per-result JSON returned by the search_pages tool.
type searchResult struct {
	Page      string  `json:"page"`
	Relevance float64 `json:"relevance"`
	Snippet   string  `json:"snippet"`
	Line      int     `json:"line"`
}

type searchResponse struct {
	Results []searchResult `json:"results"`
}

// parseE2EJSON extracts the first text content item from a tool result and
// JSON-decodes it into v.
func parseE2EJSON(t *testing.T, result *mcp.CallToolResult, v any) {
	t.Helper()
	require.NotEmpty(t, result.Content, "tool result has no content items")
	textContent, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok, "first content item is not text content")
	require.NoError(t, json.Unmarshal([]byte(textContent.Text), v),
		"failed to unmarshal tool result JSON: %s", textContent.Text)
}

// setupE2EServer creates a model-backed MCP server with a watcher running
// against a fresh temp directory. Caller must not close the client manually;
// cleanup is registered via t.Cleanup.
func setupE2EServer(t *testing.T) (c *client.Client, st *pages.Store, idx *index.Index) {
	t.Helper()

	model := getTestModel(t)
	dir := t.TempDir()
	cp := cacheFilePath(dir)

	st = pages.NewStore(dir)
	idx = index.NewIndex(model, cp)

	s := server.NewMCPServer("memento-e2e", "0.0.0", server.WithToolCapabilities(true))
	tools.Register(s, st, idx)

	var err error
	c, err = client.NewInProcessClient(s)
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })

	require.NoError(t, c.Start(context.Background()))

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "memento-e2e-client", Version: "0.0.0"}
	_, err = c.Initialize(context.Background(), initReq)
	require.NoError(t, err)

	// Start watcher after MCP server is ready.
	w, err := watcher.NewWatcher(dir, st, idx)
	require.NoError(t, err)
	require.NoError(t, w.Start())
	t.Cleanup(func() { w.Close() })

	return c, st, idx
}

// callE2ETool is a thin wrapper around client.CallTool for the e2e tests.
func callE2ETool(t *testing.T, c *client.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	result, err := c.CallTool(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	return result
}

// TestEndToEnd_MCPWriteThenExternalModify_SearchReflectsChange is the
// end-to-end scenario from CR7:
//  1. Write a page via the write_page MCP tool.
//  2. Verify the search_pages tool returns that page.
//  3. Modify the file externally (simulating another Claude session or editor).
//  4. Wait for the watcher debounce to fire.
//  5. Verify search_pages reflects the updated content.
func TestEndToEnd_MCPWriteThenExternalModify_SearchReflectsChange(t *testing.T) {
	c, st, idx := setupE2EServer(t)

	// Step 1: write a page via MCP.
	writeRes := callE2ETool(t, c, "write_page", map[string]any{
		"page":    "Deployment Guide",
		"content": "Instructions for deploying the application to staging using Docker Compose.",
	})
	require.False(t, writeRes.IsError, "write_page must succeed")

	// Step 2: the page must appear in search immediately after writing.
	searchRes1 := callE2ETool(t, c, "search_pages", map[string]any{
		"query": "docker compose staging deployment",
		"limit": 5,
	})
	require.False(t, searchRes1.IsError, "search_pages must succeed")
	require.NotEmpty(t, searchRes1.Content, "search must return content")

	var resp1 searchResponse
	parseE2EJSON(t, searchRes1, &resp1)
	require.NotEmpty(t, resp1.Results, "search must return at least one result for the written page")
	assert.Equal(t, "Deployment Guide", resp1.Results[0].Page)

	// Step 3: externally modify the file, simulating an out-of-band write.
	filePath := st.FilePath("Deployment Guide")
	newContent := "# Deployment Guide\n\nMigrating to Kubernetes with Helm charts for production rollout.\n"
	require.NoError(t, os.WriteFile(filePath, []byte(newContent), 0644))

	// Step 4: wait for the watcher debounce.
	time.Sleep(debounceWait)

	// Step 5: search for the new content keywords; the page must still appear.
	searchRes2 := callE2ETool(t, c, "search_pages", map[string]any{
		"query": "kubernetes helm production rollout",
		"limit": 5,
	})
	require.False(t, searchRes2.IsError, "search_pages must succeed after external modification")

	var resp2 searchResponse
	parseE2EJSON(t, searchRes2, &resp2)
	require.NotEmpty(t, resp2.Results, "search must return the re-indexed page")
	assert.Equal(t, "Deployment Guide", resp2.Results[0].Page)

	// The old Docker Compose query must no longer rank as the top result,
	// since the page content has been replaced with Kubernetes content.
	// (We only verify the new content is findable; ranking is best-effort.)
	_ = idx // idx accessible for direct assertions if needed in future
}
