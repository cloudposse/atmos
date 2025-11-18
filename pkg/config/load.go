package config

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
)

//go:embed atmos.yaml
var embeddedConfigData []byte

const (
	// MaximumImportLvL defines the maximum import level allowed.
	MaximumImportLvL = 10
	// CommandsKey is the configuration key for commands.
	commandsKey = "commands"
	// YamlType is the configuration file type.
	yamlType = "yaml"
)

var defaultHomeDirProvider = filesystem.NewOSHomeDirProvider()

// * Embedded atmos.yaml (`atmos/pkg/config/atmos.yaml`)
// * System dir (`/usr/local/etc/atmos` on Linux, `%LOCALAPPDATA%/atmos` on Windows).
// * Home directory (~/.atmos).
// * Current working directory.
// * ENV vars.
// * Command-line arguments.
func LoadConfig(configAndStacksInfo *schema.ConfigAndStacksInfo) (schema.AtmosConfiguration, error) {
	v := viper.New()
	var atmosConfig schema.AtmosConfiguration
	v.SetConfigType("yaml")
	v.SetTypeByDefaultValue(true)
	setDefaultConfiguration(v)
	// Load embed atmos.yaml
	if err := loadEmbeddedConfig(v); err != nil {
		return atmosConfig, err
	}
	if len(configAndStacksInfo.AtmosConfigFilesFromArg) > 0 || len(configAndStacksInfo.AtmosConfigDirsFromArg) > 0 {
		err := loadConfigFromCLIArgs(v, configAndStacksInfo, &atmosConfig)
		if err != nil {
			return atmosConfig, err
		}
		return atmosConfig, nil
	}

	// Load configuration from different sources.
	if err := loadConfigSources(v, configAndStacksInfo); err != nil {
		return atmosConfig, err
	}
	// If no config file is used, fall back to the default CLI config.
	if v.ConfigFileUsed() == "" {
		log.Debug("'atmos.yaml' CLI config was not found", "paths", "system dir, home dir, current dir, ENV vars")
		log.Debug("Refer to https://atmos.tools/cli/configuration for details on how to configure 'atmos.yaml'")
		log.Debug("Using the default CLI config")

		if err := mergeDefaultConfig(v); err != nil {
			return atmosConfig, err
		}
	}
	if v.ConfigFileUsed() != "" {
		// get dir of atmosConfigFilePath
		atmosConfigDir := filepath.Dir(v.ConfigFileUsed())
		atmosConfig.CliConfigPath = atmosConfigDir
		// Set the CLI config path in the atmosConfig struct
		if !filepath.IsAbs(atmosConfig.CliConfigPath) {
			absPath, err := filepath.Abs(atmosConfig.CliConfigPath)
			if err != nil {
				return atmosConfig, err
			}
			atmosConfig.CliConfigPath = absPath
		}
	}
	setEnv(v)

	// Load profiles if specified via --profile flag or ATMOS_PROFILE env var.
	// Profiles are loaded after base config but before final unmarshaling.
	// This allows profiles to override base config settings.
	if len(configAndStacksInfo.ProfilesFromArg) > 0 {
		// First, do a temporary unmarshal to get CliConfigPath and Profiles config.
		// We need these to discover and load profile directories.
		var tempConfig schema.AtmosConfiguration
		if err := v.Unmarshal(&tempConfig); err != nil {
			return atmosConfig, err
		}

		// Copy the already-computed CLI config directory into tempConfig.
		// This ensures relative profile paths resolve against the actual CLI config directory
		// rather than the current working directory.
		tempConfig.CliConfigPath = atmosConfig.CliConfigPath

		// Load each profile in order (left-to-right precedence).
		if err := loadProfiles(v, configAndStacksInfo.ProfilesFromArg, &tempConfig); err != nil {
			return atmosConfig, err
		}

		log.Debug("Profiles loaded successfully",
			"profiles", configAndStacksInfo.ProfilesFromArg,
			"count", len(configAndStacksInfo.ProfilesFromArg))
	}

	// https://gist.github.com/chazcheadle/45bf85b793dea2b71bd05ebaa3c28644
	// https://sagikazarmark.hu/blog/decoding-custom-formats-with-viper/
	err := v.Unmarshal(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}

	// Post-process to preserve case-sensitive identity names.
	// Viper lowercases all map keys, but we need to preserve original case for identity names.
	if err := preserveIdentityCase(v, &atmosConfig); err != nil {
		log.Debug("Failed to preserve identity case", "error", err)
		// Don't fail config loading if this step fails, just log it.
	}

	// Apply git root discovery for default base path.
	// This enables running Atmos from any subdirectory, similar to Git.
	if err := applyGitRootBasePath(&atmosConfig); err != nil {
		log.Debug("Failed to apply git root base path", "error", err)
		// Don't fail config loading if this step fails, just log it.
	}

	return atmosConfig, nil
}

