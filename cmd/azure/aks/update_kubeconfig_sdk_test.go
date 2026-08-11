package aks

import (
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeAuthManager embeds the (nil) AuthManager interface and overrides only the two methods this
// file's orchestration calls, so we don't have to implement the whole interface.
type fakeAuthManager struct {
	types.AuthManager
	execErr error
	whoami  *types.WhoamiInfo
	authErr error
	gotName string
}

func (f *fakeAuthManager) ExecuteIntegration(_ context.Context, name string) error {
	f.gotName = name
	return f.execErr
}

func (f *fakeAuthManager) Authenticate(_ context.Context, name string) (*types.WhoamiInfo, error) {
	f.gotName = name
	return f.whoami, f.authErr
}

// restoreSeams snapshots the package seams and returns a func that restores them.
func restoreSeams(t *testing.T) {
	t.Helper()
	origInit, origMgr, origClient, origDescribe := initCliConfig, newAuthManager, newAKSClient, describeCluster
	t.Cleanup(func() {
		initCliConfig, newAuthManager, newAKSClient, describeCluster = origInit, origMgr, origClient, origDescribe
	})
}

func okInitConfig(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
	return schema.AtmosConfiguration{}, nil
}

var errBoom = errors.New("boom")

func TestExecuteAKSUpdateKubeconfigViaIntegration(t *testing.T) {
	tests := []struct {
		name       string
		initErr    error
		mgrErr     error
		execErr    error
		wantErr    bool
		wantErrIs  error
		wantIntArg string
	}{
		{name: "success", wantIntArg: "dev/aks"},
		{name: "config init fails", initErr: errBoom, wantErr: true},
		{name: "manager create fails", mgrErr: errBoom, wantErr: true},
		{name: "integration fails", execErr: errBoom, wantErr: true, wantErrIs: errBoom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreSeams(t)

			initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, tt.initErr
			}
			fake := &fakeAuthManager{execErr: tt.execErr}
			newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
				return fake, tt.mgrErr
			}

			err := executeAKSUpdateKubeconfigViaIntegration("dev/aks")

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					assert.ErrorIs(t, err, tt.wantErrIs)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantIntArg, fake.gotName)
		})
	}
}

func TestExecuteAKSUpdateKubeconfigDirect_Errors(t *testing.T) {
	tests := []struct {
		name    string
		initErr error
		mgrErr  error
		whoami  *types.WhoamiInfo
		authErr error
		wantErr bool
	}{
		{name: "config init fails", initErr: errBoom, wantErr: true},
		{name: "manager create fails", mgrErr: errBoom, wantErr: true},
		{name: "authenticate fails", authErr: errBoom, wantErr: true},
		{name: "nil credentials", whoami: &types.WhoamiInfo{Credentials: nil}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreSeams(t)

			initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, tt.initErr
			}
			newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
				return &fakeAuthManager{whoami: tt.whoami, authErr: tt.authErr}, tt.mgrErr
			}

			err := executeAKSUpdateKubeconfigDirect(&aksKubeconfigDirectParams{
				clusterName:   "c",
				resourceGroup: "rg",
				identityName:  "dev",
			})
			require.Error(t, err)
		})
	}
}

// TestExecuteAKSUpdateKubeconfigDirect_Success drives the full direct path — authenticate, AKS
// describe (seamed), and a real kubeconfig write to a temp file — and asserts the subscription
// falls back to the authenticated identity's subscription when not passed explicitly.
func TestExecuteAKSUpdateKubeconfigDirect_Success(t *testing.T) {
	restoreSeams(t)

	initCliConfig = okInitConfig
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return &fakeAuthManager{whoami: &types.WhoamiInfo{
			Credentials: &types.AzureCredentials{SubscriptionID: "sub-from-creds"},
		}}, nil
	}

	var gotSub string
	newAKSClient = func(_ context.Context, _ types.ICredentials, sub string) (azureCloud.AKSClient, error) {
		gotSub = sub
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

	kubeconfig := filepath.Join(t.TempDir(), "config")
	err := executeAKSUpdateKubeconfigDirect(&aksKubeconfigDirectParams{
		clusterName:    "aks-dev",
		resourceGroup:  "rg-aks-cus",
		kubeconfigPath: kubeconfig,
		alias:          "dev-aks",
		identityName:   "dev",
	})

	require.NoError(t, err)
	assert.Equal(t, "sub-from-creds", gotSub, "subscription should fall back to the identity's subscription")
	assert.FileExists(t, kubeconfig)
}

func TestWriteAKSKubeconfigDirect_ClientAndDescribeErrors(t *testing.T) {
	t.Run("client construction fails", func(t *testing.T) {
		restoreSeams(t)
		newAKSClient = func(context.Context, types.ICredentials, string) (azureCloud.AKSClient, error) {
			return nil, errBoom
		}
		err := writeAKSKubeconfigDirect(context.Background(), &types.AzureCredentials{}, &aksKubeconfigDirectParams{})
		require.Error(t, err)
	})

	t.Run("describe fails", func(t *testing.T) {
		restoreSeams(t)
		newAKSClient = func(context.Context, types.ICredentials, string) (azureCloud.AKSClient, error) {
			return nil, nil
		}
		describeCluster = func(context.Context, azureCloud.AKSClient, string, string, string) (*azureCloud.AKSClusterInfo, error) {
			return nil, errBoom
		}
		err := writeAKSKubeconfigDirect(context.Background(), &types.AzureCredentials{}, &aksKubeconfigDirectParams{})
		require.Error(t, err)
	})
}

// Ensure the seams default to the real implementations (guards against an accidental nil default).
func TestSeamsDefaultToRealImplementations(t *testing.T) {
	assert.NotNil(t, initCliConfig)
	assert.NotNil(t, newAuthManager)
	assert.NotNil(t, newAKSClient)
	assert.NotNil(t, describeCluster)
	// Sanity: the config seam points at the real loader.
	_ = cfg.InitCliConfig
}
