package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func (cl *ConfigLoader) BasePathComputing(configAndStacksInfo schema.ConfigAndStacksInfo) (string, error) {
	// Check base path from CLI argument
	if configAndStacksInfo.BasePathFromArg != "" {
		return cl.resolveAndValidatePath(configAndStacksInfo.BasePathFromArg, "--config")
	}

	// Check base path from ATMOS_BASE_PATH environment variable
	if envBasePath := os.Getenv("ATMOS_BASE_PATH"); envBasePath != "" {
		return cl.resolveAndValidatePath(envBasePath, "ATMOS_BASE_PATH")
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

	// Infer base_path
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
		log.Debug("base path derived from infra", "base_path", absPath)
		return absPath, nil
	}
	// Set base_path to absolute path of ./
	absPath, err := filepath.Abs("./")
	if err != nil {
		return "", err
	}
	log.Debug("base path derived from PWD", "pwd", pwd)
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
		return "", fmt.Errorf("base path from %s is not a directory", source)
	}
	log.Debug("base path derived from", source, path)
	return absPath, nil
}

func (cl *ConfigLoader) infraBasePath(cwd string) (string, bool) {
	// CWD Check CWD for atmos.yaml, .atmos.yaml, atmos.d/**/*, .github/atmos.yaml
	// if found Set base_path to absolute path containing directory
	filePath, found := cl.SearchConfigFilePath(filepath.Join(cwd, "atmos"))
	if found {
		log.Debug("base path derived from infra atmos file", "path", filePath)
		return filepath.Dir(filePath), found
	}
	filePath, found = cl.SearchConfigFilePath(filepath.Join(cwd, ".atmos"))
	if found {
		log.Debug("base path derived from infra .atmos file", "path", filePath)
		return filepath.Dir(filePath), found
	}
	filePaths, _ := u.GetGlobMatches(filepath.ToSlash(filepath.Join(cwd, "atmos.d/**/*.yaml")))
	if len(filePaths) == 0 {
		filePaths, _ = u.GetGlobMatches(filepath.ToSlash(filepath.Join(cwd, "atmos.d/**/*.yml")))
	}
	if len(filePaths) > 0 {
		filePaths = cl.sortFilesByDepth(filePaths)
		log.Debug("base path derived from infra atmos.d file", "path", filePaths[0])
		return filepath.Dir(filePaths[0]), true
	}
	gitTopLevel, err := u.GetGitRoot()
	if err == nil {
		dirAbs, found := cl.SearchConfigFilePath(filepath.Join(gitTopLevel, "atmos"))
		if found {
			log.Debug("base path derived from infra git root", "path", filePath)
			return dirAbs, found
		}
	}

	return "", false
}
