package aws

import (
	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteAwsEksUpdateKubeconfig executes 'aws eks update-kubeconfig'
// https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html
func ExecuteAwsEksUpdateKubeconfig(kubeconfigContext c.AwsEksUpdateKubeconfigContext) error {
	err := e.ExecuteAwsEksUpdateKubeconfig(kubeconfigContext)

	if err != nil {
		u.PrintErrorToStdError(err)
		return err
	}

	return nil
}
