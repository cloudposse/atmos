package config

import (
	"fmt"
	"path/filepath"

	u "github.com/cloudposse/atmos/pkg/utils"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/pkg/errors"
)

func processAtmosConfigs(configAndStacksInfo *schema.ConfigAndStacksInfo) (schema.AtmosConfiguration, error) {
	atmosConfig, err := LoadConfig(configAndStacksInfo)
	if err != nil {
		return atmosConfig, err
	}
	atmosConfig.ProcessSchemas()

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
	return atmosConfig, nil
}

// atmosConfigAbsolutePaths Converts paths to absolute paths.
func atmosConfigAbsolutePaths(atmosConfig *schema.AtmosConfiguration) error {
	// Convert stacks base path to absolute path
	stacksBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)
	stacksBaseAbsPath, err := filepath.Abs(stacksBasePath)
	if err != nil {
		return err
	}
	atmosConfig.StacksBaseAbsolutePath = stacksBaseAbsPath

	// Convert the included stack paths to absolute paths
	includeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, atmosConfig.Stacks.IncludedPaths)
	if err != nil {
		return err
	}
	atmosConfig.IncludeStackAbsolutePaths = includeStackAbsPaths

	// Convert the excluded stack paths to absolute paths
	excludeStackAbsPaths, err := u.JoinAbsolutePathWithPaths(stacksBaseAbsPath, atmosConfig.Stacks.ExcludedPaths)
	if err != nil {
		return err
	}
	atmosConfig.ExcludeStackAbsolutePaths = excludeStackAbsPaths

	// Convert terraform dir to absolute path
	terraformBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath)
	terraformDirAbsPath, err := filepath.Abs(terraformBasePath)
	if err != nil {
		return err
	}
	atmosConfig.TerraformDirAbsolutePath = terraformDirAbsPath

	// Convert helmfile dir to absolute path
	helmfileBasePath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath)
	helmfileDirAbsPath, err := filepath.Abs(helmfileBasePath)
	if err != nil {
		return err
	}
	atmosConfig.HelmfileDirAbsolutePath = helmfileDirAbsPath

	return nil
}

func processStackConfigs(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo, includeStackAbsPaths, excludeStackAbsPaths []string) error {
	// If the specified stack name is a logical name, find all stack manifests in the provided paths
	stackConfigFilesAbsolutePaths, stackConfigFilesRelativePaths, stackIsPhysicalPath, err := FindAllStackConfigsInPathsForStack(
		*atmosConfig,
		configAndStacksInfo.Stack,
		includeStackAbsPaths,
		excludeStackAbsPaths,
	)
	if err != nil {
		return err
	}

	if len(stackConfigFilesAbsolutePaths) < 1 {
		j, err := u.ConvertToYAML(includeStackAbsPaths)
		if err != nil {
			return err
		}
		errorMessage := fmt.Sprintf("\nno stack manifests found in the provided "+
			"paths:\n%s\n\nCheck if `base_path`, 'stacks.base_path', 'stacks.included_paths' and 'stacks.excluded_paths' are correctly set in CLI config "+
			"files or ENV vars.", j)
		return errors.New(errorMessage)
	}

	atmosConfig.StackConfigFilesAbsolutePaths = stackConfigFilesAbsolutePaths
	atmosConfig.StackConfigFilesRelativePaths = stackConfigFilesRelativePaths

	if stackIsPhysicalPath {
		log.Debug(fmt.Sprintf("\nThe stack '%s' matches the stack manifest %s\n",
			configAndStacksInfo.Stack,
			stackConfigFilesRelativePaths[0]))
		atmosConfig.StackType = "Directory"
	} else {
		// The stack is a logical name
		atmosConfig.StackType = "Logical"
	}

	return nil
}
