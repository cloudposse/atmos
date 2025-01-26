package config

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/adrg/xdg"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

const MaximumImportLvL = 10

type Imports struct {
	Path  string
	Level int
}
type ConfigLoader struct {
	viper            *viper.Viper
	atmosConfig      schema.AtmosConfiguration
	configFound      bool
	debug            bool
	AtmosConfigPaths []string
}

func NewConfigLoader() *ConfigLoader {
	return &ConfigLoader{
		viper: viper.New(),
	}
}

//go:embed atmos.yaml
var embeddedConfigData []byte

// LoadConfig initiates the configuration loading process based on the defined flowchart.
func (cl *ConfigLoader) LoadConfig(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {

	logsLevelEnvVar := os.Getenv("ATMOS_LOGS_LEVEL")
	if logsLevelEnvVar == u.LogLevelDebug || logsLevelEnvVar == u.LogLevelTrace {
		cl.debug = true
	}
	// Initialize Viper
	cl.viper.SetConfigType("yaml")
	cl.viper.SetTypeByDefaultValue(true)

	// Load Atmos Schema Defaults
	err := cl.loadSchemaDefaults()
	if err != nil {
		return cl.atmosConfig, err
	}

	// Load Embedded Config
	err = cl.loadEmbeddedConfig()
	if err != nil {
		return cl.atmosConfig, err
	}

	// Deep Merge Schema Defaults and Embedded Config
	err = cl.deepMergeConfig()
	if err != nil {
		return cl.atmosConfig, err
	}

	// Check if --config is provided via cmd args os.args
	if configAndStacksInfo.AtmosConfigPathFromArg != nil {
		if err := cl.loadExplicitConfigs(configAndStacksInfo.AtmosConfigPathFromArg); err != nil {
			return cl.atmosConfig, err
		}
		err = cl.deepMergeConfig()
		if err != nil {
			return cl.atmosConfig, err
		}
		if err := cl.processConfigImports(); err != nil {
			cl.debugLogging(err.Error())
		}
		err = cl.deepMergeConfig()
		if err != nil {
			return cl.atmosConfig, err
		}
		return cl.atmosConfig, err

	}

	// Load system directory configurations
	err = cl.loadSystemConfig()
	if err != nil {
		cl.debugLogging(fmt.Sprintf("Failed to load system directory configurations: %v", err))
	} else {
		if err := cl.processConfigImports(); err != nil {
			cl.debugLogging(fmt.Sprintf("Failed to process imports after system directory configurations: %s", err.Error()))
		}
	}

	// Load  user-specific, and other configurations
	if err := cl.loadSystemAndUserConfigs(configAndStacksInfo); err != nil {
		return cl.atmosConfig, err
	}

	// Stage 2: Discover Additional Configurations
	if err := cl.discoverAdditionalConfigs(configAndStacksInfo); err != nil {
		return cl.atmosConfig, err
	}

	// Stage 3: Apply User Preferences
	if err := cl.applyUserPreferences(); err != nil {
		return cl.atmosConfig, err
	}

	// Finalize Merged Config
	if err := cl.finalizeConfig(); err != nil {
		return cl.atmosConfig, err
	}

	// Process Imports and Deep Merge
	if err := cl.processConfigImports(); err != nil {
		return cl.atmosConfig, err
	}

	// Process command-line arguments and store configurations
	if err := cl.processAdditionalConfigs(configAndStacksInfo, processStacks); err != nil {
		return cl.atmosConfig, err
	}

	// Final checks and path conversions
	if err := cl.finalChecks(); err != nil {
		return cl.atmosConfig, err
	}

	cl.atmosConfig.Initialized = true
	return cl.atmosConfig, nil
}

// loadSchemaDefaults sets the default configuration values.
func (cl *ConfigLoader) loadSchemaDefaults() error {
	cl.viper.SetDefault("components.helmfile.use_eks", true)
	cl.viper.SetDefault("components.terraform.append_user_agent", fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version))
	j, err := json.Marshal(defaultCliConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal default CLI config: %w", err)
	}

	reader := bytes.NewReader(j)
	err = cl.viper.MergeConfig(reader)
	if err != nil {
		return fmt.Errorf("failed to merge schema defaults: %w", err)
	}
	return nil
}

