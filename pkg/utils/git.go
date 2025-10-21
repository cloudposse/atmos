package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ProcessTagGitRoot returns the root directory of the Git repository using go-git.
func ProcessTagGitRoot(input string) (string, error) {
	defer perf.Track(nil, "utils.ProcessTagGitRoot")()

	str := strings.TrimPrefix(input, AtmosYamlFuncGitRoot)
	defaultValue := strings.TrimSpace(str)

	// Check if we're in test mode and should use a mock Git root
	//nolint:forbidigo // TEST_GIT_ROOT is specifically for test isolation, not application configuration
	if testGitRoot := os.Getenv("TEST_GIT_ROOT"); testGitRoot != "" {
		log.Debug("Using test Git root override", "path", testGitRoot)
		return testGitRoot, nil
	}

	startPath, err := os.Getwd()
	if err != nil {
		if defaultValue != "" {
			log.Debug("failed to get current working directory !repo-root return default value", "error", err)
			return defaultValue, nil
		}
		return "", err
	}
	// Open the repository starting from the given path
	repo, err := git.PlainOpenWithOptions(startPath, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		if defaultValue != "" {
			log.Debug("failed to open Git repository !repo-root return default value", "error", err)
			return defaultValue, nil
		}
		return "", fmt.Errorf("failed to open Git repository: %w", err)
	}
	// Get the worktree to extract the repository's root directory
	worktree, err := repo.Worktree()
	if err != nil {
		if defaultValue != "" {
			log.Debug("failed to get worktree !repo-root return default value", "error", err)
			return defaultValue, nil
		}
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}
	// Return the absolute path to the root directory
	rootPath, err := filepath.Abs(worktree.Filesystem.Root())
	if err != nil {
		if defaultValue != "" {
			log.Debug("failed to get absolute path !repo-root return default value", "error", err)
			return defaultValue, nil
		}
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	return rootPath, nil
}
