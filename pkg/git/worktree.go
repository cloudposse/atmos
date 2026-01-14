package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const worktreeSubdir = "worktree"

// CreateWorktree creates a new git worktree at the specified path, checked out to the given target ref or SHA.
// This uses `git worktree add --detach` to create an isolated worktree that shares the repository's
// object database but has its own HEAD, allowing checkout operations without affecting the main worktree.
// The repoDir should be the path to any directory in the repository (main worktree or any linked worktree).
// Returns the path to the created worktree directory.
func CreateWorktree(repoDir, targetCommit string) (string, error) {
	defer perf.Track(nil, "git.CreateWorktree")()

	// Create a temp dir for the worktree.
	// Note: We create a parent temp dir and use a subdirectory because
	// git worktree add requires the target directory to not exist.
	tempParentDir, err := os.MkdirTemp("", "atmos-worktree-")
	if err != nil {
		return "", err
	}
	worktreePath := filepath.Join(tempParentDir, worktreeSubdir)

	// Use git worktree add to create an isolated worktree.
	// The --detach flag creates the worktree with a detached HEAD, which is what we want
	// since we're checking out a specific ref/sha for comparison purposes.
	// This works correctly whether we're in a regular repo or already in a worktree.
	cmd := exec.Command("git", "worktree", "add", "--detach", worktreePath, targetCommit)
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Clean up the temp parent directory on error.
		os.RemoveAll(tempParentDir)
		return "", errUtils.Build(errUtils.ErrGitRefNotFound).
			WithCause(err).
			WithContext("ref", targetCommit).
			WithContext("output", string(output)).
			WithHint("Make sure the ref is correct and was cloned by Git from the remote, or use the '--clone-target-ref=true' flag to clone it.").
			WithHint("Refer to https://atmos.tools/cli/commands/describe/affected for more details.").
			Err()
	}

	log.Debug("Created git worktree", "dir", worktreePath, "target", targetCommit)

	return worktreePath, nil
}

// RemoveWorktree removes a git worktree using `git worktree remove`.
// This properly unregisters the worktree from git's tracking in addition to removing the directory.
// The repoDir parameter should be the path to any directory in the repository (main worktree or any linked worktree).
func RemoveWorktree(repoDir, worktreePath string) {
	defer perf.Track(nil, "git.RemoveWorktree")()

	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn("Failed to remove git worktree", "path", worktreePath, "error", err.Error(), "output", string(output))
	}
}

// GetWorktreeParentDir returns the parent directory of a worktree path.
// This is useful for cleanup since CreateWorktree creates a parent temp dir containing the worktree.
func GetWorktreeParentDir(worktreePath string) string {
	defer perf.Track(nil, "git.GetWorktreeParentDir")()

	// The worktree is always created as a "worktree" subdirectory of the parent temp dir.
	// So parent is worktreePath minus the worktree suffix.
	suffix := string(filepath.Separator) + worktreeSubdir
	if strings.HasSuffix(worktreePath, suffix) {
		return strings.TrimSuffix(worktreePath, suffix)
	}
	// Fallback: just return the path itself.
	return worktreePath
}
