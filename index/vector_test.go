package index_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/edgetools/memento/embed"
	"github.com/edgetools/memento/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Model loading — lazy, once per test binary, skip on failure.
// Using sync.Once rather than TestMain so that non-vector tests (BM25, chunk,
// graph) are unaffected when the embedding model is unavailable.
// ---------------------------------------------------------------------------

var (
	vectorModel    *embed.Model
	vectorOnce     sync.Once
	vectorModelErr error
)

// getVectorModel loads the embedding model on first call and caches it.
// The test is skipped (not failed) when the model cannot be loaded so that
// CI environments without a HuggingFace cache do not break unrelated tests.
func getVectorModel(t *testing.T) *embed.Model {
	t.Helper()
	vectorOnce.Do(func() {
		vectorModel, vectorModelErr = embed.LoadModel()
	})
	if vectorModelErr != nil {
		t.Skipf("embedding model unavailable, skipping vector index test: %v", vectorModelErr)
	}
	return vectorModel
}

// ---------------------------------------------------------------------------
// Test content — each section has well over 50 words so that ChunkPage never
// merges them away, and the two topic domains are semantically far apart so
// that similarity assertions are reliable.
// ---------------------------------------------------------------------------

// kubernetesBody: three ##-headed sections about container orchestration.
const kubernetesBody = `## Container Deployment

Kubernetes is an open-source container orchestration platform that automates the deployment, scaling, and management of containerized applications across clusters of machines. Pods are the smallest deployable units in Kubernetes, each containing one or more containers that share the same network namespace, IP address, and storage volumes. The Kubernetes scheduler places pods onto nodes based on resource availability and the scheduling constraints defined in each pod specification manifest.

## Pod Management

Managing pods in Kubernetes involves carefully defining resource requests and limits along with configuring liveness and readiness health check probes for every container. The kubelet agent running on each worker node ensures that containers are running and healthy according to the pod specification defined in YAML manifests. When a pod fails its configured health checks the kubelet restarts the container according to the restart policy specified in the workload definition.

## Service Discovery

Kubernetes services provide stable network endpoints for dynamically scheduled pods across the cluster. A service selects target pods using label selectors and automatically load balances incoming traffic across all healthy pods that satisfy the selector. ClusterIP, NodePort, LoadBalancer, and ExternalName are the four service types available in Kubernetes, each suited to a different networking scenario and access pattern.`

// databaseBody: three ##-headed sections about database backup — unrelated to containers.
const databaseBody = `## PostgreSQL Backup Strategy

Database backup is essential for data protection and disaster recovery in production systems. PostgreSQL supports both logical backups using pg_dump and physical backups using pg_basebackup, each with distinct performance and restoration characteristics. A comprehensive backup strategy combines regular full backups with incremental WAL archiving to support point-in-time recovery across any interval within the retention window.

## Recovery Procedures

When a database failure occurs, the recovery process involves first restoring the most recent full backup and then replaying archived WAL segments forward to reach a consistent and current state. Testing recovery procedures on a regular schedule ensures that backups are actually valid and that recovery time objectives can be reliably met before an incident occurs in production.

## Backup Retention Policy

Retaining database backups for appropriate durations depends on business continuity requirements and available storage capacity budgets. A typical retention policy keeps daily backups for thirty days, weekly backups for three months, and monthly backups for one year. Automated expiry tooling removes outdated backup files so that storage costs remain predictable without manual intervention by the database administrator.`

// multiSectionDeployBody: three ##-headed sections all about software deployment and releases.
// Used to verify per-page deduplication: all three chunks may match a deployment query but
// the page should appear at most once in results.
const multiSectionDeployBody = `## Release Process

A software deployment release process coordinates changes from development environments through staging to production in a controlled and auditable manner. Teams employ deployment strategies specifically designed to minimize downtime and reduce risk during releases to production servers and distributed infrastructure. Documentation of each release step helps ensure repeatability and supports post-incident review when problems arise during rollout.

## Deployment Strategies

Blue-green deployment maintains two identical production environments running simultaneously. Traffic switches from the blue environment to the green environment during a release, enabling instant rollback by reversing the traffic switch if problems are detected. Canary deployment gradually shifts a small percentage of live traffic to the new release version while monitoring error rates, latency, and business metrics before committing to a full rollout across all production capacity.

## Rollback Procedures

When a deployment release encounters critical issues, rollback procedures must restore the previous stable version as quickly as possible to minimize user impact. Automated rollback triggered by error rate thresholds or latency spikes reduces the mean time to recovery and removes the need for manual operator intervention during an incident. Practicing rollback drills during non-critical maintenance windows ensures that teams can execute the procedure reliably when it matters most under production pressure.`

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

func TestNewVectorIndex(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)
	require.NotNil(t, vi)
}

