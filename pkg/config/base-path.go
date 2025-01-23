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
	// Is --base-path Provided?
	if configAndStacksInfo.BasePathFromArg != "" {
		absPath, err := filepath.Abs(configAndStacksInfo.BasePathFromArg)
		if err != nil {
			return "", err
		}

		return absPath, nil
	}
	// Is ATMOS_BASE_PATH Set ?
	if os.Getenv("ATMOS_BASE_PATH") != "" {
		absPath, err := filepath.Abs(os.Getenv("ATMOS_BASE_PATH"))
		if err != nil {
			return "", err
		}
		return absPath, nil
	}
	// Is base_path Set in Configuration?
	if cl.atmosConfig.BasePath != "" {
		//Is base_path Absolute?
		absPath, err := filepath.Abs(configAndStacksInfo.BasePath)
		if err == nil {
			return absPath, nil
		}
		//Resolve base_path relative to current working directory
		pwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		absPath, err = filepath.Abs(pwd)
		if err != nil {
			return "", err
		}
		return absPath, nil
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
		return absPath, nil
	}
	//Set base_path to absolute path of ./
	return "./", nil
}
func (cl *ConfigLoader) infraBasePath(cwd string) (string, bool) {
	// CWD Check CWD for atmos.yaml, .atmos.yaml, atmos.d/**/*, .github/atmos.yaml
	// if found Set base_path to absolute path containing directory
	filePath, found := cl.SearchConfigFilePath(filepath.Join(cwd, "atmos"))
	if found {
		return filepath.Dir(filePath), found
	}
	filePath, found = cl.SearchConfigFilePath(filepath.Join(cwd, ".atmos"))
	if found {
		return filepath.Dir(filePath), found
	}
	filePaths, _ := u.GetGlobMatches(filepath.ToSlash(filepath.Join(cwd, "atmos.d/**/*.yaml")))
	if len(filePaths) == 0 {
		filePaths, _ = u.GetGlobMatches(filepath.ToSlash(filepath.Join(cwd, "atmos.d/**/*.yml")))
	}
	if len(filePaths) > 0 {
		filePaths = cl.sortFilesByDepth(filePaths)
		return filepath.Dir(filePaths[0]), true
	}
	gitTopLevel, err := GetGitRoot(cwd)
	if err == nil {
		dirAbs, found := cl.SearchConfigFilePath(filepath.Join(gitTopLevel, "atmos"))
		if found {
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
