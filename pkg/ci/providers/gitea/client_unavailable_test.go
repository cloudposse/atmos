package gitea

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// clearAPIEnv removes every variable that selects the Gitea API URL, token, or HTTP opt-in.
func clearAPIEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		"ATMOS_CI_GITEA_API_URL", "GITEA_API_URL", "GITHUB_API_URL", "GITEA_SERVER_URL", "GITHUB_SERVER_URL",
		"ATMOS_CI_GITEA_ALLOW_INSECURE", "ATMOS_CI_GITEA_TOKEN", "GITEA_TOKEN",
	} {
		t.Setenv(name, "")
	}
}

// captureLog captures log output at the given level and restores the logger after the test.
func captureLog(t *testing.T, level log.Level) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	oldLevel := log.GetLevel()
	log.SetLevel(level)
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(oldLevel)
	})
	return &buf
}

// countingServer records how many requests reach it.
func countingServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"id":7}`))
	}))
	t.Cleanup(s.Close)
	return s, &hits
}

func TestUnusableAPIURLDisablesAPIFeaturesWithoutFailing(t *testing.T) {
	s, hits := countingServer(t)
	for name, configure := range map[string]func(){
		"missing":              func() {},
		"http without opt-in":  func() { t.Setenv("GITHUB_SERVER_URL", s.URL) },
		"http api url":         func() { t.Setenv("ATMOS_CI_GITEA_API_URL", s.URL+"/api/v1") },
		"http opt-in disabled": func() { t.Setenv("GITEA_API_URL", s.URL+"/api/v1"); t.Setenv("ATMOS_CI_GITEA_ALLOW_INSECURE", "false") },
	} {
		t.Run(name, func(t *testing.T) {
			clearAPIEnv(t)
			configure()
			t.Setenv("ATMOS_CI_GITEA_TOKEN", "test-token")
			p := NewProvider()

			require.NoError(t, p.ValidateConfig())

			err := p.request(context.Background(), http.MethodGet, "/user", nil, nil)
			require.ErrorIs(t, err, errUtils.ErrGiteaAPIUnavailable)
			require.NotErrorIs(t, err, errUtils.ErrInvalidURL, "an unusable URL must not abort the run")
			_, err = p.CreateCheckRun(context.Background(), &provider.CreateCheckRunOptions{Owner: "acme", Repo: "infra", SHA: "abc", Name: "atmos/plan"})
			require.ErrorIs(t, err, errUtils.ErrCICheckRunCreateFailed)
			require.ErrorIs(t, err, errUtils.ErrGiteaAPIUnavailable)
		})
	}
	assert.Zero(t, hits.Load(), "no request, and so no token, may reach an HTTP endpoint without the opt-in")
}

func TestHTTPAPIURLWithOptInSendsAuthenticatedRequests(t *testing.T) {
	var auth atomic.Value
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"id":7}`))
	}))
	t.Cleanup(s.Close)
	clearAPIEnv(t)
	t.Setenv("GITHUB_SERVER_URL", s.URL)
	t.Setenv("ATMOS_CI_GITEA_ALLOW_INSECURE", "true")
	t.Setenv("ATMOS_CI_GITEA_TOKEN", "test-token")
	p := NewProvider()

	require.NoError(t, p.ValidateConfig())
	var user apiUser
	require.NoError(t, p.request(context.Background(), http.MethodGet, "/user", nil, &user))
	assert.Equal(t, int64(7), user.ID)
	assert.Equal(t, "token test-token", auth.Load())
}

func TestHTTPSAPIURLIsUsableWithoutOptIn(t *testing.T) {
	clearAPIEnv(t)
	t.Setenv("GITHUB_SERVER_URL", "https://gitea.example.com/")
	u, err := apiURL()
	require.NoError(t, err)
	assert.Equal(t, "https://gitea.example.com/api/v1", u.String())
}

func TestAPIUnavailableIsLoggedOnce(t *testing.T) {
	t.Run("warning when a token is configured", func(t *testing.T) {
		buf := captureLog(t, log.WarnLevel)
		clearAPIEnv(t)
		t.Setenv("GITHUB_SERVER_URL", "http://gitea.internal:3000")
		t.Setenv("GITEA_TOKEN", "test-token")
		p := NewProvider()

		require.NoError(t, p.ValidateConfig())
		require.NoError(t, p.ValidateConfig())

		out := buf.String()
		assert.Equal(t, 1, strings.Count(out, apiUnavailableMessage), out)
		assert.Contains(t, out, "ATMOS_CI_GITEA_ALLOW_INSECURE=true")
		assert.NotContains(t, out, "test-token")
	})
	t.Run("debug only without a token", func(t *testing.T) {
		buf := captureLog(t, log.WarnLevel)
		clearAPIEnv(t)
		p := NewProvider()

		require.NoError(t, p.ValidateConfig())
		assert.Empty(t, buf.String())

		debug := captureLog(t, log.DebugLevel)
		other := NewProvider()
		require.NoError(t, other.ValidateConfig())
		require.NoError(t, other.ValidateConfig())
		assert.Equal(t, 1, strings.Count(debug.String(), apiUnavailableMessage), debug.String())
		assert.Contains(t, debug.String(), "no Gitea API URL is configured")
	})
}