// ---------------------------------------------------------------------------
// Empty-index safety
// ---------------------------------------------------------------------------

func TestVectorIndex_SearchEmptyReturnsNil(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)

	results := vi.Search("kubernetes deployment", 10)

	assert.Nil(t, results, "searching an empty index must return nil, not an empty slice")
}

func TestVectorIndex_RemoveFromEmptyNoPanic(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)

	require.NotPanics(t, func() {
		vi.Remove("nonexistent page")
	}, "Remove on an empty index must not panic")
}

func TestVectorIndex_AddToFreshIndexReturnsNoError(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)

	err := vi.Add(makePage("Kubernetes Guide", kubernetesBody, nil))

	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Search correctness
// ---------------------------------------------------------------------------

func TestVectorIndex_SearchFindsSemanticMatch(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)

	require.NoError(t, vi.Add(makePage("Container Orchestration", kubernetesBody, nil)))
	require.NoError(t, vi.Add(makePage("Database Backup", databaseBody, nil)))

	// "kubernetes pod container" is semantically close to kubernetesBody and far from databaseBody.
	results := vi.Search("kubernetes pod container", 10)

	require.NotEmpty(t, results, "should find at least one result for a clear semantic match")
	found := false
	for _, r := range results {
		if strings.EqualFold(r.Page, "Container Orchestration") {
			found = true
			break
		}
	}
	assert.True(t, found, "'Container Orchestration' must appear in results for 'kubernetes pod container'")
}

func TestVectorIndex_SearchScoreRange(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)
	require.NoError(t, vi.Add(makePage("Container Orchestration", kubernetesBody, nil)))

	results := vi.Search("container deployment platform", 10)

	require.NotEmpty(t, results)
	for _, r := range results {
		assert.GreaterOrEqual(t, r.Score, -1.0,
			"cosine similarity must be >= -1; got %v for page %q", r.Score, r.Page)
		assert.LessOrEqual(t, r.Score, 1.0,
			"cosine similarity must be <= 1; got %v for page %q", r.Score, r.Page)
	}
}

func TestVectorIndex_SearchRespectsLimit(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)

	for _, name := range []string{"Page A", "Page B", "Page C", "Page D", "Page E"} {
		require.NoError(t, vi.Add(makePage(name, kubernetesBody, nil)))
	}

	results := vi.Search("container orchestration kubernetes", 3)

	assert.LessOrEqual(t, len(results), 3, "Search must honour the limit parameter")
}

func TestVectorIndex_SearchResultsSortedByScoreDescending(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)

	require.NoError(t, vi.Add(makePage("Container Orchestration", kubernetesBody, nil)))
	require.NoError(t, vi.Add(makePage("Database Backup", databaseBody, nil)))

	results := vi.Search("container deployment kubernetes", 10)

	require.NotEmpty(t, results)
	for i := 1; i < len(results); i++ {
		assert.GreaterOrEqual(t, results[i-1].Score, results[i].Score,
			"results[%d].Score (%v) must be >= results[%d].Score (%v)",
			i-1, results[i-1].Score, i, results[i].Score)
	}
}

// ---------------------------------------------------------------------------
// Per-page deduplication
// ---------------------------------------------------------------------------

func TestVectorIndex_SearchDeduplicatesPageToSingleResult(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)

	// multiSectionDeployBody has three ## sections — ChunkPage produces ≥ 2 chunks.
	// All three sections are about deployment, so multiple chunks may match the query.
	require.NoError(t, vi.Add(makePage("Deployment Guide", multiSectionDeployBody, nil)))

	results := vi.Search("deployment release strategy", 10)

	count := 0
	for _, r := range results {
		if strings.EqualFold(r.Page, "Deployment Guide") {
			count++
		}
	}
	assert.LessOrEqual(t, count, 1,
		"a page must appear at most once in results regardless of how many chunks matched")
}

func TestVectorIndex_SearchDeduplicatesKeepsBestChunkScoreAndLine(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)
	require.NoError(t, vi.Add(makePage("Deployment Guide", multiSectionDeployBody, nil)))

	results := vi.Search("software deployment release process", 10)

	for _, r := range results {
		if strings.EqualFold(r.Page, "Deployment Guide") {
			assert.GreaterOrEqual(t, r.Score, -1.0, "deduplicated score must be a valid cosine similarity")
			assert.LessOrEqual(t, r.Score, 1.0, "deduplicated score must be a valid cosine similarity")
			assert.Greater(t, r.Line, 0, "Line must be the 1-indexed start line of the best matching chunk")
		}
	}
}

// ---------------------------------------------------------------------------
// VectorResult field coverage
// ---------------------------------------------------------------------------

