package hooks

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
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
	log.Info("storing terraform output", "outputKey", outputKey, "store", hook.Name, "key", key, "value", outputValue)

	return store.Set(info.Stack, info.ComponentFromArg, key, outputValue)
}

func processStoreCommand(atmosConfig schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, hook Hook) error {
	if len(hook.Outputs) == 0 {
		log.Info("skipping hook. no outputs configured.", "hook", hook.Name, "outputs", hook.Outputs)
		return nil
	}

	log.Info("executing 'after-terraform-apply' hook", "hook", hook.Name, "command", hook.Command)
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
		u.LogErrorAndExit(err)
	}

	sections, err := e.ExecuteDescribeComponent(info.ComponentFromArg, info.Stack, true)
	if err != nil {
		u.LogErrorAndExit(err)
	}

	if isTerraformApplyCommand(&info.SubCommand) {
		hooks := Hooks{}
		hooks, err = hooks.ConvertToHooks(sections["hooks"].(map[string]any))
		if err != nil {
			u.LogErrorAndExit(fmt.Errorf("invalid hooks section %v", sections["hooks"]))
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
