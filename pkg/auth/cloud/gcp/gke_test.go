package gcp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/container/v1"
	"k8s.io/client-go/tools/clientcmd"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

const testCAData = "dGVzdC1jYQ=="

type fakeGKEClient struct {
	cluster      *container.Cluster
	err          error
	resourceName string
}

// GetCluster records the requested resource and returns the configured fake response.
func (f *fakeGKEClient) GetCluster(_ context.Context, resourceName string) (*container.Cluster, error) {
	f.resourceName = resourceName
	return f.cluster, f.err
}

// validGKEAPICluster returns a minimal valid cluster discovery response.
func validGKEAPICluster() *container.Cluster {
	return &container.Cluster{
		Endpoint:   "203.0.113.10",
		MasterAuth: &container.MasterAuth{ClusterCaCertificate: testCAData},
	}
}

// TestNewGKEClientValidation verifies invalid credentials are rejected before API use.
func TestNewGKEClientValidation(t *testing.T) {
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	tests := []struct {
		name  string
		creds types.ICredentials
	}{
		{name: "nil credentials", creds: nil},
		{name: "wrong credential type", creds: &types.AWSCredentials{}},
		{name: "empty access token", creds: &types.GCPCredentials{TokenExpiry: future}},
		{name: "expired access token", creds: &types.GCPCredentials{AccessToken: "expired-token", TokenExpiry: past}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGKEClient(t.Context(), tt.creds)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrGKEDescribeCluster)
		})
	}
}

// TestNewGKEClientSuccess verifies valid GCP credentials create a client.
func TestNewGKEClientSuccess(t *testing.T) {
	client, err := NewGKEClient(t.Context(), &types.GCPCredentials{
		AccessToken: "example-access-token",
		TokenExpiry: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	assert.NotNil(t, client)
}

// TestDescribeCluster verifies resource naming and normalized cluster metadata.
func TestDescribeCluster(t *testing.T) {
	client := &fakeGKEClient{cluster: validGKEAPICluster()}

	info, err := DescribeCluster(t.Context(), client, "example-project", "us-central1", "example-cluster")
	require.NoError(t, err)
	assert.Equal(t, "projects/example-project/locations/us-central1/clusters/example-cluster", client.resourceName)
	assert.Equal(t, client.resourceName, info.ID)
	assert.Equal(t, "https://203.0.113.10", info.Endpoint)
	assert.Equal(t, testCAData, info.CertificateAuthorityData)
}

// TestDescribeClusterPreservesEndpointScheme verifies explicit endpoint schemes are retained.
func TestDescribeClusterPreservesEndpointScheme(t *testing.T) {
	cluster := validGKEAPICluster()
	cluster.Endpoint = "https://gke.example.invalid"
	info, err := DescribeCluster(t.Context(), &fakeGKEClient{cluster: cluster}, "example-project", "us-central1-a", "example-cluster")
	require.NoError(t, err)
	assert.Equal(t, "https://gke.example.invalid", info.Endpoint)
}

// TestDescribeClusterErrors verifies incomplete and failed discovery responses are rejected.
func TestDescribeClusterErrors(t *testing.T) {
	tests := []struct {
		name       string
		client     *fakeGKEClient
		wantErr    error
		wantDetail string
	}{
		{name: "API error", client: &fakeGKEClient{err: errors.New("permission denied")}, wantErr: errUtils.ErrGKEDescribeCluster, wantDetail: "permission denied"},
		{name: "missing cluster", client: &fakeGKEClient{}, wantErr: errUtils.ErrGKEClusterNotFound, wantDetail: "example-cluster"},
		{name: "missing endpoint", client: &fakeGKEClient{cluster: &container.Cluster{MasterAuth: &container.MasterAuth{ClusterCaCertificate: testCAData}}}, wantErr: errUtils.ErrGKEDescribeCluster, wantDetail: "empty API endpoint"},
		{name: "missing master auth", client: &fakeGKEClient{cluster: &container.Cluster{Endpoint: "203.0.113.10"}}, wantErr: errUtils.ErrGKEDescribeCluster, wantDetail: "no CA certificate"},
		{name: "missing CA", client: &fakeGKEClient{cluster: &container.Cluster{Endpoint: "203.0.113.10", MasterAuth: &container.MasterAuth{}}}, wantErr: errUtils.ErrGKEDescribeCluster, wantDetail: "no CA certificate"},
		{name: "malformed CA", client: &fakeGKEClient{cluster: &container.Cluster{Endpoint: "203.0.113.10", MasterAuth: &container.MasterAuth{ClusterCaCertificate: "%%%"}}}, wantErr: errUtils.ErrGKEDescribeCluster, wantDetail: "malformed base64 CA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DescribeCluster(t.Context(), tt.client, "example-project", "us-central1", "example-cluster")
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Contains(t, err.Error(), tt.wantDetail)
			assert.Contains(t, err.Error(), "projects/example-project/locations/us-central1/clusters/example-cluster")
		})
	}
}

// TestBuildKubeClusterInfo verifies kubeconfig exec-plugin metadata and identity selection.
func TestBuildKubeClusterInfo(t *testing.T) {
	info := &GKEClusterInfo{
		Name:                     "example-cluster",
		ProjectID:                "example-project",
		Location:                 "us-central1",
		ID:                       ClusterResourceName("example-project", "us-central1", "example-cluster"),
		Endpoint:                 "https://gke.example.invalid",
		CertificateAuthorityData: testCAData,
	}

	got := BuildKubeClusterInfo(info, "example-deployer")
	assert.Equal(t, "example-project-us-central1", got.Region)
	assert.Equal(t, "gke", got.UserPrefix)
	assert.Equal(t, []string{"gcp", "gke", "token", "--identity=example-deployer"}, got.ExecArgs)
	require.Len(t, got.ExecEnv, 1)
	assert.Equal(t, "ATMOS_IDENTITY", got.ExecEnv[0].Name)
	assert.Equal(t, "example-deployer", got.ExecEnv[0].Value)

	config := kube.BuildClusterConfig(got, "example")
	authInfo := config.AuthInfos["atmos-gke-example-cluster-example-project-us-central1"]
	require.NotNil(t, authInfo)
	require.NotNil(t, authInfo.Exec)
	assert.Empty(t, authInfo.Token)
	assert.Equal(t, "atmos", authInfo.Exec.Command)
	assert.Equal(t, got.ExecArgs, authInfo.Exec.Args)
}

// TestGKEKubeconfigUpdateModesAndNoOp verifies shared writer update semantics for GKE.
func TestGKEKubeconfigUpdateModesAndNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	mgr, err := kube.NewKubeconfigManager(path, "")
	require.NoError(t, err)
	info := BuildKubeClusterInfo(&GKEClusterInfo{
		Name:                     "example-cluster",
		ProjectID:                "example-project",
		Location:                 "us-central1",
		ID:                       ClusterResourceName("example-project", "us-central1", "example-cluster"),
		Endpoint:                 "https://gke.example.invalid",
		CertificateAuthorityData: testCAData,
	}, "example-deployer")

	changed, err := mgr.WriteClusterConfig(info, "example", "merge")
	require.NoError(t, err)
	assert.True(t, changed)
	changed, err = mgr.WriteClusterConfig(info, "example", "merge")
	require.NoError(t, err)
	assert.False(t, changed)

	_, err = mgr.WriteClusterConfig(info, "example", "error")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrKubeconfigMerge)

	other := &kube.ClusterInfo{
		Name:                     "other-cluster",
		Endpoint:                 "https://other.example.invalid",
		CertificateAuthorityData: testCAData,
		ID:                       "other-cluster-id",
		Region:                   "other-location",
		UserPrefix:               "test",
		ExecArgs:                 []string{"example", "token"},
	}
	_, err = mgr.WriteClusterConfig(other, "other", "merge")
	require.NoError(t, err)
	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Len(t, loaded.Clusters, 2)

	_, err = mgr.WriteClusterConfig(info, "example", "replace")
	require.NoError(t, err)
	loaded, err = clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Len(t, loaded.Clusters, 1)
	assert.Contains(t, loaded.Clusters, info.ID)
}

