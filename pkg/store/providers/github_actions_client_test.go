package providers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"

	storepkg "github.com/cloudposse/atmos/pkg/store"
)

// ghTestKey is a deterministic Curve25519 public key (base64) the test server serves so that
// sealSecret can encrypt against it. The matching private key is retained to decrypt and assert.
type ghTestKey struct {
	pubB64 string
	pub    *[32]byte
	priv   *[32]byte
}

func newGHTestKey(t *testing.T) ghTestKey {
	t.Helper()
	pub, priv, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return ghTestKey{
		pubB64: base64.StdEncoding.EncodeToString(pub[:]),
		pub:    pub,
		priv:   priv,
	}
}

// newTestGitHubActionsAPIClient builds a githubActionsAPIClient whose go-github client points at
// the given httptest server (below the wrapper), exercising the real go-github request/response
// plumbing without any network access.
func newTestGitHubActionsAPIClient(t *testing.T, srv *httptest.Server, environment string) *githubActionsAPIClient {
	t.Helper()
	gh := github.NewClient(srv.Client())
	base, err := url.Parse(srv.URL + "/")
	require.NoError(t, err)
	gh.BaseURL = base
	return &githubActionsAPIClient{
		gh:          gh,
		owner:       "acme",
		repo:        "infra",
		environment: environment,
	}
}

// recordedRequest captures the method + path of an inbound request for assertions.
type recordedRequest struct {
	method string
	path   string
}

func TestGitHubActionsAPIClient_PutSecret_RepoScoped(t *testing.T) {
	key := newGHTestKey(t)
	var recorded []recordedRequest
	var captured github.EncryptedSecret

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/infra/actions/secrets/public-key", func(w http.ResponseWriter, r *http.Request) {
		recorded = append(recorded, recordedRequest{r.Method, r.URL.Path})
		writeJSON(t, w, map[string]any{"key_id": "kid-1", "key": key.pubB64})
	})
	mux.HandleFunc("/repos/acme/infra/actions/secrets/DB_PASSWORD", func(w http.ResponseWriter, r *http.Request) {
		recorded = append(recorded, recordedRequest{r.Method, r.URL.Path})
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestGitHubActionsAPIClient(t, srv, "")
	require.NoError(t, c.PutSecret(context.Background(), "DB_PASSWORD", "s3cr3t"))

	require.Len(t, recorded, 2)
	assert.Equal(t, recordedRequest{http.MethodGet, "/repos/acme/infra/actions/secrets/public-key"}, recorded[0])
	assert.Equal(t, recordedRequest{http.MethodPut, "/repos/acme/infra/actions/secrets/DB_PASSWORD"}, recorded[1])

	assert.Equal(t, "kid-1", captured.KeyID)

	// The captured ciphertext must decrypt back to the plaintext with the matching keypair.
	sealed, err := base64.StdEncoding.DecodeString(captured.EncryptedValue)
	require.NoError(t, err)
	decrypted, ok := box.OpenAnonymous(nil, sealed, key.pub, key.priv)
	require.True(t, ok, "sealed box should decrypt with the test keypair")
	assert.Equal(t, "s3cr3t", string(decrypted))
}

func TestGitHubActionsAPIClient_PutSecret_EnvScoped(t *testing.T) {
	key := newGHTestKey(t)
	var recorded []recordedRequest

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/infra", func(w http.ResponseWriter, r *http.Request) {
		recorded = append(recorded, recordedRequest{r.Method, r.URL.Path})
		writeJSON(t, w, map[string]any{"id": 12345, "name": "infra"})
	})
	mux.HandleFunc("/repositories/12345/environments/production/secrets/public-key", func(w http.ResponseWriter, r *http.Request) {
		recorded = append(recorded, recordedRequest{r.Method, r.URL.Path})
		writeJSON(t, w, map[string]any{"key_id": "kid-env", "key": key.pubB64})
	})
	mux.HandleFunc("/repositories/12345/environments/production/secrets/DB_PASSWORD", func(w http.ResponseWriter, r *http.Request) {
		recorded = append(recorded, recordedRequest{r.Method, r.URL.Path})
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestGitHubActionsAPIClient(t, srv, "production")
	require.NoError(t, c.PutSecret(context.Background(), "DB_PASSWORD", "v"))

	require.Len(t, recorded, 3)
	assert.Equal(t, "/repos/acme/infra", recorded[0].path)
	assert.Equal(t, "/repositories/12345/environments/production/secrets/public-key", recorded[1].path)
	assert.Equal(t, recordedRequest{http.MethodPut, "/repositories/12345/environments/production/secrets/DB_PASSWORD"}, recorded[2])
}

func TestGitHubActionsAPIClient_PutSecret_PublicKeyError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/infra/actions/secrets/public-key", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestGitHubActionsAPIClient(t, srv, "")
	err := c.PutSecret(context.Background(), "DB_PASSWORD", "v")
	require.ErrorIs(t, err, storepkg.ErrGitHubGetPublicKey)
}

