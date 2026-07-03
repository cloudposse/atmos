package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/github/actions"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// EnsureGitSafeDirectory adds GITHUB_WORKSPACE to git's safe.directory list
// when running in a GitHub Actions container. Container jobs run as a different
// user than the checkout owner, causing git to reject the repo as "dubious ownership".
func EnsureGitSafeDirectory() error {
	if !actions.IsGitHubActions() {
		return nil
	}

	//nolint:forbidigo // GITHUB_WORKSPACE is an external CI env var, not Atmos config.
	workspace := os.Getenv("GITHUB_WORKSPACE")
	if workspace == "" {
		return nil
	}

	// Clean the path to satisfy gosec taint analysis (G702).
	workspace = filepath.Clean(workspace)

	log.Debug("Adding GITHUB_WORKSPACE to git safe.directory.", "path", workspace)

	cmd := exec.Command("git", "config", "--global", "--add", "safe.directory", workspace) //nolint:gosec // workspace is cleaned above.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add safe.directory for %s: %w", workspace, err)
	}

	return nil
}
