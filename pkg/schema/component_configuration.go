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

// ComponentProviderOption is a provider option the user set explicitly on the command line or in the
// environment. Options the user did not set are absent, so a provider can tell them from false, zero, and empty.
type ComponentProviderOption struct {
	Value any
	// Sensitive values are masked and never logged.
	Sensitive bool
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
	// TerraformBinary is the executable the unit selects, verified against the one it was discovered with.
	TerraformBinary string
	// Exclusion is set when the provider says this operation must not run the unit.
	Exclusion *ComponentExclusion
	// FeatureFlags are the evaluated feature values of the unit, rendered as strings.
	FeatureFlags map[string]string
	// IAM is a role the host assumes for the component's commands; it is nil when the provider asks for none.
	IAM *ComponentIAM
}

// ComponentIAM is an AWS role a configuration provider asks the host to assume. The host assumes it
// on top of the invocation's credentials and exports the resulting session to the component's commands.
type ComponentIAM struct {
	Role            string
	SessionName     string
	DurationSeconds uint32
	// WebIdentityTokenFile holds the token. Saved-plan bindings keep only this path.
	WebIdentityTokenFile string
	// WebIdentityToken is a literal token. It is a secret and is never serialized.
	WebIdentityToken string `json:"-"`
	// WebIdentityTokenLiteral records that the token was a literal value, so a restored record that
	// has lost it can say why instead of assuming the role without a token.
	WebIdentityTokenLiteral bool
}

// ComponentExclusion records that a unit's exclude block prevents the operation from running.
type ComponentExclusion struct {
	// Actions lists the commands the exclusion applies to.
	Actions []string
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
