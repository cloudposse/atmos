package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// EnsureSafeDirectory configures git safe.directory for the GitHub Actions
// workspace when running inside a CI container. In containers, the repository
// owner may differ from the running user, causing git to refuse operations.
//
// This only acts when the GITHUB_WORKSPACE environment variable is set
// (i.e., inside GitHub Actions). The value is trusted because it is set
// by the GitHub Actions runner, not by user input.
//
// Idempotent: safe to call multiple times.
func EnsureSafeDirectory() error {
	defer perf.Track(nil, "git.EnsureSafeDirectory")()

	workspace := os.Getenv("GITHUB_WORKSPACE") //nolint:forbidigo // Reading CI env var directly, not an Atmos config flag.
	if workspace == "" {
		return nil
	}

	// Clean the path to normalize it.
	workspace = filepath.Clean(workspace)

	cmd := exec.Command("git", "config", "--global", "--add", "safe.directory", workspace) //nolint:gosec // workspace is from the trusted GITHUB_WORKSPACE env var set by the GitHub Actions runner.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("setting safe.directory for %s: %w\n%s", workspace, err, string(output))
	}

	log.Debug("Configured git safe.directory", "path", workspace)

	return nil
}
