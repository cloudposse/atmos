package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// awsEksCmdUpdateKubeconfigCmd executes 'aws eks update-kubeconfig' command.
var awsEksCmdUpdateKubeconfigCmd = &cobra.Command{
	Use:   "update-kubeconfig",
	Short: "Update `kubeconfig` for an EKS cluster using AWS CLI",
	Long: `This command executes ` + "`" + `aws eks update-kubeconfig` + "`" + ` to download ` + "`" + `kubeconfig` + "`" + ` from an EKS cluster and saves it to a file. The command executes ` + "`" + `aws eks update-kubeconfig` + "`" + ` in three different ways:

1. If all the required parameters (cluster name and AWS profile/role) are provided on the command-line,
then ` + "`" + `atmos` + "`" + ` executes the command without requiring the ` + "`" + `atmos.yaml` + "`" + ` CLI config and context.

2. If 'component' and 'stack' are provided on the command-line,
   then ` + "`" + `atmos` + "`" + ` executes the command using the ` + "`" + `atmos.yaml` + "`" + ` CLI config and stack's context by searching for the following settings:
  - 'components.helmfile.cluster_name_pattern' in the 'atmos.yaml' CLI config (and calculates the '--name' parameter using the pattern)
  - 'components.helmfile.helm_aws_profile_pattern' in the 'atmos.yaml' CLI config (and calculates the ` + "`" + `--profile` + "`" + ` parameter using the pattern)
  - 'components.helmfile.kubeconfig_path' in the 'atmos.yaml' CLI config
  - the variables for the component in the provided stack
  - 'region' from the variables for the component in the stack

3. Combination of the above. Provide a component and a stack, and override other parameters on the command line.

See https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html for more information.`,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := e.ExecuteAwsEksUpdateKubeconfigCommand(cmd, args)
		return err
	},
}

// https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html.
func init() {
	awsEksCmdUpdateKubeconfigCmd.DisableFlagParsing = false
	AddStackCompletion(awsEksCmdUpdateKubeconfigCmd)
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("profile", "", "Specify the AWS CLI profile to use for authentication")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("name", "", "Specify the name of the EKS cluster to update the kubeconfig for")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("region", "", "Specify the AWS region where the EKS cluster is located")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("kubeconfig", "", "Specify the path to the kubeconfig file to be updated or created for accessing the EKS cluster.")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("role-arn", "", "Specify the ARN of the IAM role to assume for authenticating with the EKS cluster.")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().Bool("dry-run", false, "Perform a dry run to simulate updating the kubeconfig without making any changes.")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().Bool("verbose", false, "Enable verbose logging to provide detailed output during the kubeconfig update process.")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("alias", "", "Specify an alias to use for the cluster context name in the kubeconfig file.")

	awsEksCmd.AddCommand(awsEksCmdUpdateKubeconfigCmd)
}