func setEnv(v *viper.Viper) {
	bindEnv(v, "settings.github_token", "GITHUB_TOKEN")
	bindEnv(v, "settings.inject_github_token", "ATMOS_INJECT_GITHUB_TOKEN")
	bindEnv(v, "settings.atmos_github_token", "ATMOS_GITHUB_TOKEN")
	bindEnv(v, "settings.github_username", "ATMOS_GITHUB_USERNAME", "GITHUB_ACTOR", "GITHUB_USERNAME")

	bindEnv(v, "settings.bitbucket_token", "BITBUCKET_TOKEN")
	bindEnv(v, "settings.atmos_bitbucket_token", "ATMOS_BITBUCKET_TOKEN")
	bindEnv(v, "settings.inject_bitbucket_token", "ATMOS_INJECT_BITBUCKET_TOKEN")
	bindEnv(v, "settings.bitbucket_username", "BITBUCKET_USERNAME")

	bindEnv(v, "settings.gitlab_token", "GITLAB_TOKEN")
	bindEnv(v, "settings.inject_gitlab_token", "ATMOS_INJECT_GITLAB_TOKEN")
	bindEnv(v, "settings.atmos_gitlab_token", "ATMOS_GITLAB_TOKEN")

	bindEnv(v, "settings.terminal.pager", "ATMOS_PAGER", "PAGER")
	bindEnv(v, "settings.terminal.color", "ATMOS_COLOR", "COLOR")
	bindEnv(v, "settings.terminal.no_color", "ATMOS_NO_COLOR", "NO_COLOR")
	bindEnv(v, "settings.terminal.theme", "ATMOS_THEME", "THEME")

	// Atmos Pro settings
	bindEnv(v, "settings.pro.base_url", AtmosProBaseUrlEnvVarName)
	bindEnv(v, "settings.pro.endpoint", AtmosProEndpointEnvVarName)
	bindEnv(v, "settings.pro.token", AtmosProTokenEnvVarName)
	bindEnv(v, "settings.pro.workspace_id", AtmosProWorkspaceIDEnvVarName)
	bindEnv(v, "settings.pro.github_run_id", "GITHUB_RUN_ID")
	bindEnv(v, "settings.pro.atmos_pro_run_id", AtmosProRunIDEnvVarName)

	// GitHub OIDC for Atmos Pro
	bindEnv(v, "settings.pro.github_oidc.request_url", "ACTIONS_ID_TOKEN_REQUEST_URL")
	bindEnv(v, "settings.pro.github_oidc.request_token", "ACTIONS_ID_TOKEN_REQUEST_TOKEN")

	// Telemetry settings
	bindEnv(v, "settings.telemetry.enabled", "ATMOS_TELEMETRY_ENABLED")
	bindEnv(v, "settings.telemetry.token", "ATMOS_TELEMETRY_TOKEN")
	bindEnv(v, "settings.telemetry.endpoint", "ATMOS_TELEMETRY_ENDPOINT")
	bindEnv(v, "settings.telemetry.logging", "ATMOS_TELEMETRY_LOGGING")

	// Profiler settings
	bindEnv(v, "profiler.enabled", "ATMOS_PROFILER_ENABLED")
	bindEnv(v, "profiler.host", "ATMOS_PROFILER_HOST")
	bindEnv(v, "profiler.port", "ATMOS_PROFILER_PORT")
	bindEnv(v, "profiler.file", "ATMOS_PROFILE_FILE")
	bindEnv(v, "profiler.profile_type", "ATMOS_PROFILE_TYPE")
}

