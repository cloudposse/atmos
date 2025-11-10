package config

import (
	"os"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// applyGitRootBasePath automatically sets the base path to the Git repository root
// when using the default configuration and no local Atmos config exists.
//
// This function implements smart defaults:
// - Only applies to default config (no explicit atmos.yaml path provided).
// - Skips if local Atmos configuration exists (atmos.yaml, .atmos/, etc.).
// - Skips if base_path is explicitly set to a non-default value.
// - Can be disabled via ATMOS_GIT_ROOT_BASEPATH=false.
//
// This allows users to run `atmos` from anywhere in a Git repository
// without needing to specify --config-dir or base_path.
func applyGitRootBasePath(atmosConfig *schema.AtmosConfiguration) error {
	// Bootstrap configuration: Read directly because this controls git root discovery during
	// config loading itself, before processEnvVars() populates the Settings struct.
	// Similar pattern: ATMOS_CLI_CONFIG_PATH in readEnvAmosConfigPath() controls WHERE to load config.
	// Cannot use atmosConfig.Settings as it's populated AFTER LoadConfig() completes.
	//nolint:forbidigo // ATMOS_GIT_ROOT_BASEPATH is bootstrap config, not application configuration.
	if os.Getenv("ATMOS_GIT_ROOT_BASEPATH") == "false" {
		log.Trace("Git root base path disabled via ATMOS_GIT_ROOT_BASEPATH=false")
		return nil
	}

	log.Debug("Git root base path discovery enabled")

	// CRITICAL: Only apply if current directory does NOT have any Atmos configuration.
	// This ensures local configs/directories take precedence over git root discovery.
	cwd, err := os.Getwd()
	if err != nil {
		log.Trace("Failed to get current directory", "error", err)
		return nil
	}

	// Check for any Atmos configuration indicators in current directory.
	if hasLocalAtmosConfig(cwd) {
		return nil
	}

	// Only apply to default config with default base path.
	if !atmosConfig.Default {
		log.Trace("Skipping git root base path (not default config)")
		return nil
	}

	if atmosConfig.BasePath != "." && atmosConfig.BasePath != "" {
		log.Trace("Skipping git root base path (explicit base_path set)", "base_path", atmosConfig.BasePath)
		return nil
	}

	// Resolve git root.
	gitRoot, err := u.ProcessTagGitRoot("!repo-root .")
	if err != nil {
		log.Trace("Git root detection failed", "error", err)
		return err
	}

	// Only update if we found a different root.
	if gitRoot != "." {
		atmosConfig.BasePath = gitRoot
		log.Debug("Using git repository root as base path", "path", gitRoot)
	} else {
		log.Trace("Git root same as current directory")
	}

	return nil
}

// hasLocalAtmosConfig checks if the current directory has any Atmos configuration.
// This includes config files, config directories, and default import directories.
//
// Returns true if ANY of these exist in the given directory:
// - atmos.yaml (main config file)
// - .atmos.yaml (hidden config file)
// - .atmos/ (config directory)
// - .atmos.d/ (default imports directory)
// - atmos.d/ (default imports directory - alternate).
func hasLocalAtmosConfig(dir string) bool {
	configIndicators := []string{
		AtmosConfigFileName,           // Main config file.
		DotAtmosConfigFileName,        // Hidden config file.
		AtmosConfigDirName,            // Config directory.
		DotAtmosDefaultImportsDirName, // Default imports directory.
		AtmosDefaultImportsDirName,    // Default imports directory (alternate).
	}

	for _, indicator := range configIndicators {
		indicatorPath := filepath.Join(dir, indicator)
		if _, err := os.Stat(indicatorPath); err == nil {
			log.Trace("Found Atmos configuration in current directory, skipping git root discovery",
				"indicator", indicator, "path", indicatorPath)
			return true
		}
	}

	return false
}
