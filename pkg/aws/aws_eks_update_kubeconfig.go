package aws

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteAwsEksUpdateKubeconfig executes 'aws eks update-kubeconfig'
// https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html
func ExecuteAwsEksUpdateKubeconfig(kubeconfigContext schema.AwsEksUpdateKubeconfigContext) error {
	err := e.ExecuteAwsEksUpdateKubeconfig(kubeconfigContext)
	if err != nil {
		return err
	}

	return nil
}