func bindEnv(v *viper.Viper, key ...string) {
	if err := v.BindEnv(key...); err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
}

// setDefaultConfiguration set default configuration for the viper instance.
func setDefaultConfiguration(v *viper.Viper) {
	v.SetDefault("components.helmfile.use_eks", true)
	v.SetDefault("components.terraform.append_user_agent",
		fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version))

	// Token injection defaults for all supported Git hosting providers.
	v.SetDefault("settings.inject_github_token", true)
	v.SetDefault("settings.inject_bitbucket_token", true)
	v.SetDefault("settings.inject_gitlab_token", true)

	v.SetDefault("logs.file", "/dev/stderr")
	v.SetDefault("logs.level", "Warning")

	v.SetDefault("settings.terminal.color", true)
	v.SetDefault("settings.terminal.no_color", false)
	v.SetDefault("settings.terminal.pager", false)
	v.SetDefault("docs.generate.readme.output", "./README.md")

	// Atmos Pro defaults
	v.SetDefault("settings.pro.base_url", AtmosProDefaultBaseUrl)
	v.SetDefault("settings.pro.endpoint", AtmosProDefaultEndpoint)
}

// loadConfigSources delegates reading configs from each source,
// returning early if any step in the chain fails.
func loadConfigSources(v *viper.Viper, configAndStacksInfo *schema.ConfigAndStacksInfo) error {
	if err := readSystemConfig(v); err != nil {
		return err
	}

	if err := readHomeConfig(v); err != nil {
		return err
	}

	if err := readWorkDirConfig(v); err != nil {
		return err
	}

	if err := readEnvAmosConfigPath(v); err != nil {
		return err
	}

	return readAtmosConfigCli(v, configAndStacksInfo.AtmosCliConfigPath)
}

// readSystemConfig load config from system dir.
func readSystemConfig(v *viper.Viper) error {
	configFilePath := ""
	if runtime.GOOS == "windows" {
		appDataDir := os.Getenv(WindowsAppDataEnvVar)
		if len(appDataDir) > 0 {
			configFilePath = appDataDir
		}
	} else {
		configFilePath = SystemDirConfigFilePath
	}

	if len(configFilePath) > 0 {
		err := mergeConfig(v, configFilePath, CliConfigFileName, false)
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
			return nil
		default:
			return err
		}
	}
	return nil
}

// readHomeConfig load config from user's HOME dir.
func readHomeConfig(v *viper.Viper) error {
	return readHomeConfigWithProvider(v, defaultHomeDirProvider)
}

// readHomeConfigWithProvider loads config from user's HOME dir using a HomeDirProvider.
func readHomeConfigWithProvider(v *viper.Viper, homeProvider filesystem.HomeDirProvider) error {
	home, err := homeProvider.Dir()
	if err != nil {
		return err
	}
	configFilePath := filepath.Join(home, ".atmos")
	err = mergeConfig(v, configFilePath, CliConfigFileName, true)
	if err != nil {
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
			return nil
		default:
			return err
		}
	}

	return nil
}

// readWorkDirConfig load config from current working directory.
func readWorkDirConfig(v *viper.Viper) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	err = mergeConfig(v, wd, CliConfigFileName, true)
	if err != nil {
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
			return nil
		default:
			return err
		}
	}
	return nil
}

func readEnvAmosConfigPath(v *viper.Viper) error {
	atmosPath := os.Getenv("ATMOS_CLI_CONFIG_PATH")
	if atmosPath == "" {
		return nil
	}
	err := mergeConfig(v, atmosPath, CliConfigFileName, true)
	if err != nil {
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
			log.Debug("config not found ENV var ATMOS_CLI_CONFIG_PATH", "file", atmosPath)
			return nil
		default:
			return err
		}
	}
	log.Debug("Found config ENV", "ATMOS_CLI_CONFIG_PATH", atmosPath)

	return nil
}

