package config

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/provisioning"
	"github.com/cloudposse/atmos/pkg/config/casemap"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/cloudposse/atmos/pkg/xdg"
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

// mergedConfigFiles tracks all config files merged during a LoadConfig call.
// This is used to extract case-sensitive map keys from all sources, not just the main config.
// The slice is reset at the start of each LoadConfig call.
//
// NOTE: This package-level state assumes sequential (non-concurrent) calls to LoadConfig.
// LoadConfig is NOT safe for concurrent use. If concurrent config loading becomes necessary,
// this should be refactored to pass state through a context or options struct.
var mergedConfigFiles []string

// resetMergedConfigFiles clears the tracked config files. Call at start of LoadConfig.
func resetMergedConfigFiles() {
	mergedConfigFiles = nil
}

// trackMergedConfigFile records a config file path for case-sensitive key extraction.
func trackMergedConfigFile(path string) {
	if path != "" {
		mergedConfigFiles = append(mergedConfigFiles, path)
	}
}

const (
	profileKey       = "profile"
	profileDelimiter = ","
)

// parseProfilesFromOsArgs parses --profile flags from os.Args using pflag.
// This is a fallback for commands with DisableFlagParsing=true (terraform, helmfile, packer).
// Uses pflag's StringSlice parser to handle all syntax variations correctly.
func parseProfilesFromOsArgs(args []string) []string {
	// Create temporary FlagSet just for parsing --profile.
	fs := pflag.NewFlagSet("profile-parser", pflag.ContinueOnError)
	fs.ParseErrorsAllowlist.UnknownFlags = true // Ignore other flags.

	// Register profile flag using pflag's StringSlice (handles comma-separated values).
	profiles := fs.StringSlice(profileKey, []string{}, "Configuration profiles")

	// Parse args - pflag handles both --profile=value and --profile value syntax.
	_ = fs.Parse(args) // Ignore errors from unknown flags.

	if profiles == nil || len(*profiles) == 0 {
		return nil
	}

	// Post-process: trim whitespace and filter empty values (maintains compatibility with manual parsing).
	result := make([]string, 0, len(*profiles))
	for _, profile := range *profiles {
		trimmed := strings.TrimSpace(profile)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// parseViperProfilesFromEnv handles Viper's quirky environment variable parsing for StringSlice.
// Viper does NOT parse comma-separated environment variables correctly:
//   - "dev,staging,prod" → []string{"dev,staging,prod"} (single element, NOT split)
//   - "dev staging prod" → []string{"dev", "staging", "prod"} (splits on whitespace)
//   - " dev , staging " → []string{"dev", ",", "staging"} (splits on whitespace, keeps commas!)
func parseViperProfilesFromEnv(profiles []string) []string {
	var parsed []string

	for _, p := range profiles {
		trimmed := strings.TrimSpace(p)
		// Skip empty strings and standalone commas (from Viper's whitespace split).
		if trimmed == "" || trimmed == "," {
			continue
		}

		// If this element contains commas, split it further.
		if strings.Contains(trimmed, ",") {
			for _, part := range strings.Split(trimmed, ",") {
				if partTrimmed := strings.TrimSpace(part); partTrimmed != "" {
					parsed = append(parsed, partTrimmed)
				}
			}
		} else {
			// No commas, use as-is.
			parsed = append(parsed, trimmed)
		}
	}

	return parsed
}

// parseProfilesFromEnvString parses comma-separated profiles from an environment variable value.
// Trims whitespace and filters empty entries.
func parseProfilesFromEnvString(envValue string) []string {
	var result []string
	for _, v := range strings.Split(envValue, profileDelimiter) {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// getProfilesFromFallbacks handles fallback profile loading when Viper doesn't have profiles set.
// Returns profiles and source ("flag" or "env") for logging.
func getProfilesFromFallbacks() ([]string, string) {
	// Fallback: For commands with DisableFlagParsing=true, Cobra never parses flags,
	// so Viper won't have flag values. Manually parse os.Args as fallback.
	profiles := parseProfilesFromOsArgs(os.Args)
	if len(profiles) > 0 {
		return profiles, "flag"
	}

	// Check environment variable directly as final fallback.
	if envProfiles := os.Getenv("ATMOS_PROFILE"); envProfiles != "" { //nolint:forbidigo
		result := parseProfilesFromEnvString(envProfiles)
		if len(result) > 0 {
			return result, "env"
		}
	}

	return nil, ""
}

// getProfilesFromFlagsOrEnv retrieves profiles from --profile flag or ATMOS_PROFILE env var.
// This is a helper function to reduce nesting complexity in LoadConfig.
// Returns profiles and source ("env" or "flag") for logging.
//
// NOTE: This function reads from Viper's global singleton, which has flag values synced
// by syncGlobalFlagsToViper() in cmd/root.go PersistentPreRun before InitCliConfig is called.
//
// IMPORTANT: For commands with DisableFlagParsing=true (terraform, helmfile, packer),
// Cobra never parses flags, so we fall back to parseProfilesFromOsArgs() to manually
// parse the --profile flag from os.Args. This ensures profiles work for all commands.
func getProfilesFromFlagsOrEnv() ([]string, string) {
	globalViper := viper.GetViper()

	// Check if profile is set in Viper (from either flag or env var).
	if !globalViper.IsSet(profileKey) {
		return getProfilesFromFallbacks()
	}

	profiles := globalViper.GetStringSlice(profileKey)
	_, envSet := os.LookupEnv("ATMOS_PROFILE")

	// Environment variable path - needs special parsing for Viper quirks.
	if envSet && len(profiles) > 0 {
		parsed := parseViperProfilesFromEnv(profiles)
		if len(parsed) > 0 {
			return parsed, "env"
		}
		return nil, ""
	}

	// CLI flag path - already parsed correctly by pflag/Cobra.
	if len(profiles) > 0 {
		return profiles, "flag"
	}

	return nil, ""
}

// LoadConfig loads the Atmos configuration from multiple sources in order of precedence:
// * Embedded atmos.yaml (`atmos/pkg/config/atmos.yaml`)
// * System dir (`/usr/local/etc/atmos` on Linux, `%LOCALAPPDATA%/atmos` on Windows).
// * Home directory (~/.atmos).
// * Current working directory.
// * ENV vars.
// * Command-line arguments.
//
// NOTE: Global flags (like --profile) must be synced to Viper before calling this function.
// This is done by syncGlobalFlagsToViper() in cmd/root.go PersistentPreRun.
func LoadConfig(configAndStacksInfo *schema.ConfigAndStacksInfo) (schema.AtmosConfiguration, error) {
	// Reset merged config file tracker at start of each LoadConfig call.
	resetMergedConfigFiles()

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
		log.Debug("'atmos.yaml' CLI config was not found", "paths", "system dir, home dir, current dir, parent dirs, ENV vars")
		log.Debug("Refer to https://atmos.tools/cli/configuration for details on how to configure 'atmos.yaml'")
		log.Debug("Using the default CLI config")

		if err := mergeDefaultConfig(v); err != nil {
			return atmosConfig, err
		}

		// Also search git root for .atmos.d even with default config.
		// This enables custom commands defined in .atmos.d at the repo root
		// to work when running from any subdirectory.
		gitRoot, err := u.ProcessTagGitRoot("!repo-root .")
		if err == nil && gitRoot != "" && gitRoot != "." {
			log.Debug("Loading .atmos.d from git root", "path", gitRoot)
			if err := mergeDefaultImports(gitRoot, v); err != nil {
				log.Trace("Failed to load .atmos.d from git root", "path", gitRoot, "error", err)
				// Non-fatal: continue with default config.
			}
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

	// If profiles weren't passed via ConfigAndStacksInfo, check if they were
	// specified via --profile flag or ATMOS_PROFILE env var.
	// Note: Global flags are bound to viper.GetViper() (global singleton), not the local viper instance.
	if len(configAndStacksInfo.ProfilesFromArg) == 0 {
		profiles, source := getProfilesFromFlagsOrEnv()
		if len(profiles) > 0 {
			configAndStacksInfo.ProfilesFromArg = profiles
			log.Debug("Profiles loaded from CLI "+source, "profiles", profiles)
		}
	}

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

	// Manually extract top-level env fields to avoid mapstructure tag collision.
	// Both AtmosConfiguration.Env and Command.Env use "env" but with different types
	// (map[string]string vs []CommandEnv), causing mapstructure to silently drop Commands.
	// Using mapstructure:"-" on the Env fields and extracting manually here fixes this.
	if envMap := v.GetStringMapString("env"); len(envMap) > 0 {
		atmosConfig.Env = envMap
	}
	if envMap := v.GetStringMapString("templates.settings.env"); len(envMap) > 0 {
		atmosConfig.Templates.Settings.Env = envMap
	}

	// Post-process to preserve case-sensitive map keys.
	// Viper lowercases all YAML map keys, but we need to preserve original case
	// for identity names and environment variables.
	preserveCaseSensitiveMaps(v, &atmosConfig)

	// Apply git root discovery for default base path.
	// This enables running Atmos from any subdirectory, similar to Git.
	if err := applyGitRootBasePath(&atmosConfig); err != nil {
		log.Debug("Failed to apply git root base path", "error", err)
		// Don't fail config loading if this step fails, just log it.
	}

	return atmosConfig, nil
}

func setEnv(v *viper.Viper) {
	// Base path configuration.
	bindEnv(v, "base_path", "ATMOS_BASE_PATH")

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
	bindEnv(v, "settings.terminal.force_color", "ATMOS_FORCE_COLOR")
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
	v.SetDefault("settings.terminal.pager", "false") // String value to match the field type
	// Note: force_color is ENV-only (ATMOS_FORCE_COLOR), no config default
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
		log.Trace("Checking for atmos.yaml in system config", "path", configFilePath)
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
	log.Trace("Checking for atmos.yaml in home directory", "path", configFilePath)
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

// readWorkDirConfig loads config from current working directory or any parent directory.
// It searches upward through the directory tree until it finds an atmos.yaml file
// or reaches the filesystem root. This enables running atmos commands from any
// subdirectory (e.g., component directories) without specifying --config-path.
// Parent directory search can be disabled by setting ATMOS_CLI_CONFIG_PATH to "." or
// any explicit path.
func readWorkDirConfig(v *viper.Viper) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	// First try the current directory.
	log.Trace("Checking for atmos.yaml in working directory", "path", wd)
	err = mergeConfig(v, wd, CliConfigFileName, true)
	if err == nil {
		return nil
	}

	// If not a ConfigFileNotFoundError, return the error.
	var configFileNotFoundError viper.ConfigFileNotFoundError
	if !errors.As(err, &configFileNotFoundError) {
		return err
	}

	// If ATMOS_CLI_CONFIG_PATH is set, don't search parent directories.
	// This allows tests and users to explicitly control config discovery.
	//nolint:forbidigo // ATMOS_CLI_CONFIG_PATH controls config loading behavior itself,
	// it must be checked before viper loads config files. Using os.Getenv is necessary
	// because this check happens during the config loading phase, before any viper
	// bindings are established.
	if os.Getenv("ATMOS_CLI_CONFIG_PATH") != "" {
		return nil
	}

	// Search parent directories for atmos.yaml.
	configDir := findAtmosConfigInParentDirs(wd)
	if configDir == "" {
		// No config found in any parent directory.
		return nil
	}

	// Found config in a parent directory, merge it.
	err = mergeConfig(v, configDir, CliConfigFileName, true)
	if err != nil {
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
			return nil
		default:
			return err
		}
	}

	log.Debug("Found atmos.yaml in parent directory", "path", configDir)
	return nil
}

// findAtmosConfigInParentDirs searches for atmos.yaml in parent directories.
// It walks up the directory tree from the given starting directory until
// it finds an atmos.yaml file or reaches the filesystem root.
// Returns the directory containing atmos.yaml, or empty string if not found.
func findAtmosConfigInParentDirs(startDir string) string {
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
			configPath := filepath.Join(dir, configName)
			if _, err := os.Stat(configPath); err == nil {
				return dir
			}
		}
	}
}

func readEnvAmosConfigPath(v *viper.Viper) error {
	atmosPath := os.Getenv("ATMOS_CLI_CONFIG_PATH")
	if atmosPath == "" {
		return nil
	}
	log.Trace("Checking for atmos.yaml from ATMOS_CLI_CONFIG_PATH", "path", atmosPath)
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
		// Wrap error with context using proper chaining.
		// This preserves the full error chain for debugging while adding our sentinel error.
		return nil, fmt.Errorf("%w: %s/%s: %w", errUtils.ErrReadConfig, path, fileName, err)
	}

	return tempViper, nil
}

// readConfigFileContent reads the content of a configuration file.
func readConfigFileContent(configFilePath string) ([]byte, error) {
	content, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrReadConfig, configFilePath, err)
	}
	return content, nil
}

