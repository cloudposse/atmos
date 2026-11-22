package gcp

// Tests for the GKE kubeconfig exec-user names shared by the writer
// (BuildKubeClusterInfo) and GKE integration cleanup.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
)

// TestGKEKubeUserName_MatchesWrittenUser verifies that cleanup computes exactly
// the user name the kubeconfig writer used, with and without an identity.
func TestGKEKubeUserName_MatchesWrittenUser(t *testing.T) {
	tests := []struct {
		name     string
		identity string
		want     string
	}{
		{name: "identity", identity: "gcp-admin", want: "atmos-gke-example-cluster-example-project-us-central1-gcp-admin"},
		{name: "no identity", want: "atmos-gke-example-cluster-example-project-us-central1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			written := BuildKubeClusterInfo(&GKEClusterInfo{Name: "example-cluster", ProjectID: "example-project", Location: "us-central1"}, tt.identity)

			assert.Equal(t, tt.identity, written.Identity)
			assert.Equal(t, tt.want, kube.UserName(written))
			assert.Equal(t, kube.UserName(written), GKEKubeUserName("example-project", "us-central1", "example-cluster", tt.identity, ""))
		})
	}
}

// TestLegacyGKEKubeUserName verifies the name versions before the identity was added wrote.
func TestLegacyGKEKubeUserName(t *testing.T) {
	assert.Equal(t, "atmos-gke-example-cluster-example-project-us-central1", LegacyGKEKubeUserName("example-project", "us-central1", "example-cluster"))
}

// TestGKEKubeUserNames verifies the cleanup candidates: the current name first,
// then the pre-realm name (only with a realm) and the legacy name.
func TestGKEKubeUserNames(t *testing.T) {
	assert.Equal(t, []string{
		"atmos-acme-gke-example-cluster-example-project-us-central1-gcp-admin",
		"atmos-gke-example-cluster-example-project-us-central1-gcp-admin",
		"atmos-gke-example-cluster-example-project-us-central1",
	}, GKEKubeUserNames("example-project", "us-central1", "example-cluster", "gcp-admin", "acme"))
	assert.Equal(t, []string{
		"atmos-gke-example-cluster-example-project-us-central1-gcp-admin",
		"atmos-gke-example-cluster-example-project-us-central1",
	}, GKEKubeUserNames("example-project", "us-central1", "example-cluster", "gcp-admin", ""))
}
