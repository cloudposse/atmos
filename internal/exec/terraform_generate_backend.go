package exec

import (
	"fmt"
	"path/filepath"

	log "github.com/charmbracelet/log"
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

	processTemplates, err := flags.GetBool("process-templates")
	if err != nil {
		return err
	}

	processYamlFunctions, err := flags.GetBool("process-functions")
	if err != nil {
		return err
	}

	skip, err := flags.GetStringSlice("skip")
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
	info.CliArgs = []string{"terraform", "generate", "backend"}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	info, err = ProcessStacks(&atmosConfig, info, true, processTemplates, processYamlFunctions, skip)
	if err != nil {
		return err
	}

	if info.ComponentBackendType == "" {
		return fmt.Errorf("'backend_type' is missing for the '%s' component", component)
	}

	if info.ComponentBackendSection == nil {
		return fmt.Errorf("could not find 'backend' config for the '%s' component", component)
	}

	componentBackendConfig, err := generateComponentBackendConfig(info.ComponentBackendType, info.ComponentBackendSection, info.TerraformWorkspace)
	if err != nil {
		return err
	}

	log.Debug("Component backend", "config", componentBackendConfig)

	// Check if the `backend` section has `workspace_key_prefix` when `backend_type` is `s3`
	if info.ComponentBackendType == "s3" {
		if _, ok := info.ComponentBackendSection["workspace_key_prefix"].(string); !ok {
			return fmt.Errorf("backend config for the '%s' component is missing 'workspace_key_prefix'", component)
		}
	}

	// Check if the `backend` section has `bucket` when `backend_type` is `gcs`
	if info.ComponentBackendType == "gcs" {
		if _, ok := info.ComponentBackendSection["bucket"].(string); !ok {
			return fmt.Errorf("backend config for the '%s' component is missing 'bucket'", component)
		}
	}

	// Write the backend config to a file
	backendFilePath := filepath.Join(
		atmosConfig.BasePath,
		atmosConfig.Components.Terraform.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
		"backend.tf.json",
	)

	log.Debug("Writing the backend config to file", "file", backendFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsJSON(backendFilePath, componentBackendConfig, 0o644)
		if err != nil {
			return err
		}
	}

	return nil
}
