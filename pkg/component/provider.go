package component

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
)

// ComponentProvider is the interface that component type implementations
// must satisfy to register with the Atmos component registry.
//
// Component providers are registered via init() functions and enable
// extensible component type support without modifying core code.
type ComponentProvider interface {
	// GetType returns the component type identifier (e.g., "terraform", "helmfile", "mock").
	GetType() string

	// GetGroup returns the component group for categorization.
	// Examples: "Infrastructure as Code", "Kubernetes", "Images", "Testing"
	GetGroup() string

	// GetBasePath returns the base directory path for this component type.
	// Example: For terraform, returns "components/terraform"
	GetBasePath(atmosConfig *schema.AtmosConfiguration) string

	// ListComponents discovers all components of this type in a stack.
	// Returns component names found in the stack configuration.
	ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error)

	// ValidateComponent validates component configuration.
	// Returns error if configuration is invalid for this component type.
	ValidateComponent(config map[string]any) error

	// Execute runs a command for this component type.
	// Context provides all necessary information for execution.
	Execute(ctx ExecutionContext) error

	// GenerateArtifacts creates necessary files for component execution.
	// Examples: backend.tf for Terraform, varfile for Helmfile
	GenerateArtifacts(ctx ExecutionContext) error

	// GetAvailableCommands returns list of commands this component type supports.
	// Example: For terraform: ["plan", "apply", "destroy", "workspace"]
	GetAvailableCommands() []string
}

// ExecutionContext provides all necessary context for component execution.
type ExecutionContext struct {
	AtmosConfig         *schema.AtmosConfiguration
	ComponentType       string
	Component           string
	Stack               string
	Command             string
	SubCommand          string
	ComponentConfig     map[string]any
	ConfigAndStacksInfo schema.ConfigAndStacksInfo
	Args                []string
	Flags               map[string]any
}

// ComponentInfo provides metadata about a component provider.
type ComponentInfo struct {
	Type        string
	Group       string
	Commands    []string
	Description string
}
