package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// terraformGenerateBackendsCmd generates backend configs for all terraform components
var terraformGenerateBackendsCmd = &cobra.Command{
	Use:                "backends",
	Short:              "Execute 'terraform generate backends' command",
	Long:               `This command generates backend configs for all terraform components`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteTerraformGenerateBackendsCmd(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	terraformGenerateBackendsCmd.DisableFlagParsing = false

	terraformGenerateBackendsCmd.PersistentFlags().String("file-template", "",
		"Backend template (the file path, file name, and file extension).\n"+
			"Supports absolute and relative paths.\n"+
			"Supports context tokens: {namespace}, {tenant}, {environment}, {region}, {stage}, {base-component}, {component}, {component-path}.\n"+
			"atmos terraform generate backends --file-template {component-path}/{tenant}/{environment}-{stage}.tf.json\n"+
			"atmos terraform generate backends --file-template {component-path}/backends/{tenant}-{environment}-{stage}.tf.json\n"+
			"atmos terraform generate backends --file-template backends/{tenant}/{environment}/{region}/{component}.tf\n"+
			"atmos terraform generate backends --file-template backends/{tenant}-{environment}-{stage}-{component}.tf\n"+
			"atmos terraform generate backends --file-template /{tenant}/{stage}/{region}/{component}.tf\n"+
			"All subdirectories in the path will be created automatically\n"+
			"If '--file-template' flag is not specified, all backend config files will be written to the corresponding terraform component folders.",
	)

	terraformGenerateBackendsCmd.PersistentFlags().String("stacks", "",
		"Only process the specified stacks (comma-separated values).\n"+
			"atmos terraform generate backends --file-template <file_template> --stacks <stack1>,<stack2>\n"+
			"The filter can contain names of the top-level stack config files (including subfolder paths), and 'atmos' stack names (derived from the context vars)\n"+
			"atmos terraform generate backends --stacks orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2\n"+
			"atmos terraform generate backends --stacks tenant1-ue2-staging,tenant1-ue2-prod\n"+
			"atmos terraform generate backends --stacks orgs/cp/tenant1/staging/us-east-2,tenant1-ue2-prod",
	)

	terraformGenerateBackendsCmd.PersistentFlags().String("components", "",
		"Only process the specified components (comma-separated values).\n"+
			"atmos terraform generate backends --file-template <file_template> --components <component1>,<component2>",
	)

	terraformGenerateBackendsCmd.PersistentFlags().String("format", "hcl", "Output format.\n"+
		"Supported formats: hcl, json ('hcl' is default).\n"+
		"atmos terraform generate backends --format=hcl|json")

	terraformGenerateCmd.AddCommand(terraformGenerateBackendsCmd)
}