func TestGitHubActionsAPIClient_PutSecret_RepoIDError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/infra", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestGitHubActionsAPIClient(t, srv, "production")
	err := c.PutSecret(context.Background(), "DB_PASSWORD", "v")
	require.ErrorIs(t, err, storepkg.ErrGitHubResolveRepoID)
}

func TestGitHubActionsAPIClient_PutSecret_PutError(t *testing.T) {
	key := newGHTestKey(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/infra/actions/secrets/public-key", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"key_id": "kid-1", "key": key.pubB64})
	})
	mux.HandleFunc("/repos/acme/infra/actions/secrets/DB_PASSWORD", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestGitHubActionsAPIClient(t, srv, "")
	err := c.PutSecret(context.Background(), "DB_PASSWORD", "v")
	require.ErrorIs(t, err, storepkg.ErrGitHubPutSecret)
	assert.Contains(t, err.Error(), "DB_PASSWORD")
}

func TestGitHubActionsAPIClient_HasSecret(t *testing.T) {
	t.Run("repo secret exists", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/acme/infra/actions/secrets/DB_PASSWORD", func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			writeJSON(t, w, map[string]any{"name": "DB_PASSWORD"})
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := newTestGitHubActionsAPIClient(t, srv, "")
		has, err := c.HasSecret(context.Background(), "DB_PASSWORD")
		require.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("repo secret missing returns false", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/acme/infra/actions/secrets/DB_PASSWORD", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := newTestGitHubActionsAPIClient(t, srv, "")
		has, err := c.HasSecret(context.Background(), "DB_PASSWORD")
		require.NoError(t, err)
		assert.False(t, has)
	})

	t.Run("transport error propagates", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/acme/infra/actions/secrets/DB_PASSWORD", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := newTestGitHubActionsAPIClient(t, srv, "")
		_, err := c.HasSecret(context.Background(), "DB_PASSWORD")
		require.ErrorIs(t, err, storepkg.ErrGitHubGetSecret)
	})

	t.Run("env secret exists", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/acme/infra", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, map[string]any{"id": 99})
		})
		mux.HandleFunc("/repositories/99/environments/production/secrets/DB_PASSWORD", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, map[string]any{"name": "DB_PASSWORD"})
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := newTestGitHubActionsAPIClient(t, srv, "production")
		has, err := c.HasSecret(context.Background(), "DB_PASSWORD")
		require.NoError(t, err)
		assert.True(t, has)
	})

	t.Run("env repoID error propagates", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/acme/infra", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := newTestGitHubActionsAPIClient(t, srv, "production")
		_, err := c.HasSecret(context.Background(), "DB_PASSWORD")
		require.ErrorIs(t, err, storepkg.ErrGitHubResolveRepoID)
	})
}

func TestGitHubActionsAPIClient_DeleteSecret(t *testing.T) {
	t.Run("repo secret deleted", func(t *testing.T) {
		var recorded []recordedRequest
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/acme/infra/actions/secrets/DB_PASSWORD", func(w http.ResponseWriter, r *http.Request) {
			recorded = append(recorded, recordedRequest{r.Method, r.URL.Path})
			w.WriteHeader(http.StatusNoContent)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := newTestGitHubActionsAPIClient(t, srv, "")
		require.NoError(t, c.DeleteSecret(context.Background(), "DB_PASSWORD"))
		require.Len(t, recorded, 1)
		assert.Equal(t, http.MethodDelete, recorded[0].method)
	})

	t.Run("missing secret is idempotent", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/acme/infra/actions/secrets/DB_PASSWORD", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := newTestGitHubActionsAPIClient(t, srv, "")
		assert.NoError(t, c.DeleteSecret(context.Background(), "DB_PASSWORD"))
	})

	t.Run("transport error propagates", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/acme/infra/actions/secrets/DB_PASSWORD", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := newTestGitHubActionsAPIClient(t, srv, "")
		err := c.DeleteSecret(context.Background(), "DB_PASSWORD")
		require.ErrorIs(t, err, storepkg.ErrGitHubDeleteSecret)
	})

	t.Run("env secret deleted", func(t *testing.T) {
		var recorded []recordedRequest
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/acme/infra", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, map[string]any{"id": 7})
		})
		mux.HandleFunc("/repositories/7/environments/production/secrets/DB_PASSWORD", func(w http.ResponseWriter, r *http.Request) {
			recorded = append(recorded, recordedRequest{r.Method, r.URL.Path})
			w.WriteHeader(http.StatusNoContent)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := newTestGitHubActionsAPIClient(t, srv, "production")
		require.NoError(t, c.DeleteSecret(context.Background(), "DB_PASSWORD"))
		require.Len(t, recorded, 1)
		assert.Equal(t, http.MethodDelete, recorded[0].method)
	})

	t.Run("env repoID error propagates", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/repos/acme/infra", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		c := newTestGitHubActionsAPIClient(t, srv, "production")
		err := c.DeleteSecret(context.Background(), "DB_PASSWORD")
		require.ErrorIs(t, err, storepkg.ErrGitHubResolveRepoID)
	})
}

