package eks

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// updateKubeconfigParser handles flag parsing with Viper precedence.
var updateKubeconfigParser *flags.StandardParser

// updateKubeconfigCmd executes 'aws eks update-kubeconfig' command.
var updateKubeconfigCmd = &cobra.Command{
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
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "eks.updateKubeconfig.RunE")()

		// Bind flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := updateKubeconfigParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		return e.ExecuteAwsEksUpdateKubeconfigCommand(cmd, args)
	},
}

// https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html.
func init() {
	// Create parser with update-kubeconfig-specific flags using functional options.
	updateKubeconfigParser = flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Specify the stack name"),
		flags.WithStringFlag("profile", "", "", "Specify the AWS CLI profile to use for authentication"),
		flags.WithStringFlag("name", "", "", "Specify the name of the EKS cluster to update the kubeconfig for"),
		flags.WithStringFlag("region", "", "", "Specify the AWS region where the EKS cluster is located"),
		flags.WithStringFlag("kubeconfig", "", "", "Specify the path to the kubeconfig file to be updated or created for accessing the EKS cluster."),
		flags.WithStringFlag("role-arn", "", "", "Specify the ARN of the IAM role to assume for authenticating with the EKS cluster."),
		flags.WithBoolFlag("dry-run", "", false, "Perform a dry run to simulate updating the kubeconfig without making any changes."),
		flags.WithBoolFlag("verbose", "", false, "Enable verbose logging to provide detailed output during the kubeconfig update process."),
		flags.WithStringFlag("alias", "", "", "Specify an alias to use for the cluster context name in the kubeconfig file."),
		// Environment variable bindings.
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("profile", "ATMOS_AWS_PROFILE", "AWS_PROFILE"),
		flags.WithEnvVars("region", "ATMOS_AWS_REGION", "AWS_REGION"),
		flags.WithEnvVars("kubeconfig", "ATMOS_KUBECONFIG", "KUBECONFIG"),
	)

	// Register flags with Cobra command.
	updateKubeconfigParser.RegisterFlags(updateKubeconfigCmd)

	// Bind to Viper for environment variable support.
	if err := updateKubeconfigParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	EksCmd.AddCommand(updateKubeconfigCmd)
}