// loadEmbeddedConfig loads the embedded atmos.yaml configuration.
func (cl *ConfigLoader) loadEmbeddedConfig() error {
	// Create a reader from the embedded YAML data
	reader := bytes.NewReader(embeddedConfigData)

	// Merge the embedded configuration into Viper
	if err := cl.viper.MergeConfig(reader); err != nil {
		return fmt.Errorf("failed to merge embedded config: %w", err)
	}

	return nil
}

// deepMergeConfig merges the loaded configurations.
func (cl *ConfigLoader) deepMergeConfig() error {
	return cl.viper.Unmarshal(&cl.atmosConfig)
}

// loadExplicitConfigs handles the loading of configurations provided via --config.
func (cl *ConfigLoader) loadExplicitConfigs(configPathsArgs []string) error {
	if configPathsArgs == nil {
		return fmt.Errorf("No config paths provided. Please provide a list of config paths using the --config flag.")
	}
	var configPaths []string
	//get all --config values from command line arguments
	for _, configPath := range configPathsArgs {
		configPath, err := filepath.Abs(configPath)
		if err != nil {
			return fmt.Errorf("failed to convert config path to absolute path: %w", err)
		}
		configPath = filepath.ToSlash(configPath)
		// check path exist
		info, err := os.Stat(configPath)
		if err != nil && err == os.ErrNotExist {
			return fmt.Errorf("config file %s does not exist", configPath)
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			paths, err := cl.SearchAtmosConfigFileDir(configPath)
			if err != nil {
				cl.debugLogging(fmt.Sprintf("Failed to find config file in directory '%s'.", configPath))
				continue
			}
			configPaths = append(configPaths, paths...)

		} else {
			path, exist := cl.SearchConfigFilePath(configPath)
			if !exist {
				cl.debugLogging(fmt.Sprintf("Failed to find config file in path '%s'.", configPath))
				continue
			}
			configPaths = append(configPaths, path)
		}

	}
	if configPaths == nil {
		return fmt.Errorf("config paths can not be found. Please provide a list of config paths using the --config flag.")
	}
	err := cl.MergePathsViber(configPaths)
	if err != nil {
		return fmt.Errorf("failed to merge explicit config files: %w", err)
	}
	cl.atmosConfig.CliConfigPath = ConnectPaths(cl.AtmosConfigPaths)

	return nil
}

// loadSystemAndUserConfigs loads system and user-specific configuration files.
func (cl *ConfigLoader) loadSystemAndUserConfigs(configAndStacksInfo schema.ConfigAndStacksInfo) error {

	// Load user-specific configurations using xdg.ConfigHome
	userConfigFile := filepath.Join(xdg.ConfigHome, CliConfigFileName)
	found, configPath, err := processConfigFile(cl.atmosConfig, userConfigFile, cl.viper)
	if err != nil {
		return err
	}
	if found {
		cl.configFound = true
		cl.atmosConfig.CliConfigPath = configPath
	}

	// Load current directory configuration
	configFilePathCwd, err := os.Getwd()
	if err != nil {
		return err
	}
	configFileCwd := filepath.Join(configFilePathCwd, CliConfigFileName)
	found, configPath, err = processConfigFile(cl.atmosConfig, configFileCwd, cl.viper)
	if err != nil {
		return err
	}
	if found {
		cl.configFound = true
		cl.atmosConfig.CliConfigPath = configPath
	}

	// Load Terraform provider specified path
	if configAndStacksInfo.AtmosCliConfigPath != "" {
		configFileTfProvider := filepath.Join(configAndStacksInfo.AtmosCliConfigPath, CliConfigFileName)
		found, configPath, err := processConfigFile(cl.atmosConfig, configFileTfProvider, cl.viper)
		if err != nil {
			return err
		}
		if found {
			cl.configFound = true
			cl.atmosConfig.CliConfigPath = configPath
		}
	}

	return nil
}

