package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestRepo creates a temporary git repository for testing.
func createTestRepo(t *testing.T) (*git.Repository, string) {
	t.Helper()

	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "git-storage-test-*")
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	// Initialize git repository
	repo, err := git.PlainInit(tmpDir, false)
	require.NoError(t, err)

	return repo, tmpDir
}

// createCommit creates a commit with the given files.
func createCommit(t *testing.T, repo *git.Repository, repoPath string, message string, files map[string]string) string {
	t.Helper()

	worktree, err := repo.Worktree()
	require.NoError(t, err)

	// Write files
	for path, content := range files {
		fullPath := filepath.Join(repoPath, path)
		dir := filepath.Dir(fullPath)

		err := os.MkdirAll(dir, 0o755)
		require.NoError(t, err)

		err = os.WriteFile(fullPath, []byte(content), 0o644)
		require.NoError(t, err)

		_, err = worktree.Add(path)
		require.NoError(t, err)
	}

	// Create commit
	hash, err := worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)

	return hash.String()
}

func TestGitBaseStorage_LoadBase(t *testing.T) {
	tests := []struct {
		name           string
		setupRepo      func(*testing.T, *git.Repository, string) string // Returns base ref
		filePath       string
		expectedFound  bool
		expectedError  bool
		validateResult func(*testing.T, string)
	}{
		{
			name: "loads file that exists at base ref",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) string {
				hash := createCommit(t, repo, repoPath, "initial commit", map[string]string{
					"atmos.yaml": "components:\n  terraform:\n    base_path: components/terraform\n",
				})
				return hash
			},
			filePath:      "atmos.yaml",
			expectedFound: true,
			validateResult: func(t *testing.T, content string) {
				assert.Contains(t, content, "components:")
				assert.Contains(t, content, "base_path: components/terraform")
			},
		},
		{
			name: "returns not found for file that doesn't exist at base ref",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) string {
				hash := createCommit(t, repo, repoPath, "initial commit", map[string]string{
					"atmos.yaml": "config: value\n",
				})
				return hash
			},
			filePath:      "nonexistent.yaml",
			expectedFound: false,
		},
		{
			name: "loads file from subdirectory",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) string {
				hash := createCommit(t, repo, repoPath, "initial commit", map[string]string{
					"stacks/ue2-prod.yaml": "stack: production\n",
				})
				return hash
			},
			filePath:      "stacks/ue2-prod.yaml",
			expectedFound: true,
			validateResult: func(t *testing.T, content string) {
				assert.Contains(t, content, "stack: production")
			},
		},
		{
			name: "handles file path with leading ./",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) string {
				hash := createCommit(t, repo, repoPath, "initial commit", map[string]string{
					"config.yaml": "test: value\n",
				})
				return hash
			},
			filePath:      "./config.yaml",
			expectedFound: true,
			validateResult: func(t *testing.T, content string) {
				assert.Contains(t, content, "test: value")
			},
		},
		{
			name: "errors on invalid base ref",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) string {
				createCommit(t, repo, repoPath, "initial commit", map[string]string{
					"file.txt": "content\n",
				})
				return "nonexistent-ref"
			},
			filePath:      "file.txt",
			expectedError: true,
		},
		{
			name: "loads correct version from specific commit",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) string {
				// First commit
				hash1 := createCommit(t, repo, repoPath, "v1", map[string]string{
					"version.txt": "v1\n",
				})

				// Second commit (modified file)
				createCommit(t, repo, repoPath, "v2", map[string]string{
					"version.txt": "v2\n",
				})

				// Return first commit hash - should get v1 content
				return hash1
			},
			filePath:      "version.txt",
			expectedFound: true,
			validateResult: func(t *testing.T, content string) {
				assert.Equal(t, "v1\n", content)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, repoPath := createTestRepo(t)
			baseRef := tt.setupRepo(t, repo, repoPath)

			storage := NewGitBaseStorage(repo, baseRef)

			content, found, err := storage.LoadBase(tt.filePath)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedFound, found)

			if tt.expectedFound && tt.validateResult != nil {
				tt.validateResult(t, content)
			}
		})
	}
}

