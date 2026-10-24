package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGitHubSTSAccessors(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	integ := newIntegration(t, "realmA", nil, &schema.IntegrationVia{Identity: "atmos-pro"})

	assert.Equal(t, integrations.KindGitHubSTS, integ.Kind())
	assert.Equal(t, "atmos-pro", integ.GetIdentity())
	assert.Empty(t, integ.GetProvider())
}

func TestGitHubSTSExecute_EmptyProToken(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	integ := newIntegration(t, "realmA", nil, &schema.IntegrationVia{Provider: "atmos-pro"})

	err := integ.Execute(context.Background(), &types.ProCredentials{Token: ""})
	require.ErrorIs(t, err, errUtils.ErrSTSMintFailed)
}

func TestGitHubSTSExecute_MintNetworkError(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())

	// Start then immediately close the server so the mint connection is refused.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	integ := newIntegration(t, "realmA", nil, &schema.IntegrationVia{Provider: "atmos-pro"})
	err := integ.Execute(context.Background(), proCreds(url))
	require.ErrorIs(t, err, errUtils.ErrSTSMintFailed)
}

func TestGitHubSTSExecute_MintBadJSON(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	integ := newIntegration(t, "realmA", nil, &schema.IntegrationVia{Provider: "atmos-pro"})
	err := integ.Execute(context.Background(), proCreds(srv.URL))
	require.ErrorIs(t, err, errUtils.ErrSTSMintFailed)
}

// TestGitHubSTSCleanup_RevokeStatuses verifies that 401/404 are treated as already-revoked
// successes and a 500 default status is a non-fatal warning — Cleanup always succeeds and
// removes the persisted state regardless of the revoke outcome.
func TestGitHubSTSCleanup_RevokeStatuses(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"already invalid 401", http.StatusUnauthorized},
		{"not found 404", http.StatusNotFound},
		{"server error 500 is non-fatal", http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			xdg := t.TempDir()
			t.Setenv("ATMOS_XDG_DATA_HOME", xdg)

			mintSrv := stsServer(t, http.StatusOK, stsResponse{
				Tokens: []stsToken{{Host: "github.com", Owner: "acme", Token: "ghs_acme", ExpiresAt: "2030-01-01T00:00:00Z"}},
			})
			defer mintSrv.Close()

			revokeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer revokeSrv.Close()

			oldBase := githubAPIBaseURL
			githubAPIBaseURL = revokeSrv.URL
			defer func() { githubAPIBaseURL = oldBase }()

			integ := newIntegration(t, "realmA", nil, &schema.IntegrationVia{Provider: "atmos-pro"})
			require.NoError(t, integ.Execute(context.Background(), proCreds(mintSrv.URL)))

			require.NoError(t, integ.Cleanup(context.Background()))

			_, err := os.Stat(stateFilePath(xdg, stateFileName))
			assert.True(t, os.IsNotExist(err), "state file must be removed after cleanup")
		})
	}
}

// TestGitHubSTSCleanup_RevokeNetworkErrorNonFatal verifies that a network failure during
// revocation is logged and swallowed: Cleanup still succeeds and removes state.
func TestGitHubSTSCleanup_RevokeNetworkErrorNonFatal(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("ATMOS_XDG_DATA_HOME", xdg)

	mintSrv := stsServer(t, http.StatusOK, stsResponse{
		Tokens: []stsToken{{Host: "github.com", Owner: "acme", Token: "ghs_acme", ExpiresAt: "2030-01-01T00:00:00Z"}},
	})
	defer mintSrv.Close()

	// A closed revoke endpoint forces a connection error in revokeToken.
	revokeSrv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	revokeURL := revokeSrv.URL
	revokeSrv.Close()

	oldBase := githubAPIBaseURL
	githubAPIBaseURL = revokeURL
	defer func() { githubAPIBaseURL = oldBase }()

	integ := newIntegration(t, "realmA", nil, &schema.IntegrationVia{Provider: "atmos-pro"})
	require.NoError(t, integ.Execute(context.Background(), proCreds(mintSrv.URL)))

	require.NoError(t, integ.Cleanup(context.Background()))

	_, err := os.Stat(stateFilePath(xdg, stateFileName))
	assert.True(t, os.IsNotExist(err), "state file must be removed even when revoke fails")
}

// TestGitHubSTSEnvironment_SkipsEmptyHostOwner verifies env mode skips tokens missing the
// host or owner (which cannot form a valid insteadOf rewrite), yielding no GIT_CONFIG_*.
func TestGitHubSTSEnvironment_SkipsEmptyHostOwner(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	integ := newIntegration(t, "realmA", &schema.IntegrationSpec{GitConfigMode: GitConfigModeEnv}, &schema.IntegrationVia{Provider: "atmos-pro"})

	require.NoError(t, integ.writeState(&gitSTSState{Tokens: []stsToken{
		{Host: "github.com", Owner: "", Token: "ghs_a"},
		{Host: "", Owner: "acme", Token: "ghs_b"},
	}}))

	env, err := integ.Environment()
	require.NoError(t, err)
	// Tokens missing host or owner cannot form a valid insteadOf rewrite, so no GIT_CONFIG_* is emitted.
	// (The default token_env raw-token bridge is a separate concern and may still surface an
	// owner-bearing token; this test asserts only the GIT_CONFIG_* invariant.)
	for k := range env {
		assert.False(t, strings.HasPrefix(k, "GIT_CONFIG_"), "unexpected GIT_CONFIG_* key %q", k)
	}
}

func TestOwnersSummary_Dedup(t *testing.T) {
	tokens := []stsToken{
		{Owner: "acme"},
		{Owner: "acme"},
		{Owner: "beta"},
	}
	assert.Equal(t, "acme, beta", ownersSummary(tokens), "duplicate owners must be collapsed once")
}
