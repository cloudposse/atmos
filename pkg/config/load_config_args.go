package config

import (
	"errors"
	"os"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/viper"
)

var (
	ErrExpectedDirOrPattern   = errors.New("--config-path expected directory found file")
	ErrFileNotFound           = errors.New("file not found")
	ErrExpectedFile           = errors.New("--config expected file found directory")
	ErrAtmosArgConfigNotFound = errors.New("atmos configuration not found")
)

// loadConfigFromCLIArgs handles the loading of configurations provided via --config-path.
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
		configPaths = append(configPaths, configFilesArgs...)
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
		return ErrAtmosArgConfigNotFound
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
				log.Debug("Failed to found atmos config", "file", v.ConfigFileUsed())
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
			log.Debug("Failed to found .atmos config", "path", confDirPath, "error", err)
			return nil, ErrAtmosArgConfigNotFound
		}
		log.Debug(".atmos config file merged", "path", v.ConfigFileUsed())
		configPaths = append(configPaths, confDirPath)
	}
	return configPaths, nil
}

func validatedIsDirs(dirPaths []string) error {
	for _, dirPath := range dirPaths {
		stat, err := os.Stat(dirPath)
		if err != nil {
			log.Debug("--config-path directory not found", "path", dirPath)
			return err
		}
		if !stat.IsDir() {
			log.Debug("--config-path expected directory found file", "path", dirPath)
			return ErrAtmosDIrConfigNotFound
		}
	}
	return nil
}

func validatedIsFiles(files []string) error {
	for _, filePath := range files {
		stat, err := os.Stat(filePath)
		if err != nil {
			log.Debug("--config file not found", "path", filePath)
			return ErrFileNotFound
		}
		if stat.IsDir() {
			log.Debug("--config expected file found directors", "path", filePath)
			return ErrExpectedFile
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
		result += path + ";"
	}
	return result
}
