package output

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ComponentConfig holds validated configuration for terraform output execution.
type ComponentConfig struct {
	// Executable is the path to the terraform/opentofu executable.
	Executable string
	// Workspace is the terraform workspace name.
	Workspace string
	// ComponentPath is the path to the component directory.
	ComponentPath string
	// BackendType is the backend type (e.g., "s3", "gcs", "azurerm").
	BackendType string
	// Backend contains the backend configuration.
	Backend map[string]any
	// Providers contains provider overrides.
	Providers map[string]any
	// Env contains environment variables from the component.
	Env map[string]any
	// AutoGenerateBackend indicates whether to auto-generate backend.tf.json.
	AutoGenerateBackend bool
	// InitRunReconfigure indicates whether to run init with -reconfigure.
	InitRunReconfigure bool
}

// IsComponentProcessable determines if a component should be processed for terraform output.
// Returns (enabled, abstract) flags from the component's metadata and vars sections.
func IsComponentProcessable(sections map[string]any) (enabled bool, abstract bool) {
	defer perf.Track(nil, "output.IsComponentProcessable")()

	abstract = false
	enabled = true

	// Check metadata section for abstract flag.
	if metadataSection, ok := sections[cfg.MetadataSectionName]; ok {
		if metadata, ok := metadataSection.(map[string]any); ok {
			abstract = isComponentAbstract(metadata)
		}
	}

	// Check vars section for enabled flag.
	if varsSection, ok := sections[cfg.VarsSectionName]; ok {
		if vars, ok := varsSection.(map[string]any); ok {
			enabled = isComponentEnabled(vars)
		}
	}

	return enabled, abstract
}

// isComponentAbstract checks if the component metadata indicates it's abstract.
func isComponentAbstract(metadataSection map[string]any) bool {
	if metadataType, ok := metadataSection["type"].(string); ok {
		return metadataType == "abstract"
	}
	return false
}

// isComponentEnabled checks if the component vars section indicates it's enabled.
func isComponentEnabled(varsSection map[string]any) bool {
	if enabled, ok := varsSection["enabled"].(bool); ok {
		return enabled
	}
	return true // Default to enabled if not specified.
}

// extractRequiredFields extracts required fields from sections into config.
func extractRequiredFields(sections map[string]any, component, stack string, config *ComponentConfig) error {
	// Extract executable (required).
	executable, ok := sections[cfg.CommandSectionName].(string)
	if !ok {
		return errUtils.Build(errUtils.ErrMissingExecutable).
			WithExplanationf("Component '%s' in stack '%s'.", component, stack).
			Err()
	}
	config.Executable = executable

	// Extract workspace (required).
	workspace, ok := sections[cfg.WorkspaceSectionName].(string)
	if !ok {
		return errUtils.Build(errUtils.ErrMissingWorkspace).
			WithExplanationf("Component '%s' in stack '%s'.", component, stack).
			Err()
	}
	config.Workspace = workspace

	// Extract component_path (required).
	componentPath, err := extractComponentPath(sections, component, stack)
	if err != nil {
		return err
	}
	config.ComponentPath = componentPath

	return nil
}

// extractComponentPath extracts and validates component_path from sections.
func extractComponentPath(sections map[string]any, component, stack string) (string, error) {
	componentInfo, ok := sections["component_info"]
	if !ok {
		return "", errUtils.Build(errUtils.ErrMissingComponentInfo).
			WithExplanationf("Component '%s' in stack '%s'.", component, stack).
			Err()
	}

	componentInfoMap, ok := componentInfo.(map[string]any)
	if !ok {
		return "", errUtils.Build(errUtils.ErrInvalidComponentInfoS).
			WithExplanationf("Component '%s' in stack '%s'.", component, stack).
			Err()
	}

	componentPath, ok := componentInfoMap["component_path"].(string)
	if !ok {
		return "", errUtils.Build(errUtils.ErrMissingComponentPath).
			WithExplanationf("Component '%s' in stack '%s'.", component, stack).
			Err()
	}

	return componentPath, nil
}

// extractOptionalFields extracts optional fields from sections into config.
func extractOptionalFields(sections map[string]any, config *ComponentConfig) {
	if backendType, ok := sections[cfg.BackendTypeSectionName].(string); ok {
		config.BackendType = backendType
	}
	if backend, ok := sections[cfg.BackendSectionName].(map[string]any); ok {
		config.Backend = backend
	}
	if providers, ok := sections[cfg.ProvidersSectionName].(map[string]any); ok {
		config.Providers = providers
	}
	if env, ok := sections[cfg.EnvSectionName].(map[string]any); ok {
		config.Env = env
	}
}

// ExtractComponentConfig extracts and validates component configuration from sections.
// Returns an error with appropriate sentinel if required fields are missing.
func ExtractComponentConfig(sections map[string]any, component, stack string, autoGenerateBackend, initRunReconfigure bool) (*ComponentConfig, error) {
	defer perf.Track(nil, "output.ExtractComponentConfig")()

	config := &ComponentConfig{
		AutoGenerateBackend: autoGenerateBackend,
		InitRunReconfigure:  initRunReconfigure,
	}

	if err := extractRequiredFields(sections, component, stack, config); err != nil {
		return nil, err
	}

	extractOptionalFields(sections, config)

	return config, nil
}

// ValidateBackendConfig validates that backend configuration is complete for backend generation.
func ValidateBackendConfig(config *ComponentConfig, component, stack string) error {
	defer perf.Track(nil, "output.ValidateBackendConfig")()

	if config.BackendType == "" {
		return errUtils.Build(errUtils.ErrBackendFileGeneration).
			WithExplanationf("Component '%s' in stack '%s' has an invalid 'backend_type' section.", component, stack).
			Err()
	}

	if config.Backend == nil {
		return errUtils.Build(errUtils.ErrBackendFileGeneration).
			WithExplanationf("Component '%s' in stack '%s' has an invalid 'backend' section.", component, stack).
			Err()
	}

	return nil
}

// GetComponentInfo returns a formatted string for logging purposes.
func GetComponentInfo(component, stack string) string {
	defer perf.Track(nil, "output.GetComponentInfo")()

	return fmt.Sprintf("component '%s' in stack '%s'", component, stack)
}