// discoverAdditionalConfigs discovers and loads additional configuration files.
func (cl *ConfigLoader) discoverAdditionalConfigs(configAndStacksInfo schema.ConfigAndStacksInfo) error {
	// Stage 2: Discover Additional Configurations
	return cl.stageDiscoverAdditionalConfigs(configAndStacksInfo)
}

// stageDiscoverAdditionalConfigs handles Stage 2: Discover Additional Configurations as per the flowchart.
func (cl *ConfigLoader) stageDiscoverAdditionalConfigs(configAndStacksInfo schema.ConfigAndStacksInfo) error {
	// 1. Check ATMOS_CLI_CONFIG_PATH ENV
	if atmosCliConfigPathEnv := os.Getenv("ATMOS_CLI_CONFIG_PATH"); atmosCliConfigPathEnv != "" {
		u.LogTrace(cl.atmosConfig, fmt.Sprintf("Checking ATMOS_CLI_CONFIG_PATH: %s", cl.atmosConfig.CliConfigPath))
		found, paths, err := cl.loadConfigsFromPath(atmosCliConfigPathEnv)
		if err != nil {
			return fmt.Errorf("error loading configs from ATMOS_CLI_CONFIG_PATH: %w", err)
		}
		if found {
			cl.atmosConfig.CliConfigPath = strings.Join(paths, string(os.PathListSeparator))
			u.LogTrace(cl.atmosConfig, fmt.Sprintf("Updated ATMOS_CLI_CONFIG_PATH with absolute paths: %s", cl.atmosConfig.CliConfigPath))
			// Process Imports and Deep Merge
			if err := cl.processConfigImports(); err != nil {
				return fmt.Errorf("error processing imports after ATMOS_CLI_CONFIG_PATH: %w", err)
			}
			return nil
		}
	}

	// 2. Check Git Repository Root
	gitRepoRoot, err := getGitRepoRoot()
	if err != nil {
		u.LogWarning(cl.atmosConfig, fmt.Sprintf("Failed to determine Git repository root: %v", err))
	} else if gitRepoRoot != "" {
		u.LogTrace(cl.atmosConfig, fmt.Sprintf("Git repository root found: %s", gitRepoRoot))
		found, paths, err := cl.loadConfigsFromPath(gitRepoRoot)
		if err != nil {
			return fmt.Errorf("error loading configs from Git repository root: %w", err)
		}
		if found {
			cl.atmosConfig.CliConfigPath = strings.Join(paths, string(os.PathListSeparator))
			u.LogTrace(cl.atmosConfig, fmt.Sprintf("Updated ATMOS_CLI_CONFIG_PATH with Git repo config paths: %s", cl.atmosConfig.CliConfigPath))
			// Process Imports and Deep Merge
			if err := cl.processConfigImports(); err != nil {
				return fmt.Errorf("error processing imports after Git repo root: %w", err)
			}
			return nil
		}
	}

	// 3. Check Current Working Directory (CWD)
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("unable to get current working directory: %w", err)
	}
	u.LogTrace(cl.atmosConfig, fmt.Sprintf("Checking Current Working Directory: %s", cwd))
	found, paths, err := cl.loadConfigsFromPath(cwd)
	if err != nil {
		return fmt.Errorf("error loading configs from CWD: %w", err)
	}
	if found {
		cl.atmosConfig.CliConfigPath = strings.Join(paths, string(os.PathListSeparator))
		u.LogTrace(cl.atmosConfig, fmt.Sprintf("Updated ATMOS_CLI_CONFIG_PATH with CWD config paths: %s", cl.atmosConfig.CliConfigPath))
		// Process Imports and Deep Merge
		if err := cl.processConfigImports(); err != nil {
			return fmt.Errorf("error processing imports after CWD: %w", err)
		}
		return nil
	}

	// 4. No configuration found in Stage 2
	u.LogTrace(cl.atmosConfig, "No configuration found in Stage 2: Discover Additional Configurations")
	return nil
}

