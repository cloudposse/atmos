package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/pkg/errors"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

// InitCliConfig finds and merges CLI configurations in the following order: system dir, home dir, current dir, ENV vars, command-line arguments
// https://dev.to/techschoolguru/load-config-from-file-environment-variables-in-golang-with-viper-2j2d
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
//
// NOTE: Global flags (like --profile) must be synced to Viper before calling this function.
// This is done by syncGlobalFlagsToViper() in cmd/root.go PersistentPreRun.
//
// TODO: Change configAndStacksInfo to pointer.
// Temporarily suppressing gocritic warnings; refactoring InitCliConfig would require extensive changes.
//
//nolint:gocritic
func InitCliConfig(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
	atmosConfig, err := processAtmosConfigs(&configAndStacksInfo)
	if err != nil {
		return atmosConfig, err
	}
	// Process the base path specified in the Terraform provider (which calls into the atmos code)
	// This overrides all other atmos base path configs (`atmos.yaml`, ENV var `ATMOS_BASE_PATH`)
	if configAndStacksInfo.AtmosBasePath != "" {
		atmosConfig.BasePath = configAndStacksInfo.AtmosBasePath
	}

	// After unmarshalling, ensure AppendUserAgent is set if still empty
	if atmosConfig.Components.Terraform.AppendUserAgent == "" {
		atmosConfig.Components.Terraform.AppendUserAgent = fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version)
	}

	// Check config
	err = checkConfig(atmosConfig, processStacks)
	if err != nil {
		return atmosConfig, err
	}

	err = AtmosConfigAbsolutePaths(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}

	// Set log config BEFORE processing stacks so pre-hooks (including auth) see the correct log level.
	setLogConfig(&atmosConfig)

	if processStacks {
		err = processStackConfigs(&atmosConfig, &configAndStacksInfo, atmosConfig.IncludeStackAbsolutePaths, atmosConfig.ExcludeStackAbsolutePaths)
		if err != nil {
			return atmosConfig, err
		}
	}

	atmosConfig.Initialized = true
	return atmosConfig, nil
}

func setLogConfig(atmosConfig *schema.AtmosConfiguration) {
	// TODO: This is a quick patch to mitigate the issue we can look for better code later
	// Issue: https://linear.app/cloudposse/issue/DEV-3093/create-a-cli-command-core-library
	if os.Getenv("ATMOS_LOGS_LEVEL") != "" {
		atmosConfig.Logs.Level = os.Getenv("ATMOS_LOGS_LEVEL")
	}
	flagKeyValue := parseFlags()
	if v, ok := flagKeyValue["logs-level"]; ok {
		atmosConfig.Logs.Level = v
	}
	if os.Getenv("ATMOS_LOGS_FILE") != "" {
		atmosConfig.Logs.File = os.Getenv("ATMOS_LOGS_FILE")
	}
	if v, ok := flagKeyValue["logs-file"]; ok {
		atmosConfig.Logs.File = v
	}
	if val, ok := flagKeyValue["no-color"]; ok {
		valLower := strings.ToLower(val)
		switch valLower {
		case "true":
			atmosConfig.Settings.Terminal.NoColor = true
			atmosConfig.Settings.Terminal.Color = false
		case "false":
			atmosConfig.Settings.Terminal.NoColor = false
			atmosConfig.Settings.Terminal.Color = true
		}
		// If value is neither "true" nor "false", leave defaults unchanged
	}

	// Handle --pager global flag
	if v, ok := flagKeyValue["pager"]; ok {
		atmosConfig.Settings.Terminal.Pager = v
	}

	// Handle NO_PAGER environment variable (standard CLI convention)
	// Check this after --pager flag so CLI flag takes precedence
	//nolint:forbidigo // NO_PAGER is a standard CLI convention that requires direct env access.
	// We intentionally don't use viper.BindEnv() here because:
	// 1. NO_PAGER uses negative logic (NO_PAGER=true disables pager)
	// 2. Atmos config convention uses positive boolean names (pager: true enables pager)
	// 3. We don't want a configurable "no_pager" field that would confuse the config schema
	// 4. NO_PAGER should remain an environment-only standard, not a config file setting
	if os.Getenv("NO_PAGER") != "" {
		// Check if --pager flag was explicitly provided
		if _, hasPagerFlag := flagKeyValue["pager"]; !hasPagerFlag {
			// NO_PAGER is set, and no explicit --pager flag was provided, disable the pager
			atmosConfig.Settings.Terminal.Pager = "false"
		}
	}

	// Configure the global logger with the log level from flags/env/config.
	// This ensures auth pre-hooks (executed during processStackConfigs) respect the log level.
	// Parse and convert log level using existing utilities for consistency.
	logLevel, err := log.ParseLogLevel(atmosConfig.Logs.Level)
	if err != nil {
		// Default to Warning on parse error.
		logLevel = log.LogLevelWarning
	}
	log.SetLevel(log.ConvertLogLevel(logLevel))
}

