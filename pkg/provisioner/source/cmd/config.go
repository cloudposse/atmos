// Package cmd provides reusable CLI command builders for source provisioning.
// These builders create Cobra commands parameterized by component type,
// enabling terraform, helmfile, and packer to share the same implementation.
package cmd

// Config holds component-type-specific configuration for source commands.
type Config struct {
	// ComponentType identifies the component type (e.g., "terraform", "helmfile", "packer",
	// "aws/cloudformation") for internal dispatch (stack-config lookups, perf.Track labels).
	ComponentType string
	// TypeLabel is the display name for the component type (e.g., "Terraform", "Helmfile", "Packer").
	TypeLabel string
	// CLIName is the command path used in generated Example/Long text (e.g. "terraform",
	// "aws cloudformation"). Falls back to ComponentType via CLI() when unset — the common
	// case for types whose CLI command name matches their internal ComponentType exactly.
	CLIName string
}

// CLI returns the command path to use in generated help/example text, falling
// back to ComponentType for types whose CLI invocation matches it exactly.
func (c *Config) CLI() string {
	if c.CLIName != "" {
		return c.CLIName
	}
	return c.ComponentType
}
