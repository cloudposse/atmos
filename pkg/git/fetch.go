package git

import (
	"errors"
	"fmt"
	"os/exec"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ErrInvalidBranchName indicates a branch name contains invalid characters.
var ErrInvalidBranchName = errors.New("invalid branch name")

// isValidBranchName validates a branch name using git's own rules.
func isValidBranchName(branch string) bool {
	if branch == "" {
		return false
	}
	cmd := exec.Command("git", "check-ref-format", "--branch", branch)
	return cmd.Run() == nil
}

// FetchRef fetches a single branch from the "origin" remote using a narrow refspec.
// This minimizes data transfer compared to a full fetch, which is important for
// CI shallow clones where remote-tracking refs may not exist locally.
// The repoDir should be a path inside the repository.
func FetchRef(repoDir, branch string) error {
	defer perf.Track(nil, "git.FetchRef")()

	if !isValidBranchName(branch) {
		return fmt.Errorf("%w: %q", ErrInvalidBranchName, branch)
	}

	refspec := fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch)
	cmd := exec.Command("git", "fetch", "origin", refspec, "--no-tags")
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fetching origin/%s: %w\n%s", branch, err, string(output))
	}

	log.Debug("Fetched remote branch", "branch", branch)

	return nil
}
