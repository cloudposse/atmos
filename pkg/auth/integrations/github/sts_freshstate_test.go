package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func rfc3339(t time.Time) string { return t.Format(time.RFC3339) }

func TestGitHubSTSHasFreshState(t *testing.T) {
	future := rfc3339(time.Now().Add(time.Hour))
	past := rfc3339(time.Now().Add(-time.Hour))

	tests := []struct {
		name   string
		tokens []stsToken
		want   bool
	}{
		{
			name:   "all tokens unexpired",
			tokens: []stsToken{{Host: "github.com", Owner: "acme", Token: "ghs_a", ExpiresAt: future}},
			want:   true,
		},
		{
			name:   "an expired token makes state stale",
			tokens: []stsToken{{Host: "github.com", Owner: "acme", Token: "ghs_a", ExpiresAt: past}},
			want:   false,
		},
		{
			name:   "mixed fresh and expired is stale",
			tokens: []stsToken{{Host: "github.com", Owner: "acme", Token: "ghs_a", ExpiresAt: future}, {Host: "github.com", Owner: "beta", Token: "ghs_b", ExpiresAt: past}},
			want:   false,
		},
		{
			name:   "unparseable expiry is stale",
			tokens: []stsToken{{Host: "github.com", Owner: "acme", Token: "ghs_a", ExpiresAt: "not-a-time"}},
			want:   false,
		},
		{
			name:   "empty token value is stale",
			tokens: []stsToken{{Host: "github.com", Owner: "acme", Token: "", ExpiresAt: future}},
			want:   false,
		},
		{
			name:   "no tokens is stale",
			tokens: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
			integ := newIntegration(t, "realmA", &schema.IntegrationSpec{GitConfigMode: GitConfigModeEnv}, &schema.IntegrationVia{Provider: "atmos-pro"})

			if tt.tokens != nil {
				require.NoError(t, integ.writeState(&gitSTSState{Tokens: tt.tokens}))
			}

			assert.Equal(t, tt.want, integ.hasFreshState())
		})
	}
}

func TestGitHubSTSHasFreshState_NoStateFile(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	integ := newIntegration(t, "realmA", nil, &schema.IntegrationVia{Provider: "atmos-pro"})
	assert.False(t, integ.hasFreshState(), "missing state file is not fresh")
}

func TestGitHubSTSHasFreshState_FileModeRequiresGitConfig(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())
	integ := newIntegration(t, "realmA", &schema.IntegrationSpec{GitConfigMode: GitConfigModeFile}, &schema.IntegrationVia{Provider: "atmos-pro"})

	// Fresh tokens but no git.config file yet → not fresh in file mode.
	require.NoError(t, integ.writeState(&gitSTSState{Tokens: []stsToken{{Host: "github.com", Owner: "acme", Token: "ghs_a", ExpiresAt: rfc3339(time.Now().Add(time.Hour))}}}))
	assert.False(t, integ.hasFreshState(), "file mode requires the on-disk git.config")

	// Once the git.config is written, the state is fresh.
	require.NoError(t, integ.writeGitConfigFile([]stsToken{{Host: "github.com", Owner: "acme", Token: "ghs_a", ExpiresAt: rfc3339(time.Now().Add(time.Hour))}}))
	assert.True(t, integ.hasFreshState())
}

func TestGitHubSTSExecute_ReusesFreshStateWithoutMinting(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())

	// A mint attempt against this server fails the test.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Errorf("github/sts must not mint when persisted state is fresh")
	}))
	defer srv.Close()

	integ := newIntegration(t, "realmA", &schema.IntegrationSpec{GitConfigMode: GitConfigModeEnv}, &schema.IntegrationVia{Provider: "atmos-pro"})
	require.NoError(t, integ.writeState(&gitSTSState{Tokens: []stsToken{
		{Host: "github.com", Owner: "acme", Token: "ghs_a", ExpiresAt: rfc3339(time.Now().Add(time.Hour))},
	}}))

	require.NoError(t, integ.Execute(context.Background(), proCreds(srv.URL)))
}

func TestGitHubSTSExecute_MintsWhenStateStale(t *testing.T) {
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())

	srv := stsServer(t, http.StatusOK, stsResponse{
		Tokens: []stsToken{{Host: "github.com", Owner: "acme", Token: "ghs_new", ExpiresAt: "2030-01-01T00:00:00Z"}},
	})
	defer srv.Close()

	integ := newIntegration(t, "realmA", &schema.IntegrationSpec{GitConfigMode: GitConfigModeEnv}, &schema.IntegrationVia{Provider: "atmos-pro"})

	// Seed expired state so the short-circuit does not apply.
	require.NoError(t, integ.writeState(&gitSTSState{Tokens: []stsToken{
		{Host: "github.com", Owner: "acme", Token: "ghs_old", ExpiresAt: rfc3339(time.Now().Add(-time.Hour))},
	}}))

	require.NoError(t, integ.Execute(context.Background(), proCreds(srv.URL)))

	// State replaced with the freshly minted token.
	state, err := integ.readState()
	require.NoError(t, err)
	require.Len(t, state.Tokens, 1)
	assert.Equal(t, "ghs_new", state.Tokens[0].Token)
}