// loadConfigsFromPath loads configuration files from the specified path.
// It returns a boolean indicating if any configs were found and loaded,
// a slice of loaded config paths, and an error if any.
func (cl *ConfigLoader) loadConfigsFromPath(path string) (bool, []string, error) {
	var loadedPaths []string

	// Define possible config files/directories to look for
	configFiles := []string{
		"atmos.yaml",
		".atmos.yaml",
		"atmos.d",
		".atmos.d",
		".github/atmos.yaml",
	}

	for _, cfg := range configFiles {
		fullPath := filepath.Join(path, cfg)
		info, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // Skip if the file/directory doesn't exist
			}
			return false, loadedPaths, fmt.Errorf("error accessing config path (%s): %w", fullPath, err)
		}

		if info.IsDir() {
			// Load all YAML files within the directory recursively
			err := filepath.Walk(fullPath, func(p string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() && (filepath.Ext(p) == ".yaml" || filepath.Ext(p) == ".yml") {
					found, cfgPath, err := ProcessConfigFile(cl.atmosConfig, p, cl.viper)
					if err != nil {
						return fmt.Errorf("error processing config file (%s): %w", p, err)
					}
					if found {
						loadedPaths = append(loadedPaths, cfgPath)
						u.LogTrace(cl.atmosConfig, fmt.Sprintf("Loaded config file: %s", cfgPath))
					}
				}
				return nil
			})
			if err != nil {
				return false, loadedPaths, fmt.Errorf("error walking through config directory (%s): %w", fullPath, err)
			}
		} else {
			// Load the single YAML file
			found, cfgPath, err := ProcessConfigFile(cl.atmosConfig, fullPath, cl.viper)
			if err != nil {
				return false, loadedPaths, fmt.Errorf("error processing config file (%s): %w", fullPath, err)
			}
			if found {
				loadedPaths = append(loadedPaths, cfgPath)
				u.LogTrace(cl.atmosConfig, fmt.Sprintf("Loaded config file: %s", cfgPath))
			}
		}
	}

	if len(loadedPaths) > 0 {
		return true, loadedPaths, nil
	}
	return false, loadedPaths, nil
}

// getGitRepoRoot attempts to find the Git repository root by traversing up the directory tree.
func getGitRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("unable to get current working directory: %w", err)
	}

	for {
		gitDir := filepath.Join(cwd, ".git")
		info, err := os.Stat(gitDir)
		if err == nil && info.IsDir() {
			return cwd, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("error accessing .git directory (%s): %w", gitDir, err)
		}

		parent := filepath.Dir(cwd)
		if parent == cwd {
			// Reached the root of the filesystem
			break
		}
		cwd = parent
	}

	return "", nil // Git repository root not found
}

// applyUserPreferences applies user-specific configuration preferences.
// applyUserPreferences applies user-specific configuration preferences.
func (cl *ConfigLoader) applyUserPreferences() error {
	configFile := filepath.Join(xdg.ConfigHome, "atmos", "atmos.yaml")
	found, configPath, err := processConfigFile(cl.atmosConfig, configFile, cl.viper)
	if err != nil {
		return err
	}
	if found {
		cl.atmosConfig.CliConfigPath = configPath
		return cl.viper.MergeInConfig()
	}

	homeDir, err := GetHomeDir()
	if err != nil {
		return err
	}
	userConfigPaths := []string{
		filepath.Join(xdg.ConfigHome, "atmos", "atmos.yaml"),
		filepath.Join(homeDir, ".atmos", "atmos.yaml"),
	}
	for _, path := range userConfigPaths {
		found, configPath, err := processConfigFile(cl.atmosConfig, path, cl.viper)
		if err != nil {
			return err
		}
		if found {
			cl.atmosConfig.CliConfigPath = configPath
			return cl.viper.MergeInConfig()
		}
	}

	return nil
}

