package helmfile

import "github.com/spf13/cobra"

// Command: atmos helmfile template.
var (
	helmfileTemplateShort = "Render Helm releases defined in a helmfile to Kubernetes manifests."
	helmfileTemplateLong  = `This command runs 'helmfile template' for the specified component and stack, rendering all
Helm releases to Kubernetes manifests using the generated values file. The rendered manifests
are written to stdout by default, or delivered to a provision target (e.g. a Git deployment
repository for GitOps/ArgoCD) when '--target' is set.

Example usage:
  atmos helmfile template echo-server --stack tenant1-ue2-dev
  atmos helmfile template echo-server --stack tenant1-ue2-dev --target deployment-repo`
)

// helmfileTemplateCmd represents the helmfile template subcommand.
var helmfileTemplateCmd = &cobra.Command{
	Use:                "template",
	Aliases:            []string{},
	Short:              helmfileTemplateShort,
	Long:               helmfileTemplateLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return helmfileRun(cmd, "template", args)
	},
}

func init() {
	helmfileCmd.AddCommand(helmfileTemplateCmd)
}
