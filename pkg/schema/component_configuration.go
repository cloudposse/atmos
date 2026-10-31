package schema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrComponentConfigurationCollision prevents dynamic settings from overriding built-in fields.
// Declared here because the host errors package depends on schema through perf.
var ErrComponentConfigurationCollision = errors.New("component configuration key collision")

// ComponentConfiguration enables one separately installed configuration provider.
// The section name is the provider name unless provider overrides it.
type ComponentConfiguration struct {
	Enabled   bool                     `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
	Provider  string                   `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
	Mode      string                   `yaml:"mode,omitempty" json:"mode,omitempty" mapstructure:"mode"`
	Discovery ComponentDiscoveryConfig `yaml:"discovery" json:"discovery" mapstructure:"discovery"`
	Options   map[string]any           `yaml:",inline" json:"-" mapstructure:",remain"`
}

// ComponentDiscoveryConfig bounds automatic discovery. Both switches default off.
type ComponentDiscoveryConfig struct {
	Enabled       bool     `yaml:"enabled" json:"enabled" mapstructure:"enabled"`
	IncludedPaths []string `yaml:"included_paths,omitempty" json:"included_paths,omitempty" mapstructure:"included_paths"`
	ExcludedPaths []string `yaml:"excluded_paths,omitempty" json:"excluded_paths,omitempty" mapstructure:"excluded_paths"`
}

// ComponentConfigurationRuntime is an invocation-owned host service. It is not a wire contract.
type ComponentConfigurationRuntime interface {
	Discover(context.Context) (map[string]map[string]any, error)
	Prepare(context.Context, *ConfigAndStacksInfo) (*PreparedComponentConfiguration, error)
}

// ComponentConfigurationFailureRuntime reports components that a provider discovered but could not
// evaluate. Bulk commands use it to reject every selected broken component before they run any
// infrastructure command. A nil error means the component is healthy or unknown to the provider.
type ComponentConfigurationFailureRuntime interface {
	ComponentFailure(ctx context.Context, stack, component string) error
}

// ComponentConfigurationRevisionRuntime supplies isolated discovery for Git comparisons.
// Providers without this capability must fail affected selection rather than reuse HEAD.
type ComponentConfigurationRevisionRuntime interface {
	ForRevision(root string) (ComponentConfigurationRuntime, error)
}

// PreparedComponentConfiguration contains evaluated inputs and literal artifacts.
type PreparedComponentConfiguration struct {
	Sources          []ComponentConfigurationSource
	Engine           string
	Source           string
	Inputs           map[string]any
	BackendType      string
	Backend          map[string]any
	Workspace        string
	Files            []ComponentConfigurationFile
	BackendGenerated bool
	// Execution is data the host runs; it is nil when the provider adds no hooks or arguments.
	Execution *ComponentExecution
}

// ComponentConfigurationFile must bypass template rendering.
type ComponentConfigurationFile struct {
	Path      string
	Content   []byte
	IfExists  string
	Sensitive bool
}

// HasComponentDiscovery reports an explicit opt-in without consulting plugins or the filesystem.
func (c *AtmosConfiguration) HasComponentDiscovery() bool {
	for _, provider := range c.Components.Terraform.ConfigurationProviders {
		if provider.Enabled && provider.Discovery.Enabled {
			return true
		}
	}
	return false
}

// MarshalJSON preserves dynamic provider sections beside built-in Terraform settings.
//
//nolint:gocritic // Value receiver preserves encoding when Terraform is held in an interface.
func (c Terraform) MarshalJSON() ([]byte, error) {
	type plain Terraform
	return marshalConfiguration(plain(c), c.ConfigurationProviders)
}

// MarshalJSON preserves provider-owned settings without adding a host-specific options wrapper.
//
//nolint:gocritic // Value receiver preserves encoding when Terraform is held in an interface.
func (c ComponentConfiguration) MarshalJSON() ([]byte, error) {
	type plain ComponentConfiguration
	return marshalConfiguration(plain(c), c.Options)
}

func marshalConfiguration[T any](base any, extras map[string]T) ([]byte, error) {
	raw, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	for name, value := range extras {
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("%w: configuration key %s", ErrComponentConfigurationCollision, name)
		}
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		result[name] = data
	}
	return json.Marshal(result)
}
