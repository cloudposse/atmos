package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	git "github.com/go-git/go-git/v5"
)

func (cl *ConfigLoader) BasePathComputing(configAndStacksInfo schema.ConfigAndStacksInfo) (string, error) {

	// Check base path from CLI argument
	if configAndStacksInfo.BasePathFromArg != "" {
		return cl.resolveAndValidatePath(configAndStacksInfo.BasePathFromArg, "CLI argument")
	}

	// Check base path from ATMOS_BASE_PATH environment variable
	if envBasePath := os.Getenv("ATMOS_BASE_PATH"); envBasePath != "" {
		return cl.resolveAndValidatePath(envBasePath, "ENV var")
	}

	// Check base path from configuration
	// Check base path from configuration
	if cl.atmosConfig.BasePath != "" {
		source := "atmos config"
		if !filepath.IsAbs(cl.atmosConfig.BasePath) {
			// If relative, make it absolute based on the current working directory
			absPath, err := filepath.Abs(cl.atmosConfig.BasePath)
			if err != nil {
				return "", fmt.Errorf("failed to resolve relative base path from %s: %w", source, err)
			}
			cl.atmosConfig.BasePath = absPath
		}
		return cl.resolveAndValidatePath(cl.atmosConfig.BasePath, source)
	}

	//Infer base_path
	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	resolvedPath, found := cl.infraBasePath(pwd)
	if found {
		absPath, err := filepath.Abs(resolvedPath)
		if err != nil {
			return "", err
		}
		cl.logging(fmt.Sprintf("base path from %s: %s", "infra", absPath))
		return absPath, nil
	}
	//Set base_path to absolute path of ./
	absPath, err := filepath.Abs("./")
	if err != nil {
		return "", err
	}
	cl.logging(fmt.Sprintf("base path from %s: %s", "PWD", pwd))
	return absPath, nil
}

// Helper to resolve and validate base path
func (cl *ConfigLoader) resolveAndValidatePath(path string, source string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("base path from %s does not exist: %w", source, err)
	}

	isDir, err := u.IsDirectory(absPath)
	if err != nil {
		if err == os.ErrNotExist {
			return "", fmt.Errorf("base path from %s does not exist: %w", source, err)
		}
		return "", err
	}

	if !isDir {
		return "", fmt.Errorf("base path from %s is not a directory: %w", source, err)
	}

	cl.logging(fmt.Sprintf("base path from %s: %s", source, path))
	return absPath, nil
}

func (cl *ConfigLoader) infraBasePath(cwd string) (string, bool) {
	// CWD Check CWD for atmos.yaml, .atmos.yaml, atmos.d/**/*, .github/atmos.yaml
	// if found Set base_path to absolute path containing directory
	filePath, found := cl.SearchConfigFilePath(filepath.Join(cwd, "atmos"))
	if found {
		cl.logging(fmt.Sprintf("base path from infra %s: %s", "atmos", filePath))
		return filepath.Dir(filePath), found
	}
	filePath, found = cl.SearchConfigFilePath(filepath.Join(cwd, ".atmos"))
	if found {
		cl.logging(fmt.Sprintf("base path from infra %s: %s", ".atmos", filePath))
		return filepath.Dir(filePath), found
	}
	filePaths, _ := u.GetGlobMatches(filepath.ToSlash(filepath.Join(cwd, "atmos.d/**/*.yaml")))
	if len(filePaths) == 0 {
		filePaths, _ = u.GetGlobMatches(filepath.ToSlash(filepath.Join(cwd, "atmos.d/**/*.yml")))
	}
	if len(filePaths) > 0 {
		filePaths = cl.sortFilesByDepth(filePaths)
		cl.logging(fmt.Sprintf("base path from infra %s: %s", "atmos.d", filePaths[0]))
		return filepath.Dir(filePaths[0]), true
	}
	gitTopLevel, err := GetGitRoot(cwd)
	if err == nil {
		dirAbs, found := cl.SearchConfigFilePath(filepath.Join(gitTopLevel, "atmos"))
		if found {
			cl.logging(fmt.Sprintf("base path from infra %s: %s", "git root", filePath))

			return dirAbs, found
		}
	}

	return "", false

}

// GetGitRoot returns the root directory of the Git repository using go-git.
func GetGitRoot(startPath string) (string, error) {
	// Open the repository starting from the given path
	repo, err := git.PlainOpenWithOptions(startPath, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to open Git repository: %w", err)
	}
	// Get the worktree to extract the repository's root directory
	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}
	// Return the absolute path to the root directory
	rootPath, err := filepath.Abs(worktree.Filesystem.Root())
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	return rootPath, nil
}
