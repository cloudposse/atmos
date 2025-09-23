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

	"gopkg.in/yaml.v3"

	log "github.com/charmbracelet/log"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/go-homedir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
)

//go:embed atmos.yaml
var embeddedConfigData []byte

const MaximumImportLvL = 10

var ErrAtmosDIrConfigNotFound = errors.New("atmos config directory not found")

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
	// https://gist.github.com/chazcheadle/45bf85b793dea2b71bd05ebaa3c28644
	// https://sagikazarmark.hu/blog/decoding-custom-formats-with-viper/
	err := v.Unmarshal(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}
	return atmosConfig, nil
}

func setEnv(v *viper.Viper) {
	bindEnv(v, "settings.github_token", "GITHUB_TOKEN")
	bindEnv(v, "settings.inject_github_token", "ATMOS_INJECT_GITHUB_TOKEN")
	bindEnv(v, "settings.atmos_github_token", "ATMOS_GITHUB_TOKEN")

	bindEnv(v, "settings.bitbucket_token", "BITBUCKET_TOKEN")
	bindEnv(v, "settings.atmos_bitbucket_token", "ATMOS_BITBUCKET_TOKEN")
	bindEnv(v, "settings.inject_bitbucket_token", "ATMOS_INJECT_BITBUCKET_TOKEN")
	bindEnv(v, "settings.bitbucket_username", "BITBUCKET_USERNAME")

	bindEnv(v, "settings.gitlab_token", "GITLAB_TOKEN")
	bindEnv(v, "settings.inject_gitlab_token", "ATMOS_INJECT_GITLAB_TOKEN")
	bindEnv(v, "settings.atmos_gitlab_token", "ATMOS_GITLAB_TOKEN")

	bindEnv(v, "settings.terminal.pager", "ATMOS_PAGER", "PAGER")
	bindEnv(v, "settings.terminal.no_color", "ATMOS_NO_COLOR", "NO_COLOR")

	// Atmos Pro settings
	bindEnv(v, "settings.pro.base_url", "ATMOS_PRO_BASE_URL")
	bindEnv(v, "settings.pro.endpoint", "ATMOS_PRO_ENDPOINT")
	bindEnv(v, "settings.pro.token", "ATMOS_PRO_TOKEN")
	bindEnv(v, "settings.pro.workspace_id", "ATMOS_PRO_WORKSPACE_ID")

	// GitHub OIDC for Atmos Pro
	bindEnv(v, "settings.pro.github_oidc.request_url", "ACTIONS_ID_TOKEN_REQUEST_URL")
	bindEnv(v, "settings.pro.github_oidc.request_token", "ACTIONS_ID_TOKEN_REQUEST_TOKEN")

	// Telemetry settings
	bindEnv(v, "settings.telemetry.enabled", "ATMOS_TELEMETRY_ENABLED")
	bindEnv(v, "settings.telemetry.token", "ATMOS_TELEMETRY_TOKEN")
	bindEnv(v, "settings.telemetry.endpoint", "ATMOS_TELEMETRY_ENDPOINT")
	bindEnv(v, "settings.telemetry.logging", "ATMOS_TELEMETRY_LOGGING")
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
	v.SetDefault("settings.inject_github_token", true)
	v.SetDefault("logs.file", "/dev/stderr")
	v.SetDefault("logs.level", "Info")

	v.SetDefault("settings.terminal.no_color", false)
	v.SetDefault("settings.terminal.pager", true)
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
	home, err := homedir.Dir()
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
	tempViper.SetConfigType("yaml")

	if err := tempViper.ReadInConfig(); err != nil {
		// Return sentinel error unwrapped for type checking
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			return nil, err
		}
		// Wrap any other error with context
		return nil, fmt.Errorf("failed to read config %s/%s: %w", path, fileName, err)
	}

	return tempViper, nil
}

// readConfigFileContent reads the content of a configuration file.
func readConfigFileContent(configFilePath string) ([]byte, error) {
	content, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", configFilePath, err)
	}
	return content, nil
}