// finalizeConfig finalizes the merged configuration and outputs it.
func (cl *ConfigLoader) finalizeConfig() error {
	if !cl.configFound {
		logsLevelEnvVar := os.Getenv("ATMOS_LOGS_LEVEL")
		if logsLevelEnvVar == u.LogLevelDebug || logsLevelEnvVar == u.LogLevelTrace {
			u.LogTrace(cl.atmosConfig, "atmos.yaml' CLI config was not found.\n"+
				"Refer to https://atmos.tools/cli/configuration\n"+
				"Using the default CLI config:\n\n")
			if err := u.PrintAsYAMLToFileDescriptor(cl.atmosConfig, defaultCliConfig); err != nil {
				return err
			}
			fmt.Println()
		}

		j, err := json.Marshal(defaultCliConfig)
		if err != nil {
			return err
		}

		reader := bytes.NewReader(j)
		if err := cl.viper.MergeConfig(reader); err != nil {
			return err
		}
	}

	if err := cl.viper.Unmarshal(&cl.atmosConfig); err != nil {
		return err
	}

	return processEnvVars(&cl.atmosConfig)
}

// processConfigImports processes any imported configuration files.
func (cl *ConfigLoader) processConfigImports() error {
	if len(cl.atmosConfig.Import) > 0 {
		importPaths := cl.atmosConfig.Import
		tempDir, err := os.MkdirTemp("", "atmos-import-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)
		resolvedPaths, err := cl.processImports(cl.atmosConfig.Import, tempDir, 1, MaximumImportLvL)
		if err != nil {
			return err
		}

		for _, configPath := range resolvedPaths {
			found, err := cl.loadConfigFileViber(cl.atmosConfig, configPath, cl.viper)
			if err != nil {
				u.LogWarning(cl.atmosConfig, fmt.Sprintf("failed to merge configuration from '%s': %v", configPath, err))
				continue
			}
			if found {
				u.LogTrace(cl.atmosConfig, fmt.Sprintf("merge import paths: %v", resolvedPaths))
			}
			cl.deepMergeConfig()

		}
		cl.atmosConfig.Import = importPaths
	}
	return nil
}

// processAdditionalConfigs handles command-line arguments and store configurations.
func (cl *ConfigLoader) processAdditionalConfigs(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) error {
	if err := processCommandLineArgs(&cl.atmosConfig, configAndStacksInfo); err != nil {
		return err
	}

	if err := processStoreConfig(&cl.atmosConfig); err != nil {
		return err
	}

	if configAndStacksInfo.AtmosBasePath != "" {
		cl.atmosConfig.BasePath = configAndStacksInfo.AtmosBasePath
	}

	if cl.atmosConfig.Components.Terraform.AppendUserAgent == "" {
		cl.atmosConfig.Components.Terraform.AppendUserAgent = fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version)
	}

	if err := checkConfig(cl.atmosConfig); err != nil {
		return err
	}

	// Convert paths to absolute
	if err := cl.convertPaths(); err != nil {
		return err
	}

	if processStacks {
		if err := cl.processStacks(configAndStacksInfo); err != nil {
			return err
		}
	}

	return nil
}

