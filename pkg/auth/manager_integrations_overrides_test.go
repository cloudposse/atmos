package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// clusterIntegrationFixture returns a cluster-shaped integration config with a
// kubeconfig block, matching what aws/eks, azure/aks, and gcp/gke read. The
// override logic is shared across all three kinds, so testing it once here
// covers every cloud.
func clusterIntegrationFixture() schema.Integration {
	return schema.Integration{
		Kind: "azure/aks",
		Via:  &schema.IntegrationVia{Identity: "dev"},
		Spec: &schema.IntegrationSpec{
			Cluster: &schema.Cluster{
				Name:          "aks-prod",
				ResourceGroup: "rg-aks-cus",
				Alias:         "configured-alias",
				Kubeconfig: &schema.KubeconfigSettings{
					Path:   "/configured/path",
					Mode:   "0600",
					Update: "merge",
				},
			},
		},
	}
}

func TestApplyIntegrationOverrides_NoOverrides(t *testing.T) {
	config := clusterIntegrationFixture()

	got := applyIntegrationOverrides(config, types.IntegrationOverrides{})

	// Nothing overridden: the configured values are preserved verbatim.
	assert.Equal(t, "configured-alias", got.Spec.Cluster.Alias)
	assert.Equal(t, "/configured/path", got.Spec.Cluster.Kubeconfig.Path)
	assert.Equal(t, "0600", got.Spec.Cluster.Kubeconfig.Mode)
	assert.Equal(t, "merge", got.Spec.Cluster.Kubeconfig.Update)
}

func TestApplyIntegrationOverrides_AliasAndPath(t *testing.T) {
	config := clusterIntegrationFixture()

	got := applyIntegrationOverrides(config, types.IntegrationOverrides{
		KubeconfigPath: "/override/path",
		Alias:          "override-alias",
	})

	assert.Equal(t, "override-alias", got.Spec.Cluster.Alias)
	assert.Equal(t, "/override/path", got.Spec.Cluster.Kubeconfig.Path)
	// Mode and update survive a path-only override (only Path is replaced).
	assert.Equal(t, "0600", got.Spec.Cluster.Kubeconfig.Mode)
	assert.Equal(t, "merge", got.Spec.Cluster.Kubeconfig.Update)
}

// TestApplyIntegrationOverrides_PathWhenKubeconfigNil verifies the path override
// synthesizes a KubeconfigSettings when the integration config had none.
func TestApplyIntegrationOverrides_PathWhenKubeconfigNil(t *testing.T) {
	config := clusterIntegrationFixture()
	config.Spec.Cluster.Kubeconfig = nil

	got := applyIntegrationOverrides(config, types.IntegrationOverrides{
		KubeconfigPath: "/override/path",
	})

	require.NotNil(t, got.Spec.Cluster.Kubeconfig)
	assert.Equal(t, "/override/path", got.Spec.Cluster.Kubeconfig.Path)
}

// TestApplyIntegrationOverrides_DoesNotMutateSource is the isolation guard
// required by CLAUDE.md: overrides must never leak back into the shared
// m.config. We check both directions - the returned copy carries the override,
// and the original fixture's cluster/kubeconfig pointers are untouched.
func TestApplyIntegrationOverrides_DoesNotMutateSource(t *testing.T) {
	config := clusterIntegrationFixture()
	originalCluster := config.Spec.Cluster
	originalKubeconfig := config.Spec.Cluster.Kubeconfig

	got := applyIntegrationOverrides(config, types.IntegrationOverrides{
		KubeconfigPath: "/override/path",
		Alias:          "override-alias",
	})

	// Result carries the overrides.
	assert.Equal(t, "override-alias", got.Spec.Cluster.Alias)
	assert.Equal(t, "/override/path", got.Spec.Cluster.Kubeconfig.Path)

	// Source is untouched (result -> src isolation): the shared config still
	// holds the configured values, and the result used fresh pointers.
	assert.Equal(t, "configured-alias", originalCluster.Alias)
	assert.Equal(t, "/configured/path", originalKubeconfig.Path)
	assert.NotSame(t, originalCluster, got.Spec.Cluster)
	assert.NotSame(t, originalKubeconfig, got.Spec.Cluster.Kubeconfig)
}

// TestApplyIntegrationOverrides_NonClusterIntegration verifies a non-cluster
// integration (no spec.cluster) is returned as-is without panicking, even when
// overrides are present.
func TestApplyIntegrationOverrides_NonClusterIntegration(t *testing.T) {
	config := schema.Integration{
		Kind: "aws/ecr",
		Via:  &schema.IntegrationVia{Identity: "dev"},
		Spec: &schema.IntegrationSpec{},
	}

	got := applyIntegrationOverrides(config, types.IntegrationOverrides{
		KubeconfigPath: "/override/path",
		Alias:          "override-alias",
	})

	assert.Nil(t, got.Spec.Cluster)
}

// TestIntegrationOptions_Builders confirms the functional options only set a
// field when non-empty, so callers can forward unset flags safely.
func TestIntegrationOptions_Builders(t *testing.T) {
	var o types.IntegrationOverrides
	types.WithKubeconfigPath("")(&o)
	types.WithAlias("")(&o)
	assert.Empty(t, o.KubeconfigPath)
	assert.Empty(t, o.Alias)

	types.WithKubeconfigPath("/p")(&o)
	types.WithAlias("a")(&o)
	assert.Equal(t, "/p", o.KubeconfigPath)
	assert.Equal(t, "a", o.Alias)
}
