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

	"github.com/charmbracelet/log"

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
		log.Debug("no atmos configurations found on system directory", "err", err)
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
	// get all --config values from command line arguments
	for _, configPath := range configPathsArgs {
		paths, err := cl.SearchAtmosConfigFileDir(configPath)
		if err != nil {
			log.Debug("Failed to find config file in directory", "path", configPath, "error", err)
			continue
		}
		configPaths = append(configPaths, paths...)
		cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, configPath)
	}

	if configPaths == nil {
		return fmt.Errorf("config paths can not be found. Please provide a list of config paths using the --config flag.")
	}
	for _, configPath := range configPaths {
		log.Debug("atmos config from config file", "path", configPath)
		err := cl.loadConfigFileViber(cl.atmosConfig, configPath, cl.viper)
		if err != nil {
			log.Debug("error load config file", "path", configPath, "error", err)
			continue
		}
		err = cl.deepMergeConfig()
		if err != nil {
			log.Debug("error merge config file", "path", configPath, "error", err)
			continue
		}
		log.Debug("atmos merged config file", "path", configPath)

		// Process Imports and Deep Merge
		if err := cl.processConfigImports(); err != nil {
			log.Debug("error processing imports after config file", "path", configPath, "error", err)
			continue
		}
		err = cl.deepMergeConfig()
		if err != nil {
			log.Debug("error merge config file", "path", configPath, "error", err)
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
		log.Debug("atmos config from ATMOS_CLI_CONFIG_PATH !repo-root Git root dir", "git path", gitRoot)
		atmosCliConfigEnv = gitRoot
	}
	var atmosFilePaths []string
	for _, configPath := range atmosCliConfigEnvPaths {
		atmosFilePath, err := cl.getPathAtmosCLIConfigPath(configPath)
		if err != nil {
			log.Debug("failed to find config file", "path", atmosFilePath, "error", err)
			continue
		}
		atmosFilePaths = append(atmosFilePaths, atmosFilePath...)
	}
	if len(atmosFilePaths) == 0 {
		return fmt.Errorf("Failed to find config files in ATMOS_CLI_CONFIG_PATH '%s'", atmosCliConfigEnv)
	}
	for _, atmosFilePath := range atmosFilePaths {
		log.Debug("atmos config from ATMOS_CLI_CONFIG_PATH ", "path", atmosFilePath)
		err := cl.loadConfigFileViber(cl.atmosConfig, atmosFilePath, cl.viper)
		if err != nil {
			log.Debug("error processing config file", "path", atmosFilePath, "error", err)
			continue
		}
		err = cl.deepMergeConfig()
		if err != nil {
			log.Debug("error merge config file", "path", atmosFilePath, "error", err)
			continue
		}
		log.Debug("atmos merged config file", "path", atmosFilePath)
		// Process Imports and Deep Merge
		if err := cl.processConfigImports(); err != nil {
			log.Debug("atmos config file found on ATMOS_CLI_CONFIG_PATH", "path", atmosFilePath, "error", err)
			continue
		}
		err = cl.deepMergeConfig()
		if err != nil {
			log.Debug("error merge config file", "path", atmosFilePath, "error", err)
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
		log.Debug("Failed to find config", "path", atmosCliConfigPathEnv)
	}
	searchDir := filepath.Join(filepath.FromSlash(atmosCliConfigPathEnv), "atmos.d/**/*")
	foundPaths, err := cl.SearchAtmosConfigFileDir(searchDir)
	if err != nil {
		log.Debug("Failed to find config file in directory", "path", searchDir, "error", err)
	}
	if len(foundPaths) == 0 {
		log.Debug("Failed to find config file in path", "path", searchDir)
	} else {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths...)
	}

	if len(atmosFoundFilePaths) == 0 {
		return nil, fmt.Errorf("Failed to find config files in path '%s'", atmosCliConfigPathEnv)
	}
	cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, atmosCliConfigPathEnv)

	return atmosFoundFilePaths, nil
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
		log.Debug("load atmos config from workdir")
		return nil
	}
	// 2. Check Git Repository Root
	found = cl.loadGitAtmosConfig()
	if found {
		log.Debug("load atmos config from Git root")
		return nil
	}

	// 4. No configuration found in Stage 2
	log.Debug("No configuration found in Discover Additional Configurations")
	return nil
}

