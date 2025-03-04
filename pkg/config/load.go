package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const MaximumImportLvL = 10

var ErrAtmosDIrConfigNotFound = errors.New("atmos config directory not found")

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
	// Load configuration from different sources.
	if err := loadConfigSources(v, configAndStacksInfo.AtmosCliConfigPath); err != nil {
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
	// get dir of atmosConfigFilePath
	atmosConfigDir := filepath.Dir(v.ConfigFileUsed())
	atmosConfig.CliConfigPath = atmosConfigDir
	// Set the CLI config path in the atmosConfig struct
	if atmosConfig.CliConfigPath != "" && !filepath.IsAbs(atmosConfig.CliConfigPath) {
		absPath, err := filepath.Abs(atmosConfig.CliConfigPath)
		if err != nil {
			return atmosConfig, err
		}
		atmosConfig.CliConfigPath = absPath
	}
	// We want the editorconfig color by default to be true
	atmosConfig.Validate.EditorConfig.Color = true
	// https://gist.github.com/chazcheadle/45bf85b793dea2b71bd05ebaa3c28644
	// https://sagikazarmark.hu/blog/decoding-custom-formats-with-viper/
	err := v.Unmarshal(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}
	return atmosConfig, nil
}

// setDefaultConfiguration set default configuration for the viper instance.
func setDefaultConfiguration(v *viper.Viper) {
	v.SetDefault("components.helmfile.use_eks", true)
	v.SetDefault("components.terraform.append_user_agent",
		fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version))
	v.SetDefault("settings.inject_github_token", true)
	v.SetDefault("logs.file", "/dev/stderr")
	v.SetDefault("logs.level", "Info")
}

// loadConfigSources delegates reading configs from each source,
// returning early if any step in the chain fails.
func loadConfigSources(v *viper.Viper, cliConfigPath string) error {
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

	return readAtmosConfigCli(v, cliConfigPath)
}

// readSystemConfig load config from system dir .
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
		err := mergeConfig(v, configFilePath, false)
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
			return nil
		default:
			return err
		}
	}
	return nil
}

// readHomeConfig load config from user's HOME dir .
func readHomeConfig(v *viper.Viper) error {
	home, err := homedir.Dir()
	if err != nil {
		return err
	}
	configFilePath := filepath.Join(home, ".atmos")
	err = mergeConfig(v, configFilePath, true)
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

// readWorkDirConfig load config from current working directory .
func readWorkDirConfig(v *viper.Viper) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	err = mergeConfig(v, wd, true)
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
	configFilePath := filepath.Join(atmosPath, CliConfigFileName)
	err := mergeConfig(v, configFilePath, true)
	if err != nil {
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
			log.Debug("config not found ENV var ATMOS_CLI_CONFIG_PATH", "file", configFilePath)
			return nil
		default:
			return err
		}
	}
	log.Debug("Found config ENV", "ATMOS_CLI_CONFIG_PATH", configFilePath)

	return nil
}

func readAtmosConfigCli(v *viper.Viper, atmosCliConfigPath string) error {
	if len(atmosCliConfigPath) == 0 {
		return nil
	}
	err := mergeConfig(v, atmosCliConfigPath, true)
	switch err.(type) {
	case viper.ConfigFileNotFoundError:
		log.Debug("config not found", "file", atmosCliConfigPath)
	default:
		return err
	}

	return nil
}

// mergeConfig merge config from a specified path directory and process imports.return error if config file not exist .
func mergeConfig(v *viper.Viper, path string, processImports bool) error {
	v.AddConfigPath(path)
	v.SetConfigName(CliConfigFileName)
	err := v.MergeInConfig()
	if err != nil {
		return err
	}
	content, err := os.ReadFile(v.ConfigFileUsed())
	if err != nil {
		return err
	}
	err = preprocessAtmosYamlFunc(content, v)
	if err != nil {
		return err
	}

	if !processImports {
		return nil
	}
	if err := mergeDefaultImports(path, v); err != nil {
		log.Debug("error process imports", "path", path, "error", err)
	}
	if err := mergeImports(v); err != nil {
		log.Debug("error process imports", "file", v.ConfigFileUsed(), "error", err)
	}
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
		return ErrAtmosDIrConfigNotFound
	}
	var atmosFoundFilePaths []string
	// Search for `atmos.d/` configurations
	searchDir := filepath.Join(filepath.FromSlash(dirPath), filepath.Join("atmos.d", "**", "*"))
	foundPaths1, err := SearchAtmosConfig(searchDir)
	if err != nil {
		log.Debug("Failed to find atmos config file", "path", searchDir, "error", err)
	}
	if len(foundPaths1) > 0 {
		atmosFoundFilePaths = append(atmosFoundFilePaths, foundPaths1...)
	}
	// Search for `.atmos.d` configurations
	searchDir = filepath.Join(filepath.FromSlash(dirPath), filepath.Join(".atmos.d", "**", "*"))
	foundPaths2, err := SearchAtmosConfig(searchDir)
	if err != nil {
		log.Debug("Failed to find atmos config file", "path", searchDir, "error", err)
	}
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

// PreprocessYAML processes the given YAML content, replacing specific directives
// (such as !env) with their corresponding values .
// It parses the YAML content into a tree structure, processes each node recursively,
// and updates the provided Viper instance with resolved values.
//
// Parameters:
// - yamlContent: The raw YAML content as a byte slice.
// - v: A pointer to a Viper instance where processed values will be stored.
//
// Returns:
// - An error if the YAML content cannot be parsed.
func preprocessAtmosYamlFunc(yamlContent []byte, v *viper.Viper) error {
	var rootNode yaml.Node
	if err := yaml.Unmarshal(yamlContent, &rootNode); err != nil {
		log.Debug("failed to parse YAML", "content", yamlContent, "error", err)
		return err
	}
	processNode(&rootNode, v, "")
	return nil
}

// processNode recursively traverses a YAML node tree and processes special directives
// (such as !env). If a directive is found, it replaces the corresponding value in Viper
// using values retrieved from Atmos custom functions.
//
// Parameters:
// - node: A pointer to the current YAML node being processed.
// - v: A pointer to a Viper instance where processed values will be stored.
// - currentPath: The hierarchical key path used to track nested YAML structures.
func processNode(node *yaml.Node, v *viper.Viper, currentPath string) {
	if node == nil {
		return
	}
	// If this node is a key-value pair in a mapping
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			newPath := keyNode.Value // Extracting the key name

			if currentPath != "" {
				newPath = currentPath + "." + newPath
			}

			processNode(valueNode, v, newPath)
		}
	}

	// If it's a scalar node with a directive tag
	if node.Kind == yaml.ScalarNode && node.Tag != "" {
		processScalarNode(node, v, currentPath)
	}

	// Process children nodes (for sequences/lists)
	for _, child := range node.Content {
		processNode(child, v, currentPath)
	}
}

func processScalarNode(node *yaml.Node, v *viper.Viper, currentPath string) {
	if node.Tag == "" {
		return
	}
	allowedDirectives := []string{AtmosYamlFuncEnv}

	for _, directive := range allowedDirectives {
		if node.Tag == directive {
			arg := node.Value
			if directive == AtmosYamlFuncEnv {
				envValue := os.Getenv(arg)
				if envValue != "" {
					node.Value = envValue
				}
				v.Set(currentPath, node.Value) // Store the value to Viper
			}
			node.Tag = ""
			break
		}
	}
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
