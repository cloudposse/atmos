package exec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

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

	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	info.ComponentFromArg = component
	info.Stack = stack
	info.ComponentType = "terraform"

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	info, err = ProcessStacks(atmosConfig, info, true, true, true, nil)
	if err != nil {
		return err
	}

	if info.ComponentBackendType == "" {
		return fmt.Errorf("\n'backend_type' is missing for the '%s' component.\n", component)
	}

	if info.ComponentBackendSection == nil {
		return fmt.Errorf("\nCould not find 'backend' config for the '%s' component.\n", component)
	}

	componentBackendConfig, err := generateComponentBackendConfig(info.ComponentBackendType, info.ComponentBackendSection, info.TerraformWorkspace)
	if err != nil {
		return err
	}

	u.LogDebug("Component backend config:\n\n")

	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		j, err := u.ConvertToJSON(componentBackendConfig)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, j)

		err = u.PrintAsJSONToFileDescriptor(atmosConfig, componentBackendConfig)
		if err != nil {
			return err
		}
	}

	// Check if the `backend` section has `workspace_key_prefix` when `backend_type` is `s3`
	if info.ComponentBackendType == "s3" {
		if _, ok := info.ComponentBackendSection["workspace_key_prefix"].(string); !ok {
			return fmt.Errorf("backend config for the '%s' component is missing 'workspace_key_prefix'", component)
		}
	}

	// Write backend config to file
	backendFilePath := filepath.Join(
		atmosConfig.BasePath,
		atmosConfig.Components.Terraform.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
		"backend.tf.json",
	)

	u.LogDebug("\nWriting the backend config to file:")
	u.LogDebug(backendFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsJSON(backendFilePath, componentBackendConfig, 0o644)
		if err != nil {
			return err
		}
	}

	return nil
}
