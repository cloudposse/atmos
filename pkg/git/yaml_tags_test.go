package git

import (
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chdirRepoWithRemote initializes a Git repository with the given remote URL in a
// temporary directory and changes the working directory into it. The repository-metadata
// tags resolve from the current working directory, so tests must run inside the repo.
func chdirRepoWithRemote(t *testing.T, remoteURL string) {
	t.Helper()

	tempDir := t.TempDir()
	repo, err := git.PlainInit(tempDir, false)
	require.NoError(t, err)

	if remoteURL != "" {
		_, err = repo.CreateRemote(&config.RemoteConfig{
			Name: "origin",
			URLs: []string{remoteURL},
		})
		require.NoError(t, err)
	}

	t.Chdir(tempDir)
}

func TestProcessRepositoryMetadataTags(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		fn        func(string) (string, error)
		input     string
		expected  string
	}{
		{
			name:      "repository slug from HTTPS remote",
			remoteURL: "https://github.com/cloudposse/atmos.git",
			fn:        ProcessTagRepository,
			input:     YAMLFuncRepository,
			expected:  "cloudposse/atmos",
		},
		{
			name:      "repository slug from SSH remote",
			remoteURL: "git@github.com:cloudposse/atmos.git",
			fn:        ProcessTagRepository,
			input:     YAMLFuncRepository,
			expected:  "cloudposse/atmos",
		},
		{
			name:      "owner from HTTPS remote",
			remoteURL: "https://github.com/cloudposse/atmos.git",
			fn:        ProcessTagOwner,
			input:     YAMLFuncOwner,
			expected:  "cloudposse",
		},
		{
			name:      "name from HTTPS remote",
			remoteURL: "https://github.com/cloudposse/atmos.git",
			fn:        ProcessTagName,
			input:     YAMLFuncName,
			expected:  "atmos",
		},
		{
			name:      "host from HTTPS remote",
			remoteURL: "https://github.com/cloudposse/atmos.git",
			fn:        ProcessTagHost,
			input:     YAMLFuncHost,
			expected:  "github.com",
		},
		{
			name:      "owner from GitLab SSH remote",
			remoteURL: "git@gitlab.com:my-group/my-project.git",
			fn:        ProcessTagOwner,
			input:     YAMLFuncOwner,
			expected:  "my-group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chdirRepoWithRemote(t, tt.remoteURL)

			result, err := tt.fn(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessTagURL(t *testing.T) {
	const remoteURL = "https://github.com/cloudposse/atmos.git"
	chdirRepoWithRemote(t, remoteURL)

	result, err := ProcessTagURL(YAMLFuncURL)
	require.NoError(t, err)
	assert.Equal(t, remoteURL, result)
}

func TestProcessTagRepository_DefaultOnNoRemote(t *testing.T) {
	// A repository with no remote has empty owner/name, so the default value is used.
	chdirRepoWithRemote(t, "")

	result, err := ProcessTagRepository(YAMLFuncRepository + " owner/fallback")
	require.NoError(t, err)
	assert.Equal(t, "owner/fallback", result)
}

func TestProcessTagRepository_ErrorOnNoRemoteWithoutDefault(t *testing.T) {
	// A repository with no remote and no default value returns an error rather than "/".
	chdirRepoWithRemote(t, "")

	result, err := ProcessTagRepository(YAMLFuncRepository)
	require.Error(t, err)
	assert.Empty(t, result)
}

func TestProcessTagOwner_DefaultOutsideRepo(t *testing.T) {
	// Outside any Git repository, GetLocalRepoInfo fails, so the default value is returned.
	t.Chdir(t.TempDir())

	result, err := ProcessTagOwner(YAMLFuncOwner + " default-owner")
	require.NoError(t, err)
	assert.Equal(t, "default-owner", result)
}
