package gcp

// Tests that GKE kubeconfig contexts written for different identities get
// their own exec user, and that Cleanup removes only its own entries,
// including the user name older versions wrote (issue #3720, the GKE
// counterpart of EKS finding 2.5).

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	gcpCloud "github.com/cloudposse/atmos/pkg/auth/cloud/gcp"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	exampleClusterID      = "projects/example-project/locations/us-central1/clusters/example-cluster"
	legacyExampleUserName = "atmos-gke-example-cluster-example-project-us-central1"
)

// stubExampleGKECluster makes Execute describe the example cluster without GCP calls.
func stubExampleGKECluster(t *testing.T) {
	t.Helper()

	origFactory, origDescribe := gkeClientFactory, gkeDescribeCluster
	t.Cleanup(func() { gkeClientFactory, gkeDescribeCluster = origFactory, origDescribe })
	gkeExpectedServers.Clear()
	t.Cleanup(gkeExpectedServers.Clear)
	gkeClientFactory = func(context.Context, types.ICredentials) (gcpCloud.GKEClient, error) {
		return nil, nil
	}
	gkeDescribeCluster = func(_ context.Context, _ gcpCloud.GKEClient, projectID, location, name string) (*gcpCloud.GKEClusterInfo, error) {
		return &gcpCloud.GKEClusterInfo{
			Name: name, ProjectID: projectID, Location: location,
			ID:       gcpCloud.ClusterResourceName(projectID, location, name),
			Endpoint: "https://gke.example.invalid", CertificateAuthorityData: "dGVzdA==",
		}, nil
	}
}

// exampleGCPCredentials returns unexpired credentials that Execute accepts.
func exampleGCPCredentials() *types.GCPCredentials {
	return &types.GCPCredentials{AccessToken: "token", TokenExpiry: time.Now().Add(time.Hour)}
}

// newScopedGKEIntegration builds a GKE integration through the factory, as
// the auth manager does, with the given alias, identity and realm.
func newScopedGKEIntegration(t *testing.T, path, alias, identity, realm string) integrations.Integration {
	t.Helper()

	integration, err := NewGKEIntegration(&integrations.IntegrationConfig{
		Name:  "example-gke",
		Realm: realm,
		Config: &schema.Integration{
			Kind: integrations.KindGCPGKE,
			Via:  &schema.IntegrationVia{Identity: identity},
			Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{
				Name: "example-cluster", ProjectID: "example-project", Location: "us-central1", Alias: alias,
				Kubeconfig: &schema.KubeconfigSettings{Path: path},
			}},
		},
	})
	require.NoError(t, err)
	return integration
}

// TestGKEIntegration_Execute_DistinctUserPerIdentity runs two integrations for
// the same cluster with different identities and verifies that each context
// keeps running as its own identity.
func TestGKEIntegration_Execute_DistinctUserPerIdentity(t *testing.T) {
	stubExampleGKECluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "example-admin", "gcp-admin", "").Execute(t.Context(), exampleGCPCredentials()))
	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "example-reader", "gcp-reader", "").Execute(t.Context(), exampleGCPCredentials()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	adminUser := config.Contexts["example-admin"].AuthInfo
	readerUser := config.Contexts["example-reader"].AuthInfo
	assert.NotEqual(t, adminUser, readerUser, "contexts for different identities must not share an exec user")
	assert.Equal(t, legacyExampleUserName+"-gcp-admin", adminUser)
	assert.Equal(t, legacyExampleUserName+"-gcp-reader", readerUser)
	assert.Contains(t, config.AuthInfos[adminUser].Exec.Args, "--identity=gcp-admin")
	assert.Contains(t, config.AuthInfos[readerUser].Exec.Args, "--identity=gcp-reader")
}

// TestGKEIntegration_Cleanup_KeepsOtherIdentityContext verifies that cleaning
// up one identity's integration leaves the other identity's context, its user
// and the shared cluster entry in place.
func TestGKEIntegration_Cleanup_KeepsOtherIdentityContext(t *testing.T) {
	stubExampleGKECluster(t)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "example-admin", "gcp-admin", "").Execute(t.Context(), exampleGCPCredentials()))
	reader := newScopedGKEIntegration(t, kubeconfigPath, "example-reader", "gcp-reader", "")
	require.NoError(t, reader.Execute(t.Context(), exampleGCPCredentials()))

	require.NoError(t, reader.Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, "example-reader")
	assert.NotContains(t, config.AuthInfos, legacyExampleUserName+"-gcp-reader")
	assert.Contains(t, config.Clusters, exampleClusterID, "the cluster is still used by the example-admin context")
	assert.Contains(t, config.AuthInfos[config.Contexts["example-admin"].AuthInfo].Exec.Args, "--identity=gcp-admin")
}

// writeLegacyGKEEntry writes an entry in the format older versions used: the
// exec user is named atmos-gke-<name>-<project>-<location>, without the identity.
func writeLegacyGKEEntry(t *testing.T, path, alias, identity string) {
	t.Helper()

	mgr, err := kube.NewKubeconfigManager(path, "")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(&kube.ClusterInfo{
		Name: "example-cluster", Endpoint: "https://gke.example.invalid", ID: exampleClusterID,
		Region: "example-project-us-central1", UserPrefix: "gke",
		ExecArgs: []string{"gcp", "gke", "token", "--identity=" + identity},
	}, alias, "merge")
	require.NoError(t, err)

	config, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	require.Contains(t, config.AuthInfos, legacyExampleUserName, "the fixture must use the legacy user name")
}

// TestGKEIntegration_Cleanup_RemovesLegacyUser verifies that Cleanup still
// removes an entry written by an older version with the legacy user name.
func TestGKEIntegration_Cleanup_RemovesLegacyUser(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeLegacyGKEEntry(t, kubeconfigPath, "example-admin", "gcp-admin")
	// An unrelated cluster keeps the file from being deleted, so the result can be inspected.
	mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(&kube.ClusterInfo{
		Name: "other", Endpoint: "https://other.example.invalid", ID: "other-id", Region: "other", UserPrefix: "gke",
	}, "other", "merge")
	require.NoError(t, err)

	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "example-admin", "gcp-admin", "").Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, "example-admin")
	assert.NotContains(t, config.Clusters, exampleClusterID)
	assert.NotContains(t, config.AuthInfos, legacyExampleUserName)
	assert.Contains(t, config.Contexts, "other")
}

// TestGKEIntegration_Cleanup_KeepsLegacyUserStillInUse verifies that a legacy
// user shared with another context is not removed with this integration's context.
func TestGKEIntegration_Cleanup_KeepsLegacyUserStillInUse(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	writeLegacyGKEEntry(t, kubeconfigPath, "example-admin", "gcp-admin")
	writeLegacyGKEEntry(t, kubeconfigPath, "example-reader", "gcp-reader")

	require.NoError(t, newScopedGKEIntegration(t, kubeconfigPath, "example-reader", "gcp-reader", "").Cleanup(t.Context()))

	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	require.NoError(t, err)
	assert.NotContains(t, config.Contexts, "example-reader")
	assert.Contains(t, config.Contexts, "example-admin")
	assert.Contains(t, config.AuthInfos, legacyExampleUserName, "the legacy user is still used by example-admin")
	assert.Contains(t, config.Clusters, exampleClusterID)
}
