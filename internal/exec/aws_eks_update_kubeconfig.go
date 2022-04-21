package exec

import (
	a "github.com/cloudposse/atmos/pkg/aws"
	"github.com/spf13/cobra"
)

func ExecuteAwsEksUpdateKubeconfigCommand(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	profile, err := flags.GetString("profile")
	if err != nil {
		return err
	}

	name, err := flags.GetString("name")
	if err != nil {
		return err
	}

	region, err := flags.GetString("region")
	if err != nil {
		return err
	}

	kubeconfig, err := flags.GetString("kubeconfig")
	if err != nil {
		return err
	}

	roleArn, err := flags.GetString("role-arn")
	if err != nil {
		return err
	}

	dryRun, err := flags.GetBool("dry-run")
	if err != nil {
		return err
	}

	verbose, err := flags.GetBool("verbose")
	if err != nil {
		return err
	}

	alias, err := flags.GetString("alias")
	if err != nil {
		return err
	}

	component := ""
	if len(args) > 0 {
		component = args[0]
	}

	executeAwsEksUpdateKubeconfigContext := a.ExecuteAwsEksUpdateKubeconfigContext{
		Component:   component,
		Stack:       stack,
		Profile:     profile,
		ClusterName: name,
		Region:      region,
		Kubeconfig:  kubeconfig,
		RoleArn:     roleArn,
		DryRun:      dryRun,
		Verbose:     verbose,
		Alias:       alias,
	}

	return a.ExecuteAwsEksUpdateKubeconfig(executeAwsEksUpdateKubeconfigContext)
}