// finalChecks performs final validation and path conversions.
func (cl *ConfigLoader) finalChecks() error {
	// Additional final checks can be implemented here
	return nil
}
func (cl *ConfigLoader) loadSystemConfig() error {
	configSysPath, found := cl.getSystemConfigPath()
	if found {
		cl.atmosConfig.CliConfigPath = ConnectPaths([]string{configSysPath})
		cl.debugLogging(fmt.Sprintf("Found system config file at: %s", configSysPath))
		ok, err := cl.loadConfigFileViber(cl.atmosConfig, configSysPath, cl.viper)
		if err != nil {
			return err
		}
		if ok {
			u.LogTrace(cl.atmosConfig, fmt.Sprintf(" system config file merged: %s", configSysPath))
			err := cl.deepMergeConfig()
			if err != nil {
				return err
			}
			return nil
		}
		return nil

	} else {
		cl.debugLogging(fmt.Sprintf("No system config file found"))
	}
	return nil
}

// Helper functions
// getSystemConfigPath returns the first found system configuration directory.
func (cl *ConfigLoader) getSystemConfigPath() (string, bool) {
	var systemFilePaths []string
	if runtime.GOOS == "windows" {
		programData := os.Getenv("PROGRAMDATA")
		if programData != "" {
			systemFilePaths = append(systemFilePaths, filepath.Join(programData, CliConfigFileName))
		}
	} else {
		systemFilePaths = append(systemFilePaths, filepath.Join(SystemDirConfigFilePath, CliConfigFileName), filepath.Join("/etc", CliConfigFileName))
	}
	var configFilesPath string
	for _, filePath := range systemFilePaths {
		resultPath, exist := cl.SearchConfigFilePath(filePath)
		if !exist {
			cl.debugLogging(fmt.Sprintf("Failed to find config file on system path '%s'", filePath))
			continue
		}
		if exist {
			cl.debugLogging(fmt.Sprintf("Found config file on system path '%s': %s", filePath, resultPath))
			configFilesPath = resultPath
			return configFilesPath, true
		}

	}
	return configFilesPath, false

}
func (cl *ConfigLoader) convertPaths() error {
	// Convert stacks base path to absolute path
	stacksBasePath := filepath.Join(cl.atmosConfig.BasePath, cl.atmosConfig.Stacks.BasePath)
	stacksBaseAbsPath, err := filepath.Abs(stacksBasePath)
	if err != nil {
		return err
	}
	cl.atmosConfig.StacksBaseAbsolutePath = stacksBaseAbsPath

	// Convert included stack paths
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, cl.atmosConfig.Stacks.IncludedPaths)
	if err != nil {
		return err
	}
	cl.atmosConfig.IncludeStackAbsolutePaths = includeStackAbsPaths

	// Convert excluded stack paths
	excludeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, cl.atmosConfig.Stacks.ExcludedPaths)
	if err != nil {
		return err
	}
	cl.atmosConfig.ExcludeStackAbsolutePaths = excludeStackAbsPaths

	// Convert terraform directory path
	terraformBasePath := filepath.Join(cl.atmosConfig.BasePath, cl.atmosConfig.Components.Terraform.BasePath)
	terraformDirAbsPath, err := filepath.Abs(terraformBasePath)
	if err != nil {
		return err
	}
	cl.atmosConfig.TerraformDirAbsolutePath = terraformDirAbsPath

	// Convert helmfile directory path
	helmfileBasePath := filepath.Join(cl.atmosConfig.BasePath, cl.atmosConfig.Components.Helmfile.BasePath)
	helmfileDirAbsPath, err := filepath.Abs(helmfileBasePath)
	if err != nil {
		return err
	}
	cl.atmosConfig.HelmfileDirAbsolutePath = helmfileDirAbsPath

	return nil
}

