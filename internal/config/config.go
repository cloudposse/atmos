package config

import (
	u "atmos/internal/utils"
	"encoding/json"
	"fmt"
	"github.com/fatih/color"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	configFileName          = "atmos.yaml"
	systemDirConfigFilePath = "/usr/local/etc/atmos"
	windowsAppDataEnvVar    = "LOCALAPPDATA"
)

type Configuration struct {
	StackNamePattern          string `mapstructure:"StackNamePattern"`
	StacksBasePath            string `mapstructure:"StacksBasePath"`
	StacksBaseAbsolutePath    string
	IncludeStackPaths         []string `mapstructure:"IncludeStackPaths"`
	IncludeStackAbsolutePaths []string
	ExcludeStackPaths         []string `mapstructure:"ExcludeStackPaths"`
	ExcludeStackAbsolutePaths []string
	TerraformDir              string `mapstructure:"TerraformDir"`
	TerraformDirAbsolutePath  string
	StackConfigFiles          []string
}

var (
	// Default values
	defaultConfig = map[string]interface{}{
		// Stacks base path
		"StacksBasePath": "./stacks",
		// Default paths (globs) to stack configs to include
		"IncludeStackPaths": []interface{}{
			"**/*",
		},
		// Default paths (globs) to stack configs to exclude
		"ExcludeStackPaths": []interface{}{
			"globals/**/*",
			"catalog/**/*",
			"**/*globals*",
		},
		// Default path to terraform components
		"TerraformDir": "./components/terraform",
		// Logical stack name pattern
		"StackNamePattern": "environment-stage",
	}

	// Config is the CLI configuration structure
	Config Configuration
)

// InitConfig processes and merges configurations in the following order: system dir, home dir, current dir, ENV vars
// https://dev.to/techschoolguru/load-config-from-file-environment-variables-in-golang-with-viper-2j2d
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func InitConfig(stack string) error {
	// Config is loaded from these locations (from lower to higher priority):
	// system dir (/usr/local/etc/atmos on Linux)
	// home dir (~/.atmos)
	// from the current directory
	// from ENV vars
	// from command-line arguments

	color.Cyan("\nProcessing and merging configurations in the following order: system dir, home dir, current dir, ENV vars\n")

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetTypeByDefaultValue(true)

	// Add default config
	err := v.MergeConfigMap(defaultConfig)
	if err != nil {
		return err
	}

	// Process config in system folder
	configFilePath1 := ""

	// https://pureinfotech.com/list-environment-variables-windows-10/
	// https://docs.microsoft.com/en-us/windows/deployment/usmt/usmt-recognized-environment-variables
	// https://softwareengineering.stackexchange.com/questions/299869/where-is-the-appropriate-place-to-put-application-configuration-files-for-each-p
	// https://stackoverflow.com/questions/37946282/why-does-appdata-in-windows-7-seemingly-points-to-wrong-folder
	if runtime.GOOS == "windows" {
		appDataDir := os.Getenv(windowsAppDataEnvVar)
		if len(appDataDir) > 0 {
			configFilePath1 = appDataDir
		}
	} else {
		configFilePath1 = systemDirConfigFilePath
	}

	if len(configFilePath1) > 0 {
		configFile1 := path.Join(configFilePath1, configFileName)
		err = processConfigFile(configFile1, v)
		if err != nil {
			return err
		}
	}

	// Process config in user's HOME dir
	configFilePath2, err := homedir.Dir()
	if err != nil {
		return err
	}
	configFile2 := path.Join(configFilePath2, ".atmos", configFileName)
	err = processConfigFile(configFile2, v)
	if err != nil {
		return err
	}

	// Process config in the current dir
	configFilePath3, err := os.Getwd()
	if err != nil {
		return err
	}
	configFile3 := path.Join(configFilePath3, configFileName)
	err = processConfigFile(configFile3, v)
	if err != nil {
		return err
	}

	// https://gist.github.com/chazcheadle/45bf85b793dea2b71bd05ebaa3c28644
	// https://sagikazarmark.hu/blog/decoding-custom-formats-with-viper/
	err = v.Unmarshal(&Config)
	if err != nil {
		return err
	}

	// Process ENV vars
	processEnvVars()

	// Check config
	err = checkConfig()
	if err != nil {
		return err
	}

	// Convert stacks base path to absolute path
	stacksBaseAbsPath, err := filepath.Abs(Config.StacksBasePath)
	if err != nil {
		return err
	}
	Config.StacksBaseAbsolutePath = stacksBaseAbsPath

	// Convert the included stack paths to absolute paths
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, Config.IncludeStackPaths)
	if err != nil {
		return err
	}
	Config.IncludeStackAbsolutePaths = includeStackAbsPaths

	// Convert the excluded stack paths to absolute paths
	excludeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, Config.ExcludeStackPaths)
	if err != nil {
		return err
	}
	Config.ExcludeStackAbsolutePaths = excludeStackAbsPaths

	// Convert terraform dir to absolute path
	terraformDirAbsPath, err := filepath.Abs(Config.TerraformDir)
	if err != nil {
		return err
	}
	Config.TerraformDirAbsolutePath = terraformDirAbsPath

	// If the specified stack name is a logical name, find all stack config files in the provided paths
	stackConfigFiles, stackIsPhysicalPath, matchedFile, err := findAllStackConfigsInPaths(stack, includeStackAbsPaths, excludeStackAbsPaths)
	if err != nil {
		return err
	}

	if len(stackConfigFiles) < 1 {
		j, err := json.MarshalIndent(includeStackAbsPaths, "", strings.Repeat(" ", 2))
		if err != nil {
			return err
		}
		errorMessage := fmt.Sprintf("No config files found in any of the provided paths:\n%s\n", j)
		return errors.New(errorMessage)
	}
	Config.StackConfigFiles = stackConfigFiles

	color.Cyan("\nFinal configuration:")
	j, _ := json.MarshalIndent(&Config, "", strings.Repeat(" ", 2))
	if err != nil {
		return err
	}
	fmt.Println(fmt.Sprintf("%s\n\n", j))

	if stackIsPhysicalPath == true {
		color.Cyan(fmt.Sprintf("Specified stack '%s' is a physical path since it matches the stack config file %s",
			stack, matchedFile))
	} else {
		// The stack is a logical name
		// Check if it matches the pattern specified in 'StackNamePattern'
		stackParts := strings.Split(stack, "-")
		stackNamePatternParts := strings.Split(Config.StackNamePattern, "-")

		if len(stackParts) == len(stackNamePatternParts) {
			color.Cyan(fmt.Sprintf("Specified stack '%s' is a logical name since it matches the stack name pattern '%s'",
				stack, Config.StackNamePattern))
		} else {
			errorMessage := fmt.Sprintf("Specified stack '%s' is a logical name but it does not match the stack name pattern '%s'",
				stack, Config.StackNamePattern)
			return errors.New(errorMessage)
		}
	}

	fmt.Println()

	return nil
}