func (cl *ConfigLoader) loadWorkdirAtmosConfig() (found bool) {
	found = false
	cwd, err := os.Getwd()
	if err != nil {
		log.Debug("Failed to get current working directory", "error", err)
		return found

	}
	workDirAtmosConfigPath, err := cl.getWorkDirAtmosConfigPaths(cwd)
	if err != nil {
		log.Debug("Failed to get work dir atmos config paths", "path", cwd, "error", err)
		return found
	}
	if len(workDirAtmosConfigPath) > 0 {
		for _, atmosFilePath := range workDirAtmosConfigPath {
			log.Debug("Found work dir atmos config file", "path", atmosFilePath)
			err := cl.loadConfigFileViber(cl.atmosConfig, atmosFilePath, cl.viper)
			if err != nil {
				log.Debug("error load config file", "path", atmosFilePath, "error", err)
				continue
			}
			err = cl.deepMergeConfig()
			if err != nil {
				log.Debug("error merge config file", "path", atmosFilePath, "error", err)
				continue
			}
			log.Debug("atmos merged config file", "path", atmosFilePath)
			// Process Imports and Deep Merge
			if err := cl.processConfigImports(); err != nil {
				log.Debug("error processing imports after work dir", "path", atmosFilePath, "error", err)
				continue
			}
			err = cl.deepMergeConfig()
			if err != nil {
				log.Debug("error merge config file", "path", atmosFilePath, "error", err)
			}
			found = true
		}
		if !found {
			log.Debug("Failed to process atmos config files fom workdir path", "path", cwd)
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
		log.Debug("Failed to find config file atmos in work directory", "path", workDir)
	}
	searchDir := filepath.Join(filepath.FromSlash(workDir), "atmos.d/**/*")
	foundPaths, err := cl.SearchAtmosConfigFileDir(searchDir)
	if err != nil {
		log.Debug("Failed to find work dir atmos config file", "path", searchDir, "error", err)
	}
	if len(foundPaths) == 0 {
		log.Debug("Failed to find config work dir", "path", searchDir)
	} else {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths...)
	}
	searchDir = filepath.Join(filepath.FromSlash(workDir), ".atmos.d/**/*")
	foundPaths, err = cl.SearchAtmosConfigFileDir(searchDir)
	if err != nil {
		log.Debug("Failed to find work dir atmos config file", "path", searchDir, "error", err)
	}
	if len(foundPaths) == 0 {
		log.Debug("Failed to find config file work dir", "path", searchDir)
	} else {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths...)
	}

	if len(atmosFoundFilePaths) == 0 {
		return nil, fmt.Errorf("Failed to find config files in path '%s'", workDir)
	}
	cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, workDir)
	return atmosFoundFilePaths, nil
}

