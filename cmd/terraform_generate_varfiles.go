package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformGenerateVarfilesCmd generates varfiles for all terraform components in all stacks
var terraformGenerateVarfilesCmd = &cobra.Command{
	Use:                "varfiles",
	Short:              "Generate varfiles for all Terraform components in all stacks",
	Long:               "This command generates varfiles for all Atmos Terraform components across all stacks.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateVarfilesCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}
	},
}

func init() {
	terraformGenerateVarfilesCmd.DisableFlagParsing = false

	terraformGenerateVarfilesCmd.PersistentFlags().String("file-template", "",
		"Varfile template (the file path, file name, and file extension).\n"+
			"Supports absolute and relative paths.\n"+
			"Supports context tokens: {namespace}, {tenant}, {environment}, {region}, {stage}, {base-component}, {component}, {component-path}.\n"+
			"atmos terraform generate varfiles --file-template {component-path}/{environment}-{stage}.tfvars.json\n"+
			"atmos terraform generate varfiles --file-template /configs/{tenant}/{environment}/{stage}/{component}.json\n"+
			"atmos terraform generate varfiles --file-template /{tenant}/{stage}/{region}/{component}.yaml\n"+
			"All subdirectories in the path will be created automatically.",
	)

	terraformGenerateVarfilesCmd.PersistentFlags().String("stacks", "",
		"Only process the specified stacks (comma-separated values).\n"+
			"atmos terraform generate varfiles --file-template <file_template> --stacks <stack1>,<stack2>\n"+
			"The filter can contain names of the top-level stack manifests (including subfolder paths), and 'atmos' stack names (derived from the context vars)\n"+
			"atmos terraform generate varfiles --stacks orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2\n"+
			"atmos terraform generate varfiles --stacks tenant1-ue2-staging,tenant1-ue2-prod\n"+
			"atmos terraform generate varfiles --stacks orgs/cp/tenant1/staging/us-east-2,tenant1-ue2-prod",
	)

	terraformGenerateVarfilesCmd.PersistentFlags().String("components", "",
		"Generate Terraform '.tfvar' files only for the specified 'atmos' components (use comma-separated values to specify multiple components).\n"+
			"atmos terraform generate varfiles --file-template <file_template> --components <component1>,<component2>",
	)

	terraformGenerateVarfilesCmd.PersistentFlags().String("format", "json", "Output format.\n"+
		"Supported formats: json, yaml, hcl ('json' is default).\n"+
		"atmos terraform generate varfiles --file-template <file_template> --format=json|yaml|hcl")

	err := terraformGenerateVarfilesCmd.MarkPersistentFlagRequired("file-template")
	if err != nil {
		u.LogErrorAndExit(schema.CliConfiguration{}, err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateVarfilesCmd)
}
