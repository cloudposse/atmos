package gcp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	errUtils "github.com/cloudposse/atmos/errors"
	gcpCloud "github.com/cloudposse/atmos/pkg/auth/cloud/gcp"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func validGKEConfig(path string) *integrations.IntegrationConfig {
	return &integrations.IntegrationConfig{
		Name: "example-gke",
		Config: &schema.Integration{
			Kind: integrations.KindGCPGKE,
			Via:  &schema.IntegrationVia{Identity: "example-deployer"},
			Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{
				Name:      "example-cluster",
				ProjectID: "example-project",
				Location:  "us-central1",
				Alias:     "example",
				Kubeconfig: &schema.KubeconfigSettings{
					Path:   path,
					Update: "merge",
				},
			}},
		},
	}
}

func TestNewGKEIntegrationValidation(t *testing.T) {
	validCluster := func() *schema.Cluster {
		return &schema.Cluster{Name: "example-cluster", ProjectID: "example-project", Location: "us-central1"}
	}

	tests := []struct {
		name       string
		config     *integrations.IntegrationConfig
		wantErr    error
		wantDetail string
	}{
		{name: "nil config", wantErr: errUtils.ErrIntegrationNotFound, wantDetail: "config is nil"},
		{name: "nil schema config", config: &integrations.IntegrationConfig{Name: "example-gke"}, wantErr: errUtils.ErrIntegrationNotFound, wantDetail: "config is nil"},
		{name: "missing cluster", config: &integrations.IntegrationConfig{Name: "example-gke", Config: &schema.Integration{Spec: &schema.IntegrationSpec{}}}, wantErr: errUtils.ErrIntegrationFailed, wantDetail: "no cluster configured"},
		{name: "missing name", config: &integrations.IntegrationConfig{Name: "example-gke", Config: &schema.Integration{Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{ProjectID: "example-project", Location: "us-central1"}}}}, wantErr: errUtils.ErrIntegrationFailed, wantDetail: "no cluster name"},
		{name: "missing project", config: &integrations.IntegrationConfig{Name: "example-gke", Config: &schema.Integration{Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{Name: "example-cluster", Location: "us-central1"}}}}, wantErr: errUtils.ErrIntegrationFailed, wantDetail: "no project_id"},
		{name: "missing location", config: &integrations.IntegrationConfig{Name: "example-gke", Config: &schema.Integration{Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{Name: "example-cluster", ProjectID: "example-project"}}}}, wantErr: errUtils.ErrIntegrationFailed, wantDetail: "no location"},
		{name: "invalid mode", config: &integrations.IntegrationConfig{Name: "example-gke", Config: &schema.Integration{Spec: &schema.IntegrationSpec{Cluster: func() *schema.Cluster {
			c := validCluster()
			c.Kubeconfig = &schema.KubeconfigSettings{Mode: "invalid"}
			return c
		}()}}}, wantErr: errUtils.ErrIntegrationFailed, wantDetail: "invalid kubeconfig mode"},
		{name: "invalid update", config: &integrations.IntegrationConfig{Name: "example-gke", Config: &schema.Integration{Spec: &schema.IntegrationSpec{Cluster: func() *schema.Cluster {
			c := validCluster()
			c.Kubeconfig = &schema.KubeconfigSettings{Update: "invalid"}
			return c
		}()}}}, wantErr: errUtils.ErrIntegrationFailed, wantDetail: "invalid kubeconfig update mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGKEIntegration(tt.config)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Contains(t, err.Error(), tt.wantDetail)
		})
	}
}

func TestNewGKEIntegrationSuccess(t *testing.T) {
	integration, err := NewGKEIntegration(validGKEConfig(filepath.Join(t.TempDir(), "config")))
	require.NoError(t, err)
	gkeIntegration := integration.(*GKEIntegration)
	assert.Equal(t, integrations.KindGCPGKE, gkeIntegration.Kind())
	assert.Equal(t, "example-deployer", gkeIntegration.identity)
	assert.Equal(t, "example-project", gkeIntegration.cluster.ProjectID)
	assert.Equal(t, "us-central1", gkeIntegration.cluster.Location)
}

func TestGKEIntegrationRegistration(t *testing.T) {
	assert.True(t, integrations.IsRegistered(integrations.KindGCPGKE))
	integration, err := integrations.Create(validGKEConfig(filepath.Join(t.TempDir(), "config")))
	require.NoError(t, err)
	assert.Equal(t, integrations.KindGCPGKE, integration.Kind())
}

func TestGKEIntegrationEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	integration, err := NewGKEIntegration(validGKEConfig(path))
	require.NoError(t, err)
	env, err := integration.Environment()
	require.NoError(t, err)
	assert.Equal(t, path, env["KUBECONFIG"])
	assert.Equal(t, path, env["KUBE_CONFIG_PATH"])
}

func installGKEExecutionFakes(t *testing.T) {
	t.Helper()
	originalFactory := gkeClientFactory
	originalDescribe := gkeDescribeCluster
	t.Cleanup(func() {
		gkeClientFactory = originalFactory
		gkeDescribeCluster = originalDescribe
	})
	gkeClientFactory = func(_ context.Context, _ types.ICredentials) (gcpCloud.GKEClient, error) {
		return nil, nil
	}
	gkeDescribeCluster = func(_ context.Context, _ gcpCloud.GKEClient, projectID, location, name string) (*gcpCloud.GKEClusterInfo, error) {
		return &gcpCloud.GKEClusterInfo{
			Name:                     name,
			ProjectID:                projectID,
			Location:                 location,
			ID:                       gcpCloud.ClusterResourceName(projectID, location, name),
			Endpoint:                 "https://gke.example.invalid",
			CertificateAuthorityData: "dGVzdC1jYQ==",
		}, nil
	}
}

func TestGKEIntegrationExecuteAndCleanup(t *testing.T) {
	installGKEExecutionFakes(t)
	path := filepath.Join(t.TempDir(), "config")
	integration, err := NewGKEIntegration(validGKEConfig(path))
	require.NoError(t, err)
	creds := &types.GCPCredentials{AccessToken: "sensitive-access-token", TokenExpiry: time.Now().Add(time.Hour)}

	require.NoError(t, integration.Execute(t.Context(), creds))
	config, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	clusterID := gcpCloud.ClusterResourceName("example-project", "us-central1", "example-cluster")
	assert.Equal(t, "https://gke.example.invalid", config.Clusters[clusterID].Server)
	assert.Equal(t, []byte("test-ca"), config.Clusters[clusterID].CertificateAuthorityData)
	assert.Contains(t, config.Contexts, "example")
	authInfo := config.AuthInfos["atmos-gke-example-cluster-example-project-us-central1"]
	require.NotNil(t, authInfo)
	assert.Empty(t, authInfo.Token)
	require.NotNil(t, authInfo.Exec)
	assert.Equal(t, []string{"gcp", "gke", "token", "--identity=example-deployer"}, authInfo.Exec.Args)

	// Re-executing the same integration is a no-op at the shared writer layer.
	require.NoError(t, integration.Execute(t.Context(), creds))

	require.NoError(t, integration.Cleanup(t.Context()))
	_, err = clientcmd.LoadFromFile(path)
	require.Error(t, err)
	// Cleanup is idempotent.
	require.NoError(t, integration.Cleanup(t.Context()))
}

func TestGKEIntegrationExecuteWrongCredentialType(t *testing.T) {
	integration, err := NewGKEIntegration(validGKEConfig(filepath.Join(t.TempDir(), "config")))
	require.NoError(t, err)
	err = integration.Execute(t.Context(), &types.AWSCredentials{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGKEIntegrationFailed)
	assert.Contains(t, err.Error(), "expected GCP credentials")
}

func TestGKEIntegrationExecuteErrors(t *testing.T) {
	config := validGKEConfig(filepath.Join(t.TempDir(), "config"))
	integration, err := NewGKEIntegration(config)
	require.NoError(t, err)
	creds := &types.GCPCredentials{AccessToken: "example-access-token", TokenExpiry: time.Now().Add(time.Hour)}

	originalFactory := gkeClientFactory
	originalDescribe := gkeDescribeCluster
	t.Cleanup(func() {
		gkeClientFactory = originalFactory
		gkeDescribeCluster = originalDescribe
	})

	gkeClientFactory = func(_ context.Context, _ types.ICredentials) (gcpCloud.GKEClient, error) {
		return nil, errors.New("client construction failed")
	}
	err = integration.Execute(t.Context(), creds)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGKEIntegrationFailed)
	assert.Contains(t, err.Error(), "client construction failed")

	gkeClientFactory = func(_ context.Context, _ types.ICredentials) (gcpCloud.GKEClient, error) { return nil, nil }
	gkeDescribeCluster = func(_ context.Context, _ gcpCloud.GKEClient, _, _, _ string) (*gcpCloud.GKEClusterInfo, error) {
		return nil, errors.New("cluster lookup failed")
	}
	err = integration.Execute(t.Context(), creds)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGKEIntegrationFailed)
	assert.Contains(t, err.Error(), "cluster lookup failed")
}