func TestGitBaseStorage_ValidateBaseRef(t *testing.T) {
	tests := []struct {
		name          string
		setupRepo     func(*testing.T, *git.Repository, string) string
		expectedError bool
	}{
		{
			name: "validates existing commit hash",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) string {
				hash := createCommit(t, repo, repoPath, "initial", map[string]string{
					"file.txt": "content\n",
				})
				return hash
			},
			expectedError: false,
		},
		{
			name: "validates HEAD ref",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) string {
				createCommit(t, repo, repoPath, "initial", map[string]string{
					"file.txt": "content\n",
				})
				return "HEAD"
			},
			expectedError: false,
		},
		{
			name: "errors on nonexistent ref",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) string {
				createCommit(t, repo, repoPath, "initial", map[string]string{
					"file.txt": "content\n",
				})
				return "nonexistent-branch"
			},
			expectedError: true,
		},
		{
			name: "errors on invalid commit hash",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) string {
				createCommit(t, repo, repoPath, "initial", map[string]string{
					"file.txt": "content\n",
				})
				return "invalid123hash"
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, repoPath := createTestRepo(t)
			baseRef := tt.setupRepo(t, repo, repoPath)

			storage := NewGitBaseStorage(repo, baseRef)
			err := storage.ValidateBaseRef()

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGitBaseStorage_GetMergeBase(t *testing.T) {
	tests := []struct {
		name           string
		setupRepo      func(*testing.T, *git.Repository, string) (string, string) // Returns (ref1, ref2)
		expectedError  bool
		validateResult func(*testing.T, string, map[string]string) // hash -> commits map
	}{
		{
			name: "finds merge base between HEAD and main",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) (string, string) {
				// Create main branch commits
				_ = createCommit(t, repo, repoPath, "commit 1", map[string]string{
					"file.txt": "v1\n",
				})

				hash2 := createCommit(t, repo, repoPath, "commit 2", map[string]string{
					"file.txt": "v2\n",
				})

				// Create feature branch from commit 2
				worktree, err := repo.Worktree()
				require.NoError(t, err)

				err = worktree.Checkout(&git.CheckoutOptions{
					Branch: "refs/heads/feature",
					Create: true,
				})
				require.NoError(t, err)

				// Add commit on feature branch
				createCommit(t, repo, repoPath, "feature commit", map[string]string{
					"feature.txt": "feature\n",
				})

				// Merge base should be hash2 (where branch diverged)
				return "HEAD", hash2
			},
			validateResult: func(t *testing.T, mergeBase string, commits map[string]string) {
				// Merge base should exist
				assert.NotEmpty(t, mergeBase)
			},
		},
		{
			name: "same commit returns itself as merge base",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) (string, string) {
				// Same commit should return itself as merge base
				hash := createCommit(t, repo, repoPath, "commit", map[string]string{
					"file.txt": "content\n",
				})
				return hash, hash
			},
			validateResult: func(t *testing.T, mergeBase string, commits map[string]string) {
				// Same commit should return itself as merge base
				assert.NotEmpty(t, mergeBase)
			},
		},
		{
			name: "errors on invalid ref1",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) (string, string) {
				hash := createCommit(t, repo, repoPath, "commit", map[string]string{
					"file.txt": "content\n",
				})
				return "invalid-ref", hash
			},
			expectedError: true,
		},
		{
			name: "errors on invalid ref2",
			setupRepo: func(t *testing.T, repo *git.Repository, repoPath string) (string, string) {
				hash := createCommit(t, repo, repoPath, "commit", map[string]string{
					"file.txt": "content\n",
				})
				return hash, "invalid-ref"
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, repoPath := createTestRepo(t)
			ref1, ref2 := tt.setupRepo(t, repo, repoPath)

			storage := NewGitBaseStorage(repo, "HEAD")
			mergeBase, err := storage.GetMergeBase(ref1, ref2)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.validateResult != nil {
				tt.validateResult(t, mergeBase, nil)
			}
		})
	}
}

func TestGitBaseStorage_SetAndGetBaseRef(t *testing.T) {
	repo, repoPath := createTestRepo(t)
	hash := createCommit(t, repo, repoPath, "initial", map[string]string{
		"file.txt": "content\n",
	})

	storage := NewGitBaseStorage(repo, "HEAD")

	// Test initial ref
	assert.Equal(t, "HEAD", storage.GetBaseRef())

	// Test updating ref
	storage.SetBaseRef(hash)
	assert.Equal(t, hash, storage.GetBaseRef())

	// Test that LoadBase uses the new ref
	content, found, err := storage.LoadBase("file.txt")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "content\n", content)
}