// TestGKEKubeconfigDistinctProjectsDoNotCollide verifies resource keys include the project.
func TestGKEKubeconfigDistinctProjectsDoNotCollide(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	mgr, err := kube.NewKubeconfigManager(path, "")
	require.NoError(t, err)

	for _, projectID := range []string{"example-project-a", "example-project-b"} {
		info := BuildKubeClusterInfo(&GKEClusterInfo{
			Name:                     "example-cluster",
			ProjectID:                projectID,
			Location:                 "us-central1",
			ID:                       ClusterResourceName(projectID, "us-central1", "example-cluster"),
			Endpoint:                 "https://gke.example.invalid",
			CertificateAuthorityData: testCAData,
		}, "example-deployer")
		_, err = mgr.WriteClusterConfig(info, "", "merge")
		require.NoError(t, err)
	}

	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Len(t, loaded.Clusters, 2)
	assert.Len(t, loaded.Contexts, 2)
	assert.Len(t, loaded.AuthInfos, 2)
	assert.Contains(t, loaded.AuthInfos, "atmos-gke-example-cluster-example-project-a-us-central1")
	assert.Contains(t, loaded.AuthInfos, "atmos-gke-example-cluster-example-project-b-us-central1")
	for _, authInfo := range loaded.AuthInfos {
		assert.Empty(t, authInfo.Token)
	}
}

// TestGetToken verifies token validation and expiration propagation.
func TestGetToken(t *testing.T) {
	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	token, gotExpiry, err := GetToken(&types.GCPCredentials{AccessToken: "example-access-token", TokenExpiry: expiry})
	require.NoError(t, err)
	assert.Equal(t, "example-access-token", token)
	assert.Equal(t, expiry, gotExpiry)

	for _, creds := range []types.ICredentials{
		&types.AWSCredentials{},
		&types.GCPCredentials{},
		&types.GCPCredentials{AccessToken: "expired-token", TokenExpiry: time.Now().Add(-time.Hour)},
	} {
		_, _, err = GetToken(creds)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrGKETokenGeneration)
	}
}

var _ GKEClient = (*fakeGKEClient)(nil)
