package flags

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// extractComponent extracts the component name from positional arguments.
//
// This is a helper for Terraform, Packer, and Helmfile parsers that all extract
// a component name from positional args. The index parameter specifies which
// positional arg contains the component:
//   - Terraform: positionalArgs[0] = component (e.g., ["vpc"])
//   - Packer:    positionalArgs[0] = component (e.g., ["aws-bastion"])
//   - Helmfile:  positionalArgs[1] = component (e.g., ["sync", "nginx"])
//
// This centralizes the extraction logic and provides a clear contract for
// where components are expected in positional args.
//
// Parameters:
//   - positionalArgs: The full array of positional arguments after flag parsing
//   - componentIndex: Zero-based index of the component in positionalArgs
//
// Returns:
//   - component: The extracted component name, or empty string if not found
//
// Example:
//
//	// Terraform/Packer: component is first positional arg
//	component := extractComponent(positionalArgs, 0)
//
//	// Helmfile: component is second positional arg (after subcommand)
//	component := extractComponent(positionalArgs, 1)
func extractComponent(positionalArgs []string, componentIndex int) string {
	defer perf.Track(nil, "flags.extractComponent")()

	if len(positionalArgs) > componentIndex {
		return positionalArgs[componentIndex]
	}
	return ""
}
