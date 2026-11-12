package downloader

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// The fake remote is never resolvable: a fetch of it can only succeed through an `insteadOf`
	// rewrite delivered in the fetch's own environment.
	fakeRemote  = "https://git.example.invalid/acme/shared.git"
	markerFile  = "owner.txt"
	fetchWindow = 2 * time.Minute
)

// gitTestRepo creates a local repository with one commit and returns its file:// URL.
func gitTestRepo(t *testing.T, owner string) string {
	t.Helper()
	if _, err := exec.LookPath(gitCommand); err != nil {
		t.Skip("git is not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(gitCommand, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init", "--quiet")
	require.NoError(t, os.WriteFile(filepath.Join(dir, markerFile), []byte(owner), 0o600))
	run("add", markerFile)
	run("-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "init")

	repoPath := filepath.ToSlash(dir)
	if !strings.HasPrefix(repoPath, "/") {
		repoPath = "/" + repoPath
	}
	return (&url.URL{Scheme: "file", Path: repoPath}).String()
}

// identityEnv is the environment an identity would hand to a fetch: a git `insteadOf` rewrite
// pointing fakeRemote at repoURL, in the "KEY=value" form git.ComposeEnvironment returns.
func identityEnv(repoURL string) []string {
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=url." + repoURL + ".insteadOf",
		"GIT_CONFIG_VALUE_0=" + fakeRemote,
	}
}

// isolateGitConfig keeps the developer's own git config out of the fetch.
func isolateGitConfig(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
}

func fetchGit(ctx context.Context, dest string) error {
	dl := NewGoGetterDownloader(&schema.AtmosConfiguration{}).(ContextFileDownloader)
	_, err := dl.FetchWithMetadataContext(ctx, "git::"+fakeRemote, dest, ClientModeDir, fetchWindow)
	return err
}

func TestCustomGitGetter_PerFetchEnvironmentReachesGitSubprocess(t *testing.T) {
	isolateGitConfig(t)
	repoURL := gitTestRepo(t, "identity-repo")

	t.Run("without a per-fetch environment the unresolvable remote fails", func(t *testing.T) {
		require.Error(t, fetchGit(context.Background(), filepath.Join(t.TempDir(), "dst")))
	})

	t.Run("with a per-fetch environment the git subprocess sees it", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "dst")
		ctx := WithEnvironment(context.Background(), identityEnv(repoURL))

		require.NoError(t, fetchGit(ctx, dest))

		got, err := os.ReadFile(filepath.Join(dest, markerFile))
		require.NoError(t, err)
		assert.Equal(t, "identity-repo", string(got))
	})

	t.Run("the process environment is never modified", func(t *testing.T) {
		_, hasCount := os.LookupEnv("GIT_CONFIG_COUNT")
		assert.False(t, hasCount, "GIT_CONFIG_COUNT must not leak into the process environment")
	})
}

// Two sources fetched concurrently, each with its own identity environment, must each see only
// their own: the same URL resolves to a different repository per context.
func TestCustomGitGetter_ConcurrentFetchesWithDifferentIdentitiesStayIsolated(t *testing.T) {
	isolateGitConfig(t)
	owners := []string{"alpha", "bravo"}
	repoURLs := make(map[string]string, len(owners))
	for _, owner := range owners {
		repoURLs[owner] = gitTestRepo(t, owner)
	}

	const fetchesPerIdentity = 4
	var wg sync.WaitGroup
	results := make(chan string, len(owners)*fetchesPerIdentity)
	errs := make(chan error, len(owners)*fetchesPerIdentity)
	for i := 0; i < fetchesPerIdentity; i++ {
		for _, owner := range owners {
			wg.Add(1)
			go func() {
				defer wg.Done()
				dest := filepath.Join(t.TempDir(), "dst")
				if err := fetchGit(WithEnvironment(context.Background(), identityEnv(repoURLs[owner])), dest); err != nil {
					errs <- fmt.Errorf("%s: %w", owner, err)
					return
				}
				got, err := os.ReadFile(filepath.Join(dest, markerFile))
				if err != nil {
					errs <- err
					return
				}
				results <- owner + "=" + string(got)
			}()
		}
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	count := 0
	for result := range results {
		owner, got, _ := strings.Cut(result, "=")
		assert.Equal(t, owner, got, "a fetch must see only its own identity's environment")
		count++
	}
	assert.Equal(t, len(owners)*fetchesPerIdentity, count)
}

// An identity's insteadOf rewrite lives only in the per-fetch environment. URL token injection
// must see it there, or it would put userinfo into the URL and shadow the rewrite.
func TestBrokerInsteadOfMatchesURLContext_ReadsPerFetchEnvironment(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "")
	parsed, err := url.Parse("https://github.com/acme/modules.git")
	require.NoError(t, err)
	rewrite := []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=url.https://x-access-token:token@github.com/acme/.insteadOf",
		"GIT_CONFIG_VALUE_0=https://github.com/acme/",
	}
	// The key above is an inline `url.<base>.insteadOf` rewrite whose value is the match target.

	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{name: "no per-fetch environment and no process rewrite", ctx: context.Background(), want: false},
		{name: "per-fetch rewrite for the same owner", ctx: WithEnvironment(context.Background(), rewrite), want: true},
		{name: "per-fetch rewrite for another owner", ctx: WithEnvironment(context.Background(), []string{
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=url.https://x-access-token:token@github.com/other/.insteadOf",
			"GIT_CONFIG_VALUE_0=https://github.com/other/",
		}), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, brokerInsteadOfMatchesURLContext(tt.ctx, parsed, "github.com"))
		})
	}
}