// processConfigImportsAndReapply processes imports and re-applies the original config for proper precedence.
func processConfigImportsAndReapply(path string, tempViper *viper.Viper, content []byte) error {
	// Parse the main config to get its commands separately.
	mainViper := viper.New()
	mainViper.SetConfigType(yamlType)
	if err := mainViper.ReadConfig(bytes.NewReader(content)); err != nil {
		return fmt.Errorf("%w: parse main config: %w", errUtils.ErrMergeConfiguration, err)
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
		return fmt.Errorf("%w: merge main config: %w", errUtils.ErrMergeConfiguration, err)
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
		return fmt.Errorf("%w: re-applying main config after processing imports: %w", errUtils.ErrMergeConfiguration, err)
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
// It also searches the git/worktree root for .atmos.d with lower priority.
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

	// Search git/worktree root FIRST (lower priority - gets overridden by config dir).
	// This enables .atmos.d to be discovered at the repo root even when running from subdirectories.
	loadAtmosDFromGitRoot(dirPath, dst)

	// Search the config directory (higher priority - loaded second, overrides git root).
	log.Trace("Checking for .atmos.d in config directory", "path", dirPath)
	loadAtmosDFromDirectory(dirPath, dst)

	return nil
}

// loadAtmosDFromGitRoot searches for .atmos.d/ at the git repository root
// and loads its configuration if different from the config directory.
func loadAtmosDFromGitRoot(dirPath string, dst *viper.Viper) {
	gitRoot, err := u.ProcessTagGitRoot("!repo-root .")
	if err != nil || gitRoot == "" || gitRoot == "." {
		return
	}

	absGitRoot, absErr := filepath.Abs(gitRoot)
	absDirPath, dirErr := filepath.Abs(dirPath)
	if absErr != nil || dirErr != nil {
		return
	}

	// Check if git root is the same as config directory.
	// Use case-insensitive comparison on Windows where paths may differ only in casing.
	pathsEqual := absGitRoot == absDirPath
	if runtime.GOOS == "windows" {
		pathsEqual = strings.EqualFold(absGitRoot, absDirPath)
	}
	if pathsEqual {
		return
	}

	// Skip if excluded for testing.
	if shouldExcludePathForTesting(absGitRoot) {
		return
	}

	log.Trace("Checking for .atmos.d in git root", "path", absGitRoot)
	loadAtmosDFromDirectory(absGitRoot, dst)
}

// loadAtmosDFromDirectory searches for atmos.d/ and .atmos.d/ in the given directory
// and loads their configurations into the destination viper instance.
func loadAtmosDFromDirectory(dirPath string, dst *viper.Viper) {
	// Search for `atmos.d/` configurations.
	searchPattern := filepath.Join(filepath.FromSlash(dirPath), filepath.Join("atmos.d", "**", "*"))
	if err := loadAtmosConfigsFromDirectory(searchPattern, dst, "atmos.d"); err != nil {
		log.Trace("Failed to load atmos.d configs", "error", err, "path", dirPath)
		// Don't return error - just log and continue.
		// This maintains existing behavior where .atmos.d loading is optional.
	}

	// Search for `.atmos.d` configurations.
	searchPattern = filepath.Join(filepath.FromSlash(dirPath), filepath.Join(".atmos.d", "**", "*"))
	if err := loadAtmosConfigsFromDirectory(searchPattern, dst, ".atmos.d"); err != nil {
		log.Trace("Failed to load .atmos.d configs", "error", err, "path", dirPath)
		// Don't return error - just log and continue.
		// This maintains existing behavior where .atmos.d loading is optional.
	}
}

// mergeImports processes imports from the atmos configuration and merges them into the destination configuration.
func mergeImports(dst *viper.Viper) error {
	var src schema.AtmosConfiguration
	err := dst.Unmarshal(&src)
	if err != nil {
		return err
	}

	// Inject provisioned identity imports before processing.
	if err := injectProvisionedIdentityImports(&src); err != nil {
		log.Debug("Failed to inject provisioned identity imports", "error", err)
		// Non-fatal: continue with config loading even if injection fails.
	}

	if err := processConfigImports(&src, dst); err != nil {
		return err
	}
	return nil
}

// injectProvisionedIdentityImports adds provisioned identity files to the import list.
// Provisioned identities are written to XDG cache during authentication and should be
// imported BEFORE manual configuration to allow manual config to override.
func injectProvisionedIdentityImports(src *schema.AtmosConfiguration) error {
	// Check if there are any auth providers configured.
	if len(src.Auth.Providers) == 0 {
		return nil
	}

	// Get XDG cache directory for provisioned identities.
	// Uses ATMOS_XDG_CACHE_HOME or XDG_CACHE_HOME if set, otherwise ~/.cache/atmos/auth.
	// Note: xdg.GetXDGCacheDir already prepends "atmos/" to the path.
	const authSubDir = "auth"
	const authDirPerms = 0o700
	baseProvisioningDir, err := xdg.GetXDGCacheDir(authSubDir, authDirPerms)
	if err != nil {
		return fmt.Errorf("failed to get provisioning cache directory: %w", err)
	}

	// Collect provisioned identity files for each provider.
	var provisionedImports []string

	for providerName := range src.Auth.Providers {
		provisionedFile := filepath.Join(baseProvisioningDir, providerName, provisioning.ProvisionedFileName)

		// Check if provisioned file exists.
		if _, err := os.Stat(provisionedFile); err == nil {
			log.Debug("Found provisioned identities file", "provider", providerName, "file", provisionedFile)
			provisionedImports = append(provisionedImports, provisionedFile)
		}
	}

	// Inject provisioned imports BEFORE existing imports.
	// This ensures manual config (in existing imports) takes precedence over provisioned config.
	if len(provisionedImports) > 0 {
		log.Debug("Injecting provisioned identity imports", "count", len(provisionedImports))
		src.Import = append(provisionedImports, src.Import...)
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

	// Track this file for case-sensitive key extraction.
	trackMergedConfigFile(path)

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

// caseSensitivePaths lists the YAML paths that need case preservation.
// Viper lowercases all map keys, but these sections need original case.
var caseSensitivePaths = []string{
	"env",             // Environment variables (e.g., GITHUB_TOKEN)
	"auth.identities", // Auth identity names (e.g., SuperAdmin)
}

// collectConfigFilesForCasePreservation gathers all config files to process for case preservation.
// It combines tracked merged files with the main config file (if not already tracked).
func collectConfigFilesForCasePreservation(mainConfig string) []string {
	filesToProcess := make([]string, 0, len(mergedConfigFiles)+1)
	filesToProcess = append(filesToProcess, mergedConfigFiles...)

	// Include the main config file if it wasn't already tracked.
	if mainConfig != "" && !slices.Contains(filesToProcess, mainConfig) {
		filesToProcess = append(filesToProcess, mainConfig)
	}

	return filesToProcess
}

// mergeCaseMapsFromFile reads a config file and merges its case mappings into the accumulated result.
// Later files override earlier ones (same precedence as config merging).
func mergeCaseMapsFromFile(configFile string, mergedCaseMaps *casemap.CaseMaps) {
	rawYAML, err := os.ReadFile(configFile)
	if err != nil {
		log.Trace("Skipping case map extraction for unreadable file", "file", configFile, "error", err)
		return
	}

	fileCaseMaps, err := casemap.ExtractFromYAML(rawYAML, caseSensitivePaths)
	if err != nil {
		log.Trace("Failed to extract case mappings", "file", configFile, "error", err)
		return
	}

	for _, path := range caseSensitivePaths {
		fileMap := fileCaseMaps.Get(path)
		if fileMap == nil {
			continue
		}
		existingMap := mergedCaseMaps.Get(path)
		if existingMap == nil {
			existingMap = make(casemap.CaseMap)
		}
		for k, v := range fileMap {
			existingMap[k] = v
		}
		mergedCaseMaps.Set(path, existingMap)
	}
}

// populateLegacyIdentityCaseMap copies auth.identities case mappings to the legacy IdentityCaseMap field.
func populateLegacyIdentityCaseMap(caseMaps *casemap.CaseMaps, atmosConfig *schema.AtmosConfiguration) {
	identityCaseMap := caseMaps.Get("auth.identities")
	if identityCaseMap == nil {
		return
	}
	if atmosConfig.Auth.IdentityCaseMap == nil {
		atmosConfig.Auth.IdentityCaseMap = make(map[string]string)
	}
	for k, v := range identityCaseMap {
		atmosConfig.Auth.IdentityCaseMap[k] = v
	}
}

// preserveCaseSensitiveMaps extracts original case for registered paths from raw YAML files.
// This creates a mapping that can be used to restore original case when accessing these maps.
// It processes all merged config files (main config + imports) with later files taking precedence.
// This function operates on a best-effort basis - errors are logged but don't fail config loading.
func preserveCaseSensitiveMaps(v *viper.Viper, atmosConfig *schema.AtmosConfiguration) {
	filesToProcess := collectConfigFilesForCasePreservation(v.ConfigFileUsed())
	if len(filesToProcess) == 0 {
		return
	}

	mergedCaseMaps := casemap.New()
	for _, configFile := range filesToProcess {
		mergeCaseMapsFromFile(configFile, mergedCaseMaps)
	}

	atmosConfig.CaseMaps = mergedCaseMaps
	populateLegacyIdentityCaseMap(mergedCaseMaps, atmosConfig)

	log.Trace("Preserved case-sensitive map keys", "paths", caseSensitivePaths, "files_processed", len(filesToProcess))
}
