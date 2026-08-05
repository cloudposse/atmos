package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	atmosGit "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ProcessTagCwd returns the current working directory.
// If a path argument is provided after the tag, it is joined with CWD.
// Format: "!cwd" or "!cwd <path>".
func ProcessTagCwd(input string) (string, error) {
	defer perf.Track(nil, "utils.ProcessTagCwd")()

	str := strings.TrimPrefix(input, AtmosYamlFuncCwd)
	pathArg := strings.TrimSpace(str)

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	// If no path argument, return CWD.
	if pathArg == "" {
		return cwd, nil
	}

	// Join CWD with the provided path.
	return filepath.Join(cwd, pathArg), nil
}

// ProcessTagGitRoot returns the root directory of the Git repository using go-git.
func ProcessTagGitRoot(input string) (string, error) {
	defer perf.Track(nil, "utils.ProcessTagGitRoot")()

	// Check if we're in test mode and should use a mock Git root.
	//nolint:forbidigo // TEST_GIT_ROOT is specifically for test isolation, not application configuration
	if testGitRoot := os.Getenv("TEST_GIT_ROOT"); testGitRoot != "" {
		log.Trace("Using test Git root override", "path", testGitRoot)
		return testGitRoot, nil
	}

	return atmosGit.ProcessTagRoot(input)
}