func (cl *ConfigLoader) processStacks(configAndStacksInfo schema.ConfigAndStacksInfo) error {
	stackConfigFilesAbsolutePaths, stackConfigFilesRelativePaths, stackIsPhysicalPath, err := FindAllStackConfigsInPathsForStack(
		cl.atmosConfig,
		configAndStacksInfo.Stack,
		cl.atmosConfig.IncludeStackAbsolutePaths,
		cl.atmosConfig.ExcludeStackAbsolutePaths,
	)
	if err != nil {
		return err
	}

	if len(stackConfigFilesAbsolutePaths) < 1 {
		j, err := u.ConvertToYAML(cl.atmosConfig.IncludeStackAbsolutePaths)
		if err != nil {
			return err
		}
		errorMessage := fmt.Sprintf("\nno stack manifests found in the provided "+
			"paths:\n%s\n\nCheck if `base_path`, 'stacks.base_path', 'stacks.included_paths' and 'stacks.excluded_paths' are correctly set in CLI config "+
			"files or ENV vars.", j)
		return fmt.Errorf(errorMessage)
	}

	cl.atmosConfig.StackConfigFilesAbsolutePaths = stackConfigFilesAbsolutePaths
	cl.atmosConfig.StackConfigFilesRelativePaths = stackConfigFilesRelativePaths

	if stackIsPhysicalPath {
		u.LogTrace(cl.atmosConfig, fmt.Sprintf("\nThe stack '%s' matches the stack manifest %s\n",
			configAndStacksInfo.Stack,
			stackConfigFilesRelativePaths[0]),
		)
		cl.atmosConfig.StackType = "Directory"
	} else {
		cl.atmosConfig.StackType = "Logical"
	}

	return nil
}
func (cl *ConfigLoader) debugLogging(msg string) {
	if cl.debug {
		u.LogTrace(cl.atmosConfig, msg)
	}
}
func (cl *ConfigLoader) SearchAtmosConfigFileDir(dirPath string) ([]string, error) {
	// Determine if dirPath is a directory or a pattern
	isDir := false
	if stat, err := os.Stat(dirPath); err == nil && stat.IsDir() {
		isDir = true
	}
	// Normalize dirPath to include patterns if it's a directory
	var patterns []string

	if isDir {
		// For directories, append a default pattern to match all files
		patterns = []string{filepath.Join(dirPath, "*.yaml"), filepath.Join(dirPath, "*.yml")}

	} else {

		ext := filepath.Ext(dirPath)
		if ext == "" {
			impYaml := dirPath + ".yaml"
			impYml := dirPath + ".yml"
			patterns = append(patterns, impYaml, impYml)
		} else {
			patterns = append(patterns, dirPath)
		}

	}

	// sort fileExtension in ascending order to prioritize  .yaml over .yml
	var atmosFilePaths []string
	for _, pattern := range patterns {
		filePaths, err := u.GetGlobMatches(pattern)
		if err == nil {
			atmosFilePaths = append(atmosFilePaths, filePaths...)
		}

	}
	if atmosFilePaths == nil {
		return nil, fmt.Errorf("no files matching name `atmos` with extensions [.yaml,.yml]  found in the provided directory: %s", dirPath)
	}
	var atmosFilePathsABS []string
	for _, path := range atmosFilePaths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			u.LogWarning(cl.atmosConfig, fmt.Sprintf("error getting absolute path for file '%s'. %v", path, err))
			continue
		}
		atmosFilePathsABS = append(atmosFilePathsABS, absPath)
	}

	atmosFilePathsABS = cl.detectPriorityFiles(atmosFilePathsABS)
	atmosFilePathsABS = cl.sortFilesByDepth(atmosFilePathsABS)
	return atmosFilePathsABS, nil

}
func (cl *ConfigLoader) SearchConfigFilePath(path string) (string, bool) {
	// Check if the provided path has a file extension and the file exists
	if filepath.Ext(path) != "" {
		return path, u.FileExists(path)
	}

	// Check the possible config file extensions
	configExtensions := []string{u.YamlFileExtension, u.YmlFileExtension}
	// sort configExtensions in ascending order to prioritize  .yaml over .yml
	for _, ext := range configExtensions {
		filePath := path + ext
		if u.FileExists(filePath) {
			return filePath, true
		}
	}

	return "", false
}