// https://github.com/NCAR/go-figure
// https://github.com/spf13/viper/issues/181
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func processConfigFile(path string, v *viper.Viper) error {
	if !u.FileExists(path) {
		fmt.Println(fmt.Sprintf("No config found at %s", path))
		return nil
	}

	color.Green("Found config at " + path)

	reader, err := os.Open(path)
	if err != nil {
		return err
	}

	defer func(reader *os.File) {
		err := reader.Close()
		if err != nil {
			color.Red("Error closing file " + path + ". " + err.Error())
		}
	}(reader)

	err = v.MergeConfig(reader)
	if err != nil {
		return err
	}

	color.Green("Processed config at " + path)

	return nil
}

func processEnvVars() {
	stacksBasePath := os.Getenv("ATMOS_STACKS_BASE_PATH")
	if len(stacksBasePath) > 0 {
		color.Green(fmt.Sprintf("Found ENV var 'ATMOS_STACKS_BASE_PATH': %s", stacksBasePath))
		Config.StacksBasePath = stacksBasePath
	}

	includeStackPaths := os.Getenv("ATMOS_INCLUDE_STACK_PATHS")
	if len(includeStackPaths) > 0 {
		color.Green(fmt.Sprintf("Found ENV var 'ATMOS_INCLUDE_STACK_PATHS': %s", includeStackPaths))
		Config.IncludeStackPaths = strings.Split(includeStackPaths, ",")
	}

	excludeStackPaths := os.Getenv("ATMOS_EXCLUDE_STACK_PATHS")
	if len(excludeStackPaths) > 0 {
		color.Green(fmt.Sprintf("Found ENV var 'ATMOS_EXCLUDE_STACK_PATHS': %s", excludeStackPaths))
		Config.IncludeStackPaths = strings.Split(excludeStackPaths, ",")
	}

	terraformDir := os.Getenv("ATMOS_TERRAFORM_DIR")
	if len(terraformDir) > 0 {
		color.Green(fmt.Sprintf("Found ENV var 'ATMOS_TERRAFORM_DIR': %s", terraformDir))
		Config.TerraformDir = terraformDir
	}

	stackNamePattern := os.Getenv("ATMOS_STACK_NAME_PATTERN")
	if len(stackNamePattern) > 0 {
		color.Green(fmt.Sprintf("Found ENV var 'ATMOS_STACK_NAME_PATTERN': %s", stackNamePattern))
		Config.StackNamePattern = stackNamePattern
	}
}

func checkConfig() error {
	if len(Config.StacksBasePath) < 1 {
		return errors.New("Stack base path must be provided in 'StacksBasePath' or 'ATMOS_STACKS_BASE_PATH' ENV variable")
	}

	if len(Config.IncludeStackPaths) < 1 {
		return errors.New("At least one path must be provided in 'IncludeStackPaths' or 'ATMOS_INCLUDE_STACK_PATHS' ENV variable")
	}

	if len(Config.TerraformDir) < 1 {
		return errors.New("Terraform dir must be provided in 'TerraformDir' or 'ATMOS_TERRAFORM_DIR' ENV variable")
	}

	if len(Config.StackNamePattern) < 1 {
		return errors.New("Stack name pattern must be provided in 'StackNamePattern' or 'ATMOS_STACK_NAME_PATTERN' ENV variable")
	}

	return nil
}
