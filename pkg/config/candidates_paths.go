package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/config/projectroot"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// This file holds the path-computation helpers shared by the config loader (load.go) and the
// non-parsing candidate resolver (candidates.go). Keeping them in one place guarantees the
// resolver reports exactly the locations the loader reads.

// envConfigPath returns the ATMOS_CLI_CONFIG_PATH value ("" when unset).
func envConfigPath() string {
	//nolint:forbidigo // ATMOS_CLI_CONFIG_PATH controls config loading behavior itself.
	return os.Getenv(AtmosCliConfigPathEnvVar)
}

// systemConfigDir returns the system-wide config directory, or "" when the platform has none.
func systemConfigDir() string {
	if runtime.GOOS == "windows" {
		return os.Getenv(WindowsAppDataEnvVar) //nolint:forbidigo // Bootstrap path lookup, not application configuration.
	}
	return SystemDirConfigFilePath
}

// homeConfigDir returns the per-user config directory (`~/.atmos`).
func homeConfigDir(homeProvider filesystem.HomeDirProvider) (string, error) {
	home, err := homeProvider.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".atmos"), nil
}

// parentConfigSearchDir returns the nearest parent directory of wd containing an Atmos config,
// or "" when the search is skipped (wd has its own Atmos configuration) or nothing is found.
// The caller is responsible for the ATMOS_CLI_CONFIG_PATH short-circuit.
func parentConfigSearchDir(wd string) string {
	log.Debug("readParentDirConfig checking CWD for local config", cwdKey, wd)

	// Skip parent search if CWD has local Atmos config.
	// This ensures --chdir to a directory with its own config uses only that config.
	if hasLocalAtmosConfig(wd) {
		log.Debug("Skipping parent directory search (local config exists)", "path", wd)
		return ""
	}
	log.Debug("No local config found, will search parent directories", cwdKey, wd)

	// No config found in any parent directory yields "".
	return findAtmosConfigInParentDirs(wd)
}

// gitRootConfigSearchDir returns the git repository root to search for atmos.yaml, or "" when
// the git-root fallback does not apply: disabled via ATMOS_GIT_ROOT_BASEPATH=false, overridden by
// ATMOS_CLI_CONFIG_PATH, CWD has local Atmos config, no repository, or the root is the CWD.
//
// The wd argument anchors the search; "" means the process working directory.
func gitRootConfigSearchDir(wd string) (string, error) {
	// Check if git root discovery is disabled.
	if projectroot.GitRootDiscoveryDisabled() {
		return "", nil
	}

	// If ATMOS_CLI_CONFIG_PATH is set, skip git root discovery.
	// The env var is an explicit override.
	if envConfigPath() != "" {
		return "", nil
	}

	// Skip git root search if CWD has local Atmos config.
	// This ensures --chdir to a directory with its own config uses only that config.
	cwd, err := workingDirOrProcess(wd)
	if err != nil {
		log.Debug("Failed to get CWD for local config check", "error", err)
	} else {
		log.Debug("readGitRootConfig checking CWD for local config", cwdKey, cwd)
		if hasLocalAtmosConfig(cwd) {
			log.Debug("Skipping git root search (local config exists)", "path", cwd)
			return "", nil
		}
		log.Debug("No local config found, will search git root", cwdKey, cwd)
	}

	gitRoot, err := processGitRoot(wd, "!repo-root")
	if err != nil {
		log.Trace("Git root detection failed", "error", err)
		return "", nil
	}

	// Skip if git root is empty or same as CWD (already handled by readWorkDirConfigOnly).
	if gitRoot == "" || gitRoot == "." {
		return "", nil
	}

	// Convert relative path to absolute.
	if !filepath.IsAbs(gitRoot) {
		cwd, err := workingDirOrProcess(wd)
		if err != nil {
			return "", err
		}
		gitRoot = filepath.Join(cwd, gitRoot)
	}
	return gitRoot, nil
}

// defaultConfigGitRootDir returns the git root whose `atmos.d` is loaded when no Atmos config file
// exists at all (the loader then runs on built-in defaults), or "" when there is none.
// The wd argument anchors the lookup; "" means the process working directory.
func defaultConfigGitRootDir(wd string) string {
	gitRoot, err := processGitRoot(wd, "!repo-root .")
	if err != nil || gitRoot == "" || gitRoot == "." {
		return ""
	}
	return gitRoot
}

// atmosDDirPath returns the path of the named default-imports directory (`atmos.d` or `.atmos.d`) under dirPath.
func atmosDDirPath(dirPath, name string) string {
	return filepath.Join(filepath.FromSlash(dirPath), name)
}

// atmosDGitRootDir returns the absolute git root whose `atmos.d`/`.atmos.d` is loaded in addition
// to the one under dirPath, or "" when it does not apply (no git root, same directory as dirPath,
// or excluded for testing). The wd argument anchors the git-root lookup; "" means the process working directory.
func atmosDGitRootDir(dirPath, wd string) string {
	gitRoot, err := processGitRoot(wd, "!repo-root .")
	if err != nil || gitRoot == "" || gitRoot == "." {
		// Not being able to determine a git root is not a user-facing error: it just
		// means the opportunistic git-root .atmos.d check is skipped.
		return ""
	}

	absGitRoot, absErr := filepath.Abs(gitRoot)
	absDirPath, dirErr := filepath.Abs(dirPath)
	if absErr != nil || dirErr != nil {
		// Same as above: path resolution failure only skips the optional check.
		return ""
	}

	// Check if git root is the same as config directory.
	// Use case-insensitive comparison on Windows where paths may differ only in casing.
	pathsEqual := absGitRoot == absDirPath
	if runtime.GOOS == "windows" {
		pathsEqual = strings.EqualFold(absGitRoot, absDirPath)
	}
	if pathsEqual {
		return ""
	}

	// Skip if excluded for testing.
	if shouldExcludePathForTesting(absGitRoot) {
		return ""
	}
	return absGitRoot
}