// TODO: This function works well, but we should generally avoid implementing manual flag parsing,
// as Cobra typically handles this.

// If there's no alternative, this approach may be necessary.
// However, this TODO serves as a reminder to revisit and verify if a better solution exists.

// Function to manually parse flags with double dash "--" like Cobra.
func parseFlags() map[string]string {
	return parseFlagsFromArgs(os.Args)
}

// parseFlagsFromArgs parses flags from the given args slice.
// This function is exposed for testing purposes.
func parseFlagsFromArgs(args []string) map[string]string {
	flags := make(map[string]string)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		// Check if the argument starts with '--' (double dash)
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		// Strip the '--' prefix and check if it's followed by a value
		arg = arg[2:]
		switch {
		case strings.Contains(arg, "="):
			// Case like --flag=value
			parts := strings.SplitN(arg, "=", 2)
			flags[parts[0]] = parts[1]
		case i+1 < len(args) && !strings.HasPrefix(args[i+1], "--"):
			// Case like --flag value
			flags[arg] = args[i+1]
			i++ // Skip the next argument as it's the value
		default:
			// Case where flag has no value, e.g., --flag (we set it to "true")
			flags[arg] = "true"
		}
	}
	return flags
}

func processAtmosConfigs(configAndStacksInfo *schema.ConfigAndStacksInfo) (schema.AtmosConfiguration, error) {
	atmosConfig, err := LoadConfig(configAndStacksInfo)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.ProcessSchemas()

	// Process ENV vars
	err = processEnvVars(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}

	// Process command-line args
	err = processCommandLineArgs(&atmosConfig, configAndStacksInfo)
	if err != nil {
		return atmosConfig, err
	}

	// Process stores config
	err = processStoreConfig(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}
	return atmosConfig, nil
}

// atmosConfigAbsolutePaths converts paths to absolute paths.
// AtmosConfigAbsolutePaths converts all base paths in the configuration to absolute paths.
// This function sets TerraformDirAbsolutePath, HelmfileDirAbsolutePath, PackerDirAbsolutePath,
// StacksBaseAbsolutePath, IncludeStackAbsolutePaths, and ExcludeStackAbsolutePaths.
// It converts a path to absolute form, resolving relative paths according to the semantics below.
//
// Resolution semantics (see docs/prd/base-path-resolution-semantics.md):
//
//  1. Absolute paths → return as-is
//  2. Explicit relative paths → resolve relative to cliConfigPath (config-file-relative):
//     - Exactly "." or ".."
//     - Starts with "./" or "../" (Unix)
//     - Starts with ".\" or "..\" (Windows)
//  3. "" (empty) or simple paths like "foo" → try git root, fallback to cliConfigPath
//
// Fallback order when primary resolution fails:
//  1. Git repository root
//  2. Config directory (cliConfigPath / dirname(atmos.yaml))
//  3. CWD (last resort)
//
// Key semantic distinctions:
//   - "." means dirname(atmos.yaml) (config-file-relative)
//   - "" means git repo root with fallback to dirname(atmos.yaml) (smart default)
//   - "./foo" means dirname(atmos.yaml)/foo (config-file-relative)
//   - "foo" means git-root/foo with fallback to dirname(atmos.yaml)/foo (search path)
//   - ".." or "../foo" means dirname(atmos.yaml)/../foo (config-file-relative navigation)
//
// This follows the convention of tsconfig.json, package.json, .eslintrc - paths in
// config files are relative to the config file location, not where you run from.
// Use the !cwd YAML tag if you need paths relative to CWD.
func resolveAbsolutePath(path string, cliConfigPath string) (string, error) {
	// If already absolute, return as-is.
	if filepath.IsAbs(path) {
		return path, nil
	}

	sep := string(filepath.Separator)

	// Check for explicit relative paths: ".", "./...", "..", or "../..."
	// These resolve relative to atmos.yaml location (config-file-relative).
	// This follows the convention of tsconfig.json, package.json, .eslintrc.
	isExplicitRelative := path == "." ||
		path == ".." ||
		strings.HasPrefix(path, "./") ||
		strings.HasPrefix(path, "."+sep) ||
		strings.HasPrefix(path, "../") ||
		strings.HasPrefix(path, ".."+sep)

	// For explicit relative paths (".", "./...", "..", "../..."):
	// Resolve relative to config directory (cliConfigPath).
	if isExplicitRelative && cliConfigPath != "" {
		basePath := filepath.Join(cliConfigPath, path)
		absPath, err := filepath.Abs(basePath)
		if err != nil {
			return "", fmt.Errorf("resolving path %q relative to config %q: %w", path, cliConfigPath, err)
		}
		return absPath, nil
	}

	// For empty path or simple relative paths (like "stacks", "components/terraform"):
	// Try git root first.
	return tryResolveWithGitRoot(path, isExplicitRelative, cliConfigPath)
}

