package config

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

// InitCliConfig finds and merges CLI configurations in the following order: system dir, home dir, current dir, ENV vars, command-line arguments
// https://dev.to/techschoolguru/load-config-from-file-environment-variables-in-golang-with-viper-2j2d
// https://medium.com/@bnprashanth256/reading-configuration-files-and-environment-variables-in-go-golang-c2607f912b63
func InitCliConfig(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
	atmosConfig, err := LoadConfig(configAndStacksInfo)
	if err != nil {
		return atmosConfig, err
	}
	// Process ENV vars
	err = processEnvVars(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}

	// Process command-line args
	err = processCommandLineArgs(&atmosConfig, configAndStacksInfo)
	if err != nil {
		return atmosConfig, err
	}

	// Process stores config
	err = processStoreConfig(&atmosConfig)
	if err != nil {
		return atmosConfig, err
	}

	// Process the base path specified in the Terraform provider (which calls into the atmos code)
	// This overrides all other atmos base path configs (`atmos.yaml`, ENV var `ATMOS_BASE_PATH`)
	if configAndStacksInfo.AtmosBasePath != "" {
		atmosConfig.BasePath = configAndStacksInfo.AtmosBasePath
	}

	// After unmarshalling, ensure AppendUserAgent is set if still empty
	if atmosConfig.Components.Terraform.AppendUserAgent == "" {
		atmosConfig.Components.Terraform.AppendUserAgent = fmt.Sprintf("Atmos/%s (Cloud Posse; +https://atmos.tools)", version.Version)
	}

	// Check config
	err = checkConfig(atmosConfig, processStacks)
	if err != nil {
		return atmosConfig, err
	}

	// Convert stacks base path to absolute path
	stacksBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)
	stacksBaseAbsPath, err := filepath.Abs(stacksBasePath)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.StacksBaseAbsolutePath = stacksBaseAbsPath

	// Convert the included stack paths to absolute paths
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, atmosConfig.Stacks.IncludedPaths)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.IncludeStackAbsolutePaths = includeStackAbsPaths

	// Convert the excluded stack paths to absolute paths
	excludeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, atmosConfig.Stacks.ExcludedPaths)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.ExcludeStackAbsolutePaths = excludeStackAbsPaths

	// Convert terraform dir to absolute path
	terraformBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath)
	terraformDirAbsPath, err := filepath.Abs(terraformBasePath)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.TerraformDirAbsolutePath = terraformDirAbsPath

	// Convert helmfile dir to absolute path
	helmfileBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath)
	helmfileDirAbsPath, err := filepath.Abs(helmfileBasePath)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.HelmfileDirAbsolutePath = helmfileDirAbsPath

	if processStacks {
		// If the specified stack name is a logical name, find all stack manifests in the provided paths
		stackConfigFilesAbsolutePaths, stackConfigFilesRelativePaths, stackIsPhysicalPath, err := FindAllStackConfigsInPathsForStack(
			atmosConfig,
			configAndStacksInfo.Stack,
			includeStackAbsPaths,
			excludeStackAbsPaths,
		)
		if err != nil {
			return atmosConfig, err
		}

		if len(stackConfigFilesAbsolutePaths) < 1 {
			j, err := u.ConvertToYAML(includeStackAbsPaths)
			if err != nil {
				return atmosConfig, err
			}
			errorMessage := fmt.Sprintf("\nno stack manifests found in the provided "+
				"paths:\n%s\n\nCheck if `base_path`, 'stacks.base_path', 'stacks.included_paths' and 'stacks.excluded_paths' are correctly set in CLI config "+
				"files or ENV vars.", j)
			return atmosConfig, errors.New(errorMessage)
		}

		atmosConfig.StackConfigFilesAbsolutePaths = stackConfigFilesAbsolutePaths
		atmosConfig.StackConfigFilesRelativePaths = stackConfigFilesRelativePaths

		if stackIsPhysicalPath {
			u.LogTrace(fmt.Sprintf("\nThe stack '%s' matches the stack manifest %s\n",
				configAndStacksInfo.Stack,
				stackConfigFilesRelativePaths[0]),
			)
			atmosConfig.StackType = "Directory"
		} else {
			// The stack is a logical name
			atmosConfig.StackType = "Logical"
		}
	}

	atmosConfig.Initialized = true
	return atmosConfig, nil
}
