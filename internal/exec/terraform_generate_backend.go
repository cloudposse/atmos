package exec

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"path"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteTerraformGenerateBackendCmd executes `terraform generate backend` command
func ExecuteTerraformGenerateBackendCmd(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	component := args[0]

	var info cfg.ConfigAndStacksInfo
	info.ComponentFromArg = component
	info.Stack = stack
	info.ComponentType = "terraform"

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		u.PrintErrorToStdError(err)
		return err
	}

	info, err = ProcessStacks(cliConfig, info, true)
	if err != nil {
		return err
	}

	if info.ComponentBackendType == "" {
		return fmt.Errorf("\n'backend_type' is missing for the '%s' component.\n", component)
	}

	if info.ComponentBackendSection == nil {
		return fmt.Errorf("\nCould not find 'backend' config for the '%s' component.\n", component)
	}

	componentBackendConfig := generateComponentBackendConfig(info.ComponentBackendType, info.ComponentBackendSection)

	u.PrintInfoVerbose(cliConfig.Logs.Verbose, "Component backend config:\n\n")
	err = u.PrintAsJSON(componentBackendConfig)
	if err != nil {
		return err
	}

	// Check if the `backend` section has `workspace_key_prefix`
	if _, ok := info.ComponentBackendSection["workspace_key_prefix"].(string); !ok {
		return fmt.Errorf("\nBackend config for the '%s' component is missing 'workspace_key_prefix'\n", component)
	}

	// Write backend config to file
	var backendFilePath = path.Join(
		cliConfig.BasePath,
		cliConfig.Components.Terraform.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
		"backend.tf.json",
	)

	fmt.Println()
	u.PrintInfo("Writing the backend config to file:")
	u.PrintMessage(backendFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsJSON(backendFilePath, componentBackendConfig, 0644)
		if err != nil {
			return err
		}
	}

	fmt.Println()
	return nil
}