// detectPriorityFiles detects which files will have priority .  yaml win over yml if file has same path
func (cl *ConfigLoader) detectPriorityFiles(files []string) []string {
	// Map to store the highest priority file for each base name
	priorityMap := make(map[string]string)

	// Iterate through the list of files
	for _, file := range files {
		dir := filepath.Dir(file)
		baseName := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		ext := filepath.Ext(file)

		// Construct a unique key for the folder + base name
		key := filepath.Join(dir, baseName)

		// Assign .yaml as priority if it exists, or fallback to .yml
		if existingFile, exists := priorityMap[key]; exists {
			if ext == ".yaml" {
				priorityMap[key] = file // Replace .yml with .yaml
			} else if ext == ".yml" && strings.HasSuffix(existingFile, ".yaml") {
				continue // Keep .yaml priority
			}
		} else {
			priorityMap[key] = file // First occurrence, add file
		}
	}

	// Collect results from the map
	var result []string
	for _, file := range priorityMap {
		result = append(result, file)
	}

	return result
}

// sortFilesByDepth sorts a list of file paths by the depth of their directories.
// Files with the same depth are sorted alphabetically by name.
func (cl *ConfigLoader) sortFilesByDepth(files []string) []string {
	// Precompute depths for each file path
	type fileDepth struct {
		path  string
		depth int
	}

	var fileDepths []fileDepth
	for _, file := range files {
		cleanPath := filepath.Clean(file)
		depth := len(strings.Split(filepath.ToSlash(filepath.Dir(cleanPath)), "/"))
		fileDepths = append(fileDepths, fileDepth{path: cleanPath, depth: depth})
	}

	// Sort by depth, and alphabetically by name as a tiebreaker
	sort.Slice(fileDepths, func(i, j int) bool {
		if fileDepths[i].depth == fileDepths[j].depth {
			// If depths are the same, compare file names alphabetically
			return fileDepths[i].path < fileDepths[j].path
		}
		// Otherwise, compare by depth
		return fileDepths[i].depth < fileDepths[j].depth
	})

	// Extract sorted paths
	sortedFiles := make([]string, len(fileDepths))
	for i, fd := range fileDepths {
		sortedFiles[i] = fd.path
	}

	return sortedFiles
}

func (cl *ConfigLoader) loadConfigFileViber(
	atmosConfig schema.AtmosConfiguration,
	path string,
	v *viper.Viper,
) (bool, error) {
	reader, err := os.Open(path)
	if err != nil {
		return false, err
	}

	defer func(reader *os.File) {
		err := reader.Close()
		if err != nil {
			u.LogWarning(atmosConfig, fmt.Sprintf("error closing file '"+path+"'. "+err.Error()))
		}
	}(reader)

	err = v.MergeConfig(reader)
	if err != nil {
		return false, err
	}

	return true, nil
}

// ConnectPaths joins multiple paths using the OS-specific path list separator.
func ConnectPaths(paths []string) string {
	return strings.Join(paths, string(os.PathListSeparator))
}
func (cl *ConfigLoader) MergePathsViber(configPaths []string) error {
	for _, configPath := range configPaths {
		found, err := cl.loadConfigFileViber(cl.atmosConfig, configPath, cl.viper)
		if err != nil {
			u.LogWarning(cl.atmosConfig, fmt.Sprintf("failed to merge configuration from '%s': %v", configPath, err))
			continue
		}
		if found {
			cl.configFound = true
			cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, configPath)
		}
	}
	return nil
}

// findGitTopLevel finds the top-level directory of a Git repository
func findGitTopLevel(startDir string) (string, error) {
	// Open the current directory as a billy filesystem
	fs := osfs.New(startDir)
	// Create a filesystem-based storage for Git (this allows us to traverse Git config)
	storer := filesystem.NewStorage(fs, nil)
	// Create a new repository from the storage
	repo, err := git.Open(storer, fs)
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	// Get the repository's top-level directory from the configuration
	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}
	return worktree.Filesystem.Root(), nil
}