// processConfigImportsAndReapply processes imports and re-applies the original config for proper precedence.
func processConfigImportsAndReapply(path string, tempViper *viper.Viper, content []byte) error {
	// Process default imports
	if err := mergeDefaultImports(path, tempViper); err != nil {
		log.Debug("error process imports", "path", path, "error", err)
	}

	// Process explicit imports
	if err := mergeImports(tempViper); err != nil {
		log.Debug("error process imports", "file", tempViper.ConfigFileUsed(), "error", err)
	}

	// Re-apply this config file's content after processing its imports
	// This ensures proper precedence: each config file's own settings override
	// the settings from any files it imports (directly or transitively).
	// For example: if A imports B, and B imports C, then:
	// - B's settings override C's settings
	// - A's settings override both B's and C's settings
	if err := tempViper.MergeConfig(bytes.NewReader(content)); err != nil {
		return fmt.Errorf("merge temp config: %w", err)
	}

	return nil
}

// marshalViperToYAML marshals a Viper instance's settings to YAML.
func marshalViperToYAML(tempViper *viper.Viper) ([]byte, error) {
	allSettings := tempViper.AllSettings()
	yamlBytes, err := yaml.Marshal(allSettings)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrFailedMarshalConfigToYaml, err)
	}
	return yamlBytes, nil
}

// mergeYAMLIntoViper merges YAML content into a Viper instance.
func mergeYAMLIntoViper(v *viper.Viper, configFilePath string, yamlContent []byte) error {
	v.SetConfigFile(configFilePath)
	if err := v.MergeConfig(strings.NewReader(string(yamlContent))); err != nil {
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrMerge, err)
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
		return fmt.Errorf("preprocess YAML functions: %w", err)
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

// mergeDefaultImports merges default imports (`atmos.d/`,`.atmos.d/`)
// from a specified directory into the destination configuration.
func mergeDefaultImports(dirPath string, dst *viper.Viper) error {
	isDir := false
	if stat, err := os.Stat(dirPath); err == nil && stat.IsDir() {
		isDir = true
	}
	if !isDir {
		return ErrAtmosDIrConfigNotFound
	}

	// Check if we should exclude .atmos.d from this directory during testing.
	if shouldExcludePathForTesting(dirPath) {
		// Silently skip without logging to avoid test output pollution.
		return nil
	}

	var atmosFoundFilePaths []string
	// Search for `atmos.d/` configurations.
	searchDir := filepath.Join(filepath.FromSlash(dirPath), filepath.Join("atmos.d", "**", "*"))
	foundPaths1, _ := SearchAtmosConfig(searchDir)
	if len(foundPaths1) > 0 {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths1...)
	}
	// Search for `.atmos.d` configurations.
	searchDir = filepath.Join(filepath.FromSlash(dirPath), filepath.Join(".atmos.d", "**", "*"))
	foundPaths2, _ := SearchAtmosConfig(searchDir)
	if len(foundPaths2) > 0 {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths2...)
	}
	for _, filePath := range atmosFoundFilePaths {
		err := mergeConfigFile(filePath, dst)
		if err != nil {
			log.Debug("error loading config file", "path", filePath, "error", err)
			continue
		}
		log.Debug("atmos merged config", "path", filePath)
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
func mergeConfigFile(
	path string,
	v *viper.Viper,
) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	err = v.MergeConfig(bytes.NewReader(content))
	if err != nil {
		return err
	}
	err = preprocessAtmosYamlFunc(content, v)
	if err != nil {
		return err
	}

	return nil
}

// loadEmbeddedConfig loads the embedded atmos.yaml configuration.
func loadEmbeddedConfig(v *viper.Viper) error {
	// Create a reader from the embedded YAML data
	reader := bytes.NewReader(embeddedConfigData)

	// Merge the embedded configuration into Viper
	if err := v.MergeConfig(reader); err != nil {
		return fmt.Errorf("failed to merge embedded config: %w", err)
	}

	return nil
}
