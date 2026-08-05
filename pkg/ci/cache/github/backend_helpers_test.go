package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/cache"
)

func TestParseNextPage(t *testing.T) {
	tests := []struct {
		name string
		link string
		want int
	}{
		{name: "empty", link: "", want: 0},
		{
			name: "single next",
			link: `<https://api.github.com/repos/o/r/actions/caches?per_page=100&page=2>; rel="next"`,
			want: 2,
		},
		{
			name: "next among multiple parts",
			link: `<https://api.github.com/x?page=1>; rel="prev", <https://api.github.com/x?per_page=100&page=3>; rel="next", <https://api.github.com/x?page=9>; rel="last"`,
			want: 3,
		},
		{
			name: "no next rel",
			link: `<https://api.github.com/x?page=9>; rel="last"`,
			want: 0,
		},
		{
			name: "next without page param",
			link: `<https://api.github.com/x?per_page=100>; rel="next"`,
			want: 0,
		},
		{
			name: "malformed brackets",
			link: `https://api.github.com/x?page=2; rel="next"`,
			want: 0,
		},
		{
			name: "non-numeric page",
			link: `<https://api.github.com/x?page=abc>; rel="next"`,
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseNextPage(tt.link))
		})
	}
}

func TestPageParam(t *testing.T) {
	assert.Equal(t, 5, pageParam("per_page=100&page=5"))
	assert.Equal(t, 7, pageParam("page=7"))
	assert.Equal(t, 0, pageParam("per_page=100"))
	assert.Equal(t, 0, pageParam("page=notanumber"))
	assert.Equal(t, 0, pageParam(""))
}

func TestSplitGitHubRepository(t *testing.T) {
	tests := []struct {
		in        string
		wantOwner string
		wantRepo  string
	}{
		{in: "owner/repo", wantOwner: "owner", wantRepo: "repo"},
		{in: "owner/repo/extra", wantOwner: "owner", wantRepo: "repo/extra"},
		{in: "bad", wantOwner: "", wantRepo: ""},
		{in: "", wantOwner: "", wantRepo: ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			owner, repo := splitGitHubRepository(tt.in)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}

func TestRepoFromEnv(t *testing.T) {
	t.Run("from options", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "envowner/envrepo")
		owner, repo := repoFromEnv(map[string]any{"owner": "o", "repo": "r"})
		assert.Equal(t, "o", owner)
		assert.Equal(t, "r", repo)
	})

	t.Run("from env fallback", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "envowner/envrepo")
		owner, repo := repoFromEnv(nil)
		assert.Equal(t, "envowner", owner)
		assert.Equal(t, "envrepo", repo)
	})

	t.Run("partial options filled from env", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "envowner/envrepo")
		owner, repo := repoFromEnv(map[string]any{"owner": "o"})
		assert.Equal(t, "o", owner)
		assert.Equal(t, "envrepo", repo)
	})

	t.Run("no env and no options", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "")
		owner, repo := repoFromEnv(nil)
		assert.Empty(t, owner)
		assert.Empty(t, repo)
	})
}

func TestResolveOwnerRepo(t *testing.T) {
	// Options take precedence and short-circuit the git fallback.
	t.Run("from options", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "")
		owner, repo := resolveOwnerRepo(map[string]any{"owner": "o", "repo": "r"})
		assert.Equal(t, "o", owner)
		assert.Equal(t, "r", repo)
	})

	// GITHUB_REPOSITORY is used when options are absent.
	t.Run("from GITHUB_REPOSITORY", func(t *testing.T) {
		t.Setenv("GITHUB_REPOSITORY", "envowner/envrepo")
		owner, repo := resolveOwnerRepo(nil)
		assert.Equal(t, "envowner", owner)
		assert.Equal(t, "envrepo", repo)
	})
}

func TestNewRESTClient(t *testing.T) {
	// Empty token returns a plain client.
	plain := newRESTClient("")
	require.NotNil(t, plain)
	assert.Equal(t, httpTimeout, plain.Timeout)

	// A token returns an oauth2-wrapped client (non-nil transport).
	withToken := newRESTClient("tok")
	require.NotNil(t, withToken)
	assert.Equal(t, httpTimeout, withToken.Timeout)
	assert.NotNil(t, withToken.Transport)
}

func TestNewBackend_Success(t *testing.T) {
	t.Setenv("ACTIONS_RUNTIME_TOKEN", "runtime-token")
	t.Setenv("ACTIONS_RESULTS_URL", "https://results.example.com/")
	t.Setenv("GITHUB_REPOSITORY", "octo/cat")

	b, err := NewBackend(cache.Options{})
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Equal(t, backendName, b.Name())

	gb, ok := b.(*Backend)
	require.True(t, ok)
	assert.Equal(t, "octo", gb.owner)
	assert.Equal(t, "cat", gb.repo)
	assert.NotEmpty(t, gb.version)
}

func TestBackend_Name(t *testing.T) {
	b := &Backend{}
	assert.Equal(t, "github/actions", b.Name())
}