func TestVectorIndex_SearchResultsHavePopulatedFields(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)
	require.NoError(t, vi.Add(makePage("Container Orchestration", kubernetesBody, nil)))

	results := vi.Search("container deployment", 10)

	require.NotEmpty(t, results)
	for _, r := range results {
		assert.NotEmpty(t, r.Page, "VectorResult.Page must not be empty")
		assert.Greater(t, r.Line, 0, "VectorResult.Line must be a positive 1-indexed line number")
		assert.GreaterOrEqual(t, r.Score, -1.0, "VectorResult.Score must be a valid cosine similarity (>= -1)")
		assert.LessOrEqual(t, r.Score, 1.0, "VectorResult.Score must be a valid cosine similarity (<= 1)")
	}
}

// ---------------------------------------------------------------------------
// Remove
// ---------------------------------------------------------------------------

func TestVectorIndex_RemoveStopsPageFromAppearing(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)
	require.NoError(t, vi.Add(makePage("Container Orchestration", kubernetesBody, nil)))

	// Confirm the page is findable before removal.
	before := vi.Search("container orchestration kubernetes", 10)
	require.NotEmpty(t, before, "page must be findable before Remove is called")

	vi.Remove("Container Orchestration")

	after := vi.Search("container orchestration kubernetes", 10)
	for _, r := range after {
		assert.False(t, strings.EqualFold(r.Page, "Container Orchestration"),
			"removed page must not appear in search results")
	}
}

func TestVectorIndex_RemoveIsCaseInsensitive(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)
	require.NoError(t, vi.Add(makePage("Container Orchestration", kubernetesBody, nil)))

	// Remove using all-lowercase variant of the name.
	vi.Remove("container orchestration")

	results := vi.Search("container orchestration kubernetes", 10)
	for _, r := range results {
		assert.False(t, strings.EqualFold(r.Page, "Container Orchestration"),
			"case-insensitive Remove must remove the page regardless of the casing used in the call")
	}
}

// ---------------------------------------------------------------------------
// Add — replace behaviour
// ---------------------------------------------------------------------------

func TestVectorIndex_AddReplacesChunksForExistingPage(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)

	// Add a stable kubernetes reference page that stays unchanged throughout.
	require.NoError(t, vi.Add(makePage("Reference K8s", kubernetesBody, nil)))

	// Index "Evolving Page" with kubernetes content initially.
	require.NoError(t, vi.Add(makePage("Evolving Page", kubernetesBody, nil)))

	// Overwrite with completely different database content.
	require.NoError(t, vi.Add(makePage("Evolving Page", databaseBody, nil)))

	// A database query should now find "Evolving Page".
	dbResults := vi.Search("postgresql database backup strategy", 10)
	found := false
	for _, r := range dbResults {
		if strings.EqualFold(r.Page, "Evolving Page") {
			found = true
			break
		}
	}
	assert.True(t, found,
		"after re-Add with new content, page must be findable via the new topic's queries")

	// For a kubernetes query, "Reference K8s" (unchanged kubernetes content) must
	// outscore "Evolving Page" (now database content). This proves the old kubernetes
	// chunks for "Evolving Page" were removed — not merely supplemented by the new ones.
	// An append-only implementation would retain the old kubernetes vectors, causing
	// "Evolving Page" to score equally with "Reference K8s", failing this check.
	k8sResults := vi.Search("kubernetes pod container orchestration", 10)
	var evolvingScore, referenceScore float64
	evolvingFound, referenceFound := false, false
	for _, r := range k8sResults {
		if strings.EqualFold(r.Page, "Evolving Page") {
			evolvingScore = r.Score
			evolvingFound = true
		}
		if strings.EqualFold(r.Page, "Reference K8s") {
			referenceScore = r.Score
			referenceFound = true
		}
	}
	require.True(t, referenceFound,
		"'Reference K8s' (kubernetes content) must appear in a kubernetes query result")
	if evolvingFound {
		assert.Greater(t, referenceScore, evolvingScore,
			"'Reference K8s' must outrank 'Evolving Page' for a kubernetes query after "+
				"'Evolving Page' was re-indexed with database content; old chunks must be gone")
	}
}

func TestVectorIndex_AddCaseInsensitiveReplaceProducesNoDuplicates(t *testing.T) {
	model := getVectorModel(t)
	vi := index.NewVectorIndex(model)

	require.NoError(t, vi.Add(makePage("My Page", kubernetesBody, nil)))

	// Adding the same page with different casing must replace, not append.
	require.NoError(t, vi.Add(makePage("MY PAGE", kubernetesBody, nil)))

	results := vi.Search("container kubernetes", 10)

	count := 0
	for _, r := range results {
		if strings.EqualFold(r.Page, "my page") {
			count++
		}
	}
	assert.LessOrEqual(t, count, 1,
		"re-adding a page under a different case must not create duplicate index entries")
}
