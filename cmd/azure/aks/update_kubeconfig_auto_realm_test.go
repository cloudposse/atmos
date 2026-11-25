package aks

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteAKSUpdateKubeconfigDirect_AutoRealmKeepsNames verifies that the
// realm Atmos derives from the atmos.yaml path when none is configured does not
// change kube names: the context stays the bare resource ID, the user has no
// realm, and ATMOS_AUTH_REALM is not added to the exec env. Only a realm set by
// auth.realm or ATMOS_AUTH_REALM scopes them.
func TestExecuteAKSUpdateKubeconfigDirect_AutoRealmKeepsNames(t *testing.T) {
	const (
		resourceID = "/subscriptions/sub-123/resourceGroups/rg-aks-cus/providers/Microsoft.ContainerService/managedClusters/aks-dev"
		autoRealm  = "462813558259d41c"
	)
	for _, tt := range []struct {
		name, configRealm, configSource, wantContext, wantUser, wantExecRealm string
	}{
		{
			name: "auto realm from config path", configRealm: autoRealm, configSource: "config-path",
			wantContext: resourceID, wantUser: "atmos-aks-aks-dev-rg-aks-cus-sub-123-dev-97954fc1",
		},
		{
			name: "default realm without config path", configRealm: "default", configSource: "default",
			wantContext: resourceID, wantUser: "atmos-aks-aks-dev-rg-aks-cus-sub-123-dev-97954fc1",
		},
		{
			name: "explicit config realm", configRealm: "acme", configSource: "config",
			wantContext: "acme/" + resourceID, wantUser: "atmos-acme-aks-aks-dev-rg-aks-cus-sub-123-dev-e92c4e34", wantExecRealm: "acme",
		},
		{
			name: "explicit env realm", configRealm: "acme", configSource: "env",
			wantContext: "acme/" + resourceID, wantUser: "atmos-acme-aks-aks-dev-rg-aks-cus-sub-123-dev-e92c4e34", wantExecRealm: "acme",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			restoreSeams(t)
			initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{Auth: schema.AuthConfig{Realm: tt.configRealm, RealmSource: tt.configSource}}, nil
			}
			newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
				return &fakeAuthManager{realm: tt.configRealm, whoami: &types.WhoamiInfo{
					Credentials: &types.AzureCredentials{SubscriptionID: "sub-123"},
				}}, nil
			}
			newAKSClient = func(context.Context, types.ICredentials, string) (azureCloud.AKSClient, error) {
				return nil, nil
			}
			describeCluster = func(_ context.Context, _ azureCloud.AKSClient, sub, rg, name string) (*azureCloud.AKSClusterInfo, error) {
				return &azureCloud.AKSClusterInfo{
					Name: name, ResourceGroup: rg, SubscriptionID: sub, ID: resourceID,
					Endpoint: "https://aks.example.test:443", CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte("test-ca")),
				}, nil
			}

			kubeconfig := filepath.Join(t.TempDir(), "config")
			require.NoError(t, executeAKSUpdateKubeconfigDirect(&aksKubeconfigDirectParams{
				clusterName: "aks-dev", resourceGroup: "rg-aks-cus", kubeconfigPath: kubeconfig, identityName: "dev",
			}))

			config, err := clientcmd.LoadFromFile(kubeconfig)
			require.NoError(t, err)
			require.Contains(t, config.Contexts, tt.wantContext)
			assert.Equal(t, tt.wantContext, config.CurrentContext)
			assert.Equal(t, tt.wantUser, config.Contexts[tt.wantContext].AuthInfo)
			user := config.AuthInfos[tt.wantUser]
			require.NotNil(t, user)
			var realmValues []string
			for _, env := range user.Exec.Env {
				if env.Name == "ATMOS_AUTH_REALM" {
					realmValues = append(realmValues, env.Value)
				}
			}
			if tt.wantExecRealm == "" {
				assert.Empty(t, realmValues, "a derived realm must not set ATMOS_AUTH_REALM, even to an empty value")
			} else {
				assert.Equal(t, []string{tt.wantExecRealm}, realmValues)
			}
		})
	}
}
