package exec

import (
	tui "github.com/cloudposse/atmos/internal/tui/atmos"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/samber/lo"
)

// ExecuteAtmosCmd executes `atmos` command
func ExecuteAtmosCmd() error {
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
		"terraform shell",
		"validate component",
		"describe component",
		"describe dependents",
	}

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
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

	componentsStacksMap := lo.MapEntries(stacksComponentsMap, func(k string, v []string) (string, []string) {
		return k, v
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
