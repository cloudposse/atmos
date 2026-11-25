package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	realmTestClusterARN = "arn:aws:eks:us-east-2:123456789012:cluster/dev-cluster"
	realmTestAutoRealm  = "462813558259d41c"
)

// serveFakeEKS points the AWS SDK EKS client at an httptest server that
// describes dev-cluster, so the real aws/eks integration runs without AWS.
func serveFakeEKS(t *testing.T) {
	t.Helper()
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config"), []byte("[default]\nregion = us-east-2\n"), 0o600))
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(configDir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(configDir, "credentials"))
	t.Setenv("AWS_PROFILE", "default")
	t.Setenv("AWS_IGNORE_CONFIGURED_ENDPOINT_URLS", "false")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/clusters/dev-cluster", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"cluster":{"name":"dev-cluster","arn":"` + realmTestClusterARN +
			`","endpoint":"https://example.eks.amazonaws.com","certificateAuthority":{"data":"dGVzdA=="}}}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	t.Setenv("AWS_ENDPOINT_URL_EKS", server.URL)
}

// newRealmTestManager builds a manager with one aws/eks integration that writes
// kubeconfigPath, with the realm resolved by pkg/config as realmValue/realmSource.
func newRealmTestManager(t *testing.T, kubeconfigPath, realmValue, realmSource string) *manager {
	t.Helper()
	t.Setenv("ATMOS_AUTH_REALM", "")
	authConfig := &schema.AuthConfig{
		Realm:       realmValue,
		RealmSource: realmSource,
		Providers:   map[string]schema.Provider{},
		Identities:  map[string]schema.Identity{},
		Integrations: map[string]schema.Integration{
			"dev/eks": {
				Kind: "aws/eks",
				Via:  &schema.IntegrationVia{Identity: "dev-user"},
				Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{
					Name: "dev-cluster", Region: "us-east-2",
					Kubeconfig: &schema.KubeconfigSettings{Path: kubeconfigPath},
				}},
			},
		},
	}
	m, err := NewAuthManager(authConfig, credentials.NewCredentialStoreWithConfig(authConfig), validation.NewValidator(), nil, "")
	require.NoError(t, err)
	impl, ok := m.(*manager)
	require.True(t, ok)
	return impl
}

// writeV1234Kubeconfig writes the entry v1.234.1 wrote for dev-cluster: the
// bare-ARN context pointing at the atmos-eks-<name>-<region> user.
func writeV1234Kubeconfig(t *testing.T, path string) {
	t.Helper()
	config := clientcmdapi.NewConfig()
	config.Clusters[realmTestClusterARN] = &clientcmdapi.Cluster{Server: "https://example.eks.amazonaws.com"}
	config.AuthInfos["atmos-eks-dev-cluster-us-east-2"] = &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{
		APIVersion: "client.authentication.k8s.io/v1beta1", Command: "atmos",
		Args: []string{"aws", "eks", "token", "--cluster-name", "dev-cluster", "--region", "us-east-2"},
	}}
	config.Contexts[realmTestClusterARN] = &clientcmdapi.Context{Cluster: realmTestClusterARN, AuthInfo: "atmos-eks-dev-cluster-us-east-2"}
	config.CurrentContext = realmTestClusterARN
	require.NoError(t, clientcmd.WriteToFile(*config, path))
}

