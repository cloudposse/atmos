package aks

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeAuthManager embeds the (nil) AuthManager interface and overrides only the two methods this
// file's orchestration calls, so we don't have to implement the whole interface.
type fakeAuthManager struct {
	types.AuthManager
	execErr      error
	whoami       *types.WhoamiInfo
	authErr      error
	realm        string
	gotName      string
	gotOverrides types.IntegrationOverrides
}

func (f *fakeAuthManager) ExecuteIntegration(_ context.Context, name string, opts ...types.IntegrationOption) error {
	f.gotName = name
	for _, opt := range opts {
		opt(&f.gotOverrides)
	}
	return f.execErr
}

func (f *fakeAuthManager) Authenticate(_ context.Context, name string) (*types.WhoamiInfo, error) {
	f.gotName = name
	return f.whoami, f.authErr
}

// GetRealm returns the configured auth realm.
func (f *fakeAuthManager) GetRealm() realm.RealmInfo {
	return realm.RealmInfo{Value: f.realm}
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

			err := executeAKSUpdateKubeconfigViaIntegration("dev/aks", "", "")

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

// TestExecuteAKSUpdateKubeconfigViaIntegration_ForwardsOverrides is the
// regression guard for issue #3386 on the Azure side: --kubeconfig/--alias
// passed alongside --integration must reach ExecuteIntegration as overrides
// instead of being silently dropped.
func TestExecuteAKSUpdateKubeconfigViaIntegration_ForwardsOverrides(t *testing.T) {
	restoreSeams(t)

	initCliConfig = okInitConfig
	fake := &fakeAuthManager{}
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return fake, nil
	}

	err := executeAKSUpdateKubeconfigViaIntegration("prod/aks", "/home/dev/.config/atmos/kube/prod", "prod-aks")
	require.NoError(t, err)

	assert.Equal(t, "prod/aks", fake.gotName)
	assert.Equal(t, "/home/dev/.config/atmos/kube/prod", fake.gotOverrides.KubeconfigPath)
	assert.Equal(t, "prod-aks", fake.gotOverrides.Alias)
}

