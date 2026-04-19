package index_test

// Tests for CR4: Search Pipeline Merge.
//
// These tests cover the composite Index when an embedding model is wired in:
//   - NewIndex(nil)  → backward-compatible BM25 + trigram + graph pipeline
//   - NewIndex(model) → BM25 + vector parallel search, merged results
//
// Model-dependent tests call getVectorModel(t) (defined in testhelpers_test.go).
// It loads the model once per test binary via sync.Once and fails hard if the
// model cannot be loaded — ensure the HuggingFace cache is populated.

import (
	"testing"

	"github.com/edgetools/memento/index"
	"github.com/edgetools/memento/pages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test content — semantically rich, two clearly separate topic domains.
// kubernetesBody and databaseBody are declared in testhelpers_test.go.
// ---------------------------------------------------------------------------

// cicdBody: three ##-headed sections about CI/CD pipelines.
// Semantically close to "deployment strategy" but deliberately uses different
// vocabulary so that only vector (not BM25 keyword) search can surface it.
const cicdBody = `## Continuous Integration

Continuous integration and continuous delivery pipelines automate the process
of building, testing, and releasing software to production environments. Every
commit triggers an automated build and test suite that validates the change
before merging into the main branch. Teams using these pipelines can ship
software changes multiple times per day with high confidence because each
release is small and thoroughly validated by automated checks.

## Release Automation

Automated release pipelines remove human error from the publishing process. A
typical pipeline includes steps for code compilation, unit testing, integration
testing, artifact packaging, staging environment promotion, and final production
rollout. Rollback triggers fire automatically when error rates or latency
metrics exceed defined thresholds in the period following an automated release.

## Pipeline Configuration

Pipeline configuration is stored as code alongside the application source in a
version-controlled repository. Configuration-as-code ensures that all changes
to the pipeline are reviewed, audited, and reproducible. Common pipeline
configuration formats include YAML manifests interpreted by the CI platform and
domain-specific languages compiled into execution graphs at runtime.`

// ---------------------------------------------------------------------------
// Backward compatibility: nil model keeps the existing BM25+trigram pipeline.
// ---------------------------------------------------------------------------

// NewIndex(nil) must not panic and must return a usable index.
func TestIndex_NilModel_NewIndexSucceeds(t *testing.T) {
	idx := index.NewIndex(nil)
	require.NotNil(t, idx)
}

// With a nil model the basic keyword search pipeline is unchanged.
func TestIndex_NilModel_KeywordSearchWorks(t *testing.T) {
	idx := index.NewIndex(nil)
	idx.Add(pages.Parse("Deployment Guide",
		[]byte("# Deployment Guide\n\nThis guide covers deployment strategies for production services.")))

	results := idx.Search("deployment", 10)

	require.NotEmpty(t, results)
	assert.Equal(t, "Deployment Guide", results[0].Page)
	assert.True(t, results[0].IsDirect)
}

// With a nil model and fewer than 3 BM25 results, trigram fallback still runs.
func TestIndex_NilModel_TrigramFallbackRunsWhenBM25ResultsAreFew(t *testing.T) {
	idx := index.NewIndex(nil)
	idx.Add(pages.Parse("Kubernetes Guide",
		[]byte("# Kubernetes Guide\n\n"+kubernetesBody)))

	// "Kubernetez" is a close trigram-neighbour of "Kubernetes" but produces
	// zero BM25 hits. Trigram fallback should surface the page.
	results := idx.Search("Kubernetez", 10)
	names := resultPageNames(results)

	assert.Contains(t, names, "Kubernetes Guide",
		"trigram fallback must fire when BM25 returns <3 results and model is nil")
}

// ---------------------------------------------------------------------------
// Add / Remove keep the vector index in sync.
// ---------------------------------------------------------------------------

// After Add, the page is findable via a semantically related query (not exact keyword).
func TestIndex_WithModel_AddSyncsVectorIndex(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	idx.Add(pages.Parse("Container Orchestration",
		[]byte("# Container Orchestration\n\n"+kubernetesBody)))

	results := idx.Search("pod scheduling and cluster workloads", 10)
	names := resultPageNames(results)

	assert.Contains(t, names, "Container Orchestration",
		"vector search must find the page after Add")
}

// After Remove, the page no longer appears in vector-driven results.
func TestIndex_WithModel_RemoveSyncsVectorIndex(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	idx.Add(pages.Parse("Container Orchestration",
		[]byte("# Container Orchestration\n\n"+kubernetesBody)))
	idx.Remove("Container Orchestration")

	results := idx.Search("pod scheduling and cluster workloads", 10)
	names := resultPageNames(results)

	assert.NotContains(t, names, "Container Orchestration",
		"vector search must not find the page after Remove")
}

// Re-adding a page (replacing it) does not cause duplicates.
func TestIndex_WithModel_AddReplaceDoesNotDuplicate(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	p := pages.Parse("Container Orchestration",
		[]byte("# Container Orchestration\n\n"+kubernetesBody))
	idx.Add(p)
	idx.Add(p) // add again — replaces

	results := idx.Search("pod scheduling and cluster workloads", 10)

	count := 0
	for _, r := range results {
		if r.Page == "Container Orchestration" {
			count++
		}
	}
	assert.Equal(t, 1, count,
		"replacing a page must produce exactly one result, not a duplicate")
}

// ---------------------------------------------------------------------------
// Vector search pipeline: semantic discovery.
// ---------------------------------------------------------------------------

// A page whose body contains none of the exact query tokens should still be
// found when its semantic content matches the query intent.
func TestIndex_WithModel_FindsSemanticMatch(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	// "CI/CD Pipeline" never contains the words "deployment strategy".
	idx.Add(pages.Parse("CI/CD Pipeline",
		[]byte("# CI/CD Pipeline\n\n"+cicdBody)))
	// Semantically unrelated page — should not appear.
	idx.Add(pages.Parse("Database Backups",
		[]byte("# Database Backups\n\n"+databaseBody)))

	results := idx.Search("deployment strategy", 10)
	names := resultPageNames(results)

	assert.Contains(t, names, "CI/CD Pipeline",
		"vector search must surface CI/CD Pipeline for 'deployment strategy' via semantic similarity")
	assert.NotContains(t, names, "Database Backups",
		"semantically unrelated page must not appear")
}

// ---------------------------------------------------------------------------
// IsDirect flag on merged results.
// ---------------------------------------------------------------------------

// A page that matches only via vector (no BM25 keyword hit) must have IsDirect=true
// because it is a direct semantic match, not merely a graph-boosted neighbour.
func TestIndex_WithModel_VectorOnlyMatchIsIsDirect(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	// cicdBody is about CI/CD pipelines but deliberately contains neither
	// "deployment" nor "strategy" — BM25 cannot match the query below, so
	// any result comes exclusively from the vector path.
	idx.Add(pages.Parse("CI/CD Pipeline",
		[]byte("# CI/CD Pipeline\n\n"+cicdBody)))
	idx.Add(pages.Parse("Database Backups",
		[]byte("# Database Backups\n\n"+databaseBody)))

	// "deployment strategy" appears in neither page body — vector-only match.
	results := idx.Search("deployment strategy", 10)

	var cicdResult *index.Result
	for i := range results {
		if results[i].Page == "CI/CD Pipeline" {
			cicdResult = &results[i]
			break
		}
	}
	require.NotNil(t, cicdResult,
		"expected CI/CD Pipeline in results for semantic query 'deployment strategy'")
	assert.True(t, cicdResult.IsDirect,
		"vector-only match must be IsDirect=true")
}

// A page returned by BM25 (direct keyword hit) must also be IsDirect=true.
func TestIndex_WithModel_BM25MatchIsIsDirect(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	idx.Add(pages.Parse("Kubernetes Ops",
		[]byte("# Kubernetes Ops\n\n"+kubernetesBody)))

	// Exact keyword match.
	results := idx.Search("kubernetes pods", 10)

	require.NotEmpty(t, results)
	assert.True(t, results[0].IsDirect,
		"BM25 direct keyword match must be IsDirect=true")
}

// Graph boost must still apply when the model is wired.
// A page linked from a direct-match page must be boosted into results and
// must carry IsDirect=false (it is not itself a semantic or keyword match).
func TestIndex_WithModel_GraphBoostStillApplies(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	// "Kubernetes Ops" is a direct keyword and semantic match for the query.
	// "Ops Runbook" links to "Kubernetes Ops" but contains none of the search
	// terms — it should appear only via graph boost.
	idx.Add(pages.Parse("Kubernetes Ops",
		[]byte("# Kubernetes Ops\n\n"+kubernetesBody)))
	idx.Add(pages.Parse("Ops Runbook",
		[]byte("# Ops Runbook\n\nSee [[Kubernetes Ops]] for container configuration details.")))

	results := idx.Search("kubernetes pods container", 10)
	names := resultPageNames(results)

	assert.Contains(t, names, "Kubernetes Ops",
		"direct match must appear in results")
	assert.Contains(t, names, "Ops Runbook",
		"page linked from a direct match must be boosted into results by the graph")

	// The graph-boosted page is not a direct semantic or keyword match.
	for _, r := range results {
		if r.Page == "Ops Runbook" {
			assert.False(t, r.IsDirect,
				"graph-boosted page that is not a direct match must have IsDirect=false")
		}
	}
}

// ---------------------------------------------------------------------------
// Merge deduplication.
// ---------------------------------------------------------------------------

// A page that appears in both BM25 and vector results must appear exactly once.
func TestIndex_WithModel_PageInBothResultsetsDeduplicatedToOne(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	// Strong BM25 match (title contains exact keywords) and strong semantic match.
	idx.Add(pages.Parse("Kubernetes Deployment",
		[]byte("# Kubernetes Deployment\n\n"+kubernetesBody)))
	idx.Add(pages.Parse("Database Backups",
		[]byte("# Database Backups\n\n"+databaseBody)))

	results := idx.Search("kubernetes deployment", 10)

	count := 0
	for _, r := range results {
		if r.Page == "Kubernetes Deployment" {
			count++
		}
	}
	assert.Equal(t, 1, count,
		"a page in both BM25 and vector results must be deduplicated to one entry")
}

// ---------------------------------------------------------------------------
// Score normalization and ordering.
// ---------------------------------------------------------------------------

// All result scores must be in [0, 1] after normalization.
func TestIndex_WithModel_MergedScoresAreNormalized(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	idx.Add(pages.Parse("Kubernetes Ops",
		[]byte("# Kubernetes Ops\n\n"+kubernetesBody)))
	idx.Add(pages.Parse("Database Backups",
		[]byte("# Database Backups\n\n"+databaseBody)))

	results := idx.Search("kubernetes container pods", 10)
	require.NotEmpty(t, results)

	for _, r := range results {
		assert.GreaterOrEqual(t, r.Score, 0.0,
			"score for %q must be >= 0.0 (got %f)", r.Page, r.Score)
		assert.LessOrEqual(t, r.Score, 1.0,
			"score for %q must be <= 1.0 (got %f)", r.Page, r.Score)
	}
}

// The highest-ranked result must have a score of exactly 1.0 (top score
// after normalisation).
func TestIndex_WithModel_TopResultScoreIsOne(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	idx.Add(pages.Parse("Kubernetes Ops",
		[]byte("# Kubernetes Ops\n\n"+kubernetesBody)))
	idx.Add(pages.Parse("Database Backups",
		[]byte("# Database Backups\n\n"+databaseBody)))

	results := idx.Search("kubernetes container pods", 10)
	require.NotEmpty(t, results)

	assert.InDelta(t, 1.0, results[0].Score, 0.001,
		"top result must have score 1.0 after normalization")
}

// Results must be ordered by score descending.
func TestIndex_WithModel_ResultsOrderedByScoreDescending(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	idx.Add(pages.Parse("Kubernetes Ops",
		[]byte("# Kubernetes Ops\n\n"+kubernetesBody)))
	idx.Add(pages.Parse("Database Backups",
		[]byte("# Database Backups\n\n"+databaseBody)))

	results := idx.Search("kubernetes container pods", 10)
	require.NotEmpty(t, results)

	for i := 1; i < len(results); i++ {
		assert.GreaterOrEqual(t, results[i-1].Score, results[i].Score,
			"results[%d].Score=%f must be >= results[%d].Score=%f",
			i-1, results[i-1].Score, i, results[i].Score)
	}
}

// ---------------------------------------------------------------------------
// Relevance threshold (50% of top score still applies after merge).
// ---------------------------------------------------------------------------

func TestIndex_WithModel_RelevanceThresholdDropsWeakResults(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	idx.Add(pages.Parse("Kubernetes Ops",
		[]byte("# Kubernetes Ops\n\n"+kubernetesBody)))
	idx.Add(pages.Parse("Database Backups",
		[]byte("# Database Backups\n\n"+databaseBody)))

	// Strongly kubernetes-flavored query — database backups page should be
	// well below 50% of top score.
	results := idx.Search("kubernetes pods container orchestration cluster scheduler", 10)
	require.NotEmpty(t, results)

	topScore := results[0].Score
	for _, r := range results {
		assert.GreaterOrEqual(t, r.Score, topScore*0.5-0.001,
			"page %q with score %f is below the 50%% relevance threshold (top=%f)",
			r.Page, r.Score, topScore)
	}
}

// ---------------------------------------------------------------------------
// Snippet population.
// ---------------------------------------------------------------------------

// All results — including pages matched only via vector — must carry a
// non-empty snippet.
func TestIndex_WithModel_AllResultsHaveSnippets(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	idx.Add(pages.Parse("CI/CD Pipeline",
		[]byte("# CI/CD Pipeline\n\n"+cicdBody)))
	idx.Add(pages.Parse("Database Backups",
		[]byte("# Database Backups\n\n"+databaseBody)))

	// Semantic query — CI/CD Pipeline matched via vector only.
	results := idx.Search("release pipeline automation", 10)
	require.NotEmpty(t, results)

	for _, r := range results {
		assert.NotEmpty(t, r.Snippet,
			"result for page %q must have a non-empty snippet", r.Page)
	}
}

// Line field on vector-only results must be a valid 1-indexed line number.
func TestIndex_WithModel_VectorOnlyResultHasValidLine(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	idx.Add(pages.Parse("CI/CD Pipeline",
		[]byte("# CI/CD Pipeline\n\n"+cicdBody)))

	results := idx.Search("release pipeline automation", 10)

	for _, r := range results {
		if r.Page == "CI/CD Pipeline" {
			assert.GreaterOrEqual(t, r.Line, 1,
				"Line must be 1-indexed (>=1) for page %q", r.Page)
		}
	}
}

// ---------------------------------------------------------------------------
// Output format is unchanged.
// ---------------------------------------------------------------------------

// The Result struct fields must all be present (format contract with callers).
func TestIndex_WithModel_ResultFormatUnchanged(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	idx.Add(pages.Parse("Kubernetes Ops",
		[]byte("# Kubernetes Ops\n\n"+kubernetesBody)))

	results := idx.Search("kubernetes pods", 10)
	require.NotEmpty(t, results)

	r := results[0]
	assert.NotEmpty(t, r.Page, "Page field must be populated")
	assert.Greater(t, r.Score, 0.0, "Score must be positive")
	assert.NotEmpty(t, r.Snippet, "Snippet must be populated")
	// Line is 0 for BM25-only results on some implementations; >= 0 is the contract.
	assert.GreaterOrEqual(t, r.Line, 0, "Line must be non-negative")
	// IsDirect is a bool — checking it is set (true for direct matches) elsewhere.
	_ = r.IsDirect
}

// ---------------------------------------------------------------------------
// Trigram does NOT run when vector is enabled.
// ---------------------------------------------------------------------------

// When a model is present, a typo query that BM25 cannot match should not
// surface results via trigram — only via vector. A page that is semantically
// unrelated to the typo'd term should not appear just because its title is a
// trigram-close neighbour.
//
// We test this by checking that a semantically unrelated page does NOT appear
// when the query is a typo of a different, semantically close page's title.
func TestIndex_WithModel_TrigramDoesNotRunWhenVectorEnabled(t *testing.T) {
	model := getVectorModel(t)
	idx := index.NewIndex(model)

	// Two pages: one about kubernetes, one about databases.
	// "Kubernetez" (typo) is trigram-close to "Kubernetes" and might trigger
	// trigram to surface the database page if trigram were to run (it wouldn't
	// for the kubernetes page, but trigram shouldn't run at all).
	// The key check: with vector enabled, results come from semantic similarity,
	// not character trigram proximity.
	idx.Add(pages.Parse("Kubernetes Guide",
		[]byte("# Kubernetes Guide\n\n"+kubernetesBody)))
	idx.Add(pages.Parse("Database Backups",
		[]byte("# Database Backups\n\n"+databaseBody)))

	// With vector enabled, "Kubernetez" should still reach the semantically
	// correct page (Kubernetes Guide) via vector, not via trigram.
	results := idx.Search("Kubernetez", 10)

	// Database Backups must not appear — it has no semantic or keyword relation.
	names := resultPageNames(results)
	assert.NotContains(t, names, "Database Backups",
		"with vector enabled, trigram must not fire and surface an unrelated page")
}
