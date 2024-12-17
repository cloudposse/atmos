package cmd

import (
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	cc "github.com/ivanpirog/coloredcobra"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands",
	Long:               `This command executes Terraform commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run:                terraformRun,
}

func terraformRun(cmd *cobra.Command, args []string) {
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
		u.LogErrorAndExit(schema.CliConfiguration{}, err)
	}

	// Exit on help
	if info.NeedHelp {
		// Check for the latest Atmos release on GitHub and print update message
		template := templates.GenerateFromBaseTemplate(cmd.Use, []templates.HelpTemplateSections{
			templates.LongDescription,
			templates.Usage,
			templates.Aliases,
			templates.Examples,
			templates.AvailableCommands,
			templates.Flags,
			templates.GlobalFlags,
			templates.NativeCommands,
			templates.Footer,
		})

		cmd.SetUsageTemplate(template)
		cc.Init(&cc.Config{
			RootCmd:  cmd,
			Headings: cc.HiCyan + cc.Bold + cc.Underline,
			Commands: cc.HiGreen + cc.Bold,
			Example:  cc.Italic,
			ExecName: cc.Bold,
			Flags:    cc.Bold,
		})

		cmd.Help()
		CheckForAtmosUpdateAndPrintMessage(cliConfig)
		return
	}
	// Check Atmos configuration
	checkAtmosConfig()

	err = e.ExecuteTerraform(info)
	if err != nil {
		u.LogErrorAndExit(schema.CliConfiguration{}, err)
	}

}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform <terraform_command> <component> -s <stack>")
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}