// loadGitAtmosConfig attempts to load configuration files from the Git repository root. It returns a boolean indicating if any configs were found and successfully loaded.
func (cl *ConfigLoader) loadGitAtmosConfig() (found bool) {
	found = false
	gitRoot, err := u.GetGitRoot()
	if err != nil {
		log.Debug("Failed to determine Git repository root", "error", err)
		return found
	}
	if gitRoot != "" {
		log.Debug("Git repository root atmos loaded", "path", gitRoot)
		isDir, err := u.IsDirectory(gitRoot)
		if err != nil {
			if err == os.ErrNotExist {
				log.Debug("Git repository root not found", "path", gitRoot)
				return found
			}
		}

		if !isDir {
			log.Debug("Git repository root not found", "path", gitRoot)
			return found
		}
		log.Debug("Git repository root found", "path", gitRoot)

		gitAtmosConfigPath, err := cl.getGitAtmosConfigPaths(gitRoot)
		if err != nil {
			log.Debug("Failed to get Git atmos config paths", "error", err)
			return found
		}
		if len(gitAtmosConfigPath) > 0 {
			for _, atmosFilePath := range gitAtmosConfigPath {
				log.Debug("Found Git atmos config file", "path", atmosFilePath)
				err := cl.loadConfigFileViber(cl.atmosConfig, atmosFilePath, cl.viper)
				if err != nil {
					log.Debug("error load config file", "path", atmosFilePath, "error", err)
					continue
				}
				err = cl.deepMergeConfig()
				if err != nil {
					log.Debug("error merge config file", "path", atmosFilePath, "error", err)
					continue
				}

				log.Debug("atmos merged config file", "path", atmosFilePath)
				// Process Imports and Deep Merge
				if err := cl.processConfigImports(); err != nil {
					log.Debug("error processing imports after git", "path", atmosFilePath, "error", err)
					continue
				}
				err = cl.deepMergeConfig()
				if err != nil {
					log.Debug("error merge config file", "path", atmosFilePath, "error", err)
				}
				found = true
			}
			if !found {
				log.Debug("Failed to process atmos config files fom git root path", "path", gitRoot)
				return false
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
		log.Debug("Failed to find config file atmos in git directory", "path", gitRootDir)
	}
	searchDir := filepath.Join(filepath.FromSlash(gitRootDir), "atmos.d/**/*")
	foundPaths, err := cl.SearchAtmosConfigFileDir(searchDir)
	if err != nil {
		log.Debug("Failed to find git atmos config file in path", "path", searchDir, "error", err)
	}
	if len(foundPaths) == 0 {
		log.Debug("Failed to find config file in git root", "path", searchDir)
	} else {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths...)
	}
	searchDir = filepath.Join(filepath.FromSlash(gitRootDir), ".atmos.d/**/*")
	foundPaths, err = cl.SearchAtmosConfigFileDir(searchDir)
	if err != nil {
		log.Debug("Failed to find git atmos config file in path", "path", searchDir, "error", err)
	}
	if len(foundPaths) == 0 {
		log.Debug("Failed to find config file in git root", "path", searchDir)
	} else {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths...)
	}

	if len(atmosFoundFilePaths) == 0 {
		return nil, fmt.Errorf("Failed to find config files in git root path", "path", gitRootDir)
	}
	cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, gitRootDir)
	return atmosFoundFilePaths, nil
}

