package cmd

import (
	"fmt"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands",
	Long:               `This command executes Terraform commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		//checkAtmosConfig()

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

		// Exit on help
		if info.NeedHelp {
			if info.SubCommand != "" {
				fmt.Printf(`Error: Unknkown command %q for %q`+"\n", args[0], cmd.CommandPath())
				fmt.Printf(`Run '%s --help' for usage`+"\n", cmd.CommandPath())
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			// Check for the latest Atmos release on GitHub and print update message
			CheckForAtmosUpdateAndPrintMessage(atmosConfig)
			return nil
		}
		// Check Atmos configuration
		checkAtmosConfig()

		err = e.ExecuteTerraform(info)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}
		return nil
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform <terraform_command> <component> -s <stack>")
	RootCmd.AddCommand(terraformCmd)
}
