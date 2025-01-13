package cmd

import (
	"fmt"
	"strings"

	e "github.com/cloudposse/atmos/internal/exec"
	h "github.com/cloudposse/atmos/pkg/hooks"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

type contextKey string

const atmosInfoKey contextKey = "atmos_info"

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands (e.g., plan, apply, destroy) using Atmos stack configurations",
	Long:               `This command allows you to execute Terraform commands, such as plan, apply, and destroy, using Atmos stack configurations for consistent infrastructure management.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	PostRun: func(cmd *cobra.Command, args []string) {
		info := getConfigAndStacksInfo("terraform", cmd, args)

		sections, err := e.ExecuteDescribeComponent(info.ComponentFromArg, info.Stack, true)
		if err != nil {
			u.LogErrorAndExit(atmosConfig, err)
		}

		if info.SubCommand == "apply" || info.SubCommand == "deploy" {
			hooks := h.Hooks{}
			hooks, err = hooks.ConvertToHooks(sections["hooks"].(map[string]any))
			if err != nil {
				u.LogErrorAndExit(atmosConfig, fmt.Errorf("invalid hooks section %v", sections["hooks"]))
			}

			for _, hook := range hooks {
				if strings.ToLower(hook.Command) == "store" {
					if len(hook.Outputs) == 0 {
						u.LogInfo(atmosConfig, fmt.Sprintf("skipping hook %q: no outputs configured", hook.Name))
						continue
					}
					u.LogInfo(atmosConfig, fmt.Sprintf("\nexecuting 'after-terraform-apply' hook '%s' with command '%s'", hook.Name, hook.Command))
					for key, value := range hook.Outputs {
						var outputValue any
						outputKey := strings.TrimPrefix(value, ".")

						if strings.Index(value, ".") == 0 {
							outputValue = e.GetTerraformOutput(&atmosConfig, info.Stack, info.ComponentFromArg, outputKey, true)
						} else {
							outputValue = value
						}

						store := atmosConfig.Stores[hook.Name]
						if store == nil {
							u.LogErrorAndExit(atmosConfig, fmt.Errorf("store %q not found in configuration", hook.Name))
						}
						u.LogInfo(atmosConfig, fmt.Sprintf("  storing terraform output '%s' in store '%s' with key '%s' and value %v", outputKey, hook.Name, key, outputValue))

						err = store.Set(info.Stack, info.ComponentFromArg, key, outputValue)
						if err != nil {
							u.LogErrorAndExit(atmosConfig, err)
						}
					}
				}
			}
		}
	},
}

// Contains checks if a slice of strings contains an exact match for the target string.
func Contains(slice []string, target string) bool {
	for _, item := range slice {
		if item == target {
			return true
		}
	}
	return false
}

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) {
	info := getConfigAndStacksInfo("terraform", cmd, args)
	err := e.ExecuteTerraform(info)
	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform <terraform_command> <component> -s <stack>")
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}