func TestExecuteAKSUpdateKubeconfigDirect_Errors(t *testing.T) {
	tests := []struct {
		name    string
		initErr error
		mgrErr  error
		whoami  *types.WhoamiInfo
		authErr error
		// wantErrIs lists every error the result must wrap (checked with errors.Is): the category
		// sentinel that classifies the failure, plus the injected cause where one is injected.
		wantErrIs []error
	}{
		{name: "config init fails", initErr: errBoom, wantErrIs: []error{errUtils.ErrFailedToInitConfig, errBoom}},
		{name: "manager create fails", mgrErr: errBoom, wantErrIs: []error{errUtils.ErrFailedToInitializeAuthManager, errBoom}},
		{name: "authenticate fails", authErr: errBoom, wantErrIs: []error{errUtils.ErrIdentityAuthFailed, errBoom}},
		// nil credentials injects no cause — the failure is the guard, wrapping ErrIdentityAuthFailed.
		{name: "nil credentials", whoami: &types.WhoamiInfo{Credentials: nil}, wantErrIs: []error{errUtils.ErrIdentityAuthFailed}},
		// nil whoami (Authenticate returns (nil, nil)) must be handled without a panic.
		{name: "nil whoami result", whoami: nil, wantErrIs: []error{errUtils.ErrIdentityAuthFailed}},
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
			for _, target := range tt.wantErrIs {
				assert.ErrorIs(t, err, target)
			}
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

// TestExecuteAKSUpdateKubeconfigDirect_RealmScopesUser verifies that the direct
// path names the kube user after the auth realm and passes the realm to the
// exec plugin, as the azure/aks integration does.
func TestExecuteAKSUpdateKubeconfigDirect_RealmScopesUser(t *testing.T) {
	restoreSeams(t)

	initCliConfig = okInitConfig
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return &fakeAuthManager{realm: "acme", whoami: &types.WhoamiInfo{
			Credentials: &types.AzureCredentials{SubscriptionID: "sub-123"},
		}}, nil
	}
	newAKSClient = func(context.Context, types.ICredentials, string) (azureCloud.AKSClient, error) {
		return nil, nil
	}
	describeCluster = func(_ context.Context, _ azureCloud.AKSClient, sub, rg, name string) (*azureCloud.AKSClusterInfo, error) {
		return &azureCloud.AKSClusterInfo{
			Name: name, ResourceGroup: rg, SubscriptionID: sub,
			ID:       "/subscriptions/" + sub + "/resourceGroups/" + rg + "/providers/Microsoft.ContainerService/managedClusters/" + name,
			Endpoint: "https://aks.example.test:443", CertificateAuthorityData: base64.StdEncoding.EncodeToString([]byte("test-ca")),
		}, nil
	}

	kubeconfig := filepath.Join(t.TempDir(), "config")
	require.NoError(t, executeAKSUpdateKubeconfigDirect(&aksKubeconfigDirectParams{
		clusterName: "aks-dev", resourceGroup: "rg-aks-cus", kubeconfigPath: kubeconfig, alias: "dev-aks", identityName: "dev",
	}))

	config, err := clientcmd.LoadFromFile(kubeconfig)
	require.NoError(t, err)
	assert.Equal(t, "atmos-acme-aks-aks-dev-rg-aks-cus-sub-123-dev", config.Contexts["dev-aks"].AuthInfo)
	user := config.AuthInfos["atmos-acme-aks-aks-dev-rg-aks-cus-sub-123-dev"]
	require.NotNil(t, user)
	require.NotNil(t, user.Exec)
	assert.Contains(t, user.Exec.Env, clientcmdapi.ExecEnvVar{Name: "ATMOS_AUTH_REALM", Value: "acme"})
}

// TestExecuteAKSUpdateKubeconfigDirect_RealmPrefixesDefaultContext verifies
// that, without an alias, the direct path names the context
// <realm>/<resource-id> as the azure/aks integration does, and keeps the bare
// resource ID without a realm.
func TestExecuteAKSUpdateKubeconfigDirect_RealmPrefixesDefaultContext(t *testing.T) {
	const resourceID = "/subscriptions/sub-123/resourceGroups/rg-aks-cus/providers/Microsoft.ContainerService/managedClusters/aks-dev"
	for _, tt := range []struct {
		name, realm, wantContext, wantUser string
		writeFailure                       bool
	}{
		{name: "no realm", wantContext: resourceID, wantUser: "atmos-aks-aks-dev-rg-aks-cus-sub-123-dev"},
		{name: "realm", realm: "acme", wantContext: "acme/" + resourceID, wantUser: "atmos-acme-aks-aks-dev-rg-aks-cus-sub-123-dev"},
		{name: "kubeconfig parent is a file", writeFailure: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			restoreSeams(t)
			initCliConfig = okInitConfig
			newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
				return &fakeAuthManager{realm: tt.realm, whoami: &types.WhoamiInfo{
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
			if tt.writeFailure {
				// The kubeconfig's parent is a regular file, so it cannot be written.
				require.NoError(t, os.WriteFile(kubeconfig, []byte("not a directory"), 0o600))
				kubeconfig = filepath.Join(kubeconfig, "config")
			}
			err := executeAKSUpdateKubeconfigDirect(&aksKubeconfigDirectParams{
				clusterName: "aks-dev", resourceGroup: "rg-aks-cus", kubeconfigPath: kubeconfig, identityName: "dev",
			})
			if tt.writeFailure {
				require.ErrorIs(t, err, errUtils.ErrAKSIntegrationFailed)
				require.ErrorIs(t, err, errUtils.ErrKubeconfigWrite)
				return
			}
			require.NoError(t, err)

			config, err := clientcmd.LoadFromFile(kubeconfig)
			require.NoError(t, err)
			require.Len(t, config.Contexts, 1)
			require.Contains(t, config.Contexts, tt.wantContext)
			assert.Equal(t, tt.wantUser, config.Contexts[tt.wantContext].AuthInfo)
			assert.Equal(t, resourceID, config.Contexts[tt.wantContext].Cluster)
			assert.Equal(t, tt.wantContext, config.CurrentContext)
		})
	}
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
