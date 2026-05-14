package marketplace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSource_BareOwnerRepo(t *testing.T) {
	info, err := ParseSource("cloudposse/atmos")
	require.NoError(t, err)
	assert.Equal(t, "github", info.Type)
	assert.Equal(t, "cloudposse", info.Owner)
	assert.Equal(t, "atmos", info.Repo)
	assert.Equal(t, "", info.Ref)
	assert.Equal(t, "https://github.com/cloudposse/atmos.git", info.URL)
	assert.Equal(t, "github.com/cloudposse/atmos", info.FullPath)
	assert.Equal(t, "atmos", info.Name)
}

func TestParseSource_BareOwnerRepoWithRef(t *testing.T) {
	info, err := ParseSource("cloudposse/atmos@v1.200.0")
	require.NoError(t, err)
	assert.Equal(t, "cloudposse", info.Owner)
	assert.Equal(t, "atmos", info.Repo)
	assert.Equal(t, "v1.200.0", info.Ref)
	assert.Equal(t, "github.com/cloudposse/atmos", info.FullPath)
}

func TestParseSource_GitHubShorthand(t *testing.T) {
	info, err := ParseSource("github.com/cloudposse/atmos")
	require.NoError(t, err)
	assert.Equal(t, "github", info.Type)
	assert.Equal(t, "cloudposse", info.Owner)
	assert.Equal(t, "atmos", info.Repo)
	assert.Equal(t, "", info.Ref)
}

func TestParseSource_GitHubShorthandWithRef(t *testing.T) {
	info, err := ParseSource("github.com/cloudposse/atmos@main")
	require.NoError(t, err)
	assert.Equal(t, "cloudposse", info.Owner)
	assert.Equal(t, "atmos", info.Repo)
	assert.Equal(t, "main", info.Ref)
}

func TestParseSource_HTTPS(t *testing.T) {
	info, err := ParseSource("https://github.com/cloudposse/atmos.git")
	require.NoError(t, err)
	assert.Equal(t, "github", info.Type)
	assert.Equal(t, "cloudposse", info.Owner)
	assert.Equal(t, "atmos", info.Repo)
}

func TestParseSource_SSH(t *testing.T) {
	info, err := ParseSource("git@github.com:cloudposse/atmos.git")
	require.NoError(t, err)
	assert.Equal(t, "github", info.Type)
	assert.Equal(t, "cloudposse", info.Owner)
	assert.Equal(t, "atmos", info.Repo)
}

func TestParseSource_InvalidFormats(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{"empty string", ""},
		{"single word", "atmos"},
		{"triple path", "a/b/c"},
		{"bare slash", "/repo"},
		{"trailing slash", "owner/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSource(tt.source)
			assert.Error(t, err)
		})
	}
}

func TestParseSource_GitHubShorthand_InvalidFormat(t *testing.T) {
	// Too many path segments after github.com/.
	_, err := ParseSource("github.com/user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")

	_, err = ParseSource("github.com/a/b/c")
	assert.Error(t, err)
}

func TestParseSource_HTTPS_InvalidFormat(t *testing.T) {
	_, err := ParseSource("https://github.com/useronly")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")

	_, err = ParseSource("https://github.com/a/b/c")
	assert.Error(t, err)
}

func TestParseSource_SSH_InvalidFormat(t *testing.T) {
	_, err := ParseSource("git@github.com:useronly")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")

	_, err = ParseSource("git@github.com:a/b/c")
	assert.Error(t, err)
}

func TestParseSource_SSHWithRef(t *testing.T) {
	// SSH format doesn't support @ ref (it starts with git@).
	info, err := ParseSource("git@github.com:user/repo.git")
	require.NoError(t, err)
	assert.Equal(t, "", info.Ref)
	assert.Equal(t, "user", info.Owner)
	assert.Equal(t, "repo", info.Repo)
}

func TestParseSource_GitSuffix(t *testing.T) {
	// .git suffix should be stripped from repo name.
	info, err := ParseSource("github.com/user/repo.git")
	require.NoError(t, err)
	assert.Equal(t, "repo", info.Repo)
}

func TestParseSource_HTTPSWithoutGitSuffix(t *testing.T) {
	info, err := ParseSource("https://github.com/user/repo")
	require.NoError(t, err)
	assert.Equal(t, "user", info.Owner)
	assert.Equal(t, "repo", info.Repo)
}

func TestParseSource_UnsupportedHost(t *testing.T) {
	_, err := ParseSource("gitlab.com/user/repo")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported source format")
}

func TestIsOwnerRepoShorthand(t *testing.T) {
	assert.True(t, isOwnerRepoShorthand("cloudposse/atmos"))
	assert.True(t, isOwnerRepoShorthand("user/repo"))

	assert.False(t, isOwnerRepoShorthand("github.com/user/repo"))
	assert.False(t, isOwnerRepoShorthand("https://github.com/user/repo"))
	assert.False(t, isOwnerRepoShorthand("git@github.com:user/repo"))
	assert.False(t, isOwnerRepoShorthand("atmos"))
	assert.False(t, isOwnerRepoShorthand("a/b/c"))
	assert.False(t, isOwnerRepoShorthand("/repo"))
	assert.False(t, isOwnerRepoShorthand("owner/"))
	assert.False(t, isOwnerRepoShorthand(""))
}
