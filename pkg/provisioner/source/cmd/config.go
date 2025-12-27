// Package cmd provides reusable CLI command builders for source provisioning.
// These builders create Cobra commands parameterized by component type,
// enabling terraform, helmfile, and packer to share the same implementation.
package cmd

// Config holds component-type-specific configuration for source commands.
type Config struct {
	ComponentType string // "terraform", "helmfile", "packer"
	TypeLabel     string // "Terraform", "Helmfile", "Packer" (for display)
}
