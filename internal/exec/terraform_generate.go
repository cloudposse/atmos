package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"path"
)

// ExecuteTerraformGenerateBackend executes `terraform generate backend` command
func ExecuteTerraformGenerateBackend(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	component := args[0]

	var configAndStacksInfo c.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.Stack = stack
	configAndStacksInfo.ComponentType = "terraform"

	configAndStacksInfo, err = ProcessStacks(configAndStacksInfo)
	if err != nil {
		return err
	}

	if configAndStacksInfo.ComponentBackendType == "" {
		return errors.New(fmt.Sprintf("\n'backend_type' is missing for the '%s' component.\n", component))
	}

	if configAndStacksInfo.ComponentBackendSection == nil {
		return errors.New(fmt.Sprintf("\nCould not find 'backend' config for the '%s' component.\n", component))
	}

	var componentBackendConfig = generateComponentBackendConfig(configAndStacksInfo.ComponentBackendType, configAndStacksInfo.ComponentBackendSection)

	fmt.Println()
	color.Cyan("Component backend config:\n\n")
	err = utils.PrintAsJSON(componentBackendConfig)
	if err != nil {
		return err
	}

	// Check if the `backend` section has `workspace_key_prefix`
	if _, ok := configAndStacksInfo.ComponentBackendSection["workspace_key_prefix"].(string); !ok {
		return errors.New(fmt.Sprintf("\nBackend config for the '%s' component is missing 'workspace_key_prefix'\n", component))
	}

	// Write backend config to file
	var backendFileName = path.Join(
		c.Config.BasePath,
		c.Config.Components.Terraform.BasePath,
		configAndStacksInfo.ComponentFolderPrefix,
		configAndStacksInfo.FinalComponent,
		"backend.tf.json",
	)

	fmt.Println()
	color.Cyan("Writing the backend config to file:")
	fmt.Println(backendFileName)
	err = utils.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0644)
	if err != nil {
		return err
	}

	fmt.Println()
	return nil
}

// ExecuteTerraformGenerateBackends executes `terraform generate backends` command
func ExecuteTerraformGenerateBackends(cmd *cobra.Command, args []string) error {
	return nil
}
