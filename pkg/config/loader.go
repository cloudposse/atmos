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
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const MaximumImportLvL = 10

type Imports struct {
	Path  string
	Level int
}
type ConfigLoader struct {
	viper            *viper.Viper
	atmosConfig      schema.AtmosConfiguration
	AtmosConfigPaths []string
	LogsLevel        string
}

func NewConfigLoader() *ConfigLoader {
	return &ConfigLoader{
		viper: viper.New(),
	}
}

//go:embed atmos.yaml
var embeddedConfigData []byte

// LoadConfig initiates the configuration loading process based on the defined flowchart.
func (cl *ConfigLoader) LoadConfig(configAndStacksInfo schema.ConfigAndStacksInfo) (schema.AtmosConfiguration, error) {
	u.LogDebug(fmt.Sprintf("start process loading config..."))
	// We want the editorconfig color by default to be true
	cl.atmosConfig.Validate.EditorConfig.Color = true

	// Initialize Viper
	cl.viper.SetConfigType("yaml")
	cl.viper.SetTypeByDefaultValue(true)
	cl.viper.SetDefault("components.helmfile.use_eks", true)
	cl.viper.SetDefault("components.terraform.append_user_agent", fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version))
	cl.viper.SetDefault("settings.inject_github_token", true)

	// Load Embedded Config
	err := cl.loadEmbeddedConfig()
	if err != nil {
		return cl.atmosConfig, err
	}

	// Deep Merge Schema Defaults and Embedded Config
	err = cl.deepMergeConfig()
	if err != nil {
		return cl.atmosConfig, err
	}

	// Check if --config is provided via cmd args os.args
	if len(configAndStacksInfo.AtmosConfigPathFromArg) > 0 {
		if err := cl.loadExplicitConfigs(configAndStacksInfo.AtmosConfigPathFromArg); err != nil {
			return cl.atmosConfig, fmt.Errorf("Failed to load --config from provided paths: %v", err)
		}
		return cl.atmosConfig, err

	}

	// Load system directory configurations
	err = cl.loadSystemConfig()
	if err != nil {
		u.LogDebug(fmt.Sprintf("Failed to load system directory configurations: %v", err))
	}
	// Stage 2: Discover Additional Configurations
	if err := cl.stageDiscoverAdditionalConfigs(); err != nil {
		return cl.atmosConfig, err
	}

	// Stage 3: Apply User Preferences
	cl.applyUserPreferences()

	return cl.atmosConfig, nil
}

// loadEmbeddedConfig loads the embedded atmos.yaml configuration.
func (cl *ConfigLoader) loadEmbeddedConfig() error {
	u.LogDebug(fmt.Sprintf("start process loading embedded config..."))
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
	u.LogDebug("Loading --config configs from provided paths")
	if configPathsArgs == nil {
		return fmt.Errorf("No config paths provided. Please provide a list of config paths using the --config flag.")
	}
	var configPaths []string
	// get all --config values from command line arguments
	for _, configPath := range configPathsArgs {
		paths, err := cl.SearchAtmosConfigFileDir(configPath)
		if err != nil {
			u.LogDebug(fmt.Sprintf("Failed to find config file in directory '%s'.", configPath))
			continue
		}
		configPaths = append(configPaths, paths...)
		cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, configPath)
	}

	if configPaths == nil {
		return fmt.Errorf("config paths can not be found. Please provide a list of config paths using the --config flag.")
	}
	for _, configPath := range configPaths {
		u.LogDebug(fmt.Sprintf("Found config file: %s", configPath))
		ok, err := cl.loadConfigFileViber(cl.atmosConfig, configPath, cl.viper)
		if !ok || err != nil {
			u.LogDebug(fmt.Sprintf("error processing config file (%s): %s", configPath, err.Error()))
			continue
		}
		err = cl.deepMergeConfig()
		if err != nil {
			u.LogDebug(fmt.Sprintf("error merge config file (%s): %s", configPath, err.Error()))
			continue
		}
		u.LogDebug(fmt.Sprintf("atmos merged config file %s", configPath))

		// Process Imports and Deep Merge
		if err := cl.processConfigImports(); err != nil {
			u.LogDebug(fmt.Sprintf("error processing imports after config file (file %s):error  %s", configPath, err.Error()))
			continue
		}
		err = cl.deepMergeConfig()
		if err != nil {
			u.LogDebug(fmt.Sprintf("error merge config file (%s): %s", configPath, err.Error()))
		}
	}

	return nil
}

