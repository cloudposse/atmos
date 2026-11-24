package aws

// Tests that EKS Cleanup only removes the kubeconfig entries that belong to
// the integration being cleaned up (issue #3720, findings 2.5 and 3.7).

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/cloud/kube"
	"github.com/cloudposse/atmos/pkg/schema"
)

// writeEKSEntry writes one EKS cluster entry to the kubeconfig at path, the
// same way Execute does after describing the cluster.
func writeEKSEntry(t *testing.T, path, clusterARN, name, region, alias, identity string) {
	t.Helper()

	mgr, err := kube.NewKubeconfigManager(path, "")
	require.NoError(t, err)
	clusterInfo, err := awsCloud.BuildKubeClusterInfo(&awsCloud.EKSClusterInfo{
		Name:                     name,
		Endpoint:                 "https://example.eks.amazonaws.com",
		CertificateAuthorityData: "dGVzdA==",
		ARN:                      clusterARN,
		Region:                   region,
	}, identity)
	require.NoError(t, err)
	_, err = mgr.WriteClusterConfig(clusterInfo, alias, "merge")
	require.NoError(t, err)
}

// newCleanupIntegration returns an EKS integration that targets the kubeconfig at path.
func newCleanupIntegration(path, name, region, alias, identity string) *EKSIntegration {
	return &EKSIntegration{
		name:     "test",
		identity: identity,
		cluster: &schema.Cluster{
			Name:       name,
			Region:     region,
			Alias:      alias,
			Kubeconfig: &schema.KubeconfigSettings{Path: path},
		},
	}
}

// TestEKSIntegration_Cleanup_IgnoresSameNamedClusterInOtherRegion verifies that
// logging out of the us-west-2 integration does not remove the us-east-2 cluster
// that has the same name.
func TestEKSIntegration_Cleanup_IgnoresSameNamedClusterInOtherRegion(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	eastARN := "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"
	writeEKSEntry(t, kubeconfigPath, eastARN, "dev-cluster", "us-east-2", "", "dev-admin")

	// The us-west-2 integration never wrote an entry, so there is nothing for it to remove.
	err := newCleanupIntegration(kubeconfigPath, "dev-cluster", "us-west-2", "", "dev-admin").Cleanup(t.Context())
	require.NoError(t, err)

	remaining, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err, "the us-east-2 entries must survive, so the file must still exist")
	assert.Contains(t, remaining.Clusters, eastARN)
	assert.Contains(t, remaining.Contexts, eastARN)
}

// TestEKSIntegration_Cleanup_RemovesOnlyMatchingRegion verifies that with both
// regions present, cleanup removes the entry of its own region and keeps the other.
func TestEKSIntegration_Cleanup_RemovesOnlyMatchingRegion(t *testing.T) {
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	eastARN := "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"
	westARN := "arn:aws:eks:us-west-2:123456789012:cluster/dev-cluster"
	writeEKSEntry(t, kubeconfigPath, eastARN, "dev-cluster", "us-east-2", "", "dev-admin")
	writeEKSEntry(t, kubeconfigPath, westARN, "dev-cluster", "us-west-2", "", "dev-admin")

	err := newCleanupIntegration(kubeconfigPath, "dev-cluster", "us-west-2", "", "dev-admin").Cleanup(t.Context())
	require.NoError(t, err)

	remaining, err := loadKubeconfig(kubeconfigPath)
	require.NoError(t, err)
	assert.Contains(t, remaining.Clusters, eastARN)
	assert.Contains(t, remaining.Contexts, eastARN)
	assert.NotContains(t, remaining.Clusters, westARN)
	assert.NotContains(t, remaining.Contexts, westARN)
}

// TestEKSIntegration_FindClusterARN_MatchesParsedARN verifies that only EKS
// cluster ARNs with the configured region and name match.
func TestEKSIntegration_FindClusterARN_MatchesParsedARN(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		want    string
		wantErr error
	}{
		{
			name: "matching region and name",
			ids:  []string{"arn:aws:eks:us-west-2:123456789012:cluster/dev-cluster"},
			want: "arn:aws:eks:us-west-2:123456789012:cluster/dev-cluster",
		},
		{
			name: "GovCloud partition",
			ids:  []string{"arn:aws-us-gov:eks:us-west-2:123456789012:cluster/dev-cluster"},
			want: "arn:aws-us-gov:eks:us-west-2:123456789012:cluster/dev-cluster",
		},
		{
			name:    "same name in another region",
			ids:     []string{"arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"},
			wantErr: errUtils.ErrEKSClusterNotFound,
		},
		{
			name:    "another service with the same resource",
			ids:     []string{"arn:aws:ecs:us-west-2:123456789012:cluster/dev-cluster"},
			wantErr: errUtils.ErrEKSClusterNotFound,
		},
		{
			name:    "cluster ID that is not an ARN",
			ids:     []string{"projects/p/locations/us-west-2/cluster/dev-cluster"},
			wantErr: errUtils.ErrEKSClusterNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
			for _, id := range tt.ids {
				mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
				require.NoError(t, err)
				_, err = mgr.WriteClusterConfig(&kube.ClusterInfo{
					Name: "dev-cluster", Endpoint: "https://example.com", ID: id,
					Region: "us-west-2", UserPrefix: "eks",
				}, "", "merge")
				require.NoError(t, err)
			}

			mgr, err := kube.NewKubeconfigManager(kubeconfigPath, "")
			require.NoError(t, err)
			got, err := newCleanupIntegration(kubeconfigPath, "dev-cluster", "us-west-2", "", "dev-admin").findClusterARN(mgr)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
