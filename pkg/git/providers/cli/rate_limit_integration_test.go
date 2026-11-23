package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Measure a real Retry-After interval through the public Push path without
// coupling the timing assertion to twelve competing Git subprocesses.
func TestHTTPRetryAfterWallClockCooldown(t *testing.T) {
	testHTTPPublishers(t, 1, false)
}

// Exercise twelve independent publishers against the same branch over smart HTTP.
// Each receives a 429 before contending for the ref; no lock serializes publication.
// Only retry scheduling is virtual: Git, HTTP, recovery, and the execution deadline
// remain real. TestHTTPRetryAfterWallClockCooldown separately checks elapsed time.
func TestHTTPConcurrentPublishersRecoverFromThrottling(t *testing.T) {
	testHTTPPublishers(t, 12, true)
}

func testHTTPPublishers(t *testing.T, publishers int, virtualScheduling bool) {
	t.Helper()
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
	workdirs := make([]string, publishers)
	for i := range workdirs {
		workdirs[i] = t.TempDir()
		gitRun(t, env, "", "clone", seed, workdirs[i])
		writeFile(t, filepath.Join(workdirs[i], fmt.Sprintf("app-%d.yaml", i)), "image: v2\n")
		gitRun(t, env, workdirs[i], "add", ".")
		gitRun(t, env, workdirs[i], "commit", "-m", fmt.Sprintf("publish app %d", i))
	}
	start := make(chan struct{})
	type result struct {
		publisher int
		err       error
		waits     []time.Duration
	}
	results := make(chan result, publishers)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for i, dir := range workdirs {
		go func() {
			<-start
			opts := &atmosgit.PushOptions{RepoContext: atmosgit.RepoContext{Workdir: dir, Env: env, Remote: fmt.Sprintf("%s/client-%d/repo.git", server.URL, i), Branch: "main"}, Retry: &schema.RetryConfig{MaxAttempts: new(16), MaxElapsedTime: new(30 * time.Second), RandomJitter: new(0.5), BackoffStrategy: schema.BackoffExponential, InitialDelay: new(50 * time.Millisecond), MaxDelay: new(time.Second)}}
			outcome := result{publisher: i}
			if virtualScheduling {
				clock := &advancingPushClock{}
				outcome.err = New().pushWithRetry(ctx, opts, opts.Retry, retry.WithClock(clock))
				outcome.waits = clock.waits
			} else {
				outcome.err = New().Push(ctx, opts)
			}
			results <- outcome
		}()
	}
	close(start)
	for range publishers {
		outcome := <-results
		assert.NoError(t, outcome.err, "publisher %d; scheduled waits %v", outcome.publisher, outcome.waits)
		if virtualScheduling && assert.NotEmpty(t, outcome.waits, "publisher %d", outcome.publisher) {
			assert.GreaterOrEqual(t, outcome.waits[0], time.Second, "publisher %d", outcome.publisher)
			assert.LessOrEqual(t, outcome.waits[0], 1500*time.Millisecond, "publisher %d", outcome.publisher)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, second, publishers)
	if !virtualScheduling {
		for client, elapsed := range second {
			require.GreaterOrEqual(t, elapsed, time.Second, client)
		}
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