func parseArraySeparator(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func (cl *ConfigLoader) loadAtmosConfigFromEnv(atmosCliConfigEnv string) error {
	atmosCliConfigEnvPaths := parseArraySeparator(atmosCliConfigEnv)
	if len(atmosCliConfigEnvPaths) == 0 {
		return fmt.Errorf("ATMOS_CLI_CONFIG_PATH is not a valid paths")
	}
	if atmosCliConfigEnv == AtmosYamlFuncGitRoot {
		gitRoot, err := u.GetGitRoot()
		if err != nil {
			return fmt.Errorf("failed to resolve base path from `!repo-root` Git root error: %w", err)
		}
		u.LogDebug(fmt.Sprintf("ATMOS_CLI_CONFIG_PATH !repo-root Git root dir : %s", gitRoot))
		atmosCliConfigEnv = gitRoot
	}
	var atmosFilePaths []string
	for _, configPath := range atmosCliConfigEnvPaths {
		atmosFilePath, err := cl.getPathAtmosCLIConfigPath(configPath)
		if err != nil {
			u.LogDebug(fmt.Sprintf("Failed to find config file in '%s' . error %s", atmosFilePaths, err.Error()))
			continue
		}
		atmosFilePaths = append(atmosFilePaths, atmosFilePath...)
	}
	if len(atmosFilePaths) == 0 {
		return fmt.Errorf("Failed to find config files in ATMOS_CLI_CONFIG_PATH '%s'", atmosCliConfigEnv)
	}
	for _, atmosFilePath := range atmosFilePaths {
		u.LogDebug(fmt.Sprintf("Found ATMOS_CLI_CONFIG_PATH: %s", atmosFilePath))
		ok, err := cl.loadConfigFileViber(cl.atmosConfig, atmosFilePath, cl.viper)
		if !ok || err != nil {
			u.LogDebug(fmt.Sprintf("error processing config file (%s): %s", atmosFilePath, err.Error()))
			continue
		}
		err = cl.deepMergeConfig()
		if err != nil {
			u.LogDebug(fmt.Sprintf("error merge config file (%s): %s", atmosFilePath, err.Error()))
			continue
		}
		u.LogDebug(fmt.Sprintf("atmos merged config file %s", atmosFilePath))

		// Process Imports and Deep Merge
		if err := cl.processConfigImports(); err != nil {
			u.LogDebug(fmt.Sprintf("error processing imports after ATMOS_CLI_CONFIG_PATH (file %s):error  %s", atmosFilePath, err.Error()))
			continue
		}
		err = cl.deepMergeConfig()
		if err != nil {
			u.LogDebug(fmt.Sprintf("error merge config file (%s): %s", atmosFilePath, err.Error()))
		}
	}
	return nil
}

func (cl *ConfigLoader) getPathAtmosCLIConfigPath(atmosCliConfigPathEnv string) ([]string, error) {
	isDir, err := u.IsDirectory(atmosCliConfigPathEnv)
	if err != nil {
		if err == os.ErrNotExist {
			return nil, fmt.Errorf("error ATMOS_CLI_CONFIG_PATH: %w", err)
		}
		return nil, fmt.Errorf("error ATMOS_CLI_CONFIG_PATH: %w", err)
	}
	if !isDir {
		return nil, fmt.Errorf("ATMOS_CLI_CONFIG_PATH is not a directory")
	}
	var atmosFoundFilePaths []string
	searchFilePath := filepath.Join(filepath.FromSlash(atmosCliConfigPathEnv), CliConfigFileName)
	configPath, found := cl.SearchConfigFilePath(searchFilePath)
	if found {
		atmosFoundFilePaths = append(atmosFoundFilePaths, configPath)
	} else {
		u.LogDebug(fmt.Sprintf("Failed to find config file atmos in directory '%s'.", atmosCliConfigPathEnv))
	}
	searchDir := filepath.Join(filepath.FromSlash(atmosCliConfigPathEnv), "atmos.d/**/*")
	foundPaths, err := cl.SearchAtmosConfigFileDir(searchDir)
	if err != nil {
		u.LogDebug(fmt.Sprintf("Failed to find config file in path '%s'.error %s", searchDir, err.Error()))
	}
	if len(foundPaths) == 0 {
		u.LogDebug(fmt.Sprintf("Failed to find config file in path '%s'", searchDir))
	} else {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths...)
	}

	if len(atmosFoundFilePaths) == 0 {
		return nil, fmt.Errorf("Failed to find config files in path '%s'", atmosCliConfigPathEnv)
	}
	cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, atmosCliConfigPathEnv)

	return atmosFoundFilePaths, err
}

