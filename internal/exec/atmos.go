package exec

import (
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"

	tui "github.com/cloudposse/atmos/internal/tui/stack_component_select"
)

// ExecuteAtmosCmd executes `atmos` command
func ExecuteAtmosCmd(cmd *cobra.Command, args []string) error {
	commands := []string{
		"terraform plan",
		"terraform apply",
		"terraform destroy",
		"terraform init",
		"terraform output",
		"terraform clean",
		"terraform workspace",
		"terraform refresh",
		"terraform show",
		"terraform validate",
		"terraform generate varfile",
		"terraform generate backend",
		"describe component",
		"describe dependents",
		"validate component",
		"helmfile diff",
		"helmfile apply",
		"helmfile generate varfile",
	}

	stacks := []string{
		"plat-ue2-dev",
		"plat-ue2-prod",
		"plat-ue2-staging",
		"plat-uw2-dev",
		"plat-uw2-prod",
		"plat-uw2-staging",
		"plat-gbl-dev",
		"plat-gbl-prod",
		"plat-gbl-staging",
	}

	components := []string{
		"vpc",
		"vpc-flow-logs-bucket",
	}

	app, err := tui.Execute(commands, components, stacks)
	if err != nil {
		return err
	}

	if !app.ExitStatusQuit() {
		selectedComponent := app.GetSelectedComponent()
		selectedStack := app.GetSelectedStack()

		data, err := ExecuteDescribeComponent(selectedComponent, selectedStack)
		if err != nil {
			return err
		}

		err = u.PrintAsYAML(data)
		if err != nil {
			return err
		}
	}

	return nil
}
