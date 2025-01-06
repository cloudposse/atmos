package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	h "github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type contextKey string

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands",
	Long:               `This command executes Terraform commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	PreRun: func(cmd *cobra.Command, args []string) {
		var argsAfterDoubleDash []string
		var finalArgs = args

		doubleDashIndex := lo.IndexOf(args, "--")
		if doubleDashIndex > 0 {
			finalArgs = lo.Slice(args, 0, doubleDashIndex)
			argsAfterDoubleDash = lo.Slice(args, doubleDashIndex+1, len(args))
		}

		info, err := e.ProcessCommandLineArgs("terraform", cmd, finalArgs, argsAfterDoubleDash)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}

		ctx := context.WithValue(context.Background(), contextKey("atmos_info"), info)
		RootCmd.SetContext(ctx)

		// Check Atmos configuration
		checkAtmosConfig()
	},
	Run: func(cmd *cobra.Command, args []string) {
		info := RootCmd.Context().Value(contextKey("atmos_info")).(schema.ConfigAndStacksInfo)

		// Exit on help
		if info.NeedHelp {
			// Check for the latest Atmos release on GitHub and print update message
			CheckForAtmosUpdateAndPrintMessage(atmosConfig)
			return
		}
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraform(info)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}

	},
	PostRun: func(cmd *cobra.Command, args []string) {
		info := RootCmd.Context().Value(contextKey("atmos_info")).(schema.ConfigAndStacksInfo)
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			u.LogErrorAndExit(atmosConfig, err)
		}

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
						u.LogInfo(atmosConfig, fmt.Sprintf("  storing terraform output '%s' in store '%s' with key '%s' and value %v", outputKey, hook.Name, key, outputValue))
						err := store.Set(info.Stack, info.ComponentFromArg, key, outputValue)

						if err != nil {
							u.LogErrorAndExit(atmosConfig, err)
						}
					}
				}
			}

		}
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform <terraform_command> <component> -s <stack>")
	RootCmd.AddCommand(terraformCmd)
}