// stageDiscoverAdditionalConfigs handles Stage 2: Discover Additional Configurations as per the flowchart.
func (cl *ConfigLoader) stageDiscoverAdditionalConfigs() error {
	// 1. load Atmos conflagration from ATMOS_CLI_CONFIG_PATH ENV
	if atmosCliConfigPathEnv := os.Getenv("ATMOS_CLI_CONFIG_PATH"); atmosCliConfigPathEnv != "" {
		if err := cl.loadAtmosConfigFromEnv(atmosCliConfigPathEnv); err != nil {
			return err
		}
	}
	// 3. Check Current Working Directory (CWD)
	found := cl.loadWorkdirAtmosConfig()
	if found {
		u.LogDebug("Workdir atmos loaded")
		return nil
	}
	// 2. Check Git Repository Root
	found = cl.loadGitAtmosConfig()
	if found {
		u.LogDebug("Git repository root atmos loaded")
		return nil
	}

	// 4. No configuration found in Stage 2
	u.LogDebug("No configuration found in Stage 2: Discover Additional Configurations")
	return nil
}

func (cl *ConfigLoader) loadWorkdirAtmosConfig() (found bool) {
	found = false
	cwd, err := os.Getwd()
	if err != nil {
		u.LogDebug(fmt.Sprintf("unable to get current working directory: %v", err))
		return found

	}
	u.LogDebug(fmt.Sprintf("Check atmos config current working directory: %s", cwd))

	workDirAtmosConfigPath, err := cl.getWorkDirAtmosConfigPaths(cwd)
	if err != nil {
		u.LogDebug(fmt.Sprintf("Failed to get work dir atmos config paths: %v", err))
	}
	if len(workDirAtmosConfigPath) > 0 {
		for _, atmosFilePath := range workDirAtmosConfigPath {
			u.LogDebug(fmt.Sprintf("Found work dir atmos config file: %s", atmosFilePath))
			ok, err := cl.loadConfigFileViber(cl.atmosConfig, atmosFilePath, cl.viper)
			if !ok || err != nil {
				u.LogDebug(fmt.Sprintf("error processing config file (%s): %s", atmosFilePath, err.Error()))
				continue
			}
			err = cl.deepMergeConfig()
			if err != nil {
				u.LogDebug(fmt.Sprintf("error merge config file (%s): %s", atmosFilePath, err.Error()))
				continue
			}
			u.LogDebug(fmt.Sprintf("atmos merged config file %s", atmosFilePath))
			// Process Imports and Deep Merge
			if err := cl.processConfigImports(); err != nil {
				u.LogDebug(fmt.Sprintf("error processing imports after work dir (file %s):error  %s", atmosFilePath, err.Error()))
				continue
			}
			err = cl.deepMergeConfig()
			if err != nil {
				u.LogDebug(fmt.Sprintf("error merge config file (%s): %s", atmosFilePath, err.Error()))
			}
			found = true
		}
		if !found {
			u.LogDebug(fmt.Sprintf("Failed to process atmos config files in path '%s'", cwd))
		}
	}

	return found
}