// tryResolveWithGitRoot attempts to resolve a path using git root as the base.
// If git root is unavailable, falls back to cliConfigPath, then CWD.
func tryResolveWithGitRoot(path string, isExplicitRelative bool, cliConfigPath string) (string, error) {
	gitRoot := getGitRootOrEmpty()
	if gitRoot == "" {
		return tryResolveWithConfigPath(path, cliConfigPath)
	}

	// Git root available - resolve relative to it.
	if path == "" {
		return gitRoot, nil
	}

	// For explicit relative paths without cliConfigPath, resolve relative to git root.
	if isExplicitRelative {
		basePath := filepath.Join(gitRoot, path)
		absPath, err := filepath.Abs(basePath)
		if err != nil {
			return "", fmt.Errorf("resolving path %q relative to git root %q: %w", path, gitRoot, err)
		}
		return absPath, nil
	}

	return filepath.Join(gitRoot, path), nil
}

// tryResolveWithConfigPath resolves a path using cliConfigPath as the base,
// falling back to CWD if cliConfigPath is unavailable.
func tryResolveWithConfigPath(path string, cliConfigPath string) (string, error) {
	// Fallback: resolve relative to atmos.yaml dir (cliConfigPath).
	if cliConfigPath != "" {
		if path == "" {
			absPath, err := filepath.Abs(cliConfigPath)
			if err != nil {
				return "", fmt.Errorf("resolving config path %q: %w", cliConfigPath, err)
			}
			return absPath, nil
		}
		basePath := filepath.Join(cliConfigPath, path)
		absPath, err := filepath.Abs(basePath)
		if err != nil {
			return "", fmt.Errorf("resolving path %q relative to config %q: %w", path, cliConfigPath, err)
		}
		return absPath, nil
	}

	// Last resort (3rd fallback): resolve relative to CWD.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving path %q: %w", path, err)
	}
	return absPath, nil
}

// getGitRootOrEmpty returns the git repository root path, or empty string if not in a git repo.
// This is used for base path resolution to anchor simple relative paths to the repo root.
func getGitRootOrEmpty() string {
	// Check if git root discovery is disabled.
	//nolint:forbidigo // ATMOS_GIT_ROOT_BASEPATH is bootstrap config, not application configuration.
	if os.Getenv("ATMOS_GIT_ROOT_BASEPATH") == "false" {
		return ""
	}

	gitRoot, err := u.ProcessTagGitRoot("!repo-root")
	if err != nil {
		log.Trace("Git root detection failed", "error", err)
		return ""
	}

	// ProcessTagGitRoot returns "." when called with just "!repo-root" and no default.
	// We need to convert it to an absolute path.
	if gitRoot == "" || gitRoot == "." {
		// Get absolute path of current directory as fallback.
		cwd, err := os.Getwd()
		if err != nil {
			return ""
		}
		// Check if we're at git root by looking for .git.
		if _, err := os.Stat(filepath.Join(cwd, ".git")); err == nil {
			return cwd
		}
		return ""
	}

	return gitRoot
}

