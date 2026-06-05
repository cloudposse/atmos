package ansible

import (
	"context"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AnsibleComponentProvider implements ComponentProvider for Ansible components.
// It wraps the existing Ansible execution logic in internal/exec/ansible.go
// and provides a registry-compatible interface for component operations.
type AnsibleComponentProvider struct{}

func init() {
	defer perf.Track(nil, "ansible.init")()

	// Self-register with the component registry.
	if err := component.Register(&AnsibleComponentProvider{}); err != nil {
		panic(fmt.Sprintf("failed to register ansible component provider: %v", err))
	}
}

// GetType returns the component type identifier.
func (a *AnsibleComponentProvider) GetType() string {
	defer perf.Track(nil, "ansible.GetType")()

	return "ansible"
}

// GetGroup returns the component group for categorization.
func (a *AnsibleComponentProvider) GetGroup() string {
	defer perf.Track(nil, "ansible.GetGroup")()

	return "Configuration Management"
}

// GetBasePath returns the base directory path for this component type.
func (a *AnsibleComponentProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
	defer perf.Track(atmosConfig, "ansible.GetBasePath")()

	if atmosConfig == nil {
		return DefaultConfig().BasePath
	}

	// Use the built-in Ansible configuration from schema.
	if atmosConfig.Components.Ansible.BasePath != "" {
		return atmosConfig.Components.Ansible.BasePath
	}

	return DefaultConfig().BasePath
}

// ListComponents discovers all ansible components in a stack.
func (a *AnsibleComponentProvider) ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error) {
	defer perf.Track(nil, "ansible.ListComponents")()

	componentsSection, ok := stackConfig["components"].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	ansibleComponents, ok := componentsSection["ansible"].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	componentNames := make([]string, 0, len(ansibleComponents))
	for name := range ansibleComponents {
		componentNames = append(componentNames, name)
	}

	sort.Strings(componentNames)
	return componentNames, nil
}

// ValidateComponent validates ansible component configuration.
func (a *AnsibleComponentProvider) ValidateComponent(config map[string]any) error {
	defer perf.Track(nil, "ansible.ValidateComponent")()

	if config == nil {
		return nil
	}

	// Validate metadata section if present.
	if metadata, ok := config["metadata"].(map[string]any); ok {
		// Check for abstract component.
		if componentType, ok := metadata["type"].(string); ok && componentType == "abstract" {
			// Abstract components are valid but cannot be executed.
			return nil
		}
	}

	// Validate settings.ansible section if present.
	if settings, ok := config["settings"].(map[string]any); ok {
		if ansible, ok := settings["ansible"].(map[string]any); ok {
			// Validate playbook if specified.
			if playbook, ok := ansible["playbook"]; ok {
				if _, isString := playbook.(string); !isString && playbook != nil {
					return fmt.Errorf("%w: settings.ansible.playbook must be a string", errUtils.ErrComponentValidationFailed)
				}
			}

			// Validate inventory if specified.
			if inventory, ok := ansible["inventory"]; ok {
				if _, isString := inventory.(string); !isString && inventory != nil {
					return fmt.Errorf("%w: settings.ansible.inventory must be a string", errUtils.ErrComponentValidationFailed)
				}
			}
		}
	}

	return nil
}

// Execute runs a command for ansible components.
// Delegates to the appropriate executor function based on the subcommand.
func (a *AnsibleComponentProvider) Execute(ctx *component.ExecutionContext) error {
	defer perf.Track(ctx.AtmosConfig, "ansible.Execute")()

	// Extract ansible-specific flags from the context.
	flags := &Flags{}
	if ctx.Flags != nil {
		if playbook, ok := ctx.Flags["playbook"].(string); ok {
			flags.Playbook = playbook
		}
		if inventory, ok := ctx.Flags["inventory"].(string); ok {
			flags.Inventory = inventory
		}
	}

	// Route to the appropriate executor based on subcommand.
	switch ctx.SubCommand {
	case "version":
		return ExecuteVersion(&ctx.ConfigAndStacksInfo)
	case "playbook":
		return ExecutePlaybook(&ctx.ConfigAndStacksInfo, flags)
	default:
		// For unknown subcommands, default to playbook behavior.
		return ExecutePlaybook(&ctx.ConfigAndStacksInfo, flags)
	}
}

// GenerateArtifacts creates necessary files for ansible component execution.
// For Ansible, this includes variable files passed via --extra-vars.
func (a *AnsibleComponentProvider) GenerateArtifacts(ctx *component.ExecutionContext) error {
	defer perf.Track(ctx.AtmosConfig, "ansible.GenerateArtifacts")()

	// Artifact generation is handled within ExecuteAnsible.
	// The varfile is generated and cleaned up automatically during playbook execution.
	// This method exists to satisfy the ComponentProvider interface.
	return nil
}

// GetAvailableCommands returns list of commands this component type supports.
func (a *AnsibleComponentProvider) GetAvailableCommands() []string {
	defer perf.Track(nil, "ansible.GetAvailableCommands")()

	return []string{"playbook", "version"}
}
