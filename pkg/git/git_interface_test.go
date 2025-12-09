package git

import (
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewDefaultGitRepo tests the NewDefaultGitRepo constructor.
func TestNewDefaultGitRepo(t *testing.T) {
	gitRepo := NewDefaultGitRepo()
	assert.NotNil(t, gitRepo)
	assert.IsType(t, &DefaultGitRepo{}, gitRepo)
}

// TestDefaultGitRepo_GetLocalRepoInfo tests GetLocalRepoInfo method.
func TestDefaultGitRepo_GetLocalRepoInfo(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		expectError bool
		validate    func(t *testing.T, info *RepoInfo)
	}{
		{
			name: "successful get local repo info with remote",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				// Initialize git repo
				t.Chdir(tempDir)

				repo, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)

				// Add remote
				_, err = repo.CreateRemote(&config.RemoteConfig{
					Name: "origin",
					URLs: []string{"https://github.com/cloudposse/atmos.git"},
				})
				require.NoError(t, err)

				// Create initial commit
				createInitialCommit(t, repo, tempDir)

				return tempDir
			},
			expectError: false,
			validate: func(t *testing.T, info *RepoInfo) {
				assert.NotNil(t, info)
				assert.NotEmpty(t, info.LocalRepoPath)
				assert.NotNil(t, info.LocalWorktree)
				assert.Equal(t, "https://github.com/cloudposse/atmos.git", info.RepoUrl)
				assert.Equal(t, "cloudposse", info.RepoOwner)
				assert.Equal(t, "atmos", info.RepoName)
				assert.Equal(t, "github.com", info.RepoHost)
			},
		},
		{
			name: "error when not in git repository",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				t.Chdir(tempDir)

				return tempDir
			},
			expectError: true,
			validate: func(t *testing.T, info *RepoInfo) {
				assert.Nil(t, info)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.setup(t)

			gitRepo := NewDefaultGitRepo()
			info, err := gitRepo.GetLocalRepoInfo()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.validate != nil {
				tt.validate(t, info)
			}
		})
	}
}

// TestDefaultGitRepo_GetRepoInfo tests GetRepoInfo method with provided repo.
func TestDefaultGitRepo_GetRepoInfo(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) *git.Repository
		expectError bool
		validate    func(t *testing.T, info RepoInfo, err error)
	}{
		{
			name: "successful get repo info with HTTPS remote",
			setup: func(t *testing.T) *git.Repository {
				return createRepoWithRemote(t, "https://github.com/cloudposse/atmos.git")
			},
			expectError: false,
			validate: func(t *testing.T, info RepoInfo, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "https://github.com/cloudposse/atmos.git", info.RepoUrl)
				assert.Equal(t, "cloudposse", info.RepoOwner)
				assert.Equal(t, "atmos", info.RepoName)
				assert.Equal(t, "github.com", info.RepoHost)
			},
		},
		{
			name: "successful get repo info with SSH remote",
			setup: func(t *testing.T) *git.Repository {
				return createRepoWithRemote(t, "git@github.com:cloudposse/atmos.git")
			},
			expectError: false,
			validate: func(t *testing.T, info RepoInfo, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "git@github.com:cloudposse/atmos.git", info.RepoUrl)
				assert.Equal(t, "cloudposse", info.RepoOwner)
				assert.Equal(t, "atmos", info.RepoName)
				assert.Equal(t, "github.com", info.RepoHost)
			},
		},
		{
			name: "repo without remote returns empty info",
			setup: func(t *testing.T) *git.Repository {
				tempDir := t.TempDir()
				repo, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)
				createInitialCommit(t, repo, tempDir)
				return repo
			},
			expectError: false,
			validate: func(t *testing.T, info RepoInfo, err error) {
				assert.NoError(t, err)
				assert.Empty(t, info.RepoUrl)
				assert.Empty(t, info.RepoOwner)
				assert.Empty(t, info.RepoName)
			},
		},
		{
			name: "repo with invalid URL returns error",
			setup: func(t *testing.T) *git.Repository {
				return createRepoWithRemote(t, "invalid-url-format")
			},
			expectError: true,
			validate: func(t *testing.T, info RepoInfo, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "GetRepoInfo failed for repo")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setup(t)
			gitRepo := NewDefaultGitRepo()

			info, err := gitRepo.GetRepoInfo(repo)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.validate != nil {
				tt.validate(t, info, err)
			}
		})
	}
}

// TestDefaultGitRepo_GetCurrentCommitSHA tests GetCurrentCommitSHA method.
func TestDefaultGitRepo_GetCurrentCommitSHA(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T)
		expectError bool
		validate    func(t *testing.T, sha string, err error)
	}{
		{
			name: "successful get commit SHA",
			setup: func(t *testing.T) {
				tempDir := t.TempDir()
				t.Chdir(tempDir)

				repo, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)

				createInitialCommit(t, repo, tempDir)
			},
			expectError: false,
			validate: func(t *testing.T, sha string, err error) {
				assert.NoError(t, err)
				assert.NotEmpty(t, sha)
				assert.Len(t, sha, 40) // SHA is 40 characters
			},
		},
		{
			name: "error when not in git repository",
			setup: func(t *testing.T) {
				tempDir := t.TempDir()
				t.Chdir(tempDir)
			},
			expectError: true,
			validate: func(t *testing.T, sha string, err error) {
				assert.Error(t, err)
				assert.Empty(t, sha)
			},
		},
		{
			name: "error when no commits exist",
			setup: func(t *testing.T) {
				tempDir := t.TempDir()
				t.Chdir(tempDir)

				_, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)
				// No commits created
			},
			expectError: true,
			validate: func(t *testing.T, sha string, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to get HEAD reference")
				assert.Empty(t, sha)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)

			gitRepo := NewDefaultGitRepo()
			sha, err := gitRepo.GetCurrentCommitSHA()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.validate != nil {
				tt.validate(t, sha, err)
			}
		})
	}
}

// TestOpenWorktreeAwareRepo tests OpenWorktreeAwareRepo function.
func TestOpenWorktreeAwareRepo(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		expectError bool
		validate    func(t *testing.T, repo *git.Repository, err error)
	}{
		{
			name: "open regular repository",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				_, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)
				return tempDir
			},
			expectError: false,
			validate: func(t *testing.T, repo *git.Repository, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, repo)
			},
		},
		{
			name: "error opening non-existent path",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			expectError: true,
			validate: func(t *testing.T, repo *git.Repository, err error) {
				assert.Error(t, err)
				assert.Nil(t, repo)
			},
		},
		{
			name: "error opening non-repository path",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				// Create a regular directory without git
				return tempDir
			},
			expectError: true,
			validate: func(t *testing.T, repo *git.Repository, err error) {
				assert.Error(t, err)
				assert.Nil(t, repo)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)

			repo, err := OpenWorktreeAwareRepo(path)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.validate != nil {
				tt.validate(t, repo, err)
			}
		})
	}
}

// TestGitRepoInterface_Implementation verifies DefaultGitRepo implements GitRepoInterface.
func TestGitRepoInterface_Implementation(t *testing.T) {
	var _ GitRepoInterface = (*DefaultGitRepo)(nil)
	// If this compiles, the interface is properly implemented
}
