package output

import (
	"fmt"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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
	// AutoProvisionWorkdirForOutputs controls whether the executor auto-provisions
	// JIT working directories before terraform init.
	AutoProvisionWorkdirForOutputs bool
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
func extractRequiredFields(atmosConfig *schema.AtmosConfiguration, sections map[string]any, component, stack string, config *ComponentConfig) error {
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
	componentPath, err := extractComponentPath(atmosConfig, sections, component, stack)
	if err != nil {
		return err
	}
	config.ComponentPath = componentPath

	return nil
}

// extractComponentPath extracts and resolves the absolute component path from sections.
// It uses utils.GetComponentPath to ensure consistent path resolution across the codebase.
func extractComponentPath(atmosConfig *schema.AtmosConfiguration, sections map[string]any, component, stack string) (string, error) {
	// Validate component_info exists.
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

	// Get component type (terraform, helmfile, etc.).
	componentType, ok := componentInfoMap["component_type"].(string)
	if !ok {
		componentType = "terraform" // Default to terraform for backward compatibility.
	}

	// Get the base component name (the actual component, not the stack component name).
	// This handles derived components correctly.
	baseComponent := ""
	if comp, ok := sections[cfg.ComponentSectionName].(string); ok && comp != "" {
		baseComponent = comp
	}
	if baseComponent == "" {
		return "", errUtils.Build(errUtils.ErrMissingComponentPath).
			WithExplanationf("Component '%s' in stack '%s' has no base component defined.", component, stack).
			Err()
	}

	// Get component folder prefix if it exists in metadata.
	componentFolderPrefix := ""
	if metadata, ok := sections[cfg.MetadataSectionName].(map[string]any); ok {
		if prefix, ok := metadata["component_folder_prefix"].(string); ok {
			componentFolderPrefix = prefix
		}
	}

	// Use utils.GetComponentPath for consistent path resolution.
	// This ensures proper handling of BasePath, environment variables, and absolute paths.
	componentPath, err := u.GetComponentPath(atmosConfig, componentType, componentFolderPrefix, baseComponent)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrMissingComponentPath).
			WithCause(err).
			WithExplanationf("Component '%s' in stack '%s'.", component, stack).
			Err()
	}

	// When workdir provisioning is active, all terraform operations must target the
	// workdir, not the base component directory. The workdir path is deterministic
	// (built from basePath + componentType + stack + instance name) so we can
	// reconstruct it here even when _workdir_path is absent from freshly-described
	// sections (e.g. during hook execution where DescribeComponent is called fresh).
	if provWorkdir.IsWorkdirEnabled(sections) {
		basePath := atmosConfig.BasePath
		if basePath == "" {
			basePath = "."
		}
		workdirPath := provWorkdir.BuildPath(basePath, componentType, component, stack, sections)
		if !filepath.IsAbs(workdirPath) {
			if abs, absErr := filepath.Abs(workdirPath); absErr == nil {
				workdirPath = abs
			}
		}
		// Containment guard: reject derived paths that escape the project directory.
		// atmos_component and atmos_stack come from user-controlled YAML; a value
		// containing ../ sequences (e.g. "../../../../etc/evil") could otherwise
		// escape BasePath via filepath.Join resolution inside BuildPath.
		// Note: symlinks are not resolved — same best-effort scope as the mirror
		// guard in terraform_backend_local.go:resolveLocalBackendComponentPath.
		// Uses the already-resolved basePath local (not atmosConfig.BasePath which
		// may be "") to avoid Abs("") vs Abs(".") inconsistency.
		absBase, errBase := filepath.Abs(basePath)
		if errBase == nil {
			sep := string(filepath.Separator)
			if strings.HasPrefix(workdirPath, absBase+sep) || workdirPath == absBase {
				return workdirPath, nil
			}
			log.Debug("Derived workdir path escapes project directory; using component path",
				"derived_path", workdirPath, "base_path", basePath)
		} else {
			return workdirPath, nil
		}
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
// The autoGenerateBackend and initRunReconfigure flags are read directly from atmosConfig.
func ExtractComponentConfig(atmosConfig *schema.AtmosConfiguration, sections map[string]any, component, stack string) (*ComponentConfig, error) {
	defer perf.Track(atmosConfig, "output.ExtractComponentConfig")()

	config := &ComponentConfig{
		AutoGenerateBackend:            atmosConfig.Components.Terraform.AutoGenerateBackendFile,
		InitRunReconfigure:             atmosConfig.Components.Terraform.InitRunReconfigure,
		AutoProvisionWorkdirForOutputs: atmosConfig.Components.Terraform.AutoProvisionWorkdirForOutputs,
	}

	if err := extractRequiredFields(atmosConfig, sections, component, stack, config); err != nil {
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
