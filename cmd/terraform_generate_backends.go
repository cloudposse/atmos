package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformGenerateBackendsCmd generates backend configs for all terraform components
var terraformGenerateBackendsCmd = &cobra.Command{
	Use:                "backends",
	Short:              "Generate backend configurations for all Terraform components",
	Long:               "This command generates the backend configuration files for all Terraform components in the Atmos environment.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateBackendsCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	terraformGenerateBackendsCmd.DisableFlagParsing = false

	terraformGenerateBackendsCmd.PersistentFlags().String("file-template", "",
		"Backend file template (the file path, file name, and file extension).\n"+
			"Supports absolute and relative paths.\n"+
			"Supports context tokens: {namespace}, {tenant}, {environment}, {region}, {stage}, {base-component}, {component}, {component-path}.\n"+
			"atmos terraform generate backends --file-template {component-path}/{tenant}/{environment}-{stage}.tf.json --format json\n"+
			"atmos terraform generate backends --file-template {component-path}/backends/{tenant}-{environment}-{stage}.tf.json --format json\n"+
			"atmos terraform generate backends --file-template backends/{tenant}/{environment}/{region}/{component}.tf --format hcl\n"+
			"atmos terraform generate backends --file-template backends/{tenant}-{environment}-{stage}-{component}.tf\n"+
			"atmos terraform generate backends --file-template /{tenant}/{stage}/{region}/{component}.tf\n"+
			"atmos terraform generate backends --file-template backends/{tenant}-{environment}-{stage}-{component}.tfbackend --format backend-config\n"+
			"All subdirectories in the path will be created automatically\n"+
			"If '--file-template' flag is not specified, all backend config files will be written to the corresponding terraform component folders.",
	)

	terraformGenerateBackendsCmd.PersistentFlags().String("stacks", "",
		"Only process the specified stacks (comma-separated values).\n"+
			"atmos terraform generate backends --file-template <file_template> --stacks <stack1>,<stack2>\n"+
			"The filter can contain names of the top-level stack manifests (including subfolder paths), and 'atmos' stack names (derived from the context vars)\n"+
			"atmos terraform generate backends --stacks orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2\n"+
			"atmos terraform generate backends --stacks tenant1-ue2-staging,tenant1-ue2-prod\n"+
			"atmos terraform generate backends --stacks orgs/cp/tenant1/staging/us-east-2,tenant1-ue2-prod",
	)

	terraformGenerateBackendsCmd.PersistentFlags().String("components", "",
		"Only generate the backend files for the specified 'atmos' components (comma-separated values).\n"+
			"atmos terraform generate backends --file-template <file_template> --components <component1>,<component2>",
	)

	terraformGenerateBackendsCmd.PersistentFlags().String("format", "hcl", "Output format.\n"+
		"Supported formats: 'hcl', 'json', 'backend-config' ('hcl' is default).\n"+
		"atmos terraform generate backends --format=hcl|json|backend-config")

	terraformGenerateCmd.AddCommand(terraformGenerateBackendsCmd)
}
