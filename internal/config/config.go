package config

import (
	u "atmos/internal/utils"
	"bytes"
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

var (
	// Default values
	defaultConfig = Configuration{
		Components: Components{
			Terraform: Terraform{
				BasePath: "./components/terraform",
			},
			Helmfile: Helmfile{
				BasePath:              "./components/helmfile",
				KubeconfigPath:        "/dev/shm",
				HelmAwsProfilePattern: "{namespace}-{tenant}-gbl-{stage}-helm",
				ClusterNamePattern:    "{namespace}-{tenant}-{environment}-{stage}-eks-cluster",
			},
		},
		Stacks: Stacks{
			BasePath: "./stacks",
			IncludedPaths: []string{
				"**/*",
			},
			ExcludedPaths: []string{
				"globals/**/*",
				"catalog/**/*",
				"**/*globals*",
			},
		},
	}

	// Config is the CLI configuration structure
	Config Configuration

	// ProcessedConfig holds all the calculated values
	ProcessedConfig ProcessedConfiguration
)

// InitConfig processes and merges configurations in the following order: system dir, home dir, current dir, ENV vars
// https://dev.to/techschoolguru/load-config-from-file-environment-variables-in-golang-with-viper-2j2d
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func InitConfig(stack string) error {
	// Config is loaded from the following locations (from lower to higher priority):
	// system dir (`/usr/local/etc/atmos` on Linux, `%LOCALAPPDATA%/atmos` on Windows)
	// home dir (~/.atmos)
	// current directory
	// ENV vars

	color.Cyan("\nProcessing and merging configurations in the following order: system dir, home dir, current dir, ENV vars\n")

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetTypeByDefaultValue(true)

	// Add default config
	j, err := json.Marshal(defaultConfig)
	if err != nil {
		return err
	}
	reader := bytes.NewReader(j)
	err = v.MergeConfig(reader)
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
	stacksBaseAbsPath, err := filepath.Abs(Config.Stacks.BasePath)
	if err != nil {
		return err
	}
	ProcessedConfig.StacksBaseAbsolutePath = stacksBaseAbsPath

	// Convert the included stack paths to absolute paths
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, Config.Stacks.IncludedPaths)
	if err != nil {
		return err
	}
	ProcessedConfig.IncludeStackAbsolutePaths = includeStackAbsPaths

	// Convert the excluded stack paths to absolute paths
	excludeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, Config.Stacks.ExcludedPaths)
	if err != nil {
		return err
	}
	ProcessedConfig.ExcludeStackAbsolutePaths = excludeStackAbsPaths

	// Convert terraform dir to absolute path
	terraformDirAbsPath, err := filepath.Abs(Config.Components.Terraform.BasePath)
	if err != nil {
		return err
	}
	ProcessedConfig.TerraformDirAbsolutePath = terraformDirAbsPath

	// Convert helmfile dir to absolute path
	helmfileDirAbsPath, err := filepath.Abs(Config.Components.Helmfile.BasePath)
	if err != nil {
		return err
	}
	ProcessedConfig.HelmfileDirAbsolutePath = helmfileDirAbsPath

	// If the specified stack name is a logical name, find all stack config files in the provided paths
	stackConfigFilesAbsolutePaths, stackConfigFilesRelativePaths, stackIsPhysicalPath, err := findAllStackConfigsInPaths(
		stack,
		includeStackAbsPaths,
		excludeStackAbsPaths,
	)

	if err != nil {
		return err
	}

	if len(stackConfigFilesAbsolutePaths) < 1 {
		j, err := json.MarshalIndent(includeStackAbsPaths, "", strings.Repeat(" ", 2))
		if err != nil {
			return err
		}
		errorMessage := fmt.Sprintf("\nNo config files found in any of the provided "+
			"stack paths:\n%s\n\nCheck if `stacks.base_path` is correctly set in `atmos.yaml` CLI config files", j)
		return errors.New(errorMessage)
	}

	ProcessedConfig.StackConfigFilesAbsolutePaths = stackConfigFilesAbsolutePaths
	ProcessedConfig.StackConfigFilesRelativePaths = stackConfigFilesRelativePaths

	fmt.Println()

	if stackIsPhysicalPath == true {
		color.Cyan(fmt.Sprintf("Stack '%s' is a directory, it matches the stack config file %s",
			stack,
			stackConfigFilesRelativePaths[0]),
		)
		ProcessedConfig.StackType = "Directory"
	} else {
		// The stack is a logical name
		// Check if it matches the pattern specified in 'StackNamePattern'
		stackParts := strings.Split(stack, "-")
		stackNamePatternParts := strings.Split(Config.Stacks.NamePattern, "-")

		if len(stackParts) == len(stackNamePatternParts) {
			color.Cyan(fmt.Sprintf("Stack '%s' matches the stack name pattern '%s'",
				stack,
				Config.Stacks.NamePattern),
			)
			ProcessedConfig.StackType = "Logical"
		} else {
			errorMessage := fmt.Sprintf("Stack '%s' does not exist in the config directories, and it does not match the stack name pattern '%s'",
				stack,
				Config.Stacks.NamePattern,
			)
			return errors.New(errorMessage)
		}
	}

	color.Cyan("\nFinal CLI configuration:")
	err = u.PrintAsYAML(Config)
	if err != nil {
		return err
	}

	return nil
}

// https://github.com/NCAR/go-figure
// https://github.com/spf13/viper/issues/181
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func processConfigFile(path string, v *viper.Viper) error {
	if !u.FileExists(path) {
		fmt.Println(fmt.Sprintf("No config found in %s", path))
		return nil
	}

	color.Green("Found config in %s", path)

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

	color.Green("Processed config %s", path)

	return nil
}