func (cl *ConfigLoader) getWorkDirAtmosConfigPaths(workDir string) ([]string, error) {
	var atmosFoundFilePaths []string
	searchFilePath := filepath.Join(filepath.FromSlash(workDir), CliConfigFileName)
	configPath, found := cl.SearchConfigFilePath(searchFilePath)
	if found {
		atmosFoundFilePaths = append(atmosFoundFilePaths, configPath)
	} else {
		u.LogDebug(fmt.Sprintf("Failed to find config file atmos in work directory '%s'.", workDir))
	}
	searchDir := filepath.Join(filepath.FromSlash(workDir), "atmos.d/**/*")
	foundPaths, err := cl.SearchAtmosConfigFileDir(searchDir)
	if err != nil {
		u.LogDebug(fmt.Sprintf("Failed to find work dir atmos config file in path '%s'.error %s", searchDir, err.Error()))
	}
	if len(foundPaths) == 0 {
		u.LogDebug(fmt.Sprintf("Failed to find config file in path '%s'", searchDir))
	} else {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths...)
	}
	searchDir = filepath.Join(filepath.FromSlash(workDir), ".atmos.d/**/*")
	foundPaths, err = cl.SearchAtmosConfigFileDir(searchDir)
	if err != nil {
		u.LogDebug(fmt.Sprintf("Failed to find work dir atmos config file in path '%s'.error %s", searchDir, err.Error()))
	}
	if len(foundPaths) == 0 {
		u.LogDebug(fmt.Sprintf("Failed to find config file in path '%s'", searchDir))
	} else {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths...)
	}

	if len(atmosFoundFilePaths) == 0 {
		return nil, fmt.Errorf("Failed to find config files in path '%s'", workDir)
	}
	cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, workDir)
	return atmosFoundFilePaths, err
}

// loadGitAtmosConfig attempts to load configuration files from the Git repository root. It returns a boolean indicating if any configs were found and successfully loaded.
func (cl *ConfigLoader) loadGitAtmosConfig() (found bool) {
	found = false
	gitRoot, err := u.GetGitRoot()
	if err != nil {
		u.LogDebug(fmt.Sprintf("Failed to determine Git repository root: %v", err))
		return found
	}
	if gitRoot != "" {
		u.LogDebug(fmt.Sprintf("Git repository root found: %s", gitRoot))
		isDir, err := u.IsDirectory(gitRoot)
		if err != nil {
			if err == os.ErrNotExist {
				u.LogDebug(fmt.Sprintf("Git repository root not found: %s", gitRoot))
			}
			u.LogDebug(fmt.Sprintf("Git repository root not found: %s", gitRoot))
		}
		if isDir && err == nil {
			u.LogDebug(fmt.Sprintf("Git repository root not found: %s", gitRoot))
		}
		gitAtmosConfigPath, err := cl.getGitAtmosConfigPaths(gitRoot)
		if err != nil {
			u.LogDebug(fmt.Sprintf("Failed to get Git atmos config paths: %v", err))
		}
		if len(gitAtmosConfigPath) > 0 {
			for _, atmosFilePath := range gitAtmosConfigPath {
				u.LogDebug(fmt.Sprintf("Found Git atmos config file: %s", atmosFilePath))
				ok, err := cl.loadConfigFileViber(cl.atmosConfig, atmosFilePath, cl.viper)
				if !ok || err != nil {
					u.LogDebug(fmt.Sprintf("error processing config file (%s): %s", atmosFilePath, err.Error()))
					continue
				}
				err = cl.deepMergeConfig()
				if err != nil {
					u.LogDebug(fmt.Sprintf("error merge config file (%s): %s", atmosFilePath, err.Error()))
					continue
				}

				u.LogDebug(fmt.Sprintf("atmos merged config file %s", atmosFilePath))

				// Process Imports and Deep Merge
				if err := cl.processConfigImports(); err != nil {
					u.LogDebug(fmt.Sprintf("error processing imports after git (file %s):error  %s", atmosFilePath, err.Error()))
					continue
				}
				err = cl.deepMergeConfig()
				if err != nil {
					u.LogDebug(fmt.Sprintf("error merge config file (%s): %s", atmosFilePath, err.Error()))
				}
				found = true
			}
			if !found {
				u.LogDebug(fmt.Sprintf("Failed to process atmos config files in path '%s'", gitRoot))
			}
		}
	}
	return found
}

