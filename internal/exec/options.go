package exec

import tf "github.com/cloudposse/atmos/pkg/terraform"

// Re-export terraform options types from pkg/terraform for backwards compatibility.
// New code should import from pkg/terraform directly.
type (
	// ProcessingOptions holds common options for template and function processing.
	ProcessingOptions = tf.ProcessingOptions
	// CleanOptions holds options for the terraform clean command.
	CleanOptions = tf.CleanOptions
	// GenerateBackendOptions holds options for generating Terraform backend configs.
	GenerateBackendOptions = tf.GenerateBackendOptions
	// VarfileOptions holds options for generating Terraform varfiles.
	VarfileOptions = tf.VarfileOptions
	// ShellOptions holds options for the terraform shell command.
	ShellOptions = tf.ShellOptions
)
