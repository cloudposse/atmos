package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/samber/lo"
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

	cliConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return err
	}

	stacksComponentsMap, err := ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false)
	if err != nil {
		return err
	}

	componentsStacksMap := lo.MapEntries(stacksComponentsMap, func(k string, v any) (string, any) {
		return k, v
	})

	app, err := tui.Execute(commands, stacksComponentsMap, componentsStacksMap)
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