func readAtmosConfigCli(v *viper.Viper, atmosCliConfigPath string) error {
	if len(atmosCliConfigPath) == 0 {
		return nil
	}
	err := mergeConfig(v, atmosCliConfigPath, CliConfigFileName, true)
	switch err.(type) {
	case viper.ConfigFileNotFoundError:
		log.Debug("config not found", "file", atmosCliConfigPath)
	default:
		return err
	}

	return nil
}

// loadConfigFile reads a configuration file and returns a temporary Viper instance with its contents.
func loadConfigFile(path string, fileName string) (*viper.Viper, error) {
	tempViper := viper.New()
	tempViper.AddConfigPath(path)
	tempViper.SetConfigName(fileName)
	tempViper.SetConfigType(yamlType)

	if err := tempViper.ReadInConfig(); err != nil {
		// Return sentinel error unwrapped for type checking
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			return nil, err
		}
		// Wrap any other error with context
		return nil, errors.Join(errUtils.ErrReadConfig, fmt.Errorf("%s/%s: %w", path, fileName, err))
	}

	return tempViper, nil
}

// readConfigFileContent reads the content of a configuration file.
func readConfigFileContent(configFilePath string) ([]byte, error) {
	content, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, errors.Join(errUtils.ErrReadConfig, fmt.Errorf("%s: %w", configFilePath, err))
	}
	return content, nil
}

// processConfigImportsAndReapply processes imports and re-applies the original config for proper precedence.
func processConfigImportsAndReapply(path string, tempViper *viper.Viper, content []byte) error {
	// Parse the main config to get its commands separately.
	mainViper := viper.New()
	mainViper.SetConfigType(yamlType)
	if err := mainViper.ReadConfig(bytes.NewReader(content)); err != nil {
		return errors.Join(errUtils.ErrMergeConfiguration, fmt.Errorf("parse main config: %w", err))
	}
	mainCommands := mainViper.Get(commandsKey)

	// Process default imports (e.g., .atmos.d) first.
	// These don't need the main config to be loaded.
	if err := mergeDefaultImports(path, tempViper); err != nil {
		log.Debug("error process default imports", "path", path, "error", err)
	}
	defaultCommands := tempViper.Get(commandsKey)

	// Now load the main config temporarily to process explicit imports.
	// We need this because the import paths are defined in the main config.
	if err := tempViper.MergeConfig(bytes.NewReader(content)); err != nil {
		return errors.Join(errUtils.ErrMergeConfiguration, fmt.Errorf("merge main config: %w", err))
	}

	// Clear commands before processing imports to collect only imported commands.
	tempViper.Set(commandsKey, nil)

	// Process explicit imports.
	// This will read the import paths from the config and process them.
	if err := mergeImports(tempViper); err != nil {
		log.Debug("error process explicit imports", "file", tempViper.ConfigFileUsed(), "error", err)
	}

	// Get imported commands (without main commands).
	importedCommands := tempViper.Get(commandsKey)

	// Re-apply this config file's content after processing its imports.
	// This ensures proper precedence: each config file's own settings override
	// the settings from any files it imports (directly or transitively).
	if err := tempViper.MergeConfig(bytes.NewReader(content)); err != nil {
		return errors.Join(errUtils.ErrMergeConfiguration, fmt.Errorf("re-applying main config after processing imports: %w", err))
	}

	// Now merge commands in the correct order with proper override behavior:
	// 1. Default imports (.atmos.d)
	// 2. Explicit imports
	// 3. Main config (overrides imports on duplicates)
	var finalCommands interface{}

	// Start with defaults
	if defaultCommands != nil {
		finalCommands = defaultCommands
	}

	// Add imported, with imported overriding defaults on duplicates
	if importedCommands != nil {
		finalCommands = mergeCommandArrays(finalCommands, importedCommands)
	}

	// Add main, with main overriding all others on duplicates
	if mainCommands != nil {
		finalCommands = mergeCommandArrays(finalCommands, mainCommands)
	}

	tempViper.Set(commandsKey, finalCommands)

	return nil
}

