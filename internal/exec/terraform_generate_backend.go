package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
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

	var info c.ConfigAndStacksInfo
	info.ComponentFromArg = component
	info.Stack = stack
	info.ComponentType = "terraform"

	info, err = ProcessStacks(info, true)
	if err != nil {
		return err
	}

	if info.ComponentBackendType == "" {
		return errors.New(fmt.Sprintf("\n'backend_type' is missing for the '%s' component.\n", component))
	}

	if info.ComponentBackendSection == nil {
		return errors.New(fmt.Sprintf("\nCould not find 'backend' config for the '%s' component.\n", component))
	}

	var componentBackendConfig = generateComponentBackendConfig(info.ComponentBackendType, info.ComponentBackendSection)

	u.PrintInfoVerbose("Component backend config:\n\n")
	err = u.PrintAsJSON(componentBackendConfig)
	if err != nil {
		return err
	}

	// Check if the `backend` section has `workspace_key_prefix`
	if _, ok := info.ComponentBackendSection["workspace_key_prefix"].(string); !ok {
		return errors.New(fmt.Sprintf("\nBackend config for the '%s' component is missing 'workspace_key_prefix'\n", component))
	}

	// Write backend config to file
	var backendFilePath = path.Join(
		c.Config.BasePath,
		c.Config.Components.Terraform.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
		"backend.tf.json",
	)

	fmt.Println()
	u.PrintInfo("Writing the backend config to file:")
	fmt.Println(backendFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsJSON(backendFilePath, componentBackendConfig, 0644)
		if err != nil {
			return err
		}
	}

	fmt.Println()
	return nil
}