func (cl *ConfigLoader) getGitAtmosConfigPaths(gitRootDir string) ([]string, error) {
	var atmosFoundFilePaths []string
	searchFilePath := filepath.Join(filepath.FromSlash(gitRootDir), CliConfigFileName)
	configPath, found := cl.SearchConfigFilePath(searchFilePath)
	if found {
		atmosFoundFilePaths = append(atmosFoundFilePaths, configPath)
	} else {
		u.LogDebug(fmt.Sprintf("Failed to find config file atmos in git directory '%s'.", gitRootDir))
	}
	searchDir := filepath.Join(filepath.FromSlash(gitRootDir), "atmos.d/**/*")
	foundPaths, err := cl.SearchAtmosConfigFileDir(searchDir)
	if err != nil {
		u.LogDebug(fmt.Sprintf("Failed to find git atmos config file in path '%s'.error %s", searchDir, err.Error()))
	}
	if len(foundPaths) == 0 {
		u.LogDebug(fmt.Sprintf("Failed to find config file in path '%s'", searchDir))
	} else {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths...)
	}
	searchDir = filepath.Join(filepath.FromSlash(gitRootDir), ".atmos.d/**/*")
	foundPaths, err = cl.SearchAtmosConfigFileDir(searchDir)
	if err != nil {
		u.LogDebug(fmt.Sprintf("Failed to find git atmos config file in path '%s'.error %s", searchDir, err.Error()))
	}
	if len(foundPaths) == 0 {
		u.LogDebug(fmt.Sprintf("Failed to find config file in path '%s'", searchDir))
	} else {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths...)
	}

	if len(atmosFoundFilePaths) == 0 {
		return nil, fmt.Errorf("Failed to find config files in path '%s'", gitRootDir)
	}
	cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, gitRootDir)
	return atmosFoundFilePaths, err
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
						u.LogDebug(fmt.Sprintf("Loaded config file: %s", cfgPath))
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
				u.LogDebug(fmt.Sprintf("Loaded config file: %s", cfgPath))
			}
		}
	}

	if len(loadedPaths) > 0 {
		return true, loadedPaths, nil
	}
	return false, loadedPaths, nil
}

// applyUserPreferences applies user-specific configuration preferences.
func (cl *ConfigLoader) applyUserPreferences() {
	configPath := filepath.Join(xdg.ConfigHome, CliConfigFileName)
	atmosConfigPath, found := cl.SearchConfigFilePath(configPath)
	if found {
		ok, err := cl.loadConfigFileViber(cl.atmosConfig, atmosConfigPath, cl.viper)
		if err != nil {
			u.LogDebug(fmt.Sprintf("error processing XDG_CONFIG_HOME path: %v error : %v", configPath, err))
		}
		if ok {
			err := cl.processConfigImports()
			if err != nil {
				u.LogDebug(fmt.Sprintf("error processing imports after XDG_CONFIG_HOME atmos path: %v error : %v", atmosConfigPath, err))
			}
			u.LogDebug(fmt.Sprintf("atmos config file found on XDG_CONFIG_HOME atmos path: %v ", atmosConfigPath))
			cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, configPath)
		}
		return
	}

	userHomeDir, err := homedir.Dir()
	if err != nil {
		u.LogDebug(fmt.Sprintf("Failed to get user home directory: %v", err))
	} else {
		foundHomeDirConfig := false
		configPathHomeDIr := filepath.Join(userHomeDir, ".config", CliConfigFileName)
		atmosConfigPathHomeDir, found := cl.SearchConfigFilePath(configPathHomeDIr)
		if found {
			ok, err := cl.loadConfigFileViber(cl.atmosConfig, atmosConfigPathHomeDir, cl.viper)
			if err != nil {
				u.LogDebug(fmt.Sprintf("error processing config user HomeDir path: %s error : %v", configPathHomeDIr, err))
			}
			if ok {
				foundHomeDirConfig = true
				err := cl.processConfigImports()
				if err != nil {
					u.LogDebug(fmt.Sprintf("error processing imports after HomeDir atmos path: %s error : %v", atmosConfigPathHomeDir, err))
				}
				u.LogDebug(fmt.Sprintf("atmos config file found on HomeDir atmos path: %s ", atmosConfigPathHomeDir))
				cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, configPathHomeDIr)
			}
		}
		configPathHomeDIr = filepath.Join(userHomeDir, ".atmos", CliConfigFileName)
		atmosConfigPathHomeDir, found = cl.SearchConfigFilePath(configPathHomeDIr)
		if found {
			foundHomeDirConfig = true
			ok, err := cl.loadConfigFileViber(cl.atmosConfig, atmosConfigPathHomeDir, cl.viper)
			if err != nil {
				u.LogDebug(fmt.Sprintf("error processing config user HomeDir path: %s error : %v", configPathHomeDIr, err))
			}
			if ok {
				err := cl.processConfigImports()
				if err != nil {
					u.LogDebug(fmt.Sprintf("error processing imports after HomeDir atmos path: %s error : %v", atmosConfigPathHomeDir, err))
				}
				u.LogDebug(fmt.Sprintf("atmos config file found on HomeDir atmos path: %s ", atmosConfigPathHomeDir))
				cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, configPathHomeDIr)
			}
		}
		if foundHomeDirConfig {
			return
		}

	}
	u.LogDebug("No configuration found in user preferences")
	return
}