func TestGitHubActionsAPIClient_RepoIDCachedOnce(t *testing.T) {
	var repoGets int
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/infra", func(w http.ResponseWriter, r *http.Request) {
		repoGets++
		writeJSON(t, w, map[string]any{"id": 555})
	})
	mux.HandleFunc("/repositories/555/environments/production/secrets/A", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"name": "A"})
	})
	mux.HandleFunc("/repositories/555/environments/production/secrets/B", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"name": "B"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestGitHubActionsAPIClient(t, srv, "production")
	_, err := c.HasSecret(context.Background(), "A")
	require.NoError(t, err)
	_, err = c.HasSecret(context.Background(), "B")
	require.NoError(t, err)
	assert.Equal(t, 1, repoGets, "repository ID should be resolved at most once (sync.Once)")
}

func TestGhIsNotFound(t *testing.T) {
	assert.True(t, ghIsNotFound(&github.Response{Response: &http.Response{StatusCode: http.StatusNotFound}}, nil))
	assert.False(t, ghIsNotFound(&github.Response{Response: &http.Response{StatusCode: http.StatusOK}}, nil))
	assert.False(t, ghIsNotFound(nil, nil))

	ge := &github.ErrorResponse{Response: &http.Response{StatusCode: http.StatusNotFound}}
	assert.True(t, ghIsNotFound(nil, ge))

	geOther := &github.ErrorResponse{Response: &http.Response{StatusCode: http.StatusForbidden}}
	assert.False(t, ghIsNotFound(nil, geOther))
}

func TestResolveGitHubToken(t *testing.T) {
	t.Run("explicit wins", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "env-pro")
		assert.Equal(t, "explicit", resolveGitHubToken("explicit"))
	})

	t.Run("first env var in precedence order", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
		t.Setenv("ATMOS_GITHUB_TOKEN", "atmos-gh")
		t.Setenv("GITHUB_TOKEN", "plain-gh")
		assert.Equal(t, "atmos-gh", resolveGitHubToken(""))
	})

	t.Run("falls through to GITHUB_TOKEN", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
		t.Setenv("ATMOS_GITHUB_TOKEN", "")
		t.Setenv("GITHUB_TOKEN", "plain-gh")
		assert.Equal(t, "plain-gh", resolveGitHubToken(""))
	})

	t.Run("none set returns empty", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
		t.Setenv("ATMOS_GITHUB_TOKEN", "")
		t.Setenv("GITHUB_TOKEN", "")
		assert.Empty(t, resolveGitHubToken("  "))
	})
}

func TestNewGitHubActionsAPIClient(t *testing.T) {
	t.Run("with token", func(t *testing.T) {
		c := newGitHubActionsAPIClient(&GitHubActionsStoreOptions{
			Owner: "acme", Repo: "infra", Environment: "prod", Token: "t",
		})
		api, ok := c.(*githubActionsAPIClient)
		require.True(t, ok)
		assert.Equal(t, "acme", api.owner)
		assert.Equal(t, "infra", api.repo)
		assert.Equal(t, "prod", api.environment)
		assert.NotNil(t, api.gh)
	})

	t.Run("without token", func(t *testing.T) {
		t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
		t.Setenv("ATMOS_GITHUB_TOKEN", "")
		t.Setenv("GITHUB_TOKEN", "")
		c := newGitHubActionsAPIClient(&GitHubActionsStoreOptions{Owner: "acme", Repo: "infra"})
		api, ok := c.(*githubActionsAPIClient)
		require.True(t, ok)
		assert.NotNil(t, api.gh)
	})
}

// writeJSON marshals v and writes it as the response body, failing the test on error.
func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	b, err := json.Marshal(v)
	require.NoError(t, err)
	_, err = w.Write(b)
	require.NoError(t, err)
}
