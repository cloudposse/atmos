package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	g "github.com/cloudposse/atmos/internal/globals"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

var (
	// Default values
	defaultConfig = Configuration{
		Components: Components{
			Terraform: Terraform{
				BasePath:                "./components/terraform",
				ApplyAutoApprove:        false,
				DeployRunInit:           true,
				AutoGenerateBackendFile: false,
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
		Logs: Logs{
			Verbose: false,
			Colors:  true,
		},
	}

	// Config is the CLI configuration structure
	Config Configuration

	// ProcessedConfig holds all the calculated values
	ProcessedConfig ProcessedConfiguration
)

// InitConfig finds and merges CLI configurations in the following order: system dir, home dir, current dir, ENV vars, command-line arguments
// https://dev.to/techschoolguru/load-config-from-file-environment-variables-in-golang-with-viper-2j2d
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func InitConfig() error {
	// Config is loaded from the following locations (from lower to higher priority):
	// system dir (`/usr/local/etc/atmos` on Linux, `%LOCALAPPDATA%/atmos` on Windows)
	// home dir (~/.atmos)
	// current directory
	// ENV vars
	// Command-line arguments

	err := processLogsConfig()
	if err != nil {
		return err
	}

	if g.LogVerbose {
		color.Cyan("\nProcessing and merging configurations in the following order:\n")
		fmt.Println("system dir, home dir, current dir, ENV vars, command-line arguments")
		fmt.Println()
	}

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
		appDataDir := os.Getenv(g.WindowsAppDataEnvVar)
		if len(appDataDir) > 0 {
			configFilePath1 = appDataDir
		}
	} else {
		configFilePath1 = g.SystemDirConfigFilePath
	}

	if len(configFilePath1) > 0 {
		configFile1 := path.Join(configFilePath1, g.ConfigFileName)
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
	configFile2 := path.Join(configFilePath2, ".atmos", g.ConfigFileName)
	err = processConfigFile(configFile2, v)
	if err != nil {
		return err
	}

	// Process config in the current dir
	configFilePath3, err := os.Getwd()
	if err != nil {
		return err
	}
	configFile3 := path.Join(configFilePath3, g.ConfigFileName)
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

	return nil
}

// ProcessConfig processes and checks CLI configuration
func ProcessConfig(configAndStacksInfo ConfigAndStacksInfo) error {
	// Process ENV vars
	err := processEnvVars()
	if err != nil {
		return err
	}

	// Process command-line args
	if len(configAndStacksInfo.TerraformDir) > 0 {
		Config.Components.Terraform.BasePath = configAndStacksInfo.TerraformDir
		color.Cyan(fmt.Sprintf("Using command line argument '%s' as terraform directory", configAndStacksInfo.TerraformDir))
	}
	if len(configAndStacksInfo.HelmfileDir) > 0 {
		Config.Components.Helmfile.BasePath = configAndStacksInfo.HelmfileDir
		color.Cyan(fmt.Sprintf("Using command line argument '%s' as helmfile directory", configAndStacksInfo.HelmfileDir))
	}
	if len(configAndStacksInfo.ConfigDir) > 0 {
		Config.Stacks.BasePath = configAndStacksInfo.ConfigDir
		color.Cyan(fmt.Sprintf("Using command line argument '%s' as stacks directory", configAndStacksInfo.ConfigDir))
	}
	if len(configAndStacksInfo.StacksDir) > 0 {
		Config.Stacks.BasePath = configAndStacksInfo.StacksDir
		color.Cyan(fmt.Sprintf("Using command line argument '%s' as stacks directory", configAndStacksInfo.StacksDir))
	}
	if len(configAndStacksInfo.DeployRunInit) > 0 {
		deployRunInitBool, err := strconv.ParseBool(configAndStacksInfo.DeployRunInit)
		if err != nil {
			return err
		}
		Config.Components.Terraform.DeployRunInit = deployRunInitBool
		color.Cyan(fmt.Sprintf("Using command line argument '%s=%s'", g.DeployRunInitFlag, configAndStacksInfo.DeployRunInit))
	}
	if len(configAndStacksInfo.AutoGenerateBackendFile) > 0 {
		autoGenerateBackendFileBool, err := strconv.ParseBool(configAndStacksInfo.AutoGenerateBackendFile)
		if err != nil {
			return err
		}
		Config.Components.Terraform.AutoGenerateBackendFile = autoGenerateBackendFileBool
		color.Cyan(fmt.Sprintf("Using command line argument '%s=%s'", g.AutoGenerateBackendFileFlag, configAndStacksInfo.AutoGenerateBackendFile))
	}

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
	includeStackAbsPaths, err := utils.JoinAbsolutePathWithPaths(stacksBaseAbsPath, Config.Stacks.IncludedPaths)
	if err != nil {
		return err
	}
	ProcessedConfig.IncludeStackAbsolutePaths = includeStackAbsPaths

	// Convert the excluded stack paths to absolute paths
	excludeStackAbsPaths, err := utils.JoinAbsolutePathWithPaths(stacksBaseAbsPath, Config.Stacks.ExcludedPaths)
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
		configAndStacksInfo.Stack,
		includeStackAbsPaths,
		excludeStackAbsPaths,
	)

	if err != nil {
		return err
	}

	if len(stackConfigFilesAbsolutePaths) < 1 {
		j, err := yaml.Marshal(includeStackAbsPaths)
		if err != nil {
			return err
		}
		errorMessage := fmt.Sprintf("\nNo stack config files found in the provided "+
			"paths:\n%s\n\nCheck if 'stacks.base_path', 'stacks.included_paths' and 'stacks.excluded_paths' are correctly set in CLI config "+
			"files or ENV vars.", j)
		return errors.New(errorMessage)
	}

	ProcessedConfig.StackConfigFilesAbsolutePaths = stackConfigFilesAbsolutePaths
	ProcessedConfig.StackConfigFilesRelativePaths = stackConfigFilesRelativePaths

	if stackIsPhysicalPath == true {
		if g.LogVerbose {
			color.Cyan(fmt.Sprintf("\nThe stack '%s' matches the stack config file %s\n",
				configAndStacksInfo.Stack,
				stackConfigFilesRelativePaths[0]),
			)
		}
		ProcessedConfig.StackType = "Directory"
	} else {
		// The stack is a logical name
		// Check if it matches the pattern specified in 'StackNamePattern'
		stackParts := strings.Split(configAndStacksInfo.Stack, "-")
		stackNamePatternParts := strings.Split(Config.Stacks.NamePattern, "-")

		if len(stackParts) == len(stackNamePatternParts) {
			if g.LogVerbose {
				color.Cyan(fmt.Sprintf("\nThe stack '%s' matches the stack name pattern '%s'",
					configAndStacksInfo.Stack,
					Config.Stacks.NamePattern),
				)
			}
			ProcessedConfig.StackType = "Logical"
		} else {
			errorMessage := fmt.Sprintf("\nThe stack '%s' does not exist in the config directories, and it does not match the stack name pattern '%s'",
				configAndStacksInfo.Stack,
				Config.Stacks.NamePattern,
			)
			return errors.New(errorMessage)
		}
	}

	if g.LogVerbose {
		color.Cyan("\nFinal CLI configuration:")
		err = utils.PrintAsYAML(Config)
		if err != nil {
			return err
		}
	}

	return nil
}

// https://github.com/NCAR/go-figure
// https://github.com/spf13/viper/issues/181
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func processConfigFile(path string, v *viper.Viper) error {
	if !utils.FileExists(path) {
		if g.LogVerbose {
			fmt.Println(fmt.Sprintf("No config found in %s", path))
		}
		return nil
	}

	if g.LogVerbose {
		color.Green("Found config in %s", path)
	}

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

	if g.LogVerbose {
		color.Green("Processed config %s", path)
	}

	return nil
}
