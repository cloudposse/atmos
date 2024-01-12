package exec

import (
	tui "github.com/cloudposse/atmos/internal/tui/stack_component_select"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
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
		"validate component",
		"describe component",
		"describe dependents",
	}

	cliConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return err
	}

	stacksMap, err := ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false)
	if err != nil {
		return err
	}

	stacksComponentsMap := lo.MapEntries(stacksMap, func(k string, v any) (string, []string) {
		if v2, ok := v.(map[string]any); ok {
			if v3, ok := v2["components"].(map[string]any); ok {
				if v4, ok := v3["terraform"].(map[string]any); ok {
					return k, lo.Keys(v4)
				}
			}
		}
		return k, nil
	})

	componentsStacksMap := lo.MapEntries(stacksMap, func(k string, v any) (string, []string) {
		return k, nil
	})

	app, err := tui.Execute(commands, stacksComponentsMap, componentsStacksMap)
	if err != nil {
		return err
	}

	if app.ExitStatusQuit() {
		return nil
	}

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

	return nil
}
