package marketplace

import (
	"os"
	"path/filepath"
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

func TestParseSource_LocalPath_Exists(t *testing.T) {
	dir := t.TempDir()

	info, err := ParseSource(dir)
	require.NoError(t, err)
	assert.Equal(t, "local", info.Type)

	abs, err := filepath.Abs(dir)
	require.NoError(t, err)
	assert.Equal(t, abs, info.URL)
	assert.Equal(t, filepath.Base(abs), info.Name)
	assert.Equal(t, "local/"+filepath.Base(abs), info.FullPath)
}

func TestParseSource_LocalPath_RelativeResolvesAgainstCWD(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "my-skill")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	t.Chdir(dir)

	info, err := ParseSource("my-skill")
	require.NoError(t, err)
	assert.Equal(t, "local", info.Type)
	assert.Equal(t, "my-skill", info.Name)
}

func TestParseSource_LocalPath_DoesNotExist(t *testing.T) {
	// A plain path that doesn't exist on disk is not local -- it must fall
	// through to the "unsupported source format" error rather than being
	// silently accepted (that would make every typo look like a local install).
	missing := filepath.Join(t.TempDir(), "does-not-exist-xyz")

	_, err := ParseSource(missing)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported source format")
}

func TestParseSource_FileURL(t *testing.T) {
	dir := t.TempDir()

	info, err := ParseSource("file://" + dir)
	require.NoError(t, err)
	assert.Equal(t, "local", info.Type)

	abs, err := filepath.Abs(dir)
	require.NoError(t, err)
	assert.Equal(t, abs, info.URL)
	assert.Equal(t, filepath.Base(abs), info.Name)
}

func TestParseSource_FileURL_NotCheckedForExistence(t *testing.T) {
	// The file:// URL scheme is trusted without an existence check, mirroring
	// go-getter's behavior; a missing path only fails later, at copy time.
	missing := filepath.Join(t.TempDir(), "definitely-does-not-exist")

	info, err := ParseSource("file://" + missing)
	require.NoError(t, err)
	assert.Equal(t, "local", info.Type)
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
