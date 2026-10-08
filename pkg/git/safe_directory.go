package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/github/actions"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// EnsureGitSafeDirectory adds GITHUB_WORKSPACE to git's safe.directory list
// when running in a GitHub Actions container. Container jobs run as a different
// user than the checkout owner, causing git to reject the repo as "dubious ownership".
// It is idempotent: the entry is added only when the global list does not already hold it,
// so repeated calls leave the runner's git config unchanged.
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

	if safeDirectoryConfigured(workspace) {
		log.Debug("GITHUB_WORKSPACE is already in git safe.directory.", "path", workspace)
		return nil
	}

	log.Debug("Adding GITHUB_WORKSPACE to git safe.directory.", "path", workspace)

	cmd := exec.Command("git", "config", "--global", "--add", "safe.directory", workspace) //nolint:gosec // workspace is cleaned above.
	// git's own output goes to stderr so it can never corrupt data written to stdout.
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add safe.directory for %s: %w", workspace, err)
	}

	return nil
}

// safeDirectoryConfigured reports whether workspace is already a global safe.directory entry.
// Any failure to read the list reads as "not configured", so the caller falls back to adding it.
func safeDirectoryConfigured(workspace string) bool {
	out, err := exec.Command("git", "config", "--global", "--get-all", "safe.directory").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == workspace {
			return true
		}
	}
	return false
}
