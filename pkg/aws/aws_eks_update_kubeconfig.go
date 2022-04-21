package exec

import (
	"fmt"
	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/pkg/errors"
)

type ExecuteAwsEksUpdateKubeconfigContext struct {
	Component   string
	Stack       string
	Profile     string
	ClusterName string
	Kubeconfig  string
	RoleArn     string
	DryRun      bool
	Verbose     bool
	Alias       string
	Tenant      string
	Environment string
	Stage       string
	Region      string
}

// ExecuteAwsEksUpdateKubeconfig executes 'aws eks update-kubeconfig'
// https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html
func ExecuteAwsEksUpdateKubeconfig(kubeconfigContext ExecuteAwsEksUpdateKubeconfigContext) error {
	// AWS profile to authenticate to the cluster
	profile := kubeconfigContext.Profile

	// To assume a role for cluster authentication, specify an IAM role ARN with this option. For example, if you created a cluster while
	// assuming an IAM role, then you must also assume that role to connect to the cluster the first time
	roleArn := kubeconfigContext.RoleArn

	// AWS region
	region := kubeconfigContext.Region

	// Print the merged kubeconfig to stdout instead of writing it to the specified file
	dryRun := kubeconfigContext.DryRun

	// Print more detailed output when writing to the kubeconfig file, including the appended entries
	verbose := kubeconfigContext.Verbose

	// The name of the cluster for which to create a kubeconfig entry. This cluster must exist in your account and in
	// the specified or configured default Region for your AWS CLI installation
	clusterName := kubeconfigContext.ClusterName

	// Optionally specify a kubeconfig file to append with your configuration. By default, the configuration is written to the first file path
	// in the KUBECONFIG environment variable (if it is set) or the default kubeconfig path (.kube/config) in your home directory
	kubeconfigPath := kubeconfigContext.Kubeconfig

	// Alias for the cluster context name. Defaults to match cluster ARN
	alias := kubeconfigContext.Alias

	// Check if all the required parameters are provided to execute the command without needing `atmos.yaml` config and context
	// The rest of the parameters are optional
	requiredParamsProvided := clusterName != "" && (profile != "" || roleArn != "")

	if !requiredParamsProvided {
		// If stack is not provided, calculate the stack name from the context (tenent, environment, stage)
		if kubeconfigContext.Stack == "" {
			err := c.InitConfig()
			if err != nil {
				return err
			}

			if len(c.Config.Stacks.NamePattern) < 1 {
				return errors.New("stack name pattern must be provided in 'stacks.name_pattern' CLI config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
			}

			stack, err := c.GetStackNameFromContextAndStackNamePattern(kubeconfigContext.Tenant,
				kubeconfigContext.Environment, kubeconfigContext.Stage, c.Config.Stacks.NamePattern)
			if err != nil {
				return err
			}

			kubeconfigContext.Stack = stack
		}

		var configAndStacksInfo c.ConfigAndStacksInfo
		configAndStacksInfo.ComponentFromArg = kubeconfigContext.Component
		configAndStacksInfo.Stack = kubeconfigContext.Stack

		configAndStacksInfo.ComponentType = "terraform"
		configAndStacksInfo, err := e.ProcessStacks(configAndStacksInfo, true)
		if err != nil {
			configAndStacksInfo.ComponentType = "helmfile"
			configAndStacksInfo, err = e.ProcessStacks(configAndStacksInfo, true)
			if err != nil {
				return err
			}
		}

		context := c.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)

		if kubeconfigPath == "" {
			kubeconfigPath = fmt.Sprintf("%s/%s-kubecfg", kubeconfigContext.Kubeconfig, kubeconfigContext.Stack)
		}
		if clusterName == "" {
			clusterName = c.ReplaceContextTokens(context, c.Config.Components.Helmfile.ClusterNamePattern)
		}
		if profile == "" {
			profile = c.ReplaceContextTokens(context, c.Config.Components.Helmfile.HelmAwsProfilePattern)
		}
		if region == "" {
			region = context.Region
		}
	}

	args := []string{
		"eks",
		"update-kubeconfig",
		fmt.Sprintf("--name=%s", clusterName),
		fmt.Sprintf("--dry-run=%t", dryRun),
		fmt.Sprintf("--verbose=%t", verbose),
	}

	if profile != "" {
		args = append(args, []string{"--profile", profile}...)
	}
	if roleArn != "" {
		args = append(args, []string{"--role-arn", roleArn}...)
	}
	if kubeconfigPath != "" {
		args = append(args, []string{"--kubeconfig", kubeconfigPath}...)
	}
	if alias != "" {
		args = append(args, []string{"--alias", alias}...)
	}
	if region != "" {
		args = append(args, []string{"--region", region}...)
	}

	err := e.ExecuteShellCommand("aws", args, kubeconfigPath, nil, false)
	if err != nil {
		return err
	}

	return nil
}
