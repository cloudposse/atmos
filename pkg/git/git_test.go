package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/tests"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create an initial commit in a repository.
func createInitialCommit(t *testing.T, repo *git.Repository, tempDir string) {
	t.Helper()

	// Check if git is configured for commits
	tests.RequireGitCommitConfig(t)

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	_, err = worktree.Add("test.txt")
	require.NoError(t, err)

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)
}

// Helper function to create a repository with a remote.
func createRepoWithRemote(t *testing.T, remoteURL string) *git.Repository {
	t.Helper()

	tempDir := t.TempDir()
	repo, err := git.PlainInit(tempDir, false)
	require.NoError(t, err)

	// Add a remote.
	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	require.NoError(t, err)

	// Create an initial commit.
	createInitialCommit(t, repo, tempDir)

	return repo
}

// Helper function to validate repo info for GitHub remotes.
func validateGitHubRepoInfo(t *testing.T, info *RepoInfo, expectedURL string) {
	t.Helper()

	assert.NotEmpty(t, info.LocalRepoPath)
	assert.NotNil(t, info.LocalWorktree)
	assert.NotEmpty(t, info.LocalWorktreePath)
	assert.Equal(t, expectedURL, info.RepoUrl)
	assert.Equal(t, "cloudposse", info.RepoOwner)
	assert.Equal(t, "atmos", info.RepoName)
	assert.Equal(t, "github.com", info.RepoHost)
}

func TestGetLocalRepo(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		cleanup     func(t *testing.T, path string)
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful git repository detection",
			setup: func(t *testing.T) string {
				// Create a temporary directory with a git repository.
				tempDir := t.TempDir()
				t.Chdir(tempDir)

				_, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)

				return tempDir
			},
			cleanup: func(t *testing.T, path string) {
				// Cleanup is handled by t.TempDir().
			},
			expectError: false,
		},
		{
			name: "no git repository found",
			setup: func(t *testing.T) string {
				// Create a temporary directory without a git repository.
				tempDir := t.TempDir()
				t.Chdir(tempDir)

				return tempDir
			},
			cleanup: func(t *testing.T, path string) {
				// Cleanup is handled by t.TempDir().
			},
			expectError: true,
			errorMsg:    "repository does not exist",
		},
		{
			name: "nested directory within git repository",
			setup: func(t *testing.T) string {
				// Create a git repository with nested directories.
				tempDir := t.TempDir()

				_, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)

				nestedDir := filepath.Join(tempDir, "nested", "deep")
				err = os.MkdirAll(nestedDir, 0o755)
				require.NoError(t, err)

				t.Chdir(nestedDir)

				return tempDir
			},
			cleanup: func(t *testing.T, path string) {
				// Cleanup is handled by t.TempDir().
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			defer tt.cleanup(t, path)

			repo, err := GetLocalRepo()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, repo)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, repo)
			}
		})
	}
}

func TestGetRepoConfig(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) *git.Repository
		expectError bool
		validate    func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "get config from valid repository",
			setup: func(t *testing.T) *git.Repository {
				tempDir := t.TempDir()
				repo, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)
				return repo
			},
			expectError: false,
			validate: func(t *testing.T, cfg *config.Config) {
				assert.NotNil(t, cfg)
				assert.NotNil(t, cfg.Raw)
			},
		},
		{
			name: "remove untrackedCache option if present",
			setup: func(t *testing.T) *git.Repository {
				tempDir := t.TempDir()
				repo, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)

				// Add untrackedCache option to the config.
				cfg, err := repo.Config()
				require.NoError(t, err)

				core := cfg.Raw.Section("core")
				core.SetOption("untrackedCache", "true")

				err = repo.Storer.SetConfig(cfg)
				require.NoError(t, err)

				return repo
			},
			expectError: false,
			validate: func(t *testing.T, cfg *config.Config) {
				assert.NotNil(t, cfg)
				core := cfg.Raw.Section("core")
				assert.Empty(t, core.Option("untrackedCache"))
			},
		},
		{
			name: "config without untrackedCache option",
			setup: func(t *testing.T) *git.Repository {
				tempDir := t.TempDir()
				repo, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)

				// Ensure untrackedCache is not set.
				cfg, err := repo.Config()
				require.NoError(t, err)

				core := cfg.Raw.Section("core")
				assert.Empty(t, core.Option("untrackedCache"))

				return repo
			},
			expectError: false,
			validate: func(t *testing.T, cfg *config.Config) {
				assert.NotNil(t, cfg)
				core := cfg.Raw.Section("core")
				assert.Empty(t, core.Option("untrackedCache"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setup(t)

			cfg, err := GetRepoConfig(repo)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, cfg)
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, cfg)
				}
			}
		})
	}
}

