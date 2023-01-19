package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// atlantisGenerateRepoConfigCmd generates repository configuration for Atlantis
var atlantisGenerateRepoConfigCmd = &cobra.Command{
	Use:                "repo-config",
	Short:              "Execute 'atlantis generate repo-config`",
	Long:               "This command generates repository configuration for Atlantis",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteAtlantisGenerateRepoConfigCmd(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	atlantisGenerateRepoConfigCmd.DisableFlagParsing = false

	atlantisGenerateRepoConfigCmd.PersistentFlags().String("output-path", "", "atmos atlantis generate repo-config --output-path ./atlantis.yaml --config-template config-1 --project-template project-1 --workflow-template workflow-1")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("config-template", "", "atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --workflow-template workflow-1")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("project-template", "", "atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --workflow-template workflow-1")
	atlantisGenerateRepoConfigCmd.PersistentFlags().String("workflow-template", "", "atmos atlantis generate repo-config --config-template config-1 --project-template project-1 --workflow-template workflow-1")

	atlantisGenerateRepoConfigCmd.PersistentFlags().String("stacks", "",
		"Generate Atlantis projects for the specified stacks only (comma-separated values).\n"+
			"atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks <stack1>,<stack2>\n"+
			"The filter can contain the names of the top-level stack config files and the logical stack names (derived from the context vars)\n"+
			"atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --workflow-template <workflow_template> --stacks orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2\n"+
			"atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --workflow-template <workflow_template> --stacks tenant1-ue2-staging,tenant1-ue2-prod\n"+
			"atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --workflow-template <workflow_template> --stacks orgs/cp/tenant1/staging/us-east-2,tenant1-ue2-prod",
	)

	atlantisGenerateRepoConfigCmd.PersistentFlags().String("components", "",
		"Generate Atlantis projects for the specified components only (comma-separated values).\n"+
			"atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --workflow-template <workflow_template> --components <component1>,<component2>",
	)

	err := atlantisGenerateRepoConfigCmd.MarkPersistentFlagRequired("config-template")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	err = atlantisGenerateRepoConfigCmd.MarkPersistentFlagRequired("project-template")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	atlantisGenerateCmd.AddCommand(atlantisGenerateRepoConfigCmd)
}
