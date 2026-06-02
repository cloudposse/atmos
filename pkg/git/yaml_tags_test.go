package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests"
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

// TestRepositoryMetadataFromLinkedWorktree is a regression guard for worktree
// support: in a linked worktree, `.git` is a file pointing at the per-worktree
// gitdir, and the `origin` remote lives only in the shared common config. The
// repository-metadata tags must still resolve, which depends on GetLocalRepo
// opening with EnableDotGitCommonDir. If that flag is dropped, the remote is not
// visible from the worktree and these assertions fail.
func TestRepositoryMetadataFromLinkedWorktree(t *testing.T) {
	tests.RequireGitCommitConfig(t)

	const remoteURL = "https://github.com/cloudposse/atmos.git"

	// Create the main repository with a remote and an initial commit.
	repoDir := t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	require.NoError(t, err)

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	require.NoError(t, err)

	worktree, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("test\n"), 0o644))
	_, err = worktree.Add("README.md")
	require.NoError(t, err)
	commitHash, err := worktree.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Atmos Test", Email: "test@example.com"},
	})
	require.NoError(t, err)

	// Create a linked worktree (its `.git` is a file, not a directory).
	worktreePath, err := CreateWorktree(repoDir, commitHash.String())
	require.NoError(t, err)
	t.Cleanup(func() {
		RemoveWorktree(repoDir, worktreePath)
		os.RemoveAll(GetWorktreeParentDir(worktreePath))
	})

	// Confirm it really is a linked worktree: `.git` must be a file.
	gitPath := filepath.Join(worktreePath, ".git")
	info, err := os.Stat(gitPath)
	require.NoError(t, err)
	require.False(t, info.IsDir(), ".git should be a file in a linked worktree")

	// Resolve the metadata tags from inside the worktree.
	t.Chdir(worktreePath)

	repository, err := ProcessTagRepository(YAMLFuncRepository)
	require.NoError(t, err)
	assert.Equal(t, "cloudposse/atmos", repository)

	owner, err := ProcessTagOwner(YAMLFuncOwner)
	require.NoError(t, err)
	assert.Equal(t, "cloudposse", owner)

	name, err := ProcessTagName(YAMLFuncName)
	require.NoError(t, err)
	assert.Equal(t, "atmos", name)

	host, err := ProcessTagHost(YAMLFuncHost)
	require.NoError(t, err)
	assert.Equal(t, "github.com", host)

	url, err := ProcessTagURL(YAMLFuncURL)
	require.NoError(t, err)
	assert.Equal(t, remoteURL, url)

	// !git.root must resolve to the worktree path, not the main repo.
	root, err := ProcessTagRoot(YAMLFuncRoot)
	require.NoError(t, err)
	expectedRoot, err := filepath.EvalSymlinks(worktreePath)
	require.NoError(t, err)
	actualRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, actualRoot)
}

func TestProcessTagOwner_DefaultOutsideRepo(t *testing.T) {
	// Outside any Git repository, GetLocalRepoInfo fails, so the default value is returned.
	t.Chdir(t.TempDir())

	result, err := ProcessTagOwner(YAMLFuncOwner + " default-owner")
	require.NoError(t, err)
	assert.Equal(t, "default-owner", result)
}
