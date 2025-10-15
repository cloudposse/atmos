package exec

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ComponentProcessorOptions contains configuration for processing a component.
type ComponentProcessorOptions struct {
	ComponentType            string
	Component                string
	Stack                    string
	StackName                string
	ComponentMap             map[string]any
	AllComponentsMap         map[string]any
	ComponentsBasePath       string
	CheckBaseComponentExists bool

	// Global configurations.
	GlobalVars         map[string]any
	GlobalSettings     map[string]any
	GlobalEnv          map[string]any
	GlobalAuth         map[string]any
	GlobalCommand      string
	AtmosGlobalAuthMap map[string]any // Pre-converted atmosConfig.Auth to prevent race conditions

	// Terraform-specific options.
	TerraformProviders              map[string]any
	GlobalAndTerraformHooks         map[string]any
	GlobalBackendType               string
	GlobalBackendSection            map[string]any
	GlobalRemoteStateBackendType    string
	GlobalRemoteStateBackendSection map[string]any

	// Atmos configuration.
	AtmosConfig *schema.AtmosConfiguration
}

// ComponentProcessorResult contains the processed component data.
type ComponentProcessorResult struct {
	ComponentVars              map[string]any
	ComponentSettings          map[string]any
	ComponentEnv               map[string]any
	ComponentMetadata          map[string]any
	ComponentCommand           string
	ComponentOverrides         map[string]any
	ComponentOverridesVars     map[string]any
	ComponentOverridesSettings map[string]any
	ComponentOverridesEnv      map[string]any
	ComponentOverridesAuth     map[string]any
	ComponentOverridesCommand  string
	BaseComponentName          string
	BaseComponentVars          map[string]any
	BaseComponentSettings      map[string]any
	BaseComponentEnv           map[string]any
	BaseComponentAuth          map[string]any
	BaseComponentCommand       string
	ComponentInheritanceChain  []string
	BaseComponents             []string

	// Terraform-specific fields.
	ComponentProviders                     map[string]any
	ComponentHooks                         map[string]any
	ComponentAuth                          map[string]any
	ComponentBackendType                   string
	ComponentBackendSection                map[string]any
	ComponentRemoteStateBackendType        string
	ComponentRemoteStateBackendSection     map[string]any
	ComponentOverridesProviders            map[string]any
	ComponentOverridesHooks                map[string]any
	BaseComponentProviders                 map[string]any
	BaseComponentHooks                     map[string]any
	BaseComponentBackendType               string
	BaseComponentBackendSection            map[string]any
	BaseComponentRemoteStateBackendType    string
	BaseComponentRemoteStateBackendSection map[string]any
}

// processComponent processes a component extracting common configuration sections.
func processComponent(opts *ComponentProcessorOptions) (*ComponentProcessorResult, error) {
	defer perf.Track(opts.AtmosConfig, "exec.processComponent")()

	result := &ComponentProcessorResult{
		ComponentVars:     make(map[string]any),
		ComponentSettings: make(map[string]any),
		ComponentEnv:      make(map[string]any),
		ComponentMetadata: make(map[string]any),
		BaseComponents:    []string{},
	}

	// Extract component sections.
	if err := extractComponentSections(opts, result); err != nil {
		return nil, err
	}

	// Process overrides.
	if err := processComponentOverrides(opts, result); err != nil {
		return nil, err
	}

	// Process component inheritance.
	if err := processComponentInheritance(opts, result); err != nil {
		return nil, err
	}

	return result, nil
}