// marshalViperToYAML marshals a Viper instance's settings to YAML.
func marshalViperToYAML(tempViper *viper.Viper) ([]byte, error) {
	allSettings := tempViper.AllSettings()
	yamlBytes, err := yaml.Marshal(allSettings)
	if err != nil {
		return nil, errors.Join(errUtils.ErrFailedMarshalConfigToYaml, err)
	}
	return yamlBytes, nil
}

// mergeYAMLIntoViper merges YAML content into a Viper instance.
func mergeYAMLIntoViper(v *viper.Viper, configFilePath string, yamlContent []byte) error {
	v.SetConfigFile(configFilePath)
	if err := v.MergeConfig(strings.NewReader(string(yamlContent))); err != nil {
		return errors.Join(errUtils.ErrMerge, err)
	}
	return nil
}

// mergeConfig merges a config file and its imports with proper precedence.
// Each config file's settings override the settings from files it imports.
// This creates a hierarchy where the importing file always takes precedence over imported files.
func mergeConfig(v *viper.Viper, path string, fileName string, processImports bool) error {
	// Load the configuration file
	tempViper, err := loadConfigFile(path, fileName)
	if err != nil {
		return err
	}

	configFilePath := tempViper.ConfigFileUsed()

	// Read the config file's content
	content, err := readConfigFileContent(configFilePath)
	if err != nil {
		return err
	}

	// Process imports if requested
	if processImports {
		if err := processConfigImportsAndReapply(path, tempViper, content); err != nil {
			return err
		}
	}

	// Process YAML functions
	if err := preprocessAtmosYamlFunc(content, tempViper); err != nil {
		return errors.Join(errUtils.ErrPreprocessYAMLFunctions, err)
	}

	// Marshal to YAML
	yamlBytes, err := marshalViperToYAML(tempViper)
	if err != nil {
		return err
	}

	// Merge into the main Viper instance
	return mergeYAMLIntoViper(v, configFilePath, yamlBytes)
}

// shouldExcludePathForTesting checks if a directory path should be excluded from .atmos.d loading during testing.
// It compares the given directory path against a list of excluded paths from the TEST_EXCLUDE_ATMOS_D environment variable.
// Returns true if the path should be excluded, false otherwise.
func shouldExcludePathForTesting(dirPath string) bool {
	//nolint:forbidigo // TEST_EXCLUDE_ATMOS_D is specifically for test isolation, not application configuration.
	excludePaths := os.Getenv("TEST_EXCLUDE_ATMOS_D")
	if excludePaths == "" {
		return false
	}

	// Canonicalize the directory path we're checking.
	absDirPath, err := filepath.Abs(filepath.Clean(dirPath))
	if err != nil {
		absDirPath = dirPath
	}

	// Split paths using the OS-specific path list separator.
	for _, excludePath := range strings.Split(excludePaths, string(os.PathListSeparator)) {
		if excludePath == "" {
			continue
		}

		// Canonicalize the exclude path.
		absExcludePath, err := filepath.Abs(filepath.Clean(excludePath))
		if err != nil {
			continue
		}

		// Check if the current directory is within or equals the excluded path.
		// We currently only check for exact matches, but this could be extended
		// to check for containment using filepath.Rel if needed.
		pathsMatch := false
		if runtime.GOOS == "windows" {
			// Case-insensitive comparison on Windows.
			pathsMatch = strings.EqualFold(absDirPath, absExcludePath)
		} else {
			pathsMatch = absDirPath == absExcludePath
		}

		if pathsMatch {
			return true
		}
	}

	return false
}

