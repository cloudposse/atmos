package exec

import (
	_ "embed"
	"fmt"

	tui "github.com/cloudposse/atmos/internal/tui/help"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed help_docs/atmos_terraform_help.md
var atmosTerraformHelp string

//go:embed help_docs/atmos_helmfile_help.md
var atmosHelmfileHelp string

//go:embed help_docs/atmos_terraform_clean_help.md
var atmosTerraformCleanHelp string

//go:embed help_docs/atmos_terraform_deploy_help.md
var atmosTerraformDeployHelp string

//go:embed help_docs/atmos_terraform_shell_help.md
var atmosTerraformShellHelp string

//go:embed help_docs/atmos_terraform_workspace_help.md
var atmosTerraformWorkspaceHelp string

// processHelp processes help commands
func processHelp(componentType string, command string) error {
	cliConfig := schema.CliConfiguration{}
	var content string

	if len(command) == 0 {
		if componentType == "terraform" {
			content = atmosTerraformHelp
		}
		if componentType == "helmfile" {
			content = atmosHelmfileHelp
		}
	} else if componentType == "terraform" && command == "clean" {
		content = atmosTerraformCleanHelp
	} else if componentType == "terraform" && command == "deploy" {
		content = atmosTerraformDeployHelp
	} else if componentType == "terraform" && command == "shell" {
		content = atmosTerraformShellHelp
	} else if componentType == "terraform" && command == "workspace" {
		content = atmosTerraformWorkspaceHelp
	} else {
		// Execute `--help` for the native terraform and Helmfile commands
		err := ExecuteShellCommand(cliConfig, componentType, []string{command, "--help"}, "", nil, false, "")
		if err != nil {
			return err
		}
	}

	// Start the help UI
	if content != "" {
		_, err := tui.Execute(content)
		fmt.Println()
		if err != nil {
			return err
		}
	}

	return nil
}
