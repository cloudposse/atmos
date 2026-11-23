package aws

// Tests that a realm Atmos derives from the atmos.yaml path (no auth.realm or
// ATMOS_AUTH_REALM configured) never changes EKS kube names, and that names an
// unreleased build wrote under that realm are cleaned up as the integration's own.

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/schema"
)

// autoRealm is a realm derived from a hash of the atmos.yaml path.
const autoRealm = "462813558259d41c"

// newSourcedEKSIntegration builds the prod EKS integration with a realm and its source.
func newSourcedEKSIntegration(t *testing.T, path, realmValue, realmSource string) integrations.Integration {
	t.Helper()

	integration, err := NewEKSIntegration(&integrations.IntegrationConfig{
		Name:        "prod",
		Realm:       realmValue,
		RealmSource: realmSource,
		Config: &schema.Integration{
			Kind: integrations.KindAWSEKS,
			Via:  &schema.IntegrationVia{Identity: "admin"},
			Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{
				Name: "prod", Region: "us-east-2",
				Kubeconfig: &schema.KubeconfigSettings{Path: path},
			}},
		},
	})
	require.NoError(t, err)
	return integration
}

func TestEKSIntegration_Execute_AutoRealmKeepsNames(t *testing.T) {
	for _, tt := range []struct {
		name, source, wantContext, wantUser, wantExecRealm string
	}{
		{name: "derived realm", source: realm.SourceAuto, wantContext: prodClusterARN, wantUser: "atmos-eks-prod-us-east-2-111111111111-admin-48b9c2fe"},
		{name: "config realm", source: realm.SourceConfig, wantContext: autoRealm + "/" + prodClusterARN, wantUser: "atmos-" + autoRealm + "-eks-prod-us-east-2-111111111111-admin-5fbce2d3", wantExecRealm: autoRealm},
		{name: "env realm", source: realm.SourceEnv, wantContext: autoRealm + "/" + prodClusterARN, wantUser: "atmos-" + autoRealm + "-eks-prod-us-east-2-111111111111-admin-5fbce2d3", wantExecRealm: autoRealm},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stubProdEKSCluster(t)
			path := filepath.Join(t.TempDir(), "kubeconfig")

			require.NoError(t, newSourcedEKSIntegration(t, path, autoRealm, tt.source).Execute(t.Context(), nil))

			config, err := loadKubeconfig(path)
			require.NoError(t, err)
			assert.Equal(t, tt.wantContext, config.CurrentContext)
			require.Contains(t, config.Contexts, tt.wantContext)
			assert.Equal(t, tt.wantUser, config.Contexts[tt.wantContext].AuthInfo)
			assert.Equal(t, tt.wantExecRealm, execEnvValue(t, config.AuthInfos[tt.wantUser], "ATMOS_AUTH_REALM"))
		})
	}
}

// TestEKSIntegration_Execute_MigratesAutoRealmNames writes the entry the way an
// unreleased build did (derived realm used as a name realm), then logs in again
// without an explicit realm: the bare-ARN context comes back and the prefixed
// context and user are removed.
func TestEKSIntegration_Execute_MigratesAutoRealmNames(t *testing.T) {
	stubProdEKSCluster(t)
	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, newSourcedEKSIntegration(t, path, autoRealm, realm.SourceConfig).Execute(t.Context(), nil))

	require.NoError(t, newSourcedEKSIntegration(t, path, autoRealm, realm.SourceAuto).Execute(t.Context(), nil))

	config, err := loadKubeconfig(path)
	require.NoError(t, err)
	assert.Equal(t, []string{prodClusterARN}, mapKeys(config.Contexts))
	assert.Equal(t, prodClusterARN, config.CurrentContext)
	assert.Equal(t, []string{"atmos-eks-prod-us-east-2-111111111111-admin-48b9c2fe"}, mapKeys(config.AuthInfos))
}

// TestEKSIntegration_Cleanup_RemovesAutoRealmNames verifies that logout without
// an explicit realm removes the prefixed context and user an unreleased build
// wrote, and keeps another repository's prefixed context.
func TestEKSIntegration_Cleanup_RemovesAutoRealmNames(t *testing.T) {
	stubProdEKSCluster(t)
	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, newSourcedEKSIntegration(t, path, autoRealm, realm.SourceConfig).Execute(t.Context(), nil))
	require.NoError(t, newSourcedEKSIntegration(t, path, "0123456789abcdef", realm.SourceConfig).Execute(t.Context(), nil))

	require.NoError(t, newSourcedEKSIntegration(t, path, autoRealm, realm.SourceAuto).Cleanup(t.Context()))

	config, err := loadKubeconfig(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"0123456789abcdef/" + prodClusterARN}, mapKeys(config.Contexts))
	assert.Equal(t, []string{"atmos-0123456789abcdef-eks-prod-us-east-2-111111111111-admin-e360de54"}, mapKeys(config.AuthInfos))
}
