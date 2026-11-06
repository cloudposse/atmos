package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	execpkg "github.com/cloudposse/atmos/pkg/exec"
)

func TestReleaseClientBudget(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		permitted  bool
	}{
		{"actions boundary", `{"resources":{"core":{"limit":1000,"remaining":750}}}`, 200, true},
		{"actions low", `{"resources":{"core":{"limit":1000,"remaining":749}}}`, 200, false},
		{"PAT boundary", `{"resources":{"core":{"limit":5000,"remaining":3750}}}`, 200, true},
		{"PAT low", `{"resources":{"core":{"limit":5000,"remaining":3749}}}`, 200, false},
		{"fractional boundary", `{"resources":{"core":{"limit":3,"remaining":2}}}`, 200, false},
		{"missing", `{}`, 200, false},
		{"zero limit", `{"resources":{"core":{"limit":0,"remaining":0}}}`, 200, false},
		{"malformed", `broken`, 200, false},
		{"unauthorized", `{}`, 401, false},
		{"unavailable", `{}`, 503, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearGitHubEndpointEnv(t)
			t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
			t.Setenv("ATMOS_GITHUB_TOKEN", "test-token")
			t.Setenv("GITHUB_TOKEN", "")
			t.Setenv("ATMOS_GITHUB_CLI", "")
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
				assert.Equal(t, "/api/v3/rate_limit", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			original := http.DefaultTransport
			http.DefaultTransport = server.Client().Transport
			t.Cleanup(func() { http.DefaultTransport = original })
			t.Setenv("GITHUB_SERVER_URL", server.URL)
			t.Setenv("GITHUB_API_URL", server.URL+"/api/v3")
			client, authenticated := NewReleaseClient(context.Background())
			require.True(t, authenticated)
			err := client.CheckBudget(context.Background())
			if tc.permitted {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			assert.Equal(t, 1, calls)
		})
	}
}

func TestReleaseClientEffectiveAuth(t *testing.T) {
	clearGitHubEndpointEnv(t)
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_CLI", "")
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unauthenticated gate must not request anything") }))
	defer server.Close()
	t.Setenv("GITHUB_SERVER_URL", server.URL)
	t.Setenv("GITHUB_API_URL", server.URL+"/api/v3")
	_, authenticated := NewReleaseClient(context.Background())
	assert.False(t, authenticated, "a token must be withheld on plain HTTP")
}

func TestReleaseClientResolvesCLIAuthenticationOnce(t *testing.T) {
	clearGitHubEndpointEnv(t)
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_CLI", "gh")
	controller := gomock.NewController(t)
	commander := execpkg.NewMockCommandExecutor(controller)
	commander.EXPECT().CommandContext(gomock.Any(), "gh", "auth", "token").Return(fakeCLICmd("test-token", 0)).Times(1)
	withCommander(t, commander)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/rate_limit") {
			_, _ = w.Write([]byte(`{"resources":{"core":{"limit":1000,"remaining":1000}}}`))
		} else {
			_, _ = w.Write([]byte(`{"tag_name":"v1.110.0"}`))
		}
	}))
	defer server.Close()
	original := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = original })
	t.Setenv("GITHUB_SERVER_URL", server.URL)
	t.Setenv("GITHUB_API_URL", server.URL+"/api/v3")
	client, authenticated := NewReleaseClient(context.Background())
	require.True(t, authenticated)
	require.NoError(t, client.CheckBudget(context.Background()))
	latest, err := client.Latest(context.Background(), "cloudposse", "atmos")
	require.NoError(t, err)
	assert.Equal(t, "v1.110.0", latest)
}
