package hooks

import (
	"fmt"
	"strings"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

func isTerraformApplyCommand(cmd *string) bool {
	return strings.ToLower(*cmd) == "apply" || strings.ToLower(*cmd) == "deploy"
}

func isStoreCommand(cmd *string) bool {
	return strings.ToLower(*cmd) == "store"
}

func getOutputValue(atmosConfig schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, value string) (string, any) {
	outputKey := strings.TrimPrefix(value, ".")
	var outputValue any

	if strings.Index(value, ".") == 0 {
		outputValue = e.GetTerraformOutput(&atmosConfig, info.Stack, info.ComponentFromArg, outputKey, true)
	} else {
		outputValue = value
	}
	return outputKey, outputValue
}

func storeOutput(atmosConfig schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, hook Hook, key string, outputKey string, outputValue any) error {
	store := atmosConfig.Stores[hook.Name]
	if store == nil {
		return fmt.Errorf("store %q not found in configuration", hook.Name)
	}
	u.LogInfo(atmosConfig, fmt.Sprintf("  storing terraform output '%s' in store '%s' with key '%s' and value %v", outputKey, hook.Name, key, outputValue))

	return store.Set(info.Stack, info.ComponentFromArg, key, outputValue)
}

func processStoreCommand(atmosConfig schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, hook Hook) error {
	if len(hook.Outputs) == 0 {
		u.LogInfo(atmosConfig, fmt.Sprintf("skipping hook %q: no outputs configured", hook.Name))
		return nil
	}

	u.LogInfo(atmosConfig, fmt.Sprintf("\nexecuting 'after-terraform-apply' hook '%s' with command '%s'", hook.Name, hook.Command))
	for key, value := range hook.Outputs {
		outputKey, outputValue := getOutputValue(atmosConfig, info, value)

		err := storeOutput(atmosConfig, info, hook, key, outputKey, outputValue)
		if err != nil {
			return err
		}
	}
	return nil
}

func RunE(cmd *cobra.Command, args []string, info *schema.ConfigAndStacksInfo) error {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}

	sections, err := e.ExecuteDescribeComponent(info.ComponentFromArg, info.Stack, true)
	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}

	if isTerraformApplyCommand(&info.SubCommand) {
		hooks := Hooks{}
		hooks, err = hooks.ConvertToHooks(sections["hooks"].(map[string]any))
		if err != nil {
			u.LogErrorAndExit(atmosConfig, fmt.Errorf("invalid hooks section %v", sections["hooks"]))
		}

		for _, hook := range hooks {
			if isStoreCommand(&hook.Command) {
				err = processStoreCommand(atmosConfig, info, hook)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}