func (cl *ConfigLoader) loadSystemConfig() error {
	configSysPaths, found := cl.getSystemConfigPath()
	if found {
		for _, configSysPath := range configSysPaths {
			u.LogDebug(fmt.Sprintf("Found system config file at: %s", configSysPath))
			ok, err := cl.loadConfigFileViber(cl.atmosConfig, configSysPath, cl.viper)
			if err != nil {
				u.LogDebug(fmt.Sprintf("system config file merged: %s", configSysPath))
				continue
			}
			if ok {
				err := cl.deepMergeConfig()
				if err != nil {
					u.LogDebug(fmt.Sprintf("error merge config file (%s): %s", configSysPath, err.Error()))
					continue
				}
				u.LogDebug(fmt.Sprintf("atmos merged config file %s", configSysPath))

				// Process Imports and Deep Merge
				if err := cl.processConfigImports(); err != nil {
					u.LogDebug(fmt.Sprintf("error processing imports after system config  (file %s):error  %s", configSysPath, err.Error()))
					continue
				}
				err = cl.deepMergeConfig()
				if err != nil {
					u.LogDebug(fmt.Sprintf("error merge config file (%s): %s", configSysPath, err.Error()))
				}
				return nil
			}
		}
		return nil

	} else {
		u.LogDebug(fmt.Sprintf("No system config file found"))
	}
	return nil
}

// Helper functions
// getSystemConfigPath returns the first found system configuration directory.
func (cl *ConfigLoader) getSystemConfigPath() ([]string, bool) {
	var systemFilePaths []string
	if runtime.GOOS == "windows" {
		programData := os.Getenv(WindowsAppDataEnvVar)
		if programData != "" {
			systemFilePaths = append(systemFilePaths, programData)
		}
	} else {
		systemFilePaths = append(systemFilePaths, SystemDirConfigFilePath, "/etc")
	}
	var configFilesPaths []string
	for _, systemPath := range systemFilePaths {
		configFilePath := filepath.Join(systemPath, CliConfigFileName)
		resultPath, exist := cl.SearchConfigFilePath(configFilePath)
		if !exist {
			u.LogDebug(fmt.Sprintf("Failed to find config file on system path '%s'", configFilePath))
			continue
		}
		if exist {
			u.LogDebug(fmt.Sprintf("Found config file on system path '%s': %s", configFilePath, resultPath))
			configFilesPaths = append(configFilesPaths, resultPath)
			cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, filepath.Dir(configFilePath))
		}

	}
	if len(configFilesPaths) > 0 {
		return configFilesPaths, true
	}
	return configFilesPaths, false
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
			u.LogDebug(fmt.Sprintf("error getting absolute path for file '%s'. %v", path, err))
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
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	err = v.MergeConfig(bytes.NewReader(content))
	if err != nil {
		return false, err
	}

	type baseConfig struct {
		BasePath string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	}
	configMap, err := u.UnmarshalYAMLFromFile[baseConfig](&atmosConfig, string(content), path)
	if err != nil {
		u.LogDebug(fmt.Sprintf("error unmarshaling config file (%s): %v", path, err))
		return false, err
	}
	if configMap.BasePath != "" {
		configData, err := json.Marshal(configMap)
		if err != nil {
			u.LogDebug(fmt.Sprintf("error marshaling config data (%s): %v", path, err))
			return false, err
		}
		err = v.MergeConfig(bytes.NewReader(configData))
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

// ConnectPaths joins multiple paths using the OS-specific path list separator.
func ConnectPaths(paths []string) string {
	return strings.Join(paths, string(os.PathListSeparator))
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
				u.LogDebug(fmt.Sprintf("failed to merge configuration from '%s': %v", configPath, err))
				continue
			}
			if found {
				u.LogDebug(fmt.Sprintf("merge import paths: %v", resolvedPaths))
			}
			cl.deepMergeConfig()

		}
		cl.atmosConfig.Import = importPaths
	}
	return nil
}
