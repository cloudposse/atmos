package config

import (
	"fmt"
	"os"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// loadConfigFromCLIArgs handles the loading of configurations provided via --config-path and --config.
func loadConfigFromCLIArgs(v *viper.Viper, configAndStacksInfo *schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration) error {
	log.Debug("loading config from command line arguments")

	configFilesArgs := configAndStacksInfo.AtmosConfigFilesFromArg
	configDirsArgs := configAndStacksInfo.AtmosConfigDirsFromArg
	var configPaths []string

	// Merge all config from --config files
	if len(configFilesArgs) > 0 {
		if err := mergeFiles(v, configFilesArgs); err != nil {
			return err
		}
		for _, configFilePath := range configFilesArgs {
			configPaths = append(configPaths, filepath.Dir(configFilePath))
		}
	}

	// Merge config from --config-path directories
	if len(configDirsArgs) > 0 {
		paths, err := mergeConfigFromDirectories(v, configDirsArgs)
		if err != nil {
			return err
		}
		configPaths = append(configPaths, paths...)
	}

	// Check if any config files were found from command line arguments
	if len(configPaths) == 0 {
		log.Debug("no config files found from command line arguments")
		return fmt.Errorf("%w: no config files found from command line arguments (--config or --config-path)", errUtils.ErrAtmosArgConfigNotFound)
	}

	if err := v.Unmarshal(atmosConfig); err != nil {
		return err
	}

	atmosConfig.CliConfigPath = connectPaths(configPaths)
	return nil
}

// mergeFiles merges config files from the provided paths.
func mergeFiles(v *viper.Viper, configFilePaths []string) error {
	err := validatedIsFiles(configFilePaths)
	if err != nil {
		return err
	}
	for _, configPath := range configFilePaths {
		err := mergeConfigFile(configPath, v)
		if err != nil {
			log.Debug("error loading config file", "path", configPath, "error", err)
			return err
		}
		log.Debug("config file merged", "path", configPath)
		if err := mergeDefaultImports(configPath, v); err != nil {
			log.Debug("error process imports", "path", configPath, "error", err)
		}
		if err := mergeImports(v); err != nil {
			log.Debug("error process imports", "file", configPath, "error", err)
		}
	}
	return nil
}

// mergeConfigFromDirectories merges config files from the provided directories.
func mergeConfigFromDirectories(v *viper.Viper, dirPaths []string) ([]string, error) {
	if err := validatedIsDirs(dirPaths); err != nil {
		return nil, err
	}
	var configPaths []string
	for _, confDirPath := range dirPaths {
		err := mergeConfig(v, confDirPath, CliConfigFileName, true)
		if err != nil {
			log.Debug("Failed to find atmos config", "path", confDirPath, "error", err)
			switch err.(type) {
			case viper.ConfigFileNotFoundError:
				log.Debug("Failed to found atmos config", "file", filepath.Join(confDirPath, CliConfigFileName))
			default:
				return nil, err
			}
		}
		if err == nil {
			log.Debug("atmos config file merged", "path", v.ConfigFileUsed())
			configPaths = append(configPaths, confDirPath)
			continue
		}
		err = mergeConfig(v, confDirPath, DotCliConfigFileName, true)
		if err != nil {
			log.Debug("Failed to found .atmos config", "path", filepath.Join(confDirPath, CliConfigFileName), "error", err)
			return nil, fmt.Errorf("%w: %s", errUtils.ErrAtmosFilesDirConfigNotFound, confDirPath)
		}
		log.Debug(".atmos config file merged", "path", v.ConfigFileUsed())
		configPaths = append(configPaths, confDirPath)
	}
	return configPaths, nil
}

func validatedIsDirs(dirPaths []string) error {
	for _, dirPath := range dirPaths {
		if dirPath == "" {
			return fmt.Errorf("%w: --config-path requires a non-empty directory path", errUtils.ErrEmptyConfigPath)
		}
		stat, err := os.Stat(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				log.Debug("--config-path directory not found", "path", dirPath)
				return fmt.Errorf("%w: --config-path directory '%s' does not exist", errUtils.ErrAtmosDirConfigNotFound, dirPath)
			}
			// Other stat errors (permission denied, etc.)
			return fmt.Errorf("cannot access --config-path directory '%s': %w", dirPath, err)
		}
		if !stat.IsDir() {
			log.Debug("--config-path expected directory found file", "path", dirPath)
			return fmt.Errorf("%w: --config-path requires a directory but found a file at '%s'", errUtils.ErrAtmosDirConfigNotFound, dirPath)
		}
	}
	return nil
}

func validatedIsFiles(files []string) error {
	for _, filePath := range files {
		if filePath == "" {
			return fmt.Errorf("%w: --config requires a non-empty file path", errUtils.ErrEmptyConfigFile)
		}
		stat, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				log.Debug("--config file not found", "path", filePath)
				return fmt.Errorf("%w: --config file '%s' does not exist", errUtils.ErrFileNotFound, filePath)
			}
			// Other stat errors (permission denied, etc.)
			return fmt.Errorf("%w: cannot access --config file '%s': %v", errUtils.ErrFileNotFound, filePath, err)
		}
		if stat.IsDir() {
			log.Debug("--config expected file found directory", "path", filePath)
			return errUtils.ErrExpectedFile
		}
	}
	return nil
}

func connectPaths(paths []string) string {
	if len(paths) == 1 {
		return paths[0]
	}
	var result string
	for _, path := range paths {
		if path == "" {
			continue
		}
		result += path + ";"
	}
	return result
}