func AtmosConfigAbsolutePaths(atmosConfig *schema.AtmosConfiguration) error {
	// First, resolve the base path itself to an absolute path.
	// Relative paths are resolved relative to atmos.yaml location (atmosConfig.CliConfigPath).
	var atmosBasePathAbs string
	var err error
	atmosBasePathAbs, err = resolveAbsolutePath(atmosConfig.BasePath, atmosConfig.CliConfigPath)
	if err != nil {
		return err
	}

	// Clean up any path duplication that might occur from incorrect configuration or symlink resolution.
	atmosBasePathAbs = u.CleanDuplicatedPath(atmosBasePathAbs)

	// Store the absolute base path in BasePathAbsolute field.
	// This allows other code (like schema validation) to use the absolute path while
	// preserving the original BasePath value (which may be relative) for display/serialization.
	atmosConfig.BasePathAbsolute = atmosBasePathAbs

	// Convert stacks base path to an absolute path.
	// Now we join the absolute base path with the stacks base path.
	stacksBasePath := u.JoinPath(atmosBasePathAbs, atmosConfig.Stacks.BasePath)
	stacksBaseAbsPath, err := filepath.Abs(stacksBasePath)
	if err != nil {
		return err
	}
	// Clean up any path duplication that might occur from incorrect configuration or symlink resolution.
	stacksBaseAbsPath = u.CleanDuplicatedPath(stacksBaseAbsPath)
	atmosConfig.StacksBaseAbsolutePath = stacksBaseAbsPath

	// Convert the included stack paths to absolute paths
	includeStackAbsPaths, err := u.JoinPaths(stacksBaseAbsPath, atmosConfig.Stacks.IncludedPaths)
	if err != nil {
		return err
	}
	atmosConfig.IncludeStackAbsolutePaths = includeStackAbsPaths

	// Convert the excluded stack paths to absolute paths
	excludeStackAbsPaths, err := u.JoinPaths(stacksBaseAbsPath, atmosConfig.Stacks.ExcludedPaths)
	if err != nil {
		return err
	}
	atmosConfig.ExcludeStackAbsolutePaths = excludeStackAbsPaths

	// Convert Terraform dir to an absolute path.
	terraformBasePath := u.JoinPath(atmosBasePathAbs, atmosConfig.Components.Terraform.BasePath)
	terraformDirAbsPath, err := filepath.Abs(terraformBasePath)
	if err != nil {
		return err
	}
	atmosConfig.TerraformDirAbsolutePath = terraformDirAbsPath

	// Convert Helmfile dir to an absolute path.
	helmfileBasePath := u.JoinPath(atmosBasePathAbs, atmosConfig.Components.Helmfile.BasePath)
	helmfileDirAbsPath, err := filepath.Abs(helmfileBasePath)
	if err != nil {
		return err
	}
	atmosConfig.HelmfileDirAbsolutePath = helmfileDirAbsPath

	// Convert Packer dir to an absolute path.
	packerBasePath := u.JoinPath(atmosBasePathAbs, atmosConfig.Components.Packer.BasePath)
	packerDirAbsPath, err := filepath.Abs(packerBasePath)
	if err != nil {
		return err
	}
	atmosConfig.PackerDirAbsolutePath = packerDirAbsPath

	return nil
}

func processStackConfigs(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo, includeStackAbsPaths, excludeStackAbsPaths []string) error {
	// If the specified stack name is a logical name, find all stack manifests in the provided paths
	stackConfigFilesAbsolutePaths, stackConfigFilesRelativePaths, stackIsPhysicalPath, err := FindAllStackConfigsInPathsForStack(
		*atmosConfig,
		configAndStacksInfo.Stack,
		includeStackAbsPaths,
		excludeStackAbsPaths,
	)
	if err != nil {
		return err
	}

	if len(stackConfigFilesAbsolutePaths) < 1 {
		j, err := u.ConvertToYAML(includeStackAbsPaths)
		if err != nil {
			return err
		}
		errorMessage := fmt.Sprintf("\nno stack manifests found in the provided "+
			"paths:\n%s\n\nCheck if `base_path`, 'stacks.base_path', 'stacks.included_paths' and 'stacks.excluded_paths' are correctly set in CLI config "+
			"files or ENV vars.", j)
		return errors.New(errorMessage)
	}

	atmosConfig.StackConfigFilesAbsolutePaths = stackConfigFilesAbsolutePaths
	atmosConfig.StackConfigFilesRelativePaths = stackConfigFilesRelativePaths

	if stackIsPhysicalPath {
		log.Debug("The stack matches the stack manifest",
			"stack", configAndStacksInfo.Stack,
			"manifest", stackConfigFilesRelativePaths[0])
		atmosConfig.StackType = "Directory"
	} else {
		// The stack is a logical name
		atmosConfig.StackType = "Logical"
	}

	return nil
}
