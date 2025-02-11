package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Derive the effective base-base based on multiple inferences
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

	resolvedPath, found := cl.inferBasePath()
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

// Check CWD for atmos.yaml, .atmos.yaml, atmos.d/**/*, .github/atmos.yaml
// if found return base_path to absolute path containing directory
func (cl *ConfigLoader) inferBasePath() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		log.Debug("failed to get current working directory", "err", err)
		return "", false
	}

	// Check for atmos.yaml or atmos.yml
	if basePath, found := cl.checkConfigFileOrDir(filepath.Join(cwd, CliConfigFileName), "atmos configuration directory", false); found {
		return basePath, true
	}
	// Check for .atmos.yaml or .atmos.yml
	if basePath, found := cl.checkConfigFileOrDir(filepath.Join(cwd, ".atmos"), ".atmos configuration directory", false); found {
		return basePath, true
	}
	// Check for atmos.d directory exist
	found, _ := u.IsDirectory(filepath.ToSlash(filepath.Join(cwd, "atmos.d")))
	if found {
		// Check for atmos.d directory has .yaml r .yml files
		if basePath, found := cl.checkConfigFileOrDir(filepath.Join(cwd, "atmos.d"), "atmos.d/ directory", true); found {
			return basePath, true
		}
	}

	// Check for git root
	gitTopLevel, err := u.GetGitRoot()
	if err == nil {
		if basePath, found := cl.checkConfigFileOrDir(filepath.Join(gitTopLevel, CliConfigFileName), "git root", false); found {
			return basePath, true
		}
	}

	return "", false
}

// checkConfigFileOrDir checks for the existence of a atmos config file and returns the base path if found.
func (cl *ConfigLoader) checkConfigFileOrDir(path, desc string, isDir bool) (string, bool) {
	if isDir {
		filePaths, _ := u.GetGlobMatches(filepath.ToSlash(filepath.Join(path, "**/*.yaml")))
		if len(filePaths) == 0 {
			filePaths, _ = u.GetGlobMatches(filepath.ToSlash(filepath.Join(path, "**/*.yml")))
		}
		if len(filePaths) > 0 {
			// sort files by depth
			filePaths = cl.sortFilesByDepth(filePaths)
			log.Debug("base path inferred from "+desc, "path", filepath.Dir(filePaths[0]))
			// return directory of first file found
			return filepath.Dir(filePaths[0]), true
		}

	} else {
		filePath, found := cl.SearchConfigFilePath(path)
		if found {
			log.Debug("base-path inferred from "+desc, "path", filepath.Dir(filePath))
			return filepath.Dir(filePath), true
		}
	}
	return "", false
}
