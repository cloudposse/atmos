package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newIntegration builds a github/sts integration with the given realm and spec.
func newIntegration(t *testing.T, realm string, spec *schema.IntegrationSpec, via *schema.IntegrationVia) *GitHubSTSIntegration {
	t.Helper()
	cfg := &integrations.IntegrationConfig{
		Name:   "github-sts",
		Realm:  realm,
		Config: &schema.Integration{Kind: integrations.KindGitHubSTS, Via: via, Spec: spec},
	}
	integ, err := NewGitHubSTSIntegration(cfg)
	require.NoError(t, err)
	return integ.(*GitHubSTSIntegration)
}

func proCreds(baseURL string) *types.ProCredentials {
	return &types.ProCredentials{Token: "session-jwt", BaseURL: baseURL, Endpoint: "api/v1", WorkspaceID: "ws-1"}
}

// stateFilePath returns the expected state file path for a realm/name under the XDG data dir.
func stateFilePath(xdgData, realm, name, file string) string {
	return filepath.Join(xdgData, "atmos", "auth", "github-sts", realm, name, file)
}

func TestNewGitHubSTSIntegration_ViaValidation(t *testing.T) {
	tests := []struct {
		name    string
		via     *schema.IntegrationVia
		wantErr error
	}{
		{"provider only", &schema.IntegrationVia{Provider: "atmos-pro"}, nil},
		{"identity only", &schema.IntegrationVia{Identity: "atmos-pro"}, nil},
		{"neither", &schema.IntegrationVia{}, errUtils.ErrIntegrationViaMissing},
		{"nil via", nil, errUtils.ErrIntegrationViaMissing},
		{"both", &schema.IntegrationVia{Identity: "a", Provider: "b"}, errUtils.ErrIntegrationViaAmbiguous},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewGitHubSTSIntegration(&integrations.IntegrationConfig{
				Name:   "github-sts",
				Config: &schema.Integration{Kind: integrations.KindGitHubSTS, Via: tc.via},
			})
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestNewGitHubSTSIntegration_InvalidGitConfigMode(t *testing.T) {
	_, err := NewGitHubSTSIntegration(&integrations.IntegrationConfig{
		Name: "github-sts",
		Config: &schema.Integration{
			Kind: integrations.KindGitHubSTS,
			Via:  &schema.IntegrationVia{Provider: "atmos-pro"},
			Spec: &schema.IntegrationSpec{GitConfigMode: "bogus"},
		},
	})
	require.ErrorIs(t, err, errUtils.ErrIntegrationFailed)
}

// stsServer returns an httptest server that serves POST /api/v1/sts with the given response.
func stsServer(t *testing.T, status int, resp stsResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/sts", r.URL.Path)
		assert.Equal(t, "Bearer session-jwt", r.Header.Get("Authorization"))
		w.WriteHeader(status)
		if status == http.StatusOK {
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
}

func TestGitHubSTSExecuteAndEnvironment_EnvMode(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("ATMOS_XDG_DATA_HOME", xdg)

	srv := stsServer(t, http.StatusOK, stsResponse{
		Tokens: []stsToken{
			{Host: "github.com", Owner: "acme", Token: "ghs_acme", ExpiresAt: "2030-01-01T00:00:00Z"},
			{Host: "github.com", Owner: "beta", Token: "ghs_beta", ExpiresAt: "2030-01-01T00:00:00Z"},
		},
		Excluded: []stsExclusion{{Repo: "other/repo", Reason: "no_trust_policy"}},
	})
	defer srv.Close()

	integ := newIntegration(t, "realmA", &schema.IntegrationSpec{GitConfigMode: GitConfigModeEnv}, &schema.IntegrationVia{Provider: "atmos-pro"})

	require.NoError(t, integ.Execute(context.Background(), proCreds(srv.URL)))

	// State file written with restrictive perms.
	statePath := stateFilePath(xdg, "realmA", "github-sts", stateFileName)
	info, err := os.Stat(statePath)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	env, err := integ.Environment()
	require.NoError(t, err)

	// 2 owners x (https + ssh) = 4 entries.
	assert.Equal(t, "4", env["GIT_CONFIG_COUNT"])

	// Collect key=value pairs into a set for order-independent assertions.
	pairs := map[string]string{}
	for i := 0; i < 4; i++ {
		k := env["GIT_CONFIG_KEY_"+itoa(i)]
		v := env["GIT_CONFIG_VALUE_"+itoa(i)]
		require.NotEmpty(t, k)
		pairs[k+" => "+v] = v
	}
	assert.Contains(t, pairs, "url.https://x-access-token:ghs_acme@github.com/acme/.insteadOf => https://github.com/acme/")
	assert.Contains(t, pairs, "url.https://x-access-token:ghs_acme@github.com/acme/.insteadOf => ssh://git@github.com/acme/")
	assert.Contains(t, pairs, "url.https://x-access-token:ghs_beta@github.com/beta/.insteadOf => https://github.com/beta/")
	assert.Contains(t, pairs, "url.https://x-access-token:ghs_beta@github.com/beta/.insteadOf => ssh://git@github.com/beta/")
}

func TestGitHubSTSEnvironment_FileMode(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("ATMOS_XDG_DATA_HOME", xdg)

	srv := stsServer(t, http.StatusOK, stsResponse{
		Tokens: []stsToken{{Host: "github.com", Owner: "acme", Token: "ghs_acme"}},
	})
	defer srv.Close()

	integ := newIntegration(t, "realmA", &schema.IntegrationSpec{GitConfigMode: GitConfigModeFile}, &schema.IntegrationVia{Provider: "atmos-pro"})
	require.NoError(t, integ.Execute(context.Background(), proCreds(srv.URL)))

	configPath := stateFilePath(xdg, "realmA", "github-sts", configFileName)
	info, err := os.Stat(configPath)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `[url "https://x-access-token:ghs_acme@github.com/acme/"]`)
	assert.Contains(t, string(content), "insteadOf = https://github.com/acme/")
	assert.Contains(t, string(content), "insteadOf = ssh://git@github.com/acme/")

	env, err := integ.Environment()
	require.NoError(t, err)
	assert.Equal(t, "1", env["GIT_CONFIG_COUNT"])
	assert.Equal(t, "include.path", env["GIT_CONFIG_KEY_0"])
	assert.Equal(t, configPath, env["GIT_CONFIG_VALUE_0"])
}

func TestGitHubSTSExecute_EmptyTokensIsSuccess(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("ATMOS_XDG_DATA_HOME", xdg)

	srv := stsServer(t, http.StatusOK, stsResponse{
		Tokens:   nil,
		Excluded: []stsExclusion{{Repo: "a/b", Reason: "not_installed_in_workspace"}},
	})
	defer srv.Close()

	integ := newIntegration(t, "realmA", nil, &schema.IntegrationVia{Provider: "atmos-pro"})
	require.NoError(t, integ.Execute(context.Background(), proCreds(srv.URL)))

	env, err := integ.Environment()
	require.NoError(t, err)
	assert.Empty(t, env, "no tokens means no GIT_CONFIG_* output")
}

func TestGitHubSTSExecute_StatusErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr error
	}{
		{"bad request", http.StatusBadRequest, errUtils.ErrNotGitHubActionsSession},
		{"forbidden", http.StatusForbidden, errUtils.ErrSTSNoEntitlement},
		{"server error", http.StatusInternalServerError, errUtils.ErrSTSMintFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
			srv := stsServer(t, tc.status, stsResponse{})
			defer srv.Close()

			integ := newIntegration(t, "realmA", nil, &schema.IntegrationVia{Provider: "atmos-pro"})
			err := integ.Execute(context.Background(), proCreds(srv.URL))
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestGitHubSTSExecute_WrongCredentialsType(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	integ := newIntegration(t, "realmA", nil, &schema.IntegrationVia{Provider: "atmos-pro"})
	err := integ.Execute(context.Background(), &types.OIDCCredentials{Token: "x"})
	require.ErrorIs(t, err, errUtils.ErrProCredentialsType)
}

func TestGitHubSTSCleanup_RevokesAndRemoves(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("ATMOS_XDG_DATA_HOME", xdg)

	srv := stsServer(t, http.StatusOK, stsResponse{
		Tokens: []stsToken{{Host: "github.com", Owner: "acme", Token: "ghs_acme"}},
	})
	defer srv.Close()

	var revokedTokens []string
	revokeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/installation/token", r.URL.Path)
		revokedTokens = append(revokedTokens, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer revokeSrv.Close()

	oldBase := githubAPIBaseURL
	githubAPIBaseURL = revokeSrv.URL
	defer func() { githubAPIBaseURL = oldBase }()

	integ := newIntegration(t, "realmA", nil, &schema.IntegrationVia{Provider: "atmos-pro"})
	require.NoError(t, integ.Execute(context.Background(), proCreds(srv.URL)))

	require.NoError(t, integ.Cleanup(context.Background()))

	assert.Equal(t, []string{"Bearer ghs_acme"}, revokedTokens, "the minted token must be revoked exactly once")

	// State file removed; Environment is now empty.
	_, err := os.Stat(stateFilePath(xdg, "realmA", "github-sts", stateFileName))
	assert.True(t, os.IsNotExist(err), "state file must be removed after cleanup")

	env, err := integ.Environment()
	require.NoError(t, err)
	assert.Empty(t, env)
}

func TestGitHubSTSCleanup_Idempotent(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	integ := newIntegration(t, "realmA", nil, &schema.IntegrationVia{Provider: "atmos-pro"})
	// No state file written — cleanup is a no-op success.
	require.NoError(t, integ.Cleanup(context.Background()))
}

func TestGitHubSTSRealmIsolation(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("ATMOS_XDG_DATA_HOME", xdg)

	srv := stsServer(t, http.StatusOK, stsResponse{
		Tokens: []stsToken{{Host: "github.com", Owner: "acme", Token: "ghs_acme"}},
	})
	defer srv.Close()

	// Mint in realmA only.
	integA := newIntegration(t, "realmA", &schema.IntegrationSpec{GitConfigMode: GitConfigModeEnv}, &schema.IntegrationVia{Provider: "atmos-pro"})
	require.NoError(t, integA.Execute(context.Background(), proCreds(srv.URL)))

	// realmB sees nothing.
	integB := newIntegration(t, "realmB", &schema.IntegrationSpec{GitConfigMode: GitConfigModeEnv}, &schema.IntegrationVia{Provider: "atmos-pro"})
	env, err := integB.Environment()
	require.NoError(t, err)
	assert.Empty(t, env, "realmB must not see realmA's minted tokens")

	// realmA still sees its own.
	envA, err := integA.Environment()
	require.NoError(t, err)
	assert.Equal(t, "2", envA["GIT_CONFIG_COUNT"])
}

// itoa is a tiny helper to avoid importing strconv in tests just for indices.
func itoa(i int) string {
	return string(rune('0' + i))
}
