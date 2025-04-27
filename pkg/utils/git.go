package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"

	git "github.com/go-git/go-git/v5"
)

// GetGitRoot returns the root directory of the Git repository using go-git.
func ProcessTagGitRoot(input string) (string, error) {
	str := strings.TrimPrefix(input, AtmosYamlFuncGitRoot)
	defaultValue := strings.TrimSpace(str)

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
