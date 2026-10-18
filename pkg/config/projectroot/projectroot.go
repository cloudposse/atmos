// Package projectroot selects the directory that supplies project-level files
// (atmos.yaml, policy.d) for a working directory.
//
// It is a leaf package so both configuration loading (pkg/config) and policy
// discovery (pkg/policy), which pkg/config depends on, share one definition of
// the project root. It never reads or parses any project file.
package projectroot

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/flags/osargs"
	"github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// AtmosConfigFileName is the main config file name.
	AtmosConfigFileName = "atmos.yaml"
	// DotAtmosConfigFileName is the hidden config file name.
	DotAtmosConfigFileName = ".atmos.yaml"

	// AtmosConfigDirName is the config directory name.
	AtmosConfigDirName = ".atmos"
	// AtmosDefaultImportsDirName is the default imports directory name.
	AtmosDefaultImportsDirName = "atmos.d"
	// DotAtmosDefaultImportsDirName is the hidden default imports directory name.
	DotAtmosDefaultImportsDirName = ".atmos.d"

	// CliConfigPathEnvVar is the environment variable naming an explicit config location.
	CliConfigPathEnvVar = "ATMOS_CLI_CONFIG_PATH"
	// GitRootBasePathEnvVar disables Git root discovery when set to "false".
	GitRootBasePathEnvVar = "ATMOS_GIT_ROOT_BASEPATH"

	// The TEST_GIT_ROOT variable overrides the Git root for test isolation.
	testGitRootEnvVar = "TEST_GIT_ROOT"
)

// gitRootResolver resolves the Git repository root. Overridden in tests.
var gitRootResolver = defaultGitRoot

// defaultGitRoot mirrors utils.ProcessTagGitRoot, which pkg/policy cannot import.
func defaultGitRoot() (string, error) {
	//nolint:forbidigo // TEST_GIT_ROOT is specifically for test isolation, not application configuration.
	if root := os.Getenv(testGitRootEnvVar); root != "" {
		return root, nil
	}
	return git.ProcessTagRoot(git.YAMLFuncRepoRoot)
}

// HasLocalConfig reports whether dir has any Atmos configuration.
//
// Returns true if ANY of these exist in the given directory:
// - atmos.yaml (main config file)
// - .atmos.yaml (hidden config file)
// - .atmos/ (config directory)
// - .atmos.d/ (default imports directory)
// - atmos.d/ (default imports directory - alternate).
func HasLocalConfig(dir string) bool {
	defer perf.Track(nil, "projectroot.HasLocalConfig")()

	configIndicators := []string{
		AtmosConfigFileName,
		DotAtmosConfigFileName,
		AtmosConfigDirName,
		DotAtmosDefaultImportsDirName,
		AtmosDefaultImportsDirName,
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

// GitRootDiscoveryDisabled reports whether ATMOS_GIT_ROOT_BASEPATH=false.
func GitRootDiscoveryDisabled() bool {
	defer perf.Track(nil, "projectroot.GitRootDiscoveryDisabled")()

	//nolint:forbidigo // ATMOS_GIT_ROOT_BASEPATH is bootstrap config, not application configuration.
	return os.Getenv(GitRootBasePathEnvVar) == "false"
}

// FindConfigInParentDirs searches for atmos.yaml in parent directories.
// It walks up the directory tree from the given starting directory until
// it finds an atmos.yaml file or reaches the filesystem root.
// Returns the directory containing atmos.yaml, or empty string if not found.
func FindConfigInParentDirs(startDir string) string {
	defer perf.Track(nil, "projectroot.FindConfigInParentDirs")()

	dir := startDir
	for {
		// Move to parent directory.
		parent := filepath.Dir(dir)

		// Check if we've reached the root.
		if parent == dir {
			return ""
		}

		dir = parent
		log.Trace("Checking for atmos.yaml in parent directory", "path", dir)

		// Check for atmos.yaml or .atmos.yaml in this directory.
		for _, configName := range []string{AtmosConfigFileName, DotAtmosConfigFileName} {
			if _, err := os.Stat(filepath.Join(dir, configName)); err == nil {
				return dir
			}
		}
	}
}

// SearchDirs returns the directories that supply project-level files for cwd,
// highest precedence first, mirroring the effective atmos.yaml discovery:
//
//  1. Directories of --config / --config-path (flags in args, then ATMOS_CONFIG /
//     ATMOS_CONFIG_PATH). When any is selected, discovery stops there.
//  2. The directory of ATMOS_CLI_CONFIG_PATH, when set (a file path resolves to its directory).
//  3. cwd.
//  4. Unless cwd has local Atmos configuration, or step 1 or 2 applied: the Git
//     repository root (skipped with ATMOS_GIT_ROOT_BASEPATH=false), else the
//     nearest ancestor holding atmos.yaml.
//
// cwd is included even when config selection applies, so a policy.d next to the
// working directory is never silently dropped. Callers use the first directory
// that has the file they need.
func SearchDirs(cwd string, args []string) []string {
	defer perf.Track(nil, "projectroot.SearchDirs")()

	var dirs []string
	add := func(dir string) {
		for _, existing := range dirs {
			if existing == dir {
				return
			}
		}
		dirs = append(dirs, dir)
	}

	selection := ParseConfigSelectionFromOsArgs(args)
	selection.ApplyFallbacks(ConfigSelectionFromEnv())
	for _, file := range selection.Config {
		add(filepath.Dir(absolute(cwd, file)))
	}
	for _, path := range selection.ConfigPath {
		add(pathDirectory(cwd, path))
	}
	if len(dirs) > 0 {
		add(cwd)
		return dirs
	}

	//nolint:forbidigo // ATMOS_CLI_CONFIG_PATH controls config loading behavior itself.
	if configPath := os.Getenv(CliConfigPathEnvVar); configPath != "" {
		add(pathDirectory(cwd, configPath))
		add(cwd)
		return dirs
	}

	add(cwd)
	if HasLocalConfig(cwd) {
		return dirs
	}
	if root := gitRoot(cwd); root != "" {
		add(root)
	} else if parent := FindConfigInParentDirs(cwd); parent != "" {
		add(parent)
	}
	return dirs
}

// gitRoot returns the Git repository root for cwd, or "" when discovery is
// disabled or cwd is not inside a repository.
func gitRoot(cwd string) string {
	if GitRootDiscoveryDisabled() {
		return ""
	}
	root, err := gitRootResolver()
	if err != nil {
		log.Trace("Git root detection failed", "error", err)
		return ""
	}
	if root == "" || root == "." {
		return ""
	}
	return absolute(cwd, root)
}

func absolute(cwd, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cwd, path)
}

// pathDirectory resolves a config location to the directory that holds the
// config. It normally names a directory; a path to an existing file resolves to
// the file's directory.
func pathDirectory(cwd, path string) string {
	path = absolute(cwd, path)
	//nolint:gosec // Config locations come from flags and environment by design; discovery only stats them.
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return filepath.Dir(path)
	}
	return path
}

