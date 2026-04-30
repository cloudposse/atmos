package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestFetchRef_InvalidBranchName(t *testing.T) {
	tests := []struct {
		name   string
		branch string
	}{
		{"empty", ""},
		{"starts with dot", ".hidden"},
		{"contains space", "my branch"},
		{"contains double dot", "main..evil"},
		{"starts with hyphen", "-flag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FetchRef(".", tt.branch)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidBranchName)
		})
	}
}

func TestFetchRef_ValidBranchNames(t *testing.T) {
	// These should pass validation but may fail on the actual fetch (no remote).
	// We just verify git accepts them as valid branch names.
	validNames := []string{
		"main",
		"develop",
		"feature/my-feature",
		"release/v1.2.3",
		"fix-123",
		"user.name/branch",
		"_release",
		"feature+1",
	}

	for _, name := range validNames {
		assert.NoError(t, validateBranchName(name), "expected %q to be valid", name)
	}
}

func TestFetchRef_FetchFromLocalBareRepo(t *testing.T) {
	// Create a working repo to act as "origin" (with an initial commit).
	originDir := t.TempDir()
	runGit(t, originDir, "init")
	testFile := filepath.Join(originDir, "initial.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("initial"), 0o644))
	runGit(t, originDir, "add", "initial.txt")
	runGit(t, originDir, "commit", "-m", "initial commit")

	// Detect the default branch name (may be "main" or "master" depending on git config).
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = originDir
	out, err := cmd.Output()
	require.NoError(t, err)
	defaultBranch := strings.TrimSpace(string(out))

	// Create a second branch with a commit.
	runGit(t, originDir, "checkout", "-b", "test-branch")
	branchFile := filepath.Join(originDir, "branch.txt")
	require.NoError(t, os.WriteFile(branchFile, []byte("branch"), 0o644))
	runGit(t, originDir, "add", "branch.txt")
	runGit(t, originDir, "commit", "-m", "branch commit")
	runGit(t, originDir, "checkout", defaultBranch)

	// Clone only the default branch (single-branch).
	cloneDir := t.TempDir()
	runGit(t, cloneDir, "clone", "--single-branch", originDir, ".")

	// Verify origin/test-branch doesn't exist yet.
	cmd = exec.Command("git", "rev-parse", "--verify", "origin/test-branch")
	cmd.Dir = cloneDir
	assert.Error(t, cmd.Run(), "origin/test-branch should not exist before fetch")

	// Fetch the branch.
	err = FetchRef(cloneDir, "test-branch")
	require.NoError(t, err)

	// Verify origin/test-branch now exists.
	cmd = exec.Command("git", "rev-parse", "--verify", "origin/test-branch")
	cmd.Dir = cloneDir
	assert.NoError(t, cmd.Run(), "origin/test-branch should exist after fetch")
}

func TestFetchRef_NonexistentBranch(t *testing.T) {
	// Create a repo with an initial commit to act as origin.
	originDir := t.TempDir()
	runGit(t, originDir, "init")
	testFile := filepath.Join(originDir, "file.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0o644))
	runGit(t, originDir, "add", "file.txt")
	runGit(t, originDir, "commit", "-m", "initial")

	// Clone it.
	workDir := t.TempDir()
	runGit(t, workDir, "clone", originDir, ".")

	// Fetch a branch that doesn't exist.
	err := FetchRef(workDir, "nonexistent-branch")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFetchOrigin)
	assert.Contains(t, err.Error(), "origin/nonexistent-branch")
}

func TestDeepenFetch_InvalidBranchName(t *testing.T) {
	err := DeepenFetch(".", "..bad", 50)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidBranchName)
}

// runGit runs a git command in the given directory, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
}
