package index_test

// Shared test helpers and fixtures for the index package test suite.
//
// Anything used by more than one _test.go file in this package belongs here:
// model loading, and semantically rich page bodies used across vector and
// pipeline tests.

import (
	"sync"
	"testing"

	"github.com/edgetools/memento/embed"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Model loading — lazy, once per test binary, fail on error.
// Using sync.Once to load the model exactly once regardless of how many test
// files call getVectorModel, without requiring a shared TestMain.
// ---------------------------------------------------------------------------

var (
	vectorModel    *embed.Model
	vectorOnce     sync.Once
	vectorModelErr error
)

// getVectorModel loads the embedding model on first call and caches it.
// The test fails (not skips) when the model cannot be loaded — the model is
// required for production operation and must be available in all test environments.
func getVectorModel(t *testing.T) *embed.Model {
	t.Helper()
	vectorOnce.Do(func() {
		vectorModel, vectorModelErr = embed.LoadModel()
	})
	require.NoError(t, vectorModelErr, "embedding model must be loadable — ensure the HuggingFace cache is populated")
	return vectorModel
}

// ---------------------------------------------------------------------------
// Shared page body fixtures.
//
// Each body has three ##-headed sections with well over 50 words per section
// so that ChunkPage never merges them away. The two topic domains (containers
// vs. databases) are semantically far apart, making similarity assertions
// reliable across both vector and pipeline tests.
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
