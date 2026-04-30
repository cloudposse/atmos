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

// validateBranchName validates a branch name using git's own rules.
// It distinguishes between invalid branch names (ExitError) and git execution
// failures (missing binary, permissions, etc.), returning the appropriate error.
func validateBranchName(branch string) error {
	if branch == "" {
		return fmt.Errorf("%w: %q", ErrInvalidBranchName, branch)
	}
	cmd := exec.Command("git", "check-ref-format", "--branch", branch)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("%w: %q", ErrInvalidBranchName, branch)
		}
		return fmt.Errorf("validating branch %q with git check-ref-format: %w", branch, err)
	}
	return nil
}

// FetchRef fetches a single branch from the "origin" remote using a narrow refspec.
// This minimizes data transfer compared to a full fetch, which is important for
// CI shallow clones where remote-tracking refs may not exist locally.
// The repoDir should be a path inside the repository.
func FetchRef(repoDir, branch string) error {
	defer perf.Track(nil, "git.FetchRef")()

	if err := validateBranchName(branch); err != nil {
		return err
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

// DeepenFetch deepens a shallow clone by fetching additional history for a branch.
// Used as a second-stage fetch when the initial FetchRef succeeds but merge-base
// still cannot find a common ancestor (i.e., the fork point is older than the
// initial fetch depth).
//
// Depth is the number of commits to deepen by, measured from the current
// shallow boundary. A value of 0 or negative means "fetch full history"
// (equivalent to --unshallow).
func DeepenFetch(repoDir, branch string, depth int) error {
	defer perf.Track(nil, "git.DeepenFetch")()

	if err := validateBranchName(branch); err != nil {
		return err
	}

	refspec := fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch)
	args := []string{"fetch", "origin", refspec, "--no-tags"}
	if depth <= 0 {
		args = append(args, "--unshallow")
	} else {
		args = append(args, fmt.Sprintf("--deepen=%d", depth))
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("deepening origin/%s: %w\n%s", branch, err, string(output))
	}

	log.Debug("Deepened remote branch", "branch", branch, "depth", depth)

	return nil
}
