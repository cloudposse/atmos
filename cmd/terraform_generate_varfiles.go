package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// terraformGenerateVarfilesCmd generates varfiles for all terraform components in all stacks
var terraformGenerateVarfilesCmd = &cobra.Command{
	Use:                "varfiles",
	Short:              "Execute 'terraform generate varfiles' command",
	Long:               `This command generates varfiles for all terraform components in all stacks`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteTerraformGenerateVarfilesCmd(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	terraformGenerateVarfilesCmd.DisableFlagParsing = false

	terraformGenerateVarfilesCmd.PersistentFlags().String("file-template", "",
		"Varfile template (the file path, file name, and file extension).\n"+
			"Supports absolute and relative paths.\n"+
			"Supports context tokens: {namespace}, {tenant}, {environment}, {region}, {stage}, {component}, {component-path}.\n"+
			"atmos terraform generate varfiles --file-template {component-path}/{environment}-{stage}.tfvars.json\n"+
			"atmos terraform generate varfiles --file-template /configs/{tenant}/{environment}/{stage}/{component}.json\n"+
			"atmos terraform generate varfiles --file-template /{tenant}/{stage}/{region}/{component}.yaml\n"+
			"All sub-folders in the path will be created automatically.\n"+
			"The file extension from the template will be applied to all generated varfiles.",
	)

	terraformGenerateVarfilesCmd.PersistentFlags().String("stacks", "",
		"Only process the specified stacks (comma-separated values).\n"+
			"atmos terraform generate varfiles --file-template <file_template> --stacks <stack1>,<stack2>",
	)

	terraformGenerateVarfilesCmd.PersistentFlags().String("components", "",
		"Only process the specified components (comma-separated values).\n"+
			"atmos terraform generate varfiles --file-template <file_template> --components=<component1>,<component2>",
	)

	terraformGenerateVarfilesCmd.PersistentFlags().String("format", "json", "Output format.\n"+
		"atmos terraform generate varfiles --file-template <file_template> --format=json/yaml ('json' is default)")

	err := terraformGenerateVarfilesCmd.MarkPersistentFlagRequired("file-template")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateVarfilesCmd)
}
