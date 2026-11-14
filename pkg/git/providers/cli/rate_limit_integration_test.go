package cli

import (
	"fmt"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Exercise twelve independent publishers against the same branch over smart HTTP.
// Each receives a 429 before contending for the ref; no lock serializes publication.
func TestIndependentPublishersHonorHTTPCooldown(t *testing.T) {
	requireGitBinary(t)
	env := gitTestEnv(t)
	root := t.TempDir()
	bare := filepath.Join(root, "repo.git")
	gitRun(t, env, "", "init", "--bare", bare)
	gitRun(t, env, bare, "config", "http.receivepack", "true")
	seed := t.TempDir()
	gitRun(t, env, seed, "init")
	writeFile(t, filepath.Join(seed, "manifest.yaml"), "image: v1\n")
	gitRun(t, env, seed, "add", ".")
	gitRun(t, env, seed, "commit", "-m", "seed")
	gitRun(t, env, seed, "push", bare, "main")
	backend := &cgi.Handler{Path: filepath.Join(strings.TrimSpace(gitRun(t, env, "", "--exec-path")), "git-http-backend"), Env: append(env, "GIT_PROJECT_ROOT="+root, "GIT_HTTP_EXPORT_ALL=1")}
	var mu sync.Mutex
	first := map[string]time.Time{}
	second := map[string]time.Duration{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, path, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
		mu.Lock()
		at, seen := first[client]
		if !seen {
			first[client] = time.Now()
		} else if _, recorded := second[client]; !recorded {
			second[client] = time.Since(at)
		}
		mu.Unlock()
		if !seen {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		r.URL.Path = "/" + path
		backend.ServeHTTP(w, r)
	}))
	defer server.Close()
	const publishers = 12
	workdirs := make([]string, publishers)
	for i := range workdirs {
		workdirs[i] = t.TempDir()
		gitRun(t, env, "", "clone", seed, workdirs[i])
		writeFile(t, filepath.Join(workdirs[i], fmt.Sprintf("app-%d.yaml", i)), "image: v2\n")
		gitRun(t, env, workdirs[i], "add", ".")
		gitRun(t, env, workdirs[i], "commit", "-m", fmt.Sprintf("publish app %d", i))
	}
	start := make(chan struct{})
	results := make(chan error, publishers)
	for i, dir := range workdirs {
		go func() {
			<-start
			results <- New().Push(t.Context(), &atmosgit.PushOptions{RepoContext: atmosgit.RepoContext{Workdir: dir, Env: env, Remote: fmt.Sprintf("%s/client-%d/repo.git", server.URL, i), Branch: "main"}, Retry: &schema.RetryConfig{MaxAttempts: new(16), MaxElapsedTime: new(30 * time.Second), RandomJitter: new(0.5), BackoffStrategy: schema.BackoffExponential, InitialDelay: new(50 * time.Millisecond), MaxDelay: new(time.Second)}})
		}()
	}
	close(start)
	for range publishers {
		require.NoError(t, <-results)
	}
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, second, publishers)
	for client, elapsed := range second {
		require.GreaterOrEqual(t, elapsed, time.Second, client)
	}
	for i := range publishers {
		require.Equal(t, "image: v2\n", gitRun(t, env, bare, "show", fmt.Sprintf("main:app-%d.yaml", i)))
	}
	require.Equal(t, fmt.Sprint(publishers+1), strings.TrimSpace(gitRun(t, env, bare, "rev-list", "--count", "main")))
}

func TestHTTPPushDoesNotRetryBeforeLongCooldown(t *testing.T) {
	requireGitBinary(t)
	env := gitTestEnv(t)
	dir := t.TempDir()
	gitRun(t, env, dir, "init")
	gitRun(t, env, dir, "commit", "--allow-empty", "-m", "seed")
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	err := New().Push(t.Context(), &atmosgit.PushOptions{RepoContext: atmosgit.RepoContext{Workdir: dir, Env: env, Remote: server.URL, Branch: "main"}, Retry: &schema.RetryConfig{MaxAttempts: new(4), MaxElapsedTime: new(10 * time.Second)}})
	require.ErrorIs(t, err, errUtils.ErrGitRateLimited)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, requests, "a cooldown exceeding the budget must not send another request")
}
