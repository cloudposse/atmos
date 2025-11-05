package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
)

func newAwsEksUpdateKubeconfigParser() *flags.StandardParser {
	// Build parser with AWS EKS-specific flags.
	options := []flags.Option{
		flags.WithStringFlag("profile", "", "", "Specify the AWS CLI profile to use for authentication"),
		flags.WithEnvVars("profile", "AWS_PROFILE"),

		flags.WithStringFlag("name", "", "", "Specify the name of the EKS cluster to update the kubeconfig for"),
		flags.WithEnvVars("name", "ATMOS_EKS_CLUSTER_NAME"),

		flags.WithStringFlag("region", "", "", "Specify the AWS region where the EKS cluster is located"),
		flags.WithEnvVars("region", "AWS_REGION", "AWS_DEFAULT_REGION"),

		flags.WithStringFlag("kubeconfig", "", "", "Specify the path to the kubeconfig file to be updated or created for accessing the EKS cluster."),
		flags.WithEnvVars("kubeconfig", "KUBECONFIG"),

		flags.WithStringFlag("role-arn", "", "", "Specify the ARN of the IAM role to assume for authenticating with the EKS cluster."),
		flags.WithEnvVars("role-arn", "ATMOS_EKS_ROLE_ARN"),

		flags.WithBoolFlag("dry-run", "", false, "Perform a dry run to simulate updating the kubeconfig without making any changes."),
		flags.WithEnvVars("dry-run", "ATMOS_DRY_RUN"),

		flags.WithBoolFlag("verbose", "", false, "Enable verbose logging to provide detailed output during the kubeconfig update process."),
		flags.WithEnvVars("verbose", "ATMOS_VERBOSE"),

		flags.WithStringFlag("alias", "", "", "Specify an alias to use for the cluster context name in the kubeconfig file."),
		flags.WithEnvVars("alias", "ATMOS_EKS_ALIAS"),
	}

	return flags.NewStandardParser(options...)
}

var awsEksUpdateKubeconfigParser = newAwsEksUpdateKubeconfigParser()

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

	ValidArgsFunction: ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := e.ExecuteAwsEksUpdateKubeconfigCommand(cmd, args)
		return err
	},
}

// https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html.
func init() {
	// Register flags using builder pattern.
	awsEksUpdateKubeconfigParser.RegisterFlags(awsEksCmdUpdateKubeconfigCmd)
	_ = awsEksUpdateKubeconfigParser.BindToViper(viper.GetViper())

	AddStackCompletion(awsEksCmdUpdateKubeconfigCmd)

	awsEksCmd.AddCommand(awsEksCmdUpdateKubeconfigCmd)
}