const commaDelimiter = ","

// ConfigSelection holds the config-selection flags parsed early from os.Args
// before Cobra is initialized (same pattern as profile early-parsing).
type ConfigSelection struct {
	BasePath   string
	Config     []string
	ConfigPath []string
}

// ParseConfigSelectionFromOsArgs parses --base-path, --config, and --config-path
// flags from os.Args. This is used for early extraction before Cobra parses
// flags, so that InitCliConfig receives the correct config location.
func ParseConfigSelectionFromOsArgs(args []string) ConfigSelection {
	defer perf.Track(nil, "projectroot.ParseConfigSelectionFromOsArgs")()

	var sel ConfigSelection

	if basePath := strings.TrimSpace(osargs.ParseString(args, "base-path")); basePath != "" {
		sel.BasePath = basePath
	}

	for _, v := range osargs.ParseStringSlice(args, "config") {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			sel.Config = append(sel.Config, trimmed)
		}
	}

	for _, v := range osargs.ParseStringSlice(args, "config-path") {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			sel.ConfigPath = append(sel.ConfigPath, trimmed)
		}
	}

	return sel
}

// ConfigSelectionFromEnv reads config-selection values from environment variables
// as fallbacks when not provided via CLI flags.
func ConfigSelectionFromEnv() ConfigSelection {
	defer perf.Track(nil, "projectroot.ConfigSelectionFromEnv")()

	var sel ConfigSelection

	//nolint:forbidigo // Must use os.Getenv: config selection is processed before Viper configuration loads.
	if v := os.Getenv("ATMOS_BASE_PATH"); v != "" {
		sel.BasePath = strings.TrimSpace(v)
	}

	//nolint:forbidigo // Must use os.Getenv: config selection is processed before Viper configuration loads.
	sel.Config = splitList(os.Getenv("ATMOS_CONFIG"))

	//nolint:forbidigo // Must use os.Getenv: config selection is processed before Viper configuration loads.
	sel.ConfigPath = splitList(os.Getenv("ATMOS_CONFIG_PATH"))

	return sel
}

func splitList(value string) []string {
	var result []string
	for _, part := range strings.Split(value, commaDelimiter) {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ApplyFallbacks fills each unset field from fallback, so flags win over env.
func (s *ConfigSelection) ApplyFallbacks(fallback ConfigSelection) {
	if s.BasePath == "" {
		s.BasePath = fallback.BasePath
	}
	if len(s.Config) == 0 {
		s.Config = fallback.Config
	}
	if len(s.ConfigPath) == 0 {
		s.ConfigPath = fallback.ConfigPath
	}
}