func TestGetRepoInfo(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) *git.Repository
		expectError bool
		validate    func(t *testing.T, info RepoInfo)
	}{
		{
			name: "repository with HTTPS remote",
			setup: func(t *testing.T) *git.Repository {
				return createRepoWithRemote(t, "https://github.com/cloudposse/atmos.git")
			},
			expectError: false,
			validate: func(t *testing.T, info RepoInfo) {
				validateGitHubRepoInfo(t, &info, "https://github.com/cloudposse/atmos.git")
			},
		},
		{
			name: "repository with SSH remote",
			setup: func(t *testing.T) *git.Repository {
				return createRepoWithRemote(t, "git@github.com:cloudposse/atmos.git")
			},
			expectError: false,
			validate: func(t *testing.T, info RepoInfo) {
				validateGitHubRepoInfo(t, &info, "git@github.com:cloudposse/atmos.git")
			},
		},
		{
			name: "repository without remote",
			setup: func(t *testing.T) *git.Repository {
				tempDir := t.TempDir()
				repo, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)

				// Create an initial commit.
				createInitialCommit(t, repo, tempDir)

				return repo
			},
			expectError: false,
			validate: func(t *testing.T, info RepoInfo) {
				// Should return empty RepoInfo when no remotes exist.
				assert.Empty(t, info.RepoUrl)
				assert.Empty(t, info.RepoOwner)
				assert.Empty(t, info.RepoName)
				assert.Empty(t, info.RepoHost)
			},
		},
		{
			name: "repository with multiple remotes",
			setup: func(t *testing.T) *git.Repository {
				tempDir := t.TempDir()
				repo, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)

				// Add multiple remotes.
				_, err = repo.CreateRemote(&config.RemoteConfig{
					Name: "origin",
					URLs: []string{"https://github.com/cloudposse/atmos.git"},
				})
				require.NoError(t, err)

				_, err = repo.CreateRemote(&config.RemoteConfig{
					Name: "upstream",
					URLs: []string{"https://github.com/other/atmos.git"},
				})
				require.NoError(t, err)

				// Create an initial commit.
				createInitialCommit(t, repo, tempDir)

				return repo
			},
			expectError: false,
			validate: func(t *testing.T, info RepoInfo) {
				assert.NotEmpty(t, info.LocalRepoPath)
				assert.NotNil(t, info.LocalWorktree)
				assert.NotEmpty(t, info.LocalWorktreePath)
				// Should use the first remote found.
				assert.NotEmpty(t, info.RepoUrl)
				assert.NotEmpty(t, info.RepoOwner)
				assert.NotEmpty(t, info.RepoName)
				assert.NotEmpty(t, info.RepoHost)
			},
		},
		{
			name: "repository with remote but empty URL string",
			setup: func(t *testing.T) *git.Repository {
				tempDir := t.TempDir()
				repo, err := git.PlainInit(tempDir, false)
				require.NoError(t, err)

				// Add a remote with an empty URL string.
				_, err = repo.CreateRemote(&config.RemoteConfig{
					Name: "origin",
					URLs: []string{""},
				})
				require.NoError(t, err)

				// Create an initial commit.
				createInitialCommit(t, repo, tempDir)

				return repo
			},
			expectError: false,
			validate: func(t *testing.T, info RepoInfo) {
				// Should return empty RepoInfo when remote URL is empty string.
				assert.Empty(t, info.RepoUrl)
				assert.Empty(t, info.RepoOwner)
				assert.Empty(t, info.RepoName)
				assert.Empty(t, info.RepoHost)
			},
		},
		{
			name: "repository with invalid remote URL",
			setup: func(t *testing.T) *git.Repository {
				return createRepoWithRemote(t, "not-a-valid-url")
			},
			expectError: true,
			validate: func(t *testing.T, info RepoInfo) {
				// Should return error due to invalid URL format.
				assert.Empty(t, info)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setup(t)

			info, err := GetRepoInfo(repo)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, info)
				}
			}
		})
	}
}

// TestRepoInfoStruct tests the RepoInfo struct fields are properly defined.
func TestRepoInfoStruct(t *testing.T) {
	info := RepoInfo{
		LocalRepoPath:     "/path/to/repo",
		LocalWorktree:     nil,
		LocalWorktreePath: "/path/to/worktree",
		RepoUrl:           "https://github.com/example/repo.git",
		RepoOwner:         "example",
		RepoName:          "repo",
		RepoHost:          "github.com",
	}

	assert.Equal(t, "/path/to/repo", info.LocalRepoPath)
	assert.Nil(t, info.LocalWorktree)
	assert.Equal(t, "/path/to/worktree", info.LocalWorktreePath)
	assert.Equal(t, "https://github.com/example/repo.git", info.RepoUrl)
	assert.Equal(t, "example", info.RepoOwner)
	assert.Equal(t, "repo", info.RepoName)
	assert.Equal(t, "github.com", info.RepoHost)
}

// TestIntegration tests the functions working together.
func TestIntegration(t *testing.T) {
	// Create a temporary directory with a git repository.
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	// Initialize a git repository.
	_, err := git.PlainInit(tempDir, false)
	require.NoError(t, err)

	// Test GetLocalRepo.
	repo, err := GetLocalRepo()
	assert.NoError(t, err)
	assert.NotNil(t, repo)

	// Add a remote.
	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{"https://github.com/cloudposse/atmos.git"},
	})
	require.NoError(t, err)

	// Create an initial commit.
	createInitialCommit(t, repo, tempDir)

	// Test GetRepoConfig.
	cfg, err := GetRepoConfig(repo)
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Test GetRepoInfo.
	info, err := GetRepoInfo(repo)
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/cloudposse/atmos.git", info.RepoUrl)
	assert.Equal(t, "cloudposse", info.RepoOwner)
	assert.Equal(t, "atmos", info.RepoName)
	assert.Equal(t, "github.com", info.RepoHost)
	assert.NotEmpty(t, info.LocalRepoPath)
	assert.NotNil(t, info.LocalWorktree)
}
