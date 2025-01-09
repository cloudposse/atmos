package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/internal/tui/templates"
	cfg "github.com/cloudposse/atmos/pkg/config"
	h "github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	cc "github.com/ivanpirog/coloredcobra"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(cmd, cmd, args)
	},
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

		ctx := context.WithValue(context.Background(), contextKey(atmosInfoKey), info)
		RootCmd.SetContext(ctx)

		// Check Atmos configuration
		checkAtmosConfig()
	},
	PostRunE: func(cmd *cobra.Command, args []string) error {
		info, ok := RootCmd.Context().Value(atmosInfoKey).(schema.ConfigAndStacksInfo)
		if !ok {
			return fmt.Errorf("failed to retrieve atmos info from context")
		}
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
							return fmt.Errorf("store %q not found in configuration", hook.Name)
						}
						u.LogInfo(atmosConfig, fmt.Sprintf("  storing terraform output '%s' in store '%s' with key '%s' and value %v", outputKey, hook.Name, key, outputValue))

						err = store.Set(info.Stack, info.ComponentFromArg, key, outputValue)
						if err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
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

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
	var argsAfterDoubleDash []string
	var finalArgs = args

	doubleDashIndex := lo.IndexOf(args, "--")
	if doubleDashIndex > 0 {
		finalArgs = lo.Slice(args, 0, doubleDashIndex)
		argsAfterDoubleDash = lo.Slice(args, doubleDashIndex+1, len(args))
	}

	info, _ := e.ProcessCommandLineArgs("terraform", cmd, finalArgs, argsAfterDoubleDash)
	// Exit on help
	if info.NeedHelp || (info.SubCommand == "" && info.SubCommand2 == "") {
		if info.SubCommand != "" && info.SubCommand != "--help" && info.SubCommand != "help" {
			suggestions := cmd.SuggestionsFor(args[0])
			if !Contains(suggestions, args[0]) {
				if len(suggestions) > 0 {
					fmt.Printf("Unknown command: '%s'\n\nDid you mean this?\n", args[0])
					for _, suggestion := range suggestions {
						fmt.Printf("  %s\n", suggestion)
					}
				} else {
					fmt.Printf(`Error: Unknkown command %q for %q`+"\n", args[0], cmd.CommandPath())
				}
				fmt.Printf(`Run '%s --help' for usage`+"\n", cmd.CommandPath())
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
		}
		// check if this is terraform --help command. TODO: check if this is the best way to do
		if cmd == actualCmd {
			template := templates.GenerateFromBaseTemplate(actualCmd.Use, []templates.HelpTemplateSections{
				templates.LongDescription,
				templates.Usage,
				templates.Aliases,
				templates.Examples,
				templates.AvailableCommands,
				templates.Flags,
				templates.GlobalFlags,
				templates.NativeCommands,
				templates.DoubleDashHelp,
				templates.Footer,
			})
			actualCmd.SetUsageTemplate(template)
			cc.Init(&cc.Config{
				RootCmd:  actualCmd,
				Headings: cc.HiCyan + cc.Bold + cc.Underline,
				Commands: cc.HiGreen + cc.Bold,
				Example:  cc.Italic,
				ExecName: cc.Bold,
				Flags:    cc.Bold,
			})
		}

		err := actualCmd.Help()
		if err != nil {
			return err
		}

		return nil
	}
	// Check Atmos configuration
	checkAtmosConfig()

	err := e.ExecuteTerraform(info)
	if err != nil {
		u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
	}
	return nil
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform <terraform_command> <component> -s <stack>")
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}