// loadAtmosConfigsFromDirectory loads all YAML configuration files from a directory
// and merges them into the destination viper instance.
// This is used by both .atmos.d/ loading and profile loading.
//
// The directory can contain:
//   - YAML files (.yaml, .yml)
//   - Subdirectories with YAML files
//   - Special files like atmos.yaml (loaded with priority)
//
// Files are loaded in order:
//  1. Priority files (atmos.yaml) first
//  2. Sorted by depth (shallower first)
//  3. Lexicographic order within same depth
//
// Parameters:
//   - searchPattern: Glob pattern for finding files (e.g., "/path/to/dir/**/*")
//   - dst: Destination viper instance to merge configs into
//   - source: Description for error messages (e.g., ".atmos.d", "profile 'developer'")
//
// Returns error if files can't be read or YAML is invalid.
func loadAtmosConfigsFromDirectory(searchPattern string, dst *viper.Viper, source string) error {
	// Find all config files using existing search infrastructure.
	foundPaths, err := SearchAtmosConfig(searchPattern)
	if err != nil {
		return fmt.Errorf("%w: failed to search for configuration files in %s: %w", errUtils.ErrParseFile, source, err)
	}

	// No files found is not an error - just means directory is empty.
	if len(foundPaths) == 0 {
		log.Trace("No configuration files found", "source", source, "pattern", searchPattern)
		return nil
	}

	// Load and merge each file.
	for _, filePath := range foundPaths {
		if err := mergeConfigFile(filePath, dst); err != nil {
			return fmt.Errorf("%w: failed to load configuration file from %s: %s: %w", errUtils.ErrParseFile, source, filePath, err)
		}

		log.Trace("Loaded configuration file", "path", filePath, "source", source)
	}

	log.Debug("Loaded configuration directory",
		"source", source,
		"files", len(foundPaths),
		"pattern", searchPattern)

	return nil
}

// mergeDefaultImports merges default imports (`atmos.d/`,`.atmos.d/`)
// from a specified directory into the destination configuration.
func mergeDefaultImports(dirPath string, dst *viper.Viper) error {
	isDir := false
	if stat, err := os.Stat(dirPath); err == nil && stat.IsDir() {
		isDir = true
	}
	if !isDir {
		return errUtils.ErrAtmosDirConfigNotFound
	}

	// Check if we should exclude .atmos.d from this directory during testing.
	if shouldExcludePathForTesting(dirPath) {
		// Silently skip without logging to avoid test output pollution.
		return nil
	}

	// Search for `atmos.d/` configurations.
	searchPattern := filepath.Join(filepath.FromSlash(dirPath), filepath.Join("atmos.d", "**", "*"))
	if err := loadAtmosConfigsFromDirectory(searchPattern, dst, "atmos.d"); err != nil {
		log.Trace("Failed to load atmos.d configs", "error", err)
		// Don't return error - just log and continue.
		// This maintains existing behavior where .atmos.d loading is optional.
	}

	// Search for `.atmos.d` configurations.
	searchPattern = filepath.Join(filepath.FromSlash(dirPath), filepath.Join(".atmos.d", "**", "*"))
	if err := loadAtmosConfigsFromDirectory(searchPattern, dst, ".atmos.d"); err != nil {
		log.Trace("Failed to load .atmos.d configs", "error", err)
		// Don't return error - just log and continue.
		// This maintains existing behavior where .atmos.d loading is optional.
	}

	return nil
}

// mergeImports processes imports from the atmos configuration and merges them into the destination configuration.
func mergeImports(dst *viper.Viper) error {
	var src schema.AtmosConfiguration
	err := dst.Unmarshal(&src)
	if err != nil {
		return err
	}
	if err := processConfigImports(&src, dst); err != nil {
		return err
	}
	return nil
}

// mergeConfigFile merges a new configuration file with an existing config into Viper.
// For command arrays, it appends rather than replaces to allow extending commands via imports.
func mergeConfigFile(
	path string,
	v *viper.Viper,
) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Save existing commands before merge.
	existingCommands := v.Get(commandsKey)

	// Parse the new file to get its commands.
	// We need to do this because viper.MergeConfig doesn't overwrite arrays.
	tempViper := viper.New()
	tempViper.SetConfigType(yamlType)
	err = tempViper.ReadConfig(bytes.NewReader(content))
	if err != nil {
		return err
	}
	newCommands := tempViper.Get(commandsKey)

	// Do the normal merge for all other settings (non-array values).
	// This preserves viper's merge behavior for nested maps.
	err = v.MergeConfig(bytes.NewReader(content))
	if err != nil {
		return err
	}

	// Now handle command merging manually.
	// Merge commands: when duplicates exist, the file being processed (new) takes precedence.
	// This ensures local/main config can override imported/remote commands.
	if existingCommands != nil || newCommands != nil {
		// Second parameter wins on duplicates, so new commands override existing
		merged := mergeCommandArrays(existingCommands, newCommands)
		v.Set(commandsKey, merged)
	}

	err = preprocessAtmosYamlFunc(content, v)
	if err != nil {
		return err
	}

	return nil
}

