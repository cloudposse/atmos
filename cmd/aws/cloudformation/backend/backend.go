// Package backend implements the `atmos aws cloudformation backend` verb
// group: create/describe/update/delete/list for the S3 artifact bucket used
// by the aws/cloudformation component type's template-packaging flow (a
// `kind: aws/s3` provision target). It is a verb-for-verb clone of
// cmd/terraform/backend, targeting the same registered S3 backend type
// (pkg/provisioner/backend/s3.go's backendTypeS3) — no new backend type is
// introduced. The Terraform-shape adapter that bridges CFN's `kind: aws/s3`
// provision-target vocabulary to the Terraform-shaped
// `backend_type`/`backend` contract pkg/provisioner/backend expects lives in
// pkg/component/aws/cloudformation/backend.go.
package backend

import (
	"github.com/spf13/cobra"
)

// backendCmd is the `atmos aws cloudformation backend` command group. It is
// mounted under CloudFormationCmd (cmd/aws/cloudformation/cloudformation.go's
// init()), which already carries `Annotations: {"experimental": "true"}`; the
// root command's experimental-detection walks ancestors, so no redundant
// annotation is needed here (mirrors cmd/terraform/backend/backend.go, whose
// own experimental annotation is set explicitly only because its own parent,
// `terraform`, is not itself experimental).
var backendCmd = &cobra.Command{
	Use:   "backend",
	Short: "Manage the aws/cloudformation template-packaging backend",
	Long: `Create, list, describe, update, and delete the S3 bucket used by an
aws/cloudformation component's template-packaging flow (a "kind: aws/s3"
provision target).`,
}

func init() {
	backendCmd.AddCommand(createCmd)
	backendCmd.AddCommand(updateCmd)
	backendCmd.AddCommand(deleteCmd)
	backendCmd.AddCommand(describeCmd)
	backendCmd.AddCommand(listCmd)
}

// GetBackendCommand returns the backend command for parent registration.
func GetBackendCommand() *cobra.Command {
	return backendCmd
}
