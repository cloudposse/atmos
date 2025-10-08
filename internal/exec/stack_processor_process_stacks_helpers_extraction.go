package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// extractComponentSections extracts all component sections (vars, settings, env, etc.).
//
//nolint:gocognit,nestif,revive,cyclop,funlen // Extracts multiple configuration sections with type-specific handling.
func extractComponentSections(opts *ComponentProcessorOptions, result *ComponentProcessorResult) error {
	defer perf.Track(opts.AtmosConfig, "exec.extractComponentSections")()

	// Extract vars section.
	if i, ok := opts.ComponentMap[cfg.VarsSectionName]; ok {
		componentVars, ok := i.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.vars' in the file '%s'", errUtils.ErrInvalidComponentVars, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentVars = componentVars
	}

	// Extract settings section.
	if i, ok := opts.ComponentMap[cfg.SettingsSectionName]; ok {
		componentSettings, ok := i.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.settings' in the file '%s'", errUtils.ErrInvalidComponentSettings, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentSettings = componentSettings

		// Terraform-specific: validate spacelift settings.
		if opts.ComponentType == cfg.TerraformComponentType {
			if i, ok := componentSettings["spacelift"]; ok {
				_, ok = i.(map[string]any)
				if !ok {
					return fmt.Errorf("%w: 'components.%s.%s.settings.spacelift' in the file '%s'", errUtils.ErrInvalidSpaceLiftSettings, opts.ComponentType, opts.Component, opts.StackName)
				}
			}
		}
	}

	// Extract env section.
	if i, ok := opts.ComponentMap[cfg.EnvSectionName]; ok {
		componentEnv, ok := i.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.env' in the file '%s'", errUtils.ErrInvalidComponentEnv, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentEnv = componentEnv
	}

	// Terraform-specific: extract providers section.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := opts.ComponentMap[cfg.ProvidersSectionName]; ok {
			componentProviders, ok := i.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: 'components.%s.%s.providers' in the file '%s'", errUtils.ErrInvalidComponentProviders, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentProviders = componentProviders
		} else {
			result.ComponentProviders = make(map[string]any)
		}
	}

	// Terraform-specific: extract hooks section.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := opts.ComponentMap[cfg.HooksSectionName]; ok {
			componentHooks, ok := i.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: 'components.%s.%s.hooks' in the file '%s'", errUtils.ErrInvalidComponentHooks, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentHooks = componentHooks
		} else {
			result.ComponentHooks = make(map[string]any)
		}
	}

	// Extract auth section.
	if i, ok := opts.ComponentMap[cfg.AuthSectionName]; ok {
		componentAuth, ok := i.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.auth' in the file '%s'", errUtils.ErrInvalidComponentAuth, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentAuth = componentAuth
	} else {
		result.ComponentAuth = make(map[string]any)
	}

	// Extract metadata section.
	if i, ok := opts.ComponentMap[cfg.MetadataSectionName]; ok {
		componentMetadata, ok := i.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.metadata' in the file '%s'", errUtils.ErrInvalidComponentMetadata, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentMetadata = componentMetadata
	}

	// Terraform-specific: extract backend configuration.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := opts.ComponentMap[cfg.BackendTypeSectionName]; ok {
			componentBackendType, ok := i.(string)
			if !ok {
				return fmt.Errorf("%w: 'components.%s.%s.backend_type' in the file '%s'", errUtils.ErrInvalidComponentBackendType, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentBackendType = componentBackendType
		}

		if i, ok := opts.ComponentMap[cfg.BackendSectionName]; ok {
			componentBackendSection, ok := i.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: 'components.%s.%s.backend' in the file '%s'", errUtils.ErrInvalidComponentBackend, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentBackendSection = componentBackendSection
		} else {
			result.ComponentBackendSection = make(map[string]any)
		}
	}

	// Terraform-specific: extract remote state backend configuration.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := opts.ComponentMap[cfg.RemoteStateBackendTypeSectionName]; ok {
			componentRemoteStateBackendType, ok := i.(string)
			if !ok {
				return fmt.Errorf("%w: 'components.%s.%s.remote_state_backend_type' in the file '%s'", errUtils.ErrInvalidComponentRemoteStateBackendType, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentRemoteStateBackendType = componentRemoteStateBackendType
		}

		if i, ok := opts.ComponentMap[cfg.RemoteStateBackendSectionName]; ok {
			componentRemoteStateBackendSection, ok := i.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: 'components.%s.%s.remote_state_backend' in the file '%s'", errUtils.ErrInvalidComponentRemoteStateBackend, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentRemoteStateBackendSection = componentRemoteStateBackendSection
		} else {
			result.ComponentRemoteStateBackendSection = make(map[string]any)
		}
	}

	// Extract the executable command.
	if i, ok := opts.ComponentMap[cfg.CommandSectionName]; ok {
		componentCommand, ok := i.(string)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.command' in the file '%s'", errUtils.ErrInvalidComponentCommand, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentCommand = componentCommand
	}

	return nil
}
