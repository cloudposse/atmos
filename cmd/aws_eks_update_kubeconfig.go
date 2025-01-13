package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// awsEksCmdUpdateKubeconfigCmd executes 'aws eks update-kubeconfig' command
var awsEksCmdUpdateKubeconfigCmd = &cobra.Command{
	Use:   "update-kubeconfig",
	Short: "Update 'kubeconfig' for an EKS cluster using AWS CLI",
	Long: `This command executes 'aws eks update-kubeconfig' to download 'kubeconfig' from an EKS cluster and saves it to a file. The command executes 'aws eks update-kubeconfig' in three different ways:

1. If all the required parameters (cluster name and AWS profile/role) are provided on the command-line,
then 'atmos' executes the command without requiring the 'atmos.yaml' CLI config and context.
For example: atmos aws eks update-kubeconfig --profile=<profile> --name=<cluster_name>

2. If 'component' and 'stack' are provided on the command-line,
   then 'atmos' executes the command using the 'atmos.yaml' CLI config and stack's context by searching for the following settings:
  - 'components.helmfile.cluster_name_pattern' in the 'atmos.yaml' CLI config (and calculates the '--name' parameter using the pattern)
  - 'components.helmfile.helm_aws_profile_pattern' in the 'atmos.yaml' CLI config (and calculates the '--profile' parameter using the pattern)
  - 'components.helmfile.kubeconfig_path' in the 'atmos.yaml' CLI config
  - the variables for the component in the provided stack
  - 'region' from the variables for the component in the stack
For example: atmos aws eks update-kubeconfig <component> -s <stack>

3. Combination of the above. Provide a component and a stack, and override other parameters on the command line.
For example: atmos aws eks update-kubeconfig <component> -s <stack> --kubeconfig=<path_to_kubeconfig> --region=<region>

See https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html for more information.`,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		handleHelpRequest(cmd, args)
		err := e.ExecuteAwsEksUpdateKubeconfigCommand(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}
	},
}

// https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html
func init() {
	awsEksCmdUpdateKubeconfigCmd.DisableFlagParsing = false
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().StringP("stack", "s", "", "atmos aws eks update-kubeconfig <component> -s <stack>")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("profile", "", "atmos aws eks update-kubeconfig --profile <profile>")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("name", "", "atmos aws eks update-kubeconfig --name <cluster name>")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("region", "", "atmos aws eks update-kubeconfig --region <region>")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("kubeconfig", "", "atmos aws eks update-kubeconfig --kubeconfig <path_to_kubeconfig>")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("role-arn", "", "atmos aws eks update-kubeconfig --role-arn <ARN>")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().Bool("dry-run", false, "atmos aws eks update-kubeconfig --dry-run=true")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().Bool("verbose", false, "atmos aws eks update-kubeconfig --verbose=true")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("alias", "", "atmos aws eks update-kubeconfig --alias <alias for the cluster context name>")

	awsEksCmd.AddCommand(awsEksCmdUpdateKubeconfigCmd)
}