// TestManager_ExecuteIntegration_AutoRealmKeepsV1234ContextName is the upgrade
// path: with no realm configured, pkg/config sets the realm to a hash of the
// atmos.yaml path. A login after upgrading from v1.234.1 must keep the bare-ARN
// context name (kubectl --context <cluster-arn> keeps working), must not prefix
// the user with the hash, and must not pin ATMOS_AUTH_REALM in the exec env.
func TestManager_ExecuteIntegration_AutoRealmKeepsV1234ContextName(t *testing.T) {
	resetProcessIntegrationCache()
	t.Cleanup(resetProcessIntegrationCache)
	serveFakeEKS(t)
	path := filepath.Join(t.TempDir(), "kubeconfig")
	writeV1234Kubeconfig(t, path)

	m := newRealmTestManager(t, path, realmTestAutoRealm, "config-path")
	creds := &types.AWSCredentials{AccessKeyID: "test-access-key", SecretAccessKey: "test-secret-key", Region: "us-east-2"}
	require.NoError(t, m.executeIntegration(context.Background(), "dev/eks", creds))

	config, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, realmTestClusterARN, config.CurrentContext)
	require.Len(t, config.Contexts, 1)
	require.Contains(t, config.Contexts, realmTestClusterARN)
	wantUser := "atmos-eks-dev-cluster-us-east-2-123456789012-dev-user-49845aa7"
	assert.Equal(t, wantUser, config.Contexts[realmTestClusterARN].AuthInfo)
	require.Contains(t, config.AuthInfos, wantUser)
	for _, env := range config.AuthInfos[wantUser].Exec.Env {
		assert.NotEqual(t, "ATMOS_AUTH_REALM", env.Name, "an auto-derived realm must not be pinned in the exec env")
	}
}

func TestManager_IntegrationRealmSource(t *testing.T) {
	for _, tt := range []struct {
		name   string
		info   realm.RealmInfo
		config *schema.AuthConfig
		want   string
	}{
		{name: "nil config uses the manager source", info: realm.RealmInfo{Value: "acme", Source: realm.SourceConfig}, want: realm.SourceConfig},
		{name: "configured realm", info: realm.RealmInfo{Value: "acme", Source: realm.SourceConfig}, config: &schema.AuthConfig{RealmSource: "config"}, want: realm.SourceConfig},
		{name: "env realm", info: realm.RealmInfo{Value: "acme", Source: realm.SourceEnv}, config: &schema.AuthConfig{RealmSource: "env"}, want: realm.SourceEnv},
		{name: "hash of the config path", info: realm.RealmInfo{Value: realmTestAutoRealm, Source: realm.SourceConfig}, config: &schema.AuthConfig{RealmSource: "config-path"}, want: realm.SourceAuto},
		{name: "default realm", info: realm.RealmInfo{Value: "default", Source: realm.SourceConfig}, config: &schema.AuthConfig{RealmSource: "default"}, want: realm.SourceAuto},
		{name: "empty realm", info: realm.RealmInfo{Source: realm.SourceAuto}, config: &schema.AuthConfig{}, want: realm.SourceAuto},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &manager{realm: tt.info, config: tt.config}
			assert.Equal(t, tt.want, m.integrationRealmSource())
		})
	}
}

// TestManager_ExecuteIntegration_ExplicitRealmPrefixesContext is the negative
// path: a realm from auth.realm still prefixes the context and user names and
// is passed to the exec plugin.
func TestManager_ExecuteIntegration_ExplicitRealmPrefixesContext(t *testing.T) {
	resetProcessIntegrationCache()
	t.Cleanup(resetProcessIntegrationCache)
	serveFakeEKS(t)
	path := filepath.Join(t.TempDir(), "kubeconfig")

	m := newRealmTestManager(t, path, "acme", "config")
	creds := &types.AWSCredentials{AccessKeyID: "test-access-key", SecretAccessKey: "test-secret-key", Region: "us-east-2"}
	require.NoError(t, m.executeIntegration(context.Background(), "dev/eks", creds))

	config, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	wantContext := "acme/" + realmTestClusterARN
	wantUser := "atmos-acme-eks-dev-cluster-us-east-2-123456789012-dev-user-a9911425"
	assert.Equal(t, wantContext, config.CurrentContext)
	require.Contains(t, config.Contexts, wantContext)
	assert.Equal(t, wantUser, config.Contexts[wantContext].AuthInfo)
	require.Contains(t, config.AuthInfos, wantUser)
	env := map[string]string{}
	for _, entry := range config.AuthInfos[wantUser].Exec.Env {
		env[entry.Name] = entry.Value
	}
	assert.Equal(t, "acme", env["ATMOS_AUTH_REALM"])
}