// mergeCommandArrays merges two command arrays, appending new commands to existing ones.
// This allows imports to extend commands rather than replace them.
// When duplicates exist based on name, the second parameter takes precedence (override behavior).
// This ensures local commands can override imported/remote commands.
func mergeCommandArrays(first, second interface{}) []interface{} {
	// Build a map of commands by name, with later entries overriding earlier ones.
	commandMap := make(map[string]interface{})
	var orderedNames []string

	// Helper function to process a command list.
	processCommands := func(commands interface{}) {
		cmdSlice, ok := commands.([]interface{})
		if !ok {
			return
		}

		for _, cmd := range cmdSlice {
			cmdMap, ok := cmd.(map[string]interface{})
			if !ok {
				continue
			}

			name, ok := cmdMap["name"].(string)
			if !ok {
				continue
			}

			// If this name hasn't been seen before, track its order.
			if _, exists := commandMap[name]; !exists {
				orderedNames = append(orderedNames, name)
			}

			// Store or override the command.
			commandMap[name] = cmd
		}
	}

	// Process first set (will be overridden by second if duplicates).
	processCommands(first)

	// Process second set (overrides first if duplicate names).
	processCommands(second)

	// Build result in the order commands were first seen.
	var result []interface{}
	for _, name := range orderedNames {
		if cmd, exists := commandMap[name]; exists {
			result = append(result, cmd)
		}
	}

	return result
}

// loadEmbeddedConfig loads the embedded atmos.yaml configuration.
func loadEmbeddedConfig(v *viper.Viper) error {
	// Create a reader from the embedded YAML data
	reader := bytes.NewReader(embeddedConfigData)

	// Merge the embedded configuration into Viper
	if err := v.MergeConfig(reader); err != nil {
		return errors.Join(errUtils.ErrMergeEmbeddedConfig, err)
	}

	return nil
}

// preserveIdentityCase extracts original case identity names from the raw YAML and creates a case mapping.
// Viper lowercases all map keys, but we need to preserve original case for identity names.
func preserveIdentityCase(v *viper.Viper, atmosConfig *schema.AtmosConfiguration) error {
	// Get the auth.identities section from Viper before case conversion
	// Viper's AllSettings() returns the lowercased version, so we need to parse the raw YAML
	configFile := v.ConfigFileUsed()
	if configFile == "" {
		// No config file loaded, nothing to preserve
		return nil
	}

	// Read the raw YAML file
	rawYAML, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML to extract original case identity names
	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal(rawYAML, &rawConfig); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Extract auth.identities with original case
	authSection, ok := rawConfig["auth"].(map[string]interface{})
	if !ok || authSection == nil {
		// No auth section, nothing to preserve
		return nil
	}

	identitiesSection, ok := authSection["identities"].(map[string]interface{})
	if !ok || identitiesSection == nil {
		// No identities section, nothing to preserve
		return nil
	}

	// Create case mapping: lowercase -> original case
	caseMap := make(map[string]string)
	for originalName := range identitiesSection {
		lowercaseName := strings.ToLower(originalName)
		caseMap[lowercaseName] = originalName
	}

	// Store the case mapping in the config
	if atmosConfig.Auth.IdentityCaseMap == nil {
		atmosConfig.Auth.IdentityCaseMap = make(map[string]string)
	}
	for k, v := range caseMap {
		atmosConfig.Auth.IdentityCaseMap[k] = v
	}

	log.Debug("Preserved identity case mapping", "identities", len(caseMap))

	return nil
}
