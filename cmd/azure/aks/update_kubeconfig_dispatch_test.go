package aks

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setFlag sets a flag on the shared command and restores it after the test to avoid leaking state
// into the other tests that call RunE on the same command instance.
func setFlag(t *testing.T, name, value string) {
	t.Helper()
	require.NoError(t, updateKubeconfigCmd.Flags().Set(name, value))
	t.Cleanup(func() { _ = updateKubeconfigCmd.Flags().Set(name, "") })
}

func TestUpdateKubeconfigCmd_RunE_IntegrationBranch(t *testing.T) {
	restoreSeams(t)
	initCliConfig = okInitConfig
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return &fakeAuthManager{}, nil // ExecuteIntegration returns nil
	}

	setFlag(t, "integration", "dev/aks")

	err := updateKubeconfigCmd.RunE(updateKubeconfigCmd, []string{})
	require.NoError(t, err)
}

func TestUpdateKubeconfigCmd_RunE_DirectBranch(t *testing.T) {
	restoreSeams(t)
	initCliConfig = okInitConfig
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return &fakeAuthManager{whoami: &types.WhoamiInfo{
			Credentials: &types.AzureCredentials{SubscriptionID: "sub-x"},
		}}, nil
	}
	newAKSClient = func(context.Context, types.ICredentials, string) (azureCloud.AKSClient, error) {
		return nil, nil
	}
	describeCluster = func(_ context.Context, _ azureCloud.AKSClient, sub, rg, name string) (*azureCloud.AKSClusterInfo, error) {
		return &azureCloud.AKSClusterInfo{
			Name:                     name,
			ResourceGroup:            rg,
			SubscriptionID:           sub,
			ID:                       "/subscriptions/" + sub + "/resourceGroups/" + rg + "/providers/Microsoft.ContainerService/managedClusters/" + name,
			Endpoint:                 "https://aks.example.test:443",
			CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte("test-ca")),
		}, nil
	}

	// Point kubeconfig at a temp file so RunE doesn't write to the real ~/.config default.
	setFlag(t, "cluster-name", "aks-dev")
	setFlag(t, "resource-group", "rg-aks-cus")
	setFlag(t, "identity", "dev")
	setFlag(t, "kubeconfig", filepath.Join(t.TempDir(), "config"))

	err := updateKubeconfigCmd.RunE(updateKubeconfigCmd, []string{})
	require.NoError(t, err)
}