// applyUserPreferences applies user-specific configuration preferences.
func (cl *ConfigLoader) applyUserPreferences() {
	configPath := filepath.Join(xdg.ConfigHome, CliConfigFileName)
	atmosConfigPath, found := cl.SearchConfigFilePath(configPath)
	if found {
		err := cl.loadConfigFileViber(cl.atmosConfig, atmosConfigPath, cl.viper)
		if err != nil {
			log.Debug("error load config file", "path", configPath, "error", err)
		} else {
			err = cl.processConfigImports()
			if err != nil {
				log.Debug("atmos config file found on XDG_CONFIG_HOME atmos", "path", atmosConfigPath, "error", err)
			}
			log.Debug("atmos config file found on XDG_CONFIG_HOME atmos", "path", atmosConfigPath)
			cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, configPath)
			return
		}
	}

	userHomeDir, err := homedir.Dir()
	if err != nil {
		log.Debug("error getting user home directory", "error", err)
	} else {
		foundHomeDirConfig := false
		configPathHomeDIr := filepath.Join(userHomeDir, ".config", CliConfigFileName)
		atmosConfigPathHomeDir, found := cl.SearchConfigFilePath(configPathHomeDIr)
		if found {
			err := cl.loadConfigFileViber(cl.atmosConfig, atmosConfigPathHomeDir, cl.viper)
			if err != nil {
				log.Debug("error processing config user HomeDir", "path", configPathHomeDIr, "error", err)
			} else {
				foundHomeDirConfig = true
				err = cl.processConfigImports()
				if err != nil {
					log.Debug("error processing imports after HomeDir atmos path", "error", err)
				}
				log.Debug("atmos config file found on HomeDir atmos path", "path", atmosConfigPathHomeDir)
				cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, configPathHomeDIr)
			}
		}
		configPathHomeDIr = filepath.Join(userHomeDir, ".atmos", CliConfigFileName)
		atmosConfigPathHomeDir, found = cl.SearchConfigFilePath(configPathHomeDIr)
		if found {
			err := cl.loadConfigFileViber(cl.atmosConfig, atmosConfigPathHomeDir, cl.viper)
			if err != nil {
				log.Debug("error processing config user HomeDir", "path", atmosConfigPathHomeDir, "error", err)
			} else {
				foundHomeDirConfig = true
				err = cl.processConfigImports()
				if err != nil {
					log.Debug("error processing imports after HomeDir atmos", "path", atmosConfigPathHomeDir, "error", err)
				}
				log.Debug("atmos config file found on HomeDir atmos", "path", atmosConfigPathHomeDir)
				cl.AtmosConfigPaths = append(cl.AtmosConfigPaths, configPathHomeDIr)
			}
		}
		if foundHomeDirConfig {
			return
		}

	}
	log.Debug("no configuration found in user preferences")
	return
}

func (cl *ConfigLoader) loadSystemConfig() error {
	configSysPaths, found := cl.getSystemConfigPath()
	if found {
		for _, configSysPath := range configSysPaths {
			log.Debug("atmos configurations found on system directory", "path", configSysPath)
			err := cl.loadConfigFileViber(cl.atmosConfig, configSysPath, cl.viper)
			if err != nil {
				log.Debug("error load config file", "path", configSysPath, "error", err)
				continue
			}

			err = cl.deepMergeConfig()
			if err != nil {
				log.Debug("error merge config file", "path", configSysPath, "error", err)
				continue
			}
			log.Debug("atmos merged config from system path", "path", configSysPath)
			// Process Imports and Deep Merge
			if err := cl.processConfigImports(); err != nil {
				log.Debug("error processing imports after system config", "path", configSysPath, "error", err)
				continue
			}
			err = cl.deepMergeConfig()
			if err != nil {
				log.Debug("error merge config after imports", "path", configSysPath, "error", err)
			}
			return nil

		}
		return nil

	} else {
		log.Debug("no atmos configurations found on system directory")
	}
	return nil
}

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
			continue
		}
		if exist {
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
			log.Debug("error getting absolute path for file", "path", path, "error", err)
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
) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	err = v.MergeConfig(bytes.NewReader(content))
	if err != nil {
		return err
	}

	type baseConfig struct {
		BasePath string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	}
	configMap, err := u.UnmarshalYAMLFromFile[baseConfig](&atmosConfig, string(content), path)
	if err != nil {
		log.Debug("error unmarshaling config file", "path", path, "error", err)
		return err
	}
	if configMap.BasePath != "" {
		configData, err := json.Marshal(configMap)
		if err != nil {
			log.Debug("failed to unmarshal config data", "path", path, "error", err)
			return err
		}
		err = v.MergeConfig(bytes.NewReader(configData))
		if err != nil {
			return err
		}
	}

	return nil
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
			err := cl.loadConfigFileViber(cl.atmosConfig, configPath, cl.viper)
			if err != nil {
				log.Debug("error load config file", "path", configPath, "error", err)
				continue
			}
			log.Debug("atmos merged config from import path", "path", configPath)
			err = cl.deepMergeConfig()
			if err != nil {
				log.Debug("error merge config after imports", "path", configPath, "error", err)
				continue
			}
		}
		cl.atmosConfig.Import = importPaths
	}
	return nil
}
