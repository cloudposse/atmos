package kube

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// TestWriteClusterConfig_MergeRepairsDriftedAtmosEntry verifies that when the
// Atmos-managed entries in a kubeconfig were edited out-of-band (by hand or by
// another tool), the next login treats the file as changed and restores the exec
// plugin, rather than short-circuiting on a false "no-op" and leaving kubectl
// with credentials or routing Atmos did not configure.
func TestWriteClusterConfig_MergeRepairsDriftedAtmosEntry(t *testing.T) {
	info := testClusterInfo()
	userName := UserName(info)
	contextName := "dev-eks"

	tests := []struct {
		name  string
		drift func(cfg *clientcmdapi.Config)
	}{
		{name: "static token added", drift: func(c *clientcmdapi.Config) { c.AuthInfos[userName].Token = "stolen-or-stale" }},
		{name: "impersonation added", drift: func(c *clientcmdapi.Config) { c.AuthInfos[userName].Impersonate = "system:admin" }},
		{name: "impersonation groups added", drift: func(c *clientcmdapi.Config) {
			c.AuthInfos[userName].ImpersonateGroups = []string{"system:masters"}
		}},
		{name: "impersonation extra added", drift: func(c *clientcmdapi.Config) {
			c.AuthInfos[userName].ImpersonateUserExtra = map[string][]string{"scopes": {"all"}}
		}},
		{name: "auth provider added", drift: func(c *clientcmdapi.Config) {
			c.AuthInfos[userName].AuthProvider = &clientcmdapi.AuthProviderConfig{Name: "oidc", Config: map[string]string{"idp": "x"}}
		}},
		{name: "exec env changed", drift: func(c *clientcmdapi.Config) {
			c.AuthInfos[userName].Exec.Env = []clientcmdapi.ExecEnvVar{{Name: "ATMOS_IDENTITY", Value: "prod-admin"}}
		}},
		{name: "exec args changed", drift: func(c *clientcmdapi.Config) {
			c.AuthInfos[userName].Exec.Args = []string{"aws", "eks", "token", "--cluster-name", "other"}
		}},
		{name: "exec command changed", drift: func(c *clientcmdapi.Config) { c.AuthInfos[userName].Exec.Command = "/tmp/evil" }},
		{name: "cluster TLS verification disabled", drift: func(c *clientcmdapi.Config) {
			c.Clusters[info.ID].InsecureSkipTLSVerify = true
			c.Clusters[info.ID].CertificateAuthorityData = nil
		}},
		{name: "cluster proxy added", drift: func(c *clientcmdapi.Config) { c.Clusters[info.ID].ProxyURL = "http://proxy:3128" }},
		{name: "context namespace changed", drift: func(c *clientcmdapi.Config) { c.Contexts[contextName].Namespace = "kube-system" }},
		{name: "context points at another user", drift: func(c *clientcmdapi.Config) { c.Contexts[contextName].AuthInfo = "someone-else" }},
		{name: "current context switched away", drift: func(c *clientcmdapi.Config) { c.CurrentContext = "other" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			mgr, err := NewKubeconfigManager(path, "")
			require.NoError(t, err)
			_, err = mgr.WriteClusterConfig(info, contextName, "merge")
			require.NoError(t, err)

			// Simulate out-of-band edits.
			cfg, err := clientcmd.LoadFromFile(path)
			require.NoError(t, err)
			tt.drift(cfg)
			require.NoError(t, clientcmd.WriteToFile(*cfg, path))

			changed, err := mgr.WriteClusterConfig(info, contextName, "merge")
			require.NoError(t, err)
			assert.True(t, changed, "drifted entry must be rewritten, not treated as a no-op")

			// The rewritten file must match exactly what Atmos builds.
			got, err := clientcmd.LoadFromFile(path)
			require.NoError(t, err)
			want := BuildClusterConfig(info, contextName)
			assert.False(t, mergeWouldChange(got, want), "rewritten kubeconfig must equal the Atmos-built entries")
			assert.Equal(t, contextName, got.CurrentContext)
			assert.Empty(t, got.AuthInfos[userName].Token)
			assert.Nil(t, got.AuthInfos[userName].AuthProvider)
		})
	}
}

// TestWriteClusterConfig_MergePreservesThirdPartyAuthInfo verifies that merging
// the Atmos entry leaves unrelated users intact, including their tokens,
// impersonation and auth providers, so `atmos ... update-kubeconfig` never clobbers a user's
// existing kubectl credentials.
func TestWriteClusterConfig_MergePreservesThirdPartyAuthInfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")

	existing := clientcmdapi.NewConfig()
	existing.Clusters["kind-local"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:6443", InsecureSkipTLSVerify: true}
	existing.AuthInfos["kind-user"] = &clientcmdapi.AuthInfo{
		Token:                "kind-token",
		Impersonate:          "jane",
		ImpersonateGroups:    []string{"devs"},
		ImpersonateUserExtra: map[string][]string{"reason": {"debug"}},
		AuthProvider:         &clientcmdapi.AuthProviderConfig{Name: "oidc", Config: map[string]string{"client-id": "abc"}},
	}
	existing.Contexts["kind"] = &clientcmdapi.Context{Cluster: "kind-local", AuthInfo: "kind-user", Namespace: "apps"}
	existing.CurrentContext = "kind"
	require.NoError(t, clientcmd.WriteToFile(*existing, path))

	mgr, err := NewKubeconfigManager(path, "")
	require.NoError(t, err)
	changed, err := mgr.WriteClusterConfig(testClusterInfo(), "dev-eks", "merge")
	require.NoError(t, err)
	assert.True(t, changed)

	got, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	assert.True(t, authInfosEqual(existing.AuthInfos["kind-user"], got.AuthInfos["kind-user"]),
		"third-party auth info must be preserved field-for-field")
	assert.True(t, clustersEqual(existing.Clusters["kind-local"], got.Clusters["kind-local"]))
	assert.True(t, contextsEqual(existing.Contexts["kind"], got.Contexts["kind"]))
	// Merge mode activates the new context, matching `aws eks update-kubeconfig`.
	assert.Equal(t, "dev-eks", got.CurrentContext)
}
