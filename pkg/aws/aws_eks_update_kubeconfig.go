package aws

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteAwsEksUpdateKubeconfig is a public wrapper around the internal implementation
// that executes 'aws eks update-kubeconfig'.
//
// It exists so that external consumers of the Atmos Go library (for example, the
// `cloudposse/terraform-provider-utils` Terraform provider and its
// `utils_aws_eks_update_kubeconfig` data source) can build an EKS kubeconfig from an
// Atmos component/stack context without depending on Atmos internal packages.
//
// https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html
func ExecuteAwsEksUpdateKubeconfig(kubeconfigContext schema.AwsEksUpdateKubeconfigContext) error {
	return e.ExecuteAwsEksUpdateKubeconfig(kubeconfigContext)
}
