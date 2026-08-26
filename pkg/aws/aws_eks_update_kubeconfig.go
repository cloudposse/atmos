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
// PUBLIC API — DO NOT REMOVE. This wrapper has no callers inside the Atmos repository, so
// dead-code sweeps (for example, `go run golang.org/x/tools/cmd/deadcode@latest -test ./...`)
// will report it as unused. It is retained intentionally because it is part of the public Go
// API consumed by the external `cloudposse/terraform-provider-utils` provider, which cannot
// import Atmos internal packages. It was previously dropped as "dead" code in PR #2608, which
// broke that provider; see docs/fixes/2026-08-25-restore-public-provider-api-wrappers.md.
//
// See https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html for details.
func ExecuteAwsEksUpdateKubeconfig(kubeconfigContext schema.AwsEksUpdateKubeconfigContext) error {
	return e.ExecuteAwsEksUpdateKubeconfig(kubeconfigContext)
}
