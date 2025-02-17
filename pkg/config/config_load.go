package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

func InitCliConfig(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
	var err error
	v := viper.New()
	v.SetTypeByDefaultValue(true)
	wd, err := os.Getwd()
	if err != nil {
		return schema.AtmosConfiguration{}, err
	}
	IsEnvConfigPathRequired := os.Getenv("ATMOS_CLI_CONFIG_PATH") != ""
	// Set default values
	setDefaultConfiguration(v)
	configSources := []struct {
		paths    []string
		required bool
		name     string
	}{
		{paths: getSystemConfigPaths(), required: false, name: "system"},
		{paths: []string{getHomeConfigPath()}, required: false, name: "home"},
		{paths: []string{wd}, required: false, name: "work-dir"},
		{paths: []string{os.Getenv("ATMOS_CLI_CONFIG_PATH")}, required: IsEnvConfigPathRequired, name: "env"},
		{paths: []string{configAndStacksInfo.AtmosCliConfigPath}, required: false, name: "provider"},
	}
	// Read and merge configurations
	configFound, err := readAndMergeConfigs(v, configSources)
	if err != nil {
		return schema.AtmosConfiguration{}, err
	}

	// Handle missing configurations
	if !configFound {
		if err := applyDefaultConfiguration(v); err != nil {
			return schema.AtmosConfiguration{}, err
		}
	}

	// Unmarshal configuration
	var atmosConfig schema.AtmosConfiguration
	if err := v.Unmarshal(&atmosConfig); err != nil {
		return atmosConfig, err
	}

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

	// Convert stacks base path to absolute path
	stacksBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)
	stacksBaseAbsPath, err := filepath.Abs(stacksBasePath)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.StacksBaseAbsolutePath = stacksBaseAbsPath

	// Convert the included stack paths to absolute paths
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, atmosConfig.Stacks.IncludedPaths)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.IncludeStackAbsolutePaths = includeStackAbsPaths

	// Convert the excluded stack paths to absolute paths
	excludeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, atmosConfig.Stacks.ExcludedPaths)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.ExcludeStackAbsolutePaths = excludeStackAbsPaths

	// Convert terraform dir to absolute path
	terraformBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath)
	terraformDirAbsPath, err := filepath.Abs(terraformBasePath)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.TerraformDirAbsolutePath = terraformDirAbsPath

	// Convert helmfile dir to absolute path
	helmfileBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath)
	helmfileDirAbsPath, err := filepath.Abs(helmfileBasePath)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.HelmfileDirAbsolutePath = helmfileDirAbsPath

	if processStacks {
		// If the specified stack name is a logical name, find all stack manifests in the provided paths
		stackConfigFilesAbsolutePaths, stackConfigFilesRelativePaths, stackIsPhysicalPath, err := FindAllStackConfigsInPathsForStack(
			atmosConfig,
			configAndStacksInfo.Stack,
			includeStackAbsPaths,
			excludeStackAbsPaths,
		)
		if err != nil {
			return atmosConfig, err
		}

		if len(stackConfigFilesAbsolutePaths) < 1 {
			j, err := u.ConvertToYAML(includeStackAbsPaths)
			if err != nil {
				return atmosConfig, err
			}
			errorMessage := fmt.Sprintf("\nno stack manifests found in the provided "+
				"paths:\n%s\n\nCheck if `base_path`, 'stacks.base_path', 'stacks.included_paths' and 'stacks.excluded_paths' are correctly set in CLI config "+
				"files or ENV vars.", j)
			return atmosConfig, errors.New(errorMessage)
		}

		atmosConfig.StackConfigFilesAbsolutePaths = stackConfigFilesAbsolutePaths
		atmosConfig.StackConfigFilesRelativePaths = stackConfigFilesRelativePaths

		if stackIsPhysicalPath {
			u.LogTrace(fmt.Sprintf("\nThe stack '%s' matches the stack manifest %s\n",
				configAndStacksInfo.Stack,
				stackConfigFilesRelativePaths[0]),
			)
			atmosConfig.StackType = "Directory"
		} else {
			// The stack is a logical name
			atmosConfig.StackType = "Logical"
		}
	}

	atmosConfig.Initialized = true
	return atmosConfig, nil
}

// readAndMergeConfigs handles all config loading logic
func readAndMergeConfigs(v *viper.Viper, configSources []struct {
	paths    []string
	required bool
	name     string
}) (bool, error) {
	configFound := false

	for _, source := range configSources {
		for _, rawPath := range source.paths {
			if rawPath == "" {
				continue
			}

			path := expandPath(rawPath)
			absPath, err := filepath.Abs(path)
			if err != nil {
				if source.required {
					return false, fmt.Errorf("invalid config path %s: %w", path, err)
				}
				continue
			}

			// Check if path exists for required sources
			if source.required {
				if _, err := os.Stat(absPath); os.IsNotExist(err) {
					return false, fmt.Errorf("required config path not found: %s", absPath)
				}
			}

			v.AddConfigPath(absPath)
		}
	}
	// Try all configuration names and extensions
	configNames := []string{"atmos", ".atmos"}
	configExtensions := []string{"yaml", "yml"}
	// Try all combinations of config names and extensions
	for _, name := range configNames {
		v.SetConfigName(name)
		yamlExist := false
		for _, ext := range configExtensions {
			if yamlExist {
				continue
			}
			v.SetConfigType(ext)
			if err := v.MergeInConfig(); err == nil {
				configFound = true
				yamlExist = true
			} else if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return false, err
			}
		}
	}

	return configFound, nil
}

// Helper functions
func setDefaultConfiguration(v *viper.Viper) {
	v.SetDefault("components.helmfile.use_eks", true)
	v.SetDefault("components.terraform.append_user_agent",
		fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version))
	v.SetDefault("settings.inject_github_token", true)
	v.SetDefault("logs.file", "/dev/stderr")
	v.SetDefault("logs.level", "Info")

}

func getSystemConfigPaths() []string {
	if runtime.GOOS == "windows" {
		if appData := os.Getenv(WindowsAppDataEnvVar); appData != "" {
			return []string{filepath.Join(appData, "atmos")}
		}
		return []string{}
	}
	return []string{SystemDirConfigFilePath}
}

func getHomeConfigPath() string {
	home, err := homedir.Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".atmos")
}

func isRemoteConfig(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

func addRemoteConfig(v *viper.Viper, url string) error {
	parts := strings.SplitN(url, "://", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid remote config URL: %s", url)
	}

	v.AddRemoteProvider(parts[0], parts[1], "")
	v.SetConfigType("yaml")
	return v.ReadRemoteConfig()
}

func applyDefaultConfiguration(v *viper.Viper) error {
	logsLevel := os.Getenv("ATMOS_LOGS_LEVEL")
	if logsLevel == u.LogLevelDebug || logsLevel == u.LogLevelTrace {
		var atmosConfig schema.AtmosConfiguration
		u.PrintMessageInColor("Using default configuration...\n", theme.Colors.Info)
		err := u.PrintAsYAMLToFileDescriptor(atmosConfig, defaultCliConfig)
		if err != nil {
			return err
		}
	}

	defaultConfig, err := json.Marshal(defaultCliConfig)
	if err != nil {
		return err
	}
	return v.MergeConfig(bytes.NewReader(defaultConfig))
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := homedir.Dir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
