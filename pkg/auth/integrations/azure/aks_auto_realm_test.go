package azure

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newSourcedAKSIntegration builds the dev AKS integration with a realm and its source.
func newSourcedAKSIntegration(t *testing.T, path, realmValue, realmSource string) integrations.Integration {
	t.Helper()

	integration, err := NewAKSIntegration(&integrations.IntegrationConfig{
		Name: "dev", Realm: realmValue, RealmSource: realmSource,
		Config: &schema.Integration{
			Kind: integrations.KindAzureAKS,
			Via:  &schema.IntegrationVia{Identity: "azure-dev"},
			Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{
				Name: "dev-cluster", ResourceGroup: "dev-rg",
				Kubeconfig: &schema.KubeconfigSettings{Path: path},
			}},
		},
	})
	require.NoError(t, err)
	return integration
}

// TestAKSIntegration_AutoRealmKeepsNamesAndMigrates verifies that a realm
// derived from the atmos.yaml path keeps the bare resource ID context and the
// realm-less user, that a login replaces the prefixed entries an unreleased
// build wrote, and that logout then removes everything this integration owns.
func TestAKSIntegration_AutoRealmKeepsNamesAndMigrates(t *testing.T) {
	const autoRealm = "462813558259d41c"
	stubDevAKSCluster(t)
	path := filepath.Join(t.TempDir(), "kubeconfig")
	creds := &types.AzureCredentials{SubscriptionID: "sub-123"}
	require.NoError(t, newSourcedAKSIntegration(t, path, autoRealm, realm.SourceConfig).Execute(t.Context(), creds))

	integration := newSourcedAKSIntegration(t, path, autoRealm, realm.SourceAuto)
	require.NoError(t, integration.Execute(t.Context(), creds))

	config, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, devClusterResourceID, config.CurrentContext)
	require.Len(t, config.Contexts, 1)
	require.Contains(t, config.Contexts, devClusterResourceID)
	assert.Equal(t, "atmos-aks-dev-cluster-dev-rg-sub-123-azure-dev-6d99f9e9", config.Contexts[devClusterResourceID].AuthInfo)
	assert.Len(t, config.AuthInfos, 1, "the prefixed user is removed")
	assert.Empty(t, execEnvValue(t, config.AuthInfos["atmos-aks-dev-cluster-dev-rg-sub-123-azure-dev-6d99f9e9"], "ATMOS_AUTH_REALM"))

	// Logout after an unreleased build wrote the prefixed entries again.
	require.NoError(t, newSourcedAKSIntegration(t, path, autoRealm, realm.SourceConfig).Execute(t.Context(), creds))
	require.NoError(t, integration.Cleanup(t.Context()))
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "every entry is owned by the integration, so the file is removed")
}
