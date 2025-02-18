package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

type ConfigSources struct {
	paths          string
	atmosFileNames string
}

func LoadConfig(configAndStacksInfo schema.ConfigAndStacksInfo) (schema.AtmosConfiguration, error) {
	v := viper.New()
	var atmosConfig schema.AtmosConfiguration
	v.SetConfigType("yaml")
	v.SetTypeByDefaultValue(true)
	setDefaultConfiguration(v)
	err := readSystemConfig(v)
	if err != nil {
		return atmosConfig, err
	}
	err = readHomeConfig(v)
	if err != nil {
		return atmosConfig, err
	}
	err = readWorkDirConfig(v)
	if err != nil {
		return atmosConfig, err
	}
	err = readEnvAmosConfigPath(v)
	if err != nil {
		return atmosConfig, err
	}
	if configAndStacksInfo.AtmosCliConfigPath != "" {
		configFilePath := configAndStacksInfo.AtmosCliConfigPath
		if len(configFilePath) > 0 {
			err := mergeConfig(v, configFilePath, CliConfigFileName)
			switch err.(type) {
			case viper.ConfigFileNotFoundError:
				u.LogTrace(fmt.Sprintf("Not Found config file %s", configFilePath))
			default:
				return atmosConfig, err
			}
		}
	}

	atmosConfig.CliConfigPath = v.ConfigFileUsed()

	if atmosConfig.CliConfigPath == "" {
		// If `atmos.yaml` not found, use the default config
		// Set `ATMOS_LOGS_LEVEL` ENV var to "Debug" to see the message about Atmos using the default CLI config
		logsLevelEnvVar := os.Getenv("ATMOS_LOGS_LEVEL")
		if logsLevelEnvVar == u.LogLevelDebug || logsLevelEnvVar == u.LogLevelTrace {
			u.PrintMessageInColor("'atmos.yaml' CLI config was not found in any of the searched paths: system dir, home dir, current dir, ENV vars.\n"+
				"Refer to https://atmos.tools/cli/configuration for details on how to configure 'atmos.yaml'.\n"+
				"Using the default CLI config:\n\n", theme.Colors.Info)

			err = u.PrintAsYAMLToFileDescriptor(atmosConfig, defaultCliConfig)
			if err != nil {
				return atmosConfig, err
			}
			fmt.Println()
		}

		j, err := json.Marshal(defaultCliConfig)
		if err != nil {
			return atmosConfig, err
		}

		reader := bytes.NewReader(j)
		err = v.MergeConfig(reader)
		if err != nil {
			return atmosConfig, err
		}
	}
	// Set the CLI config path in the atmosConfig struct
	if !filepath.IsAbs(atmosConfig.CliConfigPath) {
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
	err = v.Unmarshal(&atmosConfig)
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

// readSystemConfig load config from system dir
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
		err := mergeConfig(v, configFilePath, CliConfigFileName)
		switch err.(type) {
		case viper.ConfigFileNotFoundError:
			return nil
		default:
			return err
		}
	}
	return nil

}

// readHomeConfig load config from user's HOME dir
func readHomeConfig(v *viper.Viper) error {
	home, err := homedir.Dir()
	if err != nil {
		return err
	}
	configFilePath := filepath.Join(home, ".atmos")
	err = mergeConfig(v, configFilePath, CliConfigFileName)
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

// readWorkDirConfig load config from current working directory
func readWorkDirConfig(v *viper.Viper) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	err = mergeConfig(v, wd, CliConfigFileName)
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
	if atmosPath != "" {
		configFilePath := filepath.Join(atmosPath, CliConfigFileName)
		err := mergeConfig(v, configFilePath, CliConfigFileName)
		if err != nil {
			switch err.(type) {
			case viper.ConfigFileNotFoundError:
				u.LogTrace(fmt.Sprintf("Not Found ENV var ATMOS_CLI_CONFIG_PATH=%s", configFilePath))
				return nil
			default:
				return err
			}
		}
		u.LogTrace(fmt.Sprintf("Found ENV var ATMOS_CLI_CONFIG_PATH=%s", configFilePath))
	}

	return nil
}

// mergeConfig merge config from a specified path and file name.
func mergeConfig(v *viper.Viper, path string, fileName string) error {
	v.AddConfigPath(path)
	v.SetConfigName(fileName)
	err := v.MergeInConfig()
	if err != nil {
		return nil
	}

	return nil
}

// applyDefaultConfiguration apply default configuration for the atmos config.
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
